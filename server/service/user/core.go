package user

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	clonepkg "kvm_console/service/clone"
	"kvm_console/service/security"
	"kvm_console/utils"
)

func isExistingVMAccessUser(username string) bool {
	username = strings.TrimSpace(username)
	if username == "" {
		return false
	}
	if model.DB == nil {
		return true
	}
	var user model.User
	return model.DB.Select("id").Where("username = ?", username).First(&user).Error == nil
}

// ListUsers 获取用户列表（含配额信息）
func ListUsers() ([]VMUserInfo, error) {
	var users []model.User
	if err := model.DB.Find(&users).Error; err != nil {
		return nil, err
	}

	var result []VMUserInfo
	for _, u := range users {
		info := VMUserInfo{
			ID:                   u.ID,
			Username:             u.Username,
			Email:                u.Email,
			Role:                 u.Role,
			CloudType:            HookNormalizeCloudType(u.CloudType),
			DedicatedVPCSwitchID: u.DedicatedVPCSwitchID,
			Status:               u.Status,
			MaxCPU:               u.MaxCPU,
			MaxMemory:            u.MaxMemory,
			MaxDisk:              u.MaxDisk,
			MaxVM:                u.MaxVM,
			MaxStorage:           u.MaxStorage,
			MaxRuntimeHours:      u.MaxRuntimeHours,
			EnablePortForward:    u.EnablePortForward,
			MaxPortForwards:      u.MaxPortForwards,
			MaxSnapshots:         u.MaxSnapshots,
			MaxBandwidthUp:       u.MaxBandwidthUp,
			MaxBandwidthDown:     u.MaxBandwidthDown,
			MaxTrafficDown:       u.MaxTrafficDown,
			MaxTrafficUp:         u.MaxTrafficUp,
			MaxPublicIPs:         u.MaxPublicIPs,
			SSHEnabled:           u.SSHEnabled,
		}

		// 读取用户的虚拟机分配列表
		if u.Role != "admin" {
			info.VMs = GetUserVMList(u.Username)

			// 弹性云用户显示用户级配额，轻量云改为单 VM 配额。
			if !HookIsLightweightCloudType(u.CloudType) {
				if quota, err := GetUserQuotaUsage(u.Username); err == nil {
					info.Quota = quota
				}
			} else {
				model.DB.Where("username = ?", u.Username).Find(&info.LightweightVMQuotas)
				for i := range info.LightweightVMQuotas {
					HookFillLightweightVMQuotaRuntime(&info.LightweightVMQuotas[i])
				}
				if regs, err := HookListLightweightVMRegistrations(u.Username, true); err == nil {
					info.LightweightVMRegistrations = regs
				}
			}
		} else {
			// 管理员也填充存储配额使用情况
			if quota, err := GetUserQuotaUsage(u.Username); err == nil {
				info.Quota = quota
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// CreateSystemUser 创建系统用户并添加到数据库
func CreateSystemUser(username, password, role string, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int, enablePortForward bool, maxPortForwards, maxSnapshots int, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64) error {
	if role == "" {
		role = "user"
	}

	// 检查用户名是否已存在（未删除的）
	var count int64
	if err := model.DB.Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return fmt.Errorf("检查用户名失败: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("用户名 %s 已存在", username)
	}

	// 清理同名的软删除记录（避免 UNIQUE 约束冲突）
	model.DB.Unscoped().Where("username = ? AND deleted_at IS NOT NULL", username).Delete(&model.User{})

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %w", err)
	}

	// 创建数据库记录
	fingerprint := security.BuildPasswordFingerprint(password)
	user := model.User{
		Username:             username,
		PasswordHash:         string(hashedPassword),
		PasswordBreachPrefix: fingerprint.Prefix,
		PasswordBreachHMAC:   fingerprint.HMAC,
		Email:                "",
		Role:                 role,
		CloudType:            "elastic",
		Status:               security.UserStatusActive,
		MaxCPU:               maxCPU,
		MaxMemory:            maxMemory,
		MaxDisk:              maxDisk,
		MaxVM:                maxVM,
		MaxStorage:           maxStorage,
		MaxRuntimeHours:      maxRuntimeHours,
		EnablePortForward:    enablePortForward,
		MaxPortForwards:      maxPortForwards,
		MaxSnapshots:         maxSnapshots,
		MaxBandwidthUp:       maxBandwidthUp,
		MaxBandwidthDown:     maxBandwidthDown,
		MaxTrafficDown:       maxTrafficDown,
		MaxTrafficUp:         maxTrafficUp,
	}
	if err := model.DB.Create(&user).Error; err != nil {
		return fmt.Errorf("创建用户失败: %w", err)
	}

	if err := ProvisionSystemUserResources(&user, password); err != nil {
		return err
	}
	if user.Role == "user" && !HookIsLightweightCloudType(user.CloudType) {
		if _, err := HookEnsureDefaultSecurityGroup(user.Username); err != nil {
			return err
		}
		if _, err := HookEnsureDefaultVPCSwitch(user.Username); err != nil {
			return err
		}
	}
	return nil
}

func ProvisionSystemUserResources(user *model.User, password string) error {
	// 根据角色创建系统用户
	if user.Role == "user" {
		// 确保 vmoperator 组存在
		utils.ExecCommand("groupadd", "-f", "vmoperator")

		// 创建系统用户（加入 kvm 组以便 sudo -u 执行导出时有存储目录写权限）
		// 默认 shell 设为 nologin，禁止 SSH 登录；管理员可通过面板 SSH 开关切换为 /bin/bash
		result := utils.ExecCommand("useradd", "-m", "-s", sshShellNologin,
			"-G", "vmoperator,libvirt,kvm", user.Username)
		if result.Error != nil {
			// 用户可能已存在，更新组和 shell
			utils.ExecCommand("usermod", "-aG", "vmoperator,libvirt,kvm",
				"-s", sshShellNologin, user.Username)
		}

		// 设置系统密码（使用 stdin 方式，避免 shell 转义问题）
		cleanPassword := strings.Map(func(r rune) rune {
			if r < 32 || r == 127 {
				return -1
			}
			return r
		}, password)
		if cleanPassword != "" {
			cmd := exec.Command("chpasswd")
			cmd.Stdin = strings.NewReader(user.Username + ":" + cleanPassword + "\n")
			if output, err := cmd.CombinedOutput(); err != nil {
				logger.App.Warn("设置系统密码失败", "user", user.Username, "error", err, "output", string(output))
			}
		}

		// 创建 VM 访问配置目录
		utils.ExecCommand("mkdir", "-p", config.GlobalConfig.VMAccessDir)

		// 初始化空的 VM 分配文件
		utils.ExecShell(fmt.Sprintf("touch %s/%s", config.GlobalConfig.VMAccessDir, utils.ShellSingleQuote(user.Username)))

		// 创建用户后同步 SSH 拒绝配置（默认禁止 SSH）
		regenerateSSHDenyConfig()
	}

	// 设置文件系统存储配额（所有角色都支持）
	if user.MaxStorage > 0 {
		if err := HookSetUserStorageQuota(user.Username, user.MaxStorage); err != nil {
			logger.App.Warn("设置用户文件系统配额失败", "user", user.Username, "error", err)
		}
	}

	return nil
}

// FindVMOwner 根据 VM 名称查找归属用户
func FindVMOwner(vmName string) string {
	vmAccessDir := config.GlobalConfig.VMAccessDir
	entries, err := os.ReadDir(vmAccessDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		username := entry.Name()
		if !isExistingVMAccessUser(username) {
			continue
		}
		vms := GetUserVMList(username)
		for _, vm := range vms {
			if vm == vmName {
				return username
			}
		}
	}
	return ""
}

// UpdateUserStatus 更新用户状态
func UpdateUserStatus(username, targetStatus string) error {
	targetStatus = strings.TrimSpace(targetStatus)
	if targetStatus != security.UserStatusActive && targetStatus != security.UserStatusDisabled {
		return fmt.Errorf("不支持的用户状态: %s", targetStatus)
	}

	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户 %s 不存在", username)
	}
	// 状态变更权限由 handler 层控制（内置 admin 保护 + 不能操作自己）
	if user.Status == security.UserStatusPendingInvite {
		return fmt.Errorf("待激活用户不支持封禁或解封")
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":                   targetStatus,
		"login_verified_until":     nil,
		"high_risk_verified_until": nil,
		"security_updated_at":      &now,
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新用户状态失败: %w", err)
	}
	return nil
}

// DisableUserAccount 封禁用户并关闭其运行中的资源
func DisableUserAccount(username string, progressFn func(int, string)) (*UserStatusChangeResult, error) {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}

	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户 %s 不存在", username)
	}
	// 封禁权限由 handler 层控制（内置 admin 保护 + 不能操作自己）
	if user.Status == security.UserStatusPendingInvite {
		return nil, fmt.Errorf("待激活用户不支持封禁")
	}

	result := &UserStatusChangeResult{
		Username: username,
		Status:   security.UserStatusDisabled,
	}

	progressFn(5, fmt.Sprintf("正在封禁用户 %s ...", username))
	if err := UpdateUserStatus(username, security.UserStatusDisabled); err != nil {
		return nil, err
	}

	progressFn(15, "正在关闭用户 SSH 访问...")
	if err := SetUserSSH(username, false); err != nil {
		result.Warnings = append(result.Warnings, "关闭 SSH 访问失败: "+err.Error())
	}

	progressFn(35, "正在关闭用户虚拟机...")
	result.StoppedVMs, result.Warnings = stopUserVMsForDisable(username, result.Warnings)

	progressFn(100, "用户已封禁，运行中的虚拟机已关闭")
	return result, nil
}

func stopUserVMsForDisable(username string, warnings []string) ([]string, []string) {
	vmNames := GetUserVMList(username)
	if len(vmNames) == 0 {
		return nil, warnings
	}

	var stopped []string
	for _, vmName := range vmNames {
		stateResult := utils.ExecCommand("virsh", "domstate", vmName)
		if stateResult.Error != nil {
			warnings = append(warnings, fmt.Sprintf("获取虚拟机 %s 状态失败: %s", vmName, stateResult.Stderr))
			continue
		}

		state := strings.ToLower(strings.TrimSpace(stateResult.Stdout))
		if state == "" || strings.Contains(state, "shut off") {
			continue
		}

		needForceOff := strings.Contains(state, "paused")
		if !needForceOff {
			if err := HookShutdownVM(vmName); err != nil {
				needForceOff = true
			} else if !WaitVMShutdownForDisable(vmName, 40*time.Second) {
				needForceOff = true
			}
		}

		if needForceOff {
			if err := HookDestroyVM(vmName); err != nil {
				warnings = append(warnings, fmt.Sprintf("关闭虚拟机 %s 失败: %s", vmName, err.Error()))
				continue
			}
		}

		stopped = append(stopped, vmName)
	}

	return stopped, warnings
}

func WaitVMShutdownForDisable(vmName string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stateResult := utils.ExecCommand("virsh", "domstate", vmName)
		if stateResult.Error == nil {
			state := strings.ToLower(strings.TrimSpace(stateResult.Stdout))
			if state == "" || strings.Contains(state, "shut off") {
				return true
			}
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// DeleteSystemUser 删除用户及其所有资产（虚拟机、存储池等）
// progressFn 可选，用于在异步任务中上报进度
func DeleteSystemUser(username string, progressFn func(int, string)) error {
	if progressFn == nil {
		progressFn = func(int, string) {} // 空实现，避免 nil panic
	}

	// 第一步：删除用户的所有虚拟机
	progressFn(5, "正在获取用户虚拟机列表...")
	userVMs := GetUserVMList(username)
	if len(userVMs) > 0 {
		totalVMs := len(userVMs)
		for i, vmName := range userVMs {
			vmProgress := 5 + (i*35)/totalVMs
			progressFn(vmProgress, fmt.Sprintf("正在删除虚拟机 %s (%d/%d)...", vmName, i+1, totalVMs))
			if err := clonepkg.DeleteVM(vmName); err != nil {
				logger.App.Warn("删除用户虚拟机失败", "user", username, "vm", vmName, "error", err)
				// 继续删除其他虚拟机，不中断流程
			}
		}
		progressFn(40, fmt.Sprintf("已删除 %d 台虚拟机", totalVMs))
	} else {
		progressFn(40, "用户没有虚拟机")
	}

	// 第二步：清理用户流量记录
	progressFn(50, "正在清理用户流量记录...")
	cleanupUserTrafficRecords(username)

	// 第三步：清理用户网络资源
	progressFn(55, "正在清理用户 VPC/OVS 网络资源...")
	if err := HookCleanupUserNetworkResources(username, userVMs); err != nil {
		logger.App.Warn("清理用户网络资源失败", "user", username, "error", err)
	}

	// 第四步：删除用户存储池目录
	progressFn(60, "正在清理用户存储池...")
	userStorageDir := fmt.Sprintf("%s/%s", HookGetStorageMountPoint(), username)
	checkResult := utils.ExecShell(fmt.Sprintf("test -d %s && echo yes || echo no", utils.ShellSingleQuote(userStorageDir)))
	if strings.TrimSpace(checkResult.Stdout) == "yes" {
		if err := os.RemoveAll(userStorageDir); err != nil {
			logger.App.Warn("删除用户存储池目录失败", "user", username, "error", err)
		}
	}

	// 第五步：清除文件系统配额
	progressFn(70, "正在清理文件系统配额...")
	if err := HookRemoveUserStorageQuota(username); err != nil {
		logger.App.Warn("清除用户文件系统配额失败", "user", username, "error", err)
	}

	// 第六步：删除数据库记录
	progressFn(80, "正在删除用户数据库记录...")
	if err := model.DB.Where("username = ?", username).Delete(&model.User{}).Error; err != nil {
		return fmt.Errorf("删除用户数据库记录失败: %w", err)
	}

	// 第七步：删除 VM 分配文件
	progressFn(85, "正在清理 VM 访问配置...")
	_ = os.Remove(filepath.Join(config.GlobalConfig.VMAccessDir, username))

	// 第八步：删除系统用户
	progressFn(90, "正在删除系统用户...")
	utils.ExecCommand("userdel", "-r", username)

	// 第九步：重新生成 polkit 规则
	progressFn(95, "正在更新权限规则...")
	if err := regeneratePolkitRules(); err != nil {
		logger.App.Warn("重新生成 polkit 规则失败", "error", err)
	}

	// 第十步：同步 SSH 拒绝配置
	regenerateSSHDenyConfig()

	progressFn(100, "用户及所有资产已删除")
	return nil
}

// cleanupUserTrafficRecords 清理用户的流量记录
func cleanupUserTrafficRecords(username string) {
	result := model.DB.Where("username = ?", username).Delete(&model.UserTrafficDaily{})
	if result.RowsAffected > 0 {
		logger.App.Info("已清理用户流量记录", "user", username, "rows", result.RowsAffected)
	}
}

// regeneratePolkitRules 重新生成 polkit 权限规则
func regeneratePolkitRules() error {
	vmAccessDir := config.GlobalConfig.VMAccessDir

	// 读取所有用户的 VM 映射
	entries, err := os.ReadDir(vmAccessDir)
	if err != nil {
		return nil
	}

	var mappings []string
	for _, entry := range entries {
		username := entry.Name()
		if !isExistingVMAccessUser(username) {
			continue
		}

		// 读取该用户的 VM 列表
		vmsResult := utils.ExecShell(fmt.Sprintf("cat %s/%s 2>/dev/null", vmAccessDir, utils.ShellSingleQuote(username)))
		if vmsResult.Error != nil || vmsResult.Stdout == "" {
			continue
		}

		var jsArr []string
		for _, vm := range strings.Split(vmsResult.Stdout, "\n") {
			vm = strings.TrimSpace(vm)
			if vm != "" {
				jsArr = append(jsArr, fmt.Sprintf(`"%s"`, vm))
			}
		}
		if len(jsArr) > 0 {
			mappings = append(mappings, fmt.Sprintf(`        "%s": [%s],`, username, strings.Join(jsArr, ", ")))
		}
	}

	mappingStr := strings.Join(mappings, "\n")

	polkitRules := fmt.Sprintf(`// 自动生成 - 请勿手动编辑
// 规则1: root 和 libvirt-dbus 拥有全部权限
polkit.addRule(function(action, subject) {
    if (action.id.indexOf("org.libvirt.") === 0) {
        if (subject.user === "root" || subject.user === "libvirtdbus") {
            return polkit.Result.YES;
        }
    }
    return polkit.Result.NOT_HANDLED;
});

// 规则2: vmoperator 用户按虚拟机分配权限
polkit.addRule(function(action, subject) {
    if (!subject.isInGroup("vmoperator")) return polkit.Result.NOT_HANDLED;
    var vmAccessMap = {
%s
    };
    var userVMs = vmAccessMap[subject.user];
    if (!userVMs || userVMs.length === 0) {
        if (action.id.indexOf("org.libvirt.") === 0) return polkit.Result.NO;
        return polkit.Result.NOT_HANDLED;
    }
    var connectActions = ["org.libvirt.unix.manage","org.libvirt.unix.monitor","org.libvirt.api.connect.getattr","org.libvirt.api.connect.read","org.libvirt.api.connect.search-domains","org.libvirt.api.connect.search-networks"];
    for (var i = 0; i < connectActions.length; i++) { if (action.id === connectActions[i]) return polkit.Result.YES; }
    var infraActions = ["org.libvirt.api.network.getattr","org.libvirt.api.network.read","org.libvirt.api.network-port.create","org.libvirt.api.network-port.delete"];
    for (var i = 0; i < infraActions.length; i++) { if (action.id === infraActions[i]) return polkit.Result.YES; }
    var domainName = action.lookup("domain_name");
    if (domainName) {
        var hasAccess = false;
        for (var i = 0; i < userVMs.length; i++) { if (domainName === userVMs[i]) { hasAccess = true; break; } }
        if (!hasAccess) return polkit.Result.NO;
        var domainActions = ["org.libvirt.api.domain.getattr","org.libvirt.api.domain.read","org.libvirt.api.domain.start","org.libvirt.api.domain.stop","org.libvirt.api.domain.reset","org.libvirt.api.domain.snapshot"];
        for (var i = 0; i < domainActions.length; i++) { if (action.id === domainActions[i]) return polkit.Result.YES; }
    }
    if (action.id.indexOf("org.libvirt.") === 0) return polkit.Result.NO;
    return polkit.Result.NOT_HANDLED;
});`, mappingStr)

	// 写入规则文件
	polkitPath := "/etc/polkit-1/rules.d/10-vmoperator.rules"
	if err := os.WriteFile(polkitPath, []byte(polkitRules), 0644); err != nil {
		return fmt.Errorf("写入 polkit 规则失败: %v", err)
	}

	// 重启 polkit
	utils.ExecCommand("systemctl", "restart", "polkit")

	return nil
}
