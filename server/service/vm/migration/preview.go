package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/utils"
)

// firstNonEmpty returns the first non-empty trimmed string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ParseVMMigrationTaskParams(raw string) (VMMigrationTaskParams, error) {
	var params VMMigrationTaskParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return params, err
	}
	params.Mode = DetectMigrationModeFromState(service.GetDomainState(params.VMName))
	return params, nil
}

func GetVMMigrationOptions(vmName string, nodeID uint) (*VMMigrationOptions, error) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return nil, fmt.Errorf("虚拟机名称不能为空")
	}
	node, err := service.GetHostNode(nodeID)
	if err != nil {
		return nil, err
	}
	if err := ensureMigrationNodeRoot(*node); err != nil {
		return nil, err
	}
	if !service.DomainExists(vmName) {
		return nil, fmt.Errorf("源虚拟机不存在")
	}
	state := service.GetDomainState(vmName)
	owner := service.FindVMOwner(vmName)
	if owner == "" {
		owner = "admin"
	}
	sourceUser := loadUserSnapshot(owner)
	cloudType := service.NormalizeCloudType(sourceUser.CloudType)
	isLightweight := service.IsLightweightCloudType(cloudType) || service.IsLightweightCloudVM(vmName)
	if isLightweight {
		cloudType = service.CloudTypeLightweight
	}
	options := &VMMigrationOptions{
		VMName:        vmName,
		SourceState:   state,
		Mode:          DetectMigrationModeFromState(state),
		Owner:         owner,
		CloudType:     cloudType,
		IsLightweight: isLightweight,
	}
	options.TargetUserExists = targetUserExists(*node, owner)
	options.WillCreateTargetUser = owner != "admin" && !options.TargetUserExists
	options.TargetStorageTargets = fetchTargetStorageTargets(*node)
	_, sws, groups := fetchTargetNetworkOptions(*node)
	options.TargetSwitches, options.TargetSecurityGroups = filterTargetMigrationNetworks(owner, isLightweight, options.TargetUserExists, sws, groups)
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).First(&binding).Error; err == nil {
		options.SourceBinding = &binding
		options.TargetSwitchID = matchTargetSwitch(binding, options.TargetSwitches, isLightweight)
		options.TargetSecurityGroupID = matchTargetSecurityGroup(binding, options.TargetSecurityGroups)
	}
	return options, nil
}

func CreateVMMigrationPreview(vmName string, req VMMigrationRequest) (*VMMigrationPreview, error) {
	preview, err := BuildVMMigrationPreview(vmName, req)
	if err != nil {
		return nil, err
	}
	if preview.Allowed {
		preview.PreviewID = storeMigrationPreview(*preview)
	}
	return preview, nil
}

func BuildVMMigrationPreview(vmName string, req VMMigrationRequest) (*VMMigrationPreview, error) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return nil, fmt.Errorf("虚拟机名称不能为空")
	}
	node, err := service.GetHostNode(req.NodeID)
	if err != nil {
		return nil, err
	}
	if err := ensureMigrationNodeRoot(*node); err != nil {
		return nil, err
	}
	if !node.Enabled {
		return nil, fmt.Errorf("目标节点已禁用")
	}
	state := service.GetDomainState(vmName)
	mode := DetectMigrationModeFromState(state)
	owner := service.FindVMOwner(vmName)
	if owner == "" {
		owner = "admin"
	}
	sourceUser := loadUserSnapshot(owner)
	cloudType := service.NormalizeCloudType(sourceUser.CloudType)
	isLightweight := service.IsLightweightCloudType(cloudType) || service.IsLightweightCloudVM(vmName)
	if isLightweight {
		cloudType = service.CloudTypeLightweight
	}
	preview := &VMMigrationPreview{
		VMName:                vmName,
		Mode:                  mode,
		Node:                  service.BuildHostNodeView(*node),
		Owner:                 owner,
		CloudType:             cloudType,
		IsLightweight:         isLightweight,
		SourceState:           state,
		TargetStoragePoolID:   strings.TrimSpace(req.TargetStoragePoolID),
		TargetSwitchID:        req.TargetSwitchID,
		TargetSecurityGroupID: req.TargetSecurityGroupID,
	}
	if !service.DomainExists(vmName) {
		preview.Blockers = append(preview.Blockers, "源虚拟机不存在")
		return finishPreview(preview), nil
	}
	if mode == MigrationModeCold && !strings.Contains(strings.ToLower(state), "shut off") {
		preview.Blockers = append(preview.Blockers, "冷迁移要求源虚拟机先关机")
	}
	if mode == MigrationModeLive && !strings.Contains(strings.ToLower(state), "running") {
		preview.Blockers = append(preview.Blockers, "热迁移要求源虚拟机正在运行")
	}
	if out, err := service.RemoteSSHCommand(context.Background(), *node, "virsh dominfo "+utils.ShellSingleQuote(vmName)+" >/dev/null 2>&1 && echo exists || echo missing", 30*time.Second); err == nil {
		if strings.TrimSpace(out) == "exists" {
			preview.Blockers = append(preview.Blockers, "目标节点已存在同名虚拟机")
		}
	}
	preview.TargetStorageTargets = fetchTargetStorageTargets(*node)
	targetStorage, ok := findTargetStorage(preview.TargetStorageTargets, req.TargetStoragePoolID)
	hasDiskStorageTargets := len(req.DiskStorageTargets) > 0
	if strings.TrimSpace(req.TargetStoragePoolID) == "" && !hasDiskStorageTargets {
		preview.Blockers = append(preview.Blockers, "请选择目标存储")
	} else if strings.TrimSpace(req.TargetStoragePoolID) != "" && !ok {
		preview.Blockers = append(preview.Blockers, "目标存储不存在或不可用于 VM")
	} else if ok {
		preview.TargetStoragePoolID = targetStorage.ID
		preview.TargetStorageDir = targetStorage.VMDir
	}
	disks, backingChecks, err := buildDiskAndBackingChecks(*node, vmName, "", req.SkipPrecheck)
	if err != nil {
		preview.Blockers = append(preview.Blockers, err.Error())
	} else {
		if diskBlockers := applyMigrationDiskStorageTargets(disks, preview.TargetStorageTargets, targetStorage, ok, req.DiskStorageTargets); len(diskBlockers) > 0 {
			preview.Blockers = append(preview.Blockers, diskBlockers...)
		}
		preview.DiskStorageTargets = buildResolvedMigrationDiskStorageTargets(disks)
		preview.Disks = disks
		preview.BackingChecks = backingChecks
		if req.SkipPrecheck {
			preview.Warnings = append(preview.Warnings, "已跳过完整预检：不会在提交前计算 backing hash，迁移任务执行失败时请按任务错误处理")
		}
		requiredByPool := map[string]int64{}
		seenTargetPath := map[string]bool{}
		for _, disk := range disks {
			preview.RequiredStorageBytes += disk.ActualSize
			if disk.TargetStoragePoolID != "" {
				requiredByPool[disk.TargetStoragePoolID] += disk.ActualSize
			}
			if disk.TargetPath != "" {
				if seenTargetPath[disk.TargetPath] {
					preview.Blockers = append(preview.Blockers, "多块磁盘目标路径重复: "+disk.TargetPath)
				}
				seenTargetPath[disk.TargetPath] = true
			}
			if diskTargetExists(*node, disk.TargetPath) {
				preview.Blockers = append(preview.Blockers, "目标磁盘已存在: "+disk.TargetPath)
			}
		}
		for poolID, requiredBytes := range requiredByPool {
			storage, storageOK := findTargetStorage(preview.TargetStorageTargets, poolID)
			if storageOK && storage.Available > 0 && requiredBytes > storage.Available {
				preview.Blockers = append(preview.Blockers, fmt.Sprintf("目标存储 %s 可用空间不足", storage.DisplayName))
			}
		}
		for _, check := range backingChecks {
			if !req.SkipPrecheck && !check.OK {
				preview.Blockers = append(preview.Blockers, "链式 backing 校验失败: "+check.Message)
			}
		}
	}
	preview.TargetUserExists = targetUserExists(*node, owner)
	preview.WillCreateTargetUser = owner != "admin" && !preview.TargetUserExists
	fillTargetVPCOptions(*node, preview)
	validateTargetNetwork(preview, req)
	if attachments := service.ListPublicIPAttachmentsForVM(vmName); len(attachments) > 0 {
		preview.Blockers = append(preview.Blockers, "当前 VM 绑定了公网 IP，请先在目标节点配置同等公网 IP 资源后再迁移")
	}
	fillMigrationPortForwards(*node, preview)
	if cred, err := service.GetVMCredential(vmName); err == nil && cred != nil {
		preview.Credential = cred
	}
	if mode == MigrationModeLive && len(preview.Blockers) == 0 {
		assessment, err := AssessLiveMigration(context.Background(), *node, vmName, req.EnableCPUThrottle, req.CPUThrottlePercent)
		if err != nil {
			preview.Blockers = append(preview.Blockers, err.Error())
		} else {
			preview.LiveAssessment = assessment
			preview.Warnings = append(preview.Warnings, assessment.Warnings...)
			if !assessment.Allowed {
				preview.Blockers = append(preview.Blockers, assessment.BlockReason)
			}
		}
	}
	return finishPreview(preview), nil
}

// ---------- small helpers used by preview & options ----------

func finishPreview(preview *VMMigrationPreview) *VMMigrationPreview {
	preview.Allowed = len(preview.Blockers) == 0
	return preview
}

func DetectMigrationModeFromState(state string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(state)), "running") {
		return MigrationModeLive
	}
	return MigrationModeCold
}

func matchTargetSwitch(binding model.VPCVMBinding, switches []model.VPCSwitch, lightweight bool) uint {
	var source model.VPCSwitch
	if model.DB.First(&source, binding.SwitchID).Error != nil {
		return 0
	}
	for _, sw := range switches {
		if lightweight && strings.TrimSpace(sw.BridgeMode) != "" && !strings.EqualFold(sw.BridgeMode, service.BridgeModeNAT) {
			continue
		}
		if sw.Username == binding.Username && sw.Name == source.Name && sw.CIDR == source.CIDR {
			return sw.ID
		}
	}
	return 0
}

func matchTargetSecurityGroup(binding model.VPCVMBinding, groups []model.VPCSecurityGroup) uint {
	var source model.VPCSecurityGroup
	if model.DB.First(&source, binding.SecurityGroupID).Error != nil {
		return 0
	}
	for _, group := range groups {
		if group.Username == source.Username && group.Name == source.Name {
			return group.ID
		}
	}
	return 0
}

func targetUserExists(node model.HostNode, username string) bool {
	if username == "admin" {
		return true
	}
	var users []service.VMUserInfo
	if _, err := service.CallNodeAPI(node, "GET", "/api/user/list", nil, &users); err != nil {
		return false
	}
	for _, user := range users {
		if user.Username == username {
			return true
		}
	}
	return false
}

func validateCachedPreviewForExecution(preview *VMMigrationPreview, params VMMigrationTaskParams) error {
	if preview == nil {
		return fmt.Errorf("迁移预检为空")
	}
	if preview.VMName != params.VMName || preview.Node.ID != params.NodeID || preview.TargetStoragePoolID != params.TargetStoragePoolID ||
		preview.TargetSwitchID != params.TargetSwitchID || preview.TargetSecurityGroupID != params.TargetSecurityGroupID {
		return fmt.Errorf("迁移表单已变化，请重新预检")
	}
	if migrationDiskStorageTargetsChanged(preview, params) {
		return fmt.Errorf("磁盘目标存储选择已变化，请重新预检")
	}
	currentMode := DetectMigrationModeFromState(service.GetDomainState(preview.VMName))
	if currentMode != preview.Mode {
		return fmt.Errorf("虚拟机运行状态已变化，请重新预检")
	}
	if !service.DomainExists(preview.VMName) {
		return fmt.Errorf("源虚拟机不存在")
	}
	node, err := service.GetHostNode(preview.Node.ID)
	if err != nil {
		return err
	}
	if err := ensureMigrationNodeRoot(*node); err != nil {
		return err
	}
	out, err := service.RemoteSSHCommand(context.Background(), *node, "virsh dominfo "+utils.ShellSingleQuote(preview.VMName)+" >/dev/null 2>&1 && echo exists || echo missing", 30*time.Second)
	if err != nil {
		return fmt.Errorf("检查目标 VM 名称失败: %w", err)
	}
	if strings.TrimSpace(out) == "exists" {
		return fmt.Errorf("目标节点已存在同名虚拟机")
	}
	for _, disk := range preview.Disks {
		if diskTargetExists(*node, disk.TargetPath) {
			return fmt.Errorf("目标磁盘已存在: %s", disk.TargetPath)
		}
	}
	return nil
}

func ensureMigrationNodeRoot(node model.HostNode) error {
	if strings.TrimSpace(node.SSHUser) != "root" {
		return fmt.Errorf("目标节点 SSH 用户必须为 root，非 root 用户没有迁移虚拟机所需的 libvirt、OVS 与存储目录权限")
	}
	return nil
}

func migrationDiskStorageTargetsChanged(preview *VMMigrationPreview, params VMMigrationTaskParams) bool {
	requested := map[string]string{}
	for _, item := range params.DiskStorageTargets {
		target := strings.TrimSpace(firstNonEmpty(item.Target, item.Device))
		if target != "" {
			requested[target] = strings.TrimSpace(item.TargetStoragePoolID)
		}
	}
	for _, disk := range preview.Disks {
		expected := strings.TrimSpace(disk.TargetStoragePoolID)
		actual := strings.TrimSpace(requested[disk.Target])
		if actual == "" {
			actual = strings.TrimSpace(params.TargetStoragePoolID)
		}
		if expected != actual {
			return true
		}
	}
	return false
}
