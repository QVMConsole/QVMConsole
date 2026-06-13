package vpc

import (
	"fmt"
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
		var bridge model.NetworkBridge
		if err := model.DB.Where("name = ?", HookBridgeNameForSwitch(sw)).First(&bridge).Error; err != nil {
			return fmt.Errorf("桥接网桥 %s 不存在", HookBridgeNameForSwitch(sw))
		}
		if err := HookEnsureOVSBridgeDirect(bridge.Name, bridge.UplinkIF, bridge.MigrateHostIP, bridge.HostAddrs, bridge.HostGateway, bridge.HostMetric); err != nil {
			return err
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
	utils.ExecShellQuiet(fmt.Sprintf("ip -4 addr show dev %s | grep -q '%s/24' || ip addr add %s/24 dev %s",
		utils.ShellSingleQuote(port), utils.ShellSingleQuote(sw.GatewayIP), utils.ShellSingleQuote(sw.GatewayIP), utils.ShellSingleQuote(port)))
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
		if sw.VLANID > 0 {
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
	uplink := HookOvsUplink()
	if uplink == "" {
		return fmt.Errorf("无法检测 VPC NAT 出口网卡，请配置 KVM_OVS_UPLINK")
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
	uplink := HookOvsUplink()
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

func removeVPCSwitchRuntime(sw model.VPCSwitch) error {
	clearVPCSwitchBandwidth(sw)
	if HookSwitchUsesDirectBridge(sw) {
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
	content := fmt.Sprintf(`interface=%s
bind-interfaces
except-interface=lo
dhcp-authoritative
dhcp-range=%s,%s,255.255.255.0,12h
dhcp-option=option:router,%s
dhcp-option=option:dns-server,%s
dhcp-hostsfile=%s
pid-file=%s
dhcp-leasefile=%s
log-dhcp
`, iface, sw.DHCPStart, sw.DHCPEnd, sw.GatewayIP, config.GlobalConfig.VPCDNS, VPCDHCPHostsPath(sw.ID), vpcDNSMasqPIDPath(sw.ID), vpcDHCPLeasesPath(sw.ID))
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
	result := utils.ExecShell(fmt.Sprintf("[ -f %s ] && kill -HUP $(cat %s)", utils.ShellSingleQuote(pidPath), utils.ShellSingleQuote(pidPath)))
	if result.Error == nil {
		return
	}
	if _, err := os.Stat(vpcDNSMasqConfigPath(id)); err == nil {
		startVPCDNSMasq(id)
	}
}

func stopVPCDNSMasq(id uint) {
	pidPath := vpcDNSMasqPIDPath(id)
	utils.ExecShell(fmt.Sprintf("[ -f %s ] && kill $(cat %s) 2>/dev/null || true", utils.ShellSingleQuote(pidPath), utils.ShellSingleQuote(pidPath)))
	_ = os.Remove(pidPath)
}

func cleanupStaleManagedNATRules(cidr, gatewayPort, uplink string) {
	// Best-effort cleanup of any stale NAT rules that might conflict.
	// This is a no-op placeholder — the original logic relies on ensureIPTablesRule
	// idempotency; explicit stale cleanup is handled by the OVS network layer.
}
