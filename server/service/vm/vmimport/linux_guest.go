package vmimport

import (
	"fmt"
	"os"
	"strings"

	"kvm_console/config"
	"kvm_console/service"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// importVMLinuxDefine handles Linux/Other VM XML construction and define for ImportVM
func importVMLinuxDefine(params *ImportVMParams, destDiskPath, format string, ramMB int, memoryMeta *vm_memory.VMMemoryMetadata, srcDiskPath string, needUEFI bool, normalizedBootType, initType string) error {
	bootOpt := ""
	if needUEFI {
		bootOpt = "--boot uefi "
	}

	vcpuArg := fmt.Sprintf("--vcpus %d", params.VCPU)
	if params.MaxVCPU > params.VCPU {
		vcpuArg = fmt.Sprintf("--vcpus %d,maxvcpus=%d", params.VCPU, params.MaxVCPU)
	}

	// 网络：仅在有主网口交换机配置时才添加网络接口，否则显式禁用
	var networkArg string
	if params.SwitchID != 0 {
		networkArg = service.BuildOVSVirtInstallNetworkArg(params.NicModel) + " "
	} else {
		networkArg = "--network none "
	}
	installCmd := fmt.Sprintf(
		"virt-install --name '%s' --ram %d %s "+
			"--machine %s "+
			bootOpt+
			"--disk '%s,format=%s,bus=virtio,discard=unmap,detect_zeroes=unmap' "+
			"--osinfo detect=on,require=off "+
			networkArg+
			"--graphics vnc,listen=0.0.0.0 "+
			"--video virtio "+
			"--import --cpu host-passthrough --print-xml",
		params.Name, ramMB, vcpuArg, params.MachineType, destDiskPath, format,
	)
	result := utils.ExecCommandLongRunning("bash", "-c", installCmd)
	if result.Error != nil {
		_ = os.Remove(destDiskPath)
		return fmt.Errorf("生成虚拟机 XML 失败: %s", result.Stderr)
	}

	// 注入 memballoon 配置
	enableFPR := initType != "windows" && initType != "other"
	vmXML := service.InjectMemballoonConfig(result.Stdout, enableFPR)
	var err error
	if memoryMeta != nil {
		vmXML, err = vm_memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, enableFPR)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
	}
	vmXML, err = service.ApplyRTCConfigToDomainXML(vmXML, params.RTCOffset, params.RTCStartDate, initType)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = vm_xml.ApplyVMGuestAgentConfigToDomainXML(vmXML, params.GuestAgent)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = vm_xml.ApplySMBIOS1ConfigToDomainXML(vmXML, params.SMBIOS1, true)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = service.ApplyVMAPICToDomainXML(vmXML, params.APIC)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = vm_xml.ApplyVMPAEToDomainXML(vmXML, params.PAE)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, initType)
	topoVCPU := service.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = service.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, initType, topoVCPU)
	vmXML = service.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		var affErr error
		vmXML, affErr = service.ApplyCPUAffinityIfSet(vmXML, topoVCPU, params.CPUAffinity)
		if affErr != nil {
			_ = os.Remove(destDiskPath)
			return affErr
		}
	}
	appliedBootType := ""
	if needUEFI {
		bootType := normalizedBootType
		if bootType == "" {
			bootType = vm_xml.VMBootTypeUEFI
		}
		vmXML, err = vm_xml.ApplyVMBootTypeToDomainXML(params.Name, vmXML, bootType)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
		appliedBootType = bootType
	}
	vmXML, err = service.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	if err := vm_xml.EnsureVMUEFINVRAMFile(params.Name, vmXML, appliedBootType); err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}

	xmlPath := fmt.Sprintf("/tmp/_vm-import-%s.xml", params.Name)
	if err := os.WriteFile(xmlPath, []byte(vmXML), 0644); err != nil {
		_ = os.Remove(destDiskPath)
		return fmt.Errorf("写入虚拟机 XML 失败: %v", err)
	}
	defer os.Remove(xmlPath)

	defineResult := utils.ExecCommand("virsh", "define", xmlPath)
	_ = os.Remove(xmlPath)
	if defineResult.Error != nil {
		_ = os.Remove(destDiskPath)
		return fmt.Errorf("定义虚拟机失败: %s", defineResult.Stderr)
	}

	// Linux 离线初始化：在 VM 定义后、启动前通过 virt-customize 完成
	// 包括 machine-id 清理、cloud-init NoCloud seed 写入、密码修改等
	if initType == "linux" {
		linuxParams := &service.CloneParams{
			Name:         params.Name,
			Hostname:     params.Hostname,
			User:         params.User,
			Password:     params.Password,
			TemplateUser: params.TemplateUser,
		}
		if err := service.PrepareLinuxCloneFirstBootIdentity(linuxParams, destDiskPath); err != nil {
			_ = utils.ExecCommand("virsh", "undefine", params.Name, "--nvram")
			_ = os.Remove(destDiskPath)
			return fmt.Errorf("Linux 离线初始化失败: %w", err)
		}
	}

	return importVMPostDefine(params.Name, srcDiskPath, destDiskPath, params.CopyDisk, memoryMeta, params.Remark, params.Freeze, params.StartAfterImport)
}

// importDiskByPathLinuxDefine handles Linux/Other VM XML construction and define for ImportDiskByPath
func importDiskByPathLinuxDefine(params *ImportDiskByPathParams, destDiskPath, format string, ramMB int, memoryMeta *vm_memory.VMMemoryMetadata, mainDiskSrc string, needUEFI bool, normalizedBootType, initType string) error {
	bootOpt := ""
	if needUEFI {
		bootOpt = "--boot uefi "
	}

	vcpuArg := fmt.Sprintf("--vcpus %d", params.VCPU)
	if params.MaxVCPU > params.VCPU {
		vcpuArg = fmt.Sprintf("--vcpus %d,maxvcpus=%d", params.VCPU, params.MaxVCPU)
	}

	// 网络：仅在有主网口交换机配置时才添加网络接口，否则显式禁用
	var networkArg string
	if params.SwitchID != 0 {
		networkArg = service.BuildOVSVirtInstallNetworkArg(params.NicModel) + " "
	} else {
		networkArg = "--network none "
	}
	installCmd := fmt.Sprintf(
		"virt-install --name '%s' --ram %d %s "+
			"--machine %s "+
			bootOpt+
			"--disk '%s,format=%s,bus=virtio,discard=unmap,detect_zeroes=unmap' "+
			"--osinfo detect=on,require=off "+
			networkArg+
			"--graphics vnc,listen=0.0.0.0 "+
			"--video virtio "+
			"--import --cpu host-passthrough --virt-type kvm --print-xml",
		params.Name, ramMB, vcpuArg, params.MachineType, destDiskPath, format,
	)
	result := utils.ExecCommandLongRunning("bash", "-c", installCmd)
	if result.Error != nil {
		_ = os.Remove(destDiskPath)
		return fmt.Errorf("生成虚拟机 XML 失败: %s", result.Stderr)
	}

	xmlOutput := result.Stdout
	if idx := strings.Index(xmlOutput, "</domain>"); idx != -1 {
		xmlOutput = xmlOutput[:idx+len("</domain>")]
	}
	if idx := strings.Index(xmlOutput, "<domain"); idx > 0 {
		xmlOutput = xmlOutput[idx:]
	}

	enableFPR := initType != "windows" && initType != "other"
	vmXML := service.InjectMemballoonConfig(xmlOutput, enableFPR)
	var err error
	if memoryMeta != nil {
		vmXML, err = vm_memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, enableFPR)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
	}
	vmXML, err = service.ApplyRTCConfigToDomainXML(vmXML, params.RTCOffset, params.RTCStartDate, initType)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = vm_xml.ApplyVMGuestAgentConfigToDomainXML(vmXML, params.GuestAgent)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = vm_xml.ApplySMBIOS1ConfigToDomainXML(vmXML, params.SMBIOS1, true)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = service.ApplyVMAPICToDomainXML(vmXML, params.APIC)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML, err = vm_xml.ApplyVMPAEToDomainXML(vmXML, params.PAE)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, initType)
	topoVCPU := service.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = service.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, initType, topoVCPU)
	vmXML = service.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		var affErr error
		vmXML, affErr = service.ApplyCPUAffinityIfSet(vmXML, topoVCPU, params.CPUAffinity)
		if affErr != nil {
			_ = os.Remove(destDiskPath)
			return affErr
		}
	}
	appliedBootType := ""
	if needUEFI {
		bootType := normalizedBootType
		if bootType == "" {
			bootType = vm_xml.VMBootTypeUEFI
		}
		vmXML, err = vm_xml.ApplyVMBootTypeToDomainXML(params.Name, vmXML, bootType)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
		appliedBootType = bootType
	}
	vmXML, err = service.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}
	if err := vm_xml.EnsureVMUEFINVRAMFile(params.Name, vmXML, appliedBootType); err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}

	// SPICE graphics（默认本地监听），与 VNC 共存；是否启用由 per-VM 开关决定，回退全局默认
	spiceEnabled := config.GlobalConfig.SpiceEnabledByDefault
	if params.SpiceEnabled != nil {
		spiceEnabled = *params.SpiceEnabled
	}
	if spiceEnabled {
		vmXML = service.InjectSPICEGraphicsToDomainXML(vmXML, "", "127.0.0.1")
		vmXML = service.EnsureQXLVideo(vmXML)
	}

	xmlPath := fmt.Sprintf("/tmp/_vm-importd-%s.xml", params.Name)
	if err := os.WriteFile(xmlPath, []byte(vmXML), 0644); err != nil {
		_ = os.Remove(destDiskPath)
		return fmt.Errorf("写入虚拟机 XML 失败: %v", err)
	}
	defer os.Remove(xmlPath)

	defineResult := utils.ExecCommand("virsh", "define", xmlPath)
	_ = os.Remove(xmlPath)
	if defineResult.Error != nil {
		_ = os.Remove(destDiskPath)
		return fmt.Errorf("定义虚拟机失败: %s", defineResult.Stderr)
	}

	// Linux 离线初始化：在 VM 定义后、启动前通过 virt-customize 完成
	if initType == "linux" {
		linuxParams := &service.CloneParams{
			Name:         params.Name,
			Hostname:     params.Hostname,
			User:         params.User,
			Password:     params.Password,
			TemplateUser: params.TemplateUser,
		}
		if err := service.PrepareLinuxCloneFirstBootIdentity(linuxParams, destDiskPath); err != nil {
			_ = utils.ExecCommand("virsh", "undefine", params.Name, "--nvram")
			_ = os.Remove(destDiskPath)
			return fmt.Errorf("Linux 离线初始化失败: %w", err)
		}
	}

	return importVMPostDefine(params.Name, mainDiskSrc, destDiskPath, params.CopyDisk, memoryMeta, params.Remark, params.Freeze, params.StartAfterImport)
}
