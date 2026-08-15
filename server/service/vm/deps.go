package vm

import (
	"context"
	"regexp"
	"time"

	"kvm_console/model"
	bwpkg "kvm_console/service/bandwidth"
	"kvm_console/service/network/vpc"
	"kvm_console/service/storage/disk"
	"kvm_console/service/storage/pool"
	"kvm_console/service/vm_xml"
)

// Deps 承载 vm 子包对外部模块的所有依赖，避免循环 import。
// 在 main.go 启动时通过 InitDeps() 一次性注入。
type Deps struct {
	// ---- Host ----
	FirstNonEmpty                 func(values ...string) string
	EnsureMaintenanceModeDisabled func(action string) error
	IsLibvirtUnavailableError     func(err error) bool
	IsMaintenanceModeEnabled      func() bool
	IsLibvirtUnavailableText      func(text string) bool

	// ---- Bandwidth ----
	GetVMBandwidthMbps           func(vmName string) (int, int)
	GetVMBandwidth               func(vmName string) (*bwpkg.BandwidthDetail, error)
	ReapplyConfiguredVMBandwidth func(vmName string) error

	// ---- User ----
	FindVMOwner        func(vmName string) string
	CheckQuotaForStart func(username, vmName string) error
	GetUserDiskDir     func(username string) string

	// ---- Clone ----
	InjectMemballoonConfig  func(xmlStr string, enableFPR bool) string
	RandomStringFromCharset func(charset string, length int) string
	ValidateStrongPassword  func(password string) error
	CloneUsernameRegexp     *regexp.Regexp

	// ---- Disk / Storage ----
	AddDiskWithBusInDir   func(vmName string, sizeGB int, format, bus, diskDir string) (string, error)
	SetDiskIOPSTune       func(vmName, dev string, iops *DiskIOPSTune) error
	GetDevPrefix          func(bus string) string
	GetVMPCIERootPorts    func(vmName string) (int, error)
	ParseQemuInfoGB       func(output, key string) string
	ParseQemuInfoStr      func(output, key string) string
	CheckVMSnapshotSafety func(vmName string) (bool, []string, error)
	GetDiskFilePath       func(vmName, device string) string
	ListDisks             func(vmName string) ([]DiskInfo, error)
	ChangeFloppy          func(vmName, imagePath, device string, forceNew bool) error

	// ---- Rescue ----
	IsInRescueMode func(vmName string) bool

	// ---- Stats ----
	GetCachedStats func(name string) *VmStats

	// ---- VPC / Network ----
	ApplyVPCBindingRuntime            func(vmName string) error
	ApplyVPCBindingToDomainXML        func(vmName, vmXML string) (string, bool, error)
	ApplyVPCSwitchToDomainXML         func(vmXML string, switchID uint) (string, error)
	SafeVMXMLFileName                 func(vmName string) string
	StripRuntimeOnlyInterfaceElements func(block string) string
	BridgeNameForSwitch               func(sw model.VPCSwitch) string
	SwitchUsesDirectBridge            func(sw model.VPCSwitch) bool
	IsPortSecurityEnabled             func() bool
	ReconcileVMPortSecurity           func(vmName string) error
	PrepareVMPortSecurityBinding      func(owner, vmName string, switchID, securityGroupID uint, allowedIPv4, allowedIPv6 string) error

	// ---- Storage pool ----
	GetAllISOs           func() ([]ISOFileInfo, error)
	ResolveVMStorageDir  func(poolID string, isAdmin bool) (string, string, error)
	ListVMStorageTargets func(isAdmin bool) ([]VMStorageTarget, error)

	// ---- OVS ----
	BuildOVSVirtInstallNetworkArg func(nicModel string) string
	EnsureOVSNetworkReady         func() error

	// ---- Snapshot ----
	FixSnapshotDiskPermissions func(vmName string)

	// ---- Lightweight ----
	ApplyLightweightVMBandwidth             func(vmName string) error
	CheckLightweightVMRuntimeQuotaAvailable func(vmName string) error
	IsLightweightCloudUser                  func(username string) bool
	IsLightweightCloudVM                    func(vmName string) bool
	GetLightweightVMQuota                   func(vmName string) (*model.LightweightVMQuota, error)

	// ---- Public IP ----
	ListPublicIPAttachmentsForVM func(vmName string) []PublicIPAttachment

	// ---- Template ----
	GetTemplateMeta       func(templateName string) *TemplateMeta
	WriteVMTemplateSource func(vmName, template, cloneMode string) error

	// ---- Resource check ----
	CheckDirWritable  func(dir string) error
	CheckStorageSpace func(dir string, requiredMB int64) error

	// ---- Host (additional) ----
	CollectHostDiskIOBytes func() (int64, int64, error)
	CollectHostNetDevices  func() ([]model.HostNetDeviceStat, error)
	CollectHostDiskDevices func() ([]model.HostDiskDeviceStat, error)

	// ---- Bandwidth (additional) ----
	RebalanceUserBandwidth func(username string) error

	// ---- Scheduler ----
	RegisterScheduler           func(def SchedulerDefinition)
	StartSchedulerEvent         func(input SchedulerEventStartInput) (*model.SchedulerEvent, error)
	FinishSchedulerEventSuccess func(event *model.SchedulerEvent, message string) error
	FinishSchedulerEventFailed  func(event *model.SchedulerEvent, message string) error

	// ---- Clone (additional - circular dep) ----
	DeleteVM func(vmName string) error

	// ---- Migration hooks ----
	HookEnsureVMNotMigrating         func(vmName, action string) error
	HookApplyVMUnderMigrationStatus  func(vm *VmInfo)
	HookDetectMigrationModeFromState func(state string) string
	HookMigrationModeLive            func() string

	// ---- CPU / Topology / Clock ----
	ParseVCPUCountFromDomainXML         func(xmlStr string) int
	ParseVMCPULimitPercentFromDomainXML func(xmlStr string, vcpu int) int
	ParseCPUAffinityFromDomainXML       func(xmlStr string) string
	ParseVMCPUTopologyModeFromDomainXML func(xmlStr string) string
	ParseVMAPICFromDomainXML            func(xmlContent string) bool
	ParseRTCOffsetFromDomainXML         func(xmlContent string) string
	ParseRTCStartDateFromDomainXML      func(xmlContent string) string
	ApplyRTCConfigToDomainXML           func(xmlContent, offset, startDate, guestType string) (string, error)
	ApplyVMAPICToDomainXML              func(xmlStr string, apic *bool) (string, error)
	ApplyVMCPULimitToDomainXML          func(xmlStr string, vcpu, percent int) string
	ApplyCPUTopologyModeToDomainXML     func(xmlStr, mode, osType string, vcpu int) string
	ParseCPUAffinity                    func(input string) ([]int, error)
	ValidateCPUAffinity                 func(cores []int) error
	ApplyCPUAffinityToDomainXML         func(xmlStr string, vcpu int, cores []int) string
	EffectiveTopologyVCPU               func(current, maxVCPU int) int
	SetVMCPUWithTopologySync            func(name string, vcpu, maxVCPU int) error
	NormalizeVMCPUTopologyMode          func(mode string) string

	// ---- Install Media ----
	NormalizeInstallISOSelection     func(primary string, paths []string) (string, []string)
	ApplyAdditionalCDROMsToDomainXML func(xmlContent string, isoPaths []string) (string, error)

	// ---- Passthrough ----
	EnsureVfioModuleLoaded   func() error
	ValidatePCIPassthrough   func(pciAddress string) error
	IsDeviceVfioBound        func(pciAddress string) bool
	BindPCIDeviceToVfio      func(pciAddress string) error
	ApplyHostDevsToDomainXML func(xmlContent string, hostDevs []HostDeviceParam) (string, error)

	// ---- VM Name ----
	ValidateVMName func(name string) error

	// ---- VM Lock ----
	IsVMLocked func(vmName string) bool

	// ---- CPU Limit ----
	VMCPULimitUnlimited       int
	ValidateVMCPULimitPercent func(percent int) error

	// ---- VM lifecycle (self-referencing: defined in vm_*.go files that will also be migrated) ----
	StartVM                     func(name string) error
	StartVMPreserveRebootAction func(name string) error
	ShutdownVM                  func(name string) error
	GetVMInactiveDomainXML      func(name string) (string, error)
	SetVMInactiveDomainXML      func(name, xml string) error
	SetVMRemark                 func(name, remark string) error
	SetVMFreeze                 func(name string, freeze bool) error
	FixOnReboot                 func(name string)

	// ---- SPICE graphics（创建即带，默认本地监听） ----
	InjectSPICEGraphics   func(xmlStr, passwd, listenAddr string) string
	EnsureQXLVideo        func(xmlStr string) string
	SpiceEnabledByDefault func() bool
}

// D is the package-level dependency container. Set via InitDeps().
var D *Deps

// InitDeps sets the dependency container. Called once at startup from main.go.
func InitDeps(d *Deps) {
	D = d
}

// ==================== Type aliases ====================
// Types from sub-packages that can be directly imported (no circular dependency).
// These aliases allow vm package code to use the same type names as the root package.

type (
	// Bandwidth types
	BandwidthDetail = bwpkg.BandwidthDetail

	// Disk types from storage/disk
	DiskIOPSTune   = disk.DiskIOPSTune
	DiskInfo       = disk.DiskInfo
	ExtraDiskParam = disk.ExtraDiskParam

	// Storage pool types
	VMStorageTarget = pool.VMStorageTarget
	ISOFileInfo     = pool.ISOFileInfo

	// VPC types
	AddVMInterfaceRequest = vpc.AddVMInterfaceRequest

	// vm_xml types
	VMGuestAgentConfig = vm_xml.VMGuestAgentConfig
	VMSMBIOS1Config    = vm_xml.VMSMBIOS1Config

	// Template type
	TemplateMeta = TemplateMetaAlias
)

// TemplateMetaAlias mirrors the TemplateMeta type for use within the vm package.
// The actual type is defined in the template subpackage; we mirror it here
// to avoid importing the template package (which would create a circular dependency
// if template itself imports vm in the future).
type TemplateMetaAlias struct {
	Type          string                 `json:"type"`
	Category      string                 `json:"category,omitempty"`
	BootType      string                 `json:"boot_type,omitempty"`
	BootVerified  bool                   `json:"boot_verified,omitempty"`
	NVRAMPath     string                 `json:"nvram_path,omitempty"`
	RootPassword  string                 `json:"root_password,omitempty"`
	TemplateUser  string                 `json:"template_user,omitempty"`
	DefaultConfig *TemplateDefaultConfig `json:"default_config,omitempty"`
	TemplateUID   string                 `json:"template_uid,omitempty"`
	NodeID        string                 `json:"node_id,omitempty"`
	ParentNodeID  string                 `json:"parent_node_id,omitempty"`
	RootNodeID    string                 `json:"root_node_id,omitempty"`
	AdminName     string                 `json:"admin_name,omitempty"`
	DisplayName   string                 `json:"display_name,omitempty"`
	CloneVisible  bool                   `json:"clone_visible"`
}

// TemplateDefaultConfig mirrors the template.TemplateDefaultConfig.
type TemplateDefaultConfig struct {
	DiskBus             string `json:"disk_bus,omitempty"`
	VideoModel          string `json:"video_model,omitempty"`
	CPUTopologyMode     string `json:"cpu_topology_mode,omitempty"`
	FirstBootRebootMode string `json:"first_boot_reboot_mode,omitempty"`
}

// PublicIPAttachment mirrors the public_ip.PublicIPAttachment type.
type PublicIPAttachment struct {
	PublicIP      string `json:"public_ip"`
	Mode          string `json:"mode"`
	ModeLabel     string `json:"mode_label"`
	VMPrivateIP   string `json:"vm_private_ip"`
	RuntimeStatus string `json:"runtime_status"`
}

// HostDeviceParam mirrors the passthrough device parameter type.
type HostDeviceParam struct {
	PCIAddress string `json:"pci_address"`
}

// WaitForVMShutOff waits for a VM to reach "shut off" state within the given timeout.
// Returns true if the VM shut off, false if the timeout expired without error.
type WaitForVMShutOffFunc func(ctx context.Context, name string, timeout time.Duration) (bool, error)

// SchedulerDefinition 调度器定义（镜像 scheduler.SchedulerDefinition，避免循环 import）。
type SchedulerDefinition struct {
	Key         string
	Name        string
	Group       string
	Description string
	Enabled     func() bool
}

// SchedulerEventStartInput 调度事件开始参数（镜像 scheduler.SchedulerEventStartInput，避免循环 import）。
type SchedulerEventStartInput struct {
	SchedulerKey   string
	SchedulerName  string
	SchedulerGroup string
	VMName         string
	VMBackend      string
	TriggerReason  string
}
