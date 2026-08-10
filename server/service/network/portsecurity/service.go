package portsecurity

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	bridgepkg "kvm_console/service/network/bridge"
	"kvm_console/utils"
)

var (
	reconcileMu      sync.Mutex
	statusMu         sync.RWMutex
	lastStatus       = Status{Healthy: true, Ports: []PortStatus{}, Issues: []Issue{}}
	reconcileTrigger = make(chan struct{}, 1)
	startOnce        sync.Once
	portNamePattern  = regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`)
	pendingWatchers  sync.Map
)

// Preflight 检查开启端口安全所需的 OVS 能力和端口身份资料。
func Preflight() (*PreflightResult, error) {
	return preflight("")
}

func preflight(allowPendingVM string) (*PreflightResult, error) {
	ports, issues, err := collectPolicyPorts()
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []Issue{}
	}
	if allowPendingVM != "" {
		filtered := issues[:0]
		for _, issue := range issues {
			if issue.Code == "missing_ipv4_address" && issue.VMName == allowPendingVM {
				continue
			}
			filtered = append(filtered, issue)
		}
		issues = filtered
	}
	bridgeSet := make(map[string]bool)
	for _, port := range ports {
		bridgeSet[port.Bridge] = true
	}
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.OVSBridge) != "" {
		bridgeSet[strings.TrimSpace(config.GlobalConfig.OVSBridge)] = true
	}
	var switches []model.VPCSwitch
	if model.DB != nil {
		_ = model.DB.Find(&switches).Error
		for _, sw := range switches {
			bridgeSet[bridgepkg.BridgeNameForSwitch(sw)] = true
		}
		issues = append(issues, validateStoredIPv6Configuration(switches)...)
	}
	bridges := make([]string, 0, len(bridgeSet))
	for bridgeName := range bridgeSet {
		if bridgeName != "" {
			bridges = append(bridges, bridgeName)
		}
	}
	sort.Strings(bridges)
	capabilities := make([]BridgeCapability, 0, len(bridges))
	for _, bridgeName := range bridges {
		capability := inspectBridgeCapability(bridgeName)
		for _, port := range ports {
			if port.Bridge == bridgeName && !port.Isolated && port.VMName != "" && port.MAC != "" {
				capability.RequiredMeters += 2
			}
		}
		if !capability.Exists {
			issues = append(issues, Issue{Code: "bridge_missing", Message: "OVS 网桥不存在", Bridge: bridgeName, Blocking: true})
		}
		if !capability.OpenFlow13 {
			issues = append(issues, Issue{Code: "openflow13_missing", Message: "OVS 网桥缺少 OpenFlow13 支持", Bridge: bridgeName, Blocking: true})
		}
		allocatableMeters := capability.MaxMeters - 1000
		if allocatableMeters > 99000 {
			allocatableMeters = 99000
		}
		if !capability.PacketMeters || allocatableMeters < capability.RequiredMeters || capability.ExistingMeters+capability.RequiredMeters > capability.MaxMeters {
			issues = append(issues, Issue{Code: "meter_capacity", Message: fmt.Sprintf("OVS meter 容量不足：需要 %d，最大 ID %d", capability.RequiredMeters, capability.MaxMeters), Bridge: bridgeName, Blocking: true})
		}
		if !capability.PacketPolicing {
			issues = append(issues, Issue{Code: "packet_policing_missing", Message: "OVS Interface 缺少包速率 policing 字段", Bridge: bridgeName, Blocking: true})
		}
		capabilities = append(capabilities, capability)
	}
	ready := true
	for _, issue := range issues {
		if issue.Blocking {
			ready = false
			break
		}
	}
	result := &PreflightResult{
		Ready: ready, Enabled: config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled,
		Capabilities: capabilities, Ports: exportPorts(ports), Issues: issues, CheckedAt: time.Now(),
	}
	return result, nil
}

func summarizeBlockingIssues(issues []Issue) string {
	const maxDetails = 3
	details := make([]string, 0, maxDetails)
	total := 0
	for _, issue := range issues {
		if !issue.Blocking {
			continue
		}
		total++
		if len(details) >= maxDetails {
			continue
		}
		location := ""
		switch {
		case issue.VMName != "":
			location = fmt.Sprintf("%s / 网卡 %d", issue.VMName, issue.InterfaceOrder+1)
		case issue.Bridge != "":
			location = issue.Bridge
		case issue.Port != "":
			location = issue.Port
		}
		if location != "" {
			details = append(details, location+": "+issue.Message)
		} else {
			details = append(details, issue.Message)
		}
	}
	if len(details) == 0 {
		return "存在未知阻断项"
	}
	if total > len(details) {
		details = append(details, fmt.Sprintf("另有 %d 项", total-len(details)))
	}
	return strings.Join(details, "; ")
}

func inspectBridgeCapability(bridgeName string) BridgeCapability {
	capability := BridgeCapability{Bridge: bridgeName, SequentialApplyGuard: true}
	capability.Exists = utils.ExecCommandQuiet("ovs-vsctl", "br-exists", bridgeName).Error == nil
	if !capability.Exists {
		return capability
	}
	capability.OpenFlow13 = utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", bridgeName).Error == nil
	capability.OpenFlow14Bundle = utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow14", "dump-flows", bridgeName).Error == nil
	meter := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "meter-features", bridgeName)
	if meter.Error == nil {
		lower := strings.ToLower(meter.Stdout)
		capability.PacketMeters = strings.Contains(lower, "pktps") && strings.Contains(lower, "burst")
		match := regexp.MustCompile(`(?i)max_meter\s*:\s*([0-9]+)`).FindStringSubmatch(meter.Stdout)
		if len(match) == 2 {
			capability.MaxMeters, _ = strconv.Atoi(match[1])
		}
	}
	dumpMeters := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-meters", bridgeName)
	if dumpMeters.Error == nil {
		capability.ExistingMeters = len(regexp.MustCompile(`(?m)^meter=`).FindAllStringIndex(dumpMeters.Stdout, -1))
	}
	columns := utils.ExecCommandQuiet("ovsdb-client", "list-columns", "Open_vSwitch", "Interface")
	capability.PacketPolicing = columns.Error == nil && strings.Contains(columns.Stdout, "ingress_policing_kpkts_rate") && strings.Contains(columns.Stdout, "ingress_policing_kpkts_burst")
	return capability
}

func validateStoredIPv6Configuration(switches []model.VPCSwitch) []Issue {
	if model.DB == nil {
		return nil
	}
	byID := make(map[uint]model.VPCSwitch, len(switches))
	var issues []Issue
	for _, sw := range switches {
		byID[sw.ID] = sw
		if !sw.IPv6SecurityEnabled {
			continue
		}
		if !bridgepkg.SwitchUsesDirectBridge(sw) {
			issues = append(issues, Issue{Code: "ipv6_mode_invalid", Message: "IPv6 防护仅适用于直通桥交换机", Bridge: bridgepkg.BridgeNameForSwitch(sw), Blocking: true})
		}
		if len(parsePrefixList(sw.TrustedIPv6Prefixes)) == 0 {
			issues = append(issues, Issue{Code: "missing_ipv6_prefix", Message: "直通桥已启用 IPv6 防护，但可信 IPv6 前缀为空", Bridge: bridgepkg.BridgeNameForSwitch(sw), Blocking: true})
		}
	}
	var bindings []model.VPCVMBinding
	_ = model.DB.Order("vm_name ASC, interface_order ASC").Find(&bindings).Error
	for _, binding := range bindings {
		sw, ok := byID[binding.SwitchID]
		if !ok || !sw.IPv6SecurityEnabled {
			continue
		}
		addresses := parseAddressList(binding.AllowedIPv6Addresses, true)
		if len(addresses) == 0 {
			issues = append(issues, Issue{Code: "missing_ipv6_address", Message: "网卡未登记可信 IPv6 地址", Bridge: bridgepkg.BridgeNameForSwitch(sw), VMName: binding.VMName, InterfaceOrder: binding.InterfaceOrder, Blocking: true})
			continue
		}
		prefixes := parsePrefixList(sw.TrustedIPv6Prefixes)
		for _, address := range addresses {
			if !addressInPrefixes(address, prefixes) {
				issues = append(issues, Issue{Code: "ipv6_outside_prefix", Message: fmt.Sprintf("IPv6 地址 %s 不属于交换机可信前缀", address), Bridge: bridgepkg.BridgeNameForSwitch(sw), VMName: binding.VMName, InterfaceOrder: binding.InterfaceOrder, Blocking: true})
			}
		}
	}
	return issues
}

// Enable 通过预检后开启总开关并应用完整策略。
func Enable(progress func(int, string)) (*Status, error) {
	if progress != nil {
		progress(5, "正在预检端口安全环境...")
	}
	preflight, err := Preflight()
	if err != nil {
		return nil, err
	}
	if !preflight.Ready {
		return nil, fmt.Errorf("端口安全预检未通过: %s", summarizeBlockingIssues(preflight.Issues))
	}
	previous := config.GlobalConfig.PortSecurityEnabled
	config.GlobalConfig.PortSecurityEnabled = true
	if progress != nil {
		progress(30, "正在隔离端口并安装身份策略...")
	}
	status, err := Reconcile()
	if err != nil {
		config.GlobalConfig.PortSecurityEnabled = previous
		_ = DisableRuntime()
		return nil, err
	}
	if err := persistEnabled(true); err != nil {
		config.GlobalConfig.PortSecurityEnabled = previous
		_ = DisableRuntime()
		return nil, err
	}
	if progress != nil {
		progress(100, "端口安全防护已启用")
	}
	return status, nil
}

// Disable 关闭总开关并仅清理本模块拥有的运行态资源。
func Disable(progress func(int, string)) (*Status, error) {
	if progress != nil {
		progress(20, "正在清理端口安全流表和速率策略...")
	}
	reconcileMu.Lock()
	if err := disableRuntimeLocked(); err != nil {
		reconcileMu.Unlock()
		return nil, err
	}
	config.GlobalConfig.PortSecurityEnabled = false
	if err := persistEnabled(false); err != nil {
		config.GlobalConfig.PortSecurityEnabled = true
		reconcileMu.Unlock()
		TriggerReconcile()
		return nil, err
	}
	reconcileMu.Unlock()
	status, err := GetStatus()
	if progress != nil {
		progress(100, "端口安全防护已关闭")
	}
	return status, err
}

func persistEnabled(enabled bool) error {
	if err := model.SetSetting("port_security_enabled", strconv.FormatBool(enabled)); err != nil {
		return fmt.Errorf("持久化端口安全开关失败: %w", err)
	}
	config.SyncEnvFile()
	return nil
}

// Reconcile 根据实时 libvirt/OVS 端口重新编译并应用全部策略。
func Reconcile() (*Status, error) {
	return reconcile("")
}

func reconcile(allowPendingVM string) (*Status, error) {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	if config.GlobalConfig == nil || !config.GlobalConfig.PortSecurityEnabled {
		return GetStatus()
	}
	preflight, err := preflight(allowPendingVM)
	if err != nil {
		return nil, err
	}
	if !preflight.Ready {
		return nil, fmt.Errorf("端口安全协调预检未通过: %s", summarizeBlockingIssues(preflight.Issues))
	}
	ports, issues, err := collectPolicyPorts()
	if err != nil {
		return nil, err
	}
	capabilityMap := make(map[string]BridgeCapability)
	for _, item := range preflight.Capabilities {
		capabilityMap[item.Bridge] = item
	}
	if err := assignMeterIDs(ports, capabilityMap); err != nil {
		return nil, err
	}
	portsByBridge := make(map[string][]policyPort)
	for i := range ports {
		port := &ports[i]
		// 先记录归属和 meter 映射，后续任一步失败时可按归属精确回收。
		if err := writePortMetadata(*port); err != nil {
			port.LastError = err.Error()
			return nil, err
		}
		if !port.Isolated && port.VMName != "" && port.MAC != "" {
			if err := applyPortMeters(*port); err != nil {
				port.LastError = err.Error()
				return nil, err
			}
		}
		if err := setPortPolicing(*port); err != nil {
			port.LastError = err.Error()
			return nil, err
		}
		portsByBridge[port.Bridge] = append(portsByBridge[port.Bridge], *port)
	}
	bridgeFlows := buildBridgeFlows(ports)
	for bridgeName, flows := range bridgeFlows {
		if err := applyBridgeFlows(bridgeName, portsByBridge[bridgeName], flows, capabilityMap[bridgeName].OpenFlow14Bundle); err != nil {
			issues = append(issues, Issue{Code: "flow_apply_failed", Message: err.Error(), Bridge: bridgeName, Blocking: true})
			updateCachedStatus(ports, issues, true, true)
			return nil, err
		}
		if err := syncBridgeMeterMap(bridgeName, portsByBridge[bridgeName]); err != nil {
			issues = append(issues, Issue{Code: "meter_map_failed", Message: err.Error(), Bridge: bridgeName, Blocking: false})
		}
	}
	for i := range ports {
		ports[i].Applied = true
		if err := writePortMetadata(ports[i]); err != nil {
			ports[i].LastError = err.Error()
			issues = append(issues, Issue{Code: "metadata_failed", Message: err.Error(), Bridge: ports[i].Bridge, Port: ports[i].Port, VMName: ports[i].VMName, Blocking: false})
		}
	}
	status := updateCachedStatus(ports, issues, true, true)
	return &status, nil
}

// DisableRuntime 清理本模块的流表、meter、包速率字段和元数据。
func DisableRuntime() error {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	return disableRuntimeLocked()
}

func disableRuntimeLocked() error {
	ports, _, collectErr := collectPolicyPorts()
	if collectErr != nil {
		return fmt.Errorf("停用前读取端口清单失败: %w", collectErr)
	}
	bridges := make(map[string]bool)
	portsByBridge := make(map[string][]policyPort)
	for _, port := range ports {
		bridges[port.Bridge] = true
		portsByBridge[port.Bridge] = append(portsByBridge[port.Bridge], port)
	}
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.OVSBridge) != "" {
		bridges[config.GlobalConfig.OVSBridge] = true
	}
	var switches []model.VPCSwitch
	if model.DB != nil {
		_ = model.DB.Find(&switches).Error
		for _, sw := range switches {
			bridges[bridgepkg.BridgeNameForSwitch(sw)] = true
		}
	}

	// 仅在检测到本模块流表时临时隔离相关端口；默认关闭且无残留规则时不触碰现有转发。
	ownedFlowBridges := make(map[string]bool)
	for bridgeName := range bridges {
		dump := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", bridgeName, "cookie="+PolicyCookie+"/0xffffffffffffffff")
		if dump.Error == nil && strings.Contains(strings.ToLower(dump.Stdout), strings.TrimPrefix(strings.ToLower(PolicyCookie), "0x")) {
			ownedFlowBridges[bridgeName] = true
		}
	}
	for bridgeName := range ownedFlowBridges {
		for _, port := range portsByBridge[bridgeName] {
			result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-flow", bridgeName,
				fmt.Sprintf("cookie=%s,table=0,priority=500,in_port=%s,actions=drop", QuarantineCookie, port.OFPort))
			if result.Error != nil {
				return fmt.Errorf("停用前隔离端口 %s 失败: %s", port.Port, firstNonEmpty(result.Stderr, result.Error.Error()))
			}
		}
	}
	for bridgeName := range ownedFlowBridges {
		result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridgeName, "cookie="+PolicyCookie+"/0xffffffffffffffff")
		if result.Error != nil {
			markPortsCleanupFailed(portsByBridge[bridgeName])
			return fmt.Errorf("移除网桥 %s 端口安全流表失败: %s", bridgeName, firstNonEmpty(result.Stderr, result.Error.Error()))
		}
	}

	for _, port := range ports {
		owner := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port.Port, "external_ids:"+ExternalIDOwner)
		if strings.Trim(strings.TrimSpace(owner.Stdout), "\"") == "true" {
			deletePortMeters(port)
			utils.ExecCommandQuiet("ovs-vsctl", "set", "Interface", port.Port, "ingress_policing_kpkts_rate=0", "ingress_policing_kpkts_burst=0")
			clearPortMetadata(port.Port, false)
		}
	}
	for bridgeName := range bridges {
		deleteBridgeOwnedMeters(bridgeName)
	}
	var releaseErrors []string
	for bridgeName := range ownedFlowBridges {
		result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridgeName, "cookie="+QuarantineCookie+"/0xffffffffffffffff")
		if result.Error != nil {
			releaseErrors = append(releaseErrors, fmt.Sprintf("%s: %s", bridgeName, firstNonEmpty(result.Stderr, result.Error.Error())))
		}
	}
	if len(releaseErrors) > 0 {
		markPortsCleanupFailed(ports)
		TriggerReconcile()
		return fmt.Errorf("释放停用隔离流失败: %s", strings.Join(releaseErrors, "; "))
	}
	updateCachedStatus(ports, nil, false, true)
	return nil
}

func markPortsCleanupFailed(ports []policyPort) {
	for _, port := range ports {
		utils.ExecCommandQuiet("ovs-vsctl", "set", "Interface", port.Port,
			"external_ids:"+ExternalIDIsolated+"=true", "external_ids:"+ExternalIDLastError+"=停用清理失败")
	}
}

func syncBridgeMeterMap(bridge string, ports []policyPort) error {
	current := make(map[uint32]bool)
	for _, port := range ports {
		if port.NeighborMeterID > 0 {
			current[port.NeighborMeterID] = true
		}
		if port.BroadcastMeterID > 0 {
			current[port.BroadcastMeterID] = true
		}
	}
	old := readBridgeMeterMap(bridge)
	for id := range old {
		if !current[id] {
			utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, "meter="+strconv.FormatUint(uint64(id), 10))
		}
	}
	ids := make([]int, 0, len(current))
	for id := range current {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = strconv.Itoa(id)
	}
	result := utils.ExecCommand("ovs-vsctl", "set", "Bridge", bridge, "external_ids:"+ExternalIDMeterMap+"="+strings.Join(values, ","))
	if result.Error != nil {
		return fmt.Errorf("保存网桥 %s meter 映射失败: %s", bridge, firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	return nil
}

func readBridgeMeterMap(bridge string) map[uint32]bool {
	result := make(map[uint32]bool)
	value := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Bridge", bridge, "external_ids:"+ExternalIDMeterMap)
	for _, item := range strings.Split(strings.Trim(strings.TrimSpace(value.Stdout), "\""), ",") {
		if id, err := strconv.ParseUint(strings.TrimSpace(item), 10, 32); err == nil && id > 0 {
			result[uint32(id)] = true
		}
	}
	return result
}

func deleteBridgeOwnedMeters(bridge string) {
	for id := range readBridgeMeterMap(bridge) {
		utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-meter", bridge, "meter="+strconv.FormatUint(uint64(id), 10))
	}
	utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "remove", "Bridge", bridge, "external_ids", ExternalIDMeterMap)
}

func deletePortMeters(port policyPort) {
	for _, key := range []string{ExternalIDNeighbor, ExternalIDBroadcast} {
		result := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port.Port, "external_ids:"+key)
		value := strings.Trim(strings.TrimSpace(result.Stdout), "\"")
		meterID, err := strconv.Atoi(value)
		if err == nil && meterID > 0 {
			utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-meter", port.Bridge, "meter="+strconv.Itoa(meterID))
		}
	}
}

func writePortMetadata(port policyPort) error {
	utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "remove", "Interface", port.Port, "external_ids", ExternalIDLastError)
	result := utils.ExecCommand("ovs-vsctl", "set", "Interface", port.Port,
		"external_ids:"+ExternalIDOwner+"=true",
		"external_ids:"+ExternalIDVM+"="+port.VMName,
		"external_ids:"+ExternalIDMode+"="+port.Mode,
		"external_ids:"+ExternalIDNeighbor+"="+strconv.FormatUint(uint64(port.NeighborMeterID), 10),
		"external_ids:"+ExternalIDBroadcast+"="+strconv.FormatUint(uint64(port.BroadcastMeterID), 10))
	if result.Error != nil {
		return fmt.Errorf("写入端口 %s 防护元数据失败: %s", port.Port, firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	return nil
}

func clearPortMetadata(port string, clearIsolation bool) {
	keys := []string{ExternalIDOwner, ExternalIDVM, ExternalIDMode, ExternalIDNeighbor, ExternalIDBroadcast, ExternalIDLastError}
	if clearIsolation {
		keys = append(keys, ExternalIDIsolated)
	}
	for _, key := range keys {
		utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "remove", "Interface", port, "external_ids", key)
	}
}

// IsolatePort 手工隔离指定 OVS Interface。
func IsolatePort(port string) (*Status, error) {
	port = strings.TrimSpace(port)
	if !portNamePattern.MatchString(port) {
		return nil, fmt.Errorf("端口名称格式无效")
	}
	if utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port, "name").Error != nil {
		return nil, fmt.Errorf("OVS 端口不存在")
	}
	result := utils.ExecCommand("ovs-vsctl", "set", "Interface", port, "external_ids:"+ExternalIDIsolated+"=true")
	if result.Error != nil {
		return nil, fmt.Errorf("隔离端口失败: %s", firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	if config.GlobalConfig.PortSecurityEnabled {
		return Reconcile()
	}
	return GetStatus()
}

// ReleasePort 释放手工隔离并重新安装端口策略。
func ReleasePort(port string) (*Status, error) {
	port = strings.TrimSpace(port)
	if !portNamePattern.MatchString(port) {
		return nil, fmt.Errorf("端口名称格式无效")
	}
	utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "remove", "Interface", port, "external_ids", ExternalIDIsolated)
	if config.GlobalConfig.PortSecurityEnabled {
		return Reconcile()
	}
	return GetStatus()
}

// GetStatus 返回实时端口清单及最近一次协调结果。
func GetStatus() (*Status, error) {
	ports, issues, err := collectPolicyPorts()
	if err != nil {
		return nil, err
	}
	enabled := config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled
	bridgeFlows := make(map[string]string)
	for i := range ports {
		if !enabled {
			ports[i].Mode = ModeDisabled
			ports[i].Applied = false
			continue
		}
		owner := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", ports[i].Port, "external_ids:"+ExternalIDOwner)
		if _, loaded := bridgeFlows[ports[i].Bridge]; !loaded {
			flowResult := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", ports[i].Bridge, "cookie="+PolicyCookie+"/0xffffffffffffffff")
			bridgeFlows[ports[i].Bridge] = flowResult.Stdout
		}
		ports[i].Applied = strings.Trim(strings.TrimSpace(owner.Stdout), "\"") == "true" && portFlowPresent(bridgeFlows[ports[i].Bridge], ports[i].OFPort)
		if !ports[i].Applied {
			ports[i].LastError = "端口策略尚未完整落地"
			issues = append(issues, Issue{Code: "policy_not_applied", Message: ports[i].LastError, Bridge: ports[i].Bridge, Port: ports[i].Port, VMName: ports[i].VMName, InterfaceOrder: ports[i].InterfaceOrder, Blocking: true})
		}
		ports[i].DropPackets = readPortFlowDropPackets(ports[i].Bridge, ports[i].OFPort)
		ports[i].NeighborDropPackets = readPortMeterDropPackets(ports[i].Bridge, ports[i].Port, ExternalIDNeighbor)
		ports[i].BroadcastDropPackets = readPortMeterDropPackets(ports[i].Bridge, ports[i].Port, ExternalIDBroadcast)
	}
	status := updateCachedStatus(ports, issues, enabled, false)
	return &status, nil
}

func portFlowPresent(flowDump, ofport string) bool {
	ofport = strings.TrimSpace(ofport)
	if ofport == "" {
		return false
	}
	for _, line := range strings.Split(flowDump, "\n") {
		if strings.Contains(line, "table=0") && strings.Contains(line, "in_port="+ofport) {
			return true
		}
	}
	return false
}

func readPortFlowDropPackets(bridge, ofport string) uint64 {
	if bridge == "" || ofport == "" {
		return 0
	}
	result := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", bridge, "cookie="+PolicyCookie+"/0xffffffffffffffff")
	if result.Error != nil {
		return 0
	}
	var total uint64
	packetPattern := regexp.MustCompile(`n_packets=([0-9]+)`)
	for _, line := range strings.Split(result.Stdout, "\n") {
		if !strings.Contains(line, "in_port="+ofport) || !strings.Contains(line, "actions=drop") {
			continue
		}
		match := packetPattern.FindStringSubmatch(line)
		if len(match) == 2 {
			value, _ := strconv.ParseUint(match[1], 10, 64)
			total += value
		}
	}
	return total
}

func readPortMeterDropPackets(bridge, port, key string) uint64 {
	idResult := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port, "external_ids:"+key)
	id := strings.Trim(strings.TrimSpace(idResult.Stdout), "\"")
	if id == "" || id == "[]" {
		return 0
	}
	result := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "meter-stats", bridge, "meter="+id)
	if result.Error != nil {
		return 0
	}
	matches := regexp.MustCompile(`packet_count[:=]\s*([0-9]+)`).FindAllStringSubmatch(result.Stdout, -1)
	if len(matches) == 0 || len(matches[len(matches)-1]) != 2 {
		return 0
	}
	// dump-meter 先输出 meter 总计数，再输出 drop band 计数；最后一项才是超限丢包数。
	value, _ := strconv.ParseUint(matches[len(matches)-1][1], 10, 64)
	return value
}

func updateCachedStatus(ports []policyPort, issues []Issue, enabled, markReconciled bool) Status {
	statusMu.Lock()
	defer statusMu.Unlock()
	lastReconciled := lastStatus.LastReconciled
	if markReconciled {
		lastReconciled = time.Now()
	}
	if issues == nil {
		issues = []Issue{}
	}
	status := Status{Enabled: enabled, Healthy: true, Ports: exportPorts(ports), Issues: issues, LastReconciled: lastReconciled}
	for _, port := range ports {
		if port.Applied {
			status.AppliedPorts++
		}
		if port.Mode == ModeCompatible {
			status.CompatiblePorts++
		}
		if port.Isolated || port.Mode == ModeQuarantined {
			status.IsolatedPorts++
		}
	}
	for _, issue := range issues {
		if issue.Blocking {
			status.Healthy = false
		}
	}
	lastStatus = status
	return status
}

func exportPorts(ports []policyPort) []PortStatus {
	result := make([]PortStatus, len(ports))
	for i := range ports {
		result[i] = ports[i].PortStatus
	}
	return result
}

// TriggerReconcile 请求后台尽快执行一次协调。
func TriggerReconcile() {
	select {
	case reconcileTrigger <- struct{}{}:
	default:
	}
}

// ReconcileVM 在生命周期操作中同步刷新端口策略，确保端口放行前已有规则。
func ReconcileVM(vmName string) error {
	if config.GlobalConfig == nil || !config.GlobalConfig.PortSecurityEnabled {
		return nil
	}
	_, err := reconcile(vmName)
	if err == nil {
		watchPendingIPv4Lease(vmName)
	}
	return err
}

func watchPendingIPv4Lease(vmName string) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return
	}
	if _, loaded := pendingWatchers.LoadOrStore(vmName, true); loaded {
		return
	}
	utils.SafeGo(func() {
		defer pendingWatchers.Delete(vmName)
		for i := 0; i < 120; i++ {
			time.Sleep(500 * time.Millisecond)
			if config.GlobalConfig == nil || !config.GlobalConfig.PortSecurityEnabled {
				return
			}
			if _, err := reconcile(""); err == nil {
				return
			}
		}
	})
}

// StartReconciler 启动端口变化监听和周期性全量协调。
func StartReconciler() {
	startOnce.Do(func() {
		startOVSDBInterfaceMonitor()
		utils.SafeGo(func() {
			timer := time.NewTimer(portSecurityReconcileInterval())
			defer timer.Stop()
			if config.GlobalConfig.PortSecurityEnabled {
				if _, err := Reconcile(); err != nil {
					logger.App.Warn("启动时恢复端口安全策略失败", "error", err)
				}
			} else {
				_ = DisableRuntime()
			}
			for {
				select {
				case <-timer.C:
				case <-reconcileTrigger:
				}
				if config.GlobalConfig.PortSecurityEnabled {
					if _, err := Reconcile(); err != nil {
						logger.App.Warn("后台协调端口安全策略失败", "error", err)
					}
				}
				timer.Reset(portSecurityReconcileInterval())
			}
		})
	})
}

func portSecurityReconcileInterval() time.Duration {
	interval := 60 * time.Second
	if config.GlobalConfig != nil {
		interval = time.Duration(config.GlobalConfig.PortSecurityReconcileIntervalSeconds) * time.Second
	}
	if interval < 10*time.Second {
		return 10 * time.Second
	}
	return interval
}

func startOVSDBInterfaceMonitor() {
	utils.SafeGo(func() {
		for {
			cmd := exec.Command("ovsdb-client", "monitor", "Open_vSwitch", "Interface", "name,ofport")
			stdout, err := cmd.StdoutPipe()
			if err != nil || cmd.Start() != nil {
				time.Sleep(10 * time.Second)
				continue
			}
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				TriggerReconcile()
			}
			_ = cmd.Wait()
			time.Sleep(2 * time.Second)
		}
	})
}

// ExecuteTask 执行端口安全异步操作。
func ExecuteTask(ctx context.Context, params TaskParams, progress func(int, string)) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	var status *Status
	var err error
	switch strings.ToLower(strings.TrimSpace(params.Action)) {
	case "enable":
		status, err = Enable(progress)
	case "disable":
		status, err = Disable(progress)
	case "reconcile":
		if progress != nil {
			progress(20, "正在协调端口安全策略...")
		}
		status, err = Reconcile()
	case "isolate":
		status, err = IsolatePort(params.Port)
	case "release":
		status, err = ReleasePort(params.Port)
	default:
		err = fmt.Errorf("端口安全任务动作无效")
	}
	if err != nil {
		return "", err
	}
	if progress != nil {
		progress(100, "端口安全任务已完成")
	}
	data, _ := json.Marshal(status)
	return string(data), nil
}
