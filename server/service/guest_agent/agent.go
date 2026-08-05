package guest_agent

import (
	"context"
	"fmt"
	"net"
	"strings"

	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// GuestAgentStatus 描述虚拟机 QEMU Guest Agent 的当前状态
type GuestAgentStatus struct {
	Configured bool   `json:"configured"` // XML 中有 org.qemu.guest_agent.0 通道
	Connected  bool   `json:"connected"`  // agent 正在响应 guest-ping
	Version    string `json:"version"`    // agent 版本号
}

// GuestAgentIPResult 按 MAC 分组的 IP 地址结果
type GuestAgentIPResult struct {
	MAC string   `json:"mac"`
	IPs []string `json:"ips"` // 该 MAC 对应的 IP 地址列表
}

// guestNetworkInterface JSON 解析用的中间结构
type guestNetworkInterface struct {
	Name            string           `json:"name"`
	HardwareAddress string           `json:"hardware-address"`
	IPAddresses     []guestIPAddress `json:"ip-addresses"`
}

// guestIPAddress JSON 解析用的 IP 地址
type guestIPAddress struct {
	IPAddressType string `json:"ip-address-type"`
	IPAddress     string `json:"ip-address"`
	Prefix        int    `json:"prefix"`
}

// guestInfoResponse JSON 解析用的 guest-info 返回
type guestInfoResponse struct {
	Return struct {
		Version           string `json:"version"`
		SupportedCommands []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"supported_commands"`
	} `json:"return"`
}

// guestPingResponse JSON 解析用的 guest-ping 返回
type guestPingResponse struct {
	Return struct{} `json:"return"`
}

// guestNetworkResponse JSON 解析用的 guest-network-get-interfaces 返回
type guestNetworkResponse struct {
	Return []guestNetworkInterface `json:"return"`
}

type guestExecStartResponse struct {
	Return struct {
		PID int `json:"pid"`
	} `json:"return"`
}

type guestExecStatusResponse struct {
	Return struct {
		Exited   bool   `json:"exited"`
		ExitCode int    `json:"exitcode"`
		ErrData  string `json:"err-data"`
	} `json:"return"`
}

type guestOSInfoResponse struct {
	Return struct {
		ID string `json:"id"`
	} `json:"return"`
}

// CheckVMGuestAgentStatus 检查虚拟机 Guest Agent 状态
// 返回的状态中 Configured 表示 XML 里有 GA 通道，Connected 表示 agent 正在响应
func CheckVMGuestAgentStatus(vmName string) *GuestAgentStatus {
	status := &GuestAgentStatus{}

	// 检查 XML 中是否配置了 GA 通道
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
		if err == nil && strings.Contains(xmlStr, "org.qemu.guest_agent.0") {
			status.Configured = true
		}
	}
	if !status.Configured {
		// 降级：通过 virsh dumpxml 检查
		result := utils.ExecCommandQuiet("virsh", "dumpxml", vmName)
		if result.Error == nil && strings.Contains(result.Stdout, "org.qemu.guest_agent.0") {
			status.Configured = true
		}
	}

	if !status.Configured {
		return status
	}

	client := NewClient(vmName)
	ctx, cancel := context.WithTimeout(context.Background(), ConnectTimeout)
	defer cancel()
	if client.Ping(ctx) == nil {
		status.Connected = true
	}

	// 获取版本号
	if status.Connected {
		if info, err := client.Info(ctx); err == nil {
			status.Version = info.Version
		}
	}

	return status
}

// GetVMGuestAgentIPs 从 QEMU Guest Agent 获取虚拟机所有网口的 IP 地址
// 返回按 MAC 分组的 IPv4/IPv6 地址列表，自动过滤 loopback 和 link-local 地址
func GetVMGuestAgentIPs(vmName string) ([]GuestAgentIPResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ConnectTimeout)
	defer cancel()
	var interfaces []guestNetworkInterface
	if err := NewClient(vmName).Command(ctx, "guest-network-get-interfaces", nil, &interfaces, ConnectTimeout); err != nil {
		return nil, err
	}

	var results []GuestAgentIPResult
	for _, iface := range interfaces {
		mac := strings.ToLower(strings.TrimSpace(iface.HardwareAddress))
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}

		var ips []string
		for _, addr := range iface.IPAddresses {
			if addr.IPAddressType != "ipv4" && addr.IPAddressType != "ipv6" {
				continue
			}
			ip := strings.TrimSpace(addr.IPAddress)
			if ip == "" {
				continue
			}

			// 过滤 loopback
			parsed := net.ParseIP(ip)
			if parsed == nil || parsed.IsLoopback() {
				continue
			}
			// 过滤链路本地地址，避免展示无法直接访问的地址。
			if parsed.IsLinkLocalUnicast() {
				continue
			}

			ips = append(ips, ip)
		}

		if len(ips) > 0 {
			results = append(results, GuestAgentIPResult{
				MAC: mac,
				IPs: ips,
			})
		}
	}

	if len(results) == 0 {
		logger.App.Debug("guest agent 未返回有效 IP 地址", "vm", vmName)
	} else {
		logger.App.Debug("guest agent 获取 IP 成功", "vm", vmName, "count", len(results))
	}

	return results, nil
}

// GetVMIPByMACFromAgent 从 Guest Agent 获取指定 MAC 的第一个 IPv4 地址
func GetVMIPByMACFromAgent(vmName, mac string) (string, bool) {
	mac = strings.ToLower(strings.TrimSpace(mac))
	if mac == "" {
		return "", false
	}

	results, err := GetVMGuestAgentIPs(vmName)
	if err != nil {
		return "", false
	}

	for _, r := range results {
		if r.MAC != mac {
			continue
		}
		for _, ip := range r.IPs {
			if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
				return ip, true
			}
		}
	}

	return "", false
}

// GetVMAllAgentIPs 获取虚拟机所有 Agent IP（汇总，去重）
func GetVMAllAgentIPs(vmName string) []string {
	results, err := GetVMGuestAgentIPs(vmName)
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var ips []string
	for _, r := range results {
		for _, ip := range r.IPs {
			if !seen[ip] {
				seen[ip] = true
				ips = append(ips, ip)
			}
		}
	}
	return ips
}

// ConfigureLinuxDHCPHotplugNetwork 为运行中的 Linux 来宾补齐附加网口的 DHCP 兜底规则。
// 主网口仍由优先级更高的 Netplan 规则管理；该规则只命中未被主规则匹配的 en* 网口。
func ConfigureLinuxDHCPHotplugNetwork(vmName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ExecuteTimeout)
	defer cancel()
	client := NewClient(vmName)
	osInfo, err := client.OSInfo(ctx)
	if err != nil {
		return fmt.Errorf("读取来宾系统信息失败: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(osInfo.ID), "mswindows") {
		return nil
	}

	const script = `if ! systemctl is-active --quiet systemd-networkd; then exit 0; fi
mkdir -p /etc/systemd/network
cat > /etc/systemd/network/99-qvm-hotplug.network <<'EOF'
[Match]
Name=en*

[Network]
DHCP=yes
LinkLocalAddressing=ipv6

[DHCP]
RouteMetric=200
UseMTU=true
EOF
networkctl reload
for qvm_iface in /sys/class/net/en*; do
  [ -e "$qvm_iface" ] || continue
  qvm_name=${qvm_iface##*/}
  ip link set dev "$qvm_name" up
  networkctl reconfigure "$qvm_name" || true
done`

	result, err := client.Execute(ctx, "/bin/sh", []string{"-c", script}, ExecuteTimeout)
	if err != nil {
		return fmt.Errorf("来宾网络配置命令执行失败: %w", err)
	}
	if result.ExitCode != 0 {
		if output := strings.TrimSpace(result.Stderr); output != "" {
			return fmt.Errorf("来宾网络配置命令失败: %s", output)
		}
		return fmt.Errorf("来宾网络配置命令失败，退出码 %d", result.ExitCode)
	}
	return nil
}
