package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
)

// LinkedCloneVmRequest 原生链式克隆请求
type LinkedCloneVmRequest struct {
	Name                 string                            `json:"name" binding:"required"`
	Remark               string                            `json:"remark"`
	Template             string                            `json:"template" binding:"required"`
	TemplateType         string                            `json:"template_type"`
	CloneMode            string                            `json:"clone_mode"`
	VCPU                 int                               `json:"vcpu" binding:"required"`
	RAM                  int                               `json:"ram" binding:"required"`
	DiskSize             *int                              `json:"disk_size"`
	Autostart            bool                              `json:"autostart"`
	Freeze               bool                              `json:"freeze"`
	APIC                 *bool                             `json:"apic"`
	PAE                  *bool                             `json:"pae"`
	RTCOffset            string                            `json:"rtc_offset"`
	RTCStartDate         string                            `json:"rtc_startdate"`
	GuestAgent           *vm_xml.VMGuestAgentConfig        `json:"guest_agent"`
	SMBIOS1              *vm_xml.VMSMBIOS1Config           `json:"smbios1"`
	BootType             string                            `json:"boot_type"`
	DiskBus              string                            `json:"disk_bus"`
	VideoModel           string                            `json:"video_model"`
	CPUTopologyMode      string                            `json:"cpu_topology_mode"`
	CPULimitPercent      int                               `json:"cpu_limit_percent"`
	CPUAffinity          string                            `json:"cpu_affinity"` // CPU 亲和性，如 "0,2,4"
	FirstBootRebootMode  string                            `json:"first_boot_reboot_mode"`
	SwitchID             uint                              `json:"switch_id"`
	SecurityGroupID      uint                              `json:"security_group_id"`
	AllowedIPv4Addresses string                            `json:"allowed_ipv4_addresses"`
	AllowedIPv6Addresses string                            `json:"allowed_ipv6_addresses"`
	ExtraNics            []service.AddVMInterfaceRequest   `json:"extra_nics"`
	StoragePoolID        string                            `json:"storage_pool_id"`
	ExtraDisks           []service.ExtraDiskParam          `json:"extra_disks"`
	HostDevices          []service.HostDeviceParam         `json:"host_devices"`
	NicModel             string                            `json:"nic_model"`
	SystemDiskIOPS       *service.DiskIOPSTune             `json:"system_disk_iops"` // 系统盘 IOPS 限制（仅管理员）
}

// LinkedCloneVm 原生链式克隆虚拟机（异步任务）
func LinkedCloneVm(c *gin.Context) {
	if !requireMaintenanceModeDisabled(c, "链式克隆并启动虚拟机") {
		return
	}

	var req LinkedCloneVmRequest
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

	if err := service.EnsureTemplateVisibleForClone(req.Template, true); err != nil {
		templateVisibilityResponse(c, err)
		return
	}

	// 显式传 0 或负数应被拒绝
	requestedDiskSize := 0
	if req.DiskSize != nil {
		if *req.DiskSize <= 0 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"code": 422, "message": "磁盘大小必须大于0GB"})
			return
		}
		requestedDiskSize = *req.DiskSize
	}
	diskSize, err := service.ResolveCloneDiskSizeGB(req.Template, requestedDiskSize)
	if err != nil {
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

	params := &service.LinkedCloneParams{
		Name:                 req.Name,
		Remark:               req.Remark,
		Template:             req.Template,
		TemplateType:         req.TemplateType,
		CloneMode:            req.CloneMode,
		VCPU:                 req.VCPU,
		RAM:                  req.RAM,
		DiskSize:             diskSize,
		Autostart:            req.Autostart,
		Freeze:               req.Freeze,
		APIC:                 req.APIC,
		PAE:                  req.PAE,
		RTCOffset:            req.RTCOffset,
		RTCStartDate:         req.RTCStartDate,
		GuestAgent:           req.GuestAgent,
		SMBIOS1:              req.SMBIOS1,
		BootType:             req.BootType,
		DiskBus:              req.DiskBus,
		VideoModel:           req.VideoModel,
		CPUTopologyMode:      req.CPUTopologyMode,
		CPULimitPercent:      req.CPULimitPercent,
		CPUAffinity:          req.CPUAffinity,
		FirstBootRebootMode:  req.FirstBootRebootMode,
		SwitchID:             req.SwitchID,
		SecurityGroupID:      req.SecurityGroupID,
		AllowedIPv4Addresses: req.AllowedIPv4Addresses,
		AllowedIPv6Addresses: req.AllowedIPv6Addresses,
		ExtraNics:            req.ExtraNics,
		StoragePoolID:        req.StoragePoolID,
		ExtraDisks:           req.ExtraDisks,
		HostDevices:          req.HostDevices,
		NicModel:             req.NicModel,
		SystemDiskIOPS:       req.SystemDiskIOPS,
		IsAdmin:              true,
	}

	username, _ := c.Get("username")
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeLinkedClone, params, username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交原生链式克隆任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "原生链式克隆任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}
