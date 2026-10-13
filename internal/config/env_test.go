package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -print-env and environment-variables.md list the operator keys only.
func TestEnvVarsAreTheOperatorKeys(t *testing.T) {
	var names []string
	for _, variable := range EnvVars(Defaults()) {
		require.True(t, strings.HasPrefix(variable.Name, EnvPrefix), variable.Name)
		require.False(t, reservedEnvNames[variable.Name], variable.Name)
		assert.False(t, variable.Secret, variable.Name)
		assert.NotEmpty(t, variable.Description, variable.Name)
		names = append(names, variable.Name)
	}
	assert.Equal(t, []string{
		"ANIX_CONTROL_DOMAIN", "ANIX_CONTROL_TLS_MODE", "ANIX_CONTROL_TLS_CERT_FILE", "ANIX_CONTROL_TLS_KEY_FILE",
		"ANIX_CONTROL_LISTEN", "ANIX_CONTROL_GRPC_PORT", "ANIX_CONTROL_GRPC_PUBLIC_NAME", "ANIX_CONTROL_SUBSCRIBE_PATH",
		"ANIX_CONTROL_DATA_DIR",
	}, names)
}

func TestLoadWithoutConfigFileUsesBuiltInDefaultsAndEnvironment(t *testing.T) {
	loaded, err := load("", []string{
		"ANIX_CONTROL_JWT_SECRET=container-secret-0123456789abcdef",
		"ANIX_CONTROL_DATABASE_HOST=db.internal",
		"ANIX_CONTROL_LISTEN=0.0.0.0:8080",
	})
	require.NoError(t, err)
	assert.Equal(t, "production", loaded.Env)
	assert.Equal(t, "release", loaded.Server.Mode)
	assert.Equal(t, []string{"0.0.0.0:8080"}, loaded.Panel().PlainAddresses)
	assert.Equal(t, "postgres", loaded.Database.Driver)
	assert.Equal(t, "db.internal", loaded.Database.Host)
	assert.Equal(t, "container-secret-0123456789abcdef", loaded.JWT.Secret)
	assert.Equal(t, "5s", loaded.Plugins.ControlPollInterval)
	assert.Empty(t, loaded.Plugins.ControlHostRuntimeDir, "private temporary host directories")
	assert.Equal(t, "jW26nr2tbthASoeq6RmIpx8Ah+uhPNIv9V1ewRVb1VE=", loaded.Plugins.OfficialPublicKey)
	require.NoError(t, loaded.ValidateForServer())
}

func TestLoadAppliesEnvironmentOverAConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("domain: panel.example.com\ndatabase:\n  driver: sqlite\n  password: from-file\n"), 0o600))
	loaded, err := load(path, []string{"ANIX_CONTROL_DATABASE_PASSWORD=from-env", "ANIX_CONTROL_DOMAIN=other.example.com"})
	require.NoError(t, err)
	assert.Equal(t, "from-env", loaded.Database.Password)
	assert.Equal(t, "sqlite", loaded.Database.Driver)
	assert.Equal(t, "other.example.com", loaded.Domain)
	assert.Equal(t, "release", loaded.Server.Mode)
}

// The JWT secret is derived from the root key when unset; a template value
// an old file still carries is refused.
func TestValidateForServerRefusesTheTemplateJWTSecret(t *testing.T) {
	for _, secret := range []string{"your-jwt-secret-key-change-in-production", " CHANGE-THIS-TO-A-VERY-LONG-RANDOM-STRING-IN-PRODUCTION "} {
		loaded := Defaults()
		loaded.JWT.Secret = secret
		require.Error(t, loaded.ValidateForServer(), "secret %q", secret)
		assert.True(t, PlaceholderJWTSecret(secret))
	}
	loaded := Defaults()
	require.NoError(t, loaded.ValidateForServer(), "the root key derives the secret")
	assert.False(t, PlaceholderJWTSecret("a-real-secret"))
}

func TestLoadValidatesDatabaseConnectionSettings(t *testing.T) {
	_, err := load("", []string{"ANIX_CONTROL_DATABASE_SSLMODE=sometimes"})
	require.ErrorContains(t, err, "database.sslmode")

	_, err = load("", []string{"ANIX_CONTROL_DATABASE_TIMEZONE=Mars/Base"})
	require.ErrorContains(t, err, "database.timezone")

	loaded, err := load("", []string{"ANIX_CONTROL_DATABASE_SSLMODE=verify-full", "ANIX_CONTROL_DATABASE_TIMEZONE=UTC"})
	require.NoError(t, err)
	assert.Equal(t, "verify-full", loaded.Database.SSLMode)
	assert.Equal(t, "UTC", loaded.Database.TimeZone)
}

func TestPathFromEnvTrimsTheConfiguredPath(t *testing.T) {
	t.Setenv(ConfigPathEnv, "  /app/config/config.yaml \n")
	assert.Equal(t, "/app/config/config.yaml", PathFromEnv())
	t.Setenv(ConfigPathEnv, "")
	assert.Empty(t, PathFromEnv())
}

// The drain delay is a built-in value now; log.format is still read.
func TestDrainDelayIsBuiltInAndLogFormatIsValidated(t *testing.T) {
	loaded, err := load("", []string{"ANIX_CONTROL_SERVER_SHUTDOWN_DRAIN_DELAY=soon"})
	require.NoError(t, err, "an ignored key is never parsed")
	delay, err := loaded.Server.DrainDelay()
	require.NoError(t, err)
	assert.Equal(t, "5s", delay.String())

	_, err = load("", []string{"ANIX_CONTROL_LOG_FORMAT=xml"})
	require.ErrorContains(t, err, "log.format")

	zero, err := (ServerConfig{}).DrainDelay()
	require.NoError(t, err)
	assert.Zero(t, zero)
	_, err = (ServerConfig{ShutdownDrainDelay: "10m"}).DrainDelay()
	require.ErrorContains(t, err, "shutdown_drain_delay")
}

// server.trusted_proxies: the built-in default is loopback only, an empty
// variable trusts no proxy, a file that does not set it keeps the default,
// and an invalid entry stops the load.
func TestTrustedProxiesDefaultsAndValidation(t *testing.T) {
	loaded, err := load("", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1/32", "::1/128"}, loaded.Server.TrustedProxies)

	loaded, err = load("", []string{"ANIX_CONTROL_SERVER_TRUSTED_PROXIES="})
	require.NoError(t, err)
	assert.NotNil(t, loaded.Server.TrustedProxies)
	assert.Empty(t, loaded.Server.TrustedProxies)

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0o600))
	loaded, err = load(path, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1/32", "::1/128"}, loaded.Server.TrustedProxies)

	_, err = load("", []string{"ANIX_CONTROL_SERVER_TRUSTED_PROXIES=127.0.0.1,proxy.example"})
	require.ErrorContains(t, err, "server.trusted_proxies")
}
