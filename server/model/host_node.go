package model

import "time"

// HostNode 保存可迁移目标节点的连接信息。
type HostNode struct {
	ID               uint       `json:"id" gorm:"primaryKey"`
	Name             string     `json:"name" gorm:"index;size:80;not null"`
	APIBaseURL       string     `json:"api_base_url" gorm:"size:255;not null"`
	APIKeyID         string     `json:"api_key_id" gorm:"size:120"`
	APIKeyEnc        string     `json:"-" gorm:"type:text"`
	SSHHost          string     `json:"ssh_host" gorm:"size:255;not null"`
	SSHPort          int        `json:"ssh_port" gorm:"default:22"`
	SSHUser          string     `json:"ssh_user" gorm:"size:64;not null;default:'root'"`
	SSHPasswordEnc   string     `json:"-" gorm:"type:text"`
	SSHKeyAuth       bool       `json:"ssh_key_auth" gorm:"default:false"` // true 时使用本机 SSH 密钥免密认证（面板不保存密钥，由用户在系统中自行配置）
	SSHKeyPath       string     `json:"ssh_key_path" gorm:"size:255"`      // 可选：本机私钥路径，留空使用默认迁移密钥
	Enabled          bool       `json:"enabled" gorm:"index;default:true"`
	Status           string     `json:"status" gorm:"index;size:32;default:'unknown'"`
	LastProbeMessage string     `json:"last_probe_message" gorm:"type:text"`
	CapabilitiesJSON string     `json:"capabilities_json" gorm:"type:text"`
	LastProbedAt     *time.Time `json:"last_probed_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (HostNode) TableName() string {
	return "host_nodes"
}
