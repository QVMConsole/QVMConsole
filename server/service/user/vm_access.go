package user

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvm_console/config"
	"kvm_console/utils"
)

// vmAccessListPath 返回用户 VM 访问清单的安全路径，拒绝任何目录穿越输入。
func vmAccessListPath(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" || username == "." || username == ".." ||
		strings.ContainsRune(username, 0) || strings.ContainsAny(username, "/\\") ||
		filepath.Base(username) != username {
		return "", fmt.Errorf("用户名不合法")
	}

	baseDir := filepath.Clean(strings.TrimSpace(config.GlobalConfig.VMAccessDir))
	if baseDir == "." || baseDir == string(filepath.Separator) {
		return "", fmt.Errorf("虚拟机访问配置目录不合法")
	}
	filePath := filepath.Join(baseDir, username)
	if filepath.Dir(filePath) != baseDir {
		return "", fmt.Errorf("虚拟机访问清单路径不合法")
	}
	return filePath, nil
}

// readUserVMAccessList 读取用户 VM 访问清单。未初始化的清单属于正常空结果。
func readUserVMAccessList(username string) ([]string, error) {
	filePath, err := vmAccessListPath(username)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取虚拟机访问清单失败: %w", err)
	}

	seen := make(map[string]struct{})
	vms := make([]string, 0)
	for _, vmName := range strings.Split(string(content), "\n") {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" {
			continue
		}
		if _, exists := seen[vmName]; exists {
			continue
		}
		seen[vmName] = struct{}{}
		vms = append(vms, vmName)
	}
	return vms, nil
}

// writeUserVMAccessList 原子写入用户 VM 访问清单，避免并发读取半写入内容。
func writeUserVMAccessList(username string, vmNames []string) error {
	filePath, err := vmAccessListPath(username)
	if err != nil {
		return err
	}

	seen := make(map[string]struct{})
	items := make([]string, 0, len(vmNames))
	for _, vmName := range vmNames {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" {
			continue
		}
		if _, exists := seen[vmName]; exists {
			continue
		}
		seen[vmName] = struct{}{}
		items = append(items, vmName)
	}

	content := strings.Join(items, "\n")
	if content != "" {
		content += "\n"
	}
	if err := utils.AtomicWriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入虚拟机访问清单失败: %w", err)
	}
	return nil
}
