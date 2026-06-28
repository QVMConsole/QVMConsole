package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	clonepkg "kvm_console/service/clone"
	libvirt_rpc "kvm_console/service/libvirt_rpc"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm/migration"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
)

// templateVisibilityResponse 根据 EnsureTemplateVisibleForClone 的错误选择合适的 HTTP 状态码返回
func templateVisibilityResponse(c *gin.Context, err error) {
	if strings.Contains(err.Error(), "模板不存在") {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error()})
	} else {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": err.Error()})
	}
}

// CloneVmRequest 克隆请求
type CloneVmRequest struct {
	Name                 string                            `json:"name" binding:"required"`
	Remark               string                            `json:"remark"`
	Template             string                            `json:"template" binding:"required"`
	TemplateType         string                            `json:"template_type"`
	CloneMode            string                            `json:"clone_mode"`
	VCPU                 int                               `json:"vcpu" binding:"required"`
	RAM                  int                               `json:"ram" binding:"required"`
	DiskSize             int                               `json:"disk_size"`
	Hostname             string                            `json:"hostname"`
	User                 string                            `json:"user"`
	Password             string                            `json:"password"`
	Autostart            bool                              `json:"autostart"`
	Freeze               bool                              `json:"freeze"`
	APIC                 *bool                             `json:"apic"`
	PAE                  *bool                             `json:"pae"`
	RTCOffset            string                            `json:"rtc_offset"`
	RTCStartDate         string                            `json:"rtc_startdate"`
	GuestAgent           *vm_xml.VMGuestAgentConfig        `json:"guest_agent"`
	SMBIOS1              *vm_xml.VMSMBIOS1Config           `json:"smbios1"`
	UEFI                 *bool                             `json:"uefi"`
	TemplateRootPass     string                            `json:"template_root_pass"`
	TemplateUser         string                            `json:"template_user"`
	DiskBus              string                            `json:"disk_bus"`
	VideoModel           string                            `json:"video_model"`
	SpiceEnabled         *bool                             `json:"spice_enabled"` // 是否启用 SPICE 显示协议（不传=回退全局默认）
	CPUTopologyMode      string                            `json:"cpu_topology_mode"`
	CPULimitPercent      int                               `json:"cpu_limit_percent"`
	CPUAffinity          string                            `json:"cpu_affinity"` // CPU 亲和性，如 "0,2,4"
	FirstBootRebootMode  string                            `json:"first_boot_reboot_mode"`
	MemoryDynamic        *vm_memory.VMMemoryDynamicRequest `json:"memory_dynamic"`
	SwitchID             uint                              `json:"switch_id"`
	SecurityGroupID      uint                              `json:"security_group_id"`
	ExtraNics            []service.AddVMInterfaceRequest   `json:"extra_nics"`
	StoragePoolID        string                            `json:"storage_pool_id"`
	ExtraDisks           []service.ExtraDiskParam          `json:"extra_disks"`
	NicModel             string                            `json:"nic_model"`
	PreserveFnOSDeviceID bool                              `json:"preserve_fnos_device_id"`
	FnOSDeviceID         string                            `json:"fnos_device_id"`
	SystemDiskIOPS       *service.DiskIOPSTune             `json:"system_disk_iops"`          // 系统盘 IOPS 限制（仅管理员）
	DisableSystemInit    bool                              `json:"disable_system_init"`       // 禁用系统初始化（跳过凭据校验和来宾系统修改）
	StaticIP             string                            `json:"static_ip"`                 // OpenWrt 静态 IP（CIDR 格式）
	Gateway              string                            `json:"gateway"`                   // OpenWrt 网关
	DNS                  string                            `json:"dns"`                       // OpenWrt DNS
	PCIERootPorts        int                               `json:"pcie_root_ports,omitempty"` // q35 预留 pcie-root-port 数量
}

// BatchCloneRequest 批量克隆请求
type BatchCloneRequest struct {
	Prefix              string                          `json:"prefix" binding:"required"`
	StartNum            int                             `json:"start_num"`
	Count               int                             `json:"count" binding:"required"`
	Template            string                          `json:"template" binding:"required"`
	TemplateType        string                          `json:"template_type"`
	CloneMode           string                          `json:"clone_mode"` // 克隆模式: linked / full
	VCPU                int                             `json:"vcpu" binding:"required"`
	RAM                 int                             `json:"ram" binding:"required"`
	DiskSize            int                             `json:"disk_size"`
	Hostname            string                          `json:"hostname"` // 主机名（空则由系统自动生成）
	User                string                          `json:"user"`     // 新用户名
	Password            string                          `json:"password"`
	Autostart           bool                            `json:"autostart"`
	Freeze              bool                            `json:"freeze"`
	APIC                *bool                           `json:"apic"`
	PAE                 *bool                           `json:"pae"`
	RTCOffset           string                          `json:"rtc_offset"`
	RTCStartDate        string                          `json:"rtc_startdate"`
	GuestAgent          *vm_xml.VMGuestAgentConfig      `json:"guest_agent"`
	SMBIOS1             *vm_xml.VMSMBIOS1Config         `json:"smbios1"`
	UEFI                *bool                           `json:"uefi"`
	TemplateRootPass    string                          `json:"template_root_pass"`
	TemplateUser        string                          `json:"template_user"`
	VideoModel          string                          `json:"video_model"`
	SpiceEnabled        *bool                           `json:"spice_enabled"`   // 是否启用 SPICE 显示协议（不传=回退全局默认）
	DiskBus             string                          `json:"disk_bus"`        // 系统盘总线类型
	NicModel            string                          `json:"nic_model"`       // 网卡模型
	StoragePoolID       string                          `json:"storage_pool_id"` // 存储池
	CPUTopologyMode     string                          `json:"cpu_topology_mode"`
	CPULimitPercent     int                             `json:"cpu_limit_percent"`
	CPUAffinity         string                          `json:"cpu_affinity"` // CPU 亲和性，如 "0,2,4"
	FirstBootRebootMode string                          `json:"first_boot_reboot_mode"`
	SwitchID            uint                            `json:"switch_id"`         // VPC 交换机 ID
	SecurityGroupID     uint                            `json:"security_group_id"` // 安全组 ID
	ExtraNics           []service.AddVMInterfaceRequest `json:"extra_nics"`
	DisableSystemInit   bool                            `json:"disable_system_init"`       // 禁用系统初始化
	StaticIP            string                          `json:"static_ip"`                 // OpenWrt 静态 IP（CIDR 格式）
	Gateway             string                          `json:"gateway"`                   // OpenWrt 网关
	DNS                 string                          `json:"dns"`                       // OpenWrt DNS
	PCIERootPorts       int                             `json:"pcie_root_ports,omitempty"` // q35 预留 pcie-root-port 数量
}

// ReinstallRequest 重装系统请求
type ReinstallRequest struct {
	Template             string `json:"template" binding:"required"`
	DiskSize             int    `json:"disk_size"`
	Hostname             string `json:"hostname"`
	User                 string `json:"user"`
	Password             string `json:"password"`
	PreserveFnOSDeviceID bool   `json:"preserve_fnos_device_id"`
	FnOSDeviceID         string `json:"fnos_device_id"`
}

// CloneVm 链式克隆虚拟机（异步任务）
func CloneVm(c *gin.Context) {
	if !requireMaintenanceModeDisabled(c, "克隆并启动虚拟机") {
		return
	}
	var req CloneVmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
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

	username, _ := c.Get("username")
	usernameStr := username.(string)
	role, _ := c.Get("role")
	isAdmin := role == "admin"
	if err := service.EnsureTemplateVisibleForClone(req.Template, isAdmin); err != nil {
		templateVisibilityResponse(c, err)
		return
	}

	meta := service.GetTemplateMeta(req.Template)
	templateType := strings.ToLower(strings.TrimSpace(req.TemplateType))
	if templateType == "" {
		templateType = meta.Type
	}
	req.User = clonepkg.NormalizeCloneUsernameForTemplate(templateType, req.User)

	cloudInitMode := ""
	if meta != nil {
		cloudInitMode = strings.ToLower(strings.TrimSpace(meta.CloudInitMode))
	}
	requireCredentials := cloudInitMode != "none" && !req.DisableSystemInit
	// OpenWrt 模板不需要常规凭据校验，但需要静态 IP
	if templateType == "openwrt" {
		requireCredentials = false
	}
	if err := clonepkg.ValidateCloneCredentialsForTemplate(templateType, req.Hostname, req.User, req.Password, requireCredentials); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}
	// OpenWrt 模板校验静态 IP
	if templateType == "openwrt" && !req.DisableSystemInit {
		if err := clonepkg.ValidateOpenWrtStaticIP(req.StaticIP); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
	}
	if strings.TrimSpace(req.FnOSDeviceID) != "" {
		if err := clonepkg.ValidateFnOSDeviceID(req.FnOSDeviceID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
		req.PreserveFnOSDeviceID = true
	}

	diskSize, err := service.ResolveCloneDiskSizeGB(req.Template, req.DiskSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 同步校验: resolve后的磁盘大小必须 > 0
	if !validateDiskSize(c, diskSize) {
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

	params := &clonepkg.CloneParams{
		Name:                 req.Name,
		Remark:               req.Remark,
		Template:             req.Template,
		TemplateType:         req.TemplateType,
		CloneMode:            req.CloneMode,
		VCPU:                 req.VCPU,
		RAM:                  req.RAM,
		DiskSize:             diskSize,
		Hostname:             req.Hostname,
		User:                 req.User,
		Password:             req.Password,
		Autostart:            req.Autostart,
		Freeze:               req.Freeze,
		APIC:                 req.APIC,
		PAE:                  req.PAE,
		RTCOffset:            req.RTCOffset,
		RTCStartDate:         req.RTCStartDate,
		GuestAgent:           req.GuestAgent,
		SMBIOS1:              req.SMBIOS1,
		UEFI:                 req.UEFI,
		TemplateRootPass:     req.TemplateRootPass,
		TemplateUser:         req.TemplateUser,
		DiskBus:              req.DiskBus,
		VideoModel:           req.VideoModel,
		SpiceEnabled:         req.SpiceEnabled,
		CPUTopologyMode:      req.CPUTopologyMode,
		CPULimitPercent:      req.CPULimitPercent,
		CPUAffinity:          req.CPUAffinity,
		FirstBootRebootMode:  req.FirstBootRebootMode,
		MemoryDynamic:        req.MemoryDynamic,
		SwitchID:             req.SwitchID,
		SecurityGroupID:      req.SecurityGroupID,
		ExtraNics:            req.ExtraNics,
		StoragePoolID:        req.StoragePoolID,
		ExtraDisks:           req.ExtraDisks,
		NicModel:             req.NicModel,
		PreserveFnOSDeviceID: req.PreserveFnOSDeviceID,
		FnOSDeviceID:         req.FnOSDeviceID,
		SystemDiskIOPS:       req.SystemDiskIOPS,
		StaticIP:             req.StaticIP,
		Gateway:              req.Gateway,
		DNS:                  req.DNS,
		PCIERootPorts:        req.PCIERootPorts,
	}

	params.IsAdmin = isAdmin
	params.DisableSystemInit = req.DisableSystemInit
	if !isAdmin {
		params.MemoryDynamic = sanitizeUserMemoryDynamicRequest(req.MemoryDynamic, req.RAM)
	}

	// 如果是普通用户，检查配额
	if role == "user" {
		totalDiskGB := diskSize
		for _, disk := range req.ExtraDisks {
			if disk.Size > 0 {
				totalDiskGB += disk.Size
			}
		}
		if err := service.CheckQuota(usernameStr, req.VCPU, req.RAM, totalDiskGB); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
		// 仅当用户指定了交换机时才解析 VPC
		if req.SwitchID != 0 {
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

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeClone, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交克隆任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "克隆任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})

	// 如果是普通用户，将VM加入用户访问列表
	if role == "user" {
		_ = service.AddVMToUser(usernameStr, req.Name)
	}
}

// BatchCloneVm 批量克隆（异步任务）
func BatchCloneVm(c *gin.Context) {
	if !requireMaintenanceModeDisabled(c, "批量克隆并启动虚拟机") {
		return
	}
	var req BatchCloneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	if err := service.ValidateVMNamePrefix(req.Prefix); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	if req.StartNum <= 0 {
		req.StartNum = 1
	}

	role, _ := c.Get("role")
	isAdmin := role == "admin"
	if err := service.EnsureTemplateVisibleForClone(req.Template, isAdmin); err != nil {
		templateVisibilityResponse(c, err)
		return
	}

	diskSize, err := service.ResolveCloneDiskSizeGB(req.Template, req.DiskSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 同步校验: resolve后的磁盘大小必须 > 0
	if !validateDiskSize(c, diskSize) {
		return
	}

	// 同步校验: 批量虚拟机名称未被占用
	if !validateBatchVMNamesNotExists(c, req.Prefix, req.StartNum, req.Count) {
		return
	}

	// 同步校验: 所有交换机对应的网桥必须存在
	if !validateSwitchBridges(c, req.SwitchID, req.ExtraNics) {
		return
	}

	params := &clonepkg.BatchCloneParams{
		Prefix:              req.Prefix,
		StartNum:            req.StartNum,
		Count:               req.Count,
		Template:            req.Template,
		TemplateType:        req.TemplateType,
		CloneMode:           req.CloneMode,
		VCPU:                req.VCPU,
		RAM:                 req.RAM,
		DiskSize:            diskSize,
		Hostname:            req.Hostname,
		User:                req.User,
		Password:            req.Password,
		Autostart:           req.Autostart,
		Freeze:              req.Freeze,
		APIC:                req.APIC,
		PAE:                 req.PAE,
		RTCOffset:           req.RTCOffset,
		RTCStartDate:        req.RTCStartDate,
		GuestAgent:          req.GuestAgent,
		SMBIOS1:             req.SMBIOS1,
		UEFI:                req.UEFI,
		TemplateRootPass:    req.TemplateRootPass,
		TemplateUser:        req.TemplateUser,
		VideoModel:          req.VideoModel,
		SpiceEnabled:        req.SpiceEnabled,
		DiskBus:             req.DiskBus,
		NicModel:            req.NicModel,
		StoragePoolID:       req.StoragePoolID,
		CPUTopologyMode:     req.CPUTopologyMode,
		CPULimitPercent:     req.CPULimitPercent,
		CPUAffinity:         req.CPUAffinity,
		FirstBootRebootMode: req.FirstBootRebootMode,
		SwitchID:            req.SwitchID,
		SecurityGroupID:     req.SecurityGroupID,
		ExtraNics:           req.ExtraNics,
		IsAdmin:             isAdmin,
		DisableSystemInit:   req.DisableSystemInit,
		StaticIP:            req.StaticIP,
		Gateway:             req.Gateway,
		DNS:                 req.DNS,
		PCIERootPorts:       req.PCIERootPorts,
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)

	// 如果是普通用户，检查配额（批量克隆需要乘以数量）
	if role == "user" {
		totalVCPU := req.VCPU * req.Count
		totalRAM := req.RAM * req.Count
		totalDiskGB := diskSize * req.Count
		if err := service.CheckQuota(usernameStr, totalVCPU, totalRAM, totalDiskGB); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "配额不足: " + err.Error(),
			})
			return
		}
		// 仅当用户指定了交换机时才解析 VPC
		if req.SwitchID != 0 {
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

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeBatch, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交批量克隆任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "批量克隆任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// ReinstallVm 重装系统（异步任务）
func ReinstallVm(c *gin.Context) {
	if !requireStrictHighRiskVerification(c, "reinstall_vm") {
		return
	}
	if !requireMaintenanceModeDisabled(c, "重装并启动虚拟机") {
		return
	}
	name := c.Param("name")
	if err := service.ValidateVMName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}
	var req ReinstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请指定模板",
		})
		return
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)
	role, _ := c.Get("role")
	isAdmin := role == "admin"

	if err := service.EnsureTemplateVisibleForClone(req.Template, isAdmin); err != nil {
		templateVisibilityResponse(c, err)
		return
	}

	meta := service.GetTemplateMeta(req.Template)
	templateType := strings.ToLower(strings.TrimSpace(meta.Type))
	if templateType == "" {
		templateType = "linux"
	}
	firstBootRebootMode := ""
	if meta.DefaultConfig != nil {
		firstBootRebootMode = meta.DefaultConfig.FirstBootRebootMode
	}
	requireCredentials := templateType == "linux" || templateType == "windows" || templateType == "fnos"
	req.User = clonepkg.NormalizeCloneUsernameForTemplate(templateType, req.User)
	if err := clonepkg.ValidateCloneCredentialsForTemplate(templateType, req.Hostname, req.User, req.Password, requireCredentials); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}
	if strings.TrimSpace(req.FnOSDeviceID) != "" {
		if err := clonepkg.ValidateFnOSDeviceID(req.FnOSDeviceID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
		req.PreserveFnOSDeviceID = true
	}
	if migration.HasActiveReinstallTask(name) {
		c.JSON(http.StatusConflict, gin.H{
			"code":    409,
			"message": "该虚拟机已有进行中的重装任务，请等待当前任务完成",
		})
		return
	}

	diskSize, err := clonepkg.ResolveReinstallDiskSizeGB(name, req.Template, req.DiskSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	params := &clonepkg.ReinstallParams{
		Name:                 name,
		Template:             req.Template,
		TemplateType:         templateType,
		DiskSize:             diskSize,
		Hostname:             strings.TrimSpace(req.Hostname),
		User:                 req.User,
		Password:             req.Password,
		TemplateRootPass:     meta.RootPassword,
		TemplateUser:         meta.TemplateUser,
		FirstBootRebootMode:  firstBootRebootMode,
		PreserveFnOSDeviceID: req.PreserveFnOSDeviceID,
		FnOSDeviceID:         strings.TrimSpace(req.FnOSDeviceID),
		Operator:             usernameStr,
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeReinstall, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交重装任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "重装系统任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// DeleteVmRequest 删除虚拟机请求
type DeleteVmRequest struct {
	DeleteDisks   []string `json:"delete_disks"`   // 要删除的磁盘路径列表
	TransferDisks []string `json:"transfer_disks"` // 要转移到用户存储的磁盘路径列表
}

// DeleteVm 删除虚拟机（异步任务）
func DeleteVm(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_vm") {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	// 检查虚拟机是否存在
	if _, _, _, _, err := libvirt_rpc.GetDomainInfoRPC(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": fmt.Sprintf("虚拟机 '%s' 不存在", name),
		})
		return
	}

	// 检查虚拟机是否已锁定
	if service.IsVMLocked(name) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "该虚拟机已锁定，无法删除。请先解锁后再操作。",
		})
		return
	}

	var req DeleteVmRequest
	c.ShouldBindJSON(&req) // 可选参数，不强制要求

	username, _ := c.Get("username")
	usernameStr := username.(string)

	params := map[string]interface{}{
		"name":           name,
		"delete_disks":   req.DeleteDisks,
		"transfer_disks": req.TransferDisks,
		"transfer_user":  usernameStr,
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeDelete, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交删除任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// GetVmQcow2Disks 获取虚拟机的 qcow2 磁盘列表（删除确认界面用）
func GetVmQcow2Disks(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	disks, err := service.GetVMQcow2Disks(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取磁盘列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    disks,
	})
}

// ForceDeleteVm 强制删除虚拟机（绕过磁盘和快照操作，处理僵尸虚拟机）
func ForceDeleteVm(c *gin.Context) {
	if !requireHighRiskVerification(c, "force_delete_vm") {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)

	params := map[string]interface{}{
		"name":   name,
		"action": "force_delete",
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeDelete, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交强制删除任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "强制删除任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}
