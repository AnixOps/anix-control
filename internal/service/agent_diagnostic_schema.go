package service

import (
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// EnsureAgentDiagnosticTaskSchema creates the diagnostic task table if it does not
// exist. Called unconditionally at startup (including production) because AutoMigrate
// only runs in dev/test. It touches only the new table, never existing ones.
func EnsureAgentDiagnosticTaskSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if db.Migrator().HasTable(&model.AgentDiagnosticTask{}) {
		return nil
	}
	return db.AutoMigrate(&model.AgentDiagnosticTask{})
}
