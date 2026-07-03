package vm

import (
	"encoding/xml"

	"kvm_console/model"
	bw "kvm_console/service/bandwidth"
	"kvm_console/service/guest_agent"
	"kvm_console/service/vm_xml"
)

// ==================== 常量 ====================

const (
	vmConfigMetadataURI = "https://kvm-console.local/domain-config"
	vmConfigMetadataKey = "kvm-console"
)

// ==================== 虚拟机信息结构体 ====================

// VmInfo 虚拟机基本信息
type VmInfo struct {
	Name                     string               `json:"name"`
	Remark                   string               `json:"remark"`
	Group                    string               `json:"group"`
	Status                   string               `json:"status"`             // running, shut off, paused, etc.
	VCPU                     int                  `json:"vcpu"`               // CPU 核心数
	Memory                   int                  `json:"memory"`             // 内存（MB）
	MaxMemory                int                  `json:"max_memory"`         // 最大内存（MB）
	IP                       string               `json:"ip"`                 // IP 地址
	IPStatus                 string               `json:"ip_status"`          // IP 状态: ""=正常, "vlan_bridge"=VLAN桥接无法获取
	DiskSize                 string               `json:"disk_size"`          // 磁盘占用
	Template                 string               `json:"template"`           // 模板来源
	Network                  string               `json:"network"`            // 网络模式
	NicModel                 string               `json:"nic_model"`          // 网卡模型: virtio/e1000e/rtl8139
	Autostart                bool                 `json:"autostart"`          // 开机自启
	MacAddress               string               `json:"mac_address"`        // MAC 地址
	VNCPort                  string               `json:"vnc_port"`           // VNC 端口
	VideoModel               string               `json:"video_model"`        // 视频模型: virtio/vga/vmvga/cirrus/ramfb
	CPUTopologyMode          string               `json:"cpu_topology_mode"`  // CPU 拓扑模式
	CPULimitPercent          int                  `json:"cpu_limit_percent"`  // CPU 限制百分比，0 表示无限制
	CPUAffinity              string               `json:"cpu_affinity"`       // CPU 亲和性，如 "0,2,4"，空字符串表示未设置
	CPUPercent               float64              `json:"cpu_percent"`        // CPU 使用率（来自缓存）
	MemPercent               float64              `json:"mem_percent"`        // 内存使用率（来自缓存）
	MemoryInitial            int                  `json:"memory_initial"`     // 动态内存启动/保障内存（MB）
	MemoryMin                int                  `json:"memory_min"`         // 动态内存最小内存（MB）
	MemoryMaxDynamic         int                  `json:"memory_max_dynamic"` // 动态内存最大内存（MB）
	MemoryBackend            string               `json:"memory_backend"`     // 动态内存后端: balloon/virtio_mem
	MemoryVirtioMemCurrent   int                  `json:"memory_virtio_mem_current"`
	MemoryDynamicEnabled     bool                 `json:"memory_dynamic_enabled"`
	MemoryAutoBalloon        bool                 `json:"memory_auto_balloon"`
	MemoryPendingApply       bool                 `json:"memory_pending_apply"`
	MemoryCompatMode         string               `json:"memory_compat_mode"`
	MemoryBalloonSupported   bool                 `json:"memory_balloon_supported"`
	MemoryBalloonStatus      string               `json:"memory_balloon_status"`
	CreatedAt                string               `json:"created_at"`      // 创建时间
	BandwidthIn              int                  `json:"bandwidth_in"`    // 下行平峰速率 Mbps
	BandwidthOut             int                  `json:"bandwidth_out"`   // 上行平峰速率 Mbps
	PublicIPs                []PublicIPAttachment `json:"public_ips"`      // 已绑定公网 IP
	InRescue                 bool                 `json:"in_rescue"`       // 是否处于救援模式
	Locked                   bool                 `json:"locked"`          // 是否已锁定
	IsLinkedClone            bool                 `json:"is_linked_clone"` // 是否为链式克隆（有backing file）
	ContinuousRuntimeSeconds int64                `json:"continuous_runtime_seconds"`
	ContinuousRunningSince   string               `json:"continuous_running_since"`
}

// BootDevice 可引导设备信息（类似 Cockpit 的引导顺序展示）
type BootDevice struct {
	Type    string `json:"type"`    // 设备类型: disk / cdrom / network
	Device  string `json:"device"`  // 设备标识: vda, sda, sdb 等
	File    string `json:"file"`    // 文件路径或 MAC 地址
	Bus     string `json:"bus"`     // 总线类型: virtio, sata, ide, scsi
	Enabled bool   `json:"enabled"` // 是否在引导列表中启用
	Order   int    `json:"order"`   // 引导顺序（0 表示未设置）
}

// VmDetail 虚拟机详细信息
type VmDetail struct {
	VmInfo
	DiskPath               string                        `json:"disk_path"`    // 磁盘路径
	DiskHealthy            *bool                         `json:"disk_healthy"` // 磁盘完整性标记: nil=未检查, true=正常, false=磁盘文件缺失
	UUID                   string                        `json:"uuid"`         // 虚拟机 UUID
	VNCPort                string                        `json:"vnc_port"`     // VNC 端口
	Snapshots              []string                      `json:"snapshots"`    // 快照列表
	OSType                 string                        `json:"os_type"`      // 系统类型
	BootType               string                        `json:"boot_type"`    // 引导方式: bios/uefi/uefi-secure
	BootOrder              []string                      `json:"boot_order"`   // 引导顺序（OS 级别: hd, cdrom, network）
	BootDevices            []BootDevice                  `json:"boot_devices"` // 所有可引导设备列表
	Arch                   string                        `json:"arch"`         // 来宾架构
	MachineType            string                        `json:"machine_type"` // 机器类型: q35/i440fx/virt
	Bandwidth              *bw.BandwidthDetail           `json:"bandwidth"`    // 带宽详情
	LightweightQuota       *model.LightweightVMQuota     `json:"lightweight_quota"`
	Stats                  *VmStats                      `json:"stats"`              // 实时资源使用（缓存数据，SSE 推送用）
	Credential             *VMCredentialInfo             `json:"credential"`         // 保存的登录凭据
	Freeze                 bool                          `json:"freeze"`             // 启动时冻结 CPU
	APIC                   bool                          `json:"apic"`               // APIC 开关
	PAE                    bool                          `json:"pae"`                // PAE 开关
	RTCOffset              string                        `json:"rtc_offset"`         // RTC 偏移值: utc/localtime
	RTCStartDate           string                        `json:"rtc_startdate"`      // RTC 开始日期
	GuestAgent             *vm_xml.VMGuestAgentConfig    `json:"guest_agent"`        // QEMU Guest Agent 配置
	GuestAgentStatus       *guest_agent.GuestAgentStatus `json:"guest_agent_status"` // QEMU Guest Agent 运行时状态
	SMBIOS1                *vm_xml.VMSMBIOS1Config       `json:"smbios1"`            // SMBIOS 类型 1 信息
	MemoryObservationUntil int64                         `json:"memory_observation_until"`
	MemoryManualPauseUntil int64                         `json:"memory_manual_pause_until"`
	PCIERootPorts          int                           `json:"pcie_root_ports"`       // pcie-root-port 数量（仅 q35/virt 机型）
	FirmwareCompat         bool                          `json:"firmware_compat"`       // UEFI 固件兼容模式（ARM 专用）
	DirectBoot             *vm_xml.DirectBootConfig      `json:"direct_boot,omitempty"` // 直接内核引导配置
}

// VmStats 虚拟机资源使用统计
type VmStats struct {
	CPUPercent  float64 `json:"cpu_percent"` // CPU 使用率
	MemUsed     int64   `json:"mem_used"`    // 已用内存（KB）
	MemTotal    int64   `json:"mem_total"`   // 总内存（KB）
	NetRxBytes  int64   `json:"net_rx_bytes"`
	NetTxBytes  int64   `json:"net_tx_bytes"`
	DiskRdBytes int64   `json:"disk_rd_bytes"`
	DiskWrBytes int64   `json:"disk_wr_bytes"`
}

// VMListOptions 虚拟机列表查询选项
type VMListOptions struct {
	IncludeResourceUsage bool
	IncludeIP            bool
	IncludeNetworkInfo   bool
	IncludeBandwidth     bool
}

// HostStats 宿主机资源信息
type HostStats struct {
	CPUCount        int     `json:"cpu_count"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemTotal        int64   `json:"mem_total"`     // KB
	MemFree         int64   `json:"mem_free"`      // KB（不含 buffer/cache，仅供参考）
	MemAvailable    int64   `json:"mem_available"` // KB（含可回收缓存，反映实际可用内存）
	MemUsed         int64   `json:"mem_used"`      // KB（基于 MemAvailable 计算的实际占用）
	SwapTotal       int64   `json:"swap_total"`    // KB
	SwapFree        int64   `json:"swap_free"`     // KB
	SwapUsed        int64   `json:"swap_used"`     // KB
	DiskTotal       int64   `json:"disk_total"`    // KB
	DiskUsed        int64   `json:"disk_used"`     // KB
	DiskFree        int64   `json:"disk_free"`     // KB
	NetRxBytes      int64   `json:"net_rx_bytes"`
	NetTxBytes      int64   `json:"net_tx_bytes"`
	DiskRdBytes     int64   `json:"disk_rd_bytes"`
	DiskWrBytes     int64   `json:"disk_wr_bytes"`
	Hostname        string  `json:"hostname"`
	Uptime          string  `json:"uptime"`
	Arch            string  `json:"arch"` // 宿主机架构
	VMRunning       int     `json:"vm_running"`
	VMTotal         int     `json:"vm_total"`
	KSMPagesShared  int64   `json:"ksm_pages_shared"`
	KSMPagesSharing int64   `json:"ksm_pages_sharing"`
	DiskIOLatencyMs float64 `json:"disk_io_latency_ms"` // ms (avg)
}

// ==================== XML 解析用结构体 ====================

type domainXML struct {
	XMLName xml.Name      `xml:"domain"`
	Name    string        `xml:"name"`
	Memory  domainMemory  `xml:"memory"`
	VCPU    int           `xml:"vcpu"`
	OS      domainOS      `xml:"os"`
	Devices domainDevices `xml:"devices"`
}

type domainMemory struct {
	Unit  string `xml:"unit,attr"`
	Value int    `xml:",chardata"`
}

type domainOS struct {
	Type struct {
		Arch string `xml:"arch,attr"`
	} `xml:"type"`
}

type domainDevices struct {
	Disks      []domainDisk      `xml:"disk"`
	Interfaces []domainInterface `xml:"interface"`
	Graphics   []domainGraphics  `xml:"graphics"`
}

type domainDisk struct {
	Type   string `xml:"type,attr"`
	Device string `xml:"device,attr"`
	Source struct {
		File string `xml:"file,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
		Bus string `xml:"bus,attr"`
	} `xml:"target"`
	Driver struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	} `xml:"driver"`
}

type domainInterface struct {
	Type   string `xml:"type,attr"`
	Source struct {
		Network string `xml:"network,attr"`
		Bridge  string `xml:"bridge,attr"`
	} `xml:"source"`
	Mac struct {
		Address string `xml:"address,attr"`
	} `xml:"mac"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
}

type domainGraphics struct {
	Type   string `xml:"type,attr"`
	Port   string `xml:"port,attr"`
	Listen string `xml:"listen,attr"`
}

// DiskInfoResult holds VM disk info for cross-package use
type DiskInfoResult struct {
	Device         string
	Path           string
	Size           string
	Template       string
	HasBackingFile bool
}

// NetInfoResult holds VM network info for cross-package use
type NetInfoResult struct {
	Network  string
	MAC      string
	NicModel string
}

// VMCredentialInfo is defined in credential.go
// VMRuntimeInfo is defined in runtime.go
