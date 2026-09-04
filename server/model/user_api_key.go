package model

import (
	"time"

	"gorm.io/gorm"
)

// UserAPIKey 用户外部 API 调用凭证。
type UserAPIKey struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	UserID     uint           `json:"user_id" gorm:"uniqueIndex;not null"`
	APIKeyID   string         `json:"api_key_id" gorm:"uniqueIndex;size:80;not null"`
	KeyHash    string         `json:"-" gorm:"size:128;not null"`
	KeyPrefix  string         `json:"key_prefix" gorm:"size:32;not null"`
	ExpiresAt  *time.Time     `json:"expires_at" gorm:"index"`
	TrustedIP  string         `json:"trusted_ip" gorm:"size:64"`
	LastUsedAt *time.Time     `json:"last_used_at"`
	RevokedAt  *time.Time     `json:"revoked_at"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名。
func (UserAPIKey) TableName() string {
	return "user_api_keys"
}
