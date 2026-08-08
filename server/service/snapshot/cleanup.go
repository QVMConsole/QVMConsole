package snapshot

import (
	"fmt"
	"os"
	"path"
	"strings"

	"kvm_console/logger"
	"kvm_console/utils"
)

// deleteExternalSnapshot 删除外部快照。
// preserveOtherSnapshots 为 true 时，不能把 overlay commit 回仍被其他外部快照用作恢复点的 backing。
func deleteExternalSnapshot(vmName, snapName string, preserveOtherSnapshots bool) error {
	// 获取快照关联的 overlay 文件
	snapFiles, err := getExternalSnapshotDiskFiles(vmName, snapName)
	if err != nil {
		logger.App.Warn("获取外部快照文件列表失败", "error", err)
	}

	// 获取当前 VM 正在使用的磁盘文件，当前活动 overlay 必须先 blockcommit + pivot，不能只删元数据。
	currentDiskList, diskErr := getCurrentVMDiskSources(vmName)
	if diskErr != nil {
		logger.App.Warn("获取当前磁盘列表失败", "error", diskErr)
	}
	activeDiskTargets := make(map[string]string)
	for _, disk := range currentDiskList {
		if disk.Source != "" && disk.Source != "-" {
			activeDiskTargets[disk.Source] = disk.Target
		}
	}
	stateResult := utils.ExecCommand("virsh", "domstate", vmName)
	isRunning := stateResult.Error == nil && strings.TrimSpace(stateResult.Stdout) == "running"
	protectedBacking, err := getProtectedExternalSnapshotRestoreFiles(vmName, snapName)
	if err != nil {
		return err
	}

	// 当前活动 overlay 删除时要保留当前状态；非活动 leaf overlay 不应该 commit 回 backing，
	// 否则会污染仍然依赖该 backing 的早期外部快照恢复点。
	for _, f := range snapFiles {
		if target, ok := activeDiskTargets[f]; ok {
			chain, err := HookQemuInfoChain(f)
			if err != nil {
				return fmt.Errorf("读取外部快照磁盘链失败 (%s): %w", f, err)
			}
			backingPath := ""
			if len(chain) >= 2 {
				backingPath = chain[1].Filename
			}
			if isRunning {
				if preserveOtherSnapshots && protectedBacking[backingPath] {
					logger.App.Info("执行 blockcopy 独立当前盘", "file", f)
					if err := copyActiveExternalOverlayToStandalone(vmName, target, f); err != nil {
						return fmt.Errorf("复制当前活动磁盘失败 (%s): %w", f, err)
					}
					continue
				}
				logger.App.Info("执行在线 blockcommit 并 pivot", "file", f)
				if err := commitActiveExternalOverlay(vmName, target, f); err != nil {
					return fmt.Errorf("合并当前正在使用的外部快照失败 (%s): %w", f, err)
				}
			} else {
				if len(chain) < 2 {
					return fmt.Errorf("外部快照 %s 没有 backing 文件，无法合并", f)
				}
				if preserveOtherSnapshots && protectedBacking[backingPath] {
					logger.App.Info("删除 overlay 文件但不 commit", "file", f)
					_ = os.Remove(f)
					continue
				}
				if err := commitInactiveExternalOverlay(vmName, f, chain[1].Filename); err != nil {
					return fmt.Errorf("合并关机状态外部快照失败 (%s): %w", f, err)
				}
			}
			continue
		}
		if _, err := os.Stat(f); err == nil {
			if preserveOtherSnapshots {
				logger.App.Info("删除 overlay 文件避免污染", "file", f)
				_ = os.Remove(f)
			} else {
				// qemu-img commit 需要合并整个 overlay 数据，属于大 IO 操作，不设置自动超时
				commitResult := utils.ExecCommandNoTimeout("qemu-img", "commit", f)
				if commitResult.Error != nil {
					logger.App.Warn("合并overlay文件失败", "file", f, "stderr", commitResult.Stderr)
				} else {
					if err := os.Remove(f); err != nil {
						logger.App.Warn("删除overlay文件失败", "file", f, "error", err)
					} else {
						logger.App.Info("已合并并删除overlay文件", "file", f)
					}
				}
			}
		}
	}

	// 删除快照元数据（对外部快照只能用 --metadata）
	return deleteSnapshotMetadataOnly(vmName, snapName, "外部快照文件已按当前删除策略处理")
}

func consolidateActiveExternalOverlays(vmName string, preserveOtherSnapshots bool) (bool, error) {
	diskList, err := getCurrentVMDiskSources(vmName)
	if err != nil {
		return false, err
	}
	protectedBacking, err := getProtectedExternalSnapshotRestoreFiles(vmName, "")
	if err != nil {
		return false, err
	}

	stateResult := utils.ExecCommand("virsh", "domstate", vmName)
	isRunning := stateResult.Error == nil && strings.TrimSpace(stateResult.Stdout) == "running"

	consolidated := false
	for _, disk := range diskList {
		if disk.Source == "" || disk.Source == "-" {
			continue
		}
		chain, err := HookQemuInfoChain(disk.Source)
		if err != nil {
			return consolidated, err
		}
		if len(chain) < 2 || strings.TrimSpace(chain[0].BackingFilename) == "" {
			continue
		}
		if !isLikelyExternalSnapshotOverlay(disk.Source) {
			continue
		}
		backingPath := chain[1].Filename
		if isRunning {
			if preserveOtherSnapshots && protectedBacking[backingPath] {
				if err := copyActiveExternalOverlayToStandalone(vmName, disk.Target, disk.Source); err != nil {
					return consolidated, err
				}
			} else {
				if err := commitActiveExternalOverlay(vmName, disk.Target, disk.Source); err != nil {
					return consolidated, err
				}
			}
		} else {
			if preserveOtherSnapshots && protectedBacking[backingPath] {
				return consolidated, fmt.Errorf("当前关机磁盘 %s 的 backing 仍是其他外部快照恢复点，不能 commit 以免污染早期快照", disk.Source)
			}
			if err := commitInactiveExternalOverlay(vmName, disk.Source, chain[1].Filename); err != nil {
				return consolidated, err
			}
		}
		consolidated = true
	}
	return consolidated, nil
}

func consolidateActiveSnapshotResidualOverlays(vmName, snapName string, preserveOtherSnapshots bool) error {
	snapName = strings.TrimSpace(snapName)
	if snapName == "" {
		return nil
	}
	diskList, err := getCurrentVMDiskSources(vmName)
	if err != nil {
		return err
	}
	protectedBacking, err := getProtectedExternalSnapshotRestoreFiles(vmName, snapName)
	if err != nil {
		return err
	}

	stateResult := utils.ExecCommand("virsh", "domstate", vmName)
	isRunning := stateResult.Error == nil && strings.TrimSpace(stateResult.Stdout) == "running"

	for _, disk := range diskList {
		if disk.Source == "" || disk.Source == "-" {
			continue
		}
		if !isLikelyExternalSnapshotOverlay(disk.Source) || !strings.Contains(path.Base(disk.Source), snapName) {
			continue
		}
		chain, err := HookQemuInfoChain(disk.Source)
		if err != nil {
			return err
		}
		if len(chain) < 2 || strings.TrimSpace(chain[0].BackingFilename) == "" {
			continue
		}
		backingPath := chain[1].Filename
		if isRunning {
			if preserveOtherSnapshots && protectedBacking[backingPath] {
				if err := copyActiveExternalOverlayToStandalone(vmName, disk.Target, disk.Source); err != nil {
					return err
				}
			} else {
				if err := commitActiveExternalOverlay(vmName, disk.Target, disk.Source); err != nil {
					return err
				}
			}
			continue
		}
		if preserveOtherSnapshots && protectedBacking[backingPath] {
			return fmt.Errorf("当前关机磁盘 %s 的 backing 仍是其他外部快照恢复点，不能 commit 以免污染早期快照", disk.Source)
		}
		if err := commitInactiveExternalOverlay(vmName, disk.Source, backingPath); err != nil {
			return err
		}
	}
	return nil
}

func isLikelyExternalSnapshotOverlay(diskPath string) bool {
	name := path.Base(strings.TrimSpace(diskPath))
	return strings.Contains(name, ".snap_") || strings.Contains(name, "-snap_")
}

func getProtectedExternalSnapshotRestoreFiles(vmName, excludeSnapName string) (map[string]bool, error) {
	protected := make(map[string]bool)
	snapshots, err := ListSnapshots(vmName)
	if err != nil {
		return protected, err
	}
	for _, snap := range snapshots {
		if snap.Name == excludeSnapName {
			continue
		}
		if snap.State != "disk-snapshot" && snap.Location != "external" {
			continue
		}
		result := utils.ExecCommand("virsh", "snapshot-dumpxml", vmName, snap.Name)
		if result.Error != nil {
			return protected, fmt.Errorf("获取快照 %s XML 失败: %s", snap.Name, result.Stderr)
		}
		files, err := parseExternalSnapshotOriginalDiskFiles(result.Stdout)
		if err != nil {
			return protected, err
		}
		for _, file := range files {
			if strings.TrimSpace(file) != "" {
				protected[file] = true
			}
		}
	}
	return protected, nil
}