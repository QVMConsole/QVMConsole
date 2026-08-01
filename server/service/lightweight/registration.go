package lightweight

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/arch"
	clonepkg "kvm_console/service/clone"
	"kvm_console/service/storage/disk"
	"kvm_console/service/vm_xml"
)

const (
	LightweightVMRegistrationStatusPending      = "pending"
	LightweightVMRegistrationStatusProvisioning = "provisioning"
	LightweightVMRegistrationStatusActive       = "active"
	LightweightVMRegistrationStatusFailed       = "failed"
)

var lightweightVMNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func createJSONText(value interface{}) (string, error) {
	if value == nil {
		return "", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseJSONText(raw string, value interface{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), value)
}

func NormalizeVMNicModel(value string) string {
	var normalized string
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "e1000e":
		normalized = "e1000e"
	case "rtl8139":
		normalized = "rtl8139"
	default:
		normalized = "virtio"
	}
	// ARM 架构只支持 virtio 网卡，降级 e1000e/rtl8139
	if arch.DetectHostArch() == arch.ArchAarch64 {
		switch normalized {
		case "e1000e", "rtl8139":
			return "virtio"
		}
	}
	return normalized
}

func validateLightweightVMRegistrationUser(user model.User) error {
	if user.Role != "user" {
		return fmt.Errorf("只能为普通用户注册轻量云服务器")
	}
	if !IsLightweightCloudType(user.CloudType) {
		return fmt.Errorf("当前用户不是轻量云用户")
	}
	// 如果用户没有配置专用VPC，跳过VPC检查（适用于选择已有VM的场景）
	if user.DedicatedVPCSwitchID == 0 {
		return nil
	}
	var count int64
	if err := model.DB.Model(&model.VPCSwitch{}).
		Where("id = ? AND (bridge_mode = '' OR bridge_mode = ? OR bridge_mode IS NULL)", user.DedicatedVPCSwitchID, BridgeModeNAT).
		Count(&count).Error; err != nil {
		return fmt.Errorf("检查专用 VPC 网络失败: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("轻量云专用 VPC 必须是有效的 NAT VPC")
	}
	return nil
}

func normalizeLightweightVMRegistrationRequest(user model.User, req LightweightVMRegistrationRequest, createdBy string) (*model.LightweightVMRegistration, error) {
	req.VMName = strings.TrimSpace(req.VMName)
	req.Template = strings.TrimSpace(req.Template)
	req.TemplateType = strings.ToLower(strings.TrimSpace(req.TemplateType))
	req.Hostname = strings.TrimSpace(req.Hostname)
	req.RTCOffset = strings.TrimSpace(req.RTCOffset)
	req.RTCStartDate = strings.TrimSpace(req.RTCStartDate)
	req.VideoModel = strings.TrimSpace(req.VideoModel)
	req.CPUTopologyMode = HookNormalizeVMCPUTopologyMode(req.CPUTopologyMode)
	req.CPULimitPercent = HookNormalizeVMCPULimitPercent(req.CPULimitPercent)
	req.FirstBootRebootMode = HookNormalizeVMFirstBootRebootMode(req.FirstBootRebootMode)
	req.DiskBus = disk.NormalizeVMDiskBus(req.DiskBus)
	req.NicModel = NormalizeVMNicModel(req.NicModel)
	req.StoragePoolID = strings.TrimSpace(req.StoragePoolID)
	if req.VMName == "" {
		return nil, fmt.Errorf("虚拟机名称不能为空")
	}
	if !lightweightVMNameRegexp.MatchString(req.VMName) {
		return nil, fmt.Errorf("虚拟机名称只能包含字母和数字")
	}
	if req.Template == "" {
		return nil, fmt.Errorf("请选择模板")
	}
	if req.VCPU <= 0 {
		return nil, fmt.Errorf("CPU 核心数必须大于 0")
	}
	if req.RAM <= 0 {
		return nil, fmt.Errorf("内存必须大于 0")
	}
	if err := HookValidateVMCPULimitPercent(req.CPULimitPercent); err != nil {
		return nil, err
	}
	if req.Hostname == "" {
		req.Hostname = HookGenerateRandomCloneHostname()
	}
	meta := HookGetTemplateMeta(req.Template)
	if req.TemplateType == "" {
		req.TemplateType = meta.Type
	}
	req.TemplateType = strings.ToLower(strings.TrimSpace(req.TemplateType))
	if req.TemplateType == "" {
		req.TemplateType = "linux"
	}
	if err := HookValidateCloneCredentialsForTemplate(req.TemplateType, req.Hostname, HookNormalizeCloneUsernameForTemplate(req.TemplateType, "admin"), "TempAa12345!", false); err != nil {
		return nil, err
	}
	if req.DiskSize < 0 {
		req.DiskSize = 0
	}
	req.TrafficDownGB = NormalizeLightweightVMQuotaRequest(LightweightVMQuotaRequest{TrafficDownGB: req.TrafficDownGB}).TrafficDownGB
	req.TrafficUpGB = NormalizeLightweightVMQuotaRequest(LightweightVMQuotaRequest{TrafficUpGB: req.TrafficUpGB}).TrafficUpGB
	if req.BandwidthDownMbps < 0 {
		req.BandwidthDownMbps = 0
	}
	if req.BandwidthUpMbps < 0 {
		req.BandwidthUpMbps = 0
	}
	if req.MaxPortForwards < 0 {
		req.MaxPortForwards = 0
	}
	if req.MaxSnapshots < 0 {
		req.MaxSnapshots = 0
	}
	if req.MaxRuntimeHours < 0 {
		req.MaxRuntimeHours = 0
	}
	guestAgentJSON, err := createJSONText(req.GuestAgent)
	if err != nil {
		return nil, fmt.Errorf("序列化 Guest Agent 配置失败: %w", err)
	}
	smbiosJSON, err := createJSONText(req.SMBIOS1)
	if err != nil {
		return nil, fmt.Errorf("序列化 SMBIOS 配置失败: %w", err)
	}
	memoryJSON, err := createJSONText(req.MemoryDynamic)
	if err != nil {
		return nil, fmt.Errorf("序列化动态内存配置失败: %w", err)
	}
	if strings.TrimSpace(req.FnOSDeviceID) != "" {
		if _, _, err := clonepkg.NormalizeFnOSDeviceID(req.FnOSDeviceID); err != nil {
			return nil, err
		}
		req.PreserveFnOSDeviceID = true
	}
	return &model.LightweightVMRegistration{
		Username:             user.Username,
		VMName:               req.VMName,
		Template:             req.Template,
		TemplateType:         req.TemplateType,
		CloneMode:            req.CloneMode,
		VCPU:                 req.VCPU,
		RAM:                  req.RAM,
		DiskSize:             req.DiskSize,
		DiskBus:              req.DiskBus,
		Hostname:             req.Hostname,
		Autostart:            req.Autostart,
		Freeze:               req.Freeze,
		APIC:                 req.APIC,
		PAE:                  req.PAE,
		RTCOffset:            req.RTCOffset,
		RTCStartDate:         req.RTCStartDate,
		GuestAgentJSON:       guestAgentJSON,
		SMBIOS1JSON:          smbiosJSON,
		VideoModel:           req.VideoModel,
		CPUTopologyMode:      req.CPUTopologyMode,
		CPULimitPercent:      req.CPULimitPercent,
		CPUAffinity:          req.CPUAffinity,
		FirstBootRebootMode:  req.FirstBootRebootMode,
		MemoryDynamicJSON:    memoryJSON,
		NicModel:             req.NicModel,
		StoragePoolID:        req.StoragePoolID,
		PreserveFnOSDeviceID: req.PreserveFnOSDeviceID,
		FnOSDeviceID:         strings.TrimSpace(req.FnOSDeviceID),
		SwitchID:             user.DedicatedVPCSwitchID,
		TrafficDownGB:        req.TrafficDownGB,
		TrafficUpGB:          req.TrafficUpGB,
		BandwidthDownMbps:    req.BandwidthDownMbps,
		BandwidthUpMbps:      req.BandwidthUpMbps,
		MaxPortForwards:      req.MaxPortForwards,
		MaxSnapshots:         req.MaxSnapshots,
		MaxRuntimeHours:      req.MaxRuntimeHours,
		Status:               LightweightVMRegistrationStatusPending,
		CreatedBy:            strings.TrimSpace(createdBy),
	}, nil
}

func CreateLightweightVMRegistrations(username string, reqs []LightweightVMRegistrationRequest, createdBy string) ([]LightweightVMRegistrationView, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(reqs) == 0 {
		return nil, nil
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}
	if err := validateLightweightVMRegistrationUser(user); err != nil {
		return nil, err
	}
	created := make([]model.LightweightVMRegistration, 0, len(reqs))
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, raw := range reqs {
			reg, err := normalizeLightweightVMRegistrationRequest(user, raw, createdBy)
			if err != nil {
				return err
			}
			if err := tx.Create(reg).Error; err != nil {
				return fmt.Errorf("创建轻量云 VM 注册失败: %w", err)
			}
			created = append(created, *reg)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	views := make([]LightweightVMRegistrationView, 0, len(created))
	for i := range created {
		views = append(views, BuildLightweightVMRegistrationView(created[i]))
	}
	return views, nil
}

func BuildLightweightVMRegistrationView(reg model.LightweightVMRegistration) LightweightVMRegistrationView {
	view := LightweightVMRegistrationView{
		ID:                   reg.ID,
		Username:             reg.Username,
		VMName:               reg.VMName,
		Template:             reg.Template,
		TemplateType:         reg.TemplateType,
		VCPU:                 reg.VCPU,
		RAM:                  reg.RAM,
		DiskSize:             reg.DiskSize,
		DiskBus:              disk.NormalizeVMDiskBus(reg.DiskBus),
		Hostname:             reg.Hostname,
		Autostart:            reg.Autostart,
		Freeze:               reg.Freeze,
		APIC:                 HookResolveVMAPICEnabled(reg.APIC),
		PAE:                  vm_xml.ResolveVMPAEEnabled(reg.PAE),
		RTCOffset:            reg.RTCOffset,
		RTCStartDate:         reg.RTCStartDate,
		VideoModel:           reg.VideoModel,
		CPUTopologyMode:      HookNormalizeVMCPUTopologyMode(reg.CPUTopologyMode),
		CPULimitPercent:      HookNormalizeVMCPULimitPercent(reg.CPULimitPercent),
		CPUAffinity:          reg.CPUAffinity,
		FirstBootRebootMode:  HookNormalizeVMFirstBootRebootMode(reg.FirstBootRebootMode),
		NicModel:             NormalizeVMNicModel(reg.NicModel),
		StoragePoolID:        reg.StoragePoolID,
		PreserveFnOSDeviceID: reg.PreserveFnOSDeviceID,
		FnOSDeviceID:         reg.FnOSDeviceID,
		SwitchID:             reg.SwitchID,
		TrafficDownGB:        reg.TrafficDownGB,
		TrafficUpGB:          reg.TrafficUpGB,
		BandwidthDownMbps:    reg.BandwidthDownMbps,
		BandwidthUpMbps:      reg.BandwidthUpMbps,
		MaxPortForwards:      reg.MaxPortForwards,
		MaxSnapshots:         reg.MaxSnapshots,
		MaxRuntimeHours:      reg.MaxRuntimeHours,
		Status:               reg.Status,
		TaskID:               reg.TaskID,
		ErrorMessage:         reg.ErrorMessage,
		CreatedBy:            reg.CreatedBy,
		CreatedAt:            reg.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:            reg.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if reg.ConfirmedAt != nil {
		view.ConfirmedAt = reg.ConfirmedAt.Format("2006-01-02 15:04:05")
	}
	if reg.SwitchID > 0 {
		var sw model.VPCSwitch
		if err := model.DB.First(&sw, reg.SwitchID).Error; err == nil {
			view.SwitchName = sw.Name
			view.SwitchCIDR = sw.CIDR
		}
	}
	return view
}

func ListLightweightVMRegistrations(username string, includeActive bool) ([]LightweightVMRegistrationView, error) {
	username = strings.TrimSpace(username)
	query := model.DB.Model(&model.LightweightVMRegistration{}).Order("id ASC")
	if username != "" {
		query = query.Where("username = ?", username)
	}
	if !includeActive {
		query = query.Where("status <> ?", LightweightVMRegistrationStatusActive)
	}
	var regs []model.LightweightVMRegistration
	if err := query.Find(&regs).Error; err != nil {
		return nil, err
	}
	views := make([]LightweightVMRegistrationView, 0, len(regs))
	for _, reg := range regs {
		views = append(views, BuildLightweightVMRegistrationView(reg))
	}
	return views, nil
}

func DeleteLightweightVMRegistration(username string, id uint) error {
	var reg model.LightweightVMRegistration
	if err := model.DB.Where("id = ? AND username = ?", id, strings.TrimSpace(username)).First(&reg).Error; err != nil {
		return fmt.Errorf("注册记录不存在")
	}
	if reg.Status == LightweightVMRegistrationStatusProvisioning || reg.Status == LightweightVMRegistrationStatusActive {
		return fmt.Errorf("当前注册记录已进入开通流程，不能删除")
	}
	return model.DB.Delete(&reg).Error
}

// ValidateLightweightVMRemoval 校验轻量云 VM 是否可移除。
func ValidateLightweightVMRemoval(username string, vmName string) error {
	username = strings.TrimSpace(username)
	vmName = strings.TrimSpace(vmName)
	if username == "" {
		return fmt.Errorf("用户不能为空")
	}
	if vmName == "" {
		return fmt.Errorf("虚拟机名称不能为空")
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}
	if err := validateLightweightVMRegistrationUser(user); err != nil {
		return err
	}

	var provisioningCount int64
	if err := model.DB.Model(&model.LightweightVMRegistration{}).
		Where("username = ? AND vm_name = ? AND status = ?", username, vmName, LightweightVMRegistrationStatusProvisioning).
		Count(&provisioningCount).Error; err != nil {
		return err
	}
	if provisioningCount > 0 {
		return fmt.Errorf("当前 VM 正在开通中，暂不能移除")
	}

	var quotaCount int64
	if err := model.DB.Model(&model.LightweightVMQuota{}).Where("username = ? AND vm_name = ?", username, vmName).Count(&quotaCount).Error; err != nil {
		return err
	}
	var regCount int64
	if err := model.DB.Model(&model.LightweightVMRegistration{}).Where("username = ? AND vm_name = ?", username, vmName).Count(&regCount).Error; err != nil {
		return err
	}
	if quotaCount == 0 && regCount == 0 {
		return fmt.Errorf("轻量云 VM 记录不存在")
	}
	return nil
}

// RemoveLightweightVMRegistrationByVMName 将已开通 VM 从注册 VM 列表中移除，不删除虚拟机本体。
func RemoveLightweightVMRegistrationByVMName(username string, vmName string) error {
	username = strings.TrimSpace(username)
	vmName = strings.TrimSpace(vmName)
	if err := ValidateLightweightVMRemoval(username, vmName); err != nil {
		return err
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("username = ? AND vm_name = ?", username, vmName).Delete(&model.LightweightVMRegistration{}).Error; err != nil {
			return err
		}
		if err := tx.Where("username = ? AND vm_name = ?", username, vmName).Delete(&model.LightweightVMQuota{}).Error; err != nil {
			return err
		}
		if err := tx.Where("username = ? AND vm_name = ?", username, vmName).Delete(&model.LightweightVMTrafficMonthly{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 从用户的VM访问列表中移除该VM
	if err := HookRemoveVMFromUser(username, vmName); err != nil {
		// 记录错误但不阻止操作完成
		logger.App.Warn("移除轻量云VM后从用户访问列表移除失败", "vm", vmName, "user", username, "error", err)
	}

	return nil
}

func UpdateLightweightVMQuotaByVMName(username string, req LightweightVMQuotaRequest) (*model.LightweightVMQuota, *LightweightVMRegistrationView, error) {
	username = strings.TrimSpace(username)
	req = NormalizeLightweightVMQuotaRequest(req)
	if username == "" {
		return nil, nil, fmt.Errorf("用户不能为空")
	}
	if req.VMName == "" {
		return nil, nil, fmt.Errorf("虚拟机名称不能为空")
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, nil, fmt.Errorf("用户不存在: %w", err)
	}
	if err := validateLightweightVMRegistrationUser(user); err != nil {
		return nil, nil, err
	}

	var quota *model.LightweightVMQuota
	var regView *LightweightVMRegistrationView
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var reg model.LightweightVMRegistration
		regErr := tx.Where("username = ? AND vm_name = ?", username, req.VMName).First(&reg).Error
		if regErr == nil {
			if reg.Status == LightweightVMRegistrationStatusProvisioning {
				return fmt.Errorf("当前 VM 正在开通中，暂不能修改配额")
			}
			reg.TrafficDownGB = req.TrafficDownGB
			reg.TrafficUpGB = req.TrafficUpGB
			reg.BandwidthDownMbps = req.BandwidthDownMbps
			reg.BandwidthUpMbps = req.BandwidthUpMbps
			reg.MaxPortForwards = req.MaxPortForwards
			reg.MaxSnapshots = req.MaxSnapshots
			reg.MaxRuntimeHours = req.MaxRuntimeHours
			if err := tx.Save(&reg).Error; err != nil {
				return fmt.Errorf("更新轻量云 VM 注册配额失败: %w", err)
			}
			view := BuildLightweightVMRegistrationView(reg)
			regView = &view
		} else if regErr != gorm.ErrRecordNotFound {
			return regErr
		}

		var existingQuota model.LightweightVMQuota
		quotaErr := tx.Where("username = ? AND vm_name = ?", username, req.VMName).First(&existingQuota).Error
		if quotaErr == nil {
			existingQuota.TrafficDownGB = req.TrafficDownGB
			existingQuota.TrafficUpGB = req.TrafficUpGB
			existingQuota.BandwidthDownMbps = req.BandwidthDownMbps
			existingQuota.BandwidthUpMbps = req.BandwidthUpMbps
			existingQuota.MaxPortForwards = req.MaxPortForwards
			existingQuota.MaxSnapshots = req.MaxSnapshots
			existingQuota.MaxRuntimeHours = req.MaxRuntimeHours
			if err := tx.Save(&existingQuota).Error; err != nil {
				return fmt.Errorf("更新轻量云 VM 运行配额失败: %w", err)
			}
			quota = FillLightweightVMQuotaRuntime(&existingQuota)
			return nil
		}
		if quotaErr != gorm.ErrRecordNotFound {
			return quotaErr
		}
		if regView == nil {
			return fmt.Errorf("未找到该轻量云 VM 注册或运行配额")
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if quota != nil {
		CheckLightweightVMTrafficAfterQuotaUpdate(req.VMName)
		SyncLightweightVMRuntimeQuotaState(req.VMName, time.Now())
		if refreshed, err := GetLightweightVMQuota(req.VMName); err == nil {
			quota = refreshed
		}
		HookRefreshVMCacheByNameAsync(req.VMName)
	}
	return quota, regView, nil
}

func FormatLightweightVMRegistrationList(regs []LightweightVMRegistrationView) string {
	if len(regs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, reg := range regs {
		fmt.Fprintf(&b, "%d. %s\n", i+1, reg.VMName)
		fmt.Fprintf(&b, "   模板：%s（%s）\n", reg.Template, displayTemplateType(reg.TemplateType))
		fmt.Fprintf(&b, "   规格：%d 核 / %d GB 内存 / %d GB 磁盘\n", reg.VCPU, reg.RAM, reg.DiskSize)
		fmt.Fprintf(&b, "   网络：%s%s\n", emptyToDash(reg.SwitchName), switchCIDRSuffix(reg.SwitchCIDR))
		fmt.Fprintf(&b, "   流量：下行 %s / 上行 %s\n", quotaGBText(reg.TrafficDownGB), quotaGBText(reg.TrafficUpGB))
		fmt.Fprintf(&b, "   带宽：下行 %s / 上行 %s\n", quotaMbpsText(reg.BandwidthDownMbps), quotaMbpsText(reg.BandwidthUpMbps))
		fmt.Fprintf(&b, "   端口转发上限：%s\n", quotaCountText(reg.MaxPortForwards))
		fmt.Fprintf(&b, "   快照上限：%s\n", quotaCountText(reg.MaxSnapshots))
		fmt.Fprintf(&b, "   运行时长配额：%s\n", quotaHoursText(reg.MaxRuntimeHours))
	}
	return b.String()
}

func displayTemplateType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "windows":
		return "Windows"
	case "fnos":
		return "fnOS"
	case "other":
		return "其他"
	default:
		return "Linux"
	}
}

func emptyToDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return strings.TrimSpace(value)
}

func switchCIDRSuffix(cidr string) string {
	if strings.TrimSpace(cidr) == "" {
		return ""
	}
	return " / " + strings.TrimSpace(cidr)
}

func quotaGBText(value float64) string {
	if value <= 0 {
		return "不限"
	}
	return fmt.Sprintf("%.2f GB/月", value)
}

func quotaHoursText(value int) string {
	if value <= 0 {
		return "不限"
	}
	return fmt.Sprintf("%d 小时", value)
}

func quotaMbpsText(value int) string {
	if value <= 0 {
		return "不限"
	}
	return fmt.Sprintf("%d Mbps", value)
}

func quotaCountText(value int) string {
	if value <= 0 {
		return "不限"
	}
	return fmt.Sprintf("%d", value)
}
