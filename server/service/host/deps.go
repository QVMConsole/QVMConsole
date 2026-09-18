package host

import (
	"context"
	"time"

	"kvm_console/model"
	vmpkg "kvm_console/service/vm"
)

var (
	HookRemoteSSHExec            func(ctx context.Context, node model.HostNode, command string, timeout time.Duration, tolerateRemoteExit bool) (string, error)
	HookCallNodeAPI              func(node model.HostNode, method, path string, body interface{}, out interface{}) ([]byte, error)
	HookShutdownVM               func(name string) error
	HookDestroyVM                func(name string) error
	HookWaitVMShutdownForDisable func(vmName string, timeout time.Duration) bool

	// ── Stats collector hooks (cross-package dependencies) ──
	HookInitializeVMRuntimeTracker                          func()
	HookInitializeUserRuntimeQuotaTracker                   func()
	HookInitializeLightweightRuntimeQuotaTracker            func()
	HookSyncAllUserRuntimeQuotaStatesWithActiveVMs          func(activeVMs map[string]struct{}, observedAt time.Time)
	HookSyncAllLightweightVMRuntimeQuotaStatesWithActiveVMs func(activeVMs map[string]struct{}, observedAt time.Time)
	HookSyncVMRuntimeStatesFromHost                         func(observedAt time.Time)
	HookGetRuntimeActiveVMSetFromHost                       func() (map[string]struct{}, error)
	HookStartTrafficQuotaChecker                            func()
	HookGetHostStats                                        func() (*vmpkg.HostStats, error)
)
