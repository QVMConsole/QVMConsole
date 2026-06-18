package bridge

const (
	BridgeModeNAT    = "nat"
	BridgeModeDirect = "bridge"
	bridgeConfigDir  = "/etc/kvm-console/bridges"
)

type HostInterfaceInfo struct {
	Name          string   `json:"name"`
	MAC           string   `json:"mac"`
	State         string   `json:"state"`
	MTU           int      `json:"mtu"`
	Addresses     []string `json:"addresses"`
	DefaultRoute  bool     `json:"default_route"`
	OVSBridge     string   `json:"ovs_bridge"`
	OVSPort       bool     `json:"ovs_port"`
	Physical      bool     `json:"physical"`
	ManagedBridge string   `json:"managed_bridge"`
	Risk          string   `json:"risk,omitempty"`
}

type NetworkBridgeInfo struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	UplinkIF      string `json:"uplink_if"`
	MigrateHostIP bool   `json:"migrate_host_ip"`
	IsDefault     bool   `json:"is_default"`
	Exists        bool   `json:"exists"`
	Active        bool   `json:"active"`
	SwitchCount   int64  `json:"switch_count"`
	HostAddrs     string `json:"host_addrs"`
	HostGateway   string `json:"host_gateway"`
	HostDNS       string `json:"host_dns"`
}

// InterfaceConfigInfo 接口当前 IP/DNS 配置信息。
type InterfaceConfigInfo struct {
	Name          string   `json:"name"`            // 接口名称
	Type          string   `json:"type"`            // bridge / nic
	BridgeName    string   `json:"bridge_name"`     // 若物理网卡已加入网桥，对应的网桥名称
	Addrs         []string `json:"addrs"`           // 当前 IPv4 地址（CIDR 格式）
	Gateway       string   `json:"gateway"`         // 默认网关
	Metric        string   `json:"metric"`          // 路由 metric
	DNS           []string `json:"dns"`             // DNS 服务器列表
	Configurable  bool     `json:"configurable"`    // 是否可配置
	Reason        string   `json:"reason"`          // 不可配置原因
	ManagedBridge bool     `json:"managed_bridge"`  // 是否为面板管理的网桥
	MigrateHostIP bool     `json:"migrate_host_ip"` // 网桥是否迁移了宿主机 IP
}

// SetInterfaceConfigRequest 设置接口 IP/DNS 配置请求。
type SetInterfaceConfigRequest struct {
	Name    string `json:"name"`    // 接口名称（网桥或物理网卡）
	Addrs   string `json:"addrs"`   // 新 IP 地址（换行或逗号分隔的 CIDR）
	Gateway string `json:"gateway"` // 新默认网关
	DNS     string `json:"dns"`     // 新 DNS 服务器（空格或逗号分隔）
	Clear   bool   `json:"clear"`   // 为 true 时清除所有静态配置
}

type NetworkBridgeRequest struct {
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	UplinkIF      string `json:"uplink_if"`
	MigrateHostIP bool   `json:"migrate_host_ip"`
}

type ipAddrJSON struct {
	IfName    string `json:"ifname"`
	Address   string `json:"address"`
	OperState string `json:"operstate"`
	MTU       int    `json:"mtu"`
	AddrInfo  []struct {
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
		Family    string `json:"family"`
	} `json:"addr_info"`
}

type ipRouteJSON struct {
	Dst     string `json:"dst"`
	Dev     string `json:"dev"`
	Gateway string `json:"gateway"`
}
