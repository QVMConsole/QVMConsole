package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm/vmimport"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
)

// ==================== 虚拟机导出 ====================

// ExportVMRequest 导出虚拟机请求
type ExportVMRequest struct {
	VMName string `json:"vm_name" binding:"required"`
}

// ExportVMHandler 导出虚拟机（用户自助）
func ExportVMHandler(c *gin.Context) {
	var req ExportVMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 需要 vm_name",
		})
		return
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)

	// 检查用户是否拥有该 VM
	if !service.UserOwnsVM(usernameStr, req.VMName) {
		role, _ := c.Get("role")
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机",
			})
			return
		}
	}

	// 检查存储池是否初始化
	if !service.IsStorageInitialized(usernameStr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "存储池未初始化，请先初始化存储池",
		})
		return
	}

	// 检查配额：要求可用空间 >= 4GB（实际写入超标由文件系统 quota 硬限制阻止）
	storageInfo, _ := service.GetUserStorageInfo(usernameStr)
	if storageInfo != nil && storageInfo.MaxBytes > 0 {
		freeBytes := storageInfo.MaxBytes - storageInfo.UsedBytes
		const minFreeBytes int64 = 4 * 1024 * 1024 * 1024 // 4GB
		if freeBytes < minFreeBytes {
			c.JSON(http.StatusForbidden, gin.H{
				"code": 403,
				"message": fmt.Sprintf("存储可用空间不足 4GB（当前可用 %s），请先删除部分文件后再导出",
					service.FormatBytesPublic(freeBytes)),
			})
			return
		}
	}

	// 提交导出任务
	params := &service.ExportVMParams{
		VMName:   req.VMName,
		Username: usernameStr,
	}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeExport, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交导出任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "导出任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// ==================== 虚拟机导入 ====================

// ImportVMRequest 导入虚拟机请求
type ImportVMRequest struct {
	Name             string                            `json:"name" binding:"required"`
	Remark           string                            `json:"remark"`
	DiskFile         string                            `json:"disk_file" binding:"required"`
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
	StartAfterImport *bool                             `json:"start_after_import"` // 导入完成后是否开启虚拟机，不传默认 true
}

// ImportVMHandler 导入虚拟机（用户自助）
func ImportVMHandler(c *gin.Context) {
	if !requireMaintenanceModeDisabled(c, "导入并启动虚拟机") {
		return
	}
	var req ImportVMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 名称、磁盘文件、CPU、内存为必填项",
		})
		return
	}

	// 名称验证
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

	// 检查存储池是否初始化
	if !service.IsStorageInitialized(usernameStr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "存储池未初始化，请先初始化存储池",
		})
		return
	}

	// 检查磁盘文件是否存在
	diskDir := service.GetUserDiskDir(usernameStr)
	diskPath := filepath.Join(diskDir, req.DiskFile)
	if _, err := os.Stat(diskPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "磁盘文件不存在: " + req.DiskFile,
		})
		return
	}

	// 配额检查（CPU/内存）
	if err := service.CheckQuota(usernameStr, req.VCPU, req.RAM, 0); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}
	if role != "admin" {
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
	}

	params := &vmimport.ImportVMParams{
		Name:             req.Name,
		Remark:           req.Remark,
		DiskFile:         req.DiskFile,
		Username:         usernameStr,
		CopyDisk:         req.CopyDisk,
		VCPU:             req.VCPU,
		RAM:              req.RAM,
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
		IsAdmin:          role == "admin",
	}
	// 默认导入后开启虚拟机（向后兼容）
	if req.StartAfterImport != nil {
		params.StartAfterImport = *req.StartAfterImport
	} else {
		params.StartAfterImport = true
	}
	if role != "admin" {
		params.MemoryDynamic = sanitizeUserMemoryDynamicRequest(req.MemoryDynamic, req.RAM)
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeImport, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交导入任务失败: " + err.Error(),
		})
		return
	}

	// 将VM添加到用户访问列表
	_ = service.AddVMToUser(usernameStr, req.Name)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "导入任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// ==================== 磁盘文件上传 ====================

// UploadDiskFile 上传磁盘文件到用户 disk 目录
func UploadDiskFile(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr := username.(string)

	// 检查存储池是否初始化
	if !service.IsStorageInitialized(usernameStr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "存储池未初始化，请先初始化",
		})
		return
	}

	// 检查是否只读
	if service.IsStorageReadonly(usernameStr) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "存储空间已超出配额，处于只读模式，请先删除部分文件",
		})
		return
	}

	// 获取上传文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "未收到上传文件: " + err.Error(),
		})
		return
	}
	defer file.Close()

	// 磁盘文件后缀检查
	nameLower := strings.ToLower(header.Filename)
	validExts := []string{".qcow2", ".raw", ".vmdk", ".vhd", ".vhdx", ".img"}
	validExt := false
	for _, ext := range validExts {
		if strings.HasSuffix(nameLower, ext) {
			validExt = true
			break
		}
	}
	if !validExt {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": fmt.Sprintf("虚拟磁盘仅支持以下格式: %s", strings.Join(validExts, ", ")),
		})
		return
	}

	// 安全检查文件名
	filename := filepath.Base(header.Filename)
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "非法文件名",
		})
		return
	}

	// 检查配额
	if err := service.CheckStorageQuota(usernameStr, header.Size); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}

	destDir := service.GetUserDiskDir(usernameStr)
	destPath := filepath.Join(destDir, filename)

	// 保存文件
	out, err := os.Create(destPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建文件失败: " + err.Error(),
		})
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		os.Remove(destPath)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "写入文件失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "磁盘文件上传成功",
		"data": gin.H{
			"filename": filename,
			"size":     header.Size,
		},
	})
}

// ==================== 用户磁盘文件列表 ====================

// ListUserDiskFiles 列出用户的虚拟磁盘文件
func ListUserDiskFiles(c *gin.Context) {
	username, _ := c.Get("username")

	files, err := service.ListUserFiles(username.(string), "disk")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取磁盘文件列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    files,
	})
}
