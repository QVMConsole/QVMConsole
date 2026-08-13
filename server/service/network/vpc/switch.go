package vpc

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/utils"
)

func ListVPCSwitches(operator, role, requestedUsername string) ([]model.VPCSwitch, error) {
	query := model.DB.Model(&model.VPCSwitch{})
	if role != "admin" {
		// 非管理员可以使用自己的空交换机，以及系统基础网络交换机。
		query = query.Where("username = ? OR is_system = ?", operator, true)
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
	vpcTopologyMutationMu.Lock()
	defer vpcTopologyMutationMu.Unlock()
	username, err := resolveVPCUsername(operator, role, req.Username)
	if err != nil {
		return nil, err
	}
	normalizeCreateTopology(role, &req)
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
	bridgeName, bridgeMode, ownsBridge, err := resolveNewVPCSwitchBridge(role, vlanID, &req)
	if err != nil {
		return nil, err
	}
	if err := validateBridgeVLANID(bridgeMode, req.BridgeVLANID); err != nil {
		return nil, err
	}
	if err := normalizeSwitchPortSecurityFields(&req, bridgeMode == BridgeModeDirect && strings.TrimSpace(req.UplinkIF) != ""); err != nil {
		return nil, err
	}
	if req.UplinkMode == UplinkModePhysical && HookValidateSwitchUplink != nil {
		if err := HookValidateSwitchUplink(req.UplinkIF, req.UplinkGateway, req.DHCPEnabled, 0, bridgeName, req.BridgeVLANID); err != nil {
			return nil, err
		}
	}
	if ownsBridge {
		if err := validateHostIPMigrationSelection(req); err != nil {
			return nil, err
		}
	}
	if req.DHCPEnabled && req.UplinkMode != UplinkModePhysical {
		return nil, fmt.Errorf("托管 DHCP/NAT 交换机需要选择物理上行链路")
	}
	// 解析网段：优先使用用户指定的 CIDR，否则自动分配
	cidr, gateway, dhcpStart, dhcpEnd, err := resolveVPCSwitchSubnet(req.DHCPEnabled, req)
	if err != nil {
		return nil, err
	}
	hostAddrs, hostGateway, hostMetric, hostDNS := "", "", "", ""
	if ownsBridge && bridgeMode == BridgeModeDirect && req.MigrateHostIP && req.UplinkIF != "" && HookCaptureHostIPConfig != nil {
		hostAddrs, hostGateway, hostMetric, hostDNS = HookCaptureHostIPConfig(req.UplinkIF)
	}
	sw := &model.VPCSwitch{
		Username:             username,
		Name:                 req.Name,
		BridgeName:           bridgeName,
		BridgeMode:           bridgeMode,
		DHCPEnabled:          req.DHCPEnabled,
		UplinkMode:           req.UplinkMode,
		UplinkIF:             req.UplinkIF,
		UplinkGateway:        strings.TrimSpace(req.UplinkGateway),
		OwnsBridge:           ownsBridge,
		MigrateHostIP:        ownsBridge && bridgeMode == BridgeModeDirect && req.MigrateHostIP,
		HostAddrs:            hostAddrs,
		HostGateway:          hostGateway,
		HostMetric:           hostMetric,
		HostDNS:              hostDNS,
		BridgeVLANID:         normalizedBridgeVLANID(bridgeMode, req.BridgeVLANID),
		AllowPromiscuous:     bridgeMode == BridgeModeDirect && req.UplinkIF != "" && req.AllowPromiscuous,
		AllowMACChange:       bridgeMode == BridgeModeDirect && req.UplinkIF != "" && req.AllowMACChange,
		AllowForgedTransmits: bridgeMode == BridgeModeDirect && req.UplinkIF != "" && req.AllowForgedTx,
		IPv6SecurityEnabled:  bridgeMode == BridgeModeDirect && req.UplinkIF != "" && req.IPv6SecurityEnabled,
		TrustedIPv6Prefixes:  req.TrustedIPv6Prefixes,
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
	lock := switchOperationLock(sw.ID)
	lock.Lock()
	defer lock.Unlock()
	if err := EnsureVPCSwitchRuntime(*sw); err != nil {
		_ = removeVPCSwitchRuntime(*sw)
		_ = model.DB.Delete(sw).Error
		return nil, err
	}
	if HookTriggerPortSecurityReconcile != nil {
		HookTriggerPortSecurityReconcile()
	}
	return sw, nil
}

func UpdateVPCSwitch(operator, role string, id uint, req VPCSwitchRequest) (*model.VPCSwitch, error) {
	lock := switchOperationLock(id)
	lock.Lock()
	defer lock.Unlock()
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
	if topologyRequestChanged(sw, req) {
		return nil, fmt.Errorf("上行链路、内置 DHCP 或桥接 VLAN 的变化请通过交换机重配置任务提交")
	}
	// 支持管理员修改交换机所属用户
	if role == "admin" {
		newUsername, err := resolveVPCUsername(operator, role, req.Username)
		if err != nil {
			return nil, err
		}
		if newUsername != "" && newUsername != sw.Username {
			sw.Username = newUsername
			// 确保新用户存在默认安全组
			if _, err := EnsureDefaultSecurityGroup(newUsername); err != nil {
				return nil, err
			}
		}
	}
	if req.BridgeName != "" && req.BridgeName != sw.BridgeName {
		return nil, fmt.Errorf("暂不支持修改交换机目标网桥")
	}
	if err := normalizeSwitchPortSecurityFields(&req, HookSwitchUsesDirectBridge(sw)); err != nil {
		return nil, err
	}
	// 禁止修改网段/网关（影响所有已绑定 VM 的网络配置）
	if strings.TrimSpace(req.CIDR) != "" && strings.TrimSpace(req.CIDR) != sw.CIDR {
		return nil, fmt.Errorf("暂不支持修改交换机网段，请删除后重新创建")
	}
	if strings.TrimSpace(req.GatewayIP) != "" && strings.TrimSpace(req.GatewayIP) != sw.GatewayIP {
		return nil, fmt.Errorf("暂不支持修改交换机网关，请删除后重新创建")
	}
	if req.Name = normalizeVPCName(req.Name); req.Name != "" {
		sw.Name = req.Name
	}
	if HookSwitchUsesDirectBridge(sw) {
		sw.AllowPromiscuous = req.AllowPromiscuous
		sw.AllowMACChange = req.AllowMACChange
		sw.AllowForgedTransmits = req.AllowForgedTx
		sw.IPv6SecurityEnabled = req.IPv6SecurityEnabled
		sw.TrustedIPv6Prefixes = req.TrustedIPv6Prefixes
	} else {
		sw.AllowPromiscuous = false
		sw.AllowMACChange = false
		sw.AllowForgedTransmits = false
		sw.IPv6SecurityEnabled = false
		sw.TrustedIPv6Prefixes = ""
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
	if HookTriggerPortSecurityReconcile != nil {
		HookTriggerPortSecurityReconcile()
	}
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
		// 数据库中没有记录，回退到 OVS 系统层检查
		if HookEnsureOVSBridgeExists != nil {
			if err := HookEnsureOVSBridgeExists(requested); err != nil {
				return "", "", fmt.Errorf("桥接网桥不存在")
			}
		}
		// 网桥在 OVS 中存在但数据库无记录，允许继续创建
		return requested, BridgeModeDirect, nil
	}
	return bridge.Name, BridgeModeDirect, nil
}

func resolveNewVPCSwitchBridge(role string, vlanID int, req *VPCSwitchRequest) (string, string, bool, error) {
	if req == nil {
		return "", "", false, fmt.Errorf("交换机拓扑参数为空")
	}
	if req.DHCPEnabled {
		req.BridgeName = HookOvsBridgeName()
		return HookOvsBridgeName(), BridgeModeNAT, false, nil
	}
	requestedBridge := strings.TrimSpace(req.BridgeName)
	// 兼容历史 API：管理员仍可引用已经存在的宿主机桥接网桥。
	if role == "admin" && requestedBridge != "" && requestedBridge != HookOvsBridgeName() && strings.TrimSpace(req.UplinkIF) == "" {
		bridgeName, bridgeMode, err := resolveVPCSwitchBridge(role, requestedBridge)
		if err != nil {
			return "", "", false, err
		}
		if HookGetOVSBridgePhysicalUplink != nil {
			req.UplinkIF = strings.TrimSpace(HookGetOVSBridgePhysicalUplink(bridgeName))
		}
		req.UplinkMode = normalizeUplinkMode(req.UplinkMode, req.UplinkIF, false)
		return bridgeName, bridgeMode, false, nil
	}
	if strings.TrimSpace(req.UplinkIF) != "" {
		shared, err := findSharedDirectSwitch(req.UplinkIF, req.BridgeVLANID, 0)
		if err != nil {
			return "", "", false, err
		}
		if shared != nil {
			req.BridgeName = HookBridgeNameForSwitch(*shared)
			req.UplinkMode = UplinkModePhysical
			return req.BridgeName, BridgeModeDirect, false, nil
		}
	}
	bridgeName := managedVPCBridgeName(vlanID)
	req.BridgeName = bridgeName
	req.UplinkMode = normalizeUplinkMode(req.UplinkMode, req.UplinkIF, false)
	return bridgeName, BridgeModeDirect, true, nil
}

// findSharedDirectSwitch 查找同一物理上行上的直通交换机，并校验共享 VLAN 唯一性。
// VLAN 0 表示不打标签，因此不能和其它逻辑交换机共享同一个物理上行。
func findSharedDirectSwitch(uplink string, bridgeVLANID int, excludeSwitchID uint) (*model.VPCSwitch, error) {
	uplink = strings.TrimSpace(uplink)
	if uplink == "" || model.DB == nil {
		return nil, nil
	}
	query := model.DB.Where("uplink_if = ? AND dhcp_enabled = ?", uplink, false)
	if excludeSwitchID > 0 {
		query = query.Where("id <> ?", excludeSwitchID)
	}
	var switches []model.VPCSwitch
	if err := query.Order("owns_bridge DESC, id ASC").Find(&switches).Error; err != nil {
		return nil, err
	}
	var shared *model.VPCSwitch
	for index := range switches {
		sw := switches[index]
		if !HookSwitchUsesDirectBridge(sw) || strings.TrimSpace(sw.UplinkIF) == "" {
			continue
		}
		if bridgeVLANID == 0 {
			return nil, fmt.Errorf("物理网卡 %s 已有直通交换机；共享物理上行时 VLAN ID 必须为 1-4094", uplink)
		}
		if sw.BridgeVLANID == bridgeVLANID {
			return nil, fmt.Errorf("物理网卡 %s 的桥接 VLAN ID %d 已被交换机「%s」使用", uplink, bridgeVLANID, sw.Name)
		}
		if shared == nil {
			copy := sw
			shared = &copy
			continue
		}
		if !strings.EqualFold(HookBridgeNameForSwitch(*shared), HookBridgeNameForSwitch(sw)) {
			return nil, fmt.Errorf("物理网卡 %s 存在多个不同的直通网桥，请先整理现有拓扑", uplink)
		}
	}
	return shared, nil
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

func GetVPCSwitchVMs(operator, role string, id uint) ([]VMSwitchInfo, error) {
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, id).Error; err != nil {
		return nil, fmt.Errorf("交换机不存在")
	}
	if role != "admin" && sw.Username != operator {
		return nil, fmt.Errorf("无权操作此交换机")
	}
	var bindings []model.VPCVMBinding
	if err := model.DB.Where("switch_id = ?", id).Order("vm_name ASC, interface_order ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	result := make([]VMSwitchInfo, len(bindings))
	for i, b := range bindings {
		result[i] = VMSwitchInfo{
			VMName:         b.VMName,
			Username:       b.Username,
			InterfaceOrder: b.InterfaceOrder,
		}
	}
	return result, nil
}

func DeleteVPCSwitch(operator, role string, id uint, force bool) error {
	vpcTopologyMutationMu.Lock()
	defer vpcTopologyMutationMu.Unlock()
	lock := switchOperationLock(id)
	lock.Lock()
	defer lock.Unlock()
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
	var bindings []model.VPCVMBinding
	model.DB.Where("switch_id = ?", id).Find(&bindings)
	if len(bindings) > 0 {
		if !force {
			return fmt.Errorf("交换机仍有 %d 台虚拟机绑定，不能删除", len(bindings))
		}
		// 强制删除：先移除所有虚拟机的网卡
		for _, binding := range bindings {
			if err := HookDetachVMInterface(binding.VMName, binding.InterfaceOrder); err != nil {
				logger.App.Warn("强制移除虚拟机网卡失败", "vm", binding.VMName, "interface_order", binding.InterfaceOrder, "switch", sw.Name, "error", err)
			} else {
				logger.App.Info("已强制移除虚拟机网卡", "vm", binding.VMName, "interface_order", binding.InterfaceOrder, "switch", sw.Name)
			}
		}
		// 删除所有绑定记录
		if err := model.DB.Where("switch_id = ?", id).Delete(&model.VPCVMBinding{}).Error; err != nil {
			return fmt.Errorf("删除虚拟机绑定记录失败: %w", err)
		}
	}
	_ = ApplyVPCACLRules()
	if err := removeVPCSwitchRuntime(sw); err != nil {
		return fmt.Errorf("清理交换机运行态失败: %w", err)
	}
	if err := model.DB.Delete(&sw).Error; err != nil {
		_ = EnsureVPCSwitchRuntime(sw)
		return err
	}
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

// resolveVPCSwitchSubnet 解析交换机子网：优先使用用户指定的 CIDR/网关，否则自动分配。
// 对于桥接直通模式，不需要 CIDR（由上级路由器分配），直接返回空值。
func resolveVPCSwitchSubnet(dhcpEnabled bool, req VPCSwitchRequest) (cidr, gateway, dhcpStart, dhcpEnd string, err error) {
	return resolveVPCSwitchSubnetExcept(dhcpEnabled, req, 0)
}

func resolveVPCSwitchSubnetExcept(dhcpEnabled bool, req VPCSwitchRequest, excludeSwitchID uint) (cidr, gateway, dhcpStart, dhcpEnd string, err error) {
	if !dhcpEnabled {
		return "", "", "", "", nil
	}
	req.CIDR = strings.TrimSpace(req.CIDR)
	req.GatewayIP = strings.TrimSpace(req.GatewayIP)
	req.DHCPStart = strings.TrimSpace(req.DHCPStart)
	req.DHCPEnd = strings.TrimSpace(req.DHCPEnd)
	// 未指定 CIDR 时自动分配
	if req.CIDR == "" {
		return allocateVPCSubnet()
	}
	// 验证 CIDR 格式
	prefix, err := netip.ParsePrefix(req.CIDR)
	if err != nil {
		return "", "", "", "", fmt.Errorf("网段格式无效: %s（正确格式如 10.0.1.0/24）", req.CIDR)
	}
	// 必须是 IPv4
	if !prefix.Addr().Is4() {
		return "", "", "", "", fmt.Errorf("仅支持 IPv4 网段")
	}
	// 拒绝过大的子网（/16 或更大），防止 DHCP 范围过大
	if prefix.Bits() < 16 {
		return "", "", "", "", fmt.Errorf("子网掩码位不能小于 16，/16 以上子网过大")
	}
	// 拒绝过小的子网（/30 或更小），至少要能容纳网关+DHCP
	if prefix.Bits() > 29 {
		return "", "", "", "", fmt.Errorf("子网掩码位不能大于 29，至少需要 6 个可用 IP")
	}
	// 验证网关 IP
	if req.GatewayIP == "" {
		// 自动选择 CIDR 内第一个 IP 作为网关
		addr := prefix.Addr().Next()
		if !prefix.Contains(addr) {
			return "", "", "", "", fmt.Errorf("无法自动计算网关地址")
		}
		req.GatewayIP = addr.String()
	}
	gatewayAddr, err := netip.ParseAddr(req.GatewayIP)
	if err != nil {
		return "", "", "", "", fmt.Errorf("网关地址格式无效: %s", req.GatewayIP)
	}
	if !prefix.Contains(gatewayAddr) {
		return "", "", "", "", fmt.Errorf("网关地址 %s 不在网段 %s 内", req.GatewayIP, req.CIDR)
	}
	// 网关不能是网络地址或广播地址
	if gatewayAddr == prefix.Addr() || gatewayAddr == broadcastAddr(prefix) {
		return "", "", "", "", fmt.Errorf("网关地址不能是网络地址或广播地址")
	}
	// 检查 CIDR 是否已被使用
	var existing model.VPCSwitch
	existingQuery := model.DB.Where("cidr = ?", req.CIDR)
	if excludeSwitchID > 0 {
		existingQuery = existingQuery.Where("id <> ?", excludeSwitchID)
	}
	if err := existingQuery.First(&existing).Error; err == nil {
		return "", "", "", "", fmt.Errorf("网段 %s 已被交换机「%s」使用", req.CIDR, existing.Name)
	}
	// 检查是否与宿主机网段冲突
	hostCIDR := getHostNetworkCIDR()
	if hostCIDR != "" {
		hostPrefix, hostErr := netip.ParsePrefix(hostCIDR)
		if hostErr == nil && (prefix.Contains(hostPrefix.Addr()) || hostPrefix.Contains(prefix.Addr())) {
			return "", "", "", "", fmt.Errorf("网段 %s 与宿主机网段 %s 冲突，请使用其他网段", req.CIDR, hostCIDR)
		}
	}
	// DHCP 范围：优先使用用户指定，否则自动计算（.10 ~ .250 或根据子网大小调整）
	if req.DHCPStart == "" {
		req.DHCPStart = defaultDHCPStart(prefix, gatewayAddr)
	}
	if req.DHCPEnd == "" {
		req.DHCPEnd = defaultDHCPEnd(prefix)
	}
	dhcpStartAddr, err := netip.ParseAddr(req.DHCPStart)
	if err != nil {
		return "", "", "", "", fmt.Errorf("DHCP 起始地址格式无效: %s", req.DHCPStart)
	}
	dhcpEndAddr, err := netip.ParseAddr(req.DHCPEnd)
	if err != nil {
		return "", "", "", "", fmt.Errorf("DHCP 结束地址格式无效: %s", req.DHCPEnd)
	}
	if !prefix.Contains(dhcpStartAddr) || !prefix.Contains(dhcpEndAddr) {
		return "", "", "", "", fmt.Errorf("DHCP 地址范围必须在网段 %s 内", req.CIDR)
	}
	if dhcpStartAddr == prefix.Addr() || dhcpEndAddr == prefix.Addr() {
		return "", "", "", "", fmt.Errorf("DHCP 地址不能是网络地址")
	}
	if dhcpStartAddr == broadcastAddr(prefix) || dhcpEndAddr == broadcastAddr(prefix) {
		return "", "", "", "", fmt.Errorf("DHCP 地址不能是广播地址")
	}
	if cmpAddr(dhcpStartAddr, dhcpEndAddr) > 0 {
		return "", "", "", "", fmt.Errorf("DHCP 起始地址不能大于结束地址")
	}
	return req.CIDR, req.GatewayIP, req.DHCPStart, req.DHCPEnd, nil
}

// broadcastAddr 计算 IPv4 子网的广播地址
func broadcastAddr(prefix netip.Prefix) netip.Addr {
	addr := prefix.Addr().As4()
	bits := prefix.Bits()
	mask := net.CIDRMask(bits, 32)
	for i := 0; i < 4; i++ {
		addr[i] |= ^mask[i]
	}
	return netip.AddrFrom4(addr)
}

// cmpAddr 比较两个地址（-1: a<b, 0: a==b, 1: a>b）
func cmpAddr(a, b netip.Addr) int {
	a4 := a.As4()
	b4 := b.As4()
	for i := 0; i < 4; i++ {
		if a4[i] < b4[i] {
			return -1
		}
		if a4[i] > b4[i] {
			return 1
		}
	}
	return 0
}

// defaultDHCPStart 计算默认 DHCP 起始地址（网关+1，至少离网关 1 个 IP）
func defaultDHCPStart(prefix netip.Prefix, gateway netip.Addr) string {
	start := gateway.Next()
	if !prefix.Contains(start) || start == broadcastAddr(prefix) {
		return gateway.String()
	}
	// 跳过网关自身，从下一个可用 IP 开始
	for prefix.Contains(start) && start != broadcastAddr(prefix) {
		if start != gateway {
			return start.String()
		}
		start = start.Next()
	}
	return gateway.String()
}

// defaultDHCPEnd 计算默认 DHCP 结束地址（广播地址前一个）
func defaultDHCPEnd(prefix netip.Prefix) string {
	bcast := broadcastAddr(prefix)
	addr := bcast.Prev()
	if prefix.Contains(addr) && addr != prefix.Addr() {
		return addr.String()
	}
	return bcast.String()
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

// getHostNetworkCIDR 获取宿主机主网卡的网段 CIDR（如 "192.168.1.0/24"）
func getHostNetworkCIDR() string {
	hostNIC := config.GlobalConfig.ExternalNIC
	if hostNIC == "" {
		// 自动检测默认路由出口网卡
		result := utils.ExecCommand("bash", "-c", `ip -4 route show default 2>/dev/null | awk '{print $5; exit}'`)
		hostNIC = strings.TrimSpace(result.Stdout)
	}
	if hostNIC == "" {
		return ""
	}
	result := utils.ExecCommand("ip", "-4", "-o", "addr", "show", "dev", hostNIC, "scope", "global")
	if result.Error != nil || result.Stdout == "" {
		return ""
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) < 4 {
		return ""
	}
	cidr := strings.TrimSpace(fields[3])
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return ""
	}
	return prefix.Masked().String()
}
