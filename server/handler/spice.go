package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/service"
)

// handler/spice.go — SPICE 显示协议 HTTP 接口。
// 与 handler/vnc.go 同构：状态查询、开启/关闭、改密、对外暴露、连接信息。
// SPICE 不走面板 WS 代理（客户端用 virt-viewer/spicy 直连），故无 WebSocket 接口；
// 取而代之的是 .vv 连接文件下载。

// SpiceStatusResponse SPICE 状态响应
type SpiceStatusResponse = service.SpiceInfo

// SpicePasswordRequest SPICE 密码请求
type SpicePasswordRequest struct {
	Password string `json:"password"`
}

// SpiceExposeRequest SPICE 暴露请求
type SpiceExposeRequest struct {
	Expose bool `json:"expose"`
}

// GetSpiceStatus 获取 SPICE 状态
func GetSpiceStatus(c *gin.Context) {
	name := c.Param("name")
	info, err := service.GetSpiceStatus(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    info,
	})
}

// EnableSpice 开启 SPICE（敏感操作，需二次验证）
func EnableSpice(c *gin.Context) {
	if !requireHighRiskVerification(c, "enable_spice") {
		return
	}
	name := c.Param("name")
	var req SpicePasswordRequest
	c.ShouldBindJSON(&req) // 密码可选

	if err := service.EnableSpice(name, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "开启 SPICE 失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "SPICE 已开启",
	})
}

// DisableSpice 关闭 SPICE（敏感操作，需二次验证）
func DisableSpice(c *gin.Context) {
	if !requireHighRiskVerification(c, "disable_spice") {
		return
	}
	name := c.Param("name")
	if err := service.DisableSpice(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "关闭 SPICE 失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "SPICE 已关闭",
	})
}

// ChangeSpicePassword 修改 SPICE 密码（敏感操作，需二次验证）
func ChangeSpicePassword(c *gin.Context) {
	if !requireHighRiskVerification(c, "change_spice_password") {
		return
	}
	name := c.Param("name")
	var req SpicePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请输入新密码",
		})
		return
	}
	if err := service.ChangeSpicePassword(name, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "修改密码失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "SPICE 密码已修改",
	})
}

// ExposeSpice 切换 SPICE 对外暴露（敏感操作，需二次验证；会联动宿主防火墙端口）
func ExposeSpice(c *gin.Context) {
	if !requireHighRiskVerification(c, "expose_spice") {
		return
	}
	name := c.Param("name")
	var req SpiceExposeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数无效",
		})
		return
	}
	if err := service.ExposeSpice(name, req.Expose); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "操作失败: " + err.Error(),
		})
		return
	}
	msg := "SPICE 已关闭对外暴露"
	if req.Expose {
		msg = "SPICE 已开启对外暴露"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": msg,
	})
}

// GetSpiceConnInfoHandler 获取 SPICE 连接信息（host:port + 密码，供用户手动连接）
func GetSpiceConnInfoHandler(c *gin.Context) {
	name := c.Param("name")
	info, err := service.GetSpiceConnInfo(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    info,
	})
}

// DownloadSpiceVV 下载 SPICE .vv 连接文件（virt-viewer / spicy 原生格式）
func DownloadSpiceVV(c *gin.Context) {
	name := c.Param("name")
	info, err := service.GetSpiceConnInfo(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}
	// 关机态 SPICE 端口由 QEMU 运行时分配（autoport），未运行时无法获取，.vv 缺端口会连不上
	if info.Port == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机未运行，SPICE 端口尚未分配，请先启动虚拟机后再下载连接文件",
		})
		return
	}
	// delete=0/false 时生成可重复使用文件（连接后不自动删除）；默认一次性（连接后自删）
	deleteThisFile := true
	if d := c.Query("delete"); d == "0" || d == "false" {
		deleteThisFile = false
	}
	vv := service.BuildSpiceVVFile(info, name, deleteThisFile)
	c.Header("Content-Disposition", "attachment; filename="+name+".vv")
	c.Data(http.StatusOK, "application/x-virt-viewer", []byte(vv))
}
