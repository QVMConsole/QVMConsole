package migration

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"kvm_console/model"
	"kvm_console/service"
)

const (
	MigrationModeCold                   = "cold"
	MigrationModeLive                   = "live"
	defaultMigrationCPUThrottlePercent  = 50
	liveMigrationDirtyRateBlockRatio    = 0.50
	liveMigrationDirtyRateThrottleRatio = 0.20
)

// VMStatusMigrating marks a VM as being actively migrated.
const VMStatusMigrating = "migrating"

type VMMigrationRequest struct {
	NodeID                uint                         `json:"node_id"`
	Mode                  string                       `json:"mode"`
	PreviewID             string                       `json:"preview_id"`
	SkipPrecheck          bool                         `json:"skip_precheck"`
	TargetStoragePoolID   string                       `json:"target_storage_pool_id"`
	DiskStorageTargets    []MigrationDiskStorageTarget `json:"disk_storage_targets"`
	TargetSwitchID        uint                         `json:"target_switch_id"`
	TargetSecurityGroupID uint                         `json:"target_security_group_id"`
	EnableCPUThrottle     bool                         `json:"enable_cpu_throttle"`
	CPUThrottlePercent    int                          `json:"cpu_throttle_percent"`
}

type VMMigrationTaskParams struct {
	VMName                string                       `json:"vm_name"`
	NodeID                uint                         `json:"node_id"`
	Mode                  string                       `json:"mode"`
	PreviewID             string                       `json:"preview_id"`
	SkipPrecheck          bool                         `json:"skip_precheck"`
	TargetStoragePoolID   string                       `json:"target_storage_pool_id"`
	DiskStorageTargets    []MigrationDiskStorageTarget `json:"disk_storage_targets"`
	TargetSwitchID        uint                         `json:"target_switch_id"`
	TargetSecurityGroupID uint                         `json:"target_security_group_id"`
	EnableCPUThrottle     bool                         `json:"enable_cpu_throttle"`
	CPUThrottlePercent    int                          `json:"cpu_throttle_percent"`
}

type VMMigrationPreview struct {
	PreviewID             string                       `json:"preview_id,omitempty"`
	VMName                string                       `json:"vm_name"`
	Mode                  string                       `json:"mode"`
	Node                  service.HostNodeView         `json:"node"`
	Owner                 string                       `json:"owner"`
	CloudType             string                       `json:"cloud_type"`
	TargetUserExists      bool                         `json:"target_user_exists"`
	WillCreateTargetUser  bool                         `json:"will_create_target_user"`
	IsLightweight         bool                         `json:"is_lightweight"`
	SourceState           string                       `json:"source_state"`
	Disks                 []MigrationDisk              `json:"disks"`
	BackingChecks         []MigrationBackingCheck      `json:"backing_checks"`
	SourceBinding         *model.VPCVMBinding          `json:"source_binding,omitempty"`
	TargetStoragePoolID   string                       `json:"target_storage_pool_id"`
	TargetStorageDir      string                       `json:"target_storage_dir"`
	DiskStorageTargets    []MigrationDiskStorageTarget `json:"disk_storage_targets"`
	TargetStorageTargets  []service.VMStorageTarget    `json:"target_storage_targets"`
	RequiredStorageBytes  int64                        `json:"required_storage_bytes"`
	TargetSwitches        []model.VPCSwitch            `json:"target_switches"`
	TargetSecurityGroups  []model.VPCSecurityGroup     `json:"target_security_groups"`
	TargetSwitchID        uint                         `json:"target_switch_id"`
	TargetSecurityGroupID uint                         `json:"target_security_group_id"`
	PortForwards          []MigrationPortForwardMap    `json:"port_forwards"`
	Credential            *service.VMCredentialInfo    `json:"credential,omitempty"`
	LiveAssessment        *MigrationLiveAssessment     `json:"live_assessment,omitempty"`
	Warnings              []string                     `json:"warnings"`
	Blockers              []string                     `json:"blockers"`
	Allowed               bool                         `json:"allowed"`
}

type MigrationDisk struct {
	Target              string `json:"target"`
	SourcePath          string `json:"source_path"`
	TargetPath          string `json:"target_path"`
	TargetStoragePoolID string `json:"target_storage_pool_id"`
	TargetStorageDir    string `json:"target_storage_dir"`
	BackingPath         string `json:"backing_path"`
	BackingFormat       string `json:"backing_format"`
	VirtualSize         int64  `json:"virtual_size"`
	ActualSize          int64  `json:"actual_size"`
	Format              string `json:"format"`
}

type MigrationDiskStorageTarget struct {
	Target              string `json:"target"`
	Device              string `json:"device,omitempty"`
	TargetStoragePoolID string `json:"target_storage_pool_id"`
}

type MigrationBackingCheck struct {
	Path              string `json:"path"`
	SourceFormat      string `json:"source_format"`
	TargetFormat      string `json:"target_format"`
	SourceVirtualSize int64  `json:"source_virtual_size"`
	TargetVirtualSize int64  `json:"target_virtual_size"`
	SourceSHA256      string `json:"source_sha256"`
	TargetSHA256      string `json:"target_sha256"`
	OK                bool   `json:"ok"`
	Message           string `json:"message"`
}

type MigrationPortForwardMap struct {
	Protocol       string `json:"protocol"`
	SourceHostPort string `json:"source_host_port"`
	TargetHostPort string `json:"target_host_port"`
	VMPort         string `json:"vm_port"`
	DestIP         string `json:"dest_ip"`
	SourceIP       string `json:"source_ip"` // 入站 IP 白名单（CIDR，0.0.0.0/0 = 不限制）
	AutoAllocated  bool   `json:"auto_allocated"`
}

type MigrationLiveAssessment struct {
	AverageBandwidthMiB   float64           `json:"average_bandwidth_mib"`
	SpeedTestSeconds      float64           `json:"speed_test_seconds"`
	DirtyRateMiB          float64           `json:"dirty_rate_mib"`
	DirtyRateRatio        float64           `json:"dirty_rate_ratio"`
	DirtyRateRatioPercent float64           `json:"dirty_rate_ratio_percent"`
	Allowed               bool              `json:"allowed"`
	RequiresCPUThrottle   bool              `json:"requires_cpu_throttle"`
	CPUThrottleEnabled    bool              `json:"cpu_throttle_enabled"`
	CPUThrottlePercent    int               `json:"cpu_throttle_percent"`
	KVMStatAvailable      bool              `json:"kvm_stat_available"`
	KVMPageFaultRate      float64           `json:"kvm_page_fault_rate"`
	KVMStatMessage        string            `json:"kvm_stat_message"`
	DirtyRateStats        map[string]string `json:"dirty_rate_stats,omitempty"`
	Dommemstat            map[string]int64  `json:"dommemstat,omitempty"`
	BlockReason           string            `json:"block_reason,omitempty"`
	Warnings              []string          `json:"warnings,omitempty"`
}

type MigrationAdoptRequest struct {
	VMName                string                       `json:"vm_name"`
	Owner                 string                       `json:"owner"`
	CloudType             string                       `json:"cloud_type"`
	TargetSwitchID        uint                         `json:"target_switch_id"`
	TargetSecurityGroupID uint                         `json:"target_security_group_id"`
	User                  MigrationUserSnapshot        `json:"user"`
	LightweightQuota      *service.LightweightVMQuotaRequest `json:"lightweight_quota,omitempty"`
	Credential            *service.VMCredentialInfo    `json:"credential,omitempty"`
	PortForwards          []MigrationPortForwardMap    `json:"port_forwards"`
}

type MigrationUserSnapshot struct {
	Username             string     `json:"username"`
	PasswordHash         string     `json:"password_hash,omitempty"`
	Email                string     `json:"email"`
	CloudType            string     `json:"cloud_type"`
	Status               string     `json:"status"`
	EmailVerifiedAt      *time.Time `json:"email_verified_at,omitempty"`
	LoginVerifiedUntil   *time.Time `json:"login_verified_until,omitempty"`
	SecurityUpdatedAt    *time.Time `json:"security_updated_at,omitempty"`
	MaxCPU               int        `json:"max_cpu"`
	MaxMemory            int        `json:"max_memory"`
	MaxDisk              int        `json:"max_disk"`
	MaxVM                int        `json:"max_vm"`
	MaxStorage           int        `json:"max_storage"`
	MaxRuntimeHours      int        `json:"max_runtime_hours"`
	EnablePortForward    bool       `json:"enable_port_forward"`
	MaxPortForwards      int        `json:"max_port_forwards"`
	MaxSnapshots         int        `json:"max_snapshots"`
	MaxBandwidthUp       float64    `json:"max_bandwidth_up"`
	MaxBandwidthDown     float64    `json:"max_bandwidth_down"`
	MaxTrafficDown       float64    `json:"max_traffic_down"`
	MaxTrafficUp         float64    `json:"max_traffic_up"`
	MaxPublicIPs         int        `json:"max_public_ips"`
	DedicatedVPCSwitchID uint       `json:"dedicated_vpc_switch_id"`
}

type MigrationAdoptResult struct {
	VMName       string                    `json:"vm_name"`
	Owner        string                    `json:"owner"`
	CreatedUser  bool                      `json:"created_user"`
	PortForwards []MigrationPortForwardMap `json:"port_forwards"`
	Warnings     []string                  `json:"warnings"`
}

type VMMigrationOptions struct {
	VMName                string                       `json:"vm_name"`
	SourceState           string                       `json:"source_state"`
	Mode                  string                       `json:"mode"`
	Owner                 string                       `json:"owner"`
	CloudType             string                       `json:"cloud_type"`
	IsLightweight         bool                         `json:"is_lightweight"`
	SourceBinding         *model.VPCVMBinding          `json:"source_binding,omitempty"`
	TargetUserExists      bool                         `json:"target_user_exists"`
	WillCreateTargetUser  bool                         `json:"will_create_target_user"`
	TargetStorageTargets  []service.VMStorageTarget    `json:"target_storage_targets"`
	DiskStorageTargets    []MigrationDiskStorageTarget `json:"disk_storage_targets"`
	TargetSwitches        []model.VPCSwitch            `json:"target_switches"`
	TargetSecurityGroups  []model.VPCSecurityGroup     `json:"target_security_groups"`
	TargetSwitchID        uint                         `json:"target_switch_id"`
	TargetSecurityGroupID uint                         `json:"target_security_group_id"`
}

type cachedMigrationPreview struct {
	Preview   VMMigrationPreview
	ExpiresAt time.Time
}

var migrationPreviewCache = struct {
	sync.Mutex
	items map[string]cachedMigrationPreview
}{items: map[string]cachedMigrationPreview{}}

// ---------- preview cache helpers ----------

func storeMigrationPreview(preview VMMigrationPreview) string {
	id, err := randomMigrationPreviewID()
	if err != nil {
		id = fmt.Sprintf("mig_%d", time.Now().UnixNano())
	}
	preview.PreviewID = id
	migrationPreviewCache.Lock()
	defer migrationPreviewCache.Unlock()
	cleanupExpiredMigrationPreviewsLocked()
	migrationPreviewCache.items[id] = cachedMigrationPreview{
		Preview:   preview,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	return id
}

func loadMigrationPreview(id string) (*VMMigrationPreview, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("请先完成迁移预检")
	}
	migrationPreviewCache.Lock()
	defer migrationPreviewCache.Unlock()
	cleanupExpiredMigrationPreviewsLocked()
	cached, ok := migrationPreviewCache.items[id]
	if !ok {
		return nil, fmt.Errorf("迁移预检已过期，请重新预检")
	}
	preview := cached.Preview
	return &preview, nil
}

func cleanupExpiredMigrationPreviewsLocked() {
	now := time.Now()
	for id, cached := range migrationPreviewCache.items {
		if now.After(cached.ExpiresAt) {
			delete(migrationPreviewCache.items, id)
		}
	}
}

func randomMigrationPreviewID() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "mig_" + base64.RawURLEncoding.EncodeToString(raw), nil
}
