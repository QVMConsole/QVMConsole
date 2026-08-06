package vpc

import (
	"fmt"
	"strconv"
	"strings"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/ip_resolver"
	"kvm_console/utils"
)

func ApplyVPCSwitchRuntime(vmName string, sw model.VPCSwitch) error {
	return applyVPCSwitchRuntime(vmName, sw, true)
}

func firstVMRuntimeInterface(vmName string) (RuntimeInterface, bool) {
	if HookParseVirshDomiflist == nil {
		return RuntimeInterface{}, false
	}
	for _, iface := range HookParseVirshDomiflist(utils.ExecCommand("virsh", "domiflist", vmName).Stdout) {
		if strings.TrimSpace(iface.Name) != "" || strings.TrimSpace(iface.MAC) != "" {
			return iface, true
		}
	}
	return RuntimeInterface{}, false
}

func firstVMRuntimeInterfaceUsesForeignBridge(vmName string) bool {
	iface, ok := firstVMRuntimeInterface(vmName)
	return ok && iface.Type == "bridge" && iface.Source != "" && iface.Source != HookOvsBridgeName()
}

func applyVPCSwitchRuntime(vmName string, sw model.VPCSwitch, ensureSwitch bool) error {
	if ensureSwitch {
		if err := EnsureVPCSwitchRuntime(sw); err != nil {
			return err
		}
	}
	if HookSwitchUsesDirectBridge(sw) {
		state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
		if state == "running" {
			for _, iface := range HookParseVirshDomiflist(utils.ExecCommand("virsh", "domiflist", vmName).Stdout) {
				if iface.Source == HookBridgeNameForSwitch(sw) {
					if err := ensureVMBridgeInterfaceConfig(vmName, HookBridgeNameForSwitch(sw), sw.BridgeVLANID); err != nil {
						return err
					}
					if err := ensureVMDirectBridgeRuntimeVLAN(vmName, sw.BridgeVLANID); err != nil {
						return err
					}
					if ensureSwitch {
						return ApplyVPCSwitchBandwidth(sw)
					}
					return nil
				}
			}
			return fmt.Errorf("桥接直通交换机切换需要先关闭虚拟机")
		}
		if err := ensureVMBridgeInterfaceConfig(vmName, HookBridgeNameForSwitch(sw), sw.BridgeVLANID); err != nil {
			return err
		}
		if ensureSwitch {
			return ApplyVPCSwitchBandwidth(sw)
		}
		return nil
	}
	if strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout) == "running" {
		if firstVMRuntimeInterfaceUsesForeignBridge(vmName) {
			return fmt.Errorf("从桥接直通交换机切换回 VPC 需要先关闭虚拟机")
		}
	}
	if err := ensureVMVPCInterfaceConfig(vmName, sw.VLANID); err != nil {
		return err
	}
	if mac := ip_resolver.GetFirstVMMAC(vmName); mac != "" {
		HookCleanOVSDHCPLease(mac, "")
	}
	if strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout) == "running" {
		if err := ensureVMVPCRuntimeInterfaceConfig(vmName, sw.VLANID); err != nil {
			return err
		}
	}
	vnetIF := ip_resolver.GetVMVnetIF(vmName)
	if vnetIF == "" {
		logger.App.Warn("无法获取 VM vnet 接口，跳过 VLAN tag 设置", "vm", vmName)
		return nil
	}
	// 检查端口是否实际存在于 OVS
	if !ovsPortExists(vnetIF) {
		logger.App.Warn("OVS 端口不存在，跳过 VLAN tag 设置", "port", vnetIF)
		return nil
	}
	targetTag := strconv.Itoa(sw.VLANID)
	if currentTag, ok := getOVSPortTag(vnetIF); !ok || currentTag != targetTag {
		result := utils.ExecCommand("ovs-vsctl", "set", "Port", vnetIF, "tag="+targetTag)
		if result.Error != nil {
			return fmt.Errorf("设置 VM OVS VLAN tag 失败: %s", result.Stderr)
		}
	}
	if ensureSwitch {
		if err := ApplyVPCSwitchBandwidth(sw); err != nil {
			return err
		}
	}
	return nil
}

// ovsPortExists 检查指定端口是否存在于 OVS 网桥上
func ovsPortExists(port string) bool {
	if strings.TrimSpace(port) == "" {
		return false
	}
	result := utils.ExecCommand("ovs-vsctl", "--if-exists", "get", "Port", port, "name")
	return result.Error == nil
}

func getOVSPortTag(port string) (string, bool) {
	if strings.TrimSpace(port) == "" {
		return "", false
	}
	result := utils.ExecCommand("ovs-vsctl", "--if-exists", "get", "Port", port, "tag")
	if result.Error != nil {
		return "", false
	}
	tag := strings.Trim(strings.TrimSpace(result.Stdout), `"`)
	if tag == "" || tag == "[]" {
		return "", false
	}
	return tag, true
}

func validateVMCanApplyVPCSwitch(vmName string, sw model.VPCSwitch) error {
	if strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout) != "running" {
		return nil
	}
	if HookSwitchUsesDirectBridge(sw) {
		for _, iface := range HookParseVirshDomiflist(utils.ExecCommand("virsh", "domiflist", vmName).Stdout) {
			if iface.Source == HookBridgeNameForSwitch(sw) {
				return nil
			}
		}
		return fmt.Errorf("桥接直通交换机切换需要先关闭虚拟机")
	}
	if firstVMRuntimeInterfaceUsesForeignBridge(vmName) {
		return fmt.Errorf("从桥接直通交换机切换回 VPC 需要先关闭虚拟机")
	}
	return nil
}

func ApplyAllVPCBindingsRuntime() error {
	return applyAllVPCBindingsRuntime(true)
}

func applyAllVPCBindingsRuntime(ensureSwitch bool) error {
	if model.DB == nil {
		return nil
	}
	var bindings []model.VPCVMBinding
	if err := model.DB.Order("vm_name ASC").Find(&bindings).Error; err != nil {
		return err
	}
	var lastErr error
	switches := map[uint]model.VPCSwitch{}
	touchedSwitches := map[uint]model.VPCSwitch{}
	ensuredSwitches := map[uint]bool{}
	for _, binding := range bindings {
		if vmLibvirtDomainMissing(binding.VMName) {
			logger.App.Info("VM 已不存在，清理残留 VPC 绑定", "vm", binding.VMName)
			CleanupVMVPCBinding(binding.VMName)
			continue
		}
		sw, ok := switches[binding.SwitchID]
		if !ok {
			if err := model.DB.First(&sw, binding.SwitchID).Error; err != nil {
				lastErr = err
				logger.App.Warn("VM 绑定的 VPC 交换机不存在", "vm", binding.VMName, "switchID", binding.SwitchID, "error", err)
				continue
			}
			switches[binding.SwitchID] = sw
		}
		if ensureSwitch && !ensuredSwitches[sw.ID] {
			if err := EnsureVPCSwitchRuntime(sw); err != nil {
				lastErr = err
				logger.App.Warn("恢复 VPC 交换机运行态失败", "switch", sw.Name, "id", sw.ID, "error", err)
				continue
			}
			ensuredSwitches[sw.ID] = true
		}
		if err := applyVPCSwitchRuntime(binding.VMName, sw, false); err != nil {
			lastErr = err
			logger.App.Warn("恢复 VM VPC 运行态失败", "vm", binding.VMName, "error", err)
			continue
		}
		touchedSwitches[sw.ID] = sw
	}
	for _, sw := range touchedSwitches {
		if err := ApplyVPCSwitchBandwidth(sw); err != nil {
			lastErr = err
			logger.App.Warn("恢复 VPC 交换机带宽策略失败", "switch", sw.Name, "id", sw.ID, "error", err)
		}
	}
	return lastErr
}

func vmLibvirtDomainMissing(vmName string) bool {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return false
	}
	result := utils.ExecCommand("virsh", "domstate", vmName)
	return isLibvirtDomainMissingResult(result)
}

func isLibvirtDomainMissingResult(result *utils.CmdResult) bool {
	if result == nil || result.Error == nil {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		result.Stdout,
		result.Stderr,
		result.Error.Error(),
	}, "\n"))
	return strings.Contains(text, "failed to get domain") ||
		strings.Contains(text, "domain not found") ||
		strings.Contains(text, "no domain with matching name") ||
		strings.Contains(text, "domain is not found")
}
