package clone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/arch"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// buildWindowsCloudbaseInitConf 返回为 CloudbaseInit 主服务配置的 cloudbase-init.conf 内容。
// 配置 ConfigDriveService 作为元数据源，并启用完整的初始化插件列表，
// 覆盖模板中可能缺失 metadata_services/plugins 的默认配置。
func buildWindowsCloudbaseInitConf() string {
	return `[DEFAULT]
username=Administrator
groups=Administrators
inject_user_password=true
config_drive_raw_hhd=true
config_drive_cdrom=true
config_drive_vfat=true
bsdtar_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\bin\bsdtar.exe
mtools_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\bin\
verbose=true
debug=true
log_dir=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\log\
log_file=cloudbase-init.log
default_log_levels=comtypes=INFO,suds=INFO,iso8601=WARN,requests=WARN
logging_serial_port_settings=COM1,115200,N,8
mtu_use_dhcp_config=true
ntp_use_dhcp_config=true
local_scripts_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\LocalScripts\
metadata_services=cloudbaseinit.metadata.services.configdrive.ConfigDriveService,cloudbaseinit.metadata.services.base.EmptyMetadataService
plugins=cloudbaseinit.plugins.common.mtu.MTUPlugin,cloudbaseinit.plugins.windows.ntpclient.NTPClientPlugin,cloudbaseinit.plugins.common.sethostname.SetHostNamePlugin,cloudbaseinit.plugins.common.setuserpassword.SetUserPasswordPlugin,cloudbaseinit.plugins.windows.extendvolumes.ExtendVolumesPlugin,cloudbaseinit.plugins.common.userdata.UserDataPlugin,cloudbaseinit.plugins.common.localscripts.LocalScriptsPlugin
first_logon_behaviour=no
rename_admin_user=false
allow_reboot=false
check_latest_version=false
`
}

// buildWindowsPantherUnattendXML 根据 Windows 版本生成 Unattend.xml 内容。
// specialize pass: 禁用 AutoLogon + 设置临时密码
// oobeSystem pass: 跳过 OOBE 向导（Server 2025 / Windows 11 需要额外的 UserAccounts + AutoLogon）
func buildWindowsPantherUnattendXML(category string) string {
	needOOBEBypass := strings.EqualFold(category, "WindowsServer2025") ||
		strings.EqualFold(category, "Windows11")

	// Windows Server 2025 oobeSystem pass 必须同时包含：
	// 1. UserAccounts/AdministratorPassword → 告知 Windows 管理员密码已配置
	// 2. AutoLogon(LogonCount=1) → 强制跳过 Server 版的密码设置屏幕
	oobeContent := `
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <NetworkLocation>Work</NetworkLocation>
        <ProtectYourPC>1</ProtectYourPC>
        <SkipMachineOOBE>true</SkipMachineOOBE>
        <SkipUserOOBE>true</SkipUserOOBE>
      </OOBE>`
	if needOOBEBypass {
		oobeContent = `
      <UserAccounts>
        <AdministratorPassword>
          <Value>Temp@BootInit#1</Value>
          <PlainText>true</PlainText>
        </AdministratorPassword>
      </UserAccounts>
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <NetworkLocation>Work</NetworkLocation>
        <ProtectYourPC>1</ProtectYourPC>
        <SkipMachineOOBE>true</SkipMachineOOBE>
        <SkipUserOOBE>true</SkipUserOOBE>
      </OOBE>`
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">
  <settings pass="generalize">
    <component name="Microsoft-Windows-PnpSysprep" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <PersistAllDeviceInstalls>true</PersistAllDeviceInstalls>
    </component>
  </settings>
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">%s
    </component>
  </settings>
  <settings pass="specialize">
    <component name="Microsoft-Windows-Deployment" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <RunSynchronous>
        <RunSynchronousCommand wcm:action="add">
          <Order>1</Order>
          <Path>cmd.exe /c reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v AutoAdminLogon /t REG_SZ /d 0 /f</Path>
          <Description>Disable AutoLogon left over from template creation</Description>
          <WillReboot>Never</WillReboot>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>2</Order>
          <Path>cmd.exe /c net user Administrator "Temp@BootInit#1" /logonpasswordchg:no /active:yes</Path>
          <Description>Set temp password to prevent passwordless auto-login before cloudbase-init sets the real password</Description>
          <WillReboot>Never</WillReboot>
        </RunSynchronousCommand>
      </RunSynchronous>
    </component>
  </settings>
</unattend>`, oobeContent)
}

// detectWindowsNTFSPartition 检测磁盘镜像中的 Windows NTFS 系统分区。
// 使用 guestfish 列举文件系统，返回包含 /Windows 目录的 NTFS 分区设备路径。
// 如果检测失败，返回空字符串。
func detectWindowsNTFSPartition(diskPath string) string {
	// 列出所有文件系统
	result := utils.ExecCommandLongRunning("guestfish", "--ro", "-a", diskPath,
		"run", ":", "list-filesystems")
	if result.Error != nil {
		logger.App.Warn("检测 Windows 分区失败: guestfish list-filesystems", "error", result.Stderr)
		return ""
	}

	// 解析输出，寻找 NTFS 分区（格式: /dev/sdaX: ntfs）
	var ntfsPartitions []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "ntfs") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 1 {
				ntfsPartitions = append(ntfsPartitions, strings.TrimSpace(parts[0]))
			}
		}
	}

	if len(ntfsPartitions) == 0 {
		logger.App.Warn("未发现 NTFS 分区", "disk", diskPath)
		return ""
	}

	// 尝试每个 NTFS 分区，找到含 /Windows 目录的系统分区
	for _, part := range ntfsPartitions {
		checkResult := utils.ExecCommandLongRunning("guestfish", "--ro", "-a", diskPath,
			"run", ":", "mount-ro", part, "/", ":", "is-dir", "/Windows")
		if checkResult.Error == nil && strings.TrimSpace(checkResult.Stdout) == "true" {
			logger.App.Info("检测到 Windows 系统分区", "disk", diskPath, "partition", part)
			return part
		}
	}

	// 未找到含 /Windows 的分区，回退使用第一个 NTFS 分区
	logger.App.Warn("未找到含 /Windows 的 NTFS 分区，使用第一个 NTFS 分区", "partition", ntfsPartitions[0])
	return ntfsPartitions[0]
}

// injectWindowsCloudbaseInitFiles 通过 virt-customize 向克隆磁盘注入配置文件：
//  1. /Program Files/Cloudbase Solutions/Cloudbase-Init/conf/Unattend.xml
//  2. /Windows/Panther/unattend.xml
//  3. /Program Files/Cloudbase Solutions/Cloudbase-Init/conf/cloudbase-init.conf
//
// 当 libguestfs 的 OS 自动检测失败时（Windows Server 2025 已知问题），
// 自动回退为显式挂载 NTFS 分区（通过 -m 参数），绕过 OS 检测。
// 注入失败仅记录警告，不中断克隆流程。
func injectWindowsCloudbaseInitFiles(vmName, cloneDisk, category string, progressFn func(int, string)) {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}
	progressFn(35, "注入 CloudbaseInit 配置文件...")

	confContent := buildWindowsCloudbaseInitConf()
	confPath := fmt.Sprintf("/tmp/_cbi-conf-%s.conf", vmName)
	_ = os.WriteFile(confPath, []byte(confContent), 0600)
	defer func() { _ = os.Remove(confPath) }()

	unattendContent := buildWindowsPantherUnattendXML(category)
	unattendPath := fmt.Sprintf("/tmp/_cbi-unattend-%s.xml", vmName)
	_ = os.WriteFile(unattendPath, []byte(unattendContent), 0600)
	defer func() { _ = os.Remove(unattendPath) }()

	// 构造注入参数（上传到三个关键路径）
	uploadArgs := []string{
		"--mkdir", "/Windows/Panther/Unattend",
		"--upload", unattendPath + `:/Program Files/Cloudbase Solutions/Cloudbase-Init/conf/Unattend.xml`,
		"--upload", unattendPath + ":/Windows/Panther/unattend.xml",
		"--upload", confPath + `:/Program Files/Cloudbase Solutions/Cloudbase-Init/conf/cloudbase-init.conf`,
		"--quiet",
	}

	// 第一次尝试：使用默认 OS 检测（适用于大多数 Windows 版本）
	args := append([]string{"-a", cloneDisk, "--no-network"}, uploadArgs...)
	injectResult := utils.ExecCommandLongRunning("virt-customize", args...)

	if injectResult.Error != nil && strings.Contains(injectResult.Stderr, "no operating system") {
		// OS 检测失败（Windows Server 2025 已知问题），回退为 guestfish 显式挂载分区
		logger.App.Info("virt-customize OS 检测失败，尝试 guestfish 显式挂载 NTFS 分区", "vm", vmName)
		progressFn(36, "检测 Windows 分区...")

		winPart := detectWindowsNTFSPartition(cloneDisk)
		if winPart == "" {
			progressFn(38, "CloudbaseInit 配置文件注入失败，无法检测 Windows 分区")
			logger.App.Warn("注入 CloudbaseInit 配置失败: 无法检测 Windows 分区", "vm", vmName)
			return
		}

		// 使用 guestfish 显式挂载分区，绕过 OS 检测
		// ntfsfix 清除可能的脏标记（重装场景中 VM 可能未正常关机）
		progressFn(37, "通过 guestfish 注入文件...")
		injectResult = utils.ExecCommandLongRunning("guestfish", "--rw", "-a", cloneDisk,
			"run", ":",
			"ntfsfix", winPart, ":",
			"mount", winPart, "/", ":",
			"mkdir-p", "/Windows/Panther/Unattend", ":",
			"upload", unattendPath, "/Program Files/Cloudbase Solutions/Cloudbase-Init/conf/Unattend.xml", ":",
			"upload", unattendPath, "/Windows/Panther/unattend.xml", ":",
			"upload", confPath, "/Program Files/Cloudbase Solutions/Cloudbase-Init/conf/cloudbase-init.conf")
	}

	if injectResult.Error != nil {
		progressFn(38, "CloudbaseInit 配置文件注入失败，首次启动可能需要手动设置")
		logger.App.Warn("注入 CloudbaseInit 配置失败", "vm", vmName, "error", injectResult.Stderr)
	} else {
		logger.App.Info("CloudbaseInit 配置文件注入成功", "vm", vmName)
	}
}

func windowsSystemDiskTargetDev(bus string) string {
	switch D.NormalizeVMDiskBus(bus) {
	case "sata", "scsi":
		return "sda"
	case "ide":
		return "hda"
	default:
		return "vda"
	}
}

func windowsDiskControllerXML(bus string) string {
	switch D.NormalizeVMDiskBus(bus) {
	case "sata":
		return "    <controller type='sata' index='0'/>\n"
	case "scsi":
		return "    <controller type='scsi' index='0' model='virtio-scsi'/>\n"
	default:
		return ""
	}
}

// cloneWindows Windows 克隆逻辑
// cloneDir: 虚拟机磁盘所在的存储目录，额外磁盘也会创建在此目录
func cloneWindows(ctx context.Context, params *CloneParams, cloneDisk string, ramMB int, memoryMeta *memory.VMMemoryMetadata, needUEFI bool, isNoInit bool, progressFn func(int, string), cloneDir string) error {
	templateDir := config.GlobalConfig.TemplateDir

	// 获取宿主机架构 Profile，参数化 arch/machine/emulator/watchdog
	hostArch := arch.DetectHostArch()
	profile := arch.GetProfile(hostArch)
	archName := profile.Arch()
	machineType := profile.DefaultMachineType()
	emulatorPath := profile.EmulatorPath()
	watchdogModel := profile.DefaultWatchdogModel()
	isX8664 := archName == arch.ArchX8664

	// Hyper-V enlightenments 仅在 x86_64 架构上支持
	var hyperVBlock, hyperVTimerBlock string
	if isX8664 {
		hyperVBlock = "    <hyperv mode='custom'>\n      <relaxed state='on'/><vapic state='on'/><spinlocks state='on' retries='8191'/>\n    </hyperv>\n    "
		hyperVTimerBlock = "<timer name='pit' tickpolicy='delay'/>\n    <timer name='hpet' present='no'/><timer name='hypervclock' present='yes'/>"
	}

	var isoPath string
	var isoErr error
	if !isNoInit {
		password := params.Password
		if password == "" {
			password = "Qwert333"
		}

		// 注入 CloudbaseInit 配置文件（cloudbase-init.conf + Panther unattend.xml）
		injectWindowsCloudbaseInitFiles(params.Name, cloneDisk, params.TemplateCategory, progressFn)

		// 创建 Config Drive ISO（包含实例 hostname、admin_pass、instance-id）
		isoPath, isoErr = createWindowsConfigDriveISO(params.Name, params.Hostname, password)
		if isoErr != nil {
			logger.App.Warn("创建 Windows Config Drive ISO 失败，CloudbaseInit 将无法自动注入密码",
				"vm", params.Name, "error", isoErr)
		}
	}

	nvramClone := ""
	if needUEFI {
		nvramTemplate := filepath.Join(templateDir, "win2k22-nvram.fd")
		nvramClone = fmt.Sprintf("/var/lib/libvirt/qemu/nvram/%s_VARS.fd", params.Name)

		if utils.FileExists(nvramTemplate) {
			if err := vm_xml.CreateQCOW2NVRAMFromTemplate(nvramTemplate, nvramClone); err != nil {
				return err
			}
		} else {
			if err := vm_xml.CreateQCOW2NVRAMFromTemplate("/usr/share/OVMF/OVMF_VARS_4M.ms.fd", nvramClone); err != nil {
				return err
			}
		}
	}

	progressFn(40, "生成 Windows VM XML...")

	// 网络接口 XML：仅在有主网口交换机配置时才添加
	var networkXML string
	if params.SwitchID != 0 {
		macResult := utils.ExecShell(`printf '52:54:00:%02x:%02x:%02x' $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256))`)
		macAddr := strings.TrimSpace(macResult.Stdout)
		if macAddr == "" {
			macAddr = "52:54:00:aa:bb:cc"
		}
		networkXML = D.BuildOVSInterfaceXML(macAddr, params.NicModel) + "\n"
	}

	ramKiB := ramMB * 1024
	diskBus := D.NormalizeVMDiskBus(params.DiskBus)
	if diskBus == "" {
		diskBus = "virtio"
	}
	diskTargetDev := windowsSystemDiskTargetDev(diskBus)
	diskControllerXML := windowsDiskControllerXML(diskBus)
	osXML := fmt.Sprintf(`  <os>
    <type arch='%s' machine='%s'>hvm</type>
    <boot dev='hd'/>
  </os>`, archName, machineType)
	smmXML := ""
	tpmXML := ""
	if needUEFI {
		// 使用显式 loader/nvram 模式，不使用 firmware='efi' 自动选择，
		// 避免 libvirt 自动填充 nvram format='raw' 与 qcow2 格式不匹配导致黑屏。
		loaderPath := vm_xml.ResolveOVMFLoaderPath(true)
		varsTemplate := vm_xml.ResolveOVMFVarsTemplatePath(true)
		osXML = fmt.Sprintf(`  <os>
    <type arch='%s' machine='%s'>hvm</type>
    <loader readonly='yes' secure='yes' type='pflash'>%s</loader>
    <nvram template='%s' templateFormat='raw' format='qcow2'>%s</nvram>
    <boot dev='hd'/>
  </os>`, archName, machineType, loaderPath, varsTemplate, nvramClone)
		smmXML = "<smm state='on'/>"
		tpmXML = "    <tpm model='tpm-crb'><backend type='emulator' version='2.0'/></tpm>\n"
	}

	rtcOffset := D.ResolveRTCOffset(params.RTCOffset, "windows")
	rtcStartDate := D.NormalizeRTCStartDate(params.RTCStartDate)
	clockOpenTag := fmt.Sprintf("<clock offset='%s'>", rtcOffset)
	if rtcStartDate != D.VMRTCStartDateNow {
		epoch, err := D.ParseRTCStartDateToEpoch(rtcStartDate)
		if err != nil {
			return err
		}
		rtcOffset = D.VMRTCOffsetAbsolute
		clockOpenTag = fmt.Sprintf("<clock offset='%s' start='%s'>", rtcOffset, epoch)
	}
	vmXML := fmt.Sprintf(`<domain type='kvm'>
  <name>%s</name>
  <memory unit='KiB'>%d</memory>
%s
%s
  <features>
    <acpi/><apic/>
    %s<vmport state='off'/>%s
  </features>
  <cpu mode='host-passthrough' check='none' migratable='on'/>
  %s
    <timer name='rtc' tickpolicy='catchup'/>%s
  </clock>
  <on_poweroff>destroy</on_poweroff><on_reboot>restart</on_reboot><on_crash>destroy</on_crash>
  <pm><suspend-to-mem enabled='no'/><suspend-to-disk enabled='no'/></pm>
  <devices>
    <emulator>%s</emulator>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2' discard='unmap' detect_zeroes='unmap'/>
      <source file='%s'/><target dev='%s' bus='%s'/>
    </disk>
    <controller type='usb' index='0' model='qemu-xhci' ports='15'/>
    <controller type='virtio-serial' index='0'/>
%s
%s
    <input type='tablet' bus='usb'/>
%s
    <graphics type='vnc' port='-1' autoport='yes' listen='0.0.0.0'>
      <listen type='address' address='0.0.0.0'/>
    </graphics>
    <video><model type='virtio' heads='1' primary='yes'/></video>
    <watchdog model='%s' action='reset'/>
    <memballoon model='virtio' freePageReporting='on'><stats period='5'/></memballoon>
  </devices>
</domain>`,
		params.Name, ramKiB, D.BuildVCPUTag(params.VCPU, params.MaxVCPU), osXML, smmXML, hyperVBlock, clockOpenTag, hyperVTimerBlock, emulatorPath, cloneDisk, diskTargetDev, diskBus, diskControllerXML, networkXML, tpmXML, watchdogModel)
	var err error
	if memoryMeta != nil {
		vmXML, err = memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, false)
		if err != nil {
			return err
		}
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
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, "windows")
	vmXML = vm_xml.ApplyWindowsGuestOptimizationsToDomainXML(vmXML)
	topoVCPU := D.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = D.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, "windows", topoVCPU)
	vmXML = D.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		var affErr error
		vmXML, affErr = D.ApplyCPUAffinityIfSet(vmXML, topoVCPU, params.CPUAffinity)
		if affErr != nil {
			return affErr
		}
	}
	firstBootColdReboot := D.ShouldUseWindowsFirstBootColdReboot(params.FirstBootRebootMode, "windows")
	if firstBootColdReboot {
		vmXML = D.ApplyFirstBootRebootModeToDomainXML(vmXML, params.FirstBootRebootMode)
	}
	vmXML, err = D.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		return err
	}

	// 将 Config Drive ISO 挂载为 SATA CD-ROM，供 CloudbaseInit 首次启动时读取
	if !isNoInit && isoPath != "" {
		vmXML = addConfigDriveCDROMToXML(vmXML, isoPath, diskBus)
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

	// 额外磁盘：在启动前冷添加，避免占用 PCIe 热插槽
	if len(params.ExtraDisks) > 0 {
		if err := D.AddExtraDisksForVM(params.Name, params.ExtraDisks, cloneDir, params.DiskBus, params.IsAdmin, nil); err != nil {
			return fmt.Errorf("挂载额外磁盘失败: %w", err)
		}
	}

	startFn := D.StartVM
	if firstBootColdReboot {
		startFn = D.StartVMPreserveRebootAction
	}
	if err := startFn(params.Name); err != nil {
		return err
	}
	if firstBootColdReboot {
		if err := D.CompleteWindowsFirstBootColdReboot(ctx, params.Name, progressFn); err != nil {
			return err
		}
	}

	// 在后台等待 QEMU Guest Agent 连接后自动弹出并清理 Config Drive CD-ROM
	if !isNoInit && isoPath != "" {
		scheduleWindowsConfigDriveEject(params.Name, diskBus)
	}

	return nil
}
