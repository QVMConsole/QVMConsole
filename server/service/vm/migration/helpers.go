package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/model"
	"kvm_console/service"
	netpkg "kvm_console/service/network"
	"kvm_console/utils"
)

func buildDiskAndBackingChecks(node model.HostNode, vmName, targetDir string, skipHash bool) ([]MigrationDisk, []MigrationBackingCheck, error) {
	disks, err := listDomainDisks(vmName)
	if err != nil {
		return nil, nil, err
	}
	var checks []MigrationBackingCheck
	for i := range disks {
		chain, err := service.QemuInfoChain(disks[i].SourcePath)
		if err != nil {
			return nil, nil, err
		}
		if len(chain) > 0 {
			disks[i].Format = chain[0].Format
			disks[i].VirtualSize = chain[0].VirtualSize
			disks[i].ActualSize = chain[0].ActualSize
			disks[i].BackingPath = firstNonEmpty(chain[0].FullBackingFilename, chain[0].BackingFilename)
		}
		if len(chain) > 1 {
			disks[i].BackingFormat = chain[1].Format
		}
		if strings.TrimSpace(targetDir) != "" {
			disks[i].TargetPath = filepath.Join(targetDir, filepath.Base(disks[i].SourcePath))
		}
		for _, backing := range chain[1:] {
			check := compareRemoteBacking(node, backing, skipHash)
			checks = append(checks, check)
		}
	}
	return disks, checks, nil
}

func applyMigrationDiskStorageTargets(disks []MigrationDisk, targets []service.VMStorageTarget, defaultStorage service.VMStorageTarget, hasDefault bool, requested []MigrationDiskStorageTarget) []string {
	requestedByTarget := map[string]string{}
	for _, item := range requested {
		target := strings.TrimSpace(firstNonEmpty(item.Target, item.Device))
		poolID := strings.TrimSpace(item.TargetStoragePoolID)
		if target == "" || poolID == "" {
			continue
		}
		requestedByTarget[target] = poolID
	}

	var blockers []string
	for i := range disks {
		poolID := strings.TrimSpace(requestedByTarget[disks[i].Target])
		if poolID == "" && hasDefault {
			poolID = defaultStorage.ID
		}
		if poolID == "" {
			blockers = append(blockers, fmt.Sprintf("请选择磁盘 %s 的目标存储", disks[i].Target))
			continue
		}
		storage, ok := findTargetStorage(targets, poolID)
		if !ok {
			blockers = append(blockers, fmt.Sprintf("磁盘 %s 的目标存储不存在或不可用于 VM", disks[i].Target))
			continue
		}
		disks[i].TargetStoragePoolID = storage.ID
		disks[i].TargetStorageDir = storage.VMDir
		disks[i].TargetPath = filepath.Join(storage.VMDir, filepath.Base(disks[i].SourcePath))
	}
	return blockers
}

func buildResolvedMigrationDiskStorageTargets(disks []MigrationDisk) []MigrationDiskStorageTarget {
	items := make([]MigrationDiskStorageTarget, 0, len(disks))
	for _, disk := range disks {
		if strings.TrimSpace(disk.Target) == "" || strings.TrimSpace(disk.TargetStoragePoolID) == "" {
			continue
		}
		items = append(items, MigrationDiskStorageTarget{
			Target:              disk.Target,
			Device:              disk.Target,
			TargetStoragePoolID: disk.TargetStoragePoolID,
		})
	}
	return items
}

func listDomainDisks(vmName string) ([]MigrationDisk, error) {
	result := utils.ExecCommand("virsh", "domblklist", vmName, "--details")
	if result.Error != nil {
		return nil, fmt.Errorf("读取虚拟机磁盘失败: %s", result.Stderr)
	}
	var disks []MigrationDisk
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[1] != "disk" {
			continue
		}
		path := fields[3]
		if path == "-" || !strings.HasPrefix(path, "/") {
			continue
		}
		disks = append(disks, MigrationDisk{
			Target:     fields[2],
			SourcePath: path,
			TargetPath: path,
		})
	}
	if len(disks) == 0 {
		return nil, fmt.Errorf("未找到可迁移磁盘")
	}
	return disks, nil
}

func compareRemoteBacking(node model.HostNode, backing service.QemuImgInfo, skipHash bool) MigrationBackingCheck {
	path := strings.TrimSpace(backing.Filename)
	check := MigrationBackingCheck{
		Path:              path,
		SourceFormat:      backing.Format,
		SourceVirtualSize: backing.VirtualSize,
	}
	if path == "" {
		check.Message = "backing 路径为空"
		return check
	}
	if skipHash {
		remoteCmd := "qemu-img info -U --output=json " + utils.ShellSingleQuote(path)
		out, err := service.RemoteSSHCommand(context.Background(), node, remoteCmd, 2*time.Minute)
		if err != nil {
			check.Message = "目标 backing 不存在或不可读: " + err.Error()
			return check
		}
		var target service.QemuImgInfo
		if err := json.Unmarshal([]byte(out), &target); err != nil {
			check.Message = "解析目标 backing 信息失败: " + err.Error()
			return check
		}
		check.TargetFormat = target.Format
		check.TargetVirtualSize = target.VirtualSize
		check.OK = check.SourceFormat == check.TargetFormat && check.SourceVirtualSize == check.TargetVirtualSize
		if !check.OK {
			check.Message = "format 或 virtual size 不一致"
		} else {
			check.Message = "已跳过 hash 校验"
		}
		return check
	}
	// 计算大文件 hash 属于 IO 操作，不设置自动超时
	sourceHash := utils.ExecCommandNoTimeout("sha256sum", path)
	if sourceHash.Error != nil {
		check.Message = "源 backing hash 失败: " + sourceHash.Stderr
		return check
	}
	check.SourceSHA256 = strings.Fields(sourceHash.Stdout)[0]
	remoteCmd := "set -e; qemu-img info -U --output=json " + utils.ShellSingleQuote(path) + "; sha256sum " + utils.ShellSingleQuote(path)
	// 远程 sha256sum 计算大文件 hash 属于 IO 操作，不设置自动超时
	out, err := service.RemoteSSHCommand(context.Background(), node, remoteCmd, 0)
	if err != nil {
		check.Message = "目标 backing 不存在或不可读: " + err.Error()
		return check
	}
	parts := strings.Split(strings.TrimSpace(out), "\n")
	if len(parts) < 2 {
		check.Message = "目标 backing 信息格式无效"
		return check
	}
	var target service.QemuImgInfo
	if err := json.Unmarshal([]byte(strings.Join(parts[:len(parts)-1], "\n")), &target); err != nil {
		check.Message = "解析目标 backing 信息失败: " + err.Error()
		return check
	}
	check.TargetFormat = target.Format
	check.TargetVirtualSize = target.VirtualSize
	hashFields := strings.Fields(parts[len(parts)-1])
	if len(hashFields) > 0 {
		check.TargetSHA256 = hashFields[0]
	}
	check.OK = check.SourceFormat == check.TargetFormat &&
		check.SourceVirtualSize == check.TargetVirtualSize &&
		check.SourceSHA256 != "" &&
		check.SourceSHA256 == check.TargetSHA256
	if !check.OK {
		check.Message = "format、virtual size 或 hash 不一致"
	} else {
		check.Message = "校验通过"
	}
	return check
}

// ---------- VPC / network helpers ----------

func fillTargetVPCOptions(node model.HostNode, preview *VMMigrationPreview) {
	_, switches, groups := fetchTargetNetworkOptions(node)
	preview.TargetSwitches, preview.TargetSecurityGroups = filterTargetMigrationNetworks(preview.Owner, preview.IsLightweight, preview.TargetUserExists, switches, groups)
	if !preview.TargetUserExists && preview.Owner != "admin" {
		preview.TargetSwitchID = 0
		preview.TargetSecurityGroupID = 0
		preview.Warnings = append(preview.Warnings, "目标节点将先创建用户，再使用该用户默认网络绑定 VM")
		return
	}
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", preview.VMName).First(&binding).Error; err == nil {
		preview.SourceBinding = &binding
		if preview.TargetSwitchID == 0 {
			preview.TargetSwitchID = matchTargetSwitch(binding, preview.TargetSwitches, preview.IsLightweight)
		}
		if preview.TargetSecurityGroupID == 0 {
			preview.TargetSecurityGroupID = matchTargetSecurityGroup(binding, preview.TargetSecurityGroups)
		}
	}
}

func validateTargetNetwork(preview *VMMigrationPreview, req VMMigrationRequest) {
	if !preview.TargetUserExists && preview.Owner != "admin" {
		return
	}
	if preview.IsLightweight {
		if preview.TargetSwitchID == 0 {
			preview.Blockers = append(preview.Blockers, "轻量云迁移必须选择目标节点轻量云 VPC")
			return
		}
		if !switchExistsInList(preview.TargetSwitchID, preview.TargetSwitches, true) {
			preview.Blockers = append(preview.Blockers, "目标轻量云 VPC 无效或不是 NAT 网络")
		}
		return
	}
	if preview.Owner != "admin" && preview.TargetSwitchID == 0 {
		preview.Blockers = append(preview.Blockers, "目标已有同名用户，请选择该用户下的目标 VPC")
		return
	}
	if preview.TargetSwitchID > 0 && !switchExistsInList(preview.TargetSwitchID, preview.TargetSwitches, false) {
		preview.Blockers = append(preview.Blockers, "目标 VPC 不存在")
	}
	if req.TargetSecurityGroupID > 0 && !securityGroupExistsInList(req.TargetSecurityGroupID, preview.TargetSecurityGroups) {
		preview.Blockers = append(preview.Blockers, "目标安全组不存在")
	}
}

func fillMigrationPortForwards(node model.HostNode, preview *VMMigrationPreview) {
	rules, err := netpkg.ListPortForwards()
	if err != nil {
		preview.Warnings = append(preview.Warnings, "读取源端口转发失败: "+err.Error())
		return
	}
	targetUsed := map[string]bool{}
	var targetRules []netpkg.PortForwardRule
	if _, err := service.CallNodeAPI(node, "GET", "/api/network/port-forward/list", nil, &targetRules); err == nil {
		for _, rule := range targetRules {
			targetUsed[strings.ToLower(rule.Protocol)+"|"+rule.HostPort] = true
		}
	}
	for _, rule := range rules {
		if strings.TrimSpace(rule.VMName) != preview.VMName {
			continue
		}
		item := MigrationPortForwardMap{
			Protocol:       strings.ToLower(firstNonEmpty(rule.Protocol, "tcp")),
			SourceHostPort: rule.HostPort,
			TargetHostPort: rule.HostPort,
			VMPort:         rule.DestPort,
			DestIP:         rule.DestIP,
		}
		if targetUsed[item.Protocol+"|"+item.TargetHostPort] {
			item.TargetHostPort = ""
			item.AutoAllocated = true
			preview.Warnings = append(preview.Warnings, "目标节点端口 "+rule.HostPort+"/"+item.Protocol+" 已占用，将自动分配新端口")
		}
		preview.PortForwards = append(preview.PortForwards, item)
	}
}

func filterTargetMigrationNetworks(owner string, lightweight, targetUserExists bool, switches []model.VPCSwitch, groups []model.VPCSecurityGroup) ([]model.VPCSwitch, []model.VPCSecurityGroup) {
	owner = strings.TrimSpace(owner)
	if owner != "admin" && !targetUserExists {
		return nil, nil
	}
	filteredSwitches := make([]model.VPCSwitch, 0, len(switches))
	for _, sw := range switches {
		if owner != "" && sw.Username != owner {
			continue
		}
		if lightweight && strings.TrimSpace(sw.BridgeMode) != "" && !strings.EqualFold(sw.BridgeMode, service.BridgeModeNAT) {
			continue
		}
		filteredSwitches = append(filteredSwitches, sw)
	}
	filteredGroups := make([]model.VPCSecurityGroup, 0, len(groups))
	for _, group := range groups {
		if owner != "" && group.Username != owner {
			continue
		}
		filteredGroups = append(filteredGroups, group)
	}
	return filteredSwitches, filteredGroups
}

func switchExistsInList(id uint, switches []model.VPCSwitch, requireNAT bool) bool {
	for _, sw := range switches {
		if sw.ID != id {
			continue
		}
		if !requireNAT {
			return true
		}
		mode := strings.TrimSpace(sw.BridgeMode)
		return mode == "" || strings.EqualFold(mode, service.BridgeModeNAT)
	}
	return false
}

func securityGroupExistsInList(id uint, groups []model.VPCSecurityGroup) bool {
	for _, group := range groups {
		if group.ID == id {
			return true
		}
	}
	return false
}

// ---------- storage & remote helpers ----------

func fetchTargetStorageTargets(node model.HostNode) []service.VMStorageTarget {
	var targets []service.VMStorageTarget
	_, _ = service.CallNodeAPI(node, "GET", "/api/storage-pool/vm-targets", nil, &targets)
	return targets
}

func fetchTargetNetworkOptions(node model.HostNode) (bool, []model.VPCSwitch, []model.VPCSecurityGroup) {
	var switches []model.VPCSwitch
	var groups []model.VPCSecurityGroup
	_, swErr := service.CallNodeAPI(node, "GET", "/api/vpc/switches", nil, &switches)
	_, groupErr := service.CallNodeAPI(node, "GET", "/api/vpc/security-groups", nil, &groups)
	return swErr == nil && groupErr == nil, switches, groups
}

func findTargetStorage(targets []service.VMStorageTarget, id string) (service.VMStorageTarget, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return service.VMStorageTarget{}, false
	}
	for _, target := range targets {
		if target.ID == id && target.Enabled && strings.TrimSpace(target.VMDir) != "" {
			return target, true
		}
	}
	return service.VMStorageTarget{}, false
}

func diskTargetExists(node model.HostNode, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	out, err := service.RemoteSSHCommand(context.Background(), node, "test -e "+utils.ShellSingleQuote(path)+" && echo exists || echo missing", 30*time.Second)
	return err == nil && strings.TrimSpace(out) == "exists"
}
