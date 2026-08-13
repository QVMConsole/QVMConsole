package public_ip

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/utils"
)

var publicIPApplyMu sync.Mutex

func ApplyPublicIPRules() error {
	publicIPApplyMu.Lock()
	defer publicIPApplyMu.Unlock()
	return applyPublicIPRulesLocked(true)
}

func applyPublicIPRulesLocked(syncPrefix bool) error {
	if syncPrefix {
		if _, err := SyncManagedPublicIPv6Addresses(); err != nil {
			return fmt.Errorf("同步动态公网 IPv6 前缀失败: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(publicIPConfigDir, "backups"), 0755); err != nil {
		return fmt.Errorf("创建公网 IP 配置目录失败: %w", err)
	}
	if _, err := os.Stat(publicIPRulesPath); err == nil {
		backup := filepath.Join(publicIPConfigDir, "backups", "rules.sh."+time.Now().Format("20060102_150405"))
		if err := copyFile(publicIPRulesPath, backup, 0755); err != nil {
			logger.App.Warn("备份公网IP规则失败", "error", err)
		}
	}
	script, err := BuildPublicIPRulesScript()
	if err != nil {
		return err
	}
	if _, err := HookWriteFileIfChanged(publicIPRulesPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("写入公网 IP 规则失败: %w", err)
	}
	result := utils.ExecCommand("bash", publicIPRulesPath)
	if result.Error != nil {
		msg := strings.TrimSpace(result.Stderr)
		if msg == "" {
			msg = result.Error.Error()
		}
		return fmt.Errorf("应用公网 IP 规则失败: %s", msg)
	}
	return nil
}

func RestorePublicIPRules() error {
	return ApplyPublicIPRules()
}

func BuildPublicIPRulesScript() (string, error) {
	var bindings []model.PublicIPBinding
	if err := model.DB.Order("public_ip ASC").Find(&bindings).Error; err != nil {
		return "", err
	}
	ipRows := map[uint]model.PublicIP{}
	var ips []model.PublicIP
	if err := model.DB.Find(&ips).Error; err != nil {
		return "", err
	}
	for _, ipRow := range ips {
		ipRows[ipRow.ID] = ipRow
	}

	var b strings.Builder
	b.WriteString("#!/bin/bash\n")
	b.WriteString("set -e\n")
	b.WriteString("# KVM 公网 IP / 浮动 IP 规则 - 自动生成\n\n")
	b.WriteString(cleanupPublicIPRulesShell())
	b.WriteString("\n")
	b.WriteString(cleanupPublicIPHostAddressesShell(ips))
	b.WriteString("\n")
	b.WriteString(cleanupPublicIPv6StateShell(ips))
	b.WriteString("\n")
	b.WriteString("sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || true\n\n")
	for _, binding := range bindings {
		if ipRow, ok := ipRows[binding.PublicIPID]; ok && publicIPIsIPv6(ipRow.IP) {
			uplink := effectivePublicIPUplink(ipRow.UplinkIF, true)
			if uplink != "" {
				b.WriteString(fmt.Sprintf("sysctl -w net.ipv6.conf.%s.accept_ra=2 >/dev/null 2>&1 || true\n", uplink))
				b.WriteString(fmt.Sprintf("sysctl -w net.ipv6.conf.%s.proxy_ndp=1 >/dev/null 2>&1 || true\n", uplink))
			}
			b.WriteString("sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null 2>&1 || true\n\n")
			break
		}
	}

	for _, binding := range bindings {
		ipRow, ok := ipRows[binding.PublicIPID]
		if !ok {
			continue
		}
		commands, err := buildPublicIPCommands(ipRow, PublicIPBindRequest{
			Username:    binding.Username,
			VMName:      binding.VMName,
			VMPrivateIP: binding.VMPrivateIP,
			Mode:        binding.Mode,
		})
		if err != nil {
			return "", err
		}
		for _, cmd := range commands {
			b.WriteString(cmd)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("exit 0\n")
	return b.String(), nil
}

func buildPublicIPCommands(ipRow model.PublicIP, req PublicIPBindRequest) ([]string, error) {
	mode := NormalizePublicIPMode(req.Mode)
	switch mode {
	case PublicIPModeNAT:
		if strings.TrimSpace(req.VMPrivateIP) == "" {
			return nil, fmt.Errorf("1:1 NAT 模式需要 VM 私网 IP")
		}
		return buildPublicIPNATCommands(ipRow, req), nil
	case PublicIPModeClassicRoute:
		return buildPublicIPClassicRouteCommands(ipRow, req), nil
	case PublicIPModeClassicBridge:
		return buildPublicIPClassicBridgeCommands(ipRow, req), nil
	default:
		return nil, fmt.Errorf("不支持的公网 IP 模式: %s", req.Mode)
	}
}

func buildPublicIPNATCommands(ipRow model.PublicIP, req PublicIPBindRequest) []string {
	publicIP := strings.TrimSpace(ipRow.IP)
	privateIP := strings.TrimSpace(req.VMPrivateIP)
	uplink := effectivePublicIPUplink(ipRow.UplinkIF, false)
	comment := publicIPRuleComment + ":" + publicIP
	var cmds []string
	if addr := publicIPAddrForHost(ipRow); addr != "" && uplink != "" {
		cmds = append(cmds, fmt.Sprintf("ip addr replace %s dev %s || true", utils.ShellSingleQuote(addr), utils.ShellSingleQuote(uplink)))
	}
	cmds = append(cmds,
		fmt.Sprintf("iptables -t nat -A PREROUTING -d %s/32 -m comment --comment %s -j DNAT --to-destination %s",
			utils.ShellSingleQuote(publicIP), utils.ShellSingleQuote(comment+":dnat"), utils.ShellSingleQuote(privateIP)),
	)
	if uplink != "" {
		cmds = append(cmds, fmt.Sprintf("iptables -t nat -I POSTROUTING 1 -s %s/32 -o %s -m comment --comment %s -j SNAT --to-source %s",
			utils.ShellSingleQuote(privateIP), utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(comment+":snat"), utils.ShellSingleQuote(publicIP)))
	} else {
		cmds = append(cmds, fmt.Sprintf("iptables -t nat -I POSTROUTING 1 -s %s/32 -m comment --comment %s -j SNAT --to-source %s",
			utils.ShellSingleQuote(privateIP), utils.ShellSingleQuote(comment+":snat"), utils.ShellSingleQuote(publicIP)))
	}
	if !HookIsVPCManagedIP(privateIP) {
		cmds = append(cmds,
			fmt.Sprintf("iptables -A FORWARD -d %s/32 -m comment --comment %s -j ACCEPT", utils.ShellSingleQuote(privateIP), utils.ShellSingleQuote(comment+":forward-in")),
			fmt.Sprintf("iptables -A FORWARD -s %s/32 -m comment --comment %s -j ACCEPT", utils.ShellSingleQuote(privateIP), utils.ShellSingleQuote(comment+":forward-out")),
		)
	}
	return cmds
}

func buildPublicIPClassicRouteCommands(ipRow model.PublicIP, req PublicIPBindRequest) []string {
	if publicIPIsIPv6(ipRow.IP) {
		return buildPublicIPv6RouteCommands(ipRow, req)
	}
	bridge := publicIPVMBridge(req.VMName)
	if bridge == "" {
		bridge = HookOvsBridgeName()
	}
	var cmds []string
	uplink := effectivePublicIPUplink(ipRow.UplinkIF, false)
	if uplink != "" {
		cmds = append(cmds, fmt.Sprintf("sysctl -w net.ipv4.conf.%s.proxy_arp=1 >/dev/null 2>&1 || true", uplink))
	}
	if strings.TrimSpace(req.VMPrivateIP) != "" {
		cmds = append(cmds, fmt.Sprintf("ip route replace %s/32 via %s dev %s || true",
			utils.ShellSingleQuote(ipRow.IP), utils.ShellSingleQuote(req.VMPrivateIP), utils.ShellSingleQuote(bridge)))
	} else {
		cmds = append(cmds, fmt.Sprintf("ip route replace %s/32 dev %s || true", utils.ShellSingleQuote(ipRow.IP), utils.ShellSingleQuote(bridge)))
	}
	cmds = append(cmds, buildPublicIPAntiSpoofCommands(ipRow, req)...)
	return cmds
}

func buildPublicIPv6RouteCommands(ipRow model.PublicIP, req PublicIPBindRequest) []string {
	publicIP := strings.TrimSpace(ipRow.IP)
	uplink := effectivePublicIPUplink(ipRow.UplinkIF, true)
	routeIF := publicIPVMRouteInterface(req.VMName)
	if routeIF == "" {
		routeIF = HookOvsBridgeName()
	}
	gateway := publicIPv6GatewayLinkLocal(req.VMName)
	comment := publicIPRuleComment + ":" + publicIP
	cmds := make([]string, 0, 6)
	if gateway != "" {
		cmds = append(cmds,
			fmt.Sprintf("ip link set dev %s up || true", utils.ShellSingleQuote(routeIF)),
			fmt.Sprintf("ip -6 addr replace %s/64 dev %s", utils.ShellSingleQuote(gateway), utils.ShellSingleQuote(routeIF)),
		)
	}
	cmds = append(cmds, fmt.Sprintf("ip -6 route replace %s/128 dev %s", utils.ShellSingleQuote(publicIP), utils.ShellSingleQuote(routeIF)))
	if uplink != "" {
		cmds = append(cmds,
			fmt.Sprintf("ip -6 neigh replace proxy %s dev %s", utils.ShellSingleQuote(publicIP), utils.ShellSingleQuote(uplink)),
			fmt.Sprintf("ip6tables -A FORWARD -i %s -o %s -d %s/128 -m comment --comment %s -j ACCEPT", utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(routeIF), utils.ShellSingleQuote(publicIP), utils.ShellSingleQuote(comment+":forward-in")),
			fmt.Sprintf("ip6tables -A FORWARD -i %s -o %s -s %s/128 -m comment --comment %s -j ACCEPT", utils.ShellSingleQuote(routeIF), utils.ShellSingleQuote(uplink), utils.ShellSingleQuote(publicIP), utils.ShellSingleQuote(comment+":forward-out")),
		)
	}
	cmds = append(cmds, buildPublicIPAntiSpoofCommands(ipRow, req)...)
	return cmds
}

func buildPublicIPClassicBridgeCommands(ipRow model.PublicIP, req PublicIPBindRequest) []string {
	return buildPublicIPAntiSpoofCommands(ipRow, req)
}

func buildPublicIPAntiSpoofCommands(ipRow model.PublicIP, req PublicIPBindRequest) []string {
	// 端口安全开启后，公网地址作为统一身份编译器的输入，不再写入 table 0 重叠规则。
	if config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled {
		return nil
	}
	if strings.TrimSpace(req.VMName) == "" {
		return nil
	}
	iface, bridge := publicIPVMInterface(req.VMName)
	if iface == "" || bridge == "" {
		return []string{fmt.Sprintf("# 未找到 VM %s 的运行态 OVS 端口，跳过经典网络防伪造流表", req.VMName)}
	}
	ofport := HookGetOVSInterfaceOfPort(iface)
	if ofport == "" {
		return []string{fmt.Sprintf("# VM %s 的 OVS ofport 无效，跳过经典网络防伪造流表", req.VMName)}
	}
	cookie := publicIPFlowCookie(ipRow.IP)
	if publicIPIsIPv6(ipRow.IP) {
		return []string{
			fmt.Sprintf("ovs-ofctl -O OpenFlow13 del-flows %s %s || true", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote("cookie="+cookie+"/-1")),
			fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=245,in_port=%s,icmp6,icmp_type=135,ipv6_src=::,nd_target=%s,actions=NORMAL", cookie, ofport, ipRow.IP))),
			fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=240,in_port=%s,ipv6,ipv6_src=%s,actions=NORMAL", cookie, ofport, ipRow.IP))),
			fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=230,in_port=%s,ipv6,actions=drop", cookie, ofport))),
		}
	}
	commands := []string{
		fmt.Sprintf("ovs-ofctl -O OpenFlow13 del-flows %s %s || true", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote("cookie="+cookie+"/-1")),
	}
	// 路由模式以 VM 私网地址作为宿主机下一跳，需保留其 ARP 应答；
	// 同时仅允许该地址和已绑定公网地址从对应端口发出。
	privateIP := net.ParseIP(strings.TrimSpace(req.VMPrivateIP))
	if NormalizePublicIPMode(req.Mode) == PublicIPModeClassicRoute && privateIP != nil && privateIP.To4() != nil {
		commands = append(commands,
			fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=240,in_port=%s,ip,nw_src=%s,actions=NORMAL", cookie, ofport, req.VMPrivateIP))),
			fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=240,in_port=%s,arp,arp_spa=%s,actions=NORMAL", cookie, ofport, req.VMPrivateIP))),
		)
	}
	commands = append(commands,
		fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=240,in_port=%s,ip,nw_src=%s,actions=NORMAL", cookie, ofport, ipRow.IP))),
		fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=240,in_port=%s,arp,arp_spa=%s,actions=NORMAL", cookie, ofport, ipRow.IP))),
		fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=230,in_port=%s,ip,actions=drop", cookie, ofport))),
		fmt.Sprintf("ovs-ofctl -O OpenFlow13 add-flow %s %s", utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(fmt.Sprintf("cookie=%s,priority=230,in_port=%s,arp,actions=drop", cookie, ofport))),
	)
	return commands
}

func cleanupPublicIPRulesShell() string {
	var b strings.Builder
	b.WriteString(`cleanup_iptables_comments() {
  table="$1"
  chain="$2"
  iptables_cmd=(iptables)
  [ "$table" = "nat" ] && iptables_cmd=(iptables -t nat)
  while true; do
    line="$("${iptables_cmd[@]}" -nL "$chain" --line-numbers 2>/dev/null | awk '/kvm-console:public-ip/ {print $1}' | sort -rn | head -n1)"
    [ -n "$line" ] || break
    "${iptables_cmd[@]}" -D "$chain" "$line" 2>/dev/null || break
  done
}

cleanup_iptables_comments nat PREROUTING
cleanup_iptables_comments nat POSTROUTING
cleanup_iptables_comments filter FORWARD

cleanup_ip6tables_comments() {
  chain="$1"
  while true; do
    line="$(ip6tables -nL "$chain" --line-numbers 2>/dev/null | awk '/kvm-console:public-ip/ {print $1}' | sort -rn | head -n1)"
    [ -n "$line" ] || break
    ip6tables -D "$chain" "$line" 2>/dev/null || break
  done
}

cleanup_ip6tables_comments FORWARD
`)
	for _, bridge := range publicIPManagedBridges() {
		b.WriteString(fmt.Sprintf("ovs-ofctl -O OpenFlow13 del-flows %s %s 2>/dev/null || true\n",
			utils.ShellSingleQuote(bridge), utils.ShellSingleQuote("cookie="+publicIPFlowPrefix+"00000000000000/"+publicIPFlowMask)))
	}
	return b.String()
}

func cleanupPublicIPv6StateShell(ipRows []model.PublicIP) string {
	var b strings.Builder
	b.WriteString("# 清理面板托管的公网 IPv6 Proxy NDP 与精确路由，后续按当前绑定重建\n")
	for _, ipRow := range ipRows {
		if !publicIPIsIPv6(ipRow.IP) {
			continue
		}
		for _, uplink := range publicIPUplinkCandidates(ipRow.UplinkIF, true) {
			b.WriteString(fmt.Sprintf("ip -6 neigh del proxy %s dev %s 2>/dev/null || true\n", utils.ShellSingleQuote(ipRow.IP), utils.ShellSingleQuote(uplink)))
		}
		b.WriteString(fmt.Sprintf("ip -6 route del %s/128 2>/dev/null || true\n", utils.ShellSingleQuote(ipRow.IP)))
	}
	return b.String()
}

func cleanupPublicIPHostAddressesShell(ipRows []model.PublicIP) string {
	type addrKey struct {
		addr string
		dev  string
	}
	seen := map[addrKey]bool{}
	var items []addrKey
	for _, ipRow := range ipRows {
		if strings.TrimSpace(ipRow.IP) == "" || net.ParseIP(strings.TrimSpace(ipRow.IP)) == nil {
			continue
		}
		uplink := effectivePublicIPUplink(ipRow.UplinkIF, publicIPIsIPv6(ipRow.IP))
		if uplink == "" {
			continue
		}
		item := addrKey{addr: publicIPAddrForHost(ipRow), dev: uplink}
		if item.addr == "" || seen[item] {
			continue
		}
		seen[item] = true
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].dev == items[j].dev {
			return items[i].addr < items[j].addr
		}
		return items[i].dev < items[j].dev
	})

	var b strings.Builder
	b.WriteString("# 清理面板托管的宿主机公网地址，后续会按当前绑定重新添加\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("ip addr del %s dev %s 2>/dev/null || true\n",
			utils.ShellSingleQuote(item.addr), utils.ShellSingleQuote(item.dev)))
	}
	return b.String()
}

func cleanupConntrackForPublicIP(publicIP string) {
	publicIP = strings.TrimSpace(publicIP)
	if publicIP == "" {
		return
	}
	if publicIPIsIPv6(publicIP) {
		utils.ExecCommandQuiet("conntrack", "-f", "ipv6", "-D", "-d", publicIP)
		utils.ExecCommandQuiet("conntrack", "-f", "ipv6", "-D", "-s", publicIP)
		return
	}
	utils.ExecCommandQuiet("conntrack", "-f", "ipv4", "-D", "-d", publicIP)
	utils.ExecCommandQuiet("conntrack", "-f", "ipv4", "-D", "-s", publicIP)
}
