package vpc

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/utils"
)

// AddVMInterface 为虚拟机新增一个网口并绑定到 VPC 交换机（管理员原语；普通用户请使用 AddVMInterfaceAsUser）
func AddVMInterface(vmName string, req AddVMInterfaceRequest) (*VMInterfaceInfo, error) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return nil, fmt.Errorf("虚拟机名称不能为空")
	}

	// 验证交换机存在
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, req.SwitchID).Error; err != nil {
		return nil, fmt.Errorf("交换机不存在")
	}
	if err := normalizeInterfacePortSecurityFields(&req, HookSwitchUsesDirectBridge(sw) && sw.IPv6SecurityEnabled); err != nil {
		return nil, err
	}
	if SwitchIsTrustedIsolated(sw) {
		req.AllowedIPv4Addresses = ""
		req.AllowedIPv6Addresses = ""
	}

	// 系统交换机使用 VM 归属用户的默认安全组
	switchOwner := sw.Username
	if sw.IsSystem {
		switchOwner = HookFindVMOwner(vmName)
		if switchOwner == "" {
			// 回退：从已有 VPC 绑定记录中获取用户名
			var binding model.VPCVMBinding
			if err := model.DB.Where("vm_name = ?", vmName).First(&binding).Error; err == nil && binding.Username != "" {
				switchOwner = binding.Username
			}
		}
		if switchOwner == "" && req.SecurityGroupID > 0 {
			// 回退：从指定安全组获取用户名
			var sg model.VPCSecurityGroup
			if err := model.DB.First(&sg, req.SecurityGroupID).Error; err == nil && sg.Username != "" {
				switchOwner = sg.Username
			}
		}
		if switchOwner == "" {
			// 回退：从 VM 缓存表获取归属用户
			var cache model.VMCache
			if err := model.DB.Where("name = ?", vmName).First(&cache).Error; err == nil && cache.OwnerUsername != "" {
				switchOwner = cache.OwnerUsername
			}
		}
		if switchOwner == "" {
			return nil, fmt.Errorf("无法识别虚拟机归属用户")
		}
	}

	// 安全组处理
	securityGroupID := req.SecurityGroupID
	if HookSwitchUsesDirectBridge(sw) {
		securityGroupID = 0
	} else {
		if securityGroupID == 0 {
			if _, err := EnsureDefaultSecurityGroup(switchOwner); err != nil {
				return nil, err
			}
			var group model.VPCSecurityGroup
			if err := model.DB.Where("username = ? AND is_default = ?", switchOwner, true).First(&group).Error; err != nil {
				return nil, fmt.Errorf("未找到用户 %s 的默认安全组", switchOwner)
			}
			securityGroupID = group.ID
		} else {
			var group model.VPCSecurityGroup
			if err := model.DB.First(&group, securityGroupID).Error; err != nil {
				return nil, fmt.Errorf("安全组不存在")
			}
			if !sw.IsSystem && group.Username != sw.Username && !isLightweightDedicatedVPCSecurityGroup(group, vmName, sw.ID) {
				return nil, fmt.Errorf("安全组必须属于交换机用户 %s", sw.Username)
			}
		}
	}

	// 确定下一个 interface_order（找第一个空闲槽位，避免间隙）
	var orders []int
	model.DB.Model(&model.VPCVMBinding{}).
		Where("vm_name = ?", vmName).
		Pluck("interface_order", &orders)
	used := make(map[int]bool, len(orders))
	for _, o := range orders {
		used[o] = true
	}
	nextOrder := 0
	for used[nextOrder] {
		nextOrder++
	}

	// 网卡型号
	nicModel := strings.TrimSpace(req.NicModel)
	if nicModel == "" {
		nicModel = "virtio"
	}

	// 确保交换机运行时已就绪
	if err := EnsureVPCSwitchRuntime(sw); err != nil {
		return nil, err
	}

	// 创建 VM 网口 XML 并附加到虚拟机
	if err := HookAttachVMInterface(vmName, sw, nicModel, nextOrder); err != nil {
		return nil, err
	}

	// 创建 VPC 绑定记录
	binding := model.VPCVMBinding{
		VMName:               vmName,
		Username:             switchOwner,
		SwitchID:             req.SwitchID,
		SecurityGroupID:      securityGroupID,
		InterfaceOrder:       nextOrder,
		NicModel:             nicModel,
		BandwidthInboundAvg:  req.BandwidthInboundAvg,
		BandwidthOutboundAvg: req.BandwidthOutboundAvg,
		AllowedIPv4Addresses: req.AllowedIPv4Addresses,
		AllowedIPv6Addresses: req.AllowedIPv6Addresses,
	}
	if err := model.DB.Create(&binding).Error; err != nil {
		_ = HookDetachVMInterface(vmName, nextOrder)
		return nil, fmt.Errorf("创建网口绑定记录失败: %w", err)
	}

	// 应用带宽设置到该网口
	if req.BandwidthInboundAvg > 0 || req.BandwidthOutboundAvg > 0 {
		if err := applyInterfaceBandwidth(vmName, nextOrder, req.BandwidthInboundAvg, req.BandwidthOutboundAvg); err != nil {
			logger.App.Warn("为新网口应用带宽限制失败", "vm", vmName, "order", nextOrder, "error", err)
		}
	}

	// 应用新网口的 VPC 运行态（只处理新接口，不影响已有接口）
	if err := applyNewInterfaceRuntime(vmName, sw, nextOrder); err != nil {
		logger.App.Warn("为新网口应用 VPC 运行态失败", "vm", vmName, "order", nextOrder, "error", err)
	}
	portSecurityEnabled := HookIsPortSecurityEnabled != nil && HookIsPortSecurityEnabled()
	if portSecurityEnabled && HookReconcileVMPortSecurity != nil {
		if err := HookReconcileVMPortSecurity(vmName); err != nil {
			_ = model.DB.Delete(&binding).Error
			_ = HookDetachVMInterface(vmName, nextOrder)
			return nil, fmt.Errorf("安装新网口端口安全策略失败，已回滚网口: %w", err)
		}
	}
	// 防护开启时网口以 link-down 热插，策略确认后再同步放行运行态与持久化配置。
	if portSecurityEnabled {
		if vnetIF := getVMVnetIFByOrder(vmName, nextOrder); vnetIF != "" {
			if err := setVMInterfaceLink(vmName, vnetIF, "up", false); err != nil {
				_ = model.DB.Delete(&binding).Error
				_ = HookDetachVMInterface(vmName, nextOrder)
				return nil, fmt.Errorf("放行新网口运行态链路失败，已回滚网口: %w", err)
			}
		}
		mac := HookGetVMMACByOrder(vmName, nextOrder)
		if mac == "" {
			_ = model.DB.Delete(&binding).Error
			_ = HookDetachVMInterface(vmName, nextOrder)
			return nil, fmt.Errorf("读取新网口 MAC 失败，已回滚网口")
		}
		if err := setVMInterfaceLink(vmName, mac, "up", true); err != nil {
			_ = model.DB.Delete(&binding).Error
			_ = HookDetachVMInterface(vmName, nextOrder)
			return nil, fmt.Errorf("持久化新网口链路状态失败，已回滚网口: %w", err)
		}
	}
	// 仅刷新交换机带宽和 ACL，不修改已有网口
	if err := ApplyVPCSwitchBandwidth(sw); err != nil {
		logger.App.Warn("刷新交换机带宽失败", "switch", sw.Name, "error", err)
	}
	if !HookSwitchUsesDirectBridge(sw) {
		_ = ApplyVPCACLRules()
	}
	if HookTriggerPortSecurityReconcile != nil {
		HookTriggerPortSecurityReconcile()
	}

	return &VMInterfaceInfo{
		Binding:       binding,
		Switch:        &sw,
		SecurityGroup: nil,
		MAC:           HookGetVMMACByOrder(vmName, nextOrder),
	}, nil
}

// UpdateVMInterface 更新虚拟机指定网口的 VPC 交换机/安全组绑定（管理员原语；普通用户请使用 UpdateVMInterfaceAsUser）
func UpdateVMInterface(vmName string, interfaceOrder int, req AddVMInterfaceRequest) error {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return fmt.Errorf("虚拟机名称不能为空")
	}

	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", vmName, interfaceOrder).First(&binding).Error; err != nil {
		return fmt.Errorf("未找到指定的网口绑定")
	}
	previousBinding := binding
	oldSwitchID := binding.SwitchID
	var oldSwitch model.VPCSwitch

	// 验证交换机存在
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, req.SwitchID).Error; err != nil {
		return fmt.Errorf("交换机不存在")
	}
	if err := normalizeInterfacePortSecurityFields(&req, HookSwitchUsesDirectBridge(sw) && sw.IPv6SecurityEnabled); err != nil {
		return err
	}
	if SwitchIsTrustedIsolated(sw) {
		req.AllowedIPv4Addresses = ""
		req.AllowedIPv6Addresses = ""
	}

	// 系统交换机使用 VM 归属用户的默认安全组
	switchOwner := sw.Username
	if sw.IsSystem {
		switchOwner = HookFindVMOwner(vmName)
		if switchOwner == "" {
			// 回退：从已有 VPC 绑定记录中获取用户名
			if binding.Username != "" {
				switchOwner = binding.Username
			}
		}
		if switchOwner == "" {
			// 回退：从 VM 缓存表获取归属用户
			var cache model.VMCache
			if err := model.DB.Where("name = ?", vmName).First(&cache).Error; err == nil && cache.OwnerUsername != "" {
				switchOwner = cache.OwnerUsername
			}
		}
		if switchOwner == "" {
			return fmt.Errorf("无法识别虚拟机归属用户")
		}
	}

	// 安全组处理
	securityGroupID := req.SecurityGroupID
	if HookSwitchUsesDirectBridge(sw) {
		securityGroupID = 0
	} else {
		if securityGroupID == 0 {
			if _, err := EnsureDefaultSecurityGroup(switchOwner); err != nil {
				return err
			}
			var group model.VPCSecurityGroup
			if err := model.DB.Where("username = ? AND is_default = ?", switchOwner, true).First(&group).Error; err != nil {
				return fmt.Errorf("未找到用户 %s 的默认安全组", switchOwner)
			}
			securityGroupID = group.ID
		} else {
			var group model.VPCSecurityGroup
			if err := model.DB.First(&group, securityGroupID).Error; err != nil {
				return fmt.Errorf("安全组不存在")
			}
			if !sw.IsSystem && group.Username != sw.Username && !isLightweightDedicatedVPCSecurityGroup(group, vmName, sw.ID) {
				return fmt.Errorf("安全组必须属于交换机用户 %s", sw.Username)
			}
		}
	}

	// 网卡型号
	nicModel := strings.TrimSpace(req.NicModel)
	if nicModel == "" {
		nicModel = binding.NicModel
	}
	portSecurityEnabled := HookIsPortSecurityEnabled != nil && HookIsPortSecurityEnabled()
	vmState := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
	linkPort := ""
	if portSecurityEnabled && vmState == "running" {
		linkPort = getVMVnetIFByOrder(vmName, interfaceOrder)
		if linkPort == "" {
			return fmt.Errorf("读取运行态网口失败，已保持原配置")
		}
		if err := setVMInterfaceLink(vmName, linkPort, "down", false); err != nil {
			return fmt.Errorf("隔离待修改网口失败: %w", err)
		}
	}
	restoreLink := func() {
		if linkPort == "" {
			return
		}
		// 热拔插会重新创建 vnet 端口，不能继续使用切换前的端口名称。
		currentPort := getVMVnetIFByOrder(vmName, interfaceOrder)
		if currentPort != "" {
			_ = setVMInterfaceLink(vmName, currentPort, "up", false)
		}
	}

	// 交换机切换必须先更新实际网口。否则数据库已指向目标交换机，
	// 但运行态和持久化 XML 仍保留旧交换机，面板与实际网络会不一致。
	networkChanged := false
	rollbackNetwork := func() error {
		if !networkChanged {
			return nil
		}
		if err := HookReconfigureVMInterfaceNetwork(vmName, interfaceOrder, oldSwitch); err != nil {
			return err
		}
		networkChanged = false
		return nil
	}
	if oldSwitchID != req.SwitchID {
		if HookReconfigureVMInterfaceNetwork == nil {
			restoreLink()
			return fmt.Errorf("网口网络重配置服务尚未初始化")
		}
		if err := model.DB.First(&oldSwitch, oldSwitchID).Error; err != nil {
			restoreLink()
			return fmt.Errorf("读取原交换机失败，已保持原配置: %w", err)
		}
		if err := HookReconfigureVMInterfaceNetwork(vmName, interfaceOrder, sw); err != nil {
			restoreLink()
			return fmt.Errorf("切换第 %d 个网口的交换机失败，已保持原配置: %w", interfaceOrder+1, err)
		}
		networkChanged = true
	}

	// 实际网口切换成功后再更新绑定记录。
	binding.Username = sw.Username
	binding.SwitchID = req.SwitchID
	binding.SecurityGroupID = securityGroupID
	binding.NicModel = nicModel
	binding.BandwidthInboundAvg = req.BandwidthInboundAvg
	binding.BandwidthOutboundAvg = req.BandwidthOutboundAvg
	binding.AllowedIPv4Addresses = req.AllowedIPv4Addresses
	binding.AllowedIPv6Addresses = req.AllowedIPv6Addresses
	if err := model.DB.Save(&binding).Error; err != nil {
		if rollbackErr := rollbackNetwork(); rollbackErr != nil {
			restoreLink()
			return fmt.Errorf("更新网口绑定记录失败，且恢复原网口失败: %v；%w", rollbackErr, err)
		}
		restoreLink()
		return fmt.Errorf("更新网口绑定记录失败: %w", err)
	}

	// 应用带宽设置到该网口
	if req.BandwidthInboundAvg > 0 || req.BandwidthOutboundAvg > 0 {
		if err := applyInterfaceBandwidth(vmName, interfaceOrder, req.BandwidthInboundAvg, req.BandwidthOutboundAvg); err != nil {
			logger.App.Warn("为网口应用带宽限制失败", "vm", vmName, "order", interfaceOrder, "error", err)
		}
	} else {
		// 带宽设为 0 时清除该网口限制
		if err := clearInterfaceBandwidth(vmName, interfaceOrder); err != nil {
			logger.App.Warn("清除网口带宽限制失败", "vm", vmName, "order", interfaceOrder, "error", err)
		}
	}

	// 实际网口已在保存绑定前完成重配置；此处只清理旧交换机 DHCP 租约。
	if oldSwitchID != req.SwitchID {
		if mac := HookGetVMMACByOrder(vmName, interfaceOrder); mac != "" {
			HookCleanOVSDHCPLease(mac, "")
		}
	}

	// 刷新交换机带宽和 ACL
	if err := ApplyVPCSwitchBandwidth(sw); err != nil {
		logger.App.Warn("刷新交换机带宽失败", "switch", sw.Name, "error", err)
	}
	if oldSwitchID != req.SwitchID {
		var oldSw model.VPCSwitch
		if model.DB.First(&oldSw, oldSwitchID).Error == nil {
			_ = ApplyVPCSwitchBandwidth(oldSw)
		}
	}
	if !HookSwitchUsesDirectBridge(sw) {
		_ = ApplyVPCACLRules()
	}
	if portSecurityEnabled && HookReconcileVMPortSecurity != nil {
		if err := HookReconcileVMPortSecurity(vmName); err != nil {
			if rollbackErr := rollbackNetwork(); rollbackErr != nil {
				restoreLink()
				return fmt.Errorf("更新端口安全策略失败，且恢复原网口失败，当前绑定保留目标交换机: %v；%w", rollbackErr, err)
			}
			if rollbackErr := model.DB.Save(&previousBinding).Error; rollbackErr != nil {
				restoreLink()
				return fmt.Errorf("更新端口安全策略失败，且恢复原绑定失败: %v；%w", rollbackErr, err)
			}
			_ = HookReconcileVMPortSecurity(vmName)
			restoreLink()
			return fmt.Errorf("更新端口安全策略失败，已恢复原绑定: %w", err)
		}
		if linkPort != "" {
			currentPort := getVMVnetIFByOrder(vmName, interfaceOrder)
			if currentPort == "" {
				return fmt.Errorf("策略已更新，但未找到切换后的运行态网口")
			}
			if err := setVMInterfaceLink(vmName, currentPort, "up", false); err != nil {
				return fmt.Errorf("策略已更新，但恢复网口链路失败: %w", err)
			}
		}
	} else if HookTriggerPortSecurityReconcile != nil {
		HookTriggerPortSecurityReconcile()
	}

	return nil
}

// setVMInterfaceLink 统一切换虚拟机网口链路状态；domif-setlink 默认作用于运行态。
func setVMInterfaceLink(vmName, interfaceRef, state string, persistent bool) error {
	args := []string{"domif-setlink", vmName, interfaceRef, state}
	if persistent {
		args = append(args, "--config")
	}
	result := utils.ExecCommandQuiet("virsh", args...)
	if result.Error != nil {
		return fmt.Errorf("%s", HookFirstNonEmpty(result.Stderr, result.Error.Error()))
	}
	return nil
}

// RemoveVMInterface 删除虚拟机的指定网口（管理员原语；普通用户请使用 RemoveVMInterfaceAsUser）
func RemoveVMInterface(vmName string, interfaceOrder int) error {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return fmt.Errorf("虚拟机名称不能为空")
	}

	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", vmName, interfaceOrder).First(&binding).Error; err != nil {
		return fmt.Errorf("未找到指定的网口绑定")
	}

	// 从虚拟机 XML 中移除网口
	if err := HookDetachVMInterface(vmName, interfaceOrder); err != nil {
		return err
	}

	// 删除绑定记录
	switchID := binding.SwitchID
	if err := model.DB.Delete(&binding).Error; err != nil {
		return fmt.Errorf("删除网口绑定记录失败: %w", err)
	}

	// 刷新交换机带宽和 ACL
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, switchID).Error; err == nil {
		_ = ApplyVPCSwitchBandwidth(sw)
		if !HookSwitchUsesDirectBridge(sw) {
			_ = ApplyVPCACLRules()
		}
	}
	if HookTriggerPortSecurityReconcile != nil {
		HookTriggerPortSecurityReconcile()
	}

	return nil
}

// checkUserInterfaceOperationAllowed 校验普通用户能否对虚拟机做网口自助操作。
// 轻量云网络由管理员分配，不允许自助管理网口。
func checkUserInterfaceOperationAllowed(operator, vmName string) error {
	if HookIsLightweightCloudUser != nil && HookIsLightweightCloudUser(operator) {
		return fmt.Errorf("轻量云服务器网络由管理员分配，不能自行管理网口")
	}
	if HookUserOwnsVM == nil || !HookUserOwnsVM(operator, vmName) {
		return fmt.Errorf("无权操作此虚拟机")
	}
	return nil
}

// validateUserInterfaceSwitchAndGroup 校验普通用户网口操作的目标交换机与安全组归属：
// 仅允许本人的非系统交换机；非二层交换机必须使用本人安全组（0 由服务层回落到默认安全组）。
func validateUserInterfaceSwitchAndGroup(operator string, req AddVMInterfaceRequest) (model.VPCSwitch, error) {
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, req.SwitchID).Error; err != nil {
		return sw, fmt.Errorf("交换机不存在")
	}
	if sw.IsSystem {
		return sw, fmt.Errorf("系统基础网络交换机不可选择，请使用自己的交换机")
	}
	if sw.Username != operator {
		return sw, fmt.Errorf("交换机不属于当前用户")
	}
	if req.SecurityGroupID != 0 && !HookSwitchUsesDirectBridge(sw) {
		var group model.VPCSecurityGroup
		if err := model.DB.First(&group, req.SecurityGroupID).Error; err != nil {
			return sw, fmt.Errorf("安全组不存在")
		}
		if group.Username != operator {
			return sw, fmt.Errorf("安全组不属于当前用户")
		}
	}
	return sw, nil
}

// ValidateExtraNicsForUser 校验普通用户创建/克隆链路中附加网口的交换机与安全组归属。
// 与网口自助管理同规则：仅本人的非系统交换机；轻量云不允许携带附加网口。
func ValidateExtraNicsForUser(operator string, extraNics []AddVMInterfaceRequest) error {
	if len(extraNics) == 0 {
		return nil
	}
	if HookIsLightweightCloudUser != nil && HookIsLightweightCloudUser(operator) {
		return fmt.Errorf("轻量云服务器网络由管理员分配，不能自行添加附加网口")
	}
	for i, nic := range extraNics {
		if nic.SwitchID == 0 {
			continue
		}
		if _, err := validateUserInterfaceSwitchAndGroup(operator, nic); err != nil {
			return fmt.Errorf("网口 #%d: %w", i+2, err)
		}
	}
	return nil
}

// AddVMInterfaceAsUser 普通用户为自己的虚拟机新增网口。
// 仅允许接入本人的非系统交换机；网口级速率限制属管理员能力，用户新增时强制为 0。
func AddVMInterfaceAsUser(operator, vmName string, req AddVMInterfaceRequest) (*VMInterfaceInfo, error) {
	if err := checkUserInterfaceOperationAllowed(operator, vmName); err != nil {
		return nil, err
	}
	if _, err := validateUserInterfaceSwitchAndGroup(operator, req); err != nil {
		return nil, err
	}
	req.BandwidthInboundAvg = 0
	req.BandwidthOutboundAvg = 0
	return AddVMInterface(vmName, req)
}

// UpdateVMInterfaceAsUser 普通用户更新自己虚拟机附加网口的交换机/安全组绑定。
// 主网口由 VPC 网络绑定管理；管理员配置的速率限制保持原值，不允许用户覆盖。
func UpdateVMInterfaceAsUser(operator, vmName string, interfaceOrder int, req AddVMInterfaceRequest) error {
	if err := checkUserInterfaceOperationAllowed(operator, vmName); err != nil {
		return err
	}
	if interfaceOrder <= 0 {
		return fmt.Errorf("主网口不能在此修改，请在 VPC 网络绑定中管理")
	}
	if _, err := validateUserInterfaceSwitchAndGroup(operator, req); err != nil {
		return err
	}
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", vmName, interfaceOrder).First(&binding).Error; err != nil {
		return fmt.Errorf("未找到指定的网口绑定")
	}
	req.BandwidthInboundAvg = binding.BandwidthInboundAvg
	req.BandwidthOutboundAvg = binding.BandwidthOutboundAvg
	return UpdateVMInterface(vmName, interfaceOrder, req)
}

// RemoveVMInterfaceAsUser 普通用户删除自己虚拟机的附加网口（主网口不可直接删除）。
func RemoveVMInterfaceAsUser(operator, vmName string, interfaceOrder int) error {
	if err := checkUserInterfaceOperationAllowed(operator, vmName); err != nil {
		return err
	}
	if interfaceOrder <= 0 {
		return fmt.Errorf("主网口不能直接删除，请在 VPC 网络绑定中管理")
	}
	return RemoveVMInterface(vmName, interfaceOrder)
}

// AttachExtraNICs 批量附加额外网口（用于创建/克隆流程）
func AttachExtraNICs(vmName string, extraNics []AddVMInterfaceRequest) error {
	attachedOrders := make([]int, 0, len(extraNics))
	for i, nic := range extraNics {
		if nic.SwitchID == 0 {
			continue
		}
		info, err := AddVMInterface(vmName, nic)
		if err != nil {
			for rollbackIndex := len(attachedOrders) - 1; rollbackIndex >= 0; rollbackIndex-- {
				_ = RemoveVMInterface(vmName, attachedOrders[rollbackIndex])
			}
			return fmt.Errorf("添加第 %d 张网卡失败: %w", i+2, err)
		}
		if info != nil {
			attachedOrders = append(attachedOrders, info.Binding.InterfaceOrder)
		}
	}
	return nil
}

// applyNewInterfaceRuntime 为新添加的网口设置 OVS VLAN tag（不影响已有网口）
func applyNewInterfaceRuntime(vmName string, sw model.VPCSwitch, interfaceOrder int) error {
	state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
	if state != "running" {
		return nil // 关机态的 VLAN 已在 XML 中配置
	}

	// 从 domiflist 获取新网口的 vnet 接口名
	vnetIF := getVMVnetIFByOrder(vmName, interfaceOrder)
	if vnetIF == "" {
		// 等待 vnet 接口出现
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			vnetIF = getVMVnetIFByOrder(vmName, interfaceOrder)
			if vnetIF != "" {
				break
			}
		}
	}
	if vnetIF == "" {
		return fmt.Errorf("无法找到新网口对应的 vnet 接口")
	}

	targetVLAN := sw.VLANID
	if HookSwitchUsesDirectBridge(sw) {
		targetVLAN = sw.BridgeVLANID
	}
	if targetVLAN > 0 {
		// 检查端口是否实际存在于 OVS
		if !ovsPortExists(vnetIF) {
			logger.App.Warn("OVS 端口不存在，跳过新网口 VLAN tag 设置", "port", vnetIF)
		} else {
			targetTag := strconv.Itoa(targetVLAN)
			result := utils.ExecCommand("ovs-vsctl", "set", "Port", vnetIF, "tag="+targetTag)
			if result.Error != nil {
				return fmt.Errorf("设置新网口 OVS VLAN tag 失败: %s", result.Stderr)
			}
		}
	}
	// 清理该接口的旧 DHCP 租约
	mac := HookGetVMMACByOrder(vmName, interfaceOrder)
	if mac != "" {
		HookCleanOVSDHCPLease(mac, "")
	}
	return nil
}

// getVMVnetIFByOrder 获取虚拟机第 N 个网口对应的 vnet 接口名
func getVMVnetIFByOrder(vmName string, order int) string {
	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	idx := 0
	for i, line := range lines {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		if idx == order {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				return fields[0] // 第一列是 Interface 名称（如 vnet0）
			}
		}
		idx++
	}
	return ""
}

// getVMVnetIFByMAC 在热拔插可能改变网口排列时按 MAC 精确定位运行态 vnet。
func getVMVnetIFByMAC(vmName, mac string) string {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return ""
	}
	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return ""
	}
	for index, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if index < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 && strings.EqualFold(fields[4], mac) {
			return fields[0]
		}
	}
	return ""
}

// ListVMInterfaces 列出虚拟机所有网口绑定
func ListVMInterfaces(vmName string) ([]VMInterfaceInfo, error) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return nil, fmt.Errorf("虚拟机名称不能为空")
	}

	var bindings []model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).Order("interface_order ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}

	switchIDs := make([]uint, 0, len(bindings))
	sgIDs := make([]uint, 0, len(bindings))
	for _, b := range bindings {
		switchIDs = append(switchIDs, b.SwitchID)
		sgIDs = append(sgIDs, b.SecurityGroupID)
	}

	switches := map[uint]*model.VPCSwitch{}
	if len(switchIDs) > 0 {
		var swList []model.VPCSwitch
		if err := model.DB.Where("id IN ?", switchIDs).Find(&swList).Error; err == nil {
			for i := range swList {
				normalizeVPCSwitchBandwidthForResponse(&swList[i])
				switches[swList[i].ID] = &swList[i]
			}
		}
	}

	secGroups := map[uint]*model.VPCSecurityGroup{}
	if len(sgIDs) > 0 {
		var sgList []model.VPCSecurityGroup
		if err := model.DB.Where("id IN ?", sgIDs).Find(&sgList).Error; err == nil {
			for i := range sgList {
				secGroups[sgList[i].ID] = &sgList[i]
			}
		}
	}

	result := make([]VMInterfaceInfo, 0, len(bindings))
	for _, b := range bindings {
		info := VMInterfaceInfo{
			Binding: b,
			MAC:     HookGetVMMACByOrder(vmName, b.InterfaceOrder),
		}
		if sw, ok := switches[b.SwitchID]; ok {
			info.Switch = sw
		}
		if sg, ok := secGroups[b.SecurityGroupID]; ok {
			info.SecurityGroup = sg
		}
		result = append(result, info)
	}
	return result, nil
}

// applyInterfaceBandwidth 对指定网口应用速率限制（通过 virsh domiftune）
func applyInterfaceBandwidth(vmName string, interfaceOrder int, inboundAvgMbps, outboundAvgMbps int) error {
	mac := HookGetVMMACByOrder(vmName, interfaceOrder)
	if mac == "" {
		return fmt.Errorf("无法获取网口 %d 的 MAC 地址", interfaceOrder)
	}

	inAvgKB := inboundAvgMbps * 125
	outAvgKB := outboundAvgMbps * 125

	// 尝试通过 libvirt RPC 设置持久化配置
	state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
	if state == "running" {
		// 运行态：先设置 --config 持久化，再设置 --live 实时生效
		configResult := utils.ExecCommand("virsh", "domiftune", vmName, mac,
			"--inbound", fmt.Sprintf("%d,%d,%d", inAvgKB, inAvgKB, inAvgKB*30),
			"--outbound", fmt.Sprintf("%d,%d,%d", outAvgKB, outAvgKB, outAvgKB*30),
			"--config")
		if configResult.Error != nil {
			return fmt.Errorf("domiftune --config 失败: %s", strings.TrimSpace(configResult.Stderr))
		}
		liveResult := utils.ExecCommand("virsh", "domiftune", vmName, mac,
			"--inbound", fmt.Sprintf("%d,%d,%d", inAvgKB, inAvgKB, inAvgKB*30),
			"--outbound", fmt.Sprintf("%d,%d,%d", outAvgKB, outAvgKB, outAvgKB*30),
			"--live")
		if liveResult.Error != nil {
			// live 失败只 warn，不影响 config 已写入
			logger.App.Warn("设置网口实时带宽失败（持久化已生效）", "vm", vmName, "order", interfaceOrder, "error", strings.TrimSpace(liveResult.Stderr))
		}
	} else {
		// 关机态：只设置 --config
		configResult := utils.ExecCommand("virsh", "domiftune", vmName, mac,
			"--inbound", fmt.Sprintf("%d,%d,%d", inAvgKB, inAvgKB, inAvgKB*30),
			"--outbound", fmt.Sprintf("%d,%d,%d", outAvgKB, outAvgKB, outAvgKB*30),
			"--config")
		if configResult.Error != nil {
			return fmt.Errorf("domiftune --config 失败: %s", strings.TrimSpace(configResult.Stderr))
		}
	}

	return nil
}

// clearInterfaceBandwidth 清除指定网口的速率限制
func clearInterfaceBandwidth(vmName string, interfaceOrder int) error {
	mac := HookGetVMMACByOrder(vmName, interfaceOrder)
	if mac == "" {
		return fmt.Errorf("无法获取网口 %d 的 MAC 地址", interfaceOrder)
	}

	state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
	if state == "running" {
		utils.ExecCommand("virsh", "domiftune", vmName, mac,
			"--inbound", "0,0,0",
			"--outbound", "0,0,0",
			"--config")
		utils.ExecCommand("virsh", "domiftune", vmName, mac,
			"--inbound", "0,0,0",
			"--outbound", "0,0,0",
			"--live")
	} else {
		utils.ExecCommand("virsh", "domiftune", vmName, mac,
			"--inbound", "0,0,0",
			"--outbound", "0,0,0",
			"--config")
	}

	return nil
}
