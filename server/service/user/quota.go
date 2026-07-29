package user

import (
	"fmt"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/snapshot"
	"kvm_console/utils"
)

// GetUserVMList 获取用户拥有的VM列表（从访问列表文件读取）
func GetUserVMList(username string) []string {
	vmsResult := utils.ExecShell(fmt.Sprintf("cat %s/%s 2>/dev/null",
		utils.ShellSingleQuote(config.GlobalConfig.VMAccessDir), username))
	if vmsResult.Error != nil || vmsResult.Stdout == "" {
		return nil
	}

	var vms []string
	for _, vm := range strings.Split(vmsResult.Stdout, "\n") {
		vm = strings.TrimSpace(vm)
		if vm != "" {
			vms = append(vms, vm)
		}
	}
	return vms
}

// GetUserQuotaUsage 查询用户的配额使用情况
func GetUserQuotaUsage(username string) (*QuotaUsage, error) {
	// 获取用户信息
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}

	usage := &QuotaUsage{
		MaxCPU:            user.MaxCPU,
		MaxMemory:         user.MaxMemory,
		MaxDisk:           user.MaxDisk,
		MaxVM:             user.MaxVM,
		MaxStorage:        user.MaxStorage,
		MaxRuntimeHours:   user.MaxRuntimeHours,
		EnablePortForward: user.EnablePortForward,
		MaxPortForwards:   user.MaxPortForwards,
		MaxSnapshots:      user.MaxSnapshots,
		MaxBandwidthUp:    user.MaxBandwidthUp,
		MaxBandwidthDown:  user.MaxBandwidthDown,
		MaxTrafficDown:    user.MaxTrafficDown,
		MaxTrafficUp:      user.MaxTrafficUp,
		MaxPublicIPs:      user.MaxPublicIPs,
	}

	// 获取用户拥有的VM列表（自建 + 管理员分配的），并剔除访问列表中残留的已删除VM
	vms := FilterExistingVMs(GetUserVMList(username))
	usage.UsedVM = len(vms)

	// 遍历每个VM，统计资源使用
	totalVMMemMB := 0
	for _, vmName := range vms {
		cpu, memMB := GetVMCPUAndMemory(vmName)
		usage.UsedCPU += cpu
		totalVMMemMB += memMB
		usage.UsedDisk += getVMDiskCapacityGB(vmName)
	}

	usage.UsedMemory = totalVMMemMB / 1024
	usage.UsedPublicIPs = HookGetUserPublicIPUsage(username)
	usage.UsedPortForwards = HookGetUserPortForwardUsage(username)
	usage.UsedSnapshots = snapshot.CountUserSnapshots(username)

	// 通过文件系统配额统计存储使用量
	quotaInfo, err := HookGetUserStorageUsage(username)
	if err == nil && quotaInfo != nil {
		usage.UsedStorage = quotaInfo.UsedBytes
	} else {
		// 回退：使用 du 统计
		isoDir := GetUserISODir(username)
		shareDir := GetUserShareDir(username)
		usage.UsedStorage = getDirSizeBytes(isoDir) + getDirSizeBytes(shareDir)
	}
	usage.UsedStorageGB = formatBytes(usage.UsedStorage)

	runtimeSnapshot := BuildUserRuntimeQuotaSnapshot(&user, time.Now())
	usage.UsedRuntimeSeconds = runtimeSnapshot.UsedSeconds
	usage.UsedRuntimeDisplay = FormatRuntimeQuotaDuration(runtimeSnapshot.UsedSeconds)
	usage.RemainingRuntimeSeconds = runtimeSnapshot.RemainingSeconds
	usage.RemainingRuntimeDisplay = FormatRuntimeQuotaDuration(runtimeSnapshot.RemainingSeconds)
	usage.RuntimeQuotaReached = runtimeSnapshot.QuotaReached

	// 填充日流量使用数据
	trafficInfo := HookGetUserTrafficUsage(username)
	if trafficInfo != nil {
		usage.UsedTrafficDown = trafficInfo.UsedTrafficDown
		usage.UsedTrafficUp = trafficInfo.UsedTrafficUp
		usage.UsedTrafficDownGB = trafficInfo.UsedTrafficDownGB
		usage.UsedTrafficUpGB = trafficInfo.UsedTrafficUpGB
		usage.IsLimitedDown = trafficInfo.IsLimitedDown
		usage.IsLimitedUp = trafficInfo.IsLimitedUp
	}

	return usage, nil
}

// CheckQuota 检查用户配额是否允许创建新VM
// 检查VM数量、磁盘、CPU和内存配额（创建后VM会立即启动，因此需要校验CPU/内存）
func CheckQuota(username string, reqCPU, reqMemoryGB, reqDiskGB int) error {
	usage, err := GetUserQuotaUsage(username)
	if err != nil {
		return err
	}

	// VM数量检查
	if usage.MaxVM > 0 && usage.UsedVM+1 > usage.MaxVM {
		return fmt.Errorf("VM数量超出配额限制（已用 %d / 上限 %d）", usage.UsedVM, usage.MaxVM)
	}

	// 磁盘检查
	if usage.MaxDisk > 0 && usage.UsedDisk+reqDiskGB > usage.MaxDisk {
		return fmt.Errorf("磁盘超出配额限制（已用 %dGB + 请求 %dGB > 上限 %dGB）",
			usage.UsedDisk, reqDiskGB, usage.MaxDisk)
	}

	// 总运行时长配额检查
	if usage.RuntimeQuotaReached {
		return fmt.Errorf("总运行时长配额已用尽（已用 %s / 上限 %d 小时），请联系管理员调整配额",
			usage.UsedRuntimeDisplay, usage.MaxRuntimeHours)
	}

	// CPU检查（基于所有已分配VM的总量）
	if usage.MaxCPU > 0 && usage.UsedCPU+reqCPU > usage.MaxCPU {
		return fmt.Errorf("CPU超出配额限制（已分配 %d核 + 请求 %d核 > 上限 %d核）",
			usage.UsedCPU, reqCPU, usage.MaxCPU)
	}

	// 内存检查（基于所有已分配VM的总量）
	if usage.MaxMemory > 0 && usage.UsedMemory+reqMemoryGB > usage.MaxMemory {
		return fmt.Errorf("内存超出配额限制（已分配 %dGB + 请求 %dGB > 上限 %dGB）",
			usage.UsedMemory, reqMemoryGB, usage.MaxMemory)
	}

	return nil
}

// CheckQuotaForEdit 检查用户配额是否允许修改VM配置
// 注意：CPU/内存配额改为在开机时检查（CheckQuotaForStart），此处仅检查磁盘增量
func CheckQuotaForEdit(username string, deltaCPU, deltaMemoryGB, deltaDiskGB int) error {
	usage, err := GetUserQuotaUsage(username)
	if err != nil {
		return err
	}

	// 磁盘检查（仅在增加时检查）
	if deltaDiskGB > 0 && usage.MaxDisk > 0 && usage.UsedDisk+deltaDiskGB > usage.MaxDisk {
		return fmt.Errorf("磁盘超出配额限制（已用 %dGB + 新增 %dGB > 上限 %dGB）",
			usage.UsedDisk, deltaDiskGB, usage.MaxDisk)
	}

	return nil
}

// GetRunningVMsResourceUsage 统计用户已运行（running）的VM的CPU和内存使用量（内存返回 MB）
func GetRunningVMsResourceUsage(username string) (runningCPU int, runningMemoryMB int, err error) {
	// 剔除访问列表中残留的已删除VM，避免逐台 virsh 查询刷错误日志
	vms := FilterExistingVMs(GetUserVMList(username))

	for _, vmName := range vms {
		// 检查VM是否正在运行
		stateResult := utils.ExecCommand("virsh", "domstate", vmName)
		if stateResult.Error != nil {
			continue
		}
		state := strings.TrimSpace(stateResult.Stdout)
		if state != "running" {
			continue
		}

		cpu, memMB := GetVMCPUAndMemory(vmName)
		runningCPU += cpu
		runningMemoryMB += memMB
	}

	return runningCPU, runningMemoryMB, nil
}

// CheckQuotaForStart 检查用户配额是否允许开机
// 仅检查已运行VM的CPU和内存总量 + 待开机VM是否超出配额
// 用户超出配额只需关闭部分VM即可释放资源
func CheckQuotaForStart(username string, vmName string) error {
	if HookIsLightweightCloudUser(username) {
		return nil
	}
	// 获取用户配额上限
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	// 如果CPU和内存都不限制，直接放行
	if user.MaxCPU <= 0 && user.MaxMemory <= 0 {
		return CheckRuntimeQuotaAvailableForUser(&user, time.Now())
	}

	if err := CheckRuntimeQuotaAvailableForUser(&user, time.Now()); err != nil {
		return err
	}

	// 统计用户当前运行中VM的资源
	runningCPU, runningMemoryMB, err := GetRunningVMsResourceUsage(username)
	if err != nil {
		return fmt.Errorf("获取运行中VM资源失败: %w", err)
	}

	// 获取待开机VM的配置
	vmCPU, vmMemMB := GetVMCPUAndMemory(vmName)

	// CPU检查（VM运行中 + 待开机VM）
	if user.MaxCPU > 0 && runningCPU+vmCPU > user.MaxCPU {
		return fmt.Errorf("CPU超出配额限制（VM运行中 %d核 + 本机 %d核 > 上限 %d核），请先关闭部分虚拟机",
			runningCPU, vmCPU, user.MaxCPU)
	}

	// 内存检查（VM运行中 + 待开机VM）
	if user.MaxMemory > 0 && (runningMemoryMB+vmMemMB) > user.MaxMemory*1024 {
		return fmt.Errorf("内存超出配额限制（VM运行中 %dMB + 本机 %dMB > 上限 %dGB），请先关闭部分虚拟机",
			runningMemoryMB, vmMemMB, user.MaxMemory)
	}

	return nil
}

// AddVMToUser 将VM加入用户的访问列表
func AddVMToUser(username, vmName string) error {
	vms := GetUserVMList(username)

	// 检查是否已存在
	for _, vm := range vms {
		if vm == vmName {
			return nil // 已存在，无需重复添加
		}
	}

	vms = append(vms, vmName)
	content := strings.Join(vms, "\n")
	utils.ExecShell(fmt.Sprintf("echo %s > %s/%s",
		utils.ShellSingleQuote(content), utils.ShellSingleQuote(config.GlobalConfig.VMAccessDir), username))

	// 重新生成 polkit 规则
	if err := regeneratePolkitRules(); err != nil {
		return err
	}
	HookUpdateVMCacheOwner(vmName, username)
	return nil
}

// RemoveVMFromUser 从用户的访问列表中移除VM
func RemoveVMFromUser(username, vmName string) error {
	vms := GetUserVMList(username)

	var newVMs []string
	for _, vm := range vms {
		if vm != vmName {
			newVMs = append(newVMs, vm)
		}
	}

	if len(newVMs) == 0 {
		// 清空文件
		utils.ExecShell(fmt.Sprintf("> %s/%s", utils.ShellSingleQuote(config.GlobalConfig.VMAccessDir), username))
	} else {
		content := strings.Join(newVMs, "\n")
		utils.ExecShell(fmt.Sprintf("echo %s > %s/%s",
			utils.ShellSingleQuote(content), utils.ShellSingleQuote(config.GlobalConfig.VMAccessDir), username))
	}

	// 重新生成 polkit 规则
	if err := regeneratePolkitRules(); err != nil {
		return err
	}
	HookSyncVMCacheOwner(vmName)
	return nil
}

// UserOwnsVM 检查用户是否拥有某个VM
func UserOwnsVM(username, vmName string) bool {
	vms := GetUserVMList(username)
	for _, vm := range vms {
		if vm == vmName {
			return true
		}
	}
	return false
}

// UpdateUserQuota 更新用户配额
func UpdateUserQuota(username string, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int, enablePortForward bool, maxPortForwards, maxSnapshots int, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64, maxPublicIPs int) error {
	result := model.DB.Model(&model.User{}).Where("username = ?", username).Updates(map[string]interface{}{
		"max_cpu":             maxCPU,
		"max_memory":          maxMemory,
		"max_disk":            maxDisk,
		"max_vm":              maxVM,
		"max_storage":         maxStorage,
		"max_runtime_hours":   maxRuntimeHours,
		"enable_port_forward": enablePortForward,
		"max_port_forwards":   maxPortForwards,
		"max_snapshots":       maxSnapshots,
		"max_bandwidth_up":    maxBandwidthUp,
		"max_bandwidth_down":  maxBandwidthDown,
		"max_traffic_down":    maxTrafficDown,
		"max_traffic_up":      maxTrafficUp,
		"max_public_ips":      maxPublicIPs,
	})
	if result.Error != nil {
		return fmt.Errorf("更新配额失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户不存在: %s", username)
	}

	// 同步存储配额到文件系统
	if err := HookSetUserStorageQuota(username, maxStorage); err != nil {
		logger.App.Warn("同步用户文件系统配额失败", "user", username, "error", err)
	}

	// 检查流量配额调整后是否需要恢复限速
	HookCheckTrafficAfterQuotaUpdate(username)

	// 重新分配用户所有 VM 的带宽（内部已兼容流量超限状态）
	if err := HookRebalanceUserBandwidth(username); err != nil {
		logger.App.Warn("重新分配用户带宽失败", "user", username, "error", err)
	}

	// 运行时长配额调整后立即同步，避免更新后仍沿用旧状态。
	SyncUserRuntimeQuotaState(username, time.Now())

	return nil
}
