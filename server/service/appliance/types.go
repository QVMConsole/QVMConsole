package appliance

// Metadata 是从 OVF/OVA 中提取的标准化虚拟机配置。
type Metadata struct {
	SourceFormat string    `json:"source_format"`
	Name         string    `json:"name"`
	Architecture string    `json:"architecture,omitempty"`
	VCPU         int       `json:"vcpu"`
	RAM          int       `json:"ram"` // GB
	BootType     string    `json:"boot_type,omitempty"`
	MachineType  string    `json:"machine_type,omitempty"`
	OSType       string    `json:"os_type,omitempty"`
	Disks        []Disk    `json:"disks"`
	Networks     []Network `json:"networks"`
	Warnings     []string  `json:"warnings"`
}

// Disk 描述虚拟机包中的一块磁盘。
type Disk struct {
	ID            string `json:"id"`
	FileRef       string `json:"file_ref"`
	CapacityBytes int64  `json:"capacity_bytes"`
	Format        string `json:"format,omitempty"`
	Bus           string `json:"bus,omitempty"`
	IsSystem      bool   `json:"is_system"`
}

// Network 描述虚拟机包中的一个逻辑网口。
type Network struct {
	Name  string `json:"name"`
	Model string `json:"model,omitempty"`
}

// ResolvedPackage 是已经解析并可供导入任务使用的虚拟机包。
type ResolvedPackage struct {
	Metadata    *Metadata
	SourcePath  string
	RootDir     string
	DiskPaths   []string
	SourceFiles []string
	cleanupDir  string
}

// Cleanup 清理 OVA 解包产生的临时目录。
func (p *ResolvedPackage) Cleanup() {
	if p == nil || p.cleanupDir == "" {
		return
	}
	removeAll(p.cleanupDir)
}

// ExportDisk 是生成 OVF 描述时使用的磁盘信息。
type ExportDisk struct {
	ID            string
	FileName      string
	FilePath      string
	CapacityBytes int64
	FileSize      int64
	Bus           string
}

// ExportConfig 是生成 OVA 描述时使用的虚拟机配置。
type ExportConfig struct {
	Name         string
	Architecture string
	VCPU         int
	RAMMB        int
	BootType     string
	MachineType  string
	OSType       string
	Disks        []ExportDisk
	Networks     []Network
}
