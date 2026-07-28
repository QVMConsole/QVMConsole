package pool

import (
	"encoding/json"
	"strings"
)

// HostStoragePoolInfo 是管理员侧存储池页面展示的宿主机块设备信息。
type HostStoragePoolInfo struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	DisplayName         string                `json:"display_name"`
	DevicePath          string                `json:"device_path"`
	KName               string                `json:"kname"`
	Type                string                `json:"type"`
	Size                int64                 `json:"size"`
	FSType              string                `json:"fstype"`
	FSVersion           string                `json:"fsver"`
	Label               string                `json:"label"`
	UUID                string                `json:"uuid"`
	Mountpoints         []string              `json:"mountpoints"`
	MountPath           string                `json:"mount_path"`
	VMDir               string                `json:"vm_dir"`
	Model               string                `json:"model"`
	Serial              string                `json:"serial"`
	Rota                bool                  `json:"rota"`
	Removable           bool                  `json:"removable"`
	Readonly            bool                  `json:"readonly"`
	Tran                string                `json:"tran"`
	PKName              string                `json:"pkname"`
	Used                int64                 `json:"used"`
	Available           int64                 `json:"available"`
	UsePercent          int                   `json:"use_percent"`
	Enabled             bool                  `json:"enabled"`
	IsDefault           bool                  `json:"is_default"`
	Configured          bool                  `json:"configured"`
	CanFormat           bool                  `json:"can_format"`
	CanUseForVM         bool                  `json:"can_use_for_vm"`
	SystemDisk          bool                  `json:"system_disk"`
	HasExistingData     bool                  `json:"has_existing_data"`
	ExistingDataWarning string                `json:"existing_data_warning,omitempty"`
	StatusReason        string                `json:"status_reason"`
	Children            []HostStoragePoolInfo `json:"children,omitempty"`
	// LVM 扩展字段
	VGName  string `json:"vg_name,omitempty"`   // 所属卷组名（LV/PV 节点）
	LVType  string `json:"lv_type,omitempty"`   // LV 类型：linear/striped/mirrored
	PVCount int    `json:"pv_count,omitempty"`  // VG 节点的 PV 数量
	LVCount int    `json:"lv_count,omitempty"`  // VG 节点的 LV 数量
	IsLVMVG bool   `json:"is_lvm_vg,omitempty"` // 标记为 LVM VG 合成节点
	// ===== VM 磁盘占用统计扩展字段 =====
	VmUsageList    []VMDiskUsageInfo `json:"vm_usage_list,omitempty"`    // 该分区/存储池上的虚拟机列表
	VmTotalVirtual int64             `json:"vm_total_virtual,omitempty"` // 所有 VM 虚拟配置总大小 (Bytes)
	VmTotalActual  int64             `json:"vm_total_actual,omitempty"`  // 所有 VM 实际占用总大小 (Bytes)
}

// VMStorageTarget 是创建虚拟机时可选择的落盘位置。
type VMStorageTarget struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	DevicePath  string `json:"device_path"`
	MountPath   string `json:"mount_path"`
	VMDir       string `json:"vm_dir"`
	Size        int64  `json:"size"`
	Used        int64  `json:"used"`
	Available   int64  `json:"available"`
	Enabled     bool   `json:"enabled"`
	IsDefault   bool   `json:"is_default"`
}

// UpdateHostStoragePoolConfigRequest 更新硬盘显示和启用配置。
type UpdateHostStoragePoolConfigRequest struct {
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
}

// ISOFileInfo ISO 文件信息（带自动推断的系统类型）
type ISOFileInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      string `json:"size"`
	SizeBytes int64  `json:"size_bytes"`
	Pool      string `json:"pool"`
	OSType    string `json:"os_type"`
	OSVariant string `json:"os_variant"`
	MinDisk   int    `json:"min_disk"`
}

// ── Internal types ──

type flexibleMountpoints []string

func (m *flexibleMountpoints) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*m = nil
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*m = arr
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			*m = nil
		} else {
			*m = []string{single}
		}
		return nil
	}
	return nil
}

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name        string              `json:"name"`
	KName       string              `json:"kname"`
	Path        string              `json:"path"`
	Type        string              `json:"type"`
	Size        int64               `json:"size"`
	FSType      string              `json:"fstype"`
	FSVersion   string              `json:"fsver"`
	Label       string              `json:"label"`
	UUID        string              `json:"uuid"`
	Mountpoints flexibleMountpoints `json:"mountpoints"`
	Model       string              `json:"model"`
	Serial      string              `json:"serial"`
	Rota        bool                `json:"rota"`
	Removable   bool                `json:"rm"`
	Readonly    bool                `json:"ro"`
	Tran        string              `json:"tran"`
	PKName      string              `json:"pkname"`
	Children    []lsblkDevice       `json:"children"`
}

type findmntOutput struct {
	Filesystems []findmntInfo `json:"filesystems"`
}

type findmntInfo struct {
	Target   string        `json:"target"`
	Source   string        `json:"source"`
	FSType   string        `json:"fstype"`
	Options  string        `json:"options"`
	Size     int64         `json:"size"`
	Used     int64         `json:"used"`
	Avail    int64         `json:"avail"`
	Children []findmntInfo `json:"children"`
}

type mountUsage struct {
	Source    string
	Target    string
	Size      int64
	Used      int64
	Available int64
}

const hostStorageRoot = "/var/lib/kvm-storage"
