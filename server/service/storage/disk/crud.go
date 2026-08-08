package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/service/guest_agent"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"

	"github.com/digitalocean/go-libvirt"
)

// DiskInfo holds information about a virtual machine disk.
type DiskInfo struct {
	Device             string `json:"device"`                 // device name (e.g. vda, vdb)
	Path               string `json:"path"`                   // disk file path
	CapacityGB         string `json:"capacity_gb"`            // capacity (GB)
	UsedGB             string `json:"used_gb"`                // used (GB)
	Bus                string `json:"bus"`                    // bus type
	Format             string `json:"format"`                 // disk format qcow2/raw
	DeviceType         string `json:"device_type"`            // disk/cdrom
	HotSupport         bool   `json:"hot_support"`            // supports hot operations
	IsSystem           bool   `json:"is_system"`              // 是否为系统盘
	Serial             string `json:"serial"`                 // 稳定磁盘序列号
	GuestDevice        string `json:"guest_device,omitempty"` // 来宾中的设备路径
	GuestMappingStatus string `json:"guest_mapping_status"`   // mapped/unavailable/unmapped
	// IOPS limits (0 = unlimited)
	IOPSTotal IOPSField `json:"iops_total"` // total IOPS limit
	IOPSRead  IOPSField `json:"iops_read"`  // read IOPS limit
	IOPSWrite IOPSField `json:"iops_write"` // write IOPS limit
}

// IOPSField represents an optional IOPS value.
type IOPSField struct {
	Value int  `json:"value"`
	IsSet bool `json:"is_set"`
}

// DiskSimpleInfo holds brief disk information (for delete confirmation UI).
type DiskSimpleInfo struct {
	Device     string `json:"device"`      // device name
	Path       string `json:"path"`        // disk file path
	CapacityGB string `json:"capacity_gb"` // capacity (GB)
	Format     string `json:"format"`      // disk format
	IsSystem   bool   `json:"is_system"`   // whether this is the system disk (first disk)
	SizeBytes  int64  `json:"size_bytes"`  // actual file size in bytes
}

// diskXMLInfo holds extra disk information extracted from XML.
type diskXMLInfo struct {
	Format        string
	DeviceType    string
	Bus           string
	Serial        string
	PCIBus        int
	PCISlot       int
	PCIFunction   int
	HasPCIAddress bool
}

// ErrNoPCIESlots is returned when PCIe slots are exhausted, triggering SCSI fallback.
var ErrNoPCIESlots = fmt.Errorf("no_pcie_slots")

// ListDisks lists all disks of a virtual machine.
func ListDisks(vmName string) ([]DiskInfo, error) {
	state, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	domainXML, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return nil, fmt.Errorf("获取磁盘列表失败: %w", err)
	}

	// parse block device list from XML (replaces virsh domblklist)
	blkList := libvirt_rpc.ParseDisksFromDomainXML(domainXML)

	// get detailed info for each disk from XML (format, device type, bus, IOPS)
	diskXMLMap := parseDiskXMLInfo(vmName)
	diskIOPSMap := ParseAllDiskIOPSTune(vmName)

	var disks []DiskInfo
	systemDiskDev := ""

	for _, blk := range blkList {
		device := blk.Target
		path := blk.Source

		// skip devices without target
		if device == "" {
			continue
		}

		disk := DiskInfo{
			Device: device,
			Path:   path,
		}

		// get info from XML
		if xmlInfo, ok := diskXMLMap[disk.Device]; ok {
			disk.Format = xmlInfo.Format
			disk.DeviceType = xmlInfo.DeviceType
			disk.Bus = xmlInfo.Bus
			disk.Serial = xmlInfo.Serial
		}

		// skip disks with empty or "-" source (but keep empty CDROMs)
		if (path == "" || path == "-") && disk.DeviceType != "cdrom" {
			continue
		}
		// clean up empty CDROM path
		if path == "-" {
			disk.Path = ""
		}

		disk.HotSupport = disk.Bus == "virtio" || disk.Bus == "scsi"
		if disk.DeviceType == "disk" && systemDiskDev == "" {
			systemDiskDev = disk.Device
		}
		disk.IsSystem = disk.Device == systemDiskDev
		disk.GuestMappingStatus = "unavailable"

		// capacity and usage
		if state == "running" && disk.Path != "" {
			capVal, allocVal, _, blkErr := libvirt_rpc.GetBlockInfoRPC(vmName, disk.Device)
			if blkErr == nil {
				disk.CapacityGB = fmt.Sprintf("%.2f", float64(capVal)/1024/1024/1024)
				disk.UsedGB = fmt.Sprintf("%.2f", float64(allocVal)/1024/1024/1024)
			}
		} else if disk.Path != "" {
			// offline: use qemu-img info
			qemuInfo := utils.ExecShell(fmt.Sprintf("qemu-img info --output=json -U %s 2>/dev/null", utils.ShellSingleQuote(disk.Path)))
			if qemuInfo.Error == nil {
				disk.CapacityGB = ParseQemuInfoGB(qemuInfo.Stdout, "virtual-size")
				disk.UsedGB = ParseQemuInfoGB(qemuInfo.Stdout, "actual-size")
				// if format is still empty, get it from qemu-img info
				if disk.Format == "" {
					disk.Format = ParseQemuInfoStr(qemuInfo.Stdout, "format")
				}
			}
		}

		// fill IOPS limits
		if iops, ok := diskIOPSMap[disk.Device]; ok {
			disk.IOPSTotal = IOPSField{Value: iops.TotalIopsSec, IsSet: true}
			disk.IOPSRead = IOPSField{Value: iops.ReadIopsSec, IsSet: true}
			disk.IOPSWrite = IOPSField{Value: iops.WriteIopsSec, IsSet: true}
		}

		disks = append(disks, disk)
	}

	if state == "running" {
		ctx, cancel := context.WithTimeout(context.Background(), guest_agent.ConnectTimeout)
		guestDisks, guestErr := guest_agent.NewClient(vmName).Disks(ctx)
		cancel()
		if guestErr == nil {
			for i := range disks {
				if disks[i].DeviceType != "disk" {
					continue
				}
				disks[i].GuestMappingStatus = "unmapped"
				xmlInfo := diskXMLMap[disks[i].Device]
				for _, guestDisk := range guestDisks {
					if guestDisk.Partition || guestDisk.Address == nil {
						continue
					}
					serialMatch := xmlInfo.Serial != "" && strings.EqualFold(xmlInfo.Serial, guestDisk.Address.Serial)
					pciMatch := xmlInfo.HasPCIAddress && guestDisk.Address.PCIDevice != nil &&
						xmlInfo.PCIBus == guestDisk.Address.PCIDevice.Bus && xmlInfo.PCISlot == guestDisk.Address.PCIDevice.Slot &&
						xmlInfo.PCIFunction == guestDisk.Address.PCIDevice.Function
					if serialMatch || pciMatch {
						disks[i].GuestDevice = guestDisk.Name
						disks[i].GuestMappingStatus = "mapped"
						break
					}
				}
			}
		}
	}

	return disks, nil
}

// parseDiskXMLInfo parses format, device type, and bus info from VM XML.
func parseDiskXMLInfo(vmName string) map[string]diskXMLInfo {
	result := make(map[string]diskXMLInfo)

	xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return result
	}

	lines := strings.Split(xmlStr, "\n")
	var currentDev string
	var currentInfo diskXMLInfo
	inDisk := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// find <disk ... device='xxx'>
		if strings.HasPrefix(trimmed, "<disk ") {
			inDisk = true
			currentInfo = diskXMLInfo{}
			if strings.Contains(trimmed, "device='disk'") {
				currentInfo.DeviceType = "disk"
			} else if strings.Contains(trimmed, "device='cdrom'") {
				currentInfo.DeviceType = "cdrom"
			}
		}

		if inDisk {
			if strings.HasPrefix(trimmed, "<serial>") && strings.Contains(trimmed, "</serial>") {
				currentInfo.Serial = strings.TrimSuffix(strings.TrimPrefix(trimmed, "<serial>"), "</serial>")
			}
			// <driver ... type='qcow2'/>
			if strings.Contains(trimmed, "<driver") && strings.Contains(trimmed, "type='") {
				parts := strings.Split(trimmed, "type='")
				if len(parts) > 1 {
					currentInfo.Format = strings.Split(parts[1], "'")[0]
				}
			}
			// <target dev='vda' bus='virtio'/>
			if strings.Contains(trimmed, "<target") {
				if strings.Contains(trimmed, "dev='") {
					parts := strings.Split(trimmed, "dev='")
					if len(parts) > 1 {
						currentDev = strings.Split(parts[1], "'")[0]
					}
				}
				if strings.Contains(trimmed, "bus='") {
					parts := strings.Split(trimmed, "bus='")
					if len(parts) > 1 {
						currentInfo.Bus = strings.Split(parts[1], "'")[0]
					}
				}
			}
			if strings.Contains(trimmed, "<address") && strings.Contains(trimmed, "type='pci'") {
				if bus, ok := parseXMLHexAttribute(trimmed, "bus"); ok {
					currentInfo.PCIBus = bus
					currentInfo.HasPCIAddress = true
				}
				currentInfo.PCISlot, _ = parseXMLHexAttribute(trimmed, "slot")
				currentInfo.PCIFunction, _ = parseXMLHexAttribute(trimmed, "function")
			}
			if strings.Contains(trimmed, "</disk>") {
				if currentDev != "" {
					result[currentDev] = currentInfo
				}
				inDisk = false
				currentDev = ""
			}
		}
	}

	return result
}

func parseXMLHexAttribute(line, name string) (int, bool) {
	needle := name + "='"
	parts := strings.SplitN(line, needle, 2)
	if len(parts) != 2 {
		return 0, false
	}
	value := strings.SplitN(parts[1], "'", 2)[0]
	parsed, err := strconv.ParseInt(strings.TrimPrefix(value, "0x"), 16, 32)
	return int(parsed), err == nil
}

// ParseQemuInfoStr parses a string value from qemu-img info JSON (top-level field only).
func ParseQemuInfoStr(output, key string) string {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return ""
	}
	raw, ok := data[key]
	if !ok {
		return ""
	}
	var val string
	if err := json.Unmarshal(raw, &val); err != nil {
		return ""
	}
	return val
}

// ParseQemuInfoGB parses a capacity value from qemu-img info JSON (top-level field only,
// avoiding interference from same-named fields in children).
func ParseQemuInfoGB(output, key string) string {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return "-"
	}
	raw, ok := data[key]
	if !ok {
		return "-"
	}
	var bytes int64
	if err := json.Unmarshal(raw, &bytes); err != nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", float64(bytes)/1024/1024/1024)
}

// xmlAttributeValue 读取 libvirt XML 行中的属性，兼容单引号和双引号。
func xmlAttributeValue(line, name string) (string, bool) {
	for _, quote := range []byte{'\'', '"'} {
		needle := name + "=" + string(quote)
		start := strings.Index(line, needle)
		if start < 0 {
			continue
		}
		start += len(needle)
		end := strings.IndexByte(line[start:], quote)
		if end >= 0 {
			return line[start : start+end], true
		}
	}
	return "", false
}

func decimalXMLAttribute(line, name string) (int, bool) {
	value, ok := xmlAttributeValue(line, name)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

// driveAddressTargetDev 将 drive 地址换算为无显式地址时 libvirt 会使用的设备名。
// SATA/SCSI 默认按 unit 排列，IDE 则按 bus 与 unit 的组合排列。
func driveAddressTargetDev(bus, prefix, addressLine string) string {
	if addressLine == "" {
		return ""
	}
	addressType, _ := xmlAttributeValue(addressLine, "type")
	controller, controllerOK := decimalXMLAttribute(addressLine, "controller")
	addressBus, busOK := decimalXMLAttribute(addressLine, "bus")
	target, targetOK := decimalXMLAttribute(addressLine, "target")
	unit, unitOK := decimalXMLAttribute(addressLine, "unit")
	if addressType != "drive" || !controllerOK || !busOK || !targetOK || !unitOK || controller != 0 || target != 0 {
		return ""
	}

	index := -1
	switch bus {
	case "sata", "scsi":
		if addressBus == 0 {
			index = unit
		}
	case "ide":
		if addressBus >= 0 && addressBus <= 1 && unit >= 0 && unit <= 1 {
			index = addressBus*2 + unit
		}
	}
	if index < 0 || index >= 26 {
		return ""
	}
	return prefix + string(rune('a'+index))
}

// planDiskTargetDevice 同时避让设备名与目标总线上的实际 drive 地址。
// 某些历史 XML 中 target 名称与 unit 并不一致，仅检查 sda/sdb 会漏掉地址冲突。
func planDiskTargetDevice(xmlStr, device, newBus string) (string, error) {
	if len(device) < 3 {
		return "", fmt.Errorf("无效的磁盘设备名: %s", device)
	}

	prefix := GetDevPrefix(newBus)
	reserved := make(map[string]bool)
	lines := strings.Split(xmlStr, "\n")
	inDisk := false
	targetDev := ""
	targetBus := ""
	addressLine := ""
	foundTarget := false

	flushDisk := func() {
		if targetDev == "" {
			return
		}
		if targetDev == device {
			foundTarget = true
			return
		}
		reserved[targetDev] = true
		if targetBus == newBus {
			if impliedDev := driveAddressTargetDev(newBus, prefix, addressLine); impliedDev != "" {
				reserved[impliedDev] = true
			}
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<disk ") {
			inDisk = true
			targetDev = ""
			targetBus = ""
			addressLine = ""
			continue
		}
		if !inDisk {
			continue
		}
		if strings.HasPrefix(trimmed, "<target ") {
			targetDev, _ = xmlAttributeValue(trimmed, "dev")
			targetBus, _ = xmlAttributeValue(trimmed, "bus")
		}
		if strings.HasPrefix(trimmed, "<address ") {
			addressLine = trimmed
		}
		if strings.Contains(trimmed, "</disk>") {
			flushDisk()
			inDisk = false
		}
	}

	if !foundTarget {
		return "", fmt.Errorf("未找到设备 %s", device)
	}

	preferred := prefix + device[2:]
	if !reserved[preferred] {
		return preferred, nil
	}
	for letter := 'a'; letter <= 'z'; letter++ {
		candidate := prefix + string(letter)
		if !reserved[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("没有可用的设备名（所有 %s* 均已被占用）", prefix)
}

func rewriteDiskBusXML(xmlStr, device, newDev, newBus string) (string, bool) {
	lines := strings.Split(xmlStr, "\n")
	result := make([]string, 0, len(lines))
	for index := 0; index < len(lines); {
		trimmed := strings.TrimSpace(lines[index])
		if !strings.HasPrefix(trimmed, "<disk ") {
			result = append(result, lines[index])
			index++
			continue
		}

		end := index
		for end < len(lines) && !strings.Contains(strings.TrimSpace(lines[end]), "</disk>") {
			end++
		}
		if end >= len(lines) {
			result = append(result, lines[index:]...)
			break
		}

		block := lines[index : end+1]
		isTargetDisk := false
		for _, line := range block {
			lineTrimmed := strings.TrimSpace(line)
			if strings.HasPrefix(lineTrimmed, "<target ") {
				targetDev, _ := xmlAttributeValue(lineTrimmed, "dev")
				if targetDev == device {
					isTargetDisk = true
					break
				}
			}
		}
		if !isTargetDisk {
			result = append(result, block...)
			index = end + 1
			continue
		}

		for _, line := range block {
			lineTrimmed := strings.TrimSpace(line)
			if strings.HasPrefix(lineTrimmed, "<address ") {
				continue
			}
			if strings.HasPrefix(lineTrimmed, "<target ") {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				line = fmt.Sprintf("%s<target dev='%s' bus='%s'/>", indent, newDev, newBus)
			}
			result = append(result, line)
		}
		index = end + 1
		return strings.Join(append(result, lines[index:]...), "\n"), true
	}
	return strings.Join(result, "\n"), false
}

func blockDeviceType(xmlStr, device string) string {
	lines := strings.Split(xmlStr, "\n")
	for index := 0; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if !strings.HasPrefix(trimmed, "<disk ") {
			continue
		}
		deviceType, _ := xmlAttributeValue(trimmed, "device")
		for index < len(lines) && !strings.Contains(strings.TrimSpace(lines[index]), "</disk>") {
			targetLine := strings.TrimSpace(lines[index])
			if strings.HasPrefix(targetLine, "<target ") {
				targetDev, _ := xmlAttributeValue(targetLine, "dev")
				if targetDev == device {
					return deviceType
				}
			}
			index++
		}
	}
	return ""
}

func domainMachineType(xmlStr string) string {
	for _, line := range strings.Split(xmlStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<type ") {
			machine, _ := xmlAttributeValue(trimmed, "machine")
			return strings.ToLower(machine)
		}
	}
	return ""
}

func setBlockDeviceBus(vmName, device, newBus, expectedType string) error {
	deviceLabel := "磁盘"
	if expectedType == "cdrom" {
		deviceLabel = "光驱"
	}
	if err := EnsureNotMigrating(vmName, "修改"+deviceLabel+"驱动类型"); err != nil {
		return err
	}
	state, _ := libvirt_rpc.GetDomainStateRPC(vmName)
	if state == "running" {
		return fmt.Errorf("修改%s驱动类型需要先关机", deviceLabel)
	}

	newBus = strings.ToLower(strings.TrimSpace(newBus))
	if expectedType == "cdrom" {
		if newBus == "" {
			return fmt.Errorf("请指定光驱驱动类型")
		}
		if _, err := normalizeCDROMBus(newBus); err != nil {
			return err
		}
	} else {
		switch newBus {
		case "virtio", "scsi", "sata", "ide":
		default:
			return fmt.Errorf("不支持的磁盘驱动类型: %s", newBus)
		}
	}

	// get current XML
	xmlResult, err := libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
	if err != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %w", err)
	}
	actualType := blockDeviceType(xmlResult, device)
	if actualType == "" {
		return fmt.Errorf("未找到设备 %s", device)
	}
	if actualType != expectedType {
		return fmt.Errorf("设备 %s 不是%s设备", device, deviceLabel)
	}
	if newBus == "ide" && strings.Contains(domainMachineType(xmlResult), "q35") {
		if expectedType == "cdrom" {
			return fmt.Errorf("当前 Q35 机型不支持 IDE 光驱，请使用 SCSI、SATA 或 USB")
		}
		return fmt.Errorf("当前 Q35 机型不支持 IDE 磁盘驱动，请使用 VirtIO、SCSI 或 SATA")
	}

	newDev, err := planDiskTargetDevice(xmlResult, device, newBus)
	if err != nil {
		return err
	}

	newXML, foundTarget := rewriteDiskBusXML(xmlResult, device, newDev, newBus)
	if !foundTarget {
		return fmt.Errorf("未找到设备 %s", device)
	}
	if _, err := libvirt_rpc.DefineDomainXMLRPC(newXML); err != nil {
		return fmt.Errorf("修改%s驱动失败: %w", deviceLabel, err)
	}

	return nil
}

// SetDiskBus changes the drive type of an existing disk (requires shutdown).
func SetDiskBus(vmName, device, newBus string) error {
	return setBlockDeviceBus(vmName, device, newBus, "disk")
}

// SetCDROMBus changes the drive type of an existing CDROM device (requires shutdown).
func SetCDROMBus(vmName, device, newBus string) error {
	return setBlockDeviceBus(vmName, device, newBus, "cdrom")
}

// ResizeDisk expands a disk to the specified size in GB.
func ResizeDisk(vmName, device string, newSizeGB int) error {
	if err := EnsureNotMigrating(vmName, "扩容磁盘"); err != nil {
		return err
	}
	vmState, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	// safety check: refuse resize if external snapshots exist
	hasExtSnap, extSnapNames, _ := CheckSnapshotSafety(vmName)
	if hasExtSnap {
		return fmt.Errorf("虚拟机存在外部快照（%s），扩容后恢复快照可能导致数据不一致。请先删除这些快照后再进行扩容操作",
			strings.Join(extSnapNames, "、"))
	}

	// get disk path (from XML)
	domainXML, xmlErr := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if xmlErr != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %w", xmlErr)
	}
	blkList := libvirt_rpc.ParseDisksFromDomainXML(domainXML)
	diskPath := ""
	for _, blk := range blkList {
		if blk.Target == device {
			diskPath = blk.Source
			break
		}
	}

	if vmState == "running" || vmState == "blocked" || vmState == "paused" || vmState == "pmsuspended" {
		newSizeBytes := uint64(newSizeGB) * 1024 * 1024 * 1024
		if err := libvirt_rpc.BlockResizeRPC(vmName, device, newSizeBytes, libvirt.DomainBlockResizeBytes); err != nil {
			return fmt.Errorf("热扩容失败: %w", err)
		}
	} else {
		if diskPath == "" {
			return fmt.Errorf("无法获取磁盘路径")
		}
		result := utils.ExecCommand("qemu-img", "resize", diskPath, fmt.Sprintf("%dG", newSizeGB))
		if result.Error != nil {
			return fmt.Errorf("扩容失败: %s", result.Stderr)
		}
	}

	return nil
}

// RemoveDisk detaches a disk from a VM and optionally deletes the file.
func RemoveDisk(vmName, device string, deleteFile bool) error {
	if err := EnsureNotMigrating(vmName, "删除磁盘"); err != nil {
		return err
	}
	vmState, _ := libvirt_rpc.GetDomainStateRPC(vmName)

	// get disk path and full disk XML from domain definition
	domainXML, xmlErr := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if xmlErr != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %w", xmlErr)
	}
	blkList := libvirt_rpc.ParseDisksFromDomainXML(domainXML)
	diskPath := ""
	for _, blk := range blkList {
		if blk.Target == device {
			diskPath = blk.Source
			break
		}
	}

	// extract the full <disk> XML block for the target device
	// using complete XML ensures the detach succeeds for both live and config,
	// avoiding the issue where virsh domblklist still shows the disk after detach
	fullDiskXML, extractErr := ExtractFullDiskXML(domainXML, device)
	if extractErr != nil {
		logger.Libvirt.Warn("提取完整磁盘XML失败，使用简化XML作为fallback", "device", device, "error", extractErr)
		// fallback: use simplified XML if extraction fails (should not happen normally)
		fullDiskXML = fmt.Sprintf("<disk type='file' device='disk'>\n  <target dev='%s'/>\n</disk>", device)
	}

	// detach disk using full XML definition
	var detachFlags uint32 = 2 // VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	if vmState == "running" {
		// virsh detach-disk --persistent = live + config
		detachFlags = 3 // VIR_DOMAIN_DEVICE_MODIFY_LIVE | VIR_DOMAIN_DEVICE_MODIFY_CONFIG
	}
	if err := libvirt_rpc.DetachDeviceFlagsRPC(vmName, fullDiskXML, detachFlags); err != nil {
		return fmt.Errorf("分离磁盘 %s 失败: %w", device, err)
	}

	// verify the disk has been removed (only for running VMs, where detach is async)
	if vmState == "running" {
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second)
			if !DiskDeviceExists(vmName, device) {
				break
			}
			if i == 9 {
				return fmt.Errorf("热删除磁盘超时: 设备 %s 仍然存在", device)
			}
		}
	}

	// delete file
	if deleteFile && diskPath != "" {
		_ = os.Remove(diskPath)
	}

	return nil
}
