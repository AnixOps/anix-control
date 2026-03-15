package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/glebarez/sqlite" // 纯Go实现的SQLite驱动，无需CGO
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db         *gorm.DB
	initialized bool
)

// Init 初始化数据库连接
func Init(cfg *config.DatabaseConfig) error {
	// 如果已经初始化，直接返回
	if initialized && db != nil {
		return nil
	}

	var dialector gorm.Dialector
	var err error

	switch cfg.Driver {
	case "sqlite", "sqlite3", "":
		// SQLite 为默认数据库
		dbPath := cfg.Database
		if dbPath == "" {
			dbPath = "data/v2board.db"
		}

		// 确保目录存在
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create database directory: %w", err)
			}
		}

		dialector = sqlite.Open(dbPath)

	case "postgres", "postgresql":
		// PostgreSQL
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
			cfg.Host,
			cfg.Port,
			cfg.Username,
			cfg.Password,
			cfg.Database,
		)
		dialector = postgres.Open(dsn)

	default:
		return fmt.Errorf("unsupported database driver: %s (supported: sqlite, postgres)", cfg.Driver)
	}

	// 配置日志级别
	logLevel := logger.Info
	if cfg.LogLevel == "silent" {
		logLevel = logger.Silent
	} else if cfg.LogLevel == "error" {
		logLevel = logger.Error
	} else if cfg.LogLevel == "warn" {
		logLevel = logger.Warn
	}

	db, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// 只有 PostgreSQL 需要设置连接池
	if cfg.Driver == "postgres" || cfg.Driver == "postgresql" {
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to get database instance: %w", err)
		}

		// 设置连接池
		if cfg.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
		}
		if cfg.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
		}
		if cfg.ConnMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
		}
	}

	initialized = true
	return nil
}

// Get 获取数据库实例
func Get() *gorm.DB {
	return db
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return db
}

// Close 关闭数据库连接
func Close() error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	if err := sqlDB.Close(); err != nil {
		return err
	}
	db = nil
	initialized = false
	return nil
}

// Reset resets the database state (for testing only)
func Reset() {
	db = nil
	initialized = false
}

// AutoMigrate 自动迁移数据库表
func AutoMigrate(models ...interface{}) error {
	return db.AutoMigrate(models...)
}

// IsSQLite 检查是否使用 SQLite
func IsSQLite() bool {
	return db.Dialector.Name() == "sqlite"
}

// IsPostgres 检查是否使用 PostgreSQL
func IsPostgres() bool {
	return db.Dialector.Name() == "postgres"
}
