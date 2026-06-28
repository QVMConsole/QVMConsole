package clone

import (
	"fmt"
	"regexp"
	"strings"

	"kvm_console/logger"
	"kvm_console/service/arch"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// InjectMemballoonConfig 向 virt-install --print-xml 生成的 XML 注入 memballoon 配置
func InjectMemballoonConfig(xmlStr string, enableFPR bool) string {
	var memballoonXML string
	if enableFPR {
		memballoonXML = `    <memballoon model="virtio" freePageReporting="on"><stats period="5"/></memballoon>`
	} else {
		memballoonXML = `    <memballoon model="virtio"><stats period="5"/></memballoon>`
	}

	if strings.Contains(xmlStr, "<memballoon") {
		re := regexp.MustCompile(`(?s)<memballoon[^>]*>.*?</memballoon>`)
		if re.MatchString(xmlStr) {
			return re.ReplaceAllString(xmlStr, memballoonXML)
		}
		reSelf := regexp.MustCompile(`<memballoon[^/]*/\s*>`)
		if reSelf.MatchString(xmlStr) {
			return reSelf.ReplaceAllString(xmlStr, memballoonXML)
		}
	}

	return strings.Replace(xmlStr, "</devices>", memballoonXML+"\n  </devices>", 1)
}

func prepareUEFITemplateNVRAMForClone(domainXML, vmName, templateNVRAMPath string) (string, error) {
	templateNVRAMPath = strings.TrimSpace(templateNVRAMPath)
	if templateNVRAMPath == "" {
		return domainXML, nil
	}
	if !utils.FileExists(templateNVRAMPath) {
		return domainXML, nil
	}
	cloneNVRAMPath := extractDomainNVRAMPath(domainXML)
	if cloneNVRAMPath == "" {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" {
			return domainXML, fmt.Errorf("无法生成克隆虚拟机 NVRAM 路径")
		}
		cloneNVRAMPath = fmt.Sprintf("/var/lib/libvirt/qemu/nvram/%s_VARS.fd", vmName)
		domainXML = ensureDomainNVRAMPath(domainXML, cloneNVRAMPath)
	}
	if err := vm_xml.CreateQCOW2NVRAMFromTemplate(templateNVRAMPath, cloneNVRAMPath); err != nil {
		return domainXML, fmt.Errorf("复制模板 UEFI NVRAM 失败: %w", err)
	}
	return vm_xml.SetDomainNVRAMFormat(domainXML, "qcow2"), nil
}

func ensureDomainNVRAMPath(domainXML, nvramPath string) string {
	nvramPath = strings.TrimSpace(nvramPath)
	if strings.TrimSpace(domainXML) == "" || nvramPath == "" {
		return domainXML
	}
	reWithContent := regexp.MustCompile(`(?s)<nvram([^>]*)>\s*[^<]*\s*</nvram>`)
	if reWithContent.MatchString(domainXML) {
		return vm_xml.SetDomainNVRAMFormat(reWithContent.ReplaceAllString(domainXML, "<nvram$1>"+nvramPath+"</nvram>"), "qcow2")
	}
	reSelfClosing := regexp.MustCompile(`(?s)<nvram([^/]*)/\s*>`)
	if reSelfClosing.MatchString(domainXML) {
		return vm_xml.SetDomainNVRAMFormat(reSelfClosing.ReplaceAllString(domainXML, "<nvram$1>"+nvramPath+"</nvram>"), "qcow2")
	}
	nvramXML := fmt.Sprintf("    <nvram template='/usr/share/OVMF/OVMF_VARS_4M.ms.fd' templateFormat='raw' format='qcow2'>%s</nvram>\n", nvramPath)
	if strings.Contains(domainXML, "</os>") {
		return strings.Replace(domainXML, "</os>", nvramXML+"  </os>", 1)
	}
	return domainXML
}

// defineAndStartNonWindowsClone 为 Linux/FnOS/Other 类型生成 XML、注入配置、定义并启动虚拟机
func defineAndStartNonWindowsClone(params *CloneParams, cloneDisk string, ramMB int, memoryMeta *memory.VMMemoryMetadata, tplType string, cloneBootType string, needUEFI bool, templateNVRAMPath string) error {
	isOther := tplType == "other"

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
		networkArg = D.BuildOVSVirtInstallNetworkArg(params.NicModel) + " "
	} else {
		networkArg = "--network none "
	}
	installCmd := fmt.Sprintf(
		"virt-install --name %s --ram %d %s "+
			fmt.Sprintf("--machine %s ", arch.GetProfile(arch.DetectHostArch()).DefaultMachineType())+
			bootOpt+
			"--disk %s,format=qcow2,bus=%s,discard=unmap,detect_zeroes=unmap "+
			"--osinfo detect=on,require=off "+
			networkArg+
			"--graphics vnc,listen=0.0.0.0 "+
			"--video virtio "+
			"--import --cpu host-passthrough --print-xml",
		utils.ShellSingleQuote(params.Name), ramMB, vcpuArg, utils.ShellSingleQuote(cloneDisk), params.DiskBus,
	)
	result := utils.ExecCommandLongRunning("bash", "-c", installCmd)
	if result.Error != nil {
		return fmt.Errorf("生成虚拟机 XML 失败: %s", result.Stderr)
	}

	vmXML := InjectMemballoonConfig(result.Stdout, !isOther)

	pciePortCount := params.PCIERootPorts
	if pciePortCount <= 0 {
		pciePortCount = 6
	}
	vmXML = D.InjectPCIERootPorts(vmXML, pciePortCount)

	var err error
	if memoryMeta != nil {
		vmXML, err = memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, !isOther)
		if err != nil {
			return err
		}
	}
	vmXML, err = D.ApplyRTCConfigToDomainXML(vmXML, params.RTCOffset, params.RTCStartDate, tplType)
	if err != nil {
		return err
	}
	vmXML, err = vm_xml.ApplyVMGuestAgentConfigToDomainXML(vmXML, params.GuestAgent)
	if err != nil {
		return err
	}
	vmXML, err = vm_xml.ApplySMBIOS1ConfigToDomainXML(vmXML, params.SMBIOS1, true)
	if err != nil {
		return err
	}
	vmXML, err = D.ApplyVMAPICToDomainXML(vmXML, params.APIC)
	if err != nil {
		return err
	}
	vmXML, err = vm_xml.ApplyVMPAEToDomainXML(vmXML, params.PAE)
	if err != nil {
		return err
	}
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, tplType)
	topoVCPU := D.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = D.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, tplType, topoVCPU)
	vmXML = D.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		var affErr error
		vmXML, affErr = D.ApplyCPUAffinityIfSet(vmXML, topoVCPU, params.CPUAffinity)
		if affErr != nil {
			return affErr
		}
	}
	if cloneBootType != "" {
		vmXML, err = vm_xml.ApplyVMBootTypeToDomainXML(params.Name, vmXML, cloneBootType)
		if err != nil {
			return err
		}
	}
	vmXML, err = D.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		return err
	}
	if needUEFI {
		vmXML, err = prepareUEFITemplateNVRAMForClone(vmXML, params.Name, templateNVRAMPath)
		if err != nil {
			return err
		}
	}
	if err := vm_xml.EnsureVMUEFINVRAMFile(params.Name, vmXML, cloneBootType); err != nil {
		return err
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

	if _, err := libvirt_rpc.DefineDomainXMLRPC(vmXML); err != nil {
		return fmt.Errorf("定义虚拟机失败: %w", err)
	}
	if memoryMeta != nil {
		if err := memory.WriteVMMemoryMetadata(params.Name, memoryMeta); err != nil {
			return err
		}
	}
	cloneMode := params.CloneMode
	if cloneMode == "" {
		cloneMode = "linked"
	}
	if err := D.WriteVMTemplateSource(params.Name, params.Template, cloneMode); err != nil {
		logger.App.Warn("写入VM模板源信息失败", "error", err)
	}
	if err := D.SetVMRemark(params.Name, params.Remark); err != nil {
		logger.App.Warn("设置VM备注失败", "error", err)
	}

	if err := D.SetVMFreeze(params.Name, params.Freeze); err != nil {
		logger.App.Warn("设置VM冻结配置失败", "error", err)
	}

	if err := D.StartVM(params.Name); err != nil {
		return err
	}
	return nil
}

// extractDomainNVRAMPath extracts the NVRAM path from domain XML
func extractDomainNVRAMPath(domainXML string) string {
	reWithContent := regexp.MustCompile(`(?s)<nvram[^>]*>\s*([^<]+)\s*</nvram>`)
	matches := reWithContent.FindStringSubmatch(domainXML)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}
