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

// ImportDiskByPath 管理员通过绝对路径导入磁盘创建虚拟机（自动转换非qcow2格式）
func ImportDiskByPath(ctx context.Context, params *ImportDiskByPathParams, progressFn func(int, string)) (*ImportVMResult, error) {
	if err := service.ValidateVMName(params.Name); err != nil {
		return nil, err
	}

	// 默认值设置
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
	if params.Hostname == "" {
		params.Hostname = params.Name
	}
	if err := service.ValidateVMCPULimitPercent(params.CPULimitPercent); err != nil {
		return nil, err
	}
	params.CPUTopologyMode = service.NormalizeVMCPUTopologyMode(params.CPUTopologyMode)

	// 检查虚拟机是否已存在
	checkVM := utils.ExecCommand("virsh", "dominfo", params.Name)
	if checkVM.ExitCode == 0 {
		return nil, fmt.Errorf("虚拟机 '%s' 已存在", params.Name)
	}

	// 解析主磁盘源路径（支持绝对路径和存储模式）
	var mainDiskSrc string
	if params.DiskSourceType == "storage" || (params.DiskPath == "" && params.DiskFile != "") {
		if params.DiskFile == "" {
			return nil, fmt.Errorf("存储模式下磁盘文件名为必填")
		}
		if params.Username == "" {
			return nil, fmt.Errorf("存储模式下需要提供用户名")
		}
		mainDiskSrc = filepath.Join(service.GetUserDiskDir(params.Username), params.DiskFile)
	} else {
		mainDiskSrc = params.DiskPath
	}

	if mainDiskSrc == "" {
		return nil, fmt.Errorf("磁盘路径不能为空")
	}
	if !filepath.IsAbs(mainDiskSrc) {
		return nil, fmt.Errorf("磁盘路径必须是绝对路径: %s", mainDiskSrc)
	}
	if !utils.FileExists(mainDiskSrc) {
		return nil, fmt.Errorf("磁盘文件不存在: %s", mainDiskSrc)
	}

	// 安全检查：拒绝将模板目录中的文件作为 VM 系统盘导入
	if config.GlobalConfig != nil && config.GlobalConfig.TemplateDir != "" {
		templateDir := filepath.Clean(config.GlobalConfig.TemplateDir)
		cleanedSrc := filepath.Clean(mainDiskSrc)
		if cleanedSrc == templateDir || strings.HasPrefix(cleanedSrc, templateDir+string(filepath.Separator)) {
			logger.App.Error("拒绝将模板文件作为 VM 系统盘导入", "disk", mainDiskSrc, "template_dir", templateDir)
			return nil, fmt.Errorf("不能直接导入模板目录中的磁盘文件，请使用模板克隆功能创建虚拟机")
		}
	}

	if err := service.EnsureOVSNetworkReady(); err != nil {
		return nil, err
	}

	// 解析存储位置
	targetDir, resolvedPoolID, err := service.ResolveVMStorageDir(params.StoragePoolID, true)
	if err != nil {
		return nil, err
	}
	params.StoragePoolID = resolvedPoolID

	progressFn(10, "检测磁盘格式...")

	// 检测源磁盘格式
	srcFormat := "qcow2"
	infoResult := utils.ExecShell(fmt.Sprintf("qemu-img info -U --output=json %s 2>/dev/null", utils.ShellSingleQuote(mainDiskSrc)))
	if infoResult.Error == nil {
		detected := service.ParseQemuInfoStr(infoResult.Stdout, "format")
		if detected != "" {
			srcFormat = detected
		}
	}

	// 目标磁盘路径（始终使用 qcow2 格式）
	destDiskPath := filepath.Join(targetDir, fmt.Sprintf("%s.qcow2", params.Name))

	// 检查取消
	select {
	case <-ctx.Done():
		return nil, taskqueue.ErrTaskCanceled
	default:
	}

	needsConversion := srcFormat != "qcow2"

	if needsConversion {
		// 非 qcow2 格式，使用 qemu-img convert 转换（源文件在 define 成功后删除）
		progressFn(12, fmt.Sprintf("检测到 %s 格式，正在转换为 qcow2（此过程可能需要较长时间）...", srcFormat))
		convertCmd := fmt.Sprintf("qemu-img convert -f '%s' -O qcow2 '%s' '%s'",
			srcFormat, mainDiskSrc, destDiskPath)
		convertResult := utils.ExecCommandLongRunning("bash", "-c", convertCmd)
		if convertResult.Error != nil {
			return nil, fmt.Errorf("磁盘格式转换失败: %s", convertResult.Stderr)
		}
		progressFn(20, "磁盘格式转换完成")
	} else {
		// 已经是 qcow2 格式，按是否保留原磁盘处理
		if params.CopyDisk {
			progressFn(12, "检测到 qcow2 格式，正在复制磁盘文件到目标存储位置（保留原文件）...")
			cpResult := utils.ExecCommandLongRunning("cp", "--sparse=always", mainDiskSrc, destDiskPath)
			if cpResult.Error != nil {
				return nil, fmt.Errorf("复制磁盘文件失败: %s", cpResult.Stderr)
			}
			progressFn(20, "磁盘文件复制完成")
		} else {
			// 不保留原文件，先复制（define 成功后再删除源文件，避免 define 失败时源数据丢失）
			progressFn(12, "检测到 qcow2 格式，正在复制磁盘文件到目标存储位置（不保留原文件）...")
			cpResult := utils.ExecCommandLongRunning("cp", "--sparse=always", mainDiskSrc, destDiskPath)
			if cpResult.Error != nil {
				return nil, fmt.Errorf("复制磁盘文件失败: %s", cpResult.Stderr)
			}
			progressFn(20, "磁盘文件复制完成")
		}
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
		efiCheck := utils.ExecShell(fmt.Sprintf(
			"virt-filesystems -a '%s' --filesystems --long 2>/dev/null | head -5 | grep -q 'vfat' && echo 'uefi'",
			destDiskPath))
		if efiCheck.Error == nil && strings.TrimSpace(efiCheck.Stdout) == "uefi" {
			needUEFI = true
		}
	}

	initType := strings.ToLower(params.InitType)
	isWindows := initType == "windows"
	format := "qcow2" // 目标格式始终是 qcow2

	if isWindows {
		if err := importDiskByPathWindowsDefine(params, destDiskPath, format, ramMB, memoryMeta, mainDiskSrc); err != nil {
			return nil, err
		}
	} else {
		if err := importDiskByPathLinuxDefine(params, destDiskPath, format, ramMB, memoryMeta, mainDiskSrc, needUEFI, normalizedBootType, initType); err != nil {
			return nil, err
		}
	}

	// 检查取消
	if err := service.CheckCanceled(ctx, params.Name, destDiskPath); err != nil {
		return nil, err
	}

	progressFn(50, "虚拟机创建成功...")

	if params.Autostart {
		utils.ExecCommand("virsh", "autostart", params.Name)
	}

	service.FixOnReboot(params.Name)

	// 获取网络名称用于清理DHCP租约
	network := config.GlobalConfig.DefaultNetwork
	cleanImportDHCPLeases(params.Name, network)

	// Linux 已在 importDiskByPathLinuxDefine 里完成离线初始化，无需 SSH
	// 尝试获取 IP（可选，短时间等待）
	ip := ""
	if params.StartAfterImport {
		time.Sleep(5 * time.Second)
		ip = ip_resolver.GetVMIP(params.Name, true)
	}

	// 处理额外导入磁盘：逐个挂载到已创建的虚拟机
	if len(params.ExtraImportDisks) > 0 {
		progressFn(90, fmt.Sprintf("正在导入 %d 块额外磁盘...", len(params.ExtraImportDisks)))
		for i, extra := range params.ExtraImportDisks {
			select {
			case <-ctx.Done():
				return nil, taskqueue.ErrTaskCanceled
			default:
			}
			subProgressFn := func(p int, msg string) {
				progressFn(90+p/(len(params.ExtraImportDisks)*10)*(len(params.ExtraImportDisks)-i), fmt.Sprintf("[额外磁盘 %d/%d] %s", i+1, len(params.ExtraImportDisks), msg))
			}
			bus := extra.Bus
			if bus == "" {
				bus = "virtio"
			}
			if _, err := importSingleDiskToVM(ctx, params.Name, &ExtraImportDiskEntry{
				DiskPath:       extra.DiskPath,
				DiskFile:       extra.DiskFile,
				DiskSourceType: extra.DiskSourceType,
				StoragePoolID:  extra.StoragePoolID,
				CopyDisk:       extra.CopyDisk,
				Bus:            bus,
			}, params.Username, subProgressFn); err != nil {
				progressFn(95, fmt.Sprintf("额外磁盘 %d 导入失败: %s", i+1, err.Error()))
				continue
			}
		}
	}

	progressFn(100, "导入完成")

	return &ImportVMResult{
		VMName:   params.Name,
		DiskPath: destDiskPath,
		IP:       ip,
	}, nil
}

// resolveImportDiskSource 解析导入磁盘的源路径（支持绝对路径和存储模式）
func resolveImportDiskSource(entry *ExtraImportDiskEntry, username string) (string, error) {
	if entry.DiskSourceType == "path" || (entry.DiskSourceType == "" && entry.DiskPath != "") {
		if !filepath.IsAbs(entry.DiskPath) {
			return "", fmt.Errorf("磁盘路径必须是绝对路径: %s", entry.DiskPath)
		}
		return entry.DiskPath, nil
	}
	// storage 模式
	if entry.DiskFile == "" {
		return "", fmt.Errorf("存储模式下磁盘文件名为必填")
	}
	if username == "" {
		return "", fmt.Errorf("存储模式下需要提供用户名")
	}
	srcDiskPath := filepath.Join(service.GetUserDiskDir(username), entry.DiskFile)
	return srcDiskPath, nil
}

// importSingleDiskToVM 将单块磁盘导入并挂载到已有虚拟机（供 ImportDiskByPath 和 ImportDiskForExistingVM 复用）
func importSingleDiskToVM(ctx context.Context, vmName string, entry *ExtraImportDiskEntry, username string, progressFn func(int, string)) (string, error) {
	// 解析源路径
	srcDiskPath, err := resolveImportDiskSource(entry, username)
	if err != nil {
		return "", err
	}

	// 检查源文件
	if !utils.FileExists(srcDiskPath) {
		return "", fmt.Errorf("磁盘文件不存在: %s", srcDiskPath)
	}

	// 解析目标存储位置
	targetDir := config.GlobalConfig.CloneDir
	if entry.StoragePoolID != "" {
		resolvedDir, _, err := service.ResolveVMStorageDir(entry.StoragePoolID, true)
		if err != nil {
			return "", err
		}
		targetDir = resolvedDir
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 检测格式
	progressFn(5, "检测磁盘格式...")
	srcFormat := "qcow2"
	infoResult := utils.ExecShell(fmt.Sprintf("qemu-img info -U --output=json %s 2>/dev/null", utils.ShellSingleQuote(srcDiskPath)))
	if infoResult.Error == nil {
		detected := service.ParseQemuInfoStr(infoResult.Stdout, "format")
		if detected != "" {
			srcFormat = detected
		}
	}

	// 查找可用设备名
	existingDisks, _ := service.ListDisks(vmName)
	devPrefix := service.GetDevPrefix(entry.Bus)
	if devPrefix == "" {
		devPrefix = "vd"
	}
	usedDevs := make(map[string]bool)
	for _, d := range existingDisks {
		usedDevs[d.Device] = true
	}
	nextDev := ""
	for _, letter := range "bcdefghijklmnop" {
		dev := devPrefix + string(letter)
		if !usedDevs[dev] {
			nextDev = dev
			break
		}
	}
	if nextDev == "" {
		return "", fmt.Errorf("没有可用的设备名")
	}

	// 目标路径
	ts := time.Now().Format("20060102150405")
	destDiskPath := filepath.Join(targetDir, fmt.Sprintf("%s-%s-%s.qcow2", vmName, nextDev, ts))

	// 复制/转换/移动
	select {
	case <-ctx.Done():
		return "", taskqueue.ErrTaskCanceled
	default:
	}

	needsConversion := srcFormat != "qcow2"
	if needsConversion {
		progressFn(10, fmt.Sprintf("检测到 %s 格式，正在转换为 qcow2...", srcFormat))
		convertCmd := fmt.Sprintf("qemu-img convert -f '%s' -O qcow2 '%s' '%s'", srcFormat, srcDiskPath, destDiskPath)
		convertResult := utils.ExecCommandLongRunning("bash", "-c", convertCmd)
		if convertResult.Error != nil {
			return "", fmt.Errorf("磁盘格式转换失败: %s", convertResult.Stderr)
		}
	} else {
		if entry.CopyDisk {
			progressFn(10, "正在复制磁盘文件（保留原文件）...")
			cpResult := utils.ExecCommandLongRunning("cp", "--sparse=always", srcDiskPath, destDiskPath)
			if cpResult.Error != nil {
				return "", fmt.Errorf("复制磁盘文件失败: %s", cpResult.Stderr)
			}
		} else {
			// 不保留原文件，先复制（挂载成功后再删除源文件，避免挂载失败时源数据丢失）
			progressFn(10, "正在复制磁盘文件（不保留原文件）...")
			cpResult := utils.ExecCommandLongRunning("cp", "--sparse=always", srcDiskPath, destDiskPath)
			if cpResult.Error != nil {
				return "", fmt.Errorf("复制磁盘文件失败: %s", cpResult.Stderr)
			}
		}
	}

	if err := utils.ChownLibvirtQEMU(destDiskPath); err != nil {
		os.Remove(destDiskPath)
		return "", fmt.Errorf("设置磁盘权限失败: %w", err)
	}

	progressFn(80, "挂载磁盘到虚拟机...")
	if _, attachErr := service.AttachExistingDisk(vmName, destDiskPath, entry.Bus); attachErr != nil {
		_ = os.Remove(destDiskPath)
		return "", fmt.Errorf("挂载磁盘失败: %w", attachErr)
	}

	// 移动模式下挂载成功，删除源磁盘文件
	if !entry.CopyDisk {
		_ = os.Remove(srcDiskPath)
	}
	progressFn(100, "磁盘导入完成")
	return nextDev, nil
}

// ImportDiskForExistingVM 为已有虚拟机通过绝对路径导入磁盘
func ImportDiskForExistingVM(ctx context.Context, params *ImportDiskForExistingVMParams, progressFn func(int, string)) (string, error) {
	if params.Bus == "" {
		params.Bus = "virtio"
	}
	return importSingleDiskToVM(ctx, params.VMName, &ExtraImportDiskEntry{
		DiskPath:       params.DiskPath,
		DiskFile:       params.DiskFile,
		DiskSourceType: params.DiskSourceType,
		StoragePoolID:  params.StoragePoolID,
		CopyDisk:       params.CopyDisk,
		Bus:            params.Bus,
	}, params.Username, progressFn)
}
