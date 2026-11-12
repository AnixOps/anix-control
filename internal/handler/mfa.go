package handler

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/branding"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const mfaAdminConfigKey = "security.mfa.config"

type mfaAdminConfig struct {
	Enabled          bool            `json:"enabled"`
	Required         bool            `json:"required"`
	EnforceForAll    bool            `json:"enforce_for_all"`
	EnforceForAdmin  bool            `json:"enforce_for_admin"`
	Methods          map[string]bool `json:"methods"`
	AllowedMethods   []string        `json:"allowed_methods"`
	TOTPIssuer       string          `json:"totp_issuer"`
	BackupCodesCount int             `json:"backup_codes_count"`
	BackupCodeCount  int             `json:"backup_code_count"`
	MaxAttempts      int             `json:"max_attempts"`
	LockoutDuration  int             `json:"lockout_duration"`
}

func defaultMFAMethods() map[string]bool {
	return map[string]bool{
		"totp":  true,
		"sms":   false,
		"email": true,
	}
}

func cloneMFAMethods(methods map[string]bool) map[string]bool {
	cloned := defaultMFAMethods()
	for k, v := range methods {
		if k == "totp" || k == "sms" || k == "email" {
			cloned[k] = v
		}
	}
	return cloned
}

func defaultMFAAdminConfig() mfaAdminConfig {
	return mfaAdminConfig{
		Enabled:          false,
		Required:         false,
		EnforceForAll:    false,
		EnforceForAdmin:  false,
		Methods:          defaultMFAMethods(),
		AllowedMethods:   []string{"totp", "email"},
		TOTPIssuer:       branding.ControlName,
		BackupCodesCount: 10,
		BackupCodeCount:  10,
		MaxAttempts:      5,
		LockoutDuration:  15,
	}
}

func boolFromValue(raw any) (bool, bool) {
	switch v := raw.(type) {
	case bool:
		return v, true
	case int:
		return v != 0, true
	case int32:
		return v != 0, true
	case int64:
		return v != 0, true
	case float64:
		return int(v) != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off", "":
			return false, true
		}
	}
	return false, false
}

func intFromValue(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		value := strings.TrimSpace(v)
		if value == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func parseMethodsMap(raw any) map[string]bool {
	parsed := map[string]bool{}

	switch v := raw.(type) {
	case map[string]any:
		for _, key := range []string{"totp", "sms", "email"} {
			if b, ok := boolFromValue(v[key]); ok {
				parsed[key] = b
			}
		}
	case map[string]bool:
		for _, key := range []string{"totp", "sms", "email"} {
			if val, ok := v[key]; ok {
				parsed[key] = val
			}
		}
	}

	return parsed
}

func parseAllowedMethods(raw any) map[string]bool {
	parsed := map[string]bool{}
	add := func(method string) {
		key := strings.ToLower(strings.TrimSpace(method))
		if key == "totp" || key == "sms" || key == "email" {
			parsed[key] = true
		}
	}

	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				add(s)
			}
		}
	case []string:
		for _, item := range v {
			add(item)
		}
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return parsed
		}
		var items []string
		if err := json.Unmarshal([]byte(text), &items); err == nil {
			for _, item := range items {
				add(item)
			}
			return parsed
		}
		for _, item := range strings.Split(text, ",") {
			add(item)
		}
	case map[string]any:
		return parseMethodsMap(v)
	case map[string]bool:
		return parseMethodsMap(v)
	}

	return parsed
}

func methodsToAllowed(methods map[string]bool) []string {
	allowed := make([]string, 0, 3)
	for _, key := range []string{"totp", "sms", "email"} {
		if methods[key] {
			allowed = append(allowed, key)
		}
	}
	return allowed
}

func normalizeMFAConfig(cfg mfaAdminConfig) mfaAdminConfig {
	cfg.Methods = cloneMFAMethods(cfg.Methods)
	if !cfg.Methods["totp"] && !cfg.Methods["sms"] && !cfg.Methods["email"] {
		cfg.Methods["totp"] = true
	}

	if strings.TrimSpace(cfg.TOTPIssuer) == "" {
		cfg.TOTPIssuer = branding.ControlName
	}
	if cfg.BackupCodesCount <= 0 {
		cfg.BackupCodesCount = 10
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.LockoutDuration <= 0 {
		cfg.LockoutDuration = 15
	}

	cfg.EnforceForAll = cfg.Required
	cfg.BackupCodeCount = cfg.BackupCodesCount
	cfg.AllowedMethods = methodsToAllowed(cfg.Methods)
	return cfg
}

func loadMFAAdminConfig(db *gorm.DB) (mfaAdminConfig, error) {
	cfg := defaultMFAAdminConfig()

	var raw map[string]any
	if err := service.NewSystemConfigService(db).GetJSON(mfaAdminConfigKey, &raw); err != nil {
		return cfg, err
	}
	if len(raw) == 0 {
		return cfg, nil
	}

	if v, ok := boolFromValue(raw["enabled"]); ok {
		cfg.Enabled = v
	}
	if v, ok := boolFromValue(raw["required"]); ok {
		cfg.Required = v
	}
	if v, ok := boolFromValue(raw["enforce_for_all"]); ok {
		cfg.Required = v
	}
	if v, ok := boolFromValue(raw["enforce_for_admin"]); ok {
		cfg.EnforceForAdmin = v
	}
	if issuer, ok := raw["totp_issuer"].(string); ok && strings.TrimSpace(issuer) != "" {
		cfg.TOTPIssuer = strings.TrimSpace(issuer)
	}
	if v, ok := intFromValue(raw["backup_codes_count"]); ok && v > 0 {
		cfg.BackupCodesCount = v
	}
	if v, ok := intFromValue(raw["backup_code_count"]); ok && v > 0 {
		cfg.BackupCodesCount = v
	}
	if v, ok := intFromValue(raw["max_attempts"]); ok && v > 0 {
		cfg.MaxAttempts = v
	}
	if v, ok := intFromValue(raw["lockout_duration"]); ok && v > 0 {
		cfg.LockoutDuration = v
	}

	if methods := parseMethodsMap(raw["methods"]); len(methods) > 0 {
		cfg.Methods = cloneMFAMethods(methods)
	} else if allowed := parseAllowedMethods(raw["allowed_methods"]); len(allowed) > 0 {
		cfg.Methods = cloneMFAMethods(allowed)
	}

	return normalizeMFAConfig(cfg), nil
}

func mfaAdminConfigResponse(cfg mfaAdminConfig) gin.H {
	return gin.H{
		"enabled":            cfg.Enabled,
		"required":           cfg.Required,
		"methods":            cfg.Methods,
		"backup_codes_count": cfg.BackupCodesCount,
		"max_attempts":       cfg.MaxAttempts,
		"lockout_duration":   cfg.LockoutDuration,

		"enforce_for_all":   cfg.EnforceForAll,
		"enforce_for_admin": cfg.EnforceForAdmin,
		"backup_code_count": cfg.BackupCodeCount,
		"allowed_methods":   cfg.AllowedMethods,
		"totp_issuer":       cfg.TOTPIssuer,
	}
}

// AdminMFASettings returns the admin MFA configuration as the admin API
// shows it, with defaults applied; KernelIdentity seeds identity with it.
func AdminMFASettings(db *gorm.DB) (map[string]any, error) {
	cfg, err := loadMFAAdminConfig(db)
	if err != nil {
		return nil, err
	}
	return mfaAdminConfigResponse(cfg), nil
}
