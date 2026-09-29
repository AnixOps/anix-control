package testutil

import (
	"gorm.io/gorm"
)

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
