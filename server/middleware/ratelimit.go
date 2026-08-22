package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
)

// RateLimitConfig 限频配置
type RateLimitConfig struct {
	// 公开接口每 IP 每分钟最大请求数（默认 20）
	PublicPerMinute int
	// 认证接口每 IP 每分钟最大请求数（默认 60）
	AuthPerMinute int
	// 清理过期条目的间隔（默认 5 分钟）
	CleanupInterval time.Duration
}

// DefaultRateLimitConfig 默认限频配置
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		PublicPerMinute: 20,
		AuthPerMinute:   0, // 0 表示不限制
		CleanupInterval: 5 * time.Minute,
	}
}

// rateLimitEntry 单个 IP 的计数器
type rateLimitEntry struct {
	count       int
	windowStart time.Time
}

// RateLimiter IP 级别滑动窗口限频器
type RateLimiter struct {
	mu      sync.RWMutex
	entries map[string]*rateLimitEntry
	config  RateLimitConfig
	stopCh  chan struct{}
}

// NewRateLimiter 创建限频器
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateLimitEntry),
		config:  config,
		stopCh:  make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// Stop 停止清理协程
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// Allow 检查指定 IP 是否允许请求，返回 (允许, 剩余次数, 重置秒数)
func (rl *RateLimiter) Allow(ip string, isPublic bool) (bool, int, int) {
	limit := rl.config.AuthPerMinute
	if isPublic {
		limit = rl.config.PublicPerMinute
	}

	// limit 为 0 表示不限制，直接放行
	if limit == 0 {
		return true, -1, 0
	}

	now := time.Now()
	windowDuration := time.Minute

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 防止内存泄漏：entries 超过上限时触发全量清理
	const maxEntries = 100000
	if len(rl.entries) > maxEntries {
		// 紧急清理：删除所有已过期的条目
		now := time.Now()
		for k, v := range rl.entries {
			if now.Sub(v.windowStart) > time.Minute {
				delete(rl.entries, k)
			}
		}
		// 如果清理后仍超限，清空所有
		if len(rl.entries) > maxEntries {
			rl.entries = make(map[string]*rateLimitEntry)
		}
	}

	entry, exists := rl.entries[ip]
	if !exists || now.Sub(entry.windowStart) > windowDuration {
		// 新窗口
		rl.entries[ip] = &rateLimitEntry{
			count:       1,
			windowStart: now,
		}
		return true, limit - 1, int(windowDuration.Seconds())
	}

	// 在窗口内
	entry.count++
	if entry.count > limit {
		remaining := 0
		resetSeconds := int(windowDuration.Seconds() - now.Sub(entry.windowStart).Seconds())
		if resetSeconds < 1 {
			resetSeconds = 1
		}
		return false, remaining, resetSeconds
	}

	remaining := limit - entry.count
	resetSeconds := int(windowDuration.Seconds() - now.Sub(entry.windowStart).Seconds())
	if resetSeconds < 1 {
		resetSeconds = 1
	}
	return true, remaining, resetSeconds
}

// cleanupLoop 定期清理过期条目
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-time.Minute)
	for ip, entry := range rl.entries {
		if entry.windowStart.Before(cutoff) {
			delete(rl.entries, ip)
		}
	}
}

// 判断请求路径是否属于公开接口（无需认证的接口）
var publicPaths = map[string]bool{
	"/api/public/settings":      true,
	"/api/public/version":       true,
	"/api/auth/login":           true,
	"/api/auth/invite":          true,
	"/api/auth/invite/complete": true,
	"/api/auth/password/forgot": true,
	"/api/auth/password/reset":  true,
}

func isPublicPath(path string) bool {
	if publicPaths[path] {
		return true
	}
	// 忘记密码子路径也视为公开
	if len(path) > 22 && path[:22] == "/api/auth/password/for" {
		return true
	}
	return false
}

// GetClientIP 获取客户端真实 IP，优先取 X-Forwarded-For / X-Real-IP（仅供可信代理场景）
func GetClientIP(c *gin.Context) string {
	// 仅当配置了可信代理且请求来自可信代理时才读取 X-Forwarded-For
	if len(config.GlobalConfig.TrustedProxies) > 0 {
		remoteAddr := c.Request.RemoteAddr
		// 提取 IP（去掉端口）
		remoteIP := remoteAddr
		if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
			remoteIP = remoteAddr[:idx]
		}
		remoteIP = strings.Trim(remoteIP, "[]") // 处理 IPv6

		if isTrustedProxy(remoteIP) {
			if ip := c.GetHeader("X-Forwarded-For"); ip != "" {
				for i := 0; i < len(ip); i++ {
					if ip[i] == ',' {
						return strings.TrimSpace(ip[:i])
					}
				}
				return strings.TrimSpace(ip)
			}
			if ip := c.GetHeader("X-Real-IP"); ip != "" {
				return strings.TrimSpace(ip)
			}
		}
	}
	return c.ClientIP()
}

// isTrustedProxy 检查 IP 是否在可信代理列表中
func isTrustedProxy(ip string) bool {
	for _, trusted := range config.GlobalConfig.TrustedProxies {
		if trusted == ip {
			return true
		}
	}
	return false
}

// RateLimitMiddleware 全局限频中间件
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := GetClientIP(c)
		isPublic := isPublicPath(c.Request.URL.Path)

		allowed, remaining, resetSeconds := limiter.Allow(ip, isPublic)
		if !allowed {
			c.Header("X-RateLimit-Limit", "0")
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", "0")
			c.Header("Retry-After", "60")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		c.Header("X-RateLimit-Remaining", itoa(remaining))
		c.Header("X-RateLimit-Reset", itoa(resetSeconds))
		c.Next()
	}
}

// itoa 小整数转字符串，避免导入 strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
