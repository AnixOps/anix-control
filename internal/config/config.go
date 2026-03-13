package config

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	cfg  *Config
	once sync.Once
)

// Config 应用配置
type Config struct {
	Env      string         `yaml:"env"` // development, production
	Server   ServerConfig   `yaml:"server"`
	Frontend FrontendConfig `yaml:"frontend"`
	Database DatabaseConfig `yaml:"database"`
	Cache    CacheConfig    `yaml:"cache"`
	Log      LogConfig      `yaml:"log"`
	JWT      JWTConfig      `yaml:"jwt"`
	App      AppConfig      `yaml:"app"`
	Admin    AdminConfig    `yaml:"admin"`
	TLS      TLSConfig      `yaml:"tls"`
}

// TLSConfig TLS/HTTPS 配置
type TLSConfig struct {
	Enable   bool   `yaml:"enable"`    // 是否启用 TLS
	CertFile string `yaml:"cert_file"` // 证书文件路径
	KeyFile  string `yaml:"key_file"`  // 私钥文件路径
	Domain   string `yaml:"domain"`    // 域名 (用于生成订阅链接等)
}

// AdminConfig 默认管理员配置
type AdminConfig struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	Mode           string   `yaml:"mode"` // debug, release, test
	ReadTimeout    int      `yaml:"read_timeout"`
	WriteTimeout   int      `yaml:"write_timeout"`
	TrustedProxies []string `yaml:"trusted_proxies"` // 可信代理 IP 列表
}

// FrontendConfig 前端服务器配置
type FrontendConfig struct {
	Enable bool   `yaml:"enable"` // 是否启用前端服务
	Port   int    `yaml:"port"`   // 前端服务端口
	Path   string `yaml:"path"`   // 前端静态文件目录
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Driver          string `yaml:"driver"`            // sqlite, postgres
	Database        string `yaml:"database"`          // SQLite: 文件路径, PostgreSQL: 数据库名
	Host            string `yaml:"host"`              // PostgreSQL only
	Port            int    `yaml:"port"`              // PostgreSQL only
	Username        string `yaml:"username"`          // PostgreSQL only
	Password        string `yaml:"password"`          // PostgreSQL only
	LogLevel        string `yaml:"log_level"`         // silent, error, warn, info
	MaxIdleConns    int    `yaml:"max_idle_conns"`    // PostgreSQL only
	MaxOpenConns    int    `yaml:"max_open_conns"`    // PostgreSQL only
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"` // PostgreSQL only (seconds)
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Driver string `yaml:"driver"` // memory (默认), redis
	// Redis 专用配置 (使用 memory 时可忽略)
	RedisHost     string `yaml:"redis_host"`
	RedisPort     int    `yaml:"redis_port"`
	RedisPassword string `yaml:"redis_password"`
	RedisDB       int    `yaml:"redis_db"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `yaml:"level"`
	Output     string `yaml:"output"`
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret string `yaml:"secret"`
	Expire int    `yaml:"expire"`
}

// AppConfig 应用配置
type AppConfig struct {
	Name             string `yaml:"name"`
	Version          string `yaml:"version"`
	APIToken         string `yaml:"api_token"`
	TrafficLogEnable bool   `yaml:"traffic_log_enable"`
	SubscribePath    string `yaml:"subscribe_path"`
}

// Load 加载配置文件
func Load(path string) (*Config, error) {
	var err error
	once.Do(func() {
		var data []byte
		data, err = os.ReadFile(path)
		if err != nil {
			return
		}

		cfg = &Config{}
		err = yaml.Unmarshal(data, cfg)
	})

	return cfg, err
}

// Get 获取配置
func Get() *Config {
	return cfg
}

// Set 设置配置 (主要用于测试)
func Set(c *Config) {
	cfg = c
}
