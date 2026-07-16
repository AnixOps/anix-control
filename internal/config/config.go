package config

import (
	"os"
	"strings"
	"sync"

	"github.com/AnixOps/anix-control/v3/internal/branding"
	"gopkg.in/yaml.v3"
)

var (
	cfg  *Config
	once sync.Once
)

// Config is the root application configuration loaded from config.yaml.
type Config struct {
	Env            string               `yaml:"env"`
	Server         ServerConfig         `yaml:"server"`
	Frontend       FrontendConfig       `yaml:"frontend"`
	Database       DatabaseConfig       `yaml:"database"`
	Cache          CacheConfig          `yaml:"cache"`
	Log            LogConfig            `yaml:"log"`
	JWT            JWTConfig            `yaml:"jwt"`
	Auth           AuthConfig           `yaml:"auth"`
	App            AppConfig            `yaml:"app"`
	Admin          AdminConfig          `yaml:"admin"`
	TLS            TLSConfig            `yaml:"tls"`
	ForwardRuntime ForwardRuntimeConfig `yaml:"forward_runtime"`
	Plugins        PluginConfig         `yaml:"plugins"`
	GRPC           GRPCConfig           `yaml:"grpc"`
}

// PluginConfig controls the official plugin trust root and the opt-in durable
// lifecycle dispatcher. Third-party plugin execution is intentionally
// unsupported by the control kernel.
type PluginConfig struct {
	OfficialPublicKey       string `yaml:"official_public_key"`
	ControlExecutionEnabled bool   `yaml:"control_execution_enabled"`
	ControlPollInterval     string `yaml:"control_poll_interval"`
	DispatchEnabled         bool   `yaml:"dispatch_enabled"`
	DispatchPollInterval    string `yaml:"dispatch_poll_interval"`
}

// GRPCConfig controls the node-facing gRPC server (AnixOps Agent nodes connect here).
type GRPCConfig struct {
	Enable      bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	APIToken    string `yaml:"api_token"`
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
}

// AuthConfig defines authentication security settings.
type AuthConfig struct {
	LoginRateLimit    LoginRateLimitConfig   `yaml:"login_rate_limit"`
	RegisterRateLimit LoginRateLimitConfig   `yaml:"register_rate_limit"`
	Registration      RegistrationAuthConfig `yaml:"registration"`
}

// LoginRateLimitConfig controls brute-force protection for login.
type LoginRateLimitConfig struct {
	Enabled        *bool `yaml:"enabled"`
	MaxAttempts    int   `yaml:"max_attempts"`
	WindowSeconds  int   `yaml:"window_seconds"`
	LockoutSeconds int   `yaml:"lockout_seconds"`
}

// RegistrationAuthConfig controls public account registration policy.
type RegistrationAuthConfig struct {
	Enabled             *bool    `yaml:"enabled"`
	RequireInvite       bool     `yaml:"require_invite"`
	AllowedEmailDomains []string `yaml:"allowed_email_domains"`
	BlockedEmailDomains []string `yaml:"blocked_email_domains"`
}

// ForwardRuntimeConfig stores the canonical forward runtime settings from config.yaml.
type ForwardRuntimeConfig struct {
	NodeXMode       *bool                          `yaml:"nodex_mode"`
	Backend         string                         `yaml:"backend"`
	NodeX           ForwardRuntimeNodeXConfig      `yaml:"nodex"`
	CleanAgent      ForwardRuntimeCleanAgentConfig `yaml:"clean_agent"`
	NftablesAnsible ForwardRuntimeAnsibleConfig    `yaml:"nftables_ansible"`
	Jobs            ForwardRuntimeJobsConfig       `yaml:"jobs"`
	GostStats       ForwardRuntimeGostStatsConfig  `yaml:"gost_stats"`
	Latency         ForwardRuntimeLatencyConfig    `yaml:"latency"`
}

// ForwardRuntimeNodeXConfig stores NodeX or gost runtime settings.
type ForwardRuntimeNodeXConfig struct {
	BaseURL        string `yaml:"base_url"`
	Token          string `yaml:"token"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// ForwardRuntimeCleanAgentConfig stores clean-room forward agent defaults.
type ForwardRuntimeCleanAgentConfig struct {
	PublicURL                string `yaml:"public_url"`
	HeartbeatIntervalSeconds int    `yaml:"heartbeat_interval_seconds"`
	ActionTimeoutSeconds     int    `yaml:"action_timeout_seconds"`
	TokenExpireSeconds       int    `yaml:"token_expire_seconds"`
	LegacyBridgeEnabled      bool   `yaml:"legacy_bridge_enabled"`
}

// ForwardRuntimeAnsibleConfig stores local ansible runtime settings.
type ForwardRuntimeAnsibleConfig struct {
	Inventory      string            `yaml:"inventory"`
	ApplyPlaybook  string            `yaml:"apply_playbook"`
	RemovePlaybook string            `yaml:"remove_playbook"`
	Become         bool              `yaml:"become"`
	ExtraVars      map[string]any    `yaml:"extra_vars"`
	Command        string            `yaml:"command"`
	WorkingDir     string            `yaml:"working_dir"`
	TargetPattern  string            `yaml:"target_pattern"`
	Environment    map[string]string `yaml:"environment"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
}

// ForwardRuntimeJobsConfig stores local ansible job executor polling settings.
type ForwardRuntimeJobsConfig struct {
	PollInterval     string `yaml:"poll_interval"`
	IdlePollInterval string `yaml:"idle_poll_interval"`
	ErrorLogInterval string `yaml:"error_log_interval"`
	BatchSize        int    `yaml:"batch_size"`
	TimeoutSeconds   int    `yaml:"timeout_seconds"`
}

// ForwardRuntimeGostStatsConfig stores gost stats polling settings.
type ForwardRuntimeGostStatsConfig struct {
	PollInterval     string `yaml:"poll_interval"`
	IdlePollInterval string `yaml:"idle_poll_interval"`
	ErrorLogInterval string `yaml:"error_log_interval"`
}

// ForwardRuntimeLatencyConfig stores latency prober (TCPing) settings.
type ForwardRuntimeLatencyConfig struct {
	PollInterval     string `yaml:"poll_interval"`
	IdlePollInterval string `yaml:"idle_poll_interval"`
	ErrorLogInterval string `yaml:"error_log_interval"`
	Dials            int    `yaml:"dials"`
	DialTimeout      string `yaml:"dial_timeout"`
	Concurrency      int    `yaml:"concurrency"`
	RetentionDays    int    `yaml:"retention_days"`
}

// TLSConfig defines TLS settings.
type TLSConfig struct {
	Enable   bool   `yaml:"enable"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	Domain   string `yaml:"domain"`
}

// AdminConfig defines the bootstrap admin account.
type AdminConfig struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

// ServerConfig defines backend server settings.
type ServerConfig struct {
	Host           string     `yaml:"host"`
	Port           int        `yaml:"port"`
	Mode           string     `yaml:"mode"`
	ReadTimeout    int        `yaml:"read_timeout"`
	WriteTimeout   int        `yaml:"write_timeout"`
	TrustedProxies []string   `yaml:"trusted_proxies"`
	CORS           CORSConfig `yaml:"cors"`
}

// CORSConfig defines Cross-Origin Resource Sharing settings.
type CORSConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"` // preflight cache duration in seconds
}

// FrontendConfig defines static frontend serving settings.
type FrontendConfig struct {
	Enable bool   `yaml:"enable"`
	Port   int    `yaml:"port"`
	Path   string `yaml:"path"`
}

// DatabaseConfig defines database connection settings.
type DatabaseConfig struct {
	Driver          string `yaml:"driver"`
	Database        string `yaml:"database"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Username        string `yaml:"username"`
	Password        string `yaml:"password"`
	LogLevel        string `yaml:"log_level"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
}

// CacheConfig defines cache settings.
type CacheConfig struct {
	Driver        string `yaml:"driver"`
	RedisHost     string `yaml:"redis_host"`
	RedisPort     int    `yaml:"redis_port"`
	RedisPassword string `yaml:"redis_password"`
	RedisDB       int    `yaml:"redis_db"`
}

// LogConfig defines log settings.
type LogConfig struct {
	Level      string `yaml:"level"`
	Output     string `yaml:"output"`
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

// JWTConfig defines JWT settings.
type JWTConfig struct {
	Secret string `yaml:"secret"`
	Expire int    `yaml:"expire"`
}

// AppConfig defines generic app settings.
type AppConfig struct {
	Name             string `yaml:"name"`
	Version          string `yaml:"version"`
	APIToken         string `yaml:"api_token"`
	TrafficLogEnable bool   `yaml:"traffic_log_enable"`
	SubscribePath    string `yaml:"subscribe_path"`
}

// Load reads a YAML config file once per process.
func Load(path string) (*Config, error) {
	var err error
	once.Do(func() {
		var data []byte
		data, err = os.ReadFile(path) // #nosec G304 -- config path is supplied by the operator or test harness, not by an HTTP user.
		if err != nil {
			return
		}

		cfg = &Config{}
		err = yaml.Unmarshal(data, cfg)
		if err == nil {
			normalizeBrandDefaults(cfg)
		}
	})

	return cfg, err
}

// Get returns the loaded config.
func Get() *Config {
	return cfg
}

// Set replaces the loaded config, mainly for tests.
func Set(c *Config) {
	normalizeBrandDefaults(c)
	cfg = c
}

func normalizeBrandDefaults(c *Config) {
	if c == nil {
		return
	}
	if strings.TrimSpace(c.App.Name) == "" {
		c.App.Name = branding.ControlName
	}
}
