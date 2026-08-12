package vpc

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kvm_console/config"
	"kvm_console/model"
	"kvm_console/utils"
)

func BuildVPCACLRules() (string, error) {
	table := config.GlobalConfig.VPCACLTable
	if strings.TrimSpace(table) == "" {
		table = "kvm_console_vpc_acl"
	}
	var bindings []model.VPCVMBinding
	model.DB.Find(&bindings)
	var b strings.Builder
	b.WriteString("table inet ")
	b.WriteString(table)
	b.WriteString(" {\n")
	b.WriteString("  chain forward {\n")
	b.WriteString("    type filter hook forward priority -40; policy accept;\n")
	var vmAddresses []string
	for _, binding := range bindings {
		var sw model.VPCSwitch
		if err := model.DB.First(&sw, binding.SwitchID).Error; err == nil && HookSwitchUsesDirectBridge(sw) {
			continue
		}
		bindingAddresses := vpcFirewallAddressesForVM(binding.VMName)
		if len(bindingAddresses) == 0 {
			continue
		}
		for _, vmAddress := range bindingAddresses {
			allows, err := buildVPCIngressAllowRules(binding, vmAddress)
			if err != nil {
				return "", err
			}
			for _, line := range allows {
				b.WriteString(line)
			}
			// DNAT 仅适用于 IPv4；路由型公网 IPv6 使用普通目的地址规则。
			if addressFamilyExpression(vmAddress) == "ip" {
				b.WriteString(fmt.Sprintf("    ct status dnat ip daddr %s reject\n", vmAddress))
			}
			vmAddresses = append(vmAddresses, vmAddress)
		}
	}
	b.WriteString("    ct state established,related accept\n")
	sort.Strings(vmAddresses)
	for _, vmAddress := range vmAddresses {
		b.WriteString(fmt.Sprintf("    %s daddr %s reject\n", addressFamilyExpression(vmAddress), vmAddress))
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String(), nil
}

func vpcFirewallAddressesForVM(vmName string) []string {
	candidates := vpcFirewallIPsForVM(vmName)
	if model.DB != nil {
		var bindings []model.PublicIPBinding
		model.DB.Where("vm_name = ?", vmName).Find(&bindings)
		for _, binding := range bindings {
			if address, err := netip.ParseAddr(strings.TrimSpace(binding.PublicIP)); err == nil && address.Is6() {
				candidates = append(candidates, address.String())
			}
		}
	}
	seen := map[string]bool{}
	addresses := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		address, err := netip.ParseAddr(strings.TrimSpace(candidate))
		if err != nil || seen[address.String()] {
			continue
		}
		seen[address.String()] = true
		addresses = append(addresses, address.String())
	}
	sort.Strings(addresses)
	return addresses
}

func addressFamilyExpression(value string) string {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err == nil && address.Is6() {
		return "ip6"
	}
	return "ip"
}

func vpcFirewallIPsForVM(vmName string) []string {
	candidates := []string{HookGetFirewallVMIP(vmName)}
	candidates = append(candidates, HookPublicIPNATPrivateIPsForVM(vmName)...)
	seen := map[string]bool{}
	var ips []string
	for _, candidate := range candidates {
		ip := normalizeFirewallIPv4(candidate)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}

func normalizeFirewallIPv4(ipText string) string {
	ipText = strings.TrimSpace(ipText)
	if ipText == "" || ipText == "unknown" {
		return ""
	}
	ipText = strings.Fields(ipText)[0]
	ipText = strings.TrimSuffix(ipText, "(静态)")
	if addr, err := netip.ParseAddr(ipText); err == nil && addr.Is4() {
		return ipText
	}
	return ""
}

func buildVPCIngressAllowRules(binding model.VPCVMBinding, vmIP string) ([]string, error) {
	var rules []model.VPCSecurityGroupRule
	model.DB.Where("security_group_id = ? AND direction = ?", binding.SecurityGroupID, "ingress").Find(&rules)
	var lines []string
	for _, rule := range rules {
		family := addressFamilyExpression(vmIP)
		ruleFamily := "ipv4"
		if family == "ip6" {
			ruleFamily = "ipv6"
		}
		if effectiveSecurityGroupRuleAddressFamily(rule) != ruleFamily {
			continue
		}
		sources, err := resolveRuleSources(rule)
		if err != nil {
			return nil, err
		}
		for _, src := range sources {
			if !sameAddressFamily(vmIP, src) {
				continue
			}
			match := fmt.Sprintf("    %s daddr %s %s saddr %s", family, vmIP, family, src)
			switch rule.Protocol {
			case "tcp", "udp":
				portMatch := strconv.Itoa(rule.PortStart)
				if rule.PortEnd > rule.PortStart {
					portMatch = fmt.Sprintf("%d-%d", rule.PortStart, rule.PortEnd)
				}
				match += fmt.Sprintf(" %s dport %s accept\n", rule.Protocol, portMatch)
			case "icmp":
				if family == "ip6" {
					// 兼容 address_family 字段加入前已保存的 IPv6 ICMP 规则。
					match += " meta l4proto ipv6-icmp accept\n"
				} else {
					match += " icmp type echo-request accept\n"
				}
			case "icmpv6":
				match += " meta l4proto ipv6-icmp accept\n"
			default:
				match += " accept\n"
			}
			lines = append(lines, match)
		}
	}
	sort.Strings(lines)
	return lines, nil
}

func sameAddressFamily(addressText, prefixText string) bool {
	address, err := netip.ParseAddr(strings.TrimSpace(addressText))
	if err != nil {
		return false
	}
	prefix, err := netip.ParsePrefix(normalizeCIDROrIP(prefixText))
	return err == nil && address.Is4() == prefix.Addr().Is4()
}

func resolveRuleSources(rule model.VPCSecurityGroupRule) ([]string, error) {
	switch rule.TargetType {
	case "cidr":
		return []string{normalizeCIDROrIP(rule.TargetValue)}, nil
	case "switch":
		id, _ := strconv.Atoi(rule.TargetValue)
		var sw model.VPCSwitch
		if err := model.DB.First(&sw, id).Error; err != nil {
			return nil, fmt.Errorf("安全组规则引用的交换机不存在")
		}
		if !sw.IsSystem && !sw.DHCPEnabled {
			return nil, nil
		}
		sources := []string{sw.CIDR}
		var bindings []model.VPCVMBinding
		model.DB.Where("switch_id = ?", id).Find(&bindings)
		for _, binding := range bindings {
			for _, address := range vpcFirewallAddressesForVM(binding.VMName) {
				sources = append(sources, addressWithHostPrefix(address))
			}
		}
		return uniqueSortedStrings(sources), nil
	case "security_group":
		id, _ := strconv.Atoi(rule.TargetValue)
		var bindings []model.VPCVMBinding
		model.DB.Where("security_group_id = ?", id).Find(&bindings)
		var sources []string
		for _, binding := range bindings {
			for _, address := range vpcFirewallAddressesForVM(binding.VMName) {
				sources = append(sources, addressWithHostPrefix(address))
			}
		}
		return uniqueSortedStrings(sources), nil
	default:
		return nil, fmt.Errorf("安全组规则目标类型无效")
	}
}

func addressWithHostPrefix(value string) string {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	if address.Is6() {
		return address.String() + "/128"
	}
	return address.String() + "/32"
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

func PreviewVPCACLRules() (string, error) {
	return BuildVPCACLRules()
}

func ApplyVPCACLRules() error {
	rules, err := BuildVPCACLRules()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(VPCConfigDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(VPCConfigDir, "acl.nft")
	if err := os.WriteFile(path, []byte(rules), 0644); err != nil {
		return fmt.Errorf("写入 VPC ACL 规则失败: %w", err)
	}
	check := utils.ExecCommand("nft", "-c", "-f", path)
	if check.Error != nil {
		return fmt.Errorf("VPC ACL 规则校验失败: %s", check.Stderr)
	}
	table := config.GlobalConfig.VPCACLTable
	if table == "" {
		table = "kvm_console_vpc_acl"
	}
	result := utils.ExecShell(fmt.Sprintf("nft delete table inet %s 2>/dev/null || true; nft -f %s", utils.ShellSingleQuote(table), utils.ShellSingleQuote(path)))
	if result.Error != nil {
		return fmt.Errorf("应用 VPC ACL 失败: %s", result.Stderr)
	}
	HookRemoveVPCPortForwardAcceptRules()
	_ = HookSavePortForwardRules()
	return nil
}
