package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration
type Config struct {
	Env      string         `yaml:"env"`
	Server   ServerConfig   `yaml:"server"`
	GRPC     GRPCConfig     `yaml:"grpc"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Plugins  PluginsConfig  `yaml:"plugins"`
	Logging  LoggingConfig  `yaml:"logging"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // debug, release, test
}

// GRPCConfig holds gRPC server configuration
type GRPCConfig struct {
	Enable           bool   `yaml:"enable"`
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	KeepaliveTime    int    `yaml:"keepalive_time"`    // seconds
	KeepaliveTimeout int    `yaml:"keepalive_timeout"` // seconds
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver   string `yaml:"driver"`   // sqlite, postgres
	Database string `yaml:"database"` // for sqlite: file path, for postgres: database name
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Provider string           `yaml:"provider"` // local, oauth, ldap, saml
	JWT      JWTConfig        `yaml:"jwt"`
	Local    *LocalAuthConfig `yaml:"local,omitempty"`
	OAuth    *OAuthConfig     `yaml:"oauth,omitempty"`
	LDAP     *LDAPConfig      `yaml:"ldap,omitempty"`
	SAML     *SAMLConfig      `yaml:"saml,omitempty"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret        string `yaml:"secret"`
	Expire        int    `yaml:"expire"`         // seconds
	RefreshExpire int    `yaml:"refresh_expire"` // seconds
	Issuer        string `yaml:"issuer"`
}

// LocalAuthConfig holds local authentication configuration
type LocalAuthConfig struct {
	AdminEmail    string `yaml:"admin_email"`
	AdminPassword string `yaml:"admin_password"`
}

// OAuthConfig holds OAuth configuration
type OAuthConfig struct {
	Providers []OAuthProvider `yaml:"providers"`
}

// OAuthProvider holds a single OAuth provider configuration
type OAuthProvider struct {
	Name         string   `yaml:"name"` // google, github, gitlab, custom
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURL  string   `yaml:"redirect_url"`
	Scopes       []string `yaml:"scopes"`
	AuthURL      string   `yaml:"auth_url"`      // for custom providers
	TokenURL     string   `yaml:"token_url"`     // for custom providers
	UserInfoURL  string   `yaml:"user_info_url"` // for custom providers
}

// LDAPConfig holds LDAP configuration
type LDAPConfig struct {
	URL          string `yaml:"url"`     // ldap://server:389
	BindDN       string `yaml:"bind_dn"` // cn=admin,dc=example,dc=com
	BindPassword string `yaml:"bind_password"`
	BaseDN       string `yaml:"base_dn"` // ou=users,dc=example,dc=com
	Filter       string `yaml:"filter"`  // (uid=%s)
	TLS          bool   `yaml:"tls"`
	Insecure     bool   `yaml:"insecure"` // skip certificate verification
}

// SAMLConfig holds SAML configuration
type SAMLConfig struct {
	EntityID        string `yaml:"entity_id"`
	IDPMetadataURL  string `yaml:"idp_metadata_url"`
	IDPMetadataFile string `yaml:"idp_metadata_file"`
	CertificateFile string `yaml:"certificate_file"`
	KeyFile         string `yaml:"key_file"`
}

// PluginsConfig holds plugin configurations
type PluginsConfig struct {
	Ansible map[string]interface{} `yaml:"ansible"`
	V2board map[string]interface{} `yaml:"v2board"`
	V2bX    map[string]interface{} `yaml:"v2bx"`
	Agent   map[string]interface{} `yaml:"agent"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, text
	Output string `yaml:"output"` // stdout, stderr, file path
}

// Load loads configuration from a file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	setDefaults(&cfg)

	return &cfg, nil
}

// LoadFromString loads configuration from a YAML string
func LoadFromString(yamlStr string) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	setDefaults(&cfg)
	return &cfg, nil
}

// Save saves configuration to a file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// setDefaults sets default values for configuration
func setDefaults(cfg *Config) {
	if cfg.Env == "" {
		cfg.Env = "development"
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "debug"
	}
	if cfg.GRPC.Port == 0 {
		cfg.GRPC.Port = 50052
	}
	if cfg.GRPC.KeepaliveTime == 0 {
		cfg.GRPC.KeepaliveTime = 30
	}
	if cfg.GRPC.KeepaliveTimeout == 0 {
		cfg.GRPC.KeepaliveTimeout = 10
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.Database.Database == "" {
		cfg.Database.Database = "data/anixops.db"
	}
	if cfg.Auth.Provider == "" {
		cfg.Auth.Provider = "local"
	}
	if cfg.Auth.JWT.Expire == 0 {
		cfg.Auth.JWT.Expire = 86400 // 24 hours
	}
	if cfg.Auth.JWT.RefreshExpire == 0 {
		cfg.Auth.JWT.RefreshExpire = 604800 // 7 days
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "text"
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = "stdout"
	}
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	cfg := &Config{}
	setDefaults(cfg)
	return cfg
}
