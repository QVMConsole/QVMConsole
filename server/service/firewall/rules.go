package firewall

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"

	"kvm_console/model"
	"kvm_console/service/ip_resolver"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// BuildFirewallRules 将策略转换为 nftables 规则。
func BuildFirewallRules(policy *FirewallPolicy) (string, error) {
	policy = normalizeFirewallPolicy(policy)
	if err := ValidateFirewallPolicy(policy); err != nil {
		return "", err
	}

	regionMap := make(map[string]FirewallRegion)
	for _, region := range policy.Regions {
		regionMap[normalizeRegionCode(region.Code)] = region
	}

	scopes := firewallNetworkScopes(policy)
	managedCIDRs := firewallScopeCIDRs(scopes)

	var b strings.Builder
	b.WriteString("table inet ")
	b.WriteString(firewallTable)
	b.WriteString(" {\n")

	writeSet := func(name string, cidrs []string) {
		cidrs = normalizeCIDRList(cidrs)
		b.WriteString("  set ")
		b.WriteString(name)
		b.WriteString(" {\n    type ipv4_addr\n    flags interval\n")
		if len(cidrs) > 0 {
			b.WriteString("    elements = { ")
			b.WriteString(strings.Join(cidrs, ", "))
			b.WriteString(" }\n")
		}
		b.WriteString("  }\n\n")
	}

	writeSet("vm_subnets4", managedCIDRs)
	writeSet("whitelist4", append(defaultWhitelistCIDRsForScopes(scopes), policy.WhitelistCIDRs...))
	writeSet("out_allowed4", collectRegionCIDRs(policy.OutboundAllowedRegions, regionMap))
	writeSet("in_allowed4", collectRegionCIDRs(policy.InboundAllowedRegions, regionMap))

	vmNames := sortedMapKeys(policy.VMOverrides)
	for _, vmName := range vmNames {
		override := policy.VMOverrides[vmName]
		mode := normalizeOverrideMode(override.Mode)
		if mode == "allow" || mode == "block" {
			writeSet(vmSetName(vmName), collectRegionCIDRs(override.Regions, regionMap))
		}
	}

	b.WriteString("  chain forward {\n")
	b.WriteString("    type filter hook forward priority -50; policy accept;\n")
	b.WriteString("    ct state established,related accept\n")
	for _, scope := range scopes {
		b.WriteString(fmt.Sprintf("    iifname %q ip saddr %s ip daddr @whitelist4 accept\n", scope.IfName, scope.CIDR))
		b.WriteString(fmt.Sprintf("    oifname %q ip daddr %s ip saddr @whitelist4 accept\n", scope.IfName, scope.CIDR))
		b.WriteString(fmt.Sprintf("    iifname %q oifname %q accept\n", scope.IfName, scope.IfName))
	}
	// 已绑定的路由型公网 IPv6 是显式放行资源；否则全局“禁用 VM IPv6”会先于
	// Proxy NDP 转发命中并阻断。VPC 端口安全仍负责精确源地址校验。
	for _, address := range boundPublicIPv6Addresses() {
		b.WriteString(fmt.Sprintf("    ip6 saddr %s accept\n", address))
		b.WriteString(fmt.Sprintf("    ip6 daddr %s accept\n", address))
	}

	if policy.DisableVMIPv6 {
		for _, scope := range scopes {
			b.WriteString(fmt.Sprintf("    iifname %q meta nfproto ipv6 reject\n", scope.IfName))
			b.WriteString(fmt.Sprintf("    oifname %q meta nfproto ipv6 reject\n", scope.IfName))
		}
	}

	for _, rule := range currentPortForwardRulesForPolicy(policy) {
		if !policy.PortForwardExemptions[rule.StableKey()] {
			continue
		}
		proto := strings.ToLower(rule.Protocol)
		if proto == "tcp" || proto == "udp" {
			scope, ok := firewallScopeForIP(scopes, rule.DestIP)
			if !ok {
				continue
			}
			b.WriteString(fmt.Sprintf("    oifname %q ip daddr %s %s dport %s accept\n", scope.IfName, rule.DestIP, proto, rule.DestPort))
		}
	}

	for _, vmName := range vmNames {
		override := policy.VMOverrides[vmName]
		vmIP := GetFirewallVMIP(vmName)
		if vmIP == "" {
			continue
		}
		scope, ok := firewallScopeForIP(scopes, vmIP)
		if !ok {
			continue
		}
		switch normalizeOverrideMode(override.Mode) {
		case "inbound_only":
			b.WriteString(fmt.Sprintf("    iifname %q ip saddr %s %s\n", scope.IfName, vmIP, policy.BlockAction))
		case "disabled":
			b.WriteString(fmt.Sprintf("    ip saddr %s accept\n", vmIP))
			b.WriteString(fmt.Sprintf("    ip daddr %s accept\n", vmIP))
		case "allow":
			setName := vmSetName(vmName)
			b.WriteString(fmt.Sprintf("    iifname %q ip saddr %s ip daddr != @%s %s\n", scope.IfName, vmIP, setName, policy.BlockAction))
			b.WriteString(fmt.Sprintf("    oifname %q ip daddr %s ip saddr != @%s %s\n", scope.IfName, vmIP, setName, policy.BlockAction))
		case "block":
			setName := vmSetName(vmName)
			b.WriteString(fmt.Sprintf("    iifname %q ip saddr %s ip daddr @%s %s\n", scope.IfName, vmIP, setName, policy.BlockAction))
			b.WriteString(fmt.Sprintf("    oifname %q ip daddr %s ip saddr @%s %s\n", scope.IfName, vmIP, setName, policy.BlockAction))
		}
	}

	if policy.OutboundEnabled && len(collectRegionCIDRs(policy.OutboundAllowedRegions, regionMap)) > 0 {
		for _, scope := range scopes {
			b.WriteString(fmt.Sprintf("    iifname %q ip saddr %s ip daddr != @out_allowed4 %s\n", scope.IfName, scope.CIDR, policy.BlockAction))
		}
	}
	if policy.InboundEnabled && len(collectRegionCIDRs(policy.InboundAllowedRegions, regionMap)) > 0 {
		for _, scope := range scopes {
			b.WriteString(fmt.Sprintf("    oifname %q ip daddr %s ip saddr != @in_allowed4 %s\n", scope.IfName, scope.CIDR, policy.BlockAction))
		}
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String(), nil
}

func boundPublicIPv6Addresses() []string {
	if model.DB == nil {
		return []string{}
	}
	var bindings []model.PublicIPBinding
	if err := model.DB.Order("public_ip ASC").Find(&bindings).Error; err != nil {
		return []string{}
	}
	seen := map[string]bool{}
	addresses := make([]string, 0)
	for _, binding := range bindings {
		address, err := netip.ParseAddr(strings.TrimSpace(binding.PublicIP))
		if err != nil || address.Is4() || seen[address.String()] {
			continue
		}
		seen[address.String()] = true
		addresses = append(addresses, address.String())
	}
	sort.Strings(addresses)
	return addresses
}

func firewallNetworkScopes(policy *FirewallPolicy) []firewallNetworkScope {
	scopes := []firewallNetworkScope{{
		IfName: strings.TrimSpace(policy.Bridge),
		CIDR:   strings.TrimSpace(policy.VMSubnet),
	}}
	seen := map[string]bool{scopes[0].IfName + "|" + scopes[0].CIDR: true}
	if model.DB != nil {
		var switches []model.VPCSwitch
		model.DB.Order("id ASC").Find(&switches)
		for _, sw := range switches {
			if !sw.IsSystem && !sw.DHCPEnabled {
				continue
			}
			if strings.TrimSpace(sw.CIDR) == "" {
				continue
			}
			if _, err := netip.ParsePrefix(sw.CIDR); err != nil {
				continue
			}
			scope := firewallNetworkScope{IfName: HookVPCGatewayPortName(sw.ID), CIDR: sw.CIDR}
			key := scope.IfName + "|" + scope.CIDR
			if seen[key] {
				continue
			}
			seen[key] = true
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

func firewallScopeCIDRs(scopes []firewallNetworkScope) []string {
	cidrs := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if strings.TrimSpace(scope.CIDR) != "" {
			cidrs = append(cidrs, scope.CIDR)
		}
	}
	return normalizeCIDRList(cidrs)
}

func firewallScopeForIP(scopes []firewallNetworkScope, ipText string) (firewallNetworkScope, bool) {
	ip, err := netip.ParseAddr(strings.TrimSpace(ipText))
	if err != nil || !ip.Is4() {
		return firewallNetworkScope{}, false
	}
	for _, scope := range scopes {
		prefix, err := netip.ParsePrefix(scope.CIDR)
		if err == nil && prefix.Contains(ip) {
			return scope, true
		}
	}
	return firewallNetworkScope{}, false
}

func defaultWhitelistCIDRsForScopes(scopes []firewallNetworkScope) []string {
	cidrs := append([]string{}, firewallScopeCIDRs(scopes)...)
	cidrs = append(cidrs,
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
	)
	return cidrs
}

func collectRegionCIDRs(codes []string, regions map[string]FirewallRegion) []string {
	var cidrs []string
	for _, code := range codes {
		if region, ok := regions[normalizeRegionCode(code)]; ok {
			cidrs = append(cidrs, region.CIDRs...)
		}
	}
	return normalizeCIDRList(cidrs)
}

func currentPortForwardRulesForPolicy(policy *FirewallPolicy) []PortForwardRule {
	rules, err := HookListLivePortForwardsFromIPTables()
	if err != nil {
		return []PortForwardRule{}
	}
	return rules
}

func GetFirewallVMIP(vmName string) string {
	// 按 VM 实际运行状态解析 IP：关机时走被动路径（仅静态绑定），不触发主动 ARP/nmap 扫描。
	// 启动期 BuildVPCACLRules 会对每个 VPC VM 取防火墙 IP；若一律按 running 处理，关机 VM 会反复 nmap 全网扫描（15s 超时）拖慢启动。
	isRunning := false
	if state, err := libvirt_rpc.GetDomainStateRPC(vmName); err == nil {
		isRunning = strings.EqualFold(state, "running")
	}
	ip := strings.TrimSpace(ip_resolver.GetVMIP(vmName, isRunning))
	if ip == "" || ip == "unknown" {
		return ""
	}
	ip = strings.Fields(ip)[0]
	ip = strings.TrimSuffix(ip, "(静态)")
	if addr, err := netip.ParseAddr(ip); err == nil && addr.Is4() {
		return ip
	}
	return ""
}

func ListAllVMNames() []string {
	result := utils.ExecCommand("virsh", "list", "--all", "--name")
	if result.Error != nil {
		return []string{}
	}
	var names []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ImportFirewallRegionCIDRs 导入本地区域 CIDR。
func ImportFirewallRegionCIDRs(params FirewallImportParams) (*FirewallPolicy, error) {
	policy, err := GetFirewallPolicy()
	if err != nil {
		return nil, err
	}
	code := normalizeRegionCode(params.Code)
	if code == "" {
		return nil, fmt.Errorf("区域代码不能为空")
	}
	cidrs := parseCIDRText(params.CIDRs)
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("未解析到有效 IPv4 CIDR")
	}
	region := FirewallRegion{
		Code:      code,
		Name:      strings.TrimSpace(params.Name),
		CIDRs:     cidrs,
		Source:    strings.TrimSpace(params.Source),
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	if region.Name == "" {
		region.Name = strings.ToUpper(code)
	}
	if region.Source == "" {
		region.Source = "local-import"
	}
	upsertFirewallRegion(policy, region)
	if err := SaveFirewallPolicy(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func upsertFirewallRegion(policy *FirewallPolicy, region FirewallRegion) {
	for i := range policy.Regions {
		if normalizeRegionCode(policy.Regions[i].Code) == region.Code {
			policy.Regions[i] = region
			return
		}
	}
	policy.Regions = append(policy.Regions, region)
	sort.Slice(policy.Regions, func(i, j int) bool {
		return policy.Regions[i].Code < policy.Regions[j].Code
	})
}

func parseCIDRText(text string) []string {
	var values []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(strings.ReplaceAll(line, ",", " "))
		for _, part := range strings.Fields(line) {
			values = append(values, part)
		}
	}
	return normalizeCIDRList(values)
}

// UpdateFirewallGeoIP 下载指定区域的 IPdeny 聚合 CIDR。
func UpdateFirewallGeoIP(ctx context.Context, params FirewallGeoUpdateParams, progress func(int, string)) error {
	policy, err := GetFirewallPolicy()
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(params.BaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(policy.GeoIPBaseURL, "/")
	}
	if baseURL == "" {
		baseURL = defaultGeoBaseURL
	}
	codes := normalizeStringList(params.Codes)
	if len(codes) == 0 {
		return fmt.Errorf("请至少选择一个国家或地区代码")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	for i, code := range codes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if progress != nil {
			progress(10+i*80/len(codes), fmt.Sprintf("正在下载区域 %s 的 CIDR 数据...", strings.ToUpper(code)))
		}
		url := fmt.Sprintf("%s/%s-aggregated.zone", baseURL, code)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("下载 %s 失败: %w", code, err)
		}
		data, err := readAllAndClose(resp)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("下载 %s 失败: HTTP %d", code, resp.StatusCode)
		}
		cidrs := parseCIDRText(string(data))
		if len(cidrs) == 0 {
			return fmt.Errorf("区域 %s 下载结果中没有有效 IPv4 CIDR", code)
		}
		upsertFirewallRegion(policy, FirewallRegion{
			Code:      code,
			Name:      strings.ToUpper(code),
			CIDRs:     cidrs,
			Source:    url,
			UpdatedAt: time.Now().Format(time.RFC3339),
		})
	}
	policy.GeoIPBaseURL = baseURL
	if err := SaveFirewallPolicy(policy); err != nil {
		return err
	}
	if progress != nil {
		progress(100, "GeoIP 区域数据已更新")
	}
	return nil
}

func readAllAndClose(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	var b strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		b.WriteString(scanner.Text())
		b.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取下载内容失败: %w", err)
	}
	return []byte(b.String()), nil
}
