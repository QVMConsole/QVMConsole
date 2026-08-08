package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// PrepareTemplate creates a template from a VM (async task logic).
func PrepareTemplate(params *PrepareTemplateParams, progressFn func(int, string)) error {
	templateDir := config.GlobalConfig.TemplateDir
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		return fmt.Errorf("创建模板目录失败: %w", err)
	}
	if err := ValidateTemplateName(params.TemplateName); err != nil {
		return err
	}

	diskInfo := HookGetVMDiskInfo(params.VMName)
	if strings.TrimSpace(diskInfo.Path) == "" {
		return fmt.Errorf("无法获取虚拟机 %s 的磁盘路径", params.VMName)
	}

	state, err := libvirt_rpc.GetDomainStateRPC(params.VMName)
	if err != nil {
		return fmt.Errorf("获取虚拟机状态失败: %w", err)
	}
	if state == "running" {
		return fmt.Errorf("虚拟机正在运行，请先关机再制作模板")
	}

	destPath := filepath.Join(templateDir, params.TemplateName+".qcow2")
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("模板已存在: %s", params.TemplateName)
	}
	// 制作模板复制磁盘属于大 IO 操作，不设置自动超时
	result := utils.ExecCommandNoTimeout("cp", "--sparse=always", diskInfo.Path, destPath)
	if result.Error != nil {
		return fmt.Errorf("复制磁盘失败: %s", result.Stderr)
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
	adminName := strings.TrimSpace(params.TemplateName)
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		displayName = adminName
	}

	bootType := DetectVMBootType(params.VMName)
	if bootType == "" {
		bootType = DetectTemplateBootType(destPath)
	}
	defaultConfig := collectVMTemplateDefaultConfig(params.VMName)
	meta := &TemplateMeta{
		Type:             tplType,
		Category:         normalizeTemplateCategoryForName(tplType, params.Category, params.TemplateName),
		BootType:         bootType,
		RootPassword:     params.RootPassword,
		TemplateUser:     params.TemplateUser,
		CloudInitMode:    params.CloudInitMode,
		PostBootCommand:  params.PostBootCommand,
		PostBootBlocking: params.PostBootBlocking,
		DefaultConfig:    defaultConfig,
		NodeID:           generateTemplateID("node"),
		AdminName:        adminName,
		DisplayName:      displayName,
		CreatedFromVM:    params.VMName,
		CreatedAt:        time.Now().Format(time.RFC3339),
	}
	if bootType == "uefi" {
		meta.NVRAMPath = copyTemplateNVRAMFromVM(params.VMName, destPath)
	}

	if sourceTpl, err := resolveSourceTemplateForVM(params.VMName, diskInfo.Template); err == nil && sourceTpl != nil {
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

	// Linux 模板：预装 cloud-init 和 growpart 依赖（制作时一次性安装，克隆时直接使用）
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
		_ = os.Remove(destPath)
		return err
	}
	meta.MD5 = hash.MD5
	meta.SHA256 = hash.SHA256
	meta.FileSize = hash.FileSize

	if err := saveTemplateMeta(destPath, meta); err != nil {
		_ = os.Remove(destPath)
		return err
	}
	_ = utils.ChownLibvirtQEMU(destPath)
	// saveTemplateMeta 已将 meta.json 设为不可变，需先移除再 chown
	_ = utils.RemoveFileImmutable(getMetaPath(destPath))
	_ = utils.ChownLibvirtQEMU(getMetaPath(destPath))
	// 设置模板文件为不可变，防止误删
	_ = utils.SetFileImmutable(destPath)
	_ = utils.SetFileImmutable(getMetaPath(destPath))
	return nil
}

func resolveSourceTemplateForVM(vmName, fallbackTemplateName string) (*TemplateInfo, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	if source := ReadVMTemplateSource(vmName); source != nil {
		// 完整克隆的 VM 与模板之间没有 backing chain 依赖，其模板应为独立根节点
		if source.CloneMode == "full" {
			return nil, fmt.Errorf("VM 是完整克隆，不继承模板父子关系")
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
	return nil, fmt.Errorf("未找到来源模板")
}
