package native

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"golang.org/x/crypto/bcrypt"
)

// defaultTOTPIssuer is Control's product name, the v2 default issuer.
const defaultTOTPIssuer = "AnixOps Control"

// adminMFA is the admin MFA configuration, with the v2 defaults, coercions
// and normalization (handler/mfa.go in the kernel).
type adminMFA struct {
	Enabled          bool
	Required         bool
	EnforceForAll    bool
	EnforceForAdmin  bool
	Methods          map[string]bool
	AllowedMethods   []string
	TOTPIssuer       string
	BackupCodesCount int
	BackupCodeCount  int
	MaxAttempts      int
	LockoutDuration  int
}

var mfaMethodNames = []string{"totp", "sms", "email"}

func defaultMFAMethods() map[string]bool {
	return map[string]bool{"totp": true, "sms": false, "email": true}
}

func defaultAdminMFA() adminMFA {
	return adminMFA{
		Methods: defaultMFAMethods(), AllowedMethods: []string{"totp", "email"}, TOTPIssuer: defaultTOTPIssuer,
		BackupCodesCount: 10, BackupCodeCount: 10, MaxAttempts: 5, LockoutDuration: 15,
	}
}

func cloneMFAMethods(methods map[string]bool) map[string]bool {
	cloned := defaultMFAMethods()
	for key, value := range methods {
		if key == "totp" || key == "sms" || key == "email" {
			cloned[key] = value
		}
	}
	return cloned
}

func boolFromValue(raw any) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case float64:
		return int(value) != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off", "":
			return false, true
		}
	}
	return false, false
}

func intFromValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case float64:
		return int(value), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		if parsed, err := strconv.Atoi(trimmed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func parseMethodsMap(raw any) map[string]bool {
	parsed := map[string]bool{}
	if values, ok := raw.(map[string]any); ok {
		for _, key := range mfaMethodNames {
			if value, ok := boolFromValue(values[key]); ok {
				parsed[key] = value
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
	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return parsed
		}
		var items []string
		if json.Unmarshal([]byte(text), &items) == nil {
			for _, item := range items {
				add(item)
			}
			return parsed
		}
		for _, item := range strings.Split(text, ",") {
			add(item)
		}
	case map[string]any:
		return parseMethodsMap(value)
	}
	return parsed
}

func (c adminMFA) normalize() adminMFA {
	c.Methods = cloneMFAMethods(c.Methods)
	if !c.Methods["totp"] && !c.Methods["sms"] && !c.Methods["email"] {
		c.Methods["totp"] = true
	}
	if strings.TrimSpace(c.TOTPIssuer) == "" {
		c.TOTPIssuer = defaultTOTPIssuer
	}
	if c.BackupCodesCount <= 0 {
		c.BackupCodesCount = 10
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.LockoutDuration <= 0 {
		c.LockoutDuration = 15
	}
	c.EnforceForAll = c.Required
	c.BackupCodeCount = c.BackupCodesCount
	c.AllowedMethods = make([]string, 0, 3)
	for _, key := range mfaMethodNames {
		if c.Methods[key] {
			c.AllowedMethods = append(c.AllowedMethods, key)
		}
	}
	return c
}

// apply overlays a stored or submitted document; submitted allowed_methods
// win over methods (update is true for an admin update).
func (c adminMFA) apply(raw map[string]any, update bool) adminMFA {
	if value, ok := boolFromValue(raw["enabled"]); ok {
		c.Enabled = value
	}
	if value, ok := boolFromValue(raw["required"]); ok {
		c.Required = value
	}
	if value, ok := boolFromValue(raw["enforce_for_all"]); ok {
		c.Required = value
	}
	if value, ok := boolFromValue(raw["enforce_for_admin"]); ok {
		c.EnforceForAdmin = value
	}
	if issuer, ok := raw["totp_issuer"].(string); ok && strings.TrimSpace(issuer) != "" {
		c.TOTPIssuer = strings.TrimSpace(issuer)
	}
	for _, key := range []string{"backup_codes_count", "backup_code_count"} {
		if value, ok := intFromValue(raw[key]); ok && value > 0 {
			c.BackupCodesCount = value
		}
	}
	if value, ok := intFromValue(raw["max_attempts"]); ok && value > 0 {
		c.MaxAttempts = value
	}
	if value, ok := intFromValue(raw["lockout_duration"]); ok && value > 0 {
		c.LockoutDuration = value
	}
	if update {
		if methods, exists := raw["methods"]; exists {
			if parsed := parseMethodsMap(methods); len(parsed) > 0 {
				c.Methods = cloneMFAMethods(parsed)
			}
		}
		if allowed, exists := raw["allowed_methods"]; exists {
			if parsed := parseAllowedMethods(allowed); len(parsed) > 0 {
				c.Methods = cloneMFAMethods(parsed)
			}
		}
		return c
	}
	if methods := parseMethodsMap(raw["methods"]); len(methods) > 0 {
		c.Methods = cloneMFAMethods(methods)
	} else if allowed := parseAllowedMethods(raw["allowed_methods"]); len(allowed) > 0 {
		c.Methods = cloneMFAMethods(allowed)
	}
	return c
}

func (c adminMFA) response() map[string]any {
	return map[string]any{
		"enabled": c.Enabled, "required": c.Required, "methods": c.Methods, "backup_codes_count": c.BackupCodesCount,
		"max_attempts": c.MaxAttempts, "lockout_duration": c.LockoutDuration, "enforce_for_all": c.EnforceForAll,
		"enforce_for_admin": c.EnforceForAdmin, "backup_code_count": c.BackupCodeCount,
		"allowed_methods": c.AllowedMethods, "totp_issuer": c.TOTPIssuer,
	}
}

// loadAdminMFA reads the stored configuration: defaults when there is none.
func loadAdminMFA(stored map[string]any) adminMFA {
	if len(stored) == 0 {
		return defaultAdminMFA()
	}
	return defaultAdminMFA().apply(stored, false).normalize()
}

func (s *Service) adminMFA(ctx context.Context, stores *Stores) (adminMFA, error) {
	config, err := s.settings(ctx, stores)
	if err != nil {
		return adminMFA{}, err
	}
	return loadAdminMFA(config.AdminMFA), nil
}

// AdminMFAConfig is GET /api/v2/admin/mfa/config.
func (s *Service) AdminMFAConfig(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	config, err := s.adminMFA(ctx, stores)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(config.response())
}

// UpdateAdminMFAConfig is PUT /api/v2/admin/mfa/config.
func (s *Service) UpdateAdminMFAConfig(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var submitted map[string]any
	if err := binding.JSON.BindBody(request.Body, &submitted); err != nil {
		return s.panelError(err.Error())
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	config, err := s.adminMFA(ctx, stores)
	if err != nil {
		return s.panelError(err.Error())
	}
	config = config.apply(submitted, true).normalize()
	if err := s.storeAdminMFA(ctx, stores, config.response()); err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(config.response())
}

// storeAdminMFA replaces admin_mfa in the settings document, keeping the
// document's other fields as they are.
func (s *Service) storeAdminMFA(ctx context.Context, stores *Stores, value map[string]any) error {
	raw, _, err := stores.Settings.Get(ctx, SettingsKey)
	if err != nil {
		return err
	}
	document := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &document); err != nil {
			return err
		}
	}
	document["admin_mfa"] = value
	encoded, err := json.Marshal(document)
	if err != nil {
		return err
	}
	return stores.Settings.Put(ctx, SettingsKey, encoded)
}

// MFAStatus is GET /api/v2/user/mfa/status.
func (s *Service) MFAStatus(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	state, err := stores.Accounts.Status(ctx, uint64(request.Principal.ActorID))
	if err != nil {
		return s.panelError(err.Error())
	}
	if !state.Exists {
		return s.panel(map[string]any{
			"enabled": false, "has_backup_codes": false, "remaining_codes": 0, "last_used": nil, "enforced": false,
		})
	}
	var lastUsed any
	if !state.LastUsed.IsZero() {
		lastUsed = state.LastUsed
	}
	return s.panel(map[string]any{
		"enabled": state.Enabled, "has_backup_codes": state.HasBackupCodes, "remaining_codes": state.RemainingCodes,
		"last_used": lastUsed, "last_method": state.LastMethod,
	})
}

// SetupTOTP is POST /api/v2/user/mfa/totp/setup.
func (s *Service) SetupTOTP(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	users, err := stores.Accounts.Get(ctx, []uint64{uint64(request.Principal.ActorID)})
	if err != nil {
		return s.panelError("failed to load user")
	}
	if len(users) == 0 {
		return s.panelError("user not found")
	}
	config, err := s.adminMFA(ctx, stores)
	if err != nil {
		return s.panelError(err.Error())
	}
	setup, err := stores.Accounts.SetupTOTP(ctx, users[0].UserID, config.TOTPIssuer, users[0].Email, config.BackupCodesCount)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"secret": setup.Secret, "url": setup.URL, "qr_code": setup.QRCode, "backup_codes": setup.BackupCodes})
}

// EnableTOTP is POST /api/v2/user/mfa/totp/enable.
func (s *Service) EnableTOTP(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := stores.Accounts.EnableTOTP(ctx, uint64(request.Principal.ActorID), req.Code); err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"message": "MFA enabled successfully"})
}

// DisableMFA is POST /api/v2/user/mfa/disable.
func (s *Service) DisableMFA(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	users, err := stores.Accounts.Get(ctx, []uint64{uint64(request.Principal.ActorID)})
	if err != nil {
		return s.panelError(err.Error())
	}
	if len(users) == 0 {
		return s.panelError("record not found")
	}
	if bcrypt.CompareHashAndPassword([]byte(users[0].PasswordHash), []byte(req.Password)) != nil {
		return s.panelError("invalid password")
	}
	if err := stores.Accounts.DisableMFA(ctx, users[0].UserID); err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"message": "MFA disabled successfully"})
}

// VerifyMFA is POST /api/v2/user/mfa/verify.
func (s *Service) VerifyMFA(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Code   string `json:"code" binding:"required"`
		Method string `json:"method"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	valid, err := stores.Accounts.VerifyMFA(ctx, uint64(request.Principal.ActorID), req.Code, req.Method)
	if err != nil {
		return s.panelError(err.Error())
	}
	if !valid {
		return s.panelError("invalid code")
	}
	return s.panel(map[string]any{"message": "verified successfully"})
}

// RegenerateBackupCodes is POST /api/v2/user/mfa/backup-codes/regenerate.
func (s *Service) RegenerateBackupCodes(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	config, err := s.adminMFA(ctx, stores)
	if err != nil {
		return s.panelError(err.Error())
	}
	codes, err := stores.Accounts.RegenerateBackupCodes(ctx, uint64(request.Principal.ActorID), config.BackupCodesCount)
	if errors.Is(err, account.ErrMFANotEnabled) {
		return s.panelError(err.Error())
	}
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"backup_codes": codes})
}
