package model

import "time"

// LightweightVMRegistration 保存轻量云待开通服务器配置。
type LightweightVMRegistration struct {
	ID                   uint       `json:"id" gorm:"primaryKey"`
	Username             string     `json:"username" gorm:"index;not null;size:64"`
	VMName               string     `json:"vm_name" gorm:"uniqueIndex;not null;size:128"`
	Template             string     `json:"template" gorm:"not null;size:128"`
	TemplateType         string     `json:"template_type" gorm:"size:32"`
	CloneMode            string     `json:"clone_mode" gorm:"size:16;default:linked"`
	VCPU                 int        `json:"vcpu" gorm:"not null"`
	RAM                  int        `json:"ram" gorm:"not null"`
	DiskSize             int        `json:"disk_size" gorm:"default:0"`
	DiskBus              string     `json:"disk_bus" gorm:"size:32;default:virtio"`
	Hostname             string     `json:"hostname" gorm:"size:64"`
	Autostart            bool       `json:"autostart" gorm:"default:false"`
	Freeze               bool       `json:"freeze" gorm:"default:false"`
	APIC                 *bool      `json:"apic"`
	PAE                  *bool      `json:"pae"`
	RTCOffset            string     `json:"rtc_offset" gorm:"size:32"`
	RTCStartDate         string     `json:"rtc_startdate" gorm:"size:64"`
	GuestAgentJSON       string     `json:"-" gorm:"type:text"`
	SMBIOS1JSON          string     `json:"-" gorm:"type:text"`
	VideoModel           string     `json:"video_model" gorm:"size:32"`
	CPUTopologyMode      string     `json:"cpu_topology_mode" gorm:"size:32"`
	CPULimitPercent      int        `json:"cpu_limit_percent" gorm:"default:0"`
	CPUAffinity          string     `json:"cpu_affinity" gorm:"size:255"`
	FirstBootRebootMode  string     `json:"first_boot_reboot_mode" gorm:"size:32"`
	NicModel             string     `json:"nic_model" gorm:"size:32;default:virtio"`
	StoragePoolID        string     `json:"storage_pool_id" gorm:"size:255"`
	PreserveFnOSDeviceID bool       `json:"preserve_fnos_device_id" gorm:"default:false"`
	FnOSDeviceID         string     `json:"fnos_device_id" gorm:"size:40"`
	SwitchID             uint       `json:"switch_id" gorm:"default:0"`
	TrafficDownGB        float64    `json:"traffic_down_gb" gorm:"default:0"`
	TrafficUpGB          float64    `json:"traffic_up_gb" gorm:"default:0"`
	BandwidthDownMbps    int        `json:"bandwidth_down_mbps" gorm:"default:0"`
	BandwidthUpMbps      int        `json:"bandwidth_up_mbps" gorm:"default:0"`
	MaxPortForwards      int        `json:"max_port_forwards" gorm:"default:10"`
	MaxSnapshots         int        `json:"max_snapshots" gorm:"default:2"`
	MaxRuntimeHours      int        `json:"max_runtime_hours" gorm:"default:0"`
	Status               string     `json:"status" gorm:"index;not null;size:32;default:pending"`
	TaskID               uint       `json:"task_id" gorm:"default:0"`
	ErrorMessage         string     `json:"error_message" gorm:"type:text"`
	CreatedBy            string     `json:"created_by" gorm:"index;size:64"`
	ConfirmedAt          *time.Time `json:"confirmed_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (LightweightVMRegistration) TableName() string {
	return "lightweight_vm_registrations"
}
