package template

import (
	"fmt"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

// PreinstallLinuxCloudInitDeps 在制作 Linux 模板时预装 cloud-init 和 growpart 依赖
// 使用国内镜像源加速，安装失败仅告警不阻断模板制作
func PreinstallLinuxCloudInitDeps(templatePath string) error {
	logger.App.Info("预装 Linux 克隆依赖包（cloud-init, growpart）", "template", templatePath)

	// 构建 virt-customize 命令，使用国内镜像源并安装依赖
	args := []string{
		"-a", templatePath,
		"--network",
		// 检测并安装 cloud-init 和 growpart（使用国内镜像源加速）
		"--run-command", `
			set -e
			# === DNF 系（Fedora/RHEL/CentOS/openEuler 等）===
			if command -v dnf >/dev/null 2>&1; then
				if ! rpm -q cloud-init cloud-utils-growpart &>/dev/null; then
					echo "[QVM] 检测到 DNF 包管理器，配置国内镜像源..."
					# 配置国内镜像源（阿里云）
					for repo in /etc/yum.repos.d/*.repo; do
						[ -f "$repo" ] || continue
						sed -i 's|^mirrorlist=|#mirrorlist=|g; s|^metalink=|#metalink=|g' "$repo"
						sed -i 's|mirror.centos.org|mirrors.aliyun.com|g; s|dl.fedoraproject.org/pub|mirrors.aliyun.com|g' "$repo"
						sed -i 's|^#baseurl=|baseurl=|g' "$repo"
					done
					echo "[QVM] 安装 cloud-init 和 cloud-utils-growpart..."
					if dnf install -y cloud-init cloud-utils-growpart 2>&1; then
						echo "[QVM] 依赖安装成功"
					else
						echo "[QVM-WARN] DNF 安装失败，磁盘自动扩容功能可能不可用" >&2
					fi
				else
					echo "[QVM] cloud-init 和 cloud-utils-growpart 已安装，跳过"
				fi
			# === APT 系（Debian/Ubuntu 等）===
			elif command -v apt-get >/dev/null 2>&1; then
				if ! dpkg -s cloud-init cloud-guest-utils &>/dev/null; then
					echo "[QVM] 检测到 APT 包管理器，配置国内镜像源..."
					# 配置国内镜像源（阿里云）
					for f in /etc/apt/sources.list /etc/apt/sources.list.d/*.list; do
						[ -f "$f" ] || continue
						sed -i 's|http://archive.ubuntu.com|https://mirrors.aliyun.com|g; s|http://security.ubuntu.com|https://mirrors.aliyun.com|g; s|http://deb.debian.org|https://mirrors.aliyun.com|g; s|http://security.debian.org|https://mirrors.aliyun.com/debian-security|g' "$f"
					done
					echo "[QVM] 更新软件包索引..."
					if apt-get update -qq 2>&1; then
						echo "[QVM] 安装 cloud-init 和 cloud-guest-utils..."
						if DEBIAN_FRONTEND=noninteractive apt-get install -y cloud-init cloud-guest-utils 2>&1; then
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
			# === YUM 系（旧版 CentOS 等）===
			elif command -v yum >/dev/null 2>&1; then
				if ! rpm -q cloud-init cloud-utils-growpart &>/dev/null; then
					echo "[QVM] 检测到 YUM 包管理器，配置国内镜像源..."
					# 配置国内镜像源（阿里云）
					for repo in /etc/yum.repos.d/*.repo; do
						[ -f "$repo" ] || continue
						sed -i 's|^mirrorlist=|#mirrorlist=|g' "$repo"
						sed -i 's|mirror.centos.org|mirrors.aliyun.com|g' "$repo"
						sed -i 's|^#baseurl=|baseurl=|g' "$repo"
					done
					echo "[QVM] 安装 cloud-init 和 cloud-utils-growpart..."
					if yum install -y cloud-init cloud-utils-growpart 2>&1; then
						echo "[QVM] 依赖安装成功"
					else
						echo "[QVM-WARN] YUM 安装失败，磁盘自动扩容功能可能不可用" >&2
					fi
				else
					echo "[QVM] cloud-init 和 cloud-utils-growpart 已安装，跳过"
				fi
			else
				echo "[QVM-WARN] 未检测到支持的包管理器（dnf/apt/yum）" >&2
				exit 20
			fi

			if command -v rpm >/dev/null 2>&1; then
				rpm -q cloud-init cloud-utils-growpart >/dev/null 2>&1 || exit 21
			elif command -v dpkg >/dev/null 2>&1; then
				dpkg -s cloud-init cloud-guest-utils >/dev/null 2>&1 || exit 21
			else
				exit 20
			fi
		`,
		"--quiet",
	}

	result := utils.ExecCommandLongRunning("virt-customize", args...)
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
	statusResult := utils.ExecCommandLongRunning("virt-cat", "-a", templatePath, "/var/lib/dpkg/status")
	if statusResult.Error == nil {
		return debianCloneDepsInstalled(statusResult.Stdout), nil
	}

	statusError := commandResultText(statusResult.Error, statusResult.Stderr)
	if isGuestfsLaunchError(statusError) {
		return false, fmt.Errorf("Linux 克隆依赖检查失败: %s", statusError)
	}

	// RPM 系发行版没有可稳定直接解析的纯文本包状态文件，使用无网络 guestfs
	// 执行包查询。该步骤不会修改模板，也不会拉起 passt 网络后端。
	args := []string{
		"-a", templatePath,
		"--no-network",
		"--run-command", `
			if command -v rpm >/dev/null 2>&1; then
				rpm -q cloud-init cloud-utils-growpart >/dev/null 2>&1 || exit 80
			elif command -v dpkg >/dev/null 2>&1; then
				dpkg -s cloud-init cloud-guest-utils >/dev/null 2>&1 || exit 80
			else
				exit 81
			fi
		`,
		"--quiet",
	}
	result := utils.ExecCommandLongRunning("virt-customize", args...)
	if result.Error == nil {
		return true, nil
	}

	checkError := commandResultText(result.Error, result.Stderr)
	if strings.Contains(checkError, "exit status 80") {
		return false, nil
	}
	return false, fmt.Errorf("Linux 克隆依赖检查失败: %s", checkError)
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
	return debianPackageInstalled(status, "cloud-init") && debianPackageInstalled(status, "cloud-guest-utils")
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

	progressFn(15, "检查 Linux 克隆依赖...")
	_ = utils.RemoveFileImmutable(templatePath)
	defer utils.SetFileImmutable(templatePath)

	progressFn(35, "检查并补齐 cloud-init 与磁盘扩容依赖...")
	err = EnsureLinuxCloudInitDeps(templatePath)
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
