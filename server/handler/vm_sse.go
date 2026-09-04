package handler

// VM SSE 实时推送相关 handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/middleware"
	"kvm_console/service"
)

// GetVmListSSE SSE 实时推送虚拟机列表及状态
func GetVmListSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	clientGone := c.Request.Context().Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	listOptions := buildVMListOptions(c)
	role, _ := c.Get("role")
	isAdmin := role == "admin"

	// 立即发送一次
	if isAdmin {
		service.TriggerAdminVMCacheRefreshIfNeeded()
	}
	if vms, err := service.ListCachedVMs(listOptions); err == nil {
		c.SSEvent("vm_list", vms)
		c.Writer.Flush()
	}

	for {
		select {
		case <-clientGone:
			return
		case <-ticker.C:
			if !middleware.StreamingSessionValid(c) {
				c.SSEvent("session_expired", gin.H{"message": "登录会话因长时间未操作已失效，请重新登录"})
				c.Writer.Flush()
				return
			}
			if isAdmin {
				service.TriggerAdminVMCacheRefreshIfNeeded()
			}
			vms, err := service.ListCachedVMs(listOptions)
			if err != nil {
				if service.IsLibvirtUnavailableError(err) {
					c.SSEvent("vm_list", []service.VmInfo{})
					c.Writer.Flush()
				}
				continue
			}
			c.SSEvent("vm_list", vms)
			c.Writer.Flush()
		}
	}
}

// GetVmDetailSSE SSE 实时推送虚拟机详情
func GetVmDetailSSE(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	clientGone := c.Request.Context().Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// 立即发送一次
	if vm, err := service.GetVM(name); err == nil {
		c.SSEvent("vm_detail", vm)
		c.Writer.Flush()
	}

	for {
		select {
		case <-clientGone:
			return
		case <-ticker.C:
			if !middleware.StreamingSessionValid(c) {
				c.SSEvent("session_expired", gin.H{"message": "登录会话因长时间未操作已失效，请重新登录"})
				c.Writer.Flush()
				return
			}
			vm, err := service.GetVM(name)
			if err != nil {
				continue
			}
			c.SSEvent("vm_detail", vm)
			c.Writer.Flush()
		}
	}
}
