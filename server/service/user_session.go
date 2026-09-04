package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"gorm.io/gorm"

	"kvm_console/logger"
	"kvm_console/model"
)

const PublicSessionIdleTimeout = 30 * time.Minute

var ErrSessionExpired = errors.New("登录会话因长时间未操作已失效，请重新登录")

// CreateUserSession 为正式访问令牌创建服务端会话。
func CreateUserSession(userID uint, expiresAt time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	now := time.Now()
	session := model.UserSession{
		SessionID:      base64.RawURLEncoding.EncodeToString(raw),
		UserID:         userID,
		LastActivityAt: now,
		ExpiresAt:      expiresAt,
	}
	if err := model.DB.Create(&session).Error; err != nil {
		return "", err
	}
	return session.SessionID, nil
}

// ValidatePublicSession 校验公网请求对应的会话是否仍有效。
func ValidatePublicSession(sessionID string, userID uint) (*model.UserSession, error) {
	if sessionID == "" {
		return nil, ErrSessionExpired
	}
	var session model.UserSession
	if err := model.DB.Where("session_id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionExpired
		}
		return nil, fmt.Errorf("读取登录会话失败: %w", err)
	}
	now := time.Now()
	if session.RevokedAt != nil || !now.Before(session.ExpiresAt) || !now.Before(session.LastActivityAt.Add(PublicSessionIdleTimeout)) {
		if session.RevokedAt == nil {
			_ = model.DB.Model(&session).Update("revoked_at", &now).Error
		}
		return nil, ErrSessionExpired
	}
	return &session, nil
}

// TouchUserSession 仅由真实用户活动上报刷新空闲时间。
func TouchUserSession(sessionID string, userID uint) (time.Time, error) {
	session, err := ValidatePublicSession(sessionID, userID)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now()
	if err := model.DB.Model(session).Update("last_activity_at", now).Error; err != nil {
		return time.Time{}, err
	}
	return now.Add(PublicSessionIdleTimeout), nil
}

// RevokeUserSession 撤销指定登录会话。
func RevokeUserSession(sessionID string, userID uint) error {
	if sessionID == "" {
		return nil
	}
	now := time.Now()
	return model.DB.Model(&model.UserSession{}).
		Where("session_id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).
		Update("revoked_at", &now).Error
}

// StartUserSessionCleanup 定期清理已过期或长期撤销的会话。
func StartUserSessionCleanup() {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-24 * time.Hour)
			if err := model.DB.Where("expires_at < ? OR (revoked_at IS NOT NULL AND revoked_at < ?)", time.Now(), cutoff).
				Delete(&model.UserSession{}).Error; err != nil {
				logger.App.Warn("清理过期登录会话失败", "error", err)
			}
		}
	}()
}
