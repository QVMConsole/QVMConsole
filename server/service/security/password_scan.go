package security

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"kvm_console/logger"
	"kvm_console/model"
)

const (
	PasswordBreachScanSourceManual    = "manual"
	PasswordBreachScanSourceScheduled = "scheduled"
)

var passwordBreachNotifyMu sync.Mutex

// PasswordBreachScanResult 泄露密码扫描结果。
type PasswordBreachScanResult struct {
	CheckedUsers   int       `json:"checked_users"`
	BreachedTotal  int       `json:"breached_total"`
	BreachedAdmins int       `json:"breached_admins"`
	BreachedUsers  int       `json:"breached_users"`
	NewlyDetected  int       `json:"newly_detected"`
	FailedPrefixes int       `json:"failed_prefixes"`
	FinishedAt     time.Time `json:"finished_at"`
}

// PasswordBreachAffectedAccount 管理员可见的受影响账户。
type PasswordBreachAffectedAccount struct {
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	Count       int64      `json:"breach_count"`
	DetectedAt  *time.Time `json:"detected_at"`
	TOTPEnabled bool       `json:"totp_enabled"`
	Action      string     `json:"action"`
}

// PasswordBreachStatus 密码泄露检测整体状态。
type PasswordBreachStatus struct {
	SchedulerEnabled bool                            `json:"scheduler_enabled"`
	LastCheckedAt    *time.Time                      `json:"last_checked_at"`
	BreachedTotal    int                             `json:"breached_total"`
	BreachedAdmins   int                             `json:"breached_admins"`
	BreachedUsers    int                             `json:"breached_users"`
	AffectedAccounts []PasswordBreachAffectedAccount `json:"affected_accounts"`
}

// PasswordEnrollmentResult 旧账户首次登录纳管结果。
type PasswordEnrollmentResult struct {
	NewlyDetected bool
	Breached      bool
	Count         int64
}

// EnrollAndCheckPassword 为账户登记指纹，并在需要时立即检查。
func EnrollAndCheckPassword(user *model.User, password string, checkNow bool) (PasswordEnrollmentResult, error) {
	result := PasswordEnrollmentResult{}
	if user == nil || strings.TrimSpace(password) == "" {
		return result, nil
	}
	fingerprint := BuildPasswordFingerprint(password)
	fingerprintChanged := user.PasswordBreachPrefix != fingerprint.Prefix || user.PasswordBreachHMAC != fingerprint.HMAC
	if fingerprintChanged {
		updates := PasswordFingerprintUpdateFields(password)
		// 首次纳管只登记指纹，不改变默认密码等既有强制改密原因。
		delete(updates, "force_password_change_reason")
		if err := model.DB.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return result, fmt.Errorf("登记密码安全指纹失败: %w", err)
		}
		user.PasswordBreachPrefix = fingerprint.Prefix
		user.PasswordBreachHMAC = fingerprint.HMAC
		user.PasswordBreached = false
		user.PasswordBreachCount = 0
	}
	if !checkNow || !fingerprintChanged {
		return result, nil
	}
	breached, count, _, err := CheckPasswordBreachedWithCount(password)
	if err != nil {
		return result, err
	}
	newlyDetected, err := applyPasswordBreachResult(user, fingerprint, breached, count, time.Now())
	if err != nil {
		return result, err
	}
	result.NewlyDetected = newlyDetected
	result.Breached = breached
	result.Count = count
	return result, nil
}

// RunPasswordBreachScan 执行一次完整账户扫描。
func RunPasswordBreachScan(ctx context.Context, source string, progress func(int, string)) (PasswordBreachScanResult, error) {
	result := PasswordBreachScanResult{}
	var users []model.User
	if err := model.DB.Where("status = ? AND password_breach_prefix <> '' AND password_breach_hmac <> ''", UserStatusActive).Find(&users).Error; err != nil {
		return result, fmt.Errorf("读取待检测账户失败: %w", err)
	}
	if progress != nil {
		progress(5, fmt.Sprintf("已加载 %d 个待检测账户", len(users)))
	}

	groups := make(map[string][]model.User)
	for _, user := range users {
		prefix := strings.ToUpper(strings.TrimSpace(user.PasswordBreachPrefix))
		if len(prefix) == 5 {
			groups[prefix] = append(groups[prefix], user)
		}
	}
	prefixes := make([]string, 0, len(groups))
	for prefix := range groups {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)

	now := time.Now()
	for index, prefix := range prefixes {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		suffixes, err := GetHIBPRange(prefix)
		if err != nil {
			result.FailedPrefixes++
			logger.App.Warn("泄露密码前缀检测失败", "prefix", prefix, "error", err)
			continue
		}
		matched := make(map[string]int64)
		for suffix, count := range suffixes {
			matched[PasswordHashHMAC(prefix+suffix)] = count
		}
		for i := range groups[prefix] {
			user := &groups[prefix][i]
			count := matched[strings.ToUpper(user.PasswordBreachHMAC)]
			fingerprint := PasswordFingerprint{Prefix: prefix, HMAC: user.PasswordBreachHMAC}
			newlyDetected, updateErr := applyPasswordBreachResult(user, fingerprint, count > 0, count, now)
			if updateErr != nil {
				logger.App.Warn("更新账户泄露密码状态失败", "user", user.Username, "error", updateErr)
				continue
			}
			result.CheckedUsers++
			if count > 0 {
				result.BreachedTotal++
				if user.Role == "admin" {
					result.BreachedAdmins++
				} else {
					result.BreachedUsers++
				}
			}
			if newlyDetected {
				result.NewlyDetected++
			}
		}
		if progress != nil && len(prefixes) > 0 {
			progress(10+(index+1)*70/len(prefixes), fmt.Sprintf("已检测 %d/%d 个哈希前缀", index+1, len(prefixes)))
		}
	}

	if progress != nil {
		progress(85, "正在发送泄露密码通知")
	}
	if err := SendPendingPasswordBreachNotifications(source == PasswordBreachScanSourceScheduled); err != nil {
		logger.App.Warn("发送泄露密码通知存在失败", "error", err)
	}
	result.FinishedAt = time.Now()
	_ = model.SetSetting("password_breach_last_checked_at", result.FinishedAt.Format(time.RFC3339))
	if progress != nil {
		progress(100, fmt.Sprintf("检测完成：管理员 %d 个，普通用户 %d 个", result.BreachedAdmins, result.BreachedUsers))
	}
	if len(prefixes) > 0 && result.FailedPrefixes == len(prefixes) {
		return result, fmt.Errorf("泄露密码检测服务暂时不可用，所有哈希前缀均检测失败")
	}
	return result, nil
}

func applyPasswordBreachResult(user *model.User, fingerprint PasswordFingerprint, breached bool, count int64, checkedAt time.Time) (bool, error) {
	if user == nil {
		return false, nil
	}
	wasBreached := user.PasswordBreached
	updates := map[string]interface{}{
		"password_breach_checked_at": &checkedAt,
		"password_breach_count":      count,
		"password_breached":          breached,
	}
	if breached {
		if !wasBreached {
			updates["password_breach_detected_at"] = &checkedAt
			updates["password_breach_user_notified_at"] = nil
			updates["password_breach_admin_notified_at"] = nil
			if user.Role == "admin" {
				updates["security_updated_at"] = &checkedAt
				if user.TOTPEnabled {
					updates["force_password_change"] = true
					updates["force_password_change_reason"] = ForcePasswordChangeReasonBreach
				}
			}
		}
	} else if wasBreached {
		updates["password_breach_detected_at"] = nil
		updates["password_breach_user_notified_at"] = nil
		updates["password_breach_admin_notified_at"] = nil
		if user.ForcePasswordChangeReason == ForcePasswordChangeReasonBreach {
			updates["force_password_change"] = false
			updates["force_password_change_reason"] = ""
		}
	}
	query := model.DB.Model(&model.User{}).
		Where("id = ? AND password_breach_prefix = ? AND password_breach_hmac = ?", user.ID, fingerprint.Prefix, fingerprint.HMAC)
	// 首次建立和首次清除均使用状态条件，避免登录即时检测与后台扫描并发重复处置。
	if breached && !wasBreached {
		query = query.Where("password_breached = ?", false)
	} else if !breached && wasBreached {
		query = query.Where("password_breached = ?", true)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	user.PasswordBreached = breached
	user.PasswordBreachCount = count
	user.PasswordBreachCheckedAt = &checkedAt
	if breached && !wasBreached {
		user.PasswordBreachDetectedAt = &checkedAt
	}
	return breached && !wasBreached, nil
}

// GetPasswordBreachStatus 返回管理员概览需要的泄露状态。
func GetPasswordBreachStatus(schedulerEnabled bool) (PasswordBreachStatus, error) {
	status := PasswordBreachStatus{SchedulerEnabled: schedulerEnabled, AffectedAccounts: []PasswordBreachAffectedAccount{}}
	var users []model.User
	if err := model.DB.Where("status = ? AND password_breached = ?", UserStatusActive, true).
		Order("role ASC, password_breach_detected_at ASC, username ASC").Find(&users).Error; err != nil {
		return status, err
	}
	for _, user := range users {
		action := "请尽快修改密码"
		if user.Role == "admin" && !user.TOTPEnabled {
			action = "登录已锁定，请运行 qvmc-manage.sh 修改密码"
		} else if user.Role == "admin" {
			action = "登录后必须完成 2FA 并修改密码"
		}
		status.AffectedAccounts = append(status.AffectedAccounts, PasswordBreachAffectedAccount{
			Username: user.Username, Role: user.Role, Count: user.PasswordBreachCount,
			DetectedAt: user.PasswordBreachDetectedAt, TOTPEnabled: user.TOTPEnabled, Action: action,
		})
		status.BreachedTotal++
		if user.Role == "admin" {
			status.BreachedAdmins++
		} else {
			status.BreachedUsers++
		}
	}
	if value, ok := model.GetSetting("password_breach_last_checked_at"); ok {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			status.LastCheckedAt = &parsed
		}
	}
	return status, nil
}

// SendPendingPasswordBreachNotifications 发送尚未成功送达的首次通知及定时摘要。
func SendPendingPasswordBreachNotifications(includeDailyAdminSummary bool) error {
	passwordBreachNotifyMu.Lock()
	defer passwordBreachNotifyMu.Unlock()
	if !IsSMTPConfigured() {
		return fmt.Errorf("SMTP 尚未配置")
	}
	var errors []string
	var ordinaryUsers []model.User
	if err := model.DB.Where("status = ? AND role = ? AND password_breached = ? AND password_breach_user_notified_at IS NULL", UserStatusActive, "user", true).Find(&ordinaryUsers).Error; err != nil {
		return err
	}
	for i := range ordinaryUsers {
		user := &ordinaryUsers[i]
		if user.EmailVerifiedAt == nil || strings.TrimSpace(user.Email) == "" {
			continue
		}
		body := fmt.Sprintf("您好，%s：\n\n系统检测到您当前使用的密码已出现在公开泄露数据库中（记录次数：%d，检测时间：%s）。\n请登录后尽快前往安全中心修改密码。\n\n此邮件不包含您的密码或密码哈希。", user.Username, user.PasswordBreachCount, formatBreachTime(user.PasswordBreachDetectedAt))
		if err := SendEmail(user.Email, "账户密码泄露安全提醒", body); err != nil {
			errors = append(errors, fmt.Sprintf("用户 %s: %v", user.Username, err))
			continue
		}
		now := time.Now()
		_ = model.DB.Model(&model.User{}).Where("id = ? AND password_breached = ?", user.ID, true).Update("password_breach_user_notified_at", &now).Error
	}

	var pendingAdminNotice []model.User
	query := model.DB.Where("status = ? AND password_breached = ?", UserStatusActive, true)
	if !includeDailyAdminSummary {
		query = query.Where("password_breach_admin_notified_at IS NULL")
	}
	if err := query.Order("role ASC, username ASC").Find(&pendingAdminNotice).Error; err != nil {
		return err
	}
	if len(pendingAdminNotice) > 0 {
		recipients, err := listPasswordBreachAdminRecipients()
		if err != nil {
			return err
		}
		if len(recipients) == 0 {
			errors = append(errors, "没有已验证邮箱的管理员收件人")
		} else {
			body := buildAdminBreachMailBody(pendingAdminNotice, includeDailyAdminSummary)
			sentAll := true
			for _, email := range recipients {
				if err := SendEmail(email, "密码泄露检测安全报告", body); err != nil {
					errors = append(errors, fmt.Sprintf("管理员邮箱 %s: %v", MaskEmail(email), err))
					sentAll = false
					continue
				}
			}
			if sentAll {
				now := time.Now()
				ids := make([]uint, 0, len(pendingAdminNotice))
				for _, user := range pendingAdminNotice {
					ids = append(ids, user.ID)
				}
				_ = model.DB.Model(&model.User{}).Where("id IN ? AND password_breached = ?", ids, true).Update("password_breach_admin_notified_at", &now).Error
			}
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "；"))
	}
	return nil
}

func listPasswordBreachAdminRecipients() ([]string, error) {
	var admins []model.User
	if err := model.DB.Where("status = ? AND role = ? AND email_verified_at IS NOT NULL AND email <> ''", UserStatusActive, "admin").Find(&admins).Error; err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(admins))
	for _, admin := range admins {
		email := strings.ToLower(strings.TrimSpace(admin.Email))
		if email != "" && !seen[email] {
			seen[email] = true
			result = append(result, email)
		}
	}
	return result, nil
}

func buildAdminBreachMailBody(users []model.User, daily bool) string {
	title := "系统检测到以下账户的当前密码已泄露："
	if daily {
		title = "以下账户的密码泄露问题仍未处理："
	}
	lines := []string{title, ""}
	for _, user := range users {
		totp := "未绑定"
		if user.TOTPEnabled {
			totp = "已绑定"
		}
		action := "通知用户尽快修改密码"
		if user.Role == "admin" && user.TOTPEnabled {
			action = "已撤销会话，登录后将强制修改密码"
		} else if user.Role == "admin" {
			action = "已撤销会话并锁定登录，需运行 qvmc-manage.sh 修改密码"
		}
		lines = append(lines, fmt.Sprintf("- 用户名：%s；角色：%s；泄露次数：%d；发现时间：%s；2FA：%s；处置：%s", user.Username, user.Role, user.PasswordBreachCount, formatBreachTime(user.PasswordBreachDetectedAt), totp, action))
	}
	lines = append(lines, "", "此邮件不包含任何密码或密码哈希。")
	return strings.Join(lines, "\n")
}

func formatBreachTime(value *time.Time) string {
	if value == nil {
		return "未知"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}
