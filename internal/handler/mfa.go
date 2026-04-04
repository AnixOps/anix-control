package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// MFAHandler handles user and admin MFA endpoints.
type MFAHandler struct {
	mfaService          *service.MFAService
	systemConfigService *service.SystemConfigService
}

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
		TOTPIssuer:       "V2Board",
		BackupCodesCount: 10,
		BackupCodeCount:  10,
		MaxAttempts:      5,
		LockoutDuration:  15,
	}
}

func boolFromValue(raw interface{}) (bool, bool) {
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

func intFromValue(raw interface{}) (int, bool) {
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

func parseMethodsMap(raw interface{}) map[string]bool {
	parsed := map[string]bool{}

	switch v := raw.(type) {
	case map[string]interface{}:
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

func parseAllowedMethods(raw interface{}) map[string]bool {
	parsed := map[string]bool{}
	add := func(method string) {
		key := strings.ToLower(strings.TrimSpace(method))
		if key == "totp" || key == "sms" || key == "email" {
			parsed[key] = true
		}
	}

	switch v := raw.(type) {
	case []interface{}:
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
	case map[string]interface{}:
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
		cfg.TOTPIssuer = "V2Board"
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

func (h *MFAHandler) loadAdminConfig() (mfaAdminConfig, error) {
	cfg := defaultMFAAdminConfig()

	var raw map[string]interface{}
	if err := h.systemConfigService.GetJSON(mfaAdminConfigKey, &raw); err != nil {
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

func (h *MFAHandler) applyRuntimeConfig(cfg mfaAdminConfig) {
	allowedBytes, _ := json.Marshal(cfg.AllowedMethods)
	h.mfaService.SetConfig(&model.MFAConfig{
		Enabled:         cfg.Enabled,
		EnforceForAll:   cfg.Required,
		EnforceForAdmin: cfg.EnforceForAdmin,
		AllowedMethods:  string(allowedBytes),
		TOTPIssuer:      cfg.TOTPIssuer,
		BackupCodeCount: cfg.BackupCodesCount,
	})
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

// NewMFAHandler creates MFA handler.
func NewMFAHandler() *MFAHandler {
	db := database.Get()
	handler := &MFAHandler{
		mfaService:          service.NewMFAService(db, nil),
		systemConfigService: service.NewSystemConfigService(db),
	}

	if cfg, err := handler.loadAdminConfig(); err == nil {
		handler.applyRuntimeConfig(cfg)
	}

	return handler
}

// GetStatus gets MFA status for current user.
func (h *MFAHandler) GetStatus(c *gin.Context) {
	userID := c.GetUint("user_id")

	mfa, err := h.mfaService.GetUserMFA(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if mfa == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"enabled":          false,
				"has_backup_codes": false,
				"remaining_codes":  0,
				"last_used":        nil,
				"enforced":         false,
			},
		})
		return
	}

	remainingCodes, _ := h.mfaService.GetRemainingBackupCodes(userID)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"enabled":          mfa.Enabled,
			"has_backup_codes": len(mfa.BackupCodes) > 0,
			"remaining_codes":  remainingCodes,
			"last_used":        mfa.LastUsed,
			"last_method":      mfa.LastMethod,
		},
	})
}

// SetupTOTP sets up TOTP for current user.
func (h *MFAHandler) SetupTOTP(c *gin.Context) {
	userID := c.GetUint("user_id")

	var user model.User
	if err := database.Get().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	setup, err := h.mfaService.SetupTOTP(userID, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"secret":       setup.Secret,
			"url":          setup.URL,
			"qr_code":      setup.QRCode,
			"backup_codes": setup.BackupCodes,
		},
	})
}

// EnableTOTP enables TOTP with verification code.
func (h *MFAHandler) EnableTOTP(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.mfaService.EnableTOTP(userID, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "MFA enabled successfully"})
}

// DisableMFA disables MFA with password confirmation.
func (h *MFAHandler) DisableMFA(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.mfaService.DisableMFA(userID, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "MFA disabled successfully"})
}

// VerifyMFA verifies MFA code.
func (h *MFAHandler) VerifyMFA(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req struct {
		Code   string `json:"code" binding:"required"`
		Method string `json:"method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	valid, err := h.mfaService.Verify(userID, req.Code, req.Method)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "verified successfully"})
}

// RegenerateBackupCodes regenerates backup codes for current user.
func (h *MFAHandler) RegenerateBackupCodes(c *gin.Context) {
	userID := c.GetUint("user_id")

	codes, err := h.mfaService.RegenerateBackupCodes(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"backup_codes": codes,
		},
	})
}

// GetAdminConfig returns global MFA admin config.
func (h *MFAHandler) GetAdminConfig(c *gin.Context) {
	cfg, err := h.loadAdminConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.applyRuntimeConfig(cfg)
	c.JSON(http.StatusOK, gin.H{"data": mfaAdminConfigResponse(cfg)})
}

// UpdateAdminConfig updates and persists MFA admin config.
func (h *MFAHandler) UpdateAdminConfig(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg, err := h.loadAdminConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if v, ok := boolFromValue(req["enabled"]); ok {
		cfg.Enabled = v
	}
	if v, ok := boolFromValue(req["required"]); ok {
		cfg.Required = v
	}
	if v, ok := boolFromValue(req["enforce_for_all"]); ok {
		cfg.Required = v
	}
	if v, ok := boolFromValue(req["enforce_for_admin"]); ok {
		cfg.EnforceForAdmin = v
	}
	if issuer, ok := req["totp_issuer"].(string); ok && strings.TrimSpace(issuer) != "" {
		cfg.TOTPIssuer = strings.TrimSpace(issuer)
	}
	if v, ok := intFromValue(req["backup_codes_count"]); ok && v > 0 {
		cfg.BackupCodesCount = v
	}
	if v, ok := intFromValue(req["backup_code_count"]); ok && v > 0 {
		cfg.BackupCodesCount = v
	}
	if v, ok := intFromValue(req["max_attempts"]); ok && v > 0 {
		cfg.MaxAttempts = v
	}
	if v, ok := intFromValue(req["lockout_duration"]); ok && v > 0 {
		cfg.LockoutDuration = v
	}

	if methods, exists := req["methods"]; exists {
		parsed := parseMethodsMap(methods)
		if len(parsed) > 0 {
			cfg.Methods = cloneMFAMethods(parsed)
		}
	}
	if allowed, exists := req["allowed_methods"]; exists {
		parsed := parseAllowedMethods(allowed)
		if len(parsed) > 0 {
			cfg.Methods = cloneMFAMethods(parsed)
		}
	}

	cfg = normalizeMFAConfig(cfg)
	if err := h.systemConfigService.SetJSON(
		mfaAdminConfigKey,
		cfg,
		"security",
		"mfa admin config",
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.applyRuntimeConfig(cfg)
	c.JSON(http.StatusOK, gin.H{
		"message": "MFA config updated",
		"data":    mfaAdminConfigResponse(cfg),
	})
}
