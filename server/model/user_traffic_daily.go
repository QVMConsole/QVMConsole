package model

import (
	"time"
)

// UserTrafficDaily 用户每日流量记录
type UserTrafficDaily struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Username      string    `gorm:"index;not null;size:64" json:"username"` // 用户名
	Date          string    `gorm:"index;not null;size:10" json:"date"`     // 日期 YYYY-MM-DD
	TrafficDown   int64     `json:"traffic_down"`                           // 当日下行流量（字节）
	TrafficUp     int64     `json:"traffic_up"`                             // 当日上行流量（字节）
	OffsetDown    int64     `json:"offset_down"`                            // 下行偏移量（重置时记录的基线）
	OffsetUp      int64     `json:"offset_up"`                              // 上行偏移量（重置时记录的基线）
	IsLimitedDown bool      `json:"is_limited_down"`                        // 下行是否已触发限速
	IsLimitedUp   bool      `json:"is_limited_up"`                          // 上行是否已触发限速
	UpdatedAt     time.Time `json:"updated_at"`
}
