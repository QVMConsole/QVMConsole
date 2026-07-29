package middleware

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"kvm_console/config"
	"kvm_console/model"
	"kvm_console/service"
)

// Claims JWT 自定义声明
type Claims struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	TokenType   string `json:"token_type"`
	Operation   string `json:"operation,omitempty"`
	Fingerprint string `json:"fp,omitempty"` // 会话指纹
	jwt.RegisteredClaims
}

// GenerateToken 生成默认访问 Token
func GenerateToken(userID uint, username, role string) (string, error) {
	return GenerateTokenWithTTL(userID, username, role, service.TokenTypeAccess,
		time.Duration(config.GlobalConfig.JWTExpireHours)*time.Hour)
}

// GenerateTokenWithTTL 生成指定类型和有效期的 Token
func GenerateTokenWithTTL(userID uint, username, role, tokenType string, ttl time.Duration) (string, error) {
	return generateTokenInternal(userID, username, role, tokenType, "", "", ttl)
}

// GenerateTokenWithOperation 生成带操作范围的 Token
func GenerateTokenWithOperation(userID uint, username, role, tokenType, operation string, ttl time.Duration) (string, error) {
	return generateTokenInternal(userID, username, role, tokenType, operation, "", ttl)
}

// generateTokenInternal 内部 Token 生成，支持会话指纹
func generateTokenInternal(userID uint, username, role, tokenType, operation, fingerprint string, ttl time.Duration) (string, error) {
	if tokenType == "" {
		tokenType = service.TokenTypeAccess
	}
	claims := &Claims{
		UserID:      userID,
		Username:    username,
		Role:        role,
		TokenType:   tokenType,
		Operation:   operation,
		Fingerprint: fingerprint,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "qvmconsole",
			Audience:  jwt.ClaimStrings{"qvmconsole-api"},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GlobalConfig.JWTSecret))
}

// GenerateTokenWithContext 生成带会话指纹的 Token（从 gin.Context 提取 IP 和 User-Agent）
func GenerateTokenWithContext(c *gin.Context, userID uint, username, role, tokenType string, ttl time.Duration) (string, error) {
	fp := ""
	if isSessionFingerprintEnabled() {
		fp = GenerateSessionFingerprint(c.ClientIP(), c.GetHeader("User-Agent"))
	}
	return generateTokenInternal(userID, username, role, tokenType, "", fp, ttl)
}

// GenerateAccessTokenWithContext 生成带会话指纹的访问 Token
func GenerateAccessTokenWithContext(c *gin.Context, userID uint, username, role string) (string, error) {
	ttl := time.Duration(config.GlobalConfig.JWTExpireHours) * time.Hour
	return GenerateTokenWithContext(c, userID, username, role, service.TokenTypeAccess, ttl)
}

// ParseToken 解析 JWT Token
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.GlobalConfig.JWTSecret), nil
	}, jwt.WithLeeway(5*time.Second))
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		if claims.TokenType == "" {
			claims.TokenType = service.TokenTypeAccess
		}
		// iss/aud 校验（过渡期兼容：旧 token 无 iss/aud 时不拒绝）
		if !validateIssuerAudience(claims) {
			return nil, jwt.ErrTokenInvalidClaims
		}
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

// validateIssuerAudience 校验 iss/aud 声明，兼容无 iss/aud 的旧 token
func validateIssuerAudience(claims *Claims) bool {
	// 新 token 必须携带正确的 iss
	if claims.Issuer != "" && claims.Issuer != "qvmconsole" {
		return false
	}
	// 新 token 必须携带正确的 aud
	if len(claims.Audience) > 0 {
		for _, aud := range claims.Audience {
			if aud == "qvmconsole-api" {
				return true
			}
		}
		return false
	}
	// 无 iss 且无 aud 的旧 token 允许通过
	return true
}

// AuthMiddleware 仅允许正式访问 Token
func AuthMiddleware() gin.HandlerFunc {
	return TokenTypeMiddleware(service.TokenTypeAccess)
}

// TokenTypeMiddleware 允许指定类型的 Token
func TokenTypeMiddleware(allowedTypes ...string) gin.HandlerFunc {
	return tokenTypeMiddleware(true, allowedTypes...)
}

// JWTTokenTypeMiddleware 仅允许 JWT，不接受 API Key。
func JWTTokenTypeMiddleware(allowedTypes ...string) gin.HandlerFunc {
	return tokenTypeMiddleware(false, allowedTypes...)
}

func tokenTypeMiddleware(allowAPIKey bool, allowedTypes ...string) gin.HandlerFunc {
	allowed := make(map[string]bool)
	for _, tokenType := range allowedTypes {
		allowed[tokenType] = true
	}
	return func(c *gin.Context) {
		if apiKeyID, apiKey, ok := extractAPIKeyCredentials(c); ok {
			if !allowAPIKey || !allowed[service.TokenTypeAccess] {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "当前接口不支持 API Key 认证",
				})
				c.Abort()
				return
			}
			user, err := service.AuthenticateAPIKey(apiKeyID, apiKey)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			setAuthenticatedUser(c, user, service.TokenTypeAccess, "api_key", apiKeyID)
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			if token := c.Query("token"); token != "" {
				authHeader = "Bearer " + token
			}
		}
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未登录，请先登录",
			})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证格式无效",
			})
			c.Abort()
			return
		}

		claims, err := ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "Token 无效或已过期",
			})
			c.Abort()
			return
		}
		if !allowed[claims.TokenType] {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "当前登录状态不允许访问该接口",
			})
			c.Abort()
			return
		}

		// 指纹校验（仅JWT认证，API Key跳过）
		if claims.Fingerprint != "" && isSessionFingerprintEnabled() {
			currentFP := GenerateSessionFingerprint(c.ClientIP(), c.GetHeader("User-Agent"))
			if claims.Fingerprint != currentFP {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "登录环境发生变化，请重新登录",
				})
				c.Abort()
				return
			}
		}

		var user model.User
		if err := model.DB.First(&user, claims.UserID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "用户不存在或已被删除",
			})
			c.Abort()
			return
		}
		if user.Status == service.UserStatusDisabled {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "账号已被禁用",
			})
			c.Abort()
			return
		}
		if claims.TokenType == service.TokenTypeAccess && user.Status != service.UserStatusActive {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "账号尚未激活",
			})
			c.Abort()
			return
		}
		if user.SecurityUpdatedAt != nil && claims.IssuedAt != nil && claims.IssuedAt.Time.Before(*user.SecurityUpdatedAt) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "登录状态已失效，请重新登录",
			})
			c.Abort()
			return
		}

		setAuthenticatedUser(c, &user, claims.TokenType, "jwt", "")
		c.Next()
	}
}

func extractAPIKeyCredentials(c *gin.Context) (string, string, bool) {
	apiKeyID := strings.TrimSpace(firstHeader(c, "X-API-Key-ID", "X-API-ID", "X-KVM-API-Key-ID"))
	apiKey := strings.TrimSpace(firstHeader(c, "X-API-Key", "X-KVM-API-Key"))
	if apiKeyID != "" || apiKey != "" {
		return apiKeyID, apiKey, true
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	for _, prefix := range []string{"ApiKey ", "KVM-API-Key "} {
		if !strings.HasPrefix(authHeader, prefix) {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(authHeader, prefix)), ":", 2)
		if len(parts) != 2 {
			return "", "", true
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
	}
	return "", "", false
}

func firstHeader(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := c.GetHeader(name); value != "" {
			return value
		}
	}
	return ""
}

func setAuthenticatedUser(c *gin.Context, user *model.User, tokenType, authType, apiKeyID string) {
	c.Set("user_id", user.ID)
	c.Set("username", user.Username)
	c.Set("role", user.Role)
	c.Set("token_type", tokenType)
	c.Set("auth_type", authType)
	c.Set("current_user", user)
	if apiKeyID != "" {
		c.Set("api_key_id", apiKeyID)
	}
}

// AdminMiddleware 管理员权限中间件
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "需要管理员权限",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// ForcePasswordChangeMiddleware 强制修改密码中间件
// 当用户 ForcePasswordChange=true 时，仅允许访问修改密码和退出登录接口
func ForcePasswordChangeMiddleware() gin.HandlerFunc {
	// 白名单：这些路径在强制修改密码期间仍可访问
	whitelist := map[string]bool{
		"/api/auth/password":   true, // PUT 修改密码
		"/api/auth/info":       true, // GET 获取用户信息（含 force_password_change 标志）
		"/api/auth/logout":     true, // POST 退出登录
		"/api/public/settings": true, // GET 公共设置
		"/api/public/version":  true, // GET 版本
	}
	return func(c *gin.Context) {
		current, exists := c.Get("current_user")
		if !exists {
			c.Next()
			return
		}
		user, ok := current.(*model.User)
		if !ok || !user.ForcePasswordChange {
			c.Next()
			return
		}
		if authType, _ := c.Get("auth_type"); authType == "api_key" && user.ForcePasswordChangeReason == "password_breach" {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if whitelist[path] {
			c.Next()
			return
		}
		message := "请先修改默认密码后再使用其他功能"
		if user.ForcePasswordChangeReason == "password_breach" {
			message = "检测到当前管理员密码已泄露，请先修改密码后再使用其他功能"
		}
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": message,
			"data": gin.H{
				"force_password_change": true,
			},
		})
		c.Abort()
	}
}

// ElasticCloudOnlyMiddleware 禁止轻量云用户访问弹性云自助能力。
func ElasticCloudOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			c.Next()
			return
		}
		current, _ := c.Get("current_user")
		if user, ok := current.(*model.User); ok && service.IsLightweightCloudType(user.CloudType) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "轻量云用户无权使用此功能",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// VMAccessMiddleware VM访问权限中间件
// 非admin用户操作VM时，检查是否拥有该VM（通过URL参数 :name 获取VM名称）
func VMAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			c.Next()
			return
		}

		vmName := c.Param("name")
		if vmName == "" {
			c.Next()
			return
		}

		username, _ := c.Get("username")
		if !checkUserOwnsVM(username.(string), vmName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// checkUserOwnsVM 检查用户是否拥有指定VM（读取VM访问列表文件）
func checkUserOwnsVM(username, vmName string) bool {
	// 安全校验：用户名不允许包含路径分隔符、.. 序列和空字符
	if username == "" || strings.Contains(username, "..") || strings.ContainsAny(username, "/\\") || strings.ContainsRune(username, 0) {
		return false
	}
	// 使用 filepath.Join + 前缀校验双保险
	baseDir := filepath.Clean(config.GlobalConfig.VMAccessDir)
	filePath := filepath.Join(baseDir, username)
	if !strings.HasPrefix(filePath, baseDir+string(filepath.Separator)) && filePath != baseDir {
		return false
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == vmName {
			return true
		}
	}
	return false
}
