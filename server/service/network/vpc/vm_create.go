package vpc

import (
	"fmt"
	"strconv"
	"strings"

	"kvm_console/model"
	"kvm_console/service/ip_resolver"
)

func ApplyVPCBindingRuntime(vmName string) error {
	if model.DB == nil || strings.TrimSpace(vmName) == "" {
		return nil
	}
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).First(&binding).Error; err != nil {
		return nil
	}
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, binding.SwitchID).Error; err != nil {
		return fmt.Errorf("VPC 交换机不存在: %w", err)
	}
	return ApplyVPCSwitchRuntime(vmName, sw)
}

func EnsureVPCForVMCreate(username string, switchID, securityGroupID uint) error {
	_, _, err := ResolveVPCForVMCreate(username, switchID, securityGroupID)
	return err
}

func ResolveVPCForVMCreate(username string, switchID, securityGroupID uint) (uint, uint, error) {
	if _, err := EnsureDefaultSecurityGroup(username); err != nil {
		return 0, 0, err
	}
	if _, err := EnsureDefaultVPCSwitch(username); err != nil {
		return 0, 0, err
	}
	if switchID == 0 {
		resolvedSwitchID, err := resolveDefaultVPCSwitchIDForVMCreate(username)
		if err != nil {
			return 0, 0, err
		}
		switchID = resolvedSwitchID
	}
	if securityGroupID == 0 {
		resolvedGroupID, err := resolveDefaultVPCSecurityGroupIDForVMCreate(username)
		if err != nil {
			return 0, 0, err
		}
		securityGroupID = resolvedGroupID
	}
	var sw model.VPCSwitch
	if err := model.DB.Where("id = ?", switchID).First(&sw).Error; err != nil {
		return 0, 0, fmt.Errorf("交换机不存在")
	}
	// 系统交换机人人可用；用户交换机需要校验归属
	if !sw.IsSystem && sw.Username != username {
		return 0, 0, fmt.Errorf("交换机不属于当前用户")
	}
	if HookSwitchUsesDirectBridge(sw) {
		return 0, 0, fmt.Errorf("桥接直通交换机仅管理员可用于创建虚拟机")
	}
	// 系统基础网络交换机不检查流量配额（不限）
	if !sw.IsSystem {
		var user model.User
		if err := model.DB.Where("username = ?", username).First(&user).Error; err == nil {
			if user.MaxTrafficDown > 0 && sw.TrafficDownGB <= 0 {
				return 0, 0, fmt.Errorf("交换机下行月流量配额不足，无法创建虚拟机")
			}
			if user.MaxTrafficUp > 0 && sw.TrafficUpGB <= 0 {
				return 0, 0, fmt.Errorf("交换机上行月流量配额不足，无法创建虚拟机")
			}
			if user.MaxBandwidthDown > 0 && sw.BandwidthDownMbps <= 0 {
				return 0, 0, fmt.Errorf("交换机下行总带宽配额不足，无法创建虚拟机")
			}
			if user.MaxBandwidthUp > 0 && sw.BandwidthUpMbps <= 0 {
				return 0, 0, fmt.Errorf("交换机上行总带宽配额不足，无法创建虚拟机")
			}
		}
	}
	// 安全组校验：系统交换机使用用户自己的默认安全组
	switchOwner := sw.Username
	if sw.IsSystem {
		switchOwner = username
	}
	var sg model.VPCSecurityGroup
	if err := model.DB.Where("id = ? AND username = ?", securityGroupID, switchOwner).First(&sg).Error; err != nil {
		return 0, 0, fmt.Errorf("安全组不存在或不属于当前用户")
	}
	return switchID, securityGroupID, nil
}

func resolveDefaultVPCSwitchIDForVMCreate(username string) (uint, error) {
	// 优先使用系统基础网络交换机
	var sysSwitch model.VPCSwitch
	if err := model.DB.Where("is_system = ?", true).First(&sysSwitch).Error; err == nil {
		return sysSwitch.ID, nil
	}
	var switches []model.VPCSwitch
	if err := model.DB.Where("username = ?", username).Order("id ASC").Find(&switches).Error; err != nil {
		return 0, err
	}
	if len(switches) == 0 {
		return 0, fmt.Errorf("请先在 VPC 网络中创建交换机后再创建虚拟机")
	}
	if len(switches) == 1 {
		return switches[0].ID, nil
	}
	for _, sw := range switches {
		if sw.Name == DefaultVPCSwitchName {
			return sw.ID, nil
		}
	}
	return 0, fmt.Errorf("请选择要接入的 VPC 交换机")
}

func resolveDefaultVPCSecurityGroupIDForVMCreate(username string) (uint, error) {
	var group model.VPCSecurityGroup
	if err := model.DB.Where("username = ? AND is_default = ?", username, true).First(&group).Error; err == nil {
		return group.ID, nil
	}
	if err := model.DB.Where("username = ?", username).Order("id ASC").First(&group).Error; err == nil {
		return group.ID, nil
	}
	return 0, fmt.Errorf("请选择要应用的安全组")
}

func EnsureSecurityGroupAllowsPortForward(vmName, protocol, portText string) error {
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).First(&binding).Error; err != nil {
		return nil
	}
	portStart, portEnd, err := parsePortForwardPortRange(portText)
	if err != nil {
		return err
	}
	protocols := []string{strings.ToLower(strings.TrimSpace(protocol))}
	if protocols[0] == "" {
		protocols[0] = "tcp"
	}
	if protocols[0] == "both" {
		protocols = []string{"tcp", "udp"}
	}
	for _, proto := range protocols {
		var count int64
		model.DB.Model(&model.VPCSecurityGroupRule{}).
			Where("security_group_id = ? AND direction = ? AND protocol = ? AND port_start <= ? AND port_end >= ? AND target_type = ? AND target_value = ?",
				binding.SecurityGroupID, "ingress", proto, portStart, portEnd, "cidr", "0.0.0.0/0").
			Count(&count)
		if count > 0 {
			continue
		}
		rule := model.VPCSecurityGroupRule{
			SecurityGroupID: binding.SecurityGroupID,
			Direction:       "ingress",
			Protocol:        proto,
			PortStart:       portStart,
			PortEnd:         portEnd,
			TargetType:      "cidr",
			TargetValue:     "0.0.0.0/0",
			Remark:          AutoPortForwardSecurityGroupRuleNote,
		}
		if err := model.DB.Create(&rule).Error; err != nil {
			return err
		}
	}
	return ApplyVPCACLRules()
}

func parsePortForwardPortRange(portText string) (int, int, error) {
	ports := strings.Split(strings.ReplaceAll(strings.TrimSpace(portText), "-", ":"), ":")
	if len(ports) == 0 || strings.TrimSpace(ports[0]) == "" {
		return 0, 0, fmt.Errorf("端口不能为空")
	}
	portStart, err := strconv.Atoi(strings.TrimSpace(ports[0]))
	if err != nil || portStart <= 0 || portStart > 65535 {
		return 0, 0, fmt.Errorf("端口格式无效")
	}
	portEnd := portStart
	if len(ports) > 1 {
		portEnd, err = strconv.Atoi(strings.TrimSpace(ports[1]))
		if err != nil || portEnd <= 0 || portEnd > 65535 {
			return 0, 0, fmt.Errorf("端口格式无效")
		}
	}
	if portEnd < portStart {
		return 0, 0, fmt.Errorf("端口范围无效")
	}
	return portStart, portEnd, nil
}

func RemoveSecurityGroupAllowsPortForwardIfUnused(destIP, protocol, portText string) error {
	destIP = strings.TrimSpace(destIP)
	if destIP == "" || strings.TrimSpace(portText) == "" || !IsVPCManagedIP(destIP) {
		return nil
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		protocol = "tcp"
	}
	portStart, portEnd, err := parsePortForwardPortRange(portText)
	if err != nil {
		return err
	}
	binding, ok, err := findVPCBindingByIP(destIP)
	if err != nil || !ok {
		return err
	}
	if portForwardTargetStillExists(destIP, protocol, portStart, portEnd) {
		return nil
	}
	deleted, err := deleteAutoSecurityGroupPortForwardRules(binding.SecurityGroupID, protocol, portStart, portEnd)
	if err != nil || deleted == 0 {
		return err
	}
	return ApplyVPCACLRules()
}

func portForwardTargetStillExists(destIP, protocol string, portStart, portEnd int) bool {
	rules, err := HookListLivePortForwards()
	if err != nil {
		return false
	}
	for _, rule := range rules {
		if strings.TrimSpace(rule.DestIP) != strings.TrimSpace(destIP) {
			continue
		}
		if strings.ToLower(strings.TrimSpace(rule.Protocol)) != strings.ToLower(strings.TrimSpace(protocol)) {
			continue
		}
		ruleStart, ruleEnd, err := parsePortForwardPortRange(rule.DestPort)
		if err != nil {
			continue
		}
		if ruleStart == portStart && ruleEnd == portEnd {
			return true
		}
	}
	return false
}

func deleteAutoSecurityGroupPortForwardRules(securityGroupID uint, protocol string, portStart, portEnd int) (int64, error) {
	result := model.DB.Where(
		"security_group_id = ? AND direction = ? AND protocol = ? AND port_start = ? AND port_end = ? AND target_type = ? AND target_value = ? AND remark = ?",
		securityGroupID, "ingress", strings.ToLower(strings.TrimSpace(protocol)), portStart, portEnd, "cidr", "0.0.0.0/0", AutoPortForwardSecurityGroupRuleNote,
	).Delete(&model.VPCSecurityGroupRule{})
	return result.RowsAffected, result.Error
}

func findVPCBindingByIP(ipText string) (*model.VPCVMBinding, bool, error) {
	ipText = strings.TrimSpace(ipText)
	if ipText == "" || model.DB == nil {
		return nil, false, nil
	}
	var switches []model.VPCSwitch
	if err := model.DB.Find(&switches).Error; err != nil {
		return nil, false, err
	}
	for _, sw := range switches {
		if !IPInCIDR(ipText, sw.CIDR) {
			continue
		}
		if binding, ok, err := findVPCBindingByStaticHostIP(sw.ID, ipText); err != nil || ok {
			return binding, ok, err
		}
		if binding, ok, err := findVPCBindingByLeaseIP(sw.ID, ipText); err != nil || ok {
			return binding, ok, err
		}
		var bindings []model.VPCVMBinding
		if err := model.DB.Where("switch_id = ?", sw.ID).Find(&bindings).Error; err != nil {
			return nil, false, err
		}
		for _, binding := range bindings {
			if HookGetFirewallVMIP(binding.VMName) == ipText {
				return &binding, true, nil
			}
		}
	}
	return nil, false, nil
}

func findVPCBindingByStaticHostIP(switchID uint, ipText string) (*model.VPCVMBinding, bool, error) {
	hosts, err := HookListVPCStaticHosts(switchID)
	if err != nil {
		return nil, false, err
	}
	for _, host := range hosts {
		if strings.TrimSpace(host.IP) != ipText {
			continue
		}
		var binding model.VPCVMBinding
		query := model.DB.Where("switch_id = ?", switchID)
		if strings.TrimSpace(host.VMName) != "" {
			if err := query.Where("vm_name = ?", strings.TrimSpace(host.VMName)).First(&binding).Error; err == nil {
				return &binding, true, nil
			}
		}
		if strings.TrimSpace(host.MAC) == "" {
			continue
		}
		var bindings []model.VPCVMBinding
		if err := model.DB.Where("switch_id = ?", switchID).Find(&bindings).Error; err != nil {
			return nil, false, err
		}
		for _, candidate := range bindings {
			if strings.EqualFold(ip_resolver.GetFirstVMMAC(candidate.VMName), host.MAC) {
				return &candidate, true, nil
			}
		}
	}
	return nil, false, nil
}

func findVPCBindingByLeaseIP(switchID uint, ipText string) (*model.VPCVMBinding, bool, error) {
	leases, err := HookListVPCDHCPLeases(switchID)
	if err != nil {
		return nil, false, err
	}
	for _, lease := range leases {
		if strings.TrimSpace(lease.IP) != ipText {
			continue
		}
		var bindings []model.VPCVMBinding
		if err := model.DB.Where("switch_id = ?", switchID).Find(&bindings).Error; err != nil {
			return nil, false, err
		}
		for _, binding := range bindings {
			if strings.EqualFold(ip_resolver.GetFirstVMMAC(binding.VMName), lease.MAC) {
				return &binding, true, nil
			}
		}
	}
	return nil, false, nil
}
