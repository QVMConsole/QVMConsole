package template

import (
	"encoding/xml"
	"os"
	"regexp"
	"sync"
	"time"

	"kvm_console/utils"
)

// ── Constants ──

const (
	vmTemplateSourceMetadataURI  = "https://kvm-console.local/template-source"
	vmTemplateSourceMetadataKey  = "template-source"
	templateBootDetectTimeout    = 2 * time.Minute
	TemplateDeleteModeCascade    = "cascade"
	TemplateDeleteModePromote    = "promote_children"
	TemplateDeleteModePromoteHot = "promote_children_hot"
	templateDiskInfoWorkerLimit  = 4

	defaultLinuxTemplateCategory   = "Ubuntu"
	defaultWindowsTemplateCategory = "WindowsServer2022"
	defaultOpenWrtTemplateCategory = "OpenWrt"
)

// VMCPUTopologyAuto mirrors the constant from service root to avoid import cycle.
const VMCPUTopologyAuto = "auto"

var (
	templateSourceNamePattern      = regexp.MustCompile(`template_name=['"]([^'"]+)['"]`)
	templateSourceNodePattern      = regexp.MustCompile(`node_id=['"]([^'"]+)['"]`)
	templateSourceCloneModePattern = regexp.MustCompile(`clone_mode=['"]([^'"]+)['"]`)

	linuxTemplateCategories = []string{
		defaultLinuxTemplateCategory,
		"Debian",
		"CentOS",
	}

	windowsTemplateCategories = []string{
		defaultWindowsTemplateCategory,
		"WindowsServer2025",
		"Windows11",
		"Windows10",
		"WindowsServer2012R2",
		"其它",
	}

	openwrtTemplateCategories = []string{
		defaultOpenWrtTemplateCategory,
		"iStoreOS",
	}
)

// ── Exported types ──

// TemplateMeta 模板元数据（保存在 .meta.json 文件中，由程序维护）
type TemplateMeta struct {
	Type             string                 `json:"type"`                         // 类型: linux/windows/fnos/other
	Category         string                 `json:"category,omitempty"`           // 二级分类，当前用于 Linux 发行版和 Windows 版本
	BootType         string                 `json:"boot_type,omitempty"`          // 启动类型: bios/uefi
	BootVerified     bool                   `json:"boot_verified,omitempty"`      // 是否已确认启动类型
	NVRAMPath        string                 `json:"nvram_path,omitempty"`         // UEFI 模板 NVRAM 变量文件
	RootPassword     string                 `json:"root_password,omitempty"`      // 模板 root 密码（已废弃，保留兼容旧元数据）
	TemplateUser     string                 `json:"template_user,omitempty"`      // 模板中的普通用户名（克隆时用于用户名重命名）
	CloudInitMode    string                 `json:"cloud_init_mode,omitempty"`    // 初始化模式: "nocloud"=cloud-init, "configdrive"=Windows ConfigDrive, "fnos"=FnOS, "none"=不初始化
	PostBootCommand  string                 `json:"post_boot_command,omitempty"`  // Linux 模板启动后执行的自定义命令
	PostBootBlocking bool                   `json:"post_boot_blocking,omitempty"` // 启动后命令阻塞模式：true=阻塞系统启动直到命令完成（SSH不可用）
	DefaultConfig    *TemplateDefaultConfig `json:"default_config,omitempty"`     // 模板默认硬件配置
	TemplateUID      string                 `json:"template_uid,omitempty"`       // 模板族唯一标识
	NodeID           string                 `json:"node_id,omitempty"`            // 当前节点唯一标识
	ParentNodeID     string                 `json:"parent_node_id,omitempty"`     // 父节点 ID
	RootNodeID       string                 `json:"root_node_id,omitempty"`       // 根节点 ID
	AdminName        string                 `json:"admin_name,omitempty"`         // 管理员侧名称
	DisplayName      string                 `json:"display_name,omitempty"`       // 用户侧显示文本
	CloneVisible     bool                   `json:"clone_visible"`                // 是否允许普通用户克隆
	Disabled         bool                   `json:"disabled"`                     // 是否禁用克隆
	CreatedFromVM    string                 `json:"created_from_vm,omitempty"`    // 来源 VM
	CreatedAt        string                 `json:"created_at,omitempty"`         // 创建时间
	MD5              string                 `json:"md5,omitempty"`                // 模板磁盘 MD5
	SHA256           string                 `json:"sha256,omitempty"`             // 模板磁盘 SHA256
	FileSize         int64                  `json:"file_size,omitempty"`          // 模板磁盘字节数
	LinuxInitStatus  string                 `json:"linux_init_status,omitempty"`  // Linux 克隆依赖状态: unknown/ready/failed
	LinuxInitChecked string                 `json:"linux_init_checked,omitempty"` // Linux 克隆依赖最近检查时间
	LinuxInitError   string                 `json:"linux_init_error,omitempty"`   // Linux 克隆依赖预处理错误摘要
}

// TemplateDefaultConfig 模板默认硬件配置
type TemplateDefaultConfig struct {
	VCPU                int    `json:"vcpu,omitempty"`
	RAM                 int    `json:"ram,omitempty"`
	DiskSize            int    `json:"disk_size,omitempty"`
	DiskBus             string `json:"disk_bus,omitempty"`
	NicModel            string `json:"nic_model,omitempty"`
	VideoModel          string `json:"video_model,omitempty"`
	CPUTopologyMode     string `json:"cpu_topology_mode,omitempty"`
	FirstBootRebootMode string `json:"first_boot_reboot_mode,omitempty"`
}

// TemplateInfo 模板信息
type TemplateInfo struct {
	Name             string                 `json:"name"`                         // 文件名兼容标识
	ActualSize       string                 `json:"actual_size"`                  // 实际磁盘占用
	VirtualSize      string                 `json:"virtual_size"`                 // 虚拟大小
	Type             string                 `json:"type"`                         // 类型: linux/windows/fnos/other
	Category         string                 `json:"category,omitempty"`           // 二级分类，当前用于 Linux 发行版和 Windows 版本
	BootType         string                 `json:"boot_type,omitempty"`          // 启动类型: bios/uefi
	NVRAMPath        string                 `json:"nvram_path,omitempty"`         // UEFI 模板 NVRAM 变量文件
	IsDefault        bool                   `json:"is_default"`                   // 是否默认模板
	Path             string                 `json:"path"`                         // 完整路径
	RootPassword     string                 `json:"root_password,omitempty"`      // 模板 root 密码（已废弃）
	TemplateUser     string                 `json:"template_user,omitempty"`      // 模板中的普通用户名
	CloudInitMode    string                 `json:"cloud_init_mode,omitempty"`    // 初始化模式: "nocloud"=cloud-init, "configdrive"=Windows ConfigDrive, "fnos"=FnOS
	PostBootCommand  string                 `json:"post_boot_command,omitempty"`  // Linux 模板启动后执行的自定义命令
	PostBootBlocking bool                   `json:"post_boot_blocking,omitempty"` // 启动后命令阻塞模式
	DefaultConfig    *TemplateDefaultConfig `json:"default_config,omitempty"`
	HasMeta          bool                   `json:"has_meta"`              // 是否有元数据文件
	Exported         bool                   `json:"exported"`              // 是否存在导出文件
	ExportPath       string                 `json:"export_path,omitempty"` // 当前导出文件下载路径
	TemplateUID      string                 `json:"template_uid,omitempty"`
	NodeID           string                 `json:"node_id,omitempty"`
	ParentNodeID     string                 `json:"parent_node_id,omitempty"`
	RootNodeID       string                 `json:"root_node_id,omitempty"`
	AdminName        string                 `json:"admin_name,omitempty"`
	DisplayName      string                 `json:"display_name,omitempty"`
	CloneVisible     bool                   `json:"clone_visible"`
	Disabled         bool                   `json:"disabled"`
	CreatedFromVM    string                 `json:"created_from_vm,omitempty"`
	CreatedAt        string                 `json:"created_at,omitempty"`
	MD5              string                 `json:"md5,omitempty"`
	SHA256           string                 `json:"sha256,omitempty"`
	FileSize         int64                  `json:"file_size,omitempty"`
	LinuxInitStatus  string                 `json:"linux_init_status,omitempty"`
	LinuxInitChecked string                 `json:"linux_init_checked,omitempty"`
	LinuxInitError   string                 `json:"linux_init_error,omitempty"`
	HashStatus       string                 `json:"hash_status"` // ok/missing/size_mismatch
	Level            int                    `json:"level"`
	IsRoot           bool                   `json:"is_root"`
	HasChildren      bool                   `json:"has_children"`
	ChildrenCount    int                    `json:"children_count"`
	DirectVMCount    int                    `json:"direct_vm_count"`
	TreeVMCount      int                    `json:"tree_vm_count"`
}

// TemplateRelatedVM 模板关联的虚拟机信息
type TemplateRelatedVM struct {
	Name      string `json:"name"`       // 虚拟机名称
	Status    string `json:"status"`     // 虚拟机状态
	IP        string `json:"ip"`         // 虚拟机 IP
	Template  string `json:"template"`   // 直接来源模板
	NodeID    string `json:"node_id"`    // 直接来源节点
	CloneMode string `json:"clone_mode"` // 克隆模式: linked / full
}

// LinuxTemplatePrepareCheck Linux 模板预处理前的链式依赖检查结果。
type LinuxTemplatePrepareCheck struct {
	TemplateName string              `json:"template_name"`
	LinkedVMs    []TemplateRelatedVM `json:"linked_vms"`
	CanPrepare   bool                `json:"can_prepare"`
}

// PrepareTemplateParams 制作模板参数
type PrepareTemplateParams struct {
	VMName           string `json:"vm_name"`
	TemplateName     string `json:"template_name"`                // 管理员侧名称，同时作为文件名
	DisplayName      string `json:"display_name,omitempty"`       // 用户侧显示文本
	Type             string `json:"type,omitempty"`               // linux/windows/fnos/other
	Category         string `json:"category,omitempty"`           // 二级分类，当前用于 Linux 发行版和 Windows 版本
	RootPassword     string `json:"root_password,omitempty"`      // 已废弃，保留兼容
	TemplateUser     string `json:"template_user,omitempty"`      // 模板中的普通用户名
	CloudInitMode    string `json:"cloud_init_mode,omitempty"`    // 初始化模式: "nocloud"/"configdrive"/"fnos"/"none"
	PostBootCommand  string `json:"post_boot_command,omitempty"`  // Linux 模板启动后执行的自定义命令
	PostBootBlocking bool   `json:"post_boot_blocking,omitempty"` // 启动后命令阻塞模式
}

// DeleteTemplateParams 删除模板参数
type DeleteTemplateParams struct {
	TemplateName string   `json:"template_name"`
	DeleteVMs    bool     `json:"delete_vms"`
	ExpectedVMs  []string `json:"expected_vms,omitempty"`
	DeleteMode   string   `json:"delete_mode,omitempty"`
}

// DeleteTemplateResult 删除模板结果
type DeleteTemplateResult struct {
	TemplateName      string   `json:"template_name"`
	DeletedTemplates  []string `json:"deleted_templates"`
	DeletedVMs        []string `json:"deleted_vms"`
	PromotedTemplates []string `json:"promoted_templates,omitempty"`
	RebasedVMs        []string `json:"rebased_vms,omitempty"`
}

// DeleteTemplatePreview 删除模板预览
type DeleteTemplatePreview struct {
	TemplateName       string              `json:"template_name"`
	Templates          []TemplateInfo      `json:"templates"`
	RelatedVMs         []TemplateRelatedVM `json:"related_vms"`
	ParentTemplate     *TemplateInfo       `json:"parent_template,omitempty"`
	PromotedTemplates  []TemplateInfo      `json:"promoted_templates,omitempty"`
	RebasedVMs         []TemplateRelatedVM `json:"rebased_vms,omitempty"`
	CanPromote         bool                `json:"can_promote"`
	PromoteBlockers    []string            `json:"promote_blockers,omitempty"`
	CanPromoteHot      bool                `json:"can_promote_hot"`
	PromoteHotBlockers []string            `json:"promote_hot_blockers,omitempty"`
}

// UpdateTemplatePublishParams 更新模板可见性参数
type UpdateTemplatePublishParams struct {
	AdminName           string `json:"admin_name,omitempty"`
	DisplayName         string `json:"display_name,omitempty"`
	CloneVisible        bool   `json:"clone_visible"`
	Disabled            bool   `json:"disabled"`
	Category            string `json:"category,omitempty"`
	VCPU                int    `json:"vcpu,omitempty"`
	RAM                 int    `json:"ram,omitempty"`
	DiskSize            int    `json:"disk_size,omitempty"`
	DiskBus             string `json:"disk_bus,omitempty"`
	NicModel            string `json:"nic_model,omitempty"`
	VideoModel          string `json:"video_model,omitempty"`
	CPUTopologyMode     string `json:"cpu_topology_mode,omitempty"`
	FirstBootRebootMode string `json:"first_boot_reboot_mode,omitempty"`
	PostBootCommand     string `json:"post_boot_command,omitempty"`
	PostBootBlocking    bool   `json:"post_boot_blocking,omitempty"`
}

// UpdateTemplateMetaParams 更新模板元数据参数（旧接口兼容）
type UpdateTemplateMetaParams struct {
	AdminName           string `json:"admin_name,omitempty"`
	DisplayName         string `json:"display_name,omitempty"`
	CloneVisible        bool   `json:"clone_visible"`
	Disabled            bool   `json:"disabled"`
	Category            string `json:"category,omitempty"`
	VCPU                int    `json:"vcpu,omitempty"`
	RAM                 int    `json:"ram,omitempty"`
	DiskSize            int    `json:"disk_size,omitempty"`
	DiskBus             string `json:"disk_bus,omitempty"`
	NicModel            string `json:"nic_model,omitempty"`
	VideoModel          string `json:"video_model,omitempty"`
	CPUTopologyMode     string `json:"cpu_topology_mode,omitempty"`
	FirstBootRebootMode string `json:"first_boot_reboot_mode,omitempty"`
	PostBootCommand     string `json:"post_boot_command,omitempty"`
	PostBootBlocking    bool   `json:"post_boot_blocking,omitempty"`
}

// TemplateFileHash 模板文件哈希
type TemplateFileHash struct {
	MD5      string `json:"md5"`
	SHA256   string `json:"sha256"`
	FileSize int64  `json:"file_size"`
}

// ExportTemplateParams 导出模板参数
type ExportTemplateParams struct {
	TemplateName string `json:"template_name"`
	Scope        string `json:"scope,omitempty"` // node/root
}

// TemplateDownloadLink 模板下载链接
type TemplateDownloadLink struct {
	Label        string `json:"label"`
	FileName     string `json:"file_name"`
	DownloadPath string `json:"download_path"`
}

// ExportTemplateResult 导出模板结果
type ExportTemplateResult struct {
	TemplateName   string                 `json:"template_name"`
	FileName       string                 `json:"file_name"`
	FileSize       string                 `json:"file_size"`
	DownloadPath   string                 `json:"download_path"`
	MetaFileName   string                 `json:"meta_file_name,omitempty"`
	ExtraDownloads []TemplateDownloadLink `json:"extra_downloads,omitempty"`
}

// ImportTemplateParams 导入模板参数
type ImportTemplateParams struct {
	TemplateName  string `json:"template_name,omitempty"` // 旧 qcow2 导入兼容
	UploadPath    string `json:"upload_path,omitempty"`
	UploadName    string `json:"upload_name,omitempty"`
	SourcePath    string `json:"source_path,omitempty"`
	SourceName    string `json:"source_name,omitempty"`
	CleanupSource bool   `json:"cleanup_source,omitempty"`
	Type          string `json:"type,omitempty"`
	RootPassword  string `json:"root_password,omitempty"`
	TemplateUser  string `json:"template_user,omitempty"`
}

// ImportTemplateResult 导入模板结果
type ImportTemplateResult struct {
	TemplateName string   `json:"template_name,omitempty"`
	Path         string   `json:"path,omitempty"`
	Type         string   `json:"type,omitempty"`
	HasMeta      bool     `json:"has_meta"`
	Mode         string   `json:"mode,omitempty"`
	Imported     []string `json:"imported,omitempty"`
	Skipped      []string `json:"skipped,omitempty"`
}

// TemplatePackageManifest 模板包清单
type TemplatePackageManifest struct {
	Version     int                   `json:"version"`
	ExportedAt  string                `json:"exported_at"`
	Scope       string                `json:"scope"`
	RootNodeID  string                `json:"root_node_id"`
	TemplateUID string                `json:"template_uid"`
	Nodes       []TemplatePackageNode `json:"nodes"`
}

// TemplatePackageNode 模板包节点
type TemplatePackageNode struct {
	Name     string       `json:"name"`
	DiskFile string       `json:"disk_file"`
	MetaFile string       `json:"meta_file"`
	Meta     TemplateMeta `json:"meta"`
	FileSize int64        `json:"file_size"`
	MD5      string       `json:"md5"`
	SHA256   string       `json:"sha256"`
}

// ImportTemplatePreviewNode 导入预览节点
type ImportTemplatePreviewNode struct {
	Name           string       `json:"name"`
	AdminName      string       `json:"admin_name"`
	DisplayName    string       `json:"display_name"`
	Category       string       `json:"category,omitempty"`
	TemplateUID    string       `json:"template_uid"`
	NodeID         string       `json:"node_id"`
	ParentNodeID   string       `json:"parent_node_id"`
	RootNodeID     string       `json:"root_node_id"`
	Type           string       `json:"type"`
	CloneVisible   bool         `json:"clone_visible"`
	Disabled       bool         `json:"disabled"`
	FileSize       int64        `json:"file_size"`
	MD5            string       `json:"md5"`
	SHA256         string       `json:"sha256"`
	Exists         bool         `json:"exists"`
	WillImport     bool         `json:"will_import"`
	ConflictReason string       `json:"conflict_reason,omitempty"`
	Meta           TemplateMeta `json:"meta"`
}

// ImportTemplatePreviewResult 导入预览结果
type ImportTemplatePreviewResult struct {
	Token       string                      `json:"token"`
	Mode        string                      `json:"mode"` // create/update
	TemplateUID string                      `json:"template_uid"`
	RootNodeID  string                      `json:"root_node_id"`
	Nodes       []ImportTemplatePreviewNode `json:"nodes"`
	CanImport   bool                        `json:"can_import"`
	Message     string                      `json:"message"`
}

// ── Internal types ──

type templateTreeData struct {
	templates []TemplateInfo
	byName    map[string]TemplateInfo
	byNodeID  map[string]TemplateInfo
	children  map[string][]string
	vmByNode  map[string][]TemplateRelatedVM
}

type vmTemplateSource struct {
	XMLName      xml.Name `xml:"template-source"`
	XMLNS        string   `xml:"xmlns,attr"`
	TemplateName string   `xml:"template_name,attr"`
	TemplateUID  string   `xml:"template_uid,attr,omitempty"`
	NodeID       string   `xml:"node_id,attr,omitempty"`
	CloneMode    string   `xml:"clone_mode,attr,omitempty"`
}

type templateDiskInfoCacheEntry struct {
	FileSize        int64
	ModTimeUnixNano int64
	ActualSize      string
	VirtualSize     string
}

type templateImportPreviewSession struct {
	SourcePath    string
	SourceName    string
	CleanupSource bool
	CreatedAt     time.Time
}

// ── Hook I/O types (mirror root-package unexported types) ──

// VMDiskBrief mirrors service.diskInfoResult for hook injection.
type VMDiskBrief struct {
	Device   string
	Path     string
	Size     string
	Template string
}

// VMNetBrief mirrors service.netInfoResult for hook injection.
type VMNetBrief struct {
	Network  string
	MAC      string
	NICModel string
}

// DiskBrief mirrors the subset of disk.DiskInfo used by template.
type DiskBrief struct {
	DeviceType string
	CapacityGB string
	Bus        string
}

// QemuChainEntry mirrors service.QemuImgInfo for hook injection.
type QemuChainEntry struct {
	Filename            string
	VirtualSize         int64
	ActualSize          int64
	BackingFilename     string
	FullBackingFilename string
}

// ── Package-level variables ──

var templateDiskInfoStat = os.Stat

var templateDiskInfoCommand = func(path string) *utils.CmdResult {
	return utils.ExecCommand("qemu-img", "info", "--output=json", "-U", path)
}

var templateDiskInfoCache = struct {
	sync.RWMutex
	items map[string]templateDiskInfoCacheEntry
}{
	items: make(map[string]templateDiskInfoCacheEntry),
}

var templateImportPreviewStore = struct {
	sync.Mutex
	items map[string]templateImportPreviewSession
}{items: make(map[string]templateImportPreviewSession)}

var templateNamePatternForTransfer = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

const (
	templateTransferRetention = 24 * time.Hour
)

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
