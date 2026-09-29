package config_test

import (
	"os"
	"testing"

	"github.com/AnixOps/anix-control/control-center/internal/core/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Check defaults are set
	if cfg.Env != "development" {
		t.Errorf("expected env 'development', got '%s'", cfg.Env)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected server host '0.0.0.0', got '%s'", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Database.Driver != "sqlite" {
		t.Errorf("expected database driver 'sqlite', got '%s'", cfg.Database.Driver)
	}

	if cfg.Auth.Provider != "local" {
		t.Errorf("expected auth provider 'local', got '%s'", cfg.Auth.Provider)
	}
}

func TestLoadFromString(t *testing.T) {
	yaml := `
env: "production"
server:
  host: "127.0.0.1"
  port: 9000
database:
  driver: "postgres"
  host: "localhost"
  port: 5432
`

	cfg, err := config.LoadFromString(yaml)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Env != "production" {
		t.Errorf("expected env 'production', got '%s'", cfg.Env)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected server host '127.0.0.1', got '%s'", cfg.Server.Host)
	}

	if cfg.Server.Port != 9000 {
		t.Errorf("expected server port 9000, got %d", cfg.Server.Port)
	}

	if cfg.Database.Driver != "postgres" {
		t.Errorf("expected database driver 'postgres', got '%s'", cfg.Database.Driver)
	}
}

func TestLoadFromString_InvalidYAML(t *testing.T) {
	invalidYaml := `
env: "production
  invalid indentation
`

	_, err := config.LoadFromString(invalidYaml)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestSetDefaults(t *testing.T) {
	// Call internal defaults function through LoadFromString with empty config

	emptyYaml := ``
	loaded, err := config.LoadFromString(emptyYaml)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check that defaults are applied
	if loaded.Server.Port == 0 {
		t.Error("expected default server port to be set")
	}

	if loaded.Auth.JWT.Expire == 0 {
		t.Error("expected default JWT expire to be set")
	}
}

func TestJWTConfigDefaults(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Auth.JWT.Expire != 86400 {
		t.Errorf("expected JWT expire 86400, got %d", cfg.Auth.JWT.Expire)
	}

	if cfg.Auth.JWT.RefreshExpire != 604800 {
		t.Errorf("expected JWT refresh expire 604800, got %d", cfg.Auth.JWT.RefreshExpire)
	}
}

func TestGRPCConfigDefaults(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.GRPC.Port != 50052 {
		t.Errorf("expected gRPC port 50052, got %d", cfg.GRPC.Port)
	}

	if cfg.GRPC.KeepaliveTime != 30 {
		t.Errorf("expected keepalive time 30, got %d", cfg.GRPC.KeepaliveTime)
	}
}

func TestLoggingConfigDefaults(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Logging.Level != "info" {
		t.Errorf("expected logging level 'info', got '%s'", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "text" {
		t.Errorf("expected logging format 'text', got '%s'", cfg.Logging.Format)
	}
}

func TestConfig_Save(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Env = "test-save"

	// Create temp file
	tmpFile := "/tmp/test-config-save.yaml"
	defer os.Remove(tmpFile)

	err := cfg.Save(tmpFile)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("config file not created")
	}

	// Load and verify
	loaded, err := config.Load(tmpFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Env != "test-save" {
		t.Errorf("expected env 'test-save', got '%s'", loaded.Env)
	}
}

func TestConfig_Save_InvalidPath(t *testing.T) {
	cfg := config.DefaultConfig()

	// Try to save to a non-existent directory
	err := cfg.Save("/nonexistent/directory/config.yaml")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := config.Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestServerConfig(t *testing.T) {
	cfg := config.ServerConfig{
		Host: "localhost",
		Port: 3000,
		Mode: "release",
	}

	if cfg.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", cfg.Host)
	}
	if cfg.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Port)
	}
	if cfg.Mode != "release" {
		t.Errorf("expected mode 'release', got '%s'", cfg.Mode)
	}
}

func TestGRPCConfig(t *testing.T) {
	cfg := config.GRPCConfig{
		Enable:           true,
		Host:             "0.0.0.0",
		Port:             50051,
		KeepaliveTime:    60,
		KeepaliveTimeout: 20,
	}

	if !cfg.Enable {
		t.Error("expected Enable to be true")
	}
	if cfg.Port != 50051 {
		t.Errorf("expected port 50051, got %d", cfg.Port)
	}
}

func TestDatabaseConfig(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "postgres",
		Database: "anixops",
		Host:     "db.example.com",
		Port:     5432,
		User:     "admin",
		Password: "secret",
	}

	if cfg.Driver != "postgres" {
		t.Errorf("expected driver 'postgres', got '%s'", cfg.Driver)
	}
	if cfg.Host != "db.example.com" {
		t.Errorf("expected host 'db.example.com', got '%s'", cfg.Host)
	}
}

func TestAuthConfig(t *testing.T) {
	cfg := config.AuthConfig{
		Provider: "oauth",
		JWT: config.JWTConfig{
			Secret:        "test-secret",
			Expire:        3600,
			RefreshExpire: 86400,
			Issuer:        "test",
		},
	}

	if cfg.Provider != "oauth" {
		t.Errorf("expected provider 'oauth', got '%s'", cfg.Provider)
	}
	if cfg.JWT.Secret != "test-secret" {
		t.Errorf("expected JWT secret 'test-secret', got '%s'", cfg.JWT.Secret)
	}
}

func TestJWTConfig(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:        "my-secret",
		Expire:        7200,
		RefreshExpire: 1209600,
		Issuer:        "anixops",
	}

	if cfg.Secret != "my-secret" {
		t.Errorf("expected secret 'my-secret', got '%s'", cfg.Secret)
	}
	if cfg.Expire != 7200 {
		t.Errorf("expected expire 7200, got %d", cfg.Expire)
	}
}

func TestLocalAuthConfig(t *testing.T) {
	cfg := config.LocalAuthConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "password123",
	}

	if cfg.AdminEmail != "admin@example.com" {
		t.Errorf("expected email 'admin@example.com', got '%s'", cfg.AdminEmail)
	}
}

func TestOAuthConfig(t *testing.T) {
	cfg := config.OAuthConfig{
		Providers: []config.OAuthProvider{
			{
				Name:         "github",
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				RedirectURL:  "http://localhost/callback",
				Scopes:       []string{"user:email"},
			},
		},
	}

	if len(cfg.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "github" {
		t.Errorf("expected provider name 'github', got '%s'", cfg.Providers[0].Name)
	}
}

func TestOAuthProvider(t *testing.T) {
	provider := config.OAuthProvider{
		Name:         "google",
		ClientID:     "google-client-id",
		ClientSecret: "google-secret",
		RedirectURL:  "http://localhost/oauth/callback",
		Scopes:       []string{"email", "profile"},
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
	}

	if provider.Name != "google" {
		t.Errorf("expected name 'google', got '%s'", provider.Name)
	}
}

func TestLDAPConfig(t *testing.T) {
	cfg := config.LDAPConfig{
		URL:          "ldap://localhost:389",
		BindDN:       "cn=admin,dc=example,dc=com",
		BindPassword: "admin-password",
		BaseDN:       "ou=users,dc=example,dc=com",
		Filter:       "(uid=%s)",
		TLS:          true,
		Insecure:     false,
	}

	if cfg.URL != "ldap://localhost:389" {
		t.Errorf("expected URL 'ldap://localhost:389', got '%s'", cfg.URL)
	}
}

func TestSAMLConfig(t *testing.T) {
	cfg := config.SAMLConfig{
		EntityID:        "anixops",
		IDPMetadataURL:  "https://idp.example.com/metadata",
		IDPMetadataFile: "/etc/anixops/idp-metadata.xml",
		CertificateFile: "/etc/anixops/cert.pem",
		KeyFile:         "/etc/anixops/key.pem",
	}

	if cfg.EntityID != "anixops" {
		t.Errorf("expected EntityID 'anixops', got '%s'", cfg.EntityID)
	}
}

func TestPluginsConfig(t *testing.T) {
	cfg := config.PluginsConfig{
		Ansible: map[string]interface{}{"playbook_dir": "/playbooks"},
		V2board: map[string]interface{}{"host": "http://localhost"},
		V2bX:    map[string]interface{}{"nodes": []string{"node1"}},
		Agent:   map[string]interface{}{"enabled": true},
	}

	if cfg.Ansible["playbook_dir"] != "/playbooks" {
		t.Error("expected Ansible config")
	}
}

func TestLoggingConfig(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "/var/log/anixops.log",
	}

	if cfg.Level != "debug" {
		t.Errorf("expected level 'debug', got '%s'", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Errorf("expected format 'json', got '%s'", cfg.Format)
	}
}

func TestLoadFromString_WithAllSections(t *testing.T) {
	yaml := `
env: "staging"
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"
grpc:
  enable: true
  host: "0.0.0.0"
  port: 50051
  keepalive_time: 60
  keepalive_timeout: 20
database:
  driver: "postgres"
  database: "anixops_staging"
  host: "db.staging.example.com"
  port: 5432
  user: "anixops"
  password: "staging-password"
auth:
  provider: "oauth"
  jwt:
    secret: "staging-secret"
    expire: 86400
    refresh_expire: 604800
    issuer: "anixops-staging"
logging:
  level: "info"
  format: "json"
  output: "stdout"
`

	cfg, err := config.LoadFromString(yaml)
	if err != nil {
		t.Fatalf("LoadFromString failed: %v", err)
	}

	if cfg.Env != "staging" {
		t.Errorf("expected env 'staging', got '%s'", cfg.Env)
	}
	if cfg.Server.Mode != "release" {
		t.Errorf("expected server mode 'release', got '%s'", cfg.Server.Mode)
	}
	if !cfg.GRPC.Enable {
		t.Error("expected gRPC to be enabled")
	}
	if cfg.Database.Driver != "postgres" {
		t.Errorf("expected database driver 'postgres', got '%s'", cfg.Database.Driver)
	}
	if cfg.Auth.Provider != "oauth" {
		t.Errorf("expected auth provider 'oauth', got '%s'", cfg.Auth.Provider)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected logging format 'json', got '%s'", cfg.Logging.Format)
	}
}
