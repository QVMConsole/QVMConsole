//go:build linux

package utils

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// ReadMemInfo 解析 /proc/meminfo，返回以 kB 为单位的 map
// key 为字段名（如 "MemTotal", "MemAvailable", "SwapTotal", "SwapFree"）
func ReadMemInfo() (map[string]int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("打开 /proc/meminfo 失败: %w", err)
	}
	defer f.Close()

	result := make(map[string]int64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 格式: "Key:    Value kB" 或 "Key:    Value"
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		valuePart := strings.TrimSpace(line[colonIdx+1:])

		// 去掉可能的 "kB" 后缀
		valuePart = strings.TrimSuffix(valuePart, " kB")
		valuePart = strings.TrimSpace(valuePart)

		val, err := strconv.ParseInt(valuePart, 10, 64)
		if err != nil {
			continue
		}
		result[key] = val
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 /proc/meminfo 失败: %w", err)
	}

	return result, nil
}

// GetDiskSpace 使用 syscall.Statfs 获取指定路径的磁盘空间信息
// 返回值均为 kB 单位
func GetDiskSpace(path string) (total, used, available int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, fmt.Errorf("获取磁盘空间失败: %w", err)
	}

	// Bsize 可能很大，先转 int64 再运算避免溢出
	bsize := int64(stat.Bsize)
	total = (int64(stat.Blocks) * bsize) / 1024
	available = (int64(stat.Bavail) * bsize) / 1024
	used = total - available

	return total, used, available, nil
}

// IsFilesystemReadOnly 检查指定路径所属文件系统是否以只读方式挂载。
func IsFilesystemReadOnly(path string) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false
	}
	return stat.Flags&unix.ST_RDONLY != 0
}

// GetAllMountedDiskSpace 统计所有已挂载本地文件系统的空间总和（kB）。
// 口径：df 中源设备以 /dev/ 开头（排除 tmpfs/overlay/网络存储等虚拟文件系统），
// 排除 loop 设备（snap 镜像 / ISO 挂载等），同一源设备多处挂载（bind mount、
// btrfs 子卷）只统计一次，避免重复计算。
func GetAllMountedDiskSpace() (total, used, available int64, err error) {
	result := ExecCommand("df", "-B1", "--output=source,size,used,avail,target")
	if result.Error != nil {
		return 0, 0, 0, fmt.Errorf("读取磁盘空间失败: %s", result.Stderr)
	}

	seen := make(map[string]bool)
	for i, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" {
			continue // 跳过表头与空行
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		source := fields[0]
		if !strings.HasPrefix(source, "/dev/") || strings.HasPrefix(source, "/dev/loop") {
			continue
		}
		if seen[source] {
			continue
		}
		seen[source] = true

		size, _ := strconv.ParseInt(fields[1], 10, 64)
		usedBytes, _ := strconv.ParseInt(fields[2], 10, 64)
		avail, _ := strconv.ParseInt(fields[3], 10, 64)
		total += size / 1024
		used += usedBytes / 1024
		available += avail / 1024
	}

	return total, used, available, nil
}

// GetFileCreateTime 获取文件的创建时间（从 stat 的 Ctim 字段获取，若为 0 则回退到 Mtim）
// 返回 Unix 秒数，如果获取失败返回 0
func GetFileCreateTime(path string) int64 {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return 0
	}
	ts := stat.Ctim.Sec
	if ts <= 0 {
		ts = stat.Mtim.Sec
	}
	return ts
}

// TMPFS_MAGIC 是 Linux tmpfs 文件系统的魔数（来自 linux/magic.h）
const TMPFS_MAGIC int64 = 0x01021994

// IsTmpOnTmpfs 检测 /tmp 是否挂载为 tmpfs 文件系统
func IsTmpOnTmpfs() bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/tmp", &stat); err != nil {
		return false
	}
	return stat.Type == TMPFS_MAGIC
}

// GetTmpAvailableBytes 返回 /tmp 目录当前可用空间（字节）
func GetTmpAvailableBytes() int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/tmp", &stat); err != nil {
		return 0
	}
	return int64(stat.Bavail) * int64(stat.Bsize)
}

// GetTmpTotalBytes 返回 /tmp 目录总空间（字节）
func GetTmpTotalBytes() int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/tmp", &stat); err != nil {
		return 0
	}
	return int64(stat.Blocks) * int64(stat.Bsize)
}
