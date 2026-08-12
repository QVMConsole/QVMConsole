package public_ip

import (
	"crypto/sha256"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
	"kvm_console/model"
	"kvm_console/utils"
)

var publicIPInterfacePattern = regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`)

// DiscoverPublicIPv6Prefixes 从指定网卡的全局地址中提取当前公网 IPv6 前缀。
// 网卡留空时使用 IPv6 默认路由的出口网卡。
func DiscoverPublicIPv6Prefixes(uplinkIF string) ([]PublicIPv6PrefixInfo, error) {
	uplinkIF = strings.TrimSpace(uplinkIF)
	if uplinkIF == "" {
		uplinkIF = detectDefaultIPv6Uplink()
	}
	if uplinkIF == "" {
		return nil, fmt.Errorf("未检测到 IPv6 默认出口网卡")
	}
	if !publicIPInterfacePattern.MatchString(uplinkIF) {
		return nil, fmt.Errorf("IPv6 出口网卡名称格式无效")
	}
	result := utils.ExecCommand("ip", "-6", "-o", "addr", "show", "dev", uplinkIF, "scope", "global")
	if result.Error != nil {
		return nil, fmt.Errorf("读取网卡 %s 的 IPv6 地址失败: %s", uplinkIF, firstNonEmpty(strings.TrimSpace(result.Stderr), result.Error.Error()))
	}
	gateway := detectDefaultIPv6Gateway(uplinkIF)
	seen := map[string]bool{}
	items := make([]PublicIPv6PrefixInfo, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		for index, field := range fields {
			if field != "inet6" || index+1 >= len(fields) {
				continue
			}
			prefix, err := netip.ParsePrefix(fields[index+1])
			if err != nil || prefix.Addr().Is4() || prefix.Addr().IsPrivate() || !prefix.Addr().IsGlobalUnicast() {
				break
			}
			masked := prefix.Masked().String()
			if seen[masked] {
				break
			}
			seen[masked] = true
			items = append(items, PublicIPv6PrefixInfo{
				UplinkIF: uplinkIF,
				Address:  prefix.Addr().String(),
				Prefix:   masked,
				Gateway:  gateway,
			})
			break
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("网卡 %s 当前没有公网 IPv6 前缀", uplinkIF)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Prefix < items[j].Prefix })
	return items, nil
}

func detectDefaultIPv6Uplink() string {
	fields := strings.Fields(utils.ExecCommand("ip", "-6", "route", "show", "default").Stdout)
	for index, field := range fields {
		if field == "dev" && index+1 < len(fields) {
			return strings.TrimSpace(fields[index+1])
		}
	}
	return ""
}

func detectDefaultIPv6Gateway(uplinkIF string) string {
	fields := strings.Fields(utils.ExecCommand("ip", "-6", "route", "show", "default", "dev", uplinkIF).Stdout)
	for index, field := range fields {
		if field == "via" && index+1 < len(fields) {
			return strings.TrimSpace(fields[index+1])
		}
	}
	return ""
}

// ImportPublicIPv6Prefix 创建可逐台绑定的公网 IPv6 /128 地址资源。
func ImportPublicIPv6Prefix(req PublicIPv6PrefixImportRequest) (*PublicIPv6PrefixImportResult, error) {
	if model.DB == nil {
		return nil, fmt.Errorf("数据库尚未初始化")
	}
	if req.Count < 1 || req.Count > 4096 {
		return nil, fmt.Errorf("单次导入数量需在 1 - 4096 之间")
	}
	discovered, err := DiscoverPublicIPv6Prefixes(req.UplinkIF)
	if err != nil {
		return nil, err
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(req.Prefix))
	if err != nil || prefix.Addr().Is4() || prefix.Bits() >= 128 {
		return nil, fmt.Errorf("IPv6 前缀格式无效")
	}
	prefix = prefix.Masked()
	matched := false
	uplinkIF := ""
	for _, item := range discovered {
		if item.Prefix == prefix.String() {
			matched = true
			uplinkIF = item.UplinkIF
			break
		}
	}
	if !matched {
		return nil, fmt.Errorf("IPv6 前缀 %s 当前不属于网卡 %s", prefix, strings.TrimSpace(req.UplinkIF))
	}

	created := make([]model.PublicIP, 0, req.Count)
	skipped := 0
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var existingRows []model.PublicIP
		if findErr := tx.Where("uplink_if = ?", uplinkIF).Find(&existingRows).Error; findErr != nil {
			return findErr
		}
		existing := make(map[string]bool, len(existingRows))
		for _, row := range existingRows {
			existing[strings.TrimSpace(row.IP)] = true
		}

		maxAttempts := req.Count*32 + len(existingRows) + 128
		for ordinal := 0; ordinal < maxAttempts && len(created) < req.Count; ordinal++ {
			address := generatedIPv6Address(prefix, ordinal)
			if !address.IsValid() || existing[address.String()] {
				skipped++
				continue
			}
			var count int64
			if countErr := tx.Model(&model.PublicIP{}).Where("ip = ?", address.String()).Count(&count).Error; countErr != nil {
				return countErr
			}
			if count > 0 {
				existing[address.String()] = true
				skipped++
				continue
			}
			row := model.PublicIP{
				IP:               address.String(),
				CIDR:             prefix.String(),
				UplinkIF:         uplinkIF,
				SupportedModes:   PublicIPModeClassicRoute,
				Status:           PublicIPStatusFree,
				Remark:           strings.TrimSpace(req.Remark),
				AutoIPv6:         true,
				IPv6SourcePrefix: prefix.String(),
			}
			if createErr := tx.Create(&row).Error; createErr != nil {
				return fmt.Errorf("导入 IPv6 地址失败: %w", createErr)
			}
			existing[row.IP] = true
			created = append(created, row)
		}
		if len(created) != req.Count {
			return fmt.Errorf("IPv6 前缀可用地址不足，本次仅生成 %d 个地址", len(created))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &PublicIPv6PrefixImportResult{Prefix: prefix.String(), Created: created, Skipped: skipped}, nil
}

func generatedIPv6Address(prefix netip.Prefix, ordinal int) netip.Addr {
	base := prefix.Masked().Addr().As16()
	digest := sha256.Sum256([]byte(fmt.Sprintf("qvm-public-ipv6:%s:%d", prefix.String(), ordinal)))
	bits := prefix.Bits()
	for bit := bits; bit < 128; bit++ {
		byteIndex := bit / 8
		bitMask := byte(1 << (7 - uint(bit%8)))
		digestBit := digest[(bit-bits)/8] & byte(1<<(7-uint((bit-bits)%8)))
		if digestBit != 0 {
			base[byteIndex] |= bitMask
		} else {
			base[byteIndex] &^= bitMask
		}
	}
	address := netip.AddrFrom16(base)
	if address == prefix.Masked().Addr() {
		base[15] = 1
		address = netip.AddrFrom16(base)
	}
	return address
}

// SyncManagedPublicIPv6Addresses 在上游动态前缀变化后保留主机位并更新地址与绑定快照。
func SyncManagedPublicIPv6Addresses() (int, error) {
	if model.DB == nil {
		return 0, nil
	}
	var rows []model.PublicIP
	if err := model.DB.Where("auto_ipv6 = ?", true).Order("uplink_if ASC, id ASC").Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	discoveredByIF := map[string][]PublicIPv6PrefixInfo{}
	changed := 0
	for index := range rows {
		row := &rows[index]
		oldPrefix, err := netip.ParsePrefix(strings.TrimSpace(row.IPv6SourcePrefix))
		if err != nil || oldPrefix.Addr().Is4() {
			return changed, fmt.Errorf("公网 IPv6 %s 的来源前缀无效", row.IP)
		}
		items, ok := discoveredByIF[row.UplinkIF]
		if !ok {
			items, err = DiscoverPublicIPv6Prefixes(row.UplinkIF)
			if err != nil {
				return changed, err
			}
			discoveredByIF[row.UplinkIF] = items
		}
		var currentPrefix netip.Prefix
		for _, item := range items {
			candidate, parseErr := netip.ParsePrefix(item.Prefix)
			if parseErr != nil || candidate.Bits() != oldPrefix.Bits() {
				continue
			}
			candidate = candidate.Masked()
			if !currentPrefix.IsValid() {
				currentPrefix = candidate
			}
			if candidate == oldPrefix.Masked() {
				currentPrefix = candidate
				break
			}
		}
		if !currentPrefix.IsValid() {
			return changed, fmt.Errorf("网卡 %s 未发现 /%d 公网 IPv6 前缀", row.UplinkIF, oldPrefix.Bits())
		}
		if currentPrefix == oldPrefix.Masked() {
			continue
		}
		oldAddress, parseErr := netip.ParseAddr(row.IP)
		if parseErr != nil || oldAddress.Is4() {
			return changed, fmt.Errorf("托管公网 IPv6 地址无效: %s", row.IP)
		}
		newAddress := combineIPv6PrefixAndHost(currentPrefix, oldAddress)
		if !newAddress.IsValid() {
			return changed, fmt.Errorf("生成新公网 IPv6 地址失败")
		}
		updates := map[string]interface{}{
			"ip":                 newAddress.String(),
			"cidr":               currentPrefix.String(),
			"ipv6_source_prefix": currentPrefix.String(),
		}
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			if updateErr := tx.Model(row).Updates(updates).Error; updateErr != nil {
				return updateErr
			}
			return tx.Model(&model.PublicIPBinding{}).Where("public_ip_id = ?", row.ID).Updates(map[string]interface{}{
				"public_ip":          newAddress.String(),
				"guest_ipv6_status":  "pending",
				"guest_ipv6_message": "公网 IPv6 前缀已变化，等待同步来宾配置",
			}).Error
		}); err != nil {
			return changed, fmt.Errorf("更新动态公网 IPv6 失败: %w", err)
		}
		utils.ExecCommand("ip", "-6", "neigh", "del", "proxy", oldAddress.String(), "dev", row.UplinkIF)
		utils.ExecCommand("ip", "-6", "route", "del", oldAddress.String()+"/128")
		cleanupConntrackForPublicIP(oldAddress.String())
		changed++
	}
	return changed, nil
}

func combineIPv6PrefixAndHost(prefix netip.Prefix, host netip.Addr) netip.Addr {
	if !prefix.IsValid() || prefix.Addr().Is4() || !host.IsValid() || host.Is4() {
		return netip.Addr{}
	}
	base := prefix.Masked().Addr().As16()
	hostBytes := host.As16()
	for bit := prefix.Bits(); bit < 128; bit++ {
		byteIndex := bit / 8
		bitMask := byte(1 << (7 - uint(bit%8)))
		if hostBytes[byteIndex]&bitMask != 0 {
			base[byteIndex] |= bitMask
		} else {
			base[byteIndex] &^= bitMask
		}
	}
	return netip.AddrFrom16(base)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
