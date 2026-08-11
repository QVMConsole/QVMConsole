package ip_resolver

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"kvm_console/model"
	"kvm_console/service/guest_agent"
	"kvm_console/utils"
)

// ==================== 依赖注入回调 ====================
// ip_resolver 不能 import service（会循环依赖），通过回调注入 service 层的 VPC/OVS 函数。
// service 包在初始化时调用 SetResolverCallbacks 注册这些回调。

// VPCCallbacks VPC/OVS 相关回调，由 service 包在初始化时注入
type VPCCallbacks struct {
	// GetVPCSwitchForVM 返回 VM 所属 VPC 交换机（包含 CIDR 等信息）
	GetVPCSwitchForVM func(vmName string) (*model.VPCSwitch, bool)
	// GetVPCLeaseIPForVM 通过 VPC 租约/静态绑定查找 VM IP
	GetVPCLeaseIPForVM func(vmName string) string
	// GetOVSLeaseIPByMAC 通过 MAC 在 OVS DHCP 租约中查找 IP
	GetOVSLeaseIPByMAC func(mac string) string
	// GetOVSStaticIPByMAC 通过 MAC 在 OVS 静态绑定中查找 IP
	GetOVSStaticIPByMAC func(mac string) string
	// GetVPCStaticIPByMACAndCIDR 在所有 VPC 静态绑定中按 MAC+CIDR 查找 IP
	GetVPCStaticIPByMACAndCIDR func(mac, cidr string) string
}

var vpcCallbacks VPCCallbacks

// SetResolverCallbacks 注册 service 层的 VPC/OVS 回调函数（避免循环依赖）
func SetResolverCallbacks(cb VPCCallbacks) {
	vpcCallbacks = cb
}

// ==================== IP 解析 ====================

// GetVMIP 获取虚拟机 IP 地址
// VPC VM 优先使用 VPC 静态绑定或租约；普通 VM 依次尝试 Guest Agent → ARP 表 → DHCP 租约 → 静态绑定
// isRunning 表示虚拟机是否在运行，关机状态跳过前三种方式（避免无效命令调用）
func GetVMIP(name string, isRunning bool) string {
	ipRe := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`)

	// VPC 路径
	if vpcCallbacks.GetVPCSwitchForVM != nil {
		if sw, ok := vpcCallbacks.GetVPCSwitchForVM(name); ok {
			if ip := vpcCallbacks.GetVPCLeaseIPForVM(name); ip != "" {
				return ip
			}
			if isRunning {
				// 优先使用 Guest Agent
				if mac := GetFirstVMMAC(name); mac != "" {
					if ip, ok := guest_agent.GetVMIPByMACFromAgent(name, mac); ok && IPInCIDR(ip, sw.CIDR) {
						return ip
					}
				}
				if ip := getVMIPFromDomifaddrSource(name, "arp", ipRe, sw.CIDR, true); ip != "" {
					return ip
				}
				if mac := GetFirstVMMAC(name); mac != "" {
					if vpcCallbacks.GetOVSLeaseIPByMAC != nil {
						if ip := vpcCallbacks.GetOVSLeaseIPByMAC(mac); ip != "" && IPInCIDR(ip, sw.CIDR) {
							return ip
						}
					}
					if vpcCallbacks.GetVPCStaticIPByMACAndCIDR != nil {
						if ip := vpcCallbacks.GetVPCStaticIPByMACAndCIDR(mac, sw.CIDR); ip != "" {
							return ip
						}
					}
					if ip := GetHostNeighborIPByMAC(mac, sw.CIDR, true); ip != "" {
						return ip
					}
				}
				// VPC 所有方式均失败时，尝试主动 ARP 扫描（桥接直通模式兜底）
				if ip := getVMIPByActiveScan(name); ip != "" {
					return ip
				}
				// 桥接/直通模式兜底：ARP 不加 CIDR 过滤
				if ip := execDomifaddrARP(name, ipRe); ip != "" {
					return ip
				}
			}
			return ""
		}
	}

	if isRunning {
		// 方式1: QEMU Guest Agent（最准确，需虚拟机安装 qemu-guest-agent）
		if mac := GetFirstVMMAC(name); mac != "" {
			if ip, ok := guest_agent.GetVMIPByMACFromAgent(name, mac); ok {
				return ip
			}
		}

		// 方式2: ARP 表（反映当前实际网络通信状态，比 DHCP 租约更可靠）
		result := utils.ExecCommandQuiet("virsh", "domifaddr", name, "--source", "arp")
		if result.Error == nil {
			allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
			if len(allMatches) == 1 {
				return allMatches[0][1]
			} else if len(allMatches) > 1 {
				for i := len(allMatches) - 1; i >= 0; i-- {
					ip := allMatches[i][1]
					pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", ip)
					if pingResult.ExitCode == 0 {
						return ip
					}
				}
				return allMatches[len(allMatches)-1][1]
			}
		}

		// 方式3: VPC dnsmasq 租约（VM 已绑定 VPC 时优先读取对应交换机租约）
		if vpcCallbacks.GetVPCLeaseIPForVM != nil {
			if ip := vpcCallbacks.GetVPCLeaseIPForVM(name); ip != "" {
				return ip
			}
		}

		// 方式4: OVS dnsmasq 租约
		if mac := GetFirstVMMAC(name); mac != "" {
			if vpcCallbacks.GetOVSLeaseIPByMAC != nil {
				if ip := vpcCallbacks.GetOVSLeaseIPByMAC(mac); ip != "" {
					return ip
				}
			}
			if ip := GetHostNeighborIPByMAC(mac, "", true); ip != "" {
				return ip
			}
		}

		// 方式5: libvirt DHCP 租约
		result = utils.ExecCommandQuiet("virsh", "domifaddr", name, "--source", "lease")
		if result.Error == nil {
			allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
			if len(allMatches) > 0 {
				return allMatches[len(allMatches)-1][1]
			}
		}

		// 方式6: 默认模式
		result = utils.ExecCommandQuiet("virsh", "domifaddr", name)
		if result.Error == nil {
			allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
			if len(allMatches) > 0 {
				return allMatches[len(allMatches)-1][1]
			}
		}
	}

	// 方式7: 从 OVS 静态绑定中查找（兜底方案，不依赖虚拟机运行状态）
	mac := GetFirstVMMAC(name)
	if mac != "" {
		if vpcCallbacks.GetOVSStaticIPByMAC != nil {
			if ip := vpcCallbacks.GetOVSStaticIPByMAC(mac); ip != "" {
				return ip + " (静态)"
			}
		}
	}

	// 方式8: 桥接模式主动 ARP 扫描（兜底，有频率限制）
	if isRunning {
		if ip := getVMIPByActiveScan(name); ip != "" {
			return ip
		}
	}

	return ""
}

// GetVMIPStatus 获取虚拟机 IP 状态（用于前端展示"无法获取"提示）
// 返回 "" 表示正常，返回非空表示无法获取的原因
func GetVMIPStatus(name string, isRunning bool) string {
	if !isRunning {
		return "shut_off"
	}
	if HasVMBridgeVLANTag(name) {
		return "vlan_bridge"
	}
	return ""
}

// ==================== 辅助函数 ====================

// IPInCIDR 检查 IP 是否属于指定 CIDR 子网
func IPInCIDR(ipText, cidrText string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipText))
	_, network, err := net.ParseCIDR(strings.TrimSpace(cidrText))
	return ip != nil && err == nil && network.Contains(ip)
}

func getVMIPFromDomifaddrSource(name, source string, ipRe *regexp.Regexp, cidr string, verifyPing bool) string {
	result := utils.ExecCommandQuiet("virsh", "domifaddr", name, "--source", source)
	if result.Error != nil {
		return ""
	}
	allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
	var candidates []string
	for _, match := range allMatches {
		if len(match) < 2 || match[1] == "127.0.0.1" {
			continue
		}
		if cidr != "" && !IPInCIDR(match[1], cidr) {
			continue
		}
		candidates = append(candidates, match[1])
	}
	if len(candidates) == 0 {
		return ""
	}
	if verifyPing {
		for i := len(candidates) - 1; i >= 0; i-- {
			pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", candidates[i])
			if pingResult.ExitCode == 0 {
				return candidates[i]
			}
		}
	}
	return candidates[0]
}

// execDomifaddrARP 通过 virsh domifaddr --source arp 获取 VM IP（不限制 CIDR）
func execDomifaddrARP(name string, ipRe *regexp.Regexp) string {
	result := utils.ExecCommandQuiet("virsh", "domifaddr", name, "--source", "arp")
	if result.Error != nil {
		return ""
	}
	allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
	if len(allMatches) == 0 {
		return ""
	}
	if len(allMatches) == 1 {
		return allMatches[0][1]
	}
	for i := len(allMatches) - 1; i >= 0; i-- {
		ip := allMatches[i][1]
		pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", ip)
		if pingResult.ExitCode == 0 {
			return ip
		}
	}
	return allMatches[len(allMatches)-1][1]
}

func GetHostNeighborIPByMAC(mac, cidr string, verifyPing bool) string {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return ""
	}
	result := utils.ExecCommand("ip", "neigh", "show")
	if result.Error != nil {
		return ""
	}
	candidates := parseHostNeighborIPsByMAC(result.Stdout, mac, cidr)
	if len(candidates) == 0 {
		return ""
	}
	if verifyPing {
		for i := len(candidates) - 1; i >= 0; i-- {
			pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", candidates[i])
			if pingResult.ExitCode == 0 {
				return candidates[i]
			}
		}
	}
	return candidates[len(candidates)-1]
}

func parseHostNeighborIPsByMAC(text, mac, cidr string) []string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	var ips []string
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		ip := fields[0]
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] == "lladdr" && strings.EqualFold(fields[i+1], mac) {
				if cidr == "" || IPInCIDR(ip, cidr) {
					ips = append(ips, ip)
				}
				break
			}
		}
	}
	return ips
}

// ==================== 主动 ARP 扫描（桥接模式 IP 发现） ====================

var (
	activeScanMu         sync.Mutex
	lastActiveScanTime   time.Time
	lastActiveScanBridge string
	lastActiveScanMacIPs map[string]string // MAC -> IP 映射缓存
)

// getVMIPByActiveScan 通过主动 ARP 扫描获取桥接模式虚拟机的 IP
func getVMIPByActiveScan(vmName string) string {
	if HasVMBridgeVLANTag(vmName) {
		return ""
	}

	mac := GetFirstVMMAC(vmName)
	if mac == "" {
		return ""
	}
	mac = strings.ToLower(strings.TrimSpace(mac))

	bridge := GetVMBridgeInterface(vmName)
	if bridge == "" {
		return ""
	}

	activeScanMu.Lock()
	if lastActiveScanBridge == bridge && time.Since(lastActiveScanTime) < 12*time.Second && lastActiveScanMacIPs != nil {
		cached := lastActiveScanMacIPs[mac]
		activeScanMu.Unlock()
		return cached
	}
	activeScanMu.Unlock()

	cidr := getInterfaceCIDR(bridge)
	if cidr == "" {
		return ""
	}

	var macIPs map[string]string

	// 方式1: arp-scan
	result := utils.ExecCommandWithTimeout("arp-scan",
		20*time.Second,
		"--interface="+bridge,
		"--localnet",
		"--quiet",
		"--ignoredups")
	if result.Error == nil && result.Stdout != "" {
		macIPs = parseARPScanMacIPs(result.Stdout)
		if len(macIPs) > 0 {
			activeScanMu.Lock()
			lastActiveScanTime = time.Now()
			lastActiveScanBridge = bridge
			lastActiveScanMacIPs = macIPs
			activeScanMu.Unlock()
			return macIPs[mac]
		}
	}

	// 方式2: nmap
	_ = utils.ExecCommandWithTimeout("nmap", 15*time.Second,
		"-sn", "-PR", cidr,
		"--max-retries", "1",
		"--host-timeout", "1s")
	time.Sleep(500 * time.Millisecond)
	macIPs = getAllHostNeighborMacIPs(bridge)
	if len(macIPs) > 0 {
		activeScanMu.Lock()
		lastActiveScanTime = time.Now()
		lastActiveScanBridge = bridge
		lastActiveScanMacIPs = macIPs
		activeScanMu.Unlock()
		return macIPs[mac]
	}

	return ""
}

func getInterfaceCIDR(iface string) string {
	result := utils.ExecShell(fmt.Sprintf("ip -4 -o addr show %s 2>/dev/null | awk '{print $4}' | head -1", utils.ShellSingleQuote(iface)))
	cidr := strings.TrimSpace(result.Stdout)
	if cidr == "" {
		return ""
	}
	return cidr
}

func parseARPScanMacIPs(output string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ip := fields[0]
			mac := strings.ToLower(fields[1])
			if matched, _ := regexp.MatchString(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`, mac); matched {
				m[mac] = ip
			}
		}
	}
	return m
}

func getAllHostNeighborMacIPs(bridge string) map[string]string {
	result := utils.ExecShell(fmt.Sprintf("ip neigh show dev %s 2>/dev/null", utils.ShellSingleQuote(bridge)))
	if result.Error != nil {
		return nil
	}
	m := make(map[string]string)
	macRe := regexp.MustCompile(`([0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2})`)
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip := fields[0]
		macMatches := macRe.FindAllString(strings.ToLower(line), 1)
		if len(macMatches) > 0 {
			m[macMatches[0]] = ip
		}
	}
	return m
}

// GetAllVMIPs 获取虚拟机所有 IP 地址（IPv4 + IPv6），去重后返回
// 优先通过 Guest Agent 获取，失败则通过 virsh domifaddr 解析
func GetAllVMIPs(name string, isRunning bool) []string {
	seen := make(map[string]bool)
	var ips []string

	addIP := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" || seen[ip] {
			return
		}
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
			return
		}
		seen[ip] = true
		ips = append(ips, ip)
	}

	// 方式1: Guest Agent（最准确，同时支持 IPv4 和 IPv6）
	if isRunning {
		agentIPs := guest_agent.GetVMAllAgentIPs(name)
		for _, ip := range agentIPs {
			addIP(ip)
		}
		if len(ips) > 0 {
			return ips
		}
	}

	// 方式2: virsh domifaddr（默认 source，同时显示 IPv4 和 IPv6）
	ipRe := regexp.MustCompile(`(?:ipv4\s+(\S+)|ipv6\s+(\S+))`)
	result := utils.ExecCommandQuiet("virsh", "domifaddr", name)
	if result.Error == nil {
		matches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
		for _, m := range matches {
			if len(m) >= 3 {
				if m[1] != "" {
					addIP(strings.SplitN(m[1], "/", 2)[0])
				} else if m[2] != "" {
					addIP(strings.SplitN(m[2], "/", 2)[0])
				}
			}
		}
		if len(ips) > 0 {
			return ips
		}
	}

	// 方式3: 如果只有一个 IP，作为兜底
	if isRunning {
		if ip := GetVMIP(name, true); ip != "" {
			addIP(ip)
		}
	}

	return ips
}

// init 初始化日志
// 回调注册由 service 包在 SetResolverCallbacks 中完成
