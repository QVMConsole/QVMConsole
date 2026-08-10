package portsecurity

import "time"

const (
	PolicyCookie        = "0x51564d5053454301"
	QuarantineCookie    = "0x51564d50534543ff"
	TableIdentity       = 10
	TableRateLimit      = 20
	TableBandwidth      = 30
	ModeStrict          = "strict"
	ModeCompatible      = "compatible"
	ModeQuarantined     = "quarantined"
	ModeDisabled        = "disabled"
	ExternalIDOwner     = "kvm-console-port-security"
	ExternalIDIsolated  = "kvm-console-port-isolated"
	ExternalIDVM        = "kvm-console-port-security-vm"
	ExternalIDMode      = "kvm-console-port-security-mode"
	ExternalIDNeighbor  = "kvm-console-port-security-neighbor-meter"
	ExternalIDBroadcast = "kvm-console-port-security-broadcast-meter"
	ExternalIDMeterMap  = "kvm-console-port-security-meter-map"
	ExternalIDLastError = "kvm-console-port-security-last-error"
)

// Issue 表示端口安全预检或协调过程中发现的问题。
type Issue struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	Bridge         string `json:"bridge,omitempty"`
	Port           string `json:"port,omitempty"`
	VMName         string `json:"vm_name,omitempty"`
	InterfaceOrder int    `json:"interface_order,omitempty"`
	Blocking       bool   `json:"blocking"`
}

// BridgeCapability 描述单个 OVS 网桥的防护能力。
type BridgeCapability struct {
	Bridge               string `json:"bridge"`
	Exists               bool   `json:"exists"`
	OpenFlow13           bool   `json:"openflow13"`
	OpenFlow14Bundle     bool   `json:"openflow14_bundle"`
	PacketMeters         bool   `json:"packet_meters"`
	PacketPolicing       bool   `json:"packet_policing"`
	MaxMeters            int    `json:"max_meters"`
	ExistingMeters       int    `json:"existing_meters"`
	RequiredMeters       int    `json:"required_meters"`
	SequentialApplyGuard bool   `json:"sequential_apply_guard"`
}

// PortStatus 描述虚拟机 OVS 端口的身份和运行时防护状态。
type PortStatus struct {
	Bridge                string   `json:"bridge"`
	Port                  string   `json:"port"`
	OFPort                string   `json:"ofport"`
	VMName                string   `json:"vm_name"`
	InterfaceOrder        int      `json:"interface_order"`
	MAC                   string   `json:"mac"`
	SwitchID              uint     `json:"switch_id"`
	SwitchName            string   `json:"switch_name"`
	DirectBridge          bool     `json:"direct_bridge"`
	Mode                  string   `json:"mode"`
	IPv6Enabled           bool     `json:"ipv6_enabled"`
	AllowedIPv4Addresses  []string `json:"allowed_ipv4_addresses"`
	AllowedIPv6Addresses  []string `json:"allowed_ipv6_addresses"`
	TrustedIPv6Prefixes   []string `json:"trusted_ipv6_prefixes"`
	NeighborMeterID       uint32   `json:"neighbor_meter_id,omitempty"`
	BroadcastMeterID      uint32   `json:"broadcast_meter_id,omitempty"`
	PolicingKpps          int      `json:"policing_kpps"`
	PolicingBurstKPackets int      `json:"policing_burst_kpackets"`
	Isolated              bool     `json:"isolated"`
	Applied               bool     `json:"applied"`
	DropPackets           uint64   `json:"drop_packets"`
	NeighborDropPackets   uint64   `json:"neighbor_drop_packets"`
	BroadcastDropPackets  uint64   `json:"broadcast_drop_packets"`
	LastError             string   `json:"last_error,omitempty"`
}

// PreflightResult 是开启总开关前的完整检查结果。
type PreflightResult struct {
	Ready        bool               `json:"ready"`
	Enabled      bool               `json:"enabled"`
	Capabilities []BridgeCapability `json:"capabilities"`
	Ports        []PortStatus       `json:"ports"`
	Issues       []Issue            `json:"issues"`
	CheckedAt    time.Time          `json:"checked_at"`
}

// Status 是端口安全的聚合运行状态。
type Status struct {
	Enabled         bool         `json:"enabled"`
	Healthy         bool         `json:"healthy"`
	AppliedPorts    int          `json:"applied_ports"`
	CompatiblePorts int          `json:"compatible_ports"`
	IsolatedPorts   int          `json:"isolated_ports"`
	Ports           []PortStatus `json:"ports"`
	Issues          []Issue      `json:"issues"`
	LastReconciled  time.Time    `json:"last_reconciled,omitempty"`
}

// TaskParams 描述端口安全异步任务。
type TaskParams struct {
	Action string `json:"action"`
	Port   string `json:"port,omitempty"`
}

type policyPort struct {
	PortStatus
	StrictIPv4 bool
}
