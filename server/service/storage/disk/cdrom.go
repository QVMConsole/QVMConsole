package disk

import (
	"fmt"
	"strings"

	"kvm_console/service/arch"
	"kvm_console/service/libvirt_rpc"

	"github.com/digitalocean/go-libvirt"
)

// ChangeCDROM changes/inserts a CD/DVD disc.
// forceNew: when true, force-add a new CDROM device instead of replacing existing.
// requestedBus: optional bus used only when a new CDROM device is created.
func ChangeCDROM(vmName, isoPath, device string, forceNew bool, requestedBus ...string) error {
	if err := EnsureNotMigrating(vmName, "更换光盘"); err != nil {
		return err
	}
	vmState, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	// force new mode: add a new CDROM device directly
	if forceNew {
		return attachNewCDROM(vmName, isoPath, vmState, firstRequestedCDROMBus(requestedBus))
	}

	if device == "" {
		// auto-find cdrom device
		device = findCDROMDevice(vmName)
		if device == "" {
			// no existing cdrom device, add a new one
			return attachNewCDROM(vmName, isoPath, vmState, firstRequestedCDROMBus(requestedBus))
		}
	}

	// 获取当前 CDROM 设备的实际总线类型
	cdromBus := getExistingCDROMBus(vmName, device)
	if cdromBus == "" {
		cdromBus = arch.GetProfile(arch.DetectHostArch()).GetCDROMBus()
	}

	// change CDROM media via libvirt_rpc.AttachDeviceFlagsRPC
	cdromXML := fmt.Sprintf(
		"<disk type='file' device='cdrom'>\n"+
			"  <driver name='qemu' type='raw'/>\n"+
			"  <source file='%s'/>\n"+
			"  <target dev='%s' bus='%s'/>\n"+
			"</disk>", isoPath, device, cdromBus)

	var attachFlags uint32 = 2 // VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	if vmState == "running" {
		attachFlags = 3 // VIR_DOMAIN_DEVICE_MODIFY_LIVE | VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	}
	if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, cdromXML, attachFlags); err != nil {
		return fmt.Errorf("插入光盘失败: %w", err)
	}
	return nil
}

// EjectCDROM ejects a CD/DVD disc (keeps the device).
func EjectCDROM(vmName, device string) error {
	if err := EnsureNotMigrating(vmName, "弹出光盘"); err != nil {
		return err
	}
	vmState, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	if device == "" {
		device = findCDROMDevice(vmName)
		if device == "" {
			return fmt.Errorf("未找到 CD/DVD 设备")
		}
	}

	// 获取当前 CDROM 设备的实际总线类型
	ejectBus := getExistingCDROMBus(vmName, device)
	if ejectBus == "" {
		ejectBus = arch.GetProfile(arch.DetectHostArch()).GetCDROMBus()
	}

	// eject CDROM media (leave source empty)
	cdromXML := fmt.Sprintf(
		"<disk type='file' device='cdrom'>\n"+
			"  <driver name='qemu' type='raw'/>\n"+
			"  <target dev='%s' bus='%s'/>\n"+
			"</disk>", device, ejectBus)

	var attachFlags uint32 = 2 // VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	if vmState == "running" {
		attachFlags = 3 // VIR_DOMAIN_DEVICE_MODIFY_LIVE | VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	}
	if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, cdromXML, attachFlags); err != nil {
		// ignore "no media" errors
		if strings.Contains(err.Error(), "doesn't have media") ||
			strings.Contains(err.Error(), "is not removable") {
			return nil
		}
		return fmt.Errorf("弹出光盘失败: %w", err)
	}
	return nil
}

// RemoveCDROM completely removes a CD/DVD device by editing XML.
func RemoveCDROM(vmName, device string) error {
	if err := EnsureNotMigrating(vmName, "移除光驱"); err != nil {
		return err
	}
	vmState, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	if device == "" {
		device = findCDROMDevice(vmName)
		if device == "" {
			return fmt.Errorf("未找到 CD/DVD 设备")
		}
	}

	// running VMs cannot directly detach cdrom device, must edit XML
	if vmState == "running" {
		// eject media first
		EjectCDROM(vmName, device)
	}

	// get current XML
	xmlResult, err := libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
	if err != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %w", err)
	}

	// remove the cdrom disk node from XML
	xmlStr := xmlResult
	lines := strings.Split(xmlStr, "\n")
	var newLines []string
	var diskBuffer []string
	inCdromDisk := false
	isCdrom := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// detect cdrom disk block start
		if strings.Contains(trimmed, "<disk ") && strings.Contains(trimmed, "device='cdrom'") {
			inCdromDisk = true
			isCdrom = false
			diskBuffer = []string{line}
			continue
		}

		if inCdromDisk {
			diskBuffer = append(diskBuffer, line)
			// check if it contains target device name
			if strings.Contains(trimmed, "<target") && strings.Contains(trimmed, "dev='"+device+"'") {
				isCdrom = true
			}
			// if reached </disk>, decide whether to keep
			if strings.Contains(trimmed, "</disk>") {
				inCdromDisk = false
				if isCdrom {
					// this is the cdrom node to delete, discard buffer
				} else {
					// keep this disk node
					newLines = append(newLines, diskBuffer...)
				}
				diskBuffer = nil
			}
			continue
		}

		newLines = append(newLines, line)
	}

	newXML := strings.Join(newLines, "\n")
	if _, err := libvirt_rpc.DefineDomainXMLRPC(newXML); err != nil {
		return fmt.Errorf("移除光驱失败: %w", err)
	}

	return nil
}

// findCDROMDevice finds the first cdrom device name of a VM.
func findCDROMDevice(vmName string) string {
	devices := findAllCDROMDevices(vmName)
	if len(devices) > 0 {
		return devices[0]
	}
	return ""
}

// findAllCDROMDevices finds all cdrom device names of a VM.
func findAllCDROMDevices(vmName string) []string {
	xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return nil
	}

	var devices []string
	lines := strings.Split(xmlStr, "\n")
	inCdrom := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "device='cdrom'") {
			inCdrom = true
		}
		if inCdrom && strings.Contains(trimmed, "<target") {
			parts := strings.Split(trimmed, "dev='")
			if len(parts) > 1 {
				dev := strings.Split(parts[1], "'")[0]
				devices = append(devices, dev)
			}
		}
		if inCdrom && strings.Contains(trimmed, "</disk>") {
			inCdrom = false
		}
	}
	return devices
}

// attachNewCDROM attaches a new cdrom device.
func attachNewCDROM(vmName, isoPath, vmState, requestedBus string) error {
	existingDisks, _ := ListDisks(vmName)
	bus, err := selectNewCDROMBus(vmState, existingDisks, requestedBus)
	if err != nil {
		return err
	}
	if err := validateCDROMBusForVM(vmName, bus); err != nil {
		return err
	}
	if vmState == "running" {
		if err := ensureHotplugCDROMController(vmName); err != nil {
			return err
		}
	}

	usedDevs := make(map[string]bool)
	for _, d := range existingDisks {
		usedDevs[d.Device] = true
	}

	devPrefix := GetDevPrefix(bus)
	nextDev := ""
	for _, letter := range "abcdefghijklmnop" {
		dev := devPrefix + string(letter)
		if !usedDevs[dev] {
			nextDev = dev
			break
		}
	}
	if nextDev == "" {
		return fmt.Errorf("没有可用的 %s 光驱设备名", strings.ToUpper(bus))
	}

	// add CDROM device via libvirt_rpc.AttachDeviceFlagsRPC
	cdromXML := fmt.Sprintf(
		"<disk type='file' device='cdrom'>\n"+
			"  <driver name='qemu' type='raw'/>\n"+
			"  <source file='%s'/>\n"+
			"  <target dev='%s' bus='%s'/>\n"+
			"  <readonly/>\n"+
			"</disk>", isoPath, nextDev, bus)

	var attachFlags uint32 = 2 // VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	if vmState == "running" {
		attachFlags = 3 // VIR_DOMAIN_DEVICE_MODIFY_LIVE | VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	}
	if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, cdromXML, attachFlags); err != nil {
		if vmState == "running" && strings.Contains(err.Error(), "cannot be hotplugged") {
			return fmt.Errorf("当前虚拟机不支持通过 %s 总线热添加光驱，请先关机后再添加", strings.ToUpper(bus))
		}
		return fmt.Errorf("添加光驱失败: %w", err)
	}
	return nil
}

func validateCDROMBusForVM(vmName, bus string) error {
	if bus != "ide" {
		return nil
	}
	xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
	if err != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %w", err)
	}
	if strings.Contains(domainMachineType(xmlStr), "q35") {
		return fmt.Errorf("当前 Q35 机型不支持 IDE 光驱，请使用 SCSI、SATA 或 USB")
	}
	return nil
}

// ensureHotplugCDROMController ensures the running VM has a controller for hot-adding CDROM.
func ensureHotplugCDROMController(vmName string) error {
	liveXMLStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return fmt.Errorf("获取运行中虚拟机 XML 失败: %w", err)
	}
	configXMLStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
	if err != nil {
		return fmt.Errorf("获取持久化虚拟机 XML 失败: %w", err)
	}

	hasLiveController := hasSCSIController(liveXMLStr)
	hasConfigController := hasSCSIController(configXMLStr)
	if hasLiveController && hasConfigController {
		return nil
	}

	controllerXML, buildErr := buildHotplugSCSIControllerXML(vmName, liveXMLStr)
	if buildErr != nil {
		return buildErr
	}

	if !hasLiveController {
		if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, controllerXML, 1); err != nil { // 1=VIR_DOMAIN_DEVICE_MODIFY_LIVE
			if !strings.Contains(err.Error(), "Duplicate ID") && !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("为热添加光驱准备 SCSI 控制器失败: %w", err)
			}
		}
	}

	if !hasConfigController {
		if err := libvirt_rpc.AttachDeviceFlagsRPC(vmName, controllerXML, 2); err != nil { // 2=VIR_DOMAIN_DEVICE_MODIFY_CONFIG
			if !strings.Contains(err.Error(), "Duplicate ID") && !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("为持久化配置写入 SCSI 控制器失败: %w", err)
			}
		}
	}

	return nil
}

func firstRequestedCDROMBus(requestedBus []string) string {
	if len(requestedBus) == 0 {
		return ""
	}
	return requestedBus[0]
}

func normalizeCDROMBus(bus string) (string, error) {
	bus = strings.ToLower(strings.TrimSpace(bus))
	if bus == "" {
		return "", nil
	}
	switch bus {
	case "scsi", "sata", "ide", "usb":
		return bus, nil
	default:
		return "", fmt.Errorf("不支持的光驱驱动类型: %s", bus)
	}
}

func selectNewCDROMBus(vmState string, existingDisks []DiskInfo, requestedBus string) (string, error) {
	if vmState == "running" {
		return "scsi", nil
	}
	bus, err := normalizeCDROMBus(requestedBus)
	if err != nil {
		return "", err
	}
	if bus != "" {
		return bus, nil
	}
	for _, disk := range existingDisks {
		if disk.DeviceType == "cdrom" && strings.TrimSpace(disk.Bus) != "" {
			return strings.TrimSpace(disk.Bus), nil
		}
	}
	return arch.GetProfile(arch.DetectHostArch()).GetCDROMBus(), nil
}

// getExistingCDROMBus 从域 XML 中获取指定 CDROM 设备的实际总线类型
func getExistingCDROMBus(vmName, devName string) string {
	xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return ""
	}
	lines := strings.Split(xmlStr, "\n")
	inCdrom := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "device='cdrom'") {
			inCdrom = true
		}
		if inCdrom && strings.Contains(trimmed, "<target") && strings.Contains(trimmed, "dev='"+devName+"'") {
			// extract bus attribute
			parts := strings.Split(trimmed, "bus='")
			if len(parts) > 1 {
				bus := strings.Split(parts[1], "'")[0]
				return bus
			}
		}
		if inCdrom && strings.Contains(trimmed, "</disk>") {
			inCdrom = false
		}
	}
	return ""
}
