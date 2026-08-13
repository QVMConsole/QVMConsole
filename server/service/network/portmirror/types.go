package portmirror

import "time"

const (
	ConfigDir         = "/etc/kvm-console/port-mirror"
	ConfigPath        = ConfigDir + "/config.json"
	RuntimePath       = "/run/kvm-console/port-mirror-runtime.json"
	WatchdogPath      = "/run/kvm-console/port-mirror-watchdog.json"
	WatchdogUnit      = "qvm-port-mirror-watchdog"
	CookiePrefix      = uint64(0x51564d4d00000000)
	CookiePrefixText  = "0x51564d4d00000000"
	CookiePrefixMask  = "0xffffffff00000000"
	IngressPreference = 49152
	EgressPreference  = 49153
	DirectionIngress  = "ingress"
	DirectionEgress   = "egress"
	DirectionBoth     = "both"
)

type TargetConfig struct {
	SwitchID   uint   `json:"switch_id"`
	SwitchName string `json:"switch_name"`
	Bridge     string `json:"bridge"`
}

// Config 是持久化的端口镜像目标状态，不保存虚拟机运行态信息。
type Config struct {
	Enabled          bool           `json:"enabled"`
	SourceInterfaces []string       `json:"source_interfaces"`
	Targets          []TargetConfig `json:"targets"`
	Direction        string         `json:"direction"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type RuntimeSource struct {
	Name          string `json:"name"`
	ClsactCreated bool   `json:"clsact_created"`
}

type RuntimeConnection struct {
	SourceInterface string `json:"source_interface"`
	TargetSwitchID  uint   `json:"target_switch_id"`
	TargetBridge    string `json:"target_bridge"`
	VethSource      string `json:"veth_source"`
	OVSPort         string `json:"ovs_port"`
	OFPort          int    `json:"ofport"`
	Cookie          string `json:"cookie"`
}

// RuntimeState 记录多源与多目标笛卡尔积所创建的临时对象。
type RuntimeState struct {
	Config
	Sources     []RuntimeSource     `json:"sources"`
	Connections []RuntimeConnection `json:"connections"`
}

type EnableRequest struct {
	SourceInterfaces []string `json:"source_interfaces"`
	TargetSwitchIDs  []uint   `json:"target_switch_ids"`
	Direction        string   `json:"direction"`
}

type TaskParams struct {
	Action  string        `json:"action"`
	Request EnableRequest `json:"request,omitempty"`
}

type SourceOption struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	State        string   `json:"state"`
	Addresses    []string `json:"addresses"`
	DefaultRoute bool     `json:"default_route"`
	CaptureStage string   `json:"capture_stage"`
	Risk         string   `json:"risk,omitempty"`
}

type TargetOption struct {
	SwitchID   uint   `json:"switch_id"`
	SwitchName string `json:"switch_name"`
	Bridge     string `json:"bridge"`
	VMCount    int64  `json:"vm_count"`
}

type Options struct {
	Sources []SourceOption `json:"sources"`
	Targets []TargetOption `json:"targets"`
}

type DirectionStats struct {
	Enabled bool   `json:"enabled"`
	Packets uint64 `json:"packets"`
	Bytes   uint64 `json:"bytes"`
	Dropped uint64 `json:"dropped"`
}

type SourceStatus struct {
	SourceInterface string         `json:"source_interface"`
	Ingress         DirectionStats `json:"ingress"`
	Egress          DirectionStats `json:"egress"`
}

type TargetStatus struct {
	SwitchID    uint   `json:"switch_id"`
	SwitchName  string `json:"switch_name"`
	Bridge      string `json:"bridge"`
	Connections int    `json:"connections"`
	OVSPackets  uint64 `json:"ovs_packets"`
	OVSBytes    uint64 `json:"ovs_bytes"`
}

type Status struct {
	Enabled          bool           `json:"enabled"`
	Healthy          bool           `json:"healthy"`
	SourceInterfaces []string       `json:"source_interfaces"`
	Targets          []TargetConfig `json:"targets"`
	Direction        string         `json:"direction,omitempty"`
	Sources          []SourceStatus `json:"sources"`
	TargetStats      []TargetStatus `json:"target_stats"`
	Ingress          DirectionStats `json:"ingress"`
	Egress           DirectionStats `json:"egress"`
	OVSPackets       uint64         `json:"ovs_packets"`
	OVSBytes         uint64         `json:"ovs_bytes"`
	Issues           []string       `json:"issues"`
	UpdatedAt        time.Time      `json:"updated_at,omitempty"`
}

type watchdogState struct {
	Token   string       `json:"token"`
	Unit    string       `json:"unit"`
	Runtime RuntimeState `json:"runtime"`
}
