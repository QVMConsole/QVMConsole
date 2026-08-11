package service

import (
	"kvm_console/logger"
	"kvm_console/model"
	templatepkg "kvm_console/service/template"
	vmpkg "kvm_console/service/vm"
)

func init() {
	// ── Clone (circular dependency) ──
	templatepkg.HookDeleteVM = DeleteVM
	templatepkg.HookFinalizeDeletedVM = func(vmName string) {
		if err := DeleteVMCredential(vmName); err != nil {
			logger.App.Warn("移动制作模板后清理虚拟机凭据失败", "vm", vmName, "error", err)
		}
		if err := model.DeleteVMLock(vmName); err != nil {
			logger.App.Warn("移动制作模板后清理虚拟机锁失败", "vm", vmName, "error", err)
		}
		MarkVMCacheMissingAsync(vmName)
	}

	// ── VM helpers (private in service root) ──
	templatepkg.HookGetVMDiskInfo = func(vmName string) templatepkg.VMDiskBrief {
		info := vmpkg.GetVMDiskInfo(vmName)
		return templatepkg.VMDiskBrief{
			Device:   info.Device,
			Path:     info.Path,
			Size:     info.Size,
			Template: info.Template,
		}
	}
	templatepkg.HookGetVMNetworkInfo = func(vmName string) templatepkg.VMNetBrief {
		info := vmpkg.GetVMNetworkInfo(vmName)
		return templatepkg.VMNetBrief{
			Network:  info.Network,
			MAC:      info.MAC,
			NICModel: info.NicModel,
		}
	}
	templatepkg.HookGetDomainState = GetDomainState
	templatepkg.HookCopyDiskFileSparse = vmpkg.CopyDiskFileSparse

	// ── VM normalization ──
	templatepkg.HookNormalizeVMDiskBus = NormalizeVMDiskBus
	templatepkg.HookNormalizeVMNicModel = NormalizeVMNicModel
	templatepkg.HookNormalizeVMCPUTopologyMode = NormalizeVMCPUTopologyMode
	templatepkg.HookNormalizeVMFirstBootRebootMode = NormalizeVMFirstBootRebootMode
	templatepkg.HookParseVMCPUTopologyModeFromDomainXML = ParseVMCPUTopologyModeFromDomainXML

	// ── Disk / snapshot helpers ──
	templatepkg.HookListDisks = func(vmName string) ([]templatepkg.DiskBrief, error) {
		disks, err := ListDisks(vmName)
		if err != nil {
			return nil, err
		}
		briefs := make([]templatepkg.DiskBrief, len(disks))
		for i, d := range disks {
			briefs[i] = templatepkg.DiskBrief{
				DeviceType: d.DeviceType,
				CapacityGB: d.CapacityGB,
				Bus:        d.Bus,
			}
		}
		return briefs, nil
	}
	templatepkg.HookQemuInfoChain = func(path string) ([]templatepkg.QemuChainEntry, error) {
		chain, err := QemuInfoChain(path)
		if err != nil {
			return nil, err
		}
		entries := make([]templatepkg.QemuChainEntry, len(chain))
		for i, c := range chain {
			entries[i] = templatepkg.QemuChainEntry{
				Filename:            c.Filename,
				VirtualSize:         c.VirtualSize,
				ActualSize:          c.ActualSize,
				BackingFilename:     c.BackingFilename,
				FullBackingFilename: c.FullBackingFilename,
			}
		}
		return entries, nil
	}
	templatepkg.HookSetLibvirtDiskFileOwner = vmpkg.SetLibvirtDiskFileOwner
	templatepkg.HookUpdateInactiveDomainDiskPath = vmpkg.UpdateInactiveDomainDiskPath
	templatepkg.HookSameCleanPath = vmpkg.SameCleanPath
	templatepkg.HookCheckVMSnapshotSafety = CheckVMSnapshotSafety

	// ── User management ──
	templatepkg.HookFindVMOwner = FindVMOwner
	templatepkg.HookRemoveVMFromUser = RemoveVMFromUser
	templatepkg.HookRebalanceUserBandwidth = RebalanceUserBandwidth

	// ── Utility ──
	templatepkg.HookFormatBytesPublic = FormatBytesPublic
	templatepkg.HookFirstNonEmpty = FirstNonEmpty
}
