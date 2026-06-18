package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"kvm_console/service"
)

func ListHostInterfaces(c *gin.Context) {
	items, err := service.ListHostPhysicalInterfaces()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": items})
}

func ListNetworkBridges(c *gin.Context) {
	items, err := service.ListNetworkBridges()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": items})
}

func CreateNetworkBridge(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_network_bridge") {
		return
	}
	var req service.NetworkBridgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	bridge, err := service.CreateNetworkBridge(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "网桥已创建", "data": bridge})
}

func DeleteNetworkBridge(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_network_bridge") {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	name := c.Query("name")
	// 当 ID 为 0 但提供了名称时，按名称删除（处理 OVS 残留网桥）
	if id == 0 && name != "" {
		if err := service.DeleteNetworkBridgeByName(name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "网桥已删除"})
		return
	}
	if err := service.DeleteNetworkBridge(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "网桥已删除"})
}

// GetInterfaceConfig 获取接口（网桥或物理网卡）的当前 IP/DNS 配置。
func GetInterfaceConfig(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "接口名称不能为空"})
		return
	}
	info, err := service.GetInterfaceConfig(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": info})
}

// SetInterfaceConfig 设置接口的 IP/DNS 配置（高风险操作，需二次验证）。
func SetInterfaceConfig(c *gin.Context) {
	if !requireHighRiskVerification(c, "set_interface_config") {
		return
	}
	var req service.SetInterfaceConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	// 接口名称从 URL 路径参数获取（前端通过 path 传递）
	req.Name = c.Param("name")
	info, err := service.SetInterfaceConfig(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "接口配置已更新", "data": info})
}
