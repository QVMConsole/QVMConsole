package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/service/vm/vmimport"
	"kvm_console/taskqueue"
)

// InspectApplianceRequest 是虚拟机包检查请求。
type InspectApplianceRequest struct {
	ApplianceFile string `json:"appliance_file"`
	AppliancePath string `json:"appliance_path"`
	SourceType    string `json:"source_type"`
}

// ImportApplianceRequest 复用普通磁盘导入的硬件配置，并增加虚拟机包来源字段。
type ImportApplianceRequest struct {
	ImportDiskByPathRequest
	ApplianceFile string `json:"appliance_file"`
	AppliancePath string `json:"appliance_path"`
	SourceType    string `json:"source_type"`
	ConfigMode    string `json:"config_mode"`
	CopySource    bool   `json:"copy_source"`
}

// InspectSelfAppliance 检查我的存储中的 OVF/OVA。
func InspectSelfAppliance(c *gin.Context) {
	inspectAppliance(c, false)
}

// InspectAdminAppliance 检查管理员指定的 OVF/OVA。
func InspectAdminAppliance(c *gin.Context) {
	inspectAppliance(c, true)
}

func inspectAppliance(c *gin.Context, admin bool) {
	var req InspectApplianceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请提供虚拟机包来源"})
		return
	}
	username, _ := c.Get("username")
	req.SourceType = normalizeApplianceSourceType(req.SourceType, req.AppliancePath, admin)
	params := &vmimport.ImportApplianceParams{
		ImportDiskByPathParams: vmimport.ImportDiskByPathParams{Username: username.(string)},
		ApplianceFile:          req.ApplianceFile,
		AppliancePath:          req.AppliancePath,
		SourceType:             req.SourceType,
	}
	if !admin {
		params.SourceType = "storage"
		params.AppliancePath = ""
	}
	if params.SourceType != "storage" && params.SourceType != "path" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "虚拟机包来源仅支持 storage 或 path"})
		return
	}
	if params.SourceType == "storage" || !admin {
		if !service.IsStorageInitialized(username.(string)) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先开通我的存储"})
			return
		}
	}
	metadata, err := vmimport.InspectAppliance(params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "虚拟机包检查完成", "data": metadata})
}

// ImportSelfAppliance 从我的存储导入虚拟机包。
func ImportSelfAppliance(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_vm") {
		return
	}
	importApplianceHandler(c, false)
}

// ImportAdminAppliance 通过管理员来源导入虚拟机包。
func ImportAdminAppliance(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_vm") {
		return
	}
	importApplianceHandler(c, true)
}

func importApplianceHandler(c *gin.Context, admin bool) {
	if !requireMaintenanceModeDisabled(c, "导入虚拟机包") {
		return
	}
	var req ImportApplianceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "虚拟机名称、CPU、内存和虚拟机包为必填项"})
		return
	}
	if err := service.ValidateVMName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if !validateVMNameNotExists(c, req.Name) {
		return
	}
	username, _ := c.Get("username")
	usernameStr := username.(string)
	req.SourceType = normalizeApplianceSourceType(req.SourceType, req.AppliancePath, admin)
	req.ConfigMode = vmimport.NormalizeApplianceConfigMode(req.ConfigMode)
	if req.ConfigMode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "虚拟机包配置方式仅支持 ovf 或 custom"})
		return
	}
	if !admin {
		req.StoragePoolID = ""
		req.SourceType = "storage"
		req.AppliancePath = ""
	}
	if req.SourceType != "storage" && req.SourceType != "path" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "虚拟机包来源仅支持 storage 或 path"})
		return
	}
	if req.SourceType == "storage" || !admin {
		if !service.IsStorageInitialized(usernameStr) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先开通我的存储"})
			return
		}
	}

	startAfterImport := true
	if req.StartAfterImport != nil {
		startAfterImport = *req.StartAfterImport
	}
	params := buildImportApplianceParams(req, usernameStr, admin, startAfterImport)
	// 提交阶段只做无磁盘读取的来源语法校验；归档、清单、架构和配额由异步任务复核。
	if _, err := vmimport.ResolveApplianceSourcePath(params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if !admin {
		if req.SwitchID != 0 {
			switchID, securityGroupID, err := service.ResolveVPCForVMCreate(usernameStr, req.SwitchID, req.SecurityGroupID)
			if err != nil {
				c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": err.Error()})
				return
			}
			params.SwitchID = switchID
			params.SecurityGroupID = securityGroupID
		}
		params.MemoryDynamic = sanitizeUserMemoryDynamicRequest(req.MemoryDynamic, req.RAM)
	}
	if !validateSwitchBridges(c, params.SwitchID, params.ExtraNics) {
		return
	}
	if err := service.AddVMToUser(usernameStr, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "预留虚拟机归属失败: " + err.Error()})
		return
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeImportAppliance, params, usernameStr)
	if err != nil {
		_ = service.RemoveVMFromUser(usernameStr, req.Name)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交虚拟机包导入任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "message": "虚拟机包导入任务已提交，包校验与配额检查将在任务中执行",
		"data": gin.H{"task_id": task.ID},
	})
}

func normalizeApplianceSourceType(sourceType, appliancePath string, admin bool) string {
	if !admin {
		return "storage"
	}
	normalized := strings.ToLower(strings.TrimSpace(sourceType))
	if normalized == "" {
		if strings.TrimSpace(appliancePath) != "" {
			return "path"
		}
		return "storage"
	}
	return normalized
}

func buildImportApplianceParams(req ImportApplianceRequest, username string, admin, startAfterImport bool) *vmimport.ImportApplianceParams {
	return &vmimport.ImportApplianceParams{
		ImportDiskByPathParams: vmimport.ImportDiskByPathParams{
			Name: req.Name, Remark: req.Remark, StoragePoolID: req.StoragePoolID,
			VCPU: req.VCPU, RAM: req.RAM, InitType: req.InitType, Hostname: req.Hostname,
			User: req.User, Password: req.Password, Autostart: req.Autostart, Freeze: req.Freeze,
			APIC: req.APIC, PAE: req.PAE, RTCOffset: req.RTCOffset, RTCStartDate: req.RTCStartDate,
			GuestAgent: req.GuestAgent, SMBIOS1: req.SMBIOS1, BootType: req.BootType,
			MachineType: req.MachineType, NicModel: req.NicModel, VideoModel: req.VideoModel,
			SpiceEnabled: req.SpiceEnabled, CPUTopologyMode: req.CPUTopologyMode,
			CPULimitPercent: req.CPULimitPercent, CPUAffinity: req.CPUAffinity,
			TemplateRootPass: req.TemplateRootPass, TemplateUser: req.TemplateUser,
			MemoryDynamic: req.MemoryDynamic, SwitchID: req.SwitchID, SecurityGroupID: req.SecurityGroupID,
			ExtraNics: req.ExtraNics, SystemDiskIOPS: req.SystemDiskIOPS,
			StartAfterImport: startAfterImport, KVMHidden: req.KVMHidden, VendorID: req.VendorID,
			Username: username,
		},
		ApplianceFile: req.ApplianceFile, AppliancePath: req.AppliancePath,
		SourceType: req.SourceType, ConfigMode: req.ConfigMode,
		CopySource: req.CopySource, IsAdmin: admin,
	}
}
