package utils

import (
	"encoding/json"
	"strings"
)

// Redact 脱敏字符串
// 规则：保留前4位和后4位，中间用 **** 替代
// 如果长度不足8位，全部用 **** 替代
func Redact(s string) string {
	if s == "" {
		return ""
	}
	length := len(s)
	if length <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[length-4:]
}

// RedactIP 脱敏 IP 地址
// IPv4: 192.168.1.100 -> 192.168.*.*
// IPv6: 保留前两段
func RedactIP(ip string) string {
	if ip == "" {
		return ""
	}
	// IPv4
	if strings.Contains(ip, ".") {
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			return parts[0] + "." + parts[1] + ".*.*"
		}
	}
	// IPv6
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) >= 2 {
			return parts[0] + ":" + parts[1] + ":****"
		}
	}
	return Redact(ip)
}

// SensitiveKeys 敏感字段名列表
var SensitiveKeys = []string{
	"password",
	"secret",
	"api_key",
	"apikey",
	"api-key",
	"token",
	"access_token",
	"refresh_token",
	"auth_key",
	"private_key",
	"credential",
}

// RedactMap 脱敏 Map 中的敏感字段
func RedactMap(data map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range data {
		lowerKey := strings.ToLower(key)
		isSensitive := false
		for _, sk := range SensitiveKeys {
			if strings.Contains(lowerKey, sk) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			if str, ok := value.(string); ok {
				result[key] = Redact(str)
			} else {
				result[key] = "[REDACTED]"
			}
		} else {
			// 递归处理嵌套 map
			if nested, ok := value.(map[string]any); ok {
				result[key] = RedactMap(nested)
			} else {
				result[key] = value
			}
		}
	}
	return result
}

// LogSafe 生成安全的日志字符串
type LogSafe struct {
	fields map[string]any
}

// NewLogSafe 创建安全日志对象
func NewLogSafe() *LogSafe {
	return &LogSafe{
		fields: make(map[string]any),
	}
}

// SetRaw 设置字段（不脱敏，用于非敏感字段）
func (l *LogSafe) SetRaw(key string, value any) *LogSafe {
	l.fields[key] = value
	return l
}

// SetIP 设置 IP 字段（脱敏）
func (l *LogSafe) SetIP(key, ip string) *LogSafe {
	l.fields[key] = RedactIP(ip)
	return l
}

// String 输出 JSON 格式
func (l *LogSafe) String() string {
	result, _ := json.Marshal(l.fields)
	return string(result)
}
