package service

import "strings"

const SensitiveSystemConfigPlaceholder = "********"

var sensitiveSystemConfigMarkers = []string{
	"token",
	"secret",
	"password",
	"passwd",
	"private_key",
	"privatekey",
	"api_key",
	"apikey",
	"access_key",
	"accesskey",
	"client_secret",
}

func IsSensitiveSystemConfigKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}

	for _, marker := range sensitiveSystemConfigMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}

	return false
}

func MaskSystemConfigValue(key, value string) (displayValue string, sensitive bool, hasValue bool) {
	sensitive = IsSensitiveSystemConfigKey(key)
	hasValue = strings.TrimSpace(value) != ""
	if !sensitive {
		return value, false, hasValue
	}
	if !hasValue {
		return "", true, false
	}
	return SensitiveSystemConfigPlaceholder, true, true
}
