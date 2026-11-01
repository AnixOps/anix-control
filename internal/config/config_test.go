package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/branding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAndGet(t *testing.T) {
	cfg := &Config{
		Env: "test",
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}
	Set(cfg)

	got := Get()
	require.NotNil(t, got)
	assert.Equal(t, "test", got.Env)
	assert.Equal(t, "localhost", got.Server.Host)
	assert.Equal(t, 8080, got.Server.Port)
}

func TestGetNil(t *testing.T) {
	resetConfig()

	got := Get()
	assert.Nil(t, got)
}

// TestLoad: a 4.x style file still loads. The keys still read are applied,
// the ignored ones keep their built-in values, and each warns.
func TestLoad(t *testing.T) {
	resetConfig()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configContent := `
env: development
server:
  host: 127.0.0.1
  port: 3000
  mode: debug
database:
  driver: sqlite
  database: test.db
jwt:
  secret: test-secret
  expire: 3600
app:
  name: TestApp
  version: 1.0.0
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	loadedCfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, loadedCfg)

	assert.Equal(t, "production", loadedCfg.Env, "env is ignored")
	assert.Equal(t, "127.0.0.1", loadedCfg.Server.Host)
	assert.Equal(t, 3000, loadedCfg.Server.Port)
	assert.Equal(t, "release", loadedCfg.Server.Mode, "server.mode is ignored; -debug sets debug")
	assert.Equal(t, "sqlite", loadedCfg.Database.Driver)
	assert.Equal(t, "test.db", loadedCfg.Database.Database)
	assert.Equal(t, "test-secret", loadedCfg.JWT.Secret)
	assert.Equal(t, 86400, loadedCfg.JWT.Expire, "jwt.expire is ignored")
	assert.Equal(t, branding.ControlName, loadedCfg.App.Name, "app.name is ignored")
	var paths []string
	for _, key := range loadedCfg.Deprecated.Keys() {
		paths = append(paths, key.Path)
	}
	assert.Equal(t, []string{"app.name", "app.version", "database.database", "database.driver", "env", "jwt.expire", "jwt.secret", "server.host", "server.mode", "server.port"}, paths)
}

// The registration policy is still read (until M3); the rate limits are
// built-in values.
func TestLoadAuthRegistrationConfig(t *testing.T) {
	resetConfig()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configContent := `
auth:
  register_rate_limit:
    enabled: true
    max_attempts: 4
    window_seconds: 1800
    lockout_seconds: 3600
  registration:
    enabled: false
    require_invite: true
    allowed_email_domains:
      - example.com
    blocked_email_domains:
      - disposable.example
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	loadedCfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, loadedCfg)
	require.NotNil(t, loadedCfg.Auth.Registration.Enabled)
	require.NotNil(t, loadedCfg.Auth.RegisterRateLimit.Enabled)

	assert.False(t, *loadedCfg.Auth.Registration.Enabled)
	assert.True(t, loadedCfg.Auth.Registration.RequireInvite)
	assert.Equal(t, []string{"example.com"}, loadedCfg.Auth.Registration.AllowedEmailDomains)
	assert.Equal(t, []string{"disposable.example"}, loadedCfg.Auth.Registration.BlockedEmailDomains)
	assert.Equal(t, Defaults().Auth.RegisterRateLimit, loadedCfg.Auth.RegisterRateLimit, "the rate limits are built-in values")
}

// The plugins keys a deployment still needs are read; the tuning keys and
// the P7 switches keep their built-in values.
func TestLoadPluginConfig(t *testing.T) {
	resetConfig()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := `
plugins:
  official_public_key: "ZmFrZS1wdWJsaWMta2V5"
  identity_bootstrap_package_dir: "/var/lib/anixops/bootstrap"
  control_execution_enabled: false
  control_poll_interval: "3s"
  control_host_runtime_dir: "/var/lib/anixops/plugin-hosts"
  control_host_artifact_dir: "/var/lib/anixops/plugin-artifacts"
  control_host_startup_timeout: "7s"
  control_host_request_timeout: "45s"
  control_host_websocket_session_timeout: "6h"
  control_host_max_request_bytes: 2097152
  control_host_max_response_bytes: 134217728
  dispatch_enabled: true
  dispatch_poll_interval: 7s
  topology_execution_enabled: true
  topology_poll_interval: 11s
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))

	loaded, err := Load(configPath)
	require.NoError(t, err)
	defaults := Defaults().Plugins
	assert.Equal(t, "ZmFrZS1wdWJsaWMta2V5", loaded.Plugins.OfficialPublicKey)
	assert.Equal(t, "/var/lib/anixops/bootstrap", loaded.Plugins.IdentityBootstrapPackageDir)
	assert.Equal(t, "/var/lib/anixops/plugin-hosts", loaded.Plugins.ControlHostRuntimeDir)
	assert.Equal(t, "/var/lib/anixops/plugin-artifacts", loaded.Plugins.ControlHostArtifactDir)
	assert.Equal(t, defaults.ControlPollInterval, loaded.Plugins.ControlPollInterval)
	assert.Equal(t, defaults.ControlHostStartupTimeout, loaded.Plugins.ControlHostStartupTimeout)
	assert.Equal(t, DefaultControlHostPayloadBytes, loaded.Plugins.ControlHostRequestBodyLimit())
	assert.Equal(t, DefaultControlHostPayloadBytes, loaded.Plugins.ControlHostResponseBodyLimit(), "an oversized ignored limit cannot stop the start")
	assert.Len(t, loaded.Deprecated.Keys(), 15)
}

func TestPluginPayloadLimitsDefaultAndValidate(t *testing.T) {
	var plugins PluginConfig
	require.NoError(t, plugins.ValidatePayloadLimits())
	assert.Equal(t, DefaultControlHostPayloadBytes, plugins.ControlHostRequestBodyLimit())
	assert.Equal(t, DefaultControlHostPayloadBytes, plugins.ControlHostResponseBodyLimit())

	plugins.ControlHostMaxResponseBytes = MaxControlHostPayloadBytes
	require.NoError(t, plugins.ValidatePayloadLimits())
	assert.Equal(t, MaxControlHostPayloadBytes, plugins.ControlHostResponseBodyLimit())

	for _, invalid := range []int64{-1, MaxControlHostPayloadBytes + 1} {
		plugins = PluginConfig{ControlHostMaxRequestBytes: invalid}
		require.ErrorContains(t, plugins.ValidatePayloadLimits(), "plugins.control_host_max_request_bytes")
		assert.Equal(t, DefaultControlHostPayloadBytes, plugins.ControlHostRequestBodyLimit())
		plugins = PluginConfig{ControlHostMaxResponseBytes: invalid}
		require.ErrorContains(t, plugins.ValidatePayloadLimits(), "plugins.control_host_max_response_bytes")
	}
}

// The shipped template is the one domain line: it loads with no deprecated
// key, no secret and the built-in values for everything else.
func TestShippedTemplateIsTheDomainLine(t *testing.T) {
	const officialPublicKey = "jW26nr2tbthASoeq6RmIpx8Ah+uhPNIv9V1ewRVb1VE="
	decoded, err := base64.StdEncoding.DecodeString(officialPublicKey)
	require.NoError(t, err)
	require.Len(t, decoded, 32, "official Ed25519 public key must contain 32 raw bytes")

	path := filepath.Join("..", "..", "config", "config.yaml.example")
	loaded, report, err := LoadFile(path, nil)
	require.NoError(t, err)
	assert.Empty(t, report.Deprecated)
	assert.Equal(t, "panel.example.com", loaded.Domain)
	assert.Equal(t, TLSModeACME, loaded.TLSModeOrDefault())
	assert.Equal(t, officialPublicKey, loaded.Plugins.OfficialPublicKey)
	assert.Empty(t, loaded.JWT.Secret)
	assert.Empty(t, loaded.App.APIToken)
	assert.Empty(t, loaded.ModuleRuntime.CAKEK)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var keys []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			keys = append(keys, line)
		}
	}
	assert.Equal(t, []string{"domain: panel.example.com"}, keys)
	for _, retired := range []string{"config.prod.yaml", "config.dev.yaml.example"} {
		_, err := os.Stat(filepath.Join("..", "..", "config", retired))
		assert.True(t, os.IsNotExist(err), "%s was replaced by config.yaml.example", retired)
	}
}

func TestLoadUsesAnixOpsControlAsDefaultAppName(t *testing.T) {
	resetConfig()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("env: development\n"), 0o644))

	loadedCfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, loadedCfg)
	assert.Equal(t, branding.ControlName, loadedCfg.App.Name)
}

func TestLoadIgnoresTheRemovedForwardRuntime(t *testing.T) {
	resetConfig()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configContent := `
forward_runtime:
  backend: gost
  nodex:
    base_url: http://127.0.0.1:18081
    token: test-nodex-token
  clean_agent:
    legacy_bridge_enabled: true
  iptables_ansible:
    inventory: config/deploy/ansible/inventory.ini
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	loadedCfg, err := Load(configPath)
	require.NoError(t, err)
	var ignored []string
	for _, key := range loadedCfg.Deprecated.Keys() {
		if key.Status == KeyIgnored {
			ignored = append(ignored, key.Path)
		}
	}
	assert.Equal(t, []string{"forward_runtime.backend", "forward_runtime.clean_agent.legacy_bridge_enabled", "forward_runtime.iptables_ansible", "forward_runtime.nodex.base_url", "forward_runtime.nodex.token"}, ignored)
}

func TestLoadFileNotFound(t *testing.T) {
	resetConfig()

	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestLoadInvalidYAML(t *testing.T) {
	resetConfig()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// 无效的 YAML
	invalidYAML := `
env: development
server:
  host: [invalid
  port: not_a_number
`
	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	require.NoError(t, err)

	_, err = Load(configPath)
	assert.Error(t, err)
}

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{
		Env: "production",
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "release",
		},
		Database: DatabaseConfig{
			Driver:   "postgres",
			Database: "v2board",
			Host:     "localhost",
			Port:     5432,
		},
		Cache: CacheConfig{
			Driver: "redis",
		},
		JWT: JWTConfig{
			Secret: "secret",
			Expire: 86400,
		},
		App: AppConfig{
			Name:    branding.ControlName,
			Version: branding.DefaultVersion,
		},
		TLS: TLSConfig{
			Enable: true,
			Domain: "example.com",
		},
	}

	Set(cfg)
	got := Get()

	assert.Equal(t, "production", got.Env)
	assert.Equal(t, "0.0.0.0", got.Server.Host)
	assert.Equal(t, 8080, got.Server.Port)
	assert.Equal(t, "postgres", got.Database.Driver)
	assert.Equal(t, "redis", got.Cache.Driver)
	assert.True(t, got.TLS.Enable)
}

func TestServerConfig(t *testing.T) {
	tests := []struct {
		name   string
		config ServerConfig
	}{
		{"default", ServerConfig{Host: "0.0.0.0", Port: 8080, Mode: "debug"}},
		{"custom port", ServerConfig{Host: "localhost", Port: 3000, Mode: "release"}},
		{"with timeouts", ServerConfig{Host: "0.0.0.0", Port: 8080, ReadTimeout: 30, WriteTimeout: 30}},
		{"with trusted proxies", ServerConfig{Host: "0.0.0.0", Port: 8080, TrustedProxies: []string{"10.0.0.0/8"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Server: tt.config}
			Set(cfg)
			got := Get()
			assert.Equal(t, tt.config.Host, got.Server.Host)
			assert.Equal(t, tt.config.Port, got.Server.Port)
		})
	}
}

func TestDatabaseConfig(t *testing.T) {
	tests := []struct {
		name   string
		config DatabaseConfig
	}{
		{"sqlite", DatabaseConfig{Driver: "sqlite", Database: "test.db"}},
		{"postgres", DatabaseConfig{Driver: "postgres", Host: "localhost", Port: 5432, Database: "v2board"}},
		{"with credentials", DatabaseConfig{Driver: "postgres", Username: "user", Password: "pass"}},
		{"with pool settings", DatabaseConfig{Driver: "postgres", MaxIdleConns: 10, MaxOpenConns: 100}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Database: tt.config}
			Set(cfg)
			got := Get()
			assert.Equal(t, tt.config.Driver, got.Database.Driver)
		})
	}
}

func TestCacheConfig(t *testing.T) {
	tests := []struct {
		name   string
		config CacheConfig
	}{
		{"memory", CacheConfig{Driver: "memory"}},
		{"redis default", CacheConfig{Driver: "redis"}},
		{"redis custom", CacheConfig{Driver: "redis", RedisHost: "redis.local", RedisPort: 6380, RedisDB: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Cache: tt.config}
			Set(cfg)
			got := Get()
			assert.Equal(t, tt.config.Driver, got.Cache.Driver)
		})
	}
}

func TestJWTConfig(t *testing.T) {
	cfg := &Config{
		JWT: JWTConfig{
			Secret: "my-super-secret-key",
			Expire: 86400,
		},
	}
	Set(cfg)

	got := Get()
	assert.Equal(t, "my-super-secret-key", got.JWT.Secret)
	assert.Equal(t, 86400, got.JWT.Expire)
}

func TestAppConfig(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:             "TestApp",
			Version:          "1.0.0",
			APIToken:         "api-token-123",
			TrafficLogEnable: true,
			SubscribePath:    "s",
		},
	}
	Set(cfg)

	got := Get()
	assert.Equal(t, "TestApp", got.App.Name)
	assert.Equal(t, "1.0.0", got.App.Version)
	assert.Equal(t, "api-token-123", got.App.APIToken)
	assert.True(t, got.App.TrafficLogEnable)
	assert.Equal(t, "s", got.App.SubscribePath)
}

func TestTLSConfig(t *testing.T) {
	cfg := &Config{
		TLS: TLSConfig{
			Enable:   true,
			CertFile: "/path/to/cert.pem",
			KeyFile:  "/path/to/key.pem",
			Domain:   "example.com",
		},
	}
	Set(cfg)

	got := Get()
	assert.True(t, got.TLS.Enable)
	assert.Equal(t, "/path/to/cert.pem", got.TLS.CertFile)
	assert.Equal(t, "example.com", got.TLS.Domain)
}

func TestFrontendConfig(t *testing.T) {
	cfg := &Config{
		Frontend: FrontendConfig{
			Enable: true,
			Port:   3000,
			Path:   "web/public",
		},
	}
	Set(cfg)

	got := Get()
	assert.True(t, got.Frontend.Enable)
	assert.Equal(t, 3000, got.Frontend.Port)
	assert.Equal(t, "web/public", got.Frontend.Path)
}

func TestLogConfig(t *testing.T) {
	cfg := &Config{
		Log: LogConfig{
			Level:      "info",
			Output:     "stdout",
			FilePath:   "/var/log/app.log",
			MaxSize:    100,
			MaxBackups: 5,
			MaxAge:     30,
		},
	}
	Set(cfg)

	got := Get()
	assert.Equal(t, "info", got.Log.Level)
	assert.Equal(t, "stdout", got.Log.Output)
	assert.Equal(t, 100, got.Log.MaxSize)
}

// app.edition is gone; a file or variable that still sets it does not stop
// the start.
func TestRetiredAppEditionStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("app:\n  subscribe_path: s\n  edition: commercial\n"), 0o600))
	loaded, err := load(path, nil)
	require.NoError(t, err)
	require.Len(t, loaded.Deprecated.Keys(), 2)
	loaded, err = load("", []string{"ANIX_CONTROL_APP_EDITION=community"})
	require.NoError(t, err)
	require.Len(t, loaded.Deprecated.Keys(), 1)
	assert.Equal(t, "ANIX_CONTROL_APP_EDITION", loaded.Deprecated.Keys()[0].Source)
}

func TestForwardAnixOpsExperimentalDefaultsOff(t *testing.T) {
	loaded, err := load("", nil)
	require.NoError(t, err)
	require.False(t, loaded.Forward.AnixOpsExperimental)

	// The experimental engine stays off: the key is ignored.
	loaded, err = load("", []string{"ANIX_CONTROL_FORWARD_ANIXOPS_EXPERIMENTAL=true"})
	require.NoError(t, err)
	require.False(t, loaded.Forward.AnixOpsExperimental)
}

func TestAgentInstallConfigValidate(t *testing.T) {
	require.NoError(t, AgentInstallConfig{}.Validate())
	require.NoError(t, AgentInstallConfig{
		PublicURL: "https://ctl.example.com:8443", GRPCTarget: "[2001:db8::1]:50051", AgentVersion: "v4.2.0-rc.1",
		ArtifactDir: "/srv/agent", CNMirrorURL: "https://mirror.example.cn/anix-agent", SignatureFile: "/app/bootstrap/agent-install/agent-install.sh.sig",
	}.Validate())
	for name, broken := range map[string]AgentInstallConfig{
		"relative URL":     {PublicURL: "ctl.example.com"},
		"query":            {PublicURL: "https://ctl.example.com/?a=1"},
		"credentials":      {CNMirrorURL: "https://user:pw@mirror.example.cn"},
		"ftp":              {CNMirrorURL: "ftp://mirror.example.cn"},
		"http":             {PublicURL: "http://ctl.example.com"},
		"no port":          {GRPCTarget: "ctl.example.com"},
		"port range":       {GRPCTarget: "ctl.example.com:70000"},
		"version":          {AgentVersion: "latest"},
		"relative dir":     {ArtifactDir: "agent"},
		"relative sigfile": {SignatureFile: "install.sh.sig"},
	} {
		assert.Error(t, broken.Validate(), name)
	}
}
