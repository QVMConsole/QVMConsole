package disk

import (
	"strings"

	"kvm_console/service/arch"
)

// GuestMountConfig 是磁盘创建、克隆和编辑链路共享的来宾挂载配置。
type GuestMountConfig struct {
	Enabled     bool   `json:"enabled"`
	Filesystem  string `json:"filesystem,omitempty"`
	MountPoint  string `json:"mount_point,omitempty"`
	DriveLetter string `json:"drive_letter,omitempty"`
}

// ExtraDiskParam defines extra disk parameters when creating a VM.
// Moved from service root vm_create.go to avoid circular dependency.
type ExtraDiskParam struct {
	Size          int              `json:"size"`            // GB
	Format        string           `json:"format"`          // qcow2/raw
	Bus           string           `json:"bus"`             // disk bus: virtio/scsi/sata/ide
	StoragePoolID string           `json:"storage_pool_id"` // storage pool for extra disk
	GuestMount    GuestMountConfig `json:"guest_mount,omitempty"`
	// IOPS limits (admin only, 0 = unlimited)
	IOPSTotal int `json:"iops_total,omitempty"`
	IOPSRead  int `json:"iops_read,omitempty"`
	IOPSWrite int `json:"iops_write,omitempty"`
}

// NormalizeVMDiskBus normalizes a disk bus string to a known value.
// Moved from service root lightweight_vm_registration.go to avoid circular dependency.
func NormalizeVMDiskBus(value string) string {
	var normalized string
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		normalized = ""
	case "scsi":
		normalized = "scsi"
	case "sata":
		normalized = "sata"
	case "ide":
		normalized = "ide"
	default:
		normalized = "virtio"
	}
	// ARM 架构不支持 sata/ide 磁盘总线，降级为 virtio
	if arch.DetectHostArch() == arch.ArchAarch64 {
		switch normalized {
		case "sata", "ide":
			return "virtio"
		}
	}
	return normalized
}
