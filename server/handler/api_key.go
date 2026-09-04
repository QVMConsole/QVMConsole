package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/service"
)

type RotateAPIKeyRequest struct {
	TrustedIP string `json:"trusted_ip"`
}

// GetAPIKeyInfo 获取当前用户 API Key 元信息。
func GetAPIKeyInfo(c *gin.Context) {
	user := getCurrentUser(c)
	info, err := service.GetUserAPIKeyInfo(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取 API 凭证失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": info})
}

// RotateAPIKey 生成或重新生成当前用户 API Key。
func RotateAPIKey(c *gin.Context) {
	if authType, _ := c.Get("auth_type"); authType == "api_key" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "API Key 创建或轮换必须使用 JWT 登录会话并完成二次验证"})
		return
	}
	user := getCurrentUser(c)
	var req RotateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if service.IsAdminAPIKeyPublicPolicyEnabled(user) {
		normalized, err := service.NormalizeTrustedIP(req.TrustedIP)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		req.TrustedIP = normalized
		if !requireDualAPIKeyVerification(c, user, "rotate_api_key") {
			return
		}
	} else if !requireHighRiskVerification(c, "rotate_api_key") {
		return
	}
	key, err := service.RotateUserAPIKey(user, req.TrustedIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成 API 凭证失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "API 凭证已生成，请立即复制保存 API Key", "data": key})
}

// RevokeAPIKey 撤销当前用户 API Key。
func RevokeAPIKey(c *gin.Context) {
	if authType, _ := c.Get("auth_type"); authType == "api_key" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "API Key 撤销必须使用 JWT 登录会话"})
		return
	}
	if !requireHighRiskVerification(c, "revoke_api_key") {
		return
	}
	user := getCurrentUser(c)
	if err := service.RevokeUserAPIKey(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "撤销 API 凭证失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "API 凭证已撤销"})
}
