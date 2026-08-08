package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

const (
	windowsRecoveryPartitionTypeGUID = "DE94BBA4-06D1-4D40-A16A-BFD50179D6AC"
	gptBackupReservedSectors         = int64(34)
	defaultPartitionAlignmentSectors = int64(2048)
)

type guestDiskPartition struct {
	Num           int
	StartBytes    int64
	EndBytes      int64
	SizeBytes     int64
	FileSystem    string
	GPTType       string
	GPTGUID       string
	Name          string
	GPTAttributes string
}

type guestDiskLayout struct {
	Device      string
	PartType    string
	SectorSize  int64
	DiskSectors int64
	Partitions  []guestDiskPartition
}

// prepareWindowsSystemDiskExpansion 会先调整分区表，再离线扩展 NTFS。
// 如果 NTFS 状态不允许安全扩容，会让克隆任务失败并提示用户先修复模板。
func prepareWindowsSystemDiskExpansion(ctx context.Context, cloneDisk string, progressFn func(int, string)) error {
	layout, err := inspectGuestDiskLayout(cloneDisk)
	if err != nil {
		return fmt.Errorf("检测 Windows 磁盘分区失败: %v", err)
	}
	if layout.SectorSize <= 0 || layout.DiskSectors <= 0 {
		return fmt.Errorf("Windows 磁盘扇区信息无效")
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

	osPart := layout.findWindowsOSPartition()
	if osPart == nil {
		logger.App.Warn("未找到可扩展的 Windows NTFS 系统分区，跳过离线分区调整", "disk", cloneDisk)
		return nil
	}

	recoveryPart := layout.findRecoveryPartitionAfter(osPart)
	if strings.EqualFold(layout.PartType, "gpt") && recoveryPart != nil && recoveryPart.Num == lastPartition.Num && strings.EqualFold(recoveryPart.FileSystem, "ntfs") {
		progressFn(24, "移动 Windows 恢复分区并扩展系统分区...")
		return moveRecoveryAndExpandWindowsPartition(ctx, cloneDisk, layout, *osPart, *recoveryPart, progressFn)
	}

	if osPart.Num == lastPartition.Num {
		progressFn(24, "扩展 Windows 系统分区...")
		return expandLastWindowsPartition(ctx, cloneDisk, layout, *osPart, progressFn)
	}

	logger.App.Warn("Windows 系统分区后存在非恢复分区，跳过离线分区调整",
		"disk", cloneDisk, "os_part", osPart.Num, "last_part", lastPartition.Num)
	return nil
}

func inspectGuestDiskLayout(diskPath string) (*guestDiskLayout, error) {
	device := "/dev/sda"
	script := fmt.Sprintf(`guestfish --ro -a %s <<'GUESTFISH'
run
echo __PARTTYPE__
part-get-parttype %s
echo __SECTOR_SIZE__
blockdev-getss %s
echo __DISK_SECTORS__
blockdev-getsz %s
echo __PART_LIST__
part-list %s
echo __FILESYSTEMS__
list-filesystems
GUESTFISH`, utils.ShellSingleQuote(diskPath), device, device, device, device)

	// guestfish 检测磁盘属于 IO 操作，不设置自动超时
	result := utils.ExecShellNoTimeout(script)
	if result.Error != nil {
		return nil, fmt.Errorf("%s", result.Stderr)
	}

	layout, err := parseGuestDiskLayout(result.Stdout, device)
	if err != nil {
		return nil, err
	}
	if len(layout.Partitions) == 0 {
		return layout, nil
	}
	if !strings.EqualFold(layout.PartType, "gpt") {
		return layout, nil
	}

	metaScript := buildGuestfishPartitionMetaScript(diskPath, device, layout.Partitions)
	// guestfish 检测磁盘属于 IO 操作，不设置自动超时
	metaResult := utils.ExecShellNoTimeout(metaScript)
	if metaResult.Error != nil {
		return nil, fmt.Errorf("%s", metaResult.Stderr)
	}
	if err := applyGuestDiskPartitionMeta(layout, metaResult.Stdout); err != nil {
		return nil, err
	}

	return layout, nil
}

func buildGuestfishPartitionMetaScript(diskPath, device string, partitions []guestDiskPartition) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("guestfish --ro -a %s <<'GUESTFISH'\nrun\n", utils.ShellSingleQuote(diskPath)))
	for _, part := range partitions {
		b.WriteString(fmt.Sprintf("echo __PART_META_%d__\n", part.Num))
		b.WriteString(fmt.Sprintf("part-get-gpt-type %s %d\n", device, part.Num))
		b.WriteString(fmt.Sprintf("part-get-gpt-guid %s %d\n", device, part.Num))
		b.WriteString(fmt.Sprintf("part-get-name %s %d\n", device, part.Num))
		b.WriteString(fmt.Sprintf("part-get-gpt-attributes %s %d\n", device, part.Num))
	}
	b.WriteString("GUESTFISH")
	return b.String()
}

func parseGuestDiskLayout(output, device string) (*guestDiskLayout, error) {
	layout := &guestDiskLayout{Device: device}
	section := ""
	partitions := make(map[int]*guestDiskPartition)
	fileSystems := make(map[string]string)

	partRe := regexp.MustCompile(`part_(num|start|end|size):\s+([0-9]+)`)
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "__") && strings.HasSuffix(line, "__") {
			section = line
			continue
		}

		switch section {
		case "__PARTTYPE__":
			layout.PartType = line
		case "__SECTOR_SIZE__":
			layout.SectorSize, _ = strconv.ParseInt(line, 10, 64)
		case "__DISK_SECTORS__":
			layout.DiskSectors, _ = strconv.ParseInt(line, 10, 64)
		case "__PART_LIST__":
			matches := partRe.FindStringSubmatch(line)
			if len(matches) != 3 {
				continue
			}
			value, _ := strconv.ParseInt(matches[2], 10, 64)
			if matches[1] == "num" {
				partitions[int(value)] = &guestDiskPartition{Num: int(value)}
				continue
			}
			part := latestPartition(partitions)
			if part == nil {
				continue
			}
			switch matches[1] {
			case "start":
				part.StartBytes = value
			case "end":
				part.EndBytes = value
			case "size":
				part.SizeBytes = value
			}
		case "__FILESYSTEMS__":
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				fileSystems[strings.TrimSuffix(fields[0], ":")] = strings.ToLower(fields[1])
			}
		}
	}

	for _, part := range partitions {
		part.FileSystem = fileSystems[fmt.Sprintf("%s%d", device, part.Num)]
		layout.Partitions = append(layout.Partitions, *part)
	}
	sort.Slice(layout.Partitions, func(i, j int) bool {
		return layout.Partitions[i].StartBytes < layout.Partitions[j].StartBytes
	})

	return layout, nil
}

func latestPartition(partitions map[int]*guestDiskPartition) *guestDiskPartition {
	var latest *guestDiskPartition
	for _, part := range partitions {
		if latest == nil || part.Num > latest.Num {
			latest = part
		}
	}
	return latest
}

func applyGuestDiskPartitionMeta(layout *guestDiskLayout, output string) error {
	currentPart := 0
	currentValueIndex := 0
	metaMarkerRe := regexp.MustCompile(`^__PART_META_([0-9]+)__$`)

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" && currentPart == 0 {
			continue
		}
		if matches := metaMarkerRe.FindStringSubmatch(line); len(matches) == 2 {
			currentPart, _ = strconv.Atoi(matches[1])
			currentValueIndex = 0
			continue
		}
		if currentPart == 0 {
			continue
		}

		part := layout.partitionByNum(currentPart)
		if part == nil {
			continue
		}
		switch currentValueIndex {
		case 0:
			part.GPTType = strings.ToUpper(line)
		case 1:
			part.GPTGUID = strings.ToUpper(line)
		case 2:
			part.Name = line
		case 3:
			part.GPTAttributes = line
		}
		currentValueIndex++
	}

	return nil
}

func (layout *guestDiskLayout) partitionByNum(num int) *guestDiskPartition {
	for i := range layout.Partitions {
		if layout.Partitions[i].Num == num {
			return &layout.Partitions[i]
		}
	}
	return nil
}

func (layout *guestDiskLayout) lastPartition() *guestDiskPartition {
	if len(layout.Partitions) == 0 {
		return nil
	}
	return &layout.Partitions[len(layout.Partitions)-1]
}

func (layout *guestDiskLayout) lastUsableSector() int64 {
	if strings.EqualFold(layout.PartType, "gpt") {
		return layout.DiskSectors - gptBackupReservedSectors
	}
	return layout.DiskSectors - 1
}

func (layout *guestDiskLayout) findWindowsOSPartition() *guestDiskPartition {
	var selected *guestDiskPartition
	for i := range layout.Partitions {
		part := &layout.Partitions[i]
		if !strings.EqualFold(part.FileSystem, "ntfs") || isWindowsRecoveryPartition(part) {
			continue
		}
		if selected == nil || part.SizeBytes > selected.SizeBytes {
			selected = part
		}
	}
	return selected
}

func (layout *guestDiskLayout) findRecoveryPartitionAfter(osPart *guestDiskPartition) *guestDiskPartition {
	for i := range layout.Partitions {
		part := &layout.Partitions[i]
		if part.StartBytes <= osPart.EndBytes {
			continue
		}
		if isWindowsRecoveryPartition(part) {
			return part
		}
	}
	return nil
}

func isWindowsRecoveryPartition(part *guestDiskPartition) bool {
	return strings.EqualFold(part.GPTType, windowsRecoveryPartitionTypeGUID) ||
		strings.Contains(strings.ToLower(part.Name), "recovery")
}

func moveRecoveryAndExpandWindowsPartition(ctx context.Context, diskPath string, layout *guestDiskLayout, osPart, recoveryPart guestDiskPartition, progressFn func(int, string)) error {
	lastUsableSector := layout.lastUsableSector()
	recoverySizeSectors := bytesToSectorsCeil(recoveryPart.SizeBytes, layout.SectorSize)
	newRecoveryStart := alignDown(lastUsableSector-recoverySizeSectors+1, defaultPartitionAlignmentSectors)
	newOSEnd := newRecoveryStart - 1
	if newOSEnd <= bytesToSectorEnd(osPart.EndBytes, layout.SectorSize) {
		return nil
	}

	backupPath := filepath.Join("/tmp", fmt.Sprintf("_kvm-console-recovery-%d.ntfsclone", time.Now().UnixNano()))
	var commands []string
	commands = append(commands,
		"run",
		fmt.Sprintf("part-expand-gpt %s", layout.Device),
		fmt.Sprintf("ntfsclone-out %s%d %s force:true", layout.Device, recoveryPart.Num, backupPath),
		fmt.Sprintf("part-del %s %d", layout.Device, recoveryPart.Num),
		fmt.Sprintf("part-resize %s %d %d", layout.Device, osPart.Num, newOSEnd),
		fmt.Sprintf("part-add %s p %d -%d", layout.Device, newRecoveryStart, gptBackupReservedSectors),
		fmt.Sprintf("part-set-gpt-type %s %d %s", layout.Device, recoveryPart.Num, coalesceGUID(recoveryPart.GPTType, windowsRecoveryPartitionTypeGUID)),
	)
	if recoveryPart.GPTGUID != "" {
		commands = append(commands, fmt.Sprintf("part-set-gpt-guid %s %d %s", layout.Device, recoveryPart.Num, recoveryPart.GPTGUID))
	}
	if recoveryPart.Name != "" {
		commands = append(commands, fmt.Sprintf("part-set-name %s %d %s", layout.Device, recoveryPart.Num, guestfishQuote(recoveryPart.Name)))
	}
	if recoveryPart.GPTAttributes != "" {
		commands = append(commands, fmt.Sprintf("part-set-gpt-attributes %s %d %s", layout.Device, recoveryPart.Num, recoveryPart.GPTAttributes))
	}
	commands = append(commands,
		fmt.Sprintf("blockdev-rereadpt %s", layout.Device),
		fmt.Sprintf("ntfsclone-in %s %s%d", backupPath, layout.Device, recoveryPart.Num),
		fmt.Sprintf("ntfsfix %s%d", layout.Device, recoveryPart.Num),
	)

	if err := runWritableGuestfish(ctx, diskPath, commands, []string{backupPath}); err != nil {
		return err
	}

	progressFn(26, "扩展 Windows NTFS 文件系统...")
	return runWritableGuestfish(ctx, diskPath, []string{
		"run",
		fmt.Sprintf("debug sh %s", guestfishQuote(fmt.Sprintf("ntfsresize -f %s%d", layout.Device, osPart.Num))),
	}, nil)
}

func expandLastWindowsPartition(ctx context.Context, diskPath string, layout *guestDiskLayout, osPart guestDiskPartition, progressFn func(int, string)) error {
	lastUsableSector := layout.lastUsableSector()
	currentEnd := bytesToSectorEnd(osPart.EndBytes, layout.SectorSize)
	if lastUsableSector <= currentEnd {
		return nil
	}
	if err := runWritableGuestfish(ctx, diskPath, buildExpandLastWindowsPartitionCommands(layout, osPart, lastUsableSector), nil); err != nil {
		return err
	}

	progressFn(26, "扩展 Windows NTFS 文件系统...")
	return runWritableGuestfish(ctx, diskPath, []string{
		"run",
		fmt.Sprintf("debug sh %s", guestfishQuote(fmt.Sprintf("ntfsresize -f %s%d", layout.Device, osPart.Num))),
	}, nil)
}

func buildExpandLastWindowsPartitionCommands(layout *guestDiskLayout, osPart guestDiskPartition, lastUsableSector int64) []string {
	commands := []string{"run"}
	if strings.EqualFold(layout.PartType, "gpt") {
		commands = append(commands, fmt.Sprintf("part-expand-gpt %s", layout.Device))
	}
	commands = append(commands,
		fmt.Sprintf("part-resize %s %d %d", layout.Device, osPart.Num, lastUsableSector),
		fmt.Sprintf("blockdev-rereadpt %s", layout.Device),
	)
	return commands
}

func runWritableGuestfish(ctx context.Context, diskPath string, commands []string, cleanupPaths []string) error {
	return runWritableGuestfishOperation(
		ctx,
		diskPath,
		commands,
		cleanupPaths,
		"Windows 磁盘扩容",
	)
}

func runWritableGuestfishOperation(ctx context.Context, diskPath string, commands []string, cleanupPaths []string, operationName string) error {
	var b strings.Builder
	b.WriteString("set +e\n")
	b.WriteString(fmt.Sprintf("guestfish -a %s <<'GUESTFISH'\n", utils.ShellSingleQuote(diskPath)))
	for _, command := range commands {
		b.WriteString(command)
		b.WriteByte('\n')
	}
	b.WriteString("GUESTFISH\n")
	b.WriteString("guestfish_status=$?\n")
	for _, cleanupPath := range cleanupPaths {
		b.WriteString(fmt.Sprintf("rm -f %s\n", utils.ShellSingleQuote(cleanupPath)))
	}
	b.WriteString("exit $guestfish_status\n")

	// guestfish 磁盘扩容属于大 IO 操作，不设置自动超时，仅响应任务取消
	result := utils.ExecShellContext(ctx, b.String())
	if result.Error != nil {
		for _, cleanupPath := range cleanupPaths {
			_ = os.Remove(cleanupPath)
		}
		if ctx != nil && ctx.Err() != nil {
			return fmt.Errorf("%s已取消", operationName)
		}
		return fmt.Errorf("%s", result.Stderr)
	}
	return nil
}

func bytesToSectorsCeil(bytes, sectorSize int64) int64 {
	if sectorSize <= 0 {
		return 0
	}
	return (bytes + sectorSize - 1) / sectorSize
}

func bytesToSectorEnd(endByte, sectorSize int64) int64 {
	if sectorSize <= 0 {
		return 0
	}
	return endByte / sectorSize
}

func alignDown(value, alignment int64) int64 {
	if alignment <= 0 {
		return value
	}
	return value - value%alignment
}

func coalesceGUID(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func guestfishQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
