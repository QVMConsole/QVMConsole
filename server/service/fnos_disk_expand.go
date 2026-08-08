package service

import (
	"context"
	"fmt"
	"strings"

	"kvm_console/logger"
)

// prepareFnOSSystemDiskExpansion 会在 FnOS 首次开机前离线扩展最后一个 ext 系统分区。
func prepareFnOSSystemDiskExpansion(ctx context.Context, cloneDisk string, progressFn func(int, string)) error {
	layout, err := inspectGuestDiskLayout(cloneDisk)
	if err != nil {
		return fmt.Errorf("检测 FnOS 磁盘分区失败: %v", err)
	}
	if layout.SectorSize <= 0 || layout.DiskSectors <= 0 {
		return fmt.Errorf("FnOS 磁盘扇区信息无效")
	}

	lastPartition := layout.lastPartition()
	if lastPartition == nil {
		return nil
	}
	lastUsableSector := layout.lastUsableSector()
	lastPartitionEndSector := bytesToSectorEnd(lastPartition.EndBytes, layout.SectorSize)
	if lastUsableSector-lastPartitionEndSector < defaultPartitionAlignmentSectors {
		return nil
	}

	systemPart := layout.findFnOSSystemPartition()
	if systemPart == nil {
		logger.App.Warn("未找到可扩展的 FnOS ext 系统分区，跳过离线分区调整", "disk", cloneDisk)
		return nil
	}
	if systemPart.Num != lastPartition.Num {
		return fmt.Errorf("FnOS 系统分区后存在其他分区，无法安全自动扩容")
	}

	progressFn(24, "扩展 FnOS 系统分区...")
	if err := runWritableGuestfishOperation(
		ctx,
		cloneDisk,
		buildFnOSSystemDiskExpansionCommands(layout, *systemPart, lastUsableSector),
		nil,
		"FnOS 系统分区扩容",
	); err != nil {
		return fmt.Errorf("FnOS 系统分区扩容失败: %v", err)
	}

	progressFn(26, "FnOS 系统分区扩容完成")
	return nil
}

func (layout *guestDiskLayout) findFnOSSystemPartition() *guestDiskPartition {
	var selected *guestDiskPartition
	for i := range layout.Partitions {
		part := &layout.Partitions[i]
		if !isExtFilesystem(part.FileSystem) {
			continue
		}
		if selected == nil || part.SizeBytes > selected.SizeBytes {
			selected = part
		}
	}
	return selected
}

func isExtFilesystem(fs string) bool {
	switch strings.ToLower(strings.TrimSpace(fs)) {
	case "ext2", "ext3", "ext4":
		return true
	default:
		return false
	}
}

func buildFnOSSystemDiskExpansionCommands(layout *guestDiskLayout, systemPart guestDiskPartition, lastUsableSector int64) []string {
	partDev := fmt.Sprintf("%s%d", layout.Device, systemPart.Num)
	commands := []string{"run"}
	if strings.EqualFold(layout.PartType, "gpt") {
		commands = append(commands, fmt.Sprintf("part-expand-gpt %s", layout.Device))
	}
	commands = append(commands,
		fmt.Sprintf("part-resize %s %d %d", layout.Device, systemPart.Num, lastUsableSector),
		fmt.Sprintf("blockdev-rereadpt %s", layout.Device),
		fmt.Sprintf("e2fsck-f %s", partDev),
		fmt.Sprintf("resize2fs %s", partDev),
	)
	return commands
}
