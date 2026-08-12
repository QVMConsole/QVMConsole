package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

func ListPublicIPs(c *gin.Context) {
	items, err := service.ListPublicIPs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": items})
}

func CreatePublicIP(c *gin.Context) {
	var req service.PublicIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	row, err := service.CreatePublicIP(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "公网 IP 已添加", "data": row})
}

func DiscoverPublicIPv6Prefixes(c *gin.Context) {
	items, err := service.DiscoverPublicIPv6Prefixes(c.Query("uplink_if"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": items})
}

func ImportPublicIPv6Prefix(c *gin.Context) {
	var req service.PublicIPv6PrefixImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	result, err := service.ImportPublicIPv6Prefix(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "IPv6 前缀已导入", "data": result})
}

func UpdatePublicIP(c *gin.Context) {
	id, err := service.ParsePublicIPID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var req service.PublicIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	row, err := service.UpdatePublicIP(id, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "公网 IP 已更新", "data": row})
}

func DeletePublicIP(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_public_ip") {
		return
	}
	id, err := service.ParsePublicIPID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := service.DeletePublicIP(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "公网 IP 已删除"})
}

func PreviewPublicIP(c *gin.Context) {
	id, err := service.ParsePublicIPID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var req service.PublicIPBindRequest
	_ = c.ShouldBindJSON(&req)
	preview, err := service.PreviewPublicIPBinding(id, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": preview})
}

func BindPublicIP(c *gin.Context) {
	if !requireHighRiskVerification(c, "bind_public_ip") {
		return
	}
	id, err := service.ParsePublicIPID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var req service.PublicIPBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	submitPublicIPTask(c, service.PublicIPOperationParams{Action: "bind", PublicIPID: id, BindRequest: req})
}

func UnbindPublicIP(c *gin.Context) {
	if !requireHighRiskVerification(c, "unbind_public_ip") {
		return
	}
	id, err := service.ParsePublicIPID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	submitPublicIPTask(c, service.PublicIPOperationParams{Action: "unbind", PublicIPID: id})
}

func MigratePublicIP(c *gin.Context) {
	if !requireHighRiskVerification(c, "migrate_public_ip") {
		return
	}
	id, err := service.ParsePublicIPID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var req service.PublicIPBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	submitPublicIPTask(c, service.PublicIPOperationParams{Action: "migrate", PublicIPID: id, BindRequest: req})
}

func ApplyPublicIPRules(c *gin.Context) {
	if !requireHighRiskVerification(c, "apply_public_ip") {
		return
	}
	submitPublicIPTask(c, service.PublicIPOperationParams{Action: "apply_all"})
}

func submitPublicIPTask(c *gin.Context, params service.PublicIPOperationParams) {
	username, _ := c.Get("username")
	createdBy, _ := username.(string)
	task, err := taskqueue.SubmitWithStruct(model.TaskTypePublicIPApply, params, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交公网 IP 任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "公网 IP 任务已提交",
		"data": gin.H{
			"task_id": task.ID,
			"status":  task.Status,
		},
	})
}
