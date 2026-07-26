package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

// GetTemplateList 获取模板列表
func GetTemplateList(c *gin.Context) {
	templates, err := service.ListTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取模板列表失败: " + err.Error(),
		})
		return
	}
	role, _ := c.Get("role")
	if role != "admin" {
		filtered := make([]service.TemplateInfo, 0, len(templates))
		for _, tpl := range templates {
			if tpl.CloneVisible && !tpl.Disabled {
				filtered = append(filtered, tpl)
			}
		}
		templates = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    templates,
	})
}

// PrepareTemplateRequest 制作模板请求
type PrepareTemplateRequest struct {
	VMName           string `json:"vm_name" binding:"required"`
	TemplateName     string `json:"template_name" binding:"required"`
	DisplayName      string `json:"display_name"`
	Type             string `json:"type"`                         // linux/windows/fnos
	Category         string `json:"category"`                     // 二级分类，当前用于 Linux 发行版和 Windows 版本
	RootPassword     string `json:"root_password"`                // 模板 root 密码
	TemplateUser     string `json:"template_user"`                // 模板中的普通用户名
	CloudInitMode    string `json:"cloud_init_mode,omitempty"`    // 初始化模式: "nocloud"/"configdrive"/"fnos"/"none"
	PostBootCommand  string `json:"post_boot_command,omitempty"`  // Linux 模板启动后执行的自定义命令
	PostBootBlocking bool   `json:"post_boot_blocking,omitempty"` // 启动后命令阻塞模式
}

// PrepareTemplate 制作模板（异步任务）
func PrepareTemplate(c *gin.Context) {
	var req PrepareTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 需要 vm_name 和 template_name",
		})
		return
	}

	username, _ := c.Get("username")
	task, err := taskqueue.SubmitWithStruct(model.TaskTypePrepare, req, username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "模板制作任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// PrepareImportedLinuxTemplate 为历史导入的 Linux 模板补齐离线克隆依赖。
func PrepareImportedLinuxTemplate(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少模板名称"})
		return
	}
	if err := service.ValidateTemplateName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	username, _ := c.Get("username")
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeTemplateLinuxPrepare, struct {
		TemplateName string `json:"template_name"`
	}{TemplateName: name}, username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交模板预处理任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Linux 模板离线克隆依赖预处理任务已提交",
		"data":    gin.H{"task_id": task.ID},
	})
}

// GetTemplateVMs 获取模板关联虚拟机列表
func GetTemplateVMs(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缺少模板名称",
		})
		return
	}

	vms, err := service.ListTemplateVMs(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取模板关联虚拟机失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    vms,
	})
}

// DeleteTemplateRequest 删除模板请求
type DeleteTemplateRequest struct {
	DeleteVMs   bool     `json:"delete_vms"`
	ExpectedVMs []string `json:"expected_vms"`
	DeleteMode  string   `json:"delete_mode"`
}

// GetDeleteTemplatePreview 获取删除模板链路预览
func GetDeleteTemplatePreview(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缺少模板名称",
		})
		return
	}
	preview, err := service.GetDeleteTemplatePreview(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取模板删除预览失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    preview,
	})
}

// DeleteTemplate 删除模板
func DeleteTemplate(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_template") {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缺少模板名称",
		})
		return
	}

	var req DeleteTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "删除参数错误",
		})
		return
	}

	preview, err := service.GetDeleteTemplatePreview(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取模板关联虚拟机失败: " + err.Error(),
		})
		return
	}
	deleteMode := strings.TrimSpace(req.DeleteMode)
	if deleteMode == "" {
		deleteMode = service.TemplateDeleteModeCascade
	}
	if deleteMode != service.TemplateDeleteModePromote && deleteMode != service.TemplateDeleteModePromoteHot && len(preview.RelatedVMs) > 0 && !req.DeleteVMs {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "该模板链路创建的虚拟机尚未确认删除，请重新确认后再操作",
			"data":    preview,
		})
		return
	}

	username, _ := c.Get("username")
	params := &service.DeleteTemplateParams{
		TemplateName: name,
		DeleteVMs:    req.DeleteVMs,
		ExpectedVMs:  req.ExpectedVMs,
		DeleteMode:   deleteMode,
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeDeleteTemplate, params, username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交删除模板任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除模板任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// UpdateTemplateMetaRequest 更新模板展示配置请求（兼容旧路径）
type UpdateTemplateMetaRequest struct {
	AdminName           string `json:"admin_name"`
	DisplayName         string `json:"display_name"`
	CloneVisible        bool   `json:"clone_visible"`
	Disabled            bool   `json:"disabled"`
	Category            string `json:"category"`
	VCPU                int    `json:"vcpu"`
	RAM                 int    `json:"ram"`
	DiskSize            int    `json:"disk_size"`
	DiskBus             string `json:"disk_bus"`
	NicModel            string `json:"nic_model"`
	VideoModel          string `json:"video_model"`
	CPUTopologyMode     string `json:"cpu_topology_mode"`
	FirstBootRebootMode string `json:"first_boot_reboot_mode"`
	PostBootCommand     string `json:"post_boot_command"`
	PostBootBlocking    bool   `json:"post_boot_blocking"`
}

// UpdateTemplateMeta 更新模板展示配置
func UpdateTemplateMeta(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缺少模板名称",
		})
		return
	}

	var req UpdateTemplateMetaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	params := &service.UpdateTemplateMetaParams{
		AdminName:           strings.TrimSpace(req.AdminName),
		DisplayName:         strings.TrimSpace(req.DisplayName),
		CloneVisible:        req.CloneVisible,
		Disabled:            req.Disabled,
		Category:            strings.TrimSpace(req.Category),
		VCPU:                req.VCPU,
		RAM:                 req.RAM,
		DiskSize:            req.DiskSize,
		DiskBus:             req.DiskBus,
		NicModel:            req.NicModel,
		VideoModel:          req.VideoModel,
		CPUTopologyMode:     req.CPUTopologyMode,
		FirstBootRebootMode: req.FirstBootRebootMode,
		PostBootCommand:     req.PostBootCommand,
		PostBootBlocking:    req.PostBootBlocking,
	}

	if err := service.UpdateTemplateMeta(name, params); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新模板元数据失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "模板展示配置更新成功",
	})
}

// UpdateTemplatePublish 更新模板发布展示配置
func UpdateTemplatePublish(c *gin.Context) {
	UpdateTemplateMeta(c)
}
