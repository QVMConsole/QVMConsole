package portsecurity

import (
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"kvm_console/config"
	"kvm_console/utils"
)

func assignMeterIDs(ports []policyPort, capabilities map[string]BridgeCapability) error {
	portsByBridge := make(map[string][]*policyPort)
	for i := range ports {
		portsByBridge[ports[i].Bridge] = append(portsByBridge[ports[i].Bridge], &ports[i])
	}
	for bridgeName, bridgePorts := range portsByBridge {
		capability := capabilities[bridgeName]
		upper := capability.MaxMeters - 1
		if upper > 99999 {
			upper = 99999
		}
		if upper < 2000 {
			return fmt.Errorf("OVS 网桥 %s 可用 meter ID 空间不足", bridgeName)
		}
		used := existingMeterIDs(bridgeName)
		// 当前端口已登记的本模块 meter 可原位复用，避免每次协调漂移并遗留旧 meter。
		for _, port := range bridgePorts {
			for _, key := range []string{ExternalIDNeighbor, ExternalIDBroadcast} {
				result := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port.Port, "external_ids:"+key)
				value := strings.Trim(strings.TrimSpace(result.Stdout), "\"")
				if id, err := strconv.ParseUint(value, 10, 32); err == nil {
					delete(used, uint32(id))
				}
			}
		}
		sort.Slice(bridgePorts, func(i, j int) bool { return bridgePorts[i].Port < bridgePorts[j].Port })
		for _, port := range bridgePorts {
			if port.Isolated || port.VMName == "" || port.MAC == "" {
				continue
			}
			var ok bool
			port.NeighborMeterID, ok = allocateMeterID(bridgeName+"\x00"+port.Port+"\x00neighbor", upper, used)
			if !ok {
				return fmt.Errorf("OVS 网桥 %s 没有可用的邻居协议 meter ID", bridgeName)
			}
			port.BroadcastMeterID, ok = allocateMeterID(bridgeName+"\x00"+port.Port+"\x00broadcast", upper, used)
			if !ok {
				return fmt.Errorf("OVS 网桥 %s 没有可用的广播 meter ID", bridgeName)
			}
		}
	}
	return nil
}

func existingMeterIDs(bridge string) map[uint32]bool {
	used := make(map[uint32]bool)
	result := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-meters", bridge)
	if result.Error != nil {
		return used
	}
	pattern := regexp.MustCompile(`(?m)meter=([0-9]+)`)
	for _, match := range pattern.FindAllStringSubmatch(result.Stdout, -1) {
		if len(match) == 2 {
			if id, err := strconv.ParseUint(match[1], 10, 32); err == nil {
				used[uint32(id)] = true
			}
		}
	}
	return used
}

func allocateMeterID(key string, upper int, used map[uint32]bool) (uint32, bool) {
	const lower = 1000
	span := uint32(upper - lower + 1)
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	id := uint32(lower) + h.Sum32()%span
	for attempts := uint32(0); attempts < span && used[id]; attempts++ {
		id++
		if id > uint32(upper) {
			id = lower
		}
	}
	if used[id] {
		return 0, false
	}
	used[id] = true
	return id, true
}

func buildBridgeFlows(ports []policyPort) map[string][]string {
	flows := make(map[string][]string)
	for _, port := range ports {
		bridgeFlows := flows[port.Bridge]
		if port.Isolated || port.VMName == "" || port.MAC == "" {
			bridgeFlows = append(bridgeFlows, fmt.Sprintf("cookie=%s,table=0,priority=450,in_port=%s,actions=drop", PolicyCookie, port.OFPort))
			flows[port.Bridge] = bridgeFlows
			continue
		}
		bridgeFlows = append(bridgeFlows, fmt.Sprintf("cookie=%s,table=0,priority=400,in_port=%s,actions=goto_table:%d", PolicyCookie, port.OFPort, TableIdentity))
		bridgeFlows = append(bridgeFlows, buildIdentityFlows(port)...)
		bridgeFlows = append(bridgeFlows, buildRateLimitFlows(port)...)
		flows[port.Bridge] = bridgeFlows
	}
	for bridgeName := range flows {
		flows[bridgeName] = append(flows[bridgeName], fmt.Sprintf("cookie=%s,table=%d,priority=0,actions=NORMAL", PolicyCookie, TableBandwidth))
		sort.Strings(flows[bridgeName])
	}
	return flows
}

func buildIdentityFlows(port policyPort) []string {
	in := port.OFPort
	mac := port.MAC
	next := fmt.Sprintf("goto_table:%d", TableRateLimit)
	flow := func(priority int, match, actions string) string {
		return fmt.Sprintf("cookie=%s,table=%d,priority=%d,in_port=%s,%s,actions=%s", PolicyCookie, TableIdentity, priority, in, match, actions)
	}
	var flows []string
	// DHCP 客户端在获得地址前也需要通过；虚拟机发出的 DHCP 服务端报文始终丢弃。
	flows = append(flows,
		flow(370, "udp,tp_src=67,tp_dst=68", "drop"),
		flow(360, "udp,dl_src="+mac+",tp_src=68,tp_dst=67", next),
	)

	if port.StrictIPv4 {
		allowed := append([]string{"0.0.0.0"}, port.AllowedIPv4Addresses...)
		for _, address := range uniqueStrings(allowed) {
			flows = append(flows, flow(340, "arp,dl_src="+mac+",arp_sha="+mac+",arp_spa="+address, next))
		}
		for _, address := range uniqueStrings(port.AllowedIPv4Addresses) {
			flows = append(flows, flow(330, "ip,dl_src="+mac+",nw_src="+address, next))
		}
	} else {
		flows = append(flows,
			flow(340, "arp,dl_src="+mac+",arp_sha="+mac, next),
			flow(330, "ip,dl_src="+mac, next),
		)
	}
	flows = append(flows, flow(310, "arp", "drop"))
	if port.StrictIPv4 {
		flows = append(flows, flow(300, "ip", "drop"))
	}

	// IPv6 仅在直通桥显式配置可信前缀及精确地址时开放。
	flows = append(flows,
		flow(390, "udp6,tp_src=547,tp_dst=546", "drop"),
		flow(388, "icmp6,icmp_type=134", "drop"),
		flow(387, "icmp6,icmp_type=137", "drop"),
	)
	if port.IPv6Enabled {
		flows = append(flows, flow(380, "udp6,dl_src="+mac+",tp_src=546,tp_dst=547", next))
		for _, address := range uniqueStrings(port.AllowedIPv6Addresses) {
			flows = append(flows,
				flow(375, "icmp6,icmp_type=135,dl_src="+mac+",ipv6_src=::,nd_target="+address, next),
				flow(365, "icmp6,icmp_type=135,dl_src="+mac+",ipv6_src="+address+",nd_sll="+mac, next),
				flow(365, "icmp6,icmp_type=136,dl_src="+mac+",ipv6_src="+address+",nd_target="+address+",nd_tll="+mac, next),
				flow(350, "ipv6,dl_src="+mac+",ipv6_src="+address, next),
			)
		}
		flows = append(flows,
			flow(360, "icmp6,icmp_type=135", "drop"),
			flow(360, "icmp6,icmp_type=136", "drop"),
			flow(320, "ipv6", "drop"),
		)
	} else {
		flows = append(flows, flow(320, "ipv6", "drop"))
	}

	// 允许同一 MAC 的其他二层控制协议，所有伪造源 MAC 最终命中端口默认丢弃。
	flows = append(flows,
		flow(100, "dl_src="+mac, next),
		fmt.Sprintf("cookie=%s,table=%d,priority=1,in_port=%s,actions=drop", PolicyCookie, TableIdentity, in),
	)
	return flows
}

func buildRateLimitFlows(port policyPort) []string {
	in := port.OFPort
	next := fmt.Sprintf("goto_table:%d", TableBandwidth)
	neighbor := fmt.Sprintf("meter:%d,%s", port.NeighborMeterID, next)
	broadcast := fmt.Sprintf("meter:%d,%s", port.BroadcastMeterID, next)
	flow := func(priority int, match, actions string) string {
		return fmt.Sprintf("cookie=%s,table=%d,priority=%d,in_port=%s,%s,actions=%s", PolicyCookie, TableRateLimit, priority, in, match, actions)
	}
	return []string{
		flow(300, "arp", neighbor),
		flow(290, "icmp6,icmp_type=133", neighbor),
		flow(290, "icmp6,icmp_type=135", neighbor),
		flow(290, "icmp6,icmp_type=136", neighbor),
		flow(200, "dl_dst=01:00:00:00:00:00/01:00:00:00:00:00", broadcast),
		fmt.Sprintf("cookie=%s,table=%d,priority=1,in_port=%s,actions=%s", PolicyCookie, TableRateLimit, in, next),
	}
}

func applyPortMeters(port policyPort) error {
	cfg := config.GlobalConfig
	if cfg == nil {
		return fmt.Errorf("端口安全配置尚未初始化")
	}
	if err := replacePacketMeter(port.Bridge, port.NeighborMeterID, cfg.PortSecurityNeighborPPS, cfg.PortSecurityNeighborBurstPackets); err != nil {
		return err
	}
	if err := replacePacketMeter(port.Bridge, port.BroadcastMeterID, cfg.PortSecurityBroadcastPPS, cfg.PortSecurityBroadcastBurstPackets); err != nil {
		return err
	}
	return nil
}

func replacePacketMeter(bridge string, meterID uint32, rate, burst int) error {
	utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, "meter="+strconv.FormatUint(uint64(meterID), 10))
	arg := fmt.Sprintf("meter=%d,pktps,burst,stats,band=type=drop,rate=%d,burst_size=%d", meterID, rate, burst)
	result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-meter", bridge, arg)
	if result.Error != nil {
		return fmt.Errorf("配置端口 %s 的报文速率 meter 失败: %s", bridge, firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	return nil
}

func setPortPolicing(port policyPort) error {
	cfg := config.GlobalConfig
	result := utils.ExecCommand("ovs-vsctl", "set", "Interface", port.Port,
		"ingress_policing_kpkts_rate="+strconv.Itoa(cfg.PortSecurityTotalKpps),
		"ingress_policing_kpkts_burst="+strconv.Itoa(cfg.PortSecurityTotalBurstKPackets))
	if result.Error != nil {
		return fmt.Errorf("配置端口 %s 总包速率失败: %s", port.Port, firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	return nil
}

func applyBridgeFlows(bridge string, ports []policyPort, flows []string, useBundle bool) error {
	file, err := os.CreateTemp("", "qvm-port-security-flows-*.txt")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if useBundle {
		bundleFlows := []string{
			"delete cookie=" + PolicyCookie + "/0xffffffffffffffff",
			"delete cookie=" + QuarantineCookie + "/0xffffffffffffffff",
		}
		for _, flow := range flows {
			bundleFlows = append(bundleFlows, "add "+flow)
		}
		if _, err = file.WriteString(strings.Join(bundleFlows, "\n") + "\n"); err != nil {
			_ = file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		result := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow14", "--bundle", "add-flows", bridge, path)
		if result.Error == nil {
			if err := verifyBridgeFlows(bridge); err != nil {
				return err
			}
			return nil
		}
		// OpenFlow 1.4 协商成功但 bundle 被交换机拒绝时，继续走隔离保护的兼容流程。
		file, err = os.CreateTemp("", "qvm-port-security-flows-fallback-*.txt")
		if err != nil {
			return err
		}
		path = file.Name()
		defer os.Remove(path)
	}
	// 先以更高优先级隔离目标端口，再更新正式策略，兼容缺少 OpenFlow bundle 的旧版宿主机。
	for _, port := range ports {
		result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-flow", bridge,
			fmt.Sprintf("cookie=%s,table=0,priority=500,in_port=%s,actions=drop", QuarantineCookie, port.OFPort))
		if result.Error != nil {
			return fmt.Errorf("隔离端口 %s 失败: %s", port.Port, firstNonEmpty(result.Stderr, result.Error.Error()))
		}
	}
	utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridge, "cookie="+PolicyCookie+"/0xffffffffffffffff")
	if _, err = file.WriteString(strings.Join(flows, "\n") + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-flows", bridge, path)
	if result.Error != nil {
		return fmt.Errorf("应用网桥 %s 端口安全流表失败: %s", bridge, firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	if err := verifyBridgeFlows(bridge); err != nil {
		return err
	}
	utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridge, "cookie="+QuarantineCookie+"/0xffffffffffffffff")
	return nil
}

func verifyBridgeFlows(bridge string) error {
	verify := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", bridge, "cookie="+PolicyCookie+"/0xffffffffffffffff")
	if verify.Error != nil || !strings.Contains(strings.ToLower(verify.Stdout), strings.TrimPrefix(strings.ToLower(PolicyCookie), "0x")) {
		return fmt.Errorf("网桥 %s 端口安全流表校验失败", bridge)
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "未知错误"
}
