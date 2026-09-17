// Package pci 统一维护 PCI 直通设备的准入策略。
//
// 设备过滤不再依赖 lspci 输出的英文类别名（受发行版与本地化影响，且多数设备没有
// 中文映射，容易出现漏网设备），统一基于 sysfs 中的 PCI class code 判定；同时结合
// 宿主机实际使用情况（根文件系统所在存储控制器、键鼠所在 USB 控制器）以及 IOMMU
// 组隔离情况做准入，避免用户把 CPU uncore、桥片、宿主机根盘直通给虚拟机。
package pci

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DeviceFacts 直通准入判定所需的最小设备信息
type DeviceFacts struct {
	Address     string // PCI 地址，如 0000:3f:0b.3
	ClassCode   string // PCI 类别码，如 088000（主类 2 位 + 子类 2 位 + 接口 2 位）
	Driver      string // 当前绑定驱动，无驱动为空
	VendorID    string // 厂商 ID
	ProductID   string // 设备 ID
	VendorName  string // 厂商名称
	ProductName string // 设备名称
	VFIOBound   bool   // 是否已绑定 vfio-pci
	IOMMUGroup  int    // IOMMU 组号，-1 表示无分组
}

// sysfsPCIDevicesDir 宿主机 PCI 设备 sysfs 根目录
const sysfsPCIDevicesDir = "/sys/bus/pci/devices"

// pciAddressPattern 匹配 sysfs 路径中出现的 PCI 地址
var pciAddressPattern = regexp.MustCompile(`[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]`)

// pciAddressFullPattern 校验完整的 PCI 地址字符串
var pciAddressFullPattern = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)

// nonHexPattern 用于过滤类别码中的非十六进制字符
var nonHexPattern = regexp.MustCompile(`[^0-9a-f]`)

// 主类代码，用于提升可读性
const (
	classBaseMemoryController = "05"
	classBaseBridge           = "06"
	classBaseSystemPeripheral = "08"
	classBaseProcessor        = "0b"
	classBaseSignalProcessing = "11"
	classBaseInstrumentation  = "13"
)

// blockedClassBaseReasons 主类级硬排除：这些设备寄存器与 CPU/内存子系统或桥片直接相关
var blockedClassBaseReasons = map[string]string{
	classBaseMemoryController: "内存控制器属于 CPU 内存子系统，直通会导致宿主机崩溃",
	classBaseBridge:           "桥设备（Host/ISA/PCI 桥）是宿主机拓扑的一部分，不可直通",
	classBaseSystemPeripheral: "系统外设（CPU uncore 寄存器、IOMMU/VT-d 等），直通会导致宿主机崩溃",
	classBaseProcessor:        "处理器（Processor）类设备是 CPU 内建寄存器，不可直通",
	classBaseSignalProcessing: "信号处理控制器（CPU uncore 性能计数器），直通无意义且会导致宿主机异常",
	classBaseInstrumentation:  "Non-Essential Instrumentation 属于主机 SoC 内建器件，不可直通",
}

// blockedClassSubReasons 子类级硬排除
var blockedClassSubReasons = map[string]string{
	"0101": "IDE 控制器通常挂载宿主机磁盘，不可直通",
	"0104": "硬件 RAID 控制器承载宿主机数据，直通会导致宿主机不可用",
	"0780": "通信控制器（Intel MEI/HECI 管理引擎接口），直通会破坏宿主机带外管理",
	"0c05": "SMBus 控制器用于宿主机温度与电源管理，不可直通",
	"0c07": "IPMI 接口是宿主机带外管理通道，直通会带来安全风险",
	"0c08": "SMBus 控制器用于宿主机温度与电源管理，不可直通",
}

// blockedDriverReasons 驱动级硬排除：设备本身可能可直通，但当前驱动代表它正被宿主机关键功能占用
var blockedDriverReasons = map[string]string{
	"megaraid_sas": "硬件 RAID 驱动正在使用该控制器",
	"megaraid":     "硬件 RAID 驱动正在使用该控制器",
	"aacraid":      "硬件 RAID 驱动正在使用该控制器",
	"lpc_ich":      "LPC/ISA 桥驱动正在使用该设备",
	"lpc_sch":      "LPC/ISA 桥驱动正在使用该设备",
	"mei_me":       "Intel 管理引擎驱动正在使用该设备",
	"mei":          "Intel 管理引擎驱动正在使用该设备",
	"vmwgfx":       "VMware 虚拟显卡（解绑会导致宿主机崩溃）",
	"nvidia":       "NVIDIA 驱动正在使用该显卡，需先按文档将其配置为 vfio-pci",
	"ehci-pci":     "USB 2.0（EHCI）控制器不支持直通，请选择 USB 3.0 控制器",
	"ehci_hcd":     "USB 2.0（EHCI）控制器不支持直通，请选择 USB 3.0 控制器",
	"ohci-pci":     "USB 1.1（OHCI）控制器不支持直通，请选择 USB 3.0 控制器",
	"ohci_hcd":     "USB 1.1（OHCI）控制器不支持直通，请选择 USB 3.0 控制器",
	"uhci_hcd":     "USB 1.1（UHCI）控制器不支持直通，请选择 USB 3.0 控制器",
	"pcieport":     "PCIe 端口属于宿主机拓扑，不可直通",
}

// blockedDriverPrefixes 驱动名前缀级硬排除（覆盖各代 CPU 的 uncore 驱动，如 ivbep_uncore）
var blockedDriverPrefixes = []string{"uncore"}

// virtualVendorReasons 虚拟化平台自身的设备
var virtualVendorReasons = map[string]string{
	"1af4": "虚拟化平台（virtio）设备，无法再次直通",
	"1b36": "虚拟化平台（QEMU）设备，无法再次直通",
	"1234": "虚拟化平台（Bochs/QEMU）显卡，无法再次直通",
	"15ad": "虚拟化平台（VMware）设备，无法再次直通",
	"80ee": "虚拟化平台（VirtualBox）设备，无法再次直通",
}

// virtualOrBMCGPUDrivers 虚拟机显示与 BMC 管理显卡驱动
// BMC 显卡就是宿主机控制台，直通无意义；虚拟机显示设备无法二次直通。
var virtualOrBMCGPUDrivers = map[string]bool{
	"virtio-pci": true,
	"virtio_gpu": true,
	"vmwgfx":     true,
	"bochs-drm":  true,
	"cirrus":     true,
	"qxl":        true,
	"ast":        true,
	"mgag200":    true,
	"mga":        true,
	"gma500":     true,
	"vboxvideo":  true,
	"vmware":     true,
}

// thunderboltKeywords 用于识别归类为系统外设（0880）的雷电/USB4 控制器
var thunderboltKeywords = []string{
	"thunderbolt", "usb4", "jhl", "maple ridge", "titan ridge",
	"alpine ridge", "barlow ridge", "ice lake thunderbolt",
}

// classSubUSBController USB 控制器子类（需要判断是否挂有宿主机键鼠）
const classSubUSBController = "0c03"

// usbControllerProgIfReasons 按接口类型（prog-if）判定 USB 控制器是否可直通。
// 只有 USB 3.0（xHCI，30）与 USB4 主机接口（40）可以整体直通，
// USB 1.1/2.0（UHCI/OHCI/EHCI）控制器直通后无法被主流客户机正常驱动。
var usbControllerProgIfReasons = map[string]string{
	"00": "USB 1.1（UHCI）控制器不支持直通，请选择 USB 3.0（xHCI）控制器",
	"10": "USB 1.1（OHCI）控制器不支持直通，请选择 USB 3.0（xHCI）控制器",
	"20": "USB 2.0（EHCI）控制器不支持直通，请选择 USB 3.0（xHCI）控制器",
	"fe": "USB 设备（非控制器）无法整体直通",
}

// classProgIf 返回类别码中的接口字节（prog-if），如 0c0330 -> 30
func classProgIf(code string) string {
	normalized := NormalizeClassCode(code)
	if len(normalized) < 6 {
		return ""
	}
	return normalized[4:6]
}

// hasProgIf 判断类别码是否携带接口字节。sysfs（0x0c0330）与 lspci -nn 的完整类别码
// 为 6 位，而部分来源只提供 4 位主类+子类，此时无法判定接口类型。
func hasProgIf(code string) bool {
	trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(code)), "0x")
	return len(nonHexPattern.ReplaceAllString(trimmed, "")) >= 6
}

// NormalizeClassCode 归一化 PCI 类别码为 6 位十六进制。
// sysfs 的 class 文件形如 0x088000，lspci -nn 输出形如 0880，这里先去掉 0x 前缀
// 再做补齐，否则前缀里的 0 会被当成有效位，导致类别码整体左移一位（08 变成 00），
// 使所有按主类/子类做的排除判定失效。
func NormalizeClassCode(code string) string {
	trimmed := strings.ToLower(strings.TrimSpace(code))
	trimmed = strings.TrimPrefix(trimmed, "0x")
	value := nonHexPattern.ReplaceAllString(trimmed, "")
	if value == "" {
		return ""
	}
	switch {
	case len(value) >= 6:
		return value[:6]
	case len(value) == 4:
		return value + "00"
	case len(value) == 2:
		return value + "0000"
	default:
		for len(value) < 4 {
			value += "0"
		}
		return value + "00"
	}
}

// ClassBase 返回主类（类别码前 2 位），如 088000 -> 08
func ClassBase(code string) string {
	normalized := NormalizeClassCode(code)
	if len(normalized) < 2 {
		return ""
	}
	return normalized[:2]
}

// ClassSub 返回主类 + 子类（类别码前 4 位），如 088000 -> 0880
func ClassSub(code string) string {
	normalized := NormalizeClassCode(code)
	if len(normalized) < 4 {
		return ""
	}
	return normalized[:4]
}

// ValidateAddress 校验 PCI 地址格式
func ValidateAddress(address string) bool {
	return pciAddressFullPattern.MatchString(strings.ToLower(strings.TrimSpace(address)))
}

// IsVirtualOrBMCGPUDriver 判断驱动是否为虚拟机显示或 BMC 管理显卡驱动，
// 硬件直通页面与核显直通页面的设备检测共用该判定，避免两处结论不一致。
func IsVirtualOrBMCGPUDriver(driver string) bool {
	return virtualOrBMCGPUDrivers[strings.ToLower(strings.TrimSpace(driver))]
}

// isThunderboltController 判断系统外设类设备是否为可直通的雷电/USB4 控制器。
// 雷电控制器在 PCI 类别上同样属于 0880，但它是普通外设控制器，可以安全直通；
// 通过内核 thunderbolt 驱动或产品名识别，避免硬编码设备 ID 表。
func isThunderboltController(f DeviceFacts) bool {
	if strings.EqualFold(strings.TrimSpace(f.Driver), "thunderbolt") {
		return true
	}
	name := strings.ToLower(f.ProductName + " " + f.VendorName)
	for _, keyword := range thunderboltKeywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

// isTransparentBridge 判断是否为只承担拓扑转发的桥片（Host 桥 / PCI-PCIe 桥）。
// 这类桥片留在宿主机上不会阻断同组设备直通，因此不参与 IOMMU 组可用性判定；
// 而 ISA/EISA/MCA 桥（0601/0602/0603）连接的是宿主机实体器件，必须阻断。
func isTransparentBridge(code string) bool {
	switch ClassSub(code) {
	case "0600", "0604", "0605", "0606", "0607", "0608", "0609", "060a":
		return true
	default:
		return false
	}
}

// Context 保存一次判定过程中读取到的宿主机状态，避免逐设备重复读取系统文件
type Context struct {
	once           sync.Once
	hostStorageSet map[string]string // PCI 地址 -> 承载的宿主机挂载点/交换分区描述
	inputDeviceSet map[string]bool   // 挂了键盘/鼠标的 PCI 地址
}

// NewContext 创建判定上下文（宿主机状态在首次判定时惰性读取）
func NewContext() *Context {
	return &Context{}
}

// Evaluate 判定单个设备是否允许直通，第二个返回值为不可直通的中文原因
func (c *Context) Evaluate(f DeviceFacts) (bool, string) {
	code := NormalizeClassCode(f.ClassCode)
	base := ClassBase(code)
	sub := ClassSub(code)
	driver := strings.ToLower(strings.TrimSpace(f.Driver))
	vendor := strings.ToLower(strings.TrimSpace(f.VendorID))

	// 1. 无法识别类别的无效设备
	if code == "" && driver == "" && !f.VFIOBound {
		return false, "无法识别设备类别且无驱动，不能确认可以安全直通"
	}

	// 2. 虚拟化平台自身设备
	if reason, ok := virtualVendorReasons[vendor]; ok {
		return false, reason
	}
	if reason, ok := blockedDriverReasons[driver]; ok {
		return false, reason
	}
	for _, prefix := range blockedDriverPrefixes {
		if driver != "" && strings.Contains(driver, prefix) {
			return false, "CPU uncore 性能监控驱动正在使用该设备"
		}
	}
	if IsVirtualOrBMCGPUDriver(driver) {
		return false, "虚拟机显示设备或 BMC 管理显卡，直通无意义"
	}

	// 3. 类别码硬排除（雷电/USB4 控制器例外）
	if reason, ok := blockedClassBaseReasons[base]; ok {
		if base != classBaseSystemPeripheral || !isThunderboltController(f) {
			return false, reason
		}
	}
	if reason, ok := blockedClassSubReasons[sub]; ok {
		return false, reason
	}

	// 3.1 USB 控制器按接口类型判定：只放开 USB 3.0（xHCI）与 USB4 主机接口，
	// 类别码未携带接口字节（lspci 仅给出 4 位类别）时退回驱动判定，避免误杀 xHCI。
	if sub == classSubUSBController && hasProgIf(f.ClassCode) {
		if reason, ok := usbControllerProgIfReasons[classProgIf(f.ClassCode)]; ok {
			return false, reason
		}
	}

	// 4. 承载宿主机数据的设备
	if description, ok := c.hostStorageDevices()[f.Address]; ok {
		return false, fmt.Sprintf("该设备上挂载了宿主机正在使用的存储（%s），直通会导致宿主机不可用", description)
	}

	// 5. 挂有键鼠的 USB 控制器
	if sub == classSubUSBController && c.inputDevices()[f.Address] {
		return false, "该 USB 控制器上挂有宿主机键盘或鼠标，直通后宿主机将失去输入设备"
	}

	return true, ""
}

// EvaluateWithGroup 在单设备判定的基础上叠加 IOMMU 组可用性判定。
// 组内除拓扑桥片外的其它成员都必须可以直通，否则整组无法独立隔离，
// 绑定后会因为组不可用而无法启动虚拟机。
func (c *Context) EvaluateWithGroup(f DeviceFacts, group []DeviceFacts) (bool, string) {
	if allowed, reason := c.Evaluate(f); !allowed {
		return false, reason
	}
	if f.IOMMUGroup < 0 || len(group) == 0 {
		return true, ""
	}
	for _, member := range group {
		if strings.EqualFold(member.Address, f.Address) || isTransparentBridge(member.ClassCode) {
			continue
		}
		if allowed, reason := c.Evaluate(member); !allowed {
			return false, fmt.Sprintf("同一 IOMMU 组内的设备 %s 不可直通（%s），整组无法独立隔离", member.Address, reason)
		}
	}
	return true, ""
}

// ReadDeviceFacts 从 sysfs 读取准入判定所需信息（不依赖 lspci）
func ReadDeviceFacts(address string) (DeviceFacts, error) {
	normalized := strings.ToLower(strings.TrimSpace(address))
	if !ValidateAddress(normalized) {
		return DeviceFacts{}, fmt.Errorf("PCI 地址 %s 格式不正确", address)
	}
	basePath := filepath.Join(sysfsPCIDevicesDir, normalized)
	classCode := NormalizeClassCode(readTextFile(filepath.Join(basePath, "class")))
	if classCode == "" {
		return DeviceFacts{}, fmt.Errorf("设备 %s 不存在或无法读取 PCI 类别码", normalized)
	}

	driver := ""
	if link, err := os.Readlink(filepath.Join(basePath, "driver")); err == nil {
		driver = filepath.Base(link)
	}
	group := -1
	if link, err := os.Readlink(filepath.Join(basePath, "iommu_group")); err == nil {
		if parsed, convErr := strconv.Atoi(filepath.Base(link)); convErr == nil {
			group = parsed
		}
	}

	return DeviceFacts{
		Address:    normalized,
		ClassCode:  classCode,
		Driver:     driver,
		VendorID:   strings.TrimPrefix(strings.ToLower(readTextFile(filepath.Join(basePath, "vendor"))), "0x"),
		ProductID:  strings.TrimPrefix(strings.ToLower(readTextFile(filepath.Join(basePath, "device"))), "0x"),
		VFIOBound:  strings.EqualFold(driver, "vfio-pci"),
		IOMMUGroup: group,
	}, nil
}

// hostStorageDevices 返回承载宿主机已挂载文件系统或交换分区的 PCI 设备
func (c *Context) hostStorageDevices() map[string]string {
	c.once.Do(c.loadHostState)
	return c.hostStorageSet
}

// inputDevices 返回挂了键盘或鼠标（含 BMC 虚拟键鼠）的 PCI 设备
func (c *Context) inputDevices() map[string]bool {
	c.once.Do(c.loadHostState)
	return c.inputDeviceSet
}

// loadHostState 读取宿主机挂载与输入设备状态
func (c *Context) loadHostState() {
	c.hostStorageSet = make(map[string]string)
	c.inputDeviceSet = make(map[string]bool)

	devices := mountedBlockDevices()
	devices = append(devices, swapBlockDevices()...)
	for _, device := range devices {
		for _, address := range hostStoragePCIAddresses(device.majmin) {
			// 取最短的挂载点，通常是根文件系统 "/"，便于用户理解
			if exist, ok := c.hostStorageSet[address]; !ok || len(device.description) < len(exist) {
				c.hostStorageSet[address] = device.description
			}
		}
	}

	for _, address := range inputPCIAddresses() {
		c.inputDeviceSet[address] = true
	}
}

// blockDeviceRef 块设备引用
type blockDeviceRef struct {
	majmin      string // major:minor
	description string // 挂载点或交换分区描述
}

// mountedBlockDevices 解析 /proc/self/mountinfo，返回已挂载文件系统所在的真实块设备
func mountedBlockDevices() []blockDeviceRef {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil
	}

	var devices []blockDeviceRef
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		majmin := fields[2]
		mountPoint := fields[4]
		if seen[majmin] || !isRealBlockDevice(majmin) {
			continue
		}
		seen[majmin] = true
		devices = append(devices, blockDeviceRef{majmin: majmin, description: mountPoint})
	}
	return devices
}

// swapBlockDevices 解析 /proc/swaps，返回活动交换分区所在的真实块设备
func swapBlockDevices() []blockDeviceRef {
	data, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return nil
	}

	var devices []blockDeviceRef
	for index, line := range strings.Split(string(data), "\n") {
		if index == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		// 交换文件（file）所在文件系统已由挂载点覆盖，这里只处理分区
		if len(fields) < 2 || fields[1] != "partition" {
			continue
		}
		majmin := blockDeviceMajorMinor(fields[0])
		if majmin != "" {
			devices = append(devices, blockDeviceRef{majmin: majmin, description: "交换分区"})
		}
	}
	return devices
}

// isRealBlockDevice 判断 major:minor 是否对应真实块设备（tmpfs 等伪文件系统没有对应节点）
func isRealBlockDevice(majmin string) bool {
	if majmin == "" {
		return false
	}
	_, err := os.Stat(filepath.Join("/sys/dev/block", majmin))
	return err == nil
}

// blockDeviceMajorMinor 通过 /sys/class/block 获取块设备的 major:minor
func blockDeviceMajorMinor(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	name := filepath.Base(resolved)
	return readTextFile(filepath.Join("/sys/class/block", name, "dev"))
}

// hostStoragePCIAddresses 从块设备向上追溯宿主侧 PCI 控制器地址。
// 逐层展开 slaves，兼容 LVM、软 RAID、多路径等堆叠设备。
func hostStoragePCIAddresses(majmin string) []string {
	found := make(map[string]bool)
	visited := make(map[string]bool)
	queue := []string{filepath.Join("/sys/dev/block", majmin)}

	for depth := 0; depth < 4 && len(queue) > 0; depth++ {
		var next []string
		for _, node := range queue {
			resolved, err := filepath.EvalSymlinks(node)
			if err != nil || visited[resolved] {
				continue
			}
			visited[resolved] = true
			for _, address := range pciAddressPattern.FindAllString(resolved, -1) {
				found[address] = true
			}
			slaves, _ := filepath.Glob(filepath.Join(resolved, "slaves", "*"))
			next = append(next, slaves...)
		}
		queue = next
	}

	addresses := make([]string, 0, len(found))
	for address := range found {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses
}

// inputPCIAddresses 返回挂了键盘或鼠标（含 BMC 虚拟键鼠）的 PCI 控制器地址
func inputPCIAddresses() []string {
	found := make(map[string]bool)

	// 1. 已注册的输入设备：覆盖 USB、平台设备与 BMC 虚拟键鼠
	if data, err := os.ReadFile("/proc/bus/input/devices"); err == nil {
		for _, block := range strings.Split(string(data), "\n\n") {
			if !hasKeyboardOrMouseHandler(block) {
				continue
			}
			for _, address := range pciAddressPattern.FindAllString(block, -1) {
				found[address] = true
			}
		}
	}

	// 2. USB HID 接口：部分设备不上报 kbd/mouse handler
	classPaths, _ := filepath.Glob("/sys/bus/usb/devices/*/bInterfaceClass")
	for _, classPath := range classPaths {
		if readTextFile(classPath) != "03" {
			continue
		}
		protocol := readTextFile(filepath.Join(filepath.Dir(classPath), "bInterfaceProtocol"))
		// 01 键盘，02 鼠标
		if protocol != "01" && protocol != "02" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Dir(classPath))
		if err != nil {
			continue
		}
		for _, address := range pciAddressPattern.FindAllString(resolved, -1) {
			found[address] = true
		}
	}

	addresses := make([]string, 0, len(found))
	for address := range found {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses
}

// hasKeyboardOrMouseHandler 判断输入设备描述块是否注册了键盘或鼠标 handler
func hasKeyboardOrMouseHandler(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "H: Handlers=") {
			continue
		}
		for _, handler := range strings.Fields(strings.TrimPrefix(trimmed, "H: Handlers=")) {
			if handler == "kbd" || handler == "mouse" {
				return true
			}
		}
	}
	return false
}

// readTextFile 读取文本文件并去除首尾空白，失败返回空字符串
func readTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
