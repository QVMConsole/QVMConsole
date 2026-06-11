package vpc

import (
	"fmt"
	"strings"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/ip_resolver"
)

func BindVMToVPC(username, vmName string, switchID, securityGroupID uint) error {
	if strings.TrimSpace(vmName) == "" {
		return fmt.Errorf("虚拟机名称不能为空")
	}
	if username == "" {
		username = HookFindVMOwner(vmName)
	}
	if username == "" && switchID > 0 {
		var sw model.VPCSwitch
		if err := model.DB.First(&sw, switchID).Error; err == nil && !sw.IsSystem {
			username = sw.Username
		}
	}
	if username == "" {
		return fmt.Errorf("无法识别虚拟机归属用户")
	}
	if _, err := EnsureDefaultSecurityGroup(username); err != nil {
		return err
	}
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, switchID).Error; err != nil {
		return fmt.Errorf("交换机不存在")
	}
	// 系统交换机人人可用；用户交换机需要校验归属
	if !sw.IsSystem && sw.Username != username {
		return fmt.Errorf("交换机不存在或不属于该用户")
	}
	if err := validateVMCanApplyVPCSwitch(vmName, sw); err != nil {
		return err
	}
	if HookSwitchUsesDirectBridge(sw) {
		if securityGroupID != 0 {
			var sg model.VPCSecurityGroup
			if err := model.DB.Where("id = ? AND username = ?", securityGroupID, username).First(&sg).Error; err != nil {
				return fmt.Errorf("安全组不存在或不属于该用户")
			}
		}
	} else {
		var sg model.VPCSecurityGroup
		if err := model.DB.Where("id = ? AND username = ?", securityGroupID, username).First(&sg).Error; err != nil {
			return fmt.Errorf("安全组不存在或不属于该用户")
		}
	}
	var oldSwitchID uint
	var oldTrafficDown, oldTrafficUp int64
	newTrafficDown, newTrafficUp := AggregateSwitchMonthlyTraffic(switchID)
	binding := model.VPCVMBinding{
		VMName:          vmName,
		Username:        username,
		SwitchID:        switchID,
		SecurityGroupID: securityGroupID,
	}
	var existing model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).First(&existing).Error; err == nil {
		if existing.SwitchID != switchID {
			oldSwitchID = existing.SwitchID
			oldTrafficDown, oldTrafficUp = AggregateSwitchMonthlyTraffic(oldSwitchID)
			if mac := ip_resolver.GetFirstVMMAC(vmName); mac != "" {
				if HookRemoveVPCStaticHost != nil {
					if _, err := HookRemoveVPCStaticHost(existing.SwitchID, vmName, mac); err != nil {
						logger.App.Warn("RemoveVPCStaticHost failed", "vm", vmName, "error", err)
					}
				}
			}
		}
		existing.Username = username
		existing.SwitchID = switchID
		existing.SecurityGroupID = securityGroupID
		if err := model.DB.Save(&existing).Error; err != nil {
			return err
		}
	} else if err := model.DB.Create(&binding).Error; err != nil {
		return err
	}
	if oldSwitchID != 0 {
		rebaseVPCSwitchTrafficMonthly(oldSwitchID, oldTrafficDown, oldTrafficUp)
	}
	rebaseVPCSwitchTrafficMonthly(switchID, newTrafficDown, newTrafficUp)
	// VM 绑定 VPC 后默认由交换机控制聚合带宽，清理旧的 VM 级 OVS 限速流表。
	if err := HookClearVMBandwidth(vmName); err != nil {
		logger.App.Warn("清理 VM 单机速率限制失败", "vm", vmName, "error", err)
	}
	if err := ApplyVPCSwitchRuntime(vmName, sw); err != nil {
		return err
	}
	if HookSwitchUsesDirectBridge(sw) {
		return nil
	}
	return ApplyVPCACLRules()
}

func BindVMToVPCAsAdmin(vmName string, switchID, securityGroupID uint) error {
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, switchID).Error; err != nil {
		return fmt.Errorf("交换机不存在")
	}
	// 系统交换机使用 VM 归属用户的默认安全组
	switchOwner := sw.Username
	if sw.IsSystem {
		switchOwner = HookFindVMOwner(vmName)
	}
	if _, err := EnsureDefaultSecurityGroup(switchOwner); err != nil {
		return err
	}
	if HookSwitchUsesDirectBridge(sw) {
		return BindVMToVPC(switchOwner, vmName, switchID, 0)
	}
	if securityGroupID == 0 {
		var group model.VPCSecurityGroup
		if err := model.DB.Where("username = ? AND is_default = ?", switchOwner, true).First(&group).Error; err != nil {
			return fmt.Errorf("未找到用户 %s 的默认安全组", switchOwner)
		}
		securityGroupID = group.ID
	} else {
		var group model.VPCSecurityGroup
		if err := model.DB.First(&group, securityGroupID).Error; err != nil {
			return fmt.Errorf("安全组不存在")
		}
		if !sw.IsSystem && group.Username != sw.Username {
			return fmt.Errorf("安全组必须属于交换机用户 %s", sw.Username)
		}
	}
	return BindVMToVPC(switchOwner, vmName, switchID, securityGroupID)
}

func ApplyVPCSwitchToDomainXML(vmXML string, switchID uint) (string, error) {
	if switchID == 0 {
		return vmXML, nil
	}
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, switchID).Error; err != nil {
		return "", fmt.Errorf("VPC 交换机不存在: %w", err)
	}
	if err := EnsureVPCSwitchRuntime(sw); err != nil {
		return "", err
	}
	if HookSwitchUsesDirectBridge(sw) {
		updated, changed := setFirstOVSInterfaceDirectBridge(vmXML, HookBridgeNameForSwitch(sw), sw.BridgeVLANID)
		if !changed {
			return "", fmt.Errorf("无法在虚拟机 XML 中找到可接入桥接网桥的 OVS 网卡")
		}
		return updated, nil
	}
	// 系统基础网络交换机（VLANID == 0）：只设置 OVS 网桥源，不打 VLAN
	if sw.VLANID == 0 {
		updated, bridgeChanged := setFirstOVSInterfaceBridge(vmXML, HookOvsBridgeName())
		if !bridgeChanged {
			return "", fmt.Errorf("无法在虚拟机 XML 中找到可接入 VPC 的 OVS 网卡")
		}
		// 移除可能存在的 VLAN 标签
		updated = removeFirstInterfaceVLAN(updated)
		return updated, nil
	}
	updated, changed := setFirstOVSInterfaceVPC(vmXML, sw.VLANID)
	if !changed {
		return "", fmt.Errorf("无法在虚拟机 XML 中找到可接入 VPC 的 OVS 网卡")
	}
	return updated, nil
}

func GetVPCBindingInfo(operator, role, vmName string) (*VPCBindingInfo, error) {
	if role != "admin" && !HookUserOwnsVM(operator, vmName) {
		return nil, fmt.Errorf("无权操作此虚拟机")
	}
	isLightweightOperator := role != "admin" && HookIsLightweightCloudUser(operator)
	username := HookFindVMOwner(vmName)
	if username == "" && role != "admin" {
		username = operator
	}
	if username != "" && !isLightweightOperator {
		_, _ = EnsureDefaultSecurityGroup(username)
	}
	info := &VPCBindingInfo{}
	var allBindings []model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).Order("interface_order ASC").Find(&allBindings).Error; err == nil && len(allBindings) > 0 {
		info.Bindings = allBindings
		// 第一个绑定作为主绑定（向后兼容）
		binding := allBindings[0]
		info.Binding = &binding
		var sw model.VPCSwitch
		if model.DB.First(&sw, binding.SwitchID).Error == nil {
			normalizeVPCSwitchBandwidthForResponse(&sw)
			info.Switch = &sw
		}
		var sg model.VPCSecurityGroup
		if model.DB.Preload("Rules").First(&sg, binding.SecurityGroupID).Error == nil {
			info.SecurityGroup = &sg
		}
		if username == "" {
			username = binding.Username
		}
	}
	if quota, err := HookGetLightweightVMQuota(vmName); err == nil {
		info.LightweightQuota = quota
	}
	if isLightweightOperator {
		if info.Switch != nil {
			info.Switches = []model.VPCSwitch{*info.Switch}
		} else {
			var user model.User
			if err := model.DB.Where("username = ?", operator).First(&user).Error; err == nil && user.DedicatedVPCSwitchID > 0 {
				var sw model.VPCSwitch
				if err := model.DB.First(&sw, user.DedicatedVPCSwitchID).Error; err == nil {
					normalizeVPCSwitchBandwidthForResponse(&sw)
					info.Switches = []model.VPCSwitch{sw}
				}
			}
		}
		if info.SecurityGroup != nil && info.SecurityGroup.IsVMScoped && info.SecurityGroup.VMName == vmName {
			info.Groups = []model.VPCSecurityGroup{*info.SecurityGroup}
		} else {
			model.DB.Where("vm_name = ? AND is_vm_scoped = ?", vmName, true).Order("id ASC").Find(&info.Groups)
		}
		return info, nil
	}
	if role == "admin" {
		model.DB.Order("username ASC, id ASC").Find(&info.Switches)
		model.DB.Order("username ASC, is_default DESC, id ASC").Find(&info.Groups)
	} else if username != "" {
		model.DB.Where("username = ? AND (bridge_mode = '' OR bridge_mode = ? OR bridge_mode IS NULL)", username, BridgeModeNAT).Order("id ASC").Find(&info.Switches)
		model.DB.Where("username = ?", username).Order("is_default DESC, id ASC").Find(&info.Groups)
	}
	for i := range info.Switches {
		normalizeVPCSwitchBandwidthForResponse(&info.Switches[i])
	}
	return info, nil
}

func SwitchVMSecurityGroup(operator, role, vmName string, securityGroupID uint) error {
	if role != "admin" && HookIsLightweightCloudUser(operator) {
		return fmt.Errorf("轻量云服务器使用管理员分配的专属安全组，不能切换安全组")
	}
	info, err := GetVPCBindingInfo(operator, role, vmName)
	if err != nil {
		return err
	}
	if info.Binding == nil {
		return fmt.Errorf("请先保存 VPC 绑定，再单独切换安全组")
	}
	var group model.VPCSecurityGroup
	if err := model.DB.Where("id = ? AND username = ?", securityGroupID, info.Binding.Username).First(&group).Error; err != nil {
		return fmt.Errorf("安全组不存在或不属于该用户")
	}
	info.Binding.SecurityGroupID = securityGroupID
	if err := model.DB.Save(info.Binding).Error; err != nil {
		return err
	}
	return ApplyVPCACLRules()
}
