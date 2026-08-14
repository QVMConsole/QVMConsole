package lightweight

import (
	"kvm_console/service/vm_xml"
)

// LightweightVMQuotaRequest 管理员为轻量云 VM 设置的单机配额。
type LightweightVMQuotaRequest struct {
	VMName            string  `json:"vm_name"`
	TrafficDownGB     float64 `json:"traffic_down_gb"`
	TrafficUpGB       float64 `json:"traffic_up_gb"`
	BandwidthDownMbps int     `json:"bandwidth_down_mbps"`
	BandwidthUpMbps   int     `json:"bandwidth_up_mbps"`
	MaxPortForwards   int     `json:"max_port_forwards"`
	MaxSnapshots      int     `json:"max_snapshots"`
	MaxRuntimeHours   int     `json:"max_runtime_hours"`
}

// LightweightVMRegistrationRequest 是管理员登记轻量云 VM 的表单数据。
type LightweightVMRegistrationRequest struct {
	VMName               string                     `json:"vm_name"`
	Template             string                     `json:"template"`
	TemplateType         string                     `json:"template_type"`
	CloneMode            string                     `json:"clone_mode"`
	VCPU                 int                        `json:"vcpu"`
	RAM                  int                        `json:"ram"`
	DiskSize             int                        `json:"disk_size"`
	DiskBus              string                     `json:"disk_bus"`
	Hostname             string                     `json:"hostname"`
	Autostart            bool                       `json:"autostart"`
	Freeze               bool                       `json:"freeze"`
	APIC                 *bool                      `json:"apic"`
	PAE                  *bool                      `json:"pae"`
	RTCOffset            string                     `json:"rtc_offset"`
	RTCStartDate         string                     `json:"rtc_startdate"`
	GuestAgent           *vm_xml.VMGuestAgentConfig `json:"guest_agent"`
	SMBIOS1              *vm_xml.VMSMBIOS1Config    `json:"smbios1"`
	VideoModel           string                     `json:"video_model"`
	CPUTopologyMode      string                     `json:"cpu_topology_mode"`
	CPULimitPercent      int                        `json:"cpu_limit_percent"`
	CPUAffinity          string                     `json:"cpu_affinity,omitempty"` // CPU 亲和性，如 "0,2,4"
	FirstBootRebootMode  string                     `json:"first_boot_reboot_mode"`
	NicModel             string                     `json:"nic_model"`
	StoragePoolID        string                     `json:"storage_pool_id"`
	PreserveFnOSDeviceID bool                       `json:"preserve_fnos_device_id"`
	FnOSDeviceID         string                     `json:"fnos_device_id"`
	TrafficDownGB        float64                    `json:"traffic_down_gb"`
	TrafficUpGB          float64                    `json:"traffic_up_gb"`
	BandwidthDownMbps    int                        `json:"bandwidth_down_mbps"`
	BandwidthUpMbps      int                        `json:"bandwidth_up_mbps"`
	MaxPortForwards      int                        `json:"max_port_forwards"`
	MaxSnapshots         int                        `json:"max_snapshots"`
	MaxRuntimeHours      int                        `json:"max_runtime_hours"`
}

// LightweightVMConfirmRequest 是用户确认开通时补齐的登录凭据。
type LightweightVMConfirmRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LightweightVMProvisionParams 轻量云 VM 开通参数。
type LightweightVMProvisionParams struct {
	RegistrationID     uint   `json:"registration_id"`
	Username           string `json:"username"`
	CredentialUsername string `json:"credential_username"`
	CredentialPassword string `json:"credential_password"`
	Operator           string `json:"operator"`
}

// LightweightVMRegistrationView 轻量云 VM 注册记录视图。
type LightweightVMRegistrationView struct {
	ID                   uint    `json:"id"`
	Username             string  `json:"username"`
	VMName               string  `json:"vm_name"`
	Template             string  `json:"template"`
	TemplateType         string  `json:"template_type"`
	VCPU                 int     `json:"vcpu"`
	RAM                  int     `json:"ram"`
	DiskSize             int     `json:"disk_size"`
	DiskBus              string  `json:"disk_bus"`
	Hostname             string  `json:"hostname"`
	Autostart            bool    `json:"autostart"`
	Freeze               bool    `json:"freeze"`
	APIC                 bool    `json:"apic"`
	PAE                  bool    `json:"pae"`
	RTCOffset            string  `json:"rtc_offset"`
	RTCStartDate         string  `json:"rtc_startdate"`
	VideoModel           string  `json:"video_model"`
	CPUTopologyMode      string  `json:"cpu_topology_mode"`
	CPULimitPercent      int     `json:"cpu_limit_percent"`
	CPUAffinity          string  `json:"cpu_affinity,omitempty"`
	FirstBootRebootMode  string  `json:"first_boot_reboot_mode"`
	NicModel             string  `json:"nic_model"`
	StoragePoolID        string  `json:"storage_pool_id"`
	PreserveFnOSDeviceID bool    `json:"preserve_fnos_device_id"`
	FnOSDeviceID         string  `json:"fnos_device_id"`
	SwitchID             uint    `json:"switch_id"`
	SwitchName           string  `json:"switch_name"`
	SwitchCIDR           string  `json:"switch_cidr"`
	TrafficDownGB        float64 `json:"traffic_down_gb"`
	TrafficUpGB          float64 `json:"traffic_up_gb"`
	BandwidthDownMbps    int     `json:"bandwidth_down_mbps"`
	BandwidthUpMbps      int     `json:"bandwidth_up_mbps"`
	MaxPortForwards      int     `json:"max_port_forwards"`
	MaxSnapshots         int     `json:"max_snapshots"`
	MaxRuntimeHours      int     `json:"max_runtime_hours"`
	Status               string  `json:"status"`
	TaskID               uint    `json:"task_id"`
	ErrorMessage         string  `json:"error_message"`
	CreatedBy            string  `json:"created_by"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
	ConfirmedAt          string  `json:"confirmed_at,omitempty"`
}

// LightweightVMRuntimeQuotaSnapshot 轻量云单 VM 运行时长配额快照。
type LightweightVMRuntimeQuotaSnapshot struct {
	UsedSeconds      int64
	RemainingSeconds int64
	QuotaReached     bool
}

// LightweightRuntimeQuotaShutdownResult 轻量云运行时长配额自动关机结果。
type LightweightRuntimeQuotaShutdownResult struct {
	Username string   `json:"username"`
	VMName   string   `json:"vm_name"`
	Stopped  bool     `json:"stopped"`
	Warnings []string `json:"warnings,omitempty"`
}
