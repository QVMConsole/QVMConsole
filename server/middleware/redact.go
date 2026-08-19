package middleware

import (
	"encoding/json"
	"strings"
)

// redactedValue 响应体脱敏后的占位值
const redactedValue = "[REDACTED]"

// isSensitiveResponseKey 判断响应 JSON 中的字段名是否属于敏感字段。
// 与查询参数脱敏规则（isSensitiveRequestLogQueryKey）独立的保守集合：
//   - 命中的字段值（仅字符串）会被替换为 [REDACTED]
//   - 刻意不匹配 code / key / name 等常见业务字段，避免破坏日志可读性
//   - 数字与布尔值（如 has_password: true）不做替换，避免误伤业务标志
func isSensitiveResponseKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "totp", "otp", "recovery_code", "verify_code", "auth_code",
		"captcha", "authorization", "passwd", "credential", "credentials":
		return true
	}
	return strings.Contains(k, "token") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "password") ||
		strings.Contains(k, "passwd") ||
		strings.Contains(k, "api_key") ||
		strings.Contains(k, "apikey")
}

// redactJSONValue 递归遍历 JSON 值，将敏感字段的字符串值替换为 [REDACTED]。
func redactJSONValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for key, item := range val {
			if isSensitiveResponseKey(key) {
				if _, ok := item.(string); ok {
					val[key] = redactedValue
					continue
				}
				// 敏感字段下若嵌套对象（如凭据对象），整体递归脱敏内部字符串
				val[key] = redactJSONValue(item)
				continue
			}
			val[key] = redactJSONValue(item)
		}
		return val
	case []any:
		for i, item := range val {
			val[i] = redactJSONValue(item)
		}
		return val
	default:
		return v
	}
}

// redactResponseJSON 对响应体 JSON 做脱敏处理，返回脱敏后的 JSON 字符串。
// 入参不是合法 JSON 时原样返回（由调用方按非 JSON 响应处理）。
func redactResponseJSON(raw []byte) (string, bool) {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", false
	}
	redacted := redactJSONValue(parsed)
	out, err := json.Marshal(redacted)
	if err != nil {
		return "", false
	}
	return string(out), true
}