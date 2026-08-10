package vpc

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"kvm_console/config"
	"kvm_console/model"
)

func normalizeAddressField(value string, ipv6 bool) (string, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	seen := map[string]bool{}
	var result []string
	for _, field := range fields {
		addr, err := netip.ParseAddr(strings.TrimSpace(field))
		if err != nil || addr.Is4() == ipv6 {
			if ipv6 {
				return "", fmt.Errorf("IPv6 地址格式无效: %s", field)
			}
			return "", fmt.Errorf("IPv4 地址格式无效: %s", field)
		}
		value := addr.String()
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return strings.Join(result, "\n"), nil
}

// UpdateVMInterfaceAllowedAddresses 更新指定网卡的端口安全可信地址清单。
func UpdateVMInterfaceAllowedAddresses(vmName string, interfaceOrder int, allowedIPv4, allowedIPv6 string) error {
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", strings.TrimSpace(vmName), interfaceOrder).First(&binding).Error; err != nil {
		return fmt.Errorf("未找到指定的网口绑定")
	}
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, binding.SwitchID).Error; err != nil {
		return fmt.Errorf("交换机不存在")
	}
	req := AddVMInterfaceRequest{AllowedIPv4Addresses: allowedIPv4, AllowedIPv6Addresses: allowedIPv6}
	if err := normalizeInterfacePortSecurityFields(&req, HookSwitchUsesDirectBridge(sw) && sw.IPv6SecurityEnabled); err != nil {
		return err
	}
	binding.AllowedIPv4Addresses = req.AllowedIPv4Addresses
	binding.AllowedIPv6Addresses = req.AllowedIPv6Addresses
	if err := model.DB.Save(&binding).Error; err != nil {
		return fmt.Errorf("保存网卡可信地址失败: %w", err)
	}
	if HookTriggerPortSecurityReconcile != nil {
		HookTriggerPortSecurityReconcile()
	}
	return nil
}

func normalizeIPv6PrefixField(value string) (string, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	seen := map[string]bool{}
	var result []string
	for _, field := range fields {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(field))
		if err != nil || prefix.Addr().Is4() {
			return "", fmt.Errorf("IPv6 前缀格式无效: %s", field)
		}
		value := prefix.Masked().String()
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return strings.Join(result, "\n"), nil
}

func normalizeSwitchPortSecurityFields(req *VPCSwitchRequest, direct bool) error {
	if req == nil {
		return nil
	}
	if !direct && req.IPv6SecurityEnabled {
		return fmt.Errorf("IPv6 端口安全仅适用于直通桥交换机")
	}
	prefixes, err := normalizeIPv6PrefixField(req.TrustedIPv6Prefixes)
	if err != nil {
		return err
	}
	req.TrustedIPv6Prefixes = prefixes
	if req.IPv6SecurityEnabled && config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled && prefixes == "" {
		return fmt.Errorf("启用直通桥 IPv6 防护时需要填写可信 IPv6 前缀")
	}
	if !direct {
		req.IPv6SecurityEnabled = false
		req.TrustedIPv6Prefixes = ""
	}
	return nil
}

func normalizeInterfacePortSecurityFields(req *AddVMInterfaceRequest, ipv6Required bool) error {
	if req == nil {
		return nil
	}
	v4, err := normalizeAddressField(req.AllowedIPv4Addresses, false)
	if err != nil {
		return err
	}
	v6, err := normalizeAddressField(req.AllowedIPv6Addresses, true)
	if err != nil {
		return err
	}
	if ipv6Required && config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled && v6 == "" {
		return fmt.Errorf("该直通桥已启用 IPv6 防护，请填写网卡可信 IPv6 地址")
	}
	req.AllowedIPv4Addresses = v4
	req.AllowedIPv6Addresses = v6
	return nil
}
