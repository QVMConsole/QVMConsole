package template

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

// DeleteTemplate deletes a template (only if no linked VMs).
func DeleteTemplate(templateName string) error {
	preview, err := GetDeleteTemplatePreview(templateName)
	if err != nil {
		return err
	}
	if len(preview.RelatedVMs) > 0 {
		return fmt.Errorf("模板链路下仍存在 %d 台虚拟机，请确认联动删除后再试", len(preview.RelatedVMs))
	}
	for i := len(preview.Templates) - 1; i >= 0; i-- {
		if err := deleteTemplateFiles(preview.Templates[i].Name); err != nil {
			return err
		}
	}
	return nil
}

func normalizeTemplateDeleteMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case TemplateDeleteModePromoteHot:
		return TemplateDeleteModePromoteHot
	case TemplateDeleteModePromote:
		return TemplateDeleteModePromote
	default:
		return TemplateDeleteModeCascade
	}
}

// DeleteTemplateWithVMs deletes a template and optionally its linked VMs.
func DeleteTemplateWithVMs(params *DeleteTemplateParams, progressFn func(int, string)) (*DeleteTemplateResult, error) {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}
	if params == nil || strings.TrimSpace(params.TemplateName) == "" {
		return nil, fmt.Errorf("模板名称不能为空")
	}

	progressFn(5, "正在检查模板链路...")
	preview, err := GetDeleteTemplatePreview(params.TemplateName)
	if err != nil {
		return nil, err
	}
	deleteMode := normalizeTemplateDeleteMode(params.DeleteMode)

	currentVMNames := make([]string, 0, len(preview.RelatedVMs))
	for _, vm := range preview.RelatedVMs {
		currentVMNames = append(currentVMNames, vm.Name)
	}
	if deleteMode == TemplateDeleteModeCascade && len(currentVMNames) > 0 && !params.DeleteVMs {
		return nil, fmt.Errorf("模板链路下仍存在 %d 台虚拟机，请确认联动删除后再试", len(currentVMNames))
	}
	if params.DeleteVMs && len(params.ExpectedVMs) > 0 && !sameStringSet(currentVMNames, params.ExpectedVMs) {
		return nil, fmt.Errorf("模板关联虚拟机列表已发生变化，请刷新页面后重新确认")
	}
	if deleteMode == TemplateDeleteModePromote || deleteMode == TemplateDeleteModePromoteHot {
		if len(params.ExpectedVMs) > 0 && !sameStringSet(currentVMNames, params.ExpectedVMs) {
			return nil, fmt.Errorf("模板关联虚拟机列表已发生变化，请刷新页面后重新确认")
		}
		if deleteMode == TemplateDeleteModePromoteHot {
			return deleteTemplatePromoteChildrenHot(params, preview, progressFn)
		}
		return deleteTemplatePromoteChildren(params, preview, progressFn)
	}

	deletedVMs := make([]string, 0, len(currentVMNames))
	affectedUsers := make(map[string]bool)
	totalVMs := len(preview.RelatedVMs)
	for i, vm := range preview.RelatedVMs {
		progress := 10 + (i * 55 / maxInt(totalVMs, 1))
		progressFn(progress, fmt.Sprintf("正在删除关联虚拟机 %s (%d/%d)...", vm.Name, i+1, totalVMs))
		owner := HookFindVMOwner(vm.Name)
		if err := HookDeleteVM(vm.Name); err != nil {
			return nil, fmt.Errorf("删除关联虚拟机 %s 失败: %w", vm.Name, err)
		}
		if owner != "" {
			if err := HookRemoveVMFromUser(owner, vm.Name); err != nil {
				logger.App.Warn("从用户访问列表移除虚拟机失败", "owner", owner, "vm", vm.Name, "error", err)
			}
			affectedUsers[owner] = true
		}
		deletedVMs = append(deletedVMs, vm.Name)
	}

	deletedTemplates := make([]string, 0, len(preview.Templates))
	progressFn(75, "正在删除模板链路文件...")
	for i := len(preview.Templates) - 1; i >= 0; i-- {
		name := preview.Templates[i].Name
		if err := deleteTemplateFiles(name); err != nil {
			return nil, err
		}
		deletedTemplates = append(deletedTemplates, name)
	}

	for username := range affectedUsers {
		if err := HookRebalanceUserBandwidth(username); err != nil {
			logger.App.Warn("删除模板后重新分配用户带宽失败", "user", username, "error", err)
		}
	}

	progressFn(100, "模板链路已删除")
	return &DeleteTemplateResult{
		TemplateName:     params.TemplateName,
		DeletedTemplates: deletedTemplates,
		DeletedVMs:       deletedVMs,
	}, nil
}

// GetDeleteTemplatePreview returns a preview of what will be deleted.
func GetDeleteTemplatePreview(templateName string) (*DeleteTemplatePreview, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	tpl, ok := tree.byName[templateName]
	if !ok {
		return nil, fmt.Errorf("模板不存在: %s", templateName)
	}
	templates := collectTemplateSubtree(tree, tpl.NodeID)
	vms := hydrateTemplateRelatedVMs(collectTemplateSubtreeVMs(tree, tpl.NodeID))
	directVMs := hydrateTemplateRelatedVMs(filterLinkedVMs(tree.vmByNode[tpl.NodeID]))
	preview := &DeleteTemplatePreview{
		TemplateName:      templateName,
		Templates:         templates,
		RelatedVMs:        vms,
		PromotedTemplates: directChildTemplates(tree, tpl.NodeID),
		RebasedVMs:        directVMs,
	}
	if parentID := strings.TrimSpace(tpl.ParentNodeID); parentID != "" {
		if parent, ok := tree.byNodeID[parentID]; ok {
			parentCopy := parent
			preview.ParentTemplate = &parentCopy
		}
	}
	preview.PromoteBlockers = buildTemplatePromoteBlockers(preview)
	preview.CanPromote = len(preview.PromoteBlockers) == 0
	preview.PromoteHotBlockers = buildTemplatePromoteHotBlockers(preview)
	preview.CanPromoteHot = len(preview.PromoteHotBlockers) == 0
	return preview, nil
}

// ListTemplateVMs returns the VMs directly linked to a template node.
func ListTemplateVMs(templateName string) ([]TemplateRelatedVM, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	tpl, ok := tree.byName[templateName]
	if !ok {
		return nil, fmt.Errorf("模板不存在: %s", templateName)
	}
	return append([]TemplateRelatedVM{}, tree.vmByNode[tpl.NodeID]...), nil
}

// ListTemplateSubtreeVMs returns all linked VMs in a template's subtree.
func ListTemplateSubtreeVMs(templateName string) ([]TemplateRelatedVM, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	tpl, ok := tree.byName[templateName]
	if !ok {
		return nil, fmt.Errorf("模板不存在: %s", templateName)
	}
	return collectTemplateSubtreeVMs(tree, tpl.NodeID), nil
}

// ── Promote helpers ──

func buildTemplatePromoteBlockers(preview *DeleteTemplatePreview) []string {
	var blockers []string
	if preview == nil {
		return []string{"删除预览为空"}
	}
	if preview.ParentTemplate == nil {
		blockers = append(blockers, "根模板没有上级节点，无法提升子节点")
	}
	if len(preview.PromotedTemplates) == 0 && len(preview.RebasedVMs) == 0 {
		blockers = append(blockers, "当前节点没有可提升的子模板或可重定向的直接 VM")
	}
	for _, vm := range preview.RelatedVMs {
		if !isVMStateShutoff(vm.Status) {
			blockers = append(blockers, fmt.Sprintf("关联虚拟机 %s 当前状态为 %s，请先关机", vm.Name, HookFirstNonEmpty(vm.Status, "unknown")))
		}
	}
	return blockers
}

func buildTemplatePromoteHotBlockers(preview *DeleteTemplatePreview) []string {
	var blockers []string
	if preview == nil {
		return []string{"删除预览为空"}
	}
	if preview.ParentTemplate == nil {
		blockers = append(blockers, "根模板没有上级节点，无法热提升子节点")
	}
	if len(preview.PromotedTemplates) == 0 && len(preview.RebasedVMs) == 0 {
		blockers = append(blockers, "当前节点没有可热提升的子模板或可热重定向的直接 VM")
	}
	for _, vm := range preview.RelatedVMs {
		if !isVMStateShutoff(vm.Status) && !strings.EqualFold(strings.TrimSpace(vm.Status), "running") {
			blockers = append(blockers, fmt.Sprintf("关联虚拟机 %s 当前状态为 %s，热提升仅支持 running 或 shut off", vm.Name, HookFirstNonEmpty(vm.Status, "unknown")))
		}
	}
	return blockers
}

func deleteTemplatePromoteChildren(params *DeleteTemplateParams, preview *DeleteTemplatePreview, progressFn func(int, string)) (*DeleteTemplateResult, error) {
	if preview == nil {
		return nil, fmt.Errorf("删除预览为空")
	}
	if len(preview.PromoteBlockers) > 0 {
		return nil, fmt.Errorf("无法提升子节点: %s", strings.Join(preview.PromoteBlockers, "；"))
	}
	parent := preview.ParentTemplate
	if parent == nil {
		return nil, fmt.Errorf("根模板没有上级节点，无法提升子节点")
	}
	deleteTpl, err := GetTemplateInfoByName(params.TemplateName)
	if err != nil {
		return nil, err
	}
	parentPath, err := EnsureTemplatePath(parent.Name)
	if err != nil {
		return nil, fmt.Errorf("父模板不可用: %w", err)
	}
	deletePath, err := EnsureTemplatePath(deleteTpl.Name)
	if err != nil {
		return nil, err
	}

	totalWork := len(preview.PromotedTemplates) + len(preview.RebasedVMs)
	if totalWork == 0 {
		return nil, fmt.Errorf("当前节点没有可提升的子模板或可重定向的直接 VM")
	}
	vmDiskPaths, err := validateTemplatePromoteRebaseTargets(preview, deletePath)
	if err != nil {
		return nil, err
	}

	promotedTemplates := make([]string, 0, len(preview.PromotedTemplates))
	rebasedVMs := make([]string, 0, len(preview.RebasedVMs))
	progressFn(10, "正在安全改写模板链路 backing...")
	for i, child := range preview.PromotedTemplates {
		progress := 10 + (i * 45 / maxInt(totalWork, 1))
		progressFn(progress, fmt.Sprintf("正在提升子模板 %s ...", child.AdminName))
		if err := rebaseQcow2BackingToParent(child.Path, deletePath, parentPath); err != nil {
			return nil, fmt.Errorf("提升子模板 %s 失败: %w", child.AdminName, err)
		}
		if err := updatePromotedTemplateMeta(child, *parent); err != nil {
			return nil, err
		}
		promotedTemplates = append(promotedTemplates, child.Name)
	}

	progressFn(55, "正在安全改写直接关联 VM backing...")
	for i, vm := range preview.RebasedVMs {
		progress := 55 + (i * 25 / maxInt(len(preview.RebasedVMs), 1))
		progressFn(progress, fmt.Sprintf("正在重定向虚拟机 %s 的链式硬盘...", vm.Name))
		if err := rebaseQcow2BackingToParent(vmDiskPaths[vm.Name], deletePath, parentPath); err != nil {
			return nil, fmt.Errorf("重定向虚拟机 %s 失败: %w", vm.Name, err)
		}
		if err := WriteVMTemplateSource(vm.Name, parent.Name, "linked"); err != nil {
			return nil, err
		}
		rebasedVMs = append(rebasedVMs, vm.Name)
	}

	progressFn(85, "正在删除当前模板节点文件...")
	if err := deleteTemplateFiles(deleteTpl.Name); err != nil {
		return nil, err
	}

	progressFn(100, "模板节点已删除，子节点已提升")
	return &DeleteTemplateResult{
		TemplateName:      params.TemplateName,
		DeletedTemplates:  []string{deleteTpl.Name},
		DeletedVMs:        []string{},
		PromotedTemplates: promotedTemplates,
		RebasedVMs:        rebasedVMs,
	}, nil
}

func deleteTemplatePromoteChildrenHot(params *DeleteTemplateParams, preview *DeleteTemplatePreview, progressFn func(int, string)) (*DeleteTemplateResult, error) {
	if preview == nil {
		return nil, fmt.Errorf("删除预览为空")
	}
	if len(preview.PromoteHotBlockers) > 0 {
		return nil, fmt.Errorf("无法热提升子节点: %s", strings.Join(preview.PromoteHotBlockers, "；"))
	}
	parent := preview.ParentTemplate
	if parent == nil {
		return nil, fmt.Errorf("根模板没有上级节点，无法热提升子节点")
	}
	deleteTpl, err := GetTemplateInfoByName(params.TemplateName)
	if err != nil {
		return nil, err
	}
	parentPath, err := EnsureTemplatePath(parent.Name)
	if err != nil {
		return nil, fmt.Errorf("父模板不可用: %w", err)
	}
	deletePath, err := EnsureTemplatePath(deleteTpl.Name)
	if err != nil {
		return nil, err
	}
	if len(preview.PromotedTemplates)+len(preview.RebasedVMs) == 0 {
		return nil, fmt.Errorf("当前节点没有可热提升的子模板或可热重定向的直接 VM")
	}
	if err := validateTemplatePromoteHotTargets(preview, deletePath); err != nil {
		return nil, err
	}

	progressFn(10, "正在热提升子模板 backing...")
	for i, child := range preview.PromotedTemplates {
		progressFn(10+(i*40/maxInt(len(preview.PromotedTemplates), 1)), fmt.Sprintf("正在热提升子模板 %s ...", child.AdminName))
		vms := runningVMsForTemplateSubtree(preview, child.NodeID)
		vmBackingPaths := runningVMBackingPaths(preview, vms, child.Path)
		if err := hotSwapPromotedTemplate(child, *parent, deletePath, parentPath, vms, vmBackingPaths); err != nil {
			return nil, err
		}
	}

	rebasedVMs := make([]string, 0, len(preview.RebasedVMs))
	progressFn(55, "正在热重定向直接关联 VM backing...")
	for i, vm := range preview.RebasedVMs {
		progressFn(55+(i*25/maxInt(len(preview.RebasedVMs), 1)), fmt.Sprintf("正在处理虚拟机 %s ...", vm.Name))
		diskInfo := HookGetVMDiskInfo(vm.Name)
		if strings.TrimSpace(diskInfo.Path) == "" || strings.TrimSpace(diskInfo.Device) == "" {
			return nil, fmt.Errorf("无法获取虚拟机 %s 的系统盘路径或设备名", vm.Name)
		}
		if strings.EqualFold(strings.TrimSpace(vm.Status), "running") {
			if err := blockPullRunningVMToBase(vm.Name, diskInfo.Device, diskInfo.Path, parentPath); err != nil {
				return nil, err
			}
		} else {
			if err := rebaseQcow2BackingToParent(diskInfo.Path, deletePath, parentPath); err != nil {
				return nil, fmt.Errorf("重定向虚拟机 %s 失败: %w", vm.Name, err)
			}
		}
		if err := WriteVMTemplateSource(vm.Name, parent.Name, "linked"); err != nil {
			return nil, err
		}
		rebasedVMs = append(rebasedVMs, vm.Name)
	}

	progressFn(85, "正在删除当前模板节点文件...")
	if err := deleteTemplateFiles(deleteTpl.Name); err != nil {
		return nil, err
	}

	promotedTemplates := make([]string, 0, len(preview.PromotedTemplates))
	for _, child := range preview.PromotedTemplates {
		promotedTemplates = append(promotedTemplates, child.Name)
	}
	progressFn(100, "模板节点已热删除，子节点已提升")
	return &DeleteTemplateResult{
		TemplateName:      params.TemplateName,
		DeletedTemplates:  []string{deleteTpl.Name},
		DeletedVMs:        []string{},
		PromotedTemplates: promotedTemplates,
		RebasedVMs:        rebasedVMs,
	}, nil
}

// ── Validation helpers ──

func validateTemplatePromoteHotTargets(preview *DeleteTemplatePreview, deletePath string) error {
	for _, child := range preview.PromotedTemplates {
		if strings.TrimSpace(child.Path) == "" {
			return fmt.Errorf("子模板 %s 缺少磁盘路径", child.AdminName)
		}
		if err := ensureDiskCanLeaveOldBacking(child.Path, deletePath); err != nil {
			return fmt.Errorf("子模板 %s 不满足热提升条件: %w", child.AdminName, err)
		}
	}
	for _, vm := range preview.RebasedVMs {
		diskInfo := HookGetVMDiskInfo(vm.Name)
		if strings.TrimSpace(diskInfo.Path) == "" || strings.TrimSpace(diskInfo.Device) == "" {
			return fmt.Errorf("无法获取虚拟机 %s 的系统盘路径或设备名", vm.Name)
		}
		if err := ensureDiskCanLeaveOldBacking(diskInfo.Path, deletePath); err != nil {
			return fmt.Errorf("虚拟机 %s 不满足热重定向条件: %w", vm.Name, err)
		}
	}
	for _, vm := range preview.RelatedVMs {
		if hasExternal, names, err := HookCheckVMSnapshotSafety(vm.Name); err != nil {
			return fmt.Errorf("检查虚拟机 %s 快照状态失败: %w", vm.Name, err)
		} else if hasExternal {
			return fmt.Errorf("虚拟机 %s 存在外部快照（%s），请先删除这些快照后再热提升", vm.Name, strings.Join(names, "、"))
		}
		if strings.EqualFold(strings.TrimSpace(vm.Status), "running") {
			diskInfo := HookGetVMDiskInfo(vm.Name)
			if strings.TrimSpace(diskInfo.Path) == "" || strings.TrimSpace(diskInfo.Device) == "" {
				return fmt.Errorf("无法获取运行中虚拟机 %s 的系统盘路径或设备名", vm.Name)
			}
		}
	}
	return nil
}

func validateTemplatePromoteRebaseTargets(preview *DeleteTemplatePreview, deletePath string) (map[string]string, error) {
	for _, child := range preview.PromotedTemplates {
		if strings.TrimSpace(child.Path) == "" {
			return nil, fmt.Errorf("子模板 %s 缺少磁盘路径", child.AdminName)
		}
		if err := ensureDiskCanLeaveOldBacking(child.Path, deletePath); err != nil {
			return nil, fmt.Errorf("子模板 %s 不满足 rebase 条件: %w", child.AdminName, err)
		}
	}

	vmDiskPaths := make(map[string]string, len(preview.RebasedVMs))
	for _, vm := range preview.RebasedVMs {
		diskInfo := HookGetVMDiskInfo(vm.Name)
		if strings.TrimSpace(diskInfo.Path) == "" {
			return nil, fmt.Errorf("无法获取虚拟机 %s 的系统盘路径", vm.Name)
		}
		if err := ensureDiskCanLeaveOldBacking(diskInfo.Path, deletePath); err != nil {
			return nil, fmt.Errorf("虚拟机 %s 不满足 rebase 条件: %w", vm.Name, err)
		}
		vmDiskPaths[vm.Name] = diskInfo.Path
	}
	return vmDiskPaths, nil
}

// ── Rebase / disk helpers ──

func rebaseQcow2BackingToParent(diskPath, oldParentPath, newParentPath string) error {
	diskPath = strings.TrimSpace(diskPath)
	if diskPath == "" {
		return fmt.Errorf("磁盘路径为空")
	}
	if _, err := os.Stat(diskPath); err != nil {
		return fmt.Errorf("磁盘文件不可读: %w", err)
	}
	if _, err := os.Stat(newParentPath); err != nil {
		return fmt.Errorf("目标父级模板不可读: %w", err)
	}
	if err := ensureDiskCanLeaveOldBacking(diskPath, oldParentPath); err != nil {
		return err
	}
	// qemu-img rebase 需要读写大量磁盘数据，不设置自动超时，仅依赖调用方的取消信号
	result := utils.ExecCommandNoTimeout(
		"qemu-img",
		"rebase",
		"-f", "qcow2",
		"-F", "qcow2",
		"-b", newParentPath,
		diskPath,
	)
	if result.Error != nil {
		return fmt.Errorf("安全 rebase 失败: %s", HookFirstNonEmpty(result.Stderr, result.Error.Error()))
	}
	if err := ensureDiskBackingMatches(diskPath, newParentPath); err != nil {
		return err
	}
	_ = utils.ChownLibvirtQEMU(diskPath)
	return nil
}

func ensureDiskCanLeaveOldBacking(diskPath, oldParentPath string) error {
	chain, err := HookQemuInfoChain(diskPath)
	if err != nil {
		return err
	}
	if len(chain) < 2 {
		return nil
	}
	currentBacking := HookFirstNonEmpty(chain[0].FullBackingFilename, chain[0].BackingFilename, chain[1].Filename)
	if HookSameCleanPath(currentBacking, oldParentPath) {
		return nil
	}
	return fmt.Errorf("当前 backing 为 %s，不是即将删除的模板 %s，已拒绝自动改写", currentBacking, oldParentPath)
}

func ensureDiskBackingMatches(diskPath, expectedBacking string) error {
	chain, err := HookQemuInfoChain(diskPath)
	if err != nil {
		return err
	}
	if len(chain) < 2 {
		return fmt.Errorf("rebase 后磁盘未形成链式 backing")
	}
	currentBacking := HookFirstNonEmpty(chain[0].FullBackingFilename, chain[0].BackingFilename, chain[1].Filename)
	if !HookSameCleanPath(currentBacking, expectedBacking) {
		return fmt.Errorf("rebase 后 backing 不匹配，当前为 %s，期望为 %s", currentBacking, expectedBacking)
	}
	return nil
}

func updatePromotedTemplateMeta(child TemplateInfo, parent TemplateInfo) error {
	meta := GetTemplateMeta(child.Name)
	meta.ParentNodeID = parent.NodeID
	meta.TemplateUID = parent.TemplateUID
	meta.RootNodeID = parent.RootNodeID
	if strings.TrimSpace(meta.RootNodeID) == "" {
		meta.RootNodeID = parent.NodeID
	}
	hash, err := CalculateFileHashes(child.Path)
	if err != nil {
		return err
	}
	meta.MD5 = hash.MD5
	meta.SHA256 = hash.SHA256
	meta.FileSize = hash.FileSize
	if err := saveTemplateMeta(child.Path, meta); err != nil {
		return err
	}
	// saveTemplateMeta 已将 meta.json 设为不可变，需先移除再 chown
	_ = utils.RemoveFileImmutable(getMetaPath(child.Path))
	_ = utils.ChownLibvirtQEMU(getMetaPath(child.Path))
	_ = utils.SetFileImmutable(getMetaPath(child.Path))
	return nil
}

// ── Hot promote helpers ──

func hotSwapPromotedTemplate(child TemplateInfo, parent TemplateInfo, deletePath, parentPath string, runningVMs []TemplateRelatedVM, vmBackingPaths map[string]string) error {
	tempPath := child.Path + ".promote-new-" + time.Now().Format("20060102150405")
	backupPath := child.Path + ".promote-old-" + time.Now().Format("20060102150405")
	if err := HookCopyDiskFileSparse(context.Background(), child.Path, tempPath); err != nil {
		return fmt.Errorf("复制子模板临时文件失败: %w", err)
	}
	if err := rebaseQcow2BackingToParent(tempPath, deletePath, parentPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("改写子模板临时 backing 失败: %w", err)
	}
	if err := os.Rename(child.Path, backupPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("备份原子模板文件失败: %w", err)
	}
	if err := os.Rename(tempPath, child.Path); err != nil {
		_ = os.Rename(backupPath, child.Path)
		_ = os.Remove(tempPath)
		return fmt.Errorf("替换子模板文件失败: %w", err)
	}
	_ = HookSetLibvirtDiskFileOwner(child.Path)
	if err := updatePromotedTemplateMeta(child, parent); err != nil {
		return err
	}

	for _, vm := range runningVMs {
		backingPath := strings.TrimSpace(vmBackingPaths[vm.Name])
		if backingPath == "" {
			backingPath = child.Path
		}
		if err := pivotRunningVMToTemplateBacking(vm.Name, backingPath); err != nil {
			return fmt.Errorf("运行中虚拟机 %s 切换到新子模板 backing 失败: %w", vm.Name, err)
		}
	}
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除子模板旧 backing 文件失败: %w", err)
	}
	return nil
}

func runningVMBackingPaths(preview *DeleteTemplatePreview, vms []TemplateRelatedVM, fallbackPath string) map[string]string {
	templateByNode := make(map[string]TemplateInfo, len(preview.Templates))
	for _, tpl := range preview.Templates {
		templateByNode[tpl.NodeID] = tpl
	}
	paths := make(map[string]string, len(vms))
	for _, vm := range vms {
		if tpl, ok := templateByNode[vm.NodeID]; ok && strings.TrimSpace(tpl.Path) != "" {
			paths[vm.Name] = tpl.Path
		} else {
			paths[vm.Name] = fallbackPath
		}
	}
	return paths
}

func runningVMsForTemplateSubtree(preview *DeleteTemplatePreview, nodeID string) []TemplateRelatedVM {
	templateByNode := make(map[string]TemplateInfo, len(preview.Templates))
	for _, tpl := range preview.Templates {
		templateByNode[tpl.NodeID] = tpl
	}
	var vms []TemplateRelatedVM
	for _, vm := range preview.RelatedVMs {
		if strings.EqualFold(strings.TrimSpace(vm.Status), "running") && templateNodeInSubtree(vm.NodeID, nodeID, templateByNode) {
			vms = append(vms, vm)
		}
	}
	return vms
}

func templateNodeInSubtree(nodeID, rootNodeID string, templateByNode map[string]TemplateInfo) bool {
	nodeID = strings.TrimSpace(nodeID)
	rootNodeID = strings.TrimSpace(rootNodeID)
	for nodeID != "" {
		if nodeID == rootNodeID {
			return true
		}
		tpl, ok := templateByNode[nodeID]
		if !ok {
			return false
		}
		nodeID = strings.TrimSpace(tpl.ParentNodeID)
	}
	return false
}

func pivotRunningVMToTemplateBacking(vmName, backingPath string) error {
	diskInfo := HookGetVMDiskInfo(vmName)
	if strings.TrimSpace(diskInfo.Path) == "" || strings.TrimSpace(diskInfo.Device) == "" {
		return fmt.Errorf("无法获取系统盘路径或设备名")
	}
	chain, err := HookQemuInfoChain(diskInfo.Path)
	if err != nil {
		return fmt.Errorf("无法读取活动硬盘容量: %w", err)
	}
	if len(chain) == 0 || chain[0].VirtualSize <= 0 {
		return fmt.Errorf("无法读取活动硬盘容量")
	}
	targetPath, err := nextHotPromoteDiskPath(diskInfo.Path)
	if err != nil {
		return err
	}
	createCmd := "qemu-img create -f qcow2 -F qcow2 -b " + utils.ShellSingleQuote(backingPath) + " " + utils.ShellSingleQuote(targetPath) + " " + strconv.FormatInt(chain[0].VirtualSize, 10)
	createResult := utils.ExecShellNoTimeout(createCmd)
	if createResult.Error != nil {
		return fmt.Errorf("创建热切换目标 overlay 失败: %s", HookFirstNonEmpty(createResult.Stderr, createResult.Error.Error()))
	}
	_ = HookSetLibvirtDiskFileOwner(targetPath)
	cmd := strings.Join([]string{
		"virsh blockcopy",
		utils.ShellSingleQuote(vmName),
		utils.ShellSingleQuote(diskInfo.Device),
		"--dest", utils.ShellSingleQuote(targetPath),
		"--format qcow2",
		"--wait --verbose --pivot --transient-job --shallow --reuse-external",
	}, " ")
	// virsh blockcopy 会复制整块磁盘数据，属于大 IO 操作，不设置自动超时
	result := utils.ExecShellNoTimeout(cmd)
	if result.Error != nil {
		_ = utils.ExecCommand("virsh", "blockjob", vmName, diskInfo.Device, "--abort", "--async")
		_ = os.Remove(targetPath)
		return fmt.Errorf("热切换运行中 VM backing 失败: %s", HookFirstNonEmpty(result.Stderr, result.Error.Error()))
	}
	if err := HookUpdateInactiveDomainDiskPath(vmName, diskInfo.Path, targetPath); err != nil {
		return err
	}
	if err := os.Remove(diskInfo.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("热切换已完成，但删除旧活动 overlay 失败: %w", err)
	}
	return nil
}

func blockPullRunningVMToBase(vmName, device, diskPath, basePath string) error {
	cmd := strings.Join([]string{
		"virsh blockpull",
		utils.ShellSingleQuote(vmName),
		utils.ShellSingleQuote(device),
		"--base", utils.ShellSingleQuote(basePath),
		"--wait --verbose",
	}, " ")
	// virsh blockpull 会复制整块磁盘数据，属于大 IO 操作，不设置自动超时
	result := utils.ExecShellNoTimeout(cmd)
	if result.Error != nil {
		return fmt.Errorf("运行中 VM 在线拉平到上级模板失败: %s", HookFirstNonEmpty(result.Stderr, result.Error.Error()))
	}
	if err := ensureDiskBackingMatches(diskPath, basePath); err != nil {
		return err
	}
	return nil
}

func nextHotPromoteDiskPath(sourcePath string) (string, error) {
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	nameOnly := strings.TrimSuffix(base, ext)
	if ext == "" {
		ext = ".qcow2"
	}
	stamp := time.Now().Format("20060102150405")
	for i := 0; i < 100; i++ {
		suffix := stamp
		if i > 0 {
			suffix += "-" + strconv.Itoa(i)
		}
		candidate := filepath.Join(dir, nameOnly+"_hotpromote_"+suffix+ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("检查热删除目标硬盘路径失败: %w", err)
		}
	}
	return "", fmt.Errorf("无法生成不冲突的热删除目标硬盘路径")
}
