package bandwidth

import (
	"fmt"
	"strings"

	"github.com/digitalocean/go-libvirt"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/ip_resolver"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// ==================== OVS 带宽相关 ====================

// GetOVSInterfaceOfPort 获取 OVS 接口的 ofport 号
func GetOVSInterfaceOfPort(vnetIF string) string {
	if strings.TrimSpace(vnetIF) == "" {
		return ""
	}
	result := utils.ExecCommand("ovs-vsctl", "get", "Interface", strings.TrimSpace(vnetIF), "ofport")
	if result.Error != nil {
		return ""
	}
	ofport := strings.TrimSpace(result.Stdout)
	if ofport == "" || ofport == "-1" || ofport == "[]" {
		return ""
	}
	return ofport
}

func getVMBandwidthIP(vmName, mac string) string {
	if strings.TrimSpace(mac) != "" {
		if ip := HookGetOVSStaticIPByMAC(mac); ip != "" {
			return ip
		}
		// 补充查询所有 VPC 交换机的静态 MAC 绑定
		if allVpcHosts, err := HookListAllVPCStaticHosts(); err == nil {
			for _, host := range allVpcHosts {
				if strings.EqualFold(host.MAC, mac) {
					return host.IP
				}
			}
		}
		if ip := HookGetOVSLeaseIPByMAC(mac); ip != "" {
			return ip
		}
	}
	if host, ok := HookGetOVSStaticHostByVMName(vmName); ok {
		if strings.TrimSpace(host.MAC) == "" || strings.EqualFold(host.MAC, mac) {
			return host.IP
		}
	}
	// 仅在缺少 MAC 精确记录时回退 VM 名称，兼容旧版主网卡静态绑定。
	if allVpcHosts, err := HookListAllVPCStaticHosts(); err == nil {
		for _, host := range allVpcHosts {
			if strings.TrimSpace(host.VMName) == strings.TrimSpace(vmName) &&
				(strings.TrimSpace(host.MAC) == "" || strings.EqualFold(host.MAC, mac)) {
				return host.IP
			}
		}
	}
	return ""
}

func getVMBandwidthConfigRaw(vmName string) (VmBandwidthConfigRaw, error) {
	mac := ip_resolver.GetFirstVMMAC(vmName)
	if mac == "" {
		return VmBandwidthConfigRaw{}, fmt.Errorf("无法获取虚拟机 %s 的网卡 MAC 地址", vmName)
	}

	params, err := libvirt_rpc.GetInterfaceParametersRPC(vmName, mac, uint32(libvirt.DomainAffectConfig))
	if err != nil {
		return VmBandwidthConfigRaw{}, fmt.Errorf("获取速率限制失败: %w", err)
	}
	cfg := VmBandwidthConfigRaw{}
	for _, p := range params {
		switch p.Field {
		case libvirt.DomainBandwidthInAverage:
			if v, ok := p.Value.I.(int64); ok {
				cfg.InboundAvg = int(v)
			} else if v, ok := p.Value.I.(uint64); ok {
				cfg.InboundAvg = int(v)
			} else if v, ok := p.Value.I.(uint32); ok {
				cfg.InboundAvg = int(v)
			}
		case libvirt.DomainBandwidthInPeak:
			if v, ok := p.Value.I.(int64); ok {
				cfg.InboundPeak = int(v)
			} else if v, ok := p.Value.I.(uint64); ok {
				cfg.InboundPeak = int(v)
			} else if v, ok := p.Value.I.(uint32); ok {
				cfg.InboundPeak = int(v)
			}
		case libvirt.DomainBandwidthInBurst:
			if v, ok := p.Value.I.(int64); ok {
				cfg.InboundBurst = int(v)
			} else if v, ok := p.Value.I.(uint64); ok {
				cfg.InboundBurst = int(v)
			} else if v, ok := p.Value.I.(uint32); ok {
				cfg.InboundBurst = int(v)
			}
		case libvirt.DomainBandwidthOutAverage:
			if v, ok := p.Value.I.(int64); ok {
				cfg.OutboundAvg = int(v)
			} else if v, ok := p.Value.I.(uint64); ok {
				cfg.OutboundAvg = int(v)
			} else if v, ok := p.Value.I.(uint32); ok {
				cfg.OutboundAvg = int(v)
			}
		case libvirt.DomainBandwidthOutPeak:
			if v, ok := p.Value.I.(int64); ok {
				cfg.OutboundPeak = int(v)
			} else if v, ok := p.Value.I.(uint64); ok {
				cfg.OutboundPeak = int(v)
			} else if v, ok := p.Value.I.(uint32); ok {
				cfg.OutboundPeak = int(v)
			}
		case libvirt.DomainBandwidthOutBurst:
			if v, ok := p.Value.I.(int64); ok {
				cfg.OutboundBurst = int(v)
			} else if v, ok := p.Value.I.(uint64); ok {
				cfg.OutboundBurst = int(v)
			} else if v, ok := p.Value.I.(uint32); ok {
				cfg.OutboundBurst = int(v)
			}
		}
	}
	return cfg, nil
}

func buildOVSBandwidthFlows(cookie, ofport, vmIP, subnetCIDR string, downQueueID, upMeterID uint32, downRateBps, upRateKbit int) []string {
	table := ovsBandwidthFlowTable()
	var flows []string
	if upRateKbit > 0 {
		flows = append(flows,
			fmt.Sprintf("cookie=%s,%spriority=220,in_port=%s,ip,nw_src=%s,nw_dst=%s,actions=NORMAL", cookie, table, ofport, vmIP, subnetCIDR),
			fmt.Sprintf("cookie=%s,%spriority=120,in_port=%s,ip,nw_src=%s,actions=meter:%d,output:LOCAL", cookie, table, ofport, vmIP, upMeterID),
		)
	}
	if downRateBps > 0 {
		flows = append(flows,
			fmt.Sprintf("cookie=%s,%spriority=220,in_port=LOCAL,ip,nw_src=%s,nw_dst=%s,actions=NORMAL", cookie, table, subnetCIDR, vmIP),
			fmt.Sprintf("cookie=%s,%spriority=120,in_port=LOCAL,ip,nw_dst=%s,actions=set_queue:%d,output:%s,pop_queue", cookie, table, vmIP, downQueueID, ofport),
		)
	}
	return flows
}

func buildOVSVPCBandwidthFlows(cookie, vmOfport, gatewayOfport, vmIP, switchCIDR string, downQueueID, upMeterID uint32, downRateBps, upRateKbit int) []string {
	table := ovsBandwidthFlowTable()
	var flows []string
	if upRateKbit > 0 || downRateBps > 0 {
		flows = append(flows,
			fmt.Sprintf("cookie=%s,%spriority=220,in_port=%s,ip,nw_src=%s,nw_dst=%s,actions=NORMAL", cookie, table, vmOfport, switchCIDR, switchCIDR),
			fmt.Sprintf("cookie=%s,%spriority=220,in_port=%s,ip,nw_src=%s,nw_dst=%s,actions=NORMAL", cookie, table, gatewayOfport, switchCIDR, switchCIDR),
		)
	}
	if upRateKbit > 0 {
		flows = append(flows,
			fmt.Sprintf("cookie=%s,%spriority=120,in_port=%s,ip,nw_src=%s,actions=meter:%d,output:%s", cookie, table, vmOfport, vmIP, upMeterID, gatewayOfport),
		)
	}
	if downRateBps > 0 {
		flows = append(flows,
			fmt.Sprintf("cookie=%s,%spriority=120,in_port=%s,ip,nw_dst=%s,actions=set_queue:%d,output:%s,pop_queue", cookie, table, gatewayOfport, vmIP, downQueueID, vmOfport),
		)
	}
	return flows
}

func ovsBandwidthFlowTable() string {
	if config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled {
		return "table=30,"
	}
	return ""
}

// GetVPCSwitchForVM 获取 VM 所属的 VPC 交换机。
// 若 VM 绑定多个交换机，优先返回有 DHCP 子网的 NAT 交换机（CIDR 非空），
// 避免返回桥接直通交换机导致静态 IP / 带宽等操作指向错误的交换机。
func GetVPCSwitchForVM(vmName string) (*model.VPCSwitch, bool) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" || model.DB == nil {
		return nil, false
	}
	var bindings []model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).Order("interface_order ASC").Find(&bindings).Error; err != nil || len(bindings) == 0 {
		return HookInferVPCSwitchForVM(vmName)
	}
	// 优先找有 DHCP 子网的 NAT 交换机（CIDR 非空）
	for _, b := range bindings {
		var sw model.VPCSwitch
		if err := model.DB.First(&sw, b.SwitchID).Error; err != nil {
			continue
		}
		if strings.TrimSpace(sw.CIDR) != "" {
			return &sw, true
		}
	}
	// 回退：返回第一个绑定对应的交换机（可能是桥接直通）
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, bindings[0].SwitchID).Error; err != nil {
		return HookInferVPCSwitchForVM(vmName)
	}
	return &sw, true
}

func FindOVSUUIDs(table, vmName, direction string) []string {
	result := utils.ExecCommand("ovs-vsctl", "--bare", "--columns=_uuid", "find", table,
		"external-ids:kvm-console-vm="+vmName,
		"external-ids:kvm-console-direction="+direction)
	if result.Error != nil {
		return nil
	}
	return strings.Fields(result.Stdout)
}

func destroyOVSRecords(table string, uuids []string) {
	for _, uuid := range uuids {
		utils.ExecCommand("ovs-vsctl", "--if-exists", "destroy", table, uuid)
	}
}

func clearOVSBandwidthLimit(vmName, vnetIF string) {
	cookie := OvsBandwidthCookie(vmName)
	interfaces := listVMBandwidthInterfaces(vmName)
	bridges := map[string]bool{HookOvsBridgeName(): true}
	ports := map[string]bool{}
	if strings.TrimSpace(vnetIF) != "" {
		ports[strings.TrimSpace(vnetIF)] = true
	}
	for _, iface := range interfaces {
		ports[iface.Name] = true
		bridges[iface.Source] = true
	}
	for bridge := range bridges {
		if strings.TrimSpace(bridge) == "" {
			continue
		}
		utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridge, "cookie="+cookie+"/0xffffffffffffffff")
		utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, OvsBandwidthMeterArg(OvsBandwidthMeterID(vmName, "down")))
		utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, OvsBandwidthMeterArg(OvsBandwidthMeterID(vmName, "up")))
	}
	for port := range ports {
		utils.ExecCommand("ovs-vsctl", "--if-exists", "clear", "Port", port, "qos")
	}
	destroyOVSRecords("QoS", FindOVSUUIDs("QoS", vmName, "down"))
	destroyOVSRecords("Queue", FindOVSUUIDs("Queue", vmName, "down"))
	destroyOVSRecords("Queue", FindOVSUUIDs("Queue", vmName, "up"))
}

func setOVSPortQueue(port, vmName, direction string, queueID uint32, maxRateBps int) error {
	if strings.TrimSpace(port) == "" || maxRateBps <= 0 {
		return nil
	}
	queueKey := OvsBandwidthQueueKey(queueID)
	rateArg := fmt.Sprintf("other-config:max-rate=%d", maxRateBps)
	result := utils.ExecCommand("ovs-vsctl",
		"--", "--id=@q", "create", "Queue", rateArg,
		"external-ids:kvm-console-vm="+vmName,
		"external-ids:kvm-console-direction="+direction,
		"--", "--id=@qos", "create", "QoS", "type=linux-htb", "queues:"+queueKey+"=@q",
		"external-ids:kvm-console-vm="+vmName,
		"external-ids:kvm-console-direction="+direction,
		"--", "set", "Port", strings.TrimSpace(port), "qos=@qos")
	if result.Error != nil {
		return fmt.Errorf("配置 OVS 下行队列失败: %s", result.Stderr)
	}
	return nil
}

// AddOVSBandwidthMeter 在 OVS 交换机上添加 meter 限速规则
func AddOVSBandwidthMeter(bridge string, meterID uint32, rateKbit int) error {
	if rateKbit <= 0 {
		return nil
	}
	arg := fmt.Sprintf("meter=%d,kbps,band=type=drop,rate=%d", meterID, rateKbit)
	result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-meter", bridge, arg)
	if result.Error != nil {
		if strings.Contains(result.Stderr, "METER_EXISTS") {
			utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, fmt.Sprintf("meter=%d", meterID))
			result = utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-meter", bridge, arg)
			if result.Error != nil {
				return fmt.Errorf("配置 OVS 上行限速失败: %s", result.Stderr)
			}
			return nil
		}
		return fmt.Errorf("配置 OVS 上行限速失败: %s", result.Stderr)
	}
	return nil
}

func applyOVSBandwidthLimit(vmName string, iface vmBandwidthInterface, downAvg, upAvg int) error {
	mac := iface.MAC
	vnetIF := iface.Name
	clearTCBandwidthLimit(vnetIF)

	downRateBps := OvsBandwidthMaxRateBps(downAvg)
	upRateKbit := OvsBandwidthRateKbit(upAvg)
	if downRateBps <= 0 && upRateKbit <= 0 {
		return nil
	}

	vmOfport := GetOVSInterfaceOfPort(vnetIF)
	if vmOfport == "" {
		return fmt.Errorf("无法获取虚拟机 %s 的 OVS 端口号", vmName)
	}

	bridge := strings.TrimSpace(iface.Source)
	if bridge == "" {
		bridge = HookOvsBridgeName()
	}
	sw, hasSwitch := getVPCSwitchForVMInterface(vmName, iface.Order)
	directBridge := hasSwitch && strings.TrimSpace(sw.CIDR) == ""
	vmIP := ""
	if !directBridge {
		vmIP = getVMBandwidthIP(vmName, mac)
	}
	macOnlyMatch := directBridge || vmIP == ""
	downQueueID := OvsBandwidthQueueID(vmName, "down")
	upMeterID := OvsBandwidthMeterID(vmName, "up")
	if err := setOVSPortQueue(vnetIF, vmName, "down", downQueueID, downRateBps); err != nil {
		return err
	}
	if err := AddOVSBandwidthMeter(bridge, upMeterID, upRateKbit); err != nil {
		return err
	}

	var flows []string
	if hasSwitch && !directBridge && !macOnlyMatch {
		gatewayOfport := GetOVSInterfaceOfPort(HookVPCGatewayPortName(sw.ID))
		if gatewayOfport == "" {
			return fmt.Errorf("无法获取虚拟机 %s 的 VPC 网关端口号", vmName)
		}
		flows = buildOVSVPCBandwidthFlows(OvsBandwidthCookie(vmName), vmOfport, gatewayOfport, vmIP, sw.CIDR, downQueueID, upMeterID, downRateBps, upRateKbit)
	} else if bridge == HookOvsBridgeName() && !macOnlyMatch {
		flows = buildOVSBandwidthFlows(OvsBandwidthCookie(vmName), vmOfport, vmIP, HookOvsSubnetCIDR(), downQueueID, upMeterID, downRateBps, upRateKbit)
	} else {
		if upRateKbit > 0 {
			flows = append(flows, fmt.Sprintf("cookie=%s,%spriority=120,in_port=%s,dl_src=%s,actions=meter:%d,NORMAL", OvsBandwidthCookie(vmName), ovsBandwidthFlowTable(), vmOfport, mac, upMeterID))
		}
		if downRateBps > 0 {
			flows = append(flows, fmt.Sprintf("cookie=%s,%spriority=120,dl_dst=%s,actions=set_queue:%d,NORMAL,pop_queue", OvsBandwidthCookie(vmName), ovsBandwidthFlowTable(), mac, downQueueID))
		}
	}
	for _, flow := range flows {
		result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-flow", bridge, flow)
		if result.Error != nil {
			return fmt.Errorf("添加 OVS 外网限速流表失败: %s", result.Stderr)
		}
	}
	return nil
}

func getVPCSwitchForVMInterface(vmName string, interfaceOrder int) (*model.VPCSwitch, bool) {
	if model.DB == nil {
		return GetVPCSwitchForVM(vmName)
	}
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", vmName, interfaceOrder).First(&binding).Error; err != nil {
		return GetVPCSwitchForVM(vmName)
	}
	var sw model.VPCSwitch
	if err := model.DB.First(&sw, binding.SwitchID).Error; err != nil {
		return GetVPCSwitchForVM(vmName)
	}
	return &sw, true
}

// ApplyVMNICBandwidth 设置单台 VM 的网卡口径速率限制。
// 该路径用于轻量云：不依赖 VPC 网关、IP 租约或 OVS 流表命中，直接在 VM 的 vnet 口限制上下行。
// domiftune 只保存 config，运行态使用 TC/IFB；低速惩罚时叠加 live domiftune 容易因为大 burst 产生卡顿。
func ApplyVMNICBandwidth(vmName string, downAvg, downPeak, downBurst, upAvg, upPeak, upBurst int) error {
	interfaces := listVMBandwidthInterfaces(vmName)
	if len(interfaces) == 0 {
		return fmt.Errorf("无法获取虚拟机 %s 的网卡 MAC 地址", vmName)
	}

	configParams := BuildBandwidthParams(downAvg, downPeak, downBurst, upAvg, upPeak, upBurst)

	for _, iface := range interfaces {
		if err := libvirt_rpc.SetInterfaceParametersRPC(vmName, iface.MAC, configParams, uint32(libvirt.DomainAffectConfig)); err != nil {
			return fmt.Errorf("设置第 %d 张网卡速率限制失败(config): %w", iface.Order+1, err)
		}
	}

	state, _ := libvirt_rpc.GetDomainStateRPC(vmName)
	if state == "running" {
		zeroParams := BuildBandwidthParams(0, 0, 0, 0, 0, 0)
		clearOVSBandwidthLimit(vmName, "")
		for _, iface := range interfaces {
			if err := libvirt_rpc.SetInterfaceParametersRPC(vmName, iface.MAC, zeroParams, uint32(libvirt.DomainAffectLive)); err != nil {
				logger.App.Warn("清理实时domiftune速率限制失败", "vm", vmName, "order", iface.Order, "error", err)
			}
			if iface.Name != "" {
				applyTCDownloadLimit(iface.Name, downAvg, downPeak, downBurst)
				applyTCUploadLimit(iface.Name, upAvg)
			}
		}
	}

	return nil
}

// GetVMBandwidth 获取 VM 当前的速率限制配置
func GetVMBandwidth(vmName string) (*BandwidthDetail, error) {
	config, err := getVMBandwidthConfigRaw(vmName)
	if err != nil {
		return nil, err
	}
	detail := &BandwidthDetail{
		InboundAvg:    KBpsToMbps(config.InboundAvg),
		InboundPeak:   KBpsToMbps(config.InboundPeak),
		InboundBurst:  config.InboundBurst,
		OutboundAvg:   KBpsToMbps(config.OutboundAvg),
		OutboundPeak:  KBpsToMbps(config.OutboundPeak),
		OutboundBurst: config.OutboundBurst,
	}
	return detail, nil
}
