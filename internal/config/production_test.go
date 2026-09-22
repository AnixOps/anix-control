package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func productionReadyConfig() *Config {
	registrationEnabled := false
	return &Config{
		Env:      "production",
		Server:   ServerConfig{Host: "0.0.0.0", Port: 8080, Mode: "release", ReadTimeout: 30, WriteTimeout: 30, TrustedProxies: []string{"127.0.0.1"}},
		Frontend: FrontendConfig{Enable: true, Port: 3000},
		Database: DatabaseConfig{
			Driver: "postgres", Host: "db.internal", Port: 5432, Username: "anixops",
			Password: strings.Repeat("d", 32), Database: "anixops",
		},
		Cache: CacheConfig{Driver: "memory"},
		JWT:   JWTConfig{Secret: strings.Repeat("j", 32)},
		Auth:  AuthConfig{Registration: RegistrationAuthConfig{Enabled: &registrationEnabled, RequireInvite: true}},
		App:   AppConfig{PublicURL: "https://control.company.net", APIToken: strings.Repeat("a", 32)},
		Admin: AdminConfig{Email: "owner@company.net", Password: strings.Repeat("p", 32)},
		Plugins: PluginConfig{
			OfficialPublicKey:       "IaqXgif/OGydNv/mQHoyFmqOvzeplICaMZndrhqMG0M=",
			ControlExecutionEnabled: true,
			DispatchEnabled:         true,
		},
		GRPC: GRPCConfig{Enable: true, Host: "127.0.0.1", Port: 50051},
		Maintenance: MaintenanceConfig{
			WorkerInterval: "30s",
			SMTP: MaintenanceSMTPConfig{
				Host: "smtp.company.net", Port: 587, From: "alerts@company.net",
				Username: "alerts@company.net", Password: strings.Repeat("s", 32),
			},
			Telegram: MaintenanceTelegramConfig{BotToken: "123456789:real-looking-test-token"},
			ExternalMonitoring: MaintenanceExternalMonitoringConfig{
				Provider: "operator-selected", MonitorID: "monitor-123", HealthURL: "https://control.company.net/health",
				CheckIntervalSeconds: 60, EmailEnabled: true, TelegramEnabled: true,
			},
		},
	}
}

func TestValidateForStartupAcceptsCompleteProductionConfig(t *testing.T) {
	require.NoError(t, ValidateForStartup(productionReadyConfig()))
}

func TestValidateForStartupRejectsProductionPlaceholdersAndDisabledRuntime(t *testing.T) {
	cfg := productionReadyConfig()
	cfg.App.PublicURL = "https://control.example.com"
	cfg.Plugins.DispatchEnabled = false
	cfg.Maintenance.Telegram.BotToken = "REPLACE-WITH-TOKEN"
	cfg.Maintenance.ExternalMonitoring.EmailEnabled = false

	err := ValidateForStartup(cfg)
	require.Error(t, err)
	for _, expected := range []string{"app.public_url", "plugins.dispatch_enabled", "telegram.bot_token", "email_enabled"} {
		require.ErrorContains(t, err, expected)
	}
}

func TestValidateForStartupAllowsDevelopmentWithoutProductionSecrets(t *testing.T) {
	require.NoError(t, ValidateForStartup(&Config{Env: "development"}))
}

func TestValidateForStartupRejectsUnknownEnvironment(t *testing.T) {
	require.ErrorContains(t, ValidateForStartup(&Config{Env: "prod"}), "unsupported environment")
}

func TestValidateForStartupRequiresSecretKeyringForTopologyExecution(t *testing.T) {
	cfg := productionReadyConfig()
	cfg.Plugins.TopologyExecutionEnabled = true
	err := ValidateForStartup(cfg)
	require.ErrorContains(t, err, "plugins.secret_encryption")

	cfg.Plugins.SecretEncryption = PluginSecretEncryptionConfig{
		ActiveKeyID: "primary",
		Keys:        map[string]string{"primary": base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))},
	}
	require.NoError(t, ValidateForStartup(cfg))
}
