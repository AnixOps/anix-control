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

// Cleanup 清理所有数据
func (t *TestDB) Cleanup() {
	t.DB.Exec("DELETE FROM v2_user")
	t.DB.Exec("DELETE FROM v2_plan")
	t.DB.Exec("DELETE FROM v2_order")
	t.DB.Exec("DELETE FROM v2_node")
	t.DB.Exec("DELETE FROM v2_node_protocol")
	t.DB.Exec("DELETE FROM v2_payment")
	t.DB.Exec("DELETE FROM v2_payment_log")
	t.DB.Exec("DELETE FROM v2_ticket")
	t.DB.Exec("DELETE FROM v2_coupon")
	t.DB.Exec("DELETE FROM v2_knowledge")
	t.DB.Exec("DELETE FROM v2_subscription_group")
	t.DB.Exec("DELETE FROM v2_subscription_template")
	t.DB.Exec("DELETE FROM v2_authorized_key")
	t.DB.Exec("DELETE FROM v2_stat_user")
	t.DB.Exec("DELETE FROM v2_stat_server")
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