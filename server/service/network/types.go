package network

import "strings"

// StaticIPInfo 静态 IP 绑定信息
type StaticIPInfo struct {
	VMName string `json:"vm_name"`
	IP     string `json:"ip"`
	MAC    string `json:"mac"`
}

// IPListInfo 完整 IP 信息
type IPListInfo struct {
	StaticBindings []StaticIPInfo  `json:"static_bindings"`
	DHCPLeases     []DHCPLeaseInfo `json:"dhcp_leases"`
}

// DHCPLeaseInfo DHCP 租约信息
type DHCPLeaseInfo struct {
	ExpiryTime string `json:"expiry_time"`
	MAC        string `json:"mac"`
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	VMName     string `json:"vm_name"` // 通过 MAC 地址关联的虚拟机名称
}

// PortForwardRule 端口转发规则
type PortForwardRule struct {
	ID                    int    `json:"id"`                      // 规则编号
	Protocol              string `json:"protocol"`                // tcp/udp
	HostPort              string `json:"host_port"`               // 宿主机端口
	AccessIP              string `json:"access_ip"`               // 对外访问 IP
	AccessAddress         string `json:"access_address"`          // 对外完整访问地址
	DestIP                string `json:"dest_ip"`                 // 目标 IP
	DestPort              string `json:"dest_port"`               // 目标端口
	VMName                string `json:"vm_name"`                 // 关联虚拟机
	OwnerUsername         string `json:"owner_username"`          // 归属用户
	FirewallKey           string `json:"firewall_key"`            // 防火墙豁免使用的稳定标识
	RegionFilterEnabled   bool   `json:"region_filter_enabled"`   // 是否继承入站区域限制
	RegionFilterInherited bool   `json:"region_filter_inherited"` // 是否继承全局入站策略
	RuleKey               string `json:"rule_key"`                // 稳定规则标识
}

// StableKey 返回端口转发规则的稳定标识，避免依赖 iptables 行号。
func (r PortForwardRule) StableKey() string {
	return strings.ToLower(strings.TrimSpace(r.Protocol)) + "|" +
		strings.TrimSpace(r.HostPort) + "|" +
		strings.TrimSpace(r.DestIP) + "|" +
		strings.TrimSpace(r.DestPort)
}

// PortForwardAddParams 添加端口转发参数
type PortForwardAddParams struct {
	VMIP           string `json:"vm_ip"`
	HostPort       string `json:"host_port"`
	VMPort         string `json:"vm_port"`
	Protocol       string `json:"protocol"` // tcp/udp/both
	Comment        string `json:"comment"`
	CreatedBy      string `json:"created_by"`
	CreatedByAdmin bool   `json:"created_by_admin"`
}

// PortForwardAutoAddParams 自动分配端口参数
type PortForwardAutoAddParams struct {
	VMIP     string `json:"vm_ip" binding:"required"`
	VMPort   string `json:"vm_port" binding:"required"`
	Protocol string `json:"protocol"`
	Comment  string `json:"comment"`
}

// PortForwardUpdateParams 编辑端口转发参数
type PortForwardUpdateParams struct {
	VMIP           string `json:"vm_ip"`
	HostPort       string `json:"host_port"`
	VMPort         string `json:"vm_port"`
	Protocol       string `json:"protocol"`
	Comment        string `json:"comment"`
	CreatedBy      string `json:"created_by"`
	CreatedByAdmin bool   `json:"created_by_admin"`
}

type portForwardTargetInfo struct {
	VMName        string
	OwnerUsername string
}

var listPortForwardRulesForAvailability = listLivePortForwardsFromIPTables
var canListenOnHostPort = canBindHostPort
