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
	"github.com/glebarez/sqlite" // 绾疓o瀹炵幇鐨凷QLite椹卞姩锛屾棤闇€CGO
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db          *gorm.DB
	initialized bool
)

// Init 鍒濆鍖栨暟鎹簱杩炴帴
func Init(cfg *config.DatabaseConfig) error {
	// 濡傛灉宸茬粡鍒濆鍖栵紝鐩存帴杩斿洖
	if initialized && db != nil {
		return nil
	}

	var dialector gorm.Dialector
	var err error

	switch cfg.Driver {
	case "sqlite", "sqlite3", "":
		// SQLite 涓洪粯璁ゆ暟鎹簱
		dbPath := SQLitePath(cfg)

		// 纭繚鐩綍瀛樺湪
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

	// 閰嶇疆鏃ュ織绾у埆
	logLevel := logger.Info
	switch cfg.LogLevel {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	}

	gormLogger := logger.Default
	if logging.JSON() {
		// Same settings as logger.Default, written through the JSON handler.
		gormLogger = logger.New(log.New(logging.StdWriter(), "", 0), logger.Config{
			SlowThreshold: 200 * time.Millisecond,
			LogLevel:      logger.Warn,
		})
	}
	db, err = gorm.Open(dialector, &gorm.Config{
		Logger: gormLogger.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// 鍙湁 PostgreSQL 闇€瑕佽缃繛鎺ユ睜
	if cfg.Driver == "postgres" || cfg.Driver == "postgresql" {
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to get database instance: %w", err)
		}

		// 璁剧疆杩炴帴姹?
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

// Get 鑾峰彇鏁版嵁搴撳疄渚?
func Get() *gorm.DB {
	return db
}

// GetDB 鑾峰彇鏁版嵁搴撳疄渚?
func GetDB() *gorm.DB {
	return db
}

// Close 鍏抽棴鏁版嵁搴撹繛鎺?
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

// AutoMigrate 鑷姩杩佺Щ鏁版嵁搴撹〃
func AutoMigrate(models ...any) error {
	return db.AutoMigrate(models...)
}
