package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

// prepareLinuxNoCloudInit 通过 virt-customize 完成 Linux 克隆全部初始化
// 无需 SSH 连接：清理身份信息、写入 cloud-init NoCloud seed 文件、离线修改密码与用户名
// 依赖必须在模板制作或模板预处理阶段完成，克隆阶段始终禁用网络。
func prepareLinuxNoCloudInit(params *CloneParams, cloneDisk string, progressFn func(int, string)) error {
	targetUser := resolveLinuxCloneTargetUser(params.User, params.TemplateUser)

	// 生成 cloud-init seed 文件内容
	metaData := buildNoCloudMetaData(params)
	userData := buildNoCloudUserData(params)

	// 写入临时目录，通过 virt-customize --upload 注入磁盘
	tmpDir, err := os.MkdirTemp("", "nocloud-*")
	if err != nil {
		return fmt.Errorf("创建 cloud-init 临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	metaPath := filepath.Join(tmpDir, "meta-data")
	userPath := filepath.Join(tmpDir, "user-data")
	if err := os.WriteFile(metaPath, []byte(metaData), 0644); err != nil {
		return fmt.Errorf("写入 cloud-init meta-data 失败: %w", err)
	}
	if err := os.WriteFile(userPath, []byte(userData), 0644); err != nil {
		return fmt.Errorf("写入 cloud-init user-data 失败: %w", err)
	}

	args := []string{
		"-a", cloneDisk,
		"--no-network",
		// 1. 清理 machine-id（重置实例身份）
		"--run-command", "truncate -s 0 /etc/machine-id 2>/dev/null || rm -f /etc/machine-id",
		"--run-command", "rm -f /var/lib/dbus/machine-id 2>/dev/null || true",
		// 2. 清理 DHCP 租约（防止 IP 冲突）
		"--run-command", "rm -f /var/lib/dhcp/*.leases 2>/dev/null || true",
		"--run-command", "rm -f /var/lib/NetworkManager/*.lease 2>/dev/null || true",
		"--run-command", "rm -f /var/lib/systemd/network/*.lease 2>/dev/null || true",
		"--run-command", "rm -rf /run/systemd/netif/leases/* 2>/dev/null || true",
		// 3. 启用 cloud-init（删除 disabled 标记，允许首次启动时执行）
		"--run-command", "rm -f /etc/cloud/cloud-init.disabled",
		// 4. 清理安装器遗留的 cloud-init 配置文件
		//    99-installer.cfg: subiquity/curtin 安装后写入，强制 datasource=None + 重建 disabled 文件 + 禁用 growpart
		//    这些配置对克隆后的 VM 有害，必须删除以恢复 NoCloud 数据源和磁盘扩容功能
		"--run-command", "rm -f /etc/cloud/cloud.cfg.d/99-installer.cfg 2>/dev/null || true",
		"--run-command", "rm -f /etc/cloud/cloud.cfg.d/00-subiquity-disable-cloudinit-networking.cfg 2>/dev/null || true",
		"--run-command", "rm -f /etc/cloud/cloud.cfg.d/curtin-preserve-sources.cfg 2>/dev/null || true",
		// 5. 清理 cloud-init 实例缓存（强制重新初始化，而非跳过）
		"--run-command", "rm -rf /var/lib/cloud/instances/* /var/lib/cloud/instance",
		// 6. 启用 SSH 密码认证，并按目标用户决定是否允许 root 登录。
		"--run-command", buildLinuxSSHPasswordAuthCommand(targetUser),
		// 7. 写入 cloud-init NoCloud seed 文件（文件系统方式，无时序问题）
		"--run-command", "mkdir -p /var/lib/cloud/seed/nocloud",
		"--upload", metaPath + ":/var/lib/cloud/seed/nocloud/meta-data",
		"--upload", userPath + ":/var/lib/cloud/seed/nocloud/user-data",
		// 8. 离线写入 hostname（即使模板无 cloud-init 也保证生效）
		"--run-command", fmt.Sprintf("printf '%%s\\n' %s > /etc/hostname", utils.ShellSingleQuote(params.Hostname)),
		"--run-command", buildLinuxHostsCommand(params.Hostname),
		"--quiet",
	}
	if netplanCommand := buildLinuxNetplanMACCompatCommand(params.PrimaryMAC); netplanCommand != "" {
		args = append(args, "--run-command", netplanCommand)
	} else {
		args = append(args, "--run-command", buildLinuxNetplanDHCPHotplugCompatCommand())
	}
	// 为后续从面板热插的网口保留 DHCP 兜底规则，不覆盖主网口的 Netplan 配置。
	args = append(args, "--run-command", buildLinuxNetworkdDHCPHotplugFallbackCommand())

	// 7. 先按磁盘内真实账户准备目标用户，不能仅依赖模板元数据判断账户是否存在。
	if targetUser != "" && targetUser != "root" {
		args = append(args, "--run-command", buildEnsureLinuxCloneUserCommand(params.TemplateUser, targetUser))
	}

	// 8. 非 root 场景锁定 root，仅为目标用户设置密码；显式选择 root 时才启用 root 密码。
	if targetUser == "root" {
		if params.Password != "" {
			args = append(args, "--password", "root:password:"+params.Password)
		}
	} else {
		args = append(args, "--run-command", "usermod -L root")
		if params.Password != "" && targetUser != "" {
			args = append(args, "--password", targetUser+":password:"+params.Password)
		}
	}

	// 9. 阻塞式启动后命令：注入 systemd oneshot 服务，在 SSH 启动前执行用户命令
	// 服务配置为 Before=sshd/ssh，确保命令完成前 SSH 不可用
	if strings.TrimSpace(params.PostBootCommand) != "" && params.PostBootBlocking {
		scriptContent := buildPostBootBlockingScript(params.PostBootCommand)
		unitContent := buildPostBootBlockingUnit()

		scriptPath := filepath.Join(tmpDir, "qvm-post-boot.sh")
		unitPath := filepath.Join(tmpDir, "qvm-post-boot.service")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			return fmt.Errorf("写入阻塞启动脚本失败: %w", err)
		}
		if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
			return fmt.Errorf("写入阻塞启动服务单元失败: %w", err)
		}

		args = append(args,
			"--upload", scriptPath+":/opt/qvm-post-boot.sh",
			"--upload", unitPath+":/etc/systemd/system/qvm-post-boot.service",
			"--run-command", "chmod +x /opt/qvm-post-boot.sh",
			"--run-command", "ln -sf /etc/systemd/system/qvm-post-boot.service /etc/systemd/system/multi-user.target.wants/qvm-post-boot.service",
		)
	}

	// virt-customize 需要读写整个磁盘镜像，属于大 IO 操作，不设置自动超时
	result := utils.ExecCommandNoTimeout("virt-customize", args...)
	if result.Error != nil {
		return fmt.Errorf("Linux 克隆离线初始化失败: %s", D.FirstNonEmpty(result.Stderr, result.Error.Error()))
	}
	logger.App.Info("Linux 离线初始化完成（NoCloud）", "vm", params.Name, "hostname", params.Hostname)
	return nil
}

// buildNoCloudMetaData 生成 cloud-init meta-data YAML 内容
// instance-id 每次克隆唯一，确保 cloud-init 识别为首次启动
func buildNoCloudMetaData(params *CloneParams) string {
	instanceID := fmt.Sprintf("iid-%s-%d", params.Name, time.Now().Unix())
	return fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceID, params.Hostname)
}

// buildNoCloudUserData 生成 cloud-init user-data cloud-config 内容
// 负责 hostname 确认 + 用户密码解锁 + 磁盘自动扩容；密码/用户名已在 virt-customize 阶段离线写入 /etc/shadow
func buildNoCloudUserData(params *CloneParams) string {
	var sb strings.Builder
	targetUser := resolveLinuxCloneTargetUser(params.User, params.TemplateUser)
	permitRoot := linuxPermitRootLoginValue(targetUser)
	rootLoginAllowed := targetUser == "root"

	sb.WriteString("#cloud-config\n\n")
	sb.WriteString(fmt.Sprintf("hostname: %s\n", params.Hostname))
	sb.WriteString("manage_etc_hosts: true\n\n")
	sb.WriteString("ssh_pwauth: true\n")
	sb.WriteString(fmt.Sprintf("disable_root: %t\n\n", !rootLoginAllowed))

	// 防止 cloud-init 重新锁定用户密码
	// Ubuntu 等发行版 cloud.cfg 中 default_user 设置 lock_passwd: true，
	// 首次启动时 cloud-init 会在 /etc/shadow 密码哈希前添加 '!' 导致无法登录
	// 此处显式声明 lock_passwd: false，确保 virt-customize 离线设置的密码不被覆盖
	if targetUser != "" && targetUser != "root" {
		sb.WriteString("users:\n")
		sb.WriteString(fmt.Sprintf("  - name: %s\n", targetUser))
		sb.WriteString("    lock_passwd: false\n")
		sb.WriteString("    shell: /bin/bash\n")
		sb.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n\n")
	}

	sb.WriteString("chpasswd:\n  expire: false\n\n")

	// growpart 对普通分区有效；对 LVM 系统由下方 runcmd 补充处理
	sb.WriteString("growpart:\n  mode: auto\n  devices: ['/']\nresize_rootfs: true\n\n")
	sb.WriteString("runcmd:\n")
	sb.WriteString(fmt.Sprintf("  - hostnamectl set-hostname %s 2>/dev/null || true\n", params.Hostname))
	// 确保 cloud-init 执行后 SSH 配置不被覆盖（部分发行版 cloud-init 会重置 sshd_config）
	sb.WriteString("  - |\n")
	sb.WriteString(fmt.Sprintf("    sed -i 's/^\\s*#\\?\\s*PermitRootLogin.*/PermitRootLogin %s/' /etc/ssh/sshd_config 2>/dev/null || true\n", permitRoot))
	sb.WriteString("    sed -i 's/^\\s*#\\?\\s*PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config 2>/dev/null || true\n")
	sb.WriteString(fmt.Sprintf("    if [ -d /etc/ssh/sshd_config.d ]; then for f in /etc/ssh/sshd_config.d/*.conf; do [ -f \"$f\" ] || continue; sed -i 's/^\\s*PermitRootLogin.*/PermitRootLogin %s/' \"$f\"; sed -i 's/^\\s*PasswordAuthentication.*/PasswordAuthentication yes/' \"$f\"; done; fi\n", permitRoot))
	sb.WriteString("    systemctl restart sshd 2>/dev/null || systemctl restart ssh 2>/dev/null || true\n")
	// LVM 感知磁盘扩容脚本：自动检测根分区是否为 LVM，并执行 pvresize + lvextend
	// 使用 /sys/class/block 获取父磁盘和分区号，比 lsblk pkname/partn 更可靠
	sb.WriteString("  - |\n")
	sb.WriteString("    set +e\n")
	sb.WriteString("    ROOT_DEV=$(findmnt -n -o SOURCE /)\n")
	sb.WriteString("    if echo \"$ROOT_DEV\" | grep -q 'mapper'; then\n")
	sb.WriteString("      VG=$(lvs --noheadings -o vg_name \"$ROOT_DEV\" 2>/dev/null | awk '{print $1}' | head -1)\n")
	sb.WriteString("      PV=$(pvs --noheadings -o pv_name,vg_name 2>/dev/null | awk -v vg=\"$VG\" '$2==vg{print $1;exit}')\n")
	sb.WriteString("      if [ -n \"$PV\" ]; then\n")
	sb.WriteString("        PV_NAME=$(basename \"$PV\")\n")
	sb.WriteString("        SYS=$(readlink -f \"/sys/class/block/$PV_NAME\")\n")
	sb.WriteString("        DISK=$(basename \"$(dirname \"$SYS\")\")\n")
	sb.WriteString("        PARTNUM=$(cat \"/sys/class/block/$PV_NAME/partition\" 2>/dev/null)\n")
	sb.WriteString("        if [ -n \"$DISK\" ] && [ -n \"$PARTNUM\" ]; then\n")
	sb.WriteString("          growpart \"/dev/$DISK\" \"$PARTNUM\" 2>/dev/null || true\n")
	sb.WriteString("          pvresize \"$PV\" 2>/dev/null || true\n")
	sb.WriteString("          lvextend -r -l +100%FREE \"$ROOT_DEV\" 2>/dev/null || true\n")
	sb.WriteString("        fi\n")
	sb.WriteString("      fi\n")
	sb.WriteString("    fi\n")

	// 启动后执行的自定义命令（来自模板元数据）
	// 非阻塞模式：放在 cloud-init runcmd 中，不影响 SSH 启动
	if strings.TrimSpace(params.PostBootCommand) != "" && !params.PostBootBlocking {
		// 使用 bash heredoc 包装，确保 bash 特有语法（如进程替换 <(...))在 /bin/sh 环境下正常执行
		sb.WriteString("  - |\n")
		sb.WriteString("    /bin/bash <<'QVMPOSTBOOT'\n")
		for _, line := range strings.Split(params.PostBootCommand, "\n") {
			sb.WriteString("    " + line + "\n")
		}
		sb.WriteString("    QVMPOSTBOOT\n")
	}

	return sb.String()
}

// buildPostBootBlockingScript 生成阻塞式启动后命令的 shell 脚本内容
// 脚本执行完毕后自动禁用服务并清理自身，确保仅首次启动时运行
func buildPostBootBlockingScript(command string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("# QVMConsole 阻塞式启动后命令 - 仅首次启动时执行\n")
	sb.WriteString("# 此服务在 SSH 启动前运行，阻塞系统启动直到命令完成\n\n")
	// 执行用户自定义命令
	for _, line := range strings.Split(command, "\n") {
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n")
	// 执行完毕后自动禁用服务并清理文件
	sb.WriteString("# 清理：禁用服务并删除自身\n")
	sb.WriteString("systemctl disable qvm-post-boot.service 2>/dev/null || true\n")
	sb.WriteString("rm -f /etc/systemd/system/qvm-post-boot.service\n")
	sb.WriteString("rm -f /etc/systemd/system/multi-user.target.wants/qvm-post-boot.service\n")
	sb.WriteString("rm -f /opt/qvm-post-boot.sh\n")
	sb.WriteString("systemctl daemon-reload 2>/dev/null || true\n")
	return sb.String()
}

// buildPostBootBlockingUnit 生成 systemd oneshot 服务单元文件内容
// 配置为 Before=sshd.service ssh.service，确保命令完成前 SSH 不可用
func buildPostBootBlockingUnit() string {
	return `[Unit]
Description=QVMConsole Post-Boot Initialization (blocking)
After=network-online.target
Before=sshd.service ssh.service
Wants=network-online.target
ConditionPathExists=/opt/qvm-post-boot.sh

[Service]
Type=oneshot
RemainAfterExit=no
ExecStart=/bin/bash /opt/qvm-post-boot.sh
TimeoutStartSec=infinity
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`
}
