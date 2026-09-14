package vm

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kvm_console/logger"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// PCIDevice 直通 PCI 设备信息
type PCIDevice struct {
	PCIAddress           string `json:"pci_address"`            // 0000:04:00.0 格式
	Domain               string `json:"domain"`                 // PCI 域
	Bus                  string `json:"bus"`                    // PCI 总线
	Slot                 string `json:"slot"`                   // PCI 插槽
	Function             string `json:"function"`               // PCI 功能
	VendorID             string `json:"vendor_id"`              // 厂商 ID
	VendorName           string `json:"vendor_name"`            // 厂商名称
	ProductID            string `json:"product_id"`             // 产品 ID
	ProductName          string `json:"product_name"`           // 产品名称
	ClassName            string `json:"class_name"`             // 设备类别
	IOMMUGroup           int    `json:"iommu_group"`            // IOMMU 组号
	DriverInUse          string `json:"driver_in_use"`          // 当前驱动
	IsVfioBound          bool   `json:"is_vfio_bound"`          // 是否已绑定 vfio-pci
	IsUsedByVM           bool   `json:"is_used_by_vm"`          // 是否已被虚拟机使用
	UsedByVMName         string `json:"used_by_vm_name"`        // 使用该设备的虚拟机名
	IsPassthroughCapable bool   `json:"is_passthrough_capable"` // 是否可直通
	CapabilityNote       string `json:"capability_note"`        // 不可直通原因
}

// hostdevXML Template for libvirt PCI passthrough
const hostdevXMLTemplate = `<hostdev mode='subsystem' type='pci' managed='yes'>
  <source>
    <address domain='0x%s' bus='0x%s' slot='0x%s' function='0x%s'/>
  </source>
</hostdev>`

var readPCIDeviceClass = os.ReadFile

// 直通设备扫描会访问宿主机全部 PCI 设备。结果短时间内变化很少，
// 使用短 TTL 缓存并保证同一时刻最多只有一个扫描任务，避免请求风暴。
const passthroughDeviceCacheTTL = 10 * time.Second

type passthroughDeviceCacheState struct {
	sync.Mutex
	devices     []PCIDevice
	refreshedAt time.Time
	refreshing  bool
	wait        chan struct{}
	lastErr     error
}

var passthroughCache passthroughDeviceCacheState

// parsePCIAddress 解析 PCI 地址 "0000:04:00.0" 为 domain/bus/slot/function
func parsePCIAddress(addr string) (domain, bus, slot, function string, err error) {
	// 格式: domain:bus:slot.function，例如 0000:04:00.0
	parts := strings.Split(addr, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", "", "", fmt.Errorf("无效的 PCI 地址格式: %s（应为 domain:bus:slot.function）", addr)
	}

	domain = parts[0]
	var busSlotPart string

	if len(parts) == 3 {
		// 标准格式 domain:bus:slot.function → parts[1]=bus, parts[2]=slot.function
		bus = parts[1]
		busSlotPart = parts[2]
	} else {
		// 兼容旧格式 domain:bus-slot.function
		busSlotPart = parts[1]
	}

	subParts := strings.Split(busSlotPart, ".")
	if len(subParts) != 2 {
		return "", "", "", "", fmt.Errorf("无效的 PCI 地址格式: %s", addr)
	}
	slot = subParts[0]
	function = subParts[1]

	return domain, bus, slot, function, nil
}

// ListPCIDevicesForPassthrough 列出所有可直通的 PCI 设备
func ListPCIDevicesForPassthrough() ([]PCIDevice, error) {
	now := time.Now()
	passthroughCache.Lock()
	if !passthroughCache.refreshedAt.IsZero() && now.Sub(passthroughCache.refreshedAt) < passthroughDeviceCacheTTL {
		devices := clonePCIDevices(passthroughCache.devices)
		passthroughCache.Unlock()
		return devices, nil
	}

	// 已有旧快照时立即返回，并在后台刷新，避免慢扫描占用 HTTP 请求。
	if len(passthroughCache.devices) > 0 {
		devices := clonePCIDevices(passthroughCache.devices)
		if !passthroughCache.refreshing {
			passthroughCache.refreshing = true
			passthroughCache.wait = make(chan struct{})
			go refreshPassthroughDeviceCache(passthroughCache.wait)
		}
		passthroughCache.Unlock()
		return devices, nil
	}

	// 没有可返回的旧快照时复用当前扫描结果。扫描已被限制为单任务，
	// 因此只会有首个冷启动请求等待，不会产生并发子进程风暴或全局请求排队。
	if passthroughCache.refreshing {
		wait := passthroughCache.wait
		passthroughCache.Unlock()
		<-wait
		passthroughCache.Lock()
		devices := clonePCIDevices(passthroughCache.devices)
		err := passthroughCache.lastErr
		passthroughCache.Unlock()
		return devices, err
	}

	passthroughCache.refreshing = true
	passthroughCache.wait = make(chan struct{})
	wait := passthroughCache.wait
	passthroughCache.Unlock()

	return runPassthroughDeviceRefresh(wait)
}

// WarmupPassthroughDeviceCache 在服务启动后异步预热直通设备缓存。
// 预热不会阻塞 HTTP 服务启动，首个打开直通页面时通常可直接命中缓存。
func WarmupPassthroughDeviceCache() {
	passthroughCache.Lock()
	if passthroughCache.refreshing || len(passthroughCache.devices) > 0 {
		passthroughCache.Unlock()
		return
	}
	passthroughCache.refreshing = true
	passthroughCache.wait = make(chan struct{})
	wait := passthroughCache.wait
	passthroughCache.Unlock()
	go refreshPassthroughDeviceCache(wait)
}

// InvalidatePassthroughDeviceCache 使直通设备缓存立即进入后台刷新状态。
// 绑定、解绑、添加或移除设备后调用，下一次读取不会等待慢扫描。
func InvalidatePassthroughDeviceCache() {
	passthroughCache.Lock()
	passthroughCache.refreshedAt = time.Time{}
	if len(passthroughCache.devices) == 0 && !passthroughCache.refreshing {
		passthroughCache.refreshing = true
		passthroughCache.wait = make(chan struct{})
		wait := passthroughCache.wait
		passthroughCache.Unlock()
		go refreshPassthroughDeviceCache(wait)
		return
	}
	if !passthroughCache.refreshing && len(passthroughCache.devices) > 0 {
		passthroughCache.refreshing = true
		passthroughCache.wait = make(chan struct{})
		wait := passthroughCache.wait
		passthroughCache.Unlock()
		go refreshPassthroughDeviceCache(wait)
		return
	}
	passthroughCache.Unlock()
}

func runPassthroughDeviceRefresh(wait chan struct{}) ([]PCIDevice, error) {
	devices, err := scanPCIDevicesForPassthrough()
	passthroughCache.Lock()
	if err == nil {
		passthroughCache.devices = clonePCIDevices(devices)
		passthroughCache.refreshedAt = time.Now()
	} else if len(passthroughCache.devices) > 0 {
		// 后台刷新失败时短暂保留旧快照，避免每个请求都重新启动扫描。
		passthroughCache.refreshedAt = time.Now()
	}
	passthroughCache.lastErr = err
	passthroughCache.refreshing = false
	if passthroughCache.wait == wait {
		close(wait)
		passthroughCache.wait = nil
	}
	result := clonePCIDevices(passthroughCache.devices)
	passthroughCache.Unlock()
	return result, err
}

func refreshPassthroughDeviceCache(wait chan struct{}) {
	if _, err := runPassthroughDeviceRefresh(wait); err != nil {
		logger.App.Warn("后台刷新 PCI 直通设备缓存失败", "error", err)
	}
}

func clonePCIDevices(devices []PCIDevice) []PCIDevice {
	if len(devices) == 0 {
		return []PCIDevice{}
	}
	return append([]PCIDevice(nil), devices...)
}

// scanPCIDevicesForPassthrough 执行一次实际扫描。
// 优先使用单次 lspci + sysfs 读取，失败时回退到旧的 virsh 逐设备路径。
func scanPCIDevicesForPassthrough() ([]PCIDevice, error) {
	// 获取已绑定的 hostdev（用于标记占用情况）
	hostdevMap := buildHostDevUsageMap()
	fastDevices, fastErr := collectPCIDevicesFast()
	if fastErr == nil {
		devices := make([]PCIDevice, 0, len(fastDevices))
		for _, dev := range fastDevices {
			if !isPCIDevicePassthroughCapable(dev) {
				continue
			}
			if vmName, ok := hostdevMap[dev.PCIAddress]; ok {
				dev.IsUsedByVM = true
				dev.UsedByVMName = vmName
			}
			devices = append(devices, dev)
		}
		sort.Slice(devices, func(i, j int) bool { return devices[i].PCIAddress < devices[j].PCIAddress })
		return devices, nil
	}

	// lspci 不可用时回退到兼容路径，保证特殊发行版仍可读取设备。
	listResult := utils.ExecCommand("virsh", "nodedev-list", "--cap", "pci")
	if listResult.Error != nil {
		return nil, fmt.Errorf("获取 PCI 设备列表失败: %s（快速扫描失败: %v）", listResult.Stderr, fastErr)
	}

	var devices []PCIDevice
	for _, name := range strings.Split(listResult.Stdout, "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		dev, err := getPCIDeviceDetail(name)
		if err != nil {
			continue // 跳过无法读取详情的设备
		}

		// 过滤掉系统关键设备（Host bridge, ISA bridge, PCI bridge, SMBus, SATA, USB 控制器等）
		if !isPCIDevicePassthroughCapable(dev) {
			continue
		}

		// 检查 IOMMU 组
		dev.IOMMUGroup = getPCIIOMMUGroup(dev.PCIAddress)

		// 检查是否已被虚拟机使用
		if vmName, ok := hostdevMap[dev.PCIAddress]; ok {
			dev.IsUsedByVM = true
			dev.UsedByVMName = vmName
		}

		devices = append(devices, dev)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].PCIAddress < devices[j].PCIAddress })

	return devices, nil
}

// collectPCIDevicesFast 使用一次 lspci 和 sysfs 读取全部 PCI 设备信息，
// 避免对每个设备分别启动 virsh/lspci 子进程。
func collectPCIDevicesFast() (map[string]PCIDevice, error) {
	result := utils.ExecCommand("lspci", "-D", "-mm", "-nn")
	if result.Error != nil {
		return nil, fmt.Errorf("执行 lspci 快速扫描失败: %s", result.Stderr)
	}

	devices := make(map[string]PCIDevice)
	scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		address, fields, ok := parseLspciMachineLine(line)
		if !ok || address == "" || len(fields) < 3 {
			continue
		}
		className, _ := parseLspciNameAndID(fields[0])
		vendorName, _ := parseLspciNameAndID(fields[1])
		productName, _ := parseLspciNameAndID(fields[2])
		brackets := parseLspciBracketFields(line)
		classID := ""
		vendorID := ""
		productID := ""
		if len(brackets) > 0 {
			classID = brackets[0]
		}
		if len(brackets) > 1 {
			vendorID = brackets[1]
		}
		if len(brackets) > 2 {
			productID = brackets[2]
		}
		dev := PCIDevice{
			PCIAddress:  address,
			VendorID:    strings.ToLower(vendorID),
			VendorName:  vendorName,
			ProductID:   strings.ToLower(productID),
			ProductName: productName,
			ClassName:   className,
		}
		parts := parsePCIAddressFromString(address)
		dev.Domain = parts["domain"]
		dev.Bus = parts["bus"]
		dev.Slot = parts["slot"]
		dev.Function = parts["function"]
		if classCode := readPCIClassCode(address); classCode != "" {
			mappedClass := classCodeToName(classCode)
			// 关键桥接/芯片组设备保留 lspci 的英文类别，便于现有过滤逻辑精确匹配。
			if isCriticalPCIClass(className) {
				dev.ClassName = className
			} else if mappedClass != "未知设备" && !strings.HasPrefix(mappedClass, "PCI 设备(") {
				dev.ClassName = mappedClass
			}
		} else if classID != "" {
			dev.ClassName = classCodeToName(classID)
		}
		dev.DriverInUse = readPCIDriver(address)
		dev.IsVfioBound = dev.DriverInUse == "vfio-pci"
		dev.IOMMUGroup = readPCIIOMMUGroup(address)
		dev.IsPassthroughCapable = isPCIDevicePassthroughCapable(dev)
		devices[address] = dev
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 lspci 输出失败: %w", err)
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("lspci 未返回有效 PCI 设备")
	}
	return devices, nil
}

// parseLspciMachineLine 解析 lspci -mm 输出的一行。
func parseLspciMachineLine(line string) (string, []string, bool) {
	quote := strings.IndexByte(line, '"')
	if quote <= 0 {
		return "", nil, false
	}
	address := strings.TrimSpace(line[:quote])
	var fields []string
	for i := quote; i < len(line); {
		if line[i] != '"' {
			i++
			continue
		}
		start := i
		i++
		escaped := false
		for i < len(line) {
			if !escaped && line[i] == '"' {
				break
			}
			if !escaped && line[i] == '\\' {
				escaped = true
			} else {
				escaped = false
			}
			i++
		}
		if i >= len(line) {
			return "", nil, false
		}
		value, err := strconv.Unquote(line[start : i+1])
		if err != nil {
			return "", nil, false
		}
		fields = append(fields, value)
		i++
	}
	return address, fields, true
}

func parseLspciNameAndID(value string) (string, string) {
	idx := strings.LastIndex(value, " [")
	if idx < 0 || !strings.HasSuffix(value, "]") {
		return strings.TrimSpace(value), ""
	}
	return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx+2 : len(value)-1])
}

// parseLspciBracketFields 提取 lspci -nn 输出中的方括号字段。
func parseLspciBracketFields(line string) []string {
	fields := make([]string, 0, 3)
	for start := 0; start < len(line); {
		open := strings.IndexByte(line[start:], '[')
		if open < 0 {
			break
		}
		open += start
		close := strings.IndexByte(line[open+1:], ']')
		if close < 0 {
			break
		}
		close += open + 1
		value := strings.TrimSpace(line[open+1 : close])
		if value != "" {
			fields = append(fields, value)
		}
		start = close + 1
	}
	return fields
}

func isCriticalPCIClass(className string) bool {
	switch strings.ToLower(strings.TrimSpace(className)) {
	case "host bridge", "pci bridge", "isa bridge", "smbus", "memory controller":
		return true
	default:
		return false
	}
}

func readPCIClassCode(address string) string {
	data, err := os.ReadFile(filepath.Join("/sys/bus/pci/devices", address, "class"))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(string(data))), "0x")
}

func readPCIDriver(address string) string {
	link, err := os.Readlink(filepath.Join("/sys/bus/pci/devices", address, "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

func readPCIIOMMUGroup(address string) int {
	link, err := os.Readlink(filepath.Join("/sys/bus/pci/devices", address, "iommu_group"))
	if err != nil {
		return -1
	}
	value := filepath.Base(link)
	group := -1
	if _, err := fmt.Sscanf(value, "%d", &group); err != nil {
		return -1
	}
	return group
}

// GetVMPCIDevices 获取指定虚拟机直通的 PCI 设备
func GetVMPCIDevices(vmName string) ([]PCIDevice, error) {
	xmlResult := utils.ExecCommand("virsh", "dumpxml", vmName, "--inactive")
	if xmlResult.Error != nil {
		return nil, fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	var devices []PCIDevice
	xmlStr := xmlResult.Stdout

	// 解析 hostdev 元素
	lines := strings.Split(xmlStr, "\n")
	inHostDev := false
	inSource := false
	var currentDev PCIDevice
	var domain, bus, slot, function string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "<hostdev") {
			inHostDev = true
			inSource = false
			currentDev = PCIDevice{}
			domain = ""
			bus = ""
			slot = ""
			function = ""
		}
		if inHostDev {
			if strings.Contains(trimmed, "<source>") || strings.Contains(trimmed, "<source ") {
				inSource = true
			}
			if strings.Contains(trimmed, "</source>") {
				inSource = false
			}
			// 只解析 <source> 内部的物理设备地址，忽略来宾侧 <address type='pci'>
			if inSource {
				if strings.Contains(trimmed, "domain='") {
					domain = extractXMLAttr(trimmed, "domain='")
				}
				if strings.Contains(trimmed, "bus='") {
					bus = extractXMLAttr(trimmed, "bus='")
				}
				if strings.Contains(trimmed, "slot='") {
					slot = extractXMLAttr(trimmed, "slot='")
				}
				if strings.Contains(trimmed, "function='") {
					function = extractXMLAttr(trimmed, "function='")
				}
			}
			if strings.Contains(trimmed, "</hostdev>") {
				inHostDev = false
				if domain != "" && bus != "" && slot != "" && function != "" {
					currentDev.PCIAddress = formatPCIAddress(domain, bus, slot, function)
					parts := parsePCIAddressFromString(currentDev.PCIAddress)
					currentDev.Domain = parts["domain"]
					currentDev.Bus = parts["bus"]
					currentDev.Slot = parts["slot"]
					currentDev.Function = parts["function"]
					currentDev.IsUsedByVM = true
					currentDev.UsedByVMName = vmName
					devices = append(devices, currentDev)
				}
			}
		}
	}

	// 为已绑定的设备补充信息
	for i, dev := range devices {
		nodedevName := fmt.Sprintf("pci_%s_%s_%s_%s", dev.Domain, dev.Bus, dev.Slot, dev.Function)
		detail, err := getPCIDeviceDetail(nodedevName)
		if err == nil {
			devices[i].VendorID = detail.VendorID
			devices[i].ProductID = detail.ProductID
			devices[i].VendorName = detail.VendorName
			devices[i].ProductName = detail.ProductName
			devices[i].ClassName = detail.ClassName
			devices[i].DriverInUse = detail.DriverInUse
			devices[i].IsVfioBound = detail.IsVfioBound
			devices[i].IOMMUGroup = getPCIIOMMUGroup(dev.PCIAddress)
		}
	}

	return devices, nil
}

// AttachPCIDeviceToVM 将 PCI 设备直通到虚拟机
func AttachPCIDeviceToVM(vmName, pciAddress string) error {
	domain, bus, slot, function, err := parsePCIAddress(pciAddress)
	if err != nil {
		return err
	}

	// 验证设备是否已绑定 vfio-pci
	if !isDeviceVfioBound(pciAddress) {
		return fmt.Errorf("设备 %s 未绑定到 vfio-pci 驱动，请先绑定", pciAddress)
	}

	hostdevXML := fmt.Sprintf(hostdevXMLTemplate, domain, bus, slot, function)

	tmpFile := fmt.Sprintf("/tmp/_hostdev-%s.xml", vmName)
	if err := os.WriteFile(tmpFile, []byte(hostdevXML), 0644); err != nil {
		return fmt.Errorf("写入临时 XML 失败: %w", err)
	}
	defer os.Remove(tmpFile)

	result := utils.ExecCommand("virsh", "attach-device", vmName, tmpFile, "--config")
	if result.Error != nil {
		return fmt.Errorf("添加 PCI 直通设备失败: %s", result.Stderr)
	}
	if err := SyncVMPrimaryPassthroughDisplay(vmName); err != nil {
		rollback := utils.ExecCommand("virsh", "detach-device", vmName, tmpFile, "--config")
		if rollback.Error != nil {
			return fmt.Errorf("同步直通主显卡失败: %v；回滚新设备失败: %s", err, rollback.Stderr)
		}
		return fmt.Errorf("同步直通主显卡失败，已回滚新设备: %w", err)
	}

	RefreshVMCacheByNameAsync(vmName)
	InvalidatePassthroughDeviceCache()
	return nil
}

// DetachPCIDeviceFromVM 从虚拟机移除 PCI 直通设备
func DetachPCIDeviceFromVM(vmName, pciAddress string) error {
	domain, bus, slot, function, err := parsePCIAddress(pciAddress)
	if err != nil {
		return err
	}

	hostdevXML := fmt.Sprintf(hostdevXMLTemplate, domain, bus, slot, function)

	tmpFile := fmt.Sprintf("/tmp/_hostdev-detach-%s.xml", vmName)
	if err := os.WriteFile(tmpFile, []byte(hostdevXML), 0644); err != nil {
		return fmt.Errorf("写入临时 XML 失败: %w", err)
	}
	defer os.Remove(tmpFile)

	result := utils.ExecCommand("virsh", "detach-device", vmName, tmpFile, "--config")
	if result.Error != nil {
		return fmt.Errorf("移除 PCI 直通设备失败: %s", result.Stderr)
	}
	if err := SyncVMPrimaryPassthroughDisplay(vmName); err != nil {
		return fmt.Errorf("设备已移除，但同步直通主显卡配置失败: %w", err)
	}

	RefreshVMCacheByNameAsync(vmName)
	InvalidatePassthroughDeviceCache()
	return nil
}

// BindPCIDeviceToVfio 将 PCI 设备绑定到 vfio-pci 驱动
func BindPCIDeviceToVfio(pciAddress string) error {
	if isDeviceVfioBound(pciAddress) {
		return fmt.Errorf("设备 %s 已绑定到 vfio-pci", pciAddress)
	}

	iommuGroup := getPCIIOMMUGroup(pciAddress)
	if iommuGroup < 0 {
		return fmt.Errorf("无法确定设备 %s 的 IOMMU 组，请确认 IOMMU 已启用", pciAddress)
	}

	// 安全检查：仅当显示设备是宿主机的活动帧缓冲控制台时才拒绝绑定
	// 若存在多个 VGA 设备（如 BMC 显卡 + 核显），允许直通非控制台使用的 GPU
	classResult := utils.ExecShell(fmt.Sprintf(
		"cat /sys/bus/pci/devices/%s/class 2>/dev/null", pciAddress))
	classCode := strings.TrimPrefix(strings.TrimSpace(classResult.Stdout), "0x")
	if strings.HasPrefix(classCode, "03") {
		if isDeviceActiveFramebuffer(pciAddress) {
			return fmt.Errorf("设备 %s 是当前宿主机活动的帧缓冲控制台，直通会导致显示崩溃，已拒绝操作", pciAddress)
		}
		// 非活动控制台的 VGA 设备允许直通，但需要确保 vfio-pci 能接管
		logger.App.Info("检测到非活动控制台的显示设备，允许尝试 vfio-pci 绑定",
			"pci_address", pciAddress,
			"class_code", classCode)
	}

	vendorResult := utils.ExecShell(fmt.Sprintf("cat /sys/bus/pci/devices/%s/vendor 2>/dev/null", utils.ShellSingleQuote(pciAddress)))
	if vendorResult.Error != nil || strings.TrimSpace(vendorResult.Stdout) == "" {
		return fmt.Errorf("无法读取设备 %s 的厂商 ID", pciAddress)
	}
	deviceResult := utils.ExecShell(fmt.Sprintf("cat /sys/bus/pci/devices/%s/device 2>/dev/null", utils.ShellSingleQuote(pciAddress)))
	if deviceResult.Error != nil || strings.TrimSpace(deviceResult.Stdout) == "" {
		return fmt.Errorf("无法读取设备 %s 的设备 ID", pciAddress)
	}
	vendorID := strings.TrimSpace(vendorResult.Stdout)
	deviceID := strings.TrimSpace(deviceResult.Stdout)

	// 解绑原驱动（3秒超时，避免系统挂起卡死）
	currentDriverResult := utils.ExecShell(fmt.Sprintf(
		"readlink -f /sys/bus/pci/devices/%s/driver 2>/dev/null", pciAddress))
	currentDriver := ""
	if currentDriverResult.Error == nil {
		currentDriver = strings.TrimSpace(currentDriverResult.Stdout)
		parts := strings.Split(currentDriver, "/")
		currentDriver = parts[len(parts)-1]
	}

	if currentDriver != "" && currentDriver != "vfio-pci" {
		unbindResult := utils.ExecShellWithTimeout(
			fmt.Sprintf("echo '%s' > /sys/bus/pci/devices/%s/driver/unbind 2>/dev/null", pciAddress, pciAddress),
			5*time.Second)
		if unbindResult.Error != nil {
			return fmt.Errorf("从 %s 驱动解绑设备 %s 失败: （可能是设备正被系统使用）%s", currentDriver, pciAddress, unbindResult.Stderr)
		}
	}

	// 绑定到 vfio-pci（5秒超时）
	newIDResult := utils.ExecShellWithTimeout(
		fmt.Sprintf("echo %s > /sys/bus/pci/drivers/vfio-pci/new_id 2>/dev/null", utils.ShellSingleQuote(vendorID+" "+deviceID)),
		5*time.Second)
	_ = newIDResult

	bindResult := utils.ExecShellWithTimeout(
		fmt.Sprintf("echo '%s' > /sys/bus/pci/drivers/vfio-pci/bind 2>/dev/null", pciAddress),
		5*time.Second)
	if bindResult.Error != nil && !isDeviceVfioBound(pciAddress) {
		return fmt.Errorf("绑定 %s 到 vfio-pci 失败: %s", pciAddress, bindResult.Stderr)
	}

	// 可能需要一点时间让驱动生效
	time.Sleep(200 * time.Millisecond)

	if !isDeviceVfioBound(pciAddress) {
		return fmt.Errorf("绑定 %s 到 vfio-pci 未生效，可能设备被其他驱动占用", pciAddress)
	}
	InvalidatePassthroughDeviceCache()

	return nil
}

// UnbindPCIDeviceFromVfio 从 vfio-pci 驱动解绑 PCI 设备
func UnbindPCIDeviceFromVfio(pciAddress string) error {
	if !isDeviceVfioBound(pciAddress) {
		return fmt.Errorf("设备 %s 未绑定到 vfio-pci", pciAddress)
	}

	unbindResult := utils.ExecShell(fmt.Sprintf(
		"echo '%s' | tee /sys/bus/pci/drivers/vfio-pci/unbind 2>/dev/null", pciAddress))
	if unbindResult.Error != nil {
		return fmt.Errorf("从 vfio-pci 解绑设备 %s 失败: %s", pciAddress, unbindResult.Stderr)
	}

	// 触发设备重新探测
	utils.ExecShell(fmt.Sprintf("echo 1 | tee /sys/bus/pci/devices/%s/remove 2>/dev/null", utils.ShellSingleQuote(pciAddress)))
	utils.ExecShell("echo 1 | tee /sys/bus/pci/rescan 2>/dev/null")
	InvalidatePassthroughDeviceCache()

	return nil
}

// ValidatePCIPassthrough 验证 PCI 设备是否可直通
func ValidatePCIPassthrough(pciAddress string) error {
	// 检查 IOMMU 是否启用（多种方式，兼容 Intel/AMD/ARM 不同 sysfs 路径）
	iommuOk := false
	// 方法1: Intel IOMMU version 文件
	intelCheck := utils.ExecShellQuiet("cat /sys/class/iommu/*/intel-iommu/version 2>/dev/null")
	if intelCheck.Error == nil && strings.TrimSpace(intelCheck.Stdout) != "" {
		iommuOk = true
	}
	// 方法2: AMD IOMMU（可能有 version/cap/features 等文件）
	if !iommuOk {
		amdCheck := utils.ExecShellQuiet("ls /sys/class/iommu/*/amd-iommu/ 2>/dev/null | head -1")
		if amdCheck.Error == nil && strings.TrimSpace(amdCheck.Stdout) != "" {
			iommuOk = true
		}
	}
	// 方法3: IOMMU 组存在（内核已启用 IOMMU 的通用标志）
	if !iommuOk {
		groupsCheck := utils.ExecShellQuiet("ls /sys/kernel/iommu_groups/ 2>/dev/null | wc -l")
		if groupsCheck.Error == nil {
			count := strings.TrimSpace(groupsCheck.Stdout)
			if count != "" && count != "0" {
				iommuOk = true
			}
		}
	}
	// 方法4: dmesg 日志
	if !iommuOk {
		dmesgCheck := utils.ExecShellQuiet("dmesg 2>/dev/null | grep -qiE 'amd-vi|intel-iommu.*enabled|DMAR.*IOMMU' && echo ok || echo fail")
		if strings.TrimSpace(dmesgCheck.Stdout) == "ok" {
			iommuOk = true
		}
	}
	if !iommuOk {
		return fmt.Errorf("IOMMU 未启用，请确保 BIOS 已开启 Intel VT-d 或 AMD IOMMU，并在旧内核上添加 intel_iommu=on 或 amd_iommu=on 内核参数后重启")
	}

	// 检查 vfio-pci 驱动
	vfioCheck := utils.ExecShellQuiet("lsmod | grep -q vfio_pci && echo ok || echo fail")
	if strings.TrimSpace(vfioCheck.Stdout) != "ok" {
		return fmt.Errorf("vfio-pci 内核模块未加载，请执行 modprobe vfio-pci")
	}

	// 检查设备是否存在
	devCheck := utils.ExecShell(fmt.Sprintf("test -e /sys/bus/pci/devices/%s && echo ok || echo fail", utils.ShellSingleQuote(pciAddress)))
	if strings.TrimSpace(devCheck.Stdout) != "ok" {
		return fmt.Errorf("设备 %s 不存在", pciAddress)
	}

	return nil
}

// ==================== 辅助函数 ====================

// getPCIDeviceDetail 通过 virsh nodedev-dumpxml 获取 PCI 设备详情
func getPCIDeviceDetail(name string) (PCIDevice, error) {
	result := utils.ExecCommand("virsh", "nodedev-dumpxml", name)
	if result.Error != nil {
		return PCIDevice{}, result.Error
	}

	xmlStr := result.Stdout
	dev := PCIDevice{}

	// 解析 domain, bus, slot, function
	dev.PCIAddress = extractPCIAddressFromNodedev(xmlStr)
	if dev.PCIAddress != "" {
		parts := parsePCIAddressFromString(dev.PCIAddress)
		dev.Domain = parts["domain"]
		dev.Bus = parts["bus"]
		dev.Slot = parts["slot"]
		dev.Function = parts["function"]
	}

	// 解析 vendor ID 和 product ID
	dev.VendorID = extractXMLAttr(xmlStr, "vendor id='")
	dev.ProductID = extractXMLAttr(xmlStr, "product id='")

	// 通过 lspci 获取可读名称
	if dev.VendorID != "" && dev.ProductID != "" {
		dev.VendorName, dev.ProductName = getPCINames(dev.VendorID, dev.ProductID)
	}

	// 解析设备类别
	dev.ClassName = extractClassFromNodedev(xmlStr)

	// 检测当前驱动
	if dev.PCIAddress != "" {
		drvResult := utils.ExecShell(fmt.Sprintf(
			"readlink -f /sys/bus/pci/devices/%s/driver 2>/dev/null | xargs basename 2>/dev/null", dev.PCIAddress))
		dev.DriverInUse = strings.TrimSpace(drvResult.Stdout)
	}

	dev.IsVfioBound = isDeviceVfioBound(dev.PCIAddress)
	dev.IsPassthroughCapable = isPCIDevicePassthroughCapable(dev)

	return dev, nil
}

// extractPCIAddressFromNodedev 从 nodedev XML 提取 PCI 地址
func extractPCIAddressFromNodedev(xmlStr string) string {
	var domain, bus, slot, function string
	for _, line := range strings.Split(xmlStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<domain>") {
			domain = extractTagContent(trimmed, "domain")
		}
		if strings.HasPrefix(trimmed, "<bus>") {
			bus = extractTagContent(trimmed, "bus")
		}
		if strings.HasPrefix(trimmed, "<slot>") {
			slot = extractTagContent(trimmed, "slot")
		}
		if strings.HasPrefix(trimmed, "<function>") {
			function = extractTagContent(trimmed, "function")
		}
	}
	if domain == "" {
		domain = "0x0000"
	}
	return formatPCIAddressDecimal(domain, bus, slot, function)
}

// formatPCIAddress 将 libvirt XML 中常见的 0x 前缀地址统一为标准 PCI 地址。
func formatPCIAddress(domain, bus, slot, function string) string {
	d := parsePCIHexValue(domain)
	b := parsePCIHexValue(bus)
	s := parsePCIHexValue(slot)
	f := parsePCIHexValue(function)
	return fmt.Sprintf("%04x:%02x:%02x.%x", d, b, s, f)
}

// formatPCIAddressDecimal 处理 virsh nodedev XML 中以十进制标签表示的地址字段。
func formatPCIAddressDecimal(domain, bus, slot, function string) string {
	parse := func(value string) uint64 {
		value = strings.TrimSpace(strings.ToLower(value))
		base := 10
		if strings.HasPrefix(value, "0x") {
			value = strings.TrimPrefix(value, "0x")
			base = 16
		}
		parsed, err := strconv.ParseUint(value, base, 64)
		if err != nil && base == 10 {
			parsed, err = strconv.ParseUint(value, 16, 64)
		}
		if err != nil {
			return 0
		}
		return parsed
	}
	return fmt.Sprintf("%04x:%02x:%02x.%x", parse(domain), parse(bus), parse(slot), parse(function))
}

func parsePCIHexValue(value string) uint64 {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x")
	parsed, err := strconv.ParseUint(value, 16, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// extractTagContent 提取 XML 标签内容
func extractTagContent(line, tag string) string {
	start := fmt.Sprintf("<%s>", tag)
	end := fmt.Sprintf("</%s>", tag)
	if idx := strings.Index(line, start); idx >= 0 {
		content := line[idx+len(start):]
		if endIdx := strings.Index(content, end); endIdx >= 0 {
			return content[:endIdx]
		}
	}
	return ""
}

// extractXMLAttr 从 XML 提取属性值
func extractXMLAttr(content, attrPrefix string) string {
	idx := strings.Index(content, attrPrefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(attrPrefix)
	remaining := content[start:]
	endIdx := strings.Index(remaining, "'")
	if endIdx < 0 {
		return ""
	}
	val := remaining[:endIdx]
	val = strings.TrimPrefix(val, "0x")
	return strings.ToLower(val)
}

// extractClassFromNodedev 从 nodedev XML 提取设备类别
func extractClassFromNodedev(xmlStr string) string {
	for _, line := range strings.Split(xmlStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<class>") {
			code := extractTagContent(trimmed, "class")
			return classCodeToName(code)
		}
	}
	return ""
}

// classCodeToName 将 PCI 类别代码转换为可读名称
func classCodeToName(code string) string {
	code = strings.TrimPrefix(code, "0x")
	switch code {
	case "030000", "038000":
		return "VGA 显示控制器"
	case "030200":
		return "3D 控制器 / GPU"
	case "020000":
		return "以太网控制器"
	case "028000":
		return "网络控制器"
	case "010601", "010701":
		return "SATA/AHCI 控制器"
	case "010802":
		return "NVMe 控制器"
	case "040300":
		return "音频设备"
	case "0c0330":
		return "USB 3.0 控制器"
	case "0c0320":
		return "USB 2.0 控制器"
	default:
		if len(code) >= 4 {
			return fmt.Sprintf("PCI 设备(%s)", code)
		}
		return "未知设备"
	}
}

// parsePCIAddressFromString 解析 PCI 地址字符串
func parsePCIAddressFromString(addr string) map[string]string {
	result := map[string]string{"domain": "0000", "bus": "00", "slot": "00", "function": "0"}
	domain, bus, slot, function, err := parsePCIAddress(strings.TrimSpace(addr))
	if err == nil {
		result["domain"] = domain
		result["bus"] = bus
		result["slot"] = slot
		result["function"] = function
	}
	return result
}

// getPCINames 通过 lspci 数据库获取厂商和产品名称
func getPCINames(vendorID, productID string) (string, string) {
	vendorArgs := []string{"-d", vendorID + ":" + productID}
	if vendorID != "" && productID != "" {
		result := utils.ExecCommand("lspci", vendorArgs...)
		if result.Error == nil {
			fullName := strings.TrimSpace(result.Stdout)
			// lspci 输出格式: "04:00.0 VGA compatible controller: NVIDIA Corporation ..."
			parts := strings.SplitN(fullName, ":", 2)
			if len(parts) == 2 {
				desc := strings.TrimSpace(parts[1])
				descParts := strings.SplitN(desc, ": ", 2)
				if len(descParts) == 2 {
					return strings.TrimSpace(descParts[0]), strings.TrimSpace(descParts[1])
				}
				return "", desc
			}
		}
	}

	// 回退：只查询厂商名
	if vendorID != "" {
		result := utils.ExecCommand("lspci", "-n", "-d", vendorID+":*")
		if result.Error == nil && result.Stdout != "" {
			result2 := utils.ExecCommand("lspci", "-d", vendorID+":*")
			if result2.Error == nil {
				fullName := strings.TrimSpace(result2.Stdout)
				if idx := strings.LastIndex(fullName, ":"); idx >= 0 {
					desc := strings.TrimSpace(fullName[idx+1:])
					descParts := strings.SplitN(desc, ": ", 2)
					if len(descParts) == 2 {
						return strings.TrimSpace(descParts[0]), strings.TrimSpace(descParts[1])
					}
					return "", desc
				}
			}
		}
	}

	return "", ""
}

// getPCIIOMMUGroup 获取 PCI 设备的 IOMMU 组号
func getPCIIOMMUGroup(pciAddress string) int {
	// 探测类命令：设备无 IOMMU 分组属预期情况，失败仅记 DEBUG
	result := utils.ExecShellQuiet(fmt.Sprintf(
		"readlink /sys/bus/pci/devices/%s/iommu_group 2>/dev/null | xargs basename 2>/dev/null", pciAddress))
	if result.Error != nil || strings.TrimSpace(result.Stdout) == "" {
		return -1
	}
	num := 0
	fmt.Sscanf(strings.TrimSpace(result.Stdout), "%d", &num)
	return num
}

// isDeviceActiveFramebuffer 检查 PCI 显示设备是否为宿主机当前活动的帧缓冲控制台
// 只有被 fb0 使用的 GPU 才是关键显示设备，其他 GPU（如 BMC 服务器上的核显）可安全直通
func isDeviceActiveFramebuffer(pciAddress string) bool {
	// 获取当前活动的 framebuffer 控制台对应的 DRM card
	fbDRMPath := utils.ExecShell("readlink -f /sys/class/graphics/fb0/device/drm/card* 2>/dev/null")
	fbDRM := strings.TrimSpace(fbDRMPath.Stdout)
	if fbDRM == "" {
		// 没有 fb0，无法判断，保守拒绝
		return true
	}

	// 获取该 PCI 设备对应的 DRM card 路径
	pciDRMPath := utils.ExecShell(fmt.Sprintf(
		"readlink -f /sys/bus/pci/devices/%s/drm/card* 2>/dev/null", pciAddress))
	pciDRM := strings.TrimSpace(pciDRMPath.Stdout)
	if pciDRM == "" {
		// 该 PCI 设备没有 DRM 输出，不是帧缓冲设备，可以直通
		return false
	}

	// 比较两者的 DRM card 路径是否一致
	return fbDRM == pciDRM
}

// isDeviceVfioBound 检查设备是否已绑定到 vfio-pci
func isDeviceVfioBound(pciAddress string) bool {
	result := utils.ExecShell(fmt.Sprintf(
		"readlink -f /sys/bus/pci/devices/%s/driver 2>/dev/null | xargs basename 2>/dev/null", pciAddress))
	return strings.TrimSpace(result.Stdout) == "vfio-pci"
}

// isPCIDevicePassthroughCapable 判断 PCI 设备是否适合直通（非系统关键设备）
func isPCIDevicePassthroughCapable(dev PCIDevice) bool {
	lowerName := strings.ToLower(dev.ClassName)
	criticalClasses := []string{
		"host bridge", "pci bridge",
		"isa bridge", "smbus", "memory controller",
		"raid", "raid bus controller",
	}

	for _, critical := range criticalClasses {
		if lowerName == critical {
			return false
		}
	}

	// 排除系统关键驱动
	criticalDrivers := []string{
		"megaraid_sas", "ahci", "nvme", "xhci_hcd", "ehci-pci",
		"vmwgfx", // VMware 虚拟显卡（解除绑定会导致系统崩溃）
		"nvidia", // 物理 NVIDIA 驱动（需要先卸载后再绑定到 vfio-pci）
	}
	for _, drv := range criticalDrivers {
		if strings.ToLower(dev.DriverInUse) == drv {
			return false
		}
	}

	// 排除没有分类且无驱动的未知设备
	if dev.ClassName == "" && dev.DriverInUse == "" && !dev.IsVfioBound {
		return false
	}

	return true
}

// buildHostDevUsageMap 构建设备使用映射（PCI地址 -> VM名称）
func buildHostDevUsageMap() map[string]string {
	usageMap := make(map[string]string)

	// 获取所有虚拟机
	listResult := utils.ExecCommand("virsh", "list", "--all", "--name")
	if listResult.Error != nil {
		return usageMap
	}

	for _, vmName := range strings.Split(listResult.Stdout, "\n") {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" {
			continue
		}

		xmlResult := utils.ExecCommand("virsh", "dumpxml", vmName, "--inactive")
		if xmlResult.Error != nil {
			continue
		}

		// 解析 hostdev 中的 PCI 地址
		xmlStr := xmlResult.Stdout
		lines := strings.Split(xmlStr, "\n")
		var domain, bus, slot, function string
		inHostDev := false
		inSource := false

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "<hostdev") {
				inHostDev = true
				inSource = false
				domain = ""
				bus = ""
				slot = ""
				function = ""
			}
			if inHostDev {
				if strings.Contains(trimmed, "<source>") || strings.Contains(trimmed, "<source ") {
					inSource = true
				}
				if strings.Contains(trimmed, "</source>") {
					inSource = false
				}
				// 只解析 <source> 内部的物理设备地址
				if inSource {
					if strings.Contains(trimmed, "domain='") {
						domain = extractXMLAttr(trimmed, "domain='")
					}
					if strings.Contains(trimmed, "bus='") {
						bus = extractXMLAttr(trimmed, "bus='")
					}
					if strings.Contains(trimmed, "slot='") {
						slot = extractXMLAttr(trimmed, "slot='")
					}
					if strings.Contains(trimmed, "function='") {
						function = extractXMLAttr(trimmed, "function='")
					}
				}
				if strings.Contains(trimmed, "</hostdev>") {
					inHostDev = false
					if domain != "" && bus != "" && slot != "" && function != "" {
						addr := formatPCIAddress(domain, bus, slot, function)
						usageMap[addr] = vmName
					}
				}
			}
		}
	}

	return usageMap
}

// GenerateHostDevXML 生成 hostdev XML 片段
func GenerateHostDevXML(pciAddress string) (string, error) {
	domain, bus, slot, function, err := parsePCIAddress(pciAddress)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(hostdevXMLTemplate, domain, bus, slot, function), nil
}

// ApplyHostDevsToDomainXML 将 hostdev 设备应用到 domain XML
func ApplyHostDevsToDomainXML(xmlContent string, hostDevs []HostDeviceParam) (string, error) {
	if len(hostDevs) == 0 {
		return xmlContent, nil
	}

	var hostdevXMLs []string
	for _, hd := range hostDevs {
		xml, err := GenerateHostDevXML(hd.PCIAddress)
		if err != nil {
			return "", fmt.Errorf("生成 hostdev XML 失败 (%s): %w", hd.PCIAddress, err)
		}
		hostdevXMLs = append(hostdevXMLs, xml)
	}

	// 将 hostdev 插入到 </devices> 之前
	injectContent := strings.Join(hostdevXMLs, "\n")
	xmlContent = strings.Replace(xmlContent, "</devices>",
		"\n"+injectContent+"\n  </devices>", 1)

	return ApplyPrimaryPassthroughDisplayToDomainXML(xmlContent)
}

// ApplyPrimaryPassthroughDisplayToDomainXML 根据虚拟显示模型同步直通主显卡。
// none 是独立的无头模式；当且仅当 XML 中存在一张 PCI VGA 直通设备时，
// 自动为其设置 x-vga=true。多张 VGA 时拒绝猜测主卡。
func ApplyPrimaryPassthroughDisplayToDomainXML(xmlContent string) (string, error) {
	if vm_xml.ParseVMVideoModelFromDomainXML(xmlContent) != vm_xml.VMVideoModelNone {
		return vm_xml.ApplyPrimaryGPUXVGAToDomainXML(xmlContent, "")
	}

	var vgaDevices []string
	for _, pciAddress := range vm_xml.ParsePCIHostDeviceAddresses(xmlContent) {
		classPath := filepath.Join("/sys/bus/pci/devices", pciAddress, "class")
		classData, err := readPCIDeviceClass(classPath)
		if err != nil {
			continue
		}
		classCode := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(string(classData))), "0x")
		if strings.HasPrefix(classCode, "0300") {
			vgaDevices = append(vgaDevices, pciAddress)
		}
	}

	if len(vgaDevices) > 1 {
		return "", fmt.Errorf("虚拟显示设备为 none 时检测到多张直通 VGA，无法自动确定主显卡")
	}
	if len(vgaDevices) == 0 {
		return vm_xml.ApplyPrimaryGPUXVGAToDomainXML(xmlContent, "")
	}
	return vm_xml.ApplyPrimaryGPUXVGAToDomainXML(xmlContent, vgaDevices[0])
}

// SyncVMPrimaryPassthroughDisplay 将无头显示与直通 GPU 的 x-vga 配置同步到持久化 XML。
func SyncVMPrimaryPassthroughDisplay(vmName string) error {
	xmlResult := utils.ExecCommand("virsh", "dumpxml", vmName, "--inactive")
	if xmlResult.Error != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	updatedXML, err := ApplyPrimaryPassthroughDisplayToDomainXML(xmlResult.Stdout)
	if err != nil {
		return err
	}
	if updatedXML == xmlResult.Stdout {
		return nil
	}

	tmpFile, err := os.CreateTemp("", "qvm-primary-display-*.xml")
	if err != nil {
		return fmt.Errorf("创建临时 XML 失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.WriteString(updatedXML); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("写入临时 XML 失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("关闭临时 XML 失败: %w", err)
	}

	defineResult := utils.ExecCommand("virsh", "define", "--validate", tmpPath)
	if defineResult.Error != nil {
		return fmt.Errorf("应用直通主显卡配置失败: %s", defineResult.Stderr)
	}
	return nil
}

// EnsureVfioModuleLoaded 确保 vfio-pci 模块已加载
func EnsureVfioModuleLoaded() error {
	checkResult := utils.ExecShellQuiet("lsmod | grep -q vfio_pci && echo ok || echo fail")
	if strings.TrimSpace(checkResult.Stdout) == "ok" {
		return nil
	}

	loadResult := utils.ExecCommand("modprobe", "vfio-pci")
	if loadResult.Error != nil {
		return fmt.Errorf("加载 vfio-pci 模块失败: %s", loadResult.Stderr)
	}
	return nil
}

// IsDeviceVfioBound 公共导出函数，检查设备是否已绑定到 vfio-pci
func IsDeviceVfioBound(pciAddress string) bool {
	return isDeviceVfioBound(pciAddress)
}
