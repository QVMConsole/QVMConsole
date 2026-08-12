package bridge

import (
	"fmt"
	"strings"

	"kvm_console/utils"
)

// HostIPConfig 存储从接口捕获的 IP 配置，用于持久化到数据库和恢复脚本。
type HostIPConfig struct {
	Addrs    string // 换行分隔的 IPv4 CIDR 地址列表
	Gateway  string // IPv4 默认网关 IP
	Metric   string // IPv4 路由 metric
	DNS      string // 空格分隔的 DNS 服务器 IP 列表（可含 IPv4/IPv6）
	Addrs6   string // 换行分隔的 IPv6 CIDR 地址列表
	Gateway6 string // IPv6 默认网关 IP
	Metric6  string // IPv6 路由 metric
}

// CaptureInterfaceIPv4 从指定接口捕获当前 IPv4 配置（地址、网关、metric、DNS）。
func CaptureInterfaceIPv4(iface string) HostIPConfig {
	var cfg HostIPConfig
	result := utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip -4 -o addr show dev %s scope global 2>/dev/null | awk '{print $4}'`,
		utils.ShellSingleQuote(iface)))
	cfg.Addrs = strings.TrimSpace(result.Stdout)

	result = utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip -4 route show default dev %s 2>/dev/null | awk '{print $3; exit}'`,
		utils.ShellSingleQuote(iface)))
	cfg.Gateway = strings.TrimSpace(result.Stdout)

	result = utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip -4 route show default dev %s 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="metric") {print $(i+1); exit}}'`,
		utils.ShellSingleQuote(iface)))
	cfg.Metric = strings.TrimSpace(result.Stdout)
	cfg.DNS = captureInterfaceDNSServers(iface)
	return cfg
}

// CaptureInterfaceIPv6 从指定接口捕获当前 IPv6 配置（地址、网关、metric）。
// 不捕获 DNS（DNS 在 resolvectl 中是全局的，不区分地址族）。
func CaptureInterfaceIPv6(iface string) HostIPConfig {
	var cfg HostIPConfig
	result := utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip -6 -o addr show dev %s scope global 2>/dev/null | awk '{print $4}'`,
		utils.ShellSingleQuote(iface)))
	cfg.Addrs6 = strings.TrimSpace(result.Stdout)

	result = utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip -6 route show default dev %s 2>/dev/null | awk '{print $3; exit}'`,
		utils.ShellSingleQuote(iface)))
	cfg.Gateway6 = strings.TrimSpace(result.Stdout)

	result = utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip -6 route show default dev %s 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="metric") {print $(i+1); exit}}'`,
		utils.ShellSingleQuote(iface)))
	cfg.Metric6 = strings.TrimSpace(result.Stdout)
	return cfg
}

// CaptureInterfaceIP 同时捕获指定接口的 IPv4 和 IPv6 配置。
// DNS 仅从 IPv4 捕获函数获取（resolvectl DNS 不区分地址族）。
func CaptureInterfaceIP(iface string) HostIPConfig {
	v4 := CaptureInterfaceIPv4(iface)
	v6 := CaptureInterfaceIPv6(iface)
	return HostIPConfig{
		Addrs: v4.Addrs, Gateway: v4.Gateway, Metric: v4.Metric, DNS: v4.DNS,
		Addrs6: v6.Addrs6, Gateway6: v6.Gateway6, Metric6: v6.Metric6,
	}
}

// captureInterfaceDNSServers 从 resolvectl 捕获指定接口的 DNS 服务器，返回空格分隔的 IP 列表。
// 优先从指定接口捕获，回退到全局 DNS。
// 使用 Go 原生 IP 解析（resolvectlDNSServers），避免 shell sed 正则被 IPv6 地址中的冒号干扰。
func captureInterfaceDNSServers(iface string) string {
	// 先从指定接口获取
	servers := resolvectlDNSServers(iface)
	if len(servers) > 0 {
		return strings.Join(servers, " ")
	}
	// 回退到全局
	servers = resolvectlDNSServers("")
	if len(servers) > 0 {
		return strings.Join(servers, " ")
	}
	return ""
}

func migrateInterfaceIPToBridge(uplink, bridge string) {
	script := fmt.Sprintf(`set -e
UPLINK=%s
BRIDGE=%s
%s
`, utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(bridge), bridgeHostIPMigrationShell())
	utils.ExecCommand("bash", "-c", script)
}

// migrateInterfaceIPv4ToBridge 兼容旧调用，等价于 migrateInterfaceIPToBridge。
func migrateInterfaceIPv4ToBridge(uplink, bridge string) {
	migrateInterfaceIPToBridge(uplink, bridge)
}

// applyStaticIPToBridge 使用静态存储的 IP 配置应用到网桥（用于重启后恢复）。
// 同时处理 IPv4 和 IPv6。
func applyStaticIPToBridge(bridge string, cfg HostIPConfig) {
	if strings.TrimSpace(cfg.Addrs) == "" && strings.TrimSpace(cfg.Addrs6) == "" {
		return
	}
	script := fmt.Sprintf(`set -e
BRIDGE=%s
HOST_ADDRS=%s
HOST_GW=%s
HOST_METRIC=%s
HOST_ADDRS6=%s
HOST_GW6=%s
HOST_METRIC6=%s
%s
`, utils.ShellSingleQuote(bridge),
		utils.ShellSingleQuote(cfg.Addrs),
		utils.ShellSingleQuote(cfg.Gateway),
		utils.ShellSingleQuote(cfg.Metric),
		utils.ShellSingleQuote(cfg.Addrs6),
		utils.ShellSingleQuote(cfg.Gateway6),
		utils.ShellSingleQuote(cfg.Metric6),
		bridgeHostIPApplyStaticShell())
	utils.ExecCommand("bash", "-c", script)
}

// applyStaticIPv4ToBridge 兼容旧调用，仅处理 IPv4 部分时使用。
// 新调用方应使用 applyStaticIPToBridge。
func applyStaticIPv4ToBridge(bridge string, cfg HostIPConfig) {
	applyStaticIPToBridge(bridge, cfg)
}

func migrateBridgeIPToInterface(bridge, uplink string) {
	script := fmt.Sprintf(`set -e
BRIDGE=%s
UPLINK=%s
%s
`, utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(uplink), bridgeHostIPRollbackShell())
	utils.ExecCommand("bash", "-c", script)
}

// migrateBridgeIPv4ToInterface 兼容旧调用，等价于 migrateBridgeIPToInterface。
func migrateBridgeIPv4ToInterface(bridge, uplink string) {
	migrateBridgeIPToInterface(bridge, uplink)
}

func bridgeHostIPMigrationShell() string {
	return bridgeHostIPCaptureShell() + bridgeHostIPApplyShell()
}

func bridgeHostIPRollbackShell() string {
	return bridgeHostIPCaptureFromBridgeShell() + bridgeHostIPApplyToUplinkShell()
}

func bridgeHostIPCaptureShell() string {
	return `# 捕获 IPv4
HOST_ADDRS="$(ip -4 -o addr show dev "$UPLINK" scope global 2>/dev/null | awk '{print $4}')"
HOST_GW="$(ip -4 route show default dev "$UPLINK" 2>/dev/null | awk '{print $3; exit}')"
HOST_METRIC="$(ip -4 route show default dev "$UPLINK" 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="metric") {print $(i+1); exit}}')"
# 捕获 IPv6
HOST_ADDRS6="$(ip -6 -o addr show dev "$UPLINK" scope global 2>/dev/null | awk '{print $4}')"
HOST_GW6="$(ip -6 route show default dev "$UPLINK" 2>/dev/null | awk '{print $3; exit}')"
HOST_METRIC6="$(ip -6 route show default dev "$UPLINK" 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="metric") {print $(i+1); exit}}')"
`
}

func bridgeHostIPCaptureFromBridgeShell() string {
	return `# 捕获 IPv4
HOST_ADDRS="$(ip -4 -o addr show dev "$BRIDGE" scope global 2>/dev/null | awk '{print $4}')"
HOST_GW="$(ip -4 route show default dev "$BRIDGE" 2>/dev/null | awk '{print $3; exit}')"
HOST_METRIC="$(ip -4 route show default dev "$BRIDGE" 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="metric") {print $(i+1); exit}}')"
# 捕获 IPv6
HOST_ADDRS6="$(ip -6 -o addr show dev "$BRIDGE" scope global 2>/dev/null | awk '{print $4}')"
HOST_GW6="$(ip -6 route show default dev "$BRIDGE" 2>/dev/null | awk '{print $3; exit}')"
HOST_METRIC6="$(ip -6 route show default dev "$BRIDGE" 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="metric") {print $(i+1); exit}}')"
`
}

func bridgeHostIPApplyShell() string {
	return `# 应用 IPv4
if [ -n "$HOST_ADDRS" ]; then
  ip -4 addr flush dev "$UPLINK" scope global 2>/dev/null || true
  while IFS= read -r addr; do
    [ -n "$addr" ] || continue
    ip addr replace "$addr" dev "$BRIDGE"
  done <<< "$HOST_ADDRS"
fi
if [ -n "$HOST_GW" ]; then
  ip route del "$HOST_GW" dev "$UPLINK" 2>/dev/null || true
  ip route replace "$HOST_GW" dev "$BRIDGE" scope link
  if [ -n "$HOST_METRIC" ]; then
    ip route replace default via "$HOST_GW" dev "$BRIDGE" metric "$HOST_METRIC"
  else
    ip route replace default via "$HOST_GW" dev "$BRIDGE"
  fi
fi
# 应用 IPv6
if [ -n "$HOST_ADDRS6" ]; then
  ip -6 addr flush dev "$UPLINK" scope global 2>/dev/null || true
  while IFS= read -r addr; do
    [ -n "$addr" ] || continue
    ip addr replace "$addr" dev "$BRIDGE"
  done <<< "$HOST_ADDRS6"
fi
if [ -n "$HOST_GW6" ]; then
  ip -6 route del "$HOST_GW6" dev "$UPLINK" 2>/dev/null || true
  ip -6 route replace "$HOST_GW6" dev "$BRIDGE" scope link 2>/dev/null || true
  if [ -n "$HOST_METRIC6" ]; then
    ip -6 route replace default via "$HOST_GW6" dev "$BRIDGE" metric "$HOST_METRIC6"
  else
    ip -6 route replace default via "$HOST_GW6" dev "$BRIDGE"
  fi
fi
`
}

func bridgeHostIPApplyToUplinkShell() string {
	return `ip link set "$UPLINK" up
# 应用 IPv4
if [ -n "$HOST_ADDRS" ]; then
  ip -4 addr flush dev "$BRIDGE" scope global 2>/dev/null || true
  while IFS= read -r addr; do
    [ -n "$addr" ] || continue
    ip addr replace "$addr" dev "$UPLINK"
  done <<< "$HOST_ADDRS"
fi
if [ -n "$HOST_GW" ]; then
  ip route del "$HOST_GW" dev "$BRIDGE" 2>/dev/null || true
  ip route replace "$HOST_GW" dev "$UPLINK" scope link
  if [ -n "$HOST_METRIC" ]; then
    ip route replace default via "$HOST_GW" dev "$UPLINK" metric "$HOST_METRIC"
  else
    ip route replace default via "$HOST_GW" dev "$UPLINK"
  fi
fi
# 应用 IPv6
if [ -n "$HOST_ADDRS6" ]; then
  ip -6 addr flush dev "$BRIDGE" scope global 2>/dev/null || true
  while IFS= read -r addr; do
    [ -n "$addr" ] || continue
    ip addr replace "$addr" dev "$UPLINK"
  done <<< "$HOST_ADDRS6"
fi
if [ -n "$HOST_GW6" ]; then
  ip -6 route del "$HOST_GW6" dev "$BRIDGE" 2>/dev/null || true
  ip -6 route replace "$HOST_GW6" dev "$UPLINK" scope link 2>/dev/null || true
  if [ -n "$HOST_METRIC6" ]; then
    ip -6 route replace default via "$HOST_GW6" dev "$UPLINK" metric "$HOST_METRIC6"
  else
    ip -6 route replace default via "$HOST_GW6" dev "$UPLINK"
  fi
fi
`
}

// bridgeHostIPApplyStaticShell 生成使用静态变量应用 IP 的 shell 代码。
// 同时处理 IPv4 和 IPv6。
func bridgeHostIPApplyStaticShell() string {
	return `# 应用 IPv4
if [ -n "$HOST_ADDRS" ]; then
  while IFS= read -r addr; do
    [ -n "$addr" ] || continue
    ip addr replace "$addr" dev "$BRIDGE"
  done <<< "$HOST_ADDRS"
fi
if [ -n "$HOST_GW" ]; then
  ip route replace "$HOST_GW" dev "$BRIDGE" scope link 2>/dev/null || true
  if [ -n "$HOST_METRIC" ]; then
    ip route replace default via "$HOST_GW" dev "$BRIDGE" metric "$HOST_METRIC"
  else
    ip route replace default via "$HOST_GW" dev "$BRIDGE"
  fi
fi
# 应用 IPv6
if [ -n "$HOST_ADDRS6" ]; then
  while IFS= read -r addr; do
    [ -n "$addr" ] || continue
    ip addr replace "$addr" dev "$BRIDGE"
  done <<< "$HOST_ADDRS6"
fi
if [ -n "$HOST_GW6" ]; then
  ip -6 route replace "$HOST_GW6" dev "$BRIDGE" scope link 2>/dev/null || true
  if [ -n "$HOST_METRIC6" ]; then
    ip -6 route replace default via "$HOST_GW6" dev "$BRIDGE" metric "$HOST_METRIC6"
  else
    ip -6 route replace default via "$HOST_GW6" dev "$BRIDGE"
  fi
fi
`
}
