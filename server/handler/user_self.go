package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	libvirt_rpc "kvm_console/service/libvirt_rpc"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
)

// ==================== 用户自助 API ====================
// 本文件包含用户自助操作的 API handler，如查看配额、查看/管理自己的 VM 等。
// 管理员 API 保留在 user.go 中。

// GetSelfQuota 获取当前用户的配额信息
func GetSelfQuota(c *gin.Context) {
	username, _ := c.Get("username")

	usage, err := service.GetUserQuotaUsage(username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取配额信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    usage,
	})
}

// GetSelfVMs 获取当前用户的VM列表
func GetSelfVMs(c *gin.Context) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	usernameStr, _ := username.(string)

	// 管理员返回所有VM
	if role == "admin" {
		service.TriggerAdminVMCacheRefreshIfNeeded()
		vms, err := service.ListCachedVMs(buildVMListOptions(c))
		if err != nil {
			respondVMListError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data":    vms,
		})
		return
	}

	// 普通用户只返回自己的VM
	allVMs, err := service.ListCachedVMsByOwner(usernameStr, buildVMListOptions(c))
	if err != nil {
		respondVMListError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    allVMs,
	})
}

// GetSelfVMsSSE SSE 实时推送当前用户的VM列表
func GetSelfVMsSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	username, _ := c.Get("username")
	role, _ := c.Get("role")
	usernameStr, _ := username.(string)
	isAdmin := role == "admin"

	clientGone := c.Request.Context().Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	listOptions := buildVMListOptions(c)

	// 获取用户的 VM 列表数据
	getUserVMs := func() ([]service.VmInfo, error) {
		if isAdmin {
			service.TriggerAdminVMCacheRefreshIfNeeded()
			return service.ListCachedVMs(listOptions)
		}
		return service.ListCachedVMsByOwner(usernameStr, listOptions)
	}

	// 立即发送一次
	if vms, err := getUserVMs(); err == nil {
		c.SSEvent("vm_list", vms)
		c.Writer.Flush()
	}

	for {
		select {
		case <-clientGone:
			return
		case <-ticker.C:
			vms, err := getUserVMs()
			if err != nil {
				if service.IsLibvirtUnavailableError(err) {
					c.SSEvent("vm_list", []service.VmInfo{})
					c.Writer.Flush()
				}
				continue
			}
			c.SSEvent("vm_list", vms)
			c.Writer.Flush()
		}
	}
}

// GetSelfLightweightVMRegistrations 获取当前用户待确认轻量云服务器。
func GetSelfLightweightVMRegistrations(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr := username.(string)
	regs, err := service.ListLightweightVMRegistrations(usernameStr, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取待开通服务器失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": regs})
}

// ConfirmSelfLightweightVMRegistration 用户确认并开通轻量云服务器。
func ConfirmSelfLightweightVMRegistration(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_vm") {
		return
	}
	if !requireMaintenanceModeDisabled(c, "开通轻量云服务器") {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "注册记录 ID 无效"})
		return
	}
	var req service.LightweightVMConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	username, _ := c.Get("username")
	usernameStr := username.(string)
	params, err := service.BuildLightweightVMProvisionParams(uint(id), usernameStr, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeLightweightVMProvision, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交开通任务失败: " + err.Error()})
		return
	}
	service.MarkLightweightVMRegistrationTask(params.RegistrationID, task.ID)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "轻量云服务器开通任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// SelfCloneVmRequest 用户自助克隆请求
type SelfCloneVmRequest struct {
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
	DiskBus              string                            `json:"disk_bus"`
	VideoModel           string                            `json:"video_model"`
	SpiceEnabled         *bool                             `json:"spice_enabled"` // 是否启用 SPICE 显示协议（不传=回退全局默认）
	CPUTopologyMode      string                            `json:"cpu_topology_mode"`
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
	DisableSystemInit    bool                              `json:"disable_system_init"`       // 禁用系统初始化
	PCIERootPorts        int                               `json:"pcie_root_ports,omitempty"` // q35 预留 pcie-root-port 数量
}

// SelfCloneVm 用户自助从模板克隆VM
func SelfCloneVm(c *gin.Context) {
	if !requireMaintenanceModeDisabled(c, "克隆并启动虚拟机") {
		return
	}
	var req SelfCloneVmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)
	if err := service.ValidateVMName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	if err := service.EnsureTemplateVisibleForClone(req.Template, false); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}

	// 从模板元数据获取类型
	meta := service.GetTemplateMeta(req.Template)
	templateType := req.TemplateType
	if templateType == "" {
		templateType = meta.Type
	}
	req.User = service.NormalizeCloneUsernameForTemplate(templateType, req.User)

	cloudInitMode := ""
	if meta != nil {
		cloudInitMode = strings.ToLower(strings.TrimSpace(meta.CloudInitMode))
	}
	requireCredentials := cloudInitMode != "none" && !req.DisableSystemInit
	if err := service.ValidateCloneCredentialsForTemplate(templateType, req.Hostname, req.User, req.Password, requireCredentials); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}
	if strings.TrimSpace(req.FnOSDeviceID) != "" {
		if err := service.ValidateFnOSDeviceID(req.FnOSDeviceID); err != nil {
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

	// 配额检查：模板系统盘和额外数据盘都计入普通用户硬盘配额。
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
		req.SwitchID = switchID
		req.SecurityGroupID = securityGroupID
	}

	params := &service.CloneParams{
		Name:                 req.Name,
		Remark:               req.Remark,
		Template:             req.Template,
		TemplateType:         templateType,
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
		DiskBus:              req.DiskBus,
		VideoModel:           req.VideoModel,
		SpiceEnabled:         req.SpiceEnabled,
		CPUTopologyMode:      req.CPUTopologyMode,
		FirstBootRebootMode:  req.FirstBootRebootMode,
		TemplateRootPass:     meta.RootPassword,
		TemplateUser:         meta.TemplateUser,
		MemoryDynamic:        sanitizeUserMemoryDynamicRequest(req.MemoryDynamic, req.RAM),
		SwitchID:             req.SwitchID,
		SecurityGroupID:      req.SecurityGroupID,
		ExtraNics:            req.ExtraNics,
		StoragePoolID:        req.StoragePoolID,
		ExtraDisks:           req.ExtraDisks,
		NicModel:             req.NicModel,
		PreserveFnOSDeviceID: req.PreserveFnOSDeviceID,
		FnOSDeviceID:         req.FnOSDeviceID,
		IsAdmin:              false,
		DisableSystemInit:    req.DisableSystemInit,
		PCIERootPorts:        req.PCIERootPorts,
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeClone, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交克隆任务失败: " + err.Error(),
		})
		return
	}

	// 将VM添加到用户的访问列表（任务还未完成，但先注册归属）
	_ = service.AddVMToUser(usernameStr, req.Name)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "克隆任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// SelfDeleteVmRequest 用户自助删除VM请求
type SelfDeleteVmRequest struct {
	DeleteDisks   []string `json:"delete_disks"`   // 要删除的磁盘路径列表
	TransferDisks []string `json:"transfer_disks"` // 要转移到用户存储的磁盘路径列表
}

// SelfDeleteVm 用户自助删除VM
func SelfDeleteVm(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_vm") {
		return
	}
	name := c.Param("name")
	username, _ := c.Get("username")
	usernameStr := username.(string)

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

	// 检查用户是否拥有该VM
	if !service.UserOwnsVM(usernameStr, name) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "无权操作此虚拟机",
		})
		return
	}

	var req SelfDeleteVmRequest
	c.ShouldBindJSON(&req) // 可选参数

	// 如果有需要转移的磁盘，先检查存储池是否已初始化，再检查配额
	if len(req.TransferDisks) > 0 {
		// 检查存储池是否已开通
		if !service.IsStorageInitialized(usernameStr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "您尚未开通「我的存储」，无法转移磁盘。请先前往「我的存储」页面初始化存储池，或勾选所有磁盘直接删除。",
			})
			return
		}
		// 检查存储配额
		_, err := service.CheckDiskTransferQuota(usernameStr, req.TransferDisks)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
	}

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

	// 从用户访问列表中移除
	_ = service.RemoveVMFromUser(usernameStr, name)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}
