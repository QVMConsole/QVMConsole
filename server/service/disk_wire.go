package service

import (
	"kvm_console/service/snapshot"
	"kvm_console/service/storage/disk"
)

// init wires disk package function variables to service root implementations.
// This breaks the circular dependency: disk package cannot import service,
// so it exposes function variables that we set here.
func init() {
	disk.EnsureNotMigrating = func(vmName, action string) error {
		if HookEnsureVMNotMigrating != nil {
			return HookEnsureVMNotMigrating(vmName, action)
		}
		return nil
	}
	disk.ResolveStorageDir = ResolveVMStorageDir
	disk.GetStorageMountPointFn = GetStorageMountPoint
	disk.GetUserDiskDirFn = GetUserDiskDir
	disk.CheckSnapshotSafety = checkSnapshotSafetyForDisk
}

// checkSnapshotSafetyForDisk adapts the service-level ListSnapshots + SnapshotInfo
// into the simplified signature expected by the disk package.
func checkSnapshotSafetyForDisk(vmName string) (bool, []string, error) {
	ss, err := snapshot.ListSnapshots(vmName)
	if err != nil {
		return false, nil, err
	}
	var externalSnaps []string
	for _, snap := range ss {
		if snap.State == "disk-snapshot" || snap.Location == "external" {
			externalSnaps = append(externalSnaps, snap.Name)
		}
	}
	return len(externalSnaps) > 0, externalSnaps, nil
}
