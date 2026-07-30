package vm

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/appliance"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

const (
	ExportFormatQCOW2 = "qcow2"
	ExportFormatOVA   = "ova"
)

// ExportVMParams 导出虚拟机参数。
type ExportVMParams struct {
	VMName      string   `json:"vm_name"`
	Username    string   `json:"username"`
	Format      string   `json:"format,omitempty"`
	DiskDevices []string `json:"disk_devices,omitempty"`
}

// ExportVMResult 导出结果。
type ExportVMResult struct {
	VMName   string `json:"vm_name"`
	Format   string `json:"format"`
	FileName string `json:"file_name"`
	FilePath string `json:"file_path"`
	FileSize string `json:"file_size"`
}

// VMExportDiskOption 是导出弹窗展示的磁盘选项。
type VMExportDiskOption struct {
	Device        string `json:"device"`
	Format        string `json:"format"`
	Bus           string `json:"bus"`
	CapacityBytes int64  `json:"capacity_bytes"`
	ActualBytes   int64  `json:"actual_bytes"`
	IsSystem      bool   `json:"is_system"`
	Supported     bool   `json:"supported"`
	Reason        string `json:"reason,omitempty"`
	path          string
}

// VMExportOptions 是导出前可读取的虚拟机状态与磁盘列表。
type VMExportOptions struct {
	VMName string               `json:"vm_name"`
	Status string               `json:"status"`
	Disks  []VMExportDiskOption `json:"disks"`
}

type exportDomainXML struct {
	Name   string `xml:"name"`
	VCPU   int    `xml:"vcpu"`
	Memory struct {
		Value int64  `xml:",chardata"`
		Unit  string `xml:"unit,attr"`
	} `xml:"memory"`
	OS struct {
		Type struct {
			Arch    string `xml:"arch,attr"`
			Machine string `xml:"machine,attr"`
		} `xml:"type"`
		Loader string `xml:"loader"`
	} `xml:"os"`
}

// GetVMExportOptions 返回所有本地持久磁盘，供 OVA 导出选择。
func GetVMExportOptions(vmName string) (*VMExportOptions, error) {
	if !DomainExists(vmName) {
		return nil, fmt.Errorf("虚拟机 '%s' 不存在", vmName)
	}
	disks, err := D.ListDisks(vmName)
	if err != nil {
		return nil, err
	}
	options := &VMExportOptions{VMName: vmName, Status: strings.ToLower(strings.TrimSpace(GetDomainState(vmName)))}
	for _, disk := range disks {
		if disk.DeviceType != "disk" || disk.Path == "" {
			continue
		}
		capacityGB, _ := strconv.ParseFloat(strings.TrimSpace(disk.CapacityGB), 64)
		option := VMExportDiskOption{
			Device:        disk.Device,
			Format:        disk.Format,
			Bus:           disk.Bus,
			CapacityBytes: int64(capacityGB * 1024 * 1024 * 1024),
			IsSystem:      disk.IsSystem,
			Supported:     true,
			path:          disk.Path,
		}
		if info, statErr := os.Stat(disk.Path); statErr == nil {
			option.ActualBytes = info.Size()
			if !info.Mode().IsRegular() && info.Mode()&os.ModeDevice == 0 {
				option.Supported = false
				option.Reason = "当前存储源不是本地文件或块设备"
			}
		} else {
			option.Supported = false
			option.Reason = "磁盘源文件不存在"
		}
		options.Disks = append(options.Disks, option)
	}
	if len(options.Disks) == 0 {
		return nil, fmt.Errorf("虚拟机没有可导出的持久磁盘")
	}
	return options, nil
}

// GetVMExportSize 预估默认系统盘导出文件大小。
func GetVMExportSize(vmName string) (int64, error) {
	options, err := GetVMExportOptions(vmName)
	if err != nil {
		return 0, err
	}
	for _, disk := range options.Disks {
		if disk.IsSystem {
			if disk.ActualBytes > 0 {
				return disk.ActualBytes * 110 / 100, nil
			}
			return disk.CapacityBytes, nil
		}
	}
	return 0, fmt.Errorf("没有可导出的系统盘")
}

// ExportVM 根据 format 导出 QCOW2 磁盘或标准 OVA 虚拟机包。
func ExportVM(ctx context.Context, params *ExportVMParams, progressFn func(int, string)) (*ExportVMResult, error) {
	format := strings.ToLower(strings.TrimSpace(params.Format))
	if format == "" {
		format = ExportFormatQCOW2
	}
	switch format {
	case ExportFormatQCOW2:
		return exportVMQCOW2(ctx, params, progressFn)
	case ExportFormatOVA:
		return exportVMOVA(ctx, params, progressFn)
	default:
		return nil, fmt.Errorf("导出格式仅支持 qcow2 或 ova")
	}
}

func exportVMQCOW2(ctx context.Context, params *ExportVMParams, progressFn func(int, string)) (*ExportVMResult, error) {
	diskInfo := GetVMDiskInfo(params.VMName)
	if diskInfo.Path == "" {
		return nil, fmt.Errorf("获取虚拟机 %s 的系统盘路径失败", params.VMName)
	}
	progressFn(10, "获取系统盘信息...")
	timestamp := time.Now().Format("20060102-150405")
	fileName := fmt.Sprintf("%s-export-%s.qcow2", params.VMName, timestamp)
	exportDir := D.GetUserDiskDir(params.Username)
	exportPath := filepath.Join(exportDir, fileName)
	if err := os.MkdirAll(exportDir, 0o775); err != nil {
		return nil, fmt.Errorf("创建导出目录失败: %w", err)
	}
	useSudo := prepareExportUser(params.Username)
	if err := checkExportCanceled(ctx); err != nil {
		return nil, err
	}

	backingResult := utils.ExecShell(fmt.Sprintf("qemu-img info -U %s 2>/dev/null | grep 'backing file:'", utils.ShellSingleQuote(diskInfo.Path)))
	hasBacking := backingResult.Error == nil && strings.TrimSpace(backingResult.Stdout) != ""
	if hasBacking {
		progressFn(20, "检测到链式磁盘，正在展平并导出...")
		args := []string{"qemu-img", "convert", "-O", "qcow2", diskInfo.Path, exportPath}
		if useSudo {
			args = append([]string{"sudo", "-u", params.Username}, args...)
		}
		result := utils.ExecCommandContextWithTimeout(ctx, args[0], 2*time.Hour, args[1:]...)
		if result.Error != nil {
			_ = os.Remove(exportPath)
			return nil, fmt.Errorf("展平系统盘失败: %s", strings.TrimSpace(result.Stderr))
		}
	} else {
		progressFn(20, "正在复制系统盘文件...")
		args := []string{"cp", "--sparse=always", diskInfo.Path, exportPath}
		if useSudo {
			args = append([]string{"sudo", "-u", params.Username}, args...)
		}
		result := utils.ExecCommandContextWithTimeout(ctx, args[0], 2*time.Hour, args[1:]...)
		if result.Error != nil {
			_ = os.Remove(exportPath)
			return nil, fmt.Errorf("复制系统盘失败: %s", strings.TrimSpace(result.Stderr))
		}
	}
	if err := checkExportCanceled(ctx); err != nil {
		_ = os.Remove(exportPath)
		return nil, err
	}
	_ = utils.ChownLibvirtQEMU(exportPath)
	return finishExport(params.VMName, ExportFormatQCOW2, fileName, exportPath, progressFn), nil
}

func exportVMOVA(ctx context.Context, params *ExportVMParams, progressFn func(int, string)) (*ExportVMResult, error) {
	options, err := GetVMExportOptions(params.VMName)
	if err != nil {
		return nil, err
	}
	if options.Status != "shut off" && options.Status != "shutoff" {
		return nil, fmt.Errorf("标准 OVA 导出要求虚拟机先关机，当前状态: %s", options.Status)
	}
	selected, err := selectExportDisks(options.Disks, params.DiskDevices)
	if err != nil {
		return nil, err
	}
	progressFn(5, "已确认虚拟机配置和导出磁盘...")

	tempBase := config.GlobalConfig.ApplianceTempDir
	if tempBase == "" {
		tempBase = filepath.Join(os.TempDir(), "kvm_console", "appliance")
	}
	if err := os.MkdirAll(tempBase, 0o755); err != nil {
		return nil, fmt.Errorf("创建 OVA 临时目录失败: %w", err)
	}
	if err := checkOVAExportTempSpace(tempBase, selected); err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp(tempBase, "export-")
	if err != nil {
		return nil, fmt.Errorf("创建 OVA 工作目录失败: %w", err)
	}
	defer os.RemoveAll(workDir)

	exportDisks := make([]appliance.ExportDisk, 0, len(selected))
	for i, disk := range selected {
		if err := checkExportCanceled(ctx); err != nil {
			return nil, err
		}
		progress := 10 + i*55/len(selected)
		progressFn(progress, fmt.Sprintf("正在转换磁盘 %s（%d/%d）...", disk.Device, i+1, len(selected)))
		fileName := fmt.Sprintf("disk-%02d-%s.vmdk", i+1, disk.Device)
		target := filepath.Join(workDir, fileName)
		result := utils.ExecCommandContextWithTimeout(ctx, "qemu-img", 4*time.Hour,
			"convert", "-O", "vmdk", "-o", "subformat=streamOptimized", disk.path, target)
		if result.Error != nil {
			return nil, fmt.Errorf("转换磁盘 %s 为 VMDK 失败: %s", disk.Device, strings.TrimSpace(result.Stderr))
		}
		info, statErr := os.Stat(target)
		if statErr != nil {
			return nil, fmt.Errorf("读取转换后磁盘 %s 失败: %w", disk.Device, statErr)
		}
		exportDisks = append(exportDisks, appliance.ExportDisk{
			ID: disk.Device, FileName: fileName, FilePath: target,
			CapacityBytes: disk.CapacityBytes, FileSize: info.Size(), Bus: disk.Bus,
		})
	}

	progressFn(70, "正在生成 OVF 描述和完整性清单...")
	configData, err := buildApplianceExportConfig(params.VMName, exportDisks)
	if err != nil {
		return nil, err
	}
	ovf, err := appliance.BuildOVF(configData)
	if err != nil {
		return nil, err
	}
	descriptorName := params.VMName + ".ovf"
	tempOVA := filepath.Join(workDir, params.VMName+".ova")
	progressFn(76, "正在封装 OVA 文件...")
	if err := appliance.CreateOVA(ctx, tempOVA, descriptorName, ovf, exportDisks); err != nil {
		return nil, fmt.Errorf("封装 OVA 失败: %w", err)
	}
	if err := checkExportCanceled(ctx); err != nil {
		return nil, err
	}

	timestamp := time.Now().Format("20060102-150405")
	fileName := fmt.Sprintf("%s-export-%s.ova", params.VMName, timestamp)
	exportDir := D.GetUserDiskDir(params.Username)
	if err := os.MkdirAll(exportDir, 0o775); err != nil {
		return nil, fmt.Errorf("创建导出目录失败: %w", err)
	}
	finalPath := filepath.Join(exportDir, fileName)
	partialPath := finalPath + ".partial"
	defer os.Remove(partialPath)
	progressFn(90, "正在写入我的存储并校验配额...")
	writeUser, err := prepareOVAExportUser(params.Username, workDir, tempOVA)
	if err != nil {
		return nil, err
	}
	args := []string{"cp", tempOVA, partialPath}
	if writeUser != "" {
		args = append([]string{"sudo", "-u", writeUser}, args...)
	}
	copyResult := utils.ExecCommandContextWithTimeout(ctx, args[0], 4*time.Hour, args[1:]...)
	if copyResult.Error != nil {
		return nil, fmt.Errorf("写入我的存储失败，请检查存储配额: %s", strings.TrimSpace(copyResult.Stderr))
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		return nil, fmt.Errorf("完成 OVA 文件落盘失败: %w", err)
	}
	logger.App.Info("OVA 导出完成", "vm", params.VMName, "disks", len(selected), "file", finalPath)
	return finishExport(params.VMName, ExportFormatOVA, fileName, finalPath, progressFn), nil
}

func checkOVAExportTempSpace(tempDir string, disks []VMExportDiskOption) error {
	var sourceBytes int64
	for _, disk := range disks {
		if disk.ActualBytes > 0 {
			sourceBytes += disk.ActualBytes
		} else {
			sourceBytes += disk.CapacityBytes
		}
	}
	_, _, availableKB, err := utils.GetDiskSpace(tempDir)
	if err != nil {
		if runtime.GOOS == "linux" {
			return fmt.Errorf("检查 OVA 临时空间失败: %w", err)
		}
		return nil
	}
	required := sourceBytes*220/100 + 64*1024*1024
	if availableKB*1024 < required {
		return fmt.Errorf("OVA 临时空间不足，需要至少 %d MB，当前可用 %d MB", required/1024/1024, availableKB/1024)
	}
	return nil
}

func selectExportDisks(all []VMExportDiskOption, devices []string) ([]VMExportDiskOption, error) {
	requested := map[string]bool{}
	for _, device := range devices {
		if value := strings.TrimSpace(device); value != "" {
			requested[value] = true
		}
	}
	known := map[string]bool{}
	for _, disk := range all {
		known[disk.Device] = true
	}
	for device := range requested {
		if !known[device] {
			return nil, fmt.Errorf("请求导出的磁盘设备不存在: %s", device)
		}
	}
	var selected []VMExportDiskOption
	systemSelected := false
	for _, disk := range all {
		want := len(requested) == 0 || requested[disk.Device] || disk.IsSystem
		if !want {
			continue
		}
		if !disk.Supported {
			if disk.IsSystem || requested[disk.Device] {
				return nil, fmt.Errorf("磁盘 %s 当前不可导出: %s", disk.Device, disk.Reason)
			}
			continue
		}
		selected = append(selected, disk)
		systemSelected = systemSelected || disk.IsSystem
	}
	if !systemSelected {
		return nil, fmt.Errorf("OVA 导出需要包含系统盘")
	}
	return selected, nil
}

func buildApplianceExportConfig(vmName string, disks []appliance.ExportDisk) (appliance.ExportConfig, error) {
	xmlText, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return appliance.ExportConfig{}, fmt.Errorf("读取虚拟机 XML 失败: %w", err)
	}
	var domain exportDomainXML
	if err := xml.Unmarshal([]byte(xmlText), &domain); err != nil {
		return appliance.ExportConfig{}, fmt.Errorf("解析虚拟机 XML 失败: %w", err)
	}
	ramBytes := memoryToBytes(domain.Memory.Value, domain.Memory.Unit)
	networks := []appliance.Network{}
	for i, nic := range libvirt_rpc.ParseInterfacesFromDomainXML(xmlText) {
		networks = append(networks, appliance.Network{Name: fmt.Sprintf("network-%d", i+1), Model: nic.Model})
	}
	bootType := "bios"
	if strings.TrimSpace(domain.OS.Loader) != "" {
		bootType = "uefi"
	}
	return appliance.ExportConfig{
		Name: vmName, Architecture: domain.OS.Type.Arch, VCPU: domain.VCPU,
		RAMMB: int(ramBytes / 1024 / 1024), BootType: bootType,
		MachineType: domain.OS.Type.Machine, OSType: DetectVMOSType("", xmlText),
		Disks: disks, Networks: networks,
	}, nil
}

func memoryToBytes(value int64, unit string) int64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "b", "bytes":
		return value
	case "mb", "mib":
		return value * 1024 * 1024
	case "gb", "gib":
		return value * 1024 * 1024 * 1024
	default:
		return value * 1024
	}
}

func prepareExportUser(username string) bool {
	if username == "" {
		return false
	}
	check := utils.ExecCommandQuiet("id", username)
	if check.ExitCode != 0 {
		return false
	}
	utils.ExecCommand("usermod", "-aG", "kvm", username)
	return true
}

func prepareOVAExportUser(username, workDir, sourcePath string) (string, error) {
	writeUser := ""
	if username != "" {
		if _, _, err := utils.GetUserIDs(username, ""); err == nil {
			writeUser = username
		}
	}
	if writeUser == "" {
		qemuUser, err := utils.ResolveLibvirtQEMUUser()
		if err != nil {
			if runtime.GOOS != "linux" {
				return "", nil
			}
			return "", fmt.Errorf("准备我的存储写入身份失败: %w", err)
		}
		writeUser = qemuUser
	}
	if result := utils.ExecCommand("chown", writeUser, workDir, sourcePath); result.Error != nil {
		return "", fmt.Errorf("设置 OVA 临时文件用户权限失败: %s", strings.TrimSpace(result.Stderr))
	}
	if err := os.Chmod(workDir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(sourcePath, 0o600); err != nil {
		return "", err
	}
	return writeUser, nil
}

func checkExportCanceled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return taskqueue.ErrTaskCanceled
	default:
		return nil
	}
}

func finishExport(vmName, format, fileName, path string, progressFn func(int, string)) *ExportVMResult {
	sizeResult := utils.ExecShell(fmt.Sprintf("du -h %s | awk '{print $1}'", utils.ShellSingleQuote(path)))
	fileSize := "未知"
	if sizeResult.Error == nil {
		fileSize = strings.TrimSpace(sizeResult.Stdout)
	}
	progressFn(100, fmt.Sprintf("导出完成，文件大小: %s", fileSize))
	return &ExportVMResult{VMName: vmName, Format: format, FileName: fileName, FilePath: path, FileSize: fileSize}
}
