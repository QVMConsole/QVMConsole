package vmimport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service"
	"kvm_console/service/arch"
	"kvm_console/service/ip_resolver"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

// ImportVM 从磁盘文件导入虚拟机
func ImportVM(ctx context.Context, params *ImportVMParams, progressFn func(int, string)) (*ImportVMResult, error) {
	cloneDir := config.GlobalConfig.CloneDir
	if err := service.ValidateVMName(params.Name); err != nil {
		return nil, err
	}

	// 默认值
	if params.Network == "" {
		params.Network = config.GlobalConfig.DefaultNetwork
	}
	if !params.IsAdmin {
		params.CPULimitPercent = service.VMCPULimitUnlimited
	}
	if err := service.ValidateVMCPULimitPercent(params.CPULimitPercent); err != nil {
		return nil, err
	}
	params.CPUTopologyMode = service.NormalizeVMCPUTopologyMode(params.CPUTopologyMode)
	if params.Hostname == "" {
		params.Hostname = params.Name
	}

	// 根据宿主机架构解析机器类型和引导类型
	hostArch := arch.DetectHostArch()
	hostProfile := arch.GetProfile(hostArch)
	params.MachineType = arch.NormalizeMachineType(hostArch, params.MachineType)
	if params.MachineType == "" {
		params.MachineType = hostProfile.DefaultMachineType()
	} else if !slices.Contains(hostProfile.SupportedMachineTypes(), params.MachineType) {
		params.MachineType = hostProfile.DefaultMachineType()
	}
	if params.BootType == "" {
		params.BootType = hostProfile.DefaultBootType()
	} else if !slices.Contains(hostProfile.SupportedBootTypes(), params.BootType) {
		params.BootType = hostProfile.DefaultBootType()
	}
	if params.NicModel == "" {
		params.NicModel = "virtio"
	}

	// 检查虚拟机是否已存在（存在性探测：域不存在属预期情况，失败仅记 DEBUG）
	checkVM := utils.ExecCommandQuiet("virsh", "dominfo", params.Name)
	if checkVM.ExitCode == 0 {
		return nil, fmt.Errorf("虚拟机 '%s' 已存在", params.Name)
	}

	// 安全检查：文件名不能包含路径分隔符
	if strings.Contains(params.DiskFile, "/") || strings.Contains(params.DiskFile, "..") {
		return nil, fmt.Errorf("非法磁盘文件名: %s", params.DiskFile)
	}
	if ext := strings.ToLower(filepath.Ext(params.DiskFile)); ext == ".ova" || ext == ".ovf" || ext == ".mf" {
		return nil, fmt.Errorf("普通磁盘导入不接受 %s，请使用导入虚拟机功能", ext)
	}

	// 源磁盘路径（用户 disk 目录中）
	srcDiskPath := filepath.Join(service.GetUserDiskDir(params.Username), params.DiskFile)

	// 检查源文件是否存在
	if !utils.FileExists(srcDiskPath) {
		return nil, fmt.Errorf("磁盘文件不存在: %s", params.DiskFile)
	}
	if err := service.EnsureOVSNetworkReady(); err != nil {
		return nil, err
	}

	progressFn(5, "检测磁盘格式...")

	// 检查取消
	select {
	case <-ctx.Done():
		return nil, taskqueue.ErrTaskCanceled
	default:
	}

	// 确定目标磁盘路径（移动到 CloneDir）
	// 检测磁盘格式
	format := "qcow2"
	infoResult := utils.ExecShell(fmt.Sprintf("qemu-img info -U --output=json %s 2>/dev/null", utils.ShellSingleQuote(srcDiskPath)))
	if infoResult.Error == nil {
		detected := service.ParseQemuInfoStr(infoResult.Stdout, "format")
		if detected != "" {
			format = detected
		}
	}

	destDiskPath := filepath.Join(cloneDir, fmt.Sprintf("%s.%s", params.Name, format))

	// 根据 CopyDisk 选项决定移动还是复制
	if err := importVMCopyDisk(ctx, srcDiskPath, destDiskPath, format, params.CopyDisk, progressFn); err != nil {
		return nil, err
	}

	// 设置权限
	if err := utils.ChownLibvirtQEMU(destDiskPath); err != nil {
		os.Remove(destDiskPath)
		return nil, fmt.Errorf("设置磁盘权限失败: %w", err)
	}

	// 检查取消
	select {
	case <-ctx.Done():
		_ = os.Remove(destDiskPath)
		return nil, taskqueue.ErrTaskCanceled
	default:
	}

	progressFn(30, "创建虚拟机定义...")

	memoryMeta, ramMB, _, err := vm_memory.BuildVMMemoryMetadataForCreate(params.RAM, params.MemoryDynamic)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return nil, err
	}

	// 检测是否为 UEFI 磁盘
	normalizedBootType := vm_xml.NormalizeVMBootType(params.BootType)
	needUEFI := false
	if normalizedBootType == vm_xml.VMBootTypeUEFI || normalizedBootType == vm_xml.VMBootTypeUEFISecure {
		needUEFI = true
	} else if params.BootType == "" || params.BootType == "bios" {
		// 自动检测
		efiCheck := utils.ExecShell(fmt.Sprintf(
			"virt-filesystems -a %s --filesystems --long 2>/dev/null | head -5 | grep -q 'vfat' && echo 'uefi'",
			utils.ShellSingleQuote(destDiskPath)))
		if efiCheck.Error == nil && strings.TrimSpace(efiCheck.Stdout) == "uefi" {
			needUEFI = true
		}
	}

	initType := strings.ToLower(params.InitType)
	isWindows := initType == "windows"

	if isWindows {
		if err := importVMWindowsDefine(params, destDiskPath, format, ramMB, memoryMeta, srcDiskPath, needUEFI); err != nil {
			return nil, err
		}
	} else {
		if err := importVMLinuxDefine(params, destDiskPath, format, ramMB, memoryMeta, srcDiskPath, needUEFI, normalizedBootType, initType); err != nil {
			return nil, err
		}
	}

	// 检查取消
	if err := service.CheckCanceled(ctx, params.Name, destDiskPath); err != nil {
		return nil, err
	}

	progressFn(50, "虚拟机创建成功...")

	// 开机自启
	if params.Autostart {
		utils.ExecCommand("virsh", "autostart", params.Name)
	}

	// 修复重启变关机
	service.FixOnReboot(params.Name)

	// 获取 VM 的 MAC 地址，清理宿主机侧旧的 DHCP 租约
	// 防止导入的磁盘中 machine-id 不同导致同一 MAC 出现多条租约
	cleanImportDHCPLeases(params.Name, params.Network)

	// 根据初始化类型执行初始化（仅当启动了虚拟机时）
	ip := ""
	if params.StartAfterImport {
		ip = importVMInitByType(ctx, params, initType, destDiskPath, progressFn)
	}

	progressFn(100, "导入完成")

	return &ImportVMResult{
		VMName:   params.Name,
		DiskPath: destDiskPath,
		IP:       ip,
	}, nil
}

// importVMCopyDisk handles disk copy/move for ImportVM
func importVMCopyDisk(ctx context.Context, srcDiskPath, destDiskPath, format string, copyDisk bool, progressFn func(int, string)) error {
	if copyDisk {
		// 保留原文件，复制到 CloneDir
		progressFn(12, fmt.Sprintf("检测到 %s 格式，正在复制磁盘文件到虚拟机目录（保留原文件）...", format))
		// 复制磁盘文件属于大 IO 操作，不设置自动超时
		cpResult := utils.ExecCommandNoTimeout("cp", "--sparse=always", srcDiskPath, destDiskPath)
		if cpResult.Error != nil {
			return fmt.Errorf("复制磁盘文件失败: %s", cpResult.Stderr)
		}
		progressFn(20, "磁盘文件复制完成")
	} else {
		// 不保留原文件，先复制到 CloneDir（define 成功后再删除源文件，避免 define 失败时源数据丢失）
		progressFn(12, fmt.Sprintf("检测到 %s 格式，正在复制磁盘文件到虚拟机目录（不保留原文件）...", format))
		// 复制磁盘文件属于大 IO 操作，不设置自动超时
		cpResult := utils.ExecCommandNoTimeout("cp", "--sparse=always", srcDiskPath, destDiskPath)
		if cpResult.Error != nil {
			return fmt.Errorf("复制磁盘文件失败: %s", cpResult.Stderr)
		}
		progressFn(20, "磁盘文件复制完成")
	}
	return nil
}

// importVMInitByType runs the init-type-specific logic after VM creation
func importVMInitByType(_ context.Context, params *ImportVMParams, _ string, _ string, _ func(int, string)) string {
	// Linux 已在 importVMLinuxDefine 里完成离线初始化，无需 SSH
	// 尝试获取 IP（短时等待）
	time.Sleep(5 * time.Second)
	return ip_resolver.GetVMIP(params.Name, true)
}

// importVMPostDefine handles post-define steps shared by Windows and Linux import paths
func importVMPostDefine(vmName, srcDiskPath, destDiskPath string, copyDisk bool, memoryMeta *vm_memory.VMMemoryMetadata, remark string, freeze, startAfterImport bool, owner string, switchID, securityGroupID uint, allowedIPv4, allowedIPv6 string) error {
	if !copyDisk {
		_ = os.Remove(srcDiskPath)
	}
	if memoryMeta != nil {
		if err := vm_memory.WriteVMMemoryMetadata(vmName, memoryMeta); err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
	}
	if err := service.SetVMRemark(vmName, remark); err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}

	if err := service.SetVMFreeze(vmName, freeze); err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	if err := service.PrepareVMPortSecurityBinding(owner, vmName, switchID, securityGroupID, allowedIPv4, allowedIPv6); err != nil {
		return fmt.Errorf("启动前准备端口安全绑定失败: %w", err)
	}

	if startAfterImport {
		if err := service.StartVM(vmName); err != nil {
			return err
		}
	}
	return nil
}

// cleanImportDHCPLeases 清理导入VM对应MAC地址的宿主机侧旧DHCP租约
// 避免磁盘中旧的 machine-id 导致同一 MAC 出现多条不同 client-id 的租约
func cleanImportDHCPLeases(vmName, network string) {
	// 获取 VM 的 MAC 地址
	mac := ip_resolver.GetFirstVMMAC(vmName)
	if mac == "" {
		return
	}

	service.CleanOVSDHCPLease(mac, "")
	service.ReloadOVSDNSMasq()

	logger.App.Info("已清理旧 OVS DHCP 租约", "mac", mac)
}
