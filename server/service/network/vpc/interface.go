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
			if !sw.IsSystem && group.Username != sw.Username {
				return nil, fmt.Errorf("安全组必须属于交换机用户 %s", sw.Username)
			}
		}
	}

	// 确定下一个 interface_order
	var maxOrder int
	if err := model.DB.Model(&model.VPCVMBinding{}).
		Where("vm_name = ?", vmName).
		Select("COALESCE(MAX(interface_order), -1) as max_order").
		Scan(&maxOrder).Error; err != nil {
		return nil, fmt.Errorf("查询现有网口失败: %w", err)
	}
	nextOrder := maxOrder + 1

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

	// 如果 nextOrder == 0 表示没有现有绑定，需要检查是否已有默认绑定（旧数据迁移场景）
	if nextOrder == 0 {
		var existingCount int64
		model.DB.Model(&model.VPCVMBinding{}).Where("vm_name = ?", vmName).Count(&existingCount)
		if existingCount > 0 {
			var newMax int
			model.DB.Model(&model.VPCVMBinding{}).
				Where("vm_name = ?", vmName).
				Select("COALESCE(MAX(interface_order), 0) as max_order").
				Scan(&newMax)
			nextOrder = newMax + 1
		}
	}

	// 创建 VPC 绑定记录
	binding := model.VPCVMBinding{
		VMName:          vmName,
		Username:        sw.Username,
		SwitchID:        req.SwitchID,
		SecurityGroupID: securityGroupID,
		InterfaceOrder:  nextOrder,
		NicModel:        nicModel,
	}
	if err := model.DB.Create(&binding).Error; err != nil {
		return nil, fmt.Errorf("创建网口绑定记录失败: %w", err)
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

// RemoveVMInterface 删除虚拟机的指定网口（仅管理员）
func RemoveVMInterface(vmName string, interfaceOrder int) error {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return fmt.Errorf("虚拟机名称不能为空")
	}

	if interfaceOrder == 0 {
		return fmt.Errorf("不能删除主网口（接口序号 0），请先确保有其他网口存在或直接删除虚拟机")
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
func AttachExtraNICs(vmName string, extraNics []AddVMInterfaceRequest) {
	for i, nic := range extraNics {
		if nic.SwitchID == 0 {
			continue
		}
		if _, err := AddVMInterface(vmName, nic); err != nil {
			logger.App.Warn("添加额外网口失败", "vm", vmName, "order", i+1, "switchID", nic.SwitchID, "error", err)
		}
	}
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

	result := make([]VMInterfaceInfo, 0, len(bindings))
	for _, b := range bindings {
		info := VMInterfaceInfo{Binding: b}
		var sw model.VPCSwitch
		if model.DB.First(&sw, b.SwitchID).Error == nil {
			normalizeVPCSwitchBandwidthForResponse(&sw)
			info.Switch = &sw
		}
		var sg model.VPCSecurityGroup
		if model.DB.First(&sg, b.SecurityGroupID).Error == nil {
			info.SecurityGroup = &sg
		}
		result = append(result, info)
	}
	return result, nil
}
