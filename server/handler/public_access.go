package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/service"
)

type UpdatePublicAccessRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// UpdatePublicAccess 修改公网访问状态。
func UpdatePublicAccess(c *gin.Context) {
	var req UpdatePublicAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	enabled := *req.Enabled
	if enabled {
		user := getCurrentUser(c)
		if user != nil && user.ForcePasswordChange {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "请先完成当前管理员密码初始化，再开启公网访问"})
			return
		}
	}
	if enabled == config.GlobalConfig.PublicAccessEnabled {
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "公网访问状态未变化", "data": gin.H{"enabled": enabled, "revoked_api_keys": 0}})
		return
	}

	if enabled {
		if config.GlobalConfig.DevelopmentMode {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "开发模式下不能开启公网访问，请先关闭开发模式"})
			return
		}
		usernames, err := service.ListActiveAdminsWithoutTOTP()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "检查管理员 2FA 状态失败"})
			return
		}
		if len(usernames) > 0 {
			c.JSON(http.StatusConflict, gin.H{
				"code": 409, "message": "全部有效管理员必须先启用 2FA",
				"data": gin.H{"admins_without_2fa": usernames},
			})
			return
		}
		if !requireFreshTOTPVerification(c, "enable_public_access") {
			return
		}
	}

	revoked, err := service.SetPublicAccessEnabled(enabled, enabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新公网访问状态失败: " + err.Error()})
		return
	}
	message := "公网访问已关闭"
	if enabled {
		message = "公网访问已开启，现有管理员 API Key 已撤销"
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "message": message,
		"data": gin.H{"enabled": enabled, "revoked_api_keys": revoked},
	})
}
