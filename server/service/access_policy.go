package service

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"gorm.io/gorm"

	"kvm_console/config"
	"kvm_console/model"
)

var lanPrefixes = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

// ParseClientIP 解析并规范化请求来源地址。
func ParseClientIP(raw string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || addr.Zone() != "" {
		return netip.Addr{}, fmt.Errorf("IP 地址无效")
	}
	return addr.Unmap(), nil
}

// IsLANIP 判断地址是否属于内建局域网范围。
func IsLANIP(raw string) bool {
	addr, err := ParseClientIP(raw)
	if err != nil {
		return false
	}
	for _, prefix := range lanPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// NormalizeTrustedIP 校验 API Key 绑定的单个固定 IP。
func NormalizeTrustedIP(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("必须设置受信任 IP")
	}
	if strings.ContainsAny(trimmed, "/,; \t\r\n") {
		return "", fmt.Errorf("受信任 IP 只能填写一个固定 IPv4 或 IPv6 地址，不支持网段或多个地址")
	}
	addr, err := ParseClientIP(trimmed)
	if err != nil || addr.IsUnspecified() || addr.IsMulticast() {
		return "", fmt.Errorf("受信任 IP 格式无效")
	}
	return addr.String(), nil
}

// ListActiveAdminsWithoutTOTP 返回尚未启用 2FA 的有效管理员。
func ListActiveAdminsWithoutTOTP() ([]string, error) {
	var usernames []string
	err := model.DB.Model(&model.User{}).
		Where("role = ? AND status = ? AND totp_enabled = ?", "admin", UserStatusActive, false).
		Order("id ASC").
		Pluck("username", &usernames).Error
	return usernames, err
}

// IsAdminAPIKeyPublicPolicyEnabled 判断当前用户是否需要管理员公网密钥策略。
func IsAdminAPIKeyPublicPolicyEnabled(user *model.User) bool {
	return config.GlobalConfig.PublicAccessEnabled && user != nil && user.Role == "admin"
}

// SetPublicAccessEnabled 持久化公网访问状态，并可在开启时撤销全部管理员 API Key。
func SetPublicAccessEnabled(enabled, revokeAdminKeys bool) (int64, error) {
	var revoked int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		setting := model.SystemSetting{Key: "public_access_enabled", Value: fmt.Sprintf("%t", enabled)}
		if err := tx.Where("`key` = ?", setting.Key).
			Assign(model.SystemSetting{Value: setting.Value}).
			FirstOrCreate(&setting).Error; err != nil {
			return err
		}

		if enabled && revokeAdminKeys {
			now := time.Now()
			adminIDs := tx.Model(&model.User{}).Select("id").Where("role = ?", "admin")
			result := tx.Model(&model.UserAPIKey{}).
				Where("user_id IN (?) AND revoked_at IS NULL", adminIDs).
				Updates(map[string]interface{}{"revoked_at": &now, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			revoked = result.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	config.GlobalConfig.PublicAccessEnabled = enabled
	config.SyncEnvFile()
	return revoked, nil
}
