package vpc

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
)

func ListVPCSecurityGroups(operator, role, requestedUsername string) ([]model.VPCSecurityGroup, error) {
	if role != "admin" && HookIsLightweightCloudUser(operator) {
		vmNames := HookGetUserVMList(operator)
		if len(vmNames) == 0 {
			return []model.VPCSecurityGroup{}, nil
		}
		var groups []model.VPCSecurityGroup
		if err := model.DB.Preload("Rules").
			Where("is_vm_scoped = ? AND vm_name IN ?", true, vmNames).
			Order("vm_name ASC, id ASC").
			Find(&groups).Error; err != nil {
			return nil, err
		}
		normalizeSecurityGroupRulesForResponse(groups)
		return groups, nil
	}
	if role != "admin" {
		if _, err := EnsureDefaultSecurityGroup(operator); err != nil {
			return nil, err
		}
	} else if strings.TrimSpace(requestedUsername) != "" {
		_, _ = EnsureDefaultSecurityGroup(strings.TrimSpace(requestedUsername))
	}
	cleanupInvalidVPCSecurityGroupRules(operator, role, requestedUsername)
	query := model.DB.Preload("Rules").Model(&model.VPCSecurityGroup{})
	if role != "admin" {
		query = query.Where("username = ?", operator)
	} else if strings.TrimSpace(requestedUsername) != "" {
		query = query.Where("username = ?", strings.TrimSpace(requestedUsername))
	}
	var groups []model.VPCSecurityGroup
	if err := query.Order("username ASC, is_default DESC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	normalizeSecurityGroupRulesForResponse(groups)
	return groups, nil
}

func normalizeSecurityGroupRulesForResponse(groups []model.VPCSecurityGroup) {
	for groupIndex := range groups {
		for ruleIndex := range groups[groupIndex].Rules {
			rule := &groups[groupIndex].Rules[ruleIndex]
			rule.AddressFamily = effectiveSecurityGroupRuleAddressFamily(*rule)
			if rule.AddressFamily == "ipv6" && strings.EqualFold(rule.Protocol, "icmp") {
				rule.Protocol = "icmpv6"
			}
		}
	}
}

func cleanupInvalidVPCSecurityGroupRules(operator, role, requestedUsername string) {
	if model.DB == nil {
		return
	}
	groupQuery := model.DB.Model(&model.VPCSecurityGroup{})
	if role != "admin" {
		groupQuery = groupQuery.Where("username = ?", operator)
	} else if strings.TrimSpace(requestedUsername) != "" {
		groupQuery = groupQuery.Where("username = ?", strings.TrimSpace(requestedUsername))
	}
	var groups []model.VPCSecurityGroup
	if err := groupQuery.Find(&groups).Error; err != nil || len(groups) == 0 {
		return
	}
	groupUsernames := map[uint]string{}
	groupIDs := make([]uint, 0, len(groups))
	for _, group := range groups {
		groupIDs = append(groupIDs, group.ID)
		groupUsernames[group.ID] = group.Username
	}
	var rules []model.VPCSecurityGroupRule
	if err := model.DB.Where("security_group_id IN ? AND target_type IN ?", groupIDs, []string{"switch", "security_group"}).Find(&rules).Error; err != nil {
		return
	}
	for _, rule := range rules {
		username := groupUsernames[rule.SecurityGroupID]
		if err := validateSecurityGroupRuleTarget(username, rule.TargetType, rule.TargetValue); err != nil {
			logger.App.Warn("清理异常安全组规则", "id", rule.ID, "error", err)
			_ = model.DB.Delete(&rule).Error
		}
	}
}

func CreateVPCSecurityGroup(operator, role string, req VPCSecurityGroupRequest) (*model.VPCSecurityGroup, error) {
	if role != "admin" && HookIsLightweightCloudUser(operator) {
		return nil, fmt.Errorf("轻量云用户不能创建全局安全组")
	}
	username, err := resolveVPCUsername(operator, role, req.Username)
	if err != nil {
		return nil, err
	}
	req.Name = normalizeVPCName(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("安全组名称不能为空")
	}
	var count int64
	model.DB.Model(&model.VPCSecurityGroup{}).Where("username = ? AND name = ?", username, req.Name).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("安全组名称已存在")
	}
	group := &model.VPCSecurityGroup{Username: username, Name: req.Name, Remark: strings.TrimSpace(req.Remark)}
	if err := model.DB.Create(group).Error; err != nil {
		return nil, err
	}
	// 安全组默认全放通开关开启时，自动添加 IPv4/IPv6 全放通入站规则
	if config.GlobalConfig != nil && config.GlobalConfig.SecurityGroupDefaultAllowAll {
		if err := appendDefaultAllowAllRules(group.ID); err != nil {
			logger.App.Warn("新建安全组自动添加全放通规则失败", "group_id", group.ID, "error", err)
		}
	}
	return group, nil
}

// appendDefaultAllowAllRules 为安全组添加 IPv4 和 IPv6 全放通入站规则。
// 协议为 all，目标为 0.0.0.0/0 和 ::/0，方向为 ingress。
func appendDefaultAllowAllRules(groupID uint) error {
	rules := []model.VPCSecurityGroupRule{
		{
			SecurityGroupID: groupID,
			Direction:       "ingress",
			AddressFamily:   "ipv4",
			Protocol:        "all",
			PortStart:       0,
			PortEnd:         0,
			TargetType:      "cidr",
			TargetValue:     "0.0.0.0/0",
			Remark:          "系统默认全放通规则（IPv4）",
		},
		{
			SecurityGroupID: groupID,
			Direction:       "ingress",
			AddressFamily:   "ipv6",
			Protocol:        "all",
			PortStart:       0,
			PortEnd:         0,
			TargetType:      "cidr",
			TargetValue:     "::/0",
			Remark:          "系统默认全放通规则（IPv6）",
		},
	}
	for i := range rules {
		if err := model.DB.Create(&rules[i]).Error; err != nil {
			return fmt.Errorf("创建全放通规则失败: %w", err)
		}
	}
	return nil
}

func UpdateVPCSecurityGroup(operator, role string, id uint, req VPCSecurityGroupRequest) (*model.VPCSecurityGroup, error) {
	if role != "admin" && HookIsLightweightCloudUser(operator) {
		return nil, fmt.Errorf("轻量云用户不能修改安全组")
	}
	var group model.VPCSecurityGroup
	if err := model.DB.First(&group, id).Error; err != nil {
		return nil, fmt.Errorf("安全组不存在")
	}
	if role != "admin" && group.Username != operator {
		return nil, fmt.Errorf("无权操作此安全组")
	}
	nextName := group.Name
	if strings.TrimSpace(req.Name) != "" {
		nextName = normalizeVPCName(req.Name)
		if nextName == "" {
			return nil, fmt.Errorf("安全组名称不能为空")
		}
	}
	if group.IsDefault && nextName != group.Name {
		return nil, fmt.Errorf("默认安全组不能修改名称")
	}
	if nextName != group.Name {
		var count int64
		model.DB.Model(&model.VPCSecurityGroup{}).
			Where("username = ? AND name = ? AND id <> ?", group.Username, nextName, group.ID).
			Count(&count)
		if count > 0 {
			return nil, fmt.Errorf("安全组名称已存在")
		}
		group.Name = nextName
	}
	group.Remark = strings.TrimSpace(req.Remark)
	if err := model.DB.Save(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func DeleteVPCSecurityGroup(operator, role string, id uint) error {
	var group model.VPCSecurityGroup
	if err := model.DB.First(&group, id).Error; err != nil {
		return fmt.Errorf("安全组不存在")
	}
	if role != "admin" && HookIsLightweightCloudUser(operator) {
		return fmt.Errorf("轻量云用户不能删除安全组")
	}
	if role != "admin" && group.Username != operator {
		return fmt.Errorf("无权操作此安全组")
	}
	if group.IsDefault {
		return fmt.Errorf("默认安全组不能删除")
	}
	var count int64
	model.DB.Model(&model.VPCVMBinding{}).Where("security_group_id = ?", id).Count(&count)
	if count > 0 {
		return fmt.Errorf("安全组仍被虚拟机使用，不能删除")
	}
	model.DB.Where("security_group_id = ?", id).Delete(&model.VPCSecurityGroupRule{})
	return model.DB.Delete(&group).Error
}

func AddVPCSecurityGroupRule(operator, role string, groupID uint, req VPCSecurityGroupRuleRequest) (*model.VPCSecurityGroupRule, error) {
	var group model.VPCSecurityGroup
	if err := model.DB.First(&group, groupID).Error; err != nil {
		return nil, fmt.Errorf("安全组不存在")
	}
	if role != "admin" && group.Username != operator && !(group.IsVMScoped && HookUserOwnsVM(operator, group.VMName)) {
		return nil, fmt.Errorf("无权操作此安全组")
	}
	if role != "admin" && HookIsLightweightCloudUser(operator) {
		targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
		if targetType == "" {
			targetType = "cidr"
		}
		if targetType != "cidr" {
			return nil, fmt.Errorf("轻量云安全组规则仅支持 CIDR 目标")
		}
	}
	rule, err := normalizeSecurityGroupRule(groupID, req)
	if err != nil {
		return nil, err
	}
	if err := validateSecurityGroupRuleTarget(group.Username, rule.TargetType, rule.TargetValue); err != nil {
		return nil, err
	}
	if err := model.DB.Create(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

func DeleteVPCSecurityGroupRule(operator, role string, ruleID uint) error {
	var rule model.VPCSecurityGroupRule
	if err := model.DB.First(&rule, ruleID).Error; err != nil {
		return fmt.Errorf("安全组规则不存在")
	}
	var group model.VPCSecurityGroup
	if err := model.DB.First(&group, rule.SecurityGroupID).Error; err != nil {
		if role == "admin" {
			return model.DB.Delete(&rule).Error
		}
		return fmt.Errorf("安全组不存在")
	}
	if role != "admin" && group.Username != operator && !(group.IsVMScoped && HookUserOwnsVM(operator, group.VMName)) {
		return fmt.Errorf("无权操作此安全组规则")
	}
	return model.DB.Delete(&rule).Error
}

func normalizeSecurityGroupRule(groupID uint, req VPCSecurityGroupRuleRequest) (*model.VPCSecurityGroupRule, error) {
	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	if direction == "" {
		direction = "ingress"
	}
	if direction != "ingress" && direction != "egress" {
		return nil, fmt.Errorf("方向只支持 ingress 或 egress")
	}
	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	if proto == "" {
		proto = "tcp"
	}
	if proto != "tcp" && proto != "udp" && proto != "icmp" && proto != "icmpv6" && proto != "all" {
		return nil, fmt.Errorf("协议只支持 tcp/udp/icmp/icmpv6/all")
	}
	addressFamily := strings.ToLower(strings.TrimSpace(req.AddressFamily))
	targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
	if targetType == "" {
		targetType = "cidr"
	}
	if targetType != "cidr" && targetType != "switch" && targetType != "security_group" {
		return nil, fmt.Errorf("目标类型无效")
	}
	targetValue := strings.TrimSpace(req.TargetValue)
	if targetValue == "" {
		if targetType == "cidr" {
			if addressFamily == "ipv6" {
				targetValue = "::/0"
			} else {
				targetValue = "0.0.0.0/0"
			}
		} else {
			return nil, fmt.Errorf("请选择目标交换机或安全组")
		}
	}
	if targetType == "cidr" {
		prefix, err := netip.ParsePrefix(normalizeCIDROrIP(targetValue))
		if err != nil {
			return nil, fmt.Errorf("CIDR 无效: %s", targetValue)
		}
		targetValue = normalizeCIDROrIP(targetValue)
		targetFamily := "ipv4"
		if prefix.Addr().Is6() {
			targetFamily = "ipv6"
		}
		if addressFamily != "" && addressFamily != targetFamily {
			return nil, fmt.Errorf("地址族与 CIDR 不一致")
		}
		addressFamily = targetFamily
	}
	if addressFamily == "" {
		// 兼容未传地址族的旧客户端；非 CIDR 目标沿用历史 IPv4 语义。
		addressFamily = "ipv4"
	}
	if addressFamily != "ipv4" && addressFamily != "ipv6" {
		return nil, fmt.Errorf("地址族只支持 ipv4 或 ipv6")
	}
	if addressFamily == "ipv6" && proto == "icmp" {
		proto = "icmpv6"
	}
	if addressFamily == "ipv4" && proto == "icmpv6" {
		return nil, fmt.Errorf("ICMPv6 仅适用于 IPv6 规则")
	}
	if req.PortEnd == 0 {
		req.PortEnd = req.PortStart
	}
	if (proto == "tcp" || proto == "udp") && (req.PortStart < 1 || req.PortStart > 65535 || req.PortEnd < req.PortStart || req.PortEnd > 65535) {
		return nil, fmt.Errorf("端口范围无效")
	}
	if proto == "icmp" || proto == "icmpv6" || proto == "all" {
		req.PortStart = 0
		req.PortEnd = 0
	}
	return &model.VPCSecurityGroupRule{
		SecurityGroupID: groupID,
		Direction:       direction,
		AddressFamily:   addressFamily,
		Protocol:        proto,
		PortStart:       req.PortStart,
		PortEnd:         req.PortEnd,
		TargetType:      targetType,
		TargetValue:     targetValue,
		Remark:          strings.TrimSpace(req.Remark),
	}, nil
}

// effectiveSecurityGroupRuleAddressFamily 为历史规则推导地址族。
// 旧表没有 address_family 字段时，IPv6 CIDR 规则仍应继续按 IPv6 生效。
func effectiveSecurityGroupRuleAddressFamily(rule model.VPCSecurityGroupRule) string {
	if rule.TargetType == "cidr" {
		if prefix, err := netip.ParsePrefix(normalizeCIDROrIP(rule.TargetValue)); err == nil && prefix.Addr().Is6() {
			return "ipv6"
		}
	}
	if strings.EqualFold(strings.TrimSpace(rule.Protocol), "icmpv6") {
		return "ipv6"
	}
	if strings.EqualFold(strings.TrimSpace(rule.AddressFamily), "ipv6") {
		return "ipv6"
	}
	return "ipv4"
}

func validateSecurityGroupRuleTarget(username, targetType, targetValue string) error {
	switch targetType {
	case "switch":
		id, err := strconv.Atoi(strings.TrimSpace(targetValue))
		if err != nil || id <= 0 {
			return fmt.Errorf("请选择有效的目标交换机")
		}
		var count int64
		model.DB.Model(&model.VPCSwitch{}).Where("id = ? AND username = ?", id, username).Count(&count)
		if count == 0 {
			return fmt.Errorf("目标交换机不存在或不属于该用户")
		}
	case "security_group":
		id, err := strconv.Atoi(strings.TrimSpace(targetValue))
		if err != nil || id <= 0 {
			return fmt.Errorf("请选择有效的目标安全组")
		}
		var count int64
		model.DB.Model(&model.VPCSecurityGroup{}).Where("id = ? AND username = ?", id, username).Count(&count)
		if count == 0 {
			return fmt.Errorf("目标安全组不存在或不属于该用户")
		}
	}
	return nil
}
