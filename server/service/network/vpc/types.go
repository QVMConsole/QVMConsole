package vpc

import "kvm_console/model"

const (
	VPCConfigDir                         = "/etc/kvm-console/vpc"
	VPCSwitchTrafficPenaltyMbps          = 1
	DefaultVPCSwitchName                 = "默认交换机"
	SystemBaseNetworkName                = "基础网络"
	AutoPortForwardSecurityGroupRuleNote = "端口转发自动放行"
	UplinkModeNone                       = "none"
	UplinkModePhysical                   = "physical"
	UplinkModeSystem                     = "system"
)

type VPCSwitchRequest struct {
	Username            string  `json:"username"`
	Name                string  `json:"name"`
	BridgeName          string  `json:"bridge_name"`
	DHCPEnabled         bool    `json:"dhcp_enabled"`
	UplinkMode          string  `json:"uplink_mode"`
	UplinkIF            string  `json:"uplink_if"`
	UplinkGateway       string  `json:"uplink_gateway"` // 自动检测不到默认路由时使用的物理出口网关
	MigrateHostIP       bool    `json:"migrate_host_ip"`
	BridgeVLANID        int     `json:"bridge_vlan_id"`
	AllowPromiscuous    bool    `json:"allow_promiscuous"`
	AllowMACChange      bool    `json:"allow_mac_change"`
	AllowForgedTx       bool    `json:"allow_forged_transmits"`
	IPv6SecurityEnabled bool    `json:"ipv6_security_enabled"`
	TrustedIPv6Prefixes string  `json:"trusted_ipv6_prefixes"`
	CIDR                string  `json:"cidr"`       // 自定义网段（如 10.0.1.0/24），留空则自动分配
	GatewayIP           string  `json:"gateway_ip"` // 自定义网关地址，留空则自动计算（CIDR 内第一个可用 IP）
	DHCPStart           string  `json:"dhcp_start"` // DHCP 起始地址，留空则自动计算
	DHCPEnd             string  `json:"dhcp_end"`   // DHCP 结束地址，留空则自动计算
	TrafficDownGB       float64 `json:"traffic_down_gb"`
	TrafficUpGB         float64 `json:"traffic_up_gb"`
	BandwidthMbps       int     `json:"bandwidth_mbps"` // 兼容旧版字段，传入时同时作为上下行默认值
	BandwidthDownMbps   int     `json:"bandwidth_down_mbps"`
	BandwidthUpMbps     int     `json:"bandwidth_up_mbps"`
}

type VPCSecurityGroupRequest struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Remark   string `json:"remark"`
}

type VPCSecurityGroupRuleRequest struct {
	Direction     string `json:"direction"`
	AddressFamily string `json:"address_family"`
	Protocol      string `json:"protocol"`
	PortStart     int    `json:"port_start"`
	PortEnd       int    `json:"port_end"`
	TargetType    string `json:"target_type"`
	TargetValue   string `json:"target_value"`
	Remark        string `json:"remark"`
}

type VPCQuotaInfo struct {
	Username               string  `json:"username"`
	MaxTrafficDown         float64 `json:"max_traffic_down"`
	MaxTrafficUp           float64 `json:"max_traffic_up"`
	AllocatedTrafficDown   float64 `json:"allocated_traffic_down"`
	AllocatedTrafficUp     float64 `json:"allocated_traffic_up"`
	RemainingTrafficDown   float64 `json:"remaining_traffic_down"`
	RemainingTrafficUp     float64 `json:"remaining_traffic_up"`
	MaxBandwidthDown       float64 `json:"max_bandwidth_down"`
	MaxBandwidthUp         float64 `json:"max_bandwidth_up"`
	AllocatedBandwidthDown float64 `json:"allocated_bandwidth_down"`
	AllocatedBandwidthUp   float64 `json:"allocated_bandwidth_up"`
	RemainingBandwidthDown float64 `json:"remaining_bandwidth_down"`
	RemainingBandwidthUp   float64 `json:"remaining_bandwidth_up"`
}

type VPCBindingInfo struct {
	OwnerUsername    string                    `json:"owner_username"`
	Binding          *model.VPCVMBinding       `json:"binding"`
	Bindings         []model.VPCVMBinding      `json:"bindings,omitempty"`
	Switch           *model.VPCSwitch          `json:"switch"`
	SecurityGroup    *model.VPCSecurityGroup   `json:"security_group"`
	Groups           []model.VPCSecurityGroup  `json:"groups"`
	Switches         []model.VPCSwitch         `json:"switches"`
	LightweightQuota *model.LightweightVMQuota `json:"lightweight_quota,omitempty"`
}

// AddVMInterfaceRequest 添加虚拟机网口的请求参数
type AddVMInterfaceRequest struct {
	SwitchID             uint   `json:"switch_id"`
	SecurityGroupID      uint   `json:"security_group_id"`
	NicModel             string `json:"nic_model"`
	BandwidthInboundAvg  int    `json:"bandwidth_inbound_avg"`
	BandwidthOutboundAvg int    `json:"bandwidth_outbound_avg"`
	AllowedIPv4Addresses string `json:"allowed_ipv4_addresses"`
	AllowedIPv6Addresses string `json:"allowed_ipv6_addresses"`
}

// VMInterfaceInfo 网口信息
type VMInterfaceInfo struct {
	Binding       model.VPCVMBinding      `json:"binding"`
	Switch        *model.VPCSwitch        `json:"switch"`
	SecurityGroup *model.VPCSecurityGroup `json:"security_group"`
	MAC           string                  `json:"mac"`
}

// VMSwitchInfo 交换机下虚拟机简要信息
type VMSwitchInfo struct {
	VMName         string `json:"vm_name"`
	Username       string `json:"username"`
	InterfaceOrder int    `json:"interface_order"`
}
