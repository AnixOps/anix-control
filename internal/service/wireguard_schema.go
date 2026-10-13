package service

import (
	"github.com/AnixOps/anix-control/v5/internal/model"
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

// EnsureNodeRuntimeHealthSchema adds the v2_node columns introduced after the
// table was first created: the runtime-health columns used by WireGuard relay
// supervision, and the node group columns of 5.0 (node_group_id,
// machine_fingerprint, approval, last_public_address; internal/nodegroup). It
// is kept separate because production startup intentionally does not run the
// complete application AutoMigrate set.
func EnsureNodeRuntimeHealthSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.AutoMigrate(&model.Node{})
}
