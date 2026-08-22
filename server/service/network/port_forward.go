package network

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/utils"
)

// normalizePortForwardSourceIP 规范化入站 IP 白名单：
// 空值视为 0.0.0.0/0（不限制）；仅支持 IPv4 单 IP（自动补 /32）或 IPv4 CIDR。
func normalizePortForwardSourceIP(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "0.0.0.0/0", nil
	}
	if strings.Contains(source, ":") {
		return "", fmt.Errorf("入站 IP 仅支持 IPv4 地址，不支持 IPv6")
	}
	if !strings.Contains(source, "/") {
		ip := net.ParseIP(source)
		if ip == nil {
			return "", fmt.Errorf("入站 IP 格式无效: %s", source)
		}
		return source + "/32", nil
	}
	ip, _, err := net.ParseCIDR(source)
	if err != nil || ip.To4() == nil {
		return "", fmt.Errorf("入站 IP 格式无效: %s（请输入 IPv4 地址或 CIDR，如 1.2.3.4 或 10.0.0.0/8）", source)
	}
	return source, nil
}

// sourceArgForPortForward 构建 iptables -s 参数片段；0.0.0.0/0（不限制）时返回空串。
func sourceArgForPortForward(source string) string {
	normalized, err := normalizePortForwardSourceIP(source)
	if err != nil || normalized == "0.0.0.0/0" {
		return ""
	}
	return " -s " + utils.ShellSingleQuote(normalized)
}

// parseSourceIPFromListLine 从 iptables -L -n 列表行中解析源地址列（不带 -v 时第 5 列）。
// 未显式指定 -s 时 iptables 显示 0.0.0.0/0（即不限制）。
func parseSourceIPFromListLine(fields []string) string {
	if len(fields) > 4 {
		return fields[4]
	}
	return "0.0.0.0/0"
}

func buildVMOwnerMap() map[string]string {
	owners := make(map[string]string)
	vmAccessDir := config.GlobalConfig.VMAccessDir

	entries, err := os.ReadDir(vmAccessDir)
	if err != nil {
		return owners
	}

	for _, entry := range entries {
		username := entry.Name()
		for _, vmName := range HookGetUserVMList(username) {
			vmName = strings.TrimSpace(vmName)
			if vmName != "" {
				owners[vmName] = username
			}
		}
	}

	return owners
}

func buildPortForwardTargetInfoMap() map[string]portForwardTargetInfo {
	targetMap := make(map[string]portForwardTargetInfo)
	ownerMap := buildVMOwnerMap()

	setTarget := func(ipAddr, vmName string) {
		ipAddr = strings.TrimSpace(ipAddr)
		vmName = strings.TrimSpace(vmName)
		if ipAddr == "" || vmName == "" {
			return
		}
		targetMap[ipAddr] = portForwardTargetInfo{
			VMName:        vmName,
			OwnerUsername: ownerMap[vmName],
		}
	}

	staticHosts, _ := ListStaticIPs()
	if staticHosts != nil {
		for _, item := range staticHosts.StaticBindings {
			setTarget(item.IP, item.VMName)
		}
		for _, item := range staticHosts.DHCPLeases {
			setTarget(item.IP, item.VMName)
		}
	}

	var manualIPs []model.PortForwardIP
	if err := model.DB.Find(&manualIPs).Error; err == nil {
		for _, item := range manualIPs {
			setTarget(item.IP, item.VMName)
		}
	}

	return targetMap
}

func populatePortForwardRuleMetadata(rule *PortForwardRule, targetMap map[string]portForwardTargetInfo) {
	if rule == nil {
		return
	}
	info, ok := targetMap[strings.TrimSpace(rule.DestIP)]
	if !ok {
		return
	}
	rule.VMName = info.VMName
	rule.OwnerUsername = info.OwnerUsername
}

// ListLivePortForwardsFromIPTables exports listLivePortForwardsFromIPTables for service root
func ListLivePortForwardsFromIPTables() ([]PortForwardRule, error) {
	return listLivePortForwardsFromIPTables()
}

func listLivePortForwardsFromIPTables() ([]PortForwardRule, error) {
	result := utils.ExecShellQuiet("iptables -t nat -L PREROUTING -n --line-numbers 2>/dev/null | grep DNAT")
	if result.Error != nil || result.Stdout == "" {
		return []PortForwardRule{}, nil
	}
	policy, _ := HookGetFirewallPolicy()
	hostIP := getHostIP()
	targetMap := buildPortForwardTargetInfoMap()

	var rules []PortForwardRule
	lines := strings.Split(result.Stdout, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		rule := PortForwardRule{}

		// 编号
		fmt.Sscanf(fields[0], "%d", &rule.ID)

		// 协议
		proto := fields[2]
		switch proto {
		case "6":
			rule.Protocol = "TCP"
		case "17":
			rule.Protocol = "UDP"
		default:
			rule.Protocol = strings.ToUpper(proto)
		}

		// 宿主机端口
		dportRe := regexp.MustCompile(`dpts?:(\S+)`)
		if m := dportRe.FindStringSubmatch(line); len(m) > 1 {
			rule.HostPort = m[1]
		}
		rule.AccessIP = hostIP
		rule.AccessAddress = buildPortForwardAccessAddress(hostIP, rule.HostPort)

		// 目标
		destRe := regexp.MustCompile(`to:(\S+)`)
		if m := destRe.FindStringSubmatch(line); len(m) > 1 {
			dest := m[1]
			parts := strings.SplitN(dest, ":", 2)
			rule.DestIP = parts[0]
			if len(parts) > 1 {
				rule.DestPort = parts[1]
			}
		}
		// 入站 IP 白名单（未显式限制时 iptables 展示 0.0.0.0/0）
		rule.SourceIP = parseSourceIPFromListLine(fields)
		rule.FirewallKey = rule.StableKey()
		rule.RuleKey = rule.StableKey()
		rule.RegionFilterInherited = true
		rule.RegionFilterEnabled = true
		if policy != nil && policy.PortForwardExemptions != nil && policy.PortForwardExemptions[rule.FirewallKey] {
			rule.RegionFilterEnabled = false
			rule.RegionFilterInherited = false
		}
		populatePortForwardRuleMetadata(&rule, targetMap)

		rules = append(rules, rule)
	}

	return rules, nil
}

// ListPortForwards 列出端口转发规则
func ListPortForwards() ([]PortForwardRule, error) {
	rules, err := listLivePortForwardsFromIPTables()
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// GetPortForwardRuleByID 根据当前 iptables 行号获取端口转发规则。
func GetPortForwardRuleByID(ruleID int) (*PortForwardRule, error) {
	rules, err := listLivePortForwardsFromIPTables()
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == ruleID {
			rule := rules[i]
			return &rule, nil
		}
	}
	return nil, fmt.Errorf("规则编号 %d 不存在", ruleID)
}

func findLivePortForwardByStableKey(ruleKey string) (*PortForwardRule, error) {
	rules, err := listLivePortForwardsFromIPTables()
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].StableKey() == strings.TrimSpace(ruleKey) {
			rule := rules[i]
			return &rule, nil
		}
	}
	return nil, nil
}

// AddPortForward 添加端口转发（内置端口冲突检测）
func AddPortForward(params *PortForwardAddParams) error {
	if err := HookEnsureOVSNetworkReady(); err != nil {
		return err
	}
	if params.VMPort == "" {
		params.VMPort = params.HostPort
	}
	if err := CheckRequestedPortForwardHostPortAvailable(params.HostPort, params.Protocol, nil); err != nil {
		return err
	}
	if params.Protocol == "" {
		params.Protocol = "tcp"
	}

	protocols := []string{params.Protocol}
	if params.Protocol == "both" {
		protocols = []string{"tcp", "udp"}
	}

	// 端口冲突检测：无论自动分配还是手动指定都要检查
	for _, proto := range protocols {
		available, reason := IsPortAvailable(params.HostPort, proto)
		if !available {
			return fmt.Errorf("宿主机端口 %s/%s 已被占用: %s", params.HostPort, proto, reason)
		}
	}

	// 入站 IP 白名单（先校验，避免规则部分添加后再报错）
	sourceIP, err := normalizePortForwardSourceIP(params.SourceIP)
	if err != nil {
		return err
	}
	srcArg := sourceArgForPortForward(sourceIP)

	hostIP := getHostIP()

	for _, proto := range protocols {
		// 目标端口格式转换
		destPort := strings.Replace(params.VMPort, ":", "-", 1)

		// DNAT 规则 (PREROUTING - 外部流量)
		cmd := fmt.Sprintf("iptables -t nat -A PREROUTING%s -d %s -p %s --dport %s -j DNAT --to-destination %s:%s",
			srcArg, utils.ShellSingleQuote(hostIP), utils.ShellSingleQuote(proto), utils.ShellSingleQuote(params.HostPort), utils.ShellSingleQuote(params.VMIP), destPort)
		result := utils.ExecShell(cmd)
		if result.Error != nil {
			return fmt.Errorf("添加 %s PREROUTING NAT 规则失败: %s", proto, result.Stderr)
		}

		// DNAT 规则 (OUTPUT - 宿主机本地流量，解决本地访问端口转发不生效问题)
		outputCmd := fmt.Sprintf("iptables -t nat -A OUTPUT%s -d %s -p %s --dport %s -j DNAT --to-destination %s:%s",
			srcArg, utils.ShellSingleQuote(hostIP), utils.ShellSingleQuote(proto), utils.ShellSingleQuote(params.HostPort), utils.ShellSingleQuote(params.VMIP), destPort)
		outputResult := utils.ExecShell(outputCmd)
		if outputResult.Error != nil {
			// 回滚已添加的 PREROUTING DNAT 规则
			utils.ExecShell(fmt.Sprintf("iptables -t nat -D PREROUTING%s -d %s -p %s --dport %s -j DNAT --to-destination %s:%s 2>/dev/null",
				srcArg, utils.ShellSingleQuote(hostIP), utils.ShellSingleQuote(proto), utils.ShellSingleQuote(params.HostPort), utils.ShellSingleQuote(params.VMIP), destPort))
			return fmt.Errorf("添加 %s OUTPUT NAT 规则失败: %s", proto, outputResult.Stderr)
		}

		// 非 VPC 转发继续使用传统 FORWARD 放行；VPC 转发必须经过安全组 ACL。
		if !isVPCManagedIP(params.VMIP) {
			fwdCmd := fmt.Sprintf("iptables -I FORWARD%s -d %s -p %s --dport %s -j ACCEPT",
				srcArg, utils.ShellSingleQuote(params.VMIP), utils.ShellSingleQuote(proto), destPort)
			fwdResult := utils.ExecShell(fwdCmd)
			if fwdResult.Error != nil {
				// 回滚已添加的 PREROUTING 和 OUTPUT DNAT 规则
				utils.ExecShell(fmt.Sprintf("iptables -t nat -D PREROUTING%s -d %s -p %s --dport %s -j DNAT --to-destination %s:%s 2>/dev/null",
					srcArg, utils.ShellSingleQuote(hostIP), utils.ShellSingleQuote(proto), utils.ShellSingleQuote(params.HostPort), utils.ShellSingleQuote(params.VMIP), destPort))
				utils.ExecShell(fmt.Sprintf("iptables -t nat -D OUTPUT%s -d %s -p %s --dport %s -j DNAT --to-destination %s:%s 2>/dev/null",
					srcArg, utils.ShellSingleQuote(hostIP), utils.ShellSingleQuote(proto), utils.ShellSingleQuote(params.HostPort), utils.ShellSingleQuote(params.VMIP), destPort))
				return fmt.Errorf("添加 %s FORWARD 放行规则失败: %s", proto, fwdResult.Stderr)
			}
		}

		// 无论宿主机防火墙当前是否启用，都写入 UFW 持久规则，避免下次开启后拦截已有端口转发。
		if err := HookEnsureHostFirewallPortForwardRule(params.HostPort, proto, params.Comment); err != nil {
			return err
		}
	}

	// 自动持久化规则
	go SavePortForwardRules()

	return nil
}

func stripIPTablesCIDR(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, "/"); idx >= 0 {
		return value[:idx]
	}
	return value
}

func iptablesArgValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

// RemoveVPCPortForwardAcceptRules 移除所有指向 VPC 管理的 IP 的 FORWARD ACCEPT 规则
func RemoveVPCPortForwardAcceptRules() {
	result := utils.ExecShellQuiet("iptables -S FORWARD 2>/dev/null | grep -- '-j ACCEPT' | grep -- '-d ' | grep -- '--dport '")
	if result.Error != nil || strings.TrimSpace(result.Stdout) == "" {
		return
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		args := strings.Fields(line)
		if len(args) < 3 || args[0] != "-A" || args[1] != "FORWARD" {
			continue
		}
		destIP := stripIPTablesCIDR(iptablesArgValue(args, "-d"))
		if !isVPCManagedIP(destIP) {
			continue
		}
		args[0] = "-D"
		utils.ExecCommand("iptables", args...)
	}
}

func removePortForwardsForCIDR(cidr string) {
	rules, err := listLivePortForwardsFromIPTables()
	if err != nil || len(rules) == 0 {
		return
	}
	var ids []int
	for _, rule := range rules {
		if ipInCIDR(rule.DestIP, cidr) {
			ids = append(ids, rule.ID)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	for _, id := range ids {
		if err := DeletePortForward(id); err != nil {
			logger.App.Warn("删除端口转发规则失败", "cidr", cidr, "id", id, "error", err)
		}
	}
}

func cleanupOVSStaticHostsForVMs(vmNames []string) {
	if len(vmNames) == 0 {
		return
	}
	vmSet := make(map[string]bool, len(vmNames))
	for _, vmName := range vmNames {
		vmName = strings.TrimSpace(vmName)
		if vmName != "" {
			vmSet[vmName] = true
		}
	}
	if len(vmSet) == 0 {
		return
	}
	hosts, err := HookListOVSStaticHosts()
	if err != nil || len(hosts) == 0 {
		return
	}
	next := make([]OVSStaticHost, 0, len(hosts))
	changed := false
	for _, host := range hosts {
		if vmSet[strings.TrimSpace(host.VMName)] {
			RemovePortForwardsForIP(host.IP)
			changed = true
			continue
		}
		next = append(next, host)
	}
	if !changed {
		return
	}
	if err := HookWriteOVSStaticHosts(next); err != nil {
		logger.App.Warn("清理 OVS 静态 IP 绑定失败", "error", err)
		return
	}
	HookReloadOVSDNSMasq()
}

func deletePortForwardWithOptions(ruleID int) error {
	// 直接用 iptables 行号获取规则信息（不过滤 grep，避免行号错位）
	ruleInfo := utils.ExecShell(fmt.Sprintf(
		"iptables -t nat -L PREROUTING %d -n 2>/dev/null", ruleID))
	if ruleInfo.Error != nil || ruleInfo.Stdout == "" {
		return fmt.Errorf("规则编号 %d 不存在", ruleID)
	}
	if !strings.Contains(ruleInfo.Stdout, "DNAT") {
		return fmt.Errorf("规则编号 %d 不是端口转发规则", ruleID)
	}

	// 解析目标信息用于删除 FORWARD 规则
	destRe := regexp.MustCompile(`to:(\S+)`)
	var destIP, destPort string
	if m := destRe.FindStringSubmatch(ruleInfo.Stdout); len(m) > 1 {
		parts := strings.SplitN(m[1], ":", 2)
		destIP = parts[0]
		if len(parts) > 1 {
			destPort = parts[1]
		}
	}

	dportRe := regexp.MustCompile(`dpts?:(\S+)`)
	var hostPort string
	if m := dportRe.FindStringSubmatch(ruleInfo.Stdout); len(m) > 1 {
		hostPort = m[1]
	}

	// 入站 IP 白名单：iptables -D 需与规则参数完全匹配，源地址非 0.0.0.0/0 时必须携带 -s
	srcArg := ""
	src := parseSourceIPFromListLine(strings.Fields(ruleInfo.Stdout))
	if src != "" && src != "0.0.0.0/0" {
		srcArg = " -s " + utils.ShellSingleQuote(src)
	}

	protoRe := regexp.MustCompile(`\s+(tcp|udp|6|17)\s+`)
	proto := "tcp"
	if m := protoRe.FindStringSubmatch(ruleInfo.Stdout); len(m) > 1 {
		switch m[1] {
		case "6":
			proto = "tcp"
		case "17":
			proto = "udp"
		default:
			proto = m[1]
		}
	}
	stableKey := PortForwardRule{
		Protocol: strings.ToLower(proto),
		HostPort: hostPort,
		DestIP:   destIP,
		DestPort: destPort,
	}.StableKey()

	// 删除 NAT 规则 (PREROUTING)
	utils.ExecShell(fmt.Sprintf("iptables -t nat -D PREROUTING %d", ruleID))

	// 删除 NAT 规则 (OUTPUT - 清理本地流量 DNAT)
	if hostPort != "" {
		utils.ExecShell(fmt.Sprintf(
			"iptables -t nat -D OUTPUT%s -d %s -p %s --dport %s -j DNAT --to-destination %s:%s 2>/dev/null",
			srcArg, utils.ShellSingleQuote(getHostIP()), utils.ShellSingleQuote(proto), utils.ShellSingleQuote(hostPort), utils.ShellSingleQuote(destIP), utils.ShellSingleQuote(destPort)))
	}

	// 删除 FORWARD 规则
	if destIP != "" && destPort != "" {
		utils.ExecShell(fmt.Sprintf(
			"iptables -D FORWARD%s -d %s -p %s --dport %s -j ACCEPT 2>/dev/null",
			srcArg, utils.ShellSingleQuote(destIP), utils.ShellSingleQuote(proto), utils.ShellSingleQuote(destPort)))
	}

	// 删除 UFW 规则
	if hostPort != "" {
		_ = HookDeleteHostFirewallPortForwardRule(hostPort, proto)
	}
	_ = HookClearPortForwardFirewallExemption(stableKey)

	cleanupErr := removeSecurityGroupAllowsPortForwardIfUnused(destIP, proto, destPort, src)

	// 自动持久化规则
	go SavePortForwardRules()

	return cleanupErr
}

// DeletePortForward 按编号删除端口转发规则
func DeletePortForward(ruleID int) error {
	return deletePortForwardWithOptions(ruleID)
}

// DeletePortForwards 按批量删除端口转发规则。
func DeletePortForwards(ruleIDs []int) error {
	if len(ruleIDs) == 0 {
		return nil
	}

	unique := make(map[int]struct{})
	var ids []int
	for _, id := range ruleIDs {
		if id <= 0 {
			continue
		}
		if _, exists := unique[id]; exists {
			continue
		}
		unique[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))

	for _, id := range ids {
		if err := DeletePortForward(id); err != nil {
			return err
		}
	}
	return nil
}

func normalizeEditablePortForwardProtocol(protocol string) (string, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return "tcp", nil
	}
	switch protocol {
	case "tcp", "udp":
		return protocol, nil
	default:
		return "", fmt.Errorf("编辑端口转发仅支持单协议 tcp 或 udp")
	}
}

// UpdatePortForward 编辑单条端口转发规则。
func UpdatePortForward(ruleID int, params *PortForwardUpdateParams) error {
	if params == nil {
		return fmt.Errorf("更新参数不能为空")
	}

	oldRule, err := GetPortForwardRuleByID(ruleID)
	if err != nil {
		return err
	}

	oldProtocol := strings.ToLower(strings.TrimSpace(oldRule.Protocol))
	newProtocol := params.Protocol
	if strings.TrimSpace(newProtocol) == "" {
		newProtocol = oldProtocol
	}
	newProtocol, err = normalizeEditablePortForwardProtocol(newProtocol)
	if err != nil {
		return err
	}

	hostPort := strings.TrimSpace(params.HostPort)
	if hostPort == "" {
		hostPort = strings.TrimSpace(oldRule.HostPort)
	}
	vmIP := strings.TrimSpace(params.VMIP)
	if vmIP == "" {
		vmIP = strings.TrimSpace(oldRule.DestIP)
	}
	vmPort := strings.TrimSpace(params.VMPort)
	if vmPort == "" {
		vmPort = strings.TrimSpace(oldRule.DestPort)
	}
	comment := strings.TrimSpace(params.Comment)
	if comment == "" {
		comment = strings.TrimSpace(oldRule.VMName)
	}
	if comment == "" {
		comment = "port-forward"
	}

	oldPolicy, _ := HookGetFirewallPolicy()
	oldExempt := oldPolicy != nil && oldPolicy.PortForwardExemptions[oldRule.FirewallKey]
	rollbackParams := &PortForwardAddParams{
		VMIP:           oldRule.DestIP,
		HostPort:       oldRule.HostPort,
		VMPort:         oldRule.DestPort,
		Protocol:       oldProtocol,
		SourceIP:       oldRule.SourceIP,
		Comment:        comment,
		CreatedBy:      strings.TrimSpace(params.CreatedBy),
		CreatedByAdmin: params.CreatedByAdmin,
	}

	if err := DeletePortForward(ruleID); err != nil {
		return err
	}

	addParams := &PortForwardAddParams{
		VMIP:           vmIP,
		HostPort:       hostPort,
		VMPort:         vmPort,
		Protocol:       newProtocol,
		SourceIP:       params.SourceIP,
		Comment:        comment,
		CreatedBy:      strings.TrimSpace(params.CreatedBy),
		CreatedByAdmin: params.CreatedByAdmin,
	}
	if err := AddPortForward(addParams); err != nil {
		restoreErr := AddPortForward(rollbackParams)
		if restoreErr == nil && oldExempt {
			_, _ = HookSetPortForwardFirewallExemption(oldRule.FirewallKey, true)
		}
		if restoreErr != nil {
			return fmt.Errorf("更新端口转发失败，且恢复原规则失败: %v；原始错误: %w", restoreErr, err)
		}
		return fmt.Errorf("更新端口转发失败，已恢复原规则: %w", err)
	}

	if oldExempt {
		newRule := PortForwardRule{
			Protocol: newProtocol,
			HostPort: hostPort,
			DestIP:   vmIP,
			DestPort: vmPort,
		}
		if _, err := HookSetPortForwardFirewallExemption(newRule.StableKey(), true); err != nil {
			return fmt.Errorf("端口转发已更新，但恢复入站区域限制豁免失败: %w", err)
		}
	}

	return nil
}
