package pool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"kvm_console/logger"
	"kvm_console/utils"
)

// VMDiskUsageInfo 虚拟机磁盘使用信息
type VMDiskUsageInfo struct {
	Name        string `json:"name"`         // 虚拟机名称
	DiskPath    string `json:"disk_path"`    // 磁盘完整路径
	VirtualSize int64  `json:"virtual_size"` // 虚拟配置大小（字节）
	ActualSize  int64  `json:"actual_size"`  // 实际占用大小（字节）
	MountPath   string `json:"mount_path"`   // 所在挂载点路径
}

// getAllVMDiskUsage 获取所有虚拟机的磁盘使用详情（内联实现，避免 import cycle）
func getAllVMDiskUsage() []VMDiskUsageInfo {
	var usageList []VMDiskUsageInfo

	// 1. 获取所有虚拟机列表
	virshResult := utils.ExecCommand("virsh", "list", "--all", "--name")
	if virshResult.Error != nil {
		logger.App.Warn("获取虚拟机列表失败", "error", virshResult.Stderr)
		return usageList
	}

	vmNames := strings.Split(virshResult.Stdout, "\n")
	for _, name := range vmNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		// 2. 通过 dumpxml 获取磁盘路径
		xmlResult := utils.ExecCommand("virsh", "dumpxml", name)
		if xmlResult.Error != nil {
			continue
		}

		// 3. 解析 XML 获取磁盘路径
		diskPath := extractDiskPathFromXML(xmlResult.Stdout)
		if diskPath == "" {
			continue
		}

		// 4. qemu-img info 获取大小
		qemuImgResult := utils.ExecShell(fmt.Sprintf("qemu-img info --output=json -U '%s' 2>/dev/null", diskPath))
		if qemuImgResult.Error != nil {
			continue
		}

		var virtualSize, actualSize int64
		var qemuData map[string]interface{}
		if err := json.Unmarshal([]byte(qemuImgResult.Stdout), &qemuData); err == nil {
			if vs, ok := qemuData["virtual-size"].(float64); ok {
				virtualSize = int64(vs)
			}
			if as, ok := qemuData["actual-size"].(float64); ok {
				actualSize = int64(as)
			}
		}

		// 5. 计算所属挂载点
		mountPath := getMountPathFromDiskPath(diskPath)

		usageList = append(usageList, VMDiskUsageInfo{
			Name:        name,
			DiskPath:    diskPath,
			VirtualSize: virtualSize,
			ActualSize:  actualSize,
			MountPath:   mountPath,
		})
	}

	return usageList
}

// extractDiskPathFromXML 从 domain XML 中提取第一块磁盘的 file 路径
func extractDiskPathFromXML(xmlStr string) string {
	lines := strings.Split(xmlStr, "\n")
	inDisk := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<disk ") && strings.Contains(trimmed, "type='file'") {
			inDisk = true
		}
		if inDisk && strings.Contains(trimmed, "<source") {
			// 提取 file='xxx'属性
			if idx := strings.Index(trimmed, "file='"); idx != -1 {
				start := idx + 6
				end := strings.Index(trimmed[start:], "'")
				if end != -1 {
					return trimmed[start : start+end]
				}
			}
		}
		if inDisk && strings.HasPrefix(trimmed, "</disk>") {
			break
		}
	}
	return ""
}

// getMountPathFromDiskPath 从磁盘路径反推挂载点
func getMountPathFromDiskPath(diskPath string) string {
	baseDir := filepath.Dir(diskPath)
	vmDisksIdx := strings.LastIndex(baseDir, "/vm-disks")
	if vmDisksIdx > 0 {
		return baseDir[:vmDisksIdx]
	}
	return filepath.Dir(diskPath)
}
