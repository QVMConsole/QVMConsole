package vpc

import (
	"fmt"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
)

func ListVPCSwitches(operator, role, requestedUsername string) ([]model.VPCSwitch, error) {
	query := model.DB.Model(&model.VPCSwitch{})
	if role != "admin" {
		// 非管理员：自己的 NAT 交换机 + 系统基础网络交换机
		query = query.Where("(username = ? AND (bridge_mode = '' OR bridge_mode = ? OR bridge_mode IS NULL)) OR is_system = ?", operator, BridgeModeNAT, true)
	} else if strings.TrimSpace(requestedUsername) != "" {
		query = query.Where("username = ?", strings.TrimSpace(requestedUsername))
	}
	var switches []model.VPCSwitch
	if err := query.Order("is_system DESC, username ASC, id ASC").Find(&switches).Error; err != nil {
		return nil, err
	}
	for i := range switches {
		fillVPCSwitchUsageForResponse(&switches[i])
	}
	return switches, nil
}

func CreateVPCSwitch(operator, role string, req VPCSwitchRequest) (*model.VPCSwitch, error) {
	username, err := resolveVPCUsername(operator, role, req.Username)
	if err != nil {
		return nil, err
	}
	bridgeName, bridgeMode, err := resolveVPCSwitchBridge(role, req.BridgeName)
	if err != nil {
		return nil, err
	}
	if err := validateBridgeVLANID(bridgeMode, req.BridgeVLANID); err != nil {
		return nil, err
	}
	if _, err := EnsureDefaultSecurityGroup(username); err != nil {
		return nil, err
	}
	req.Name = normalizeVPCName(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("交换机名称不能为空")
	}
	normalizeVPCSwitchBandwidthRequest(&req)
	if err := checkSwitchResourceQuota(username, 0, req); err != nil {
		return nil, err
	}
	var count int64
	model.DB.Model(&model.VPCSwitch{}).Where("username = ? AND name = ?", username, req.Name).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("交换机名称已存在")
	}
	vlanID, err := allocateVPCVLANID()
	if err != nil {
		return nil, err
	}
	cidr, gateway, dhcpStart, dhcpEnd, err := allocateVPCSubnet()
	if err != nil {
		return nil, err
	}
	sw := &model.VPCSwitch{
		Username:             username,
		Name:                 req.Name,
		BridgeName:           bridgeName,
		BridgeMode:           bridgeMode,
		BridgeVLANID:         normalizedBridgeVLANID(bridgeMode, req.BridgeVLANID),
		AllowPromiscuous:     bridgeMode == BridgeModeDirect && req.AllowPromiscuous,
		AllowMACChange:       bridgeMode == BridgeModeDirect && req.AllowMACChange,
		AllowForgedTransmits: bridgeMode == BridgeModeDirect && req.AllowForgedTx,
		VLANID:               vlanID,
		CIDR:                 cidr,
		GatewayIP:            gateway,
		DHCPStart:            dhcpStart,
		DHCPEnd:              dhcpEnd,
		TrafficDownGB:        req.TrafficDownGB,
		TrafficUpGB:          req.TrafficUpGB,
		BandwidthMbps:        req.BandwidthMbps,
		BandwidthDownMbps:    req.BandwidthDownMbps,
		BandwidthUpMbps:      req.BandwidthUpMbps,
	}
	if err := model.DB.Create(sw).Error; err != nil {
		return nil, fmt.Errorf("创建交换机失败: %w", err)
	}
	if err := EnsureVPCSwitchRuntime(*sw); err != nil {
		return sw, err
	}
	return sw, nil
}

func UpdateVPCSwitch(operator, role string, id uint, req VPCSwitchRequest) (*model.VPCSwitch, error) {
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, id).Error; err != nil {
		return nil, fmt.Errorf("交换机不存在")
	}
	if sw.IsSystem {
		return nil, fmt.Errorf("系统基础网络交换机不可编辑")
	}
	if role != "admin" && sw.Username != operator {
		return nil, fmt.Errorf("无权操作此交换机")
	}
	if req.BridgeName != "" && req.BridgeName != sw.BridgeName {
		return nil, fmt.Errorf("暂不支持修改交换机目标网桥")
	}
	if err := validateBridgeVLANID(HookBridgeModeForSwitch(sw), req.BridgeVLANID); err != nil {
		return nil, err
	}
	if req.Name = normalizeVPCName(req.Name); req.Name != "" {
		sw.Name = req.Name
	}
	sw.BridgeVLANID = normalizedBridgeVLANID(HookBridgeModeForSwitch(sw), req.BridgeVLANID)
	if HookSwitchUsesDirectBridge(sw) {
		sw.AllowPromiscuous = req.AllowPromiscuous
		sw.AllowMACChange = req.AllowMACChange
		sw.AllowForgedTransmits = req.AllowForgedTx
	} else {
		sw.AllowPromiscuous = false
		sw.AllowMACChange = false
		sw.AllowForgedTransmits = false
	}
	sw.TrafficDownGB = req.TrafficDownGB
	sw.TrafficUpGB = req.TrafficUpGB
	normalizeVPCSwitchBandwidthRequest(&req)
	sw.BandwidthMbps = req.BandwidthMbps
	sw.BandwidthDownMbps = req.BandwidthDownMbps
	sw.BandwidthUpMbps = req.BandwidthUpMbps
	if err := checkSwitchResourceQuota(sw.Username, sw.ID, req); err != nil {
		return nil, err
	}
	if err := model.DB.Save(&sw).Error; err != nil {
		return nil, err
	}
	if HookSwitchUsesDirectBridge(sw) {
		for _, vmName := range listVPCSwitchVMNames(sw) {
			if err := ApplyVPCSwitchRuntime(vmName, sw); err != nil {
				return nil, err
			}
		}
	}
	CheckVPCSwitchTrafficAfterQuotaUpdate(sw.ID)
	_ = EnsureVPCSwitchRuntime(sw)
	fillVPCSwitchUsageForResponse(&sw)
	return &sw, nil
}

func resolveVPCSwitchBridge(role, requested string) (string, string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" || requested == HookOvsBridgeName() {
		return HookOvsBridgeName(), BridgeModeNAT, nil
	}
	if role != "admin" {
		return "", "", fmt.Errorf("仅管理员可创建桥接直通交换机")
	}
	var bridge model.NetworkBridge
	if err := model.DB.Where("name = ? AND mode = ?", requested, BridgeModeDirect).First(&bridge).Error; err != nil {
		return "", "", fmt.Errorf("桥接网桥不存在")
	}
	return bridge.Name, BridgeModeDirect, nil
}

func validateBridgeVLANID(bridgeMode string, vlanID int) error {
	if HookNormalizeBridgeMode(bridgeMode) != BridgeModeDirect {
		return nil
	}
	if vlanID < 0 || vlanID > 4094 {
		return fmt.Errorf("桥接 VLAN ID 必须为 0-4094，0 表示不打 VLAN")
	}
	return nil
}

func normalizedBridgeVLANID(bridgeMode string, vlanID int) int {
	if HookNormalizeBridgeMode(bridgeMode) != BridgeModeDirect {
		return 0
	}
	if vlanID < 0 || vlanID > 4094 {
		return 0
	}
	return vlanID
}

func ResetVPCSwitchTraffic(operator, role string, id uint) error {
	if role != "admin" {
		return fmt.Errorf("仅管理员可重置交换机流量计数器")
	}
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, id).Error; err != nil {
		return fmt.Errorf("交换机不存在")
	}
	rawDown, rawUp := aggregateSwitchMonthlyTrafficRaw(id)
	record := getOrCreateVPCSwitchTrafficMonthly(sw, CurrentTrafficMonth())
	record.OffsetDown = rawDown
	record.OffsetUp = rawUp
	record.TrafficDown = 0
	record.TrafficUp = 0
	record.IsLimitedDown = false
	record.IsLimitedUp = false
	if err := saveVPCSwitchTrafficMonthly(record); err != nil {
		return err
	}
	if err := ApplyVPCSwitchBandwidth(sw); err != nil {
		return fmt.Errorf("解除交换机限速失败: %w", err)
	}
	logger.App.Info("管理员已重置交换机流量计数器", "operator", operator, "switch", sw.Name, "id", sw.ID)
	return nil
}

func DeleteVPCSwitch(operator, role string, id uint) error {
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, id).Error; err != nil {
		return fmt.Errorf("交换机不存在")
	}
	if sw.IsSystem {
		return fmt.Errorf("系统基础网络交换机不可删除")
	}
	if role != "admin" && sw.Username != operator {
		return fmt.Errorf("无权操作此交换机")
	}
	var count int64
	model.DB.Model(&model.VPCVMBinding{}).Where("switch_id = ?", id).Count(&count)
	if count > 0 {
		return fmt.Errorf("交换机仍有虚拟机绑定，不能删除")
	}
	if err := model.DB.Delete(&sw).Error; err != nil {
		return err
	}
	_ = removeVPCSwitchRuntime(sw)
	return nil
}

func allocateVPCVLANID() (int, error) {
	start, end := config.GlobalConfig.VPCVLANStart, config.GlobalConfig.VPCVLANEnd
	if start <= 0 {
		start = 100
	}
	if end < start {
		end = 4094
	}
	var switches []model.VPCSwitch
	model.DB.Find(&switches)
	used := map[int]bool{}
	for _, sw := range switches {
		used[sw.VLANID] = true
	}
	for id := start; id <= end; id++ {
		if !used[id] {
			return id, nil
		}
	}
	return 0, fmt.Errorf("VLAN 范围 %d-%d 内没有可用 ID", start, end)
}

func allocateVPCSubnet() (cidr, gateway, dhcpStart, dhcpEnd string, err error) {
	prefix := strings.Trim(config.GlobalConfig.VPCSubnetPrefix, ". ")
	if prefix == "" {
		prefix = "10.200"
	}
	var switches []model.VPCSwitch
	model.DB.Find(&switches)
	used := map[string]bool{}
	for _, sw := range switches {
		used[sw.CIDR] = true
	}
	for i := 1; i <= 254; i++ {
		base := fmt.Sprintf("%s.%d", prefix, i)
		candidate := base + ".0/24"
		if !used[candidate] {
			return candidate, base + ".1", base + ".10", base + ".250", nil
		}
	}
	return "", "", "", "", fmt.Errorf("VPC 子网池 %s.1-254 已用尽", prefix)
}
