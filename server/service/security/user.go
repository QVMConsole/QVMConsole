package security

import (
	"fmt"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/model"
)

// SecurityState 前端展示用安全状态
type SecurityState struct {
	Email                    string  `json:"email"`
	MaskedEmail              string  `json:"masked_email"`
	EmailVerified            bool    `json:"email_verified"`
	TOTPEnabled              bool    `json:"totp_enabled"`
	MustBindEmail            bool    `json:"must_bind_email"`
	MustBind2FA              bool    `json:"must_bind_2fa"`
	RequiresLoginVerify      bool    `json:"requires_login_verify"`
	SMTPConfigured           bool    `json:"smtp_configured"`
	DevelopmentMode          bool    `json:"development_mode"`
	MaintenanceMode          bool    `json:"maintenance_mode"`
	BootstrapSkipped         bool    `json:"bootstrap_skipped"` // 管理员是否跳过了安全初始化
	Status                   string  `json:"status"`
	LoginVerifiedUntil       *string `json:"login_verified_until"`
	HighRiskMethod           string  `json:"high_risk_method"`
	HasRecoveryCodes         bool    `json:"has_recovery_codes"` // 是否有可用恢复码
	PasswordBreached         bool    `json:"password_breached"`
	PasswordBreachCount      int64   `json:"password_breach_count"`
	PasswordBreachDetectedAt *string `json:"password_breach_detected_at"`
}

// IsSecurityVerificationDisabled 是否禁用安全验证
func IsSecurityVerificationDisabled() bool {
	return config.GlobalConfig != nil && config.GlobalConfig.DevelopmentMode
}

// BuildSecurityState 构建安全状态
func BuildSecurityState(user *model.User) SecurityState {
	state := SecurityState{
		Email:         strings.TrimSpace(user.Email),
		MaskedEmail:   MaskEmail(user.Email),
		EmailVerified: user.EmailVerifiedAt != nil,
		TOTPEnabled:   user.TOTPEnabled,
		// 普通用户邮箱是可选资料：未填写时不进入强制绑定流程；管理员保留原有安全初始化要求。
		MustBindEmail:       user.EmailVerifiedAt == nil && (user.Role == "admin" || strings.TrimSpace(user.Email) != ""),
		MustBind2FA:         user.Role == "admin" && !user.TOTPEnabled,
		SMTPConfigured:      IsSMTPConfigured(),
		DevelopmentMode:     IsSecurityVerificationDisabled(),
		MaintenanceMode:     D.IsMaintenanceModeEnabled(),
		BootstrapSkipped:    user.BootstrapSkipped,
		Status:              user.Status,
		HasRecoveryCodes:    HasRecoveryCodes(user),
		PasswordBreached:    user.PasswordBreached,
		PasswordBreachCount: user.PasswordBreachCount,
	}
	if user.PasswordBreachDetectedAt != nil {
		formatted := user.PasswordBreachDetectedAt.Format(time.RFC3339)
		state.PasswordBreachDetectedAt = &formatted
	}
	if state.DevelopmentMode {
		state.MustBindEmail = false
		state.MustBind2FA = false
		state.RequiresLoginVerify = false
		state.HighRiskMethod = ""
		if user.LoginVerifiedUntil != nil {
			formatted := user.LoginVerifiedUntil.Format(time.RFC3339)
			state.LoginVerifiedUntil = &formatted
		}
		return state
	}
	if user.LoginVerifiedUntil != nil {
		formatted := user.LoginVerifiedUntil.Format(time.RFC3339)
		state.LoginVerifiedUntil = &formatted
		state.RequiresLoginVerify = time.Now().After(*user.LoginVerifiedUntil)
	} else {
		state.RequiresLoginVerify = true
	}
	if user.Role != "admin" && strings.TrimSpace(user.Email) == "" && !user.TOTPEnabled {
		state.RequiresLoginVerify = false
		state.HighRiskMethod = ""
		return state
	}
	if user.TOTPEnabled {
		state.HighRiskMethod = ChallengeMethodTOTP
	} else {
		state.HighRiskMethod = ChallengeMethodEmail
	}
	return state
}

// GetSecurityStateByUserID 获取用户安全状态
func GetSecurityStateByUserID(userID uint) (*SecurityState, *model.User, error) {
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		return nil, nil, fmt.Errorf("用户不存在")
	}
	state := BuildSecurityState(&user)
	return &state, &user, nil
}

// MaskEmail 掩码邮箱
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + parts[1]
}

// TouchSecurityUpdatedAt 更新安全时间戳
func TouchSecurityUpdatedAt(userID uint) error {
	now := time.Now()
	return model.DB.Model(&model.User{}).Where("id = ?", userID).
		Update("security_updated_at", &now).Error
}

// ResetHighRiskTrust 清除高风险邮箱信任窗口
func ResetHighRiskTrust(userID uint) error {
	return model.DB.Model(&model.User{}).Where("id = ?", userID).
		Update("high_risk_verified_until", nil).Error
}
