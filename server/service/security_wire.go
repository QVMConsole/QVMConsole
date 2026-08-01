package service

import (
	"time"

	"kvm_console/model"

	securitypkg "kvm_console/service/security"
)

// ── Type aliases ──

type SMTPConfigView = securitypkg.SMTPConfigView
type SMTPTestConfig = securitypkg.SMTPTestConfig
type TOTPSetupInfo = securitypkg.TOTPSetupInfo
type TOTPRecoverySetup = securitypkg.TOTPRecoverySetup
type InviteDetail = securitypkg.InviteDetail
type PasswordResetAccountCandidate = securitypkg.PasswordResetAccountCandidate
type SecurityState = securitypkg.SecurityState

// ── Constants re-export ──

const (
	UserStatusPendingInvite = securitypkg.UserStatusPendingInvite
	UserStatusActive        = securitypkg.UserStatusActive
	UserStatusDisabled      = securitypkg.UserStatusDisabled
)

const (
	TokenTypeAccess              = securitypkg.TokenTypeAccess
	TokenTypeBootstrap           = securitypkg.TokenTypeBootstrap
	TokenTypeLogin               = securitypkg.TokenTypeLogin
	TokenTypeHighRisk            = securitypkg.TokenTypeHighRisk
	TokenTypePasswordResetSelect = securitypkg.TokenTypePasswordResetSelect
	TokenTypePasswordReset       = securitypkg.TokenTypePasswordReset
)

const (
	ChallengeMethodEmail    = securitypkg.ChallengeMethodEmail
	ChallengeMethodTOTP     = securitypkg.ChallengeMethodTOTP
	ChallengeMethodRecovery = securitypkg.ChallengeMethodRecovery
)

const (
	ChallengePurposeBindEmail      = securitypkg.ChallengePurposeBindEmail
	ChallengePurposeLoginEmail     = securitypkg.ChallengePurposeLoginEmail
	ChallengePurposeHighRiskEmail  = securitypkg.ChallengePurposeHighRiskEmail
	ChallengePurposeSMTPTest       = securitypkg.ChallengePurposeSMTPTest
	ChallengePurposePasswordForgot = securitypkg.ChallengePurposePasswordForgot
)

const (
	ActionTokenPurposeInviteRegister = securitypkg.ActionTokenPurposeInviteRegister
	ActionTokenPurposePasswordReset  = securitypkg.ActionTokenPurposePasswordReset
)

const (
	LoginVerificationWindow  = securitypkg.LoginVerificationWindow
	HighRiskEmailTrustWindow = securitypkg.HighRiskEmailTrustWindow
	InviteLinkTTL            = securitypkg.InviteLinkTTL
	PasswordResetLinkTTL     = securitypkg.PasswordResetLinkTTL
	EmailCodeTTL             = securitypkg.EmailCodeTTL
)

// init wires security package function variables to service root implementations.
// This breaks the circular dependency: security package cannot import service,
// so it exposes function variables that we set here.
func init() {
	securitypkg.InitDeps(&securitypkg.Deps{
		// ---- Cloud type ----
		NormalizeCloudType:     NormalizeCloudType,
		CloudTypeElastic:       CloudTypeElastic,
		IsLightweightCloudType: IsLightweightCloudType,

		// ---- User provisioning ----
		ProvisionSystemUserResources: ProvisionSystemUserResources,

		// ---- VPC defaults ----
		EnsureDefaultSecurityGroup: EnsureDefaultSecurityGroup,
		EnsureDefaultVPCSwitch:     EnsureDefaultVPCSwitch,

		// ---- Validation ----
		ValidateStrongPassword: ValidateStrongPassword,

		// ---- Lightweight cloud ----
		ListLightweightVMRegistrations: func(username string, includeActive bool) ([]securitypkg.LightweightVMRegistrationView, error) {
			views, err := ListLightweightVMRegistrations(username, includeActive)
			if err != nil {
				return nil, err
			}
			result := make([]securitypkg.LightweightVMRegistrationView, len(views))
			for i, v := range views {
				result[i] = securitypkg.LightweightVMRegistrationView(v)
			}
			return result, nil
		},
		FormatLightweightVMRegistrationList: func(regs []securitypkg.LightweightVMRegistrationView) string {
			views := make([]LightweightVMRegistrationView, len(regs))
			for i, r := range regs {
				views[i] = LightweightVMRegistrationView(r)
			}
			return FormatLightweightVMRegistrationList(views)
		},

		// ---- Network constants ----
		BridgeModeNAT: BridgeModeNAT,

		// ---- Maintenance mode ----
		IsMaintenanceModeEnabled: IsMaintenanceModeEnabled,
	})
}

// ---- Crypto functions ----

func EncryptSecurityText(plainText string) (string, error) {
	return securitypkg.EncryptSecurityText(plainText)
}

func DecryptSecurityText(cipherText string) (string, error) {
	return securitypkg.DecryptSecurityText(cipherText)
}

// ---- Token functions ----

func CreateAuthActionToken(userID uint, purpose string, ttl time.Duration) (string, *model.AuthActionToken, error) {
	return securitypkg.CreateAuthActionToken(userID, purpose, ttl)
}

func FindValidAuthActionToken(rawToken, purpose string) (*model.AuthActionToken, *model.User, error) {
	return securitypkg.FindValidAuthActionToken(rawToken, purpose)
}

func ConsumeAuthActionToken(recordID uint) error {
	return securitypkg.ConsumeAuthActionToken(recordID)
}

func InvalidateAuthActionTokens(userID uint, purpose string) error {
	return securitypkg.InvalidateAuthActionTokens(userID, purpose)
}

// ---- SMTP functions ----

func GetSMTPConfigView() SMTPConfigView {
	return securitypkg.GetSMTPConfigView()
}

func IsSMTPConfigured() bool {
	return securitypkg.IsSMTPConfigured()
}

func SendEmailWithConfig(cfg SMTPTestConfig, to, subject, body string) error {
	return securitypkg.SendEmailWithConfig(cfg, to, subject, body)
}

func SetSMTPPassword(plainPassword string) error {
	return securitypkg.SetSMTPPassword(plainPassword)
}

func SendEmail(to, subject, body string) error {
	return securitypkg.SendEmail(to, subject, body)
}

// ---- Challenge functions ----

func IssueEmailChallenge(user *model.User, purpose, targetEmail, subject, contentPrefix string) (*model.SecurityChallenge, error) {
	return securitypkg.IssueEmailChallenge(user, purpose, targetEmail, subject, contentPrefix)
}

func IssuePublicEmailChallenge(purpose, targetEmail, subject, contentPrefix string) (*model.SecurityChallenge, error) {
	return securitypkg.IssuePublicEmailChallenge(purpose, targetEmail, subject, contentPrefix)
}

func VerifyEmailChallenge(userID uint, challengeID uint, purpose, code string) (*model.SecurityChallenge, error) {
	return securitypkg.VerifyEmailChallenge(userID, challengeID, purpose, code)
}

func VerifyPublicEmailChallenge(challengeID uint, purpose, targetEmail, code string) (*model.SecurityChallenge, error) {
	return securitypkg.VerifyPublicEmailChallenge(challengeID, purpose, targetEmail, code)
}

// ---- TOTP functions ----

func GenerateTOTPSetup(username string) (*TOTPSetupInfo, error) {
	return securitypkg.GenerateTOTPSetup(username)
}

func ValidateTOTPCode(secret, code string) error {
	return securitypkg.ValidateTOTPCode(secret, code)
}

func GenerateRecoveryCodes() ([]string, []string, error) {
	return securitypkg.GenerateRecoveryCodes()
}

func ValidateAndConsumeRecoveryCode(encryptedEnc, inputCode string) (bool, string, error) {
	return securitypkg.ValidateAndConsumeRecoveryCode(encryptedEnc, inputCode)
}

func GetRecoveryCodesRemaining(encryptedEnc string) int {
	return securitypkg.GetRecoveryCodesRemaining(encryptedEnc)
}

// ---- Account functions ----

func CreatePendingInvitedUser(username, email, role, cloudType string, dedicatedVPCSwitchID uint, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int, enablePortForward bool, maxPortForwards, maxSnapshots int, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64, maxPublicIPs int) (*model.User, string, error) {
	return securitypkg.CreatePendingInvitedUser(username, email, role, cloudType, dedicatedVPCSwitchID, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours, enablePortForward, maxPortForwards, maxSnapshots, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp, maxPublicIPs)
}

func CreatePendingInvitedUserWithExistingVMs(username, email, role, cloudType string, dedicatedVPCSwitchID uint, useExistingVMs bool, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int, enablePortForward bool, maxPortForwards, maxSnapshots int, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64, maxPublicIPs int) (*model.User, string, error) {
	return securitypkg.CreatePendingInvitedUserWithExistingVMs(username, email, role, cloudType, dedicatedVPCSwitchID, useExistingVMs, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours, enablePortForward, maxPortForwards, maxSnapshots, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp, maxPublicIPs)
}

func ResendInviteToken(username string) (*model.User, string, error) {
	return securitypkg.ResendInviteToken(username)
}

func BuildInviteDetail(rawToken string) (*InviteDetail, *model.AuthActionToken, *model.User, error) {
	return securitypkg.BuildInviteDetail(rawToken)
}

func CompleteInviteRegistration(rawToken, password string) (*model.User, error) {
	return securitypkg.CompleteInviteRegistration(rawToken, password)
}

func SendInviteEmail(user *model.User, inviteURL string) error {
	return securitypkg.SendInviteEmail(user, inviteURL)
}

func SendLightweightVMRegistrationEmail(user *model.User, panelURL string, regs []LightweightVMRegistrationView) error {
	// Convert service.LightweightVMRegistrationView to security.LightweightVMRegistrationView
	secRegs := make([]securitypkg.LightweightVMRegistrationView, len(regs))
	for i := range regs {
		secRegs[i] = securitypkg.LightweightVMRegistrationView(regs[i])
	}
	return securitypkg.SendLightweightVMRegistrationEmail(user, panelURL, secRegs)
}

func SendPasswordResetEmail(user *model.User, resetURL string) error {
	return securitypkg.SendPasswordResetEmail(user, resetURL)
}

func CreatePasswordResetToken(email string) (*model.User, string, error) {
	return securitypkg.CreatePasswordResetToken(email)
}

func ListPasswordResetAccountsByEmail(email string) ([]PasswordResetAccountCandidate, error) {
	return securitypkg.ListPasswordResetAccountsByEmail(email)
}

func FindPasswordResetUser(email, username string) (*model.User, error) {
	return securitypkg.FindPasswordResetUser(email, username)
}

func ResetPasswordByToken(rawToken, newPassword string) error {
	return securitypkg.ResetPasswordByToken(rawToken, newPassword)
}

func ResetPasswordByUserID(userID uint, newPassword string) error {
	return securitypkg.ResetPasswordByUserID(userID, newPassword)
}

func BindUserEmail(userID uint, email string, verifiedAt time.Time) error {
	return securitypkg.BindUserEmail(userID, email, verifiedAt)
}

func EnableUserTOTP(userID uint, secret string) (*TOTPRecoverySetup, error) {
	return securitypkg.EnableUserTOTP(userID, secret)
}

func DisableUserTOTP(userID uint) error {
	return securitypkg.DisableUserTOTP(userID)
}

func GetUserTOTPSecret(user *model.User) (string, error) {
	return securitypkg.GetUserTOTPSecret(user)
}

func HasRecoveryCodes(user *model.User) bool {
	return securitypkg.HasRecoveryCodes(user)
}

func RegenerateRecoveryCodes(userID uint) (*TOTPRecoverySetup, error) {
	return securitypkg.RegenerateRecoveryCodes(userID)
}

func UpdateLoginVerificationWindow(userID uint) error {
	return securitypkg.UpdateLoginVerificationWindow(userID)
}

func GrantHighRiskVerificationTrust(userID uint) error {
	return securitypkg.GrantHighRiskVerificationTrust(userID)
}

func CanSkipHighRiskVerification(user *model.User) bool {
	return securitypkg.CanSkipHighRiskVerification(user)
}

func CanEnterBootstrap(user *model.User) bool {
	return securitypkg.CanEnterBootstrap(user)
}

func SkipAdminBootstrap(userID uint) error {
	return securitypkg.SkipAdminBootstrap(userID)
}

func TryClearBootstrapSkipped(userID uint) error {
	return securitypkg.TryClearBootstrapSkipped(userID)
}

func CreateActiveUserDirectly(username, email, password, role, cloudType string, dedicatedVPCSwitchID uint, useExistingVMs bool,
	maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int,
	enablePortForward bool, maxPortForwards, maxSnapshots int,
	maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64, maxPublicIPs int) (*model.User, error) {
	return securitypkg.CreateActiveUserDirectly(username, email, password, role, cloudType, dedicatedVPCSwitchID, useExistingVMs,
		maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours,
		enablePortForward, maxPortForwards, maxSnapshots,
		maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp, maxPublicIPs)
}

func UpdateManagedUserAccount(username string, email *string, newPassword string) (*model.User, bool, error) {
	return securitypkg.UpdateManagedUserAccount(username, email, newPassword)
}

func NeedsLoginVerification(user *model.User) bool {
	return securitypkg.NeedsLoginVerification(user)
}

func SyncUserPassword(username, password string) error {
	return securitypkg.SyncUserPassword(username, password)
}

// ---- User security functions ----

func IsSecurityVerificationDisabled() bool {
	return securitypkg.IsSecurityVerificationDisabled()
}

func BuildSecurityState(user *model.User) SecurityState {
	return securitypkg.BuildSecurityState(user)
}

func GetSecurityStateByUserID(userID uint) (*SecurityState, *model.User, error) {
	return securitypkg.GetSecurityStateByUserID(userID)
}

func MaskEmail(email string) string {
	return securitypkg.MaskEmail(email)
}

func TouchSecurityUpdatedAt(userID uint) error {
	return securitypkg.TouchSecurityUpdatedAt(userID)
}

func ResetHighRiskTrust(userID uint) error {
	return securitypkg.ResetHighRiskTrust(userID)
}
