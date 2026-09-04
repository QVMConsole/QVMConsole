package security

import "time"

const (
	UserStatusPendingInvite = "pending_invite"
	UserStatusActive        = "active"
	UserStatusDisabled      = "disabled"
)

const (
	TokenTypeAccess              = "access"
	TokenTypeBootstrap           = "bootstrap"
	TokenTypeLogin               = "login"
	TokenTypeHighRisk            = "high_risk"
	TokenTypePasswordResetSelect = "password_reset_select"
	TokenTypePasswordReset       = "password_reset"
)

const (
	ChallengeMethodEmail     = "email"
	ChallengeMethodTOTP      = "totp"
	ChallengeMethodRecovery  = "recovery"
	ChallengeMethodTOTPEmail = "totp_email"
)

const (
	ChallengePurposeBindEmail      = "bind_email"
	ChallengePurposeLoginEmail     = "login_email_verify"
	ChallengePurposeHighRiskEmail  = "high_risk_email_verify"
	ChallengePurposeSMTPTest       = "smtp_test"
	ChallengePurposePasswordForgot = "password_forgot_verify"
)

const (
	ActionTokenPurposeInviteRegister = "invite_register"
	ActionTokenPurposePasswordReset  = "password_reset"
)

const (
	LoginVerificationWindow  = 24 * time.Hour
	HighRiskEmailTrustWindow = time.Hour
	InviteLinkTTL            = 72 * time.Hour
	PasswordResetLinkTTL     = time.Hour
	EmailCodeTTL             = 10 * time.Minute
)
