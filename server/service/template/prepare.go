package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// PrepareTemplate creates a template from a VM (async task logic).
func PrepareTemplate(params *PrepareTemplateParams, progressFn func(int, string)) (retErr error) {
	if params == nil {
		return fmt.Errorf("制作模板参数为空")
	}
	if progressFn == nil {
		progressFn = func(int, string) {}
	}

	params.TransferMode = normalizeTemplateTransferMode(params.TransferMode)
	if params.TransferMode == "" {
		return fmt.Errorf("磁盘处理方式仅支持 copy 或 move")
	}
	if params.Compress && params.TransferMode == TemplateTransferModeMove {
		return fmt.Errorf("移动磁盘时固定为不压缩")
	}

	templateDir := config.GlobalConfig.TemplateDir
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		return fmt.Errorf("创建模板目录失败: %w", err)
	}
	if err := ValidateTemplateName(params.TemplateName); err != nil {
		return err
	}

	diskInfo := HookGetVMDiskInfo(params.VMName)
	if strings.TrimSpace(diskInfo.Path) == "" {
		return fmt.Errorf("未获取到虚拟机 %s 的系统磁盘路径", params.VMName)
	}
	if _, err := os.Stat(diskInfo.Path); err != nil {
		return fmt.Errorf("虚拟机系统磁盘不存在: %s", diskInfo.Path)
	}

	state, err := libvirt_rpc.GetDomainStateRPC(params.VMName)
	if err != nil {
		return fmt.Errorf("获取虚拟机状态失败: %w", err)
	}
	if state != "shut off" {
		return fmt.Errorf("虚拟机必须处于关机状态才能制作模板，当前状态: %s", state)
	}

	tplType := normalizeTemplateType(params.Type)
	if tplType == "" {
		tplType = "linux"
	}
	// 其它模板禁止写入任何来宾初始化配置。
	if tplType == "other" {
		params.CloudInitMode = "none"
		params.TemplateUser = ""
		params.PostBootCommand = ""
		params.PostBootBlocking = false
	}
	if err := ValidateTemplateCategory(tplType, params.Category); err != nil {
		return err
	}

	// 在移动磁盘或删除源虚拟机前收集全部 VM 信息，避免移动后 XML 中的旧路径影响识别。
	bootType := DetectVMBootType(params.VMName)
	defaultConfig := collectVMTemplateDefaultConfig(params.VMName)
	sourceTpl, err := resolveSourceTemplateForVM(params.VMName, diskInfo.Template, diskInfo.Path)
	if err != nil {
		return fmt.Errorf("解析源虚拟机模板层级失败: %w", err)
	}

	destPath := filepath.Join(templateDir, params.TemplateName+".qcow2")
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("模板已存在: %s", params.TemplateName)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查模板目标路径失败: %w", err)
	}

	diskMoved := false
	deleteSourceStarted := false
	nvramPath := ""
	defer func() {
		if retErr == nil || deleteSourceStarted {
			return
		}
		_ = utils.RemoveFileImmutable(destPath)
		_ = utils.RemoveFileImmutable(getMetaPath(destPath))
		_ = os.Remove(getMetaPath(destPath))
		if nvramPath != "" {
			_ = utils.RemoveFileImmutable(nvramPath)
			_ = os.Remove(nvramPath)
		}
		if diskMoved {
			rollback := utils.ExecCommandNoTimeout("mv", "--", destPath, diskInfo.Path)
			if rollback.Error != nil {
				retErr = fmt.Errorf("%w；源磁盘回滚失败: %s", retErr, commandErrorText(rollback))
			}
			return
		}
		_ = os.Remove(destPath)
	}()

	progressFn(20, templateDiskTransferProgressText(params))
	switch {
	case params.Compress:
		args := []string{"convert", "-c", "-O", "qcow2"}
		if sourceTpl != nil {
			backingPath, verified, err := getDirectBackingPath(diskInfo.Path)
			if err != nil || !verified || !sameTemplatePath(backingPath, sourceTpl.Path) {
				return fmt.Errorf("压缩子模板前无法确认源磁盘与父模板的 backing 关系，请先检查磁盘链")
			}
			// 压缩输出继续以原父模板为 backing，保持模板族的实际磁盘链和元数据层级一致。
			args = append(args, "-B", sourceTpl.Path, "-F", "qcow2")
		}
		args = append(args, diskInfo.Path, destPath)
		result := utils.ExecCommandNoTimeout("qemu-img", args...)
		if result.Error != nil {
			return fmt.Errorf("压缩模板磁盘失败: %s", commandErrorText(result))
		}
	case params.TransferMode == TemplateTransferModeMove:
		result := utils.ExecCommandNoTimeout("mv", "--", diskInfo.Path, destPath)
		if result.Error != nil {
			return fmt.Errorf("移动模板磁盘失败: %s", commandErrorText(result))
		}
		diskMoved = true
	default:
		result := utils.ExecCommandNoTimeout("cp", "--sparse=always", "--", diskInfo.Path, destPath)
		if result.Error != nil {
			return fmt.Errorf("复制模板磁盘失败: %s", commandErrorText(result))
		}
	}

	if bootType == "" {
		bootType = DetectTemplateBootType(destPath)
	}
	adminName := strings.TrimSpace(params.TemplateName)
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		displayName = adminName
	}
	meta := &TemplateMeta{
		Type:               tplType,
		Category:           normalizeTemplateCategoryForName(tplType, params.Category, params.TemplateName),
		BootType:           bootType,
		RootPassword:       params.RootPassword,
		TemplateUser:       params.TemplateUser,
		CloudInitMode:      params.CloudInitMode,
		PostBootCommand:    params.PostBootCommand,
		PostBootBlocking:   params.PostBootBlocking,
		DefaultConfig:      defaultConfig,
		NodeID:             generateTemplateID("node"),
		AdminName:          adminName,
		DisplayName:        displayName,
		CreatedFromVM:      params.VMName,
		CreatedAt:          time.Now().Format(time.RFC3339),
		DiskCompressed:     params.Compress,
		SourceTransferMode: params.TransferMode,
	}
	if bootType == "uefi" {
		nvramPath = copyTemplateNVRAMFromVM(params.VMName, destPath)
		meta.NVRAMPath = nvramPath
	}

	if sourceTpl != nil {
		meta.TemplateUID = sourceTpl.TemplateUID
		meta.ParentNodeID = sourceTpl.NodeID
		meta.RootNodeID = sourceTpl.RootNodeID
		if meta.RootNodeID == "" {
			meta.RootNodeID = sourceTpl.NodeID
		}
		meta.CloneVisible = false
	} else {
		meta.TemplateUID = generateTemplateID("tpl")
		meta.RootNodeID = meta.NodeID
		meta.CloneVisible = true
	}

	// Linux 模板：预装 cloud-init 和 growpart 依赖（制作时一次性安装，克隆时直接使用）。
	if tplType == "linux" {
		progressFn(50, "安装必要依赖...")
		if err := EnsureLinuxCloudInitDeps(destPath); err != nil {
			updateLinuxInitStatus(meta, err)
		} else {
			updateLinuxInitStatus(meta, nil)
		}
	}

	progressFn(70, "计算模板校验和...")
	hash, err := CalculateFileHashes(destPath)
	if err != nil {
		return err
	}
	meta.MD5 = hash.MD5
	meta.SHA256 = hash.SHA256
	meta.FileSize = hash.FileSize

	if err := saveTemplateMeta(destPath, meta); err != nil {
		return err
	}
	_ = utils.ChownLibvirtQEMU(destPath)
	// saveTemplateMeta 已将 meta.json 设为不可变，需先移除再 chown。
	_ = utils.RemoveFileImmutable(getMetaPath(destPath))
	_ = utils.ChownLibvirtQEMU(getMetaPath(destPath))
	_ = utils.SetFileImmutable(destPath)
	_ = utils.SetFileImmutable(getMetaPath(destPath))

	if params.TransferMode == TemplateTransferModeMove {
		progressFn(92, "模板已保存，正在删除源虚拟机...")
		deleteSourceStarted = true
		owner := ""
		if HookFindVMOwner != nil {
			owner = HookFindVMOwner(params.VMName)
		}
		if HookDeleteVM == nil {
			return fmt.Errorf("模板磁盘已移动并保存，但虚拟机删除组件未初始化")
		}
		if err := HookDeleteVM(params.VMName); err != nil {
			return fmt.Errorf("模板磁盘已移动并保存，但删除源虚拟机失败: %w", err)
		}
		if HookFinalizeDeletedVM != nil {
			HookFinalizeDeletedVM(params.VMName)
		}
		if owner != "" && HookRemoveVMFromUser != nil {
			if err := HookRemoveVMFromUser(owner, params.VMName); err != nil {
				logger.App.Warn("移动制作模板后移除用户虚拟机授权失败", "owner", owner, "vm", params.VMName, "error", err)
			}
		}
		if owner != "" && HookRebalanceUserBandwidth != nil {
			if err := HookRebalanceUserBandwidth(owner); err != nil {
				logger.App.Warn("移动制作模板后重新分配用户带宽失败", "owner", owner, "error", err)
			}
		}
	}
	return nil
}

func normalizeTemplateTransferMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", TemplateTransferModeCopy:
		return TemplateTransferModeCopy
	case TemplateTransferModeMove:
		return TemplateTransferModeMove
	default:
		return ""
	}
}

func templateDiskTransferProgressText(params *PrepareTemplateParams) string {
	if params.Compress {
		return "正在压缩模板磁盘..."
	}
	if params.TransferMode == TemplateTransferModeMove {
		return "正在移动模板磁盘..."
	}
	return "正在复制模板磁盘..."
}

func commandErrorText(result *utils.CmdResult) string {
	if result == nil {
		return "命令未返回结果"
	}
	if message := strings.TrimSpace(result.Stderr); message != "" {
		return message
	}
	if result.Error != nil {
		return result.Error.Error()
	}
	return "未知错误"
}

func sameTemplatePath(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	if HookSameCleanPath != nil {
		return HookSameCleanPath(left, right)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func getDirectBackingPath(diskPath string) (string, bool, error) {
	if HookQemuInfoChain == nil {
		return "", false, fmt.Errorf("磁盘链查询组件未初始化")
	}
	chain, err := HookQemuInfoChain(diskPath)
	if err != nil {
		return "", false, err
	}
	if len(chain) == 0 {
		return "", true, nil
	}
	backingPath := strings.TrimSpace(chain[0].FullBackingFilename)
	if backingPath == "" {
		backingPath = strings.TrimSpace(chain[0].BackingFilename)
	}
	if backingPath != "" && !filepath.IsAbs(backingPath) {
		backingPath = filepath.Join(filepath.Dir(diskPath), backingPath)
	}
	return backingPath, true, nil
}

func resolveSourceTemplateForVM(vmName, fallbackTemplateName, diskPath string) (*TemplateInfo, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	// 以 qcow2 的直接 backing 为准，避免来源元数据滞后时写出错误的父子节点关系。
	if backingPath, verified, chainErr := getDirectBackingPath(diskPath); chainErr == nil && verified {
		if backingPath == "" {
			return nil, nil
		}
		for _, tpl := range tree.templates {
			if sameTemplatePath(backingPath, tpl.Path) {
				matched := tpl
				return &matched, nil
			}
		}
		return nil, fmt.Errorf("VM 系统盘的 backing 未匹配到现有模板")
	}

	if source := ReadVMTemplateSource(vmName); source != nil {
		if source.CloneMode == "full" {
			return nil, nil
		}
		if source.NodeID != "" {
			if tpl, ok := tree.byNodeID[source.NodeID]; ok {
				return &tpl, nil
			}
		}
		if source.TemplateName != "" {
			if tpl, ok := tree.byName[source.TemplateName]; ok {
				return &tpl, nil
			}
		}
	}
	if fallbackTemplateName != "" {
		if tpl, ok := tree.byName[fallbackTemplateName]; ok {
			return &tpl, nil
		}
	}
	return nil, nil
}
