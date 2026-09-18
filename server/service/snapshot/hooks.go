package snapshot

import (
	"time"

	"kvm_console/model"
)

// hooks.go — 子包通过 hook 变量调用 service 根包函数，避免循环 import。
// service 根包在 snapshot_register.go 的 init() 中为这些变量赋值。

// ShareBrief 等价于 service.ShareInfo 的关键字段，避免 snapshot 包引用 service 根包类型。
type ShareBrief struct {
	Tag    string
	Source string
	Target string
}

// QemuImgInfo 等价于 service.qemuImgInfo，避免 snapshot 包引用 service 根包类型。
type QemuImgInfo struct {
	Filename            string
	Format              string
	VirtualSize         int64
	ActualSize          int64
	BackingFilename     string
	FullBackingFilename string
}

// CloudType 常量（与 service 根包一致，避免循环 import）
const (
	CloudTypeLightweight = "lightweight"
	CloudTypeElastic     = "elastic"
)

// HostStorageRoot 与 service 根包的 hostStorageRoot 一致
const HostStorageRoot = "/var/lib/kvm-storage"

// ── Lifecycle hooks ──
var (
	HookEnsureVMNotMigrating func(vmName, action string) error
	HookStartVM              func(name string) error
	HookShutdownVM           func(name string) error
)

// ── VM XML hooks ──
var (
	HookGetVMInactiveDomainXML func(name string) (string, error)
	HookSetVMInactiveDomainXML func(name, xmlContent string) error
)

// ── Runtime state hook ──
var (
	HookUpdateVMRuntimeState func(name, status string, observedAt time.Time)
)

// ── Disk / validation hooks ──
var (
	HookValidateDiskBackingChain func(diskPath string) error
	HookExtractDomainNVRAMPath   func(xmlContent string) string
	HookListShares               func(vmName string) ([]ShareBrief, error)
	HookQemuInfoChain            func(path string) ([]QemuImgInfo, error)
)

// ── Lightweight cloud hooks ──
var (
	HookIsLightweightCloudVM            func(vmName string) bool
	HookIsLightweightCloudUser          func(username string) bool
	HookGetLightweightVMQuota           func(vmName string) (*model.LightweightVMQuota, error)
	HookCheckLightweightVMSnapshotQuota func(username, vmName string, delta int) error
	HookGetUserVMList                   func(username string) []string
)
