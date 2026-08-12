package bridge

import (
	"net"
	"strings"

	"kvm_console/utils"
)

func ensureBridgeResolvedDNS(uplink, bridge string) {
	ensureBridgeResolvedDNSWithStatic(uplink, bridge, "")
}

// ensureBridgeResolvedDNSWithStatic 确保网桥 DNS 配置正确。
// 优先使用 staticDNS（空格分隔的静态 DNS 列表），为空时动态从 uplink 捕获。
func ensureBridgeResolvedDNSWithStatic(uplink, bridge, staticDNS string) {
	uplink = strings.TrimSpace(uplink)
	bridge = strings.TrimSpace(bridge)
	if bridge == "" {
		return
	}
	if result := utils.ExecCommand("bash", "-c", "command -v resolvectl"); result.Error != nil {
		return
	}
	// 1. 优先使用持久化的静态 DNS（过滤无效 IP 防御脏数据）
	var servers []string
	for _, s := range strings.Fields(strings.TrimSpace(staticDNS)) {
		if net.ParseIP(s) != nil {
			servers = append(servers, s)
		}
	}
	// 2. 从 uplink 动态捕获
	if len(servers) == 0 && uplink != "" {
		servers = resolvectlDNSServers(uplink)
	}
	// 3. 全局 DNS
	if len(servers) == 0 {
		servers = resolvectlDNSServers("")
	}
	// 4. 回退：使用网关作为 DNS（常见于家用路由器）
	if len(servers) == 0 {
		servers = resolvectlDNSServers(bridge)
	}
	if len(servers) > 0 {
		args := append([]string{"dns", bridge}, servers...)
		utils.ExecCommand("resolvectl", args...)
	}
	utils.ExecCommand("resolvectl", "default-route", bridge, "yes")
	utils.ExecCommand("resolvectl", "domain", bridge, "~.")
}

// restoreInterfaceResolvedDNS 将迁移到网桥的 DNS 配置回迁至物理接口。
func restoreInterfaceResolvedDNS(uplink, bridge, staticDNS string) {
	uplink = strings.TrimSpace(uplink)
	bridge = strings.TrimSpace(bridge)
	if uplink == "" || utils.ExecCommand("bash", "-c", "command -v resolvectl").Error != nil {
		return
	}
	var servers []string
	for _, value := range strings.Fields(strings.TrimSpace(staticDNS)) {
		if net.ParseIP(value) != nil {
			servers = append(servers, value)
		}
	}
	if len(servers) == 0 && bridge != "" {
		servers = resolvectlDNSServers(bridge)
	}
	if len(servers) > 0 {
		utils.ExecCommand("resolvectl", append([]string{"dns", uplink}, servers...)...)
	}
	utils.ExecCommand("resolvectl", "default-route", uplink, "yes")
	utils.ExecCommand("resolvectl", "domain", uplink, "~.")
	if bridge != "" {
		utils.ExecCommand("resolvectl", "revert", bridge)
	}
}

func resolvectlDNSServers(link string) []string {
	args := []string{"dns"}
	if strings.TrimSpace(link) != "" {
		args = append(args, strings.TrimSpace(link))
	}
	result := utils.ExecCommand("resolvectl", args...)
	if result.Error != nil {
		return nil
	}
	return parseResolvectlDNSServers(result.Stdout)
}

func parseResolvectlDNSServers(text string) []string {
	seen := map[string]bool{}
	var servers []string
	for _, field := range strings.Fields(text) {
		value := strings.Trim(field, ",;")
		if strings.HasPrefix(value, "[") || strings.HasSuffix(value, "]") || strings.Contains(value, "(") || strings.Contains(value, ")") {
			continue
		}
		host, _, splitErr := net.SplitHostPort(value)
		if splitErr == nil {
			value = host
		}
		ip := net.ParseIP(value)
		if ip == nil || seen[value] {
			continue
		}
		seen[value] = true
		servers = append(servers, value)
	}
	return servers
}

func bridgeResolvedDNSShell() string {
	// 注意：sed 's/^[^:]*://' 匹配到第一个冒号即停止（非贪婪），
	// 避免 IPv6 地址（如 fe80::1）中的冒号被贪婪匹配导致结果错误。
	return `if command -v resolvectl >/dev/null 2>&1; then
  DNS_SERVERS="$(resolvectl dns "$UPLINK" 2>/dev/null | sed 's/^[^:]*://' | xargs)"
  if [ -z "$DNS_SERVERS" ]; then
    DNS_SERVERS="$(resolvectl dns 2>/dev/null | sed 's/^[^:]*://' | xargs)"
  fi
  if [ -n "$DNS_SERVERS" ]; then
    resolvectl dns "$BRIDGE" $DNS_SERVERS || true
  fi
  resolvectl default-route "$BRIDGE" yes || true
  resolvectl domain "$BRIDGE" '~.' || true
fi
`
}

// bridgeResolvedDNSStaticShell 使用静态 HOST_DNS 变量配置 DNS，避免重启后动态捕获失败。
func bridgeResolvedDNSStaticShell() string {
	return `if command -v resolvectl >/dev/null 2>&1; then
  if [ -n "$HOST_DNS" ]; then
    resolvectl dns "$BRIDGE" $HOST_DNS || true
  fi
  resolvectl default-route "$BRIDGE" yes || true
  resolvectl domain "$BRIDGE" '~.' || true
fi
`
}
