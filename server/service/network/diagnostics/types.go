package diagnostics

import "time"

// NetworkDiagnosticFilter 网络诊断过滤条件
type NetworkDiagnosticFilter struct {
	Protocol   string `json:"protocol"`
	SourceIP   string `json:"source_ip"`
	DestIP     string `json:"dest_ip"`
	Port       int    `json:"port"`
	SourcePort int    `json:"source_port"`
	DestPort   int    `json:"dest_port"`
}

// NetworkCaptureRequest 网络抓包请求
type NetworkCaptureRequest struct {
	InterfaceName   string                  `json:"interface_name"`
	Filter          NetworkDiagnosticFilter `json:"filter"`
	DurationSeconds int                     `json:"duration_seconds"`
	MaxMB           int                     `json:"max_mb"`
	MaxPackets      int                     `json:"max_packets"`
}

// NetworkCaptureParams 网络抓包参数
type NetworkCaptureParams struct {
	VMName string `json:"vm_name"`
	NetworkCaptureRequest
}

// NetworkDiagnosticTemplate 网络诊断模板
type NetworkDiagnosticTemplate struct {
	Key         string                  `json:"key"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Filter      NetworkDiagnosticFilter `json:"filter"`
}

// VMNetworkDiagnostics 虚拟机网络诊断结果
type VMNetworkDiagnostics struct {
	VMName              string                      `json:"vm_name"`
	State               string                      `json:"state"`
	Interfaces          []VMNetworkInterface        `json:"interfaces"`
	Neighbors           []string                    `json:"neighbors"`
	Templates           []NetworkDiagnosticTemplate `json:"templates"`
	PortForwards        []PortForwardRule           `json:"port_forwards"`
	DefaultInterface    string                      `json:"default_interface"`
	DefaultIP           string                      `json:"default_ip"`
	PortSecurityEnabled bool                        `json:"port_security_enabled"`
	Issues              []string                    `json:"issues"`
}

// NetworkCaptureSession 网络抓包会话
type NetworkCaptureSession struct {
	TaskID          uint                    `json:"task_id"`
	VMName          string                  `json:"vm_name"`
	InterfaceName   string                  `json:"interface_name"`
	Filter          NetworkDiagnosticFilter `json:"filter"`
	BPF             string                  `json:"bpf"`
	Status          string                  `json:"status"`
	Message         string                  `json:"message"`
	FileName        string                  `json:"file_name"`
	DownloadPath    string                  `json:"download_path"`
	FileSize        int64                   `json:"file_size"`
	DurationSeconds int                     `json:"duration_seconds"`
	MaxMB           int                     `json:"max_mb"`
	MaxPackets      int                     `json:"max_packets"`
	SummaryLines    []string                `json:"summary_lines"`
	StartedAt       time.Time               `json:"started_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
	FinishedAt      *time.Time              `json:"finished_at,omitempty"`
}

// ── Equivalent types (避免直接引用 service 根包，避免循环 import) ──

// VMNetworkInterface 等价于 ovs.VMNetworkInterface
type VMNetworkInterface struct {
	InterfaceType   string                `json:"interface_type"`
	Target          string                `json:"target"`
	SourceBridge    string                `json:"source_bridge"`
	SourceNetwork   string                `json:"source_network"`
	Model           string                `json:"model"`
	MAC             string                `json:"mac"`
	VirtualPortType string                `json:"virtualport_type"`
	OFPort          string                `json:"ofport"`
	IP              string                `json:"ip"`
	IPSource        string                `json:"ip_source"`
	PortSecurity    *VMPortSecurityStatus `json:"port_security,omitempty"`
	Issues          []string              `json:"issues"`
}

// VMPortSecurityStatus 是诊断模块使用的逐端口防护状态镜像。
type VMPortSecurityStatus struct {
	Mode                  string   `json:"mode"`
	AllowedIPv4Addresses  []string `json:"allowed_ipv4_addresses"`
	AllowedIPv6Addresses  []string `json:"allowed_ipv6_addresses"`
	NeighborMeterID       uint32   `json:"neighbor_meter_id,omitempty"`
	BroadcastMeterID      uint32   `json:"broadcast_meter_id,omitempty"`
	PolicingKpps          int      `json:"policing_kpps"`
	PolicingBurstKPackets int      `json:"policing_burst_kpackets"`
	DropPackets           uint64   `json:"drop_packets"`
	NeighborDropPackets   uint64   `json:"neighbor_drop_packets"`
	BroadcastDropPackets  uint64   `json:"broadcast_drop_packets"`
	Applied               bool     `json:"applied"`
	Isolated              bool     `json:"isolated"`
	LastError             string   `json:"last_error,omitempty"`
}

// VMNetworkRuntimeStatus 等价于 ovs.VMNetworkRuntimeStatus
type VMNetworkRuntimeStatus struct {
	VMName              string               `json:"vm_name"`
	State               string               `json:"state"`
	Bridge              string               `json:"bridge"`
	Interfaces          []VMNetworkInterface `json:"interfaces"`
	PortSecurityEnabled bool                 `json:"port_security_enabled"`
	Issues              []string             `json:"issues"`
}

// PortForwardRule 等价于 network.PortForwardRule（仅含 diagnostics 所需字段）
type PortForwardRule struct {
	Protocol string `json:"protocol"`
	HostPort string `json:"host_port"`
	DestIP   string `json:"dest_ip"`
	DestPort string `json:"dest_port"`
	RuleKey  string `json:"rule_key"`
}

// StableKey 返回端口转发规则的稳定标识
func (r PortForwardRule) StableKey() string {
	// 使用 RuleKey 如果可用，否则构造
	if r.RuleKey != "" {
		return r.RuleKey
	}
	return r.Protocol + "|" + r.HostPort + "|" + r.DestIP + "|" + r.DestPort
}
