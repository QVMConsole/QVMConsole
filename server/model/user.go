package model

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID                            uint           `json:"id" gorm:"primaryKey"`
	Username                      string         `json:"username" gorm:"uniqueIndex;size:64;not null"`
	PasswordHash                  string         `json:"-" gorm:"size:256"`
	PasswordBreachPrefix          string         `json:"-" gorm:"size:5;index"`
	PasswordBreachHMAC            string         `json:"-" gorm:"size:64"`
	PasswordBreached              bool           `json:"-" gorm:"default:false;index"`
	PasswordBreachCount           int64          `json:"-" gorm:"default:0"`
	PasswordBreachCheckedAt       *time.Time     `json:"-"`
	PasswordBreachDetectedAt      *time.Time     `json:"-"`
	PasswordBreachUserNotifiedAt  *time.Time     `json:"-"`
	PasswordBreachAdminNotifiedAt *time.Time     `json:"-"`
	Email                         string         `json:"email" gorm:"size:255;index"`
	Role                          string         `json:"role" gorm:"index;size:20;not null;default:'user'"`    // admin / user
	CloudType                     string         `json:"cloud_type" gorm:"size:32;not null;default:'elastic'"` // elastic / lightweight
	DedicatedVPCSwitchID          uint           `json:"dedicated_vpc_switch_id" gorm:"default:0"`
	Status                        string         `json:"status" gorm:"index;size:32;not null;default:'active'"`
	EmailVerifiedAt               *time.Time     `json:"email_verified_at"`
	TOTPEnabled                   bool           `json:"totp_enabled" gorm:"default:false"`
	TOTPSecretEnc                 string         `json:"-" gorm:"type:text"`
	TOTPBoundAt                   *time.Time     `json:"totp_bound_at"`
	TOTPRecoveryCodesEnc          string         `json:"-" gorm:"type:text"` // 加密的恢复码哈希列表（JSON 数组）
	LoginVerifiedUntil            *time.Time     `json:"login_verified_until"`
	HighRiskVerifiedUntil         *time.Time     `json:"high_risk_verified_until"`
	BootstrapSkipped              bool           `json:"bootstrap_skipped" gorm:"default:false"` // 管理员是否跳过安全初始化
	SecurityUpdatedAt             *time.Time     `json:"security_updated_at"`
	MaxCPU                        int            `json:"max_cpu" gorm:"default:0"`           // CPU 配额（核心数），0=不限
	MaxMemory                     int            `json:"max_memory" gorm:"default:0"`        // 内存配额（GB），0=不限
	MaxDisk                       int            `json:"max_disk" gorm:"default:0"`          // 磁盘配额（GB），0=不限
	MaxVM                         int            `json:"max_vm" gorm:"default:0"`            // 最大 VM 数量，0=不限
	MaxStorage                    int            `json:"max_storage" gorm:"default:0"`       // 存储配额（GB），0=不限
	MaxRuntimeHours               int            `json:"max_runtime_hours" gorm:"default:0"` // 总运行时长配额（小时），0=不限
	EnablePortForward             bool           `json:"enable_port_forward" gorm:"default:true"`
	MaxPortForwards               int            `json:"max_port_forwards" gorm:"default:10"`        // 端口转发数量配额，0=不限
	MaxSnapshots                  int            `json:"max_snapshots" gorm:"default:5"`             // 快照数量配额，0=不限
	MaxBandwidthUp                float64        `json:"max_bandwidth_up" gorm:"default:0"`          // 上行带宽配额（Mbps），0=不限
	MaxBandwidthDown              float64        `json:"max_bandwidth_down" gorm:"default:0"`        // 下行带宽配额（Mbps），0=不限
	MaxTrafficDown                float64        `json:"max_traffic_down" gorm:"default:0"`          // 下行日流量配额（GB），0=不限
	MaxTrafficUp                  float64        `json:"max_traffic_up" gorm:"default:0"`            // 上行日流量配额（GB），0=不限
	MaxPublicIPs                  int            `json:"max_public_ips" gorm:"default:0"`            // 公网 IP 配额，0=不限
	UsedRuntimeSeconds            int64          `json:"-" gorm:"default:0"`                         // 已累计总运行时长（秒）
	RuntimeActiveVMCount          int            `json:"-" gorm:"default:0"`                         // 上次观测时的运行中 VM 数量
	RuntimeLastObservedAt         *time.Time     `json:"-"`                                          // 上次统计运行时长的时间
	RuntimeWarningSentAt          *time.Time     `json:"-"`                                          // 2 小时预警邮件发送时间
	SSHEnabled                    bool           `json:"ssh_enabled" gorm:"default:false"`           // 是否允许 SSH 登录，默认关闭
	ForcePasswordChange           bool           `json:"force_password_change" gorm:"default:false"` // 首次登录强制修改密码
	ForcePasswordChangeReason     string         `json:"-" gorm:"size:32"`
	CreatedAt                     time.Time      `json:"created_at"`
	UpdatedAt                     time.Time      `json:"updated_at"`
	DeletedAt                     gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
