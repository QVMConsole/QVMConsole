package handler

import (
	"fmt"
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

// BatchCreatePublicIPs 批量新增公网 IP（管理员）
func BatchCreatePublicIPs(c *gin.Context) {
	var req service.PublicIPBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	result, err := service.BatchCreatePublicIPs(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("成功新增 %d 个公网 IP（跳过 %d，失败 %d）", result.Created, result.Skipped, result.Failed),
		"data":    result,
	})
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

// BatchDeletePublicIPs 批量删除公网 IP（管理员，高风险）。
// 已绑定的 IP 自动跳过，部分失败不影响其他 IP。
func BatchDeletePublicIPs(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_public_ip") {
		return
	}
	var req service.PublicIPBatchOpRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: 需要公网 IP ID 列表"})
		return
	}
	result, err := service.BatchDeletePublicIPs(req.IDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("批量删除完成（成功 %d / 失败 %d / 跳过 %d）", result.Success, result.Failed, result.Skipped),
		"data":    result,
	})
}

// BatchBindPublicIPs 批量绑定公网 IP（管理员，高风险）。
// 通过任务队列异步应用规则，只提交一个任务，逐条绑定后统一应用。
func BatchBindPublicIPs(c *gin.Context) {
	if !requireHighRiskVerification(c, "bind_public_ip") {
		return
	}
	var req service.PublicIPBatchOpRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: 需要批量绑定条目"})
		return
	}
	submitPublicIPTask(c, service.PublicIPOperationParams{Action: "batch_bind", BatchItems: req.Items})
}

// BatchUnbindPublicIPs 批量解绑公网 IP（管理员，高风险）。
// 通过任务队列异步应用规则，只提交一个任务，逐条解绑后统一应用。
func BatchUnbindPublicIPs(c *gin.Context) {
	if !requireHighRiskVerification(c, "unbind_public_ip") {
		return
	}
	var req service.PublicIPBatchOpRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: 需要公网 IP ID 列表"})
		return
	}
	items := make([]service.PublicIPBatchOpItem, 0, len(req.IDs))
	for _, id := range req.IDs {
		items = append(items, service.PublicIPBatchOpItem{PublicIPID: id})
	}
	submitPublicIPTask(c, service.PublicIPOperationParams{Action: "batch_unbind", BatchItems: items})
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
