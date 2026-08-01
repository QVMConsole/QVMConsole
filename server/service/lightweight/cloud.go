package lightweight

import (
	"fmt"
	"strconv"
	"strings"

	"kvm_console/logger"
	"kvm_console/model"
)

const (
	CloudTypeElastic     = "elastic"
	CloudTypeLightweight = "lightweight"

	defaultLightweightTrafficPenaltyMbps = 1
	lightweightTrafficPenaltySettingKey  = "lightweight_traffic_penalty_mbps"
)

func lightweightTrafficPenaltyMbps() int {
	if model.DB == nil {
		return defaultLightweightTrafficPenaltyMbps
	}
	value, ok := model.GetSetting(lightweightTrafficPenaltySettingKey)
	if !ok {
		return defaultLightweightTrafficPenaltyMbps
	}
	mbps, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || mbps <= 0 {
		return defaultLightweightTrafficPenaltyMbps
	}
	return mbps
}

func NormalizeCloudType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CloudTypeLightweight:
		return CloudTypeLightweight
	default:
		return CloudTypeElastic
	}
}

func IsLightweightCloudType(value string) bool {
	return NormalizeCloudType(value) == CloudTypeLightweight
}

func IsLightweightCloudUser(username string) bool {
	if strings.TrimSpace(username) == "" || model.DB == nil {
		return false
	}
	var user model.User
	if err := model.DB.Where("username = ?", strings.TrimSpace(username)).First(&user).Error; err != nil {
		return false
	}
	return IsLightweightCloudType(user.CloudType)
}

func IsLightweightCloudVM(vmName string) bool {
	if strings.TrimSpace(vmName) == "" || model.DB == nil {
		return false
	}
	var count int64
	model.DB.Model(&model.LightweightVMQuota{}).Where("vm_name = ?", strings.TrimSpace(vmName)).Count(&count)
	return count > 0
}

func UpdateUserCloudProfile(username, cloudType string, dedicatedVPCSwitchID uint) error {
	username = strings.TrimSpace(username)
	cloudType = NormalizeCloudType(cloudType)
	if username == "" {
		return fmt.Errorf("用户不能为空")
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}
	if user.Role == "admin" {
		cloudType = CloudTypeElastic
		dedicatedVPCSwitchID = 0
	}
	if user.Role == "user" && IsLightweightCloudType(cloudType) {
		if dedicatedVPCSwitchID == 0 {
			return fmt.Errorf("轻量云用户必须选择专用 VPC 网络")
		}
		var count int64
		if err := model.DB.Model(&model.VPCSwitch{}).
			Where("id = ? AND (bridge_mode = '' OR bridge_mode = ? OR bridge_mode IS NULL)", dedicatedVPCSwitchID, BridgeModeNAT).
			Count(&count).Error; err != nil {
			return fmt.Errorf("检查专用 VPC 网络失败: %w", err)
		}
		if count == 0 {
			return fmt.Errorf("请选择有效的 NAT 类型专用 VPC 网络")
		}
	} else {
		dedicatedVPCSwitchID = 0
	}
	if err := model.DB.Model(&model.User{}).Where("username = ?", username).Updates(map[string]interface{}{
		"cloud_type":              cloudType,
		"dedicated_vpc_switch_id": dedicatedVPCSwitchID,
	}).Error; err != nil {
		return err
	}
	if user.Role != "user" {
		return nil
	}
	if IsLightweightCloudType(cloudType) {
		for _, vmName := range HookGetUserVMList(username) {
			if _, err := GetLightweightVMQuota(vmName); err != nil {
				if _, err := UpsertLightweightVMQuota(username, DefaultLightweightVMQuota(vmName)); err != nil {
					return err
				}
			}
			if err := EnsureLightweightVMNetwork(username, vmName); err != nil {
				return err
			}
		}
		return nil
	}
	for _, vmName := range HookGetUserVMList(username) {
		HookCleanupVMVPCBinding(vmName)
		CleanupLightweightVMResources(vmName)
	}
	if !IsLightweightCloudType(cloudType) {
		if _, err := HookEnsureDefaultSecurityGroup(username); err != nil {
			return err
		}
		if _, err := HookEnsureDefaultVPCSwitch(username); err != nil {
			return err
		}
	}
	return nil
}

func EnsureLightweightVMNetwork(username, vmName string) error {
	username = strings.TrimSpace(username)
	vmName = strings.TrimSpace(vmName)
	if username == "" || vmName == "" {
		return fmt.Errorf("用户和虚拟机不能为空")
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}
	if !IsLightweightCloudType(user.CloudType) {
		return nil
	}
	// 如果用户没有配置专用VPC，跳过网络配置（适用于选择已有VM的场景）
	if user.DedicatedVPCSwitchID == 0 {
		return nil
	}
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, user.DedicatedVPCSwitchID).Error; err != nil {
		return fmt.Errorf("专用 VPC 网络不存在")
	}
	if HookSwitchUsesDirectBridge(sw) {
		return fmt.Errorf("轻量云专用 VPC 不能使用桥接直通网络")
	}
	groupOwner := strings.TrimSpace(sw.Username)
	if sw.IsSystem || groupOwner == "" {
		groupOwner = username
	}
	group, err := ensureLightweightVMSecurityGroup(groupOwner, vmName)
	if err != nil {
		return err
	}
	return HookBindVMToVPCAsAdmin(vmName, sw.ID, group.ID)
}

func ensureLightweightVMSecurityGroup(groupOwner, vmName string) (*model.VPCSecurityGroup, error) {
	var group model.VPCSecurityGroup
	if err := model.DB.Where("vm_name = ? AND is_vm_scoped = ?", vmName, true).First(&group).Error; err == nil {
		groupOwner = strings.TrimSpace(groupOwner)
		if groupOwner != "" && group.Username != groupOwner {
			if err := model.DB.Model(&group).Update("username", groupOwner).Error; err != nil {
				return nil, fmt.Errorf("修复轻量云 VM 专属安全组归属失败: %w", err)
			}
			group.Username = groupOwner
		}
		return &group, nil
	}
	name := "light-" + vmName
	if len(name) > 64 {
		name = name[:64]
	}
	group = model.VPCSecurityGroup{
		Username:   groupOwner,
		VMName:     vmName,
		Name:       name,
		IsVMScoped: true,
		Remark:     "轻量云 VM 专属安全组",
	}
	if err := model.DB.Create(&group).Error; err != nil {
		return nil, fmt.Errorf("创建轻量云 VM 专属安全组失败: %w", err)
	}
	return &group, nil
}

func CleanupLightweightVMResources(vmName string) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" || model.DB == nil {
		return
	}
	model.DB.Where("vm_name = ?", vmName).Delete(&model.LightweightVMTrafficMonthly{})
	model.DB.Where("vm_name = ?", vmName).Delete(&model.LightweightVMQuota{})
	model.DB.Where("vm_name = ?", vmName).Delete(&model.LightweightVMRegistration{})
	releaseLightweightRuntimeQuotaEnforcement(vmName)

	var groups []model.VPCSecurityGroup
	model.DB.Where("vm_name = ? AND is_vm_scoped = ?", vmName, true).Find(&groups)
	for _, group := range groups {
		model.DB.Where("security_group_id = ?", group.ID).Delete(&model.VPCSecurityGroupRule{})
		model.DB.Delete(&group)
	}
	if len(groups) > 0 {
		if err := HookApplyVPCACLRules(); err != nil {
			logger.App.Warn("清理 VM 专属安全组后重建 VPC ACL 失败", "component", "轻量云", "vm", vmName, "error", err)
		}
	}
}
