package snapshot

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/utils"
)

// fixSnapshotDiskPermissions 修正外部快照创建后的权限和 XML 配置问题
// 1. 修正 overlay 文件权限（libvirt 创建的默认归 root:root，QEMU 无法读取）
// 2. 清理 VM XML 中不完整的 backingStore 标签（避免 AppArmor 无法遍历完整 backing chain）
func FixSnapshotDiskPermissions(vmName string) {
	diskPaths, err := getCurrentVMDiskSourcePaths(vmName)
	if err != nil {
		logger.App.Warn("快照权限修正", "error", err)
		return
	}
	if err := ensureSnapshotDiskAccessForPaths(diskPaths); err != nil {
		logger.App.Error("修复磁盘访问权限失败", "error", err)
	}

	// 清理 VM XML 中不完整的 backingStore 标签
	// 创建外部快照后 libvirt 会在 XML 中写入一层 backingStore，但 backing chain 可能更深，
	// 导致 virt-aa-helper 只为第一层 backing file 生成 AppArmor 规则，
	// 最深层的模板文件不在白名单中，QEMU 启动时访问被 AppArmor 拒绝。
	// 清理后让 libvirt 在下次启动时自动检测并生成完整的 backing chain 权限
	dumpResult := utils.ExecCommand("virsh", "dumpxml", vmName)
	if dumpResult.Error == nil && strings.Contains(dumpResult.Stdout, "<backingStore") {
		shellCmd := fmt.Sprintf("EDITOR=\"sed -i '/<backingStore type/,/<\\/backingStore>/d'\" virsh edit %s", utils.ShellSingleQuote(vmName))
		cleanResult := utils.ExecShell(shellCmd)
		if cleanResult.Error != nil {
			logger.App.Warn("清理 backingStore XML 失败", "stderr", cleanResult.Stderr)
		} else {
			logger.App.Info("已清理 backingStore XML", "vm", vmName)
		}
	}
}

func getCurrentVMDiskSources(vmName string) ([]vmDiskSource, error) {
	blkResult := utils.ExecCommand("virsh", "domblklist", vmName, "--details")
	if blkResult.Error != nil {
		return nil, fmt.Errorf("获取磁盘列表失败: %s", blkResult.Stderr)
	}

	var diskSources []vmDiskSource
	for _, line := range strings.Split(blkResult.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Type") || strings.HasPrefix(line, "-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[1] == "disk" {
			diskSources = append(diskSources, vmDiskSource{Target: fields[2], Source: fields[3]})
		}
	}
	return diskSources, nil
}

func getCurrentVMDiskSourcePaths(vmName string) ([]string, error) {
	diskSources, err := getCurrentVMDiskSources(vmName)
	if err != nil {
		return nil, err
	}
	var diskPaths []string
	for _, disk := range diskSources {
		if disk.Source == "" || disk.Source == "-" {
			continue
		}
		diskPaths = append(diskPaths, disk.Source)
	}
	return diskPaths, nil
}

func ensureSnapshotDiskAccessForPaths(diskPaths []string) error {
	// 记录主路径（调用方原始传入的路径），用于区分 backing chain 扩展出的路径
	primaryPaths := make(map[string]bool)
	for _, p := range diskPaths {
		if p != "" && p != "-" {
			primaryPaths[p] = true
		}
	}

	diskPaths = expandDiskPathsWithBackingChain(diskPaths)
	if err := EnsureLibvirtStorageAppArmorAccessForPaths(diskPaths); err != nil {
		return err
	}

	for _, diskPath := range uniqueNonEmptyStrings(diskPaths) {
		if diskPath == "" || diskPath == "-" {
			continue
		}
		if _, err := os.Stat(diskPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("检查磁盘文件失败 %s: %w", diskPath, err)
		}
		// 修正文件权限为 libvirt-qemu:kvm，避免切回原始盘或 overlay 后 QEMU 无法访问。
		if err := utils.ChownLibvirtQEMU(diskPath); err != nil {
			// 对 backing chain 中非主路径的文件（如模板），chown 失败不阻断操作：
			// 这些文件在部署时已有正确权限，只是当前系统用户/组命名不匹配
			if primaryPaths[diskPath] {
				return fmt.Errorf("chown %s 失败: %w", diskPath, err)
			}
			logger.App.Warn("backing chain 文件 chown 失败（已跳过，文件应已有正确权限）", "path", diskPath, "error", err)
		} else {
			logger.App.Info("已修正磁盘权限", "path", diskPath)
		}
	}
	return nil
}

func expandDiskPathsWithBackingChain(diskPaths []string) []string {
	expanded := append([]string{}, uniqueNonEmptyStrings(diskPaths)...)
	for _, diskPath := range uniqueNonEmptyStrings(diskPaths) {
		if _, err := os.Stat(diskPath); err != nil {
			continue
		}
		chain, err := HookQemuInfoChain(diskPath)
		if err != nil {
			logger.App.Warn("读取磁盘 backing chain 失败", "path", diskPath, "error", err)
			continue
		}
		for _, item := range chain {
			expanded = append(expanded, item.Filename)
			expanded = append(expanded, item.FullBackingFilename)
			if path.IsAbs(item.BackingFilename) {
				expanded = append(expanded, item.BackingFilename)
			}
		}
	}
	return uniqueNonEmptyStrings(expanded)
}

// 快照残留文件清理

func cleanupSnapshotResidualFiles(vmName, snapName string, allSnapshots bool) error {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return nil
	}
	protected := collectSnapshotCleanupProtectedPaths(vmName)
	var removeErrors []string
	for _, root := range snapshotCleanupRoots() {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			base := path.Base(filepath.ToSlash(filePath))
			if !isManagedSnapshotResidualFileName(vmName, snapName, base, allSnapshots) {
				return nil
			}
			cleanPath := normalizeSnapshotFilePath(filepath.ToSlash(filePath))
			if protected[cleanPath] {
				logger.App.Info("跳过仍被引用的文件", "path", cleanPath)
				return nil
			}
			if err := os.Remove(filePath); err != nil {
				if !os.IsNotExist(err) {
					removeErrors = append(removeErrors, fmt.Sprintf("%s: %v", cleanPath, err))
				}
				return nil
			}
			logger.App.Info("已删除残留文件", "path", cleanPath)
			return nil
		})
		if err != nil {
			removeErrors = append(removeErrors, fmt.Sprintf("%s: %v", root, err))
		}
	}
	if len(removeErrors) > 0 {
		return fmt.Errorf("%s", strings.Join(removeErrors, "; "))
	}
	return nil
}

func snapshotCleanupRoots() []string {
	return uniqueNonEmptyStrings([]string{
		"/var/lib/libvirt/images",
		HostStorageRoot,
	})
}

func isManagedSnapshotResidualFileName(vmName, snapName, fileName string, allSnapshots bool) bool {
	if !strings.Contains(fileName, vmName) {
		return false
	}
	if !strings.Contains(fileName, ".snap_") && !strings.Contains(fileName, "-snap_") {
		return false
	}
	if allSnapshots || strings.TrimSpace(snapName) == "" {
		return true
	}
	return strings.Contains(fileName, snapName)
}

func collectSnapshotCleanupProtectedPaths(vmName string) map[string]bool {
	protected := make(map[string]bool)
	addProtectedPaths(protected, getCurrentVMDiskSourcePathsOrEmpty(vmName))
	addProtectedPaths(protected, extractCurrentQEMUBlockPaths(vmName))

	dumpResult := utils.ExecCommand("virsh", "dumpxml", vmName)
	if dumpResult.Error == nil {
		addProtectedPaths(protected, extractSourceFilePathsFromXML(dumpResult.Stdout))
	}

	snapshots, err := ListSnapshots(vmName)
	if err == nil {
		for _, snap := range snapshots {
			result := utils.ExecCommand("virsh", "snapshot-dumpxml", vmName, snap.Name)
			if result.Error != nil {
				continue
			}
			addProtectedPaths(protected, extractSourceFilePathsFromXML(result.Stdout))
		}
	}
	return protected
}

func getCurrentVMDiskSourcePathsOrEmpty(vmName string) []string {
	diskPaths, err := getCurrentVMDiskSourcePaths(vmName)
	if err != nil {
		logger.App.Warn("获取当前磁盘路径失败", "error", err)
		return nil
	}
	return diskPaths
}

func addProtectedPaths(protected map[string]bool, diskPaths []string) {
	for _, diskPath := range expandDiskPathsWithBackingChain(diskPaths) {
		diskPath = normalizeSnapshotFilePath(diskPath)
		if diskPath == "" {
			continue
		}
		protected[diskPath] = true
	}
}

func extractCurrentQEMUBlockPaths(vmName string) []string {
	// 残留文件清理期间虚拟机可能已经关机，Monitor 查询失败属于预期情况。
	result := utils.ExecCommandQuiet("virsh", "qemu-monitor-command", vmName, "--hmp", "info block")
	if result.Error != nil {
		return nil
	}
	return extractAbsoluteFilePaths(result.Stdout)
}

func extractSourceFilePathsFromXML(content string) []string {
	matches := regexp.MustCompile(`<source\b[^>]*\bfile=['"]([^'"]+)['"]`).FindAllStringSubmatch(content, -1)
	files := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			files = append(files, strings.TrimSpace(match[1]))
		}
	}
	return uniqueNonEmptyStrings(files)
}

func extractAbsoluteFilePaths(content string) []string {
	matches := regexp.MustCompile(`/[A-Za-z0-9._~+%:=,@/-]+`).FindAllString(content, -1)
	return uniqueNonEmptyStrings(matches)
}

func normalizeSnapshotFilePath(filePath string) string {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" || filePath == "-" {
		return ""
	}
	return path.Clean(filepath.ToSlash(filePath))
}

// AppArmor 磁盘访问管理

func EnsureLibvirtStorageAppArmorAccessForPaths(diskPaths []string) error {
	roots := managedLibvirtAccessRootsForPaths(diskPaths)
	if len(roots) == 0 {
		return nil
	}
	return ensureLibvirtStorageAppArmorAccess(roots)
}

func isManagedHostStoragePath(item string) bool {
	item = strings.TrimSpace(item)
	if item == "" {
		return false
	}
	return isPathWithinRoot(item, HostStorageRoot)
}

func managedLibvirtAccessRootsForPaths(paths []string) []string {
	var roots []string
	knownRoots := managedLibvirtAccessRoots()
	for _, item := range uniqueNonEmptyStrings(paths) {
		matchedKnownRoot := false
		for _, root := range knownRoots {
			if !isPathWithinRoot(item, root) {
				continue
			}
			roots = append(roots, root)
			matchedKnownRoot = true
			break
		}
		if matchedKnownRoot {
			continue
		}

		// 管理员可以把已有分区直接挂载到 /data、/data2 等任意路径。
		// 这类存储池不在固定的 HostStorageRoot 下，需要从实际磁盘路径推导
		// 最小访问目录，避免 virt-aa-helper 无法读取无扩展名的外部快照层。
		if root := deriveLibvirtAccessRoot(item); root != "" {
			roots = append(roots, root)
		}
	}
	return uniqueNonEmptyStrings(roots)
}

func deriveLibvirtAccessRoot(item string) string {
	cleanPath := path.Clean(strings.TrimSpace(item))
	if cleanPath == "." || !path.IsAbs(cleanPath) {
		return ""
	}
	if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
		if cleanPath == "/" {
			return ""
		}
		return cleanPath
	}

	parent := path.Dir(cleanPath)
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}

func managedLibvirtAccessRoots() []string {
	roots := []string{HostStorageRoot}
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.TemplateDir) != "" {
		roots = append(roots, config.GlobalConfig.TemplateDir)
	} else {
		roots = append(roots, "/var/lib/libvirt/images/templates")
	}
	return uniqueNonEmptyStrings(roots)
}

func isPathWithinRoot(item, root string) bool {
	item = strings.TrimSpace(item)
	root = strings.TrimSpace(root)
	if item == "" || root == "" {
		return false
	}
	cleanPath := path.Clean(item)
	cleanRoot := path.Clean(root)
	return cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+"/")
}

func ensureLibvirtStorageAppArmorAccess(roots []string) error {
	if _, err := os.Stat("/sys/module/apparmor"); err != nil {
		return nil
	}
	if _, err := os.Stat("/etc/apparmor.d"); err != nil {
		return nil
	}

	changed := false
	var helperRules []string
	var qemuRules []string
	for _, root := range uniqueNonEmptyStrings(roots) {
		storagePath := strings.TrimRight(path.Clean(root), "/")
		helperRules = append(helperRules,
			fmt.Sprintf("%s/ r,", storagePath),
			fmt.Sprintf("%s/**/ r,", storagePath),
			fmt.Sprintf("%s/** r,", storagePath),
		)
		qemuRules = append(qemuRules,
			fmt.Sprintf("%s/ r,", storagePath),
			fmt.Sprintf("%s/**/ r,", storagePath),
			fmt.Sprintf("%s/** rwk,", storagePath),
		)
	}
	helperBlock := buildManagedAppArmorBlock(helperRules)
	qemuBlock := buildManagedAppArmorBlock(qemuRules)

	helperChanged, err := upsertManagedAppArmorBlock(appArmorVirtAAHelperLocalPath, helperBlock)
	if err != nil {
		return fmt.Errorf("写入 virt-aa-helper AppArmor 规则失败: %w", err)
	}
	changed = changed || helperChanged

	qemuChanged, err := upsertManagedAppArmorBlock(appArmorLibvirtQemuStoragePath, qemuBlock)
	if err != nil {
		return fmt.Errorf("写入 libvirt-qemu AppArmor 规则失败: %w", err)
	}
	changed = changed || qemuChanged

	if changed {
		if err := reloadVirtAAHelperAppArmorProfile(); err != nil {
			return err
		}
		logger.App.Info("已更新 libvirt AppArmor 规则", "roots", strings.Join(uniqueNonEmptyStrings(roots), ", "))
	}
	return nil
}

func buildManagedAppArmorBlock(rules []string) string {
	return appArmorManagedBlockBegin + "\n" + strings.Join(rules, "\n") + "\n" + appArmorManagedBlockEnd + "\n"
}

func upsertManagedAppArmorBlock(filePath, block string) (bool, error) {
	if err := os.MkdirAll(path.Dir(filePath), 0755); err != nil {
		return false, err
	}

	existingBytes, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	existing := string(existingBytes)
	updated := upsertManagedBlock(existing, block)
	if updated == existing {
		return false, nil
	}
	if err := os.WriteFile(filePath, []byte(updated), 0644); err != nil {
		return false, err
	}
	return true, nil
}

func upsertManagedBlock(existing, block string) string {
	start := strings.Index(existing, appArmorManagedBlockBegin)
	end := strings.Index(existing, appArmorManagedBlockEnd)
	if start >= 0 && end >= start {
		end += len(appArmorManagedBlockEnd)
		if end < len(existing) && existing[end] == '\n' {
			end++
		}
		return existing[:start] + block + existing[end:]
	}

	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return block
	}
	return trimmed + "\n\n" + block
}

func reloadVirtAAHelperAppArmorProfile() error {
	parser, err := exec.LookPath("apparmor_parser")
	if err != nil {
		return nil
	}
	if _, err := os.Stat(appArmorVirtAAHelperProfilePath); err != nil {
		return nil
	}
	result := utils.ExecCommandWithTimeout(parser, 30*time.Second, "-r", appArmorVirtAAHelperProfilePath)
	if result.Error != nil {
		return fmt.Errorf("重载 virt-aa-helper AppArmor 规则失败: %s", result.Stderr)
	}
	return nil
}

func uniqueNonEmptyStrings(values []string) []string {
	var result []string
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
