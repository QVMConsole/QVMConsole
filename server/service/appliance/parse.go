package appliance

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

type ovfEnvelope struct {
	References struct {
		Files []ovfFile `xml:"File"`
	} `xml:"References"`
	DiskSection struct {
		Disks []ovfDisk `xml:"Disk"`
	} `xml:"DiskSection"`
	NetworkSection struct {
		Networks []ovfNetwork `xml:"Network"`
	} `xml:"NetworkSection"`
	VirtualSystems []ovfVirtualSystem `xml:"VirtualSystem"`
}

type ovfFile struct {
	ID   string `xml:"id,attr"`
	Href string `xml:"href,attr"`
	Size int64  `xml:"size,attr"`
}

type ovfDisk struct {
	ID            string `xml:"diskId,attr"`
	FileRef       string `xml:"fileRef,attr"`
	Capacity      string `xml:"capacity,attr"`
	CapacityUnits string `xml:"capacityAllocationUnits,attr"`
	PopulatedSize int64  `xml:"populatedSize,attr"`
	Format        string `xml:"format,attr"`
}

type ovfNetwork struct {
	Name string `xml:"name,attr"`
}

type ovfVirtualSystem struct {
	ID       string `xml:"id,attr"`
	Name     string `xml:"Name"`
	Arch     string `xml:"architecture,attr"`
	BootType string `xml:"bootType,attr"`
	Machine  string `xml:"machineType,attr"`
	OS       struct {
		ID          string `xml:"id,attr"`
		Description string `xml:"Description"`
	} `xml:"OperatingSystemSection"`
	Hardware struct {
		System struct {
			VirtualSystemType string `xml:"VirtualSystemType"`
		} `xml:"System"`
		Items []ovfItem `xml:"Item"`
	} `xml:"VirtualHardwareSection"`
}

type ovfItem struct {
	InstanceID      string `xml:"InstanceID"`
	ResourceType    string `xml:"ResourceType"`
	ResourceSubType string `xml:"ResourceSubType"`
	VirtualQuantity string `xml:"VirtualQuantity"`
	AllocationUnits string `xml:"AllocationUnits"`
	Parent          string `xml:"Parent"`
	HostResource    string `xml:"HostResource"`
	Connection      string `xml:"Connection"`
	ElementName     string `xml:"ElementName"`
	Bus             string `xml:"bus,attr"`
}

// ParseOVF 将 OVF XML 转换为面板统一元数据。
func ParseOVF(data []byte) (*Metadata, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("OVF 描述为空")
	}
	if len(data) > 16*1024*1024 {
		return nil, fmt.Errorf("OVF 描述超过 16MB 限制")
	}
	upper := bytes.ToUpper(data)
	if bytes.Contains(upper, []byte("<!DOCTYPE")) || bytes.Contains(upper, []byte("<!ENTITY")) {
		return nil, fmt.Errorf("OVF 描述包含不受支持的实体声明")
	}

	var env ovfEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("解析 OVF 描述失败: %w", err)
	}
	if len(env.VirtualSystems) != 1 {
		return nil, fmt.Errorf("虚拟机包需要包含且仅包含一个 VirtualSystem")
	}
	vs := env.VirtualSystems[0]
	meta := &Metadata{
		SourceFormat: "ovf",
		Name:         firstNonEmpty(strings.TrimSpace(vs.Name), strings.TrimSpace(vs.ID), "imported-vm"),
		Architecture: normalizeArchitecture(firstNonEmpty(vs.Arch, vs.Hardware.System.VirtualSystemType, vs.OS.Description)),
		BootType:     normalizeBootType(firstNonEmpty(vs.BootType, vs.Hardware.System.VirtualSystemType)),
		MachineType:  normalizeMachineType(firstNonEmpty(vs.Machine, vs.Hardware.System.VirtualSystemType)),
		OSType:       normalizeOSType(firstNonEmpty(vs.OS.Description, vs.OS.ID)),
		Disks:        make([]Disk, 0),
		Networks:     make([]Network, 0),
		Warnings:     make([]string, 0),
	}

	files := make(map[string]ovfFile, len(env.References.Files))
	for _, file := range env.References.Files {
		href, err := decodeHref(file.Href)
		if err != nil {
			return nil, err
		}
		file.Href = href
		files[file.ID] = file
	}

	controllers := map[string]string{}
	diskParents := map[string]string{}
	diskBuses := map[string]string{}
	var itemDiskIDs []string
	for _, item := range vs.Hardware.Items {
		switch strings.TrimSpace(item.ResourceType) {
		case "3":
			meta.VCPU = parsePositiveInt(item.VirtualQuantity)
		case "4":
			bytesValue := quantityBytes(item.VirtualQuantity, item.AllocationUnits)
			if bytesValue > 0 {
				meta.RAM = int(math.Ceil(float64(bytesValue) / float64(1024*1024*1024)))
			}
		case "5", "6", "20":
			controllers[item.InstanceID] = normalizeDiskBus(item.ResourceSubType, item.ResourceType)
		case "10":
			meta.Networks = append(meta.Networks, Network{
				Name:  firstNonEmpty(strings.TrimSpace(item.Connection), "默认网络"),
				Model: normalizeNICModel(item.ResourceSubType),
			})
		case "17":
			id := strings.TrimPrefix(strings.TrimSpace(item.HostResource), "ovf:/disk/")
			id = strings.TrimPrefix(id, "/disk/")
			itemDiskIDs = append(itemDiskIDs, id)
			diskParents[id] = item.Parent
			diskBuses[id] = item.Bus
		}
	}
	if meta.VCPU <= 0 {
		meta.VCPU = 2
		meta.Warnings = append(meta.Warnings, "虚拟机包未声明 CPU 数量，已使用 2 核默认值")
	}
	if meta.RAM <= 0 {
		meta.RAM = 2
		meta.Warnings = append(meta.Warnings, "虚拟机包未声明内存容量，已使用 2GB 默认值")
	}
	if meta.Architecture == "" {
		meta.Warnings = append(meta.Warnings, "虚拟机包未声明可识别的架构，导入时将使用当前宿主机架构")
	}
	if meta.BootType == "" {
		meta.Warnings = append(meta.Warnings, "虚拟机包未声明可识别的固件类型，导入时将使用当前宿主机默认固件")
	}

	orderedDisks := make([]ovfDisk, 0, len(env.DiskSection.Disks))
	seenDisks := map[string]bool{}
	for _, id := range itemDiskIDs {
		for _, disk := range env.DiskSection.Disks {
			if disk.ID == id && !seenDisks[disk.ID] {
				orderedDisks = append(orderedDisks, disk)
				seenDisks[disk.ID] = true
			}
		}
	}
	for _, disk := range env.DiskSection.Disks {
		if !seenDisks[disk.ID] {
			orderedDisks = append(orderedDisks, disk)
		}
	}
	for _, disk := range orderedDisks {
		file, ok := files[disk.FileRef]
		if !ok || file.Href == "" {
			return nil, fmt.Errorf("磁盘 %s 缺少有效文件引用", disk.ID)
		}
		capacity := quantityBytes(disk.Capacity, disk.CapacityUnits)
		if capacity <= 0 {
			capacity = disk.PopulatedSize
		}
		bus := ""
		if diskBuses[disk.ID] != "" {
			bus = normalizeDiskBus(diskBuses[disk.ID], "")
		}
		if bus == "" {
			bus = controllers[diskParents[disk.ID]]
		}
		// 兼容旧版面板导出：描述曾固定写为 LSI SCSI，但文件名仍保留 vda/vdb。
		if isNativeOVF(vs) {
			if inferred := inferNativeDiskBus(file.Href); inferred != "" && inferred != bus {
				bus = inferred
				meta.Warnings = append(meta.Warnings, fmt.Sprintf("已根据导出设备名恢复磁盘 %s 的 %s 总线", disk.ID, inferred))
			}
		}
		if bus == "" {
			bus = "scsi"
		}
		meta.Disks = append(meta.Disks, Disk{
			ID:            firstNonEmpty(disk.ID, disk.FileRef),
			FileRef:       file.Href,
			CapacityBytes: capacity,
			Format:        normalizeDiskFormat(disk.Format, file.Href),
			Bus:           bus,
			IsSystem:      len(meta.Disks) == 0,
		})
	}
	if len(meta.Disks) == 0 {
		return nil, fmt.Errorf("OVF 描述中未发现可导入磁盘")
	}
	if len(meta.Networks) == 0 {
		for _, network := range env.NetworkSection.Networks {
			meta.Networks = append(meta.Networks, Network{Name: firstNonEmpty(network.Name, "默认网络"), Model: "virtio"})
		}
	}
	if len(meta.Networks) == 0 {
		meta.Networks = append(meta.Networks, Network{Name: "默认网络", Model: "virtio"})
	}
	return meta, nil
}

func isNativeOVF(vs ovfVirtualSystem) bool {
	return strings.TrimSpace(vs.Arch) != "" || strings.TrimSpace(vs.BootType) != "" || strings.TrimSpace(vs.Machine) != ""
}

func inferNativeDiskBus(fileRef string) string {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(fileRef)), filepath.Ext(fileRef))
	parts := strings.Split(name, "-")
	device := parts[len(parts)-1]
	switch {
	case strings.HasPrefix(device, "vd") && len(device) > 2:
		return "virtio"
	case strings.HasPrefix(device, "hd") && len(device) > 2:
		return "ide"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parsePositiveInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	if n < 0 {
		return 0
	}
	return n
}

func quantityBytes(value, units string) int64 {
	quantity, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || quantity <= 0 {
		return 0
	}
	lower := strings.ToLower(strings.TrimSpace(units))
	multiplier := float64(1)
	if marker := strings.Index(lower, "2^"); marker >= 0 {
		digits := strings.TrimLeft(lower[marker+2:], " ")
		end := 0
		for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
			end++
		}
		if exponent, parseErr := strconv.Atoi(digits[:end]); parseErr == nil && exponent >= 0 && exponent <= 60 {
			multiplier = math.Pow(2, float64(exponent))
			return int64(quantity * multiplier)
		}
	}
	switch {
	case strings.Contains(lower, "2^40"), strings.Contains(lower, "tb"):
		multiplier = 1 << 40
	case strings.Contains(lower, "2^30"), strings.Contains(lower, "gb"):
		multiplier = 1 << 30
	case strings.Contains(lower, "2^20"), strings.Contains(lower, "mb"), lower == "megabytes":
		multiplier = 1 << 20
	case strings.Contains(lower, "2^10"), strings.Contains(lower, "kb"):
		multiplier = 1 << 10
	}
	return int64(quantity * multiplier)
}

func decodeHref(href string) (string, error) {
	decoded, err := url.PathUnescape(strings.TrimSpace(href))
	if err != nil {
		return "", fmt.Errorf("OVF 文件引用编码错误: %w", err)
	}
	if decoded == "" || filepath.IsAbs(decoded) || strings.Contains(decoded, "\\") || strings.Contains(decoded, "://") {
		return "", fmt.Errorf("OVF 文件引用不安全: %s", href)
	}
	cleaned := filepath.Clean(decoded)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("OVF 文件引用越过包目录: %s", href)
	}
	if filepath.Base(cleaned) != cleaned {
		return "", fmt.Errorf("OVF 配套文件引用需要使用同目录文件名: %s", href)
	}
	return filepath.ToSlash(cleaned), nil
}

func normalizeArchitecture(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "aarch64"), strings.Contains(lower, "arm64"):
		return "aarch64"
	case strings.Contains(lower, "x86_64"), strings.Contains(lower, "amd64"), strings.Contains(lower, "x86"):
		return "x86_64"
	case strings.Contains(lower, "riscv64"):
		return "riscv64"
	default:
		return ""
	}
}

func normalizeBootType(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "uefi") || strings.Contains(lower, "efi") {
		return "uefi"
	}
	return "bios"
}

func normalizeMachineType(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "q35"):
		return "q35"
	case strings.Contains(lower, "i440"), strings.Contains(lower, "pc-i440"):
		return "i440fx"
	case strings.Contains(lower, "virt") && (strings.Contains(lower, "arm") || strings.Contains(lower, "aarch64")):
		return "virt"
	default:
		return ""
	}
}

func normalizeOSType(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "windows") || strings.Contains(lower, "microsoft") {
		return "windows"
	}
	if strings.Contains(lower, "linux") || strings.Contains(lower, "ubuntu") || strings.Contains(lower, "debian") || strings.Contains(lower, "centos") {
		return "linux"
	}
	return "other"
}

func normalizeDiskBus(subType, resourceType string) string {
	lower := strings.ToLower(subType)
	switch {
	case strings.Contains(lower, "ide") || resourceType == "5":
		return "ide"
	case strings.Contains(lower, "sata"):
		return "sata"
	case strings.Contains(lower, "virtio"):
		return "virtio"
	default:
		return "scsi"
	}
}

func normalizeNICModel(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "e1000e"):
		return "e1000e"
	case strings.Contains(lower, "e1000"):
		return "e1000"
	case strings.Contains(lower, "rtl8139"):
		return "rtl8139"
	default:
		return "virtio"
	}
}

func normalizeDiskFormat(format, href string) string {
	lower := strings.ToLower(format + " " + filepath.Ext(href))
	switch {
	case strings.Contains(lower, "vmdk"):
		return "vmdk"
	case strings.Contains(lower, "qcow2"):
		return "qcow2"
	case strings.Contains(lower, "vhdx"):
		return "vhdx"
	case strings.Contains(lower, "vhd"):
		return "vpc"
	case strings.Contains(lower, "raw"), strings.Contains(lower, ".img"):
		return "raw"
	default:
		return ""
	}
}
