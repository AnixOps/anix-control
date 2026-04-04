package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anixops/v2board/internal/config"
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
		dbPath := cfg.Database
		if dbPath == "" {
			dbPath = "config/data/v2board.db"
		}

		// 纭繚鐩綍瀛樺湪
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

	// 閰嶇疆鏃ュ織绾у埆
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
func AutoMigrate(models ...interface{}) error {
	return db.AutoMigrate(models...)
}

// IsSQLite 妫€鏌ユ槸鍚︿娇鐢?SQLite
func IsSQLite() bool {
	return db.Dialector.Name() == "sqlite"
}

// IsPostgres 妫€鏌ユ槸鍚︿娇鐢?PostgreSQL
func IsPostgres() bool {
	return db.Dialector.Name() == "postgres"
}
