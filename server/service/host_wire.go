package service

import (
	"context"
	"time"

	"kvm_console/model"
	hostpkg "kvm_console/service/host"
	vmpkg "kvm_console/service/vm"
)

// init wires host package function variables to service root implementations.
// This breaks the circular dependency: host package cannot import service,
// so it exposes function variables that we set here.
func init() {
	hostpkg.HookRemoteSSHExec = remoteSSHExec
	hostpkg.HookCallNodeAPI = CallNodeAPI
	hostpkg.HookShutdownVM = ShutdownVM
	hostpkg.HookDestroyVM = DestroyVM
	hostpkg.HookWaitVMShutdownForDisable = waitVMShutdownForDisable
	// ── Stats collector hooks ──
	hostpkg.HookInitializeVMRuntimeTracker = InitializeVMRuntimeTracker
	hostpkg.HookInitializeUserRuntimeQuotaTracker = InitializeUserRuntimeQuotaTracker
	hostpkg.HookInitializeLightweightRuntimeQuotaTracker = InitializeLightweightRuntimeQuotaTracker
	hostpkg.HookSyncAllUserRuntimeQuotaStatesWithActiveVMs = SyncAllUserRuntimeQuotaStatesWithActiveVMs
	hostpkg.HookSyncAllLightweightVMRuntimeQuotaStatesWithActiveVMs = syncAllLightweightVMRuntimeQuotaStatesWithActiveVMs
	hostpkg.HookSyncVMRuntimeStatesFromHost = SyncVMRuntimeStatesFromHost
	hostpkg.HookGetRuntimeActiveVMSetFromHost = getRuntimeActiveVMSetFromHost
	hostpkg.HookStartTrafficQuotaChecker = StartTrafficQuotaChecker
	hostpkg.HookGetHostStats = GetHostStats
}

// ── Type aliases ──

type HostNodeRequest = hostpkg.HostNodeRequest
type HostNodeView = hostpkg.HostNodeView
type HostKSMProfile = hostpkg.HostKSMProfile
type HostKSMStatus = hostpkg.HostKSMStatus
type HostKSMRuntimeConfig = hostpkg.HostKSMRuntimeConfig
type HostKSMMetrics = hostpkg.HostKSMMetrics
type HostZRAMProfile = hostpkg.HostZRAMProfile
type HostZRAMStatus = hostpkg.HostZRAMStatus
type HostZRAMRuntimeConfig = hostpkg.HostZRAMRuntimeConfig
type HostZRAMPersistentConfig = hostpkg.HostZRAMPersistentConfig
type HostDiskInfo = hostpkg.HostDiskInfo
type HostCPUHardware = hostpkg.HostCPUHardware
type HostMemoryModule = hostpkg.HostMemoryModule
type HostMemoryModulesInfo = hostpkg.HostMemoryModulesInfo
type IgpuPassthroughStatus = hostpkg.HardwarePassthroughStatus
type HardwarePassthroughStatus = hostpkg.HardwarePassthroughStatus
type IgpuDeviceInfo = hostpkg.PassthroughDeviceInfo
type PassthroughDeviceInfo = hostpkg.PassthroughDeviceInfo
type IommuEnableResult = hostpkg.IommuEnableResult
type VfioLoadResult = hostpkg.VfioLoadResult
type MaintenanceModeTaskParams = hostpkg.MaintenanceModeTaskParams
type MaintenanceModeTaskResult = hostpkg.MaintenanceModeTaskResult

// ── Exported delegates (used by handler and other packages) ──

func ListHostNodes() ([]hostpkg.HostNodeView, error) {
	return hostpkg.ListHostNodes()
}

func GetHostNode(id uint) (*model.HostNode, error) {
	return hostpkg.GetHostNode(id)
}

func CreateHostNode(req hostpkg.HostNodeRequest) (*hostpkg.HostNodeView, error) {
	return hostpkg.CreateHostNode(req)
}

func UpdateHostNode(id uint, req hostpkg.HostNodeRequest) (*hostpkg.HostNodeView, error) {
	return hostpkg.UpdateHostNode(id, req)
}

func DeleteHostNode(id uint) error {
	return hostpkg.DeleteHostNode(id)
}

func ProbeHostNode(id uint) (*hostpkg.HostNodeView, error) {
	return hostpkg.ProbeHostNode(id)
}

func BuildHostNodeView(node model.HostNode) hostpkg.HostNodeView {
	return hostpkg.BuildHostNodeView(node)
}

func GetHostKSMProfiles() []hostpkg.HostKSMProfile {
	return hostpkg.GetHostKSMProfiles()
}

func GetHostKSMStatus() *hostpkg.HostKSMStatus {
	return hostpkg.GetHostKSMStatus()
}

func SetHostKSMProfile(profileKey string) (*hostpkg.HostKSMStatus, error) {
	return hostpkg.SetHostKSMProfile(profileKey)
}

func GetHostZRAMProfiles() []hostpkg.HostZRAMProfile {
	return hostpkg.GetHostZRAMProfiles()
}

func ApplyHostZRAMPersistentProfile() error {
	return hostpkg.ApplyHostZRAMPersistentProfile()
}

func GetHostZRAMStatus() *hostpkg.HostZRAMStatus {
	return hostpkg.GetHostZRAMStatus()
}

func SetHostZRAMProfile(profileKey string) (*hostpkg.HostZRAMStatus, error) {
	return hostpkg.SetHostZRAMProfile(profileKey)
}

func GetHostDiskInfos() ([]hostpkg.HostDiskInfo, error) {
	return hostpkg.GetHostDiskInfos()
}

func GetHostCPUHardware() *hostpkg.HostCPUHardware {
	return hostpkg.GetHostCPUHardware()
}

func GetHostMemoryModules() *hostpkg.HostMemoryModulesInfo {
	return hostpkg.GetHostMemoryModules()
}

func IsMaintenanceModeEnabled() bool {
	return hostpkg.IsMaintenanceModeEnabled()
}

func EnsureMaintenanceModeDisabled(action string) error {
	return hostpkg.EnsureMaintenanceModeDisabled(action)
}

func EnterMaintenanceMode(ctx context.Context, params *hostpkg.MaintenanceModeTaskParams, progressFn func(int, string)) (*hostpkg.MaintenanceModeTaskResult, error) {
	return hostpkg.EnterMaintenanceMode(ctx, params, progressFn)
}

func ExitMaintenanceMode(ctx context.Context, params *hostpkg.MaintenanceModeTaskParams, progressFn func(int, string)) (*hostpkg.MaintenanceModeTaskResult, error) {
	return hostpkg.ExitMaintenanceMode(ctx, params, progressFn)
}

func ParseMaintenanceServiceUnits(raw string) []string {
	return hostpkg.ParseMaintenanceServiceUnits(raw)
}

func IsMaintenanceModeError(err error) bool {
	return hostpkg.IsMaintenanceModeError(err)
}

func IsLibvirtUnavailableError(err error) bool {
	return hostpkg.IsLibvirtUnavailableError(err)
}

func FirstNonEmpty(values ...string) string {
	return hostpkg.FirstNonEmpty(values...)
}

// ── Unexported delegates (used by other service files within service root) ──

func decryptHostNodeSSHPassword(node model.HostNode) (string, error) {
	return hostpkg.DecryptHostNodeSSHPassword(node)
}

func decryptHostNodeAPIKey(node model.HostNode) (string, error) {
	return hostpkg.DecryptHostNodeAPIKey(node)
}

func encryptNodeSecret(plainText string) (string, error) {
	return hostpkg.EncryptNodeSecret(plainText)
}

func decryptNodeSecret(cipherText string) (string, error) {
	return hostpkg.DecryptNodeSecret(cipherText)
}

func buildNodeSecretKey() []byte {
	return hostpkg.BuildNodeSecretKey()
}

func collectHostDiskIOBytes() (int64, int64, error) {
	return hostpkg.CollectHostDiskIOBytes()
}

func isLibvirtUnavailableText(text string) bool {
	return hostpkg.IsLibvirtUnavailableText(text)
}

// ── Stats collector delegates ──

func StartStatsCollector() {
	hostpkg.StartStatsCollector()
}

func GetCachedStats(name string) *vmpkg.VmStats {
	return hostpkg.GetCachedStats(name)
}

func GetAllCachedStats() map[string]*vmpkg.VmStats {
	return hostpkg.GetAllCachedStats()
}

func DeleteVMStatsRecords(name string) {
	hostpkg.DeleteVMStatsRecords(name)
}

func QueryVMStatsHistory(name string, start, end time.Time) ([]model.VmStatsRecord, error) {
	return hostpkg.QueryVMStatsHistory(name, start, end)
}

func QueryHostStatsHistory(start, end time.Time) ([]model.HostStatsRecord, error) {
	return hostpkg.QueryHostStatsHistory(start, end)
}

// ── Network wait-online ──

func SetNetworkWaitOnlineDisabled(disabled bool) error {
	return hostpkg.SetNetworkWaitOnlineDisabled(disabled)
}

// ── iGPU passthrough ──

func GetIgpuPassthroughStatus() *hostpkg.HardwarePassthroughStatus {
	return hostpkg.GetHardwarePassthroughStatus()
}

func GetHardwarePassthroughStatus() *hostpkg.HardwarePassthroughStatus {
	return hostpkg.GetHardwarePassthroughStatus()
}

func EnableIommuInGrub() *hostpkg.IommuEnableResult {
	return hostpkg.EnableIommuInGrub()
}

func LoadVfioPciModule() *hostpkg.VfioLoadResult {
	return hostpkg.LoadVfioPciModule()
}
