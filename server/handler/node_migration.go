package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/service/vm/migration"
	"kvm_console/taskqueue"
)

func ListHostNodes(c *gin.Context) {
	nodes, err := service.ListHostNodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取节点列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": nodes})
}

func CreateHostNode(c *gin.Context) {
	var req service.HostNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数无效"})
		return
	}
	node, err := service.CreateHostNode(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": node})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "节点已创建", "data": node})
}

func UpdateHostNode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req service.HostNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数无效"})
		return
	}
	node, err := service.UpdateHostNode(uint(id), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": node})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "节点已更新", "data": node})
}

func DeleteHostNode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := service.DeleteHostNode(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "节点已删除"})
}

func ProbeHostNode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	node, err := service.ProbeHostNode(uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": node})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "节点探测通过", "data": node})
}

func GetNodeMigrationOptions(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	vmName := strings.TrimSpace(c.Query("vm_name"))
	if vmName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "虚拟机名称不能为空"})
		return
	}
	options, err := migration.GetVMMigrationOptions(vmName, uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": options})
}

func PreviewVMMigration(c *gin.Context) {
	var req migration.VMMigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数无效"})
		return
	}
	preview, err := migration.CreateVMMigrationPreview(c.Param("name"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": preview})
}

func MigrateVM(c *gin.Context) {
	if !requireHighRiskVerification(c, "migrate_vm") {
		return
	}
	if err := migration.EnsureVMNotMigrating(c.Param("name"), "迁移"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}
	var req migration.VMMigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数无效"})
		return
	}
	username, _ := c.Get("username")
	params := migration.VMMigrationTaskParams{
		VMName:                c.Param("name"),
		NodeID:                req.NodeID,
		Mode:                  req.Mode,
		PreviewID:             req.PreviewID,
		SkipPrecheck:          req.SkipPrecheck,
		TargetStoragePoolID:   req.TargetStoragePoolID,
		DiskStorageTargets:    req.DiskStorageTargets,
		TargetSwitchID:        req.TargetSwitchID,
		TargetSecurityGroupID: req.TargetSecurityGroupID,
		EnableCPUThrottle:     req.EnableCPUThrottle,
		CPUThrottlePercent:    req.CPUThrottlePercent,
	}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeVMMigrate, params, username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交迁移任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "迁移任务已提交", "data": task})
}

func AdoptMigratedVM(c *gin.Context) {
	var req migration.MigrationAdoptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数无效"})
		return
	}
	result, err := migration.AdoptMigratedVM(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "目标面板已接管虚拟机", "data": result})
}
