package clone

import (
	"context"
	"time"

	"kvm_console/service/network/vpc"
	"kvm_console/service/storage/disk"
	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
)

// Deps holds function variables injected from the service root package
// to avoid circular imports. Set once at startup via InitDeps().
type Deps struct {
	// ---- VM lifecycle ----
	StartVM                     func(name string) error
	StartVMPreserveRebootAction func(name string) error
	GetVMInactiveDomainXML      func(name string) (string, error)
	SetVMInactiveDomainXML      func(name, xml string) error
	SetVMRemark                 func(name, remark string) error
	SetVMFreeze                 func(name string, freeze bool) error
	FixOnReboot                 func(name string)

	// ---- Template ----
	GetTemplateMeta           func(templateName string) *TemplateMeta
	GetTemplateMinDiskSizeGB  func(templateName string) (int, error)
	EnsureTemplatePath        func(templateName string) (string, error)
	NormalizeTemplateBootType func(bootType string) string
	DetectTemplateBootType    func(templatePath string) string
	ResolveCloneDiskSizeGB    func(templateName string, requestedDiskSize int) (int, error)
	WriteVMTemplateSource     func(vmName, template, cloneMode string) error

	// ---- VM validation / normalization ----
	ValidateVMName                 func(name string) error
	NormalizeVMNicModel            func(model string) string
	NormalizeVMDiskBus             func(bus string) string
	NormalizeVMCPUTopologyMode     func(mode string) string
	NormalizeVMFirstBootRebootMode func(mode string) string
	VMCPULimitUnlimited            int
	ValidateVMCPULimitPercent      func(pct int) error

	// ---- Resource checks ----
	ResolveVMStorageDir func(poolID string, isAdmin bool) (string, string, error)
	CheckStorageSpace   func(dir string, neededBytes int64) error
	CheckDirWritable    func(dir string) error
	CheckHostMemory     func(ramMB int) error

	// ---- Network / OVS ----
	EnsureOVSNetworkReady         func() error
	BuildOVSVirtInstallNetworkArg func(model string) string
	BuildOVSInterfaceXML          func(mac, model string) string
	GetOVSStaticIPByMAC           func(mac string) string
	ListAllVPCStaticHosts         func() ([]OVSStaticHost, error)
	GetOVSLeaseIPByMAC            func(mac string) string
	ApplyVPCBindingToDomainXML    func(vmName, xml string) (string, bool, error)
	PrepareVMPortSecurityBinding  func(owner, vmName string, switchID, securityGroupID uint, allowedIPv4, allowedIPv6 string) error

	// ---- XML modification helpers ----
	ApplyRTCConfigToDomainXML           func(xmlStr, offset, startDate, tplType string) (string, error)
	ApplyVMAPICToDomainXML              func(xmlStr string, apic *bool) (string, error)
	ApplyCPUTopologyModeToDomainXML     func(xmlStr, mode, tplType string, vcpu int) string
	ApplyVMCPULimitToDomainXML          func(xmlStr string, vcpu, pct int) string
	ApplyCPUAffinityIfSet               func(xmlStr string, vcpu int, affinity string) (string, error)
	ApplyVPCSwitchToDomainXML           func(xmlStr string, switchID uint) (string, error)
	ApplyFirstBootRebootModeToDomainXML func(xmlStr, mode string) string
	EffectiveTopologyVCPU               func(vcpu, maxVCPU int) int
	ShouldUseWindowsFirstBootColdReboot func(mode, tplType string) bool
	CompleteWindowsFirstBootColdReboot  func(ctx context.Context, name string, progressFn func(int, string)) error
	BuildVCPUTag                        func(vcpu, maxVCPU int) string
	ResolveRTCOffset                    func(offset, tplType string) string
	NormalizeRTCStartDate               func(s string) string
	ParseRTCStartDateToEpoch            func(s string) (string, error)
	VMRTCStartDateNow                   string
	VMRTCOffsetAbsolute                 string
	InjectPCIERootPorts                 func(xmlContent string, portCount int) string

	// ---- Disk / storage ----
	AddExtraDisksForVM func(vmName string, extraDisks []disk.ExtraDiskParam, cloneDir, diskBus string, isAdmin bool, progressFn func(int, string)) error
	GetUserDiskDir     func(username string) string
	CheckStorageQuota  func(username string, additionalBytes int64) error

	// ---- VM credentials ----
	SaveVMCredential func(vmName, username, password, source, operator string, markReset bool) error

	// ---- VM cleanup ----
	DeleteVMStatsRecords          func(name string)
	DeleteVMRuntimeRecord         func(name string)
	CleanupVMVPCBinding           func(vmName string)
	CleanupLightweightVMResources func(vmName string)
	DeleteVMSchedules             func(vmName string) error

	// ---- CPU affinity ----
	ParseCPUAffinity            func(input string) ([]int, error)
	ValidateCPUAffinity         func(cores []int) error
	ApplyCPUAffinityToDomainXML func(xmlStr string, vcpu int, cores []int) string

	// ---- Template boot type ----
	ResolveTemplateBootType func(templatePath, templateType, bootType string, bootVerified bool, detector func(string) string) (string, bool)

	// ---- VM first boot ----
	WaitForVMShutOff func(ctx context.Context, name string, timeout time.Duration) (bool, error)

	// ---- Utility ----
	FirstNonEmpty func(values ...string) string
	GetVMDiskInfo func(name string) VMDiskInfoResult

	// ---- Disk expansion ----
	PrepareFnOSSystemDiskExpansion    func(ctx context.Context, cloneDisk string, progressFn func(int, string)) error
	PrepareWindowsSystemDiskExpansion func(ctx context.Context, cloneDisk string, progressFn func(int, string)) error

	// ---- Migration hook ----
	HookEnsureVMNotMigrating func(vmName, action string) error

	// ---- SPICE graphics（创建即带，默认本地监听） ----
	InjectSPICEGraphics   func(xmlStr, passwd, listenAddr string) string
	EnsureQXLVideo        func(xmlStr string) string
	SpiceEnabledByDefault func() bool
}

// TemplateMeta mirrors service.TemplateMeta for use within the clone package.
type TemplateMeta struct {
	Type             string                 `json:"type"`
	Category         string                 `json:"category"` // 二级分类（如 WindowsServer2025/WindowsServer2022 等）
	BootType         string                 `json:"boot_type"`
	RootPassword     string                 `json:"root_password"`      // 已废弃，保留兼容旧元数据
	TemplateUser     string                 `json:"template_user"`      // 模板中的普通用户名（用于用户名重命名）
	CloudInitMode    string                 `json:"cloud_init_mode"`    // 初始化模式: "nocloud"/"configdrive"/"fnos"/"none"
	PostBootCommand  string                 `json:"post_boot_command"`  // Linux 模板启动后执行的自定义命令
	PostBootBlocking bool                   `json:"post_boot_blocking"` // 启动后命令阻塞模式
	NVRAMPath        string                 `json:"nvram_path"`
	DefaultConfig    *TemplateDefaultConfig `json:"default_config,omitempty"`
}

// TemplateDefaultConfig mirrors service.TemplateDefaultConfig.
type TemplateDefaultConfig struct {
	DiskBus             string `json:"disk_bus,omitempty"`
	VideoModel          string `json:"video_model,omitempty"`
	CPUTopologyMode     string `json:"cpu_topology_mode,omitempty"`
	FirstBootRebootMode string `json:"first_boot_reboot_mode,omitempty"`
}

// OVSStaticHost mirrors the service type for VPC static host entries.
type OVSStaticHost struct {
	MAC string
	IP  string
}

// VMDiskInfoResult mirrors the unexported diskInfoResult from service root.
type VMDiskInfoResult struct {
	Path   string
	Device string
	Size   string
}

// D is the package-level dependency container. Set via InitDeps().
var D *Deps

// InitDeps sets the dependency container. Called once at startup from main.go.
func InitDeps(d *Deps) {
	D = d
}

// Type aliases for external types used in CloneParams / BatchCloneParams.
type (
	AddVMInterfaceRequest = vpc.AddVMInterfaceRequest
	ExtraDiskParam        = disk.ExtraDiskParam
	DiskIOPSTune          = disk.DiskIOPSTune
)

// Re-export memory and vm_xml types used in clone params
type (
	VMMemoryDynamicRequest = memory.VMMemoryDynamicRequest
	VMGuestAgentConfig     = vm_xml.VMGuestAgentConfig
	VMSMBIOS1Config        = vm_xml.VMSMBIOS1Config
)

// Re-export constants from vm_xml
var (
	VMBootTypeUEFI       = vm_xml.VMBootTypeUEFI
	VMBootTypeUEFISecure = vm_xml.VMBootTypeUEFISecure
	VMBootTypeBIOS       = vm_xml.VMBootTypeBIOS
)
