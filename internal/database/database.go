package database

import (
	"fmt"
	"github.com/AnixOps/anix-control/v4/internal/logging"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/glebarez/sqlite" // 纯Go实现的SQLite驱动，无需CGO
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db          *gorm.DB
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
		dbPath := SQLitePath(cfg)

		// 确保目录存在
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return fmt.Errorf("failed to create database directory: %w", err)
			}
		}

		dialector = sqlite.Open(sqliteConnectionString(dbPath))

	case "postgres", "postgresql":
		// PostgreSQL
		dialector = postgres.Open(PostgresDSN(cfg))

	default:
		return fmt.Errorf("unsupported database driver: %s (supported: sqlite, postgres)", cfg.Driver)
	}

	// 配置日志级别
	logLevel := logger.Info
	switch cfg.LogLevel {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	}

	// logger.Default's writer, or the JSON handler's.
	var sqlLogWriter logger.Writer = log.New(os.Stdout, "\r\n", log.LstdFlags)
	colorful := true
	if logging.JSON() {
		sqlLogWriter = log.New(logging.StdWriter(), "", 0)
		colorful = false
	}
	db, err = gorm.Open(dialector, &gorm.Config{
		Logger: newSQLLogger(sqlLogWriter, colorful, logLevel),
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

// newSQLLogger returns GORM's SQL logger: logger.Default's settings at the
// configured level. It logs each statement with its placeholders and never
// the bound values (ParameterizedQueries): those are node API keys, tokens,
// password hashes and subscription UUIDs, and database.log_level: info logs
// every statement.
func newSQLLogger(writer logger.Writer, colorful bool, level logger.LogLevel) logger.Interface {
	return logger.New(writer, logger.Config{
		SlowThreshold:        200 * time.Millisecond,
		LogLevel:             level,
		Colorful:             colorful,
		ParameterizedQueries: true,
	})
}

// DefaultSQLitePath is the SQLite database file used when none is configured.
const DefaultSQLitePath = "config/data/v2board.db"

// SQLitePath returns the configured SQLite database file.
func SQLitePath(cfg *config.DatabaseConfig) string {
	if cfg == nil || cfg.Database == "" {
		return DefaultSQLitePath
	}
	return cfg.Database
}

// PackageStorageSource returns what package storage derives package
// connections from: the kernel's PostgreSQL connection string, or the
// absolute path of the SQLite database file. It holds kernel credentials and
// must stay in the kernel.
func PackageStorageSource(cfg *config.DatabaseConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("database configuration is required")
	}
	switch cfg.Driver {
	case "postgres", "postgresql":
		return PostgresDSN(cfg), nil
	case "sqlite", "sqlite3", "":
		return filepath.Abs(SQLitePath(cfg))
	default:
		return "", fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
}

func sqliteConnectionString(databasePath string) string {
	databasePath = strings.TrimSpace(databasePath)
	separator := "?"
	if strings.Contains(databasePath, "?") {
		separator = "&"
	}
	dsn := databasePath + separator + "_pragma=busy_timeout(5000)"
	if databasePath == ":memory:" || strings.HasPrefix(databasePath, "file::memory:") {
		return dsn
	}
	return dsn + "&_pragma=journal_mode(WAL)"
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
func AutoMigrate(models ...any) error {
	return db.AutoMigrate(models...)
}
