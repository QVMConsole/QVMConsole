package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm/vmimport"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
)

// CreateVmRequest 普通创建虚拟机请求（不通过模板）
type CreateVmRequest struct {
	Name            string                            `json:"name" binding:"required"`
	Remark          string                            `json:"remark"`
	VCPU            int                               `json:"vcpu" binding:"required"`
	RAM             int                               `json:"ram" binding:"required"`
	DiskSize        int                               `json:"disk_size" binding:"required"`
	DiskFormat      string                            `json:"disk_format"`
	DiskBus         string                            `json:"disk_bus"` // 磁盘总线: virtio/scsi/sata/ide
	OSVariant       string                            `json:"os_variant"`
	ISOPath         string                            `json:"iso_path"`
	ISOPaths        []string                          `json:"iso_paths"`
	NicModel        string                            `json:"nic_model"` // 网卡类型: virtio/e1000e/rtl8139
	Autostart       bool                              `json:"autostart"`
	Freeze          bool                              `json:"freeze"`
	APIC            *bool                             `json:"apic"`
	PAE             *bool                             `json:"pae"`
	RTCOffset       string                            `json:"rtc_offset"`
	RTCStartDate    string                            `json:"rtc_startdate"`
	GuestAgent      *vm_xml.VMGuestAgentConfig        `json:"guest_agent"`
	SMBIOS1         *vm_xml.VMSMBIOS1Config           `json:"smbios1"`
	OSType          string                            `json:"os_type"`
	MachineType     string                            `json:"machine_type"`
	BootType        string                            `json:"boot_type"`
	Watchdog        string                            `json:"watchdog"`
	BootOrder       []string                          `json:"boot_order"`
	VideoModel      string                            `json:"video_model"`
	SpiceEnabled    *bool                             `json:"spice_enabled"` // 是否启用 SPICE 显示协议（不传=回退全局默认）
	CPUTopologyMode string                            `json:"cpu_topology_mode"`
	CPULimitPercent int                               `json:"cpu_limit_percent"`
	CPUAffinity     string                            `json:"cpu_affinity"` // CPU 亲和性，如 "0,2,4"
	VirtType        string                            `json:"virt_type"`    // 虚拟化方案: kvm/qemu
	Arch            string                            `json:"arch"`         // 目标架构: x86_64/aarch64/riscv64
	MemoryDynamic   *vm_memory.VMMemoryDynamicRequest `json:"memory_dynamic"`
	SwitchID        uint                              `json:"switch_id"`
	SecurityGroupID uint                              `json:"security_group_id"`
	ExtraNics       []service.AddVMInterfaceRequest   `json:"extra_nics"`
	StoragePoolID   string                            `json:"storage_pool_id"`
	SystemDiskIOPS  *service.DiskIOPSTune             `json:"system_disk_iops"`          // 系统盘 IOPS 限制（仅管理员）
	HostDevices     []service.HostDeviceParam         `json:"host_devices"`              // 硬件直通设备
	PCIERootPorts   int                               `json:"pcie_root_ports,omitempty"` // q35 预留 pcie-root-port 数量
	ExtraDisks      []struct {
		Size          int    `json:"size"`
		Format        string `json:"format"`
		Bus           string `json:"bus"` // 磁盘总线
		StoragePoolID string `json:"storage_pool_id"`
		IOPSTotal     int    `json:"iops_total"` // IOPS 限制, 0=不限制
		IOPSRead      int    `json:"iops_read"`
		IOPSWrite     int    `json:"iops_write"`
	} `json:"extra_disks"`
}

// CreateVm 普通方式创建虚拟机（异步任务）
func CreateVm(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_vm") {
		return
	}
	if !requireMaintenanceModeDisabled(c, "创建并启动虚拟机") {
		return
	}
	var req CreateVmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 名称、CPU、内存、磁盘大小为必填项",
		})
		return
	}

	if err := service.ValidateVMName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 同步校验：磁盘大小 > 0
	if !validateDiskSize(c, req.DiskSize) {
		return
	}

	// 同步校验：虚拟机名称未被占用
	if !validateVMNameNotExists(c, req.Name) {
		return
	}

	// 同步校验：所有交换机对应的网桥存在
	if !validateSwitchBridges(c, req.SwitchID, req.ExtraNics) {
		return
	}

	params := &service.CreateVMParams{
		Name:            req.Name,
		Remark:          req.Remark,
		VCPU:            req.VCPU,
		RAM:             req.RAM,
		DiskSize:        req.DiskSize,
		DiskFormat:      req.DiskFormat,
		DiskBus:         req.DiskBus,
		OSVariant:       req.OSVariant,
		ISOPath:         req.ISOPath,
		ISOPaths:        req.ISOPaths,
		NicModel:        req.NicModel,
		Autostart:       req.Autostart,
		Freeze:          req.Freeze,
		APIC:            req.APIC,
		PAE:             req.PAE,
		RTCOffset:       req.RTCOffset,
		RTCStartDate:    req.RTCStartDate,
		GuestAgent:      req.GuestAgent,
		SMBIOS1:         req.SMBIOS1,
		OSType:          req.OSType,
		MachineType:     req.MachineType,
		BootType:        req.BootType,
		Watchdog:        req.Watchdog,
		BootOrder:       req.BootOrder,
		VideoModel:      req.VideoModel,
		SpiceEnabled:    req.SpiceEnabled,
		CPUTopologyMode: req.CPUTopologyMode,
		CPULimitPercent: req.CPULimitPercent,
		CPUAffinity:     req.CPUAffinity,
		VirtType:        req.VirtType,
		Arch:            req.Arch,
		MemoryDynamic:   req.MemoryDynamic,
		SwitchID:        req.SwitchID,
		SecurityGroupID: req.SecurityGroupID,
		ExtraNics:       req.ExtraNics,
		StoragePoolID:   req.StoragePoolID,
		SystemDiskIOPS:  req.SystemDiskIOPS,
		HostDevices:     req.HostDevices,
		PCIERootPorts:   req.PCIERootPorts,
	}

	// 额外磁盘
	for _, d := range req.ExtraDisks {
		if d.Size > 0 {
			params.ExtraDisks = append(params.ExtraDisks, service.ExtraDiskParam{
				Size:          d.Size,
				Format:        d.Format,
				Bus:           d.Bus,
				StoragePoolID: d.StoragePoolID,
				IOPSTotal:     d.IOPSTotal,
				IOPSRead:      d.IOPSRead,
				IOPSWrite:     d.IOPSWrite,
			})
		}
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)
	role, _ := c.Get("role")
	params.IsAdmin = role == "admin"
	if role != "admin" {
		params.MemoryDynamic = sanitizeUserMemoryDynamicRequest(req.MemoryDynamic, req.RAM)
	}

	// 如果是普通用户，检查配额
	if role == "user" {
		totalDisk := req.DiskSize
		for _, d := range req.ExtraDisks {
			totalDisk += d.Size
		}
		if err := service.CheckQuota(usernameStr, req.VCPU, req.RAM, totalDisk); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
		// 仅当用户指定了交换机或有网口配置时才解析 VPC
		if req.SwitchID != 0 || len(req.ExtraNics) > 0 {
			switchID, securityGroupID, err := service.ResolveVPCForVMCreate(usernameStr, req.SwitchID, req.SecurityGroupID)
			if err != nil {
				c.JSON(http.StatusForbidden, gin.H{
					"code":    403,
					"message": err.Error(),
				})
				return
			}
			params.SwitchID = switchID
			params.SecurityGroupID = securityGroupID
		}
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeCreate, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交创建任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "创建任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})

	// 如果是普通用户，将VM加入用户访问列表
	if role == "user" {
		_ = service.AddVMToUser(usernameStr, req.Name)
	}
}

// GetOSVariants 获取系统变体列表
func GetOSVariants(c *gin.Context) {
	variants, err := service.ListOSVariants()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取系统变体列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    variants,
	})
}

// GetISOList 获取可用 ISO 列表
func GetISOList(c *gin.Context) {
	isos, err := service.ListISOs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取 ISO 列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    isos,
	})
}

// ImportDiskByPathRequest 管理员通过绝对路径导入磁盘请求
type ImportDiskByPathRequest struct {
	Name             string                            `json:"name" binding:"required"`
	Remark           string                            `json:"remark"`
	DiskPath         string                            `json:"disk_path"`
	DiskFile         string                            `json:"disk_file"`
	DiskSourceType   string                            `json:"disk_source_type"`
	StoragePoolID    string                            `json:"storage_pool_id"`
	VCPU             int                               `json:"vcpu" binding:"required"`
	RAM              int                               `json:"ram" binding:"required"`
	CopyDisk         bool                              `json:"copy_disk"`
	InitType         string                            `json:"init_type"`
	Hostname         string                            `json:"hostname"`
	User             string                            `json:"user"`
	Password         string                            `json:"password"`
	Autostart        bool                              `json:"autostart"`
	Freeze           bool                              `json:"freeze"`
	APIC             *bool                             `json:"apic"`
	PAE              *bool                             `json:"pae"`
	RTCOffset        string                            `json:"rtc_offset"`
	RTCStartDate     string                            `json:"rtc_startdate"`
	GuestAgent       *vm_xml.VMGuestAgentConfig        `json:"guest_agent"`
	SMBIOS1          *vm_xml.VMSMBIOS1Config           `json:"smbios1"`
	BootType         string                            `json:"boot_type"`
	MachineType      string                            `json:"machine_type"`
	NicModel         string                            `json:"nic_model"`
	VideoModel       string                            `json:"video_model"`
	SpiceEnabled     *bool                             `json:"spice_enabled"` // 是否启用 SPICE 显示协议（不传=回退全局默认）
	CPUTopologyMode  string                            `json:"cpu_topology_mode"`
	CPULimitPercent  int                               `json:"cpu_limit_percent"`
	CPUAffinity      string                            `json:"cpu_affinity"` // CPU 亲和性，如 "0,2,4"
	TemplateRootPass string                            `json:"template_root_pass"`
	TemplateUser     string                            `json:"template_user"`
	MemoryDynamic    *vm_memory.VMMemoryDynamicRequest `json:"memory_dynamic"`
	SwitchID         uint                              `json:"switch_id"`
	SecurityGroupID  uint                              `json:"security_group_id"`
	ExtraNics        []service.AddVMInterfaceRequest   `json:"extra_nics"`
	ExtraImportDisks []vmimport.ExtraImportDiskEntry   `json:"extra_import_disks"`
	SystemDiskIOPS   *service.DiskIOPSTune             `json:"system_disk_iops"`   // 系统盘 IOPS 限制（仅管理员）
	StartAfterImport *bool                             `json:"start_after_import"` // 导入完成后是否开启虚拟机，不传默认 true
}

// AdminImportDisk 管理员通过绝对路径导入磁盘创建虚拟机（异步任务）
func AdminImportDisk(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_vm") {
		return
	}
	if !requireMaintenanceModeDisabled(c, "导入磁盘并创建虚拟机") {
		return
	}
	var req ImportDiskByPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 名称、CPU、内存为必填项",
		})
		return
	}

	if err := service.ValidateVMName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 同步校验: 虚拟机名称未被占用
	if !validateVMNameNotExists(c, req.Name) {
		return
	}

	// 同步校验: 所有交换机对应的网桥必须存在
	if !validateSwitchBridges(c, req.SwitchID, req.ExtraNics) {
		return
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)

	// 默认导入后开启虚拟机（向后兼容）
	startAfterImport := true
	if req.StartAfterImport != nil {
		startAfterImport = *req.StartAfterImport
	}

	params := &vmimport.ImportDiskByPathParams{
		Name:             req.Name,
		Remark:           req.Remark,
		DiskPath:         req.DiskPath,
		DiskFile:         req.DiskFile,
		DiskSourceType:   req.DiskSourceType,
		StoragePoolID:    req.StoragePoolID,
		VCPU:             req.VCPU,
		RAM:              req.RAM,
		CopyDisk:         req.CopyDisk,
		InitType:         req.InitType,
		Hostname:         req.Hostname,
		User:             req.User,
		Password:         req.Password,
		Autostart:        req.Autostart,
		Freeze:           req.Freeze,
		APIC:             req.APIC,
		PAE:              req.PAE,
		RTCOffset:        req.RTCOffset,
		RTCStartDate:     req.RTCStartDate,
		GuestAgent:       req.GuestAgent,
		SMBIOS1:          req.SMBIOS1,
		BootType:         req.BootType,
		MachineType:      req.MachineType,
		NicModel:         req.NicModel,
		VideoModel:       req.VideoModel,
		SpiceEnabled:     req.SpiceEnabled,
		CPUTopologyMode:  req.CPUTopologyMode,
		CPULimitPercent:  req.CPULimitPercent,
		CPUAffinity:      req.CPUAffinity,
		TemplateRootPass: req.TemplateRootPass,
		TemplateUser:     req.TemplateUser,
		MemoryDynamic:    req.MemoryDynamic,
		SwitchID:         req.SwitchID,
		SecurityGroupID:  req.SecurityGroupID,
		ExtraNics:        req.ExtraNics,
		ExtraImportDisks: req.ExtraImportDisks,
		Username:         usernameStr,
		SystemDiskIOPS:   req.SystemDiskIOPS,
		StartAfterImport: startAfterImport,
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeImportDisk, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交导入磁盘任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "导入磁盘任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}
