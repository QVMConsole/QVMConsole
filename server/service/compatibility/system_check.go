package compatibility

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/model"
	rootservice "kvm_console/service"
	"kvm_console/service/arch"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

const (
	stagePassed = "passed"
	stageFailed = "failed"
)

// Options 描述安装兼容性实机测试使用的最小资源参数。
type Options struct {
	VCPU      int
	RAMGB     int
	DiskGB    int
	ReportDir string
	Context   context.Context
	InitError error
}

// StageResult 记录一个兼容性检查阶段的结果。
type StageResult struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
	CheckedAt time.Time         `json:"checked_at"`
}

// VMParameters 记录实际送入正式 CreateVM 链路的关键参数。
type VMParameters struct {
	Name         string `json:"name"`
	Arch         string `json:"arch"`
	VirtType     string `json:"virt_type"`
	MachineType  string `json:"machine_type"`
	BootType     string `json:"boot_type"`
	VCPU         int    `json:"vcpu"`
	RAMGB        int    `json:"ram_gb"`
	DiskGB       int    `json:"disk_gb"`
	DiskFormat   string `json:"disk_format"`
	DiskBus      string `json:"disk_bus"`
	NICModel     string `json:"nic_model"`
	VideoModel   string `json:"video_model"`
	SwitchID     uint   `json:"switch_id"`
	SecurityID   uint   `json:"security_group_id"`
	OVSBridge    string `json:"ovs_bridge"`
	StoragePool  string `json:"storage_pool_id,omitempty"`
	StoragePath  string `json:"storage_path,omitempty"`
	NestedVirt   bool   `json:"nested_virt"`
	GuestAgent   bool   `json:"guest_agent"`
	PCIERootPort int    `json:"pcie_root_ports"`
}

// Report 是安装兼容性测试的持久化报告。
type Report struct {
	Version       int               `json:"version"`
	Compatible    bool              `json:"compatible"`
	StartedAt     time.Time         `json:"started_at"`
	FinishedAt    time.Time         `json:"finished_at"`
	HostArch      string            `json:"host_arch"`
	VM            VMParameters      `json:"vm"`
	Stages        []StageResult     `json:"stages"`
	Error         string            `json:"error,omitempty"`
	CleanupError  string            `json:"cleanup_error,omitempty"`
	ReportPath    string            `json:"report_path"`
	XMLPath       string            `json:"xml_path,omitempty"`
	ActiveXMLPath string            `json:"active_xml_path,omitempty"`
	DiagnosticLog string            `json:"diagnostic_log,omitempty"`
	OVSStatus     map[string]any    `json:"ovs_status,omitempty"`
	Extra         map[string]string `json:"extra,omitempty"`
}

type runner struct {
	options      Options
	report       *Report
	progress     func(string)
	vmName       string
	diskPath     string
	expectedDisk string
	created      bool
	xmlContent   string
	activeXML    string
}

type domainXML struct {
	Type string `xml:"type,attr"`
	OS   struct {
		Type struct {
			Arch    string `xml:"arch,attr"`
			Machine string `xml:"machine,attr"`
		} `xml:"type"`
	} `xml:"os"`
	Clock struct {
		Offset string `xml:"offset,attr"`
	} `xml:"clock"`
	Features struct {
		APIC *struct{} `xml:"apic"`
		PAE  *struct{} `xml:"pae"`
	} `xml:"features"`
	CPU struct {
		Features []domainCPUFeature `xml:"feature"`
	} `xml:"cpu"`
	Devices struct {
		Interfaces  []domainInterface  `xml:"interface"`
		Disks       []domainDisk       `xml:"disk"`
		Controllers []domainController `xml:"controller"`
	} `xml:"devices"`
}

type domainCPUFeature struct {
	Name   string `xml:"name,attr"`
	Policy string `xml:"policy,attr"`
}

type domainDisk struct {
	Device string `xml:"device,attr"`
	Driver struct {
		Type string `xml:"type,attr"`
	} `xml:"driver"`
	Target struct {
		Bus string `xml:"bus,attr"`
	} `xml:"target"`
}

type domainController struct {
	Model string `xml:"model,attr"`
}

type domainInterface struct {
	Type string `xml:"type,attr"`
	MAC  struct {
		Address string `xml:"address,attr"`
	} `xml:"mac"`
	Source struct {
		Bridge string `xml:"bridge,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
	} `xml:"target"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
	VirtualPort struct {
		Type string `xml:"type,attr"`
	} `xml:"virtualport"`
}

// RunSystemCheck 通过正式虚拟机创建链路执行一次宿主机兼容性实机测试。
func RunSystemCheck(options Options, progress func(string)) (report *Report, retErr error) {
	options = normalizeOptions(options)
	if progress == nil {
		progress = func(string) {}
	}
	if err := os.MkdirAll(options.ReportDir, 0700); err != nil {
		return nil, fmt.Errorf("创建兼容性报告目录失败: %w", err)
	}
	_ = os.Chmod(options.ReportDir, 0700)

	startedAt := time.Now()
	vmName := fmt.Sprintf("qvmcompat-%s-%d", startedAt.UTC().Format("20060102-150405"), os.Getpid())
	baseName := vmName + "-report"
	reportPath := filepath.Join(options.ReportDir, baseName+".json")

	r := &runner{
		options:  options,
		progress: progress,
		vmName:   vmName,
		report: &Report{
			Version:    1,
			StartedAt:  startedAt,
			VM:         VMParameters{Name: vmName},
			ReportPath: reportPath,
			Extra:      map[string]string{},
		},
	}

	defer func() {
		if retErr != nil {
			r.report.Error = retErr.Error()
			r.captureFailureDiagnostics()
		}
		cleanupErr := r.cleanup()
		if cleanupErr != nil {
			r.report.CleanupError = cleanupErr.Error()
			if retErr == nil {
				retErr = cleanupErr
				r.report.Error = cleanupErr.Error()
			}
		}
		r.report.Compatible = retErr == nil
		r.report.FinishedAt = time.Now()
		if writeErr := r.writeReport(); writeErr != nil {
			if retErr == nil {
				retErr = writeErr
				r.report.Error = writeErr.Error()
			}
			r.report.Compatible = false
		}
		report = r.report
	}()

	if err := r.run(); err != nil {
		retErr = err
	}
	return r.report, retErr
}

func normalizeOptions(options Options) Options {
	if options.VCPU <= 0 {
		options.VCPU = 1
	}
	if options.RAMGB <= 0 {
		options.RAMGB = 1
	}
	if options.DiskGB <= 0 {
		options.DiskGB = 1
	}
	if strings.TrimSpace(options.ReportDir) == "" {
		options.ReportDir = filepath.Join("logs", "compatibility")
	}
	options.ReportDir = filepath.Clean(options.ReportDir)
	if options.Context == nil {
		options.Context = context.Background()
	}
	return options
}

func (r *runner) run() error {
	if err := r.checkInterrupted(); err != nil {
		return r.fail("用户中断", err)
	}
	hostArch := arch.DetectHostArch()
	r.report.HostArch = hostArch
	r.report.VM = vmParametersFromCreateParams(r.buildCreateParams(hostArch, 0, 0), "")
	if r.options.InitError != nil {
		return r.fail("libvirt RPC 初始化", r.options.InitError)
	}
	r.progress("检查 KVM、libvirt、QEMU 与 OVS 命令...")
	if err := r.checkRequiredRuntime(); err != nil {
		return r.fail("运行环境检查", err)
	}
	r.pass("运行环境检查", "KVM 设备及虚拟化命令均可用", nil)
	if err := r.checkInterrupted(); err != nil {
		return r.fail("用户中断", err)
	}

	r.progress("检查并准备基础 OVS 网络...")
	if err := rootservice.EnsureOVSNetworkReady(); err != nil {
		return r.fail("OVS 基础网络", err)
	}
	ovsStatus, err := rootservice.GetOVSStatus()
	if err != nil {
		return r.fail("OVS 基础网络", err)
	}
	if data, marshalErr := json.Marshal(ovsStatus); marshalErr == nil {
		_ = json.Unmarshal(data, &r.report.OVSStatus)
	}
	if !ovsStatus.Healthy {
		return r.fail("OVS 基础网络", fmt.Errorf("OVS 基础网络检查未通过: %s", strings.Join(ovsStatus.Issues, "；")))
	}
	r.pass("OVS 基础网络", "OVS 服务、网桥、网关、DHCP、转发与 NAT 规则均正常", map[string]string{
		"bridge":  ovsStatus.Bridge,
		"gateway": ovsStatus.GatewayIP,
		"uplink":  ovsStatus.Uplink,
	})
	if err := r.checkInterrupted(); err != nil {
		return r.fail("用户中断", err)
	}

	r.progress("准备系统基础交换机和测试安全组...")
	if err := rootservice.EnsureSystemBaseNetwork(); err != nil {
		return r.fail("基础交换机准备", err)
	}
	var systemSwitch model.VPCSwitch
	if err := model.DB.Where("is_system = ?", true).Order("id ASC").First(&systemSwitch).Error; err != nil {
		return r.fail("基础交换机准备", fmt.Errorf("读取系统基础交换机失败: %w", err))
	}
	systemBridge := rootservice.BridgeNameForSwitch(systemSwitch)
	configuredBridge := strings.TrimSpace(config.GlobalConfig.OVSBridge)
	if configuredBridge == "" || systemBridge != configuredBridge {
		return r.fail("基础交换机准备", fmt.Errorf("系统基础交换机网桥与 OVS 配置不一致: switch=%s config=%s", systemBridge, configuredBridge))
	}
	adminUsername := strings.TrimSpace(config.GlobalConfig.DefaultAdminUser)
	if adminUsername == "" {
		adminUsername = "admin"
	}
	securityGroup, err := rootservice.EnsureDefaultSecurityGroup(adminUsername)
	if err != nil {
		return r.fail("基础交换机准备", err)
	}
	r.pass("基础交换机准备", "系统基础交换机和默认安全组已就绪", map[string]string{
		"switch_id":         strconv.FormatUint(uint64(systemSwitch.ID), 10),
		"security_group_id": strconv.FormatUint(uint64(securityGroup.ID), 10),
		"bridge":            systemBridge,
	})
	if err := r.checkInterrupted(); err != nil {
		return r.fail("用户中断", err)
	}

	params := r.buildCreateParams(hostArch, systemSwitch.ID, securityGroup.ID)
	r.expectedDisk = filepath.Join(config.GlobalConfig.CloneDir, r.vmName+".qcow2")
	r.report.VM = vmParametersFromCreateParams(params, systemBridge)

	r.progress("通过面板正式链路创建并启动测试虚拟机...")
	diskPath, err := rootservice.CreateVM(params, func(percent int, message string) {
		r.progress(fmt.Sprintf("[%d%%] %s", percent, message))
	})
	if err != nil {
		return r.fail("虚拟机创建与启动", err)
	}
	r.created = true
	r.diskPath = diskPath
	r.report.VM.StoragePath = diskPath
	r.report.VM.StoragePool = params.StoragePoolID
	if err := r.checkInterrupted(); err != nil {
		return r.fail("用户中断", err)
	}
	if err := rootservice.BindVMToVPCAsAdmin(params.Name, params.SwitchID, params.SecurityGroupID); err != nil {
		return r.fail("虚拟机创建与启动", fmt.Errorf("虚拟机已启动，但绑定基础 OVS 网络失败: %w", err))
	}
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ? AND interface_order = ?", params.Name, 0).First(&binding).Error; err != nil {
		return r.fail("虚拟机创建与启动", fmt.Errorf("读取测试虚拟机 VPC 绑定失败: %w", err))
	}
	if binding.SwitchID != params.SwitchID || binding.SecurityGroupID != params.SecurityGroupID {
		return r.fail("虚拟机创建与启动", fmt.Errorf(
			"测试虚拟机 VPC 绑定与创建参数不一致: switch=%d/%d security_group=%d/%d",
			binding.SwitchID, params.SwitchID, binding.SecurityGroupID, params.SecurityGroupID,
		))
	}
	r.pass("虚拟机创建与启动", "测试虚拟机已通过正式创建链路启动并完成 VPC 绑定", map[string]string{
		"disk_path": diskPath,
	})
	if err := r.checkInterrupted(); err != nil {
		return r.fail("用户中断", err)
	}

	r.progress("验证虚拟机状态、XML 与 OVS 运行端口...")
	if err := r.verifyVM(systemBridge); err != nil {
		return r.fail("虚拟机与 OVS 联合验证", err)
	}
	r.pass("虚拟机与 OVS 联合验证", "虚拟机处于运行态，XML 与运行端口均已接入基础 OVS 网桥", nil)
	return nil
}

func vmParametersFromCreateParams(params *rootservice.CreateVMParams, ovsBridge string) VMParameters {
	return VMParameters{
		Name:         params.Name,
		Arch:         params.Arch,
		VirtType:     params.VirtType,
		MachineType:  params.MachineType,
		BootType:     params.BootType,
		VCPU:         params.VCPU,
		RAMGB:        params.RAM,
		DiskGB:       params.DiskSize,
		DiskFormat:   params.DiskFormat,
		DiskBus:      params.DiskBus,
		NICModel:     params.NicModel,
		VideoModel:   params.VideoModel,
		SwitchID:     params.SwitchID,
		SecurityID:   params.SecurityGroupID,
		OVSBridge:    ovsBridge,
		StoragePool:  params.StoragePoolID,
		NestedVirt:   params.NestedVirt != nil && *params.NestedVirt,
		GuestAgent:   params.GuestAgent != nil && params.GuestAgent.Enabled,
		PCIERootPort: params.PCIERootPorts,
	}
}

func (r *runner) checkRequiredRuntime() error {
	info, err := os.Stat("/dev/kvm")
	if err != nil {
		return fmt.Errorf("读取 /dev/kvm 失败: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("/dev/kvm 不是有效的 KVM 设备")
	}
	kvmDevice, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("打开 /dev/kvm 失败: %w", err)
	}
	_ = kvmDevice.Close()

	qemuCandidates := []string{arch.GetProfile(arch.DetectHostArch()).EmulatorPath(), "qemu-system-x86_64", "qemu-kvm", "/usr/libexec/qemu-kvm"}
	if arch.DetectHostArch() == arch.ArchAarch64 {
		qemuCandidates = []string{arch.GetProfile(arch.ArchAarch64).EmulatorPath(), "qemu-system-aarch64", "qemu-kvm", "/usr/libexec/qemu-kvm"}
	}
	commands := []string{"virsh", "virt-install", "qemu-img", "ovs-vsctl", "ip", "iptables", "dnsmasq"}
	for _, command := range commands {
		if _, err := exec.LookPath(command); err != nil {
			return fmt.Errorf("未找到必要命令 %s", command)
		}
	}
	qemuBinary := ""
	for _, command := range qemuCandidates {
		if executableAvailable(command) {
			qemuBinary = command
			break
		}
	}
	if qemuBinary == "" {
		return fmt.Errorf("未找到必要的 QEMU 命令（已检查 %s）", strings.Join(qemuCandidates, "、"))
	}
	r.report.Extra["qemu_binary"] = qemuBinary
	if result := utils.ExecCommandQuiet("virsh", "uri"); result.Error != nil {
		return fmt.Errorf("libvirt system 连接检查失败: %s", firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	return nil
}

func executableAvailable(command string) bool {
	if strings.ContainsRune(command, os.PathSeparator) {
		info, err := os.Stat(command)
		return err == nil && !info.IsDir() && info.Mode().Perm()&0111 != 0
	}
	_, err := exec.LookPath(command)
	return err == nil
}

func (r *runner) buildCreateParams(hostArch string, switchID, securityGroupID uint) *rootservice.CreateVMParams {
	profile := arch.GetProfile(hostArch)
	apicEnabled := profile.SupportsAPIC()
	paeEnabled := profile.SupportsPAE()
	spiceEnabled := false
	nestedVirt := true
	machineType := profile.DefaultMachineType()
	bootType := profile.DefaultBootType()
	videoModel := vm_xml.ResolveVMVideoModel("", "linux", hostArch)
	pcieRootPorts := 0
	if machineType == "q35" {
		pcieRootPorts = vm_xml.DefaultCreatePCIERootPorts
	}

	return &rootservice.CreateVMParams{
		Name:            r.vmName,
		Remark:          "QVMConsole 安装兼容性测试临时虚拟机",
		VCPU:            r.options.VCPU,
		RAM:             r.options.RAMGB,
		DiskSize:        r.options.DiskGB,
		DiskFormat:      "qcow2",
		DiskBus:         "virtio",
		NicModel:        "virtio",
		APIC:            &apicEnabled,
		PAE:             &paeEnabled,
		RTCOffset:       "utc",
		RTCStartDate:    "now",
		GuestAgent:      &vm_xml.VMGuestAgentConfig{Enabled: true},
		SMBIOS1:         &vm_xml.VMSMBIOS1Config{},
		OSType:          "linux",
		MachineType:     machineType,
		BootType:        bootType,
		Watchdog:        "none",
		BootOrder:       []string{"hd"},
		VideoModel:      videoModel,
		SpiceEnabled:    &spiceEnabled,
		CPUTopologyMode: "auto",
		VirtType:        "kvm",
		Arch:            hostArch,
		SwitchID:        switchID,
		SecurityGroupID: securityGroupID,
		IsAdmin:         true,
		PCIERootPorts:   pcieRootPorts,
		NestedVirt:      &nestedVirt,
	}
}

func (r *runner) verifyVM(expectedBridge string) error {
	stateResult := utils.ExecCommand("virsh", "domstate", r.vmName)
	if stateResult.Error != nil {
		return fmt.Errorf("读取测试虚拟机状态失败: %s", firstNonEmpty(stateResult.Stderr, stateResult.Error.Error()))
	}
	if strings.TrimSpace(strings.ToLower(stateResult.Stdout)) != "running" {
		return fmt.Errorf("测试虚拟机未保持运行状态，当前状态: %s", strings.TrimSpace(stateResult.Stdout))
	}

	inactiveResult := utils.ExecCommand("virsh", "dumpxml", r.vmName, "--inactive")
	if inactiveResult.Error != nil {
		return fmt.Errorf("读取测试虚拟机持久化 XML 失败: %s", firstNonEmpty(inactiveResult.Stderr, inactiveResult.Error.Error()))
	}
	r.xmlContent = inactiveResult.Stdout
	if err := r.saveXML(r.xmlContent, false); err != nil {
		return err
	}
	if err := r.verifyFormalCreationXML(r.xmlContent); err != nil {
		return fmt.Errorf("面板正式创建参数 XML 验证失败: %w", err)
	}

	expectedBridge = strings.TrimSpace(expectedBridge)
	if expectedBridge == "" {
		expectedBridge = strings.TrimSpace(config.GlobalConfig.OVSBridge)
	}
	if _, err := findOVSInterface(r.xmlContent, expectedBridge, false); err != nil {
		return fmt.Errorf("持久化 XML 验证失败: %w", err)
	}

	activeResult := utils.ExecCommand("virsh", "dumpxml", r.vmName)
	if activeResult.Error != nil {
		return fmt.Errorf("读取测试虚拟机运行态 XML 失败: %s", firstNonEmpty(activeResult.Stderr, activeResult.Error.Error()))
	}
	r.activeXML = activeResult.Stdout
	if err := r.saveXML(r.activeXML, true); err != nil {
		return err
	}
	iface, err := findOVSInterface(r.activeXML, expectedBridge, true)
	if err != nil {
		return fmt.Errorf("运行态 XML 验证失败: %w", err)
	}
	target := strings.TrimSpace(iface.Target.Dev)
	bridgeResult := utils.ExecCommand("ovs-vsctl", "iface-to-br", target)
	if bridgeResult.Error != nil {
		return fmt.Errorf("OVS 端口 %s 未接入网桥: %s", target, firstNonEmpty(bridgeResult.Stderr, bridgeResult.Error.Error()))
	}
	if strings.TrimSpace(bridgeResult.Stdout) != expectedBridge {
		return fmt.Errorf("OVS 端口 %s 接入了错误网桥 %s", target, strings.TrimSpace(bridgeResult.Stdout))
	}
	ofportResult := utils.ExecCommand("ovs-vsctl", "get", "Interface", target, "ofport")
	if ofportResult.Error != nil {
		return fmt.Errorf("读取 OVS 端口 %s 的 ofport 失败: %s", target, firstNonEmpty(ofportResult.Stderr, ofportResult.Error.Error()))
	}
	ofport := strings.Trim(strings.TrimSpace(ofportResult.Stdout), "\"")
	if ofport == "" || ofport == "-1" || ofport == "[]" {
		return fmt.Errorf("OVS 端口 %s 的 ofport 无效: %s", target, ofport)
	}
	r.report.Extra["vnet_port"] = target
	r.report.Extra["ofport"] = ofport
	return nil
}

func (r *runner) verifyFormalCreationXML(xmlContent string) error {
	var domain domainXML
	if err := xml.Unmarshal([]byte(xmlContent), &domain); err != nil {
		return fmt.Errorf("解析持久化 XML 失败: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(domain.Type)) != r.report.VM.VirtType {
		return fmt.Errorf("虚拟化类型不一致: xml=%s expected=%s", domain.Type, r.report.VM.VirtType)
	}
	if actual := vm_xml.ParseVMArchFromDomainXML(xmlContent); actual != r.report.VM.Arch {
		return fmt.Errorf("目标架构不一致: xml=%s expected=%s", actual, r.report.VM.Arch)
	}
	if actual := vm_xml.ParseVMMachineTypeFromDomainXML(xmlContent); actual != r.report.VM.MachineType {
		return fmt.Errorf("机器类型不一致: xml=%s expected=%s", actual, r.report.VM.MachineType)
	}
	if actual := vm_xml.ParseVMBootTypeFromDomainXML(xmlContent); actual != r.report.VM.BootType {
		return fmt.Errorf("引导固件不一致: xml=%s expected=%s", actual, r.report.VM.BootType)
	}
	if actual := vm_xml.ParseVMVideoModelFromDomainXML(xmlContent); actual != r.report.VM.VideoModel {
		return fmt.Errorf("显示设备不一致: xml=%s expected=%s", actual, r.report.VM.VideoModel)
	}
	if guestAgent := vm_xml.ParseVMGuestAgentConfigFromDomainXML(xmlContent); guestAgent == nil || !guestAgent.Enabled {
		return fmt.Errorf("Guest Agent 通道未按创建参数写入")
	}
	if strings.ToLower(strings.TrimSpace(domain.Clock.Offset)) != "utc" {
		return fmt.Errorf("RTC 偏移不一致: xml=%s expected=utc", domain.Clock.Offset)
	}
	diskMatched := false
	for _, disk := range domain.Devices.Disks {
		if disk.Device == "disk" && disk.Driver.Type == r.report.VM.DiskFormat && disk.Target.Bus == r.report.VM.DiskBus {
			diskMatched = true
			break
		}
	}
	if !diskMatched {
		return fmt.Errorf("未找到 format=%s、bus=%s 的系统盘", r.report.VM.DiskFormat, r.report.VM.DiskBus)
	}

	if r.report.VM.Arch == arch.ArchX8664 {
		if domain.Features.APIC == nil || domain.Features.PAE == nil {
			return fmt.Errorf("x86_64 XML 缺少 APIC 或 PAE 特性")
		}
		pcieRootPorts := 0
		for _, controller := range domain.Devices.Controllers {
			if controller.Model == "pcie-root-port" {
				pcieRootPorts++
			}
		}
		if pcieRootPorts < r.report.VM.PCIERootPort {
			return fmt.Errorf("PCIe root port 数量不足: xml=%d expected=%d", pcieRootPorts, r.report.VM.PCIERootPort)
		}
		if nestedFeature := vm_xml.DetectHostNestedVirtFeatureName(); nestedFeature != "" {
			nestedMatched := false
			for _, feature := range domain.CPU.Features {
				if feature.Name == nestedFeature && feature.Policy == "require" {
					nestedMatched = true
					break
				}
			}
			if !nestedMatched {
				return fmt.Errorf("嵌套虚拟化特性 %s 未按创建参数写入", nestedFeature)
			}
		}
	} else if r.report.VM.Arch == arch.ArchAarch64 && (domain.Features.APIC != nil || domain.Features.PAE != nil) {
		return fmt.Errorf("aarch64 XML 不应包含 APIC 或 PAE 特性")
	}
	return nil
}

func findOVSInterface(xmlContent, expectedBridge string, requireTarget bool) (*domainInterface, error) {
	var domain domainXML
	if err := xml.Unmarshal([]byte(xmlContent), &domain); err != nil {
		return nil, fmt.Errorf("解析虚拟机 XML 失败: %w", err)
	}
	for index := range domain.Devices.Interfaces {
		iface := &domain.Devices.Interfaces[index]
		if iface.Source.Bridge != expectedBridge || iface.VirtualPort.Type != "openvswitch" {
			continue
		}
		if iface.Model.Type != "virtio" {
			return nil, fmt.Errorf("测试网卡型号不符合面板参数，当前型号: %s", iface.Model.Type)
		}
		if requireTarget && strings.TrimSpace(iface.Target.Dev) == "" {
			return nil, fmt.Errorf("测试网卡缺少运行态 vnet 目标端口")
		}
		return iface, nil
	}
	return nil, fmt.Errorf("虚拟机 XML 中未找到连接 %s 且类型为 openvswitch 的网卡", expectedBridge)
}

func (r *runner) saveXML(xmlContent string, active bool) error {
	if strings.TrimSpace(xmlContent) == "" {
		return nil
	}
	suffix := ".xml"
	if active {
		suffix = "-active.xml"
	}
	path := filepath.Join(r.options.ReportDir, r.vmName+suffix)
	if err := os.WriteFile(path, []byte(xmlContent), 0600); err != nil {
		return fmt.Errorf("保存测试虚拟机 XML 失败: %w", err)
	}
	if active {
		r.report.ActiveXMLPath = path
	} else {
		r.report.XMLPath = path
	}
	return nil
}

func (r *runner) captureFailureDiagnostics() {
	if r.xmlContent == "" {
		if result := utils.ExecCommandQuiet("virsh", "dumpxml", r.vmName, "--inactive"); result.Error == nil {
			r.xmlContent = result.Stdout
			_ = r.saveXML(r.xmlContent, false)
		}
	}
	if r.activeXML == "" {
		if result := utils.ExecCommandQuiet("virsh", "dumpxml", r.vmName); result.Error == nil {
			r.activeXML = result.Stdout
			_ = r.saveXML(r.activeXML, true)
		}
	}

	logPath := filepath.Join(r.options.ReportDir, r.vmName+"-diagnostics.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer file.Close()

	_, _ = fmt.Fprintf(file, "QVMConsole 系统兼容性诊断\n虚拟机: %s\n时间: %s\n\n", r.vmName, time.Now().Format(time.RFC3339))
	qemuLog := filepath.Join("/var/log/libvirt/qemu", r.vmName+".log")
	if source, openErr := os.Open(qemuLog); openErr == nil {
		_, _ = fmt.Fprintf(file, "===== %s =====\n", qemuLog)
		_, _ = io.Copy(file, source)
		_ = source.Close()
		_, _ = fmt.Fprintln(file)
	}

	units := []string{"libvirtd", "virtqemud", "openvswitch-switch", "openvswitch", "kvm-console-ovs-dnsmasq"}
	for _, unit := range units {
		result := utils.ExecCommandQuiet("journalctl", "-u", unit, "-n", "120", "--no-pager")
		if strings.TrimSpace(result.Stdout) == "" && strings.TrimSpace(result.Stderr) == "" {
			continue
		}
		_, _ = fmt.Fprintf(file, "===== journalctl -u %s =====\n%s\n%s\n", unit, result.Stdout, result.Stderr)
	}
	r.report.DiagnosticLog = logPath
}

func (r *runner) cleanup() error {
	r.progress("清理兼容性测试临时资源...")
	var firstErr error
	domainPresent := r.created || utils.ExecCommandQuiet("virsh", "dominfo", r.vmName).Error == nil
	if domainPresent {
		state := utils.ExecCommandQuiet("virsh", "domstate", r.vmName)
		stateName := strings.TrimSpace(strings.ToLower(state.Stdout))
		if state.Error == nil && stateName != "shut off" && stateName != "关闭" {
			if destroy := utils.ExecCommandQuiet("virsh", "destroy", r.vmName); destroy.Error != nil {
				firstErr = fmt.Errorf("停止测试虚拟机失败: %s", firstNonEmpty(destroy.Stderr, destroy.Error.Error()))
			}
		}

		undefine := utils.ExecCommandQuiet("virsh", "undefine", r.vmName, "--nvram", "--snapshots-metadata")
		if undefine.Error != nil {
			fallback := utils.ExecCommandQuiet("virsh", "undefine", r.vmName, "--snapshots-metadata")
			if fallback.Error != nil && firstErr == nil {
				firstErr = fmt.Errorf("取消测试虚拟机定义失败: %s", firstNonEmpty(fallback.Stderr, fallback.Error.Error()))
			}
		}
	}

	rootservice.CleanupVMVPCBinding(r.vmName)
	rootservice.DeleteVMRuntimeRecord(r.vmName)
	rootservice.DeleteVMStatsRecords(r.vmName)
	if err := rootservice.DeleteVMCredential(r.vmName); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("清理测试虚拟机凭据失败: %w", err)
	}
	if err := rootservice.DeleteVMSchedules(r.vmName); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("清理测试虚拟机定时任务失败: %w", err)
	}
	nvramPath := vm_xml.GetVMNVRAMPath(r.vmName)
	if parsedPath := strings.TrimSpace(vm_xml.ExtractDomainNVRAMPath(r.xmlContent)); parsedPath != "" {
		nvramPath = parsedPath
	}
	for _, path := range []string{r.diskPath, r.expectedDisk, nvramPath} {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = fmt.Errorf("删除测试资源 %s 失败: %w", path, err)
		}
	}

	domainExists := utils.ExecCommandQuiet("virsh", "dominfo", r.vmName).Error == nil
	diskExists := false
	for _, path := range []string{r.diskPath, r.expectedDisk} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			diskExists = true
		}
	}
	nvramExists := false
	if strings.TrimSpace(nvramPath) != "" {
		_, statErr := os.Stat(nvramPath)
		nvramExists = statErr == nil
	}
	var bindingCount, statsCount, credentialCount, scheduleCount int64
	if model.DB != nil {
		model.DB.Model(&model.VPCVMBinding{}).Where("vm_name = ?", r.vmName).Count(&bindingCount)
		model.DB.Model(&model.VmStatsRecord{}).Where("vm_name = ?", r.vmName).Count(&statsCount)
		model.DB.Model(&model.VMCredential{}).Where("vm_name = ?", r.vmName).Count(&credentialCount)
		model.DB.Model(&model.VMSchedule{}).Where("vm_name = ?", r.vmName).Count(&scheduleCount)
	}
	if domainExists || diskExists || nvramExists || bindingCount > 0 || statsCount > 0 || credentialCount > 0 || scheduleCount > 0 {
		return r.fail("临时资源清理", fmt.Errorf(
			"测试资源清理不完整: domain=%t disk=%t nvram=%t vpc_bindings=%d stats=%d credentials=%d schedules=%d",
			domainExists, diskExists, nvramExists, bindingCount, statsCount, credentialCount, scheduleCount,
		))
	}
	if firstErr != nil {
		return r.fail("临时资源清理", firstErr)
	}
	r.pass("临时资源清理", "测试虚拟机、磁盘、NVRAM、域内存元数据、运行记录和 VPC 绑定均已清理", nil)
	return nil
}

func (r *runner) pass(name, message string, details map[string]string) {
	r.report.Stages = append(r.report.Stages, StageResult{
		Name: name, Status: stagePassed, Message: message, Details: details, CheckedAt: time.Now(),
	})
}

func (r *runner) fail(name string, err error) error {
	r.report.Stages = append(r.report.Stages, StageResult{
		Name: name, Status: stageFailed, Message: err.Error(), CheckedAt: time.Now(),
	})
	return fmt.Errorf("%s失败: %w", name, err)
}

func (r *runner) checkInterrupted() error {
	select {
	case <-r.options.Context.Done():
		return r.options.Context.Err()
	default:
		return nil
	}
}

func (r *runner) writeReport() error {
	data, err := json.MarshalIndent(r.report, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化兼容性报告失败: %w", err)
	}
	if err := os.WriteFile(r.report.ReportPath, data, 0600); err != nil {
		return fmt.Errorf("写入兼容性报告失败: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "未知错误"
}
