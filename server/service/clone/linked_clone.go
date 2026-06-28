package clone

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/arch"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"

	"github.com/digitalocean/go-libvirt"
)

// LinkedCloneParams 原生链式克隆参数
type LinkedCloneParams struct {
	Name                string                  `json:"name"`
	Remark              string                  `json:"remark,omitempty"`
	Template            string                  `json:"template"`
	TemplateType        string                  `json:"template_type,omitempty"`
	CloneMode           string                  `json:"clone_mode,omitempty"` // 克隆模式: linked（链式克隆，默认）/ full（完整克隆）
	VCPU                int                     `json:"vcpu"`
	MaxVCPU             int                     `json:"max_vcpu,omitempty"` // CPU 热添加上限
	RAM                 int                     `json:"ram"`
	DiskSize            int                     `json:"disk_size,omitempty"`
	Network             string                  `json:"network,omitempty"`
	Autostart           bool                    `json:"autostart,omitempty"`
	Freeze              bool                    `json:"freeze,omitempty"`
	APIC                *bool                   `json:"apic,omitempty"`
	PAE                 *bool                   `json:"pae,omitempty"`
	RTCOffset           string                  `json:"rtc_offset,omitempty"`
	RTCStartDate        string                  `json:"rtc_startdate,omitempty"`
	GuestAgent          *VMGuestAgentConfig     `json:"guest_agent,omitempty"`
	SMBIOS1             *VMSMBIOS1Config        `json:"smbios1,omitempty"`
	BootType            string                  `json:"boot_type,omitempty"`
	DiskBus             string                  `json:"disk_bus,omitempty"`
	VideoModel          string                  `json:"video_model,omitempty"`
	CPUTopologyMode     string                  `json:"cpu_topology_mode,omitempty"`
	CPULimitPercent     int                     `json:"cpu_limit_percent,omitempty"`
	CPUAffinity         string                  `json:"cpu_affinity,omitempty"` // CPU 亲和性，如 "0,2,4"
	FirstBootRebootMode string                  `json:"first_boot_reboot_mode,omitempty"`
	MemoryDynamic       *VMMemoryDynamicRequest `json:"memory_dynamic,omitempty"`
	SwitchID            uint                    `json:"switch_id,omitempty"`
	SecurityGroupID     uint                    `json:"security_group_id,omitempty"`
	ExtraNics           []AddVMInterfaceRequest `json:"extra_nics,omitempty"`
	StoragePoolID       string                  `json:"storage_pool_id,omitempty"`
	ExtraDisks          []ExtraDiskParam        `json:"extra_disks,omitempty"`
	NicModel            string                  `json:"nic_model,omitempty"`
	SystemDiskIOPS      *DiskIOPSTune           `json:"system_disk_iops,omitempty"` // 系统盘 IOPS 限制
	IsAdmin             bool                    `json:"is_admin,omitempty"`
	PCIERootPorts       int                     `json:"pcie_root_ports,omitempty"` // q35 预留 pcie-root-port 数量
}

// LinkedCloneResult 原生链式克隆结果
type LinkedCloneResult struct {
	VMName   string `json:"vm_name"`
	DiskPath string `json:"disk_path"`
	Template string `json:"template"`
}

// ParseLinkedCloneParams 从 JSON 解析原生链式克隆参数
func ParseLinkedCloneParams(jsonStr string) (*LinkedCloneParams, error) {
	var params LinkedCloneParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	return &params, nil
}

// LinkedCloneVM 原生链式克隆虚拟机，不执行任何来宾初始化。
func LinkedCloneVM(ctx context.Context, params *LinkedCloneParams, progressFn func(int, string)) (*LinkedCloneResult, error) {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}
	if err := D.ValidateVMName(params.Name); err != nil {
		return nil, err
	}

	templateDir := config.GlobalConfig.TemplateDir
	cloneDir, resolvedStoragePoolID, err := D.ResolveVMStorageDir(params.StoragePoolID, params.IsAdmin)
	if err != nil {
		return nil, err
	}
	params.StoragePoolID = resolvedStoragePoolID

	if params.Network == "" {
		params.Network = config.GlobalConfig.DefaultNetwork
	}
	if !params.IsAdmin {
		params.CPULimitPercent = D.VMCPULimitUnlimited
	}
	if err := D.ValidateVMCPULimitPercent(params.CPULimitPercent); err != nil {
		return nil, err
	}
	params.NicModel = D.NormalizeVMNicModel(params.NicModel)
	params.DiskBus = D.NormalizeVMDiskBus(params.DiskBus)
	params.BootType = strings.TrimSpace(params.BootType)

	templatePath := filepath.Join(templateDir, params.Template+".qcow2")
	if !utils.FileExists(templatePath) {
		return nil, fmt.Errorf("模板不存在: %s", params.Template)
	}

	// 检查虚拟机是否已存在
	if _, _, _, _, err := libvirt_rpc.GetDomainInfoRPC(params.Name); err == nil {
		return nil, fmt.Errorf("虚拟机 '%s' 已存在", params.Name)
	}

	meta := D.GetTemplateMeta(params.Template)
	if params.DiskBus == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.DiskBus) != "" {
		params.DiskBus = D.NormalizeVMDiskBus(meta.DefaultConfig.DiskBus)
	}
	if strings.TrimSpace(params.VideoModel) == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.VideoModel) != "" {
		params.VideoModel = strings.TrimSpace(meta.DefaultConfig.VideoModel)
	}
	if strings.TrimSpace(params.CPUTopologyMode) == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.CPUTopologyMode) != "" {
		params.CPUTopologyMode = strings.TrimSpace(meta.DefaultConfig.CPUTopologyMode)
	}
	params.CPUTopologyMode = D.NormalizeVMCPUTopologyMode(params.CPUTopologyMode)
	if strings.TrimSpace(params.FirstBootRebootMode) == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.FirstBootRebootMode) != "" {
		params.FirstBootRebootMode = strings.TrimSpace(meta.DefaultConfig.FirstBootRebootMode)
	}
	params.FirstBootRebootMode = D.NormalizeVMFirstBootRebootMode(params.FirstBootRebootMode)
	templateType := strings.ToLower(strings.TrimSpace(params.TemplateType))
	if templateType == "" {
		templateType = strings.ToLower(strings.TrimSpace(meta.Type))
	}
	if templateType == "" {
		templateType = "linux"
	}
	params.TemplateType = templateType
	if params.DiskBus == "" {
		params.DiskBus = "virtio"
	}

	resolvedDiskSize, err := D.ResolveCloneDiskSizeGB(params.Template, params.DiskSize)
	if err != nil {
		return nil, err
	}
	params.DiskSize = resolvedDiskSize

	bootType := params.BootType
	if bootType == "" {
		bootType = meta.BootType
	}
	bootType, _ = D.ResolveTemplateBootType(templatePath, templateType, bootType, true, D.DetectTemplateBootType)
	if bootType == "" {
		bootType = "bios"
	}
	params.BootType = bootType

	cloneDisk := filepath.Join(cloneDir, params.Name+".qcow2")

	// 预检查：目录可写性和存储空间
	if err := D.CheckDirWritable(filepath.Dir(cloneDisk)); err != nil {
		return nil, fmt.Errorf("克隆磁盘存储目录不可用: %w", err)
	}
	// 链式克隆空间需求较小（仅差异数据），预留2GB
	if err := D.CheckStorageSpace(filepath.Dir(cloneDisk), 2048); err != nil {
		return nil, err
	}

	if params.CloneMode == "full" {
		progressFn(10, "创建原生完整克隆磁盘（脱离链式条件）...")
		var convertCmd string
		if params.DiskSize > 0 {
			convertCmd = fmt.Sprintf("qemu-img convert -f qcow2 -O qcow2 %s %s && qemu-img resize %s %dG",
				utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk), utils.ShellSingleQuote(cloneDisk), params.DiskSize)
		} else {
			convertCmd = fmt.Sprintf("qemu-img convert -f qcow2 -O qcow2 %s %s",
				utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk))
		}
		result := utils.ExecShellWithTimeout(convertCmd, 2*time.Hour)
		if result.Error != nil {
			return nil, fmt.Errorf("创建完整克隆磁盘失败: %s", result.Stderr)
		}
	} else {
		progressFn(10, "创建原生链式克隆磁盘...")
		createCmd := fmt.Sprintf("qemu-img create -f qcow2 -F qcow2 -b %s %s", utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk))
		if params.DiskSize > 0 {
			createCmd = fmt.Sprintf("qemu-img create -f qcow2 -F qcow2 -b %s %s %dG", utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk), params.DiskSize)
		}
		result := utils.ExecShell(createCmd)
		if result.Error != nil {
			return nil, fmt.Errorf("创建链式克隆磁盘失败: %s", result.Stderr)
		}
	}

	if err := CheckCanceled(ctx, "", cloneDisk); err != nil {
		return nil, err
	}

	progressFn(30, "准备虚拟机网络与资源配置...")
	if err := D.EnsureOVSNetworkReady(); err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}

	memoryMeta, ramMB, _, err := memory.BuildVMMemoryMetadataForCreate(params.RAM, params.MemoryDynamic)
	if err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}

	progressFn(55, "生成虚拟机定义...")
	vcpuArg := fmt.Sprintf("--vcpus %d", params.VCPU)
	if params.MaxVCPU > params.VCPU {
		vcpuArg = fmt.Sprintf("--vcpus %d,maxvcpus=%d", params.VCPU, params.MaxVCPU)
	}
	cmdParts := []string{
		"virt-install",
		fmt.Sprintf("--name %s", utils.ShellSingleQuote(params.Name)),
		fmt.Sprintf("--ram %d", ramMB),
		vcpuArg,
		fmt.Sprintf("--machine %s", arch.GetProfile(arch.DetectHostArch()).DefaultMachineType()),
		fmt.Sprintf("--disk %s,format=qcow2,bus=%s,discard=unmap,detect_zeroes=unmap", utils.ShellSingleQuote(cloneDisk), params.DiskBus),
		"--osinfo detect=on,require=off",
	}
	// 仅在有主网口交换机配置时才添加网络接口，否则显式禁用
	if params.SwitchID != 0 {
		cmdParts = append(cmdParts, D.BuildOVSVirtInstallNetworkArg(params.NicModel))
	} else {
		cmdParts = append(cmdParts, "--network none")
	}
	cmdParts = append(cmdParts,
		"--graphics vnc,listen=0.0.0.0",
		"--video virtio",
		"--import",
		"--cpu host-passthrough",
	)
	switch bootType {
	case "uefi":
		cmdParts = append(cmdParts, "--boot uefi")
	case "uefi-secure":
		cmdParts = append(cmdParts, "--boot uefi,firmware.feature0.name=secure-boot,firmware.feature0.enabled=yes")
	}

	cmdParts = append(cmdParts, "--print-xml")

	installCmd := strings.Join(cmdParts, " ")
	installResult := utils.ExecCommandLongRunning("bash", "-c", installCmd)
	if installResult.Error != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, fmt.Errorf("生成虚拟机 XML 失败: %s", installResult.Stderr)
	}

	xmlOutput := installResult.Stdout
	if idx := strings.Index(xmlOutput, "</domain>"); idx != -1 {
		xmlOutput = xmlOutput[:idx+len("</domain>")]
	}
	if idx := strings.Index(xmlOutput, "<domain"); idx > 0 {
		xmlOutput = xmlOutput[idx:]
	}

	enableFPR := templateType != "windows" && templateType != "other"
	vmXML := InjectMemballoonConfig(xmlOutput, enableFPR)

	// 注入 pcie-root-port 控制器（q35 机型热插拔预留，默认 6 个）
	pciePortCount := params.PCIERootPorts
	if pciePortCount <= 0 {
		pciePortCount = 6
	}
	vmXML = D.InjectPCIERootPorts(vmXML, pciePortCount)

	if memoryMeta != nil {
		vmXML, err = memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, enableFPR)
		if err != nil {
			cleanupLinkedCloneArtifacts("", cloneDisk)
			return nil, err
		}
	}
	vmXML, err = D.ApplyRTCConfigToDomainXML(vmXML, params.RTCOffset, params.RTCStartDate, templateType)
	if err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}
	vmXML, err = vm_xml.ApplyVMGuestAgentConfigToDomainXML(vmXML, params.GuestAgent)
	if err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}
	vmXML, err = vm_xml.ApplySMBIOS1ConfigToDomainXML(vmXML, params.SMBIOS1, true)
	if err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}
	vmXML, err = D.ApplyVMAPICToDomainXML(vmXML, params.APIC)
	if err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}
	vmXML, err = vm_xml.ApplyVMPAEToDomainXML(vmXML, params.PAE)
	if err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, templateType)
	if templateType == "windows" {
		vmXML = vm_xml.ApplyWindowsGuestOptimizationsToDomainXML(vmXML)
	}
	topoVCPU := D.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = D.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, templateType, topoVCPU)
	vmXML = D.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		affinityCores, affErr := D.ParseCPUAffinity(params.CPUAffinity)
		if affErr != nil {
			cleanupLinkedCloneArtifacts("", cloneDisk)
			return nil, fmt.Errorf("CPU 亲和性格式错误: %w", affErr)
		}
		if len(affinityCores) > 0 {
			if affErr := D.ValidateCPUAffinity(affinityCores); affErr != nil {
				cleanupLinkedCloneArtifacts("", cloneDisk)
				return nil, affErr
			}
		}
		vmXML = D.ApplyCPUAffinityToDomainXML(vmXML, topoVCPU, affinityCores)
	}
	normalizedBootType := vm_xml.NormalizeVMBootType(bootType)
	if normalizedBootType != "" {
		vmXML, err = vm_xml.ApplyVMBootTypeToDomainXML(params.Name, vmXML, normalizedBootType)
		if err != nil {
			cleanupLinkedCloneArtifacts("", cloneDisk)
			return nil, err
		}
	}
	firstBootColdReboot := D.ShouldUseWindowsFirstBootColdReboot(params.FirstBootRebootMode, templateType)
	if firstBootColdReboot {
		vmXML = D.ApplyFirstBootRebootModeToDomainXML(vmXML, params.FirstBootRebootMode)
	}
	vmXML, err = D.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}
	if err := vm_xml.EnsureVMUEFINVRAMFile(params.Name, vmXML, normalizedBootType); err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, err
	}

	// 定义虚拟机（直接通过 RPC，无需临时文件）
	if _, err := libvirt_rpc.DefineDomainXMLRPC(vmXML); err != nil {
		cleanupLinkedCloneArtifacts("", cloneDisk)
		return nil, fmt.Errorf("定义虚拟机失败: %w", err)
	}

	if memoryMeta != nil {
		if err := memory.WriteVMMemoryMetadata(params.Name, memoryMeta); err != nil {
			cleanupLinkedCloneArtifacts(params.Name, cloneDisk)
			return nil, err
		}
	}
	if err := D.WriteVMTemplateSource(params.Name, params.Template, params.CloneMode); err != nil {
		cleanupLinkedCloneArtifacts(params.Name, cloneDisk)
		return nil, err
	}
	if err := D.SetVMRemark(params.Name, params.Remark); err != nil {
		cleanupLinkedCloneArtifacts(params.Name, cloneDisk)
		return nil, err
	}
	if err := D.SetVMFreeze(params.Name, params.Freeze); err != nil {
		cleanupLinkedCloneArtifacts(params.Name, cloneDisk)
		return nil, err
	}

	// 额外磁盘：在启动前冷添加，避免占用 PCIe 热插槽
	if len(params.ExtraDisks) > 0 {
		progressFn(78, "挂载额外磁盘...")
		if err := D.AddExtraDisksForVM(params.Name, params.ExtraDisks, cloneDir, params.DiskBus, params.IsAdmin, func(_ int, msg string) {
			progressFn(78, msg)
		}); err != nil {
			cleanupLinkedCloneArtifacts(params.Name, cloneDisk)
			return nil, err
		}
	}

	progressFn(88, "启动虚拟机...")
	startFn := D.StartVM
	if firstBootColdReboot {
		startFn = D.StartVMPreserveRebootAction
	}
	if err := startFn(params.Name); err != nil {
		cleanupLinkedCloneArtifacts(params.Name, cloneDisk)
		return nil, err
	}
	if firstBootColdReboot {
		if err := D.CompleteWindowsFirstBootColdReboot(ctx, params.Name, progressFn); err != nil {
			return nil, err
		}
	}

	if err := CheckCanceled(ctx, params.Name, cloneDisk); err != nil {
		return nil, err
	}

	if params.Autostart {
		if err := libvirt_rpc.SetDomainAutostartRPC(params.Name, true); err != nil {
			logger.App.Warn("设置虚拟机自动启动失败", "vm", params.Name, "error", err)
		}
	}
	D.FixOnReboot(params.Name)

	progressFn(100, "原生链式克隆完成")
	return &LinkedCloneResult{
		VMName:   params.Name,
		DiskPath: cloneDisk,
		Template: params.Template,
	}, nil
}

func cleanupLinkedCloneArtifacts(vmName, diskPath string) {
	// 如果提供了 VM 名称，尝试清理 libvirt 定义
	if strings.TrimSpace(vmName) != "" {
		// 尝试销毁（如果 VM 正在运行）
		if err := libvirt_rpc.DestroyDomainRPC(vmName); err != nil {
			logger.Libvirt.Warn("销毁虚拟机失败", "vm", vmName, "error", err)
		} else {
			logger.Libvirt.Info("已销毁虚拟机", "vm", vmName)
		}
		// 尝试取消定义（含 NVRAM 和快照元数据）
		if err := libvirt_rpc.UndefineDomainRPC(vmName, libvirt.DomainUndefineNvram|libvirt.DomainUndefineSnapshotsMetadata); err != nil {
			logger.Libvirt.Warn("取消定义虚拟机失败", "vm", vmName, "error", err)
		} else {
			logger.Libvirt.Info("已取消定义虚拟机", "vm", vmName)
		}
	}
	// 删除磁盘文件
	if strings.TrimSpace(diskPath) != "" {
		if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
			logger.App.Warn("删除磁盘文件失败", "path", diskPath, "error", err)
		} else if err == nil {
			logger.App.Info("已删除磁盘文件", "path", diskPath)
		}
	}
}
