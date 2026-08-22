package network

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"kvm_console/config"
	"kvm_console/model"
	"kvm_console/utils"
)

// GetUserPortForwardUsage 获取用户当前已使用的端口转发数量。
func GetUserPortForwardUsage(username string) int {
	rules, err := listLivePortForwardsFromIPTables()
	if err != nil {
		return 0
	}
	count := 0
	for _, rule := range rules {
		if strings.TrimSpace(rule.OwnerUsername) == strings.TrimSpace(username) {
			count++
		}
	}
	return count
}

// CheckUserPortForwardFeatureEnabled 检查用户是否允许使用端口转发功能。
func CheckUserPortForwardFeatureEnabled(username string) error {
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}
	if user.Role == "admin" {
		return nil
	}
	if !user.EnablePortForward {
		return fmt.Errorf("当前用户未开通端口转发功能")
	}
	return nil
}

// CheckUserPortForwardQuota 检查用户端口转发数量配额。
func CheckUserPortForwardQuota(username string, delta int) error {
	if delta <= 0 {
		return nil
	}

	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}
	if user.Role == "admin" || user.MaxPortForwards <= 0 {
		return nil
	}

	used := GetUserPortForwardUsage(username)
	if used+delta > user.MaxPortForwards {
		return fmt.Errorf("端口转发数量超出配额限制（已用 %d / 上限 %d）", used, user.MaxPortForwards)
	}
	return nil
}

func buildPortForwardAccessAddress(hostIP, hostPort string) string {
	hostIP = strings.TrimSpace(hostIP)
	hostPort = strings.TrimSpace(hostPort)
	if hostIP == "" && hostPort == "" {
		return ""
	}
	if hostIP == "" {
		return hostPort
	}
	if hostPort == "" {
		return hostIP
	}
	return hostIP + ":" + hostPort
}

// GetConfiguredPortForwardHostIP 返回端口转发对外展示使用的 IP。
func GetConfiguredPortForwardHostIP() string {
	return getHostIP()
}

// BuildPortForwardAccessAddressForMessage 生成端口转发对外访问地址文本。
func BuildPortForwardAccessAddressForMessage(hostPort string) string {
	return buildPortForwardAccessAddress(getHostIP(), hostPort)
}

// getOccupiedPorts 获取所有被占用的端口集合（TCP/UDP监听 + iptables DNAT）
func getOccupiedPorts() map[int]bool {
	usedPorts := make(map[int]bool)

	// TCP 监听端口
	tcpResult := utils.ExecShellQuiet(`ss -tlnH 2>/dev/null | awk '{print $4}' | grep -oP '\d+$' | sort -un`)
	for _, line := range strings.Split(tcpResult.Stdout, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			var port int
			fmt.Sscanf(p, "%d", &port)
			usedPorts[port] = true
		}
	}

	// UDP 监听端口
	udpResult := utils.ExecShellQuiet(`ss -ulnH 2>/dev/null | awk '{print $4}' | grep -oP '\d+$' | sort -un`)
	for _, line := range strings.Split(udpResult.Stdout, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			var port int
			fmt.Sscanf(p, "%d", &port)
			usedPorts[port] = true
		}
	}

	// iptables DNAT 已用端口
	iptResult := utils.ExecShellQuiet(`iptables -t nat -L PREROUTING -n 2>/dev/null | grep DNAT | grep -oP 'dpts?:\K\S+'`)
	for _, line := range strings.Split(iptResult.Stdout, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			var port int
			fmt.Sscanf(p, "%d", &port)
			usedPorts[port] = true
		}
	}

	return usedPorts
}

func normalizePortForwardProtocols(protocol string) ([]string, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return []string{"tcp"}, nil
	}
	switch protocol {
	case "tcp", "udp":
		return []string{protocol}, nil
	case "both":
		return []string{"tcp", "udp"}, nil
	default:
		return nil, fmt.Errorf("不支持的端口转发协议: %s", protocol)
	}
}

func buildPortForwardAvailabilityExcludeSet(hostPort string, protocols []string, currentRule *PortForwardRule) map[string]struct{} {
	if currentRule == nil {
		return nil
	}
	if strings.TrimSpace(hostPort) != strings.TrimSpace(currentRule.HostPort) {
		return nil
	}

	currentProtocol := strings.ToLower(strings.TrimSpace(currentRule.Protocol))
	for _, protocol := range protocols {
		if protocol == currentProtocol {
			return map[string]struct{}{
				currentRule.StableKey(): {},
			}
		}
	}
	return nil
}

func canBindHostPort(protocol string, port int) bool {
	address := ":" + strconv.Itoa(port)
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		conn, err := net.ListenPacket("udp", address)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	default:
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return false
		}
		_ = listener.Close()
		return true
	}
}

func detectHostPortListener(protocol string, port int) (bool, string) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))

	ssFlag := "-tlnH"
	ssProcessFlag := "-tlpnH"
	if protocol == "udp" {
		ssFlag = "-ulnH"
		ssProcessFlag = "-ulpnH"
	}

	// 端口未被监听时 grep 无匹配（exit 1）属正常信号，用 Quiet 避免误报 ERROR
	ssResult := utils.ExecShellQuiet(fmt.Sprintf(
		"ss %s 2>/dev/null | awk '{print $4}' | grep -P ':%d$'", ssFlag, port))
	if strings.TrimSpace(ssResult.Stdout) != "" {
		procResult := utils.ExecShellQuiet(fmt.Sprintf(
			"ss %s 2>/dev/null | grep ':%d ' | head -1",
			ssProcessFlag, port))
		procInfo := strings.TrimSpace(procResult.Stdout)
		if procInfo != "" {
			procRe := regexp.MustCompile(`users:\(\("([^"]+)"`)
			if matches := procRe.FindStringSubmatch(procInfo); len(matches) > 1 {
				return true, fmt.Sprintf("端口被宿主机服务 \"%s\" 占用（%s/%d）", matches[1], protocol, port)
			}
		}
		return true, fmt.Sprintf("端口已被宿主机监听占用（%s/%d）", protocol, port)
	}

	if canListenOnHostPort != nil && !canListenOnHostPort(protocol, port) {
		return true, fmt.Sprintf("端口已被宿主机监听占用（%s/%d）", protocol, port)
	}

	return false, ""
}

func isPortAvailableWithExclusions(portStr string, protocol string, excludeRuleKeys map[string]struct{}) (bool, string) {
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 || port > 65535 {
		return false, "端口格式无效"
	}

	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		protocol = "tcp"
	}

	if listPortForwardRulesForAvailability != nil {
		if rules, err := listPortForwardRulesForAvailability(); err == nil {
			for _, rule := range rules {
				if strings.ToLower(strings.TrimSpace(rule.Protocol)) != protocol {
					continue
				}
				if strings.TrimSpace(rule.HostPort) != strconv.Itoa(port) {
					continue
				}
				if _, excluded := excludeRuleKeys[rule.StableKey()]; excluded {
					continue
				}
				return false, fmt.Sprintf("已存在端口转发规则（%s/%s -> %s:%s）", rule.Protocol, rule.HostPort, rule.DestIP, rule.DestPort)
			}
		}
	}

	if occupied, reason := detectHostPortListener(protocol, port); occupied {
		return false, reason
	}

	return true, ""
}

// IsPortAvailable 检查指定端口是否可用（未被系统服务、其他进程或 iptables 占用）
// 返回 (是否可用, 占用原因)
func IsPortAvailable(portStr string, protocol string) (bool, string) {
	return isPortAvailableWithExclusions(portStr, protocol, nil)
}

// CheckRequestedPortForwardHostPortAvailable 检查用户手动指定的宿主机端口是否可用。
// currentRule 仅用于编辑场景忽略当前规则自身，避免同端口更新时误判冲突。
func CheckRequestedPortForwardHostPortAvailable(hostPort, protocol string, currentRule *PortForwardRule) error {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return nil
	}
	if strings.TrimSpace(protocol) == "" && currentRule != nil {
		protocol = currentRule.Protocol
	}

	protocols, err := normalizePortForwardProtocols(protocol)
	if err != nil {
		return err
	}
	excludeRuleKeys := buildPortForwardAvailabilityExcludeSet(hostPort, protocols, currentRule)
	for _, proto := range protocols {
		available, reason := isPortAvailableWithExclusions(hostPort, proto, excludeRuleKeys)
		if !available {
			return fmt.Errorf("宿主机端口 %s/%s 已被占用: %s", hostPort, proto, reason)
		}
	}
	return nil
}

// AutoAllocatePort 自动分配端口（包含 TCP+UDP 全面检测）
func AutoAllocatePort() (int, error) {
	start := config.GlobalConfig.AutoPortStart
	end := config.GlobalConfig.AutoPortEnd

	// 获取所有被占用的端口
	usedPorts := getOccupiedPorts()

	for port := start; port <= end; port++ {
		if !usedPorts[port] {
			return port, nil
		}
	}

	return 0, fmt.Errorf("范围 %d-%d 内没有可用端口", start, end)
}
