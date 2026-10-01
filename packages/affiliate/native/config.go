package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
)

// defaultInviteConfig is the configuration the kernel creates when there is
// none (InviteService.GetConfig).
func defaultInviteConfig() InviteConfig {
	return InviteConfig{
		Enabled:             true,
		AutoGenerate:        true,
		CodeCount:           5,
		CodeExpireDays:      0,
		CommissionEnabled:   true,
		CommissionType:      1,
		CommissionRate:      0.1,
		CommissionFixed:     0,
		CommissionMinAmount: 10,
	}
}

// loadConfig reads the stored configuration, creating the default one when
// there is none, as the kernel does on each read.
func loadConfig(db *gorm.DB) (*InviteConfig, error) {
	var cfg InviteConfig
	err := db.First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cfg = defaultInviteConfig()
		if err := db.Create(&cfg).Error; err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// frontendConfig is the JSON value of the invite.frontend.config system
// configuration key.
type frontendConfig struct {
	CodePrefix      string   `json:"code_prefix"`
	CodeLength      int      `json:"code_length"`
	WithdrawFee     float64  `json:"withdraw_fee"`
	WithdrawMethods []string `json:"withdraw_methods"`
}

func defaultFrontendConfig() frontendConfig {
	return frontendConfig{CodePrefix: "INV", CodeLength: 8, WithdrawFee: 0, WithdrawMethods: []string{"alipay"}}
}

// loadFrontendConfig reads the frontend settings through
// kapi_affiliate_settings_v1 as the kernel's loadFrontendConfig reads the
// key: defaults when it cannot be read or parsed, else the stored values
// over the defaults, the fee always.
func loadFrontendConfig(db *gorm.DB) frontendConfig {
	cfg := defaultFrontendConfig()
	var row Settings
	err := db.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row.Value, err = "", nil
	}
	if err != nil {
		return cfg
	}
	var stored frontendConfig
	if row.Value != "" {
		if err := json.Unmarshal([]byte(row.Value), &stored); err != nil {
			return cfg
		}
	}
	if strings.TrimSpace(stored.CodePrefix) != "" {
		cfg.CodePrefix = strings.TrimSpace(stored.CodePrefix)
	}
	if stored.CodeLength > 0 {
		cfg.CodeLength = stored.CodeLength
	}
	if len(stored.WithdrawMethods) > 0 {
		cfg.WithdrawMethods = stored.WithdrawMethods
	}
	cfg.WithdrawFee = stored.WithdrawFee
	return cfg
}

func commissionTypeText(t int) string {
	if t == 2 {
		return "fixed"
	}
	return "percent"
}

// configResponse is the kernel's inviteConfigResponse.
func configResponse(cfg *InviteConfig, frontend frontendConfig) map[string]any {
	ratePercent := cfg.CommissionRate
	if ratePercent <= 1 {
		ratePercent = ratePercent * 100
	}
	return map[string]any{
		"id":                    cfg.ID,
		"enabled":               cfg.Enabled,
		"auto_generate":         cfg.AutoGenerate,
		"code_count":            cfg.CodeCount,
		"code_expire_days":      cfg.CodeExpireDays,
		"commission_enabled":    cfg.CommissionEnabled,
		"commission_type":       commissionTypeText(cfg.CommissionType),
		"commission_type_code":  cfg.CommissionType,
		"commission_rate":       ratePercent,
		"commission_rate_ratio": cfg.CommissionRate,
		"commission_fixed":      cfg.CommissionFixed,
		"commission_min":        cfg.CommissionMinAmount,
		"min_withdraw":          cfg.CommissionMinAmount,
		"first_order_bonus":     cfg.FirstOrderBonus,
		"first_traffic_bonus":   cfg.FirstTrafficBonus,
		"created_at":            cfg.CreatedAt,
		"updated_at":            cfg.UpdatedAt,
		"code_prefix":           frontend.CodePrefix,
		"code_length":           frontend.CodeLength,
		"withdraw_fee":          frontend.WithdrawFee,
		"withdraw_methods":      frontend.WithdrawMethods,
	}
}

// AdminConfig is GET /api/v2/admin/invite/config: the invite configuration
// (created with its defaults when there is none) and the frontend settings.
func (s *Service) AdminConfig(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	cfg, err := loadConfig(db)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	return s.panel(configResponse(cfg, loadFrontendConfig(db)))
}
