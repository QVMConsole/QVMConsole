package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/middleware"
	"kvm_console/service"
)

// ReportSessionActivity 仅在真实用户操作时刷新公网会话活动时间。
func ReportSessionActivity(c *gin.Context) {
	if !middleware.IsPublicRequest(c) {
		c.JSON(http.StatusOK, gin.H{
			"code": 200, "message": "ok",
			"data": gin.H{"public_session": false},
		})
		return
	}
	user := getCurrentUser(c)
	sessionID, _ := c.Get("session_id")
	idleExpiresAt, err := service.TouchUserSession(stringValue(sessionID), user.ID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "登录会话因长时间未操作已失效，请重新登录"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "message": "ok",
		"data": gin.H{
			"public_session":       true,
			"idle_expires_at":      idleExpiresAt.Format(time.RFC3339),
			"idle_timeout_seconds": int(service.PublicSessionIdleTimeout.Seconds()),
		},
	})
}

// Logout 撤销当前正式登录会话。
func Logout(c *gin.Context) {
	user := getCurrentUser(c)
	sessionID, _ := c.Get("session_id")
	if err := service.RevokeUserSession(stringValue(sessionID), user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "退出登录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "已退出登录"})
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}
