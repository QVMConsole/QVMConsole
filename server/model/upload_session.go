package model

import "time"

// UploadSession 分片上传会话：支持断点续传与秒传。
// 以目标文件绝对路径为主键，记录分片接收进度。
type UploadSession struct {
	// FilePath 目标文件绝对路径，同时作为主键与前端续传凭证（sessionKey）
	FilePath      string    `gorm:"primaryKey" json:"file_path"`
	Username      string    `gorm:"index;size:100;not null" json:"username"`
	Category      string    `gorm:"size:32;not null;default:''" json:"category"` // iso | share | disk | template
	FileName      string    `gorm:"size:255;not null;default:''" json:"file_name"`
	TotalSize     int64     `gorm:"not null;default:0" json:"total_size"`
	ChunkSize     int       `gorm:"not null;default:0" json:"chunk_size"`
	TotalChunks   int       `gorm:"not null;default:0" json:"total_chunks"`
	FileHash      string    `gorm:"index;size:64;not null;default:''" json:"file_hash"`       // 整文件 MD5（秒传 + 最终校验）
	ReceivedBmp   string    `gorm:"type:text" json:"received_bmp"`                            // 已收分片位图（big.Int 十六进制文本，bit i = 第 i 片已收）
	UploadedBytes int64     `gorm:"not null;default:0" json:"uploaded_bytes"`                 // 已收字节数（进度）
	Status        string    `gorm:"index;size:16;not null;default:'uploading'" json:"status"` // uploading | completed | canceled
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	ExpiresAt     time.Time `gorm:"index" json:"expires_at"` // 无活动过期时间，供后台清理
}

func (UploadSession) TableName() string {
	return "upload_sessions"
}

// 分片上传会话状态常量
const (
	UploadStatusUploading = "uploading"
	UploadStatusCompleted = "completed"
	UploadStatusCanceled  = "canceled"
)
