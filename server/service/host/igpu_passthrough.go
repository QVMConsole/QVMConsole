package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvm_console/service/pci"
	"kvm_console/utils"
)

// PassthroughDeviceInfo 可用于直通的设备信息
type PassthroughDeviceInfo struct {
	PCIAddress          string `json:"pci_address"`           // PCI 地址，如 0000:00:02.0
	VendorID            string `json:"vendor_id"`             // 厂商 ID
	VendorName          string `json:"vendor_name"`           // 厂商名称
	ProductID           string `json:"product_id"`            // 产品 ID
	ProductName         string `json:"product_name"`          // 产品名称
	CurrentDriver       string `json:"current_driver"`        // 当前使用的驱动
	IsVFIOBound         bool   `json:"is_vfio_bound"`         // 是否已绑定到 vfio-pci
	IsActiveFramebuffer bool   `json:"is_active_framebuffer"` // 是否为当前活动帧缓冲（不可直通）
	IOMMUGroup          int    `json:"iommu_group"`           // IOMMU 组号，-1 表示无
}

// HardwarePassthroughStatus 硬件直通状态
type HardwarePassthroughStatus struct {
	// IOMMU 状态
	IommuEnabled bool   `json:"iommu_enabled"` // IOMMU 是否已在 OS 层启用
	IommuType    string `json:"iommu_type"`    // intel / amd / none
	// BIOS/UEFI 层面的 IOMMU 状态
	BiosIommuEnabled bool   `json:"bios_iommu_enabled"` // BIOS 是否开启了 IOMMU（VT-d / AMD-Vi）
	BiosIommuMessage string `json:"bios_iommu_message"` // BIOS 检测详情
	// CPU 虚拟化支持
	CpuVirtFlag string `json:"cpu_virt_flag"` // vmx (Intel) 或 svm (AMD) 或空
	// vfio-pci 模块状态
	VfioPCILoaded bool `json:"vfio_pci_loaded"` // vfio-pci 模块是否已加载
	// 内核启动参数中是否含有 iommu 参数
	IommuInCmdline bool `json:"iommu_in_cmdline"` // 内核启动参数中是否包含 iommu=on
	// 可直通设备列表
	PassthroughDevices []PassthroughDeviceInfo `json:"passthrough_devices"` // 发现的可直通设备
	// 总体评估
	Ready        bool   `json:"ready"`         // 硬件直通条件是否满足
	ReadyMessage string `json:"ready_message"` // 状态描述
	// 架构信息
	Architecture string `json:"architecture"` // x86_64 / aarch64
}

// GetHardwarePassthroughStatus 获取硬件直通状态
func GetHardwarePassthroughStatus() *HardwarePassthroughStatus {
	status := &HardwarePassthroughStatus{
		IommuEnabled:       false,
		IommuType:          "none",
		VfioPCILoaded:      false,
		IommuInCmdline:     false,
		PassthroughDevices: []PassthroughDeviceInfo{},
		Ready:              false,
		ReadyMessage:       "",
	}

	// 1. 检测架构
	archResult := utils.ExecShellQuiet("uname -m")
	status.Architecture = strings.TrimSpace(archResult.Stdout)

	// 2. 检测 CPU 虚拟化支持
	status.CpuVirtFlag = detectCPUVirtFlag()

	// 3. 检测 BIOS/UEFI 是否开启了 IOMMU
	status.BiosIommuEnabled, status.BiosIommuMessage = detectBiosIommu()

	// 4. 检测 IOMMU 是否在 OS 层启用
	status.IommuEnabled, status.IommuType = detectIOMMU()

	// 5. 检测内核启动参数中是否包含 iommu
	status.IommuInCmdline = detectIommuInCmdline()

	// 6. 检测 vfio-pci 模块
	vfioCheck := utils.ExecShellQuiet("lsmod | grep -q vfio_pci && echo ok || echo fail")
	status.VfioPCILoaded = strings.TrimSpace(vfioCheck.Stdout) == "ok"

	// 7. 检测可直通设备（VGA 兼容控制器 / 显示控制器）
	status.PassthroughDevices = detectPassthroughDevices()

	// 8. 总体评估
	if !status.BiosIommuEnabled {
		status.ReadyMessage = status.BiosIommuMessage
	} else if !status.IommuEnabled {
		status.ReadyMessage = "BIOS 已开启 IOMMU，但 OS 层未激活。请在宿主机内核启动参数中添加 " + iommuParamHint(status.Architecture) + " 并重启宿主机。"
	} else if !status.VfioPCILoaded {
		status.ReadyMessage = "vfio-pci 内核模块未加载，请执行 modprobe vfio-pci"
	} else if len(status.PassthroughDevices) == 0 {
		status.ReadyMessage = "未检测到可用于直通的设备"
	} else {
		readyCount := 0
		for _, dev := range status.PassthroughDevices {
			if dev.IsVFIOBound {
				readyCount++
			}
		}
		if readyCount > 0 {
			status.Ready = true
			status.ReadyMessage = "硬件直通已就绪"
		} else {
			status.Ready = true
			status.ReadyMessage = "IOMMU 和 vfio-pci 已就绪，可在硬件直通页面将设备绑定到 vfio-pci"
		}
	}

	return status
}

// detectCPUVirtFlag 检测 CPU 虚拟化标志位
func detectCPUVirtFlag() string {
	// Intel: vmx, AMD: svm
	intelCheck := utils.ExecShellQuiet("grep -qc vmx /proc/cpuinfo 2>/dev/null && echo vmx || echo ''")
	if v := strings.TrimSpace(intelCheck.Stdout); v == "vmx" {
		return "vmx"
	}
	amdCheck := utils.ExecShellQuiet("grep -qc svm /proc/cpuinfo 2>/dev/null && echo svm || echo ''")
	if v := strings.TrimSpace(amdCheck.Stdout); v == "svm" {
		return "svm"
	}
	return ""
}

// detectBiosIommu 检测 BIOS/UEFI 是否开启了 IOMMU（VT-d / AMD-Vi）
// 通过分析 dmesg 日志判断 BIOS 是否已将 IOMMU 硬件暴露给操作系统
func detectBiosIommu() (enabled bool, message string) {
	// 方法1: 检查 dmesg 中是否有 BIOS IOMMU 相关消息
	dmesgOut := utils.ExecShellQuiet("dmesg 2>/dev/null | grep -iE 'DMAR|AMD-Vi|IOMMU|VT-d' | head -40")
	dmesgStr := dmesgOut.Stdout

	// ── Intel VT-d 检测 ──
	// BIOS 已开启: "DMAR: IOMMU enabled"
	if strings.Contains(dmesgStr, "DMAR: IOMMU enabled") {
		return true, "BIOS 已开启 Intel VT-d，DMAR ACPI 表已正确传递给操作系统。"
	}
	// BIOS 未开启或 ACPI 表缺失: "DMAR: IOMMU disabled" 或 "No DMAR table found"
	if strings.Contains(dmesgStr, "DMAR: IOMMU disabled") ||
		strings.Contains(dmesgStr, "No DMAR table") {
		return false, "Intel VT-d 已在 BIOS 中关闭或 ACPI DMAR 表缺失。请进入 BIOS 设置，在 Advanced → CPU Configuration 或 Chipset 中找到 VT-d / IOMMU 选项并开启，然后重启宿主机。"
	}

	// ── AMD-Vi 检测 ──
	// BIOS 已开启: "AMD-Vi: Using" (如 "AMD-Vi: Using global IVHD...")
	if strings.Contains(dmesgStr, "AMD-Vi: Using") {
		return true, "BIOS 已开启 AMD-Vi，IVRS ACPI 表已正确传递给操作系统。"
	}
	// BIOS 未开启: "AMD-Vi: Disabled"
	if strings.Contains(dmesgStr, "AMD-Vi: Disabled") {
		return false, "AMD-Vi 已在 BIOS 中关闭。请进入 BIOS 设置，在 Advanced → AMD CBS 或 NBIO Common Options 中找到 IOMMU 选项并开启，然后重启宿主机。"
	}

	// ── 通用 IOMMU 检测 ──
	if strings.Contains(dmesgStr, "IOMMU: disabled by BIOS") ||
		strings.Contains(dmesgStr, "iommu: Disabled") {
		return false, "IOMMU 已被 BIOS 禁用。请进入 BIOS/UEFI 设置，找到 Intel VT-d 或 AMD IOMMU 选项并开启。"
	}

	// ── ARM SMMU 检测 ──
	if strings.Contains(dmesgStr, "arm-smmu") {
		return true, "ARM SMMU 已探测到，BIOS 已正确配置 IOMMU。"
	}

	// ── 降级判断：有 IOMMU 组但没有明确 BIOS 消息 ──
	// 可能是旧内核或嵌入式场景，此时如果已有 iommu_groups 就认为 BIOS OK
	groupsCheck := utils.ExecShellQuiet("ls /sys/kernel/iommu_groups/ 2>/dev/null | wc -l")
	if groupsCheck.Error == nil {
		count := strings.TrimSpace(groupsCheck.Stdout)
		if count != "" && count != "0" {
			return true, "IOMMU 组已存在（" + count + " 个），BIOS 已正确配置。"
		}
	}

	// ── 无法确定 ──
	// 有 CPU 虚拟化标志但无 IOMMU 消息 → BIOS 可能关了 IOMMU
	cpuVirt := detectCPUVirtFlag()
	if cpuVirt != "" {
		return false, "CPU 支持虚拟化（" + cpuVirt + "），但未检测到 BIOS IOMMU 相关消息。请进入 BIOS/UEFI 设置：Intel 平台找到 VT-d 选项，AMD 平台找到 IOMMU 选项，确认已开启。"
	}

	// 完全无法判断
	return false, "无法判断 BIOS IOMMU 状态。请手动确认 BIOS 中 Intel VT-d 或 AMD IOMMU 是否已开启，以及内核启动参数是否包含 intel_iommu=on 或 amd_iommu=on。"
}

// detectIOMMU 检测 IOMMU 是否启用及类型
func detectIOMMU() (enabled bool, iommuType string) {
	// 方法1: 通过 /sys/class/iommu 检测 Intel
	intelCheck := utils.ExecShellQuiet("cat /sys/class/iommu/*/intel-iommu/version 2>/dev/null")
	if intelCheck.Error == nil && strings.TrimSpace(intelCheck.Stdout) != "" {
		return true, "intel"
	}
	// 方法2: AMD IOMMU — 检查 amd-iommu 目录是否存在（不一定有 version 文件）
	amdCheck := utils.ExecShellQuiet("ls /sys/class/iommu/*/amd-iommu/ 2>/dev/null | head -1")
	if amdCheck.Error == nil && strings.TrimSpace(amdCheck.Stdout) != "" {
		return true, "amd"
	}

	// 方法3: 通过 /sys/kernel/iommu_groups 检测（通用兜底）
	groupsCheck := utils.ExecShellQuiet("ls /sys/kernel/iommu_groups/ 2>/dev/null | wc -l")
	if groupsCheck.Error == nil && strings.TrimSpace(groupsCheck.Stdout) != "0" {
		return true, "unknown"
	}

	// 方法4: 通过 dmesg 检测
	dmesgCheck := utils.ExecShellQuiet("dmesg 2>/dev/null | grep -qiE 'amd-vi|intel-iommu.*enabled|DMAR.*IOMMU' && echo ok || echo fail")
	if strings.TrimSpace(dmesgCheck.Stdout) == "ok" {
		return true, "unknown"
	}

	return false, "none"
}

// detectIommuInCmdline 检测内核启动参数中是否有 iommu 参数
func detectIommuInCmdline() bool {
	cmdline := utils.ExecShellQuiet("cat /proc/cmdline 2>/dev/null")
	return strings.Contains(strings.ToLower(cmdline.Stdout), "iommu")
}

// iommuParamHint 返回对应架构的 IOMMU 内核参数提示
func iommuParamHint(arch string) string {
	switch arch {
	case "aarch64":
		return "iommu.passthrough=1（ARM 架构通常默认启用 IOMMU/SMMU，无需额外参数）"
	default:
		return "intel_iommu=on（Intel CPU）或 amd_iommu=on（AMD CPU）"
	}
}

// isActiveFramebuffer 检测 PCI 设备是否为当前活动的帧缓冲控制台
func isActiveFramebuffer(pciAddress string) bool {
	// 检查 /sys/devices/pci.../.../graphics/fb0 是否存在
	check := utils.ExecShellQuiet("ls /sys/bus/pci/devices/" + pciAddress + "/graphics/fb0 2>/dev/null")
	return check.Error == nil && strings.TrimSpace(check.Stdout) != ""
}

// detectPassthroughDevices 检测可用于直通的显示设备
func detectPassthroughDevices() []PassthroughDeviceInfo {
	var devices []PassthroughDeviceInfo

	// 使用 lspci 列出所有 VGA/Display 控制器
	result := utils.ExecShellQuiet("lspci -D -mm -n 2>/dev/null | grep -E '0300|0380'")
	if result.Error != nil {
		return devices
	}

	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// lspci -Dmmn 格式: pci_address class vendor_id product_id ...
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pciAddr := fields[0]
		classCode := strings.Trim(fields[1], "\"") // lspci -mm 带引号，如 "0300"
		vendorID := strings.Trim(fields[2], "\"")
		productID := strings.Trim(fields[3], "\"")

		// 只处理显示类设备
		if classCode != "0300" && classCode != "0380" {
			continue
		}

		// 获取可读名称
		vendorName, productName := getPCIDeviceNames(vendorID, productID)

		// 获取当前驱动
		drvResult := utils.ExecShellQuiet("readlink -f /sys/bus/pci/devices/" + pciAddr + "/driver 2>/dev/null | xargs basename 2>/dev/null")
		currentDriver := strings.TrimSpace(drvResult.Stdout)

		// 过滤掉非核显设备（虚拟设备等）
		if isVirtualGPU(currentDriver) {
			continue
		}

		dev := PassthroughDeviceInfo{
			PCIAddress:          pciAddr,
			VendorID:            vendorID,
			VendorName:          vendorName,
			ProductID:           productID,
			ProductName:         productName,
			CurrentDriver:       currentDriver,
			IsVFIOBound:         currentDriver == "vfio-pci",
			IsActiveFramebuffer: isActiveFramebuffer(pciAddr),
			IOMMUGroup:          getIOMMUGroupID(pciAddr),
		}

		devices = append(devices, dev)
	}

	return devices
}

// isVirtualGPU 检查驱动是否为虚拟 GPU 或 BMC 管理显卡（如 virtio-gpu, vmwgfx, ASPEED 等）。
// 判定表与硬件直通列表共用 pci 包，避免两处结论不一致。
func isVirtualGPU(driver string) bool {
	return pci.IsVirtualOrBMCGPUDriver(driver)
}

// getPCIDeviceNames 通过 lspci 获取设备可读名称
func getPCIDeviceNames(vendorID, productID string) (vendorName, productName string) {
	// 使用 lspci 的 PCI ID 数据库查询
	result := utils.ExecShellQuiet("lspci -d " + vendorID + ":" + productID + " 2>/dev/null | head -1")
	if result.Error == nil {
		line := strings.TrimSpace(result.Stdout)
		if line != "" {
			// 格式: pci_address Class Vendor Product: ...
			// 提取 Vendor 和 Product 部分
			parts := strings.SplitN(line, " ", 3)
			if len(parts) >= 3 {
				descParts := strings.SplitN(parts[2], ": ", 2)
				if len(descParts) >= 1 {
					vendorName = descParts[0]
				}
				if len(descParts) >= 2 {
					productName = descParts[1]
				}
			}
		}
	}
	return vendorName, productName
}

// getIOMMUGroupID 获取设备的 IOMMU 组号
func getIOMMUGroupID(pciAddress string) int {
	result := utils.ExecShellQuiet("readlink /sys/bus/pci/devices/" + pciAddress + "/iommu_group 2>/dev/null | xargs basename 2>/dev/null")
	if result.Error != nil || strings.TrimSpace(result.Stdout) == "" {
		return -1
	}
	groupStr := strings.TrimSpace(result.Stdout)
	var groupID int
	if _, err := fmt.Sscanf(groupStr, "%d", &groupID); err != nil {
		return -1
	}
	return groupID
}

// ────────────────────────────────────────────────────
// 一键开启功能
// ────────────────────────────────────────────────────

// IommuEnableResult IOMMU 启用操作结果
type IommuEnableResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	IommuType   string `json:"iommu_type"`   // intel / amd
	ParamAdded  string `json:"param_added"`  // 添加的内核参数
	NeedReboot  bool   `json:"need_reboot"`  // 是否需要重启
	GrubUpdated bool   `json:"grub_updated"` // 是否更新了 grub
}

// VfioLoadResult vfio-pci 加载结果
type VfioLoadResult struct {
	Success           bool   `json:"success"`
	Message           string `json:"message"`
	ModuleLoaded      bool   `json:"module_loaded"`      // 当前是否加载成功
	PersistConfigured bool   `json:"persist_configured"` // 是否配置了开机自动加载
}

// EnableIommuInGrub 一键开启 IOMMU：写入内核参数并 update-grub
func EnableIommuInGrub() *IommuEnableResult {
	result := &IommuEnableResult{
		Success:    false,
		NeedReboot: true,
	}

	grubPath := "/etc/default/grub"

	// 1. 检测当前 IOMMU 类型
	_, iommuType := detectIOMMU()
	result.IommuType = iommuType

	// 2. 确定要添加的内核参数
	var param string
	switch iommuType {
	case "intel":
		param = "intel_iommu=on"
	case "amd":
		param = "amd_iommu=on"
	default:
		// 无法自动确定类型，尝试通过 CPU 标志猜测
		arch := detectCPUVirtFlag()
		switch arch {
		case "vmx":
			param = "intel_iommu=on"
			result.IommuType = "intel"
		case "svm":
			param = "amd_iommu=on"
			result.IommuType = "amd"
		default:
			result.Message = "无法自动识别 IOMMU 类型（Intel VT-d / AMD-Vi），请手动确认。"
			return result
		}
	}
	result.ParamAdded = param

	// 3. 检查 grub 文件是否存在
	if _, err := os.Stat(grubPath); os.IsNotExist(err) {
		result.Message = "未找到 /etc/default/grub 文件，该系统可能使用非 GRUB 引导方式（如 systemd-boot），请手动配置内核参数。"
		return result
	}

	// 4. 读取 grub 配置
	grubData, err := os.ReadFile(grubPath)
	if err != nil {
		result.Message = "读取 /etc/default/grub 失败: " + err.Error()
		return result
	}

	content := string(grubData)

	// 5. 检查是否已存在 iommu 参数
	if strings.Contains(content, param) {
		result.Success = true
		result.Message = "内核参数 " + param + " 已存在于 GRUB 配置中，无需重复添加。请重启宿主机使 IOMMU 生效。"
		return result
	}

	// 检查是否已有同类型参数（如已有 intel_iommu=off）
	if iommuType == "intel" && strings.Contains(content, "intel_iommu=") {
		result.Message = "GRUB 中已存在 intel_iommu 参数，但不是 'on'。请手动检查 /etc/default/grub 中的 GRUB_CMDLINE_LINUX 配置。"
		return result
	}
	if iommuType == "amd" && strings.Contains(content, "amd_iommu=") {
		result.Message = "GRUB 中已存在 amd_iommu 参数，但不是 'on'。请手动检查 /etc/default/grub 中的 GRUB_CMDLINE_LINUX 配置。"
		return result
	}

	// 6. 修改 GRUB_CMDLINE_LINUX
	newContent := injectKernelParam(content, param)
	if newContent == content {
		result.Message = "无法修改 GRUB 配置：未找到 GRUB_CMDLINE_LINUX 行"
		return result
	}

	// 7. 备份原文件
	backupPath := grubPath + ".qvmconsole.bak"
	if err := os.WriteFile(backupPath, grubData, 0644); err != nil {
		result.Message = "备份 grub 文件失败: " + err.Error()
		return result
	}

	// 8. 写入新配置
	if err := os.WriteFile(grubPath, []byte(newContent), 0644); err != nil {
		result.Message = "写入 grub 文件失败: " + err.Error()
		return result
	}

	// 9. 更新 grub
	grubUpdated := false
	// 优先使用 update-grub (Debian/Ubuntu)
	updateResult := utils.ExecShellQuiet("which update-grub 2>/dev/null && update-grub 2>&1 || echo 'no-update-grub'")
	if !strings.Contains(updateResult.Stdout, "no-update-grub") {
		if updateResult.Error == nil {
			grubUpdated = true
		}
	}
	// 其次尝试 grub2-mkconfig (RHEL/CentOS/Fedora)
	if !grubUpdated {
		mkconfigResult := utils.ExecShellQuiet("grub2-mkconfig -o /boot/grub2/grub.cfg 2>&1 || grub2-mkconfig -o /boot/grub/grub.cfg 2>&1")
		if mkconfigResult.Error == nil {
			grubUpdated = true
		}
	}
	result.GrubUpdated = grubUpdated

	if grubUpdated {
		result.Success = true
		result.Message = "已添加内核参数 " + param + " 并更新了 GRUB 配置（备份: " + backupPath + "）。请重启宿主机使 IOMMU 生效。"
	} else {
		result.Success = true
		result.Message = "已添加内核参数 " + param + " 到 GRUB 配置（备份: " + backupPath + "）。但自动更新 GRUB 失败，请手动执行 update-grub 或 grub2-mkconfig 后重启。"
	}

	return result
}

// injectKernelParam 向 GRUB_CMDLINE_LINUX 注入内核参数
func injectKernelParam(content, param string) string {
	lines := strings.Split(content, "\n")
	var newLines []string
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "GRUB_CMDLINE_LINUX=") {
			found = true
			// 提取引号内的内容
			rest := trimmed[len("GRUB_CMDLINE_LINUX="):]
			quote := byte(0)
			if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
				quote = rest[0]
				rest = rest[1:]
			}
			// 去掉末尾引号
			if quote != 0 && len(rest) > 0 && rest[len(rest)-1] == quote {
				rest = rest[:len(rest)-1]
			}

			// 添加新参数
			newValue := strings.TrimSpace(rest)
			if newValue == "" {
				newValue = param
			} else if !strings.Contains(newValue, param) {
				newValue = newValue + " " + param
			}

			// 重新构建该行
			if quote != 0 {
				newLines = append(newLines, "GRUB_CMDLINE_LINUX="+string(quote)+newValue+string(quote))
			} else {
				newLines = append(newLines, "GRUB_CMDLINE_LINUX="+newValue)
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	if !found {
		// 没有 GRUB_CMDLINE_LINUX 行，添加一行
		newLines = append(newLines, "GRUB_CMDLINE_LINUX=\""+param+"\"")
	}

	return strings.Join(newLines, "\n")
}

// LoadVfioPciModule 一键加载 vfio-pci 模块并配置开机自动加载
func LoadVfioPciModule() *VfioLoadResult {
	result := &VfioLoadResult{}

	// 1. 检查是否已加载
	checkResult := utils.ExecShellQuiet("lsmod | grep -q vfio_pci && echo loaded || echo not_loaded")
	alreadyLoaded := strings.TrimSpace(checkResult.Stdout) == "loaded"

	if alreadyLoaded {
		result.ModuleLoaded = true
	} else {
		// 2. modprobe 加载模块
		modResult := utils.ExecCommand("modprobe", "vfio-pci")
		if modResult.Error != nil {
			result.Message = "加载 vfio-pci 模块失败: " + modResult.Stderr
			return result
		}
		result.ModuleLoaded = true
	}

	// 3. 配置开机自动加载
	modulesDir := "/etc/modules-load.d"
	confPath := filepath.Join(modulesDir, "vfio-pci.conf")

	// 确保目录存在
	if err := os.MkdirAll(modulesDir, 0755); err != nil {
		result.Message = "创建 " + modulesDir + " 目录失败: " + err.Error()
		return result
	}

	// 检查文件是否已存在
	existingData, _ := os.ReadFile(confPath)
	if strings.Contains(string(existingData), "vfio-pci") {
		result.PersistConfigured = true
		result.Success = true
		result.Message = "vfio-pci 模块已加载并已配置开机自动加载。"
		return result
	}

	// 写入配置文件
	content := "# QVMConsole auto-generated - vfio-pci for PCI passthrough\nvfio-pci\n"
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		result.Message = "写入 " + confPath + " 失败: " + err.Error()
		return result
	}

	result.PersistConfigured = true
	result.Success = true
	if alreadyLoaded {
		result.Message = "vfio-pci 模块已加载，已配置开机自动加载到 " + confPath + "。"
	} else {
		result.Message = "vfio-pci 模块已加载成功，并配置开机自动加载到 " + confPath + "。"
	}

	return result
}
