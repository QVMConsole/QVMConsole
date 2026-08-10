package portsecurity

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"kvm_console/config"
	"kvm_console/model"
	bridgepkg "kvm_console/service/network/bridge"
	"kvm_console/service/ovs"
	"kvm_console/utils"
)

type runtimeInterface struct {
	Port   string
	Type   string
	Source string
	Model  string
	MAC    string
}

type domainInterfaceXML struct {
	Type string `xml:"type,attr"`
	MAC  struct {
		Address string `xml:"address,attr"`
	} `xml:"mac"`
	Source struct {
		Bridge  string `xml:"bridge,attr"`
		Network string `xml:"network,attr"`
		Dev     string `xml:"dev,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
	} `xml:"target"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
}

type domainDevicesXML struct {
	Interfaces []domainInterfaceXML `xml:"devices>interface"`
}

var macPattern = regexp.MustCompile(`(?i)^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)

func collectPolicyPorts() ([]policyPort, []Issue, error) {
	if model.DB == nil {
		return nil, nil, nil
	}
	var bindings []model.VPCVMBinding
	if err := model.DB.Order("vm_name ASC, interface_order ASC").Find(&bindings).Error; err != nil {
		return nil, nil, err
	}
	var switches []model.VPCSwitch
	if err := model.DB.Order("id ASC").Find(&switches).Error; err != nil {
		return nil, nil, err
	}
	bindingByVMOrder := make(map[string]model.VPCVMBinding)
	bindingsByVM := make(map[string][]model.VPCVMBinding)
	for _, binding := range bindings {
		bindingByVMOrder[vmOrderKey(binding.VMName, binding.InterfaceOrder)] = binding
		bindingsByVM[strings.TrimSpace(binding.VMName)] = append(bindingsByVM[strings.TrimSpace(binding.VMName)], binding)
	}
	switchByID := make(map[uint]model.VPCSwitch)
	for _, sw := range switches {
		switchByID[sw.ID] = sw
	}

	staticByMAC, leaseByMAC := collectKnownIPv4ByMAC()
	publicByVM := collectPublicIPv4ByVM()
	vmNames, err := listActiveVMNames()
	if err != nil {
		return nil, nil, err
	}
	var ports []policyPort
	var issues []Issue
	seenPorts := make(map[string]bool)
	for _, vmName := range vmNames {
		interfaces := collectRuntimeInterfaces(vmName)
		vmUUID := strings.TrimSpace(utils.ExecCommandQuiet("virsh", "domuuid", vmName).Stdout)
		bindingByMAC := collectVMBindingsByMAC(vmName, bindingsByVM[vmName])
		usedBindingOrders := make(map[int]bool)
		for runtimeOrder, iface := range interfaces {
			if iface.Port == "" || iface.Port == "-" || iface.Source == "" {
				continue
			}
			ofport := interfaceOFPort(iface.Port)
			if ofport == "" {
				continue
			}
			inferredSwitch, hasInferredSwitch := inferSwitchForRuntimePort(iface, switches)
			binding, hasBinding := resolveRuntimeBinding(
				vmName, runtimeOrder, iface, bindingsByVM[vmName], bindingByMAC,
				bindingByVMOrder, inferredSwitch, hasInferredSwitch, usedBindingOrders,
			)
			interfaceOrder := runtimeOrder
			if hasBinding {
				interfaceOrder = binding.InterfaceOrder
				usedBindingOrders[binding.InterfaceOrder] = true
			}
			sw, hasSwitch := switchByID[binding.SwitchID]
			if !hasBinding || !hasSwitch {
				sw, hasSwitch = inferredSwitch, hasInferredSwitch
			}
			port := policyPort{PortStatus: PortStatus{
				Bridge: iface.Source, Port: iface.Port, OFPort: ofport, VMName: vmName,
				InterfaceOrder: interfaceOrder, MAC: normalizeMAC(iface.MAC),
			}}
			if config.GlobalConfig != nil {
				port.PolicingKpps = config.GlobalConfig.PortSecurityTotalKpps
				port.PolicingBurstKPackets = config.GlobalConfig.PortSecurityTotalBurstKPackets
			}
			if hasSwitch {
				port.SwitchID = sw.ID
				port.SwitchName = sw.Name
				port.DirectBridge = bridgepkg.SwitchUsesDirectBridge(sw)
				// 新建虚拟机首次暂停启动时，VPC 绑定可能尚未落库；此时先按 IPv4/MAC
				// 策略隔离 IPv6，绑定保存并再次协调后再启用精确 IPv6 白名单。
				port.IPv6Enabled = port.DirectBridge && sw.IPv6SecurityEnabled && hasBinding
				port.TrustedIPv6Prefixes = parsePrefixList(sw.TrustedIPv6Prefixes)
			}
			if hasBinding {
				port.AllowedIPv4Addresses = parseAddressList(binding.AllowedIPv4Addresses, false)
				port.AllowedIPv6Addresses = parseAddressList(binding.AllowedIPv6Addresses, true)
			}
			port.AllowedIPv4Addresses = appendUniqueAddresses(port.AllowedIPv4Addresses, staticByMAC[port.MAC]...)
			port.AllowedIPv4Addresses = appendUniqueAddresses(port.AllowedIPv4Addresses, leaseByMAC[port.MAC]...)
			// 现有公网 IP 绑定模型归属于虚拟机主网卡；不要扩散到额外网卡，避免跨端口地址冒用。
			if interfaceOrder == 0 {
				port.AllowedIPv4Addresses = appendUniqueAddresses(port.AllowedIPv4Addresses, publicByVM[vmName]...)
			}
			port.StrictIPv4 = !port.DirectBridge || len(port.AllowedIPv4Addresses) > 0
			port.Mode = ModeStrict
			if port.DirectBridge && !port.StrictIPv4 {
				port.Mode = ModeCompatible
			}
			port.Isolated = readPortIsolation(iface.Port)
			port.LastError = readInterfaceExternalID(iface.Port, ExternalIDLastError)
			if port.Isolated {
				port.Mode = ModeQuarantined
			}
			if port.MAC == "" || !macPattern.MatchString(port.MAC) {
				issues = append(issues, Issue{Code: "missing_mac", Message: "运行态网卡缺少有效 MAC 地址", Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
			}
			actualBridge := strings.TrimSpace(utils.ExecCommandQuiet("ovs-vsctl", "port-to-br", port.Port).Stdout)
			if actualBridge == "" || actualBridge != port.Bridge {
				issues = append(issues, Issue{Code: "port_bridge_mismatch", Message: fmt.Sprintf("OVS 端口实际归属网桥 %s，与 libvirt 声明 %s 不一致", firstNonEmpty(actualBridge, "未知"), port.Bridge), Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
			}
			attachedMAC := readInterfaceExternalID(port.Port, "attached-mac")
			if attachedMAC != "" && !strings.EqualFold(attachedMAC, port.MAC) {
				issues = append(issues, Issue{Code: "port_mac_ownership_mismatch", Message: "OVS 端口 attached-mac 与 libvirt MAC 不一致", Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
			}
			attachedVM := readInterfaceExternalID(port.Port, "vm-id")
			if attachedVM != "" && vmUUID != "" && !strings.EqualFold(attachedVM, vmUUID) {
				issues = append(issues, Issue{Code: "port_vm_ownership_mismatch", Message: "OVS 端口 vm-id 与 libvirt 域 UUID 不一致", Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
			}
			if !hasSwitch {
				issues = append(issues, Issue{Code: "missing_switch", Message: "运行态网卡未匹配到逻辑交换机", Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
			}
			if !port.DirectBridge && len(port.AllowedIPv4Addresses) == 0 {
				issues = append(issues, Issue{Code: "missing_ipv4_address", Message: "系统/NAT 网卡缺少静态绑定或有效 DHCP 租约", Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
			}
			if port.IPv6Enabled {
				if len(port.TrustedIPv6Prefixes) == 0 {
					issues = append(issues, Issue{Code: "missing_ipv6_prefix", Message: "直通桥已启用 IPv6 防护，但可信 IPv6 前缀为空", Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
				}
				if len(port.AllowedIPv6Addresses) == 0 {
					issues = append(issues, Issue{Code: "missing_ipv6_address", Message: "直通桥已启用 IPv6 防护，但网卡可信 IPv6 地址为空", Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
				}
				for _, address := range port.AllowedIPv6Addresses {
					if !addressInPrefixes(address, port.TrustedIPv6Prefixes) {
						issues = append(issues, Issue{Code: "ipv6_outside_prefix", Message: fmt.Sprintf("IPv6 地址 %s 不属于交换机可信前缀", address), Bridge: port.Bridge, Port: port.Port, VMName: vmName, InterfaceOrder: interfaceOrder, Blocking: true})
					}
				}
			}
			ports = append(ports, port)
			seenPorts[port.Bridge+"\x00"+port.Port] = true
		}
	}

	// 对未关联 libvirt 网卡的 vnet/tap 端口采用隔离策略，避免残留端口成为旁路。
	for _, bridgeName := range managedBridges(switches, ports) {
		result := utils.ExecCommand("ovs-vsctl", "list-ports", bridgeName)
		if result.Error != nil {
			continue
		}
		for _, portName := range strings.Fields(result.Stdout) {
			if seenPorts[bridgeName+"\x00"+portName] || (!strings.HasPrefix(portName, "vnet") && !strings.HasPrefix(portName, "tap")) {
				continue
			}
			ofport := interfaceOFPort(portName)
			if ofport == "" {
				continue
			}
			ports = append(ports, policyPort{PortStatus: PortStatus{Bridge: bridgeName, Port: portName, OFPort: ofport, Mode: ModeQuarantined, Isolated: true}})
			issues = append(issues, Issue{Code: "orphan_port", Message: "发现未关联虚拟机的 OVS 端口，启用后将保持隔离", Bridge: bridgeName, Port: portName, Blocking: false})
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Bridge == ports[j].Bridge {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Bridge < ports[j].Bridge
	})
	return ports, issues, nil
}

func listActiveVMNames() ([]string, error) {
	result := utils.ExecCommand("virsh", "list", "--name")
	if result.Error != nil {
		return nil, fmt.Errorf("读取运行中虚拟机清单失败: %s", firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	var names []string
	for _, name := range strings.Split(result.Stdout, "\n") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func collectRuntimeInterfaces(vmName string) []runtimeInterface {
	domiflist := parseDomiflist(utils.ExecCommand("virsh", "domiflist", vmName).Stdout)
	byPort := make(map[string]runtimeInterface, len(domiflist))
	byMAC := make(map[string]runtimeInterface, len(domiflist))
	for _, iface := range domiflist {
		byPort[iface.Port] = iface
		byMAC[normalizeMAC(iface.MAC)] = iface
	}

	xmlResult := utils.ExecCommand("virsh", "dumpxml", vmName)
	if xmlResult.Error != nil {
		return domiflist
	}
	var domain domainDevicesXML
	if err := xml.Unmarshal([]byte(xmlResult.Stdout), &domain); err != nil {
		return domiflist
	}
	seen := make(map[string]bool)
	interfaces := make([]runtimeInterface, 0, len(domain.Interfaces))
	for _, item := range domain.Interfaces {
		mac := normalizeMAC(item.MAC.Address)
		live, ok := byPort[strings.TrimSpace(item.Target.Dev)]
		if !ok {
			live, ok = byMAC[mac]
		}
		if !ok {
			continue
		}
		iface := runtimeInterface{
			Port: strings.TrimSpace(item.Target.Dev), Type: strings.TrimSpace(item.Type),
			Source: strings.TrimSpace(item.Source.Bridge), Model: strings.TrimSpace(item.Model.Type), MAC: mac,
		}
		if iface.Port == "" {
			iface.Port = live.Port
		}
		if iface.Type == "" {
			iface.Type = live.Type
		}
		if iface.Source == "" {
			iface.Source = firstNonEmpty(item.Source.Network, item.Source.Dev, live.Source)
		}
		if iface.Model == "" {
			iface.Model = live.Model
		}
		if iface.MAC == "" {
			iface.MAC = live.MAC
		}
		interfaces = append(interfaces, iface)
		seen[iface.Port] = true
	}
	for _, iface := range domiflist {
		if !seen[iface.Port] {
			interfaces = append(interfaces, iface)
		}
	}
	return interfaces
}

func parseDomiflist(text string) []runtimeInterface {
	var result []runtimeInterface
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] == "Interface" || !macPattern.MatchString(fields[4]) {
			continue
		}
		result = append(result, runtimeInterface{Port: fields[0], Type: fields[1], Source: fields[2], Model: fields[3], MAC: normalizeMAC(fields[4])})
	}
	return result
}

func collectVMBindingsByMAC(vmName string, bindings []model.VPCVMBinding) map[string]model.VPCVMBinding {
	result := make(map[string]model.VPCVMBinding)
	if HookGetVMMACByOrder == nil {
		return result
	}
	ambiguous := make(map[string]bool)
	for _, binding := range bindings {
		mac := normalizeMAC(HookGetVMMACByOrder(vmName, binding.InterfaceOrder))
		if mac == "" || !macPattern.MatchString(mac) || ambiguous[mac] {
			continue
		}
		if previous, exists := result[mac]; exists && previous.InterfaceOrder != binding.InterfaceOrder {
			delete(result, mac)
			ambiguous[mac] = true
			continue
		}
		result[mac] = binding
	}
	return result
}

func resolveRuntimeBinding(
	vmName string,
	runtimeOrder int,
	iface runtimeInterface,
	bindings []model.VPCVMBinding,
	bindingByMAC map[string]model.VPCVMBinding,
	bindingByVMOrder map[string]model.VPCVMBinding,
	inferredSwitch model.VPCSwitch,
	hasInferredSwitch bool,
	usedBindingOrders map[int]bool,
) (model.VPCVMBinding, bool) {
	if binding, exists := bindingByMAC[normalizeMAC(iface.MAC)]; exists &&
		!usedBindingOrders[binding.InterfaceOrder] &&
		(!hasInferredSwitch || binding.SwitchID == inferredSwitch.ID) {
		return binding, true
	}

	if hasInferredSwitch {
		var matched model.VPCVMBinding
		matchCount := 0
		for _, binding := range bindings {
			if binding.SwitchID == inferredSwitch.ID && !usedBindingOrders[binding.InterfaceOrder] {
				matched = binding
				matchCount++
			}
		}
		if matchCount == 1 {
			return matched, true
		}
	}

	binding, exists := bindingByVMOrder[vmOrderKey(vmName, runtimeOrder)]
	if exists && !usedBindingOrders[binding.InterfaceOrder] {
		return binding, true
	}
	return model.VPCVMBinding{}, false
}

func inferSwitchForRuntimePort(iface runtimeInterface, switches []model.VPCSwitch) (model.VPCSwitch, bool) {
	tag := readPortTag(iface.Port)
	for _, sw := range switches {
		if bridgepkg.BridgeNameForSwitch(sw) != iface.Source {
			continue
		}
		if bridgepkg.SwitchUsesDirectBridge(sw) {
			if sw.BridgeVLANID == 0 || sw.BridgeVLANID == tag {
				return sw, true
			}
			continue
		}
		if sw.VLANID == tag {
			return sw, true
		}
	}
	return model.VPCSwitch{}, false
}

func readPortTag(port string) int {
	result := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Port", port, "tag")
	value := strings.Trim(strings.TrimSpace(result.Stdout), "\"")
	if value == "" || value == "[]" {
		return 0
	}
	tag, _ := strconv.Atoi(value)
	return tag
}

func interfaceOFPort(port string) string {
	result := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port, "ofport")
	value := strings.Trim(strings.TrimSpace(result.Stdout), "\"")
	if value == "" || value == "[]" || value == "-1" {
		return ""
	}
	if _, err := strconv.Atoi(value); err != nil {
		return ""
	}
	return value
}

func collectKnownIPv4ByMAC() (map[string][]string, map[string][]string) {
	staticByMAC := make(map[string][]string)
	leaseByMAC := make(map[string][]string)
	if hosts, err := ovs.ListOVSStaticHosts(); err == nil {
		for _, host := range hosts {
			staticByMAC[normalizeMAC(host.MAC)] = appendUniqueAddresses(staticByMAC[normalizeMAC(host.MAC)], host.IP)
		}
	}
	if hosts, err := ovs.ListAllVPCStaticHosts(); err == nil {
		for _, host := range hosts {
			staticByMAC[normalizeMAC(host.MAC)] = appendUniqueAddresses(staticByMAC[normalizeMAC(host.MAC)], host.IP)
		}
	}
	if leases, err := ovs.ListOVSDHCPLeases(); err == nil {
		for _, lease := range leases {
			leaseByMAC[normalizeMAC(lease.MAC)] = appendUniqueAddresses(leaseByMAC[normalizeMAC(lease.MAC)], lease.IP)
		}
	}
	if leases, err := ovs.ListVPCDHCPLeases(); err == nil {
		for _, lease := range leases {
			leaseByMAC[normalizeMAC(lease.MAC)] = appendUniqueAddresses(leaseByMAC[normalizeMAC(lease.MAC)], lease.IP)
		}
	}
	return staticByMAC, leaseByMAC
}

func collectPublicIPv4ByVM() map[string][]string {
	result := make(map[string][]string)
	if model.DB == nil {
		return result
	}
	var bindings []model.PublicIPBinding
	if err := model.DB.Find(&bindings).Error; err != nil {
		return result
	}
	for _, binding := range bindings {
		if strings.EqualFold(binding.Mode, "nat") {
			result[binding.VMName] = appendUniqueAddresses(result[binding.VMName], binding.VMPrivateIP)
		} else {
			result[binding.VMName] = appendUniqueAddresses(result[binding.VMName], binding.PublicIP, binding.VMPrivateIP)
		}
	}
	return result
}

func managedBridges(switches []model.VPCSwitch, ports []policyPort) []string {
	seen := map[string]bool{}
	add := func(value string) {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = true
		}
	}
	if config.GlobalConfig != nil {
		add(config.GlobalConfig.OVSBridge)
	}
	for _, sw := range switches {
		add(bridgepkg.BridgeNameForSwitch(sw))
	}
	for _, port := range ports {
		add(port.Bridge)
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func parseAddressList(value string, ipv6 bool) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ';' || r == ' ' || r == '\t' })
	var result []string
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if prefix, err := netip.ParsePrefix(field); err == nil {
			if prefix.Bits() == prefix.Addr().BitLen() {
				field = prefix.Addr().String()
			} else {
				continue
			}
		}
		ip := net.ParseIP(field)
		if ip == nil || (ipv6 && ip.To4() != nil) || (!ipv6 && ip.To4() == nil) {
			continue
		}
		result = appendUniqueAddresses(result, field)
	}
	return result
}

func parsePrefixList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ';' || r == ' ' || r == '\t' })
	var result []string
	for _, field := range fields {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(field))
		if err != nil || prefix.Addr().Is4() {
			continue
		}
		result = appendUniqueAddresses(result, prefix.Masked().String())
	}
	return result
}

func addressInPrefixes(address string, prefixes []string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(address))
	if err != nil {
		return false
	}
	for _, value := range prefixes {
		if prefix, parseErr := netip.ParsePrefix(value); parseErr == nil && prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func appendUniqueAddresses(values []string, candidates ...string) []string {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[strings.TrimSpace(value)] = true
	}
	for _, value := range candidates {
		value = strings.TrimSpace(value)
		if ip := net.ParseIP(value); ip == nil || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func vmOrderKey(vmName string, order int) string {
	return strings.TrimSpace(vmName) + "\x00" + strconv.Itoa(order)
}

func normalizeMAC(mac string) string {
	return strings.ToLower(strings.TrimSpace(mac))
}

func readPortIsolation(port string) bool {
	result := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port, "external_ids:"+ExternalIDIsolated)
	value := strings.Trim(strings.TrimSpace(result.Stdout), "\"")
	return value == "true"
}

func readInterfaceExternalID(port, key string) string {
	result := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "get", "Interface", port, "external_ids:"+key)
	value := strings.Trim(strings.TrimSpace(result.Stdout), "\"")
	if value == "[]" {
		return ""
	}
	return value
}
