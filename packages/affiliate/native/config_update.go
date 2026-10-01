package native

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// KernelSettings is the part of the kernel's KernelSettings contract the
// native routes call.
type KernelSettings interface {
	PutSettings(ctx context.Context, in *kernelsettingsv1.PutSettingsRequest, opts ...grpc.CallOption) (*kernelsettingsv1.PutSettingsResponse, error)
}

// ConfigUpdateRouteID is the route that updates the invite configuration.
const ConfigUpdateRouteID = "affiliate.admin.invite.config.put"

const (
	// inviteNamespace is the KernelSettings namespace of the frontend
	// settings.
	inviteNamespace = "invite"
	// frontendConfigKey is the system configuration key holding them.
	frontendConfigKey = "invite.frontend.config"
)

// ConfigRequestID names an invite configuration update in the kernel's
// settings request ledger, so a retried request is applied once. token
// identifies the HTTP request: its Idempotency-Key, else its request id,
// else a fresh value.
func ConfigRequestID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("affiliate.invite_config:%x", sum[:12])
}

// requestToken is what identifies the request for ConfigRequestID.
func (s *Service) requestToken(request pluginhostsdk.NativeRequest) string {
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		for key, values := range request.Metadata.Headers {
			if strings.EqualFold(key, name) && len(values) > 0 {
				if value := strings.TrimSpace(values[0]); value != "" {
					return value
				}
			}
		}
	}
	if s.NewToken != nil {
		return s.NewToken()
	}
	return uuid.NewString()
}

// intFromAny is the kernel's inviteIntFromAny.
func intFromAny(v any) (int, bool) {
	switch raw := v.(type) {
	case int:
		return raw, true
	case int32:
		return int(raw), true
	case int64:
		return int(raw), true
	case float64:
		return int(raw), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// floatFromAny is the kernel's inviteFloatFromAny.
func floatFromAny(v any) (float64, bool) {
	switch raw := v.(type) {
	case float64:
		return raw, true
	case float32:
		return float64(raw), true
	case int:
		return float64(raw), true
	case int32:
		return float64(raw), true
	case int64:
		return float64(raw), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// stringSliceFromAny is the kernel's inviteStringSliceFromAny.
func stringSliceFromAny(v any) ([]string, bool) {
	switch raw := v.(type) {
	case []string:
		return raw, true
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			trimmed := strings.TrimSpace(str)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, true
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return []string{}, true
		}
		var parsed []string
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			return parsed, true
		}
		parts := strings.Split(trimmed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if p != "" {
				out = append(out, p)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// applyConfigRequest applies an update request to cfg and frontend as the
// kernel's UpdateConfig does, field by field, and answers the message of
// the first field it refuses.
func applyConfigRequest(req map[string]any, cfg *InviteConfig, frontend *frontendConfig) string {
	booleans := []struct {
		field  string
		target *bool
	}{{"enabled", &cfg.Enabled}, {"commission_enabled", &cfg.CommissionEnabled}, {"auto_generate", &cfg.AutoGenerate}}
	for _, boolean := range booleans {
		if raw, ok := req[boolean.field]; ok {
			value, valid := boolFromAny(raw)
			if !valid {
				return boolean.field + " must be boolean"
			}
			*boolean.target = value
		}
	}
	if raw, ok := req["code_count"]; ok {
		codeCount, valid := intFromAny(raw)
		if !valid || codeCount < 0 {
			return "code_count must be non-negative integer"
		}
		cfg.CodeCount = codeCount
	}
	if raw, ok := req["code_expire_days"]; ok {
		expireDays, valid := intFromAny(raw)
		if !valid || expireDays < 0 {
			return "code_expire_days must be non-negative integer"
		}
		cfg.CodeExpireDays = expireDays
	}
	if raw, ok := req["commission_type"]; ok {
		switch v := raw.(type) {
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "":
			case "percent", "percentage", "rate":
				cfg.CommissionType = 1
			case "fixed", "fixed_amount":
				cfg.CommissionType = 2
			default:
				return "commission_type must be percent/fixed or 1/2"
			}
		default:
			commissionType, valid := intFromAny(raw)
			if !valid {
				return "commission_type must be percent/fixed or 1/2"
			}
			if commissionType != 0 {
				if commissionType != 1 && commissionType != 2 {
					return "commission_type must be percent/fixed or 1/2"
				}
				cfg.CommissionType = commissionType
			}
		}
	}
	if raw, ok := req["commission_type_code"]; ok {
		commissionType, valid := intFromAny(raw)
		if !valid {
			return "commission_type_code must be 1 or 2"
		}
		// Zero, from a fully serialized struct, is "unspecified".
		if commissionType != 0 {
			if commissionType != 1 && commissionType != 2 {
				return "commission_type_code must be 1 or 2"
			}
			cfg.CommissionType = commissionType
		}
	}
	rateRatioProvided := false
	if raw, ok := req["commission_rate_ratio"]; ok {
		rateRatio, valid := floatFromAny(raw)
		if !valid || rateRatio < 0 {
			return "commission_rate_ratio must be non-negative number"
		}
		cfg.CommissionRate = rateRatio
		rateRatioProvided = true
	}
	if raw, ok := req["commission_rate"]; ok && !rateRatioProvided {
		rate, valid := floatFromAny(raw)
		if !valid || rate < 0 {
			return "commission_rate must be non-negative number"
		}
		if rate > 1 {
			rate = rate / 100
		}
		cfg.CommissionRate = rate
	}
	floats := []struct {
		field  string
		target *float64
	}{{"commission_fixed", &cfg.CommissionFixed}, {"commission_min", &cfg.CommissionMinAmount}, {"min_withdraw", &cfg.CommissionMinAmount}, {"first_order_bonus", &cfg.FirstOrderBonus}}
	for _, number := range floats {
		if raw, ok := req[number.field]; ok {
			value, valid := floatFromAny(raw)
			if !valid || value < 0 {
				return number.field + " must be non-negative number"
			}
			*number.target = value
		}
	}
	if raw, ok := req["first_traffic_bonus"]; ok {
		firstTrafficBonus, valid := intFromAny(raw)
		if !valid || firstTrafficBonus < 0 {
			return "first_traffic_bonus must be non-negative integer"
		}
		cfg.FirstTrafficBonus = int64(firstTrafficBonus)
	}
	if raw, ok := req["code_prefix"]; ok {
		codePrefix, ok := raw.(string)
		if !ok {
			return "code_prefix must be string"
		}
		frontend.CodePrefix = strings.TrimSpace(codePrefix)
	}
	if raw, ok := req["code_length"]; ok {
		codeLength, valid := intFromAny(raw)
		if !valid || codeLength <= 0 {
			return "code_length must be positive integer"
		}
		frontend.CodeLength = codeLength
	}
	if raw, ok := req["withdraw_fee"]; ok {
		withdrawFee, valid := floatFromAny(raw)
		if !valid || withdrawFee < 0 {
			return "withdraw_fee must be non-negative number"
		}
		frontend.WithdrawFee = withdrawFee
	}
	if raw, ok := req["withdraw_methods"]; ok {
		methods, valid := stringSliceFromAny(raw)
		if !valid || len(methods) == 0 {
			return "withdraw_methods must be non-empty string array"
		}
		frontend.WithdrawMethods = methods
	}
	return ""
}

// storedFrontendConfig is the value the kernel's saveFrontendConfig stores:
// the settings with an empty prefix, length or method list defaulted.
func storedFrontendConfig(cfg frontendConfig) frontendConfig {
	if strings.TrimSpace(cfg.CodePrefix) == "" {
		cfg.CodePrefix = "INV"
	}
	if cfg.CodeLength <= 0 {
		cfg.CodeLength = 8
	}
	if len(cfg.WithdrawMethods) == 0 {
		cfg.WithdrawMethods = []string{"alipay"}
	}
	return cfg
}

// AdminUpdateConfig is PUT /api/v2/admin/invite/config. The invite
// configuration is written to the adopted v2_invite_config, the frontend
// settings through the kernel's KernelSettings (namespace invite), which
// also makes the kernel's invite services reload the configuration they
// keep in memory. Like the kernel's handler it answers the settings as
// requested, before an empty prefix, length or method list is defaulted
// for storage.
func (s *Service) AdminUpdateConfig(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return badRequest(err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	current, err := loadConfig(db)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	cfg := *current
	frontend := loadFrontendConfig(db)
	if message := applyConfigRequest(req, &cfg, &frontend); message != "" {
		return badRequest(message)
	}

	if err := db.Save(&cfg).Error; err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	value, err := json.Marshal(storedFrontendConfig(frontend))
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	userID := uint64(request.Principal.ActorID)
	if _, err := s.KernelSettings.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{
		Namespace: inviteNamespace, RequestId: ConfigRequestID(s.requestToken(request)),
		Entries: []*kernelsettingsv1.SettingEntry{{
			Key: frontendConfigKey, Value: string(value), Type: "json", Group: "invite", Remark: "Invite frontend configuration fields",
		}},
		Actor: &kernelsettingsv1.Actor{UserId: &userID, ClientIp: request.Metadata.ClientIP, UserAgent: request.Metadata.UserAgent},
	}); err != nil {
		return errorAnswer(http.StatusInternalServerError, status.Convert(err).Message())
	}
	return s.panel(configResponse(&cfg, frontend))
}
