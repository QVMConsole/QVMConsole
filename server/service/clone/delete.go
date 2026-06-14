package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/ip_resolver"
	"kvm_console/service/libvirt_rpc"
	netpkg "kvm_console/service/network"
	"kvm_console/service/snapshot"
	"kvm_console/service/storage/disk"
	"kvm_console/utils"

	"github.com/digitalocean/go-libvirt"
)

// DeleteVM 删除虚拟机（含磁盘、静态IP、端口转发）- 兼容接口，删除所有磁盘
func DeleteVM(name string) error {
	return DeleteVMWithDisks(name, nil, nil, "")
}

// DeleteVMWithDisks 删除虚拟机（支持选择性删除/转移磁盘）
func DeleteVMWithDisks(name string, deleteDisks []string, transferDisks []string, transferUser string) error {
	if err := D.HookEnsureVMNotMigrating(name, "删除虚拟机"); err != nil {
		return err
	}

	var warnings []string

	// 检查虚拟机磁盘文件是否仍然存在，避免因磁盘丢失导致快照清理失败
	diskFilesExist := vmHasDiskFiles(name)

	if diskFilesExist {
		if _, err := snapshot.DeleteAllSnapshots(name, nil); err != nil {
			logger.App.Warn("删除虚拟机快照失败", "vm", name, "error", err)
			warnings = append(warnings, fmt.Sprintf("清理快照失败: %v", err))
		}
	} else {
		logger.App.Warn("虚拟机磁盘文件已丢失，跳过快照清理", "vm", name)
	}

	// 强制关机
	_ = libvirt_rpc.DestroyDomainRPC(name)
	time.Sleep(1 * time.Second)

	// 在 undefine 之前收集虚拟机的所有 IP 地址
	vmIPs := collectVMIPs(name)

	// 解绑静态 IP
	unbindErr := netpkg.UnbindStaticIP(name)
	if unbindErr != nil {
		logger.App.Warn("清理静态IP绑定失败", "error", unbindErr)
	}

	// 无论是否有静态绑定，都确保清理所有关联 IP 的端口转发规则
	for _, ip := range vmIPs {
		netpkg.RemovePortForwardsForIP(ip)
	}

	// 如果没有指定磁盘列表，收集所有磁盘路径用于删除
	var allDiskPaths []string
	if deleteDisks == nil && transferDisks == nil {
		if diskFilesExist {
			disks, err := disk.ListDisks(name)
			if err == nil {
				for _, d := range disks {
					if d.Path != "" && d.DeviceType != "cdrom" {
						allDiskPaths = append(allDiskPaths, d.Path)
					}
				}
			}
		}
	}

	// 弹出 cdrom
	utils.ExecShell(fmt.Sprintf("virsh change-media %s hda --eject 2>/dev/null || true", utils.ShellSingleQuote(name)))

	// 取消定义
	if err := libvirt_rpc.UndefineDomainRPC(name, libvirt.DomainUndefineNvram|libvirt.DomainUndefineSnapshotsMetadata); err != nil {
		if err2 := libvirt_rpc.UndefineDomainRPC(name, libvirt.DomainUndefineSnapshotsMetadata); err2 != nil {
			if strings.Contains(err2.Error(), "not found") {
				logger.App.Warn("虚拟机已不存在，清理完成", "vm", name)
			} else {
				logger.App.Warn("取消虚拟机定义失败", "vm", name, "error", err2)
				warnings = append(warnings, fmt.Sprintf("取消虚拟机定义失败: %v", err2))
			}
		}
	}

	// 清理 Windows Config Drive ISO（如有）
	CleanupWindowsConfigDriveISO(name)

	// 处理磁盘：删除指定的磁盘
	deleteDiskList := deleteDisks
	if deleteDiskList == nil && len(allDiskPaths) > 0 {
		deleteDiskList = allDiskPaths
	}

	// 获取模板目录，用于保护模板文件不被误删
	templateDir := ""
	if config.GlobalConfig != nil {
		templateDir = filepath.Clean(config.GlobalConfig.TemplateDir)
	}

	for _, diskPath := range deleteDiskList {
		if diskPath == "" {
			continue
		}

		// 安全检查：禁止删除模板目录下的文件（防止模板被误删）
		if templateDir != "" {
			cleanedPath := filepath.Clean(diskPath)
			if isPathUnderDir(cleanedPath, templateDir) {
				logger.App.Warn("跳过删除模板目录下的磁盘文件（模板保护），VM可能错误引用了模板文件",
					"vm", name, "disk", diskPath, "template_dir", templateDir)
				// 注意：这不是删除失败，而是保护性跳过，不应阻止任务完成
				continue
			}
		}

		if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
			logger.App.Warn("删除磁盘文件失败", "path", diskPath, "error", err)
			warnings = append(warnings, fmt.Sprintf("删除磁盘 %s 失败: %v", diskPath, err))
		}
	}

	// 转移需要保留的磁盘到用户存储
	if len(transferDisks) > 0 && transferUser != "" {
		diskDir := D.GetUserDiskDir(transferUser)
		utils.ExecCommand("mkdir", "-p", diskDir)
		for _, diskPath := range transferDisks {
			if diskPath == "" {
				continue
			}
			diskExists := utils.ExecShell(fmt.Sprintf("test -f %s && echo yes", utils.ShellSingleQuote(diskPath)))
			if strings.TrimSpace(diskExists.Stdout) != "yes" {
				logger.App.Warn("转移磁盘失败，文件不存在", "path", diskPath)
				continue
			}
			filename := filepath.Base(diskPath)
			destPath := filepath.Join(diskDir, filename)
			checkResult := utils.ExecShell(fmt.Sprintf("test -f %s && echo exists", utils.ShellSingleQuote(destPath)))
			if strings.TrimSpace(checkResult.Stdout) == "exists" {
				ts := time.Now().Format("20060102150405")
				ext := filepath.Ext(filename)
				nameOnly := strings.TrimSuffix(filename, ext)
				destPath = filepath.Join(diskDir, fmt.Sprintf("%s_%s%s", nameOnly, ts, ext))
			}
			if err := os.Rename(diskPath, destPath); err != nil {
				logger.App.Warn("转移磁盘到用户存储失败", "path", diskPath, "error", err)
			} else {
				utils.ExecCommand("chown", "libvirt-qemu:kvm", destPath)
			}
		}
	}

	// 清理资源历史记录
	D.DeleteVMStatsRecords(name)
	D.DeleteVMRuntimeRecord(name)
	D.CleanupVMVPCBinding(name)
	D.CleanupLightweightVMResources(name)
	if err := D.DeleteVMSchedules(name); err != nil {
		logger.App.Warn("清理虚拟机定时任务失败", "vm", name, "error", err)
		warnings = append(warnings, fmt.Sprintf("清理定时任务失败: %v", err))
	}

	if len(warnings) > 0 {
		return fmt.Errorf("删除虚拟机 %s 部分步骤失败: %s", name, strings.Join(warnings, "; "))
	}
	return nil
}

// ForceDeleteVM 强制删除僵尸虚拟机，绕过磁盘和快照操作
func ForceDeleteVM(name string) error {
	logger.App.Info("开始强制清理虚拟机", "vm", name)

	_ = libvirt_rpc.DestroyDomainRPC(name)

	vmIPs := collectVMIPs(name)
	_ = netpkg.UnbindStaticIP(name)
	for _, ip := range vmIPs {
		netpkg.RemovePortForwardsForIP(ip)
	}

	if err := libvirt_rpc.UndefineDomainRPC(name, libvirt.DomainUndefineNvram|libvirt.DomainUndefineSnapshotsMetadata); err != nil {
		if err2 := libvirt_rpc.UndefineDomainRPC(name, libvirt.DomainUndefineSnapshotsMetadata); err2 != nil {
			if err3 := libvirt_rpc.UndefineDomainRPC(name, 0); err3 != nil {
				logger.App.Warn("undefine失败，尝试直接清理libvirt配置文件", "error", err3)

				xmlPath := fmt.Sprintf("/etc/libvirt/qemu/%s.xml", name)
				if removeErr := os.Remove(xmlPath); removeErr != nil && !os.IsNotExist(removeErr) {
					logger.App.Warn("删除XML配置文件失败", "error", removeErr)
				}

				autostartLink := fmt.Sprintf("/etc/libvirt/qemu/autostart/%s.xml", name)
				_ = os.Remove(autostartLink)

				vncPortFile := fmt.Sprintf("/etc/kvm-console/vnc-ports/%s", name)
				_ = os.Remove(vncPortFile)

				restartResult := utils.ExecCommand("systemctl", "restart", "libvirtd")
				if restartResult.Error != nil {
					logger.App.Warn("重启libvirtd失败", "error", restartResult.Error)
				}
				time.Sleep(3 * time.Second)
			}
		}
	}

	D.DeleteVMStatsRecords(name)
	D.DeleteVMRuntimeRecord(name)
	D.CleanupVMVPCBinding(name)
	D.CleanupLightweightVMResources(name)
	_ = D.DeleteVMSchedules(name)

	// 清理 Windows Config Drive ISO（如有）
	CleanupWindowsConfigDriveISO(name)

	logger.App.Info("虚拟机强制清理完成", "vm", name)
	return nil
}

// vmHasDiskFiles 检查虚拟机是否还有至少一个存在的磁盘文件
func vmHasDiskFiles(name string) bool {
	disks, err := disk.ListDisks(name)
	if err != nil {
		return false
	}
	for _, d := range disks {
		if d.Path == "" || d.DeviceType == "cdrom" {
			continue
		}
		checkResult := utils.ExecShell(fmt.Sprintf("test -f %s && echo yes", utils.ShellSingleQuote(d.Path)))
		if strings.TrimSpace(checkResult.Stdout) == "yes" {
			return true
		}
	}
	return false
}

// collectVMIPs 收集虚拟机的所有关联 IP 地址
func collectVMIPs(vmName string) []string {
	ipSet := make(map[string]bool)

	mac := ip_resolver.GetFirstVMMAC(vmName)
	if mac == "" {
		return nil
	}

	if ip := D.GetOVSStaticIPByMAC(mac); ip != "" {
		ipSet[ip] = true
	}
	if allVpcHosts, err := D.ListAllVPCStaticHosts(); err == nil {
		for _, host := range allVpcHosts {
			if strings.EqualFold(host.MAC, mac) {
				ipSet[host.IP] = true
			}
		}
	}

	if ip := D.GetOVSLeaseIPByMAC(mac); ip != "" {
		ipSet[ip] = true
	}

	ipRe := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`)
	for _, source := range []string{"agent", "arp", "lease"} {
		addrResult := utils.ExecCommandQuiet("virsh", "domifaddr", vmName, "--source", source)
		if addrResult.Error == nil {
			allMatches := ipRe.FindAllStringSubmatch(addrResult.Stdout, -1)
			for _, m := range allMatches {
				if m[1] != "127.0.0.1" {
					ipSet[m[1]] = true
				}
			}
		}
	}

	var ips []string
	for ip := range ipSet {
		ips = append(ips, ip)
	}
	return ips
}

// CheckDiskTransferQuota 检查转移磁盘是否有足够的存储配额
func CheckDiskTransferQuota(username string, diskPaths []string) (int64, error) {
	if len(diskPaths) == 0 {
		return 0, nil
	}

	var totalBytes int64
	for _, diskPath := range diskPaths {
		duResult := utils.ExecShell(fmt.Sprintf("du -b %s 2>/dev/null | awk '{print $1}'", utils.ShellSingleQuote(diskPath)))
		if duResult.Error == nil {
			size, _ := strconv.ParseInt(strings.TrimSpace(duResult.Stdout), 10, 64)
			totalBytes += size
		}
	}

	if err := D.CheckStorageQuota(username, totalBytes); err != nil {
		return totalBytes, err
	}

	return totalBytes, nil
}

// isPathUnderDir 检查给定路径是否在指定目录下（含目录本身）
func isPathUnderDir(path, dir string) bool {
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)
	if path == dir {
		return true
	}
	// 确保 dir 以路径分隔符结尾再比较前缀
	dirWithSep := dir + string(filepath.Separator)
	return strings.HasPrefix(path, dirWithSep)
}
