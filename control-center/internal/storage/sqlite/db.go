package sqlite

import (
	"fmt"
	"sync"

	"github.com/AnixOps/anix-control/control-center/internal/storage/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DB is the global database connection
var DB *gorm.DB
var mu sync.RWMutex

// Config holds database configuration
type Config struct {
	Driver   string
	Database string
	Host     string
	Port     int
	User     string
	Password string
}

// Init initializes the database connection
func Init(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	var err error

	switch cfg.Driver {
	case "sqlite":
		DB, err = gorm.Open(sqlite.Open(cfg.Database), &gorm.Config{})
	case "postgres":
		// TODO: Add postgres support
		return fmt.Errorf("postgres not yet supported")
	default:
		DB, err = gorm.Open(sqlite.Open(cfg.Database), &gorm.Config{})
	}

	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto migrate
	if err := autoMigrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}

// GetDB returns the database connection
func GetDB() *gorm.DB {
	mu.RLock()
	defer mu.RUnlock()
	return DB
}

// Close closes the database connection
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// autoMigrate runs auto migration
func autoMigrate() error {
	return DB.AutoMigrate(
		&models.User{},
		&models.APIToken{},
		&models.AuditLog{},
	)
}
