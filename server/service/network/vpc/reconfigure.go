package vpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"kvm_console/model"
	"kvm_console/utils"
)

// VPCSwitchReconfigureParams 是交换机异步重配置任务参数。
type VPCSwitchReconfigureParams struct {
	SwitchID uint             `json:"switch_id"`
	Request  VPCSwitchRequest `json:"request"`
	Operator string           `json:"operator"`
}

var vpcSwitchLocks sync.Map
var vpcTopologyMutationMu sync.Mutex

func switchOperationLock(id uint) *sync.Mutex {
	lock, _ := vpcSwitchLocks.LoadOrStore(id, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// ValidateVPCSwitchReconfigure 在任务入队前执行权限和目标拓扑预检。
func ValidateVPCSwitchReconfigure(operator, role string, id uint, req VPCSwitchRequest) (*model.VPCSwitch, error) {
	if role != "admin" {
		return nil, fmt.Errorf("仅管理员可重配置交换机拓扑")
	}
	var current model.VPCSwitch
	if err := model.DB.First(&current, id).Error; err != nil {
		return nil, fmt.Errorf("交换机不存在")
	}
	if current.IsSystem {
		return nil, fmt.Errorf("系统基础网络交换机不可重配置")
	}
	target, err := buildReconfiguredSwitch(current, req)
	if err != nil {
		return nil, err
	}
	if target.UplinkMode == UplinkModePhysical && HookValidateSwitchUplink != nil {
		if err := HookValidateSwitchUplink(target.UplinkIF, target.UplinkGateway, target.DHCPEnabled, current.ID, target.BridgeName, target.BridgeVLANID); err != nil {
			return nil, err
		}
	}
	if target.OwnsBridge {
		if err := validateHostIPMigrationSelection(req); err != nil {
			return nil, err
		}
	}
	return &target, nil
}

// ParseVPCSwitchReconfigureParams 解析任务队列中的交换机重配置参数。
func ParseVPCSwitchReconfigureParams(raw string) (VPCSwitchReconfigureParams, error) {
	var params VPCSwitchReconfigureParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return params, err
	}
	if params.SwitchID == 0 {
		return params, fmt.Errorf("交换机 ID 为空")
	}
	return params, nil
}

// ExecuteVPCSwitchReconfigure 准备目标运行态并逐网口热切换，失败时按相反顺序恢复。
func ExecuteVPCSwitchReconfigure(ctx context.Context, params VPCSwitchReconfigureParams, progress func(int, string)) (string, error) {
	vpcTopologyMutationMu.Lock()
	defer vpcTopologyMutationMu.Unlock()
	lock := switchOperationLock(params.SwitchID)
	lock.Lock()
	defer lock.Unlock()
	report := func(percent int, message string) {
		if progress != nil {
			progress(percent, message)
		}
	}
	report(5, "正在预检目标交换机拓扑")
	var current model.VPCSwitch
	if err := model.DB.First(&current, params.SwitchID).Error; err != nil {
		return "", fmt.Errorf("交换机不存在")
	}
	if current.IsSystem {
		return "", fmt.Errorf("系统基础网络交换机不可重配置")
	}
	target, err := buildReconfiguredSwitch(current, params.Request)
	if err != nil {
		return "", err
	}
	if target.UplinkMode == UplinkModePhysical && HookValidateSwitchUplink != nil {
		if err := HookValidateSwitchUplink(target.UplinkIF, target.UplinkGateway, target.DHCPEnabled, current.ID, target.BridgeName, target.BridgeVLANID); err != nil {
			return "", err
		}
	}
	if target.OwnsBridge {
		if err := validateHostIPMigrationSelection(params.Request); err != nil {
			return "", err
		}
	}

	report(15, "正在准备目标交换机运行态")
	if err := EnsureVPCSwitchRuntime(target); err != nil {
		if cleanupErr := cleanupPreparedSwitchRuntime(current, target); cleanupErr != nil {
			return "", fmt.Errorf("准备目标交换机运行态失败: %w；清理目标运行态失败: %v", err, cleanupErr)
		}
		return "", fmt.Errorf("准备目标交换机运行态失败: %w", err)
	}
	var bindings []model.VPCVMBinding
	if err := model.DB.Where("switch_id = ?", current.ID).Order("vm_name ASC, interface_order ASC").Find(&bindings).Error; err != nil {
		if cleanupErr := cleanupPreparedSwitchRuntime(current, target); cleanupErr != nil {
			return "", fmt.Errorf("读取交换机网口绑定失败: %w；清理目标运行态失败: %v", err, cleanupErr)
		}
		return "", fmt.Errorf("读取交换机网口绑定失败: %w", err)
	}
	processed := make([]model.VPCVMBinding, 0, len(bindings))
	rollback := func(cause error) (string, error) {
		var rollbackErrors []error
		if SwitchUsesManagedDHCP(current) && SwitchUsesManagedDHCP(target) {
			if err := cleanupSupersededManagedRuntime(target, current); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复旧托管运行态失败: %w", err))
			}
		} else if err := EnsureVPCSwitchRuntime(current); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复旧交换机运行态失败: %w", err))
		}
		for index := len(processed) - 1; index >= 0; index-- {
			binding := processed[index]
			if err := HookReconfigureVMInterfaceNetwork(binding.VMName, binding.InterfaceOrder, current); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复虚拟机 %s 的第 %d 个网口失败: %w", binding.VMName, binding.InterfaceOrder+1, err))
			}
		}
		if !(SwitchUsesManagedDHCP(current) && SwitchUsesManagedDHCP(target)) {
			if err := cleanupPreparedSwitchRuntime(current, target); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("清理目标交换机运行态失败: %w", err))
			}
			if err := EnsureVPCSwitchRuntime(current); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("重新校准旧交换机运行态失败: %w", err))
			}
		}
		if err := model.DB.Save(&current).Error; err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复旧交换机数据库状态失败: %w", err))
		}
		if err := ApplyVPCSwitchBandwidth(current); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复旧交换机带宽失败: %w", err))
		}
		if err := ApplyVPCACLRules(); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复旧交换机 ACL 失败: %w", err))
		}
		if err := reconcileReconfiguredVMPolicies(bindings); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复旧网口端口安全策略失败: %w", err))
		}
		if err := restoreReconfiguredInterfaceLinks(current, processed); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("恢复旧网口链路失败: %w", err))
		}
		if HookTriggerPortSecurityReconcile != nil {
			HookTriggerPortSecurityReconcile()
		}
		if len(rollbackErrors) > 0 {
			return "", fmt.Errorf("%w；回滚期间发生异常: %v", cause, errors.Join(rollbackErrors...))
		}
		return "", cause
	}
	for index, binding := range bindings {
		select {
		case <-ctx.Done():
			return rollback(fmt.Errorf("交换机重配置任务已取消"))
		default:
		}
		if HookReconfigureVMInterfaceNetwork == nil {
			return rollback(fmt.Errorf("虚拟机网口重配置服务尚未初始化"))
		}
		if err := HookReconfigureVMInterfaceNetwork(binding.VMName, binding.InterfaceOrder, target); err != nil {
			return rollback(fmt.Errorf("切换虚拟机 %s 的第 %d 个网口失败: %w", binding.VMName, binding.InterfaceOrder+1, err))
		}
		processed = append(processed, binding)
		report(20+(index+1)*55/maxInt(1, len(bindings)), fmt.Sprintf("已切换 %d/%d 个虚拟机网口", index+1, len(bindings)))
	}

	report(80, "正在提交交换机配置")
	if err := model.DB.Save(&target).Error; err != nil {
		return rollback(fmt.Errorf("保存交换机目标配置失败: %w", err))
	}
	if err := ApplyVPCSwitchBandwidth(target); err != nil {
		return rollback(fmt.Errorf("刷新交换机带宽失败: %w", err))
	}
	if err := ApplyVPCACLRules(); err != nil {
		return rollback(fmt.Errorf("刷新交换机 ACL 失败: %w", err))
	}
	report(88, "正在安装目标网口安全策略")
	if err := reconcileReconfiguredVMPolicies(bindings); err != nil {
		return rollback(fmt.Errorf("安装目标网口端口安全策略失败: %w", err))
	}
	if err := restoreReconfiguredInterfaceLinks(target, bindings); err != nil {
		return rollback(fmt.Errorf("恢复目标网口链路失败: %w", err))
	}
	report(94, "正在清理旧交换机运行态")
	if err := cleanupOldSwitchRuntime(current, target); err != nil {
		return rollback(fmt.Errorf("清理旧交换机运行态失败: %w", err))
	}
	if HookTriggerPortSecurityReconcile != nil {
		HookTriggerPortSecurityReconcile()
	}
	report(100, "交换机重配置完成")
	result, _ := json.Marshal(map[string]any{
		"switch_id": target.ID,
		"mode":      switchRuntimeMode(target),
		"vm_nics":   len(bindings),
	})
	return string(result), nil
}

func buildReconfiguredSwitch(current model.VPCSwitch, req VPCSwitchRequest) (model.VPCSwitch, error) {
	normalizeCreateTopology("admin", &req)
	target := current
	target.DHCPEnabled = req.DHCPEnabled
	target.UplinkMode = normalizeUplinkMode(req.UplinkMode, req.UplinkIF, req.DHCPEnabled)
	target.UplinkIF = strings.TrimSpace(req.UplinkIF)
	target.UplinkGateway = strings.TrimSpace(req.UplinkGateway)
	target.LegacyMigrationRequired = false
	if target.DHCPEnabled {
		if target.UplinkMode != UplinkModePhysical || target.UplinkIF == "" {
			return target, fmt.Errorf("托管 DHCP/NAT 交换机需要选择物理上行链路")
		}
		target.BridgeName = HookOvsBridgeName()
		target.BridgeMode = BridgeModeNAT
		target.OwnsBridge = false
		target.MigrateHostIP = false
		target.BridgeVLANID = 0
		target.AllowPromiscuous = false
		target.AllowMACChange = false
		target.AllowForgedTransmits = false
		target.IPv6SecurityEnabled = false
		target.TrustedIPv6Prefixes = ""
		if strings.TrimSpace(req.CIDR) == "" && strings.TrimSpace(current.CIDR) != "" {
			req.CIDR = current.CIDR
			req.GatewayIP = current.GatewayIP
			req.DHCPStart = current.DHCPStart
			req.DHCPEnd = current.DHCPEnd
		}
		cidr, gateway, start, end, err := resolveVPCSwitchSubnetExcept(true, req, current.ID)
		if err != nil {
			return target, err
		}
		target.CIDR, target.GatewayIP, target.DHCPStart, target.DHCPEnd = cidr, gateway, start, end
		return target, nil
	}

	target.BridgeMode = BridgeModeDirect
	target.MigrateHostIP = target.UplinkMode == UplinkModePhysical && req.MigrateHostIP
	target.BridgeVLANID = normalizedBridgeVLANID(BridgeModeDirect, req.BridgeVLANID)
	if err := validateBridgeVLANID(BridgeModeDirect, target.BridgeVLANID); err != nil {
		return target, err
	}
	target.AllowPromiscuous = target.UplinkMode == UplinkModePhysical && req.AllowPromiscuous
	target.AllowMACChange = target.UplinkMode == UplinkModePhysical && req.AllowMACChange
	target.AllowForgedTransmits = target.UplinkMode == UplinkModePhysical && req.AllowForgedTx
	target.IPv6SecurityEnabled = target.UplinkMode == UplinkModePhysical && req.IPv6SecurityEnabled
	target.TrustedIPv6Prefixes = strings.TrimSpace(req.TrustedIPv6Prefixes)
	if target.UplinkMode != UplinkModePhysical {
		target.OwnsBridge = true
		target.BridgeName = nextOwnedBridgeName(current, target.UplinkIF)
		target.UplinkMode = UplinkModeNone
		target.UplinkIF = ""
		target.UplinkGateway = ""
		target.MigrateHostIP = false
		target.BridgeVLANID = 0
		target.AllowPromiscuous = false
		target.AllowMACChange = false
		target.AllowForgedTransmits = false
		target.IPv6SecurityEnabled = false
		target.TrustedIPv6Prefixes = ""
	} else {
		shared, err := findSharedDirectSwitch(target.UplinkIF, target.BridgeVLANID, current.ID)
		if err != nil {
			return target, err
		}
		if shared != nil {
			if current.OwnsBridge && strings.EqualFold(strings.TrimSpace(current.UplinkIF), target.UplinkIF) &&
				strings.EqualFold(HookBridgeNameForSwitch(current), HookBridgeNameForSwitch(*shared)) {
				target.OwnsBridge = true
				target.BridgeName = HookBridgeNameForSwitch(current)
			} else {
				target.OwnsBridge = false
				target.BridgeName = HookBridgeNameForSwitch(*shared)
				target.MigrateHostIP = false
			}
		} else {
			target.OwnsBridge = true
			target.BridgeName = nextOwnedBridgeName(current, target.UplinkIF)
		}
	}
	if target.MigrateHostIP && HookCaptureHostIPConfig != nil {
		target.HostAddrs, target.HostGateway, target.HostMetric, target.HostDNS = HookCaptureHostIPConfig(target.UplinkIF)
	} else {
		target.HostAddrs, target.HostGateway, target.HostMetric, target.HostDNS = "", "", "", ""
	}
	// 关闭 DHCP 时保留最近一次托管网段，方便后续重新启用时复用。
	return target, nil
}

func nextOwnedBridgeName(current model.VPCSwitch, targetUplink string) string {
	base := managedVPCBridgeName(current.VLANID)
	if current.OwnsBridge && strings.EqualFold(strings.TrimSpace(current.UplinkIF), strings.TrimSpace(targetUplink)) {
		return strings.TrimSpace(current.BridgeName)
	}
	if current.OwnsBridge && strings.EqualFold(strings.TrimSpace(current.BridgeName), base) {
		return base + "b"
	}
	return base
}

func cleanupPreparedSwitchRuntime(current, target model.VPCSwitch) error {
	if SwitchUsesManagedDHCP(current) && SwitchUsesManagedDHCP(target) {
		return cleanupSupersededManagedRuntime(target, current)
	}
	if current.OwnsBridge && target.OwnsBridge && strings.EqualFold(current.BridgeName, target.BridgeName) {
		return EnsureVPCSwitchRuntime(current)
	}
	return removeVPCSwitchRuntime(target)
}

func cleanupOldSwitchRuntime(current, target model.VPCSwitch) error {
	if SwitchUsesManagedDHCP(current) && SwitchUsesManagedDHCP(target) {
		return cleanupSupersededManagedRuntime(current, target)
	}
	if current.OwnsBridge && target.OwnsBridge && strings.EqualFold(current.BridgeName, target.BridgeName) {
		return nil
	}
	if err := removeVPCSwitchRuntime(current); err != nil {
		return err
	}
	if SwitchUsesManagedDHCP(target) {
		return EnsureVPCSwitchRuntime(target)
	}
	return nil
}

func switchRuntimeMode(sw model.VPCSwitch) string {
	if SwitchUsesManagedDHCP(sw) {
		return "managed"
	}
	if strings.TrimSpace(sw.UplinkIF) != "" {
		return "physical"
	}
	return "isolated"
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func reconcileReconfiguredVMPolicies(bindings []model.VPCVMBinding) error {
	if HookIsPortSecurityEnabled == nil || !HookIsPortSecurityEnabled() || HookReconcileVMPortSecurity == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		vmName := strings.TrimSpace(binding.VMName)
		if vmName == "" {
			continue
		}
		if _, exists := seen[vmName]; exists {
			continue
		}
		if err := HookReconcileVMPortSecurity(vmName); err != nil {
			return fmt.Errorf("虚拟机 %s: %w", vmName, err)
		}
		seen[vmName] = struct{}{}
	}
	return nil
}

func restoreReconfiguredInterfaceLinks(sw model.VPCSwitch, bindings []model.VPCVMBinding) error {
	if SwitchIsTrustedIsolated(sw) || HookIsPortSecurityEnabled == nil || !HookIsPortSecurityEnabled() {
		return nil
	}
	if HookGetVMMACByOrder == nil {
		return fmt.Errorf("虚拟机网口查询服务尚未初始化")
	}
	for _, binding := range bindings {
		vmName := strings.TrimSpace(binding.VMName)
		mac := strings.TrimSpace(HookGetVMMACByOrder(vmName, binding.InterfaceOrder))
		if mac == "" {
			return fmt.Errorf("读取虚拟机 %s 的第 %d 个网口 MAC 失败", vmName, binding.InterfaceOrder+1)
		}
		state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
		if state == "running" {
			vnetIF := getVMVnetIFByMAC(vmName, mac)
			if vnetIF == "" {
				return fmt.Errorf("读取虚拟机 %s 的第 %d 个运行态网口失败", vmName, binding.InterfaceOrder+1)
			}
			if err := setVMInterfaceLink(vmName, vnetIF, "up", false); err != nil {
				return fmt.Errorf("放行虚拟机 %s 的第 %d 个运行态网口失败: %w", vmName, binding.InterfaceOrder+1, err)
			}
		}
		if err := setVMInterfaceLink(vmName, mac, "up", true); err != nil {
			return fmt.Errorf("持久化虚拟机 %s 的第 %d 个网口链路失败: %w", vmName, binding.InterfaceOrder+1, err)
		}
	}
	return nil
}
