package service

import (
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// EnsureStatsSchema creates or completes the traffic/stats tables:
// v2_server_log, v2_online_log, v2_stat_user, v2_stat_server.
// Called unconditionally at startup (including production) because AutoMigrate only
// runs in dev/test. Idempotent and limited to these tables; existing legacy tables
// are only expanded with missing columns/indexes.
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
		if err := db.AutoMigrate(table); err != nil {
			return err
		}
	}
	return nil
}
