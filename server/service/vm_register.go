package service

import (
	"kvm_console/config"
	"kvm_console/model"
	publicippkg "kvm_console/service/public_ip"
	schedpkg "kvm_console/service/scheduler"
	snapshotPkg "kvm_console/service/snapshot"
	templatepkg "kvm_console/service/template"
	vmpkg "kvm_console/service/vm"
)

// vm_register.go — 将 service 根包函数注入到 vm 子包的 Deps 变量中，
// 供 vm 子包通过 D.XXX() 间接调用根包函数，避免循环 import。

func init() {
	vmpkg.InitDeps(&vmpkg.Deps{
		// ---- Host ----
		FirstNonEmpty:                 FirstNonEmpty,
		EnsureMaintenanceModeDisabled: EnsureMaintenanceModeDisabled,
		IsLibvirtUnavailableError:     IsLibvirtUnavailableError,
		IsMaintenanceModeEnabled:      IsMaintenanceModeEnabled,
		IsLibvirtUnavailableText:      isLibvirtUnavailableText,

		// ---- Bandwidth ----
		GetVMBandwidthMbps:           GetVMBandwidthMbps,
		GetVMBandwidth:               GetVMBandwidth,
		ReapplyConfiguredVMBandwidth: ReapplyConfiguredVMBandwidth,

		// ---- User ----
		FindVMOwner:        FindVMOwner,
		CheckQuotaForStart: CheckQuotaForStart,
		GetUserDiskDir:     GetUserDiskDir,

		// ---- Clone ----
		InjectMemballoonConfig:  InjectMemballoonConfig,
		RandomStringFromCharset: RandomStringFromCharset,
		ValidateStrongPassword:  ValidateStrongPassword,
		CloneUsernameRegexp:     CloneUsernameRegexp,

		// ---- Disk / Storage ----
		AddDiskWithBusInDir:   AddDiskWithBusInDir,
		SetDiskIOPSTune:       SetDiskIOPSTune,
		GetDevPrefix:          GetDevPrefix,
		GetVMPCIERootPorts:    GetVMPCIERootPorts,
		ParseQemuInfoGB:       ParseQemuInfoGB,
		ParseQemuInfoStr:      ParseQemuInfoStr,
		CheckVMSnapshotSafety: CheckVMSnapshotSafety,
		GetDiskFilePath:       GetDiskFilePath,
		ListDisks:             ListDisks,
		ChangeFloppy:          ChangeFloppy,

		// ---- Rescue ----
		IsInRescueMode: IsInRescueMode,

		// ---- Stats ----
		GetCachedStats: GetCachedStats,

		// ---- VPC / Network ----
		ApplyVPCBindingRuntime:            ApplyVPCBindingRuntime,
		ApplyVPCBindingToDomainXML:        ApplyVPCBindingToDomainXML,
		ApplyVPCSwitchToDomainXML:         ApplyVPCSwitchToDomainXML,
		SafeVMXMLFileName:                 SafeVMXMLFileName,
		StripRuntimeOnlyInterfaceElements: StripRuntimeOnlyInterfaceElements,
		BridgeNameForSwitch:               BridgeNameForSwitch,
		SwitchUsesDirectBridge:            SwitchUsesDirectBridge,

		// ---- Storage pool ----
		GetAllISOs:           GetAllISOs,
		ResolveVMStorageDir:  ResolveVMStorageDir,
		ListVMStorageTargets: ListVMStorageTargets,

		// ---- OVS ----
		BuildOVSVirtInstallNetworkArg: BuildOVSVirtInstallNetworkArg,
		EnsureOVSNetworkReady:         EnsureOVSNetworkReady,

		// ---- Snapshot ----
		FixSnapshotDiskPermissions: snapshotPkg.FixSnapshotDiskPermissions,

		// ---- Lightweight ----
		ApplyLightweightVMBandwidth:             ApplyLightweightVMBandwidth,
		CheckLightweightVMRuntimeQuotaAvailable: CheckLightweightVMRuntimeQuotaAvailable,
		IsLightweightCloudUser:                  IsLightweightCloudUser,
		IsLightweightCloudVM:                    IsLightweightCloudVM,
		GetLightweightVMQuota:                   GetLightweightVMQuota,

		// ---- Public IP ----
		ListPublicIPAttachmentsForVM: listPublicIPAttachmentsForVM,

		// ---- Template ----
		GetTemplateMeta:       getTemplateMetaForVM,
		WriteVMTemplateSource: WriteVMTemplateSource,

		// ---- Resource check ----
		CheckDirWritable:  CheckDirWritable,
		CheckStorageSpace: CheckStorageSpace,

		// ---- Host (additional) ----
		CollectHostDiskIOBytes: collectHostDiskIOBytes,

		// ---- Bandwidth (additional) ----
		RebalanceUserBandwidth: RebalanceUserBandwidth,

		// ---- Scheduler ----
		RegisterScheduler:           registerSchedulerForVM,
		StartSchedulerEvent:         startSchedulerEventForVM,
		FinishSchedulerEventSuccess: FinishSchedulerEventSuccess,
		FinishSchedulerEventFailed:  FinishSchedulerEventFailed,

		// ---- Clone (additional - circular dep) ----
		DeleteVM: DeleteVM,

		// ---- Migration hooks ----
		// 使用 wrapper 函数而非直接捕获 Hook 变量，避免 init 顺序问题：
		// vm_register.go 的 init 先于 vm/migration/register.go 执行，
		// 直接捕获会得到 nil。wrapper 在调用时才解析 Hook 变量。
		HookEnsureVMNotMigrating:         EnsureVMNotMigrating,
		HookApplyVMUnderMigrationStatus:  ApplyVMUnderMigrationStatus,
		HookDetectMigrationModeFromState: DetectMigrationModeFromState,
		HookMigrationModeLive:            LiveMigrationMode,

		// ---- CPU / Topology / Clock ----
		ParseVCPUCountFromDomainXML:         vmpkg.ParseVCPUCountFromDomainXML,
		ParseVMCPULimitPercentFromDomainXML: vmpkg.ParseVMCPULimitPercentFromDomainXML,
		ParseCPUAffinityFromDomainXML:       vmpkg.ParseCPUAffinityFromDomainXML,
		ParseVMCPUTopologyModeFromDomainXML: vmpkg.ParseVMCPUTopologyModeFromDomainXML,
		ParseVMAPICFromDomainXML:            vmpkg.ParseVMAPICFromDomainXML,
		ParseRTCOffsetFromDomainXML:         vmpkg.ParseRTCOffsetFromDomainXML,
		ParseRTCStartDateFromDomainXML:      vmpkg.ParseRTCStartDateFromDomainXML,
		ApplyRTCConfigToDomainXML:           vmpkg.ApplyRTCConfigToDomainXML,
		ApplyVMAPICToDomainXML:              vmpkg.ApplyVMAPICToDomainXML,
		ApplyVMCPULimitToDomainXML:          vmpkg.ApplyVMCPULimitToDomainXML,
		ApplyCPUTopologyModeToDomainXML:     vmpkg.ApplyCPUTopologyModeToDomainXML,
		ParseCPUAffinity:                    vmpkg.ParseCPUAffinity,
		ValidateCPUAffinity:                 vmpkg.ValidateCPUAffinity,
		ApplyCPUAffinityToDomainXML:         vmpkg.ApplyCPUAffinityToDomainXML,
		EffectiveTopologyVCPU:               vmpkg.EffectiveTopologyVCPU,
		SetVMCPUWithTopologySync:            vmpkg.SetVMCPUWithTopologySync,
		NormalizeVMCPUTopologyMode:          vmpkg.NormalizeVMCPUTopologyMode,

		// ---- Install Media ----
		NormalizeInstallISOSelection:     vmpkg.NormalizeInstallISOSelection,
		ApplyAdditionalCDROMsToDomainXML: vmpkg.ApplyAdditionalCDROMsToDomainXML,

		// ---- Passthrough ----
		EnsureVfioModuleLoaded:   vmpkg.EnsureVfioModuleLoaded,
		ValidatePCIPassthrough:   vmpkg.ValidatePCIPassthrough,
		IsDeviceVfioBound:        vmpkg.IsDeviceVfioBound,
		BindPCIDeviceToVfio:      vmpkg.BindPCIDeviceToVfio,
		ApplyHostDevsToDomainXML: vmpkg.ApplyHostDevsToDomainXML,

		// ---- VM Name ----
		ValidateVMName: vmpkg.ValidateVMName,

		// ---- VM Lock ----
		IsVMLocked: vmpkg.IsVMLocked,

		// ---- CPU Limit ----
		VMCPULimitUnlimited:       vmpkg.VMCPULimitUnlimited,
		ValidateVMCPULimitPercent: vmpkg.ValidateVMCPULimitPercent,

		// ---- VM lifecycle (self-referencing: defined in vm package) ----
		StartVM:                     vmpkg.StartVM,
		StartVMPreserveRebootAction: vmpkg.StartVMPreserveRebootAction,
		ShutdownVM:                  vmpkg.ShutdownVM,
		GetVMInactiveDomainXML:      vmpkg.GetVMInactiveDomainXML,
		SetVMInactiveDomainXML:      vmpkg.SetVMInactiveDomainXML,
		SetVMRemark:                 vmpkg.SetVMRemark,
		SetVMFreeze:                 vmpkg.SetVMFreeze,
		FixOnReboot:                 vmpkg.FixOnReboot,

		// SPICE graphics（创建即带，默认本地监听）
		InjectSPICEGraphics:   InjectSPICEGraphicsToDomainXML,
		EnsureQXLVideo:        EnsureQXLVideo,
		SpiceEnabledByDefault: func() bool { return config.GlobalConfig.SpiceEnabledByDefault },
	})
}

// ── Type-conversion wrappers ──
// The vm package defines mirror types (TemplateMetaAlias, PublicIPAttachment, etc.)
// in deps.go to avoid circular imports. These wrappers convert between the canonical
// types (from template/public_ip/scheduler subpackages) and the mirror types.

// getTemplateMetaForVM wraps GetTemplateMeta, converting templatepkg.TemplateMeta
// to vmpkg.TemplateMetaAlias (a subset mirror type).
func getTemplateMetaForVM(templateName string) *vmpkg.TemplateMetaAlias {
	meta := templatepkg.GetTemplateMeta(templateName)
	if meta == nil {
		return nil
	}
	result := &vmpkg.TemplateMetaAlias{
		Type:         meta.Type,
		Category:     meta.Category,
		BootType:     meta.BootType,
		BootVerified: meta.BootVerified,
		NVRAMPath:    meta.NVRAMPath,
		RootPassword: meta.RootPassword,
		TemplateUser: meta.TemplateUser,
		TemplateUID:  meta.TemplateUID,
		NodeID:       meta.NodeID,
		ParentNodeID: meta.ParentNodeID,
		RootNodeID:   meta.RootNodeID,
		AdminName:    meta.AdminName,
		DisplayName:  meta.DisplayName,
		CloneVisible: meta.CloneVisible,
	}
	if meta.DefaultConfig != nil {
		result.DefaultConfig = &vmpkg.TemplateDefaultConfig{
			DiskBus:             meta.DefaultConfig.DiskBus,
			VideoModel:          meta.DefaultConfig.VideoModel,
			CPUTopologyMode:     meta.DefaultConfig.CPUTopologyMode,
			FirstBootRebootMode: meta.DefaultConfig.FirstBootRebootMode,
		}
	}
	return result
}

// listPublicIPAttachmentsForVM wraps ListPublicIPAttachmentsForVM,
// converting publicippkg.PublicIPAttachment to vmpkg.PublicIPAttachment.
func listPublicIPAttachmentsForVM(vmName string) []vmpkg.PublicIPAttachment {
	attachments := publicippkg.ListPublicIPAttachmentsForVM(vmName)
	result := make([]vmpkg.PublicIPAttachment, len(attachments))
	for i, a := range attachments {
		result[i] = vmpkg.PublicIPAttachment{
			PublicIP:      a.PublicIP,
			Mode:          a.Mode,
			ModeLabel:     a.ModeLabel,
			VMPrivateIP:   a.VMPrivateIP,
			RuntimeStatus: a.RuntimeStatus,
		}
	}
	return result
}

// registerSchedulerForVM wraps RegisterScheduler, converting vmpkg.SchedulerDefinition
// to schedpkg.SchedulerDefinition.
func registerSchedulerForVM(def vmpkg.SchedulerDefinition) {
	RegisterScheduler(schedpkg.SchedulerDefinition{
		Key:         def.Key,
		Name:        def.Name,
		Group:       def.Group,
		Description: def.Description,
		Enabled:     def.Enabled,
	})
}

// startSchedulerEventForVM wraps StartSchedulerEvent, converting vmpkg.SchedulerEventStartInput
// to schedpkg.SchedulerEventStartInput.
func startSchedulerEventForVM(input vmpkg.SchedulerEventStartInput) (*model.SchedulerEvent, error) {
	return StartSchedulerEvent(schedpkg.SchedulerEventStartInput{
		SchedulerKey:   input.SchedulerKey,
		SchedulerName:  input.SchedulerName,
		SchedulerGroup: input.SchedulerGroup,
		VMName:         input.VMName,
		VMBackend:      input.VMBackend,
		TriggerReason:  input.TriggerReason,
	})
}
