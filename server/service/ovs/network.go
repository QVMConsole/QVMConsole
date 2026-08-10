package ovs

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	vpcpkg "kvm_console/service/network/vpc"
	"kvm_console/utils"
)

const (
	OVSConfigDir     = "/etc/kvm-console/ovs"
	OVSStateDir      = "/var/lib/kvm-console/ovs"
	OVSDNSMasqUnit   = "kvm-console-ovs-dnsmasq.service"
	OVSDHCPHostsFile = "/etc/kvm-console/ovs/dhcp-hosts"
	OVSDNSMasqConf   = "/etc/kvm-console/ovs/dnsmasq.conf"
	OVSBridgePrep    = "/etc/kvm-console/ovs/prepare-bridge.sh"
	OVSLeasesFile    = "/var/lib/kvm-console/ovs/dnsmasq.leases"
)

// DetectOpenvswitchServiceName detects the correct systemd service name for Open vSwitch.
// Different distributions use different names: "openvswitch-switch" (Ubuntu/Debian) or "openvswitch" (openEuler/CentOS).
func DetectOpenvswitchServiceName() string {
	// Try openvswitch-switch first (Ubuntu/Debian)
	result := utils.ExecCommand("systemctl", "list-unit-files", "openvswitch-switch.service")
	if result.Error == nil && strings.Contains(result.Stdout, "openvswitch-switch.service") {
		return "openvswitch-switch"
	}
	// Try openvswitch (openEuler/CentOS)
	result = utils.ExecCommand("systemctl", "list-unit-files", "openvswitch.service")
	if result.Error == nil && strings.Contains(result.Stdout, "openvswitch.service") {
		return "openvswitch"
	}
	// Default to openvswitch-switch
	return "openvswitch-switch"
}

// NetworkBackend returns the configured network backend, defaulting to "ovs".
func NetworkBackend() string {
	if config.GlobalConfig == nil || strings.TrimSpace(config.GlobalConfig.NetworkBackend) == "" {
		return "ovs"
	}
	return strings.ToLower(strings.TrimSpace(config.GlobalConfig.NetworkBackend))
}

// UseOVSNetwork returns true if OVS is the selected network backend.
func UseOVSNetwork() bool {
	return NetworkBackend() == "ovs"
}

// OvsBridgeName returns the OVS bridge name from config or the default.
func OvsBridgeName() string {
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.OVSBridge) != "" {
		return strings.TrimSpace(config.GlobalConfig.OVSBridge)
	}
	return "br-ovs"
}

// OvsSubnetPrefix returns the OVS subnet prefix (e.g. "192.168.122").
func OvsSubnetPrefix() string {
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.SubnetPrefix) != "" {
		return strings.TrimSpace(config.GlobalConfig.SubnetPrefix)
	}
	return "192.168.122"
}

// OvsSubnetCIDR returns the OVS subnet in CIDR notation.
func OvsSubnetCIDR() string {
	return OvsSubnetPrefix() + ".0/24"
}

// OvsGatewayIP returns the gateway IP for the OVS network.
func OvsGatewayIP() string {
	return OvsSubnetPrefix() + ".1"
}

// OvsDHCPStart returns the DHCP start address.
func OvsDHCPStart() string {
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.OVSDHCPStart) != "" {
		return strings.TrimSpace(config.GlobalConfig.OVSDHCPStart)
	}
	return OvsSubnetPrefix() + ".2"
}

// OvsDHCPEnd returns the DHCP end address.
func OvsDHCPEnd() string {
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.OVSDHCPEnd) != "" {
		return strings.TrimSpace(config.GlobalConfig.OVSDHCPEnd)
	}
	return OvsSubnetPrefix() + ".254"
}

// OvsUplink 返回 OVS NAT 实际使用的三层出口接口。
func OvsUplink() string {
	configured := ""
	if config.GlobalConfig != nil {
		configured = strings.TrimSpace(config.GlobalConfig.OVSUplink)
	}
	if configured == "" {
		return detectDefaultRouteInterface()
	}

	// 宿主机地址迁移到 OVS 网桥后，配置中可能仍保存原物理口。
	// 此时 Netfilter 看到的出口是三层网桥，NAT/FORWARD 规则必须匹配网桥。
	bridgeResult := utils.ExecCommandQuiet("ovs-vsctl", "--timeout=5", "port-to-br", configured)
	bridge := strings.TrimSpace(bridgeResult.Stdout)
	if bridgeResult.Error == nil && bridge != "" && bridge != configured && interfaceHasDefaultRoute(bridge) {
		return bridge
	}
	return configured
}

func detectDefaultRouteInterface() string {
	if result := utils.ExecCommandQuiet("ip", "-4", "route", "get", "1.1.1.1"); result.Error == nil {
		if iface := routeOutputInterface(result.Stdout); iface != "" {
			return iface
		}
	}
	result := utils.ExecCommandQuiet("ip", "-4", "route", "show", "default")
	return routeOutputInterface(result.Stdout)
}

func interfaceHasDefaultRoute(iface string) bool {
	result := utils.ExecCommandQuiet("ip", "-4", "route", "show", "default", "dev", iface)
	return result.Error == nil && strings.TrimSpace(result.Stdout) != ""
}

func routeOutputInterface(output string) string {
	fields := strings.Fields(output)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "dev" {
			return fields[i+1]
		}
	}
	return ""
}

// BuildOVSVirtInstallNetworkArg builds the --network argument for virt-install using the default bridge.
func BuildOVSVirtInstallNetworkArg(model string) string {
	return HookBuildOVSVirtInstallNetworkArgForBridge(model, OvsBridgeName())
}

// BuildOVSInterfaceXML builds an OVS <interface> XML snippet for the default bridge.
func BuildOVSInterfaceXML(mac, model string) string {
	return HookBuildOVSInterfaceXMLForBridge(mac, model, OvsBridgeName())
}

// BuildOVSInterfaceXMLWithVLAN builds an OVS <interface> XML snippet with an optional VLAN tag.
func BuildOVSInterfaceXMLWithVLAN(mac, model string, vlanID int) string {
	xmlText := BuildOVSInterfaceXML(mac, model)
	if vlanID <= 0 {
		return xmlText
	}
	updated, changed := vpcpkg.SetFirstOVSInterfaceVLANTag(xmlText, vlanID)
	if !changed {
		return xmlText
	}
	return updated
}

// EnsureOVSNetworkReady verifies and prepares the OVS network environment.
func EnsureOVSNetworkReady() error {
	if !UseOVSNetwork() {
		return nil
	}
	if result := utils.ExecCommand("bash", "-c", "command -v ovs-vsctl"); result.Error != nil {
		return fmt.Errorf("OVS 未安装，请先安装 openvswitch-switch 或 openvswitch")
	}
	if result := utils.ExecCommand("bash", "-c", "command -v dnsmasq"); result.Error != nil {
		return fmt.Errorf("dnsmasq 不可用，请确认已安装 dnsmasq-base")
	}

	bridge := OvsBridgeName()
	if err := os.MkdirAll(OVSConfigDir, 0755); err != nil {
		return fmt.Errorf("创建 OVS 配置目录失败: %w", err)
	}
	if err := os.MkdirAll(OVSStateDir, 0755); err != nil {
		return fmt.Errorf("创建 OVS 状态目录失败: %w", err)
	}
	if _, err := os.Stat(OVSDHCPHostsFile); os.IsNotExist(err) {
		if err := os.WriteFile(OVSDHCPHostsFile, []byte(""), 0644); err != nil {
			return fmt.Errorf("创建 OVS DHCP 静态绑定文件失败: %w", err)
		}
	}

	ovsServiceName := DetectOpenvswitchServiceName()
	EnsureSystemdUnitEnabled(ovsServiceName)
	if !IsSystemdUnitActive(ovsServiceName) {
		utils.ExecCommand("systemctl", "start", ovsServiceName)
	}
	DisableLibvirtDefaultNetworkIfNeeded()
	if result := utils.ExecCommand("ovs-vsctl", "--may-exist", "add-br", bridge); result.Error != nil {
		return fmt.Errorf("创建 OVS 网桥失败: %s", result.Stderr)
	}
	if result := utils.ExecCommand("ip", "link", "set", bridge, "up"); result.Error != nil {
		return fmt.Errorf("启动 OVS 网桥失败: %s", result.Stderr)
	}
	uplink := OvsUplink()
	if uplink == "" {
		return fmt.Errorf("无法检测 OVS NAT 出口网卡，请配置 KVM_OVS_UPLINK")
	}
	addrResult := utils.ExecShellQuiet(fmt.Sprintf("ip -4 addr show dev %s | grep -q '%s/24'", utils.ShellSingleQuote(bridge), OvsGatewayIP()))
	if addrResult.Error != nil {
		utils.ExecCommand("ip", "addr", "flush", "dev", bridge)
		if result := utils.ExecCommand("ip", "addr", "add", OvsGatewayIP()+"/24", "dev", bridge); result.Error != nil {
			return fmt.Errorf("设置 OVS 网关地址失败: %s", result.Stderr)
		}
	}
	if err := EnsureLocalDNSMasqInputRules(bridge); err != nil {
		return err
	}

	dnsConfigChanged, err := writeOVSDNSMasqConfig()
	if err != nil {
		return err
	}
	prepareScriptChanged, err := writeOVSBridgePrepareScript()
	if err != nil {
		return err
	}
	unitChanged, err := writeOVSDNSMasqUnit()
	if err != nil {
		return err
	}
	if unitChanged {
		utils.ExecCommand("systemctl", "daemon-reload")
	}
	EnsureSystemdUnitEnabled(OVSDNSMasqUnit)
	if IsSystemdUnitFailed(OVSDNSMasqUnit) || !IsSystemdUnitActive(OVSDNSMasqUnit) {
		// 启动前释放端口：仅杀占用 gatewayIP:53 的特定 PID，避免误杀 OVS dnsmasq
		gatewayIP := OvsGatewayIP()
		for i := 0; i < 3; i++ {
			pidResult := utils.ExecShellQuiet(fmt.Sprintf("ss -tlnp | grep '%s:53' | grep -oP 'pid=\\K[0-9]+' | head -1", gatewayIP))
			pid := strings.TrimSpace(pidResult.Stdout)
			if pid == "" {
				break
			}
			utils.ExecShellQuiet(fmt.Sprintf("kill %s 2>/dev/null || true", pid))
			time.Sleep(500 * time.Millisecond)
		}
		if result := utils.ExecCommand("systemctl", "start", OVSDNSMasqUnit); result.Error != nil {
			return fmt.Errorf("启动 OVS DHCP 服务失败: %s", result.Stderr)
		}
	} else if dnsConfigChanged {
		ReloadOVSDNSMasq()
	} else if prepareScriptChanged || unitChanged {
		LogNetworkRuntimeChange("OVS DHCP 服务配置已更新，将在下次服务重启时使用新的预启动脚本")
	}

	_, _ = WriteFileIfChanged("/etc/sysctl.d/99-kvm-console-ovs.conf", []byte("net.ipv4.ip_forward=1\n"), 0644)
	if result := utils.ExecCommand("sysctl", "-n", "net.ipv4.ip_forward"); strings.TrimSpace(result.Stdout) != "1" {
		utils.ExecCommand("sysctl", "-w", "net.ipv4.ip_forward=1")
	}
	subnet := OvsSubnetCIDR()
	CleanupStaleManagedNATRules(subnet, bridge, uplink)
	if err := EnsureIPTablesRule(
		fmt.Sprintf("iptables -t nat -C POSTROUTING -s %s -o %s -j MASQUERADE", utils.ShellSingleQuote(subnet), utils.ShellSingleQuote(uplink)),
		fmt.Sprintf("iptables -t nat -A POSTROUTING -s %s -o %s -j MASQUERADE", utils.ShellSingleQuote(subnet), utils.ShellSingleQuote(uplink)),
		"配置 OVS NAT 规则",
	); err != nil {
		return fmt.Errorf("配置 OVS NAT 规则失败: %w", err)
	}
	if err := EnsureIPTablesRule(
		fmt.Sprintf("iptables -C FORWARD -i %s -o %s -j ACCEPT", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(uplink)),
		fmt.Sprintf("iptables -A FORWARD -i %s -o %s -j ACCEPT", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(uplink)),
		"配置 OVS 出站转发规则",
	); err != nil {
		return fmt.Errorf("配置 OVS 出站转发规则失败: %w", err)
	}
	if err := EnsureIPTablesRule(
		fmt.Sprintf("iptables -C FORWARD -i %s -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(bridge)),
		fmt.Sprintf("iptables -A FORWARD -i %s -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(bridge)),
		"配置 OVS 回程转发规则",
	); err != nil {
		return fmt.Errorf("配置 OVS 回程转发规则失败: %w", err)
	}
	return nil
}

// ReloadOVSDNSMasq reloads the OVS dnsmasq service.
func ReloadOVSDNSMasq() {
	if err := HookEnsureOVSBridgeExists(OvsBridgeName()); err != nil {
		logger.App.Warn("OVS 网桥不存在，跳过 dnsmasq 重载", "error", err)
		return
	}
	result := utils.ExecCommand("systemctl", "reload", OVSDNSMasqUnit)
	if result.Error != nil {
		utils.ExecCommand("systemctl", "restart", OVSDNSMasqUnit)
	}
}

// ── Internal helpers (exported for register use) ──

// DisableLibvirtDefaultNetworkIfNeeded disables libvirt's default network if active.
func DisableLibvirtDefaultNetworkIfNeeded() {
	if result := utils.ExecShell("virsh net-info default 2>/dev/null | awk '/^Active:/ {print $2}'"); strings.TrimSpace(result.Stdout) == "yes" {
		utils.ExecCommand("virsh", "net-destroy", "default")
	}
	if result := utils.ExecShell("virsh net-info default 2>/dev/null | awk '/^Autostart:/ {print $2}'"); strings.TrimSpace(result.Stdout) == "yes" {
		utils.ExecCommand("virsh", "net-autostart", "default", "--disable")
	}
	// 等待端口释放，确保 libvirt dnsmasq 完全停止
	// 使用精确 PID 匹配而非 pkill -f，避免误杀 OVS dnsmasq
	gatewayIP := OvsGatewayIP()
	for i := 0; i < 5; i++ {
		result := utils.ExecShellQuiet(fmt.Sprintf("ss -tlnp | grep '%s:53' | grep -oP 'pid=\\K[0-9]+' | head -1", gatewayIP))
		pid := strings.TrimSpace(result.Stdout)
		if pid == "" {
			break
		}
		utils.ExecShellQuiet(fmt.Sprintf("kill %s 2>/dev/null || true", pid))
		time.Sleep(500 * time.Millisecond)
	}
}

// EnsureSystemdUnitEnabled enables a systemd unit if not already enabled.
func EnsureSystemdUnitEnabled(unit string) {
	if strings.TrimSpace(unit) == "" {
		return
	}
	if result := utils.ExecCommand("systemctl", "is-enabled", "--quiet", unit); result.Error != nil {
		utils.ExecCommand("systemctl", "enable", unit)
	}
}

// IsSystemdUnitActive returns true if the given systemd unit is active.
func IsSystemdUnitActive(unit string) bool {
	if strings.TrimSpace(unit) == "" {
		return false
	}
	// 探测类命令：unit 未运行属预期情况，失败仅记 DEBUG
	return utils.ExecCommandQuiet("systemctl", "is-active", "--quiet", unit).Error == nil
}

// IsSystemdUnitFailed returns true if the given systemd unit is in failed state.
// systemctl is-failed: exit 0 = unit is failed, exit 1 = unit is not failed (normal).
func IsSystemdUnitFailed(unit string) bool {
	if strings.TrimSpace(unit) == "" {
		return false
	}
	return utils.ExecCommandQuiet("systemctl", "is-failed", "--quiet", unit).ExitCode == 0
}

// LogNetworkRuntimeChange logs a network runtime change message.
func LogNetworkRuntimeChange(message string) {
	if strings.TrimSpace(message) != "" {
		logger.App.Info(message)
	}
}

// EnsureIPTablesRule checks if an iptables rule exists; if not, adds it.
func EnsureIPTablesRule(checkCmd, addCmd, label string) error {
	if result := utils.ExecShellQuiet(checkCmd); result.Error == nil {
		return nil
	}
	if result := utils.ExecShell(addCmd); result.Error != nil {
		return fmt.Errorf("%s失败: %s", label, result.Stderr)
	}
	return nil
}

// CleanupStaleManagedNATRules removes NAT and FORWARD rules that point to stale uplink interfaces.
func CleanupStaleManagedNATRules(cidr, internalIF, currentUplink string) {
	cidr = strings.TrimSpace(cidr)
	internalIF = strings.TrimSpace(internalIF)
	currentUplink = strings.TrimSpace(currentUplink)
	if cidr == "" || internalIF == "" || currentUplink == "" {
		return
	}
	script := fmt.Sprintf(`CIDR=%s
INTERNAL_IF=%s
CURRENT_UPLINK=%s

delete_rule() {
  table="$1"
  rule="$2"
  delete_rule="${rule/-A /-D }"
  if [ "$table" = "nat" ]; then
    iptables -t nat $delete_rule 2>/dev/null || true
  else
    iptables $delete_rule 2>/dev/null || true
  fi
}

while IFS= read -r rule; do
  case "$rule" in
    *"-s $CIDR "*"-j MASQUERADE"*)
      set -- $rule
      out_if=""
      while [ "$#" -gt 0 ]; do
        if [ "$1" = "-o" ] && [ "$#" -ge 2 ]; then
          out_if="$2"
          break
        fi
        shift
      done
      if [ -n "$out_if" ] && [ "$out_if" != "$CURRENT_UPLINK" ]; then
        delete_rule nat "$rule"
      fi
      ;;
  esac
done <<EOF
$(iptables -t nat -S POSTROUTING 2>/dev/null)
EOF

while IFS= read -r rule; do
  case "$rule" in
    "-A FORWARD -i $INTERNAL_IF "*"-j ACCEPT")
      set -- $rule
      out_if=""
      while [ "$#" -gt 0 ]; do
        if [ "$1" = "-o" ] && [ "$#" -ge 2 ]; then
          out_if="$2"
          break
        fi
        shift
      done
      if [ -n "$out_if" ] && [ "$out_if" != "$CURRENT_UPLINK" ]; then
        delete_rule filter "$rule"
      fi
      ;;
    "-A FORWARD -i "*"-o $INTERNAL_IF "*"--ctstate RELATED,ESTABLISHED"*)
      set -- $rule
      in_if=""
      while [ "$#" -gt 0 ]; do
        if [ "$1" = "-i" ] && [ "$#" -ge 2 ]; then
          in_if="$2"
          break
        fi
        shift
      done
      if [ -n "$in_if" ] && [ "$in_if" != "$CURRENT_UPLINK" ]; then
        delete_rule filter "$rule"
      fi
      ;;
  esac
done <<EOF
$(iptables -S FORWARD 2>/dev/null)
EOF
`, utils.ShellSingleQuote(cidr), utils.ShellSingleQuote(internalIF), utils.ShellSingleQuote(currentUplink))
	utils.ExecCommand("bash", "-c", script)
}

// EnsureLocalDNSMasqInputRules adds iptables rules to allow DHCP/DNS traffic on the given interface.
func EnsureLocalDNSMasqInputRules(iface string) error {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return nil
	}
	rules := []struct {
		proto string
		port  string
		label string
	}{
		{proto: "udp", port: "67", label: "DHCP"},
		{proto: "udp", port: "53", label: "DNS UDP"},
		{proto: "tcp", port: "53", label: "DNS TCP"},
	}
	quotedIface := utils.ShellSingleQuote(iface)
	for _, rule := range rules {
		if err := EnsureIPTablesRule(
			fmt.Sprintf("iptables -C INPUT -i %s -p %s --dport %s -j ACCEPT", quotedIface, rule.proto, rule.port),
			fmt.Sprintf("iptables -I INPUT 1 -i %s -p %s --dport %s -j ACCEPT", quotedIface, rule.proto, rule.port),
			fmt.Sprintf("配置 %s %s 入站规则", iface, rule.label),
		); err != nil {
			return err
		}
	}
	return nil
}

// RemoveLocalDNSMasqInputRules removes DHCP/DNS iptables input rules for the given interface.
func RemoveLocalDNSMasqInputRules(iface string) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return
	}
	quotedIface := utils.ShellSingleQuote(iface)
	for _, rule := range []struct {
		proto string
		port  string
	}{
		{proto: "udp", port: "67"},
		{proto: "udp", port: "53"},
		{proto: "tcp", port: "53"},
	} {
		utils.ExecShell(fmt.Sprintf("while iptables -D INPUT -i %s -p %s --dport %s -j ACCEPT 2>/dev/null; do :; done",
			quotedIface, rule.proto, rule.port))
	}
}

// WriteFileIfChanged writes content to a file only if it differs from the current content.
func WriteFileIfChanged(path string, content []byte, perm os.FileMode) (bool, error) {
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, content) {
		info, statErr := os.Stat(path)
		if statErr == nil && info.Mode().Perm() != perm {
			if chmodErr := os.Chmod(path, perm); chmodErr != nil {
				return false, chmodErr
			}
			return true, nil
		}
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return false, err
	}
	return true, nil
}

func writeOVSDNSMasqConfig() (bool, error) {
	content := fmt.Sprintf(`interface=%s
bind-interfaces
except-interface=lo
dhcp-authoritative
dhcp-range=%s,%s,255.255.255.0,12h
dhcp-option=option:router,%s
dhcp-option=option:dns-server,223.5.5.5,223.6.6.6
dhcp-hostsfile=%s
dhcp-leasefile=%s
pid-file=/run/kvm-console-ovs-dnsmasq.pid
log-dhcp
`, OvsBridgeName(), OvsDHCPStart(), OvsDHCPEnd(), OvsGatewayIP(), OVSDHCPHostsFile, OVSLeasesFile)
	changed, err := WriteFileIfChanged(OVSDNSMasqConf, []byte(content), 0644)
	if err != nil {
		return false, fmt.Errorf("写入 OVS DHCP 配置失败: %w", err)
	}
	return changed, nil
}

func writeOVSBridgePrepareScript() (bool, error) {
	content := fmt.Sprintf(`#!/bin/bash
set -e
BRIDGE=%s
GATEWAY=%s

ovs-vsctl --may-exist add-br "$BRIDGE"
ip link set "$BRIDGE" up
if ! ip -4 addr show dev "$BRIDGE" | grep -q "$GATEWAY"; then
  ip addr flush dev "$BRIDGE"
  ip addr add "$GATEWAY" dev "$BRIDGE"
fi
# 释放端口：只杀 libvirt default 网络的 dnsmasq（监听 192.168.122.1:53），不杀 OVS dnsmasq
LIBVIRT_DNSMASQ_PID=$(ss -tlnp | grep '192.168.122.1:53' | grep -oP 'pid=\K[0-9]+' | head -1)
if [ -n "$LIBVIRT_DNSMASQ_PID" ]; then
  kill "$LIBVIRT_DNSMASQ_PID" 2>/dev/null || true
  sleep 0.5
fi
for rule in "udp 67" "udp 53" "tcp 53"; do
  proto="${rule%% *}"
  port="${rule##* }"
  iptables -C INPUT -i "$BRIDGE" -p "$proto" --dport "$port" -j ACCEPT 2>/dev/null || \
    iptables -I INPUT 1 -i "$BRIDGE" -p "$proto" --dport "$port" -j ACCEPT
done
`, utils.ShellSingleQuote(OvsBridgeName()), utils.ShellSingleQuote(OvsGatewayIP()+"/24"))
	changed, err := WriteFileIfChanged(OVSBridgePrep, []byte(content), 0755)
	if err != nil {
		return false, fmt.Errorf("写入 OVS 网桥预启动脚本失败: %w", err)
	}
	return changed, nil
}

func writeOVSDNSMasqUnit() (bool, error) {
	ovsServiceName := DetectOpenvswitchServiceName()
	content := fmt.Sprintf(`[Unit]
Description=KVM Console OVS DHCP/DNS service
After=network-online.target %s.service
Wants=network-online.target %s.service

[Service]
Type=forking
PIDFile=/run/kvm-console-ovs-dnsmasq.pid
ExecStart=/usr/sbin/dnsmasq --conf-file=/etc/kvm-console/ovs/dnsmasq.conf
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
# 网桥和端口释放由主服务 EnsureOVSNetworkReady() 统一处理，此处不再重复
# 避免 ExecStartPre 与主服务并发操作 ovs-vsctl / pkill 导致竞争

[Install]
WantedBy=multi-user.target
`, ovsServiceName, ovsServiceName)
	path := "/etc/systemd/system/" + OVSDNSMasqUnit
	changed, err := WriteFileIfChanged(path, []byte(content), 0644)
	if err != nil {
		return false, fmt.Errorf("写入 OVS DHCP systemd 服务失败: %w", err)
	}
	return changed, nil
}
