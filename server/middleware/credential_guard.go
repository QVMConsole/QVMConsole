package middleware

import (
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/logger"
)

const (
	maxAPIKeyIDLength = 80
	maxAPIKeyLength   = 256
	maxJWTLength      = 4096
)

var credentialPartPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var apiKeyIDHeaders = []string{"X-API-Key-ID", "X-API-ID", "X-KVM-API-Key-ID"}
var apiKeyHeaders = []string{"X-API-Key", "X-KVM-API-Key"}

// CredentialGuardMiddleware 在任何认证查询前校验凭据格式与入口唯一性。
func CredentialGuardMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if reason := unsupportedAPIKeyCredentialReason(c); reason != "" {
			rejectCredential(c, reason)
			return
		}

		idValues, idCount := collectCredentialHeaders(c, apiKeyIDHeaders)
		keyValues, keyCount := collectCredentialHeaders(c, apiKeyHeaders)
		if idCount > 1 || keyCount > 1 {
			rejectCredential(c, "API Key 凭据头重复或使用了多个别名")
			return
		}
		if idCount == 1 && !validCredentialPart(idValues[0], maxAPIKeyIDLength) {
			rejectCredential(c, "API Key ID 格式无效")
			return
		}
		if keyCount == 1 && !validCredentialPart(keyValues[0], maxAPIKeyLength) {
			rejectCredential(c, "API Key 格式无效")
			return
		}
		if (idCount == 0) != (keyCount == 0) {
			rejectCredential(c, "API Key ID 与 API Key 必须同时提供")
			return
		}

		authorizationValues := c.Request.Header.Values("Authorization")
		if len(authorizationValues) > 1 {
			rejectCredential(c, "Authorization 请求头重复")
			return
		}
		if len(authorizationValues) == 1 {
			authorization := authorizationValues[0]
			if idCount > 0 {
				rejectCredential(c, "禁止混用 Authorization 与 API Key 请求头")
				return
			}
			if !validateAuthorizationCredential(authorization) {
				rejectCredential(c, "Authorization 凭据格式无效")
				return
			}
		}

		if values := c.Request.Header.Values("X-High-Risk-Token"); len(values) > 0 {
			if len(values) != 1 || !validJWT(values[0]) {
				rejectCredential(c, "高风险验证令牌格式无效")
				return
			}
		}

		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			if values, ok := c.Request.URL.Query()["token"]; ok {
				// 查询令牌与请求头凭据同时出现时会产生认证来源歧义，必须拒绝，
				// 避免认证中间件按固定优先级静默选择其中一种凭据。
				if len(values) > 0 && (idCount > 0 || keyCount > 0 || len(authorizationValues) > 0) {
					rejectCredential(c, "查询令牌与请求头凭据冲突")
					return
				}
				if len(values) != 1 || !validQueryToken(c.Request.URL.Path, values[0]) {
					rejectCredential(c, "查询令牌格式或使用位置无效")
					return
				}
			}
		}

		c.Next()
	}
}

func unsupportedAPIKeyCredentialReason(c *gin.Context) string {
	for key := range c.Request.URL.Query() {
		if strings.EqualFold(key, "apikey") {
			values := c.Request.URL.Query()[key]
			if len(values) != 1 || !validCredentialPart(values[0], maxAPIKeyLength) {
				return "API Key 参数格式无效"
			}
			return "不支持通过 apikey 查询参数认证"
		}
	}
	if values := c.Request.Header.Values("X-Request-Api-Key"); len(values) > 0 {
		if len(values) != 1 || !validCredentialPart(values[0], maxAPIKeyLength) {
			return "X-Request-Api-Key 请求头格式无效"
		}
		return "不支持通过 X-Request-Api-Key 请求头认证"
	}
	return ""
}

func collectCredentialHeaders(c *gin.Context, names []string) ([]string, int) {
	values := make([]string, 0, 1)
	count := 0
	for _, name := range names {
		for _, value := range c.Request.Header.Values(name) {
			count++
			values = append(values, value)
		}
	}
	return values, count
}

func validateAuthorizationCredential(value string) bool {
	if strings.HasPrefix(value, "Bearer ") {
		return validJWT(strings.TrimPrefix(value, "Bearer "))
	}
	for _, prefix := range []string{"ApiKey ", "KVM-API-Key "} {
		if strings.HasPrefix(value, prefix) {
			credential := strings.TrimPrefix(value, prefix)
			if strings.Count(credential, ":") != 1 {
				return false
			}
			parts := strings.SplitN(credential, ":", 2)
			return validCredentialPart(parts[0], maxAPIKeyIDLength) && validCredentialPart(parts[1], maxAPIKeyLength)
		}
	}
	// 未知认证方案交给认证中间件返回普通的 401。
	return !strings.ContainsAny(value, "\r\n\x00") && len(value) <= maxJWTLength
}

func validCredentialPart(value string, maxLength int) bool {
	return value != "" && len(value) <= maxLength && credentialPartPattern.MatchString(value)
}

func validJWT(value string) bool {
	if value == "" || len(value) > maxJWTLength || strings.TrimSpace(value) != value {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !validCredentialPart(part, maxJWTLength) {
			return false
		}
		if _, err := base64.RawURLEncoding.DecodeString(part); err != nil {
			return false
		}
	}
	return true
}

func validQueryToken(path, value string) bool {
	if path == "/api/auth/invite" {
		return validCredentialPart(value, 256)
	}
	// 浏览器 EventSource/WebSocket 以及下载链接无法稳定附加 Authorization，
	// 现有客户端也会在存储、模板和抓包下载中使用 JWT 查询参数；统一只校验
	// JWT 格式，由具体认证中间件决定该路由是否接受查询令牌。
	return validJWT(value)
}

func rejectCredential(c *gin.Context, reason string) {
	logger.App.Warn("认证凭据被拒绝", "ip", c.ClientIP(), "method", c.Request.Method, "path", c.Request.URL.Path, "reason", reason)
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "认证凭据格式无效"})
}
