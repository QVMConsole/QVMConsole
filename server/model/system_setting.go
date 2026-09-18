package model

// SystemSetting 系统设置持久化表（键值对方式存储）
// 用于在面板重启后保持管理员通过界面修改的系统设置
type SystemSetting struct {
	Key   string `gorm:"primaryKey;size:100" json:"key"` // 设置项的键名
	Value string `gorm:"type:text" json:"value"`         // 设置项的值
}

// GetSetting 获取指定键的设置值
func GetSetting(key string) (string, bool) {
	var setting SystemSetting
	if err := DB.Where("`key` = ?", key).First(&setting).Error; err != nil {
		return "", false
	}
	return setting.Value, true
}

// SetSetting 设置指定键的值（不存在则创建，存在则更新）
func SetSetting(key, value string) error {
	setting := SystemSetting{
		Key:   key,
		Value: value,
	}
	// 使用 Upsert：存在则更新 value，不存在则插入
	return DB.Where("`key` = ?", key).Assign(SystemSetting{Value: value}).FirstOrCreate(&setting).Error
}

// GetAllSettings 获取所有持久化的设置
func GetAllSettings() (map[string]string, error) {
	var settings []SystemSetting
	if err := DB.Find(&settings).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// DeleteSetting 删除指定键的设置
func DeleteSetting(key string) error {
	return DB.Where("`key` = ?", key).Delete(&SystemSetting{}).Error
}
