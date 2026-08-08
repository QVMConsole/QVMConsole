package template

import (
	"fmt"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

// PreinstallLinuxCloudInitDeps 在制作 Linux 模板时预装 cloud-init、QGA 和磁盘自动化依赖。
// 保留来宾镜像自带的软件源，安装失败仅告警不阻断模板制作
func PreinstallLinuxCloudInitDeps(templatePath string) error {
	logger.App.Info("预装 Linux 克隆与来宾磁盘自动化依赖", "template", templatePath)

	// 构建 virt-customize 命令，使用来宾默认软件源安装依赖
	args := []string{
		"-a", templatePath,
		"--network",
		// 检测并安装 cloud-init 和 growpart
		"--run-command", `
			set -e
			rpm_required="cloud-init cloud-utils-growpart qemu-guest-agent e2fsprogs xfsprogs lvm2 gdisk parted"
			debian_required="cloud-init cloud-guest-utils qemu-guest-agent e2fsprogs xfsprogs lvm2 gdisk parted"
			# === DNF 系（Fedora/RHEL/CentOS/openEuler 等）===
			if command -v dnf >/dev/null 2>&1; then
				if ! rpm -q $rpm_required &>/dev/null; then
					echo "[QVM] 检测到 DNF 包管理器，使用来宾默认软件源..."
					echo "[QVM] 安装 cloud-init 和 cloud-utils-growpart..."
					if dnf install -y $rpm_required 2>&1; then
						echo "[QVM] 依赖安装成功"
					else
						echo "[QVM-WARN] DNF 安装失败，磁盘自动扩容功能可能不可用" >&2
					fi
				else
					echo "[QVM] cloud-init 和 cloud-utils-growpart 已安装，跳过"
				fi
				dnf install -y btrfs-progs >/dev/null 2>&1 || echo "[QVM-WARN] 未安装 btrfs-progs，Btrfs 磁盘自动扩容不可用" >&2
			# === APT 系（Debian/Ubuntu 等）===
			elif command -v apt-get >/dev/null 2>&1; then
				if ! dpkg -s $debian_required &>/dev/null; then
					echo "[QVM] 检测到 APT 包管理器，使用来宾默认软件源..."
					echo "[QVM] 更新软件包索引..."
					if apt-get update -qq 2>&1; then
						echo "[QVM] 安装 cloud-init 和 cloud-guest-utils..."
						if DEBIAN_FRONTEND=noninteractive apt-get install -y $debian_required 2>&1; then
							echo "[QVM] 依赖安装成功"
						else
							echo "[QVM-WARN] APT 安装失败，磁盘自动扩容功能可能不可用" >&2
						fi
					else
						echo "[QVM-WARN] APT 更新失败（可能无网络），磁盘自动扩容功能可能不可用" >&2
					fi
				else
					echo "[QVM] cloud-init 和 cloud-guest-utils 已安装，跳过"
				fi
				DEBIAN_FRONTEND=noninteractive apt-get install -y btrfs-progs >/dev/null 2>&1 || echo "[QVM-WARN] 未安装 btrfs-progs，Btrfs 磁盘自动扩容不可用" >&2
			# === YUM 系（旧版 CentOS 等）===
			elif command -v yum >/dev/null 2>&1; then
				if ! rpm -q $rpm_required &>/dev/null; then
					echo "[QVM] 检测到 YUM 包管理器，使用来宾默认软件源..."
					echo "[QVM] 安装 cloud-init 和 cloud-utils-growpart..."
					if yum install -y $rpm_required 2>&1; then
						echo "[QVM] 依赖安装成功"
					else
						echo "[QVM-WARN] YUM 安装失败，磁盘自动扩容功能可能不可用" >&2
					fi
				else
					echo "[QVM] cloud-init 和 cloud-utils-growpart 已安装，跳过"
				fi
				yum install -y btrfs-progs >/dev/null 2>&1 || echo "[QVM-WARN] 未安装 btrfs-progs，Btrfs 磁盘自动扩容不可用" >&2
			else
				echo "[QVM-WARN] 未检测到支持的包管理器（dnf/apt/yum）" >&2
				exit 20
			fi

			if command -v rpm >/dev/null 2>&1; then
				rpm -q $rpm_required >/dev/null 2>&1 || exit 21
			elif command -v dpkg >/dev/null 2>&1; then
				dpkg -s $debian_required >/dev/null 2>&1 || exit 21
			else
				exit 20
			fi
		`,
		"--quiet",
	}

	// virt-customize 需要读写整个磁盘镜像并可能在来宾系统内安装依赖，属于大 IO 操作，不设置自动超时
	result := utils.ExecCommandNoTimeout("virt-customize", args...)
	if result.Error != nil {
		logger.App.Warn("Linux 依赖预装失败（不影响模板制作）", "error", result.Stderr)
		return fmt.Errorf("Linux 克隆依赖预装失败: %s", strings.TrimSpace(result.Stderr))
	}

	logger.App.Info("Linux 克隆依赖预装完成", "template", templatePath)
	return nil
}

// HasLinuxCloudInitDeps 以无网络方式检查模板中是否已经具备离线克隆依赖。
// 返回 false, nil 表示镜像可访问但依赖尚未安装；其余错误表示 guestfs 或镜像访问异常。
func HasLinuxCloudInitDeps(templatePath string) (bool, error) {
	// virt-cat 需要读取整个磁盘镜像，属于大 IO 操作，不设置自动超时
	statusResult := utils.ExecCommandNoTimeout("virt-cat", "-a", templatePath, "/var/lib/dpkg/status")
	if statusResult.Error == nil {
		return debianCloneDepsInstalled(statusResult.Stdout), nil
	}

	statusError := commandResultText(statusResult.Error, statusResult.Stderr)
	if isGuestfsLaunchError(statusError) {
		return false, fmt.Errorf("Linux 克隆依赖检查失败: %s", statusError)
	}

	// RPM 系发行版没有可稳定直接解析的纯文本包状态文件。virt-customize 会将
	// 来宾命令的退出码统一转换为自身的 exit status 1，无法区分缺包与执行异常。
	// 改为只读检查各依赖提供的命令文件，不修改模板也不启用来宾网络。
	return rpmCloneDepsInstalled(templatePath)
}

func rpmCloneDepsInstalled(templatePath string) (bool, error) {
	directories := []struct {
		path     string
		commands []string
	}{
		{
			path:     "/usr/bin",
			commands: []string{"cloud-init", "growpart", "qemu-ga"},
		},
		{
			path:     "/usr/sbin",
			commands: []string{"resize2fs", "xfs_growfs", "lvextend", "gdisk", "parted"},
		},
	}

	for _, directory := range directories {
		// virt-ls 需要读取整个磁盘镜像，属于大 IO 操作，不设置自动超时
		result := utils.ExecCommandNoTimeout("virt-ls", "-a", templatePath, directory.path)
		if result.Error != nil {
			checkError := commandResultText(result.Error, result.Stderr)
			return false, fmt.Errorf("读取 RPM 模板目录 %s 失败: %s", directory.path, checkError)
		}

		entries := make(map[string]struct{})
		for _, entry := range strings.Fields(result.Stdout) {
			entries[entry] = struct{}{}
		}
		for _, command := range directory.commands {
			if _, exists := entries[command]; !exists {
				return false, nil
			}
		}
	}

	return true, nil
}

// EnsureLinuxCloudInitDeps 仅在模板确实缺少依赖时启用 guestfs 网络安装。
func EnsureLinuxCloudInitDeps(templatePath string) error {
	installed, err := HasLinuxCloudInitDeps(templatePath)
	if err != nil {
		return err
	}
	if installed {
		logger.App.Info("Linux 克隆依赖已存在，跳过网络预装", "template", templatePath)
		return nil
	}
	return PreinstallLinuxCloudInitDeps(templatePath)
}

func debianCloneDepsInstalled(status string) bool {
	packages := []string{"cloud-init", "cloud-guest-utils", "qemu-guest-agent", "e2fsprogs", "xfsprogs", "lvm2", "gdisk", "parted"}
	for _, packageName := range packages {
		if !debianPackageInstalled(status, packageName) {
			return false
		}
	}
	return true
}

func debianPackageInstalled(status, packageName string) bool {
	for _, paragraph := range strings.Split(status, "\n\n") {
		if strings.Contains(paragraph, "Package: "+packageName+"\n") &&
			strings.Contains(paragraph, "Status: install ok installed") {
			return true
		}
	}
	return false
}

func commandResultText(commandErr error, stderr string) string {
	parts := make([]string, 0, 2)
	if commandErr != nil {
		parts = append(parts, commandErr.Error())
	}
	if strings.TrimSpace(stderr) != "" {
		parts = append(parts, strings.TrimSpace(stderr))
	}
	return strings.TrimSpace(strings.Join(parts, ": "))
}

func isGuestfsLaunchError(message string) bool {
	return strings.Contains(message, "guestfs_launch failed") ||
		strings.Contains(message, "libguestfs appliance failed to start")
}

func updateLinuxInitStatus(meta *TemplateMeta, err error) {
	if meta == nil {
		return
	}
	meta.LinuxInitChecked = time.Now().Format(time.RFC3339)
	if err == nil {
		meta.LinuxInitStatus = "ready"
		meta.LinuxInitError = ""
		return
	}
	meta.LinuxInitStatus = "failed"
	meta.LinuxInitError = strings.TrimSpace(err.Error())
}

// isLinuxTemplateInitReady 用于识别已在模板包中确认过离线依赖的 Linux 模板。
func isLinuxTemplateInitReady(meta *TemplateMeta) bool {
	return meta != nil && strings.EqualFold(strings.TrimSpace(meta.LinuxInitStatus), "ready")
}

// PrepareImportedLinuxTemplate 为已导入的 Linux 模板补齐离线克隆依赖。
func PrepareImportedLinuxTemplate(templateName string, progressFn func(int, string)) error {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}
	templatePath, err := EnsureTemplatePath(templateName)
	if err != nil {
		return err
	}
	meta := loadTemplateMeta(templatePath)
	if meta == nil {
		return fmt.Errorf("模板元数据不存在: %s", templateName)
	}
	if normalizeTemplateType(meta.Type) != "linux" {
		return fmt.Errorf("仅 Linux 模板支持离线克隆依赖预处理")
	}
	if err := ensureLinuxTemplateCanBePrepared(templateName); err != nil {
		return err
	}

	progressFn(15, "检查 Linux 克隆依赖...")
	_ = utils.RemoveFileImmutable(templatePath)
	defer utils.SetFileImmutable(templatePath)

	progressFn(35, "检查并补齐 cloud-init 与磁盘扩容依赖...")
	err = EnsureLinuxCloudInitDeps(templatePath)
	if err == nil {
		progressFn(70, "更新模板校验和...")
		hash, hashErr := CalculateFileHashes(templatePath)
		if hashErr != nil {
			err = fmt.Errorf("更新模板校验和失败: %w", hashErr)
		} else {
			meta.MD5 = hash.MD5
			meta.SHA256 = hash.SHA256
			meta.FileSize = hash.FileSize
		}
	}
	updateLinuxInitStatus(meta, err)
	if saveErr := saveTemplateMeta(templatePath, meta); saveErr != nil {
		return saveErr
	}
	if err != nil {
		return err
	}
	progressFn(100, "Linux 模板离线克隆依赖已就绪")
	return nil
}
