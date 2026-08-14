package lightweight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kvm_console/model"
	clonepkg "kvm_console/service/clone"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

func BuildLightweightVMProvisionParams(regID uint, username string, credential LightweightVMConfirmRequest) (*LightweightVMProvisionParams, error) {
	var reg model.LightweightVMRegistration
	if err := model.DB.Where("id = ? AND username = ?", regID, strings.TrimSpace(username)).First(&reg).Error; err != nil {
		return nil, fmt.Errorf("待开通服务器不存在")
	}
	if reg.Status != LightweightVMRegistrationStatusPending && reg.Status != LightweightVMRegistrationStatusFailed {
		return nil, fmt.Errorf("当前服务器状态不允许确认开通")
	}
	credential.Username = HookNormalizeCloneUsernameForTemplate(reg.TemplateType, credential.Username)
	if err := HookValidateCloneCredentialsForTemplate(reg.TemplateType, reg.Hostname, credential.Username, credential.Password, true); err != nil {
		return nil, err
	}
	return &LightweightVMProvisionParams{
		RegistrationID:     reg.ID,
		Username:           reg.Username,
		CredentialUsername: credential.Username,
		CredentialPassword: credential.Password,
		Operator:           strings.TrimSpace(username),
	}, nil
}

func MarkLightweightVMRegistrationTask(regID uint, taskID uint) {
	model.DB.Model(&model.LightweightVMRegistration{}).Where("id = ?", regID).Updates(map[string]interface{}{
		"task_id": taskID,
		"status":  LightweightVMRegistrationStatusProvisioning,
	})
}

func ParseLightweightVMProvisionParams(jsonStr string) (*LightweightVMProvisionParams, error) {
	var params LightweightVMProvisionParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	return &params, nil
}

func ProvisionLightweightVMRegistration(ctx context.Context, params *LightweightVMProvisionParams, progressFn func(int, string)) (*clonepkg.CloneResult, error) {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}
	var reg model.LightweightVMRegistration
	if err := model.DB.First(&reg, params.RegistrationID).Error; err != nil {
		return nil, fmt.Errorf("待开通服务器不存在")
	}
	if reg.Username != strings.TrimSpace(params.Username) {
		return nil, fmt.Errorf("开通用户与注册记录不一致")
	}
	if reg.Status != LightweightVMRegistrationStatusPending && reg.Status != LightweightVMRegistrationStatusFailed && reg.Status != LightweightVMRegistrationStatusProvisioning {
		return nil, fmt.Errorf("当前服务器状态不允许开通")
	}
	var user model.User
	if err := model.DB.Where("username = ?", reg.Username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}
	if err := validateLightweightVMRegistrationUser(user); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := model.DB.Model(&reg).Updates(map[string]interface{}{
		"status":        LightweightVMRegistrationStatusProvisioning,
		"error_message": "",
		"confirmed_at":  &now,
	}).Error; err != nil {
		return nil, err
	}
	fail := func(err error) (*clonepkg.CloneResult, error) {
		model.DB.Model(&model.LightweightVMRegistration{}).Where("id = ?", reg.ID).Updates(map[string]interface{}{
			"status":        LightweightVMRegistrationStatusFailed,
			"error_message": err.Error(),
		})
		return nil, err
	}
	var guestAgent *vm_xml.VMGuestAgentConfig
	if strings.TrimSpace(reg.GuestAgentJSON) != "" {
		guestAgent = &vm_xml.VMGuestAgentConfig{}
		if err := parseJSONText(reg.GuestAgentJSON, guestAgent); err != nil {
			return fail(fmt.Errorf("读取 Guest Agent 配置失败: %w", err))
		}
	}
	var smbios1 *vm_xml.VMSMBIOS1Config
	if strings.TrimSpace(reg.SMBIOS1JSON) != "" {
		smbios1 = &vm_xml.VMSMBIOS1Config{}
		if err := parseJSONText(reg.SMBIOS1JSON, smbios1); err != nil {
			return fail(fmt.Errorf("读取 SMBIOS 配置失败: %w", err))
		}
	}
	meta := HookGetTemplateMeta(reg.Template)
	cloneParams := &clonepkg.CloneParams{
		Name:                 reg.VMName,
		Template:             reg.Template,
		TemplateType:         reg.TemplateType,
		CloneMode:            reg.CloneMode,
		VCPU:                 reg.VCPU,
		RAM:                  reg.RAM,
		DiskSize:             reg.DiskSize,
		DiskBus:              reg.DiskBus,
		Hostname:             reg.Hostname,
		User:                 params.CredentialUsername,
		Password:             params.CredentialPassword,
		Autostart:            reg.Autostart,
		Freeze:               reg.Freeze,
		APIC:                 reg.APIC,
		PAE:                  reg.PAE,
		RTCOffset:            reg.RTCOffset,
		RTCStartDate:         reg.RTCStartDate,
		GuestAgent:           guestAgent,
		SMBIOS1:              smbios1,
		TemplateRootPass:     meta.RootPassword,
		TemplateUser:         meta.TemplateUser,
		VideoModel:           reg.VideoModel,
		CPUTopologyMode:      reg.CPUTopologyMode,
		CPULimitPercent:      reg.CPULimitPercent,
		CPUAffinity:          reg.CPUAffinity,
		FirstBootRebootMode:  reg.FirstBootRebootMode,
		SwitchID:             user.DedicatedVPCSwitchID,
		StoragePoolID:        reg.StoragePoolID,
		PreserveFnOSDeviceID: reg.PreserveFnOSDeviceID,
		FnOSDeviceID:         reg.FnOSDeviceID,
		NicModel:             reg.NicModel,
		IsAdmin:              true,
	}
	progressFn(5, "正在创建轻量云服务器...")
	result, err := clonepkg.CloneVM(ctx, cloneParams, progressFn)
	if err != nil {
		if !isVMAlreadyExistsError(err) || !vmDomainExists(reg.VMName) {
			return fail(err)
		}
		progressFn(10, "检测到上次失败后保留的虚拟机，尝试继续初始化...")
		result, err = continueExistingLightweightVM(ctx, cloneParams, progressFn)
		if err != nil {
			return fail(err)
		}
	}
	if err := HookAddVMToUser(user.Username, reg.VMName); err != nil {
		return fail(fmt.Errorf("服务器已创建，但写入用户归属失败: %w", err))
	}
	quotaReq := LightweightVMQuotaRequest{
		VMName:            reg.VMName,
		TrafficDownGB:     reg.TrafficDownGB,
		TrafficUpGB:       reg.TrafficUpGB,
		BandwidthDownMbps: reg.BandwidthDownMbps,
		BandwidthUpMbps:   reg.BandwidthUpMbps,
		MaxPortForwards:   reg.MaxPortForwards,
		MaxSnapshots:      reg.MaxSnapshots,
		MaxRuntimeHours:   reg.MaxRuntimeHours,
	}
	if _, err := UpsertLightweightVMQuota(user.Username, quotaReq); err != nil {
		return fail(fmt.Errorf("服务器已创建，但写入轻量云配额失败: %w", err))
	}
	if err := EnsureLightweightVMNetwork(user.Username, reg.VMName); err != nil {
		return fail(fmt.Errorf("服务器已创建，但绑定专用 VPC 失败: %w", err))
	}
	if err := ApplyLightweightVMBandwidth(reg.VMName); err != nil {
		return fail(fmt.Errorf("服务器已创建，但应用带宽失败: %w", err))
	}
	if err := HookSaveVMCredential(reg.VMName, params.CredentialUsername, params.CredentialPassword, "lightweight_registration", params.Operator, false); err != nil {
		return fail(fmt.Errorf("服务器已创建，但保存登录凭据失败: %w", err))
	}
	if err := model.DB.Model(&model.LightweightVMRegistration{}).Where("id = ?", reg.ID).Updates(map[string]interface{}{
		"status":        LightweightVMRegistrationStatusActive,
		"error_message": "",
	}).Error; err != nil {
		return fail(fmt.Errorf("服务器已创建，但更新注册状态失败: %w", err))
	}
	return result, nil
}

func isVMAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "已存在") || strings.Contains(strings.ToLower(msg), "already exists")
}

func vmDomainExists(vmName string) bool {
	// 存在性探测：域不存在属预期情况，失败仅记 DEBUG
	result := utils.ExecCommandQuiet("virsh", "dominfo", strings.TrimSpace(vmName))
	return result.ExitCode == 0
}

func continueExistingLightweightVM(ctx context.Context, params *clonepkg.CloneParams, progressFn func(int, string)) (*clonepkg.CloneResult, error) {
	if params == nil {
		return nil, fmt.Errorf("虚拟机参数为空")
	}
	stateResult := utils.ExecCommand("virsh", "domstate", params.Name)
	if stateResult.Error != nil {
		return nil, fmt.Errorf("读取虚拟机状态失败: %s", stateResult.Stderr)
	}
	if !strings.Contains(strings.ToLower(stateResult.Stdout), "running") {
		if err := HookStartVM(params.Name); err != nil {
			return nil, err
		}
	}
	HookFixOnReboot(params.Name)
	if err := clonepkg.CheckCanceled(ctx, params.Name, ""); err != nil {
		return nil, err
	}
	progressFn(60, "虚拟机启动中...")
	// Linux 已在 CloneVM 阶段通过 virt-customize 离线初始化
	// cloud-init 将在首次启动后自动处理 hostname 确认和磁盘扩容
	time.Sleep(5 * time.Second)
	ip := clonepkg.WaitForIPWithContext(ctx, params.Name, 30)
	diskPath := HookGetVMDiskPath(params.Name)
	progressFn(100, "轻量云服务器初始化完成")
	return &clonepkg.CloneResult{
		VMName:   params.Name,
		IP:       ip,
		DiskPath: diskPath,
		Template: params.Template,
	}, nil
}
