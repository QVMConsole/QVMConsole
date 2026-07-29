package vm

import (
	"encoding/xml"
	"fmt"
	"strings"
	"unicode/utf8"

	"kvm_console/utils"
)

type vmConfigMetadata struct {
	XMLName xml.Name `xml:"config"`
	XMLNS   string   `xml:"xmlns,attr,omitempty"`
	Freeze  string   `xml:"freeze,attr,omitempty"`
	Remark  string   `xml:"remark,omitempty"`
	Group   string   `xml:"group,omitempty"`
	Tags    []string `xml:"tags>tag,omitempty"`
}

const (
	maxVMTags     = 20
	maxVMTagRunes = 32
)

func readVMConfigMetadata(name string) (*vmConfigMetadata, error) {
	result := utils.ExecCommand("virsh", "metadata", name, vmConfigMetadataURI, "--config")
	if result.Error != nil {
		errText := strings.ToLower(strings.TrimSpace(result.Stderr + "\n" + result.Stdout))
		if strings.Contains(errText, "metadata not found") || strings.Contains(errText, "no metadata") {
			return &vmConfigMetadata{}, nil
		}
		return nil, fmt.Errorf("读取虚拟机配置元数据失败: %s", strings.TrimSpace(result.Stderr))
	}

	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		return &vmConfigMetadata{}, nil
	}

	var metadata vmConfigMetadata
	if err := xml.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("解析虚拟机配置元数据失败: %w", err)
	}
	return &metadata, nil
}

func writeVMConfigMetadata(name string, metadata *vmConfigMetadata) error {
	if metadata == nil {
		return removeVMConfigMetadata(name)
	}

	metadata.XMLNS = vmConfigMetadataURI
	metadata.Remark = strings.TrimSpace(metadata.Remark)
	metadata.Group = strings.TrimSpace(metadata.Group)
	normalizedTags, err := normalizeVMTags(metadata.Tags)
	if err != nil {
		return err
	}
	metadata.Tags = normalizedTags
	if metadata.Freeze != "yes" {
		metadata.Freeze = ""
	}

	if metadata.Freeze == "" && metadata.Remark == "" && metadata.Group == "" && len(metadata.Tags) == 0 {
		return removeVMConfigMetadata(name)
	}

	xmlBytes, err := xml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("序列化虚拟机配置元数据失败: %w", err)
	}

	result := utils.ExecCommand(
		"virsh", "metadata", name, vmConfigMetadataURI,
		"--config", "--key", vmConfigMetadataKey, "--set", string(xmlBytes),
	)
	if result.Error != nil {
		return fmt.Errorf("写入虚拟机配置元数据失败: %s", strings.TrimSpace(result.Stderr))
	}
	return nil
}

func normalizeVMTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > maxVMTagRunes {
			return nil, fmt.Errorf("单个标签不能超过 %d 个字符", maxVMTagRunes)
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, tag)
		if len(normalized) > maxVMTags {
			return nil, fmt.Errorf("每台虚拟机最多设置 %d 个标签", maxVMTags)
		}
	}
	return normalized, nil
}

func removeVMConfigMetadata(name string) error {
	result := utils.ExecCommand("virsh", "metadata", name, vmConfigMetadataURI, "--config", "--remove")
	if result.Error != nil {
		errText := strings.ToLower(strings.TrimSpace(result.Stderr + "\n" + result.Stdout))
		if strings.Contains(errText, "metadata not found") || strings.Contains(errText, "no metadata") {
			return nil
		}
		return fmt.Errorf("删除虚拟机配置元数据失败: %s", strings.TrimSpace(result.Stderr))
	}
	return nil
}

func metadataFreezeEnabled(metadata *vmConfigMetadata) bool {
	if metadata == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(metadata.Freeze), "yes")
}

// GetVMRemark 获取虚拟机备注。
func GetVMRemark(name string) (string, error) {
	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(metadata.Remark), nil
}

// SetVMRemark 设置虚拟机备注。
func SetVMRemark(name, remark string) error {
	if err := D.HookEnsureVMNotMigrating(name, "设置虚拟机备注"); err != nil {
		return err
	}

	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return err
	}
	metadata.Remark = strings.TrimSpace(remark)
	if err := writeVMConfigMetadata(name, metadata); err != nil {
		return err
	}
	RefreshVMCacheByNameAsync(name)
	return nil
}

// GetVMGroup 获取虚拟机分组。
func GetVMGroup(name string) (string, error) {
	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(metadata.Group), nil
}

// SetVMGroup 设置虚拟机分组。
func SetVMGroup(name, group string) error {
	if err := D.HookEnsureVMNotMigrating(name, "设置虚拟机分组"); err != nil {
		return err
	}

	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return err
	}
	metadata.Group = strings.TrimSpace(group)
	if err := writeVMConfigMetadata(name, metadata); err != nil {
		return err
	}
	RefreshVMCacheByNameAsync(name)
	return nil
}

// GetVMTags 获取虚拟机标签。
func GetVMTags(name string) ([]string, error) {
	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return nil, err
	}
	return normalizeVMTags(metadata.Tags)
}

// SetVMTags 设置虚拟机标签。
func SetVMTags(name string, tags []string) error {
	if err := D.HookEnsureVMNotMigrating(name, "设置虚拟机标签"); err != nil {
		return err
	}

	normalized, err := normalizeVMTags(tags)
	if err != nil {
		return err
	}
	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return err
	}
	metadata.Tags = normalized
	if err := writeVMConfigMetadata(name, metadata); err != nil {
		return err
	}
	RefreshVMCacheByNameAsync(name)
	return nil
}
