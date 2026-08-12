package public_ip

import (
	"fmt"

	"kvm_console/model"
)

func ListPublicIPs() ([]PublicIPInfo, error) {
	var rows []model.PublicIP
	if err := model.DB.Order("ip ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var bindings []model.PublicIPBinding
	if err := model.DB.Find(&bindings).Error; err != nil {
		return nil, err
	}
	byIPID := map[uint]model.PublicIPBinding{}
	for _, binding := range bindings {
		byIPID[binding.PublicIPID] = binding
	}

	out := make([]PublicIPInfo, 0, len(rows))
	for _, row := range rows {
		modes := parsePublicIPModes(row.SupportedModes)
		info := PublicIPInfo{
			PublicIP:      row,
			AddressFamily: publicIPAddressFamily(row.IP),
			Modes:         modes,
			ModeLabels:    publicIPModeLabels(modes),
		}
		if binding, ok := byIPID[row.ID]; ok {
			copyBinding := binding
			info.Binding = &copyBinding
			info.RuntimeRules = publicIPRuntimeRuleSummary(row, binding)
		}
		out = append(out, info)
	}
	return out, nil
}

func CreatePublicIP(req PublicIPRequest) (*model.PublicIP, error) {
	row, err := normalizePublicIPRequest(req, nil)
	if err != nil {
		return nil, err
	}
	if row.Status == PublicIPStatusBound {
		return nil, fmt.Errorf("新增公网 IP 不能直接设置为已绑定")
	}
	if row.Status == "" {
		row.Status = PublicIPStatusFree
	}
	if err := model.DB.Create(row).Error; err != nil {
		return nil, fmt.Errorf("创建公网 IP 失败: %w", err)
	}
	return row, nil
}

func UpdatePublicIP(id uint, req PublicIPRequest) (*model.PublicIP, error) {
	var current model.PublicIP
	if err := model.DB.First(&current, id).Error; err != nil {
		return nil, fmt.Errorf("公网 IP 不存在")
	}
	row, err := normalizePublicIPRequest(req, &current)
	if err != nil {
		return nil, err
	}
	row.ID = current.ID
	var bindingCount int64
	model.DB.Model(&model.PublicIPBinding{}).Where("public_ip_id = ?", current.ID).Count(&bindingCount)
	if bindingCount > 0 && row.IP != current.IP {
		return nil, fmt.Errorf("公网 IP 已绑定，不能修改 IP 地址")
	}
	if bindingCount > 0 {
		row.Status = PublicIPStatusBound
	}
	if err := model.DB.Model(&current).Updates(map[string]interface{}{
		"ip":              row.IP,
		"cidr":            row.CIDR,
		"gateway":         row.Gateway,
		"uplink_if":       row.UplinkIF,
		"supported_modes": row.SupportedModes,
		"status":          row.Status,
		"remark":          row.Remark,
	}).Error; err != nil {
		return nil, fmt.Errorf("更新公网 IP 失败: %w", err)
	}
	if row.IP != current.IP {
		model.DB.Model(&model.PublicIPBinding{}).Where("public_ip_id = ?", current.ID).Update("public_ip", row.IP)
	}
	if err := model.DB.First(&current, id).Error; err != nil {
		return nil, err
	}
	return &current, nil
}

func DeletePublicIP(id uint) error {
	var count int64
	model.DB.Model(&model.PublicIPBinding{}).Where("public_ip_id = ?", id).Count(&count)
	if count > 0 {
		return fmt.Errorf("公网 IP 已绑定，请先解绑后再删除")
	}
	if err := model.DB.Delete(&model.PublicIP{}, id).Error; err != nil {
		return fmt.Errorf("删除公网 IP 失败: %w", err)
	}
	return nil
}
