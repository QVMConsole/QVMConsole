package template

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// CalculateFileHashes computes MD5, SHA256, and file size for a disk file.
func CalculateFileHashes(path string) (*TemplateFileHash, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取模板磁盘失败: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("读取模板磁盘信息失败: %w", err)
	}
	md5Hash := md5.New()
	sha256Hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(md5Hash, sha256Hash), file); err != nil {
		return nil, fmt.Errorf("计算模板哈希失败: %w", err)
	}
	return &TemplateFileHash{
		MD5:      hex.EncodeToString(md5Hash.Sum(nil)),
		SHA256:   hex.EncodeToString(sha256Hash.Sum(nil)),
		FileSize: info.Size(),
	}, nil
}

// VerifyTemplateFileIntegrity checks a template's file integrity against its metadata.
func VerifyTemplateFileIntegrity(tpl TemplateInfo) error {
	if tpl.MD5 == "" || tpl.SHA256 == "" || tpl.FileSize <= 0 {
		return fmt.Errorf("模板 %s 缺少完整性元数据", tpl.AdminName)
	}
	hash, err := CalculateFileHashes(tpl.Path)
	if err != nil {
		return err
	}
	if hash.FileSize != tpl.FileSize || !strings.EqualFold(hash.MD5, tpl.MD5) || !strings.EqualFold(hash.SHA256, tpl.SHA256) {
		return fmt.Errorf("模板 %s 完整性校验失败", tpl.AdminName)
	}
	return nil
}

// sameStringSet checks if two string slices contain the same elements (order-independent).
func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counter := make(map[string]int, len(left))
	for _, item := range left {
		counter[item]++
	}
	for _, item := range right {
		counter[item]--
		if counter[item] < 0 {
			return false
		}
	}
	for _, count := range counter {
		if count != 0 {
			return false
		}
	}
	return true
}
