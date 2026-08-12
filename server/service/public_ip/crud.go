package public_ip

import (
	"fmt"
	"strings"

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

// BatchCreatePublicIPs 批量新增公网 IP。
// 共用除 IP 外的字段；逐条独立校验与写入，部分失败不影响其他 IP。
// 批内重复或数据库已存在的 IP 会被跳过，不会覆盖已有记录。
func BatchCreatePublicIPs(req PublicIPBatchRequest) (*PublicIPBatchResult, error) {
	if model.DB == nil {
		return nil, fmt.Errorf("数据库尚未初始化")
	}
	if len(req.IPs) == 0 {
		return nil, fmt.Errorf("请至少提供一个公网 IP")
	}
	if len(req.IPs) > 1000 {
		return nil, fmt.Errorf("单次批量新增数量不能超过 1000")
	}

	// 预处理：去除空白行与首尾空格
	normalizedIPs := make([]string, 0, len(req.IPs))
	for _, raw := range req.IPs {
		ip := strings.TrimSpace(raw)
		if ip != "" {
			normalizedIPs = append(normalizedIPs, ip)
		}
	}
	if len(normalizedIPs) == 0 {
		return nil, fmt.Errorf("请至少提供一个公网 IP")
	}

	// 一次性查询批内 IP 是否已存在，避免逐条查询
	var existingRows []model.PublicIP
	if err := model.DB.Where("ip IN ?", normalizedIPs).Find(&existingRows).Error; err != nil {
		return nil, fmt.Errorf("查询已有公网 IP 失败: %w", err)
	}
	existing := make(map[string]bool, len(existingRows))
	for _, row := range existingRows {
		existing[strings.TrimSpace(row.IP)] = true
	}

	result := &PublicIPBatchResult{Items: make([]PublicIPBatchItemResult, 0, len(normalizedIPs))}
	seen := make(map[string]bool, len(normalizedIPs))

	for _, ip := range normalizedIPs {
		if seen[ip] {
			result.Skipped++
			result.Items = append(result.Items, PublicIPBatchItemResult{IP: ip, Status: PublicIPBatchItemSkipped, Reason: "批内重复"})
			continue
		}
		seen[ip] = true
		if existing[ip] {
			result.Skipped++
			result.Items = append(result.Items, PublicIPBatchItemResult{IP: ip, Status: PublicIPBatchItemSkipped, Reason: "该公网 IP 已存在"})
			continue
		}

		// 复用单条新增的校验逻辑，按地址族独立校验模式与网关
		row, err := normalizePublicIPRequest(PublicIPRequest{
			IP:             ip,
			CIDR:           req.CIDR,
			Gateway:        req.Gateway,
			UplinkIF:       req.UplinkIF,
			SupportedModes: req.SupportedModes,
			Status:         req.Status,
			Remark:         req.Remark,
		}, nil)
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, PublicIPBatchItemResult{IP: ip, Status: PublicIPBatchItemFailed, Reason: err.Error()})
			continue
		}
		if row.Status == PublicIPStatusBound {
			result.Failed++
			result.Items = append(result.Items, PublicIPBatchItemResult{IP: ip, Status: PublicIPBatchItemFailed, Reason: "新增公网 IP 不能直接设置为已绑定"})
			continue
		}
		if row.Status == "" {
			row.Status = PublicIPStatusFree
		}
		if err := model.DB.Create(row).Error; err != nil {
			result.Failed++
			result.Items = append(result.Items, PublicIPBatchItemResult{IP: ip, Status: PublicIPBatchItemFailed, Reason: "创建失败: " + err.Error()})
			continue
		}
		existing[row.IP] = true
		result.Created++
		result.Items = append(result.Items, PublicIPBatchItemResult{IP: ip, Status: PublicIPBatchItemCreated, Row: row})
	}
	return result, nil
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

// BatchDeletePublicIPs 批量删除公网 IP。
// 已绑定的 IP 自动跳过（不会删除），其他 IP 逐条删除；部分失败不影响其他 IP。
// 返回每条 IP 的处理结果与汇总。
func BatchDeletePublicIPs(ids []uint) (*PublicIPBatchOpSummary, error) {
	if model.DB == nil {
		return nil, fmt.Errorf("数据库尚未初始化")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("请至少选择一个公网 IP")
	}
	if len(ids) > 1000 {
		return nil, fmt.Errorf("单次批量删除数量不能超过 1000")
	}

	// 去重，保持稳定顺序
	seen := make(map[uint]bool, len(ids))
	uniqueIDs := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		uniqueIDs = append(uniqueIDs, id)
	}

	// 一次性查询所有 IP 行，便于返回 IP 文本与判断绑定状态
	var rows []model.PublicIP
	if err := model.DB.Where("id IN ?", uniqueIDs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询公网 IP 失败: %w", err)
	}
	rowByID := make(map[uint]model.PublicIP, len(rows))
	for _, row := range rows {
		rowByID[row.ID] = row
	}

	// 一次性查询哪些 IP 已绑定
	var boundIDs []uint
	if err := model.DB.Model(&model.PublicIPBinding{}).Where("public_ip_id IN ?", uniqueIDs).Pluck("public_ip_id", &boundIDs).Error; err != nil {
		return nil, fmt.Errorf("查询公网 IP 绑定状态失败: %w", err)
	}
	boundSet := make(map[uint]bool, len(boundIDs))
	for _, id := range boundIDs {
		boundSet[id] = true
	}

	summary := &PublicIPBatchOpSummary{Items: make([]PublicIPBatchOpResult, 0, len(uniqueIDs))}
	for _, id := range uniqueIDs {
		row, exists := rowByID[id]
		if !exists || row.ID == 0 {
			summary.Failed++
			summary.Items = append(summary.Items, PublicIPBatchOpResult{ID: id, IP: "-", Status: "failed", Reason: "公网 IP 不存在"})
			continue
		}
		ipText := row.IP
		if boundSet[id] {
			summary.Skipped++
			summary.Items = append(summary.Items, PublicIPBatchOpResult{ID: id, IP: ipText, Status: "skipped", Reason: "已绑定，请先解绑"})
			continue
		}
		if err := model.DB.Delete(&model.PublicIP{}, id).Error; err != nil {
			summary.Failed++
			summary.Items = append(summary.Items, PublicIPBatchOpResult{ID: id, IP: ipText, Status: "failed", Reason: "删除失败: " + err.Error()})
			continue
		}
		summary.Success++
		summary.Items = append(summary.Items, PublicIPBatchOpResult{ID: id, IP: ipText, Status: "success"})
	}
	return summary, nil
}
