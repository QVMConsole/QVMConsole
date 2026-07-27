package guest_agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

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
	IPs []string `json:"ips"` // 该 MAC 对应的 IPv4 地址列表
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

	// 检查 agent 是否连通
	pingResult := utils.ExecCommandQuiet("virsh", "qemu-agent-command", vmName,
		`{"execute":"guest-ping"}`)
	if pingResult.Error == nil && strings.Contains(pingResult.Stdout, "return") {
		status.Connected = true
	}

	// 获取版本号
	if status.Connected {
		infoResult := utils.ExecCommandQuiet("virsh", "qemu-agent-command", vmName,
			`{"execute":"guest-info"}`)
		if infoResult.Error == nil {
			var info guestInfoResponse
			if err := json.Unmarshal([]byte(infoResult.Stdout), &info); err == nil {
				status.Version = info.Return.Version
			}
		}
	}

	return status
}

// GetVMGuestAgentIPs 从 QEMU Guest Agent 获取虚拟机所有网口的 IP 地址
// 返回按 MAC 分组的 IPv4 地址列表，自动过滤 loopback 和 link-local 地址
func GetVMGuestAgentIPs(vmName string) ([]GuestAgentIPResult, error) {
	result := utils.ExecCommandQuiet("virsh", "qemu-agent-command", vmName,
		`{"execute":"guest-network-get-interfaces"}`)
	if result.Error != nil {
		return nil, fmt.Errorf("guest agent 命令执行失败: %w", result.Error)
	}

	var resp guestNetworkResponse
	if err := json.Unmarshal([]byte(result.Stdout), &resp); err != nil {
		return nil, fmt.Errorf("解析 guest agent 返回失败: %w", err)
	}

	var results []GuestAgentIPResult
	for _, iface := range resp.Return {
		mac := strings.ToLower(strings.TrimSpace(iface.HardwareAddress))
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}

		var ipv4s []string
		for _, addr := range iface.IPAddresses {
			if addr.IPAddressType != "ipv4" {
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
			// 过滤 link-local (169.254.x.x)
			if parsed.IsLinkLocalUnicast() {
				continue
			}

			ipv4s = append(ipv4s, ip)
		}

		if len(ipv4s) > 0 {
			results = append(results, GuestAgentIPResult{
				MAC: mac,
				IPs: ipv4s,
			})
		}
	}

	if len(results) == 0 {
		logger.App.Debug("guest agent 未返回有效 IPv4 地址", "vm", vmName)
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
		if r.MAC == mac && len(r.IPs) > 0 {
			return r.IPs[0], true
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
	osInfo := utils.ExecCommandQuiet("virsh", "qemu-agent-command", vmName,
		`{"execute":"guest-get-osinfo"}`)
	if osInfo.Error != nil {
		return fmt.Errorf("读取来宾系统信息失败: %w", osInfo.Error)
	}
	var osResult guestOSInfoResponse
	if err := json.Unmarshal([]byte(osInfo.Stdout), &osResult); err != nil {
		return fmt.Errorf("解析来宾系统信息失败: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(osResult.Return.ID), "mswindows") {
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

	request, err := json.Marshal(map[string]any{
		"execute": "guest-exec",
		"arguments": map[string]any{
			"path":           "/bin/sh",
			"arg":            []string{"-c", script},
			"capture-output": true,
		},
	})
	if err != nil {
		return fmt.Errorf("构造来宾网络配置命令失败: %w", err)
	}

	start := utils.ExecCommandQuiet("virsh", "qemu-agent-command", vmName, string(request))
	if start.Error != nil {
		return fmt.Errorf("启动来宾网络配置命令失败: %w", start.Error)
	}

	var started guestExecStartResponse
	if err := json.Unmarshal([]byte(start.Stdout), &started); err != nil || started.Return.PID <= 0 {
		if err != nil {
			return fmt.Errorf("解析来宾网络配置任务失败: %w", err)
		}
		return fmt.Errorf("来宾网络配置任务未返回进程号")
	}

	for i := 0; i < 20; i++ {
		statusRequest := fmt.Sprintf(`{"execute":"guest-exec-status","arguments":{"pid":%d}}`, started.Return.PID)
		status := utils.ExecCommandQuiet("virsh", "qemu-agent-command", vmName, statusRequest)
		if status.Error != nil {
			return fmt.Errorf("查询来宾网络配置任务失败: %w", status.Error)
		}

		var result guestExecStatusResponse
		if err := json.Unmarshal([]byte(status.Stdout), &result); err != nil {
			return fmt.Errorf("解析来宾网络配置任务状态失败: %w", err)
		}
		if !result.Return.Exited {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if result.Return.ExitCode != 0 {
			errOutput, decodeErr := base64.StdEncoding.DecodeString(result.Return.ErrData)
			if decodeErr == nil && strings.TrimSpace(string(errOutput)) != "" {
				return fmt.Errorf("来宾网络配置命令失败: %s", strings.TrimSpace(string(errOutput)))
			}
			return fmt.Errorf("来宾网络配置命令失败，退出码 %d", result.Return.ExitCode)
		}
		return nil
	}

	return fmt.Errorf("来宾网络配置任务超时")
}
