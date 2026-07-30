package vmimport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"kvm_console/config"
	"kvm_console/service"
	"kvm_console/service/appliance"
	"kvm_console/service/arch"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

// ImportApplianceParams 是 OVF/OVA 虚拟机包导入任务参数。
// 嵌入普通磁盘导入配置，使现有硬件、网络与高级设置保持一致。
type ImportApplianceParams struct {
	ImportDiskByPathParams
	ApplianceFile string `json:"appliance_file,omitempty"`
	AppliancePath string `json:"appliance_path,omitempty"`
	SourceType    string `json:"source_type,omitempty"` // storage/path
	ConfigMode    string `json:"config_mode,omitempty"` // ovf/custom
	CopySource    bool   `json:"copy_source,omitempty"`
	IsAdmin       bool   `json:"is_admin,omitempty"`
}

// ImportApplianceResult 是虚拟机包导入结果。
type ImportApplianceResult struct {
	VMName           string   `json:"vm_name"`
	DiskPaths        []string `json:"disk_paths"`
	ImportedDisks    int      `json:"imported_disks"`
	StartAfterImport bool     `json:"start_after_import"`
}

// ParseImportApplianceParams 解析任务参数并兼容旧任务默认启动行为。
func ParseImportApplianceParams(jsonText string) (*ImportApplianceParams, error) {
	var params ImportApplianceParams
	if err := json.Unmarshal([]byte(jsonText), &params); err != nil {
		return nil, err
	}
	if !strings.Contains(jsonText, `"start_after_import"`) {
		params.StartAfterImport = true
	}
	params.ConfigMode = NormalizeApplianceConfigMode(params.ConfigMode)
	if params.ConfigMode == "" {
		return nil, fmt.Errorf("虚拟机包配置方式仅支持 ovf 或 custom")
	}
	return &params, nil
}

// NormalizeApplianceConfigMode 标准化虚拟机包配置方式，空值按旧版自定义行为处理。
func NormalizeApplianceConfigMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "custom":
		return "custom"
	case "ovf":
		return "ovf"
	default:
		return ""
	}
}

// ResolveApplianceSourcePath 根据管理员路径或我的存储文件名解析源路径。
func ResolveApplianceSourcePath(params *ImportApplianceParams) (string, error) {
	if params == nil {
		return "", fmt.Errorf("虚拟机包参数为空")
	}
	if params.SourceType == "storage" || (params.AppliancePath == "" && params.ApplianceFile != "") {
		name := strings.TrimSpace(params.ApplianceFile)
		if name == "" || filepath.Base(name) != name || strings.Contains(name, "..") {
			return "", fmt.Errorf("虚拟机包文件名不安全")
		}
		if params.Username == "" {
			return "", fmt.Errorf("从我的存储导入时缺少用户名")
		}
		return filepath.Join(service.GetUserDiskDir(params.Username), name), nil
	}
	path := filepath.Clean(strings.TrimSpace(params.AppliancePath))
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("虚拟机包路径需要使用绝对路径")
	}
	return path, nil
}

// InspectAppliance 检查虚拟机包元数据。
func InspectAppliance(params *ImportApplianceParams) (*appliance.Metadata, error) {
	path, err := ResolveApplianceSourcePath(params)
	if err != nil {
		return nil, err
	}
	if err := checkApplianceTempSpace(path, config.GlobalConfig.ApplianceTempDir); err != nil {
		return nil, err
	}
	return appliance.InspectSourceIn(path, config.GlobalConfig.ApplianceTempDir)
}

// ImportAppliance 导入 OVF/OVA 中的全部磁盘并创建虚拟机，启动操作由任务处理器在网络绑定后执行。
func ImportAppliance(ctx context.Context, params *ImportApplianceParams, progressFn func(int, string)) (*ImportApplianceResult, error) {
	if err := service.ValidateVMName(params.Name); err != nil {
		return nil, err
	}
	sourcePath, err := ResolveApplianceSourcePath(params)
	if err != nil {
		return nil, err
	}
	if err := checkApplianceTempSpace(sourcePath, config.GlobalConfig.ApplianceTempDir); err != nil {
		return nil, err
	}
	progressFn(3, "正在校验并解包虚拟机包...")
	resolved, err := appliance.ResolveSource(ctx, sourcePath, config.GlobalConfig.ApplianceTempDir)
	if err != nil {
		if ctx.Err() != nil {
			return nil, taskqueue.ErrTaskCanceled
		}
		return nil, err
	}
	defer resolved.Cleanup()
	if len(resolved.DiskPaths) == 0 {
		return nil, fmt.Errorf("虚拟机包中没有可导入磁盘")
	}

	hostArch := arch.DetectHostArch()
	packageArch := strings.TrimSpace(resolved.Metadata.Architecture)
	if packageArch != "" && packageArch != hostArch {
		return nil, fmt.Errorf("虚拟机包架构 %s 与宿主机架构 %s 不一致", packageArch, hostArch)
	}
	if params.ConfigMode == "ovf" {
		if err := applyApplianceOVFConfig(params, resolved.Metadata); err != nil {
			return nil, err
		}
	}
	if !params.IsAdmin {
		var capacityBytes int64
		for _, disk := range resolved.Metadata.Disks {
			capacityBytes += disk.CapacityBytes
		}
		diskGB := int((capacityBytes + 1024*1024*1024 - 1) / (1024 * 1024 * 1024))
		if err := service.CheckQuota(params.Username, params.VCPU, params.RAM, diskGB); err != nil {
			return nil, fmt.Errorf("任务执行前配额检查失败: %w", err)
		}
	}
	if err := checkApplianceCanceled(ctx); err != nil {
		return nil, err
	}

	requestedStart := params.StartAfterImport
	firstParams := params.ImportDiskByPathParams
	firstParams.DiskPath = resolved.DiskPaths[0]
	firstParams.DiskFile = ""
	firstParams.DiskSourceType = "path"
	firstParams.CopyDisk = true
	firstParams.SystemDiskBus = firstNonEmptyImport(resolved.Metadata.Disks[0].Bus, "virtio")
	firstParams.ExtraImportDisks = nil
	firstParams.StartAfterImport = false
	firstParams.Autostart = false
	firstParams.trustedApplianceSource = true
	if strings.EqualFold(firstParams.InitType, "windows") || strings.EqualFold(resolved.Metadata.OSType, "windows") {
		firstParams.InitType = "windows"
	} else {
		// 虚拟机包导入只恢复设备，不修改来宾系统身份或凭据。
		firstParams.InitType = "appliance"
	}
	if firstParams.BootType == "" {
		firstParams.BootType = resolved.Metadata.BootType
	}
	if firstParams.MachineType == "" {
		firstParams.MachineType = resolved.Metadata.MachineType
	}
	if firstParams.NicModel == "" && len(resolved.Metadata.Networks) > 0 {
		firstParams.NicModel = resolved.Metadata.Networks[0].Model
	}

	mainResult, err := ImportDiskByPath(ctx, &firstParams, func(value int, message string) {
		progressFn(10+value*45/100, message)
	})
	if err != nil {
		return nil, err
	}
	createdPaths := []string{mainResult.DiskPath}
	rollback := true
	defer func() {
		if rollback {
			rollbackImportedAppliance(params.Name, createdPaths)
		}
	}()

	for i := 1; i < len(resolved.DiskPaths); i++ {
		if err := checkApplianceCanceled(ctx); err != nil {
			return nil, err
		}
		metaDisk := resolved.Metadata.Disks[i]
		base := 55 + (i-1)*35/maxInt(1, len(resolved.DiskPaths)-1)
		progressFn(base, fmt.Sprintf("正在导入数据盘 %d/%d...", i, len(resolved.DiskPaths)-1))
		device, err := importSingleDiskToVM(ctx, params.Name, &ExtraImportDiskEntry{
			DiskPath:      resolved.DiskPaths[i],
			CopyDisk:      true,
			Bus:           firstNonEmptyImport(metaDisk.Bus, "scsi"),
			StoragePoolID: params.StoragePoolID,
		}, params.Username, func(_ int, _ string) {})
		if err != nil {
			return nil, fmt.Errorf("导入数据盘 %s 失败: %w", metaDisk.FileRef, err)
		}
		if path := service.GetDiskFilePath(params.Name, device); path != "" {
			createdPaths = append(createdPaths, path)
		}
	}

	if params.Autostart {
		result := utils.ExecCommand("virsh", "autostart", params.Name)
		if result.Error != nil {
			return nil, fmt.Errorf("设置开机自启失败: %s", result.Stderr)
		}
	}
	rollback = false
	progressFn(92, "虚拟机和全部磁盘已创建，正在应用网络设置...")
	return &ImportApplianceResult{
		VMName: params.Name, DiskPaths: createdPaths, ImportedDisks: len(createdPaths), StartAfterImport: requestedStart,
	}, nil
}

// applyApplianceOVFConfig 将包内可识别硬件配置应用到任务参数，本机网络仅复用映射关系并重新生成 MAC。
func applyApplianceOVFConfig(params *ImportApplianceParams, metadata *appliance.Metadata) error {
	if metadata == nil {
		return fmt.Errorf("虚拟机包缺少硬件描述")
	}
	if metadata.VCPU > 0 {
		params.VCPU = metadata.VCPU
		params.MaxVCPU = 0
	}
	if metadata.RAM > 0 {
		params.RAM = metadata.RAM
		params.MemoryDynamic = nil
	}
	params.BootType = strings.TrimSpace(metadata.BootType)
	params.MachineType = strings.TrimSpace(metadata.MachineType)
	if len(metadata.Networks) == 0 {
		params.SwitchID = 0
		params.SecurityGroupID = 0
		params.ExtraNics = nil
		return nil
	}
	if params.SwitchID == 0 || params.SecurityGroupID == 0 {
		switchID, securityGroupID, err := service.ResolveVPCForVMCreate(params.Username, params.SwitchID, params.SecurityGroupID)
		if err != nil {
			return fmt.Errorf("映射虚拟机包网络失败: %w", err)
		}
		params.SwitchID = switchID
		params.SecurityGroupID = securityGroupID
	}
	params.NicModel = firstNonEmptyImport(metadata.Networks[0].Model, "virtio")
	params.ExtraNics = make([]service.AddVMInterfaceRequest, 0, len(metadata.Networks)-1)
	for _, network := range metadata.Networks[1:] {
		params.ExtraNics = append(params.ExtraNics, service.AddVMInterfaceRequest{
			SwitchID:        params.SwitchID,
			SecurityGroupID: params.SecurityGroupID,
			NicModel:        firstNonEmptyImport(network.Model, params.NicModel),
		})
	}
	return nil
}

func checkApplianceTempSpace(sourcePath, tempDir string) error {
	if !strings.EqualFold(filepath.Ext(sourcePath), ".ova") {
		return nil
	}
	if tempDir == "" {
		tempDir = filepath.Join(os.TempDir(), "kvm_console", "appliance")
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("创建虚拟机包临时目录失败: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("读取虚拟机包大小失败: %w", err)
	}
	_, _, availableKB, spaceErr := utils.GetDiskSpace(tempDir)
	if spaceErr != nil {
		if runtime.GOOS == "linux" {
			return fmt.Errorf("检查虚拟机包临时空间失败: %w", spaceErr)
		}
		return nil
	}
	required := info.Size()*110/100 + 64*1024*1024
	if availableKB*1024 < required {
		return fmt.Errorf("虚拟机包临时空间不足，需要至少 %d MB，当前可用 %d MB", required/1024/1024, availableKB/1024)
	}
	return nil
}

// RemoveApplianceSource 在任务完整成功后按用户选择删除源包。
func RemoveApplianceSource(params *ImportApplianceParams) error {
	if params.CopySource {
		return nil
	}
	sourcePath, err := ResolveApplianceSourcePath(params)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(sourcePath), ".ova") {
		return os.Remove(sourcePath)
	}
	resolved, err := appliance.ResolveSource(context.Background(), sourcePath, config.GlobalConfig.ApplianceTempDir)
	if err != nil {
		return err
	}
	defer resolved.Cleanup()
	for _, path := range resolved.SourceFiles {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func rollbackImportedAppliance(vmName string, paths []string) {
	utils.ExecCommandQuiet("virsh", "destroy", vmName)
	utils.ExecCommandQuiet("virsh", "undefine", vmName, "--nvram")
	for _, path := range paths {
		if path != "" {
			_ = os.Remove(path)
		}
	}
}

// RollbackApplianceImport 供任务处理器在网络或启动阶段失败时回滚已创建资源。
func RollbackApplianceImport(vmName string, paths []string) {
	rollbackImportedAppliance(vmName, paths)
}

func checkApplianceCanceled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return taskqueue.ErrTaskCanceled
	default:
		return nil
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmptyImport(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeImportDiskBus(bus string) string {
	switch strings.ToLower(strings.TrimSpace(bus)) {
	case "scsi", "sata", "ide":
		return strings.ToLower(strings.TrimSpace(bus))
	default:
		return "virtio"
	}
}

func importDiskTargetDevice(bus string) string {
	switch normalizeImportDiskBus(bus) {
	case "ide":
		return "hda"
	case "scsi", "sata":
		return "sda"
	default:
		return "vda"
	}
}
