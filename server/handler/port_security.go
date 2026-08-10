package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

// GetPortSecurityStatus 获取端口安全运行状态和逐端口诊断信息。
func GetPortSecurityStatus(c *gin.Context) {
	status, err := service.GetPortSecurityStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": status})
}

// PreflightPortSecurity 执行全量只读预检，不改变开关或运行态流表。
func PreflightPortSecurity(c *gin.Context) {
	result, err := service.PreflightPortSecurity()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "预检完成", "data": result})
}

func EnablePortSecurity(c *gin.Context) {
	submitPortSecurityTask(c, "enable", "", "enable_port_security", true)
}

func DisablePortSecurity(c *gin.Context) {
	submitPortSecurityTask(c, "disable", "", "disable_port_security", true)
}

func ReconcilePortSecurity(c *gin.Context) {
	submitPortSecurityTask(c, "reconcile", "", "", false)
}

func IsolatePortSecurityPort(c *gin.Context) {
	submitPortSecurityTask(c, "isolate", c.Param("port"), "isolate_port_security_port", true)
}

func ReleasePortSecurityPort(c *gin.Context) {
	submitPortSecurityTask(c, "release", c.Param("port"), "release_port_security_port", true)
}

func submitPortSecurityTask(c *gin.Context, action, port, operation string, highRisk bool) {
	if highRisk && !requireHighRiskVerification(c, operation) {
		return
	}
	port = strings.TrimSpace(port)
	username, _ := c.Get("username")
	createdBy, _ := username.(string)
	task, err := taskqueue.SubmitWithStruct(model.TaskTypePortSecurity, service.PortSecurityTaskParams{Action: action, Port: port}, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交端口安全任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "message": "端口安全任务已提交",
		"data": gin.H{"task_id": task.ID, "status": task.Status},
	})
}
