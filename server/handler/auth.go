package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/middleware"
	"kvm_console/model"
	"kvm_console/service"
	securitypkg "kvm_console/service/security"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginStageResponse struct {
	Stage                     string                `json:"stage"`
	Token                     string                `json:"token,omitempty"`
	Username                  string                `json:"username"`
	Role                      string                `json:"role"`
	CloudType                 string                `json:"cloud_type"`
	Security                  service.SecurityState `json:"security"`
	AllowedMethods            []string              `json:"allowed_methods,omitempty"`
	ForcePasswordChange       bool                  `json:"force_password_change,omitempty"`
	ForcePasswordChangeReason string                `json:"force_password_change_reason,omitempty"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type ChangeUsernameRequest struct {
	NewUsername string `json:"new_username" binding:"required"`
	Password    string `json:"password" binding:"required"`
}

type EmailCodeSendRequest struct {
	Email string `json:"email"`
}

type EmailBindRequest struct {
	Email       string `json:"email" binding:"required"`
	Code        string `json:"code" binding:"required"`
	ChallengeID uint   `json:"challenge_id" binding:"required"`
}

type LoginVerifyRequest struct {
	Method      string `json:"method" binding:"required"`
	Code        string `json:"code" binding:"required"`
	ChallengeID uint   `json:"challenge_id"`
}

type TOTPEnableRequest struct {
	Secret string `json:"secret" binding:"required"`
	Code   string `json:"code" binding:"required"`
}

type TOTPDisableRequest struct {
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

type HighRiskVerifyRequest struct {
	Method      string `json:"method" binding:"required"`
	Code        string `json:"code" binding:"required"`
	ChallengeID uint   `json:"challenge_id"`
	Operation   string `json:"operation" binding:"required"`
}

type InviteCompleteRequest struct {
	Token           string `json:"token" binding:"required"`
	Password        string `json:"password" binding:"required"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

type PasswordForgotRequest struct {
	Email string `json:"email" binding:"required"`
}

type PasswordForgotVerifyRequest struct {
	Email       string `json:"email" binding:"required"`
	Code        string `json:"code" binding:"required"`
	ChallengeID uint   `json:"challenge_id" binding:"required"`
}

type PasswordForgotSelectRequest struct {
	SelectionToken string `json:"selection_token" binding:"required"`
	Username       string `json:"username" binding:"required"`
}

type PasswordResetRequest struct {
	Token           string `json:"token" binding:"required"`
	Password        string `json:"password" binding:"required"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

type CheckPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

// CheckPasswordBreach 检查密码是否在泄露数据库中（公开接口，用于前端实时校验）
func CheckPasswordBreach(c *gin.Context) {
	var req CheckPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入密码"})
		return
	}

	// 如果泄露检测开关已关闭，直接返回未泄露
	if !config.GlobalConfig.PasswordBreachCheckEnabled {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data": gin.H{
				"enabled":  false,
				"breached": false,
			},
		})
		return
	}

	breached, fallback, err := securitypkg.CheckPasswordBreached(req.Password)
	if err != nil && !fallback {
		// API 不可用且本地也未匹配，不阻断用户操作
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data": gin.H{
				"enabled":  true,
				"breached": false,
				"warning":  "泄露密码检测服务暂时不可用，已跳过在线检测",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"enabled":  true,
			"breached": breached,
		},
	})
}

// Login 用户登录
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入用户名和密码"})
		return
	}

	// 登录爆破防护
	clientIP := c.ClientIP()
	if allowed, reason := service.CheckLoginAllowed(clientIP, strings.TrimSpace(req.Username)); !allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": reason})
		return
	}

	var user model.User
	if err := model.DB.Where("username = ?", strings.TrimSpace(req.Username)).First(&user).Error; err != nil {
		service.RecordLoginFailure(clientIP, strings.TrimSpace(req.Username))
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "用户名或密码错误"})
		return
	}
	if user.Status == service.UserStatusPendingInvite {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "该账户尚未完成注册，请先完成邀请注册"})
		return
	}
	if user.Status == service.UserStatusDisabled {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "该账户已被禁用"})
		return
	}
	if strings.TrimSpace(user.PasswordHash) == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		service.RecordLoginFailure(clientIP, strings.TrimSpace(req.Username))
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "用户名或密码错误"})
		return
	}
	service.ClearLoginFailures(clientIP, user.Username)

	// 旧账户首次成功校验密码时登记安全指纹；定时检测开启时同步执行一次在线检查。
	checkPasswordNow := config.GlobalConfig.ScheduledPasswordBreachCheckEnabled && !user.ForcePasswordChange
	enrollment, enrollErr := service.EnrollAndCheckPassword(&user, req.Password, checkPasswordNow)
	if enrollErr != nil {
		logger.App.Warn("登录时登记或检查密码安全指纹失败", "user", user.Username, "error", enrollErr)
	}
	if enrollment.NewlyDetected {
		service.SubmitPasswordBreachNotification(user.ID)
	}
	if err := model.DB.First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "刷新账户安全状态失败"})
		return
	}
	// 开发模式与其他安全验证保持一致，允许使用泄露标记账户继续调试。
	if !service.IsSecurityVerificationDisabled() && user.Role == "admin" && user.PasswordBreached && !user.TOTPEnabled {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "检测到管理员密码已泄露且未绑定 2FA，请在服务器运行 sudo bash qvmc-manage.sh 并选择修改管理员密码",
			"data": gin.H{
				"reason":           "admin_password_breached_no_2fa",
				"recovery_command": "sudo bash qvmc-manage.sh",
			},
		})
		return
	}

	security := service.BuildSecurityState(&user)

	// 检查是否需要强制修改密码（默认管理员账号首次登录）
	// 优先级最高：使用默认密码是最高安全风险，必须先修改密码再进行其他操作
	if user.ForcePasswordChange && user.ForcePasswordChangeReason != securitypkg.ForcePasswordChangeReasonBreach {
		accessToken, err := middleware.GenerateAccessTokenWithContext(c, user.ID, user.Username, user.Role)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成 Token 失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "登录成功，请先修改默认密码",
			"data": LoginStageResponse{
				Stage:                     "success",
				Token:                     accessToken,
				Username:                  user.Username,
				Role:                      user.Role,
				CloudType:                 service.NormalizeCloudType(user.CloudType),
				Security:                  security,
				ForcePasswordChange:       true,
				ForcePasswordChangeReason: user.ForcePasswordChangeReason,
			},
		})
		return
	}

	// 泄露管理员密码必须先完成 2FA，再进入受限的强制改密会话。
	if user.Role == "admin" && user.PasswordBreached && user.TOTPEnabled {
		token, err := middleware.GenerateTokenWithContext(c, user.ID, user.Username, user.Role, service.TokenTypeLogin, 15*time.Minute)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成登录验证令牌失败"})
			return
		}
		allowedMethods := []string{service.ChallengeMethodTOTP}
		if service.HasRecoveryCodes(&user) {
			allowedMethods = append(allowedMethods, service.ChallengeMethodRecovery)
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 200, "message": "密码已泄露，请先完成 2FA 验证",
			"data": LoginStageResponse{
				Stage: "login_verify", Token: token, Username: user.Username, Role: user.Role,
				CloudType: service.NormalizeCloudType(user.CloudType), Security: security,
				AllowedMethods: allowedMethods, ForcePasswordChange: true,
				ForcePasswordChangeReason: securitypkg.ForcePasswordChangeReasonBreach,
			},
		})
		return
	}

	if service.CanEnterBootstrap(&user) {
		token, err := middleware.GenerateTokenWithContext(c, user.ID, user.Username, user.Role, service.TokenTypeBootstrap, 30*time.Minute)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成引导令牌失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "请先完成安全初始化",
			"data": LoginStageResponse{
				Stage:     "bootstrap_security",
				Token:     token,
				Username:  user.Username,
				Role:      user.Role,
				CloudType: service.NormalizeCloudType(user.CloudType),
				Security:  security,
			},
		})
		return
	}

	if service.NeedsLoginVerification(&user) {
		token, err := middleware.GenerateTokenWithContext(c, user.ID, user.Username, user.Role, service.TokenTypeLogin, 15*time.Minute)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成登录验证令牌失败"})
			return
		}

		allowedMethods := []string{service.ChallengeMethodEmail}
		if user.Role == "admin" {
			allowedMethods = []string{service.ChallengeMethodTOTP}
			if service.HasRecoveryCodes(&user) {
				allowedMethods = append(allowedMethods, service.ChallengeMethodRecovery)
			}
		} else if user.TOTPEnabled {
			allowedMethods = []string{service.ChallengeMethodTOTP, service.ChallengeMethodEmail}
			if service.HasRecoveryCodes(&user) {
				allowedMethods = append(allowedMethods, service.ChallengeMethodRecovery)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "请完成登录验证",
			"data": LoginStageResponse{
				Stage:          "login_verify",
				Token:          token,
				Username:       user.Username,
				Role:           user.Role,
				CloudType:      service.NormalizeCloudType(user.CloudType),
				Security:       security,
				AllowedMethods: allowedMethods,
			},
		})
		return
	}

	// 检查是否需要强制修改密码（默认管理员账号首次登录）
	forcePasswordChange := user.ForcePasswordChange

	accessToken, err := middleware.GenerateAccessTokenWithContext(c, user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成 Token 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "登录成功",
		"data": LoginStageResponse{
			Stage:                     "success",
			Token:                     accessToken,
			Username:                  user.Username,
			Role:                      user.Role,
			CloudType:                 service.NormalizeCloudType(user.CloudType),
			Security:                  security,
			ForcePasswordChange:       forcePasswordChange,
			ForcePasswordChangeReason: user.ForcePasswordChangeReason,
		},
	})
}

// GetUserInfo 获取当前登录用户信息
func GetUserInfo(c *gin.Context) {
	user := getCurrentUser(c)
	security := service.BuildSecurityState(user)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"role":       user.Role,
			"cloud_type": service.NormalizeCloudType(user.CloudType),
			"security":   security,
		},
	})
}

// ChangePassword 修改当前用户密码
func ChangePassword(c *gin.Context) {
	user := getCurrentUser(c)
	// 如果是强制修改默认密码（首次登录），跳过高风险验证
	// 因为此时用户尚未设置邮箱/2FA，无法完成二次验证
	if !user.ForcePasswordChange {
		if !requireHighRiskVerification(c, "change_password") {
			return
		}
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入旧密码和新密码"})
		return
	}
	if err := service.ValidateStrongPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	user = getCurrentUser(c)
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "旧密码错误"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "密码加密失败"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"password_hash":            string(newHash),
		"force_password_change":    false,
		"high_risk_verified_until": nil,
		"login_verified_until":     nil,
		"security_updated_at":      &now,
	}
	for key, value := range securitypkg.PasswordFingerprintUpdateFields(req.NewPassword) {
		updates[key] = value
	}
	if err := model.DB.Model(user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "密码更新失败"})
		return
	}
	if user.Role == "user" {
		if err := service.SyncUserPassword(user.Username, req.NewPassword); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "密码修改成功，请重新登录"})
}

// ChangeUsername 修改当前用户用户名
func ChangeUsername(c *gin.Context) {
	var req ChangeUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入新用户名和密码"})
		return
	}
	newUsername := strings.TrimSpace(req.NewUsername)
	if len(newUsername) < 3 || len(newUsername) > 32 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "用户名长度必须在3-32个字符之间"})
		return
	}

	user := getCurrentUser(c)
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "密码验证失败"})
		return
	}

	var existing model.User
	if err := model.DB.Where("username = ? AND id != ?", newUsername, user.ID).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "该用户名已被使用"})
		return
	}

	now := time.Now()
	if err := model.DB.Model(user).Updates(map[string]interface{}{
		"username":            newUsername,
		"security_updated_at": &now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "用户名更新失败"})
		return
	}

	newToken, err := middleware.GenerateAccessTokenWithContext(c, user.ID, newUsername, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "用户名已更新，但 Token 生成失败，请重新登录"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "用户名修改成功",
		"data": gin.H{
			"token":    newToken,
			"username": newUsername,
		},
	})
}

// SendLoginEmailCode 发送登录邮箱验证码
func SendLoginEmailCode(c *gin.Context) {
	user := getCurrentUser(c)
	if user.Role == "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "管理员登录仅支持 2FA 验证"})
		return
	}
	if user.EmailVerifiedAt == nil || strings.TrimSpace(user.Email) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "当前账号尚未绑定邮箱"})
		return
	}
	challenge, err := service.IssueEmailChallenge(
		user,
		service.ChallengePurposeLoginEmail,
		user.Email,
		"登录验证",
		"您正在进行账户登录验证，请输入以下验证码完成登录。",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "发送验证码失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "验证码已发送",
		"data": gin.H{
			"challenge_id": challenge.ID,
			"masked_email": service.MaskEmail(user.Email),
			"expires_in":   int(service.EmailCodeTTL.Seconds()),
		},
	})
}

// VerifyLoginStage 完成登录期验证
func VerifyLoginStage(c *gin.Context) {
	var req LoginVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	user := getCurrentUser(c)
	method := strings.TrimSpace(req.Method)
	switch method {
	case service.ChallengeMethodTOTP:
		secret, err := service.GetUserTOTPSecret(user)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		if err := service.ValidateTOTPCode(secret, req.Code); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
	case service.ChallengeMethodRecovery:
		valid, newEnc, err := service.ValidateAndConsumeRecoveryCode(user.TOTPRecoveryCodesEnc, req.Code)
		if err != nil || !valid {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "恢复码无效或已使用"})
			return
		}
		// 更新数据库中的恢复码
		if updateErr := model.DB.Model(user).Where("id = ?", user.ID).
			Update("totp_recovery_codes_enc", newEnc).Error; updateErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新恢复码状态失败"})
			return
		}
		user.TOTPRecoveryCodesEnc = newEnc
	case service.ChallengeMethodEmail:
		if user.Role == "admin" {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "管理员登录仅支持 2FA 验证"})
			return
		}
		if _, err := service.VerifyEmailChallenge(user.ID, req.ChallengeID, service.ChallengePurposeLoginEmail, req.Code); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "不支持的验证方式"})
		return
	}

	if user.Role == "user" {
		if err := service.UpdateLoginVerificationWindow(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "刷新登录验证状态失败"})
			return
		}
	}
	accessToken, err := middleware.GenerateAccessTokenWithContext(c, user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成访问令牌失败"})
		return
	}

	state, _, err := service.GetSecurityStateByUserID(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "登录验证成功",
		"data": LoginStageResponse{
			Stage:                     "success",
			Token:                     accessToken,
			Username:                  user.Username,
			Role:                      user.Role,
			CloudType:                 service.NormalizeCloudType(user.CloudType),
			Security:                  *state,
			ForcePasswordChange:       user.ForcePasswordChange,
			ForcePasswordChangeReason: user.ForcePasswordChangeReason,
		},
	})
}

// SendEmailCode 发送邮箱绑定验证码
func SendEmailCode(c *gin.Context) {
	var req EmailCodeSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if !service.IsSMTPConfigured() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "SMTP 尚未配置，暂时无法发送邮件"})
		return
	}
	user := getCurrentUser(c)
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		email = user.Email
	}
	challenge, err := service.IssueEmailChallenge(
		user,
		service.ChallengePurposeBindEmail,
		email,
		"邮箱绑定验证",
		"您正在进行邮箱绑定或换绑，请输入以下验证码完成验证。",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "发送验证码失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "验证码已发送",
		"data": gin.H{
			"challenge_id": challenge.ID,
			"masked_email": service.MaskEmail(email),
			"expires_in":   int(service.EmailCodeTTL.Seconds()),
		},
	})
}

// BindEmail 绑定或更新邮箱
func BindEmail(c *gin.Context) {
	var req EmailBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	user := getCurrentUser(c)
	challenge, err := service.VerifyEmailChallenge(user.ID, req.ChallengeID, service.ChallengePurposeBindEmail, req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if !strings.EqualFold(strings.TrimSpace(challenge.Target), strings.TrimSpace(req.Email)) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "邮箱地址与验证码目标不一致"})
		return
	}

	now := time.Now()
	if err := service.BindUserEmail(user.ID, req.Email, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "绑定邮箱失败: " + err.Error()})
		return
	}
	if user.Role == "user" {
		_ = service.UpdateLoginVerificationWindow(user.ID)
	}

	state, freshUser, err := service.GetSecurityStateByUserID(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	tokenType, _ := c.Get("token_type")
	if tokenType == service.TokenTypeBootstrap && !service.CanEnterBootstrap(freshUser) {
		accessToken, err := middleware.GenerateAccessTokenWithContext(c, freshUser.ID, freshUser.Username, freshUser.Role)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成访问令牌失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "邮箱绑定成功",
			"data": LoginStageResponse{
				Stage:     "success",
				Token:     accessToken,
				Username:  freshUser.Username,
				Role:      freshUser.Role,
				CloudType: service.NormalizeCloudType(freshUser.CloudType),
				Security:  *state,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "邮箱绑定成功",
		"data":    gin.H{"security": state},
	})
}

// SetupTOTP 生成 2FA 配置
func SetupTOTP(c *gin.Context) {
	user := getCurrentUser(c)
	setupInfo, err := service.GenerateTOTPSetup(user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成 2FA 配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": setupInfo})
}

// EnableTOTP 启用 2FA
func EnableTOTP(c *gin.Context) {
	var req TOTPEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := service.ValidateTOTPCode(req.Secret, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	user := getCurrentUser(c)
	recoverySetup, err := service.EnableUserTOTP(user.ID, req.Secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "启用 2FA 失败: " + err.Error()})
		return
	}

	state, freshUser, err := service.GetSecurityStateByUserID(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	tokenType, _ := c.Get("token_type")
	if tokenType == service.TokenTypeBootstrap && freshUser.Role == "admin" {
		accessToken, err := middleware.GenerateAccessTokenWithContext(c, freshUser.ID, freshUser.Username, freshUser.Role)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成访问令牌失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "2FA 绑定成功",
			"data": LoginStageResponse{
				Stage:     "success",
				Token:     accessToken,
				Username:  freshUser.Username,
				Role:      freshUser.Role,
				CloudType: service.NormalizeCloudType(freshUser.CloudType),
				Security:  *state,
			},
			"recovery": recoverySetup,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":     200,
		"message":  "2FA 绑定成功",
		"data":     gin.H{"security": state},
		"recovery": recoverySetup,
	})
}

// RegenRecoveryCodes 重新生成恢复码（旧码立即失效，需验证当前2FA）
func RegenRecoveryCodes(c *gin.Context) {
	var req TOTPDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	user := getCurrentUser(c)
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "密码错误"})
		return
	}
	secret, err := service.GetUserTOTPSecret(user)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := service.ValidateTOTPCode(secret, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	recoverySetup, err := service.RegenerateRecoveryCodes(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "重新生成恢复码失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":     200,
		"message":  "恢复码已重新生成",
		"recovery": recoverySetup,
	})
}

// DisableTOTP 关闭 2FA
func DisableTOTP(c *gin.Context) {
	var req TOTPDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	user := getCurrentUser(c)
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "密码错误"})
		return
	}
	secret, err := service.GetUserTOTPSecret(user)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := service.ValidateTOTPCode(secret, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := service.DisableUserTOTP(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "关闭 2FA 失败"})
		return
	}
	state, _, err := service.GetSecurityStateByUserID(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "2FA 已关闭", "data": gin.H{"security": state}})
}

// VerifyHighRisk 完成高风险验证
func VerifyHighRisk(c *gin.Context) {
	var req HighRiskVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	user := getCurrentUser(c)
	method := strings.TrimSpace(req.Method)
	switch method {
	case service.ChallengeMethodTOTP:
		secret, err := service.GetUserTOTPSecret(user)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		if err := service.ValidateTOTPCode(secret, req.Code); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		if err := service.GrantHighRiskVerificationTrust(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "写入高风险信任状态失败"})
			return
		}
		token, err := middleware.GenerateTokenWithOperation(user.ID, user.Username, user.Role, service.TokenTypeHighRisk, req.Operation, 5*time.Minute)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成高风险令牌失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "高风险验证成功",
			"data": gin.H{
				"verification_token": token,
				"trusted_until":      time.Now().Add(service.HighRiskEmailTrustWindow).Format(time.RFC3339),
			},
		})
	case service.ChallengeMethodRecovery:
		valid, newEnc, err := service.ValidateAndConsumeRecoveryCode(user.TOTPRecoveryCodesEnc, req.Code)
		if err != nil || !valid {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "恢复码无效或已使用"})
			return
		}
		if updateErr := model.DB.Model(user).Where("id = ?", user.ID).
			Update("totp_recovery_codes_enc", newEnc).Error; updateErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新恢复码状态失败"})
			return
		}
		if err := service.GrantHighRiskVerificationTrust(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "写入高风险信任状态失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "高风险验证成功（恢复码已消耗）",
			"data": gin.H{
				"trusted_until":            time.Now().Add(service.HighRiskEmailTrustWindow).Format(time.RFC3339),
				"recovery_codes_remaining": service.GetRecoveryCodesRemaining(newEnc),
			},
		})
	case service.ChallengeMethodEmail:
		if _, err := service.VerifyEmailChallenge(user.ID, req.ChallengeID, service.ChallengePurposeHighRiskEmail, req.Code); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		if err := service.GrantHighRiskVerificationTrust(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "写入高风险信任状态失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "邮箱验证成功",
			"data":    gin.H{"trusted_until": time.Now().Add(service.HighRiskEmailTrustWindow).Format(time.RFC3339)},
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "不支持的验证方式"})
	}
}

// GetInviteInfo 获取邀请详情
func GetInviteInfo(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "邀请令牌不能为空"})
		return
	}
	detail, _, _, err := service.BuildInviteDetail(token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": detail})
}

// CompleteInvite 完成邀请注册
func CompleteInvite(c *gin.Context) {
	var req InviteCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if req.Password != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "两次输入的密码不一致"})
		return
	}
	user, err := service.CompleteInviteRegistration(req.Token, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	accessToken, err := middleware.GenerateAccessTokenWithContext(c, user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "注册成功，但自动登录失败"})
		return
	}
	state := service.BuildSecurityState(user)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "注册完成",
		"data": LoginStageResponse{
			Stage:     "success",
			Token:     accessToken,
			Username:  user.Username,
			Role:      user.Role,
			CloudType: service.NormalizeCloudType(user.CloudType),
			Security:  state,
		},
	})
}

// ForgotPassword 发送找回密码邮件
func ForgotPassword(c *gin.Context) {
	var req PasswordForgotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入邮箱"})
		return
	}
	if !service.IsSMTPConfigured() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "SMTP 尚未配置，无法找回密码"})
		return
	}
	user, token, err := service.CreatePasswordResetToken(req.Email)
	if err == nil {
		resetURL := buildBaseURL(c) + "/reset-password?token=" + token
		_ = service.SendPasswordResetEmail(user, resetURL)
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "如果邮箱对应账号存在，系统已发送找回密码邮件"})
}

// ForgotPasswordSendCode 发送忘记密码验证码
func ForgotPasswordSendCode(c *gin.Context) {
	var req PasswordForgotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入邮箱"})
		return
	}
	if !service.IsSMTPConfigured() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "SMTP 尚未配置，无法发送验证码"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	challenge, err := service.IssuePublicEmailChallenge(
		service.ChallengePurposePasswordForgot,
		email,
		"找回密码验证",
		"您正在进行忘记密码验证，请输入以下验证码继续找回账号。",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "发送验证码失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "验证码已发送",
		"data": gin.H{
			"challenge_id": challenge.ID,
			"masked_email": service.MaskEmail(email),
			"expires_in":   int(service.EmailCodeTTL.Seconds()),
		},
	})
}

// ForgotPasswordVerifyCode 校验忘记密码验证码并返回账号列表
func ForgotPasswordVerifyCode(c *gin.Context) {
	var req PasswordForgotVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if _, err := service.VerifyPublicEmailChallenge(req.ChallengeID, service.ChallengePurposePasswordForgot, email, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	accounts, err := service.ListPasswordResetAccountsByEmail(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	selectionToken, err := middleware.GenerateTokenWithTTL(0, email, "", service.TokenTypePasswordResetSelect, 10*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成账号选择令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "邮箱验证成功",
		"data": gin.H{
			"selection_token": selectionToken,
			"accounts":        accounts,
			"email":           email,
			"masked_email":    service.MaskEmail(email),
		},
	})
}

// ForgotPasswordSelectAccount 选择要重置的账号并返回重置令牌
func ForgotPasswordSelectAccount(c *gin.Context) {
	var req PasswordForgotSelectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	claims, err := middleware.ParseToken(strings.TrimSpace(req.SelectionToken))
	if err != nil || claims.TokenType != service.TokenTypePasswordResetSelect {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "账号选择状态已失效，请重新验证邮箱"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(claims.Username))
	user, err := service.FindPasswordResetUser(email, req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	resetToken, err := middleware.GenerateTokenWithTTL(user.ID, user.Username, user.Role, service.TokenTypePasswordReset, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成重置令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "账号确认成功",
		"data": gin.H{
			"reset_token": resetToken,
			"username":    user.Username,
		},
	})
}

// ResetPasswordByEmail 使用邮件链接重置密码
func ResetPasswordByEmail(c *gin.Context) {
	var req PasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if req.Password != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "两次输入的密码不一致"})
		return
	}
	token := strings.TrimSpace(req.Token)
	if claims, err := middleware.ParseToken(token); err == nil && claims.TokenType == service.TokenTypePasswordReset {
		if err := service.ResetPasswordByUserID(claims.UserID, req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
	} else {
		if err := service.ResetPasswordByToken(token, req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "密码已重置，请重新登录"})
}

// SkipBootstrapRequest 跳过安全初始化请求
type SkipBootstrapRequest struct {
	Confirm bool `json:"confirm" binding:"required"`
}

// SkipBootstrap 管理员跳过安全初始化（SMTP+邮箱+2FA）
func SkipBootstrap(c *gin.Context) {
	var req SkipBootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请确认跳过安全初始化"})
		return
	}

	user := getCurrentUser(c)
	if user.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "仅管理员可跳过安全初始化"})
		return
	}

	if err := service.SkipAdminBootstrap(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "操作失败: " + err.Error()})
		return
	}

	// 重新加载用户以获取最新状态
	var refreshed model.User
	if err := model.DB.First(&refreshed, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取用户状态失败"})
		return
	}

	accessToken, err := middleware.GenerateAccessTokenWithContext(c, refreshed.ID, refreshed.Username, refreshed.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成访问令牌失败"})
		return
	}

	state := service.BuildSecurityState(&refreshed)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "已跳过安全初始化，SMTP、邮箱和2FA均未配置，请注意账户安全",
		"data": LoginStageResponse{
			Stage:     "success",
			Token:     accessToken,
			Username:  refreshed.Username,
			Role:      refreshed.Role,
			CloudType: service.NormalizeCloudType(refreshed.CloudType),
			Security:  state,
		},
	})
}
