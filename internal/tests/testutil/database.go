package testutil

import (
	"github.com/anixops/v2board/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestDB 测试数据库包装器
type TestDB struct {
	DB *gorm.DB
}

// SetupTestDB 创建内存 SQLite 测试数据库
func SetupTestDB() *TestDB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to connect test database: " + err.Error())
	}

	// 注意: 调用者需要在测试开始时调用 database.Init() 或使用此数据库
	return &TestDB{DB: db}
}

// Close 关闭测试数据库
func (t *TestDB) Close() {
	sqlDB, _ := t.DB.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}
}

// Cleanup 清理所有表数据，自动发现所有表并重置自增计数器。
// 不需要手动维护表名列表。
func (t *TestDB) Cleanup() {
	CleanupDB(t.DB)
}

// CleanupDB 清理指定 GORM 连接中的所有表数据。
// handler_test.go 和 service_test.go 可直接调用此函数。
func CleanupDB(db *gorm.DB) {
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return
	}
	for _, table := range tables {
		db.Exec("DELETE FROM " + table)
	}
	// 重置自增计数器，确保测试数据 ID 可预测
	db.Exec("DELETE FROM sqlite_sequence")
}

// TestConfig 创建测试配置
func TestConfig() *config.Config {
	return &config.Config{
		Env: "test",
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: "debug",
		},
		Database: config.DatabaseConfig{
			Driver:   "sqlite",
			Database: ":memory:",
		},
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret-key-for-testing",
			Expire: 86400,
		},
		App: config.AppConfig{
			Name:          "V2Board Test",
			Version:       "test",
			APIToken:      "test-api-token",
			SubscribePath: "s",
		},
	}
}