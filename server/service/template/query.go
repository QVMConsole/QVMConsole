package template

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"kvm_console/config"
)

// GetTemplateMeta returns the normalized metadata for a template by name.
func GetTemplateMeta(templateName string) *TemplateMeta {
	if config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.TemplateDir) != "" {
		templatePath := filepath.Join(config.GlobalConfig.TemplateDir, templateName+".qcow2")
		if meta := loadTemplateMeta(templatePath); meta != nil {
			normalized := normalizeLoadedTemplateMeta(templateName, templatePath, meta, true)
			return &normalized
		}
		bootType, bootVerified := resolveTemplateBootType(templatePath, detectTemplateTypeFromName(templateName), "", false, DetectTemplateBootType)
		return &TemplateMeta{
			Type:         detectTemplateTypeFromName(templateName),
			Category:     normalizeTemplateCategoryForName(detectTemplateTypeFromName(templateName), "", templateName),
			BootType:     bootType,
			BootVerified: bootVerified,
		}
	}
	detectedType := detectTemplateTypeFromName(templateName)
	return &TemplateMeta{
		Type:     detectedType,
		Category: normalizeTemplateCategoryForName(detectedType, "", templateName),
	}
}

// GetTemplateInfoByName returns template info by name.
func GetTemplateInfoByName(templateName string) (*TemplateInfo, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	tpl, ok := tree.byName[templateName]
	if !ok {
		return nil, fmt.Errorf("模板不存在: %s", templateName)
	}
	return &tpl, nil
}

// GetTemplateInfoByNodeID returns template info by node ID.
func GetTemplateInfoByNodeID(nodeID string) (*TemplateInfo, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	tpl, ok := tree.byNodeID[nodeID]
	if !ok {
		return nil, fmt.Errorf("模板节点不存在: %s", nodeID)
	}
	return &tpl, nil
}

// GetTemplateMinDiskSizeGB returns the minimum disk size in GB for a template.
func GetTemplateMinDiskSizeGB(templateName string) (int, error) {
	templatePath, err := EnsureTemplatePath(templateName)
	if err != nil {
		return 0, err
	}
	result := templateDiskInfoCommand(templatePath)
	if result.Error != nil {
		return 0, fmt.Errorf("获取模板磁盘信息失败: %s", result.Stderr)
	}
	var info struct {
		VirtualSize int64 `json:"virtual-size"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &info); err != nil {
		return 0, fmt.Errorf("解析模板磁盘信息失败: %v", err)
	}
	if info.VirtualSize <= 0 {
		return 0, fmt.Errorf("模板磁盘大小无效: %s", templateName)
	}
	return int(math.Ceil(float64(info.VirtualSize) / float64(1<<30))), nil
}

// NormalizeRequestedDiskSize normalizes a requested disk size against a minimum.
func NormalizeRequestedDiskSize(requestedDiskSize, minDiskSize int) int {
	if minDiskSize <= 0 {
		return requestedDiskSize
	}
	if requestedDiskSize <= 0 || requestedDiskSize < minDiskSize {
		return minDiskSize
	}
	return requestedDiskSize
}

// ResolveCloneDiskSizeGB resolves the disk size for cloning from a template.
func ResolveCloneDiskSizeGB(templateName string, requestedDiskSize int) (int, error) {
	minDiskSize, err := GetTemplateMinDiskSizeGB(templateName)
	if err != nil {
		return 0, err
	}
	return NormalizeRequestedDiskSize(requestedDiskSize, minDiskSize), nil
}

// EnsureTemplateVisibleForClone checks if a template is visible for cloning.
func EnsureTemplateVisibleForClone(templateName string, isAdmin bool) error {
	if isAdmin {
		tpl, err := GetTemplateInfoByName(templateName)
		if err != nil {
			return err
		}
		if tpl.Disabled {
			return fmt.Errorf("该模板已禁用，无法克隆")
		}
		return nil
	}
	tpl, err := GetTemplateInfoByName(templateName)
	if err != nil {
		return err
	}
	if tpl.Disabled {
		return fmt.Errorf("该模板已禁用，无法克隆")
	}
	if !tpl.CloneVisible {
		return fmt.Errorf("该模板当前未开放克隆")
	}
	return nil
}
