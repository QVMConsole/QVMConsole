package ovs

import "encoding/xml"

// ── OVS Static Host / DHCP types ──

// OVSStaticHost represents a static DHCP host binding entry.
type OVSStaticHost struct {
	VMName string
	MAC    string
	IP     string
}

// OVSDHCPLease represents a DHCP lease entry.
type OVSDHCPLease struct {
	ExpiryTime string
	ExpiryUnix int64
	MAC        string
	IP         string
	Hostname   string
	ClientID   string
}

// ── OVS Diagnostics types ──

// OVSStatus holds the full OVS network status.
type OVSStatus struct {
	Bridge             string              `json:"bridge"`
	GatewayIP          string              `json:"gateway_ip"`
	SubnetCIDR         string              `json:"subnet_cidr"`
	Uplink             string              `json:"uplink"`
	BridgeExists       bool                `json:"bridge_exists"`
	BridgeHasGateway   bool                `json:"bridge_has_gateway"`
	OpenVSwitchService OVSServiceStatus    `json:"openvswitch_service"`
	DNSMasqService     OVSServiceStatus    `json:"dnsmasq_service"`
	IPForwardEnabled   bool                `json:"ip_forward_enabled"`
	NATRule            OVSRuleStatus       `json:"nat_rule"`
	ForwardOutRule     OVSRuleStatus       `json:"forward_out_rule"`
	ForwardReturnRule  OVSRuleStatus       `json:"forward_return_rule"`
	Healthy            bool                `json:"healthy"`
	Issues             []string            `json:"issues"`
	RepairSuggestions  []string            `json:"repair_suggestions"`
	RawCommandErrors   []OVSCommandFailure `json:"raw_command_errors,omitempty"`
}

// OVSServiceStatus describes the status of a systemd service.
type OVSServiceStatus struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
}

// OVSRuleStatus describes the status of an iptables rule.
type OVSRuleStatus struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Exists  bool   `json:"exists"`
	Error   string `json:"error,omitempty"`
}

// OVSCommandFailure records a command execution failure.
type OVSCommandFailure struct {
	Command string `json:"command"`
	Error   string `json:"error"`
}

// OVSPortList holds the list of OVS ports.
type OVSPortList struct {
	Bridge string          `json:"bridge"`
	Ports  []OVSPortStatus `json:"ports"`
	Issues []string        `json:"issues"`
}

// OVSPortStatus describes a single OVS port.
type OVSPortStatus struct {
	Name     string   `json:"name"`
	OFPort   string   `json:"ofport"`
	Type     string   `json:"type"`
	VMName   string   `json:"vm_name"`
	MAC      string   `json:"mac"`
	IP       string   `json:"ip"`
	IPSource string   `json:"ip_source"`
	Issues   []string `json:"issues"`
}

// OVSLeaseStatus holds the OVS lease information and conflicts.
type OVSLeaseStatus struct {
	StaticHosts []OVSStaticHostInfo `json:"static_hosts"`
	DHCPLeases  []OVSDHCPLeaseInfo  `json:"dhcp_leases"`
	Conflicts   []OVSLeaseConflict  `json:"conflicts"`
}

// OVSStaticHostInfo is the JSON-friendly version of OVSStaticHost.
type OVSStaticHostInfo struct {
	VMName string `json:"vm_name"`
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
}

// OVSDHCPLeaseInfo is the JSON-friendly version of OVSDHCPLease.
type OVSDHCPLeaseInfo struct {
	ExpiryTime string `json:"expiry_time"`
	MAC        string `json:"mac"`
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	ClientID   string `json:"client_id"`
}

// OVSLeaseConflict describes a conflict between a static binding and a DHCP lease.
type OVSLeaseConflict struct {
	Type         string `json:"type"`
	IP           string `json:"ip"`
	MAC          string `json:"mac"`
	StaticVMName string `json:"static_vm_name"`
	LeaseHost    string `json:"lease_host"`
	Message      string `json:"message"`
}

// OVSCheckResult is the aggregated OVS network check result.
type OVSCheckResult struct {
	Status            *OVSStatus      `json:"status"`
	Ports             *OVSPortList    `json:"ports"`
	Leases            *OVSLeaseStatus `json:"leases"`
	Healthy           bool            `json:"healthy"`
	RepairSuggestions []string        `json:"repair_suggestions"`
}

// VMNetworkRuntimeStatus describes the network runtime status of a VM.
type VMNetworkRuntimeStatus struct {
	VMName     string                 `json:"vm_name"`
	State      string                 `json:"state"`
	Bridge     string                 `json:"bridge"`
	Interfaces []VMNetworkInterface   `json:"interfaces"`
	Bandwidth  OVSBandwidthReadStatus `json:"bandwidth"`
	Issues     []string               `json:"issues"`
}

// VMIPAddress 描述网卡上的单个 IP 地址及其来源。
type VMIPAddress struct {
	Address string `json:"address"`
	Source  string `json:"source"`
}

// VMNetworkInterface describes a VM's network interface runtime info.
type VMNetworkInterface struct {
	InterfaceType   string        `json:"interface_type"`
	Target          string        `json:"target"`
	SourceBridge    string        `json:"source_bridge"`
	SourceNetwork   string        `json:"source_network"`
	Model           string        `json:"model"`
	MAC             string        `json:"mac"`
	VirtualPortType string        `json:"virtualport_type"`
	OFPort          string        `json:"ofport"`
	IP              string        `json:"ip"`
	IPSource        string        `json:"ip_source"`
	IPAddresses     []VMIPAddress `json:"ip_addresses"`
	Issues          []string      `json:"issues"`
}

// OVSBandwidthReadStatus describes the OVS bandwidth QoS read status.
type OVSBandwidthReadStatus struct {
	Enabled         bool   `json:"enabled"`
	Cookie          string `json:"cookie"`
	FlowExists      bool   `json:"flow_exists"`
	DownQoS         bool   `json:"down_qos"`
	BridgeQoS       bool   `json:"bridge_qos"`
	TCRoot          bool   `json:"tc_root"`
	TCIngress       bool   `json:"tc_ingress"`
	TCUploadPolice  bool   `json:"tc_upload_police"`
	DownQueue       bool   `json:"down_queue"`
	UpQueue         bool   `json:"up_queue"`
	DownQueueID     uint32 `json:"down_queue_id"`
	UpQueueID       uint32 `json:"up_queue_id"`
	InboundAvgMbps  int    `json:"inbound_avg_mbps"`
	OutboundAvgMbps int    `json:"outbound_avg_mbps"`
	CheckedPort     string `json:"checked_port"`
	LastFlowError   string `json:"last_flow_error,omitempty"`
}

// ── Internal types ──

// OvsRuntimeInterface describes a VM runtime interface from virsh domiflist.
type OvsRuntimeInterface struct {
	Name   string
	Type   string
	Source string
	Model  string
	MAC    string
}

type ovsInterfaceXML struct {
	Type string `xml:"type,attr"`
	MAC  struct {
		Address string `xml:"address,attr"`
	} `xml:"mac"`
	Source struct {
		Bridge  string `xml:"bridge,attr"`
		Network string `xml:"network,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
	} `xml:"target"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
	VirtualPort struct {
		Type string `xml:"type,attr"`
	} `xml:"virtualport"`
}

type ovsDomainXML struct {
	Devices struct {
		Interfaces []ovsInterfaceXML `xml:"interface"`
	} `xml:"devices"`
}

// ensure xml.Unmarshal is available at compile time
var _ = xml.Unmarshal
