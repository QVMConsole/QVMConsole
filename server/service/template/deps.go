package template

import (
	"context"
)

// deps.go - Hook variables for external dependencies.
// The service root package sets these in template_register.go init()
// to avoid circular imports.

var (
	// ── Clone (circular dependency) ──
	// HookDeleteVM deletes a VM, injected from clone package to avoid cycle.
	HookDeleteVM func(vmName string) error
	// HookFinalizeDeletedVM 清理移动制作模板后已删除 VM 的凭据、锁和缓存记录。
	HookFinalizeDeletedVM func(vmName string)

	// ── VM helpers (private in service root) ──
	HookGetVMDiskInfo      func(vmName string) VMDiskBrief
	HookGetVMNetworkInfo   func(vmName string) VMNetBrief
	HookGetDomainState     func(vmName string) string
	HookCopyDiskFileSparse func(ctx context.Context, sourcePath, targetPath string) error

	// ── VM normalization (still in service root) ──
	HookNormalizeVMDiskBus                  func(value string) string
	HookNormalizeVMNicModel                 func(value string) string
	HookNormalizeVMCPUTopologyMode          func(mode string) string
	HookNormalizeVMFirstBootRebootMode      func(mode string) string
	HookParseVMCPUTopologyModeFromDomainXML func(xmlStr string) string

	// ── Disk / snapshot helpers ──
	HookListDisks                    func(vmName string) ([]DiskBrief, error)
	HookQemuInfoChain                func(path string) ([]QemuChainEntry, error)
	HookSetLibvirtDiskFileOwner      func(path string) error
	HookUpdateInactiveDomainDiskPath func(vmName, sourcePath, targetPath string) error
	HookSameCleanPath                func(a, b string) bool
	HookCheckVMSnapshotSafety        func(vmName string) (bool, []string, error)

	// ── User management ──
	HookFindVMOwner            func(vmName string) string
	HookRemoveVMFromUser       func(username, vmName string) error
	HookRebalanceUserBandwidth func(username string) error

	// ── Utility ──
	HookFormatBytesPublic func(bytes int64) string
	HookFirstNonEmpty     func(values ...string) string
)
