package model

import "time"

// PublicIP 记录面板管理的公网 IP 资源。
type PublicIP struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	IP               string    `json:"ip" gorm:"uniqueIndex;not null;size:45"`
	CIDR             string    `json:"cidr" gorm:"column:cidr;size:64"`
	Gateway          string    `json:"gateway" gorm:"size:45"`
	UplinkIF         string    `json:"uplink_if" gorm:"size:64"`
	SupportedModes   string    `json:"supported_modes" gorm:"size:128;not null;default:nat,classic_route,classic_bridge"`
	Status           string    `json:"status" gorm:"index;size:32;not null;default:free"`
	Remark           string    `json:"remark" gorm:"size:255"`
	AutoIPv6         bool      `json:"auto_ipv6" gorm:"index;default:false"`
	IPv6SourcePrefix string    `json:"ipv6_source_prefix" gorm:"size:64"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (PublicIP) TableName() string {
	return "public_ips"
}

// PublicIPBinding 记录公网 IP 到 VM 的面板绑定关系。
type PublicIPBinding struct {
	ID               uint       `json:"id" gorm:"primaryKey"`
	PublicIPID       uint       `json:"public_ip_id" gorm:"uniqueIndex;not null"`
	PublicIP         string     `json:"public_ip" gorm:"index;not null;size:45"`
	Username         string     `json:"username" gorm:"index;not null;size:64"`
	VMName           string     `json:"vm_name" gorm:"index;not null;size:128"`
	VMPrivateIP      string     `json:"vm_private_ip" gorm:"size:45"`
	Mode             string     `json:"mode" gorm:"size:32;not null;default:nat"`
	RuntimeStatus    string     `json:"runtime_status" gorm:"size:32;not null;default:pending"`
	GuestIPv6Status  string     `json:"guest_ipv6_status" gorm:"size:32;not null;default:not_applicable"`
	GuestIPv6Message string     `json:"guest_ipv6_message" gorm:"size:255"`
	ConfigHint       string     `json:"config_hint" gorm:"type:text"`
	LastAppliedAt    *time.Time `json:"last_applied_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (PublicIPBinding) TableName() string {
	return "public_ip_bindings"
}
