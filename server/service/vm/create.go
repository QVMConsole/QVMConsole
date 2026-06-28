package vm

import (
	"encoding/json"
	"fmt"
	"kvm_console/logger"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kvm_console/config"
	"kvm_console/service/arch"
	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// CreateVMParams 普通创建虚拟机参数（不通过模板）
type CreateVMParams struct {
	Name            string                         `json:"name"`
	Remark          string                         `json:"remark,omitempty"`
	VCPU            int                            `json:"vcpu"`
	MaxVCPU         int                            `json:"max_vcpu,omitempty"` // CPU 热添加上限，0 或 <= vcpu 表示不启用热添加
	RAM             int                            `json:"ram"`
	DiskSize        int                            `json:"disk_size"`
	DiskFormat      string                         `json:"disk_format,omitempty"`
	DiskBus         string                         `json:"disk_bus,omitempty"` // 磁盘总线类型: virtio/scsi/sata/ide
	OSVariant       string                         `json:"os_variant,omitempty"`
	ISOPath         string                         `json:"iso_path,omitempty"`
	ISOPaths        []string                       `json:"iso_paths,omitempty"`
	FloppyImage     string                         `json:"floppy_image,omitempty"`
	Network         string                         `json:"network,omitempty"`
	NicModel        string                         `json:"nic_model,omitempty"` // 网卡模型: virtio/e1000e/rtl8139
	Autostart       bool                           `json:"autostart,omitempty"`
	Freeze          bool                           `json:"freeze,omitempty"` // 启动时冻结 CPU
	APIC            *bool                          `json:"apic,omitempty"`   // APIC 开关，默认启用
	PAE             *bool                          `json:"pae,omitempty"`    // PAE 开关，默认启用
	RTCOffset       string                         `json:"rtc_offset,omitempty"`
	RTCStartDate    string                         `json:"rtc_startdate,omitempty"`
	GuestAgent      *vm_xml.VMGuestAgentConfig     `json:"guest_agent,omitempty"`
	SMBIOS1         *vm_xml.VMSMBIOS1Config        `json:"smbios1,omitempty"`
	OSType          string                         `json:"os_type,omitempty"`
	MachineType     string                         `json:"machine_type,omitempty"`
	BootType        string                         `json:"boot_type,omitempty"`
	Watchdog        string                         `json:"watchdog,omitempty"`
	BootOrder       []string                       `json:"boot_order,omitempty"`
	VideoModel      string                         `json:"video_model,omitempty"` // 视频模型: virtio/vga/vmvga/cirrus/ramfb
	SpiceEnabled    *bool                          `json:"spice_enabled,omitempty"` // 是否启用 SPICE 显示协议（nil=回退全局默认）
	CPUTopologyMode string                         `json:"cpu_topology_mode,omitempty"`
	CPULimitPercent int                            `json:"cpu_limit_percent,omitempty"`
	CPUAffinity     string                         `json:"cpu_affinity,omitempty"` // CPU 亲和性，如 "0,2,4"，空字符串表示不设置
	VirtType        string                         `json:"virt_type,omitempty"`    // 虚拟化方案: kvm/qemu，默认 kvm
	Arch            string                         `json:"arch,omitempty"`         // 目标架构: x86_64/aarch64/riscv64
	ExtraDisks      []ExtraDiskParam               `json:"extra_disks,omitempty"`
	MemoryDynamic   *memory.VMMemoryDynamicRequest `json:"memory_dynamic,omitempty"`
	SystemDiskIOPS  *DiskIOPSTune                  `json:"system_disk_iops,omitempty"` // 系统盘 IOPS 限制（仅管理员）
	SwitchID        uint                           `json:"switch_id,omitempty"`
	SecurityGroupID uint                           `json:"security_group_id,omitempty"`
	ExtraNics       []AddVMInterfaceRequest        `json:"extra_nics,omitempty"`
	StoragePoolID   string                         `json:"storage_pool_id,omitempty"`
	HostDevices     []HostDeviceParam              `json:"host_devices,omitempty"` // 硬件直通设备
	IsAdmin         bool                           `json:"is_admin,omitempty"`
	PCIERootPorts   int                            `json:"pcie_root_ports,omitempty"` // q35 机型预留 pcie-root-port 数量，0 表示使用默认 6
}

// ExtraDiskParam is now defined in storage/disk package; alias in disk_compat.go.

// OSVariantInfo 系统变体信息
type OSVariantInfo struct {
	ID       string `json:"id"`       // 变体 ID
	Name     string `json:"name"`     // 显示名称
	Category string `json:"category"` // 分类: Linux/Windows/Other
}

// ListOSVariants 获取可用的系统变体列表
func ListOSVariants() ([]OSVariantInfo, error) {
	result := utils.ExecShell("virt-install --osinfo list 2>/dev/null")
	if result.Error != nil {
		return nil, fmt.Errorf("获取系统变体列表失败: %w", result.Error)
	}

	var variants []OSVariantInfo
	lines := strings.Split(result.Stdout, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 可能有别名，如 "ubuntu24.04, ubuntunoble"
		parts := strings.SplitN(line, ",", 2)
		id := strings.TrimSpace(parts[0])
		name := id
		if len(parts) > 1 {
			name = fmt.Sprintf("%s (%s)", id, strings.TrimSpace(parts[1]))
		}

		// 分类
		category := "Other"
		idLower := strings.ToLower(id)
		if strings.HasPrefix(idLower, "win") {
			category = "Windows"
		} else if strings.HasPrefix(idLower, "ubuntu") ||
			strings.HasPrefix(idLower, "debian") ||
			strings.HasPrefix(idLower, "centos") ||
			strings.HasPrefix(idLower, "fedora") ||
			strings.HasPrefix(idLower, "rhel") ||
			strings.HasPrefix(idLower, "alma") ||
			strings.HasPrefix(idLower, "rocky") ||
			strings.HasPrefix(idLower, "opensuse") ||
			strings.HasPrefix(idLower, "sles") ||
			strings.HasPrefix(idLower, "archlinux") ||
			strings.HasPrefix(idLower, "gentoo") ||
			strings.HasPrefix(idLower, "alpine") ||
			strings.HasPrefix(idLower, "freebsd") ||
			strings.HasPrefix(idLower, "openbsd") ||
			strings.HasPrefix(idLower, "linux") ||
			strings.HasPrefix(idLower, "generic") {
			category = "Linux"
		}

		variants = append(variants, OSVariantInfo{
			ID:       id,
			Name:     name,
			Category: category,
		})
	}

	return variants, nil
}

// ListISOs 列出可用的 ISO 镜像（读取系统设置中的全局 ISO 目录）
func ListISOs() ([]map[string]string, error) {
	items, err := D.GetAllISOs()
	if err != nil {
		return nil, err
	}
	var isos []map[string]string
	for _, item := range items {
		isos = append(isos, map[string]string{
			"path": item.Path,
			"name": item.Name,
			"size": item.Size,
		})
	}
	return isos, nil
}

// CreateVM 普通方式创建虚拟机（不通过模板）
func CreateVM(params *CreateVMParams, progressFn func(int, string)) (string, error) {
	params.ISOPath, params.ISOPaths = D.NormalizeInstallISOSelection(params.ISOPath, params.ISOPaths)
	if err := D.ValidateVMName(params.Name); err != nil {
		return "", err
	}

	// 默认值
	if params.Network == "" {
		params.Network = config.GlobalConfig.DefaultNetwork
	}
	if params.DiskFormat == "" {
		params.DiskFormat = "qcow2"
	}
	if params.DiskSize <= 0 {
		return "", fmt.Errorf("磁盘大小必须大于0GB")
	}
	if params.VirtType == "" {
		params.VirtType = "kvm"
	}
	if params.Arch == "" {
		params.Arch = arch.DetectHostArch()
	}
	profile := arch.GetProfile(params.Arch)
	if params.MachineType == "" {
		params.MachineType = profile.DefaultMachineType()
	} else if !slices.Contains(profile.SupportedMachineTypes(), params.MachineType) {
		params.MachineType = profile.DefaultMachineType()
	}
	if params.BootType == "" {
		params.BootType = profile.DefaultBootType()
	} else if !slices.Contains(profile.SupportedBootTypes(), params.BootType) {
		params.BootType = profile.DefaultBootType()
	}
	if params.NicModel == "" {
		params.NicModel = "virtio"
	}
	if len(params.BootOrder) == 0 {
		params.BootOrder = []string{"hd"}
	}

	if !params.IsAdmin {
		params.CPULimitPercent = D.VMCPULimitUnlimited
	}
	if err := D.ValidateVMCPULimitPercent(params.CPULimitPercent); err != nil {
		return "", err
	}
	params.CPUTopologyMode = D.NormalizeVMCPUTopologyMode(params.CPUTopologyMode)
	cloneDir, resolvedStoragePoolID, err := D.ResolveVMStorageDir(params.StoragePoolID, params.IsAdmin)
	if err != nil {
		return "", err
	}
	params.StoragePoolID = resolvedStoragePoolID

	// 检查虚拟机是否已存在
	checkVM := utils.ExecCommand("virsh", "dominfo", params.Name)
	if checkVM.ExitCode == 0 {
		return "", fmt.Errorf("虚拟机 '%s' 已存在", params.Name)
	}

	progressFn(10, "创建磁盘...")

	// 检查目录存在性和可写性
	if err := D.CheckDirWritable(cloneDir); err != nil {
		return "", fmt.Errorf("磁盘存储目录不可用: %w", err)
	}
	// 检查存储空间（预留额外1GB缓冲）
	if err := D.CheckStorageSpace(cloneDir, int64(params.DiskSize)*1024+1024); err != nil {
		return "", err
	}

	// 创建磁盘
	diskPath := filepath.Join(cloneDir, fmt.Sprintf("%s.%s", params.Name, params.DiskFormat))
	createCmd := fmt.Sprintf("qemu-img create -f %s %s %dG",
		params.DiskFormat, utils.ShellSingleQuote(diskPath), params.DiskSize)
	result := utils.ExecShell(createCmd)
	if result.Error != nil {
		return "", fmt.Errorf("创建磁盘失败: %s", result.Stderr)
	}

	progressFn(30, "生成虚拟机配置...")
	if err := D.EnsureOVSNetworkReady(); err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}

	memoryMeta, ramMB, _, err := memory.BuildVMMemoryMetadataForCreate(params.RAM, params.MemoryDynamic)
	if err != nil {
		return "", err
	}

	// 启动前检查宿主机可用内存，预留系统开销
	if err := CheckHostMemory(ramMB); err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}

	// 构建 virt-install 命令
	var cmdParts []string
	cmdParts = append(cmdParts, "virt-install")
	cmdParts = append(cmdParts, fmt.Sprintf("--name '%s'", params.Name))
	cmdParts = append(cmdParts, fmt.Sprintf("--ram %d", ramMB))
	cmdParts = append(cmdParts, fmt.Sprintf("--vcpus %d", params.VCPU))
	if params.MaxVCPU > params.VCPU {
		cmdParts[len(cmdParts)-1] = fmt.Sprintf("--vcpus %d,maxvcpus=%d", params.VCPU, params.MaxVCPU)
	}

	// 虚拟化方案
	cmdParts = append(cmdParts, fmt.Sprintf("--virt-type %s", params.VirtType))

	// 目标架构
	cmdParts = append(cmdParts, fmt.Sprintf("--arch %s", params.Arch))

	// 机器类型
	cmdParts = append(cmdParts, fmt.Sprintf("--machine %s", params.MachineType))

	// 磁盘总线类型：优先用户指定，否则根据系统类型决定
	diskBus := params.DiskBus
	if diskBus == "" {
		if params.OSType == "windows" {
			diskBus = "sata"
		} else {
			diskBus = "virtio"
		}
	}
	cmdParts = append(cmdParts, fmt.Sprintf(
		"--disk '%s,format=%s,bus=%s,discard=unmap,detect_zeroes=unmap'",
		diskPath, params.DiskFormat, diskBus))

	// OS 变体
	if params.OSVariant != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("--osinfo '%s'", params.OSVariant))
	} else {
		cmdParts = append(cmdParts, "--osinfo detect=on,require=off")
	}

	// 网络：仅在有主网口交换机配置时才添加网络接口，否则显式禁用
	if params.SwitchID != 0 {
		cmdParts = append(cmdParts, D.BuildOVSVirtInstallNetworkArg(params.NicModel))
	} else {
		cmdParts = append(cmdParts, "--network none")
	}

	// 显示设备
	cmdParts = append(cmdParts, "--graphics vnc,listen=0.0.0.0")
	cmdParts = append(cmdParts, "--video virtio")

	// ISO 镜像（如果提供）
	if params.ISOPath != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("--cdrom '%s'", params.ISOPath))
	} else {
		// 没有 ISO 也没有模板，创建空白虚拟机（从磁盘引导）
		cmdParts = append(cmdParts, "--import")
	}

	// 引导类型
	switch params.BootType {
	case "uefi":
		cmdParts = append(cmdParts, "--boot uefi")
	case "uefi-secure":
		// UEFI + 安全引导，需要 Q35 + OVMF + SMM
		cmdParts = append(cmdParts, "--boot uefi,firmware.feature0.name=secure-boot,firmware.feature0.enabled=yes")
	default:
		// BIOS 模式不需要额外参数
	}

	// 启动顺序（有 --cdrom 时不能再加 --boot，否则 virt-install 会生成两组 XML）
	if params.ISOPath == "" && len(params.BootOrder) > 0 {
		bootDevs := strings.Join(params.BootOrder, ",")
		if params.BootType == "bios" {
			cmdParts = append(cmdParts, fmt.Sprintf("--boot %s", bootDevs))
		}
	}

	// Watchdog 监督者
	if params.Watchdog != "" && params.Watchdog != "none" {
		cmdParts = append(cmdParts, fmt.Sprintf("--watchdog %s,action=reset", params.Watchdog))
	}

	// CPU 模式：根据虚拟化方案决定
	if params.VirtType == "qemu" {
		// 软件虚拟化不能使用 host-passthrough
		cpuModel := arch.GetProfile(params.Arch).DefaultCPUModel(params.VirtType)
		if cpuModel != "" {
			cmdParts = append(cmdParts, fmt.Sprintf("--cpu %s", cpuModel))
		}
	} else {
		cmdParts = append(cmdParts, "--cpu host-passthrough")
	}
	// 使用 --print-xml 生成 XML，在定义时直接注入 memballoon 配置
	cmdParts = append(cmdParts, "--print-xml")

	installCmd := strings.Join(cmdParts, " ")

	progressFn(50, "创建虚拟机...")

	installResult := utils.ExecCommandLongRunning("bash", "-c", installCmd)
	if installResult.Error != nil {
		// 生成失败，清理磁盘
		_ = os.Remove(diskPath)
		return "", fmt.Errorf("生成虚拟机 XML 失败: %s", installResult.Stderr)
	}
	// virt-install --print-xml 带 --cdrom 时会输出两个 <domain> XML（安装阶段+后续阶段）
	// 只取第一个 <domain>...</domain>
	xmlOutput := installResult.Stdout
	if idx := strings.Index(xmlOutput, "</domain>"); idx != -1 {
		xmlOutput = xmlOutput[:idx+len("</domain>")]
	}
	// 去掉 <domain> 之前可能的额外输出（如 qemu-img 的 Formatting... 行）
	if idx := strings.Index(xmlOutput, "<domain"); idx > 0 {
		xmlOutput = xmlOutput[idx:]
	}

	// 注入 memballoon 配置（非 Windows 启用 freePageReporting）
	enableFPR := params.OSType != "windows"
	vmXML := D.InjectMemballoonConfig(xmlOutput, enableFPR)

	// 注入 pcie-root-port 控制器（q35 机型热插拔预留，默认 6 个）
	pciePortCount := params.PCIERootPorts
	if pciePortCount <= 0 {
		pciePortCount = 6
	}
	vmXML = InjectPCIERootPorts(vmXML, pciePortCount)

	if memoryMeta != nil {
		vmXML, err = memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, enableFPR)
		if err != nil {
			_ = os.Remove(diskPath)
			return "", err
		}
	}
	vmXML, err = D.ApplyRTCConfigToDomainXML(vmXML, params.RTCOffset, params.RTCStartDate, params.OSType)
	if err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	vmXML, err = vm_xml.ApplyVMGuestAgentConfigToDomainXML(vmXML, params.GuestAgent)
	if err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	vmXML, err = vm_xml.ApplySMBIOS1ConfigToDomainXML(vmXML, params.SMBIOS1, true)
	if err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	vmXML, err = D.ApplyVMAPICToDomainXML(vmXML, params.APIC)
	if err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	vmXML, err = vm_xml.ApplyVMPAEToDomainXML(vmXML, params.PAE)
	if err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, params.OSType)
	if params.OSType == "windows" {
		vmXML = vm_xml.ApplyWindowsGuestOptimizationsToDomainXML(vmXML)
	}
	topoVCPU := D.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = D.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, params.OSType, topoVCPU)
	vmXML = D.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		affinityCores, err := D.ParseCPUAffinity(params.CPUAffinity)
		if err != nil {
			_ = os.Remove(diskPath)
			return "", fmt.Errorf("CPU 亲和性格式错误: %w", err)
		}
		if len(affinityCores) > 0 {
			if err := D.ValidateCPUAffinity(affinityCores); err != nil {
				_ = os.Remove(diskPath)
				return "", err
			}
		}
		vmXML = D.ApplyCPUAffinityToDomainXML(vmXML, topoVCPU, affinityCores)
	}
	normalizedBootType := vm_xml.NormalizeVMBootType(params.BootType)
	if normalizedBootType != "" {
		vmXML, err = vm_xml.ApplyVMBootTypeToDomainXML(params.Name, vmXML, normalizedBootType)
		if err != nil {
			_ = os.Remove(diskPath)
			return "", err
		}
	}
	vmXML, err = D.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	vmXML, err = D.ApplyAdditionalCDROMsToDomainXML(vmXML, params.ISOPaths)
	if err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	if err := vm_xml.EnsureVMUEFINVRAMFile(params.Name, vmXML, normalizedBootType); err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}

	// SPICE graphics（默认本地监听），与 VNC 共存；是否启用由 per-VM 开关决定，回退全局默认
	spiceEnabled := false
	if D.SpiceEnabledByDefault != nil {
		spiceEnabled = D.SpiceEnabledByDefault()
	}
	if params.SpiceEnabled != nil {
		spiceEnabled = *params.SpiceEnabled
	}
	if spiceEnabled {
		if D.InjectSPICEGraphics != nil {
			vmXML = D.InjectSPICEGraphics(vmXML, "", "127.0.0.1")
		}
		if D.EnsureQXLVideo != nil {
			vmXML = D.EnsureQXLVideo(vmXML)
		}
	}

	// 硬件直通设备
	if len(params.HostDevices) > 0 {
		progressFn(55, "配置硬件直通设备...")
		if err := D.EnsureVfioModuleLoaded(); err != nil {
			_ = os.Remove(diskPath)
			return "", fmt.Errorf("加载 vfio-pci 模块失败: %w", err)
		}
		for _, hd := range params.HostDevices {
			if err := D.ValidatePCIPassthrough(hd.PCIAddress); err != nil {
				_ = os.Remove(diskPath)
				return "", fmt.Errorf("设备 %s 直通验证失败: %w", hd.PCIAddress, err)
			}
			if !D.IsDeviceVfioBound(hd.PCIAddress) {
				if err := D.BindPCIDeviceToVfio(hd.PCIAddress); err != nil {
					_ = os.Remove(diskPath)
					return "", fmt.Errorf("绑定设备 %s 到 vfio-pci 失败: %w", hd.PCIAddress, err)
				}
			}
		}
		vmXML, err = D.ApplyHostDevsToDomainXML(vmXML, params.HostDevices)
		if err != nil {
			_ = os.Remove(diskPath)
			return "", fmt.Errorf("应用硬件直通设备失败: %w", err)
		}
	}

	// 写入临时文件并定义虚拟机
	xmlPath := fmt.Sprintf("/tmp/_vm-create-%s.xml", params.Name)
	if err := os.WriteFile(xmlPath, []byte(vmXML), 0644); err != nil {
		_ = os.Remove(diskPath)
		return "", fmt.Errorf("写入虚拟机 XML 失败: %w", err)
	}

	defineResult := utils.ExecCommand("virsh", "define", xmlPath)
	_ = os.Remove(xmlPath)
	if defineResult.Error != nil {
		_ = os.Remove(diskPath)
		return "", fmt.Errorf("定义虚拟机失败: %s", defineResult.Stderr)
	}
	if memoryMeta != nil {
		if err := memory.WriteVMMemoryMetadata(params.Name, memoryMeta); err != nil {
			_ = os.Remove(diskPath)
			return "", err
		}
	}
	if err := SetVMRemark(params.Name, params.Remark); err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	if err := SetVMFreeze(params.Name, params.Freeze); err != nil {
		_ = os.Remove(diskPath)
		return "", err
	}
	if err := StartVM(params.Name); err != nil {
		// 先 undefine VM 定义，再删除磁盘
		utils.ExecCommand("virsh", "undefine", params.Name, "--nvram", "--snapshots-metadata")
		_ = os.Remove(diskPath)
		return "", fmt.Errorf("启动虚拟机失败(已清理资源): %w", err)
	}

	progressFn(70, "配置虚拟机...")

	// 开机自启
	if params.Autostart {
		utils.ExecCommand("virsh", "autostart", params.Name)
	}

	// 修复重启变关机（virt-install 默认 on_reboot=destroy）
	FixOnReboot(params.Name)

	// 额外磁盘
	if len(params.ExtraDisks) > 0 {
		progressFn(85, "挂载额外磁盘...")
		var extraDiskFailures []string
		for i, ed := range params.ExtraDisks {
			format := ed.Format
			if format == "" {
				format = "qcow2"
			}
			bus := ed.Bus
			if bus == "" {
				bus = diskBus // 使用系统盘的总线类型
			}
			diskDir := cloneDir
			if strings.TrimSpace(ed.StoragePoolID) != "" {
				resolvedDir, _, resolveErr := D.ResolveVMStorageDir(ed.StoragePoolID, params.IsAdmin)
				if resolveErr != nil {
					extraDiskFailures = append(extraDiskFailures, fmt.Sprintf("磁盘%d: 解析存储位置失败: %s", i+1, resolveErr.Error()))
					progressFn(85, fmt.Sprintf("解析额外磁盘 %d 存储位置失败: %s", i+1, resolveErr.Error()))
					continue
				}
				diskDir = resolvedDir
			}
			_, err := D.AddDiskWithBusInDir(params.Name, ed.Size, format, bus, diskDir)
			if err != nil {
				extraDiskFailures = append(extraDiskFailures, fmt.Sprintf("磁盘%d: 挂载失败: %s", i+1, err.Error()))
				progressFn(85, fmt.Sprintf("挂载额外磁盘 %d 失败: %s", i+1, err.Error()))
			}
		}
		if len(extraDiskFailures) > 0 {
			logger.App.Warn("虚拟机额外磁盘部分失败", "vm", params.Name, "failures", strings.Join(extraDiskFailures, "; "))
		}
	}

	// 应用 IOPS 限制（仅管理员设置的值 > 0）
	if params.SystemDiskIOPS != nil && (params.SystemDiskIOPS.TotalIopsSec > 0 || params.SystemDiskIOPS.ReadIopsSec > 0 || params.SystemDiskIOPS.WriteIopsSec > 0) {
		sysDev := getFirstDiskDevice(params.Name)
		if sysDev != "" {
			if err := D.SetDiskIOPSTune(params.Name, sysDev, params.SystemDiskIOPS); err != nil {
				progressFn(95, fmt.Sprintf("设置系统盘 IOPS 限制失败: %s", err.Error()))
			}
		}
	}
	for i, ed := range params.ExtraDisks {
		if ed.IOPSTotal > 0 || ed.IOPSRead > 0 || ed.IOPSWrite > 0 {
			dev := getNthDiskDevice(params.Name, i+2) // +2 因为第1个是系统盘，额外磁盘从第2个开始
			if dev != "" {
				if err := D.SetDiskIOPSTune(params.Name, dev, &DiskIOPSTune{
					TotalIopsSec: ed.IOPSTotal,
					ReadIopsSec:  ed.IOPSRead,
					WriteIopsSec: ed.IOPSWrite,
				}); err != nil {
					progressFn(95, fmt.Sprintf("设置额外磁盘 %d IOPS 限制失败: %s", i+1, err.Error()))
				}
			}
		}
	}

	// 挂载软盘镜像（如果用户指定了）
	if strings.TrimSpace(params.FloppyImage) != "" {
		progressFn(95, "挂载软盘镜像...")
		if _, err := os.Stat(params.FloppyImage); err == nil {
			floppyErr := D.ChangeFloppy(params.Name, params.FloppyImage, "", true)
			if floppyErr != nil {
				progressFn(95, fmt.Sprintf("挂载软盘失败: %s", floppyErr.Error()))
			} else {
				progressFn(98, "软盘镜像已挂载")
			}
		} else {
			progressFn(95, fmt.Sprintf("软盘镜像文件不存在: %s", params.FloppyImage))
		}
	}

	progressFn(100, "虚拟机创建完成")

	return diskPath, nil
}

// ParseCreateVMParams 从 JSON 解析普通创建参数
func ParseCreateVMParams(jsonStr string) (*CreateVMParams, error) {
	var params CreateVMParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	return &params, nil
}

// getFirstDiskDevice 获取虚拟机第一个磁盘设备名（系统盘）
func getFirstDiskDevice(vmName string) string {
	return getNthDiskDevice(vmName, 1)
}

// getNthDiskDevice 获取虚拟机第 n 个磁盘设备名（1-based）
func getNthDiskDevice(vmName string, n int) string {
	result := utils.ExecCommand("virsh", "domblklist", vmName)
	if result.Error != nil {
		return ""
	}
	lines := strings.Split(result.Stdout, "\n")
	count := 0
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "Target" || strings.HasPrefix(line, "-") {
			continue
		}
		dev := fields[0]
		path := fields[1]
		if path == "" || path == "-" {
			continue
		}
		count++
		if count == n {
			return dev
		}
	}
	return ""
}

// CheckHostMemory 检查宿主机可用内存是否满足虚拟机创建需求
// requiredMB: 虚拟机请求的内存大小（MB）
// 返回 nil 表示内存充足，否则返回详细错误信息
func CheckHostMemory(requiredMB int) error {
	// 读取 MemAvailable（KB），比 MemFree 更准确，包含了可回收的缓存
	memInfo, err := utils.ReadMemInfo()
	if err != nil {
		return fmt.Errorf("无法获取宿主机内存信息: %w", err)
	}
	availKB, ok := memInfo["MemAvailable"]
	if !ok {
		return fmt.Errorf("无法获取宿主机可用内存信息")
	}
	availMB := int(availKB / 1024)

	// 预留 512MB 系统开销 + 10% 缓冲，确保不会因 QEMU 进程额外开销导致 OOM
	bufferMB := requiredMB/10 + 512
	if bufferMB < 256 {
		bufferMB = 256 // 最小保留 256MB 缓冲
	}
	requiredWithBuffer := requiredMB + bufferMB

	if availMB < requiredWithBuffer {
		return fmt.Errorf("宿主机内存不足: 需要 %dMB（含 %dMB 系统开销预留），可用 %dMB，请释放部分资源或降低虚拟机内存配置后重试",
			requiredWithBuffer, bufferMB, availMB)
	}
	return nil
}
