package model

import "time"

// VPCSwitch 用户逻辑交换机配置。
type VPCSwitch struct {
	ID                         uint      `json:"id" gorm:"primaryKey"`
	Username                   string    `json:"username" gorm:"index;not null;size:64"`
	IsSystem                   bool      `json:"is_system" gorm:"default:false"`
	Name                       string    `json:"name" gorm:"not null;size:64"`
	BridgeName                 string    `json:"bridge_name" gorm:"not null;default:br-ovs;size:64"`
	BridgeMode                 string    `json:"bridge_mode" gorm:"not null;default:nat;size:16"`
	BridgeVLANID               int       `json:"bridge_vlan_id" gorm:"default:0"`
	AllowPromiscuous           bool      `json:"allow_promiscuous" gorm:"default:false"`
	AllowMACChange             bool      `json:"allow_mac_change" gorm:"default:false"`
	AllowForgedTransmits       bool      `json:"allow_forged_transmits" gorm:"default:false"`
	VLANID                     int       `json:"vlan_id" gorm:"uniqueIndex;not null"`
	CIDR                       string    `json:"cidr" gorm:"uniqueIndex;not null;size:32"`
	GatewayIP                  string    `json:"gateway_ip" gorm:"not null;size:45"`
	DHCPStart                  string    `json:"dhcp_start" gorm:"not null;size:45"`
	DHCPEnd                    string    `json:"dhcp_end" gorm:"not null;size:45"`
	TrafficDownGB              float64   `json:"traffic_down_gb" gorm:"default:0"`
	TrafficUpGB                float64   `json:"traffic_up_gb" gorm:"default:0"`
	BandwidthMbps              int       `json:"bandwidth_mbps" gorm:"default:0"` // 兼容旧版单总带宽字段
	BandwidthDownMbps          int       `json:"bandwidth_down_mbps" gorm:"default:0"`
	BandwidthUpMbps            int       `json:"bandwidth_up_mbps" gorm:"default:0"`
	UsedTrafficDown            int64     `json:"used_traffic_down" gorm:"-"`
	UsedTrafficUp              int64     `json:"used_traffic_up" gorm:"-"`
	UsedTrafficDownGB          string    `json:"used_traffic_down_gb" gorm:"-"`
	UsedTrafficUpGB            string    `json:"used_traffic_up_gb" gorm:"-"`
	IsLimitedDown              bool      `json:"is_limited_down" gorm:"-"`
	IsLimitedUp                bool      `json:"is_limited_up" gorm:"-"`
	EffectiveBandwidthDownMbps int       `json:"effective_bandwidth_down_mbps" gorm:"-"`
	EffectiveBandwidthUpMbps   int       `json:"effective_bandwidth_up_mbps" gorm:"-"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

func (VPCSwitch) TableName() string {
	return "vpc_switches"
}

// VPCSecurityGroup 用户安全组。
type VPCSecurityGroup struct {
	ID         uint                   `json:"id" gorm:"primaryKey"`
	Username   string                 `json:"username" gorm:"index;not null;size:64"`
	VMName     string                 `json:"vm_name" gorm:"index;size:128"`
	Name       string                 `json:"name" gorm:"not null;size:64"`
	IsDefault  bool                   `json:"is_default" gorm:"default:false"`
	IsVMScoped bool                   `json:"is_vm_scoped" gorm:"default:false"`
	Remark     string                 `json:"remark" gorm:"size:255"`
	Rules      []VPCSecurityGroupRule `json:"rules,omitempty" gorm:"foreignKey:SecurityGroupID"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

func (VPCSecurityGroup) TableName() string {
	return "vpc_security_groups"
}

// VPCSecurityGroupRule 安全组规则。规则语义为允许，默认拒绝入站、允许出站。
type VPCSecurityGroupRule struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	SecurityGroupID uint      `json:"security_group_id" gorm:"index;not null"`
	Direction       string    `json:"direction" gorm:"size:16;not null"` // ingress/egress
	Protocol        string    `json:"protocol" gorm:"size:16;not null"`  // tcp/udp/icmp/all
	PortStart       int       `json:"port_start" gorm:"default:0"`
	PortEnd         int       `json:"port_end" gorm:"default:0"`
	TargetType      string    `json:"target_type" gorm:"size:32;not null"` // cidr/switch/security_group
	TargetValue     string    `json:"target_value" gorm:"size:128;not null"`
	Remark          string    `json:"remark" gorm:"size:255"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (VPCSecurityGroupRule) TableName() string {
	return "vpc_security_group_rules"
}

// VPCVMBinding VM 与交换机、安全组的绑定。每个 VM 可以有多个绑定（多网口），通过 interface_order 区分。
type VPCVMBinding struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	VMName          string    `json:"vm_name" gorm:"uniqueIndex:idx_vm_interface;not null;size:128"`
	Username        string    `json:"username" gorm:"index;not null;size:64"`
	SwitchID        uint      `json:"switch_id" gorm:"index;not null"`
	SecurityGroupID uint      `json:"security_group_id" gorm:"index;not null"`
	InterfaceOrder  int       `json:"interface_order" gorm:"uniqueIndex:idx_vm_interface;not null;default:0"`
	NicModel        string    `json:"nic_model" gorm:"size:32;default:virtio"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (VPCVMBinding) TableName() string {
	return "vpc_vm_bindings"
}

// VPCSwitchTrafficMonthly 交换机月流量记录。
type VPCSwitchTrafficMonthly struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	SwitchID      uint      `json:"switch_id" gorm:"uniqueIndex:idx_vpc_switch_month;not null"`
	Username      string    `json:"username" gorm:"index;not null;size:64"`
	Month         string    `json:"month" gorm:"uniqueIndex:idx_vpc_switch_month;not null;size:7"` // YYYY-MM
	TrafficDown   int64     `json:"traffic_down"`
	TrafficUp     int64     `json:"traffic_up"`
	OffsetDown    int64     `json:"offset_down"`
	OffsetUp      int64     `json:"offset_up"`
	IsLimitedDown bool      `json:"is_limited_down"`
	IsLimitedUp   bool      `json:"is_limited_up"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (VPCSwitchTrafficMonthly) TableName() string {
	return "vpc_switch_traffic_monthlies"
}
