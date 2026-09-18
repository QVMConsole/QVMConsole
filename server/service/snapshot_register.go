package service

import (
	"kvm_console/service/snapshot"
)

// snapshot_register.go — 将 service 根包函数注册到 snapshot 子包的 Hook 变量。
// snapshot 子包通过 Hook 调用 service 根包函数，避免循环 import。

func init() {
	// Lifecycle hooks
	snapshot.HookEnsureVMNotMigrating = func(vmName, action string) error {
		if HookEnsureVMNotMigrating != nil {
			return HookEnsureVMNotMigrating(vmName, action)
		}
		return nil
	}
	snapshot.HookStartVM = StartVM
	snapshot.HookShutdownVM = ShutdownVM

	// VM XML hooks
	snapshot.HookGetVMInactiveDomainXML = GetVMInactiveDomainXML
	snapshot.HookSetVMInactiveDomainXML = SetVMInactiveDomainXML

	// Runtime state hook
	snapshot.HookUpdateVMRuntimeState = UpdateVMRuntimeState

	// Disk / validation hooks
	snapshot.HookValidateDiskBackingChain = ValidateDiskBackingChain
	snapshot.HookExtractDomainNVRAMPath = ExtractDomainNVRAMPath
	snapshot.HookQemuInfoChain = func(path string) ([]snapshot.QemuImgInfo, error) {
		items, err := QemuInfoChain(path)
		if err != nil {
			return nil, err
		}
		var out []snapshot.QemuImgInfo
		for _, it := range items {
			out = append(out, snapshot.QemuImgInfo{
				Filename:            it.Filename,
				Format:              it.Format,
				VirtualSize:         it.VirtualSize,
				ActualSize:          it.ActualSize,
				BackingFilename:     it.BackingFilename,
				FullBackingFilename: it.FullBackingFilename,
			})
		}
		return out, nil
	}
	snapshot.HookListShares = func(vmName string) ([]snapshot.ShareBrief, error) {
		shares, err := ListShares(vmName)
		if err != nil {
			return nil, err
		}
		var briefs []snapshot.ShareBrief
		for _, s := range shares {
			briefs = append(briefs, snapshot.ShareBrief{
				Tag:    s.Tag,
				Source: s.Source,
				Target: s.Target,
			})
		}
		return briefs, nil
	}

	// Lightweight cloud hooks
	snapshot.HookIsLightweightCloudVM = IsLightweightCloudVM
	snapshot.HookIsLightweightCloudUser = IsLightweightCloudUser
	snapshot.HookGetLightweightVMQuota = GetLightweightVMQuota
	snapshot.HookCheckLightweightVMSnapshotQuota = CheckLightweightVMSnapshotQuota
	snapshot.HookGetUserVMList = GetUserVMList

	// Snapshot lock hook — service 根包暴露给 handler 层
	HookEnsureVMNotSnapshotting = snapshot.EnsureVMNotSnapshotting
}
