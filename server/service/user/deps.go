package user

import "kvm_console/model"

// ── Local type mirrors for service-root types that cross the package boundary ──

// ISOFileInfo mirrors service.ISOFileInfo.
type ISOFileInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      string `json:"size"`
	SizeBytes int64  `json:"size_bytes"`
	Pool      string `json:"pool"`
	OSType    string `json:"os_type"`
	OSVariant string `json:"os_variant"`
	MinDisk   int    `json:"min_disk"`
}

// LightweightVMRegistrationView mirrors service.LightweightVMRegistrationView.
type LightweightVMRegistrationView struct {
	ID                   uint   `json:"id"`
	Username             string `json:"username"`
	VMName               string `json:"vm_name"`
	Template             string `json:"template"`
	TemplateType         string `json:"template_type"`
	VCPU                 int    `json:"vcpu"`
	RAM                  int    `json:"ram"`
	DiskSize             int    `json:"disk_size"`
	DiskBus              string `json:"disk_bus"`
	Hostname             string `json:"hostname"`
	Autostart            bool   `json:"autostart"`
	Freeze               bool   `json:"freeze"`
	APIC                 bool   `json:"apic"`
	PAE                  bool   `json:"pae"`
	RTCOffset            string `json:"rtc_offset"`
	RTCStartDate         string `json:"rtc_startdate"`
	VideoModel           string `json:"video_model"`
	CPUTopologyMode      string `json:"cpu_topology_mode"`
	CPULimitPercent      int    `json:"cpu_limit_percent"`
	CPUAffinity          string `json:"cpu_affinity,omitempty"`
	FirstBootRebootMode  string `json:"first_boot_reboot_mode"`
	NicModel             string `json:"nic_model"`
	StoragePoolID        string `json:"storage_pool_id"`
	PreserveFnOSDeviceID bool   `json:"preserve_fnos_device_id"`
	FnOSDeviceID         string `json:"fnos_device_id"`
	SwitchID             uint   `json:"switch_id"`
	SwitchName           string `json:"switch_name"`
}

// LightweightVMQuotaRequest mirrors service.LightweightVMQuotaRequest.
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

// StorageQuotaInfo mirrors service.StorageQuotaInfo.
type StorageQuotaInfo struct {
	UsedBytes  int64
	LimitBytes int64
}

// TrafficUsageInfo mirrors service.TrafficUsageInfo.
type TrafficUsageInfo struct {
	MaxTrafficDown    float64
	MaxTrafficUp      float64
	UsedTrafficDown   int64
	UsedTrafficUp     int64
	UsedTrafficDownGB string
	UsedTrafficUpGB   string
	IsLimitedDown     bool
	IsLimitedUp       bool
}

// ── Hook variables ──

var (
	// Cloud / Lightweight VM hooks
	HookNormalizeCloudType                 func(value string) string
	HookIsLightweightCloudType             func(value string) bool
	HookIsLightweightCloudUser             func(username string) bool
	HookIsLightweightCloudVM               func(vmName string) bool
	HookNormalizeLightweightVMQuotaRequest func(req LightweightVMQuotaRequest) LightweightVMQuotaRequest
	HookDefaultLightweightVMQuota          func(vmName string) LightweightVMQuotaRequest
	HookFillLightweightVMQuotaRuntime      func(quota *model.LightweightVMQuota) *model.LightweightVMQuota
	HookListLightweightVMRegistrations     func(username string, includeActive bool) ([]LightweightVMRegistrationView, error)
	HookUpsertLightweightVMQuota           func(username string, req LightweightVMQuotaRequest) (*model.LightweightVMQuota, error)
	HookEnsureLightweightVMNetwork         func(username, vmName string) error
	HookCleanupLightweightVMResources      func(vmName string)
	HookCleanupVMVPCBinding                func(vmName string)

	// Network / Security hooks
	HookEnsureDefaultSecurityGroup  func(username string) (*model.VPCSecurityGroup, error)
	HookEnsureDefaultVPCSwitch      func(username string) (*model.VPCSwitch, error)
	HookCleanupUserNetworkResources func(username string, vmNames []string) error

	// VM cache hooks
	HookSyncVMCacheOwnersForAssignment func(username string, assignedVMs []string)
	HookUpdateVMCacheOwner             func(name, owner string)
	HookSyncVMCacheOwner               func(name string)

	// VM lifecycle hooks
	HookShutdownVM func(name string) error
	HookDestroyVM  func(name string) error

	// Storage hooks
	HookGetStorageMountPoint    func() string
	HookEnsureStorageFilesystem func() error
	HookSetupUserProject        func(username string, dirs []string) error
	HookGetProjectID            func(username string) (int, error)
	HookSetUserStorageQuota     func(username string, limitGB int) error
	HookRemoveUserStorageQuota  func(username string) error
	HookGetUserStorageUsage     func(username string) (*StorageQuotaInfo, error)
	HookInferOSFromISO          func(nameLower string) (osType, osVariant string)
	HookBuildISOInfo            func(filePath, poolName string) ISOFileInfo
	HookAddShare                func(vmName, hostPath, tag, securityModel string, readonly bool) error
	HookRemoveShare             func(vmName, tag string) error

	// Network / Traffic / Bandwidth hooks
	HookGetUserPublicIPUsage         func(username string) int
	HookGetUserPortForwardUsage      func(username string) int
	HookGetUserTrafficUsage          func(username string) *TrafficUsageInfo
	HookCheckTrafficAfterQuotaUpdate func(username string)
	HookRebalanceUserBandwidth       func(username string) error

	// Maintenance / Email hooks
	HookIsMaintenanceModeEnabled  func() bool
	HookIsLibvirtUnavailableText  func(text string) bool
	HookIsLibvirtUnavailableError func(err error) bool
	HookSendEmail                 func(to, subject, body string) error
)
