package service

import (
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
)

const (
	apiKeyIDPrefix     = "kvm_id_"
	apiKeySecretPrefix = "kvm_sk_"
)

// UserAPIKeyInfo 前端展示用 API Key 信息。
type UserAPIKeyInfo struct {
	APIKeyID            string     `json:"api_key_id"`
	KeyPrefix           string     `json:"key_prefix"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           *time.Time `json:"expires_at"`
	TrustedIP           string     `json:"trusted_ip"`
	LastUsedAt          *time.Time `json:"last_used_at"`
	RevokedAt           *time.Time `json:"revoked_at,omitempty"`
	Enabled             bool       `json:"enabled"`
	PublicAccessEnabled bool       `json:"public_access_enabled"`
	PublicUsable        bool       `json:"public_usable"`
}

// GeneratedAPIKey 仅在生成时返回一次的 API Key。
type GeneratedAPIKey struct {
	UserAPIKeyInfo
	APIKey string `json:"api_key"`
}

// GetUserAPIKeyInfo 获取当前用户 API Key 元信息。
func GetUserAPIKeyInfo(userID uint) (*UserAPIKeyInfo, error) {
	var key model.UserAPIKey
	if err := model.DB.Where("user_id = ?", userID).First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var user model.User
			if userErr := model.DB.First(&user, userID).Error; userErr != nil {
				return nil, userErr
			}
			return &UserAPIKeyInfo{
				PublicAccessEnabled: config.GlobalConfig.PublicAccessEnabled,
				PublicUsable:        false,
			}, nil
		}
		return nil, err
	}
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		return nil, err
	}
	info := buildAPIKeyInfo(&key, &user)
	return &info, nil
}

// RotateUserAPIKey 生成或轮换用户 API Key。明文 Key 只返回一次。
func RotateUserAPIKey(user *model.User, trustedIP string) (*GeneratedAPIKey, error) {
	if user == nil {
		return nil, fmt.Errorf("用户不存在")
	}
	var expiresAt *time.Time
	trustedIP = strings.TrimSpace(trustedIP)
	if config.GlobalConfig.PublicAccessEnabled && user.Role == "admin" {
		normalized, err := NormalizeTrustedIP(trustedIP)
		if err != nil {
			return nil, err
		}
		trustedIP = normalized
		expires := time.Now().Add(30 * 24 * time.Hour)
		expiresAt = &expires
	} else {
		trustedIP = ""
	}
	apiKeyID, err := randomToken(apiKeyIDPrefix, 18)
	if err != nil {
		return nil, err
	}
	apiKey, err := randomToken(apiKeySecretPrefix, 36)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	record := model.UserAPIKey{
		UserID:     user.ID,
		APIKeyID:   apiKeyID,
		KeyHash:    hashAPIKey(apiKey),
		KeyPrefix:  buildKeyPrefix(apiKey),
		ExpiresAt:  expiresAt,
		TrustedIP:  trustedIP,
		LastUsedAt: nil,
		RevokedAt:  nil,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	var existing model.UserAPIKey
	err = model.DB.Where("user_id = ?", user.ID).First(&existing).Error
	if err == nil {
		if err := model.DB.Model(&existing).Updates(map[string]interface{}{
			"api_key_id":   record.APIKeyID,
			"key_hash":     record.KeyHash,
			"key_prefix":   record.KeyPrefix,
			"expires_at":   record.ExpiresAt,
			"trusted_ip":   record.TrustedIP,
			"last_used_at": nil,
			"revoked_at":   nil,
			"created_at":   now,
			"updated_at":   now,
		}).Error; err != nil {
			return nil, err
		}
		record.ID = existing.ID
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := model.DB.Create(&record).Error; err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	info := buildAPIKeyInfo(&record, user)
	return &GeneratedAPIKey{
		UserAPIKeyInfo: info,
		APIKey:         apiKey,
	}, nil
}

// RevokeUserAPIKey 撤销当前用户 API Key。
func RevokeUserAPIKey(userID uint) error {
	now := time.Now()
	return model.DB.Model(&model.UserAPIKey{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]interface{}{
			"revoked_at": &now,
			"updated_at": now,
		}).Error
}

// AuthenticateAPIKey 校验外部 API 调用凭证。
func AuthenticateAPIKey(apiKeyID, apiKey, clientIP string) (*model.User, error) {
	apiKeyID = strings.TrimSpace(apiKeyID)
	apiKey = strings.TrimSpace(apiKey)
	if apiKeyID == "" || apiKey == "" {
		return nil, fmt.Errorf("API Key ID 和 API Key 不能为空")
	}

	var record model.UserAPIKey
	if err := model.DB.Where("api_key_id = ? AND revoked_at IS NULL", apiKeyID).First(&record).Error; err != nil {
		return nil, fmt.Errorf("API 凭证无效")
	}
	if subtle.ConstantTimeCompare([]byte(record.KeyHash), []byte(hashAPIKey(apiKey))) != 1 {
		// 兼容旧密钥过渡期：尝试用 legacy 密钥验证
		if config.GlobalConfig.LegacySecuritySecret == "" || subtle.ConstantTimeCompare([]byte(record.KeyHash), []byte(hashAPIKeyWithSecret(apiKey, config.GlobalConfig.LegacySecuritySecret))) != 1 {
			return nil, fmt.Errorf("API 凭证无效")
		}
		// legacy 验证成功，更新为新密钥的 hash
		newHash := hashAPIKey(apiKey)
		if err := model.DB.Model(&record).Update("key_hash", newHash).Error; err != nil {
			logger.App.Warn("API Key更新失败", "key_id", record.ID, "error", err)
		}
	}

	var user model.User
	if err := model.DB.First(&user, record.UserID).Error; err != nil {
		return nil, fmt.Errorf("用户不存在或已被删除")
	}
	if user.Status == UserStatusDisabled {
		return nil, fmt.Errorf("账号已被禁用")
	}
	if user.Status != UserStatusActive {
		return nil, fmt.Errorf("账号尚未激活")
	}
	if config.GlobalConfig.PublicAccessEnabled && user.Role == "admin" && !IsLANIP(clientIP) {
		now := time.Now()
		if record.ExpiresAt == nil || !now.Before(*record.ExpiresAt) || strings.TrimSpace(record.TrustedIP) == "" {
			return nil, fmt.Errorf("API 凭证无效")
		}
		requestIP, requestErr := ParseClientIP(clientIP)
		trustedIP, trustedErr := ParseClientIP(record.TrustedIP)
		if requestErr != nil || trustedErr != nil || requestIP != trustedIP {
			return nil, fmt.Errorf("API 凭证无效")
		}
	}

	now := time.Now()
	if err := model.DB.Model(&record).Update("last_used_at", &now).Error; err != nil {
		logger.App.Warn("API Key最后使用时间更新失败", "key_id", record.ID, "error", err)
	}
	return &user, nil
}

func buildAPIKeyInfo(key *model.UserAPIKey, user *model.User) UserAPIKeyInfo {
	enabled := key.RevokedAt == nil
	publicUsable := enabled
	if config.GlobalConfig.PublicAccessEnabled && user != nil && user.Role == "admin" {
		publicUsable = enabled && key.ExpiresAt != nil && time.Now().Before(*key.ExpiresAt) && strings.TrimSpace(key.TrustedIP) != ""
	}
	return UserAPIKeyInfo{
		APIKeyID:            key.APIKeyID,
		KeyPrefix:           key.KeyPrefix,
		CreatedAt:           key.CreatedAt,
		ExpiresAt:           key.ExpiresAt,
		TrustedIP:           key.TrustedIP,
		LastUsedAt:          key.LastUsedAt,
		RevokedAt:           key.RevokedAt,
		Enabled:             enabled,
		PublicAccessEnabled: config.GlobalConfig.PublicAccessEnabled,
		PublicUsable:        publicUsable,
	}
}

func randomToken(prefix string, byteSize int) (string, error) {
	raw := make([]byte, byteSize)
	if _, err := io.ReadFull(crand.Reader, raw); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(config.GlobalConfig.SecuritySecret + ":" + apiKey))
	return hex.EncodeToString(sum[:])
}

// hashAPIKeyWithSecret 使用指定 secret 计算 hash（兼容旧密钥）
func hashAPIKeyWithSecret(apiKey, secret string) string {
	sum := sha256.Sum256([]byte(secret + ":" + apiKey))
	return hex.EncodeToString(sum[:])
}

func buildKeyPrefix(apiKey string) string {
	if len(apiKey) <= 14 {
		return apiKey
	}
	return apiKey[:10] + "..." + apiKey[len(apiKey)-4:]
}
