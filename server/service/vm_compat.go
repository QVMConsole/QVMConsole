package service

// VM compatibility types - type aliases to service/vm subpackage
// Maintains backward compatibility for callers using service.XXX types
import (
	vmpkg "kvm_console/service/vm"
	"kvm_console/service/vm_xml"
)

// ── Core VM types ──

type VmInfo = vmpkg.VmInfo
type VmDetail = vmpkg.VmDetail
type VmStats = vmpkg.VmStats
type BootDevice = vmpkg.BootDevice
type HostStats = vmpkg.HostStats
type VMListOptions = vmpkg.VMListOptions
type VMCredentialInfo = vmpkg.VMCredentialInfo
type VMRuntimeInfo = vmpkg.VMRuntimeInfo

// ── Create / config types ──

type CreateVMParams = vmpkg.CreateVMParams
type OSVariantInfo = vmpkg.OSVariantInfo
type CPUAffinityPreset = vmpkg.CPUAffinityPreset
type DirectBootConfig = vm_xml.DirectBootConfig

// ── Passthrough types ──

type PCIDevice = vmpkg.PCIDevice

// Note: HostDeviceParam is defined in vm/deps.go as a mirror type;
// aliased here for handler compatibility (handler/vm_create.go, handler/types.go)
type HostDeviceParam = vmpkg.HostDeviceParam

// ── Monitor types ──

type VMMonitorStatus = vmpkg.VMMonitorStatus
type VMMonitorCommandResult = vmpkg.VMMonitorCommandResult

// ── Schedule types ──

type VMScheduleInput = vmpkg.VMScheduleInput
type VMScheduleItem = vmpkg.VMScheduleItem
type VMScheduledActionTaskParams = vmpkg.VMScheduledActionTaskParams

// ── Disk migration types ──

type VMDiskMigrationRequest = vmpkg.VMDiskMigrationRequest
type VMDiskMigrationTaskParams = vmpkg.VMDiskMigrationTaskParams
type VMDiskMigrationOptions = vmpkg.VMDiskMigrationOptions
type VMDiskMigrationDisk = vmpkg.VMDiskMigrationDisk

// ── Export types ──

type ExportVMParams = vmpkg.ExportVMParams
type ExportVMResult = vmpkg.ExportVMResult

// ── Password reset types ──

type ResetLinuxPasswordParams = vmpkg.ResetLinuxPasswordParams

// ── Helper types ──

type QemuImgInfo = vmpkg.QemuImgInfo

// ── VM internal result types (exported for cross-package use) ──

type DiskInfoResult = vmpkg.DiskInfoResult
type NetInfoResult = vmpkg.NetInfoResult

// ── Deps mirror types (defined in vm/deps.go) ──
// Note: TemplateMeta, TemplateDefaultConfig are in template_compat.go
// Note: PublicIPAttachment is in public_ip_compat.go
// Note: SchedulerDefinition, SchedulerEventStartInput are in scheduler_compat.go
