package service

import (
	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

// EnsureForwardNodeMetricsPortColumn adds the metrics_port column to the existing
// v2_forward_node table if it does not exist yet. Called unconditionally at startup
// (including production) because AutoMigrate only runs in dev/test. It touches only
// this one column on an existing table, never anything else.
func EnsureForwardNodeMetricsPortColumn(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if db.Migrator().HasColumn(&model.ForwardNode{}, "metrics_port") {
		return nil
	}
	return db.Migrator().AddColumn(&model.ForwardNode{}, "MetricsPort")
}
