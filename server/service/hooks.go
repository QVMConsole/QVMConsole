package service

// hooks.go - 子包通过 init() 注册实现函数，避免循环 import
// 根包通过这些变量调用子包函数（当子包函数尚未迁移时为 nil，
// 迁移后由子包 init() 赋值）。

// VPC Hooks - 由 service/network/vpc 子包注册
var (
	HookApplyVPCBindingRuntime func(vmName string) error
)

// Migration Hooks - 由 service/vm/migration 子包注册
var (
	HookEnsureVMNotMigrating         func(vmName, action string) error
	HookApplyVMUnderMigrationStatus  func(vm *VmInfo)
	HookDetectMigrationModeFromState func(state string) string
	HookMigrationModeLive            string
)

// Snapshot Hooks - 由 service/snapshot 子包注册
var (
	HookEnsureVMNotSnapshotting func(vmName, action string) error
)

// EnsureVMNotSnapshotting delegates to HookEnsureVMNotSnapshotting for handler layer compatibility
func EnsureVMNotSnapshotting(vmName, action string) error {
	if HookEnsureVMNotSnapshotting != nil {
		return HookEnsureVMNotSnapshotting(vmName, action)
	}
	return nil
}

// maxInt 返回两个 int 中的较大值。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// EnsureVMNotMigrating delegates to HookEnsureVMNotMigrating for handler layer compatibility
func EnsureVMNotMigrating(vmName, action string) error {
	if HookEnsureVMNotMigrating != nil {
		return HookEnsureVMNotMigrating(vmName, action)
	}
	return nil
}

// ApplyVMUnderMigrationStatus delegates to HookApplyVMUnderMigrationStatus.
// Uses deferred resolution to work around init-order: the sub-package sets the
// hook after vm_register.go captures the Deps, so we resolve at call time.
func ApplyVMUnderMigrationStatus(vm *VmInfo) {
	if HookApplyVMUnderMigrationStatus != nil {
		HookApplyVMUnderMigrationStatus(vm)
	}
}

// DetectMigrationModeFromState delegates to HookDetectMigrationModeFromState.
// Uses deferred resolution for the same init-order reason.
func DetectMigrationModeFromState(state string) string {
	if HookDetectMigrationModeFromState != nil {
		return HookDetectMigrationModeFromState(state)
	}
	return ""
}

// LiveMigrationMode returns the live migration mode constant.
// Uses deferred resolution for the same init-order reason as the hooks above.
func LiveMigrationMode() string {
	if HookMigrationModeLive != "" {
		return HookMigrationModeLive
	}
	return "live" // fallback to the known constant
}
