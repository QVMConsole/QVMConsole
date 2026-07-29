package vm

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/go-libvirt"

	"kvm_console/logger"
	"kvm_console/service/ip_resolver"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/vm/memory"
	"kvm_console/utils"
)

// ==================== 虚拟机列表查询 ====================

// ListVMs 列出所有虚拟机
func ListVMs(options ...VMListOptions) ([]VmInfo, error) {
	listOptions := VMListOptions{}
	if len(options) > 0 {
		listOptions = options[0]
	}

	// 获取所有虚拟机名称
	var names []string
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		if domains, err := libvirt_rpc.ListAllDomainsRPC(); err == nil {
			for _, dom := range domains {
				names = append(names, dom.Name)
			}
		} else {
			logger.Libvirt.Warn("ListAllDomainsRPC 失败，降级为 virsh", "error", err)
		}
	}
	if len(names) == 0 {
		result := utils.ExecCommand("virsh", "list", "--all", "--name")
		if result.Error != nil {
			// 维护模式下 libvirtd 可能已被主动停用，此时列表接口降级为空列表，
			// 避免前端不断弹出连接 hypervisor 失败的错误。
			if D.IsMaintenanceModeEnabled() && (D.IsLibvirtUnavailableText(result.Stderr) || D.IsLibvirtUnavailableError(result.Error)) {
				return []VmInfo{}, nil
			}
			return nil, result.Error
		}
		names = strings.Split(result.Stdout, "\n")
	}

	// 提前批量获取所有虚拟机的创建时间
	statMap := make(map[string]string)
	xmlFiles, err := filepath.Glob("/etc/libvirt/qemu/*.xml")
	if err == nil {
		for _, xmlPath := range xmlFiles {
			pathParts := strings.Split(xmlPath, "/")
			fileName := pathParts[len(pathParts)-1]
			vmName := strings.TrimSuffix(fileName, ".xml")
			if ts := utils.GetFileCreateTime(xmlPath); ts > 0 {
				statMap[vmName] = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
			}
		}
	}

	var vms []VmInfo

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		vm := VmInfo{
			Name:      name,
			CreatedAt: statMap[name],
		}

		// 获取状态
		if libvirt_rpc.IsLibvirtRPCAvailable() {
			if state, err := libvirt_rpc.GetDomainStateRPC(name); err == nil {
				vm.Status = state
			} else {
				logger.Libvirt.Warn("GetDomainStateRPC 失败，降级为 virsh", "domain", name, "error", err)
			}
		}
		if vm.Status == "" {
			stateResult := utils.ExecCommand("virsh", "domstate", name)
			if stateResult.Error == nil {
				vm.Status = strings.TrimSpace(stateResult.Stdout)
			}
		}
		UpdateVMRuntimeState(name, vm.Status, time.Now())

		// 获取基本信息（从 dominfo）
		if libvirt_rpc.IsLibvirtRPCAvailable() {
			if vcpu, maxMemKB, usedMemKB, autostart, err := libvirt_rpc.GetDomainInfoRPC(name); err == nil {
				vm.VCPU = vcpu
				vm.MaxMemory = int(maxMemKB) / 1024 // KiB -> MB
				vm.Memory = int(usedMemKB) / 1024   // KiB -> MB
				vm.Autostart = autostart
			} else {
				logger.Libvirt.Warn("GetDomainInfoRPC 失败，降级为 virsh", "domain", name, "error", err)
			}
		}
		if vm.VCPU == 0 {
			infoResult := utils.ExecCommand("virsh", "dominfo", name)
			if infoResult.Error == nil {
				vm.VCPU = parseInfoInt(infoResult.Stdout, "CPU(s):")
				maxMem := parseInfoInt(infoResult.Stdout, "Max memory:")
				vm.MaxMemory = maxMem / 1024 // KiB -> MB
				usedMem := parseInfoInt(infoResult.Stdout, "Used memory:")
				vm.Memory = usedMem / 1024 // KiB -> MB
				vm.Autostart = strings.Contains(infoResult.Stdout, "Autostart:      enable")
			}
		}
		var xmlStr string
		if libvirt_rpc.IsLibvirtRPCAvailable() {
			if xmlRPC, err := libvirt_rpc.GetDomainXMLRPC(name, libvirt.DomainXMLInactive); err == nil {
				xmlStr = xmlRPC
			} else {
				logger.Libvirt.Warn("GetDomainXMLRPC 失败，降级为 virsh", "domain", name, "error", err)
			}
		}
		if xmlStr == "" {
			if xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive"); xmlResult.Error == nil {
				xmlStr = xmlResult.Stdout
			}
		}
		if xmlStr != "" {
			// 使用持久化配置的 vCPU 覆盖在线值，确保界面显示的 vCPU 与用户配置一致
			// (libvirt 不支持在线修改 vCPU 最大值，热添加超限时持久化已更新但在线未变)
			if configVCPU := D.ParseVCPUCountFromDomainXML(xmlStr); configVCPU > 0 {
				vm.VCPU = configVCPU
			}
			applyMemoryDynamicInfoToVMInfo(&vm, memory.GetVMMemoryDynamicInfo(name, xmlStr, vm.Status))
			vm.CPULimitPercent = D.ParseVMCPULimitPercentFromDomainXML(xmlStr, vm.VCPU)
			vm.CPUAffinity = D.ParseCPUAffinityFromDomainXML(xmlStr)
		}
		if remark, err := GetVMRemark(name); err == nil {
			vm.Remark = remark
		}
		if tags, err := GetVMTags(name); err == nil {
			vm.Tags = tags
		}

		if listOptions.IncludeIP {
			vm.IP = ip_resolver.GetVMIP(name, vm.Status == "running")
		}
		vm.IPStatus = ip_resolver.GetVMIPStatus(name, vm.Status == "running")

		// 获取磁盘信息和模板来源（DiskSize 为所有磁盘虚拟配置总大小，含额外磁盘）
		diskInfo := GetVMDiskInfo(name)
		vm.DiskSize = diskInfo.TotalSizeText()
		vm.Template = diskInfo.Template
		vm.IsLinkedClone = diskInfo.Template != ""

		// 仅在需要时查询列表页未直接使用的网卡 / 带宽信息，避免每台 VM 额外触发多次 virsh 调用。
		if listOptions.IncludeNetworkInfo {
			netInfo := GetVMNetworkInfo(name)
			vm.Network = netInfo.Network
			vm.NicModel = netInfo.NicModel
			vm.MacAddress = netInfo.MAC
		}
		if listOptions.IncludeBandwidth {
			vm.BandwidthIn, vm.BandwidthOut = D.GetVMBandwidthMbps(name)
		}

		// 仅在明确请求时，从缓存补充资源使用率
		if listOptions.IncludeResourceUsage && vm.Status == "running" {
			if cached := D.GetCachedStats(name); cached != nil {
				vm.CPUPercent = cached.CPUPercent
				if cached.MemTotal > 0 {
					vm.MemPercent = float64(cached.MemUsed) / float64(cached.MemTotal) * 100
				}
			}
		}

		// 检查是否处于救援模式
		vm.InRescue = D.IsInRescueMode(name)
		runtimeInfo := GetVMRuntimeInfo(name, vm.Status)
		vm.ContinuousRuntimeSeconds = runtimeInfo.ContinuousRuntimeSeconds
		vm.ContinuousRunningSince = runtimeInfo.ContinuousRunningSince
		if D.HookApplyVMUnderMigrationStatus != nil {
			D.HookApplyVMUnderMigrationStatus(&vm)
		}

		vms = append(vms, vm)
	}

	return vms, nil
}

// GetVMIPInfo 获取单个虚拟机 IP 信息
func GetVMIPInfo(name string) (string, string, error) {
	_, _, _, _, err := libvirt_rpc.GetDomainInfoRPC(name)
	if err != nil {
		return "", "", fmt.Errorf("虚拟机不存在: %s", name)
	}

	state, err := libvirt_rpc.GetDomainStateRPC(name)
	if err != nil {
		return "", "", fmt.Errorf("获取虚拟机状态失败: %w", err)
	}
	isRunning := state == "running"

	return ip_resolver.GetVMIP(name, isRunning), ip_resolver.GetVMIPStatus(name, isRunning), nil
}

// ==================== 内部辅助函数 ====================

// parseInfoInt 从 virsh dominfo 输出中解析整数值
func parseInfoInt(output, key string) int {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// 取最后或倒数第二个字段（带 KiB 单位）
				valStr := parts[len(parts)-2]
				if val, err := strconv.Atoi(valStr); err == nil {
					return val
				}
				// 尝试最后一个字段
				valStr = parts[len(parts)-1]
				if val, err := strconv.Atoi(valStr); err == nil {
					return val
				}
			}
		}
	}
	return 0
}

// applyMemoryDynamicInfoToVMInfo 将动态内存信息写入 VmInfo
func applyMemoryDynamicInfoToVMInfo(vm *VmInfo, info *memory.VMMemoryDynamicInfo) {
	if vm == nil || info == nil {
		return
	}
	vm.MemoryInitial = info.MemoryInitial
	vm.MemoryMin = info.MemoryMin
	vm.MemoryMaxDynamic = info.MemoryMax
	vm.MemoryBackend = info.MemoryBackend
	vm.MemoryVirtioMemCurrent = info.VirtioMemCurrent
	vm.MemoryDynamicEnabled = info.DynamicEnabled
	vm.MemoryAutoBalloon = info.AutoBalloon
	vm.MemoryPendingApply = info.PendingApply
	vm.MemoryCompatMode = info.CompatMode
	vm.MemoryBalloonSupported = info.BalloonSupported
	vm.MemoryBalloonStatus = info.BalloonStatus
}
