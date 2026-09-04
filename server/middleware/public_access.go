package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/service"
)

const (
	ContextIsLANRequest    = "is_lan_request"
	ContextIsPublicRequest = "is_public_request"
)

// PublicAccessMiddleware 在路由匹配前统一限制公网请求。
func PublicAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		isLAN := service.IsLANIP(c.ClientIP())
		c.Set(ContextIsLANRequest, isLAN)
		c.Set(ContextIsPublicRequest, !isLAN)
		if !isLAN && !config.GlobalConfig.PublicAccessEnabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "公网访问已关闭",
			})
			return
		}
		c.Next()
	}
}

// IsPublicRequest 返回当前请求是否来自非局域网地址。
func IsPublicRequest(c *gin.Context) bool {
	value, _ := c.Get(ContextIsPublicRequest)
	isPublic, _ := value.(bool)
	return isPublic
}

// StreamingSessionValid 检查已建立的公网流式连接是否仍可继续。
func StreamingSessionValid(c *gin.Context) bool {
	if !IsPublicRequest(c) {
		return true
	}
	userIDValue, _ := c.Get("user_id")
	userID, _ := userIDValue.(uint)
	sessionValue, _ := c.Get("session_id")
	sessionID, _ := sessionValue.(string)
	_, err := service.ValidatePublicSession(sessionID, userID)
	return err == nil
}
