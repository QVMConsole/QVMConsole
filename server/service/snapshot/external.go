package snapshot

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// getExternalSnapshotDiskFiles 获取外部快照关联的磁盘文件列表
func getExternalSnapshotDiskFiles(vmName, snapName string) ([]string, error) {
	result := utils.ExecCommand("virsh", "snapshot-dumpxml", vmName, snapName)
	if result.Error != nil {
		return nil, fmt.Errorf("获取快照XML失败: %s", result.Stderr)
	}

	files, err := parseExternalSnapshotDiskFiles(result.Stdout)
	if err != nil {
		return nil, err
	}
	return files, nil
}

func parseExternalSnapshotDiskFiles(snapshotXML string) ([]string, error) {
	var doc externalSnapshotXML
	if err := xml.Unmarshal([]byte(snapshotXML), &doc); err != nil {
		return nil, fmt.Errorf("解析快照XML失败: %w", err)
	}

	var files []string
	seen := make(map[string]struct{})
	for _, disk := range doc.Disks {
		file := strings.TrimSpace(disk.Source.File)
		if file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		files = append(files, file)
	}
	return files, nil
}

func parseExternalSnapshotOriginalDiskFiles(snapshotXML string) (map[string]string, error) {
	var doc externalSnapshotXML
	if err := xml.Unmarshal([]byte(snapshotXML), &doc); err != nil {
		return nil, fmt.Errorf("解析快照XML失败: %w", err)
	}

	files := make(map[string]string)
	for _, disk := range doc.Domain.Devices.Disks {
		target := strings.TrimSpace(disk.Target.Dev)
		file := strings.TrimSpace(disk.Source.File)
		if target == "" || file == "" {
			continue
		}
		files[target] = file
	}
	return files, nil
}

func ensureInternalSnapshotRestoreDiskAccess(vmName, snapName string) error {
	diskPaths, err := getCurrentVMDiskSourcePaths(vmName)
	if err != nil {
		return err
	}
	snapshotDiskPaths, err := getSnapshotDomainDiskPaths(vmName, snapName)
	if err != nil {
		logger.App.Warn("获取快照磁盘配置失败", "snapshot", snapName, "error", err)
	}
	diskPaths = append(diskPaths, snapshotDiskPaths...)
	if err := ensureSnapshotDiskAccessForPaths(diskPaths); err != nil {
		return fmt.Errorf("修复快照磁盘访问权限失败: %w", err)
	}
	return nil
}

func getSnapshotDomainDiskPaths(vmName, snapName string) ([]string, error) {
	result := utils.ExecCommand("virsh", "snapshot-dumpxml", vmName, snapName)
	if result.Error != nil {
		return nil, fmt.Errorf("获取快照 XML 失败: %s", result.Stderr)
	}
	files, err := parseExternalSnapshotOriginalDiskFiles(result.Stdout)
	if err != nil {
		return nil, err
	}
	var diskPaths []string
	for _, file := range files {
		diskPaths = append(diskPaths, file)
	}
	return diskPaths, nil
}

// revertExternalSnapshot 恢复外部快照
// 外部快照不能直接用 virsh snapshot-revert，需要手动操作 qcow2 链
func revertExternalSnapshot(vmName, snapName string) error {
	// 1. 获取要恢复的快照的磁盘文件（这些是快照创建时的 overlay 文件）
	snapFiles, err := getExternalSnapshotDiskFiles(vmName, snapName)
	if err != nil {
		return fmt.Errorf("获取快照磁盘信息失败: %w", err)
	}
	if len(snapFiles) == 0 {
		return fmt.Errorf("快照 %s 没有关联的磁盘文件", snapName)
	}

	// 2. 获取快照 XML 中记录的原始磁盘信息（快照创建时 VM 使用的磁盘）
	// 快照 XML 的 <domain> 部分记录了创建快照时 VM 的配置
	snapXmlResult := utils.ExecCommand("virsh", "snapshot-dumpxml", vmName, snapName)
	if snapXmlResult.Error != nil {
		return fmt.Errorf("获取快照XML失败: %s", snapXmlResult.Stderr)
	}

	originalDisks, err := parseExternalSnapshotOriginalDiskFiles(snapXmlResult.Stdout)
	if err != nil {
		return err
	}
	if len(originalDisks) == 0 {
		return fmt.Errorf("无法从快照XML域配置中提取原始磁盘路径")
	}

	// 3. 检查虚拟机是否在运行，需要先关机
	vmState := utils.ExecCommand("virsh", "domstate", vmName)
	if vmState.Error == nil && strings.TrimSpace(vmState.Stdout) == "running" {
		destroyResult := utils.ExecCommand("virsh", "destroy", vmName)
		if destroyResult.Error != nil {
			logger.App.Warn("关闭虚拟机失败，继续尝试快照恢复", "vm", vmName, "stderr", destroyResult.Stderr)
		}
	}

	// 4. 获取当前 VM 的磁盘列表（当前指向的文件）
	currentDisks := utils.ExecCommand("virsh", "domblklist", vmName, "--details")
	if currentDisks.Error != nil {
		return fmt.Errorf("获取当前磁盘列表失败: %s", currentDisks.Stderr)
	}

	// 解析当前磁盘: type  device  target  source
	type diskInfo struct {
		target string
		source string
	}
	var currentDiskList []diskInfo
	for _, line := range strings.Split(currentDisks.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Type") || strings.HasPrefix(line, "-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[1] == "disk" {
			currentDiskList = append(currentDiskList, diskInfo{target: fields[2], source: fields[3]})
		}
	}

	// 5. 恢复外部快照前校验磁盘链完整性，防止损坏的 backing chain 导致恢复失败或数据丢失。
	for _, diskPath := range originalDisks {
		if err := HookValidateDiskBackingChain(diskPath); err != nil {
			return fmt.Errorf("磁盘链完整性校验失败，无法恢复快照: %w", err)
		}
	}

	// 6. 恢复外部快照的语义是切回快照创建时的磁盘状态。
	// 不能执行 qemu-img commit，否则会把快照后的写入合并回 backing，导致没有真正回滚。
	for _, snapFile := range snapFiles {
		if _, err := os.Stat(snapFile); os.IsNotExist(err) {
			return fmt.Errorf("快照增量文件不存在，无法确认恢复链: %s", snapFile)
		}
	}

	// 7. 为每块恢复目标盘创建新的可写 overlay。
	// 外部快照的恢复点必须只作为 backing 使用，不能让 VM 直接写入恢复点文件，
	// 否则从早期快照恢复后再创建分支快照时，会污染这个早期快照。
	restoreDisks := make(map[string]string, len(originalDisks))
	createdRestoreOverlays := []string{}
	for _, disk := range currentDiskList {
		originalDisk, ok := originalDisks[disk.target]
		if !ok {
			continue
		}
		if _, err := os.Stat(originalDisk); os.IsNotExist(err) {
			return fmt.Errorf("原始磁盘文件不存在: %s", originalDisk)
		}
		restoreOverlay := generateExternalSnapshotRestoreOverlayPath(originalDisk, vmName, disk.target, snapName)
		if err := createExternalSnapshotRestoreOverlay(originalDisk, restoreOverlay); err != nil {
			for _, created := range createdRestoreOverlays {
				_ = os.Remove(created)
			}
			return err
		}
		restoreDisks[disk.target] = restoreOverlay
		createdRestoreOverlays = append(createdRestoreOverlays, restoreOverlay)
		logger.App.Info("快照恢复磁盘信息", "device", disk.target, "backing", originalDisk, "overlay", restoreOverlay)
	}

	// 使用 sed 方式批量修改 XML（通过 EDITOR 环境变量）
	// 构建 sed 命令来替换磁盘路径
	sedParts := []string{}
	for _, disk := range currentDiskList {
		restoreDisk, ok := restoreDisks[disk.target]
		if ok && disk.source != restoreDisk {
			// 转义路径中的特殊字符
			escapedOld := strings.ReplaceAll(disk.source, "/", "\\/")
			escapedOld = strings.ReplaceAll(escapedOld, ".", "\\.")
			escapedNew := strings.ReplaceAll(restoreDisk, "/", "\\/")
			sedParts = append(sedParts, fmt.Sprintf("s|%s|%s|g", escapedOld, escapedNew))
		}
	}

	if len(sedParts) > 0 {
		sedCmd := strings.Join(sedParts, "; ")
		shellCmd := fmt.Sprintf("EDITOR=\"sed -i '%s'\" virsh edit %s", sedCmd, utils.ShellSingleQuote(vmName))
		editResult := utils.ExecShell(shellCmd)
		if editResult.Error != nil {
			for _, created := range createdRestoreOverlays {
				_ = os.Remove(created)
			}
			return fmt.Errorf("修改虚拟机磁盘配置失败: %s", editResult.Stderr)
		}
	}

	// 8. 清理 VM XML 中可能残留的 backingStore 自引用
	// 检查并移除 backingStore 中指向自己的情况
	dumpResult := utils.ExecCommand("virsh", "dumpxml", vmName)
	if dumpResult.Error == nil {
		hasBackingStore := strings.Contains(dumpResult.Stdout, "<backingStore")
		if hasBackingStore {
			shellCmd := fmt.Sprintf("EDITOR=\"sed -i '/<backingStore type/,/<\\/backingStore>/d'\" virsh edit %s", utils.ShellSingleQuote(vmName))
			cleanResult := utils.ExecShell(shellCmd)
			if cleanResult.Error != nil {
				logger.App.Warn("清理 backingStore 失败", "stderr", cleanResult.Stderr)
			}
		}
	}

	// 9. 启动前主动修复恢复后磁盘和快照 overlay 的访问权限。
	// 自定义存储池路径还需要 AppArmor 允许 virt-aa-helper 读取 backing chain。
	restoredDiskPaths := make([]string, 0, len(originalDisks)+len(restoreDisks)+len(snapFiles))
	for _, diskPath := range originalDisks {
		restoredDiskPaths = append(restoredDiskPaths, diskPath)
	}
	for _, diskPath := range restoreDisks {
		restoredDiskPaths = append(restoredDiskPaths, diskPath)
	}
	restoredDiskPaths = append(restoredDiskPaths, snapFiles...)
	if err := ensureSnapshotDiskAccessForPaths(restoredDiskPaths); err != nil {
		return fmt.Errorf("修复快照磁盘访问权限失败: %w", err)
	}

	// 10. 恢复外部快照不自动清理快照元数据或 overlay 文件，避免恢复成功后快照列表被清空。
	// 同步 libvirt 当前快照指针，否则从内部快照恢复到外部快照后，快照树仍会显示父级内部快照为当前。
	currentResult := utils.ExecCommand("virsh", "snapshot-current", vmName, snapName)
	if currentResult.Error != nil {
		return fmt.Errorf("设置当前快照标记失败: %s", currentResult.Stderr)
	}

	// 11. 启动虚拟机
	if err := HookStartVM(vmName); err != nil {
		return fmt.Errorf("恢复快照后启动虚拟机失败: %s，请检查虚拟机配置", err.Error())
	}

	logger.App.Info("虚拟机已成功恢复到快照状态并启动", "vm", vmName, "snapshot", snapName)
	return nil
}

func createExternalSnapshotRestoreOverlay(backingPath, overlayPath string) error {
	if strings.TrimSpace(backingPath) == "" || strings.TrimSpace(overlayPath) == "" {
		return fmt.Errorf("恢复 overlay 或 backing 路径为空")
	}
	if err := os.MkdirAll(path.Dir(overlayPath), 0755); err != nil {
		return fmt.Errorf("创建恢复 overlay 目录失败: %w", err)
	}
	backingFormat := vm_xml.DetectQemuImageFormat(backingPath)
	if backingFormat == "" {
		backingFormat = "qcow2"
	}
	result := utils.ExecCommandLongRunning("qemu-img", "create", "-f", "qcow2", "-F", backingFormat, "-b", backingPath, overlayPath)
	if result.Error != nil {
		_ = os.Remove(overlayPath)
		return fmt.Errorf("创建恢复 overlay 失败: %s", result.Stderr)
	}
	if err := ensureSnapshotDiskAccessForPaths([]string{backingPath, overlayPath}); err != nil {
		_ = os.Remove(overlayPath)
		return err
	}
	return nil
}

func generateExternalSnapshotRestoreOverlayPath(backingPath, vmName, target, snapName string) string {
	dir := path.Dir(backingPath)
	base := path.Base(backingPath)
	ext := path.Ext(base)
	name := base
	if ext != "" {
		name = strings.TrimSuffix(base, ext)
	}
	token := make([]byte, 4)
	if _, err := rand.Read(token); err != nil {
		token = []byte(time.Now().Format("150405"))
	}
	return path.Join(dir, fmt.Sprintf(
		"%s.snap_restore_%s_%s_%s_%s.qcow2",
		name,
		sanitizeSnapshotPathPart(vmName),
		sanitizeSnapshotPathPart(target),
		sanitizeSnapshotPathPart(snapName),
		hex.EncodeToString(token),
	))
}

func sanitizeSnapshotPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(value, "_")
}
