package handler

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/branding"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
)

const notificationEmailConfigKey = "notification.email.config"

func parseStringField(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func parseIntField(raw map[string]any, key string) int {
	v, ok := raw[key]
	if !ok || v == nil {
		return 0
	}
	switch vv := v.(type) {
	case int:
		return vv
	case int32:
		return int(vv)
	case int64:
		return int(vv)
	case float64:
		return int(vv)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(vv))
		return n
	default:
		return 0
	}
}

func normalizeEmailEncryption(raw any) (string, bool) {
	switch v := raw.(type) {
	case bool:
		if v {
			return "tls", true
		}
		return "none", true
	case float64:
		if int(v) == 1 {
			return "tls", true
		}
		if int(v) == 0 {
			return "none", true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "none", "false", "0", "off", "no":
			return "none", true
		case "tls", "true", "1", "on", "yes", "starttls":
			return "tls", true
		case "ssl":
			return "ssl", true
		}
	}
	return "", false
}

// loadEmailConfig reads the stored notification e-mail configuration over
// its defaults.
func loadEmailConfig(configs *service.SystemConfigService) (*model.EmailConfig, error) {
	cfg := &model.EmailConfig{
		Host:        "",
		Port:        587,
		Username:    "",
		Password:    "",
		FromAddress: "",
		FromName:    branding.ControlName,
		Encryption:  "tls",
	}

	value, err := configs.Get(notificationEmailConfigKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(value) == "" {
		return cfg, nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}

	if host := parseStringField(raw, "host"); host != "" {
		cfg.Host = host
	}
	if port := parseIntField(raw, "port"); port > 0 {
		cfg.Port = port
	}
	if username := parseStringField(raw, "username"); username != "" {
		cfg.Username = username
	}
	if password, ok := raw["password"]; ok {
		if p, ok := password.(string); ok {
			cfg.Password = p
		}
	}
	if fromAddress := parseStringField(raw, "from_address"); fromAddress != "" {
		cfg.FromAddress = fromAddress
	}
	if fromName := parseStringField(raw, "from_name"); fromName != "" {
		cfg.FromName = fromName
	}

	if enc, ok := raw["encryption_type"]; ok {
		if normalized, valid := normalizeEmailEncryption(enc); valid {
			cfg.Encryption = normalized
			return cfg, nil
		}
	}
	if enc, ok := raw["encryption"]; ok {
		if normalized, valid := normalizeEmailEncryption(enc); valid {
			cfg.Encryption = normalized
		}
	}

	return cfg, nil
}
