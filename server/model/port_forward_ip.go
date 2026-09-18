package model

import "time"

// PortForwardIP 端口转发手动 IP 映射
// 记录手动输入的目标 IP 与虚拟机的关联关系，使刷新页面后仍能正确显示规则
type PortForwardIP struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	VMName    string    `json:"vm_name" gorm:"size:128;not null;index"` // 虚拟机名称
	IP        string    `json:"ip" gorm:"size:64;not null"`             // 手动输入的目标 IP
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名
func (PortForwardIP) TableName() string {
	return "port_forward_ips"
}
