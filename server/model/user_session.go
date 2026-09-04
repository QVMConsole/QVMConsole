package model

import "time"

// UserSession 记录正式访问令牌的服务端会话状态。
type UserSession struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	SessionID      string     `json:"session_id" gorm:"uniqueIndex;size:64;not null"`
	UserID         uint       `json:"user_id" gorm:"index;not null"`
	LastActivityAt time.Time  `json:"last_activity_at" gorm:"index;not null"`
	ExpiresAt      time.Time  `json:"expires_at" gorm:"index;not null"`
	RevokedAt      *time.Time `json:"revoked_at" gorm:"index"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (UserSession) TableName() string {
	return "user_sessions"
}
