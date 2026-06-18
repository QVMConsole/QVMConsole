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

// fillRuntimeConfig 从系统命令填充当前 IP/DNS 配置。
func fillRuntimeConfig(info *InterfaceConfigInfo, name string) {
	cfg := CaptureInterfaceIPv4(name)
	if strings.TrimSpace(cfg.Addrs) != "" {
		info.Addrs = strings.Fields(cfg.Addrs)
	}
	info.Gateway = cfg.Gateway
	info.Metric = cfg.Metric
	if cfg.DNS != "" {
		info.DNS = strings.Fields(cfg.DNS)
	}
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

	// 校验输入
	addrs := parseAddrList(req.Addrs)
	if len(addrs) == 0 {
		return nil, fmt.Errorf("请至少配置一个 IP 地址（CIDR 格式，如 192.168.1.10/24）")
	}
	for _, addr := range addrs {
		if !isValidCIDR(addr) {
			return nil, fmt.Errorf("IP 地址格式无效: %s（请使用 CIDR 格式，如 192.168.1.10/24）", addr)
		}
	}
	gateway := strings.TrimSpace(req.Gateway)
	if gateway != "" && net.ParseIP(gateway) == nil {
		return nil, fmt.Errorf("网关地址格式无效: %s", gateway)
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
	if err := applyStaticConfig(name, addrs, gateway, dnsServers, current); err != nil {
		return nil, err
	}

	logger.App.Info("接口 IP/DNS 配置已更新", "interface", name,
		"addrs", addrs, "gateway", gateway, "dns", dnsServers)

	return GetInterfaceConfig(name)
}

// clearInterfaceConfig 清除接口上的所有静态 IP/DNS 配置。
func clearInterfaceConfig(name string, current *InterfaceConfigInfo) (*InterfaceConfigInfo, error) {
	// 清除运行时 IP
	utils.ExecCommand("bash", "-c", fmt.Sprintf(
		`ip route del default dev %s 2>/dev/null || true
ip -4 addr flush dev %s scope global 2>/dev/null || true`,
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
func applyStaticConfig(name string, addrs []string, gateway string, dns []string, current *InterfaceConfigInfo) error {
	// 保留现有 metric
	metric := current.Metric

	// 构建并执行 shell 脚本
	script := fmt.Sprintf(`set -e
IFACE=%s
# 删除旧默认路由
ip route del default dev "$IFACE" 2>/dev/null || true
# 清除旧 IPv4 全局地址
ip -4 addr flush dev "$IFACE" scope global 2>/dev/null || true
`, utils.ShellSingleQuote(name))

	for _, addr := range addrs {
		script += fmt.Sprintf("ip addr replace %s dev \"$IFACE\"\n", utils.ShellSingleQuote(addr))
	}

	if gateway != "" {
		script += fmt.Sprintf(`ip route replace %s dev "$IFACE" scope link 2>/dev/null || true
`, utils.ShellSingleQuote(gateway))
		if metric != "" {
			script += fmt.Sprintf("ip route replace default via %s dev \"$IFACE\" metric %s\n",
				utils.ShellSingleQuote(gateway), utils.ShellSingleQuote(metric))
		} else {
			script += fmt.Sprintf("ip route replace default via %s dev \"$IFACE\"\n",
				utils.ShellSingleQuote(gateway))
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
			if model.DB != nil {
				model.DB.Save(&row)
			}
			cfg := HostIPConfig{
				Addrs:   row.HostAddrs,
				Gateway: row.HostGateway,
				Metric:  row.HostMetric,
				DNS:     row.HostDNS,
			}
			if err := writeBridgeRestoreScript(row.Name, row.UplinkIF, true, cfg); err != nil {
				logger.App.Warn("重写网桥恢复脚本失败", "bridge", name, "error", err)
			}
		}
	} else if current.Type == "nic" {
		// 独立物理网卡：写入 networkd 静态配置以持久化
		if err := writeNetworkdStaticConfig(name, addrs, gateway, dns); err != nil {
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

func writeNetworkdStaticConfig(iface string, addrs []string, gateway string, dns []string) error {
	// 仅当 systemd-networkd 活跃时写入
	if utils.ExecCommand("systemctl", "is-active", "--quiet", "systemd-networkd").Error != nil {
		logger.App.Debug("systemd-networkd 不活跃，跳过 networkd 静态配置持久化", "interface", iface)
		return nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("[Match]\nName=%s\n\n[Network]\n", iface))
	for _, addr := range addrs {
		b.WriteString(fmt.Sprintf("Address=%s\n", addr))
	}
	if gateway != "" {
		b.WriteString(fmt.Sprintf("Gateway=%s\n", gateway))
	}
	for _, d := range dns {
		b.WriteString(fmt.Sprintf("DNS=%s\n", d))
	}
	b.WriteString("DHCP=no\nLinkLocalAddressing=no\n")

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
	if utils.ExecCommand("systemctl", "is-active", "--quiet", "systemd-networkd").Error == nil {
		utils.ExecCommand("networkctl", "reload")
		logger.App.Info("已移除 networkd 静态配置", "interface", iface)
	}
}
