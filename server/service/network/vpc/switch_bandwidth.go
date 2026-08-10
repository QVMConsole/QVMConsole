package vpc

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"

	"kvm_console/config"
	"kvm_console/model"
	bw "kvm_console/service/bandwidth"
	"kvm_console/utils"
)

func vpcSwitchCookie(switchID uint) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("kvm-console-vpc-switch:%d", switchID)))
	return fmt.Sprintf("0x%x", h.Sum64())
}

func vpcSwitchMeterID(switchID uint, direction string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(fmt.Sprintf("kvm-console-vpc-switch:%d:%s", switchID, direction)))
	// 与端口安全和 VM 级带宽 meter 使用不同的有界区间。
	return 150000 + h.Sum32()%40000
}

func clearVPCSwitchBandwidth(sw model.VPCSwitch) {
	bridge := HookBridgeNameForSwitch(sw)
	cookie := vpcSwitchCookie(sw.ID)
	utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridge, "cookie="+cookie+"/0xffffffffffffffff")
	utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, bw.OvsBandwidthMeterArg(vpcSwitchMeterID(sw.ID, "down")))
	utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, bw.OvsBandwidthMeterArg(vpcSwitchMeterID(sw.ID, "up")))
	if !HookSwitchUsesDirectBridge(sw) {
		HookClearTCVPCSwitchDownlink(VPCGatewayPortName(sw.ID))
	}
}

var (
	vpcSwitchBandwidthMu    sync.Mutex
	vpcSwitchBandwidthLocks = make(map[uint]*sync.Mutex)
)

func lockVPCSwitchBandwidth(switchID uint) func() {
	vpcSwitchBandwidthMu.Lock()
	mu, ok := vpcSwitchBandwidthLocks[switchID]
	if !ok {
		mu = &sync.Mutex{}
		vpcSwitchBandwidthLocks[switchID] = mu
	}
	vpcSwitchBandwidthMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func ApplyVPCSwitchBandwidth(sw model.VPCSwitch) error {
	unlock := lockVPCSwitchBandwidth(sw.ID)
	defer unlock()

	bridge := HookBridgeNameForSwitch(sw)
	if err := HookEnsureOVSBridgeExists(bridge); err != nil {
		return fmt.Errorf("配置 VPC 交换机带宽失败: %w", err)
	}

	normalizeVPCSwitchBandwidthForResponse(&sw)
	downMbps, upMbps := effectiveVPCSwitchBandwidth(sw)
	downRateKbit := downMbps * 1000
	upRateKbit := upMbps * 1000
	if downRateKbit <= 0 && upRateKbit <= 0 {
		clearVPCSwitchBandwidth(sw)
		return nil
	}
	gatewayOfport := ""
	if downRateKbit > 0 && !HookSwitchUsesDirectBridge(sw) {
		gatewayOfport = HookGetOVSInterfaceOfPort(VPCGatewayPortName(sw.ID))
		if gatewayOfport == "" {
			return fmt.Errorf("无法获取 VPC 交换机 %s 的网关端口号", sw.Name)
		}
	}
	vmOfports := []string{}
	if upRateKbit > 0 {
		vmOfports = listVPCSwitchVMOfports(sw.ID)
	}
	clearVPCSwitchBandwidth(sw)
	downMeter := vpcSwitchMeterID(sw.ID, "down")
	upMeter := vpcSwitchMeterID(sw.ID, "up")
	if downRateKbit > 0 {
		if HookSwitchUsesDirectBridge(sw) {
			if err := HookAddOVSBandwidthMeter(bridge, downMeter, downRateKbit); err != nil {
				return err
			}
		} else {
			HookApplyTCVPCSwitchDownlink(VPCGatewayPortName(sw.ID), downMbps)
		}
	}
	if upRateKbit > 0 {
		if err := HookAddOVSBandwidthMeter(bridge, upMeter, upRateKbit); err != nil {
			return err
		}
	}
	var flows []string
	var directVMPorts []vpcSwitchVMPortMatch
	if HookSwitchUsesDirectBridge(sw) {
		directVMPorts = listVPCSwitchVMPortMatches(sw)
		flows = buildDirectBridgeSwitchBandwidthFlows(sw, directVMPorts, downMeter, upMeter, downRateKbit, upRateKbit)
	} else {
		flows = buildVPCSwitchBandwidthFlows(sw, gatewayOfport, vmOfports, upMeter, downRateKbit, upRateKbit)
	}
	for _, flow := range flows {
		result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-flow", bridge, flow)
		if result.Error != nil {
			return fmt.Errorf("配置 VPC 交换机总带宽失败: %s", result.Stderr)
		}
	}
	if HookSwitchUsesDirectBridge(sw) {
		if err := applyDirectBridgePortSecurity(bridge, directVMPorts, sw.AllowPromiscuous); err != nil {
			return err
		}
	}
	return nil
}

// ReapplyAllVPCSwitchBandwidth 仅重建交换机带宽流表，不改动网桥、网关或虚拟机绑定。
func ReapplyAllVPCSwitchBandwidth() error {
	if model.DB == nil {
		return nil
	}
	var switches []model.VPCSwitch
	if err := model.DB.Order("id ASC").Find(&switches).Error; err != nil {
		return err
	}
	var lastErr error
	for _, sw := range switches {
		if err := ApplyVPCSwitchBandwidth(sw); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

type vpcSwitchVMPortMatch struct {
	PortName string
	OFPort   string
	MAC      string
}

func listVPCSwitchVMOfports(switchID uint) []string {
	if model.DB == nil {
		return nil
	}
	var bindings []model.VPCVMBinding
	model.DB.Where("switch_id = ?", switchID).Order("vm_name ASC").Find(&bindings)
	seen := map[string]bool{}
	ofports := make([]string, 0, len(bindings))
	interfacesByVM := map[string][]RuntimeInterface{}
	for _, binding := range bindings {
		ifaces, ok := interfacesByVM[binding.VMName]
		if !ok {
			ifaces = HookParseVirshDomiflist(utils.ExecCommand("virsh", "domiflist", binding.VMName).Stdout)
			interfacesByVM[binding.VMName] = ifaces
		}
		if binding.InterfaceOrder < 0 || binding.InterfaceOrder >= len(ifaces) {
			continue
		}
		ofport := HookGetOVSInterfaceOfPort(ifaces[binding.InterfaceOrder].Name)
		if ofport == "" || seen[ofport] {
			continue
		}
		seen[ofport] = true
		ofports = append(ofports, ofport)
	}
	sort.Strings(ofports)
	return ofports
}

func listVPCSwitchVMPortMatches(sw model.VPCSwitch) []vpcSwitchVMPortMatch {
	if model.DB == nil {
		return nil
	}
	seen := map[string]bool{}
	var matches []vpcSwitchVMPortMatch
	var bindings []model.VPCVMBinding
	model.DB.Where("switch_id = ?", sw.ID).Order("vm_name ASC, interface_order ASC").Find(&bindings)
	interfacesByVM := map[string][]RuntimeInterface{}
	appendInterface := func(iface RuntimeInterface) {
		vnetIF := strings.TrimSpace(iface.Name)
		mac := strings.ToLower(strings.TrimSpace(iface.MAC))
		ofport := HookGetOVSInterfaceOfPort(vnetIF)
		if ofport == "" || mac == "" || seen[ofport+"/"+mac] {
			return
		}
		seen[ofport+"/"+mac] = true
		matches = append(matches, vpcSwitchVMPortMatch{PortName: vnetIF, OFPort: ofport, MAC: mac})
	}
	for _, binding := range bindings {
		ifaces, ok := interfacesByVM[binding.VMName]
		if !ok {
			ifaces = HookParseVirshDomiflist(utils.ExecCommand("virsh", "domiflist", binding.VMName).Stdout)
			interfacesByVM[binding.VMName] = ifaces
		}
		if binding.InterfaceOrder >= 0 && binding.InterfaceOrder < len(ifaces) {
			appendInterface(ifaces[binding.InterfaceOrder])
		}
	}
	// 单一直通交换机允许纳管未落库的既有网卡，但按运行态网桥逐口匹配。
	if HookSwitchUsesDirectBridge(sw) && directBridgeSwitchCount(HookBridgeNameForSwitch(sw)) == 1 {
		for _, vmName := range HookListAllVMNames() {
			for _, iface := range HookParseVirshDomiflist(utils.ExecCommand("virsh", "domiflist", vmName).Stdout) {
				if iface.Source == HookBridgeNameForSwitch(sw) {
					appendInterface(iface)
				}
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].OFPort == matches[j].OFPort {
			return matches[i].MAC < matches[j].MAC
		}
		return matches[i].OFPort < matches[j].OFPort
	})
	return matches
}

func listVPCSwitchVMNames(sw model.VPCSwitch) []string {
	seen := map[string]bool{}
	var names []string
	var bindings []model.VPCVMBinding
	model.DB.Where("switch_id = ?", sw.ID).Order("vm_name ASC").Find(&bindings)
	for _, binding := range bindings {
		name := strings.TrimSpace(binding.VMName)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if HookSwitchUsesDirectBridge(sw) && directBridgeSwitchCount(HookBridgeNameForSwitch(sw)) == 1 {
		for _, name := range listDirectBridgeVMNames(HookBridgeNameForSwitch(sw)) {
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func directBridgeSwitchCount(bridgeName string) int64 {
	var count int64
	if strings.TrimSpace(bridgeName) == "" || model.DB == nil {
		return 0
	}
	model.DB.Model(&model.VPCSwitch{}).Where("bridge_mode = ? AND bridge_name = ?", BridgeModeDirect, bridgeName).Count(&count)
	return count
}

func listDirectBridgeVMNames(bridgeName string) []string {
	bridgeName = strings.TrimSpace(bridgeName)
	if bridgeName == "" {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, vmName := range HookListAllVMNames() {
		if vmUsesOVSBridge(vmName, bridgeName) && !seen[vmName] {
			seen[vmName] = true
			names = append(names, vmName)
		}
	}
	sort.Strings(names)
	return names
}

func vmUsesOVSBridge(vmName, bridgeName string) bool {
	for _, iface := range HookParseVirshDomiflist(utils.ExecCommand("virsh", "domiflist", vmName).Stdout) {
		if iface.Type == "bridge" && iface.Source == bridgeName {
			return true
		}
	}
	for _, args := range [][]string{{"dumpxml", vmName, "--inactive"}, {"dumpxml", vmName}} {
		result := utils.ExecCommand("virsh", args...)
		if result.Error == nil && firstOVSInterfaceUsesBridge(result.Stdout, bridgeName) {
			return true
		}
	}
	return false
}

func buildVPCSwitchBandwidthFlows(sw model.VPCSwitch, gatewayOfport string, vmOfports []string, upMeter uint32, downRateKbit, upRateKbit int) []string {
	cookie := vpcSwitchCookie(sw.ID)
	table := vpcBandwidthFlowTable()
	if upRateKbit > 0 {
		sort.Strings(vmOfports)
	}
	flows := []string{}
	if upRateKbit > 0 {
		for _, vmOfport := range vmOfports {
			if strings.TrimSpace(vmOfport) == "" {
				continue
			}
			flows = append(flows,
				fmt.Sprintf("cookie=%s,%spriority=90,in_port=%s,ip,nw_src=%s,nw_dst=%s,actions=NORMAL", cookie, table, vmOfport, sw.CIDR, sw.CIDR),
				fmt.Sprintf("cookie=%s,%spriority=80,in_port=%s,ip,nw_src=%s,actions=meter:%d,NORMAL", cookie, table, vmOfport, sw.CIDR, upMeter),
			)
		}
	}
	if downRateKbit > 0 {
		flows = append(flows, fmt.Sprintf("cookie=%s,%spriority=80,in_port=%s,ip,nw_dst=%s,actions=NORMAL", cookie, table, gatewayOfport, sw.CIDR))
	}
	return flows
}

func buildDirectBridgeSwitchBandwidthFlows(sw model.VPCSwitch, vmPorts []vpcSwitchVMPortMatch, downMeter, upMeter uint32, downRateKbit, upRateKbit int) []string {
	cookie := vpcSwitchCookie(sw.ID)
	table := vpcBandwidthFlowTable()
	var flows []string
	restrictSourceMAC := !sw.AllowMACChange || !sw.AllowForgedTransmits
	for _, item := range vmPorts {
		if strings.TrimSpace(item.OFPort) != "" {
			if restrictSourceMAC && strings.TrimSpace(item.MAC) != "" {
				action := "NORMAL"
				if upRateKbit > 0 {
					action = fmt.Sprintf("meter:%d,NORMAL", upMeter)
				}
				flows = append(flows, fmt.Sprintf("cookie=%s,%spriority=90,in_port=%s,dl_src=%s,actions=%s", cookie, table, item.OFPort, item.MAC, action))
				flows = append(flows, fmt.Sprintf("cookie=%s,%spriority=85,in_port=%s,actions=drop", cookie, table, item.OFPort))
			} else if upRateKbit > 0 {
				flows = append(flows, fmt.Sprintf("cookie=%s,%spriority=80,in_port=%s,actions=meter:%d,NORMAL", cookie, table, item.OFPort, upMeter))
			}
		}
		if downRateKbit > 0 && strings.TrimSpace(item.MAC) != "" {
			flows = append(flows, fmt.Sprintf("cookie=%s,%spriority=80,dl_dst=%s,actions=meter:%d,NORMAL", cookie, table, item.MAC, downMeter))
		}
	}
	return flows
}

func vpcBandwidthFlowTable() string {
	if config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled {
		return "table=30,"
	}
	return ""
}

func applyDirectBridgePortSecurity(bridge string, vmPorts []vpcSwitchVMPortMatch, allowPromiscuous bool) error {
	if strings.TrimSpace(bridge) == "" {
		return nil
	}
	if err := HookEnsureOVSBridgeExists(bridge); err != nil {
		return fmt.Errorf("配置桥接端口安全策略失败: %w", err)
	}
	mode := "no-flood"
	if allowPromiscuous {
		mode = "flood"
	}
	for _, item := range vmPorts {
		if strings.TrimSpace(item.PortName) == "" {
			continue
		}
		result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "mod-port", bridge, item.PortName, mode)
		if result.Error != nil {
			return fmt.Errorf("配置桥接端口混杂模式策略失败: %s", result.Stderr)
		}
	}
	return nil
}
