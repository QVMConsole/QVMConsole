package quota

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"kvm_console/logger"
	"kvm_console/utils"
)

// ==================== Linux 文件系统 Project 配额 ====================
//
// 使用专用的回环挂载文件系统 + ext4 project quota 实现按目录的配额统计。
// 每个用户分配一个唯一的 project ID（基于系统 UID），
// 用户的 ISO 和文件共享目录都设置相同的 project ID，
// 配额在内核层强制限制写入。
//
// 依赖：quota 工具包（setquota, repquota, quotaon）
// 文件系统要求：ext4 with project,quota feature + prjquota 挂载选项

// 默认存储挂载点
const defaultStorageMountPoint = "/var/lib/kvm-user-storage"

// 默认存储镜像文件路径
const defaultStorageImagePath = "/var/lib/kvm-user-storage.img"

// StorageQuotaInfo 存储配额信息
type StorageQuotaInfo struct {
	UsedBytes  int64 // 已用空间（字节）
	LimitBytes int64 // 硬限制（字节），0 = 不限
}

// QuotaUserEntry represents a user's quota info needed for syncing.
// This avoids importing the service/user package (circular dependency).
type QuotaUserEntry struct {
	Username   string
	MaxStorage int
}

// HookListQuotaUsers must be set by the service root package before SyncAllUserQuotas is called.
// It returns the list of users whose quotas need to be synced.
var HookListQuotaUsers func() ([]QuotaUserEntry, error)

// GetStorageMountPoint 获取用户存储挂载点
func GetStorageMountPoint() string {
	return defaultStorageMountPoint
}

// GetStorageImagePath 获取存储镜像文件路径
func GetStorageImagePath() string {
	if configuredPath := strings.TrimSpace(os.Getenv("KVM_USER_STORAGE_IMAGE")); configuredPath != "" && filepath.IsAbs(configuredPath) {
		return filepath.Clean(configuredPath)
	}
	return defaultStorageImagePath
}

// GetProjectID 获取用户的 project ID（基于系统 UID）
func GetProjectID(username string) (int, error) {
	result := utils.ExecShellQuiet(fmt.Sprintf("id -u %s 2>/dev/null", utils.ShellSingleQuote(username)))
	if result.Error != nil || strings.TrimSpace(result.Stdout) == "" {
		return 0, fmt.Errorf("获取用户 %s 的 UID 失败", username)
	}
	uid, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if err != nil {
		return 0, fmt.Errorf("解析 UID 失败: %w", err)
	}
	return uid, nil
}

// getProjectName 生成 project 名称
func getProjectName(username string) string {
	return fmt.Sprintf("kvmstore_%s", username)
}

// SetupUserProject 为用户设置 project 配额映射
// 将用户的 ISO 和文件共享目录都绑定到同一个 project ID
func SetupUserProject(username string, dirs []string) error {
	projectID, err := GetProjectID(username)
	if err != nil {
		return err
	}
	projectName := getProjectName(username)

	// 读取现有配置
	projectsContent, _ := os.ReadFile("/etc/projects")
	projidContent, _ := os.ReadFile("/etc/projid")

	// 更新 /etc/projects（project_id:directory）
	projectsStr := string(projectsContent)
	for _, dir := range dirs {
		entry := fmt.Sprintf("%d:%s", projectID, dir)
		if !strings.Contains(projectsStr, entry) {
			projectsStr += entry + "\n"
		}
	}

	// 更新 /etc/projid（project_name:project_id）
	projidStr := string(projidContent)
	entry := fmt.Sprintf("%s:%d", projectName, projectID)
	if !strings.Contains(projidStr, entry) {
		projidStr += entry + "\n"
	}

	// 原子写入
	if err := utils.AtomicWriteFile("/etc/projects", []byte(projectsStr), 0644); err != nil {
		return fmt.Errorf("写入 /etc/projects 失败: %w", err)
	}
	if err := utils.AtomicWriteFile("/etc/projid", []byte(projidStr), 0644); err != nil {
		return fmt.Errorf("写入 /etc/projid 失败: %w", err)
	}

	// 对每个目录设置 project ID 和继承属性
	for _, dir := range dirs {
		result := utils.ExecShell(fmt.Sprintf("chattr +P -p %d %s 2>/dev/null", projectID, utils.ShellSingleQuote(dir)))
		if result.Error != nil {
			return fmt.Errorf("设置目录 %s 的 project ID 失败: %s", dir, result.Stderr)
		}
	}

	return nil
}

// SetUserStorageQuota 设置用户的存储配额（project quota）
// limitGB 为 0 表示取消配额限制
func SetUserStorageQuota(username string, limitGB int) error {
	projectID, err := GetProjectID(username)
	if err != nil {
		return err
	}

	mountPoint := GetStorageMountPoint()

	if limitGB <= 0 {
		// 取消配额限制
		return clearProjectQuota(projectID, mountPoint)
	}

	// 将 GB 转换为 KB（setquota 使用 KB 为单位）
	limitKB := int64(limitGB) * 1024 * 1024

	// setquota -P <project_id> <block-soft> <block-hard> <inode-soft> <inode-hard> <filesystem>
	result := utils.ExecCommand("setquota", "-P",
		strconv.Itoa(projectID),
		"0",                            // block soft limit
		strconv.FormatInt(limitKB, 10), // block hard limit
		"0",                            // inode soft limit
		"0",                            // inode hard limit
		mountPoint,
	)
	if result.Error != nil {
		return fmt.Errorf("设置 project 配额失败: %s", result.Stderr)
	}

	return nil
}

// RemoveUserStorageQuota 清除用户的存储配额
func RemoveUserStorageQuota(username string) error {
	projectID, err := GetProjectID(username)
	if err != nil {
		// 用户可能已被删除，忽略
		return nil
	}

	mountPoint := GetStorageMountPoint()

	// 清除配额
	clearProjectQuota(projectID, mountPoint)

	// 清理 /etc/projects 和 /etc/projid 中的条目
	projectName := getProjectName(username)

	// 清理 /etc/projects
	if projectsContent, err := os.ReadFile("/etc/projects"); err == nil {
		var newLines []string
		prefix := fmt.Sprintf("%d:", projectID)
		for _, line := range strings.Split(string(projectsContent), "\n") {
			if strings.HasPrefix(line, prefix) {
				continue
			}
			if line != "" {
				newLines = append(newLines, line)
			}
		}
		newContent := strings.Join(newLines, "\n")
		if len(newLines) > 0 {
			newContent += "\n"
		}
		_ = utils.AtomicWriteFile("/etc/projects", []byte(newContent), 0644)
	}

	// 清理 /etc/projid
	if projidContent, err := os.ReadFile("/etc/projid"); err == nil {
		var newLines []string
		prefix := fmt.Sprintf("%s:", projectName)
		for _, line := range strings.Split(string(projidContent), "\n") {
			if strings.HasPrefix(line, prefix) {
				continue
			}
			if line != "" {
				newLines = append(newLines, line)
			}
		}
		newContent := strings.Join(newLines, "\n")
		if len(newLines) > 0 {
			newContent += "\n"
		}
		_ = utils.AtomicWriteFile("/etc/projid", []byte(newContent), 0644)
	}

	return nil
}

// clearProjectQuota 清除指定 project 的配额
func clearProjectQuota(projectID int, mountPoint string) error {
	result := utils.ExecCommand("setquota", "-P",
		strconv.Itoa(projectID),
		"0", "0", "0", "0",
		mountPoint,
	)
	if result.Error != nil {
		return fmt.Errorf("清除 project 配额失败: %s", result.Stderr)
	}
	return nil
}

// GetUserStorageUsage 获取用户的存储配额使用情况（通过 project quota）
func GetUserStorageUsage(username string) (*StorageQuotaInfo, error) {
	projectID, err := GetProjectID(username)
	if err != nil {
		return nil, err
	}

	mountPoint := GetStorageMountPoint()

	// 使用 repquota -P 获取 project 配额
	result := utils.ExecCommand("repquota", "-Ps", mountPoint)
	if result.Error != nil {
		return nil, fmt.Errorf("repquota 执行失败: %s", result.Stderr)
	}

	// 解析输出，查找目标 project ID
	projectIDStr := strconv.Itoa(projectID)
	projectName := getProjectName(username)
	lines := strings.Split(result.Stdout, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "*") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// 第一个字段是 project 名称或 ID
		if fields[0] != projectName && fields[0] != projectIDStr {
			continue
		}

		// fields[1] = 状态标记（--、+-、-+、++）
		// fields[2] = 已用 blocks（KB）
		// fields[3] = soft limit（KB）
		// fields[4] = hard limit（KB）
		usedKB, err := parseQuotaNumber(fields[2])
		if err != nil {
			return nil, fmt.Errorf("解析已用空间失败: %w", err)
		}

		hardLimitKB, err := parseQuotaNumber(fields[4])
		if err != nil {
			return nil, fmt.Errorf("解析硬限制失败: %w", err)
		}

		return &StorageQuotaInfo{
			UsedBytes:  usedKB * 1024,
			LimitBytes: hardLimitKB * 1024,
		}, nil
	}

	// 用户不在 repquota 输出中
	return &StorageQuotaInfo{UsedBytes: 0, LimitBytes: 0}, nil
}

// parseQuotaNumber 解析配额数值（可能带 * 号表示超出配额）
func parseQuotaNumber(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "*")

	// repquota -s 模式下可能输出人类可读格式（如 10M、1G）
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([KMGT]?)$`)
	matches := re.FindStringSubmatch(strings.ToUpper(s))
	if matches != nil {
		val, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return 0, err
		}
		switch matches[2] {
		case "K":
			return int64(val), nil
		case "M":
			return int64(val * 1024), nil
		case "G":
			return int64(val * 1024 * 1024), nil
		case "T":
			return int64(val * 1024 * 1024 * 1024), nil
		default:
			return int64(val), nil
		}
	}

	return strconv.ParseInt(s, 10, 64)
}

// ==================== 存储文件系统管理 ====================

// IsStorageFilesystemMounted 检查专用存储文件系统是否已挂载
func IsStorageFilesystemMounted() bool {
	mountPoint := GetStorageMountPoint()
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), mountPoint)
}

// InitStorageFilesystem 初始化专用存储文件系统
// 创建镜像文件、格式化、挂载并启用 project 配额
func InitStorageFilesystem(sizeGB int) error {
	imgPath := GetStorageImagePath()
	mountPoint := GetStorageMountPoint()

	// 检查是否已挂载
	if IsStorageFilesystemMounted() {
		return nil
	}

	// 检查镜像文件是否已存在
	if !utils.FileExists(imgPath) {
		if sizeGB <= 0 {
			// 默认与根分区大小相同
			if total, _, _, err := utils.GetDiskSpace("/"); err == nil && total > 0 {
				sizeGB = int(total / 1024 / 1024)
			}
			if sizeGB <= 0 {
				sizeGB = 100
			}
		}

		// 创建稀疏文件（不实际占用磁盘空间）
		result := utils.ExecCommand("truncate", "-s", fmt.Sprintf("%dG", sizeGB), imgPath)
		if result.Error != nil {
			return fmt.Errorf("创建存储镜像文件失败: %s", result.Stderr)
		}

		// 格式化为 ext4，启用 project 和 quota 特性
		result = utils.ExecShell(fmt.Sprintf("mkfs.ext4 -O project,quota %s", utils.ShellSingleQuote(imgPath)))
		if result.Error != nil {
			return fmt.Errorf("格式化存储文件系统失败: %s", result.Stderr)
		}
	}

	// 创建挂载点
	utils.ExecCommand("mkdir", "-p", mountPoint)

	// 挂载
	result := utils.ExecCommand("mount", "-o", "loop,prjquota", imgPath, mountPoint)
	if result.Error != nil {
		return fmt.Errorf("挂载存储文件系统失败: %s", result.Stderr)
	}

	// 启用 project 配额
	utils.ExecShellQuiet(fmt.Sprintf("quotaon -P %s 2>/dev/null || true", utils.ShellSingleQuote(mountPoint)))

	// 确保 /etc/projects 和 /etc/projid 文件存在
	utils.ExecShell("touch /etc/projects /etc/projid")

	return nil
}

// EnsureStorageFilesystem 确保存储文件系统已挂载
// 如果未挂载但镜像文件存在，自动挂载
func EnsureStorageFilesystem() error {
	if IsStorageFilesystemMounted() {
		return nil
	}

	imgPath := GetStorageImagePath()
	mountPoint := GetStorageMountPoint()

	// 检查镜像文件是否存在
	if !utils.FileExists(imgPath) {
		// 镜像不存在，初始化
		return InitStorageFilesystem(0)
	}

	// 挂载
	utils.ExecCommand("mkdir", "-p", mountPoint)
	result := utils.ExecCommand("mount", "-o", "loop,prjquota", imgPath, mountPoint)
	if result.Error != nil {
		return fmt.Errorf("挂载存储文件系统失败: %s", result.Stderr)
	}

	// 启用配额
	utils.ExecShellQuiet(fmt.Sprintf("quotaon -P %s 2>/dev/null || true", utils.ShellSingleQuote(mountPoint)))

	return nil
}

// CheckQuotaToolsAvailable 检查配额工具是否可用
func CheckQuotaToolsAvailable() error {
	result := utils.ExecShellQuiet("which setquota repquota 2>/dev/null")
	if result.Error != nil || result.Stdout == "" {
		return fmt.Errorf("配额工具未安装，请执行: apt install quota")
	}
	return nil
}

// SyncAllUserQuotas 同步所有用户的配额到文件系统
// Requires HookListQuotaUsers to be wired by the service root package.
func SyncAllUserQuotas() error {
	if err := CheckQuotaToolsAvailable(); err != nil {
		return err
	}

	if err := EnsureStorageFilesystem(); err != nil {
		return err
	}

	if HookListQuotaUsers == nil {
		return fmt.Errorf("HookListQuotaUsers 未初始化，无法同步配额")
	}

	users, err := HookListQuotaUsers()
	if err != nil {
		return fmt.Errorf("获取用户列表失败: %w", err)
	}

	var errs []string
	for _, user := range users {
		if err := SetUserStorageQuota(user.Username, user.MaxStorage); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", user.Username, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("部分用户配额同步失败: %s", strings.Join(errs, "; "))
	}

	return nil
}

// TrimStorageResult 存储回收结果
type TrimStorageResult struct {
	BeforeBlocks int64  `json:"before_blocks"` // 回收前实际占用块数（1K blocks）
	AfterBlocks  int64  `json:"after_blocks"`  // 回收后实际占用块数（1K blocks）
	TrimmedBytes int64  `json:"trimmed_bytes"` // 回收的字节数
	TrimmedHuman string `json:"trimmed_human"` // 人类可读的回收大小
}

// TrimStorage 执行存储回收
// 先对挂载点执行 fstrim，再对镜像文件执行 fallocate --dig-holes
func TrimStorage() (*TrimStorageResult, error) {
	imgPath := GetStorageImagePath()
	mountPoint := GetStorageMountPoint()

	// 获取回收前的实际占用
	beforeBlocks, err := getFileBlocks(imgPath)
	if err != nil {
		return nil, fmt.Errorf("获取回收前文件块数失败: %w", err)
	}

	// 1. 对挂载点执行 fstrim
	fstrimResult := utils.ExecShellQuiet(fmt.Sprintf("fstrim -v %s 2>&1", utils.ShellSingleQuote(mountPoint)))
	if fstrimResult.Error != nil {
		// fstrim 失败不阻断，继续执行 fallocate
		logger.App.Warn("fstrim 执行失败，将继续执行 fallocate --dig-holes", "error", fstrimResult.Error, "stderr", fstrimResult.Stderr)
	}

	// 2. 对镜像文件执行 fallocate --dig-holes 释放稀疏文件空洞
	digResult := utils.ExecShellQuiet(fmt.Sprintf("fallocate --dig-holes %s 2>&1", utils.ShellSingleQuote(imgPath)))
	if digResult.Error != nil {
		return nil, fmt.Errorf("fallocate --dig-holes 执行失败: %s", digResult.Stderr)
	}

	// 获取回收后的实际占用
	afterBlocks, err := getFileBlocks(imgPath)
	if err != nil {
		return nil, fmt.Errorf("获取回收后文件块数失败: %w", err)
	}

	trimmedBytes := (beforeBlocks - afterBlocks) * 1024
	if trimmedBytes < 0 {
		trimmedBytes = 0
	}

	return &TrimStorageResult{
		BeforeBlocks: beforeBlocks,
		AfterBlocks:  afterBlocks,
		TrimmedBytes: trimmedBytes,
		TrimmedHuman: formatBytes(trimmedBytes),
	}, nil
}

// getFileBlocks 获取文件占用的 1K 块数（通过 stat 或 ls -ls）
func getFileBlocks(path string) (int64, error) {
	result := utils.ExecShellQuiet(fmt.Sprintf("stat --format='%%b' %s 2>/dev/null", utils.ShellSingleQuote(path)))
	if result.Error != nil || strings.TrimSpace(result.Stdout) == "" {
		return 0, fmt.Errorf("获取文件块数失败: %s", result.Stderr)
	}
	blocks, err := strconv.ParseInt(strings.TrimSpace(result.Stdout), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("解析文件块数失败: %w", err)
	}
	return blocks, nil
}

// formatBytes 将字节数转换为人类可读格式
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
