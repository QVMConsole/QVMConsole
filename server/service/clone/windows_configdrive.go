package clone

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

const windowsConfigDriveInitDir = "/var/lib/libvirt/qemu/init"

// CloudbaseInit 初始化弹出策略相关常量：
// - guestAgentPollInterval：每次尝试 Guest Agent ping 的间隔
// - guestAgentTimeout：Guest Agent 未在此时间内响应则放弃弹出（处理未安装 Agent 的模板）
// - serviceCheckInterval：检测到 Agent 后轮询 cloudbase-init 服务状态的间隔
// - serviceCompletionTimeout：等待 cloudbase-init 服务完成的最大时间
// - serviceCheckGracePeriod：服务状态确认为 STOPPED 后额外等待，确保所有收尾操作完成
const (
	configDriveGuestAgentPollInterval   = 15 * time.Second
	configDriveGuestAgentTimeout        = 15 * time.Minute
	configDriveServiceCheckInterval     = 10 * time.Second
	configDriveServiceCompletionTimeout = 10 * time.Minute
	configDriveServiceCheckGracePeriod  = 15 * time.Second
)

// windowsConfigDriveISOPath 返回指定虚拟机的 Config Drive ISO 存储路径
func windowsConfigDriveISOPath(vmName string) string {
	return filepath.Join(windowsConfigDriveInitDir, vmName+"-configdrive.iso")
}

// configDriveCDROMTarget 根据系统盘总线类型返回 Config Drive CD-ROM 的目标设备名和总线。
// 确保不与系统盘目标设备发生冲突。
func configDriveCDROMTarget(diskBus string) (dev, bus string) {
	switch strings.ToLower(strings.TrimSpace(diskBus)) {
	case "sata", "scsi":
		// sata/scsi 系统盘占用 sda，CD-ROM 使用 sdb（同为 SATA 总线）
		return "sdb", "sata"
	case "ide":
		// ide 系统盘占用 hda，CD-ROM 使用 hdb
		return "hdb", "ide"
	default:
		// virtio 系统盘占用 vda，SATA 的 sda 空闲
		return "sda", "sata"
	}
}

// addConfigDriveCDROMToXML 向 VM 域 XML 的 </devices> 前注入 Config Drive CD-ROM 设备定义
func addConfigDriveCDROMToXML(vmXML, isoPath, diskBus string) string {
	cdDev, cdBus := configDriveCDROMTarget(diskBus)
	cdromXML := fmt.Sprintf(
		"\n    <disk type='file' device='cdrom'>\n"+
			"      <driver name='qemu' type='raw'/>\n"+
			"      <source file='%s'/>\n"+
			"      <target dev='%s' bus='%s'/>\n"+
			"      <readonly/>\n"+
			"    </disk>",
		isoPath, cdDev, cdBus,
	)
	return strings.Replace(vmXML, "</devices>", cdromXML+"\n  </devices>", 1)
}

// removeConfigDriveCDROMFromXML 从 VM 域 XML 中移除已有的 Config Drive CD-ROM 挂载。
// 通过检查磁盘 source 路径是否包含 windowsConfigDriveInitDir 来识别。
func removeConfigDriveCDROMFromXML(vmXML string) string {
	lines := strings.Split(vmXML, "\n")
	result := make([]string, 0, len(lines))

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// 检测 cdrom disk 块的起始
		if strings.Contains(trimmed, "<disk ") && strings.Contains(trimmed, "device='cdrom'") {
			// 向后扫描，查找是否有 configdrive.iso 路径引用
			found := false
			j := i
			for j < len(lines) {
				if strings.Contains(lines[j], windowsConfigDriveInitDir) &&
					strings.Contains(lines[j], "configdrive.iso") {
					found = true
				}
				tj := strings.TrimSpace(lines[j])
				if tj == "</disk>" || strings.HasSuffix(tj, "</disk>") {
					j++
					break
				}
				j++
			}
			if found {
				// 跳过整个 cdrom 块
				i = j
				continue
			}
		}

		result = append(result, line)
		i++
	}
	return strings.Join(result, "\n")
}

// windowsConfigDriveMetadata 是写入 meta_data.json 的 OpenStack ConfigDrive 元数据结构
type windowsConfigDriveMetadata struct {
	UUID          string `json:"uuid"`
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	AdminPass     string `json:"admin_pass"`
	AdminUsername string `json:"admin_username"`
}

// createWindowsConfigDriveISO 创建符合 OpenStack ConfigDrive 规范的 ISO 镜像（label=config-2）。
// 挂载到虚拟机后，CloudbaseInit 的 ConfigDriveService 将读取 openstack/latest/meta_data.json，
// 自动完成主机名设置和管理员密码注入，无需在磁盘中写入 unattend.xml。
func createWindowsConfigDriveISO(vmName, hostname, password string) (string, error) {
	// 生成唯一 instance-id，CloudbaseInit 依赖此字段实现幂等（重启不重复执行插件）
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	instanceID := fmt.Sprintf("%s-%s", vmName, hex.EncodeToString(randBytes))

	meta := windowsConfigDriveMetadata{
		UUID:          instanceID,
		Name:          vmName,
		Hostname:      hostname,
		AdminPass:     password,
		AdminUsername: "Administrator",
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 Config Drive 元数据失败: %w", err)
	}

	// 构造临时目录结构：openstack/latest/meta_data.json
	tmpDir := fmt.Sprintf("/tmp/_cfgdrive-%s", vmName)
	metaDir := filepath.Join(tmpDir, "openstack", "latest")
	if err := os.MkdirAll(metaDir, 0700); err != nil {
		return "", fmt.Errorf("创建 Config Drive 临时目录失败: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	metaPath := filepath.Join(metaDir, "meta_data.json")
	if err := os.WriteFile(metaPath, metaJSON, 0600); err != nil {
		return "", fmt.Errorf("写入 Config Drive meta_data.json 失败: %w", err)
	}

	// 写入 user_data PowerShell 脚本（由 UserDataPlugin 执行）
	// 在 SetUserPasswordPlugin 设置密码之后运行，清除「登录前必须更改密码」标志，确保用户可用 ConfigDrive 中的密码直接登录。
	userDataPath := filepath.Join(metaDir, "user_data")
	userDataScript := "#ps1_sysnative\r\n" +
		"net user Administrator /logonpasswordchg:no /active:yes\r\n"
	if err := os.WriteFile(userDataPath, []byte(userDataScript), 0600); err != nil {
		return "", fmt.Errorf("写入 Config Drive user_data 失败: %w", err)
	}

	// 确保 ISO 存储目录存在并设置权限
	if err := os.MkdirAll(windowsConfigDriveInitDir, 0755); err != nil {
		return "", fmt.Errorf("创建 Config Drive ISO 目录失败: %w", err)
	}
	_ = utils.ChownLibvirtQEMU(windowsConfigDriveInitDir)

	isoPath := windowsConfigDriveISOPath(vmName)
	// 清除旧 ISO（重建/重装场景）
	_ = os.Remove(isoPath)

	// genisoimage 创建 ISO（label=config-2 为 OpenStack ConfigDrive 标准卷标）
	result := utils.ExecCommandLongRunning(
		"genisoimage",
		"-output", isoPath,
		"-V", "config-2",
		"-input-charset", "utf-8",
		"-joliet", "-rock",
		tmpDir,
	)
	if result.Error != nil {
		return "", fmt.Errorf("创建 Config Drive ISO 失败: %s", strings.TrimSpace(result.Stderr))
	}

	if err := os.Chmod(isoPath, 0640); err != nil {
		logger.App.Warn("设置 Config Drive ISO 权限失败", "path", isoPath, "error", err)
	}
	_ = utils.ChownLibvirtQEMU(isoPath)

	return isoPath, nil
}

// CleanupWindowsConfigDriveISO 删除虚拟机对应的 Config Drive ISO 文件。
// 在虚拟机删除时调用，失败仅记录警告不中断流程。
func CleanupWindowsConfigDriveISO(vmName string) {
	isoPath := windowsConfigDriveISOPath(vmName)
	if err := os.Remove(isoPath); err != nil && !os.IsNotExist(err) {
		logger.App.Warn("清理 Windows Config Drive ISO 失败", "vm", vmName, "path", isoPath, "error", err)
	}
}

// isCloudbaseInitCompleted 通过 QEMU Guest Agent 检查 cloudbase-init 日志文件是否包含完成标记。
// CloudbaseInit 在执行完所有插件后一定会写入 "Plugins execution done"，这是确定性的完成信号。
// 避免依赖服务状态（服务未启动时也是 STOPPED，会导致误判）。
func isCloudbaseInitCompleted(vmName string) bool {
	// 使用 PowerShell Select-String 检测日志完成标记
	// 注意：virsh JSON 解析器要求 4 个反斜杠表示路径中的 1 个反斜杠，内部引号用 \" 转义
	execCmd := `{"execute":"guest-exec","arguments":{"path":"powershell.exe","arg":["-Command","if(Select-String -Path \"C:\\\\Program Files\\\\Cloudbase Solutions\\\\Cloudbase-Init\\\\log\\\\cloudbase-init.log\" -Pattern \"Plugins execution done\" -Quiet){exit 0}else{exit 1}"],"capture-output":true}}`
	r := utils.ExecCommand("virsh", "qemu-agent-command", "--timeout", "10", vmName, execCmd)
	if r.Error != nil {
		return false
	}

	// 解析返回的 PID
	var execResp struct {
		Return struct {
			PID int `json:"pid"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &execResp); err != nil || execResp.Return.PID == 0 {
		return false
	}

	// PowerShell 启动较慢，等待充足时间
	time.Sleep(8 * time.Second)
	statusCmd := fmt.Sprintf(`{"execute":"guest-exec-status","arguments":{"pid":%d}}`, execResp.Return.PID)
	r2 := utils.ExecCommand("virsh", "qemu-agent-command", "--timeout", "10", vmName, statusCmd)
	if r2.Error != nil {
		return false
	}

	// exit 0 表示找到完成标记
	var statusResp struct {
		Return struct {
			Exited   bool `json:"exited"`
			ExitCode int  `json:"exitcode"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(r2.Stdout), &statusResp); err != nil {
		return false
	}

	return statusResp.Return.Exited && statusResp.Return.ExitCode == 0
}

// scheduleWindowsConfigDriveEject 在后台轮询 QEMU Guest Agent，
// 检测到连接后通过 Agent 检查 cloudbase-init 日志中的完成标记，
// 确认初始化完成后再弹出并清理 Config Drive CD-ROM。
//
// 设计原则：
//   - 阶段一：轮询 Guest Agent 是否可达（处理早期启动阶段 Agent 未就绪）
//   - 阶段二：通过 Agent 检查日志文件是否包含 "Plugins execution done"
//     （不依赖服务状态，因为服务未启动时也是 STOPPED 会导致误判）
//   - 超时内未完成 → 停止轮询，ISO 将在 VM 删除时清理
func scheduleWindowsConfigDriveEject(vmName, diskBus string) {
	go func() {
		// === 阶段一：等待 Guest Agent 可达 ===
		agentDeadline := time.Now().Add(configDriveGuestAgentTimeout)
		agentDetected := false

		for time.Now().Before(agentDeadline) {
			time.Sleep(configDriveGuestAgentPollInterval)
			r := utils.ExecCommand("virsh", "qemu-agent-command",
				"--timeout", "5", vmName, `{"execute":"guest-ping"}`)
			if r.Error == nil {
				agentDetected = true
				break
			}
		}

		if !agentDetected {
			logger.App.Warn("Config Drive 弹出任务超时：未检测到 QEMU Guest Agent，跳过弹出",
				"vm", vmName, "timeout", configDriveGuestAgentTimeout)
			return
		}

		logger.App.Info("检测到 QEMU Guest Agent，开始轮询 cloudbase-init 日志完成标记",
			"vm", vmName)

		// === 阶段二：轮询 cloudbase-init 日志中的完成标记 ===
		serviceDeadline := time.Now().Add(configDriveServiceCompletionTimeout)
		serviceStopped := false

		for time.Now().Before(serviceDeadline) {
			if isCloudbaseInitCompleted(vmName) {
				serviceStopped = true
				break
			}
			time.Sleep(configDriveServiceCheckInterval)
		}

		if !serviceStopped {
			logger.App.Warn("等待 cloudbase-init 初始化完成超时，跳过弹出",
				"vm", vmName, "timeout", configDriveServiceCompletionTimeout)
			return
		}

		// 服务已停止，额外等待短暂时间确保所有收尾操作（如磁盘写入）完成
		logger.App.Info("cloudbase-init 初始化已完成，等待收尾后弹出 Config Drive",
			"vm", vmName, "grace_period", configDriveServiceCheckGracePeriod)
		time.Sleep(configDriveServiceCheckGracePeriod)

		// === 阶段三：弹出 CD-ROM 并清理 ===
		cdDev, _ := configDriveCDROMTarget(diskBus)
		// 优先尝试热弹出（VM 运行中）+ 更新持久化 XML
		r := utils.ExecCommand("virsh", "change-media", vmName, cdDev, "--eject", "--config", "--live")
		if r.Error != nil {
			// VM 已关机则只更新 XML
			r2 := utils.ExecCommand("virsh", "change-media", vmName, cdDev, "--eject", "--config")
			if r2.Error != nil {
				logger.App.Warn("自动弹出 Config Drive CD-ROM 失败",
					"vm", vmName, "dev", cdDev, "error", strings.TrimSpace(r2.Stderr))
				return
			}
		}

		CleanupWindowsConfigDriveISO(vmName)
		logger.App.Info("Config Drive CD-ROM 已自动弹出并清理", "vm", vmName)
	}()
}
