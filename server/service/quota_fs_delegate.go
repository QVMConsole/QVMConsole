package service

import (
	"kvm_console/service/storage/quota"
)

// StorageQuotaInfo 类型别名，保持向后兼容
type StorageQuotaInfo = quota.StorageQuotaInfo

func GetStorageMountPoint() string {
	return quota.GetStorageMountPoint()
}

func GetStorageImagePath() string {
	return quota.GetStorageImagePath()
}

func GetProjectID(username string) (int, error) {
	return quota.GetProjectID(username)
}

func SetupUserProject(username string, dirs []string) error {
	return quota.SetupUserProject(username, dirs)
}

func SetUserStorageQuota(username string, limitGB int) error {
	return quota.SetUserStorageQuota(username, limitGB)
}

func RemoveUserStorageQuota(username string) error {
	return quota.RemoveUserStorageQuota(username)
}

func GetUserStorageUsage(username string) (*StorageQuotaInfo, error) {
	return quota.GetUserStorageUsage(username)
}

func IsStorageFilesystemMounted() bool {
	return quota.IsStorageFilesystemMounted()
}

func InitStorageFilesystem(sizeGB int) error {
	return quota.InitStorageFilesystem(sizeGB)
}

func EnsureStorageFilesystem() error {
	return quota.EnsureStorageFilesystem()
}

func CheckQuotaToolsAvailable() error {
	return quota.CheckQuotaToolsAvailable()
}

func SyncAllUserQuotas() error {
	return quota.SyncAllUserQuotas()
}

// init wires quota package hook variables to service root implementations.
// This breaks the circular dependency: quota package cannot import service,
// so it exposes function variables that we set here.
func init() {
	quota.HookListQuotaUsers = func() ([]quota.QuotaUserEntry, error) {
		users, err := ListUsers()
		if err != nil {
			return nil, err
		}
		var entries []quota.QuotaUserEntry
		for _, u := range users {
			entries = append(entries, quota.QuotaUserEntry{
				Username:   u.Username,
				MaxStorage: u.MaxStorage,
			})
		}
		return entries, nil
	}
}
