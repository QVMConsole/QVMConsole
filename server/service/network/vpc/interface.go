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

// AddVMInterface 为虚拟机新增一个网口并绑定到 VPC 交换机（仅管理员）
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
	if !HookSwitchUsesDirectBridge(sw) {
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
	}
	if err := model.DB.Create(&binding).Error; err != nil {
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
	// 仅刷新交换机带宽和 ACL，不修改已有网口
	if err := ApplyVPCSwitchBandwidth(sw); err != nil {
		logger.App.Warn("刷新交换机带宽失败", "switch", sw.Name, "error", err)
	}
	if !HookSwitchUsesDirectBridge(sw) {
		_ = ApplyVPCACLRules()
	}

	return &VMInterfaceInfo{
		Binding:       binding,
		Switch:        &sw,
		SecurityGroup: nil,
	}, nil
}

// UpdateVMInterface 更新虚拟机指定网口的 VPC 交换机/安全组绑定（仅管理员）
func UpdateVMInterface(vmName string, interfaceOrder int, req AddVMInterfaceRequest) error {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return fmt.Errorf("虚拟机名称不能为空")
	}

	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", vmName, interfaceOrder).First(&binding).Error; err != nil {
		return fmt.Errorf("未找到指定的网口绑定")
	}

	oldSwitchID := binding.SwitchID

	// 验证交换机存在
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, req.SwitchID).Error; err != nil {
		return fmt.Errorf("交换机不存在")
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
	if !HookSwitchUsesDirectBridge(sw) {
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

	// 更新绑定记录
	binding.Username = sw.Username
	binding.SwitchID = req.SwitchID
	binding.SecurityGroupID = securityGroupID
	binding.NicModel = nicModel
	binding.BandwidthInboundAvg = req.BandwidthInboundAvg
	binding.BandwidthOutboundAvg = req.BandwidthOutboundAvg
	if err := model.DB.Save(&binding).Error; err != nil {
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

	// 如果交换机改变了，需要更新 VM 的 XML 配置（仅主网口 interface_order==0 支持）
	if oldSwitchID != req.SwitchID {
		vmState := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
		if vmState != "running" {
			// 关机态：更新 inactive XML，确保下次开机时使用正确配置
			if interfaceOrder == 0 {
				if HookSwitchUsesDirectBridge(sw) {
					if err := ensureVMBridgeInterfaceConfig(vmName, HookBridgeNameForSwitch(sw), sw.BridgeVLANID); err != nil {
						logger.App.Warn("更新桥接直通 XML 失败", "vm", vmName, "error", err)
					}
				} else {
					if err := ensureVMVPCInterfaceConfig(vmName, sw.VLANID); err != nil {
						logger.App.Warn("更新 VPC VLAN XML 失败", "vm", vmName, "error", err)
					}
				}
			} else {
				// TODO: 非主网口需要 detach-device + attach-device 或完整 XML 重写
				logger.App.Warn("非主网口交换机变更后需重启虚拟机生效", "vm", vmName, "order", interfaceOrder)
			}
		} else {
			// 运行态：尝试热更新 VLAN tag
			vnetIF := getVMVnetIFByOrder(vmName, interfaceOrder)
			if vnetIF != "" && !HookSwitchUsesDirectBridge(sw) && sw.VLANID > 0 {
				targetTag := strconv.Itoa(sw.VLANID)
				_ = utils.ExecCommand("ovs-vsctl", "set", "Port", vnetIF, "tag="+targetTag)
			}
		}
		// 清理旧交换机 DHCP 租约
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

	return nil
}

// RemoveVMInterface 删除虚拟机的指定网口（仅管理员）
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

	return nil
}

// AttachExtraNICs 批量附加额外网口（用于创建/克隆流程）
func AttachExtraNICs(vmName string, extraNics []AddVMInterfaceRequest) error {
	for i, nic := range extraNics {
		if nic.SwitchID == 0 {
			continue
		}
		if _, err := AddVMInterface(vmName, nic); err != nil {
			return fmt.Errorf("添加第 %d 张网卡失败: %w", i+2, err)
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

	if !HookSwitchUsesDirectBridge(sw) && sw.VLANID > 0 {
		// 检查端口是否实际存在于 OVS
		if !ovsPortExists(vnetIF) {
			logger.App.Warn("OVS 端口不存在，跳过新网口 VLAN tag 设置", "port", vnetIF)
		} else {
			targetTag := strconv.Itoa(sw.VLANID)
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
		info := VMInterfaceInfo{Binding: b}
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
