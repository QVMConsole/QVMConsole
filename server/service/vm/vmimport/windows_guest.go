package vmimport

import (
	"fmt"
	"os"
	"strings"

	"kvm_console/service"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// importVMWindowsDefine handles Windows VM XML construction and define for ImportVM
func importVMWindowsDefine(params *ImportVMParams, destDiskPath, format string, ramMB int, memoryMeta *vm_memory.VMMemoryMetadata, srcDiskPath string, needUEFI bool) error {
	// 网络接口 XML：仅在有主网口交换机配置时才添加
	var networkXML string
	if params.SwitchID != 0 {
		macResult := utils.ExecShell(`printf '52:54:00:%02x:%02x:%02x' $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256))`)
		macAddr := strings.TrimSpace(macResult.Stdout)
		if macAddr == "" {
			macAddr = "52:54:00:aa:bb:cc"
		}
		networkXML = service.BuildOVSInterfaceXML(macAddr, params.NicModel) + "\n"
	}

	// Generate qcow2 NVRAM
	nvramClone := fmt.Sprintf("/var/lib/libvirt/qemu/nvram/%s_VARS.fd", params.Name)
	if err := vm_xml.CreateQCOW2NVRAMFromTemplate("/usr/share/OVMF/OVMF_VARS_4M.ms.fd", nvramClone); err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}

	ramKiB := ramMB * 1024

	rtcOffset := service.ResolveRTCOffset(params.RTCOffset, "windows")
	rtcStartDate := service.NormalizeRTCStartDate(params.RTCStartDate)
	clockOpenTag := fmt.Sprintf("<clock offset='%s'>", rtcOffset)
	if rtcStartDate != service.VMRTCStartDateNow {
		epoch, err := service.ParseRTCStartDateToEpoch(rtcStartDate)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
		rtcOffset = service.VMRTCOffsetAbsolute
		clockOpenTag = fmt.Sprintf("<clock offset='%s' start='%s'>", rtcOffset, epoch)
	}

	// 使用显式 loader/nvram，不使用 firmware='efi' 自动选择，
	// 避免 libvirt 自动填充 nvram format='raw' 与 qcow2 格式不匹配导致黑屏。
	loaderPath := vm_xml.ResolveOVMFLoaderPath(true)
	varsTemplate := vm_xml.ResolveOVMFVarsTemplatePath(true)

	vmXML := fmt.Sprintf(`<domain type='kvm'>
  <name>%s</name>
  <memory unit='KiB'>%d</memory>
%s
  <os>
    <type arch='x86_64' machine='pc-q35-noble'>hvm</type>
    <loader readonly='yes' secure='yes' type='pflash'>%s</loader>
    <nvram template='%s' templateFormat='raw' format='qcow2'>%s</nvram>
    <boot dev='hd'/>
  </os>
  <features>
    <acpi/><apic/>
    <hyperv mode='custom'>
      <relaxed state='on'/><vapic state='on'/><spinlocks state='on' retries='8191'/>
    </hyperv>
    <vmport state='off'/><smm state='on'/>
  </features>
  <cpu mode='host-passthrough' check='none' migratable='on'/>
  %s
    <timer name='rtc' tickpolicy='catchup'/><timer name='pit' tickpolicy='delay'/>
    <timer name='hpet' present='no'/><timer name='hypervclock' present='yes'/>
  </clock>
  <on_poweroff>destroy</on_poweroff><on_reboot>restart</on_reboot><on_crash>destroy</on_crash>
  <pm><suspend-to-mem enabled='no'/><suspend-to-disk enabled='no'/></pm>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <disk type='file' device='disk'>
      <driver name='qemu' type='%s' discard='unmap' detect_zeroes='unmap'/>
      <source file='%s'/><target dev='vda' bus='virtio'/>
    </disk>
    <controller type='usb' index='0' model='qemu-xhci' ports='15'/>
    <controller type='virtio-serial' index='0'/>
%s
    <input type='tablet' bus='usb'/>
    <tpm model='tpm-crb'><backend type='emulator' version='2.0'/></tpm>
    <graphics type='vnc' port='-1' autoport='yes' listen='0.0.0.0'>
      <listen type='address' address='0.0.0.0'/>
    </graphics>
    <video><model type='virtio' heads='1' primary='yes'/></video>
    <watchdog model='itco' action='reset'/>
    <memballoon model='virtio' freePageReporting='on'><stats period='5'/></memballoon>
  </devices>
</domain>`,
		params.Name, ramKiB, service.BuildVCPUTag(params.VCPU, params.MaxVCPU), loaderPath, varsTemplate, nvramClone, clockOpenTag, format, destDiskPath, networkXML)

	var err error
	if memoryMeta != nil {
		vmXML, err = vm_memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, false)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
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
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, "windows")
	vmXML = vm_xml.ApplyWindowsGuestOptimizationsToDomainXML(vmXML)
	topoVCPU := service.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = service.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, "windows", topoVCPU)
	vmXML = service.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		affinityCores, affErr := service.ParseCPUAffinity(params.CPUAffinity)
		if affErr != nil {
			_ = os.Remove(destDiskPath)
			return fmt.Errorf("CPU 亲和性格式错误: %w", affErr)
		}
		if len(affinityCores) > 0 {
			if affErr := service.ValidateCPUAffinity(affinityCores); affErr != nil {
				_ = os.Remove(destDiskPath)
				return affErr
			}
		}
		vmXML = service.ApplyCPUAffinityToDomainXML(vmXML, topoVCPU, affinityCores)
	}
	vmXML, err = service.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
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

	return importVMPostDefine(params.Name, srcDiskPath, destDiskPath, params.CopyDisk, memoryMeta, params.Remark, params.Freeze, params.StartAfterImport)
}

// importDiskByPathWindowsDefine handles Windows VM XML construction and define for ImportDiskByPath
func importDiskByPathWindowsDefine(params *ImportDiskByPathParams, destDiskPath, format string, ramMB int, memoryMeta *vm_memory.VMMemoryMetadata, mainDiskSrc string) error {
	// 网络接口 XML：仅在有主网口交换机配置时才添加
	var networkXML string
	if params.SwitchID != 0 {
		macResult := utils.ExecShell(`printf '52:54:00:%02x:%02x:%02x' $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256))`)
		macAddr := strings.TrimSpace(macResult.Stdout)
		if macAddr == "" {
			macAddr = "52:54:00:aa:bb:cc"
		}
		networkXML = service.BuildOVSInterfaceXML(macAddr, params.NicModel) + "\n"
	}

	nvramClone := fmt.Sprintf("/var/lib/libvirt/qemu/nvram/%s_VARS.fd", params.Name)
	if err := vm_xml.CreateQCOW2NVRAMFromTemplate("/usr/share/OVMF/OVMF_VARS_4M.ms.fd", nvramClone); err != nil {
		_ = os.Remove(destDiskPath)
		return err
	}

	ramKiB := ramMB * 1024

	rtcOffset := service.ResolveRTCOffset(params.RTCOffset, "windows")
	rtcStartDate := service.NormalizeRTCStartDate(params.RTCStartDate)
	clockOpenTag := fmt.Sprintf("<clock offset='%s'>", rtcOffset)
	if rtcStartDate != service.VMRTCStartDateNow {
		epoch, err := service.ParseRTCStartDateToEpoch(rtcStartDate)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
		rtcOffset = service.VMRTCOffsetAbsolute
		clockOpenTag = fmt.Sprintf("<clock offset='%s' start='%s'>", rtcOffset, epoch)
	}

	// 使用显式 loader/nvram，不使用 firmware='efi' 自动选择
	loaderPath2 := vm_xml.ResolveOVMFLoaderPath(true)
	varsTemplate2 := vm_xml.ResolveOVMFVarsTemplatePath(true)

	vmXML := fmt.Sprintf(`<domain type='kvm'>
  <name>%s</name>
  <memory unit='KiB'>%d</memory>
%s
  <os>
    <type arch='x86_64' machine='pc-q35-noble'>hvm</type>
    <loader readonly='yes' secure='yes' type='pflash'>%s</loader>
    <nvram template='%s' templateFormat='raw' format='qcow2'>%s</nvram>
    <boot dev='hd'/>
  </os>
  <features>
    <acpi/><apic/>
    <hyperv mode='custom'>
      <relaxed state='on'/><vapic state='on'/><spinlocks state='on' retries='8191'/>
    </hyperv>
    <vmport state='off'/><smm state='on'/>
  </features>
  <cpu mode='host-passthrough' check='none' migratable='on'/>
  %s
    <timer name='rtc' tickpolicy='catchup'/><timer name='pit' tickpolicy='delay'/>
    <timer name='hpet' present='no'/><timer name='hypervclock' present='yes'/>
  </clock>
  <on_poweroff>destroy</on_poweroff><on_reboot>restart</on_reboot><on_crash>destroy</on_crash>
  <pm><suspend-to-mem enabled='no'/><suspend-to-disk enabled='no'/></pm>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <disk type='file' device='disk'>
      <driver name='qemu' type='%s' discard='unmap' detect_zeroes='unmap'/>
      <source file='%s'/><target dev='vda' bus='virtio'/>
    </disk>
    <controller type='usb' index='0' model='qemu-xhci' ports='15'/>
    <controller type='virtio-serial' index='0'/>
%s
    <input type='tablet' bus='usb'/>
    <tpm model='tpm-crb'><backend type='emulator' version='2.0'/></tpm>
    <graphics type='vnc' port='-1' autoport='yes' listen='0.0.0.0'>
      <listen type='address' address='0.0.0.0'/>
    </graphics>
    <video><model type='virtio' heads='1' primary='yes'/></video>
    <watchdog model='itco' action='reset'/>
    <memballoon model='virtio' freePageReporting='on'><stats period='5'/></memballoon>
  </devices>
</domain>`,
		params.Name, ramKiB, service.BuildVCPUTag(params.VCPU, params.MaxVCPU), loaderPath2, varsTemplate2, nvramClone, clockOpenTag, format, destDiskPath, networkXML)

	var err error
	if memoryMeta != nil {
		vmXML, err = vm_memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, false)
		if err != nil {
			_ = os.Remove(destDiskPath)
			return err
		}
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
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, "windows")
	vmXML = vm_xml.ApplyWindowsGuestOptimizationsToDomainXML(vmXML)
	topoVCPU := service.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = service.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, "windows", topoVCPU)
	vmXML = service.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		var affErr error
		vmXML, affErr = service.ApplyCPUAffinityIfSet(vmXML, topoVCPU, params.CPUAffinity)
		if affErr != nil {
			_ = os.Remove(destDiskPath)
			return affErr
		}
	}
	vmXML, err = service.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		_ = os.Remove(destDiskPath)
		return err
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

	return importVMPostDefine(params.Name, mainDiskSrc, destDiskPath, params.CopyDisk, memoryMeta, params.Remark, params.Freeze, params.StartAfterImport)
}
