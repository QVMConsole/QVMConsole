package public_ip

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kvm_console/model"
	"kvm_console/utils"
)

func NormalizePublicIPMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "1:1 nat", "nat":
		return PublicIPModeNAT
	case "classic", "classic_route", "经典网络-路由":
		return PublicIPModeClassicRoute
	case "classic_bridge", "经典网络-桥接":
		return PublicIPModeClassicBridge
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func PublicIPModeLabel(mode string) string {
	switch NormalizePublicIPMode(mode) {
	case PublicIPModeNAT:
		return "1:1 NAT"
	case PublicIPModeClassicRoute:
		return "经典网络-路由"
	case PublicIPModeClassicBridge:
		return "经典网络-桥接"
	default:
		return mode
	}
}

func ParsePublicIPOperationParams(raw string) (PublicIPOperationParams, error) {
	var params PublicIPOperationParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return params, err
	}
	return params, nil
}

func ParsePublicIPID(raw string) (uint, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("公网 IP ID 无效")
	}
	return uint(id), nil
}

func normalizePublicIPRequest(req PublicIPRequest, current *model.PublicIP) (*model.PublicIP, error) {
	ipText := strings.TrimSpace(req.IP)
	if current != nil && ipText == "" {
		ipText = current.IP
	}
	address, err := netip.ParseAddr(ipText)
	if err != nil {
		return nil, fmt.Errorf("公网 IP 格式无效")
	}
	ipText = address.String()
	cidr := strings.TrimSpace(req.CIDR)
	if cidr != "" {
		if err := validatePublicIPCidr(ipText, cidr); err != nil {
			return nil, err
		}
	}
	gateway := strings.TrimSpace(req.Gateway)
	if gateway != "" {
		gatewayAddr, gatewayErr := netip.ParseAddr(gateway)
		if gatewayErr != nil {
			return nil, fmt.Errorf("网关 IP 格式无效")
		}
		if gatewayAddr.Is4() != address.Is4() {
			return nil, fmt.Errorf("公网 IP 与网关必须使用相同地址族")
		}
		gateway = gatewayAddr.String()
	}
	modes := normalizeSupportedPublicIPModes(req.SupportedModes)
	if !address.Is4() {
		ipv6Modes := make([]string, 0, 2)
		for _, mode := range parsePublicIPModes(modes) {
			if mode == PublicIPModeClassicRoute || mode == PublicIPModeClassicBridge {
				ipv6Modes = append(ipv6Modes, mode)
			}
		}
		if len(ipv6Modes) == 0 {
			return nil, fmt.Errorf("IPv6 公网地址仅支持经典网络-路由或经典网络-桥接模式")
		}
		modes = strings.Join(ipv6Modes, ",")
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		if current != nil {
			status = current.Status
		} else {
			status = PublicIPStatusFree
		}
	}
	if status != PublicIPStatusFree && status != PublicIPStatusBound && status != "reserved" {
		return nil, fmt.Errorf("公网 IP 状态无效")
	}
	uplinkIF := strings.TrimSpace(req.UplinkIF)
	if uplinkIF != "" && !publicIPInterfacePattern.MatchString(uplinkIF) {
		return nil, fmt.Errorf("出口网卡名称格式无效")
	}
	return &model.PublicIP{
		IP:             ipText,
		CIDR:           cidr,
		Gateway:        gateway,
		UplinkIF:       uplinkIF,
		SupportedModes: modes,
		Status:         status,
		Remark:         strings.TrimSpace(req.Remark),
	}, nil
}

func validatePublicIPCidr(ipText, cidr string) error {
	if strings.Contains(cidr, "/") {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("CIDR/掩码格式无效")
		}
		publicIP, parseErr := netip.ParseAddr(ipText)
		if parseErr != nil || prefix.Addr().Is4() != publicIP.Is4() || !prefix.Contains(publicIP) {
			return fmt.Errorf("公网 IP 不在填写的 CIDR 范围内")
		}
		return nil
	}
	publicIP, parseErr := netip.ParseAddr(ipText)
	maskIP := net.ParseIP(cidr)
	if parseErr != nil || !publicIP.Is4() || maskIP == nil || maskIP.To4() == nil {
		return fmt.Errorf("CIDR/掩码格式无效")
	}
	return nil
}

func publicIPPrefix(ipRow model.PublicIP) int {
	if strings.Contains(ipRow.CIDR, "/") {
		prefix, err := netip.ParsePrefix(ipRow.CIDR)
		if err == nil && prefix.Bits() > 0 {
			return prefix.Bits()
		}
	}
	if maskIP := net.ParseIP(strings.TrimSpace(ipRow.CIDR)); maskIP != nil {
		mask := net.IPMask(maskIP.To4())
		if len(mask) == net.IPv4len {
			if ones, bits := mask.Size(); bits == 32 && ones >= 0 {
				return ones
			}
		}
	}
	if publicIPIsIPv6(ipRow.IP) {
		return 128
	}
	return 32
}

func publicIPAddrForHost(ipRow model.PublicIP) string {
	if publicIPIsIPv6(ipRow.IP) {
		return ""
	}
	prefix := publicIPPrefix(ipRow)
	if prefix <= 0 || prefix > 32 {
		prefix = 32
	}
	return fmt.Sprintf("%s/%d", strings.TrimSpace(ipRow.IP), prefix)
}

func publicIPAddressFamily(ipText string) string {
	address, err := netip.ParseAddr(strings.TrimSpace(ipText))
	if err == nil && !address.Is4() {
		return "ipv6"
	}
	return "ipv4"
}

func publicIPIsIPv6(ipText string) bool {
	return publicIPAddressFamily(ipText) == "ipv6"
}

func publicIPVMInterface(vmName string) (string, string) {
	for _, iface := range HookParseVirshDomiflistOutput(utils.ExecCommand("virsh", "domiflist", strings.TrimSpace(vmName)).Stdout) {
		if strings.TrimSpace(iface.Name) != "" && iface.Name != "-" && strings.TrimSpace(iface.Source) != "" {
			return iface.Name, iface.Source
		}
	}
	return "", ""
}

func publicIPVMBridge(vmName string) string {
	_, bridge := publicIPVMInterface(vmName)
	return bridge
}

func publicIPVMRouteInterface(vmName string) string {
	if HookGetVMRouteInterface != nil {
		if iface := strings.TrimSpace(HookGetVMRouteInterface(vmName)); iface != "" {
			return iface
		}
	}
	return publicIPVMBridge(vmName)
}

func publicIPv6GatewayLinkLocal(vmName string) string {
	iface := publicIPVMRouteInterface(vmName)
	if iface == "" {
		iface = strings.TrimSpace(HookOvsBridgeName())
	}
	if iface == "" {
		return ""
	}
	fields := strings.Fields(utils.ExecCommand("ip", "-6", "-o", "addr", "show", "dev", iface, "scope", "link").Stdout)
	for index, field := range fields {
		if field != "inet6" || index+1 >= len(fields) {
			continue
		}
		prefix, err := netip.ParsePrefix(fields[index+1])
		if err == nil {
			return prefix.Addr().String()
		}
	}
	// OVS 基础网桥在尚无运行中端口时可能没有内核自动生成的链路本地地址。
	// 使用接口名生成稳定地址，绑定规则应用时会把同一地址配置到该接口。
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(iface))
	value := hash.Sum64()
	return fmt.Sprintf("fe80::%x:%x:%x:%x", uint16(value>>48), uint16(value>>32), uint16(value>>16), uint16(value))
}

func publicIPFlowCookie(ipText string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(ipText)))
	value := h.Sum64() & 0x00ffffffffffffff
	return publicIPFlowPrefix + fmt.Sprintf("%014x", value)
}

func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

func publicIPManagedBridges() []string {
	seen := map[string]bool{HookOvsBridgeName(): true}
	bridges := []string{HookOvsBridgeName()}
	if model.DB != nil {
		var rows []model.NetworkBridge
		model.DB.Find(&rows)
		for _, row := range rows {
			name := strings.TrimSpace(row.Name)
			if name != "" && !seen[name] {
				seen[name] = true
				bridges = append(bridges, name)
			}
		}
	}
	sort.Strings(bridges)
	return bridges
}

func publicIPRuntimeRuleSummary(ipRow model.PublicIP, binding model.PublicIPBinding) []string {
	req := PublicIPBindRequest{
		Username:    binding.Username,
		VMName:      binding.VMName,
		VMPrivateIP: binding.VMPrivateIP,
		Mode:        binding.Mode,
	}
	commands, err := buildPublicIPCommands(ipRow, req)
	if err != nil {
		return []string{err.Error()}
	}
	return commands
}

func parsePublicIPModes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = PublicIPModeNAT + "," + PublicIPModeClassicRoute + "," + PublicIPModeClassicBridge
	}
	seen := map[string]bool{}
	var modes []string
	for _, part := range strings.Split(raw, ",") {
		mode := NormalizePublicIPMode(part)
		if mode == "" || seen[mode] {
			continue
		}
		seen[mode] = true
		modes = append(modes, mode)
	}
	return modes
}

func publicIPModeLabels(modes []string) []string {
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, PublicIPModeLabel(mode))
	}
	return labels
}

func normalizeSupportedPublicIPModes(raw string) string {
	modes := parsePublicIPModes(raw)
	if len(modes) == 0 {
		modes = []string{PublicIPModeNAT}
	}
	return strings.Join(modes, ",")
}

func publicIPModeAllowed(ipRow model.PublicIP, mode string) bool {
	mode = NormalizePublicIPMode(mode)
	for _, item := range parsePublicIPModes(ipRow.SupportedModes) {
		if item == mode {
			return true
		}
	}
	return false
}

func getPublicIP(id uint) (*model.PublicIP, error) {
	var row model.PublicIP
	if err := model.DB.First(&row, id).Error; err != nil {
		return nil, fmt.Errorf("公网 IP 不存在")
	}
	return &row, nil
}
