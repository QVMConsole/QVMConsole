package public_ip

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	netpkg "kvm_console/service/network"

	"kvm_console/model"
)

func PreviewPublicIPBinding(id uint, req PublicIPBindRequest) (*PublicIPPreview, error) {
	ipRow, err := getPublicIP(id)
	if err != nil {
		return nil, err
	}
	bindReq, warnings, err := normalizePublicIPBindRequest(*ipRow, req, false)
	if err != nil {
		return nil, err
	}
	commands, err := buildPublicIPCommands(*ipRow, bindReq)
	if err != nil {
		return nil, err
	}
	return &PublicIPPreview{
		PublicIP:   *ipRow,
		Binding:    bindReq,
		Commands:   commands,
		ConfigHint: buildPublicIPConfigHint(*ipRow, bindReq),
		Warnings:   warnings,
	}, nil
}

func ExecutePublicIPOperation(ctx context.Context, params PublicIPOperationParams, progress func(int, string)) (string, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	action := strings.ToLower(strings.TrimSpace(params.Action))
	affectedVMs := publicIPOperationAffectedVMs(params)
	progress(10, "正在校验公网 IP 操作...")

	var result interface{}
	var err error
	switch action {
	case "bind":
		result, err = bindPublicIP(params.PublicIPID, params.BindRequest)
	case "unbind":
		result, err = unbindPublicIP(params.PublicIPID)
	case "migrate":
		req := params.BindRequest
		if strings.TrimSpace(params.TargetVM) != "" {
			req.VMName = params.TargetVM
		}
		if strings.TrimSpace(params.TargetUser) != "" {
			req.Username = params.TargetUser
		}
		result, err = migratePublicIP(params.PublicIPID, req)
	case "apply_all":
		result = map[string]string{"action": "apply_all"}
	default:
		err = fmt.Errorf("不支持的公网 IP 操作: %s", action)
	}
	if err != nil {
		return "", err
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if publicIPHasVPCBindings() {
		progress(45, "正在同步 VPC 安全组规则...")
		if err := HookApplyVPCACLRules(); err != nil {
			markPublicIPBindingsRuntimeFailed(err.Error())
			return "", err
		}
	}
	progress(55, "正在写入并应用公网 IP 运行规则...")
	if err := ApplyPublicIPRules(); err != nil {
		markPublicIPBindingsRuntimeFailed(err.Error())
		return "", err
	}
	markPublicIPBindingsApplied()
	progress(90, "正在同步来宾系统 IPv6 配置...")
	reconcilePublicIPv6GuestVMs(ctx, affectedVMs, true)
	progress(100, "公网 IP 规则已应用")
	data, _ := json.Marshal(result)
	return string(data), nil
}

func bindPublicIP(id uint, req PublicIPBindRequest) (*model.PublicIPBinding, error) {
	ipRow, err := getPublicIP(id)
	if err != nil {
		return nil, err
	}
	if ipRow.Status == PublicIPStatusBound {
		return nil, fmt.Errorf("公网 IP 已绑定，请使用迁移操作")
	}
	if ipRow.Status == "reserved" {
		return nil, fmt.Errorf("公网 IP 当前为保留状态，不能绑定")
	}
	req, _, err = normalizePublicIPBindRequest(*ipRow, req, true)
	if err != nil {
		return nil, err
	}
	if err := checkPublicIPQuota(req.Username, 1); err != nil {
		return nil, err
	}
	now := time.Now()
	binding := &model.PublicIPBinding{
		PublicIPID:      ipRow.ID,
		PublicIP:        ipRow.IP,
		Username:        req.Username,
		VMName:          req.VMName,
		VMPrivateIP:     req.VMPrivateIP,
		Mode:            NormalizePublicIPMode(req.Mode),
		RuntimeStatus:   "pending",
		GuestIPv6Status: publicIPv6GuestInitialStatus(*ipRow, req),
		ConfigHint:      buildPublicIPConfigHint(*ipRow, req),
		LastAppliedAt:   &now,
	}
	if err := model.DB.Create(binding).Error; err != nil {
		return nil, fmt.Errorf("保存公网 IP 绑定失败: %w", err)
	}
	model.DB.Model(ipRow).Updates(map[string]interface{}{"status": PublicIPStatusBound})
	return binding, nil
}

func unbindPublicIP(id uint) (map[string]string, error) {
	ipRow, err := getPublicIP(id)
	if err != nil {
		return nil, err
	}
	if err := model.DB.Where("public_ip_id = ?", id).Delete(&model.PublicIPBinding{}).Error; err != nil {
		return nil, fmt.Errorf("删除公网 IP 绑定失败: %w", err)
	}
	model.DB.Model(ipRow).Updates(map[string]interface{}{"status": PublicIPStatusFree})
	cleanupConntrackForPublicIP(ipRow.IP)
	return map[string]string{"public_ip": ipRow.IP, "action": "unbind"}, nil
}

func migratePublicIP(id uint, req PublicIPBindRequest) (*model.PublicIPBinding, error) {
	ipRow, err := getPublicIP(id)
	if err != nil {
		return nil, err
	}
	var binding model.PublicIPBinding
	if err := model.DB.Where("public_ip_id = ?", id).First(&binding).Error; err != nil {
		return nil, fmt.Errorf("公网 IP 尚未绑定，不能迁移")
	}
	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = binding.Mode
	}
	req, _, err = normalizePublicIPBindRequest(*ipRow, req, true)
	if err != nil {
		return nil, err
	}
	if req.Username != binding.Username {
		if err := checkPublicIPQuota(req.Username, 1); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	if err := model.DB.Model(&binding).Updates(map[string]interface{}{
		"username":           req.Username,
		"vm_name":            req.VMName,
		"vm_private_ip":      req.VMPrivateIP,
		"mode":               NormalizePublicIPMode(req.Mode),
		"runtime_status":     "pending",
		"guest_ipv6_status":  publicIPv6GuestInitialStatus(*ipRow, req),
		"guest_ipv6_message": "",
		"config_hint":        buildPublicIPConfigHint(*ipRow, req),
		"last_applied_at":    &now,
	}).Error; err != nil {
		return nil, fmt.Errorf("迁移公网 IP 失败: %w", err)
	}
	if err := model.DB.First(&binding, binding.ID).Error; err != nil {
		return nil, err
	}
	cleanupConntrackForPublicIP(ipRow.IP)
	return &binding, nil
}

func publicIPOperationAffectedVMs(params PublicIPOperationParams) []string {
	seen := map[string]bool{}
	var result []string
	appendVM := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}
	if model.DB != nil {
		if strings.EqualFold(strings.TrimSpace(params.Action), "apply_all") {
			var bindings []model.PublicIPBinding
			model.DB.Find(&bindings)
			for _, binding := range bindings {
				appendVM(binding.VMName)
			}
		} else if params.PublicIPID > 0 {
			var binding model.PublicIPBinding
			if err := model.DB.Where("public_ip_id = ?", params.PublicIPID).First(&binding).Error; err == nil {
				appendVM(binding.VMName)
			}
		}
	}
	appendVM(params.BindRequest.VMName)
	appendVM(params.TargetVM)
	return result
}

func publicIPv6GuestInitialStatus(ipRow model.PublicIP, req PublicIPBindRequest) string {
	if publicIPIsIPv6(ipRow.IP) && NormalizePublicIPMode(req.Mode) == PublicIPModeClassicRoute {
		return "pending"
	}
	return "not_applicable"
}

func normalizePublicIPBindRequest(ipRow model.PublicIP, req PublicIPBindRequest, allowMutate bool) (PublicIPBindRequest, []string, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.VMName = strings.TrimSpace(req.VMName)
	req.VMPrivateIP = strings.TrimSpace(req.VMPrivateIP)
	req.Mode = NormalizePublicIPMode(req.Mode)
	if req.VMName == "" {
		return req, nil, fmt.Errorf("请选择虚拟机")
	}
	if req.Username == "" {
		req.Username = HookFindVMOwner(req.VMName)
	}
	if req.Username == "" {
		return req, nil, fmt.Errorf("无法识别虚拟机归属用户，请手动选择用户")
	}
	if !publicIPModeAllowed(ipRow, req.Mode) {
		return req, nil, fmt.Errorf("公网 IP 不支持 %s 模式", PublicIPModeLabel(req.Mode))
	}
	var warnings []string
	if publicIPIsIPv6(ipRow.IP) {
		if req.Mode == PublicIPModeNAT {
			return req, nil, fmt.Errorf("IPv6 公网地址使用路由模式，不使用 1:1 NAT")
		}
		req.VMPrivateIP = ""
		if req.Mode == PublicIPModeClassicRoute && strings.TrimSpace(ipRow.UplinkIF) == "" && detectDefaultIPv6Uplink() == "" {
			return req, nil, fmt.Errorf("IPv6 路由模式需要指定出口网卡或存在 IPv6 默认路由")
		}
		warnings = append(warnings, "IPv6 路由模式通过 Proxy NDP 将公网 /128 路由到 VM；运行中的 Linux 来宾会通过 QEMU Guest Agent 自动配置并持久化")
		if !publicIPv6IngressRuleConfigured(req.VMName) {
			warnings = append(warnings, "当前 VPC 安全组没有 IPv6 CIDR 入站规则；外部访问会被拒绝，请按需添加 ::/0 或更小范围的 IPv6 来源")
		}
		return req, warnings, nil
	}
	if req.Mode == PublicIPModeNAT {
		if req.VMPrivateIP == "" {
			if allowMutate {
				ip, err := netpkg.EnsureStaticIP(req.VMName)
				if err != nil {
					return req, nil, err
				}
				req.VMPrivateIP = ip
			} else if ip := ResolvePublicIPVMPrivateIP(req.VMName); ip != "" {
				req.VMPrivateIP = ip
			}
		}
		if req.VMPrivateIP == "" {
			return req, nil, fmt.Errorf("1:1 NAT 模式需要 VM 私网 IP")
		}
		if net.ParseIP(req.VMPrivateIP) == nil {
			return req, nil, fmt.Errorf("VM 私网 IP 格式无效")
		}
	} else {
		if req.VMPrivateIP == "" {
			req.VMPrivateIP = ResolvePublicIPVMPrivateIP(req.VMName)
		}
		warnings = append(warnings, "经典网络需要上游网络支持，并由用户在 VM 内手动配置公网 IP")
	}
	return req, warnings, nil
}

func checkPublicIPQuota(username string, delta int) error {
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在")
	}
	if user.Role == "admin" || user.MaxPublicIPs <= 0 {
		return nil
	}
	var count int64
	model.DB.Model(&model.PublicIPBinding{}).Where("username = ?", username).Count(&count)
	if int(count)+delta > user.MaxPublicIPs {
		return fmt.Errorf("公网 IP 配额不足（已用 %d / 上限 %d）", count, user.MaxPublicIPs)
	}
	return nil
}

func buildPublicIPConfigHint(ipRow model.PublicIP, req PublicIPBindRequest) string {
	mode := NormalizePublicIPMode(req.Mode)
	prefix := publicIPPrefix(ipRow)
	gateway := strings.TrimSpace(ipRow.Gateway)
	switch mode {
	case PublicIPModeNAT:
		return fmt.Sprintf("VM 内保持私网 IP %s，无需配置公网 IP。公网 %s 会通过 1:1 NAT 映射到该 VM。", req.VMPrivateIP, ipRow.IP)
	case PublicIPModeClassicRoute:
		if publicIPIsIPv6(ipRow.IP) {
			gateway := publicIPv6GatewayLinkLocal(req.VMName)
			if gateway == "" {
				gateway = "HOST_LINK_LOCAL"
			}
			return fmt.Sprintf("IPv6 路由：Linux VM 运行且 QEMU Guest Agent 就绪时，面板会自动配置并持久化 %s/128，默认网关为 %s；其他情况按此参数手动配置。宿主机将在 %s 上执行 Proxy NDP。VPC 安全组还需配置 IPv6 入站来源。", ipRow.IP, gateway, firstNonEmpty(strings.TrimSpace(ipRow.UplinkIF), detectDefaultIPv6Uplink()))
		}
		if gateway == "" {
			gateway = HookOvsGatewayIP()
		}
		return fmt.Sprintf("经典网络-路由：请在 VM 内配置 IP %s/%d，默认网关 %s。上游需要把该公网 IP 或公网段路由到宿主机。", ipRow.IP, prefix, gateway)
	case PublicIPModeClassicBridge:
		if publicIPIsIPv6(ipRow.IP) {
			return fmt.Sprintf("经典网络-桥接：请在 VM 内配置 IPv6 %s/%d；默认网关使用上游网络提供的 IPv6 网关或 RA。", ipRow.IP, prefix)
		}
		return fmt.Sprintf("经典网络-桥接：请在 VM 内配置 IP %s/%d，默认网关 %s。上游交换机需要允许 VM MAC 使用该公网 IP。", ipRow.IP, prefix, gateway)
	default:
		return ""
	}
}

func publicIPv6IngressRuleConfigured(vmName string) bool {
	if model.DB == nil || strings.TrimSpace(vmName) == "" {
		return false
	}
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", strings.TrimSpace(vmName), 0).First(&binding).Error; err != nil {
		return false
	}
	var rules []model.VPCSecurityGroupRule
	model.DB.Where("security_group_id = ? AND direction = ? AND target_type = ?", binding.SecurityGroupID, "ingress", "cidr").Find(&rules)
	for _, rule := range rules {
		value := strings.TrimSpace(rule.TargetValue)
		if prefix, err := netip.ParsePrefix(value); err == nil && prefix.Addr().Is6() {
			return true
		}
		if address, err := netip.ParseAddr(value); err == nil && address.Is6() {
			return true
		}
	}
	return false
}

func markPublicIPBindingsApplied() {
	now := time.Now()
	model.DB.Model(&model.PublicIPBinding{}).Where("1 = 1").Updates(map[string]interface{}{
		"runtime_status":  "applied",
		"last_applied_at": &now,
	})
}

func markPublicIPBindingsRuntimeFailed(message string) {
	model.DB.Model(&model.PublicIPBinding{}).Where("1 = 1").Update("runtime_status", "failed: "+message)
}

func ListPublicIPAttachmentsForVM(vmName string) []PublicIPAttachment {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" || model.DB == nil {
		return []PublicIPAttachment{}
	}
	var bindings []model.PublicIPBinding
	if err := model.DB.Where("vm_name = ?", vmName).Order("public_ip ASC").Find(&bindings).Error; err != nil {
		return []PublicIPAttachment{}
	}
	out := make([]PublicIPAttachment, 0, len(bindings))
	for _, binding := range bindings {
		mode := NormalizePublicIPMode(binding.Mode)
		out = append(out, PublicIPAttachment{
			PublicIP:      binding.PublicIP,
			Mode:          mode,
			ModeLabel:     PublicIPModeLabel(mode),
			VMPrivateIP:   binding.VMPrivateIP,
			RuntimeStatus: binding.RuntimeStatus,
		})
	}
	return out
}

func publicIPHasVPCBindings() bool {
	if model.DB == nil {
		return false
	}
	var count int64
	model.DB.Model(&model.VPCVMBinding{}).Count(&count)
	return count > 0
}
