// Package exitpolicy keeps what a proxy node refuses as an exit: with
// block_mainland on, the node's Agent has its kernel refuse connections to
// mainland-China destinations (the Agent ships the address list). It is
// off by default. The node's desired configuration carries the member
// "exit_policy" only while it is on, so other nodes keep their
// configuration hash, and the Agent reports what it refused on its
// heartbeat (MetricMainlandBlocked).
package exitpolicy

import (
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MetricMainlandBlocked is the Agent heartbeat metric: the connections its
// kernel refused to mainland-China destinations since the previous
// heartbeat.
const MetricMainlandBlocked = "agent_dataplane_mainland_blocked"

// Get answers a node's policy; a node without a row is off.
func Get(db *gorm.DB, nodeID uint) (model.NodeExitPolicy, error) {
	var row model.NodeExitPolicy
	err := db.Where("node_id = ?", nodeID).Limit(1).Find(&row).Error
	row.NodeID = nodeID
	return row, err
}

// Set turns the node's mainland block on or off; the counter stays.
func Set(db *gorm.DB, nodeID uint, block bool) (model.NodeExitPolicy, error) {
	row := model.NodeExitPolicy{NodeID: nodeID, BlockMainland: block, UpdatedAt: time.Now().UTC()}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"block_mainland", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return model.NodeExitPolicy{}, err
	}
	return Get(db, nodeID)
}

// NodeMember is the member "exit_policy" of the node's desired
// configuration: nil unless the block is on.
func NodeMember(db *gorm.DB, nodeID uint) (map[string]any, error) {
	row, err := Get(db, nodeID)
	if err != nil || !row.BlockMainland {
		return nil, err
	}
	return map[string]any{"block_mainland": true}, nil
}

// RecordBlocked adds n refused connections to a node whose block is on.
func RecordBlocked(db *gorm.DB, nodeID uint, n int64, now time.Time) error {
	return db.Model(&model.NodeExitPolicy{}).Where("node_id = ? AND block_mainland = ?", nodeID, true).
		Updates(map[string]any{"mainland_blocked_total": gorm.Expr("mainland_blocked_total + ?", n), "mainland_blocked_at": now.UTC()}).Error
}
