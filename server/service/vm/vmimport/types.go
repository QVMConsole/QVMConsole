package vmimport

import (
	"encoding/json"
	"strings"

	"kvm_console/service"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
)

// ImportVMParams 导入虚拟机参数
type ImportVMParams struct {
	Name             string                  `json:"name"`                         // 虚拟机名称
	Remark           string                  `json:"remark,omitempty"`             // 虚拟机备注
	DiskFile         string                  `json:"disk_file"`                    // 磁盘文件名（在用户 disk 目录中）
	Username         string                  `json:"username"`                     // 所属用户
	CopyDisk         bool                    `json:"copy_disk,omitempty"`          // true=复制磁盘文件，false=移动磁盘文件
	VCPU             int                     `json:"vcpu"`                         // CPU 核心数
	MaxVCPU          int                     `json:"max_vcpu,omitempty"`           // CPU 热添加上限
	RAM              int                     `json:"ram"`                          // 内存（GB）
	Network          string                  `json:"network,omitempty"`            // 网络
	InitType         string                  `json:"init_type,omitempty"`          // 初始化类型: linux/windows/other/空（不初始化）
	Hostname         string                  `json:"hostname,omitempty"`           // 主机名
	User             string                  `json:"user,omitempty"`               // 用户名（Linux 初始化用）
	Password         string                  `json:"password,omitempty"`           // 密码
	Autostart        bool                    `json:"autostart,omitempty"`          // 开机自启
	Freeze           bool                    `json:"freeze,omitempty"`             // 启动时冻结 CPU
	APIC             *bool                   `json:"apic,omitempty"`               // APIC 开关，默认启用
	PAE              *bool                   `json:"pae,omitempty"`                // PAE 开关，默认启用
	RTCOffset        string                  `json:"rtc_offset,omitempty"`         // RTC 使用本地时间还是 UTC
	RTCStartDate     string                  `json:"rtc_startdate,omitempty"`      // RTC 开始日期
	GuestAgent       *vm_xml.VMGuestAgentConfig `json:"guest_agent,omitempty"`        // QEMU Guest Agent 配置
	SMBIOS1          *vm_xml.VMSMBIOS1Config    `json:"smbios1,omitempty"`            // SMBIOS 类型 1 设置
	BootType         string                  `json:"boot_type,omitempty"`          // 启动类型: bios/uefi
	MachineType      string                  `json:"machine_type,omitempty"`       // 机器类型: q35/pc
	NicModel         string                  `json:"nic_model,omitempty"`          // 网卡模型
	VideoModel       string                  `json:"video_model,omitempty"`        // 视频模型: virtio/vga/vmvga/cirrus
	SpiceEnabled     *bool                   `json:"spice_enabled,omitempty"`      // 是否启用 SPICE（nil=回退全局默认）
	CPUTopologyMode  string                  `json:"cpu_topology_mode,omitempty"`  // CPU 拓扑模式
	CPULimitPercent  int                     `json:"cpu_limit_percent,omitempty"`  // CPU 限制百分比，0 表示无限制
	CPUAffinity      string                  `json:"cpu_affinity,omitempty"`      // CPU 亲和性，如 "0,2,4"
	TemplateRootPass string                  `json:"template_root_pass,omitempty"` // 模板 root 密码（SSH 初始化用）
	TemplateUser     string                  `json:"template_user,omitempty"`      // 模板用户名
	MemoryDynamic    *vm_memory.VMMemoryDynamicRequest `json:"memory_dynamic,omitempty"`
	SwitchID          uint                    `json:"switch_id,omitempty"`
	SecurityGroupID   uint                    `json:"security_group_id,omitempty"`
	ExtraNics         []service.AddVMInterfaceRequest `json:"extra_nics,omitempty"`
	IsAdmin           bool                    `json:"is_admin,omitempty"`
	StartAfterImport  bool                    `json:"start_after_import"` // 导入完成后是否开启虚拟机，默认 true
}

// ImportVMResult 导入结果
type ImportVMResult struct {
	VMName   string `json:"vm_name"`
	DiskPath string `json:"disk_path"`
	IP       string `json:"ip,omitempty"`
}

// ImportDiskByPathParams 管理员通过绝对路径导入磁盘创建虚拟机参数
type ImportDiskByPathParams struct {
	Name             string                  `json:"name"`
	Remark           string                  `json:"remark,omitempty"`
	DiskPath         string                  `json:"disk_path"` // 磁盘绝对路径（主磁盘）
	DiskFile         string                  `json:"disk_file,omitempty"` // 存储文件名（主磁盘 storage 模式）
	DiskSourceType   string                  `json:"disk_source_type,omitempty"` // path/storage（主磁盘）
	StoragePoolID    string                  `json:"storage_pool_id,omitempty"`
	VCPU             int                     `json:"vcpu"`
	MaxVCPU          int                     `json:"max_vcpu,omitempty"`           // CPU 热添加上限
	RAM              int                     `json:"ram"`
	InitType         string                  `json:"init_type,omitempty"`
	Hostname         string                  `json:"hostname,omitempty"`
	User             string                  `json:"user,omitempty"`
	Password         string                  `json:"password,omitempty"`
	Autostart        bool                    `json:"autostart,omitempty"`
	Freeze           bool                    `json:"freeze,omitempty"`
	APIC             *bool                   `json:"apic,omitempty"`
	PAE              *bool                   `json:"pae,omitempty"`
	RTCOffset        string                  `json:"rtc_offset,omitempty"`
	RTCStartDate     string                  `json:"rtc_startdate,omitempty"`
	GuestAgent       *vm_xml.VMGuestAgentConfig `json:"guest_agent,omitempty"`
	SMBIOS1          *vm_xml.VMSMBIOS1Config    `json:"smbios1,omitempty"`
	BootType         string                  `json:"boot_type,omitempty"`
	MachineType      string                  `json:"machine_type,omitempty"`
	NicModel         string                  `json:"nic_model,omitempty"`
	VideoModel       string                  `json:"video_model,omitempty"`
	SpiceEnabled     *bool                   `json:"spice_enabled,omitempty"`      // 是否启用 SPICE（nil=回退全局默认）
	CPUTopologyMode  string                  `json:"cpu_topology_mode,omitempty"`
	CPULimitPercent  int                     `json:"cpu_limit_percent,omitempty"`
	CPUAffinity      string                  `json:"cpu_affinity,omitempty"`       // CPU 亲和性，如 "0,2,4"
	TemplateRootPass string                  `json:"template_root_pass,omitempty"`
	TemplateUser     string                  `json:"template_user,omitempty"`
	MemoryDynamic    *vm_memory.VMMemoryDynamicRequest `json:"memory_dynamic,omitempty"`
	SwitchID         uint                    `json:"switch_id,omitempty"`
	SecurityGroupID  uint                    `json:"security_group_id,omitempty"`
	ExtraNics        []service.AddVMInterfaceRequest `json:"extra_nics,omitempty"`
	CopyDisk         bool                    `json:"copy_disk,omitempty"`
	ExtraImportDisks []ExtraImportDiskEntry  `json:"extra_import_disks,omitempty"` // 额外导入磁盘列表
	Username         string                  `json:"username,omitempty"` // 所属用户（存储模式需要）
	SystemDiskIOPS   *service.DiskIOPSTune           `json:"system_disk_iops,omitempty"` // 系统盘 IOPS 限制
	StartAfterImport bool                    `json:"start_after_import"` // 导入完成后是否开启虚拟机，默认 true
}

// ExtraImportDiskEntry 额外导入磁盘条目
type ExtraImportDiskEntry struct {
	DiskPath       string `json:"disk_path,omitempty"`
	DiskFile       string `json:"disk_file,omitempty"`
	DiskSourceType string `json:"disk_source_type,omitempty"` // path/storage
	StoragePoolID  string `json:"storage_pool_id,omitempty"`
	CopyDisk       bool   `json:"copy_disk,omitempty"`
	Bus            string `json:"bus,omitempty"` // virtio/scsi/sata/ide
	IOPSTotal      int    `json:"iops_total,omitempty"`  // 总 IOPS 限制
	IOPSRead       int    `json:"iops_read,omitempty"`   // 读 IOPS 限制
	IOPSWrite      int    `json:"iops_write,omitempty"`  // 写 IOPS 限制
}

// ImportDiskForExistingVMParams 为已有虚拟机导入磁盘参数
type ImportDiskForExistingVMParams struct {
	VMName         string `json:"vm_name"`
	DiskPath       string `json:"disk_path,omitempty"`
	DiskFile       string `json:"disk_file,omitempty"`
	DiskSourceType string `json:"disk_source_type,omitempty"`
	StoragePoolID  string `json:"storage_pool_id,omitempty"`
	CopyDisk       bool   `json:"copy_disk,omitempty"`
	Bus            string `json:"bus,omitempty"`
	Username       string `json:"username,omitempty"`
}

// ---------- parse helpers ----------

// ParseImportVMParams 从 JSON 解析导入参数
func ParseImportVMParams(jsonStr string) (*ImportVMParams, error) {
	var params ImportVMParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	// 向后兼容：旧任务 JSON 中无 start_after_import 字段时，默认开启虚拟机
	if !strings.Contains(jsonStr, `"start_after_import"`) {
		params.StartAfterImport = true
	}
	return &params, nil
}

// ParseImportDiskByPathParams 从 JSON 解析导入磁盘参数
func ParseImportDiskByPathParams(jsonStr string) (*ImportDiskByPathParams, error) {
	var params ImportDiskByPathParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	// 向后兼容：旧任务 JSON 中无 start_after_import 字段时，默认开启虚拟机
	if !strings.Contains(jsonStr, `"start_after_import"`) {
		params.StartAfterImport = true
	}
	return &params, nil
}

// ParseImportDiskForExistingVMParams 解析参数
func ParseImportDiskForExistingVMParams(jsonStr string) (*ImportDiskForExistingVMParams, error) {
	var params ImportDiskForExistingVMParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	return &params, nil
}
