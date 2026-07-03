package handler

import (
	"kvm_console/service"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
)

// VmOperateRequest 虚拟机操作请求
type VmOperateRequest struct {
	Action string `json:"action" binding:"required"` // start, shutdown, destroy, reboot, reset
}

// ResetLinuxPasswordRequest 重置虚拟机密码请求
type ResetLinuxPasswordRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// VmEditRequest 虚拟机编辑请求
type VmEditRequest struct {
	VCPU            int                               `json:"vcpu"`
	MaxVCPU         int                               `json:"max_vcpu,omitempty"` // CPU 热添加上限，0 或 <= vcpu 表示不启用
	Memory          int                               `json:"memory"`             // GB
	Remark          *string                           `json:"remark"`             // 备注
	Group           *string                           `json:"group"`              // 分组
	Autostart       *bool                             `json:"autostart"`          // 开机自启（指针区分是否传递）
	Freeze          *bool                             `json:"freeze"`             // 启动时冻结 CPU
	APIC            *bool                             `json:"apic"`               // APIC 开关
	PAE             *bool                             `json:"pae"`                // PAE 开关
	BootType        string                            `json:"boot_type"`          // 引导方式: bios/uefi/uefi-secure
	RTCOffset       *string                           `json:"rtc_offset"`         // RTC 时间基准
	RTCStartDate    *string                           `json:"rtc_startdate"`      // RTC 开始日期
	GuestAgent      *vm_xml.VMGuestAgentConfig        `json:"guest_agent"`        // QEMU Guest Agent 配置
	SMBIOS1         *vm_xml.VMSMBIOS1Config           `json:"smbios1"`            // SMBIOS 类型 1 配置
	BootOrder       []string                          `json:"boot_order"`         // 启动顺序
	DeviceOrder     []string                          `json:"device_order"`       // 设备级启动顺序（dev 标识符，如 sdb/sda/vda）
	NicModel        string                            `json:"nic_model"`          // 网卡类型: virtio/e1000e/rtl8139
	VideoModel      string                            `json:"video_model"`        // 视频模型: virtio/vga/vmvga/cirrus/ramfb
	CPUTopologyMode string                            `json:"cpu_topology_mode"`  // CPU 拓扑模式
	CPULimitPercent *int                              `json:"cpu_limit_percent"`  // CPU 限制百分比（仅管理员，0 表示无限制）
	CPUAffinity     *string                           `json:"cpu_affinity"`       // CPU 亲和性（仅管理员，null 表示不修改，空字符串表示清除）
	MemoryDynamic   *vm_memory.VMMemoryDynamicRequest `json:"memory_dynamic"`     // 动态内存配置（管理员）
	HostDevices     []service.HostDeviceParam         `json:"host_devices"`       // 硬件直通设备
	// 磁盘操作
	AddDisks []VmAddDiskItem `json:"add_disks"` // 新增磁盘
	// 磁盘 IOPS 限制（仅管理员，key 为设备名如 vda）
	DiskIOPS map[string]service.DiskIOPSTune `json:"disk_iops"`
	// 带宽速率限制（Mbps，指针类型区分是否传递）
	BandwidthInboundAvg  *int `json:"bandwidth_inbound_avg"`  // 下行平峰速率 Mbps
	BandwidthOutboundAvg *int `json:"bandwidth_outbound_avg"` // 上行平峰速率 Mbps
	// PCIe 热插槽数量（仅关机时可修改，0 表示不修改）
	PCIERootPorts *int `json:"pcie_root_ports,omitempty"`
	// UEFI 固件兼容模式（ARM 专用）
	FirmwareCompat *bool `json:"firmware_compat,omitempty"`
	// 直接内核引导配置
	DirectBoot *service.DirectBootConfig `json:"direct_boot,omitempty"`
}

// RescueVmRequest 救援系统请求
type RescueVmRequest struct {
	Action string `json:"action" binding:"required"` // start 或 stop
}
