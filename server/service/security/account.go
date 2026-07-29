package security

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kvm_console/model"
)

// InviteDetail 邀请详情页展示信息
type InviteDetail struct {
	Username                   string                          `json:"username"`
	Email                      string                          `json:"email"`
	Role                       string                          `json:"role"`
	CloudType                  string                          `json:"cloud_type"`
	DedicatedVPCSwitchID       uint                            `json:"dedicated_vpc_switch_id"`
	Status                     string                          `json:"status"`
	ExpiresAt                  time.Time                       `json:"expires_at"`
	MaxCPU                     int                             `json:"max_cpu"`
	MaxMemory                  int                             `json:"max_memory"`
	MaxDisk                    int                             `json:"max_disk"`
	MaxVM                      int                             `json:"max_vm"`
	MaxStorage                 int                             `json:"max_storage"`
	MaxRuntimeHours            int                             `json:"max_runtime_hours"`
	EnablePortForward          bool                            `json:"enable_port_forward"`
	MaxPortForwards            int                             `json:"max_port_forwards"`
	MaxSnapshots               int                             `json:"max_snapshots"`
	MaxBandwidthUp             float64                         `json:"max_bandwidth_up"`
	MaxBandwidthDown           float64                         `json:"max_bandwidth_down"`
	MaxTrafficDown             float64                         `json:"max_traffic_down"`
	MaxTrafficUp               float64                         `json:"max_traffic_up"`
	MaxPublicIPs               int                             `json:"max_public_ips"`
	LightweightVMRegistrations []LightweightVMRegistrationView `json:"lightweight_vm_registrations,omitempty"`
}

// PasswordResetAccountCandidate 忘记密码场景下可选择的账号。
type PasswordResetAccountCandidate struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// CreatePendingInvitedUser 创建待激活邀请用户
func CreatePendingInvitedUser(username, email, role, cloudType string, dedicatedVPCSwitchID uint, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int, enablePortForward bool, maxPortForwards, maxSnapshots int, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64, maxPublicIPs int) (*model.User, string, error) {
	return CreatePendingInvitedUserWithExistingVMs(username, email, role, cloudType, dedicatedVPCSwitchID, false, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours, enablePortForward, maxPortForwards, maxSnapshots, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp, maxPublicIPs)
}

func CreatePendingInvitedUserWithExistingVMs(username, email, role, cloudType string, dedicatedVPCSwitchID uint, useExistingVMs bool, maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int, enablePortForward bool, maxPortForwards, maxSnapshots int, maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64, maxPublicIPs int) (*model.User, string, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	if username == "" || email == "" {
		return nil, "", fmt.Errorf("用户名和邮箱不能为空")
	}
	if role == "" {
		role = "user"
	}
	cloudType = D.NormalizeCloudType(cloudType)
	if role == "admin" {
		cloudType = D.CloudTypeElastic
		dedicatedVPCSwitchID = 0
	}
	// 只有在不选择已有VM时才要求专用VPC
	if role == "user" && D.IsLightweightCloudType(cloudType) && dedicatedVPCSwitchID == 0 && !useExistingVMs {
		return nil, "", fmt.Errorf("轻量云用户必须选择专用 VPC 网络")
	}
	if role == "user" && D.IsLightweightCloudType(cloudType) && dedicatedVPCSwitchID > 0 {
		var count int64
		if err := model.DB.Model(&model.VPCSwitch{}).
			Where("id = ? AND (bridge_mode = '' OR bridge_mode = ? OR bridge_mode IS NULL)", dedicatedVPCSwitchID, D.BridgeModeNAT).
			Count(&count).Error; err != nil {
			return nil, "", fmt.Errorf("检查专用 VPC 网络失败: %w", err)
		}
		if count == 0 {
			return nil, "", fmt.Errorf("请选择有效的 NAT 类型专用 VPC 网络")
		}
	}

	var count int64
	if err := model.DB.Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return nil, "", fmt.Errorf("检查用户名失败: %w", err)
	}
	if count > 0 {
		return nil, "", fmt.Errorf("用户名 %s 已存在", username)
	}
	model.DB.Unscoped().Where("username = ? AND deleted_at IS NOT NULL", username).Delete(&model.User{})

	user := &model.User{
		Username:             username,
		Email:                email,
		Role:                 role,
		CloudType:            cloudType,
		DedicatedVPCSwitchID: dedicatedVPCSwitchID,
		Status:               UserStatusPendingInvite,
		MaxCPU:               maxCPU,
		MaxMemory:            maxMemory,
		MaxDisk:              maxDisk,
		MaxVM:                maxVM,
		MaxStorage:           maxStorage,
		MaxRuntimeHours:      maxRuntimeHours,
		EnablePortForward:    enablePortForward,
		MaxPortForwards:      maxPortForwards,
		MaxSnapshots:         maxSnapshots,
		MaxBandwidthUp:       maxBandwidthUp,
		MaxBandwidthDown:     maxBandwidthDown,
		MaxTrafficDown:       maxTrafficDown,
		MaxTrafficUp:         maxTrafficUp,
		MaxPublicIPs:         maxPublicIPs,
	}
	if err := model.DB.Create(user).Error; err != nil {
		return nil, "", fmt.Errorf("创建待激活用户失败: %w", err)
	}

	inviteToken, _, err := CreateAuthActionToken(user.ID, ActionTokenPurposeInviteRegister, InviteLinkTTL)
	if err != nil {
		return nil, "", fmt.Errorf("创建邀请令牌失败: %w", err)
	}
	return user, inviteToken, nil
}

// ResendInviteToken 重新生成邀请令牌
func ResendInviteToken(username string) (*model.User, string, error) {
	var user model.User
	if err := model.DB.Where("username = ?", strings.TrimSpace(username)).First(&user).Error; err != nil {
		return nil, "", fmt.Errorf("用户不存在")
	}
	if user.Status != UserStatusPendingInvite {
		return nil, "", fmt.Errorf("该用户已激活，不能重发邀请")
	}
	token, _, err := CreateAuthActionToken(user.ID, ActionTokenPurposeInviteRegister, InviteLinkTTL)
	if err != nil {
		return nil, "", err
	}
	return &user, token, nil
}

// BuildInviteDetail 获取邀请详情
func BuildInviteDetail(rawToken string) (*InviteDetail, *model.AuthActionToken, *model.User, error) {
	record, user, err := FindValidAuthActionToken(rawToken, ActionTokenPurposeInviteRegister)
	if err != nil {
		return nil, nil, nil, err
	}
	detail := &InviteDetail{
		Username:             user.Username,
		Email:                user.Email,
		Role:                 user.Role,
		CloudType:            D.NormalizeCloudType(user.CloudType),
		DedicatedVPCSwitchID: user.DedicatedVPCSwitchID,
		Status:               user.Status,
		ExpiresAt:            record.ExpiresAt,
		MaxCPU:               user.MaxCPU,
		MaxMemory:            user.MaxMemory,
		MaxDisk:              user.MaxDisk,
		MaxVM:                user.MaxVM,
		MaxStorage:           user.MaxStorage,
		MaxRuntimeHours:      user.MaxRuntimeHours,
		EnablePortForward:    user.EnablePortForward,
		MaxPortForwards:      user.MaxPortForwards,
		MaxSnapshots:         user.MaxSnapshots,
		MaxBandwidthUp:       user.MaxBandwidthUp,
		MaxBandwidthDown:     user.MaxBandwidthDown,
		MaxTrafficDown:       user.MaxTrafficDown,
		MaxTrafficUp:         user.MaxTrafficUp,
		MaxPublicIPs:         user.MaxPublicIPs,
	}
	if user.Role == "user" && D.IsLightweightCloudType(user.CloudType) {
		if regs, err := D.ListLightweightVMRegistrations(user.Username, false); err == nil {
			detail.LightweightVMRegistrations = regs
		}
	}
	return detail, record, user, nil
}

// CompleteInviteRegistration 完成邀请注册
func CompleteInviteRegistration(rawToken, password string) (*model.User, error) {
	record, user, err := FindValidAuthActionToken(rawToken, ActionTokenPurposeInviteRegister)
	if err != nil {
		return nil, err
	}
	if user.Status != UserStatusPendingInvite {
		return nil, fmt.Errorf("邀请已失效")
	}
	if err := D.ValidateStrongPassword(password); err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}

	now := time.Now()
	loginVerifiedUntil := now.Add(LoginVerificationWindow)
	updates := map[string]interface{}{
		"password_hash":        string(hashedPassword),
		"status":               UserStatusActive,
		"email_verified_at":    &now,
		"login_verified_until": &loginVerifiedUntil,
		"security_updated_at":  &now,
	}
	for key, value := range PasswordFingerprintUpdateFields(password) {
		updates[key] = value
	}
	if err := model.DB.Model(user).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("激活用户失败: %w", err)
	}
	if err := D.ProvisionSystemUserResources(user, password); err != nil {
		return nil, err
	}
	if user.Role == "user" && !D.IsLightweightCloudType(user.CloudType) {
		if _, err := D.EnsureDefaultSecurityGroup(user.Username); err != nil {
			return nil, err
		}
		if _, err := D.EnsureDefaultVPCSwitch(user.Username); err != nil {
			return nil, err
		}
	}
	if err := ConsumeAuthActionToken(record.ID); err != nil {
		return nil, err
	}
	user.PasswordHash = string(hashedPassword)
	user.Status = UserStatusActive
	user.EmailVerifiedAt = &now
	user.LoginVerifiedUntil = &loginVerifiedUntil
	user.SecurityUpdatedAt = &now
	return user, nil
}

// SendInviteEmail 发送邀请邮件
func SendInviteEmail(user *model.User, inviteURL string) error {
	body := fmt.Sprintf("您好，%s：\n\n管理员已为您创建账户，请通过以下链接完成注册：\n%s\n\n链接有效期：72 小时。", user.Username, inviteURL)
	if user.Role == "user" && D.IsLightweightCloudType(user.CloudType) {
		if regs, err := D.ListLightweightVMRegistrations(user.Username, false); err == nil && len(regs) > 0 {
			body += "\n\n管理员同时为您登记了以下待开通轻量云服务器。注册并登录面板后，请逐台确认配置并补全登录凭据，系统才会开始开通服务器。\n\n"
			body += D.FormatLightweightVMRegistrationList(regs)
		}
	}
	body += "\n如果不是您本人操作，请忽略此邮件。"
	return SendEmail(user.Email, "账户注册邀请", body)
}

func SendLightweightVMRegistrationEmail(user *model.User, panelURL string, regs []LightweightVMRegistrationView) error {
	if user == nil || len(regs) == 0 {
		return nil
	}
	body := fmt.Sprintf("您好，%s：\n\n管理员为您登记了新的轻量云服务器，请登录面板确认配置并补全登录凭据。确认后系统才会开始开通服务器。\n\n面板地址：%s\n\n配置清单：\n%s\n如果不是您本人操作，请联系管理员。", user.Username, panelURL, D.FormatLightweightVMRegistrationList(regs))
	return SendEmail(user.Email, "轻量云服务器开通确认", body)
}

// SendPasswordResetEmail 发送找回密码邮件
func SendPasswordResetEmail(user *model.User, resetURL string) error {
	body := fmt.Sprintf("您好，%s：\n\n请通过以下链接重置密码：\n%s\n\n链接有效期：1 小时。\n如果不是您本人操作，请忽略此邮件。", user.Username, resetURL)
	return SendEmail(user.Email, "找回账户密码", body)
}

// CreatePasswordResetToken 创建找回密码令牌
func CreatePasswordResetToken(email string) (*model.User, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var users []model.User
	if err := model.DB.
		Where("email = ? AND status = ? AND deleted_at IS NULL", email, UserStatusActive).
		Order("id ASC").
		Find(&users).Error; err != nil {
		return nil, "", fmt.Errorf("未找到对应的已激活账户")
	}
	if len(users) == 0 {
		return nil, "", fmt.Errorf("未找到对应的已激活账户")
	}
	if len(users) > 1 {
		return nil, "", fmt.Errorf("该邮箱绑定了多个账号，请使用验证码选择账号后再重置密码")
	}
	user := users[0]
	token, _, err := CreateAuthActionToken(user.ID, ActionTokenPurposePasswordReset, PasswordResetLinkTTL)
	if err != nil {
		return nil, "", err
	}
	return &user, token, nil
}

// ListPasswordResetAccountsByEmail 列出邮箱下可重置密码的账号。
func ListPasswordResetAccountsByEmail(email string) ([]PasswordResetAccountCandidate, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, fmt.Errorf("邮箱不能为空")
	}

	var users []model.User
	if err := model.DB.
		Where("email = ? AND status = ? AND deleted_at IS NULL", email, UserStatusActive).
		Order("username ASC").
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询邮箱绑定账号失败: %w", err)
	}

	candidates := make([]PasswordResetAccountCandidate, 0, len(users))
	for _, user := range users {
		candidates = append(candidates, PasswordResetAccountCandidate{
			Username: user.Username,
			Role:     user.Role,
		})
	}
	return candidates, nil
}

// FindPasswordResetUser 查找指定邮箱下的目标账号。
func FindPasswordResetUser(email, username string) (*model.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	username = strings.TrimSpace(username)
	if email == "" || username == "" {
		return nil, fmt.Errorf("邮箱和用户名不能为空")
	}

	var user model.User
	if err := model.DB.
		Where("email = ? AND username = ? AND status = ? AND deleted_at IS NULL", email, username, UserStatusActive).
		First(&user).Error; err != nil {
		return nil, fmt.Errorf("未找到对应的已激活账户")
	}
	return &user, nil
}

// ResetPasswordByToken 使用邮件令牌重置密码
func ResetPasswordByToken(rawToken, newPassword string) error {
	record, user, err := FindValidAuthActionToken(rawToken, ActionTokenPurposePasswordReset)
	if err != nil {
		return err
	}
	if err := resetUserPassword(user, newPassword); err != nil {
		return err
	}
	if err := ConsumeAuthActionToken(record.ID); err != nil {
		return err
	}
	if err := InvalidateAuthActionTokens(user.ID, ActionTokenPurposePasswordReset); err != nil {
		return err
	}
	return nil
}

// ResetPasswordByUserID 使用用户 ID 直接重置密码。
func ResetPasswordByUserID(userID uint, newPassword string) error {
	var user model.User
	if err := model.DB.Where("id = ? AND status = ? AND deleted_at IS NULL", userID, UserStatusActive).First(&user).Error; err != nil {
		return fmt.Errorf("用户不存在或已失效")
	}
	if err := resetUserPassword(&user, newPassword); err != nil {
		return err
	}
	if err := InvalidateAuthActionTokens(user.ID, ActionTokenPurposePasswordReset); err != nil {
		return err
	}
	return nil
}

func resetUserPassword(user *model.User, newPassword string) error {
	if user == nil {
		return fmt.Errorf("用户不存在或已失效")
	}
	if err := D.ValidateStrongPassword(newPassword); err != nil {
		return err
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %w", err)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"password_hash":            string(hashedPassword),
		"force_password_change":    false,
		"login_verified_until":     nil,
		"high_risk_verified_until": nil,
		"security_updated_at":      &now,
	}
	for key, value := range PasswordFingerprintUpdateFields(newPassword) {
		updates[key] = value
	}
	if err := model.DB.Model(user).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新密码失败: %w", err)
	}
	if user.Role == "user" {
		_ = SyncUserPassword(user.Username, newPassword)
	}
	return nil
}

// BindUserEmail 绑定或更新邮箱
func BindUserEmail(userID uint, email string, verifiedAt time.Time) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if err := model.DB.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"email":             email,
		"email_verified_at": &verifiedAt,
	}).Error; err != nil {
		return err
	}
	// 管理员完成邮箱绑定后，尝试清除跳过引导标记
	_ = TryClearBootstrapSkipped(userID)
	return nil
}

// EnableUserTOTP 启用用户 2FA，返回恢复码明文（仅此一次）
func EnableUserTOTP(userID uint, secret string) (*TOTPRecoverySetup, error) {
	encrypted, err := EncryptSecurityText(secret)
	if err != nil {
		return nil, err
	}

	// 生成恢复码
	plainCodes, hashedCodes, err := GenerateRecoveryCodes()
	if err != nil {
		return nil, fmt.Errorf("生成恢复码失败: %w", err)
	}

	// 序列化并加密哈希列表
	hashedJSON, err := json.Marshal(hashedCodes)
	if err != nil {
		return nil, fmt.Errorf("序列化恢复码失败: %w", err)
	}
	encryptedCodes, err := EncryptSecurityText(string(hashedJSON))
	if err != nil {
		return nil, fmt.Errorf("加密恢复码失败: %w", err)
	}

	now := time.Now()
	err = model.DB.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"totp_enabled":            true,
		"totp_secret_enc":         encrypted,
		"totp_bound_at":           &now,
		"totp_recovery_codes_enc": encryptedCodes,
	}).Error
	if err != nil {
		return nil, err
	}

	// 管理员完成 2FA 绑定后，尝试清除跳过引导标记
	_ = TryClearBootstrapSkipped(userID)

	return &TOTPRecoverySetup{RecoveryCodes: plainCodes}, nil
}

// DisableUserTOTP 关闭用户 2FA
func DisableUserTOTP(userID uint) error {
	return model.DB.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"totp_enabled":            false,
		"totp_secret_enc":         "",
		"totp_bound_at":           nil,
		"totp_recovery_codes_enc": "",
	}).Error
}

// GetUserTOTPSecret 获取用户 2FA 密钥
func GetUserTOTPSecret(user *model.User) (string, error) {
	if user == nil || strings.TrimSpace(user.TOTPSecretEnc) == "" {
		return "", fmt.Errorf("尚未绑定 2FA")
	}
	plain, upgraded, err := DecryptSecurityTextAutoUpgrade(user.TOTPSecretEnc)
	if err != nil {
		return "", fmt.Errorf("2FA 密钥解密失败，加密密钥可能已变更，请重新绑定 2FA")
	}
	// 旧密钥解密成功后透明升级为新密钥
	if upgraded != "" {
		_ = model.DB.Model(&model.User{}).Where("id = ?", user.ID).Update("totp_secret_enc", upgraded).Error
	}
	return plain, nil
}

// HasRecoveryCodes 判断用户是否有可用恢复码
func HasRecoveryCodes(user *model.User) bool {
	return user != nil && user.TOTPEnabled && GetRecoveryCodesRemaining(user.TOTPRecoveryCodesEnc) > 0
}

// RegenerateRecoveryCodes 重新生成恢复码（旧码立即失效），返回新恢复码明文
func RegenerateRecoveryCodes(userID uint) (*TOTPRecoverySetup, error) {
	plainCodes, hashedCodes, err := GenerateRecoveryCodes()
	if err != nil {
		return nil, fmt.Errorf("生成恢复码失败: %w", err)
	}

	hashedJSON, err := json.Marshal(hashedCodes)
	if err != nil {
		return nil, fmt.Errorf("序列化恢复码失败: %w", err)
	}
	encryptedCodes, err := EncryptSecurityText(string(hashedJSON))
	if err != nil {
		return nil, fmt.Errorf("加密恢复码失败: %w", err)
	}

	err = model.DB.Model(&model.User{}).Where("id = ?", userID).
		Update("totp_recovery_codes_enc", encryptedCodes).Error
	if err != nil {
		return nil, err
	}

	return &TOTPRecoverySetup{RecoveryCodes: plainCodes}, nil
}

// UpdateLoginVerificationWindow 刷新登录验证窗口
func UpdateLoginVerificationWindow(userID uint) error {
	until := time.Now().Add(LoginVerificationWindow)
	return model.DB.Model(&model.User{}).Where("id = ?", userID).
		Update("login_verified_until", &until).Error
}

// GrantHighRiskVerificationTrust 设置高风险验证信任窗口
func GrantHighRiskVerificationTrust(userID uint) error {
	until := time.Now().Add(HighRiskEmailTrustWindow)
	return model.DB.Model(&model.User{}).Where("id = ?", userID).
		Update("high_risk_verified_until", &until).Error
}

// CanSkipHighRiskVerification 判断是否可跳过高风险验证
func CanSkipHighRiskVerification(user *model.User) bool {
	if user == nil {
		return false
	}
	// 优先检查已授予的高风险验证信任，避免跳过初始化后补绑邮箱的用户
	// 在验证码校验成功并重试原操作时再次收到邮箱验证要求。
	if user.HighRiskVerifiedUntil != nil && time.Now().Before(*user.HighRiskVerifiedUntil) {
		return true
	}
	// 管理员跳过了安全初始化
	if user.BootstrapSkipped {
		// 但如果 SMTP 已配置且邮箱已验证，说明可以正常发送验证码，
		// 此时应恢复邮箱验证，不再跳过
		if IsSMTPConfigured() && user.EmailVerifiedAt != nil {
			return false
		}
		// SMTP 未配置或邮箱未验证时，继续跳过
		return true
	}
	return false
}

// CanEnterBootstrap 判断是否需要进入安全引导
func CanEnterBootstrap(user *model.User) bool {
	if IsSecurityVerificationDisabled() {
		return false
	}
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		// 管理员如果已跳过安全初始化，不再进入引导
		if user.BootstrapSkipped {
			return false
		}
		return !IsSMTPConfigured() || user.EmailVerifiedAt == nil || !user.TOTPEnabled
	}
	return user.EmailVerifiedAt == nil
}

// SkipAdminBootstrap 管理员跳过安全初始化，标记已跳过
// 注意：不更新 security_updated_at，因为这只是标记变更，不应令当前 Token 失效
func SkipAdminBootstrap(userID uint) error {
	return model.DB.Model(&model.User{}).Where("id = ? AND role = ?", userID, "admin").Update("bootstrap_skipped", true).Error
}

// TryClearBootstrapSkipped 管理员完成安全配置后，自动清除跳过引导标记
// 当管理员已绑定邮箱且启用 2FA 时，视为安全配置完成
// 注意：不更新 security_updated_at，清除标记不应令当前 Token 失效
func TryClearBootstrapSkipped(userID uint) error {
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		return err
	}
	// 非管理员或无跳过标记，无需处理
	if user.Role != "admin" || !user.BootstrapSkipped {
		return nil
	}
	// 仅当邮箱已验证且 2FA 已启用时才清除跳过标记
	if user.EmailVerifiedAt != nil && user.TOTPEnabled {
		return model.DB.Model(&model.User{}).Where("id = ?", userID).Update("bootstrap_skipped", false).Error
	}
	return nil
}

// CreateActiveUserDirectly 直接创建已激活用户（SMTP未配置时的回退方案）
func CreateActiveUserDirectly(username, email, password, role, cloudType string,
	maxCPU, maxMemory, maxDisk, maxVM, maxStorage, maxRuntimeHours int,
	enablePortForward bool, maxPortForwards, maxSnapshots int,
	maxBandwidthUp, maxBandwidthDown, maxTrafficDown, maxTrafficUp float64, maxPublicIPs int) (*model.User, error) {

	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	if username == "" || email == "" {
		return nil, fmt.Errorf("用户名和邮箱不能为空")
	}
	if role == "" {
		role = "user"
	}
	cloudType = D.NormalizeCloudType(cloudType)

	if err := D.ValidateStrongPassword(password); err != nil {
		return nil, err
	}

	var count int64
	if err := model.DB.Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("检查用户名失败: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("用户名 %s 已存在", username)
	}
	model.DB.Unscoped().Where("username = ? AND deleted_at IS NOT NULL", username).Delete(&model.User{})

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}

	now := time.Now()
	loginVerifiedUntil := now.Add(LoginVerificationWindow)
	fingerprint := BuildPasswordFingerprint(password)
	user := &model.User{
		Username:             username,
		PasswordHash:         string(hashedPassword),
		PasswordBreachPrefix: fingerprint.Prefix,
		PasswordBreachHMAC:   fingerprint.HMAC,
		Email:                email,
		Role:                 role,
		CloudType:            cloudType,
		Status:               UserStatusActive,
		EmailVerifiedAt:      &now,
		LoginVerifiedUntil:   &loginVerifiedUntil,
		MaxCPU:               maxCPU,
		MaxMemory:            maxMemory,
		MaxDisk:              maxDisk,
		MaxVM:                maxVM,
		MaxStorage:           maxStorage,
		MaxRuntimeHours:      maxRuntimeHours,
		EnablePortForward:    enablePortForward,
		MaxPortForwards:      maxPortForwards,
		MaxSnapshots:         maxSnapshots,
		MaxBandwidthUp:       maxBandwidthUp,
		MaxBandwidthDown:     maxBandwidthDown,
		MaxTrafficDown:       maxTrafficDown,
		MaxTrafficUp:         maxTrafficUp,
		MaxPublicIPs:         maxPublicIPs,
	}

	if err := model.DB.Create(user).Error; err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	// 创建系统用户和目录资源
	if err := D.ProvisionSystemUserResources(user, password); err != nil {
		model.DB.Delete(user)
		return nil, err
	}

	if role == "user" && !D.IsLightweightCloudType(cloudType) {
		if _, err := D.EnsureDefaultSecurityGroup(user.Username); err != nil {
			model.DB.Delete(user)
			return nil, err
		}
		if _, err := D.EnsureDefaultVPCSwitch(user.Username); err != nil {
			model.DB.Delete(user)
			return nil, err
		}
	}

	return user, nil
}

// NeedsLoginVerification 判断是否需要登录期验证
func NeedsLoginVerification(user *model.User) bool {
	if IsSecurityVerificationDisabled() {
		return false
	}
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		// 已绑定 2FA 的管理员必须完成登录验证（即使曾跳过安全初始化）
		if user.TOTPEnabled {
			return true
		}
		// 未绑定 2FA 且跳过了安全初始化，无法进行登录验证
		if user.BootstrapSkipped {
			return false
		}
		return true
	}
	if user.LoginVerifiedUntil == nil {
		return true
	}
	return time.Now().After(*user.LoginVerifiedUntil)
}

// SyncUserPassword 同步系统用户密码
func SyncUserPassword(username, password string) error {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return nil
	}
	// 用户名安全校验：只允许字母数字下划线横杠
	for _, r := range username {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("用户名包含非法字符")
		}
	}
	// 密码安全校验：过滤控制字符（换行、回车等），防止通过 stdin 注入额外行
	cleanPassword := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 { // 控制字符和 DEL
			return -1
		}
		return r
	}, password)
	if cleanPassword == "" {
		return fmt.Errorf("密码过滤后为空")
	}
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(username + ":" + cleanPassword + "\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("同步系统密码失败: %s", string(output))
	}
	return nil
}
