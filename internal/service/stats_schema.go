package service

import (
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// EnsureStatsSchema creates the traffic/stats tables if they do not exist:
// v2_server_log, v2_online_log, v2_stat_user, v2_stat_server.
// Called unconditionally at startup (including production) because AutoMigrate only
// runs in dev/test. Idempotent and only touches these new tables, never existing ones.
func EnsureStatsSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	tables := []any{
		&model.TrafficLog{},
		&model.OnlineLog{},
		&model.StatUser{},
		&model.StatServer{},
	}
	for _, table := range tables {
		if db.Migrator().HasTable(table) {
			continue
		}
		if err := db.AutoMigrate(table); err != nil {
			return err
		}
	}
	return nil
}
