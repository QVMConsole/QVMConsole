package snapshot

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

// ListSnapshots 列出快照
func ListSnapshots(vmName string) ([]SnapshotInfo, error) {
	result := utils.ExecCommandQuiet("virsh", "snapshot-list", vmName, "--tree")
	if result.Error != nil {
		// 没有快照时也可能返回错误
		return []SnapshotInfo{}, nil
	}

	// 使用详细命令获取每个快照信息
	listResult := utils.ExecCommandQuiet("virsh", "snapshot-list", vmName)
	if listResult.Error != nil {
		return []SnapshotInfo{}, nil
	}

	var snapshots []SnapshotInfo
	lines := strings.Split(listResult.Stdout, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "-") {
			continue
		}

		// 解析：Name  Creation Time  State
		re := regexp.MustCompile(`^(\S+)\s+(.+?\d{4}.*?\d{2}:\d{2}:\d{2})\s+(\S+.*)$`)
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 4 {
			snap := SnapshotInfo{
				Name:      matches[1],
				CreatedAt: strings.TrimSpace(matches[2]),
				State:     strings.TrimSpace(matches[3]),
			}

			// 获取描述和位置信息
			infoResult := utils.ExecCommand("virsh", "snapshot-info", vmName, snap.Name)
			if infoResult.Error == nil {
				for _, infoLine := range strings.Split(infoResult.Stdout, "\n") {
					infoLine = strings.TrimSpace(infoLine)
					if strings.HasPrefix(infoLine, "Description:") {
						snap.Description = strings.TrimSpace(strings.TrimPrefix(infoLine, "Description:"))
					}
					if strings.HasPrefix(infoLine, "Location:") {
						snap.Location = strings.TrimSpace(strings.TrimPrefix(infoLine, "Location:"))
					}
					if strings.HasPrefix(infoLine, "Children:") {
						snap.Children = parseSnapshotInfoInt(infoLine, "Children:")
					}
					if strings.HasPrefix(infoLine, "Descendants:") {
						snap.Descendants = parseSnapshotInfoInt(infoLine, "Descendants:")
					}
				}
			}
			if snap.Description == "" {
				snap.Description = getSnapshotDescriptionFromXML(vmName, snap.Name)
			}

			snapshots = append(snapshots, snap)
		}
	}

	// 获取当前快照
	currentResult := utils.ExecCommandQuiet("virsh", "snapshot-current", vmName, "--name")
	if currentResult.Error == nil {
		currentName := strings.TrimSpace(currentResult.Stdout)
		for i := range snapshots {
			if snapshots[i].Name == currentName {
				snapshots[i].IsCurrent = true
			}
		}
	}

	return snapshots, nil
}

func getSnapshotDescriptionFromXML(vmName, snapName string) string {
	result := utils.ExecCommand("virsh", "snapshot-dumpxml", vmName, snapName)
	if result.Error != nil {
		return ""
	}
	var doc snapshotXMLDescription
	if err := xml.Unmarshal([]byte(result.Stdout), &doc); err == nil {
		return strings.TrimSpace(doc.Description)
	}
	matches := regexp.MustCompile(`(?s)<description>\s*(.*?)\s*</description>`).FindStringSubmatch(result.Stdout)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

// CreateSnapshot 创建快照
// includeMemory 为 true 时保存内存状态（仅运行中的虚拟机有效），为 false 时仅保存磁盘状态
func CreateSnapshot(vmName, snapName, description string, includeMemory bool) error {
	return CreateSnapshotWithOptions(vmName, snapName, description, includeMemory, false, true, nil)
}

// CreateSnapshotWithOptions 创建快照，可在用户确认后自动修复 UEFI pflash NVRAM 格式。
// pauseForMemorySnapshot 控制运行中内存快照是否先主动暂停虚拟机。
func CreateSnapshotWithOptions(vmName, snapName, description string, includeMemory, autoFixNVRAM, pauseForMemorySnapshot bool, progressFn func(int, string)) error {
	if err := HookEnsureVMNotMigrating(vmName, "创建快照"); err != nil {
		return err
	}
	normalizedName, err := NormalizeSnapshotName(snapName)
	if err != nil {
		return err
	}
	snapName = normalizedName
	args := []string{"snapshot-create-as", vmName, "--name", snapName}
	if description != "" {
		args = append(args, "--description", description)
	}

	// 检测虚拟机当前状态
	stateResult := utils.ExecCommand("virsh", "domstate", vmName)
	isRunning := stateResult.Error == nil && strings.TrimSpace(stateResult.Stdout) == "running"

	if isRunning {
		if includeMemory {
			if unsupported, message, err := CheckInternalSnapshotVirtFSUnsupported(vmName); err != nil {
				return err
			} else if unsupported {
				return fmt.Errorf("%s", message)
			}
			if err := ensureInternalSnapshotNVRAMCompatible(vmName, true); err != nil {
				required, _, checkErr := CheckInternalSnapshotNVRAMRepairRequired(vmName)
				if !autoFixNVRAM || checkErr != nil || !required {
					return err
				}
				if err := autoRepairRunningVMNVRAMForInternalSnapshot(vmName, progressFn); err != nil {
					return err
				}
				stateResult = utils.ExecCommand("virsh", "domstate", vmName)
				isRunning = stateResult.Error == nil && strings.TrimSpace(stateResult.Stdout) == "running"
				if !isRunning {
					return fmt.Errorf("UEFI NVRAM 已修复，但虚拟机重新开机后状态不是 running，请检查虚拟机状态后重试")
				}
				if err := ensureInternalSnapshotNVRAMCompatible(vmName, true); err != nil {
					return err
				}
			}
			if progressFn != nil {
				if pauseForMemorySnapshot {
					progressFn(60, "NVRAM 兼容性检查完成，正在暂停虚拟机并创建内存快照...")
				} else {
					progressFn(60, "NVRAM 兼容性检查完成，正在直接创建内存快照...")
				}
			}
			if pauseForMemorySnapshot {
				// 运行中 + 包含内存：先暂停 VM → 创建内部快照 → 恢复运行。
				// 这样更容易得到一致的内部快照，但快照写入期间业务会处于暂停状态。
				logger.App.Info("暂停虚拟机以创建内部快照", "vm", vmName)
				pauseResult := utils.ExecCommand("virsh", "suspend", vmName)
				if pauseResult.Error != nil {
					return fmt.Errorf("暂停虚拟机失败，无法创建内部快照: %s", pauseResult.Stderr)
				}
				if progressFn != nil {
					progressFn(70, "虚拟机已暂停，正在写入内部快照...")
				}

				// 暂停后创建快照（不加 --disk-only，关机/暂停状态下默认创建内部快照）。
				// 内存快照可能耗时较长（取决于虚拟机内存大小），不设置自动超时，等待自然完成。
				result := utils.ExecCommandNoTimeout("virsh", args...)

				// 无论成功失败，都恢复 VM 运行。
				logger.App.Info("恢复虚拟机运行", "vm", vmName)
				resumeResult := utils.ExecCommand("virsh", "resume", vmName)
				if resumeResult.Error != nil {
					logger.App.Warn("恢复虚拟机运行失败", "stderr", resumeResult.Stderr)
				}

				if result.Error != nil {
					// 即使 virsh 命令报错，也可能快照已创建（如超时后 libvirt 后台继续完成）。
					if snapshotExists(vmName, snapName) {
						logger.App.Info("virsh 命令失败但快照已存在（可能因超时杀进程但后台任务已完成），视为创建成功",
							"vm", vmName, "snapshot", snapName, "virsh_error", result.Error)
						return nil
					}
					return formatSnapshotCreateError(result.Stderr)
				}
			} else {
				// 实验模式：不主动 suspend，交给 libvirt/QEMU 自行处理运行中内存快照。
				// 注意：即使是"不主动暂停"模式，libvirt/QEMU 在保存内存状态时仍会
				// 将 VM 置于 paused (saving) 状态，这不是面板行为，而是 QEMU savevm 的固有行为。
				// 该模式可能缩短面板层面的暂停窗口，但不同宿主机/libvirt 版本的行为可能不同。
				// 内存快照可能耗时较长（取决于虚拟机内存大小），不设置自动超时，等待自然完成。
				logger.App.Info("不主动暂停虚拟机，由 libvirt/QEMU 自行管理内存快照创建", "vm", vmName)
				result := utils.ExecCommandNoTimeout("virsh", args...)
				if result.Error != nil {
					// 即使 virsh 命令报错，也可能快照已创建（如超时后 libvirt 后台继续完成）。
					// 检查快照是否实际已存在。
					if snapshotExists(vmName, snapName) {
						logger.App.Info("virsh 命令失败但快照已存在（可能因超时杀进程但后台任务已完成），视为创建成功",
							"vm", vmName, "snapshot", snapName, "virsh_error", result.Error)
						// 如果 VM 仍处于 paused (saving) 状态，尝试恢复
						ensureVMRunning(vmName)
						return nil
					}
					return formatSnapshotCreateError(result.Stderr)
				}
			}
		} else {
			// 运行中 + 不包含内存：仅磁盘快照（外部）
			// 注意：外部快照不支持 virsh snapshot-revert
			logger.App.Warn("运行中创建仅磁盘快照为外部快照", "vm", vmName)
			args = append(args, "--disk-only")
			result := utils.ExecCommand("virsh", args...)
			if result.Error != nil {
				// 即使 virsh 命令报错，也可能快照已创建（如超时后 libvirt 后台继续完成）。
				if snapshotExists(vmName, snapName) {
					logger.App.Info("virsh 命令失败但快照已存在（可能因超时杀进程但后台任务已完成），视为创建成功",
						"vm", vmName, "snapshot", snapName, "virsh_error", result.Error)
					return nil
				}
				return formatSnapshotCreateError(result.Stderr)
			}

			// 修正外部快照创建后 overlay 文件的权限
			// libvirt 创建的 overlay 文件默认归 root:root 且权限 600，
			// 需要修改为 libvirt-qemu:kvm，否则 QEMU 进程无法读取导致开机失败
			FixSnapshotDiskPermissions(vmName)
		}
	} else {
		if err := ensureInternalSnapshotNVRAMCompatible(vmName, false); err != nil {
			return err
		}
		// 关机状态下不加额外参数，使用默认的内部快照
		result := utils.ExecCommand("virsh", args...)
		if result.Error != nil {
			return formatSnapshotCreateError(result.Stderr)
		}
	}

	// 创建完成后，检查实际创建的快照类型，向调用者报告
	state, location, err := getSnapshotType(vmName, snapName)
	if err == nil && location == "external" {
		logger.App.Warn("快照被创建为外部快照", "snapshot", snapName, "state", state, "location", location)
	}

	return nil
}

func formatSnapshotCreateError(stderr string) error {
	message := strings.TrimSpace(stderr)
	if message == "" {
		message = "未知错误"
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "invalid job id") {
		return fmt.Errorf("创建快照失败: 快照名称包含 libvirt/QEMU 不支持的字符，请使用英文、数字、下划线、点或短横线")
	}
	if strings.Contains(lower, "migration is disabled") && strings.Contains(lower, "virtfs") {
		return fmt.Errorf("创建快照失败: 当前虚拟机正在挂载 9p/VirtFS 共享目录，libvirt 禁止在这种状态创建包含内存的内部快照。请先卸载并移除共享目录，或取消勾选\"保存虚拟机内存状态\"创建仅磁盘快照")
	}
	return fmt.Errorf("创建快照失败: %s", message)
}

// RevertSnapshot 恢复快照
func RevertSnapshot(vmName, snapName string) error {
	if err := HookEnsureVMNotMigrating(vmName, "恢复快照"); err != nil {
		return err
	}
	// 先检查快照类型
	state, location, err := getSnapshotType(vmName, snapName)
	if err != nil {
		return fmt.Errorf("获取快照信息失败: %w", err)
	}

	// 外部快照（disk-snapshot）不能直接使用 virsh snapshot-revert
	if state == "disk-snapshot" || location == "external" {
		return revertExternalSnapshot(vmName, snapName)
	}

	if err := ensureInternalSnapshotRestoreDiskAccess(vmName, snapName); err != nil {
		return err
	}

	// 内部内存快照恢复需要重新加载虚拟机内存，耗时会随内存大小增长。
	// 不设置自动超时，避免 virsh 客户端提前退出而 libvirt 仍在后台恢复。
	result := utils.ExecCommandNoTimeout("virsh", "snapshot-revert", vmName, snapName)
	if result.Error != nil {
		return fmt.Errorf("恢复快照失败: %s", result.Stderr)
	}

	// 恢复后检查 VM 状态，如果是暂停状态（快照在暂停时创建的）则自动恢复运行
	vmState := utils.ExecCommand("virsh", "domstate", vmName)
	if vmState.Error == nil && strings.TrimSpace(vmState.Stdout) == "paused" {
		logger.App.Info("虚拟机处于暂停状态，自动恢复运行", "vm", vmName)
		resumeResult := utils.ExecCommand("virsh", "resume", vmName)
		if resumeResult.Error != nil {
			logger.App.Warn("自动恢复运行失败", "stderr", resumeResult.Stderr)
		}
	}

	return nil
}

// DeleteSnapshot 删除快照
func DeleteSnapshot(vmName, snapName string) error {
	if err := deleteSnapshot(vmName, snapName, true); err != nil {
		return err
	}
	if err := consolidateActiveSnapshotResidualOverlays(vmName, snapName, true); err != nil {
		return fmt.Errorf("清理当前快照残留 overlay 失败: %w", err)
	}
	if err := cleanupSnapshotResidualFiles(vmName, snapName, false); err != nil {
		return fmt.Errorf("清理快照残留文件失败: %w", err)
	}
	return nil
}

func deleteSnapshot(vmName, snapName string, preserveOtherSnapshots bool) error {
	if err := HookEnsureVMNotMigrating(vmName, "删除快照"); err != nil {
		return err
	}
	// 先检查快照类型
	info, err := getSnapshotInfo(vmName, snapName)
	if err != nil {
		// 如果获取失败，尝试直接删除
		result := utils.ExecCommand("virsh", "snapshot-delete", vmName, snapName)
		if result.Error != nil {
			if !preserveOtherSnapshots && isInternalSnapshotDiskMismatchDeleteError(result.Stderr) {
				return deleteSnapshotMetadataOnly(vmName, snapName, "删除全部快照时无法读取完整快照信息，且内部快照所在磁盘已不在当前活动链，仅清理 libvirt 元数据")
			}
			return formatSnapshotDeleteError(result.Stderr)
		}
		return nil
	}

	if info.Children > 0 {
		return fmt.Errorf("删除快照失败: 当前快照还有 %d 个子快照，不能直接删除父级快照。请先从快照树最末端的子快照开始处理；如果该快照是外部快照链的父节点，请先恢复/切回目标快照并确认子快照链已清理后再删除", info.Children)
	}

	if info.State == "disk-snapshot" || info.Location == "external" {
		return deleteExternalSnapshot(vmName, snapName, preserveOtherSnapshots)
	}

	// 内部快照直接删除
	result := utils.ExecCommand("virsh", "snapshot-delete", vmName, snapName)
	if result.Error != nil {
		if isInternalSnapshotDiskMismatchDeleteError(result.Stderr) {
			consolidated, err := consolidateActiveExternalOverlays(vmName, preserveOtherSnapshots)
			if err != nil {
				return fmt.Errorf("删除快照失败: 当前 VM 正在使用外部快照 overlay，自动合并并切回原始磁盘失败: %w", err)
			}
			if consolidated {
				retryResult := utils.ExecCommand("virsh", "snapshot-delete", vmName, snapName)
				if retryResult.Error == nil {
					return nil
				}
				if !preserveOtherSnapshots && isInternalSnapshotDiskMismatchDeleteError(retryResult.Stderr) {
					return deleteSnapshotMetadataOnly(vmName, snapName, "已尝试折叠当前外部 overlay，但 libvirt 仍认为该内部快照所在磁盘与当前活动磁盘不一致，删除全部快照时仅清理 libvirt 元数据")
				}
				return formatSnapshotDeleteError(retryResult.Stderr)
			}
			if !preserveOtherSnapshots {
				return deleteSnapshotMetadataOnly(vmName, snapName, "当前活动磁盘已经不再包含该内部快照，删除全部快照时仅清理 libvirt 元数据")
			}
			return fmt.Errorf("删除快照失败: 当前活动磁盘与内部快照所在磁盘不一致，但没有发现可自动合并的外部快照 overlay。请确认当前磁盘链后再删除该内部快照")
		}
		return formatSnapshotDeleteError(result.Stderr)
	}
	return nil
}

// DeleteAllSnapshots 删除虚拟机的全部快照，按快照树叶子节点逐步清理。
func DeleteAllSnapshots(vmName string, progressFn func(int, string)) (int, error) {
	if err := HookEnsureVMNotMigrating(vmName, "删除全部快照"); err != nil {
		return 0, err
	}

	deleted := 0
	for step := 0; step < 1000; step++ {
		snapshots, err := ListSnapshots(vmName)
		if err != nil {
			return deleted, err
		}
		if len(snapshots) == 0 {
			if _, err := consolidateActiveExternalOverlays(vmName, false); err != nil {
				return deleted, fmt.Errorf("清理当前快照 overlay 失败: %w", err)
			}
			if err := cleanupSnapshotResidualFiles(vmName, "", true); err != nil {
				return deleted, fmt.Errorf("清理快照残留文件失败: %w", err)
			}
			if progressFn != nil {
				progressFn(100, "全部快照已删除")
			}
			return deleted, nil
		}

		leaf := findSnapshotLeafForDelete(snapshots)
		if leaf == nil {
			return deleted, fmt.Errorf("删除全部快照失败: 未找到可删除的叶子快照，请检查快照树状态")
		}
		if progressFn != nil {
			percent := 10 + min(85, deleted*10)
			progressFn(percent, fmt.Sprintf("正在删除快照 %s，剩余 %d 个...", leaf.Name, len(snapshots)))
		}
		if err := deleteSnapshot(vmName, leaf.Name, false); err != nil {
			return deleted, err
		}
		if err := cleanupSnapshotResidualFiles(vmName, leaf.Name, false); err != nil {
			return deleted, fmt.Errorf("清理快照 %s 残留文件失败: %w", leaf.Name, err)
		}
		deleted++
	}
	return deleted, fmt.Errorf("删除全部快照失败: 快照数量或链条状态异常，已达到最大处理轮次")
}

func findSnapshotLeafForDelete(snapshots []SnapshotInfo) *SnapshotInfo {
	for i := range snapshots {
		if snapshots[i].Children == 0 {
			return &snapshots[i]
		}
	}
	return nil
}

func formatSnapshotDeleteError(stderr string) error {
	message := strings.TrimSpace(stderr)
	if message == "" {
		message = "未知错误"
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "internal snapshot") && strings.Contains(lower, "not the same as disk image currently used by vm") {
		return fmt.Errorf("删除快照失败: 当前 VM 正在使用的磁盘与目标内部快照所在磁盘不一致，libvirt 不能直接删除该内部快照。请先合并当前外部 overlay，或确认当前磁盘链后再重试")
	}
	return fmt.Errorf("删除快照失败: %s", message)
}

func isInternalSnapshotDiskMismatchDeleteError(stderr string) bool {
	lower := strings.ToLower(strings.TrimSpace(stderr))
	return strings.Contains(lower, "internal snapshot") && strings.Contains(lower, "not the same as disk image currently used by vm")
}

func deleteSnapshotMetadataOnly(vmName, snapName, reason string) error {
	if strings.TrimSpace(reason) != "" {
		logger.App.Info("删除快照元数据", "snapshot", snapName, "reason", reason)
	}
	result := utils.ExecCommand("virsh", "snapshot-delete", vmName, snapName, "--metadata")
	if result.Error != nil {
		return fmt.Errorf("删除快照元数据失败: %s", result.Stderr)
	}
	return nil
}

// ensureVMRunning 检查虚拟机状态，如果处于 paused 状态则尝试恢复运行。
// 用于内存快照创建后（尤其是超时恢复场景），确保虚拟机不会停留在 paused 状态。
func ensureVMRunning(vmName string) {
	stateResult := utils.ExecCommand("virsh", "domstate", vmName)
	if stateResult.Error != nil {
		logger.App.Warn("ensureVMRunning: 获取虚拟机状态失败", "vm", vmName, "error", stateResult.Error)
		return
	}
	state := strings.TrimSpace(stateResult.Stdout)
	// paused (saving) 或普通 paused 状态都尝试恢复
	if state == "paused" || strings.HasPrefix(state, "paused") {
		logger.App.Info("虚拟机处于暂停状态，尝试恢复运行", "vm", vmName, "state", state)
		// 等待 libvirt snapshot job 释放锁后重试
		for i := 0; i < 12; i++ {
			time.Sleep(5 * time.Second)
			resumeResult := utils.ExecCommand("virsh", "resume", vmName)
			if resumeResult.Error == nil {
				logger.App.Info("虚拟机已恢复运行", "vm", vmName)
				return
			}
			if strings.Contains(resumeResult.Stderr, "cannot acquire state change lock") {
				logger.App.Info("libvirt 锁仍被占用，5 秒后重试", "vm", vmName)
				continue
			}
			logger.App.Warn("恢复虚拟机运行失败", "vm", vmName, "stderr", resumeResult.Stderr)
			return
		}
		logger.App.Warn("恢复虚拟机运行超时（libvirt 锁持续被占用）", "vm", vmName)
	}
}
