package model

import "time"

// NetworkBridge 记录面板管理的宿主机网桥。
type NetworkBridge struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name" gorm:"uniqueIndex;not null;size:64"`
	Mode          string    `json:"mode" gorm:"not null;size:16"` // nat/bridge
	UplinkIF      string    `json:"uplink_if" gorm:"size:64"`
	MigrateHostIP bool      `json:"migrate_host_ip" gorm:"default:false"`
	HostAddrs     string    `json:"host_addrs" gorm:"size:512"`  // 换行分隔的 IPv4 CIDR 地址列表，如 "192.168.11.54/24"
	HostGateway   string    `json:"host_gateway" gorm:"size:64"` // IPv4 默认网关 IP
	HostMetric    string    `json:"host_metric" gorm:"size:16"`  // IPv4 路由 metric
	HostDNS       string    `json:"host_dns" gorm:"size:512"`    // 空格分隔的 DNS 服务器 IP，如 "192.168.10.1 223.5.5.5"
	HostAddrs6    string    `json:"host_addrs6" gorm:"size:512"`  // 换行分隔的 IPv6 CIDR 地址列表，如 "2001:db8::1/64"
	HostGateway6  string    `json:"host_gateway6" gorm:"size:64"` // IPv6 默认网关 IP
	HostMetric6   string    `json:"host_metric6" gorm:"size:16"`  // IPv6 路由 metric
	IsDefault     bool      `json:"is_default" gorm:"default:false"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (NetworkBridge) TableName() string {
	return "network_bridges"
}
