package security

import (
	"kvm_console/model"
)

// Deps holds function variables injected from the service root package
// to avoid circular imports. Set once at startup via InitDeps().
type Deps struct {
	// ---- Cloud type ----
	NormalizeCloudType     func(value string) string
	CloudTypeElastic       string
	IsLightweightCloudType func(value string) bool

	// ---- User provisioning ----
	ProvisionSystemUserResources func(user *model.User, password string) error

	// ---- VPC defaults ----
	EnsureDefaultSecurityGroup func(username string) (*model.VPCSecurityGroup, error)
	EnsureDefaultVPCSwitch     func(username string) (*model.VPCSwitch, error)

	// ---- Validation ----
	ValidateStrongPassword func(password string) error

	// ---- Lightweight cloud ----
	ListLightweightVMRegistrations      func(username string, includeActive bool) ([]LightweightVMRegistrationView, error)
	FormatLightweightVMRegistrationList func(regs []LightweightVMRegistrationView) string

	// ---- Network constants ----
	BridgeModeNAT string

	// ---- Maintenance mode ----
	IsMaintenanceModeEnabled func() bool

	// ---- Public access ----
	IsPublicAccessEnabled func() bool
}

// D is the package-level dependency container. Set via InitDeps().
var D *Deps

// InitDeps sets the dependency container. Called once at startup from main.go.
func InitDeps(d *Deps) {
	D = d
}

// LightweightVMRegistrationView mirrors service.LightweightVMRegistrationView
// for use within the security package.
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
