package vm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

// ExportVMParams 导出虚拟机参数
type ExportVMParams struct {
	VMName   string `json:"vm_name"`  // 虚拟机名称
	Username string `json:"username"` // 导出到哪个用户的存储
}

// ExportVMResult 导出结果
type ExportVMResult struct {
	VMName   string `json:"vm_name"`
	FileName string `json:"file_name"`
	FilePath string `json:"file_path"`
	FileSize string `json:"file_size"`
}

// GetVMExportSize 预估导出文件大小（字节）
// 链式克隆：计算整个 backing chain 的 actual-size 之和（接近 convert 后大小）
// 独立磁盘：返回实际文件大小
func GetVMExportSize(vmName string) (int64, error) {
	diskInfo := GetVMDiskInfo(vmName)
	if diskInfo.Path == "" {
		return 0, fmt.Errorf("无法获取虚拟机 %s 的磁盘路径", vmName)
	}

	// 检查是否有 backing file（链式克隆）
	backingResult := utils.ExecShell(fmt.Sprintf(
		"qemu-img info -U %s 2>/dev/null | grep 'backing file:'", utils.ShellSingleQuote(diskInfo.Path)))
	hasBacking := backingResult.Error == nil && strings.TrimSpace(backingResult.Stdout) != ""

	if hasBacking {
		// 链式克隆：计算整个 backing chain 的实际数据量之和
		// 使用 --backing-chain 获取所有层的 actual-size
		chainResult := utils.ExecShell(fmt.Sprintf(
			"qemu-img info -U --backing-chain --output=json %s 2>/dev/null", utils.ShellSingleQuote(diskInfo.Path)))
		if chainResult.Error == nil && strings.TrimSpace(chainResult.Stdout) != "" {
			var totalSize int64
			// 简单解析所有 actual-size 字段
			for _, line := range strings.Split(chainResult.Stdout, "\n") {
				line = strings.TrimSpace(line)
				if strings.Contains(line, `"actual-size"`) {
					line = strings.TrimPrefix(line, `"actual-size":`)
					line = strings.TrimSuffix(line, ",")
					line = strings.TrimSpace(line)
					var size int64
					fmt.Sscanf(line, "%d", &size)
					totalSize += size
				}
			}
			if totalSize > 0 {
				// 加 10% 余量，因为 convert 后可能略有差异
				return totalSize * 110 / 100, nil
			}
		}
	}

	// 独立磁盘：返回实际文件大小
	if fi, err := os.Stat(diskInfo.Path); err == nil && fi.Size() > 0 {
		return fi.Size(), nil
	}

	return 0, fmt.Errorf("无法获取磁盘大小")
}

// ExportVM 导出虚拟机磁盘到用户存储
func ExportVM(ctx context.Context, params *ExportVMParams, progressFn func(int, string)) (*ExportVMResult, error) {
	// 检查虚拟机是否存在（存在性探测：失败会返回给调用方，仅记 DEBUG）
	checkVM := utils.ExecCommandQuiet("virsh", "dominfo", params.VMName)
	if checkVM.ExitCode != 0 {
		return nil, fmt.Errorf("虚拟机 '%s' 不存在", params.VMName)
	}

	// 获取磁盘路径
	diskInfo := GetVMDiskInfo(params.VMName)
	if diskInfo.Path == "" {
		return nil, fmt.Errorf("无法获取虚拟机 %s 的磁盘路径", params.VMName)
	}

	progressFn(10, "获取磁盘信息...")

	// 生成导出文件名
	timestamp := time.Now().Format("20060102-150405")
	exportFileName := fmt.Sprintf("%s-export-%s.qcow2", params.VMName, timestamp)
	exportDir := D.GetUserDiskDir(params.Username)
	exportPath := filepath.Join(exportDir, exportFileName)

	// 确保目标目录存在
	utils.ExecCommand("mkdir", "-p", exportDir)

	// 判断是否使用系统用户执行写入（使 project quota 硬限制生效）
	// admin 没有系统用户，仍用 root；普通用户用 sudo -u 执行
	useSudo := false
	if params.Username != "admin" {
		checkUser := utils.ExecShell(fmt.Sprintf("id %s 2>/dev/null", utils.ShellSingleQuote(params.Username)))
		if checkUser.Error == nil && strings.TrimSpace(checkUser.Stdout) != "" {
			useSudo = true
			// 确保用户在 kvm 组中（兼容旧用户）
			utils.ExecCommand("usermod", "-aG", "kvm", params.Username)
		}
	}

	// 检查取消
	select {
	case <-ctx.Done():
		return nil, taskqueue.ErrTaskCanceled
	default:
	}

	// 检查是否有 backing file（链式克隆）
	backingResult := utils.ExecShell(fmt.Sprintf(
		"qemu-img info -U %s 2>/dev/null | grep 'backing file:'", utils.ShellSingleQuote(diskInfo.Path)))
	hasBacking := backingResult.Error == nil && strings.TrimSpace(backingResult.Stdout) != ""

	if hasBacking {
		// 链式克隆：使用 qemu-img convert 合并为独立文件
		progressFn(20, "检测到链式克隆磁盘，正在合并导出（可能需要较长时间）...")
		logger.App.Info("导出 VM: 链式克隆磁盘，使用 qemu-img convert 合并", "vm", params.VMName)

		var convertCmd string
		if useSudo {
			convertCmd = fmt.Sprintf("sudo -u %s qemu-img convert -O qcow2 %s %s",
				utils.ShellSingleQuote(params.Username), utils.ShellSingleQuote(diskInfo.Path), utils.ShellSingleQuote(exportPath))
		} else {
			convertCmd = fmt.Sprintf("qemu-img convert -O qcow2 %s %s",
				utils.ShellSingleQuote(diskInfo.Path), utils.ShellSingleQuote(exportPath))
		}
		result := utils.ExecShellWithTimeout(convertCmd, 2*time.Hour)
		if result.Error != nil {
			// 清理不完整的文件
			_ = os.Remove(exportPath)
			return nil, fmt.Errorf("导出失败（qemu-img convert）: %s", result.Stderr)
		}
	} else {
		// 独立磁盘：使用 cp 复制
		progressFn(20, "正在复制磁盘文件...")
		logger.App.Info("导出 VM: 独立磁盘，使用 cp 复制", "vm", params.VMName)

		var result *utils.CmdResult
		if useSudo {
			result = utils.ExecCommandLongRunning("sudo", "-u", params.Username, "cp", "--sparse=always", diskInfo.Path, exportPath)
		} else {
			result = utils.ExecCommandLongRunning("cp", "--sparse=always", diskInfo.Path, exportPath)
		}
		if result.Error != nil {
			_ = os.Remove(exportPath)
			return nil, fmt.Errorf("导出失败（cp）: %s", result.Stderr)
		}
	}

	// 检查取消
	select {
	case <-ctx.Done():
		_ = os.Remove(exportPath)
		return nil, taskqueue.ErrTaskCanceled
	default:
	}

	progressFn(90, "设置文件权限...")

	// 设置文件权限（确保 VM 和 web 服务都能访问）
	_ = utils.ChownLibvirtQEMU(exportPath)

	// 获取导出文件大小
	sizeResult := utils.ExecShell(fmt.Sprintf("du -h %s | awk '{print $1}'", utils.ShellSingleQuote(exportPath)))
	fileSize := "未知"
	if sizeResult.Error == nil {
		fileSize = strings.TrimSpace(sizeResult.Stdout)
	}

	progressFn(100, fmt.Sprintf("导出完成，文件大小: %s", fileSize))

	return &ExportVMResult{
		VMName:   params.VMName,
		FileName: exportFileName,
		FilePath: exportPath,
		FileSize: fileSize,
	}, nil
}
