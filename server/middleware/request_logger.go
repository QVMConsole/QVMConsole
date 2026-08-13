package middleware

import (
	"net/url"
	"strings"
	"time"

	"kvm_console/logger"

	"github.com/gin-gonic/gin"
)

const redactedQueryValue = "[REDACTED]"

// requestLogSensitiveQueryKeys 是请求日志中不能记录原文的查询参数。
// WebSocket/SSE 连接会在查询参数中携带访问令牌，日志中只能保留其是否存在。
var requestLogSensitiveQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"authorization": {},
	"code":          {},
	"key":           {},
	"password":      {},
	"passwd":        {},
	"refresh_token": {},
	"secret":        {},
	"token":         {},
}

func isSensitiveRequestLogQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if _, ok := requestLogSensitiveQueryKeys[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.HasSuffix(normalized, "_key")
}

func requestLogPath(path, rawQuery string) string {
	if rawQuery == "" {
		return path
	}

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		// 非法查询串不写入日志，避免绕过脱敏规则。
		return path
	}
	for key := range query {
		if isSensitiveRequestLogQueryKey(key) {
			query[key] = []string{redactedQueryValue}
		}
	}
	return path + "?" + query.Encode()
}

// RequestLoggerMiddleware 自定义 GIN 请求日志中间件，使用 slog 按状态码分级记录
func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := requestLogPath(c.Request.URL.Path, c.Request.URL.RawQuery)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		// 从 context 获取用户名（JWT 中间件设置的）
		username, _ := c.Get("username")
		user := ""
		if username != nil {
			if u, ok := username.(string); ok {
				user = u
			}
		}

		// 根据状态码选择日志级别
		if status >= 500 {
			logger.Request.Error("请求处理",
				"method", method,
				"path", path,
				"status", status,
				"latency", latency.String(),
				"ip", clientIP,
				"user", user,
				"errors", c.Errors.ByType(gin.ErrorTypePrivate).String(),
			)
		} else if status >= 400 {
			logger.Request.Warn("请求处理",
				"method", method,
				"path", path,
				"status", status,
				"latency", latency.String(),
				"ip", clientIP,
				"user", user,
			)
		} else {
			logger.Request.Info("请求处理",
				"method", method,
				"path", path,
				"status", status,
				"latency", latency.String(),
				"ip", clientIP,
				"user", user,
			)
		}
	}
}
