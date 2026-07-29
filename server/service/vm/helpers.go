package vm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kvm_console/utils"
)

// DomainExists checks whether a libvirt domain with the given name exists.
func DomainExists(vmName string) bool {
	// 存在性探测：域不存在属预期情况，失败仅记 DEBUG
	return utils.ExecCommandQuiet("virsh", "dominfo", vmName).Error == nil
}

// GetDomainState returns the current state of a libvirt domain.
func GetDomainState(vmName string) string {
	return strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)
}

// QemuImgInfo holds parsed output from qemu-img info.
type QemuImgInfo struct {
	Filename            string `json:"filename"`
	Format              string `json:"format"`
	VirtualSize         int64  `json:"virtual-size"`
	ActualSize          int64  `json:"actual-size"`
	BackingFilename     string `json:"backing-filename"`
	FullBackingFilename string `json:"full-backing-filename"`
}

// QemuInfoChain reads the full backing chain of a disk image using qemu-img info.
func QemuInfoChain(path string) ([]QemuImgInfo, error) {
	result := utils.ExecCommandWithTimeout("qemu-img", 3*time.Minute, "info", "-U", "--backing-chain", "--output=json", path)
	if result.Error != nil {
		return nil, fmt.Errorf("读取磁盘链失败: %s", result.Stderr)
	}
	var chain []QemuImgInfo
	if err := json.Unmarshal([]byte(result.Stdout), &chain); err != nil {
		return nil, fmt.Errorf("解析磁盘链失败: %w", err)
	}
	return chain, nil
}
