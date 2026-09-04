package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"kvm_console/model"
)

var actionTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{40,256}$`)

// CreateAuthActionToken 创建邮件链接令牌
func CreateAuthActionToken(userID uint, purpose string, ttl time.Duration) (string, *model.AuthActionToken, error) {
	rawToken, hashValue, err := newRandomToken()
	if err != nil {
		return "", nil, err
	}

	if err := model.DB.Where("user_id = ? AND purpose = ? AND consumed_at IS NULL", userID, purpose).
		Delete(&model.AuthActionToken{}).Error; err != nil {
		return "", nil, err
	}

	record := &model.AuthActionToken{
		UserID:    userID,
		Purpose:   purpose,
		TokenHash: hashValue,
		ExpiresAt: time.Now().Add(ttl),
	}
	if err := model.DB.Create(record).Error; err != nil {
		return "", nil, err
	}
	return rawToken, record, nil
}

// FindValidAuthActionToken 查找有效的链接令牌
func FindValidAuthActionToken(rawToken, purpose string) (*model.AuthActionToken, *model.User, error) {
	rawToken = strings.TrimSpace(rawToken)
	if !actionTokenPattern.MatchString(rawToken) {
		return nil, nil, fmt.Errorf("链接无效或已失效")
	}
	hashValue := hashToken(rawToken)

	var record model.AuthActionToken
	if err := model.DB.Where("token_hash = ? AND purpose = ?", hashValue, purpose).First(&record).Error; err != nil {
		return nil, nil, fmt.Errorf("链接无效或已失效")
	}
	if record.ConsumedAt != nil {
		return nil, nil, fmt.Errorf("链接已失效")
	}
	if time.Now().After(record.ExpiresAt) {
		return nil, nil, fmt.Errorf("链接已过期")
	}

	var user model.User
	if err := model.DB.First(&user, record.UserID).Error; err != nil {
		return nil, nil, fmt.Errorf("用户不存在")
	}
	return &record, &user, nil
}

// ConsumeAuthActionToken 标记链接令牌已使用
func ConsumeAuthActionToken(recordID uint) error {
	now := time.Now()
	return model.DB.Model(&model.AuthActionToken{}).Where("id = ?", recordID).
		Updates(map[string]interface{}{"consumed_at": &now}).Error
}

// InvalidateAuthActionTokens 失效某类未消费令牌
func InvalidateAuthActionTokens(userID uint, purpose string) error {
	now := time.Now()
	return model.DB.Model(&model.AuthActionToken{}).
		Where("user_id = ? AND purpose = ? AND consumed_at IS NULL", userID, purpose).
		Updates(map[string]interface{}{"consumed_at": &now}).Error
}

func newRandomToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := strings.TrimRight(base64.URLEncoding.EncodeToString(raw), "=")
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return base64.StdEncoding.EncodeToString(sum[:])
}
