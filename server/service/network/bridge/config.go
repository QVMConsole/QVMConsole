package bridge

import (
	"fmt"
	"net"
	"os"
	"strings"

	"kvm_console/logger"
	"kvm_console/model"
	ovspkg "kvm_console/service/ovs"
	"kvm_console/utils"
)

// ── 查询 ──

// GetInterfaceConfig 获取任意接口（网桥或物理网卡）的当前 IP/DNS 配置。
func GetInterfaceConfig(name string) (*InterfaceConfigInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("接口名称不能为空")
	}

	info := &InterfaceConfigInfo{Name: name}

	// 判断接口类型
	isBr := isOVSBridge(name)
	if isBr {
		info.Type = "bridge"
	} else if isPhysicalInterface(name) {
		info.Type = "nic"
		// 检查是否已加入网桥
		ports := readOVSPortBridgeMap()
		if br := ports[name]; br != "" {
			info.BridgeName = br
			info.Reason = fmt.Sprintf("该网卡已加入网桥 %s，请在网桥上配置 IP", br)
			// 仍然返回当前配置（网桥上的 IP），供前端展示
			fillRuntimeConfig(info, name)
			return info, nil
		}
	} else {
		info.Type = "unknown"
		info.Reason = "不是物理网卡或网桥"
		fillRuntimeConfig(info, name)
		return info, nil
	}

	// 填充运行时配置
	fillRuntimeConfig(info, name)

	// 判断是否可配置
	if isBr {
		// 默认 NAT 网桥不可配置
		if name == ovspkg.OvsBridgeName() {
			info.Reason = "默认 NAT 网桥不支持手动配置 IP"
			return info, nil
		}
		// VPC 交换机端口不可配置
		if isVPCSwitchPort(name) {
			info.Reason = "VPC 交换机端口不支持手动配置 IP"
			return info, nil
		}
		// 检查是否为面板管理的网桥
		var row model.NetworkBridge
		if model.DB != nil {
			model.DB.Where("name = ?", name).First(&row)
		}
		if row.ID > 0 {
			info.ManagedBridge = true
			info.MigrateHostIP = row.MigrateHostIP
			if !row.MigrateHostIP {
				info.Reason = "该网桥未启用宿主机 IP 迁移，无需配置 IP"
				return info, nil
			}
		}
		info.Configurable = true
	} else {
		// 独立物理网卡可配置
		info.Configurable = true
	}

	return info, nil
}

// fillRuntimeConfig 从系统命令填充当前 IP/DNS 配置（同时捕获 IPv4 和 IPv6）。
func fillRuntimeConfig(info *InterfaceConfigInfo, name string) {
	cfg := CaptureInterfaceIP(name)
	if strings.TrimSpace(cfg.Addrs) != "" {
		info.Addrs = strings.Fields(cfg.Addrs)
	}
	info.Gateway = cfg.Gateway
	info.Metric = cfg.Metric
	if cfg.DNS != "" {
		info.DNS = strings.Fields(cfg.DNS)
	}
	if strings.TrimSpace(cfg.Addrs6) != "" {
		info.Addrs6 = strings.Fields(cfg.Addrs6)
	}
	info.Gateway6 = cfg.Gateway6
	info.Metric6 = cfg.Metric6
}

// ── 设置 ──

// SetInterfaceConfig 设置接口的 IP/DNS 配置。
// 根据接口类型（网桥或物理网卡）采取不同的持久化策略。
func SetInterfaceConfig(req SetInterfaceConfigRequest) (*InterfaceConfigInfo, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("接口名称不能为空")
	}

	// 获取当前配置以判断类型和可配置性
	current, err := GetInterfaceConfig(name)
	if err != nil {
		return nil, err
	}
	if !current.Configurable {
		reason := current.Reason
		if reason == "" {
			reason = "该接口不支持配置 IP"
		}
		return nil, fmt.Errorf("%s", reason)
	}

	if req.Clear {
		return clearInterfaceConfig(name, current)
	}

	// 校验 IPv4 输入
	addrs := parseAddrList(req.Addrs)
	for _, addr := range addrs {
		if !isValidCIDR(addr) {
			return nil, fmt.Errorf("IPv4 地址格式无效: %s（请使用 CIDR 格式，如 192.168.1.10/24）", addr)
		}
	}
	gateway := strings.TrimSpace(req.Gateway)
	if gateway != "" && net.ParseIP(gateway) == nil {
		return nil, fmt.Errorf("IPv4 网关地址格式无效: %s", gateway)
	}
	// 校验 IPv6 输入
	addrs6 := parseAddrList(req.Addrs6)
	for _, addr := range addrs6 {
		if !isValidCIDR(addr) {
			return nil, fmt.Errorf("IPv6 地址格式无效: %s（请使用 CIDR 格式，如 2001:db8::1/64）", addr)
		}
	}
	gateway6 := strings.TrimSpace(req.Gateway6)
	if gateway6 != "" && net.ParseIP(gateway6) == nil {
		return nil, fmt.Errorf("IPv6 网关地址格式无效: %s", gateway6)
	}
	// IPv4 和 IPv6 至少配置一个
	if len(addrs) == 0 && len(addrs6) == 0 {
		return nil, fmt.Errorf("请至少配置一个 IP 地址（IPv4 或 IPv6，CIDR 格式）")
	}
	var dnsServers []string
	for _, d := range strings.Fields(strings.ReplaceAll(req.DNS, ",", " ")) {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if net.ParseIP(d) == nil {
			return nil, fmt.Errorf("DNS 地址格式无效: %s", d)
		}
		dnsServers = append(dnsServers, d)
	}

	// 应用静态配置
	if err := applyStaticConfig(name, addrs, gateway, dnsServers, addrs6, gateway6, current); err != nil {
		return nil, err
	}

	logger.App.Info("接口 IP/DNS 配置已更新", "interface", name,
		"addrs", addrs, "gateway", gateway, "dns", dnsServers,
		"addrs6", addrs6, "gateway6", gateway6)

	return GetInterfaceConfig(name)
}

// clearInterfaceConfig 清除接口上的所有静态 IP/DNS 配置（IPv4 和 IPv6）。
func clearInterfaceConfig(name string, current *InterfaceConfigInfo) (*InterfaceConfigInfo, error) {
	// 清除运行时 IP（IPv4 + IPv6）
	utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip route del default dev %s 2>/dev/null || true
ip -6 route del default dev %s 2>/dev/null || true
ip -4 addr flush dev %s scope global 2>/dev/null || true
ip -6 addr flush dev %s scope global 2>/dev/null || true`,
		utils.ShellSingleQuote(name), utils.ShellSingleQuote(name),
		utils.ShellSingleQuote(name), utils.ShellSingleQuote(name)))

	// 清除 DNS
	utils.ExecCommand("resolvectl", "revert", name)

	// 如果是面板管理的网桥，更新数据库和恢复脚本
	if current.ManagedBridge && current.MigrateHostIP {
		var row model.NetworkBridge
		if model.DB != nil {
			model.DB.Where("name = ?", name).First(&row)
		}
		if row.ID > 0 {
			row.HostAddrs = ""
			row.HostGateway = ""
			row.HostMetric = ""
			row.HostDNS = ""
			row.HostAddrs6 = ""
			row.HostGateway6 = ""
			row.HostMetric6 = ""
			if model.DB != nil {
				model.DB.Save(&row)
			}
			// 重写恢复脚本（不包含 IP 配置）
			writeBridgeRestoreScript(row.Name, row.UplinkIF, false, HostIPConfig{})
		}
	}

	// 如果是独立物理网卡，移除 networkd 静态配置文件
	if current.Type == "nic" {
		removeNetworkdStaticConfig(name)
	}

	logger.App.Info("接口 IP/DNS 配置已清除", "interface", name)
	return GetInterfaceConfig(name)
}

// applyStaticConfig 应用静态 IP/Gateway/DNS 到指定接口，并持久化。
// 同时处理 IPv4 和 IPv6；某地址族未提供地址时保留该族现有地址。
func applyStaticConfig(name string, addrs []string, gateway string, dns []string, addrs6 []string, gateway6 string, current *InterfaceConfigInfo) error {
	// 保留现有 metric
	metric := current.Metric
	metric6 := current.Metric6

	// 构建并执行 shell 脚本
	script := fmt.Sprintf(`set -e
IFACE=%s
`, utils.ShellSingleQuote(name))

	// 仅当用户提供了新 IPv4 地址时清除旧 IPv4 全局地址
	if len(addrs) > 0 {
		script += `# 清除旧 IPv4 全局地址
ip -4 addr flush dev "$IFACE" scope global 2>/dev/null || true
`
		for _, addr := range addrs {
			script += fmt.Sprintf("ip addr replace %s dev \"$IFACE\"\n", utils.ShellSingleQuote(addr))
		}
	}
	// 仅当用户提供了新 IPv4 网关时替换旧 IPv4 默认路由
	if gateway != "" {
		script += `# 替换 IPv4 默认路由
ip route del default dev "$IFACE" 2>/dev/null || true
`
		script += fmt.Sprintf("ip route replace %s dev \"$IFACE\" scope link 2>/dev/null || true\n",
			utils.ShellSingleQuote(gateway))
		if metric != "" {
			script += fmt.Sprintf("ip route replace default via %s dev \"$IFACE\" metric %s\n",
				utils.ShellSingleQuote(gateway), utils.ShellSingleQuote(metric))
		} else {
			script += fmt.Sprintf("ip route replace default via %s dev \"$IFACE\"\n",
				utils.ShellSingleQuote(gateway))
		}
	}

	// 仅当用户提供了新 IPv6 地址时清除旧 IPv6 全局地址
	if len(addrs6) > 0 {
		script += `# 清除旧 IPv6 全局地址
ip -6 addr flush dev "$IFACE" scope global 2>/dev/null || true
`
		for _, addr := range addrs6 {
			script += fmt.Sprintf("ip -6 addr replace %s dev \"$IFACE\"\n", utils.ShellSingleQuote(addr))
		}
	}
	// 仅当用户提供了新 IPv6 网关时替换旧 IPv6 默认路由
	if gateway6 != "" {
		script += `# 替换 IPv6 默认路由
ip -6 route del default dev "$IFACE" 2>/dev/null || true
`
		script += fmt.Sprintf("ip -6 route replace %s dev \"$IFACE\" scope link 2>/dev/null || true\n",
			utils.ShellSingleQuote(gateway6))
		if metric6 != "" {
			script += fmt.Sprintf("ip -6 route replace default via %s dev \"$IFACE\" metric %s\n",
				utils.ShellSingleQuote(gateway6), utils.ShellSingleQuote(metric6))
		} else {
			script += fmt.Sprintf("ip -6 route replace default via %s dev \"$IFACE\"\n",
				utils.ShellSingleQuote(gateway6))
		}
	}

	result := utils.ExecCommand("bash", "-c", script)
	if result.Error != nil {
		return fmt.Errorf("应用 IP 配置失败: %s", result.Stderr)
	}

	// 设置 DNS
	if len(dns) > 0 {
		args := append([]string{"dns", name}, dns...)
		utils.ExecCommand("resolvectl", args...)
	}
	utils.ExecCommand("resolvectl", "default-route", name, "yes")
	utils.ExecCommand("resolvectl", "domain", name, "~.")

	// 持久化
	if current.ManagedBridge && current.MigrateHostIP {
		// 面板管理的网桥：更新数据库 + 重写恢复脚本
		var row model.NetworkBridge
		if model.DB != nil {
			model.DB.Where("name = ?", name).First(&row)
		}
		if row.ID > 0 {
			row.HostAddrs = strings.Join(addrs, "\n")
			row.HostGateway = gateway
			row.HostMetric = metric
			row.HostDNS = strings.Join(dns, " ")
			row.HostAddrs6 = strings.Join(addrs6, "\n")
			row.HostGateway6 = gateway6
			row.HostMetric6 = metric6
			if model.DB != nil {
				model.DB.Save(&row)
			}
			cfg := HostIPConfig{
				Addrs:    row.HostAddrs,
				Gateway:  row.HostGateway,
				Metric:   row.HostMetric,
				DNS:      row.HostDNS,
				Addrs6:   row.HostAddrs6,
				Gateway6: row.HostGateway6,
				Metric6:  row.HostMetric6,
			}
			if err := writeBridgeRestoreScript(row.Name, row.UplinkIF, true, cfg); err != nil {
				logger.App.Warn("重写网桥恢复脚本失败", "bridge", name, "error", err)
			}
		}
	} else if current.Type == "nic" {
		// 独立物理网卡：写入 networkd 静态配置以持久化
		if err := writeNetworkdStaticConfig(name, addrs, gateway, dns, addrs6, gateway6); err != nil {
			logger.App.Warn("写入 networkd 静态配置失败", "interface", name, "error", err)
		}
	}

	return nil
}

// ── 辅助函数 ──

func isOVSBridge(name string) bool {
	result := utils.ExecCommand("ovs-vsctl", "br-exists", strings.TrimSpace(name))
	return result.Error == nil
}

// isVPCSwitchPort 检查名称是否为 VPC 交换机端口（OVS internal 端口）。
func isVPCSwitchPort(name string) bool {
	if model.DB == nil {
		return false
	}
	var count int64
	model.DB.Model(&model.VPCSwitch{}).Where("name = ?", name).Count(&count)
	return count > 0
}

func parseAddrList(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

func isValidCIDR(s string) bool {
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// ── networkd 静态配置持久化（独立物理网卡） ──

func networkdStaticConfigPath(iface string) string {
	return fmt.Sprintf("/etc/systemd/network/10-kvm-console-static-%s.network", iface)
}

func writeNetworkdStaticConfig(iface string, addrs []string, gateway string, dns []string, addrs6 []string, gateway6 string) error {
	// 仅当 systemd-networkd 活跃时写入（探测类命令，失败仅记 DEBUG）
	if utils.ExecCommandQuiet("systemctl", "is-active", "--quiet", "systemd-networkd").Error != nil {
		logger.App.Debug("systemd-networkd 不活跃，跳过 networkd 静态配置持久化", "interface", iface)
		return nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("[Match]\nName=%s\n\n[Network]\n", iface))
	for _, addr := range addrs {
		b.WriteString(fmt.Sprintf("Address=%s\n", addr))
	}
	for _, addr := range addrs6 {
		b.WriteString(fmt.Sprintf("Address=%s\n", addr))
	}
	if gateway != "" {
		b.WriteString(fmt.Sprintf("Gateway=%s\n", gateway))
	}
	if gateway6 != "" {
		b.WriteString(fmt.Sprintf("Gateway=%s\n", gateway6))
	}
	for _, d := range dns {
		b.WriteString(fmt.Sprintf("DNS=%s\n", d))
	}
	b.WriteString("DHCP=no\n")
	// 有 IPv6 地址时保留链路本地地址（IPv6 NDP 依赖），仅禁用 RA 自动配置；
	// 纯 IPv4 时禁用链路本地地址以避免 169.254 地址干扰。
	if len(addrs6) > 0 || gateway6 != "" {
		b.WriteString("IPv6AcceptRA=no\n")
	} else {
		b.WriteString("LinkLocalAddressing=no\n")
	}

	path := networkdStaticConfigPath(iface)
	changed, err := HookWriteFileIfChanged(path, []byte(b.String()), 0644)
	if err != nil {
		return fmt.Errorf("写入 networkd 静态配置失败: %w", err)
	}
	if changed {
		utils.ExecCommand("networkctl", "reload")
		logger.App.Info("已写入 networkd 静态配置", "interface", iface)
	}
	return nil
}

func removeNetworkdStaticConfig(iface string) {
	path := networkdStaticConfigPath(iface)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}
	if err := os.Remove(path); err != nil {
		logger.App.Warn("删除 networkd 静态配置失败", "interface", iface, "error", err)
		return
	}
	if utils.ExecCommandQuiet("systemctl", "is-active", "--quiet", "systemd-networkd").Error == nil {
		utils.ExecCommand("networkctl", "reload")
		logger.App.Info("已移除 networkd 静态配置", "interface", iface)
	}
}
