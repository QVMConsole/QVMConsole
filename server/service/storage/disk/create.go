package disk

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"kvm_console/config"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// AddDisk adds a disk with the default virtio bus.
func AddDisk(vmName string, sizeGB int, format string) (string, error) {
	return AddDiskWithBus(vmName, sizeGB, format, "virtio")
}

// AddDiskWithBus adds a disk with the specified bus type.
// Supported bus values: virtio, scsi, sata, ide.
func AddDiskWithBus(vmName string, sizeGB int, format, bus string) (string, error) {
	return AddDiskWithBusInDir(vmName, sizeGB, format, bus, config.GlobalConfig.CloneDir)
}

// AddDiskWithBusInDir adds a disk to the specified directory (used during VM creation
// to follow the selected storage pool).
func AddDiskWithBusInDir(vmName string, sizeGB int, format, bus, diskDir string) (string, error) {
	if err := EnsureNotMigrating(vmName, "添加磁盘"); err != nil {
		return "", err
	}
	if format == "" {
		format = "qcow2"
	}
	if bus == "" {
		bus = "virtio"
	}
	if strings.TrimSpace(diskDir) == "" {
		diskDir = config.GlobalConfig.CloneDir
	}
	vmState, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	// determine device prefix based on bus type
	devPrefix := GetDevPrefix(bus)

	// find available device name
	existingDisks, _ := ListDisks(vmName)
	usedDevs := make(map[string]bool)
	for _, d := range existingDisks {
		usedDevs[d.Device] = true
	}

	nextDev := ""
	for _, letter := range "bcdefghijklmnop" {
		dev := devPrefix + string(letter)
		if !usedDevs[dev] {
			nextDev = dev
			break
		}
	}
	if nextDev == "" {
		return "", fmt.Errorf("没有可用的设备名")
	}

	if err := os.MkdirAll(diskDir, 0755); err != nil {
		return "", fmt.Errorf("创建磁盘目录失败: %w", err)
	}
	diskPath := fmt.Sprintf("%s/%s-%s.%s", diskDir, vmName, nextDev, format)
	serial := newDiskSerial()

	// create disk image
	result := utils.ExecCommand("qemu-img", "create", "-f", format, diskPath, fmt.Sprintf("%dG", sizeGB))
	if result.Error != nil {
		return "", fmt.Errorf("创建磁盘失败: %s", result.Stderr)
	}

	// use attach-device + XML to support discard and detect_zeroes
	diskXML := fmt.Sprintf(
		"<disk type='file' device='disk'>\n"+
			"  <driver name='qemu' type='%s' discard='unmap' detect_zeroes='unmap'/>\n"+
			"  <source file='%s'/>\n"+
			"  <target dev='%s' bus='%s'/>\n"+
			"  <serial>%s</serial>\n"+
			"</disk>",
		format, diskPath, nextDev, bus, serial)

	// for running VMs on q35/PCIe, fill PCI address to use a free pcie-root-port
	if vmState == "running" {
		hotplugXML, err := buildDiskHotplugXML(vmName, diskXML)
		if err != nil {
			// PCIe slots exhausted, try fallback to scsi (needs existing virtio-scsi controller)
			if bus == "virtio" && strings.Contains(err.Error(), ErrNoPCIESlots.Error()) {
				scsiDev, scsiErr := tryFallbackDiskToSCSI(vmName, diskPath, format, serial, existingDisks, vmState)
				if scsiErr == nil {
					return scsiDev, nil
				}
			}
			_ = os.Remove(diskPath)
			return "", err
		}
		diskXML = hotplugXML
	}

	// attach-device: running=live+config(3), stopped=config(2)
	var attachFlags uint32 = 2 // VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	if vmState == "running" {
		attachFlags = 3 // VIR_DOMAIN_DEVICE_MODIFY_LIVE | VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	}
	if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, diskXML, attachFlags); err != nil {
		_ = os.Remove(diskPath)
		return "", fmt.Errorf("挂载磁盘失败: %w", err)
	}

	return nextDev, nil
}

// AddExtraDisksForVM creates and attaches extra data disks according to the config.
func AddExtraDisksForVM(vmName string, disks []ExtraDiskParam, defaultDir, defaultBus string, isAdmin bool, progressFn func(int, string)) error {
	if len(disks) == 0 {
		return nil
	}
	if progressFn == nil {
		progressFn = func(int, string) {}
	}
	for i, disk := range disks {
		if disk.Size <= 0 {
			continue
		}
		format := strings.TrimSpace(disk.Format)
		if format == "" {
			format = "qcow2"
		}
		if !isAdmin {
			format = "qcow2"
		}
		bus := NormalizeVMDiskBus(disk.Bus)
		if bus == "" {
			bus = NormalizeVMDiskBus(defaultBus)
		}
		if bus == "" {
			bus = "virtio"
		}
		diskDir := strings.TrimSpace(defaultDir)
		if strings.TrimSpace(disk.StoragePoolID) != "" {
			resolvedDir, _, err := ResolveStorageDir(disk.StoragePoolID, isAdmin)
			if err != nil {
				return fmt.Errorf("解析额外磁盘 %d 存储位置失败: %w", i+1, err)
			}
			diskDir = resolvedDir
		}
		progressFn(0, fmt.Sprintf("正在挂载额外磁盘 %d...", i+1))
		if _, err := AddDiskWithBusInDir(vmName, disk.Size, format, bus, diskDir); err != nil {
			return fmt.Errorf("挂载额外磁盘 %d 失败: %w", i+1, err)
		}
	}
	return nil
}

// AttachExistingDisk attaches an existing disk file to a VM.
func AttachExistingDisk(vmName, diskPath, bus string) (string, error) {
	if err := EnsureNotMigrating(vmName, "挂载磁盘"); err != nil {
		return "", err
	}
	if bus == "" {
		bus = "virtio"
	}

	// check file exists
	if !utils.FileExists(diskPath) {
		return "", fmt.Errorf("磁盘文件不存在: %s", diskPath)
	}

	// check file readability (ensure libvirt has permissions)
	if !utils.FileReadable(diskPath) {
		return "", fmt.Errorf("磁盘文件不可读（权限不足）: %s，请确保文件归 libvirt-qemu:kvm 所有", diskPath)
	}

	// if file is in user storage directory, move it to default disk directory
	storageMountPoint := GetStorageMountPointFn()
	if storageMountPoint != "" && strings.HasPrefix(diskPath, storageMountPoint) {
		destDir := config.GlobalConfig.CloneDir
		filename := filepath.Base(diskPath)
		destPath := filepath.Join(destDir, filename)

		// check if target already exists, add timestamp to avoid conflict
		destCheck := utils.ExecShell(fmt.Sprintf("test -f %s && echo exists", utils.ShellSingleQuote(destPath)))
		if strings.TrimSpace(destCheck.Stdout) == "exists" {
			ts := time.Now().Format("20060102150405")
			ext := filepath.Ext(filename)
			nameOnly := strings.TrimSuffix(filename, ext)
			destPath = filepath.Join(destDir, fmt.Sprintf("%s_%s%s", nameOnly, ts, ext))
		}

		// move file
		if err := moveDiskFile(diskPath, destPath); err != nil {
			return "", fmt.Errorf("移动磁盘文件到默认目录失败: %v", err)
		}
		// set permissions
		_ = utils.ChownLibvirtQEMU(destPath)
		diskPath = destPath
	}

	vmState, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	// detect disk format
	format := "qcow2"
	infoResult := utils.ExecCommand("qemu-img", "info", "--output=json", diskPath)
	if infoResult.Error == nil {
		detected := ParseQemuInfoStr(infoResult.Stdout, "format")
		if detected != "" {
			format = detected
		}
	}

	// determine device prefix based on bus type
	devPrefix := GetDevPrefix(bus)

	// find available device name
	existingDisks, _ := ListDisks(vmName)
	usedDevs := make(map[string]bool)
	for _, d := range existingDisks {
		usedDevs[d.Device] = true
	}

	nextDev := ""
	for _, letter := range "bcdefghijklmnop" {
		dev := devPrefix + string(letter)
		if !usedDevs[dev] {
			nextDev = dev
			break
		}
	}
	if nextDev == "" {
		return "", fmt.Errorf("没有可用的设备名")
	}
	serial := newDiskSerial()

	// use attach-device + XML to support discard and detect_zeroes
	diskXML := fmt.Sprintf(
		"<disk type='file' device='disk'>\n"+
			"  <driver name='qemu' type='%s' discard='unmap' detect_zeroes='unmap'/>\n"+
			"  <source file='%s'/>\n"+
			"  <target dev='%s' bus='%s'/>\n"+
			"  <serial>%s</serial>\n"+
			"</disk>",
		format, diskPath, nextDev, bus, serial)

	// for running VMs on q35/PCIe, fill PCI address to use a free pcie-root-port
	if vmState == "running" {
		hotplugXML, err := buildDiskHotplugXML(vmName, diskXML)
		if err != nil {
			// PCIe slots exhausted, try fallback to scsi (needs existing virtio-scsi controller)
			if bus == "virtio" && strings.Contains(err.Error(), ErrNoPCIESlots.Error()) {
				scsiDev, scsiErr := tryFallbackDiskToSCSI(vmName, diskPath, format, serial, existingDisks, vmState)
				if scsiErr == nil {
					return scsiDev, nil
				}
			}
			return "", err
		}
		diskXML = hotplugXML
	}

	// attach-device: running=live+config(3), stopped=config(2)
	var attachFlags uint32 = 2 // VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	if vmState == "running" {
		attachFlags = 3 // VIR_DOMAIN_DEVICE_MODIFY_LIVE | VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	}
	if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, diskXML, attachFlags); err != nil {
		return "", fmt.Errorf("挂载磁盘失败: %w", err)
	}

	return nextDev, nil
}

func moveDiskFile(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	// 跨文件系统复制磁盘属于大 IO 操作，不设置自动超时
	copyResult := utils.ExecCommandNoTimeout("cp", "--sparse=always", "--preserve=mode,timestamps", "--", source, destination)
	if copyResult.Error != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("跨文件系统复制磁盘失败: %s", strings.TrimSpace(copyResult.Stderr))
	}
	if err := os.Remove(source); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

// buildDiskHotplugXML fills PCI address for hot-adding disks on q35/PCIe machines.
// When the VM is a q35/PCIe machine, disks must be attached to a free pcie-root-port,
// otherwise libvirt reports "No more available PCI slots".
// When PCIe slots are exhausted, returns ErrNoPCIESlots for caller fallback.
func buildDiskHotplugXML(vmName string, diskXML string) (string, error) {
	xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		// cannot get XML, return original and let libvirt assign
		return diskXML, nil
	}
	if !hasPCIERootController(xmlStr) {
		// non-PCIe machine (i440fx etc.), no manual PCI address needed
		return diskXML, nil
	}

	freeBus, err := findFreePCIERootPortBus(vmName)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoPCIESlots, err)
	}

	// insert PCI address before </disk>
	addrLine := fmt.Sprintf("  <address type='pci' domain='0x0000' bus='0x%02x' slot='0x00' function='0x0'/>", freeBus)
	diskXML = strings.Replace(diskXML, "</disk>", addrLine+"\n</disk>", 1)
	return diskXML, nil
}

// tryFallbackDiskToSCSI falls back to scsi bus when virtio disk hot-add fails
// due to exhausted PCIe slots. It uses the existing virtio-scsi controller.
// Returns the new device name, or an error.
func tryFallbackDiskToSCSI(vmName, diskPath, format, serial string, existingDisks []DiskInfo, vmState string) (string, error) {
	// 1. confirm VM has virtio-scsi controller
	xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil || !hasSCSIController(xmlStr) {
		// try creating a virtio-scsi controller (needs PCIe slot, usually fails too)
		if err := ensureHotplugCDROMController(vmName); err != nil {
			return "", fmt.Errorf("PCIe 插槽已满且无可用的 virtio-scsi 控制器（尝试创建也失败: %v）。请先关机后再添加磁盘", err)
		}
	}

	// 2. compute scsi device name
	usedDevs := make(map[string]bool)
	for _, d := range existingDisks {
		usedDevs[d.Device] = true
	}
	nextDev := ""
	for _, letter := range "abcdefghijklmnop" {
		dev := "sd" + string(letter)
		if !usedDevs[dev] {
			nextDev = dev
			break
		}
	}
	if nextDev == "" {
		return "", fmt.Errorf("没有可用的 scsi 设备名")
	}

	// 3. build scsi bus XML (no PCIe address needed)
	diskXML := fmt.Sprintf(
		"<disk type='file' device='disk'>\n"+
			"  <driver name='qemu' type='%s' discard='unmap' detect_zeroes='unmap'/>\n"+
			"  <source file='%s'/>\n"+
			"  <target dev='%s' bus='scsi'/>\n"+
			"  <serial>%s</serial>\n"+
			"</disk>",
		format, diskPath, nextDev, serial)

	// attach-device: running=live+config(3), stopped=config(2)
	var attachFlags uint32 = 2 // VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	if vmState == "running" {
		attachFlags = 3 // VIR_DOMAIN_DEVICE_MODIFY_LIVE | VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	}
	if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, diskXML, attachFlags); err != nil {
		return "", fmt.Errorf("scsi 降级挂载失败: %w", err)
	}

	return nextDev, nil
}

func newDiskSerial() string {
	data := make([]byte, 10)
	if _, err := rand.Read(data); err == nil {
		return "qvm-" + hex.EncodeToString(data)
	}
	return fmt.Sprintf("qvm-%d", time.Now().UnixNano())
}

// GetDevPrefix returns the device name prefix based on bus type.
func GetDevPrefix(bus string) string {
	switch bus {
	case "virtio":
		return "vd"
	case "scsi", "sata", "usb":
		return "sd"
	case "ide":
		return "hd"
	default:
		return "vd"
	}
}
