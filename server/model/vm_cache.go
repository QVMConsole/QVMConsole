package model

import "time"

// VMCache 保存用于列表类接口的虚拟机缓存投影。
type VMCache struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name" gorm:"uniqueIndex;size:128;not null"`
	OwnerUsername string    `json:"owner_username" gorm:"index;size:64;not null;default:'admin'"`
	Status        string    `json:"status" gorm:"index;size:64"`
	VCPU          int       `json:"vcpu"`
	MemoryMB      int       `json:"memory_mb"`
	MaxMemoryMB   int       `json:"max_memory_mb"`
	Remark        string    `json:"remark" gorm:"size:255"`
	GroupName     string    `json:"group_name" gorm:"index;size:128"`
	TagsJSON      string    `json:"-" gorm:"type:text"`
	Template      string    `json:"template" gorm:"size:255"`
	DiskSizeText  string    `json:"disk_size_text" gorm:"size:64"`
	CreatedAtText string    `json:"created_at_text" gorm:"size:32"`
	Autostart     bool      `json:"autostart"`
	NicModel      string    `json:"nic_model" gorm:"size:64"`
	MacAddress    string    `json:"mac_address" gorm:"size:64"`
	BandwidthIn   int       `json:"bandwidth_in"`
	BandwidthOut  int       `json:"bandwidth_out"`
	InRescue      bool      `json:"in_rescue"`
	CachedIP      string    `json:"cached_ip" gorm:"size:64"`
	Present       bool      `json:"present" gorm:"index;not null;default:true"`
	LastSyncedAt  time.Time `json:"last_synced_at" gorm:"index"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (VMCache) TableName() string {
	return "vm_caches"
}
