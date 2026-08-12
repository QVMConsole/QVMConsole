package vpc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/utils"
)

func EnsureVPCSwitchRuntime(sw model.VPCSwitch) error {
	if strings.TrimSpace(sw.BridgeName) == "" {
		sw.BridgeName = HookOvsBridgeName()
	}
	if strings.TrimSpace(sw.BridgeMode) == "" {
		sw.BridgeMode = BridgeModeNAT
	}
	if HookSwitchUsesDirectBridge(sw) {
		bridgeName := HookBridgeNameForSwitch(sw)
		if sw.OwnsBridge {
			if err := HookEnsureOVSBridgeDirect(bridgeName, sw.UplinkIF, sw.MigrateHostIP, sw.HostAddrs, sw.HostGateway, sw.HostMetric, sw.HostDNS); err != nil {
				return err
			}
			return ApplyVPCSwitchBandwidth(sw)
		}
		var bridge model.NetworkBridge
		if err := model.DB.Where("name = ?", bridgeName).First(&bridge).Error; err != nil {
			// 数据库中没有记录，回退到 OVS 系统层检查
			if HookEnsureOVSBridgeExists != nil {
				if err := HookEnsureOVSBridgeExists(bridgeName); err != nil {
					return fmt.Errorf("桥接网桥 %s 不存在", bridgeName)
				}
			}
			// 网桥存在但数据库无记录，从系统层发现上联信息
			uplink := ""
			if HookGetOVSBridgePhysicalUplink != nil {
				uplink = HookGetOVSBridgePhysicalUplink(bridgeName)
			}
			if err := HookEnsureOVSBridgeDirect(bridgeName, uplink, false, "", "", "", ""); err != nil {
				return err
			}
		} else {
			if err := HookEnsureOVSBridgeDirect(bridge.Name, bridge.UplinkIF, bridge.MigrateHostIP, bridge.HostAddrs, bridge.HostGateway, bridge.HostMetric, bridge.HostDNS); err != nil {
				return err
			}
		}
		return ApplyVPCSwitchBandwidth(sw)
	}
	if err := HookEnsureOVSNetwork(); err != nil {
		return err
	}
	// 系统基础网络交换机（VLANID == 0）直接使用 br-ovs，不需要独立的网关端口和 dnsmasq
	if sw.VLANID == 0 {
		return ApplyVPCSwitchBandwidth(sw)
	}
	if err := os.MkdirAll(VPCConfigDir, 0755); err != nil {
		return fmt.Errorf("创建 VPC 配置目录失败: %w", err)
	}
	bridge := HookOvsBridgeName()
	port := VPCGatewayPortName(sw.ID)
	if result := utils.ExecCommand("ovs-vsctl", "--may-exist", "add-port", bridge, port, "tag="+strconv.Itoa(sw.VLANID), "--", "set", "Interface", port, "type=internal"); result.Error != nil {
		return fmt.Errorf("创建 VPC 网关端口失败: %s", result.Stderr)
	}
	// 修正已存在端口 VLAN tag 与数据库不一致的问题（例如数据库重建后 VLAN ID 变更，但 --may-exist 不会更新已存在端口的 tag）
	expectedTag := strconv.Itoa(sw.VLANID)
	if currentTag, ok := getOVSPortTag(port); ok && currentTag != expectedTag {
		logger.App.Warn("VPC 网关端口 VLAN tag 不一致，正在修正", "port", port, "current", currentTag, "expected", expectedTag)
		if result := utils.ExecCommand("ovs-vsctl", "set", "Port", port, "tag="+expectedTag); result.Error != nil {
			return fmt.Errorf("修正 VPC 网关端口 VLAN tag 失败: %s", result.Stderr)
		}
	}
	utils.ExecCommand("ip", "link", "set", port, "up")
	gatewayCIDR, err := switchGatewayCIDR(sw)
	if err != nil {
		return err
	}
	utils.ExecShellQuiet(fmt.Sprintf("ip -4 addr show dev %s | grep -q %s || ip addr add %s dev %s",
		utils.ShellSingleQuote(port), utils.ShellSingleQuote(gatewayCIDR), utils.ShellSingleQuote(gatewayCIDR), utils.ShellSingleQuote(port)))
	if err := HookEnsureLocalDNSMasqInput(port); err != nil {
		return err
	}
	if err := ensureVPCSwitchNAT(sw, port); err != nil {
		return err
	}
	if _, err := os.Stat(VPCDHCPHostsPath(sw.ID)); os.IsNotExist(err) {
		if err := os.WriteFile(VPCDHCPHostsPath(sw.ID), []byte(""), 0644); err != nil {
			return fmt.Errorf("创建 VPC 静态 DHCP 绑定文件失败: %w", err)
		}
	}
	configChanged, err := writeVPCDNSMasqConfig(sw, port)
	if err != nil {
		return err
	}
	ensureVPCDNSMasq(sw.ID, configChanged)
	if err := ApplyVPCSwitchBandwidth(sw); err != nil {
		return err
	}
	return nil
}

func EnsureAllVPCSwitchRuntime() error {
	if model.DB == nil {
		return nil
	}
	var switches []model.VPCSwitch
	if err := model.DB.Order("id ASC").Find(&switches).Error; err != nil {
		return err
	}
	var lastErr error
	// 清理数据库已删除但 OVS 上残留的孤儿 VPC 网关端口（例如数据库重建后）
	if err := cleanOrphanVPCSwitchPorts(switches); err != nil {
		lastErr = err
		logger.App.Warn("清理孤儿 VPC 端口失败", "error", err)
	}
	for _, sw := range switches {
		if err := EnsureVPCSwitchRuntime(sw); err != nil {
			lastErr = err
			logger.App.Warn("恢复 VPC 交换机运行态失败", "switch", sw.Name, "id", sw.ID, "error", err)
		}
	}
	if err := applyAllVPCBindingsRuntime(false); err != nil {
		lastErr = err
		logger.App.Warn("恢复 VPC VM 绑定运行态失败", "error", err)
	}
	if len(switches) > 0 {
		if err := ApplyVPCACLRules(); err != nil {
			lastErr = err
			logger.App.Warn("恢复 VPC ACL 失败", "error", err)
		}
	}
	return lastErr
}

// cleanOrphanVPCSwitchPorts 清理 OVS 上数据库中已不存在的孤儿 VPC 网关端口及关联的 dnsmasq 进程
// 例如数据库被删除重建后，旧交换机 ID 的端口仍残留在 OVS 上
func cleanOrphanVPCSwitchPorts(switches []model.VPCSwitch) error {
	bridge := HookOvsBridgeName()
	output := utils.ExecCommand("ovs-vsctl", "list-ports", bridge)
	if output.Error != nil {
		return fmt.Errorf("列出 OVS 端口失败: %s", output.Stderr)
	}
	validIDs := map[uint]bool{}
	for _, sw := range switches {
		// 仅 VLAN>0 的交换机才有独立网关端口
		if sw.VLANID > 0 && SwitchUsesManagedDHCP(sw) {
			validIDs[sw.ID] = true
		}
	}
	for _, port := range strings.Fields(output.Stdout) {
		if !strings.HasPrefix(port, "vpcsw") {
			continue
		}
		idStr := strings.TrimPrefix(port, "vpcsw")
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			continue
		}
		if validIDs[uint(id)] {
			continue
		}
		logger.App.Warn("清理孤儿 VPC 网关端口", "port", port, "id", id)
		// 停止关联的 dnsmasq
		stopVPCDNSMasq(uint(id))
		// 删除 OVS 端口
		if result := utils.ExecCommand("ovs-vsctl", "--if-exists", "del-port", bridge, port); result.Error != nil {
			logger.App.Warn("删除孤儿 VPC 端口失败", "port", port, "error", result.Stderr)
		}
		// 删除关联配置文件
		_ = os.Remove(vpcDNSMasqConfigPath(uint(id)))
		_ = os.Remove(VPCDHCPHostsPath(uint(id)))
		_ = os.Remove(vpcDHCPLeasesPath(uint(id)))
		// 尽力清理 iptables NAT 规则（端口名已知，用端口名匹配）
		uplink := HookOvsUplink()
		if uplink != "" && port != "" {
			utils.ExecShellQuiet(fmt.Sprintf("while iptables -t nat -D POSTROUTING -o %s -j MASQUERADE 2>/dev/null; do :; done", utils.ShellSingleQuote(uplink)))
			utils.ExecShellQuiet(fmt.Sprintf("while iptables -D FORWARD -i %s -o %s -j ACCEPT 2>/dev/null; do :; done", utils.ShellSingleQuote(port), utils.ShellSingleQuote(uplink)))
			utils.ExecShellQuiet(fmt.Sprintf("while iptables -D FORWARD -i %s -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; do :; done", utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(port)))
		}
	}
	return nil
}

func ensureVPCSwitchNAT(sw model.VPCSwitch, gatewayPort string) error {
	uplink := effectiveUplinkForSwitch(sw)
	if uplink == "" {
		return fmt.Errorf("无法检测 VPC NAT 出口网卡，请配置 KVM_OVS_UPLINK")
	}
	if sw.UplinkMode == UplinkModePhysical {
		if err := ensureVPCPolicyRoute(sw, uplink); err != nil {
			return err
		}
	}
	cleanupStaleManagedNATRules(sw.CIDR, gatewayPort, uplink)
	if err := HookEnsureIPTablesRule(
		fmt.Sprintf("iptables -t nat -C POSTROUTING -s %s -o %s -j MASQUERADE", utils.ShellSingleQuote(sw.CIDR), utils.ShellSingleQuote(uplink)),
		fmt.Sprintf("iptables -t nat -A POSTROUTING -s %s -o %s -j MASQUERADE", utils.ShellSingleQuote(sw.CIDR), utils.ShellSingleQuote(uplink)),
		"配置 VPC 出站 NAT",
	); err != nil {
		return err
	}
	if err := HookEnsureIPTablesRule(
		fmt.Sprintf("iptables -C FORWARD -i %s -o %s -j ACCEPT", utils.ShellSingleQuote(gatewayPort), utils.ShellSingleQuote(uplink)),
		fmt.Sprintf("iptables -A FORWARD -i %s -o %s -j ACCEPT", utils.ShellSingleQuote(gatewayPort), utils.ShellSingleQuote(uplink)),
		"配置 VPC 出站转发",
	); err != nil {
		return err
	}
	if err := HookEnsureIPTablesRule(
		fmt.Sprintf("iptables -C FORWARD -i %s -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(gatewayPort)),
		fmt.Sprintf("iptables -A FORWARD -i %s -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(gatewayPort)),
		"配置 VPC 回程转发",
	); err != nil {
		return err
	}
	return nil
}

func removeVPCSwitchNAT(sw model.VPCSwitch) {
	port := VPCGatewayPortName(sw.ID)
	HookRemoveLocalDNSMasqInput(port)
	removeVPCPolicyRoute(sw)
	removeVPCSwitchNATRules(sw)
}

// removeVPCSwitchNATRules 仅删除指定交换机配置对应的 NAT/FORWARD 规则。
// 托管网络修改网段或出口时，目标 dnsmasq 与策略路由仍在使用同一个网关端口，
// 因此不能直接调用完整的 removeVPCSwitchNAT。
func removeVPCSwitchNATRules(sw model.VPCSwitch) {
	port := VPCGatewayPortName(sw.ID)
	uplink := effectiveUplinkForSwitch(sw)
	if uplink == "" {
		return
	}
	utils.ExecShell(fmt.Sprintf("while iptables -t nat -D POSTROUTING -s %s -o %s -j MASQUERADE 2>/dev/null; do :; done",
		utils.ShellSingleQuote(sw.CIDR), utils.ShellSingleQuote(uplink)))
	utils.ExecShell(fmt.Sprintf("while iptables -D FORWARD -i %s -o %s -j ACCEPT 2>/dev/null; do :; done",
		utils.ShellSingleQuote(port), utils.ShellSingleQuote(uplink)))
	utils.ExecShell(fmt.Sprintf("while iptables -D FORWARD -i %s -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; do :; done",
		utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(port)))
}

// switchManagedRuntimeChanged 判断两个托管运行态是否需要清除旧地址与出口规则。
func switchManagedRuntimeChanged(stale, active model.VPCSwitch) bool {
	return strings.TrimSpace(stale.CIDR) != strings.TrimSpace(active.CIDR) ||
		strings.TrimSpace(stale.GatewayIP) != strings.TrimSpace(active.GatewayIP) ||
		strings.TrimSpace(effectiveUplinkForSwitch(stale)) != strings.TrimSpace(effectiveUplinkForSwitch(active))
}

// cleanupSupersededManagedRuntime 清除同一网关端口上的旧托管配置，再恢复活动配置。
// 该函数同时用于提交后的旧配置清理和失败回滚时的目标配置清理。
func cleanupSupersededManagedRuntime(stale, active model.VPCSwitch) error {
	if switchManagedRuntimeChanged(stale, active) {
		removeVPCSwitchNATRules(stale)
		if staleGatewayCIDR, staleErr := switchGatewayCIDR(stale); staleErr == nil {
			if activeGatewayCIDR, activeErr := switchGatewayCIDR(active); activeErr != nil || staleGatewayCIDR != activeGatewayCIDR {
				utils.ExecCommandQuiet("ip", "addr", "del", staleGatewayCIDR, "dev", VPCGatewayPortName(stale.ID))
			}
		}
	}
	return EnsureVPCSwitchRuntime(active)
}

func removeVPCSwitchRuntime(sw model.VPCSwitch) error {
	clearVPCSwitchBandwidth(sw)
	if HookSwitchUsesDirectBridge(sw) {
		if sw.OwnsBridge && HookDeleteOwnedSwitchBridge != nil {
			return HookDeleteOwnedSwitchBridge(HookBridgeNameForSwitch(sw), sw.UplinkIF, sw.MigrateHostIP, sw.HostDNS)
		}
		return nil
	}
	// 系统基础网络交换机（VLANID == 0）不需要清理独立端口和 dnsmasq
	if sw.VLANID == 0 {
		return nil
	}
	removeVPCSwitchNAT(sw)
	stopVPCDNSMasq(sw.ID)
	utils.ExecCommand("ovs-vsctl", "--if-exists", "del-port", HookOvsBridgeName(), VPCGatewayPortName(sw.ID))
	_ = os.Remove(vpcDNSMasqConfigPath(sw.ID))
	_ = os.Remove(VPCDHCPHostsPath(sw.ID))
	_ = os.Remove(vpcDHCPLeasesPath(sw.ID))
	return nil
}

func VPCGatewayPortName(id uint) string {
	return fmt.Sprintf("vpcsw%d", id)
}

func vpcDNSMasqConfigPath(id uint) string {
	return filepath.Join(VPCConfigDir, fmt.Sprintf("dnsmasq-%d.conf", id))
}

func vpcDNSMasqPIDPath(id uint) string {
	return filepath.Join(VPCConfigDir, fmt.Sprintf("dnsmasq-%d.pid", id))
}

func VPCDHCPHostsPath(id uint) string {
	return filepath.Join(VPCConfigDir, fmt.Sprintf("dhcp-hosts-%d", id))
}

func vpcDHCPLeasesPath(id uint) string {
	return filepath.Join(VPCConfigDir, fmt.Sprintf("leases-%d", id))
}

func writeVPCDNSMasqConfig(sw model.VPCSwitch, iface string) (bool, error) {
	mask, err := switchNetmask(sw)
	if err != nil {
		return false, err
	}
	content := fmt.Sprintf(`interface=%s
bind-interfaces
except-interface=lo
dhcp-authoritative
dhcp-range=%s,%s,%s,12h
dhcp-option=option:router,%s
dhcp-option=option:dns-server,%s
dhcp-hostsfile=%s
pid-file=%s
dhcp-leasefile=%s
log-dhcp
`, iface, sw.DHCPStart, sw.DHCPEnd, mask, sw.GatewayIP, config.GlobalConfig.VPCDNS, VPCDHCPHostsPath(sw.ID), vpcDNSMasqPIDPath(sw.ID), vpcDHCPLeasesPath(sw.ID))
	changed, err := HookWriteFileIfChanged(vpcDNSMasqConfigPath(sw.ID), []byte(content), 0644)
	if err != nil {
		return false, fmt.Errorf("写入 VPC DHCP 配置失败: %w", err)
	}
	return changed, nil
}

func startVPCDNSMasq(id uint) {
	stopVPCDNSMasq(id)
	configPath := vpcDNSMasqConfigPath(id)
	if _, err := os.Stat(configPath); err != nil {
		logger.App.Warn("dnsmasq 配置文件不存在", "path", configPath)
		return
	}
	result := utils.ExecCommand("dnsmasq", "--conf-file="+configPath)
	if result.Error != nil {
		logger.App.Warn("启动 dnsmasq 失败", "stderr", result.Stderr)
	}
}

func ensureVPCDNSMasq(id uint, configChanged bool) {
	if configChanged {
		startVPCDNSMasq(id)
		return
	}
	if !isVPCDNSMasqRunning(id) {
		startVPCDNSMasq(id)
	}
}

func isVPCDNSMasqRunning(id uint) bool {
	pidPath := vpcDNSMasqPIDPath(id)
	result := utils.ExecShellQuiet(fmt.Sprintf("[ -f %s ] && ps -p $(cat %s) -o comm= | grep -q '^dnsmasq$'",
		utils.ShellSingleQuote(pidPath), utils.ShellSingleQuote(pidPath)))
	return result.Error == nil
}

func ReloadVPCDNSMasq(id uint) {
	pidPath := vpcDNSMasqPIDPath(id)
	result := utils.ExecShellQuiet(fmt.Sprintf("[ -f %s ] && kill -HUP $(cat %s)", utils.ShellSingleQuote(pidPath), utils.ShellSingleQuote(pidPath)))
	if result.Error == nil {
		return
	}
	if _, err := os.Stat(vpcDNSMasqConfigPath(id)); err == nil {
		startVPCDNSMasq(id)
	}
}

func stopVPCDNSMasq(id uint) {
	pidPath := vpcDNSMasqPIDPath(id)
	utils.ExecShellQuiet(fmt.Sprintf("[ -f %s ] && kill $(cat %s) 2>/dev/null || true", utils.ShellSingleQuote(pidPath), utils.ShellSingleQuote(pidPath)))
	_ = os.Remove(pidPath)
}

func cleanupStaleManagedNATRules(cidr, gatewayPort, uplink string) {
	if HookCleanupStaleNATRules != nil {
		HookCleanupStaleNATRules(cidr, gatewayPort, uplink)
	}
}

func effectiveUplinkForSwitch(sw model.VPCSwitch) string {
	if sw.UplinkMode == UplinkModePhysical && strings.TrimSpace(sw.UplinkIF) != "" {
		if HookEffectiveL3Interface != nil {
			return strings.TrimSpace(HookEffectiveL3Interface(sw.UplinkIF))
		}
		return strings.TrimSpace(sw.UplinkIF)
	}
	if HookOvsUplink != nil {
		return strings.TrimSpace(HookOvsUplink())
	}
	return ""
}

func switchGatewayCIDR(sw model.VPCSwitch) (string, error) {
	_, network, err := net.ParseCIDR(strings.TrimSpace(sw.CIDR))
	if err != nil || network == nil {
		return "", fmt.Errorf("交换机 %s 的网段配置无效", sw.Name)
	}
	ones, _ := network.Mask.Size()
	return fmt.Sprintf("%s/%d", strings.TrimSpace(sw.GatewayIP), ones), nil
}

func switchNetmask(sw model.VPCSwitch) (string, error) {
	_, network, err := net.ParseCIDR(strings.TrimSpace(sw.CIDR))
	if err != nil || network == nil {
		return "", fmt.Errorf("交换机 %s 的网段配置无效", sw.Name)
	}
	return net.IP(network.Mask).String(), nil
}

const vpcPolicyRouteBase = 100000

func vpcPolicyRouteID(id uint) uint64 {
	return uint64(vpcPolicyRouteBase) + uint64(id)
}

func ensureVPCPolicyRoute(sw model.VPCSwitch, uplink string) error {
	if HookCaptureHostIPConfig == nil {
		return fmt.Errorf("策略路由接口信息读取器尚未初始化")
	}
	gateway := strings.TrimSpace(sw.UplinkGateway)
	if gateway == "" {
		_, gateway, _, _ = HookCaptureHostIPConfig(uplink)
		gateway = strings.TrimSpace(gateway)
	}
	if gateway == "" {
		return fmt.Errorf("上行接口 %s 未检测到 IPv4 默认网关，请填写上行网关", uplink)
	}
	tableID := strconv.FormatUint(vpcPolicyRouteID(sw.ID), 10)
	priority := tableID
	utils.ExecCommandQuiet("ip", "rule", "del", "priority", priority)
	if result := utils.ExecCommand("ip", "route", "replace", "table", tableID, "default", "via", gateway, "dev", uplink, "onlink"); result.Error != nil {
		return fmt.Errorf("配置交换机策略路由失败: %s", result.Stderr)
	}
	if result := utils.ExecCommand("ip", "rule", "add", "priority", priority, "from", strings.TrimSpace(sw.CIDR), "lookup", tableID); result.Error != nil {
		utils.ExecCommandQuiet("ip", "route", "flush", "table", tableID)
		return fmt.Errorf("配置交换机源地址路由规则失败: %s", result.Stderr)
	}
	return nil
}

func removeVPCPolicyRoute(sw model.VPCSwitch) {
	if sw.ID == 0 {
		return
	}
	tableID := strconv.FormatUint(vpcPolicyRouteID(sw.ID), 10)
	utils.ExecCommandQuiet("ip", "rule", "del", "priority", tableID)
	utils.ExecCommandQuiet("ip", "route", "flush", "table", tableID)
}
