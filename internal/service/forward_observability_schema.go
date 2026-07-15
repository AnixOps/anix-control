package service

import (
	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

// EnsureObservabilitySchema creates the latency bucket table if it does not exist.
// Called unconditionally at startup (including production) because AutoMigrate only
// runs in dev/test. It touches only the new observability table, never existing tables.
func EnsureObservabilitySchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if db.Migrator().HasTable(&model.ForwardLatencyBucket{}) {
		return nil
	}
	return db.AutoMigrate(&model.ForwardLatencyBucket{})
}
