package service

import (
	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

// EnsureWireGuardPeerSchema creates or upgrades only the WireGuard peer table.
// The server intentionally skips the full AutoMigrate set in production, so
// newly introduced runtime tables need an explicit idempotent schema hook.
func EnsureWireGuardPeerSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.AutoMigrate(&model.WireGuardPeer{})
}

// EnsureNodeRuntimeHealthSchema adds the runtime-health columns used by
// WireGuard relay supervision. It is kept separate because production startup
// intentionally does not run the complete application AutoMigrate set.
func EnsureNodeRuntimeHealthSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.AutoMigrate(&model.Node{})
}
