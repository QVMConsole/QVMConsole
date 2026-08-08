package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvm_console/logger"
	"kvm_console/utils"
)

// cloneOpenWrt OpenWrt 克隆初始化逻辑
// 兼容两种 OpenWrt 磁盘布局：
//   - ext4 根分区（标准 OpenWrt）：使用 virt-customize --upload 直接注入
//   - squashfs + overlay（iStoreOS 等）：使用 guestfish 写入 overlay 分区的 upper 目录
//
// 注意：OpenWrt 使用 BusyBox，缺少 setarch 等工具，因此禁止使用 --run-command，
// 所有修改必须通过文件注入完成。
func cloneOpenWrt(params *CloneParams, cloneDisk string, progressFn func(int, string)) error {
	staticIP := strings.TrimSpace(params.StaticIP)
	if staticIP == "" {
		return fmt.Errorf("OpenWrt 克隆需要指定静态 IP 地址")
	}

	progressFn(25, "配置 OpenWrt 网络...")

	hostname := params.Hostname
	if hostname == "" {
		hostname = "OpenWrt"
	}

	// 检测磁盘是否为 squashfs overlay 布局
	overlayDev := detectSquashfsOverlay(cloneDisk)
	if overlayDev != "" {
		logger.App.Info("检测到 squashfs overlay 布局，使用 guestfish 注入", "overlay", overlayDev)
		return cloneOpenWrtSquashfs(params, cloneDisk, overlayDev, hostname, staticIP, progressFn)
	}

	// 标准 ext4 布局：使用 virt-customize --upload
	return cloneOpenWrtExt4(params, cloneDisk, hostname, staticIP, progressFn)
}

// cloneOpenWrtExt4 处理标准 ext4 根分区的 OpenWrt
func cloneOpenWrtExt4(params *CloneParams, cloneDisk, hostname, staticIP string, progressFn func(int, string)) error {
	tmpDir, err := os.MkdirTemp("", "openwrt-init-*")
	if err != nil {
		return fmt.Errorf("创建 OpenWrt 初始化临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. 网络配置
	networkConfig := buildOpenWrtNetworkConfig(staticIP, params.Gateway, params.DNS)
	networkPath := filepath.Join(tmpDir, "network")
	if err := os.WriteFile(networkPath, []byte(networkConfig), 0644); err != nil {
		return fmt.Errorf("写入 OpenWrt 网络配置临时文件失败: %w", err)
	}

	// 2. 系统配置（hostname）
	systemConfig := buildOpenWrtSystemConfig(hostname)
	systemPath := filepath.Join(tmpDir, "system")
	if err := os.WriteFile(systemPath, []byte(systemConfig), 0644); err != nil {
		return fmt.Errorf("写入 OpenWrt 系统配置临时文件失败: %w", err)
	}

	// 构建 virt-customize 参数（仅使用 --upload，不使用 --run-command）
	args := []string{
		"-a", cloneDisk,
		"--no-network",
		"--upload", networkPath + ":/etc/config/network",
		"--upload", systemPath + ":/etc/config/system",
		"--quiet",
	}

	// 3. 设置 root 密码（--password 不依赖 guest 内 shell，直接修改 /etc/shadow）
	if params.Password != "" {
		args = append(args, "--password", "root:password:"+params.Password)
	}

	// virt-customize 需要读写整个磁盘镜像，属于大 IO 操作，不设置自动超时
	result := utils.ExecCommandNoTimeout("virt-customize", args...)
	if result.Error != nil {
		return fmt.Errorf("OpenWrt 克隆初始化失败: %s", D.FirstNonEmpty(result.Stderr, result.Error.Error()))
	}

	logger.App.Info("OpenWrt ext4 离线初始化完成", "vm", params.Name, "hostname", hostname, "static_ip", staticIP)
	return nil
}

// cloneOpenWrtSquashfs 处理 squashfs + overlay 布局的 OpenWrt（如 iStoreOS）
// 通过 guestfish 挂载 overlay 分区，将配置写入 /upper/etc/config/ 目录
func cloneOpenWrtSquashfs(params *CloneParams, cloneDisk, overlayDev, hostname, staticIP string, progressFn func(int, string)) error {
	tmpDir, err := os.MkdirTemp("", "openwrt-squashfs-init-*")
	if err != nil {
		return fmt.Errorf("创建 OpenWrt 初始化临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. 网络配置
	networkConfig := buildOpenWrtNetworkConfig(staticIP, params.Gateway, params.DNS)
	networkPath := filepath.Join(tmpDir, "network")
	if err := os.WriteFile(networkPath, []byte(networkConfig), 0644); err != nil {
		return fmt.Errorf("写入 OpenWrt 网络配置临时文件失败: %w", err)
	}

	// 2. 系统配置（hostname）
	systemConfig := buildOpenWrtSystemConfig(hostname)
	systemPath := filepath.Join(tmpDir, "system")
	if err := os.WriteFile(systemPath, []byte(systemConfig), 0644); err != nil {
		return fmt.Errorf("写入 OpenWrt 系统配置临时文件失败: %w", err)
	}

	progressFn(35, "注入 OpenWrt overlay 配置...")

	// 构建 guestfish 脚本：挂载 overlay 分区并上传文件
	// guestfish 支持挂载指定分区，写入 overlay 的 upper 目录
	gfScript := fmt.Sprintf(`add %s
run
mount %s /
mkdir-p /upper/etc/config
upload %s /upper/etc/config/network
upload %s /upper/etc/config/system
`, cloneDisk, overlayDev, networkPath, systemPath)

	// 3. 密码设置：生成 shadow 文件写入 overlay
	if params.Password != "" {
		passHash := generatePasswordHash(params.Password)
		if passHash != "" {
			shadowContent := buildOpenWrtShadowFile(passHash)
			shadowPath := filepath.Join(tmpDir, "shadow")
			if err := os.WriteFile(shadowPath, []byte(shadowContent), 0600); err != nil {
				return fmt.Errorf("写入 shadow 临时文件失败: %w", err)
			}
			gfScript += fmt.Sprintf("upload %s /upper/etc/shadow\n", shadowPath)
		}
	}

	// 写入 guestfish 脚本文件
	scriptPath := filepath.Join(tmpDir, "guestfish.script")
	if err := os.WriteFile(scriptPath, []byte(gfScript), 0644); err != nil {
		return fmt.Errorf("写入 guestfish 脚本失败: %w", err)
	}

	// 执行 guestfish（读写磁盘镜像，属于大 IO 操作，不设置自动超时）
	result := utils.ExecCommandNoTimeout("guestfish", "--file", scriptPath)
	if result.Error != nil {
		return fmt.Errorf("OpenWrt squashfs 克隆初始化失败: %s", D.FirstNonEmpty(result.Stderr, result.Error.Error()))
	}

	logger.App.Info("OpenWrt squashfs overlay 离线初始化完成", "vm", params.Name, "hostname", hostname, "static_ip", staticIP, "overlay", overlayDev)
	return nil
}

// detectSquashfsOverlay 检测磁盘是否包含 squashfs 文件系统
// 如果存在 squashfs，返回 ext4 overlay 分区的设备路径（如 /dev/sda3）
// 否则返回空字符串
func detectSquashfsOverlay(diskPath string) string {
	result := utils.ExecCommand("virt-filesystems", "-a", diskPath, "--long", "--filesystems")
	if result.Error != nil {
		return ""
	}

	hasSquashfs := false
	var ext4Dev string

	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		dev := fields[0]
		fsType := fields[2]

		if fsType == "squashfs" {
			hasSquashfs = true
		}
		// 记录最后一个 ext4 分区作为 overlay（通常是 squashfs 之后的分区）
		if fsType == "ext4" {
			ext4Dev = dev
		}
	}

	if hasSquashfs && ext4Dev != "" {
		return ext4Dev
	}
	return ""
}

// generatePasswordHash 使用 openssl 生成 SHA-512 密码哈希
func generatePasswordHash(password string) string {
	// 通过管道传递密码避免在进程列表中暴露
	cmd := fmt.Sprintf("printf '%%s' %s | openssl passwd -6 -stdin", utils.ShellSingleQuote(password))
	result := utils.ExecShell(cmd)
	if result.Error != nil {
		logger.App.Warn("生成密码哈希失败", "error", result.Error)
		return ""
	}
	hash := strings.TrimSpace(result.Stdout)
	// 验证哈希格式
	if !strings.HasPrefix(hash, "$6$") {
		logger.App.Warn("密码哈希格式异常", "hash", hash)
		return ""
	}
	return hash
}

// buildOpenWrtShadowFile 生成 OpenWrt /etc/shadow 文件内容
func buildOpenWrtShadowFile(passHash string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("root:%s:0:0:99999:7:::\n", passHash))
	sb.WriteString("daemon:*:0:0:99999:7:::\n")
	sb.WriteString("ftp:*:0:0:99999:7:::\n")
	sb.WriteString("network:*:0:0:99999:7:::\n")
	sb.WriteString("nobody:*:0:0:99999:7:::\n")
	sb.WriteString("dnsmasq:x:0:0:99999:7:::\n")
	return sb.String()
}

// buildOpenWrtNetworkConfig 生成 OpenWrt UCI 网络配置文件内容
// 将 LAN 接口设置为用户指定的静态 IP
func buildOpenWrtNetworkConfig(staticIP, gateway, dns string) string {
	var sb strings.Builder

	sb.WriteString("config interface 'loopback'\n")
	sb.WriteString("\toption device 'lo'\n")
	sb.WriteString("\toption proto 'static'\n")
	sb.WriteString("\tlist ipaddr '127.0.0.1/8'\n")
	sb.WriteString("\n")

	sb.WriteString("config globals 'globals'\n")
	sb.WriteString("\toption ula_prefix 'fd5d:bfac:5035::/48'\n")
	sb.WriteString("\n")

	sb.WriteString("config device\n")
	sb.WriteString("\toption name 'br-lan'\n")
	sb.WriteString("\toption type 'bridge'\n")
	sb.WriteString("\tlist ports 'eth0'\n")
	sb.WriteString("\n")

	sb.WriteString("config interface 'lan'\n")
	sb.WriteString("\toption device 'br-lan'\n")
	sb.WriteString("\toption proto 'static'\n")

	// 处理 IP 地址：确保有 CIDR 后缀
	ipAddr := staticIP
	if !strings.Contains(ipAddr, "/") {
		ipAddr += "/24"
	}
	sb.WriteString(fmt.Sprintf("\tlist ipaddr '%s'\n", ipAddr))

	// 网关
	if gateway := strings.TrimSpace(gateway); gateway != "" {
		sb.WriteString(fmt.Sprintf("\toption gateway '%s'\n", gateway))
	}

	// DNS
	if dns := strings.TrimSpace(dns); dns != "" {
		// 支持多个 DNS，用空格或逗号分隔
		dnsList := strings.FieldsFunc(dns, func(r rune) bool {
			return r == ',' || r == ' '
		})
		for _, d := range dnsList {
			d = strings.TrimSpace(d)
			if d != "" {
				sb.WriteString(fmt.Sprintf("\tlist dns '%s'\n", d))
			}
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// buildOpenWrtSystemConfig 生成 OpenWrt /etc/config/system 配置文件内容
func buildOpenWrtSystemConfig(hostname string) string {
	var sb strings.Builder
	sb.WriteString("config system\n")
	sb.WriteString(fmt.Sprintf("\toption hostname '%s'\n", hostname))
	sb.WriteString("\toption timezone 'GMT0'\n")
	sb.WriteString("\toption zonename 'UTC'\n")
	sb.WriteString("\toption ttylogin '0'\n")
	sb.WriteString("\toption log_size '128'\n")
	sb.WriteString("\toption urandom_seed '0'\n")
	sb.WriteString("\n")
	sb.WriteString("config timeserver 'ntp'\n")
	sb.WriteString("\toption enabled '1'\n")
	sb.WriteString("\toption enable_server '0'\n")
	sb.WriteString("\tlist server '0.openwrt.pool.ntp.org'\n")
	sb.WriteString("\tlist server '1.openwrt.pool.ntp.org'\n")
	sb.WriteString("\tlist server '2.openwrt.pool.ntp.org'\n")
	sb.WriteString("\tlist server '3.openwrt.pool.ntp.org'\n")
	return sb.String()
}
