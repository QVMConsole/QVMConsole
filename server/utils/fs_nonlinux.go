//go:build !linux

package utils

import "fmt"

// ReadMemInfo stub for non-linux platforms.
func ReadMemInfo() (map[string]int64, error) {
	return nil, fmt.Errorf("ReadMemInfo is only supported on Linux")
}

// GetDiskSpace stub for non-linux platforms.
func GetDiskSpace(path string) (total, used, available int64, err error) {
	return 0, 0, 0, fmt.Errorf("GetDiskSpace is only supported on Linux")
}

// IsFilesystemReadOnly stub for non-linux platforms.
func IsFilesystemReadOnly(path string) bool {
	return false
}

// GetAllMountedDiskSpace stub for non-linux platforms.
func GetAllMountedDiskSpace() (total, used, available int64, err error) {
	return 0, 0, 0, fmt.Errorf("GetAllMountedDiskSpace is only supported on Linux")
}

// GetFileCreateTime stub for non-linux platforms.
func GetFileCreateTime(path string) int64 {
	return 0
}

// IsTmpOnTmpfs stub for non-linux platforms.
func IsTmpOnTmpfs() bool {
	return false
}

// GetTmpAvailableBytes stub for non-linux platforms.
func GetTmpAvailableBytes() int64 {
	return 0
}

// GetTmpTotalBytes stub for non-linux platforms.
func GetTmpTotalBytes() int64 {
	return 0
}
