package host

import "time"

// --- node.go types ---

type HostNodeRequest struct {
	Name        string `json:"name"`
	APIBaseURL  string `json:"api_base_url"`
	APIKeyID    string `json:"api_key_id"`
	APIKey      string `json:"api_key"`
	SSHHost     string `json:"ssh_host"`
	SSHPort     int    `json:"ssh_port"`
	SSHUser     string `json:"ssh_user"`
	SSHPassword string `json:"ssh_password"`
	Enabled     *bool  `json:"enabled"`
}

type HostNodeView struct {
	ID               uint                   `json:"id"`
	Name             string                 `json:"name"`
	APIBaseURL       string                 `json:"api_base_url"`
	APIKeyID         string                 `json:"api_key_id"`
	SSHHost          string                 `json:"ssh_host"`
	SSHPort          int                    `json:"ssh_port"`
	SSHUser          string                 `json:"ssh_user"`
	Enabled          bool                   `json:"enabled"`
	Status           string                 `json:"status"`
	LastProbeMessage string                 `json:"last_probe_message"`
	Capabilities     map[string]interface{} `json:"capabilities"`
	LastProbedAt     *time.Time             `json:"last_probed_at"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// --- ksm.go types ---

type HostKSMProfile struct {
	Key              string `json:"key"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Run              int    `json:"run"`
	PagesToScan      int    `json:"pages_to_scan"`
	SleepMillisecs   int    `json:"sleep_millisecs"`
	MergeAcrossNodes bool   `json:"merge_across_nodes"`
	UseZeroPages     bool   `json:"use_zero_pages"`
	SmartScan        bool   `json:"smart_scan"`
}

type HostKSMRuntimeConfig struct {
	Run              *int  `json:"run"`
	PagesToScan      *int  `json:"pages_to_scan"`
	SleepMillisecs   *int  `json:"sleep_millisecs"`
	MergeAcrossNodes *bool `json:"merge_across_nodes"`
	UseZeroPages     *bool `json:"use_zero_pages"`
	SmartScan        *bool `json:"smart_scan"`
}

type HostKSMMetrics struct {
	PagesShared   *int64 `json:"pages_shared"`
	PagesSharing  *int64 `json:"pages_sharing"`
	PagesUnshared *int64 `json:"pages_unshared"`
	PagesVolatile *int64 `json:"pages_volatile"`
	PagesScanned  *int64 `json:"pages_scanned"`
	FullScans     *int64 `json:"full_scans"`
	GeneralProfit *int64 `json:"general_profit"`
}

type HostKSMStatus struct {
	Supported            bool                  `json:"supported"`
	Enabled              bool                  `json:"enabled"`
	CurrentProfile       string                `json:"current_profile"`
	PersistentConfigured bool                  `json:"persistent_configured"`
	PersistentProfile    string                `json:"persistent_profile"`
	RuntimeConfig        HostKSMRuntimeConfig  `json:"runtime_config"`
	PersistentConfig     *HostKSMRuntimeConfig `json:"persistent_config,omitempty"`
	Metrics              HostKSMMetrics        `json:"metrics"`
	Profiles             []HostKSMProfile      `json:"profiles"`
	Message              string                `json:"message"`
}

// --- zram.go types ---

type HostZRAMProfile struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SizePercent int    `json:"size_percent"`
	MaxSizeMB   int    `json:"max_size_mb"`
	Algorithm   string `json:"algorithm"`
	Priority    int    `json:"priority"`
}

type HostZRAMRuntimeConfig struct {
	Device          string `json:"device"`
	SizeBytes       *int64 `json:"size_bytes"`
	SizeMB          *int64 `json:"size_mb"`
	UsedBytes       *int64 `json:"used_bytes"`
	UsedMB          *int64 `json:"used_mb"`
	OriginalBytes   *int64 `json:"original_bytes"`
	CompressedBytes *int64 `json:"compressed_bytes"`
	Algorithm       string `json:"algorithm"`
	Priority        *int   `json:"priority"`
}

type HostZRAMPersistentConfig struct {
	Profile     string `json:"profile"`
	SizePercent int    `json:"size_percent"`
	MaxSizeMB   int    `json:"max_size_mb"`
	Algorithm   string `json:"algorithm"`
	Priority    int    `json:"priority"`
}

type HostZRAMStatus struct {
	Supported            bool                      `json:"supported"`
	Enabled              bool                      `json:"enabled"`
	CurrentProfile       string                    `json:"current_profile"`
	PersistentConfigured bool                      `json:"persistent_configured"`
	PersistentProfile    string                    `json:"persistent_profile"`
	RuntimeConfig        HostZRAMRuntimeConfig     `json:"runtime_config"`
	PersistentConfig     *HostZRAMPersistentConfig `json:"persistent_config,omitempty"`
	Profiles             []HostZRAMProfile         `json:"profiles"`
	Message              string                    `json:"message"`
}

// --- disk.go types ---

type HostDiskInfo struct {
	MountPoint string `json:"mount_point"`
	Device     string `json:"device"`
	FSType     string `json:"fs_type"`
	TotalKB    int64  `json:"total_kb"`
	UsedKB     int64  `json:"used_kb"`
	FreeKB     int64  `json:"free_kb"`
	UsePercent string `json:"use_percent"`
	ReadOnly   bool   `json:"read_only"`
}

// --- hardware.go types ---

// HostCPUHardware 宿主机 CPU 硬件信息与每核实时使用率
type HostCPUHardware struct {
	Model        string    `json:"model"`          // CPU 型号
	Sockets      int       `json:"sockets"`        // 物理插槽数
	Cores        int       `json:"cores"`          // 物理核心数
	Threads      int       `json:"threads"`        // 逻辑线程数
	PerCoreUsage []float64 `json:"per_core_usage"` // 每核使用率（%），下标即核心序号
}

// HostMemoryModule 单根内存条（DIMM）信息
type HostMemoryModule struct {
	Slot            string `json:"slot"`             // 插槽位置（Locator）
	SizeMB          int64  `json:"size_mb"`          // 容量（MB）
	Type            string `json:"type"`             // 类型（DDR4/DDR5 等）
	Speed           string `json:"speed"`            // 标称频率
	ConfiguredSpeed string `json:"configured_speed"` // 实际运行频率
	Manufacturer    string `json:"manufacturer"`     // 厂商
	PartNumber      string `json:"part_number"`      // 型号编码
}

// HostMemoryModulesInfo 宿主机内存条汇总信息
type HostMemoryModulesInfo struct {
	TotalSlots int                `json:"total_slots"` // 总插槽数
	Installed  int                `json:"installed"`   // 已插条数
	Modules    []HostMemoryModule `json:"modules"`     // 内存条列表
	Message    string             `json:"message"`     // 不可用时的说明
}

// --- maintenance.go types ---

type MaintenanceModeTaskParams struct {
	ServiceUnits []string `json:"service_units,omitempty"`
}

type MaintenanceModeTaskResult struct {
	StoppedVMs       []string `json:"stopped_vms,omitempty"`
	DisabledServices []string `json:"disabled_services,omitempty"`
	EnabledServices  []string `json:"enabled_services,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}
