package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/service"
	securitypkg "kvm_console/service/security"
)

// GetPasswordBreachStatus 获取密码泄露扫描与受影响账户状态。
func GetPasswordBreachStatus(c *gin.Context) {
	status, activeTask, err := service.GetPasswordBreachStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取密码泄露状态失败: " + err.Error()})
		return
	}
	data := gin.H{"status": status}
	if activeTask != nil {
		data["active_task"] = activeTask
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": data})
}

// StartPasswordBreachScan 立即提交密码泄露扫描，不受检测开关限制。
func StartPasswordBreachScan(c *gin.Context) {
	if !requireStrictHighRiskVerification(c, "run_password_breach_scan") {
		return
	}
	createdBy := c.GetString("username")
	if createdBy == "" {
		createdBy = "admin"
	}
	task, reused, err := service.SubmitPasswordBreachScan(securitypkg.PasswordBreachScanSourceManual, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交密码泄露检测失败: " + err.Error()})
		return
	}
	message := "密码泄露检测任务已提交"
	if reused {
		message = "已有密码泄露检测任务正在执行，已返回现有任务"
	}
	c.JSON(http.StatusAccepted, gin.H{
		"code": 202, "message": message, "data": gin.H{"task": task, "reused": reused},
	})
}
