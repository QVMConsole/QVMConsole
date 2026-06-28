package clone

import (
	"encoding/json"
	"fmt"
	"regexp"

	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
)

const LinuxCloneIPWaitSeconds = 180

var fnOSDeviceIDRegexp = regexp.MustCompile(`^[0-9a-fA-F]{32}([0-9a-fA-F]{8})?$`)

// CloneParams 克隆参数
type CloneParams struct {
	Name                  string                         `json:"name"`                             // 虚拟机名称
	Remark                string                         `json:"remark,omitempty"`                 // 虚拟机备注
	Template              string                         `json:"template"`                         // 模板名称
	TemplateType          string                         `json:"template_type,omitempty"`          // 模板类型: linux/windows/fnos/other
	TemplateCategory      string                         `json:"template_category,omitempty"`      // 模板二级分类（如 WindowsServer2025/WindowsServer2022 等）
	CloneMode             string                         `json:"clone_mode,omitempty"`             // 克隆模式: linked（链式克隆，默认）/ full（完整克隆）
	VCPU                  int                            `json:"vcpu"`                             // CPU 核心数
	MaxVCPU               int                            `json:"max_vcpu,omitempty"`               // CPU 热添加上限，0 或 <= vcpu 表示不启用
	RAM                   int                            `json:"ram"`                              // 内存（GB）
	DiskSize              int                            `json:"disk_size,omitempty"`              // 磁盘大小（GB，可选）
	Network               string                         `json:"network,omitempty"`                // 网络（默认 default）
	Hostname              string                         `json:"hostname,omitempty"`               // 主机名
	User                  string                         `json:"user,omitempty"`                   // 新用户名
	Password              string                         `json:"password,omitempty"`               // 新密码
	Autostart             bool                           `json:"autostart,omitempty"`              // 开机自启
	Freeze                bool                           `json:"freeze,omitempty"`                 // 启动时冻结 CPU
	APIC                  *bool                          `json:"apic,omitempty"`                   // APIC 开关，默认启用
	PAE                   *bool                          `json:"pae,omitempty"`                    // PAE 开关，默认启用
	RTCOffset             string                         `json:"rtc_offset,omitempty"`             // RTC 使用本地时间还是 UTC
	RTCStartDate          string                         `json:"rtc_startdate,omitempty"`          // RTC 开始日期
	GuestAgent            *vm_xml.VMGuestAgentConfig     `json:"guest_agent,omitempty"`            // QEMU Guest Agent 配置
	SMBIOS1               *vm_xml.VMSMBIOS1Config        `json:"smbios1,omitempty"`                // SMBIOS 类型 1 设置
	UEFI                  *bool                          `json:"uefi,omitempty"`                   // 是否使用 UEFI 启动（nil=自动检测）
	TemplateRootPass      string                         `json:"template_root_pass,omitempty"`     // 模板 root 密码（用于 SSH 初始化）
	TemplateUser          string                         `json:"template_user,omitempty"`          // 模板中已有的用户名
	DiskBus               string                         `json:"disk_bus,omitempty"`               // 系统盘总线类型: virtio/scsi/sata/ide
	VideoModel            string                         `json:"video_model,omitempty"`            // 视频模型: virtio/vga/vmvga/cirrus
	SpiceEnabled          *bool                          `json:"spice_enabled,omitempty"`          // 是否启用 SPICE（nil=回退全局默认）
	CPUTopologyMode       string                         `json:"cpu_topology_mode,omitempty"`      // CPU 拓扑模式: auto/single_socket/host_default
	CPULimitPercent       int                            `json:"cpu_limit_percent,omitempty"`      // CPU 限制百分比，0 表示无限制
	CPUAffinity           string                         `json:"cpu_affinity,omitempty"`           // CPU 亲和性，如 "0,2,4"
	FirstBootRebootMode   string                         `json:"first_boot_reboot_mode,omitempty"` // 首次重启策略: normal/cold
	MemoryDynamic         *memory.VMMemoryDynamicRequest `json:"memory_dynamic,omitempty"`
	SwitchID              uint                           `json:"switch_id,omitempty"`
	SecurityGroupID       uint                           `json:"security_group_id,omitempty"`
	ExtraNics             []AddVMInterfaceRequest        `json:"extra_nics,omitempty"`
	StoragePoolID         string                         `json:"storage_pool_id,omitempty"`
	ExtraDisks            []ExtraDiskParam               `json:"extra_disks,omitempty"`
	NicModel              string                         `json:"nic_model,omitempty"` // 网卡模型: virtio/e1000e/rtl8139
	PreserveFnOSDeviceID  bool                           `json:"preserve_fnos_device_id,omitempty"`
	FnOSDeviceID          string                         `json:"fnos_device_id,omitempty"`
	SystemDiskIOPS        *DiskIOPSTune                  `json:"system_disk_iops,omitempty"` // 系统盘 IOPS 限制
	IsAdmin               bool                           `json:"is_admin,omitempty"`
	DisableSystemInit     bool                           `json:"disable_system_init,omitempty"` // 禁用系统初始化（跳过凭据校验和来宾系统修改）
	StaticIP              string                         `json:"static_ip,omitempty"`           // OpenWrt 静态 IP（CIDR 格式，如 192.168.1.100/24）
	Gateway               string                         `json:"gateway,omitempty"`             // OpenWrt 网关地址
	DNS                   string                         `json:"dns,omitempty"`                 // OpenWrt DNS 服务器
	LinuxIdentityPrepared bool                           `json:"-"`                             // Linux 首次启动前是否已离线重置 machine-id/DHCP 身份
	PCIERootPorts         int                            `json:"pcie_root_ports,omitempty"`     // q35 预留 pcie-root-port 数量
	PostBootCommand       string                         `json:"post_boot_command,omitempty"`   // Linux 模板启动后执行的自定义命令
	PostBootBlocking      bool                           `json:"post_boot_blocking,omitempty"`  // 启动后命令阻塞模式
}

// BatchCloneParams 批量克隆参数
type BatchCloneParams struct {
	Prefix              string                     `json:"prefix"`                  // 名称前缀
	StartNum            int                        `json:"start_num"`               // 起始编号
	Count               int                        `json:"count"`                   // 数量
	Template            string                     `json:"template"`                // 模板
	TemplateType        string                     `json:"template_type,omitempty"` // 模板类型
	CloneMode           string                     `json:"clone_mode,omitempty"`    // 克隆模式: linked / full
	VCPU                int                        `json:"vcpu"`
	MaxVCPU             int                        `json:"max_vcpu,omitempty"` // CPU 热添加上限
	RAM                 int                        `json:"ram"`
	DiskSize            int                        `json:"disk_size,omitempty"`
	Network             string                     `json:"network,omitempty"`
	Hostname            string                     `json:"hostname,omitempty"` // 主机名（空则由系统自动生成）
	User                string                     `json:"user,omitempty"`     // 新用户名
	Password            string                     `json:"password,omitempty"`
	Autostart           bool                       `json:"autostart,omitempty"`
	Freeze              bool                       `json:"freeze,omitempty"`
	APIC                *bool                      `json:"apic,omitempty"`
	PAE                 *bool                      `json:"pae,omitempty"`
	RTCOffset           string                     `json:"rtc_offset,omitempty"`
	RTCStartDate        string                     `json:"rtc_startdate,omitempty"`
	GuestAgent          *vm_xml.VMGuestAgentConfig `json:"guest_agent,omitempty"`
	SMBIOS1             *vm_xml.VMSMBIOS1Config    `json:"smbios1,omitempty"`
	UEFI                *bool                      `json:"uefi,omitempty"`
	TemplateRootPass    string                     `json:"template_root_pass,omitempty"`     // 模板 root 密码
	TemplateUser        string                     `json:"template_user,omitempty"`          // 模板中已有的用户名
	VideoModel          string                     `json:"video_model,omitempty"`            // 视频模型
	SpiceEnabled        *bool                      `json:"spice_enabled,omitempty"`          // 是否启用 SPICE（nil=回退全局默认）
	DiskBus             string                     `json:"disk_bus,omitempty"`               // 系统盘总线类型
	CPUTopologyMode     string                     `json:"cpu_topology_mode,omitempty"`      // CPU 拓扑模式
	CPULimitPercent     int                        `json:"cpu_limit_percent,omitempty"`      // CPU 限制百分比，0 表示无限制
	CPUAffinity         string                     `json:"cpu_affinity,omitempty"`           // CPU 亲和性，如 "0,2,4"
	FirstBootRebootMode string                     `json:"first_boot_reboot_mode,omitempty"` // 首次重启策略
	NicModel            string                     `json:"nic_model,omitempty"`              // 网卡模型
	StoragePoolID       string                     `json:"storage_pool_id,omitempty"`        // 存储池
	SwitchID            uint                       `json:"switch_id,omitempty"`              // VPC 交换机 ID
	SecurityGroupID     uint                       `json:"security_group_id,omitempty"`      // 安全组 ID
	ExtraNics           []AddVMInterfaceRequest    `json:"extra_nics,omitempty"`
	IsAdmin             bool                       `json:"is_admin,omitempty"`            // 是否管理员
	DisableSystemInit   bool                       `json:"disable_system_init,omitempty"` // 禁用系统初始化
	StaticIP            string                     `json:"static_ip,omitempty"`           // OpenWrt 静态 IP
	Gateway             string                     `json:"gateway,omitempty"`             // OpenWrt 网关
	DNS                 string                     `json:"dns,omitempty"`                 // OpenWrt DNS
}

// ReinstallParams 重装系统参数
type ReinstallParams struct {
	Name                  string `json:"name"`                              // 虚拟机名称
	Template              string `json:"template"`                          // 新模板名称
	TemplateType          string `json:"template_type,omitempty"`           // 模板类型
	DiskSize              int    `json:"disk_size,omitempty"`               // 系统盘大小（GB）
	Hostname              string `json:"hostname,omitempty"`                // 主机名
	User                  string `json:"user,omitempty"`                    // 登录用户
	Password              string `json:"password,omitempty"`                // 登录密码
	TemplateRootPass      string `json:"template_root_pass,omitempty"`      // 模板 root 密码
	TemplateUser          string `json:"template_user,omitempty"`           // 模板默认用户
	FirstBootRebootMode   string `json:"first_boot_reboot_mode,omitempty"`  // Windows 首次重启策略
	PreserveFnOSDeviceID  bool   `json:"preserve_fnos_device_id,omitempty"` // 是否保留 FnOS 设备 ID
	FnOSDeviceID          string `json:"fnos_device_id,omitempty"`          // 自定义 FnOS 设备 ID
	Operator              string `json:"operator,omitempty"`                // 操作人
	LinuxIdentityPrepared bool   `json:"-"`                                 // Linux 身份是否已离线重置
}

// CloneResult 克隆结果
type CloneResult struct {
	VMName   string `json:"vm_name"`
	IP       string `json:"ip"`
	DiskPath string `json:"disk_path"`
	Template string `json:"template"`
	Password string `json:"password,omitempty"` // 实际使用的密码（批量模式为空时自动生成）
	Error    string `json:"error,omitempty"`    // 失败原因（空表示成功）
}

// padNum 零填充数字
func padNum(n int) string {
	return fmt.Sprintf("%02d", n)
}

// CloneTaskHandler 克隆任务处理器（用于任务队列）
func CloneTaskHandler(task interface{}, progressFn func(int, string)) (string, error) {
	return "", nil
}

// RegisterCloneHandlers 注册克隆相关任务处理器
func RegisterCloneHandlers() {
}

// ParseCloneParams 从 JSON 解析克隆参数
func ParseCloneParams(jsonStr string) (*CloneParams, error) {
	var params CloneParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	return &params, nil
}

// ParseBatchCloneParams 从 JSON 解析批量克隆参数
func ParseBatchCloneParams(jsonStr string) (*BatchCloneParams, error) {
	var params BatchCloneParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	return &params, nil
}

// ParseReinstallParams 从 JSON 解析重装参数
func ParseReinstallParams(jsonStr string) (*ReinstallParams, error) {
	var params ReinstallParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	return &params, nil
}
