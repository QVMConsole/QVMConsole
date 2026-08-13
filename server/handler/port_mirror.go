package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

// GetPortMirrorOptions 获取源接口与目标空交换机选项。
func GetPortMirrorOptions(c *gin.Context) {
	options, err := service.GetPortMirrorOptions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": options})
}

// GetPortMirrorStatus 从 tc 与 OVS 回读端口镜像实时状态。
func GetPortMirrorStatus(c *gin.Context) {
	status, err := service.GetPortMirrorStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": status})
}

// EnablePortMirror 预检后提交端口镜像启用任务。
func EnablePortMirror(c *gin.Context) {
	var req service.PortMirrorEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "端口镜像参数错误"})
		return
	}
	if _, err := service.PreflightPortMirror(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if !requireHighRiskVerification(c, "enable_port_mirror") {
		return
	}
	submitPortMirrorTask(c, service.PortMirrorTaskParams{Action: "enable", Request: req})
}

// DisablePortMirror 提交端口镜像停用和清理任务。
func DisablePortMirror(c *gin.Context) {
	if !requireHighRiskVerification(c, "disable_port_mirror") {
		return
	}
	submitPortMirrorTask(c, service.PortMirrorTaskParams{Action: "disable"})
}

func submitPortMirrorTask(c *gin.Context, params service.PortMirrorTaskParams) {
	username, _ := c.Get("username")
	createdBy, _ := username.(string)
	task, err := taskqueue.SubmitWithStruct(model.TaskTypePortMirror, params, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交端口镜像任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "message": "端口镜像任务已提交",
		"data": gin.H{"task_id": task.ID, "status": task.Status},
	})
}
