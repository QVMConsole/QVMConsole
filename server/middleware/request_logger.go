package middleware

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"time"

	"kvm_console/config"
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

// bodyCaptureWriter 包装 gin.ResponseWriter，用于捕获响应体内容用于请求日志。
//   - 只缓冲前 maxBodyBytes 字节，超出即停止缓冲并标记截断，避免大响应拖垮内存与日志文件
//   - 一旦调用 Flush（SSE/流式响应特征）立即切换为直写模式并清空缓冲，绝不阻塞流式推送
type bodyCaptureWriter struct {
	gin.ResponseWriter
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
	streaming bool
}

// Write 缓冲响应体并透传原 writer。
func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	if !w.streaming && !w.truncated {
		remaining := w.maxBytes - w.buf.Len()
		if len(b) > remaining {
			w.buf.Write(b[:remaining])
			w.truncated = true
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

// WriteHeader 透传状态码。
func (w *bodyCaptureWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

// Flush 实现 http.Flusher：SSE 等流式响应会周期调用，一旦触发即停止捕获。
func (w *bodyCaptureWriter) Flush() {
	w.streaming = true
	w.buf.Reset()
	w.ResponseWriter.Flush()
}

// bodyPreview 返回捕获到的响应体（含截断标记），流式响应或空响应返回空串。
func (w *bodyCaptureWriter) bodyPreview() string {
	if w.streaming || w.buf.Len() == 0 {
		return ""
	}
	s := w.buf.String()
	if w.truncated {
		s += "\n...[响应体已截断]"
	}
	return s
}

// isStreamingSSEPath 判断是否为 SSE 流式推送路径，这类响应不能缓冲捕获响应体。
func isStreamingSSEPath(path string) bool {
	return strings.Contains(path, "/sse")
}

// RequestLoggerMiddleware 自定义 GIN 请求日志中间件，使用 slog 按状态码分级记录。
// 受开关（配置项 request_detail_log_enabled，默认开启）控制：
//   - 开启：记录基础请求信息（方法/路径/状态码/耗时/来源/用户），并额外记录脱敏后的响应体 JSON；
//   - 关闭：完全停止请求日志记录（request.log 不再新增任何记录），已写入的历史记录保留在文件中。
func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开关关闭时跳过请求日志记录
		if config.GlobalConfig == nil || !config.GlobalConfig.RequestDetailLogEnabled {
			c.Next()
			return
		}

		start := time.Now()
		path := requestLogPath(c.Request.URL.Path, c.Request.URL.RawQuery)

		// 捕获响应体：仅当详细日志开关开启且非 SSE 流式路径
		var capture *bodyCaptureWriter
		if !isStreamingSSEPath(c.Request.URL.Path) {
			maxBytes := 8192
			if config.GlobalConfig.RequestLogMaxBodyBytes > 0 {
				maxBytes = config.GlobalConfig.RequestLogMaxBodyBytes
			}
			capture = &bodyCaptureWriter{
				ResponseWriter: c.Writer,
				maxBytes:       maxBytes,
			}
			c.Writer = capture
		}

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

		// 组装通用属性
		attrs := []any{
			"method", method,
			"path", path,
			"status", status,
			"latency", latency.String(),
			"ip", clientIP,
			"user", user,
		}

		// 响应体详情（脱敏后）
		if capture != nil {
			if detail := capture.bodyPreview(); detail != "" {
				if redacted, ok := redactResponseJSON([]byte(detail)); ok {
					attrs = append(attrs, "body", redacted)
				} else {
					attrs = append(attrs, "body", fmt.Sprintf("[非JSON响应 %d 字节]", len(detail)))
				}
			}
		}

		// 根据状态码选择日志级别
		if status >= 500 {
			attrs = append(attrs, "errors", c.Errors.ByType(gin.ErrorTypePrivate).String())
			logger.Request.Error("请求处理", attrs...)
		} else if status >= 400 {
			logger.Request.Warn("请求处理", attrs...)
		} else {
			logger.Request.Info("请求处理", attrs...)
		}
	}
}