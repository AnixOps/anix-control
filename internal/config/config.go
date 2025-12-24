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
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Log      LogConfig      `yaml:"log"`
	JWT      JWTConfig      `yaml:"jwt"`
	App      AppConfig      `yaml:"app"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Mode         string `yaml:"mode"`
	ReadTimeout  int    `yaml:"read_timeout"`
	WriteTimeout int    `yaml:"write_timeout"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Driver          string `yaml:"driver"`           // sqlite, postgres
	Database        string `yaml:"database"`         // SQLite: 文件路径, PostgreSQL: 数据库名
	Host            string `yaml:"host"`             // PostgreSQL only
	Port            int    `yaml:"port"`             // PostgreSQL only
	Username        string `yaml:"username"`         // PostgreSQL only
	Password        string `yaml:"password"`         // PostgreSQL only
	LogLevel        string `yaml:"log_level"`        // silent, error, warn, info
	MaxIdleConns    int    `yaml:"max_idle_conns"`   // PostgreSQL only
	MaxOpenConns    int    `yaml:"max_open_conns"`   // PostgreSQL only
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"` // PostgreSQL only (seconds)
}

// RedisConfig Redis配置
type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
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
