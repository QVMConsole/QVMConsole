package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/middleware"
	"kvm_console/model"
	"kvm_console/service"
)

func getCurrentUser(c *gin.Context) *model.User {
	user, _ := c.Get("current_user")
	currentUser, _ := user.(*model.User)
	return currentUser
}

func buildBaseURL(c *gin.Context) string {
	if configured := normalizeBaseURL(config.GlobalConfig.PublicBaseURL); configured != "" {
		return configured
	}

	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil || c.Request.URL.Scheme == "https" {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func normalizeBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return "http://" + trimmed
}

func requireHighRiskVerification(c *gin.Context, operation string) bool {
	return requireHighRiskVerificationWithOptions(c, operation)
}

func requireStrictHighRiskVerification(c *gin.Context, operation string) bool {
	// API Key 调用允许 API Key 的业务接口时不触发交互式二次验证；
	// 账户安全和凭证管理流程通过路由级 JWT-only 或处理函数单独限制。
	return requireHighRiskVerificationWithOptions(c, operation)
}

func requireHighRiskVerificationWithOptions(c *gin.Context, operation string) bool {
	if authType, _ := c.Get("auth_type"); authType == "api_key" {
		// API Key 本身已经完成密钥哈希、账号状态、权限和接口授权校验。
		// 对允许 API Key 的业务接口不再触发浏览器交互式 428；需要更强
		// 保护的账户安全入口必须在路由或处理函数中显式限制 API Key。
		return true
	}
	if service.IsSecurityVerificationDisabled() {
		return true
	}
	user := getCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "用户未登录",
		})
		return false
	}
	if service.CanSkipHighRiskVerification(user) {
		return true
	}
	// SMTP 未配置时，回退到 TOTP 验证（如果已启用）；TOTP 也未启用则跳过
	if !service.IsSMTPConfigured() {
		if user.TOTPEnabled {
			// TOTP 已启用，走 TOTP 验证流程（不在此处 return，让下面的 TOTP 逻辑处理）
		} else {
			// SMTP 和 TOTP 都不可用，无法进行二次验证
			return true
		}
	}

	if operation == "" {
		operation = "high_risk_operation"
	}

	if highRiskTokenMatches(c, user, operation, "") {
		return true
	}

	if user.TOTPEnabled {
		respData := gin.H{
			"method":    service.ChallengeMethodTOTP,
			"operation": operation,
		}
		if service.HasRecoveryCodes(user) {
			respData["has_recovery"] = true
		}
		c.JSON(http.StatusPreconditionRequired, gin.H{
			"code":    http.StatusPreconditionRequired,
			"message": "当前操作需要 2FA 验证",
			"data":    respData,
		})
		return false
	}

	if user.EmailVerifiedAt == nil || strings.TrimSpace(user.Email) == "" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "请先绑定并验证邮箱后再执行该操作",
		})
		return false
	}
	if !service.IsSMTPConfigured() {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "当前未配置 SMTP，暂时无法完成邮箱验证",
		})
		return false
	}
	challenge, err := service.IssueEmailChallenge(
		user,
		service.ChallengePurposeHighRiskEmail,
		user.Email,
		"高风险操作验证",
		fmt.Sprintf("您正在执行高风险操作（%s），请输入以下验证码继续。", operation),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "发送邮箱验证码失败: " + err.Error(),
		})
		return false
	}

	c.JSON(http.StatusPreconditionRequired, gin.H{
		"code":    http.StatusPreconditionRequired,
		"message": "当前操作需要邮箱验证",
		"data": gin.H{
			"method":       service.ChallengeMethodEmail,
			"operation":    operation,
			"challenge_id": challenge.ID,
			"masked_email": service.MaskEmail(user.Email),
			"expires_in":   int(service.EmailCodeTTL.Seconds()),
		},
	})
	return false
}

func highRiskTokenMatches(c *gin.Context, user *model.User, operation, requiredLevel string) bool {
	if user == nil {
		return false
	}
	token := strings.TrimSpace(c.GetHeader("X-High-Risk-Token"))
	if token == "" {
		return false
	}
	claims, err := middleware.ParseToken(token)
	if err != nil || claims.TokenType != service.TokenTypeHighRisk || claims.UserID != user.ID || claims.Operation != operation {
		return false
	}
	if requiredLevel == "" {
		return true
	}
	if claims.VerificationLevel == requiredLevel {
		return true
	}
	return requiredLevel == "totp" && claims.VerificationLevel == "totp_email"
}

// requireFreshTOTPVerification 强制当前管理员重新完成 2FA，不使用历史信任窗口。
func requireFreshTOTPVerification(c *gin.Context, operation string) bool {
	user := getCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "用户未登录"})
		return false
	}
	if !user.TOTPEnabled {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "请先启用 2FA"})
		return false
	}
	if highRiskTokenMatches(c, user, operation, "totp") {
		return true
	}
	data := gin.H{"method": service.ChallengeMethodTOTP, "operation": operation}
	if service.HasRecoveryCodes(user) {
		data["has_recovery"] = true
	}
	c.JSON(http.StatusPreconditionRequired, gin.H{
		"code": http.StatusPreconditionRequired, "message": "当前操作需要新的 2FA 验证", "data": data,
	})
	return false
}

// requireDualAPIKeyVerification 强制管理员同时完成 2FA 与邮箱验证码校验。
func requireDualAPIKeyVerification(c *gin.Context, user *model.User, operation string) bool {
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "用户未登录"})
		return false
	}
	if !user.TOTPEnabled {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "请先启用 2FA"})
		return false
	}
	if user.EmailVerifiedAt == nil || strings.TrimSpace(user.Email) == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "请先绑定并验证邮箱"})
		return false
	}
	if !service.IsSMTPConfigured() {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "当前未配置 SMTP，无法完成邮箱验证"})
		return false
	}
	if highRiskTokenMatches(c, user, operation, "totp_email") {
		return true
	}
	challenge, err := service.IssueEmailChallenge(
		user,
		service.ChallengePurposeHighRiskEmail,
		user.Email,
		"API 凭证双重验证",
		"您正在创建或轮换管理员 API 凭证，请同时完成 2FA 与邮箱验证码校验。",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "发送邮箱验证码失败: " + err.Error()})
		return false
	}
	data := gin.H{
		"method": service.ChallengeMethodTOTPEmail, "operation": operation,
		"challenge_id": challenge.ID, "masked_email": service.MaskEmail(user.Email),
		"expires_in": int(service.EmailCodeTTL.Seconds()),
	}
	if service.HasRecoveryCodes(user) {
		data["has_recovery"] = true
	}
	c.JSON(http.StatusPreconditionRequired, gin.H{
		"code": http.StatusPreconditionRequired, "message": "当前操作需要 2FA 与邮箱双重验证", "data": data,
	})
	return false
}

func requireMaintenanceModeDisabled(c *gin.Context, action string) bool {
	if err := service.EnsureMaintenanceModeDisabled(action); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return false
	}
	return true
}
