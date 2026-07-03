package service

// VM function delegates - forward to service/vm subpackage
// Maintains backward compatibility for callers using service.XXX()
import (
	"context"
	"time"

	"kvm_console/model"
	"kvm_console/service/libvirt_rpc"
	vmpkg "kvm_console/service/vm"
	"kvm_console/service/vm_xml"
)

// ── Lifecycle ──

func StartVM(name string) error {
	return vmpkg.StartVM(name)
}

func StartVMPreserveRebootAction(name string) error {
	return vmpkg.StartVMPreserveRebootAction(name)
}

func ShutdownVM(name string) error {
	return vmpkg.ShutdownVM(name)
}

func DestroyVM(name string) error {
	return vmpkg.DestroyVM(name)
}

func RebootVM(name string) error {
	return vmpkg.RebootVM(name)
}

func ResetVM(name string) error {
	return vmpkg.ResetVM(name)
}

func FixOnReboot(name string) {
	vmpkg.FixOnReboot(name)
}

// ── List / Detail ──

func ListVMs(options ...VMListOptions) ([]VmInfo, error) {
	opts := make([]vmpkg.VMListOptions, len(options))
	for i, o := range options {
		opts[i] = vmpkg.VMListOptions(o)
	}
	return vmpkg.ListVMs(opts...)
}

func GetVMIPInfo(name string) (string, string, error) {
	return vmpkg.GetVMIPInfo(name)
}

func GetVM(name string) (*VmDetail, error) {
	return vmpkg.GetVM(name)
}

// ── Create ──

func CreateVM(params *CreateVMParams, progressFn func(int, string)) (string, error) {
	return vmpkg.CreateVM((*vmpkg.CreateVMParams)(params), progressFn)
}

func ParseCreateVMParams(jsonStr string) (*CreateVMParams, error) {
	return vmpkg.ParseCreateVMParams(jsonStr)
}

func ListOSVariants() ([]OSVariantInfo, error) {
	return vmpkg.ListOSVariants()
}

func ListISOs() ([]map[string]string, error) {
	return vmpkg.ListISOs()
}

func CheckHostMemory(requiredMB int) error {
	return vmpkg.CheckHostMemory(requiredMB)
}

// ── Config ──

func EditVMConfig(name string, vcpu, maxVCPU, memoryMB int) error {
	return vmpkg.EditVMConfig(name, vcpu, maxVCPU, memoryMB)
}

func SetVMAutostart(name string, autostart bool) error {
	return vmpkg.SetVMAutostart(name, autostart)
}

func SetVMBootOrder(name string, bootOrder []string) error {
	return vmpkg.SetVMBootOrder(name, bootOrder)
}

func ReorderVMDevices(name string, deviceOrder []string) error {
	return vmpkg.ReorderVMDevices(name, deviceOrder)
}

func SetVMNicModel(name, nicModel string) error {
	return vmpkg.SetVMNicModel(name, nicModel)
}

// ── Config metadata ──

func GetVMRemark(name string) (string, error) {
	return vmpkg.GetVMRemark(name)
}

func SetVMRemark(name, remark string) error {
	return vmpkg.SetVMRemark(name, remark)
}

func GetVMGroup(name string) (string, error) {
	return vmpkg.GetVMGroup(name)
}

func SetVMGroup(name, group string) error {
	return vmpkg.SetVMGroup(name, group)
}

// ── Freeze ──

func GetVMFreeze(name string) (bool, error) {
	return vmpkg.GetVMFreeze(name)
}

func SetVMFreeze(name string, freeze bool) error {
	return vmpkg.SetVMFreeze(name, freeze)
}

// ── Cache ──

func BootstrapVMCacheFromHost() error {
	return vmpkg.BootstrapVMCacheFromHost()
}

func TriggerAdminVMCacheRefreshIfNeeded() {
	vmpkg.TriggerAdminVMCacheRefreshIfNeeded()
}

func ListCachedVMs(options ...VMListOptions) ([]VmInfo, error) {
	opts := make([]vmpkg.VMListOptions, len(options))
	for i, o := range options {
		opts[i] = vmpkg.VMListOptions(o)
	}
	return vmpkg.ListCachedVMs(opts...)
}

func ListCachedVMsByOwner(username string, options ...VMListOptions) ([]VmInfo, error) {
	opts := make([]vmpkg.VMListOptions, len(options))
	for i, o := range options {
		opts[i] = vmpkg.VMListOptions(o)
	}
	return vmpkg.ListCachedVMsByOwner(username, opts...)
}

func SyncVMCacheFromHost() error {
	return vmpkg.SyncVMCacheFromHost()
}

func RefreshVMCacheByName(name string) error {
	return vmpkg.RefreshVMCacheByName(name)
}

func MarkVMCacheMissing(name string) error {
	return vmpkg.MarkVMCacheMissing(name)
}

func RefreshVMCacheByNameAsync(name string) {
	vmpkg.RefreshVMCacheByNameAsync(name)
}

func MarkVMCacheMissingAsync(name string) {
	vmpkg.MarkVMCacheMissingAsync(name)
}

func UpdateVMCacheOwner(name, owner string) {
	vmpkg.UpdateVMCacheOwner(name, owner)
}

func SyncVMCacheOwner(name string) {
	vmpkg.SyncVMCacheOwner(name)
}

func SyncVMCacheOwnersForAssignment(username string, assignedVMs []string) {
	vmpkg.SyncVMCacheOwnersForAssignment(username, assignedVMs)
}

// ── Credential ──

func SaveVMCredential(vmName, username, password, source, operator string, markReset bool) error {
	return vmpkg.SaveVMCredential(vmName, username, password, source, operator, markReset)
}

func GetVMCredential(vmName string) (*VMCredentialInfo, error) {
	return vmpkg.GetVMCredential(vmName)
}

func DeleteVMCredential(vmName string) error {
	return vmpkg.DeleteVMCredential(vmName)
}

// ── Runtime ──

func InitializeVMRuntimeTracker() {
	vmpkg.InitializeVMRuntimeTracker()
}

func SyncVMRuntimeStatesFromHost(observedAt time.Time) {
	vmpkg.SyncVMRuntimeStatesFromHost(observedAt)
}

func UpdateVMRuntimeState(name, status string, observedAt time.Time) {
	vmpkg.UpdateVMRuntimeState(name, status, observedAt)
}

func ResetVMContinuousRuntime(name string, observedAt time.Time) {
	vmpkg.ResetVMContinuousRuntime(name, observedAt)
}

func DeleteVMRuntimeRecord(name string) {
	vmpkg.DeleteVMRuntimeRecord(name)
}

func GetVMRuntimeInfo(name, status string) VMRuntimeInfo {
	return vmpkg.GetVMRuntimeInfo(name, status)
}

// ── Stats ──

func GetVMStats(name string) (*VmStats, error) {
	return vmpkg.GetVMStats(name)
}

func GetHostStats() (*HostStats, error) {
	return vmpkg.GetHostStats()
}

// ── Interface ──

func CountVMInterfaces(vmName string) int {
	return vmpkg.CountVMInterfaces(vmName)
}

func GetVMMACByOrder(vmName string, order int) string {
	return vmpkg.GetVMMACByOrder(vmName, order)
}

func UpsertVMBindingNicModel(vmName string, interfaceOrder int, nicModel string) error {
	return vmpkg.UpsertVMBindingNicModel(vmName, interfaceOrder, nicModel)
}

func GetNextVMBindingOrder(vmName string) int {
	return vmpkg.GetNextVMBindingOrder(vmName)
}

// ── XML ──

func ValidateVMInactiveDomainXML(name, xmlContent string) error {
	return vmpkg.ValidateVMInactiveDomainXML(name, xmlContent)
}

func GetVMInactiveDomainXML(name string) (string, error) {
	return vmpkg.GetVMInactiveDomainXML(name)
}

func SetVMInactiveDomainXML(name, xmlContent string) error {
	return vmpkg.SetVMInactiveDomainXML(name, xmlContent)
}

// ── Display ──

func SetVMVideoModel(name, videoModel string) error {
	return vmpkg.SetVMVideoModel(name, videoModel)
}

// ── Boot type ──

func SetVMBootType(name, bootType string) error {
	return vmpkg.SetVMBootType(name, bootType)
}

// SetVMFirmwareCompat 设置虚拟机 UEFI 固件兼容模式（ARM 专用）。
func SetVMFirmwareCompat(name string, enabled bool) error {
	return vmpkg.SetVMFirmwareCompat(name, enabled)
}

// SetVMDirectBoot 设置虚拟机直接内核引导配置。
func SetVMDirectBoot(name string, cfg *vm_xml.DirectBootConfig) error {
	return vmpkg.SetVMDirectBoot(name, cfg)
}

// ── Clock / RTC ──

func NormalizeRTCOffset(offset string) string {
	return vmpkg.NormalizeRTCOffset(offset)
}

func DefaultRTCOffsetForGuestType(guestType string) string {
	return vmpkg.DefaultRTCOffsetForGuestType(guestType)
}

func ResolveRTCOffset(offset, guestType string) string {
	return vmpkg.ResolveRTCOffset(offset, guestType)
}

func NormalizeRTCStartDate(startDate string) string {
	return vmpkg.NormalizeRTCStartDate(startDate)
}

func ParseRTCOffsetFromDomainXML(xmlContent string) string {
	return vmpkg.ParseRTCOffsetFromDomainXML(xmlContent)
}

func ParseRTCStartDateFromDomainXML(xmlContent string) string {
	return vmpkg.ParseRTCStartDateFromDomainXML(xmlContent)
}

func ParseRTCStartDateToEpoch(startDate string) (string, error) {
	return vmpkg.ParseRTCStartDateToEpoch(startDate)
}

func ApplyRTCConfigToDomainXML(xmlContent, offset, startDate, guestType string) (string, error) {
	return vmpkg.ApplyRTCConfigToDomainXML(xmlContent, offset, startDate, guestType)
}

func SetVMRTCConfig(name, offset, startDate string) error {
	return vmpkg.SetVMRTCConfig(name, offset, startDate)
}

// ── APIC ──

func ResolveVMAPICEnabled(enabled *bool) bool {
	return vmpkg.ResolveVMAPICEnabled(enabled)
}

func ParseVMAPICFromDomainXML(xmlContent string) bool {
	return vmpkg.ParseVMAPICFromDomainXML(xmlContent)
}

func ApplyVMAPICToDomainXML(xmlContent string, enabled *bool) (string, error) {
	return vmpkg.ApplyVMAPICToDomainXML(xmlContent, enabled)
}

func SetVMAPICConfig(name string, enabled bool) error {
	return vmpkg.SetVMAPICConfig(name, enabled)
}

// ── PAE ──

func SetVMPAEConfig(name string, enabled bool) error {
	return vmpkg.SetVMPAEConfig(name, enabled)
}

// ── SMBIOS ──

func SetVMSMBIOS1Config(name string, cfg *vm_xml.VMSMBIOS1Config) error {
	return vmpkg.SetVMSMBIOS1Config(name, cfg)
}

// ── CPU affinity ──

func ParseCPUAffinity(input string) ([]int, error) {
	return vmpkg.ParseCPUAffinity(input)
}

func GetSystemCPUCores() (int, error) {
	return vmpkg.GetSystemCPUCores()
}

func ValidateCPUAffinity(cores []int) error {
	return vmpkg.ValidateCPUAffinity(cores)
}

func FormatCPUAffinity(cores []int) string {
	return vmpkg.FormatCPUAffinity(cores)
}

func ParseCPUAffinityFromDomainXML(xmlStr string) string {
	return vmpkg.ParseCPUAffinityFromDomainXML(xmlStr)
}

func ApplyCPUAffinityToDomainXML(xmlStr string, vcpu int, cores []int) string {
	return vmpkg.ApplyCPUAffinityToDomainXML(xmlStr, vcpu, cores)
}

func SetVMCPUAffinity(name string, coresStr string) error {
	return vmpkg.SetVMCPUAffinity(name, coresStr)
}

func ApplyCPUAffinityIfSet(xmlStr string, vcpu int, cpuAffinity string) (string, error) {
	return vmpkg.ApplyCPUAffinityIfSet(xmlStr, vcpu, cpuAffinity)
}

func GetCPUAffinityPresets() []CPUAffinityPreset {
	return vmpkg.GetCPUAffinityPresets()
}

func SaveCPUAffinityPresets(presets []CPUAffinityPreset) error {
	return vmpkg.SaveCPUAffinityPresets(presets)
}

// ── CPU limit ──

func NormalizeVMCPULimitPercent(percent int) int {
	return vmpkg.NormalizeVMCPULimitPercent(percent)
}

func ValidateVMCPULimitPercent(percent int) error {
	return vmpkg.ValidateVMCPULimitPercent(percent)
}

func ParseVMCPULimitPercentFromDomainXML(xmlStr string, vcpu int) int {
	return vmpkg.ParseVMCPULimitPercentFromDomainXML(xmlStr, vcpu)
}

func ApplyVMCPULimitToDomainXML(xmlStr string, vcpu, percent int) string {
	return vmpkg.ApplyVMCPULimitToDomainXML(xmlStr, vcpu, percent)
}

func SetVMCPULimitPercent(name string, vcpu, percent int) error {
	return vmpkg.SetVMCPULimitPercent(name, vcpu, percent)
}

// ── CPU topology ──

func NormalizeVMCPUTopologyMode(mode string) string {
	return vmpkg.NormalizeVMCPUTopologyMode(mode)
}

func ApplyCPUTopologyModeToDomainXML(xmlStr, mode, osType string, vcpu int) string {
	return vmpkg.ApplyCPUTopologyModeToDomainXML(xmlStr, mode, osType, vcpu)
}

func RemoveCPUTopologyFromDomainXML(xmlStr string) string {
	return vmpkg.RemoveCPUTopologyFromDomainXML(xmlStr)
}

func ParseVMCPUTopologyModeFromDomainXML(xmlStr string) string {
	return vmpkg.ParseVMCPUTopologyModeFromDomainXML(xmlStr)
}

func ApplyWindowsCPUTopologyToDomainXML(xmlStr string, vcpu int) string {
	return vmpkg.ApplyWindowsCPUTopologyToDomainXML(xmlStr, vcpu)
}

func ParseVCPUCountFromDomainXML(xmlStr string) int {
	return vmpkg.ParseVCPUCountFromDomainXML(xmlStr)
}

func ParseMaxVCPUFromDomainXML(xmlStr string) int {
	return vmpkg.ParseMaxVCPUFromDomainXML(xmlStr)
}

func BuildVCPUTag(current, maxVCPU int) string {
	return vmpkg.BuildVCPUTag(current, maxVCPU)
}

func EffectiveTopologyVCPU(current, maxVCPU int) int {
	return vmpkg.EffectiveTopologyVCPU(current, maxVCPU)
}

func SetVMCPUTopologyMode(name, mode string) error {
	return vmpkg.SetVMCPUTopologyMode(name, mode)
}

// ── Passthrough ──

func ListPCIDevicesForPassthrough() ([]PCIDevice, error) {
	return vmpkg.ListPCIDevicesForPassthrough()
}

func GetVMPCIDevices(vmName string) ([]PCIDevice, error) {
	return vmpkg.GetVMPCIDevices(vmName)
}

func AttachPCIDeviceToVM(vmName, pciAddress string) error {
	return vmpkg.AttachPCIDeviceToVM(vmName, pciAddress)
}

func DetachPCIDeviceFromVM(vmName, pciAddress string) error {
	return vmpkg.DetachPCIDeviceFromVM(vmName, pciAddress)
}

func BindPCIDeviceToVfio(pciAddress string) error {
	return vmpkg.BindPCIDeviceToVfio(pciAddress)
}

func UnbindPCIDeviceFromVfio(pciAddress string) error {
	return vmpkg.UnbindPCIDeviceFromVfio(pciAddress)
}

func ValidatePCIPassthrough(pciAddress string) error {
	return vmpkg.ValidatePCIPassthrough(pciAddress)
}

func GenerateHostDevXML(pciAddress string) (string, error) {
	return vmpkg.GenerateHostDevXML(pciAddress)
}

func ApplyHostDevsToDomainXML(xmlContent string, hostDevs []HostDeviceParam) (string, error) {
	hd := make([]vmpkg.HostDeviceParam, len(hostDevs))
	for i, h := range hostDevs {
		hd[i] = vmpkg.HostDeviceParam(h)
	}
	return vmpkg.ApplyHostDevsToDomainXML(xmlContent, hd)
}

func EnsureVfioModuleLoaded() error {
	return vmpkg.EnsureVfioModuleLoaded()
}

func IsDeviceVfioBound(pciAddress string) bool {
	return vmpkg.IsDeviceVfioBound(pciAddress)
}

// ── Guest agent ──

func SetVMGuestAgentConfig(name string, cfg *vm_xml.VMGuestAgentConfig) error {
	return vmpkg.SetVMGuestAgentConfig(name, cfg)
}

// ── Install media ──

func NormalizeInstallISOSelection(primary string, paths []string) (string, []string) {
	return vmpkg.NormalizeInstallISOSelection(primary, paths)
}

func ApplyAdditionalCDROMsToDomainXML(xmlContent string, isoPaths []string) (string, error) {
	return vmpkg.ApplyAdditionalCDROMsToDomainXML(xmlContent, isoPaths)
}

// ── Disk migration ──

func ParseVMDiskMigrationTaskParams(raw string) (VMDiskMigrationTaskParams, error) {
	return vmpkg.ParseVMDiskMigrationTaskParams(raw)
}

func GetVMDiskMigrationOptions(vmName string) (*VMDiskMigrationOptions, error) {
	return vmpkg.GetVMDiskMigrationOptions(vmName)
}

func ExecuteVMDiskMigration(ctx context.Context, params VMDiskMigrationTaskParams, progress func(int, string)) (map[string]string, error) {
	return vmpkg.ExecuteVMDiskMigration(ctx, params, progress)
}

// ── First boot ──

func NormalizeVMFirstBootRebootMode(mode string) string {
	return vmpkg.NormalizeVMFirstBootRebootMode(mode)
}

func ShouldUseWindowsFirstBootColdReboot(mode, osType string) bool {
	return vmpkg.ShouldUseWindowsFirstBootColdReboot(mode, osType)
}

func ApplyFirstBootRebootModeToDomainXML(xmlStr, mode string) string {
	return vmpkg.ApplyFirstBootRebootModeToDomainXML(xmlStr, mode)
}

func CompleteWindowsFirstBootColdReboot(ctx context.Context, name string, progressFn func(int, string)) error {
	return vmpkg.CompleteWindowsFirstBootColdReboot(ctx, name, progressFn)
}

// ── Password reset ──

func ParseResetLinuxPasswordParams(jsonStr string) (*ResetLinuxPasswordParams, error) {
	return vmpkg.ParseResetLinuxPasswordParams(jsonStr)
}

func ValidateResetGuestPasswordParams(username, password, guestType string) error {
	return vmpkg.ValidateResetGuestPasswordParams(username, password, guestType)
}

func ValidateResetLinuxPasswordParams(username, password string) error {
	return vmpkg.ValidateResetLinuxPasswordParams(username, password)
}

func ResetLinuxPassword(ctx context.Context, params *ResetLinuxPasswordParams, progressFn func(int, string)) error {
	return vmpkg.ResetLinuxPassword(ctx, (*vmpkg.ResetLinuxPasswordParams)(params), progressFn)
}

func SubmitResetLinuxPasswordTask(params *ResetLinuxPasswordParams, operator string) (*model.Task, error) {
	return vmpkg.SubmitResetLinuxPasswordTask((*vmpkg.ResetLinuxPasswordParams)(params), operator)
}

// ── VM name ──

func ValidateVMName(name string) error {
	return vmpkg.ValidateVMName(name)
}

func ValidateVMNamePrefix(prefix string) error {
	return vmpkg.ValidateVMNamePrefix(prefix)
}

func GenerateRandomVMName() string {
	return vmpkg.GenerateRandomVMName()
}

// ── VM lock ──

func LockVM(vmName, username string) error {
	return vmpkg.LockVM(vmName, username)
}

func UnlockVM(vmName, username string) error {
	return vmpkg.UnlockVM(vmName, username)
}

func IsVMLocked(vmName string) bool {
	return vmpkg.IsVMLocked(vmName)
}

func GetVMLockInfo(vmName string) *model.VMLock {
	return vmpkg.GetVMLockInfo(vmName)
}

// ── Monitor ──

func GetVMMonitorStatus(name string) (*VMMonitorStatus, error) {
	return vmpkg.GetVMMonitorStatus(name)
}

func ExecuteVMMonitorCommand(name, command string) (*VMMonitorCommandResult, error) {
	return vmpkg.ExecuteVMMonitorCommand(name, command)
}

// ── Schedule ──

func ListVMSchedules(vmName string) ([]VMScheduleItem, error) {
	return vmpkg.ListVMSchedules(vmName)
}

func CreateVMSchedule(vmName, createdBy string, input VMScheduleInput) (*VMScheduleItem, error) {
	return vmpkg.CreateVMSchedule(vmName, createdBy, vmpkg.VMScheduleInput(input))
}

func UpdateVMSchedule(vmName string, id uint, input VMScheduleInput) (*VMScheduleItem, error) {
	return vmpkg.UpdateVMSchedule(vmName, id, vmpkg.VMScheduleInput(input))
}

func DeleteVMSchedule(vmName string, id uint) error {
	return vmpkg.DeleteVMSchedule(vmName, id)
}

func DeleteVMSchedules(vmName string) error {
	return vmpkg.DeleteVMSchedules(vmName)
}

func StartVMScheduleRunner() {
	vmpkg.StartVMScheduleRunner()
}

func RunVMScheduledAction(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
	return vmpkg.RunVMScheduledAction(ctx, task, progress)
}

// ── Export ──

func GetVMExportSize(vmName string) (int64, error) {
	return vmpkg.GetVMExportSize(vmName)
}

func ExportVM(ctx context.Context, params *ExportVMParams, progressFn func(int, string)) (*ExportVMResult, error) {
	return vmpkg.ExportVM(ctx, (*vmpkg.ExportVMParams)(params), progressFn)
}

// ── Helpers ──

func DomainExists(vmName string) bool {
	return vmpkg.DomainExists(vmName)
}

func DomainExistsRPC(vmName string) (bool, error) {
	return libvirt_rpc.DomainExistsRPC(vmName)
}

func GetDomainState(vmName string) string {
	return vmpkg.GetDomainState(vmName)
}

func QemuInfoChain(path string) ([]QemuImgInfo, error) {
	return vmpkg.QemuInfoChain(path)
}

// ── Constants re-exported ──

const (
	VMRTCOffsetUTC       = vmpkg.VMRTCOffsetUTC
	VMRTCOffsetLocaltime = vmpkg.VMRTCOffsetLocaltime
	VMRTCOffsetAbsolute  = vmpkg.VMRTCOffsetAbsolute
	VMRTCStartDateNow    = vmpkg.VMRTCStartDateNow
)

const (
	VMCPUTopologyAuto         = vmpkg.VMCPUTopologyAuto
	VMCPUTopologySingleSocket = vmpkg.VMCPUTopologySingleSocket
	VMCPUTopologyHostDefault  = vmpkg.VMCPUTopologyHostDefault
)

const (
	VMCPULimitUnlimited = vmpkg.VMCPULimitUnlimited
)

const (
	VMFirstBootRebootNormal = vmpkg.VMFirstBootRebootNormal
	VMFirstBootRebootCold   = vmpkg.VMFirstBootRebootCold
)

const (
	VMScheduleEventTypePower     = vmpkg.VMScheduleEventTypePower
	VMScheduleEventTypeLifecycle = vmpkg.VMScheduleEventTypeLifecycle
	VMScheduleActionStart        = vmpkg.VMScheduleActionStart
	VMScheduleActionShutdown     = vmpkg.VMScheduleActionShutdown
	VMScheduleActionDelete       = vmpkg.VMScheduleActionDelete
	VMScheduleTypeOnce           = vmpkg.VMScheduleTypeOnce
	VMScheduleTypeDaily          = vmpkg.VMScheduleTypeDaily
	VMScheduleTypeWeekly         = vmpkg.VMScheduleTypeWeekly
	VMScheduleExecStatusPending  = vmpkg.VMScheduleExecStatusPending
	VMScheduleExecStatusRunning  = vmpkg.VMScheduleExecStatusRunning
	VMScheduleExecStatusSuccess  = vmpkg.VMScheduleExecStatusSuccess
	VMScheduleExecStatusFailed   = vmpkg.VMScheduleExecStatusFailed
)

// ── VM internal helpers (exported from vm package for cross-package use) ──

func GetVMDiskInfo(name string) DiskInfoResult {
	return vmpkg.GetVMDiskInfo(name)
}

func GetVMNetworkInfo(name string) NetInfoResult {
	return vmpkg.GetVMNetworkInfo(name)
}

func WaitForVMShutOff(ctx context.Context, name string, timeout time.Duration) (bool, error) {
	return vmpkg.WaitForVMShutOff(ctx, name, timeout)
}

func InjectPCIERootPorts(xmlContent string, portCount int) string {
	return vmpkg.InjectPCIERootPorts(xmlContent, portCount)
}

func CopyDiskFileSparse(ctx context.Context, sourcePath, targetPath string) error {
	return vmpkg.CopyDiskFileSparse(ctx, sourcePath, targetPath)
}

func SetLibvirtDiskFileOwner(path string) error {
	return vmpkg.SetLibvirtDiskFileOwner(path)
}

func UpdateInactiveDomainDiskPath(vmName, sourcePath, targetPath string) error {
	return vmpkg.UpdateInactiveDomainDiskPath(vmName, sourcePath, targetPath)
}

func SameCleanPath(a, b string) bool {
	return vmpkg.SameCleanPath(a, b)
}

func DetectVMOSType(templateName, xmlStr string) string {
	return vmpkg.DetectVMOSType(templateName, xmlStr)
}

func AttachVMInterface(vmName string, sw model.VPCSwitch, nicModel string, interfaceOrder int) error {
	return vmpkg.AttachVMInterface(vmName, sw, nicModel, interfaceOrder)
}

func DetachVMInterface(vmName string, interfaceOrder int) error {
	return vmpkg.DetachVMInterface(vmName, interfaceOrder)
}
