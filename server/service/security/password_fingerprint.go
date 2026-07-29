package security

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"kvm_console/config"
)

const ForcePasswordChangeReasonBreach = "password_breach"

// PasswordFingerprint 保存可定时重检、但不能直接还原密码的指纹。
type PasswordFingerprint struct {
	Prefix string
	HMAC   string
}

// BuildPasswordFingerprint 使用账户安全密钥为密码 SHA-1 建立 HMAC 指纹。
func BuildPasswordFingerprint(password string) PasswordFingerprint {
	fullHash := PasswordSHA1(password)
	return PasswordFingerprint{
		Prefix: fullHash[:5],
		HMAC:   PasswordHashHMAC(fullHash),
	}
}

// PasswordSHA1 返回 HIBP 使用的大写 SHA-1。
func PasswordSHA1(password string) string {
	hash := sha1.Sum([]byte(password))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

// PasswordHashHMAC 对完整 SHA-1 进行 HMAC-SHA256，数据库不保存完整 SHA-1。
func PasswordHashHMAC(fullHash string) string {
	secret := ""
	if config.GlobalConfig != nil {
		secret = config.GlobalConfig.SecuritySecret
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.ToUpper(strings.TrimSpace(fullHash))))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

// PasswordFingerprintUpdateFields 构造密码写入时需要同步更新的安全字段。
func PasswordFingerprintUpdateFields(password string) map[string]interface{} {
	fingerprint := BuildPasswordFingerprint(password)
	return map[string]interface{}{
		"password_breach_prefix":            fingerprint.Prefix,
		"password_breach_hmac":              fingerprint.HMAC,
		"password_breached":                 false,
		"password_breach_count":             0,
		"password_breach_checked_at":        nil,
		"password_breach_detected_at":       nil,
		"password_breach_user_notified_at":  nil,
		"password_breach_admin_notified_at": nil,
		"force_password_change_reason":      "",
	}
}
