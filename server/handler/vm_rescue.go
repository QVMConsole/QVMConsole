package handler

// VM 救援系统和密码重置相关 handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

// RescueVm 启动/关闭救援系统（异步任务）
func RescueVm(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}
	if err := service.EnsureVMNotMigrating(name, "救援系统"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	var req RescueVmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请指定操作类型（action: start/stop）",
		})
		return
	}

	if req.Action != "start" && req.Action != "stop" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "不支持的操作，仅支持 start 或 stop",
		})
		return
	}

	// 启动救援时检查是否已配置救援 ISO
	if req.Action == "start" {
		if !requireMaintenanceModeDisabled(c, "启动救援模式") {
			return
		}
		rescueISO := config.GlobalConfig.RescueISO
		if rescueISO == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "未配置救援系统 ISO，请先在系统设置中选择救援 ISO",
			})
			return
		}
	}

	// 获取操作用户
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	// 提交异步任务
	params := map[string]string{
		"vm_name": name,
		"action":  req.Action,
	}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeRescue, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交救援任务失败: " + err.Error(),
		})
		return
	}

	actionText := "启动"
	if req.Action == "stop" {
		actionText = "关闭"
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("%s救援系统任务已提交", actionText),
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// ResetLinuxPassword 重置虚拟机密码（异步任务）
func ResetLinuxPassword(c *gin.Context) {
	if !requireHighRiskVerification(c, "reset_vm_password") {
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
	if err := service.EnsureVMNotMigrating(name, "重置密码"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	var req ResetLinuxPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请提供用户名和新密码",
		})
		return
	}

	vm, err := service.GetVM(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}

	if err := service.ValidateResetGuestPasswordParams(req.Username, req.Password, vm.OSType); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	operator, _ := c.Get("username")
	task, err := service.SubmitResetLinuxPasswordTask(&service.ResetLinuxPasswordParams{
		VMName:   name,
		Username: req.Username,
		Password: req.Password,
		Mode:     req.Mode,
	}, operator.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交重置密码任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "重置密码任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}
