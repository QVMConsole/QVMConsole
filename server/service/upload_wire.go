package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	templatepkg "kvm_console/service/template"
	uploadpkg "kvm_console/service/upload"
	userpkg "kvm_console/service/user"
)

// upload_wire.go 将 service/upload 子包的能力转发为根 service 包函数，
// 并在此层完成「目录计算 + 配额检查 + 文件名安全」，保持 upload 子包无业务依赖。

// userStorageDir 返回用户某类别存储目录；非法类别返回空。
func userStorageDir(username, category string) string {
	switch category {
	case "iso":
		return userpkg.GetUserISODir(username)
	case "share":
		return userpkg.GetUserShareDir(username)
	case "disk":
		return userpkg.GetUserDiskDir(username)
	}
	return ""
}

// sanitizeFileName 取 basename 并禁止路径穿越。
func sanitizeFileName(name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == "/" ||
		strings.Contains(base, "..") || strings.ContainsAny(base, `/\`) {
		return "", errors.New("非法文件名")
	}
	return base, nil
}

// InitUserStorageUpload 初始化「用户存储」分片上传会话（iso/share/disk）。
func InitUserStorageUpload(username, category, fileName string, totalSize int64, fileHash string) (*uploadpkg.InitResult, error) {
	if !userpkg.IsStorageInitialized(username) {
		return nil, errors.New("存储池未初始化，请先初始化")
	}
	if userpkg.IsStorageReadonly(username) {
		return nil, errors.New("存储空间已超出配额，处于只读模式，请先删除部分文件")
	}
	dir := userStorageDir(username, category)
	if dir == "" {
		return nil, fmt.Errorf("无效的存储类别: %s", category)
	}
	safeName, err := sanitizeFileName(fileName)
	if err != nil {
		return nil, err
	}
	// 类别后缀校验
	if err := validateStorageFileSuffix(category, safeName); err != nil {
		return nil, err
	}
	// 配额预检（按总大小）
	if err := userpkg.CheckStorageQuota(username, totalSize); err != nil {
		return nil, err
	}
	return uploadpkg.InitUpload(uploadpkg.InitParams{
		FilePath:   filepath.Join(dir, safeName),
		AllowedDir: dir,
		Username:   username,
		Category:   category,
		FileName:   safeName,
		TotalSize:  totalSize,
		ChunkSize:  uploadpkg.DefaultChunkSize,
		FileHash:   fileHash,
	})
}

// validateStorageFileSuffix iso 类别仅允许 .iso；disk 类别仅允许虚拟磁盘格式。
func validateStorageFileSuffix(category, name string) error {
	lower := strings.ToLower(name)
	switch category {
	case "iso":
		if !strings.HasSuffix(lower, ".iso") {
			return errors.New("ISO 类别仅支持 .iso 文件")
		}
	case "disk":
		exts := []string{".qcow2", ".raw", ".vmdk", ".vhd", ".vhdx", ".img", ".vfd", ".ova", ".ovf", ".mf"}
		ok := false
		for _, e := range exts {
			if strings.HasSuffix(lower, e) {
				ok = true
				break
			}
		}
		if !ok {
			return errors.New("虚拟磁盘仅支持: .qcow2, .raw, .vmdk, .vhd, .vhdx, .img, .vfd, .ova, .ovf, .mf")
		}
	}
	return nil
}

// InitTemplateUpload 初始化「模板导入」分片上传会话，目标为导入临时目录下的唯一临时文件。
// 临时文件名以 fileHash 为标识，支持同一模板包重传时复用（续传/秒传）。
func InitTemplateUpload(username, fileName string, totalSize int64, fileHash string) (*uploadpkg.InitResult, error) {
	safeName, err := sanitizeFileName(fileName)
	if err != nil {
		return nil, err
	}
	dir := templatepkg.GetTemplateImportTempDir()
	// 用 hash 前缀避免不同用户/不同包同名冲突，同时保留原扩展名供后续解包识别
	shortHash := fileHash
	if len(shortHash) > 12 {
		shortHash = shortHash[:12]
	}
	tempName := fmt.Sprintf("tpl-%s-%s", shortHash, safeName)
	return uploadpkg.InitUpload(uploadpkg.InitParams{
		FilePath:   filepath.Join(dir, tempName),
		AllowedDir: dir,
		Username:   username,
		Category:   "template",
		FileName:   safeName,
		TotalSize:  totalSize,
		ChunkSize:  uploadpkg.DefaultChunkSize,
		FileHash:   fileHash,
	})
}

// ResolveTemplateUploadPath 返回模板分片上传最终落地的临时文件路径（供 preview/confirm 复用）。
func ResolveTemplateUploadPath(fileName, fileHash string) (string, error) {
	safeName, err := sanitizeFileName(fileName)
	if err != nil {
		return "", err
	}
	dir := templatepkg.GetTemplateImportTempDir()
	shortHash := fileHash
	if len(shortHash) > 12 {
		shortHash = shortHash[:12]
	}
	return filepath.Join(dir, fmt.Sprintf("tpl-%s-%s", shortHash, safeName)), nil
}

// SaveUploadChunk 转发：写入单个分片。
func SaveUploadChunk(filePath, username string, index int, offset int64, data []byte) error {
	return uploadpkg.SaveChunk(filePath, username, index, offset, data)
}

// CompleteUpload 转发：完成校验，返回是否完成及缺失分片清单。
func CompleteUpload(filePath, username, expectedHash string) (*uploadpkg.CompleteResult, error) {
	return uploadpkg.CompleteUpload(filePath, username, expectedHash)
}

// GetUploadStatus 转发：查询会话状态。
func GetUploadStatus(filePath, username string) (*uploadpkg.StatusResult, error) {
	return uploadpkg.GetUploadStatus(filePath, username)
}

// CancelUpload 转发：取消上传。
func CancelUpload(filePath, username string) error {
	return uploadpkg.CancelUpload(filePath, username)
}

// RemoveUpload 转发：强制删除上传文件与会话（用于清理模板导入等已完成的上传临时包）。
func RemoveUpload(filePath, username string) error {
	return uploadpkg.RemoveUpload(filePath, username)
}

// ListUserStoragePending 列出用户存储（iso/share/disk）未完成的上传会话，供主动恢复。
func ListUserStoragePending(username string) ([]uploadpkg.PendingSession, error) {
	all, err := uploadpkg.ListPendingSessions(username)
	if err != nil {
		return nil, err
	}
	out := make([]uploadpkg.PendingSession, 0, len(all))
	for _, s := range all {
		if s.Category == "iso" || s.Category == "share" || s.Category == "disk" {
			out = append(out, s)
		}
	}
	return out, nil
}

// StartExpiredUploadSessionCleanup 启动过期上传会话清理 goroutine。
func StartExpiredUploadSessionCleanup() {
	uploadpkg.StartExpiredSessionCleanup()
}
