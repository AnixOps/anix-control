package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetConfig 重置全局配置和 once (仅用于测试)
func resetConfig() {
	cfg = nil
	once = sync.Once{}
}

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

func TestLoad(t *testing.T) {
	resetConfig()

	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

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
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	loadedCfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, loadedCfg)

	assert.Equal(t, "development", loadedCfg.Env)
	assert.Equal(t, "127.0.0.1", loadedCfg.Server.Host)
	assert.Equal(t, 3000, loadedCfg.Server.Port)
	assert.Equal(t, "debug", loadedCfg.Server.Mode)
	assert.Equal(t, "sqlite", loadedCfg.Database.Driver)
	assert.Equal(t, "test.db", loadedCfg.Database.Database)
	assert.Equal(t, "test-secret", loadedCfg.JWT.Secret)
	assert.Equal(t, 3600, loadedCfg.JWT.Expire)
	assert.Equal(t, "TestApp", loadedCfg.App.Name)
}

func TestLoadAuthRegistrationConfig(t *testing.T) {
	resetConfig()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
env: development
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
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	loadedCfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, loadedCfg)
	require.NotNil(t, loadedCfg.Auth.Registration.Enabled)
	require.NotNil(t, loadedCfg.Auth.RegisterRateLimit.Enabled)

	assert.False(t, *loadedCfg.Auth.Registration.Enabled)
	assert.True(t, loadedCfg.Auth.Registration.RequireInvite)
	assert.Equal(t, []string{"example.com"}, loadedCfg.Auth.Registration.AllowedEmailDomains)
	assert.Equal(t, []string{"disposable.example"}, loadedCfg.Auth.Registration.BlockedEmailDomains)
	assert.True(t, *loadedCfg.Auth.RegisterRateLimit.Enabled)
	assert.Equal(t, 4, loadedCfg.Auth.RegisterRateLimit.MaxAttempts)
	assert.Equal(t, 1800, loadedCfg.Auth.RegisterRateLimit.WindowSeconds)
	assert.Equal(t, 3600, loadedCfg.Auth.RegisterRateLimit.LockoutSeconds)
}

func TestLoadForwardRuntimeConfig(t *testing.T) {
	resetConfig()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
env: development
forward_runtime:
  nodex_mode: true
  backend: gost
  jobs:
    poll_interval: 7s
    idle_poll_interval: 30s
    error_log_interval: 1m
    batch_size: 4
    timeout_seconds: 90
  gost_stats:
    poll_interval: 45s
    idle_poll_interval: 2m
    error_log_interval: 5m
  nodex:
    base_url: http://127.0.0.1:18081
    token: test-nodex-token
    timeout_seconds: 20
  clean_agent:
    public_url: https://panel.example.com
    heartbeat_interval_seconds: 11
    action_timeout_seconds: 121
    token_expire_seconds: 3600
    legacy_bridge_enabled: true
  iptables_ansible:
    inventory: config/deploy/ansible/inventory.ini
    apply_playbook: config/deploy/ansible/playbooks/forward_apply.yml
    remove_playbook: config/deploy/ansible/playbooks/forward_remove.yml
    working_dir: config/deploy/ansible
    target_pattern: "{{node.host}}"
    timeout_seconds: 120
    become: true
    extra_vars:
      retry: 3
    environment:
      ANSIBLE_CONFIG: config/deploy/ansible/ansible.cfg
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	loadedCfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, loadedCfg)
	require.NotNil(t, loadedCfg.ForwardRuntime.NodeXMode)

	assert.True(t, *loadedCfg.ForwardRuntime.NodeXMode)
	assert.Equal(t, "gost", loadedCfg.ForwardRuntime.Backend)
	assert.Equal(t, "7s", loadedCfg.ForwardRuntime.Jobs.PollInterval)
	assert.Equal(t, "30s", loadedCfg.ForwardRuntime.Jobs.IdlePollInterval)
	assert.Equal(t, "1m", loadedCfg.ForwardRuntime.Jobs.ErrorLogInterval)
	assert.Equal(t, 4, loadedCfg.ForwardRuntime.Jobs.BatchSize)
	assert.Equal(t, 90, loadedCfg.ForwardRuntime.Jobs.TimeoutSeconds)
	assert.Equal(t, "45s", loadedCfg.ForwardRuntime.GostStats.PollInterval)
	assert.Equal(t, "2m", loadedCfg.ForwardRuntime.GostStats.IdlePollInterval)
	assert.Equal(t, "5m", loadedCfg.ForwardRuntime.GostStats.ErrorLogInterval)
	assert.Equal(t, "http://127.0.0.1:18081", loadedCfg.ForwardRuntime.NodeX.BaseURL)
	assert.Equal(t, "test-nodex-token", loadedCfg.ForwardRuntime.NodeX.Token)
	assert.Equal(t, 20, loadedCfg.ForwardRuntime.NodeX.TimeoutSeconds)
	assert.Equal(t, "https://panel.example.com", loadedCfg.ForwardRuntime.CleanAgent.PublicURL)
	assert.Equal(t, 11, loadedCfg.ForwardRuntime.CleanAgent.HeartbeatIntervalSeconds)
	assert.Equal(t, 121, loadedCfg.ForwardRuntime.CleanAgent.ActionTimeoutSeconds)
	assert.Equal(t, 3600, loadedCfg.ForwardRuntime.CleanAgent.TokenExpireSeconds)
	assert.True(t, loadedCfg.ForwardRuntime.CleanAgent.LegacyBridgeEnabled)
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
			Name:    "V2Board",
			Version: "2.0.2-test.1",
		},
		TLS: TLSConfig{
			Enable: true,
			Domain: "example.com",
		},
		Admin: AdminConfig{
			Email:    "admin@example.com",
			Password: "password",
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
	assert.Equal(t, "admin@example.com", got.Admin.Email)
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
