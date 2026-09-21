package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// ValidateForStartup applies strict, fail-closed checks in production while
// preserving the lightweight development and test profiles.
func ValidateForStartup(c *Config) error {
	if c == nil {
		return errors.New("configuration is nil")
	}
	environment := strings.ToLower(strings.TrimSpace(c.Env))
	switch environment {
	case "", "development", "test":
		return nil
	case "production":
	default:
		return fmt.Errorf("unsupported environment %q; expected development, test, or production", c.Env)
	}

	var problems []string
	require := func(ok bool, message string) {
		if !ok {
			problems = append(problems, message)
		}
	}
	validSecret := func(value string) bool {
		trimmed := strings.TrimSpace(value)
		lower := strings.ToLower(trimmed)
		return len(trimmed) >= 32 &&
			!strings.Contains(lower, "change") &&
			!strings.Contains(lower, "replace") &&
			!strings.Contains(lower, "your-")
	}

	require(c.Server.Mode == "release", "server.mode must be release")
	require(strings.TrimSpace(c.Server.Host) != "", "server.host is required")
	require(c.Server.Port > 0 && c.Server.Port <= 65535, "server.port must be valid")
	require(c.Server.ReadTimeout > 0 && c.Server.WriteTimeout > 0, "server read/write timeouts must be positive")
	require(len(c.Server.TrustedProxies) > 0, "server.trusted_proxies must name the production reverse proxy")
	for _, proxy := range c.Server.TrustedProxies {
		require(validTrustedProxy(proxy), "server.trusted_proxies contains an invalid or universal address")
	}
	require(validProductionHTTPSURL(c.App.PublicURL), "app.public_url must be a non-placeholder absolute HTTPS URL")
	require(c.Frontend.Enable, "frontend.enable must be true")
	require(c.Frontend.Port > 0 && c.Frontend.Port <= 65535, "frontend.port must be valid")
	if c.TLS.Enable {
		require(filepath.IsAbs(c.TLS.CertFile), "tls.cert_file must be absolute when native TLS is enabled")
		require(filepath.IsAbs(c.TLS.KeyFile), "tls.key_file must be absolute when native TLS is enabled")
		require(nonPlaceholder(c.TLS.Domain) && !placeholderDomain(c.TLS.Domain), "tls.domain is required and must not be a placeholder when native TLS is enabled")
	}
	require(strings.EqualFold(c.Database.Driver, "postgres"), "database.driver must be postgres")
	require(strings.TrimSpace(c.Database.Host) != "", "database.host is required")
	require(c.Database.Port > 0 && c.Database.Port <= 65535, "database.port must be valid")
	require(strings.TrimSpace(c.Database.Username) != "", "database.username is required")
	require(validSecret(c.Database.Password), "database.password must be at least 32 characters and not a placeholder")
	require(strings.TrimSpace(c.Database.Database) != "", "database.database is required")
	require(strings.EqualFold(c.Cache.Driver, "memory"), "cache.driver must be memory; this release has no Redis cache backend")
	require(validSecret(c.JWT.Secret), "jwt.secret must be at least 32 characters and not a placeholder")
	require(validSecret(c.App.APIToken), "app.api_token must be at least 32 characters and not a placeholder")
	decodedKey, keyErr := base64.StdEncoding.DecodeString(strings.TrimSpace(c.Plugins.OfficialPublicKey))
	require(keyErr == nil && len(decodedKey) == 32, "plugins.official_public_key must be one base64 Ed25519 public key")
	require(validMailbox(c.Admin.Email) && !placeholderDomain(c.Admin.Email), "admin.email must be a non-placeholder mailbox")
	require(validSecret(c.Admin.Password), "admin.password must be at least 32 characters and not a placeholder")
	require(c.Auth.Registration.Enabled != nil, "auth.registration.enabled must be explicit")
	if c.Auth.Registration.Enabled != nil && *c.Auth.Registration.Enabled {
		require(c.Auth.Registration.RequireInvite, "public registration must require an invite in production")
	}

	require(c.Plugins.ControlExecutionEnabled, "plugins.control_execution_enabled must be true for machine telemetry operations")
	require(c.Plugins.DispatchEnabled, "plugins.dispatch_enabled must be true for Agent operations")
	require(c.GRPC.Enable, "grpc.enabled must be true for Agent operations")
	require(c.GRPC.Port > 0 && c.GRPC.Port <= 65535, "grpc.port must be valid")
	grpcHost := strings.TrimSpace(c.GRPC.Host)
	require(grpcHost != "", "grpc.host is required")
	certFile := strings.TrimSpace(c.GRPC.TLSCertFile)
	keyFile := strings.TrimSpace(c.GRPC.TLSKeyFile)
	require((certFile == "") == (keyFile == ""), "grpc.tls_cert_file and grpc.tls_key_file must be configured together")
	if grpcHost != "" && !isLoopbackHost(grpcHost) {
		require(c.GRPC.BehindTLSProxy || (certFile != "" && keyFile != ""), "non-loopback gRPC listeners require TLS files or behind_tls_proxy=true")
	}
	if c.GRPC.BehindTLSProxy {
		require(certFile == "" && keyFile == "", "grpc TLS files must be empty when behind_tls_proxy=true")
	} else if certFile != "" || keyFile != "" {
		require(filepath.IsAbs(certFile) && filepath.IsAbs(keyFile), "grpc TLS files must use absolute paths")
	}

	if raw := strings.TrimSpace(c.Maintenance.WorkerInterval); raw != "" {
		interval, err := time.ParseDuration(raw)
		require(err == nil && interval >= time.Second && interval <= 5*time.Minute, "maintenance.worker_interval must be between 1s and 5m")
	}
	require(strings.TrimSpace(c.Maintenance.SMTP.Host) != "", "maintenance.smtp.host is required")
	require(!placeholderDomain(c.Maintenance.SMTP.Host), "maintenance.smtp.host must not use a placeholder domain")
	require(c.Maintenance.SMTP.Port > 0 && c.Maintenance.SMTP.Port <= 65535, "maintenance.smtp.port must be valid")
	require(validMailbox(c.Maintenance.SMTP.From) && !placeholderDomain(c.Maintenance.SMTP.From), "maintenance.smtp.from must be one non-placeholder mailbox address")
	require(strings.TrimSpace(c.Maintenance.SMTP.Username) != "", "maintenance.smtp.username is required")
	require(validSecret(c.Maintenance.SMTP.Password), "maintenance.smtp.password must be at least 32 characters and not a placeholder")
	token := strings.TrimSpace(c.Maintenance.Telegram.BotToken)
	require(len(token) >= 16 && strings.Contains(token, ":") && !strings.Contains(strings.ToLower(token), "replace"), "maintenance.telegram.bot_token is invalid or still a placeholder")

	monitor := c.Maintenance.ExternalMonitoring
	require(nonPlaceholder(monitor.Provider), "maintenance.external_monitoring.provider is required and must not be a placeholder")
	require(nonPlaceholder(monitor.MonitorID), "maintenance.external_monitoring.monitor_id is required and must not be a placeholder")
	require(validProductionHTTPSURL(monitor.HealthURL), "maintenance.external_monitoring.health_url must be a non-placeholder absolute HTTPS URL")
	require(monitor.CheckIntervalSeconds >= 30 && monitor.CheckIntervalSeconds <= 300, "maintenance.external_monitoring.check_interval_seconds must be between 30 and 300")
	require(monitor.EmailEnabled, "maintenance.external_monitoring.email_enabled must be true")
	require(monitor.TelegramEnabled, "maintenance.external_monitoring.telegram_enabled must be true")

	if len(problems) > 0 {
		return fmt.Errorf("production configuration is not ready: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func validProductionHTTPSURL(raw string) bool {
	if !validHTTPSURL(raw) || placeholderDomain(raw) {
		return false
	}
	parsed, _ := url.Parse(strings.TrimSpace(raw))
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		return !ip.IsLoopback() && !ip.IsUnspecified()
	}
	return true
}

func nonPlaceholder(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	return trimmed != "" && !strings.Contains(lower, "replace") && !strings.Contains(lower, "change_me") && !strings.Contains(lower, "change-me")
}

func placeholderDomain(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.Contains(lower, "example.com") || strings.Contains(lower, "example.org") ||
		strings.Contains(lower, "example.net") || strings.Contains(lower, ".invalid") ||
		strings.Contains(lower, ".test") || strings.Contains(lower, ".localhost") ||
		lower == "localhost" || strings.Contains(lower, "your-domain")
}

func validMailbox(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(trimmed)
	return err == nil && parsed.Address == trimmed && !strings.ContainsAny(trimmed, "\r\n")
}

func isLoopbackHost(raw string) bool {
	host := strings.Trim(strings.TrimSpace(raw), "[]")
	if host == "localhost" {
		return true
	}
	if parsedHost, _, err := net.SplitHostPort(raw); err == nil {
		host = strings.Trim(parsedHost, "[]")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func validTrustedProxy(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "0.0.0.0/0" || trimmed == "::/0" {
		return false
	}
	if ip := net.ParseIP(trimmed); ip != nil {
		return !ip.IsUnspecified()
	}
	_, _, err := net.ParseCIDR(trimmed)
	return err == nil
}
