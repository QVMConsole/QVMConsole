package migration

import "kvm_console/service"

func init() {
	service.HookEnsureVMNotMigrating = EnsureVMNotMigrating
	service.HookApplyVMUnderMigrationStatus = ApplyVMUnderMigrationStatus
	service.HookDetectMigrationModeFromState = DetectMigrationModeFromState
	service.HookMigrationModeLive = MigrationModeLive
}
