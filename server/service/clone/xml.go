package clone

import (
	"fmt"
	"regexp"
	"strings"

	"kvm_console/logger"
	"kvm_console/service/arch"
	"kvm_console/service/libvirt_rpc"
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

func setCloneShimFallbackNoReboot(vmName, domainXML string) {
	nvramPath := extractDomainNVRAMPath(domainXML)
	if nvramPath == "" {
		return
	}
	if err := vm_xml.SetShimFallbackNoReboot(nvramPath); err != nil {
		logger.App.Warn("设置 UEFI 克隆首次启动连续引导失败，首次启动时可能短暂显示启动项恢复界面",
			"vm", vmName, "nvram", nvramPath, "error", err)
	}
}

// defineAndStartNonWindowsClone 为 Linux/FnOS/OpenWrt/Other 类型生成 XML、注入配置、定义并启动虚拟机
// extraDiskDir: 额外磁盘的存储目录
func defineAndStartNonWindowsClone(params *CloneParams, cloneDisk string, ramMB int, tplType string, cloneBootType string, needUEFI bool, templateNVRAMPath string, extraDiskDir string) error {
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
		networkArg = withVirtInstallMAC(networkArg, params.PrimaryMAC)
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
			"--graphics vnc,listen=127.0.0.1 "+
			"--video virtio "+
			"--import --cpu host-passthrough --print-xml",
		utils.ShellSingleQuote(params.Name), ramMB, vcpuArg, utils.ShellSingleQuote(cloneDisk), params.DiskBus,
	)
	result := utils.ExecCommandLongRunning("bash", "-c", installCmd)
	if result.Error != nil {
		return fmt.Errorf("生成虚拟机 XML 失败: %s", result.Stderr)
	}

	vmXML := InjectMemballoonConfig(result.Stdout, !isOther)

	additionalPCIEDevices := len(params.ExtraNics) + len(params.ExtraDisks) + len(params.HostDevices)
	pciePortCount := vm_xml.ResolveCreatePCIERootPortCount(vmXML, params.PCIERootPorts, additionalPCIEDevices)
	vmXML = D.InjectPCIERootPorts(vmXML, pciePortCount)

	var err error
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
	if needUEFI {
		setCloneShimFallbackNoReboot(params.Name, vmXML)
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
	vmXML, err = applyCloneHostDevicesToDomainXML(vmXML, params.HostDevices, params.IsAdmin)
	if err != nil {
		return err
	}

	// 嵌套虚拟化开关（默认启用，host-passthrough 下需 policy='disable' 覆盖）
	if params.NestedVirt == nil || *params.NestedVirt {
		featureName := vm_xml.DetectHostNestedVirtFeatureName()
		enabled := true
		vmXML, err = vm_xml.ApplyNestedVirtToDomainXML(vmXML, &enabled, featureName)
		if err != nil {
			return err
		}
	} else {
		featureName := vm_xml.DetectHostNestedVirtFeatureName()
		disabled := false
		vmXML, err = vm_xml.ApplyNestedVirtToDomainXML(vmXML, &disabled, featureName)
		if err != nil {
			return err
		}
	}

	// 隐藏 KVM 标志
	if params.KVMHidden != nil {
		vmXML, err = vm_xml.ApplyKVMHiddenToDomainXML(vmXML, params.KVMHidden)
		if err != nil {
			return err
		}
	}

	// Hyper-V vendor_id 伪装
	if strings.TrimSpace(params.VendorID) != "" {
		vmXML, err = vm_xml.ApplyVendorIDToHyperVBlock(vmXML, params.VendorID)
		if err != nil {
			return err
		}
	}

	if _, err := libvirt_rpc.DefineDomainXMLRPC(vmXML); err != nil {
		return fmt.Errorf("定义虚拟机失败: %w", err)
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

	// 额外磁盘：在启动前冷添加，避免占用 PCIe 热插槽
	if len(params.ExtraDisks) > 0 {
		if err := D.AddExtraDisksForVM(params.Name, params.ExtraDisks, extraDiskDir, params.DiskBus, params.IsAdmin, nil); err != nil {
			return fmt.Errorf("挂载额外磁盘失败: %w", err)
		}
	}
	if D.PrepareVMPortSecurityBinding != nil {
		if err := D.PrepareVMPortSecurityBinding(params.Owner, params.Name, params.SwitchID, params.SecurityGroupID, params.AllowedIPv4Addresses, params.AllowedIPv6Addresses); err != nil {
			return fmt.Errorf("启动前准备端口安全绑定失败: %w", err)
		}
	}

	if err := D.StartVM(params.Name); err != nil {
		return err
	}
	return nil
}

func withVirtInstallMAC(networkArg, mac string) string {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return networkArg
	}
	end := strings.LastIndex(networkArg, "'")
	if end < 0 {
		return networkArg
	}
	return networkArg[:end] + ",mac=" + mac + networkArg[end:]
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
