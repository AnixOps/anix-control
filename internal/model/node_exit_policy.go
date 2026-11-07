package model

import "time"

// NodeExitPolicy is what a proxy node refuses as an exit
// (internal/exitpolicy): with BlockMainland its Agent has the kernel refuse
// connections to mainland-China destinations. Off by default; a node without
// a row is off. MainlandBlockedTotal and MainlandBlockedAt count what the
// node's Agent reported refusing while it was on.
type NodeExitPolicy struct {
	NodeID               uint       `gorm:"primaryKey;autoIncrement:false" json:"node_id"`
	BlockMainland        bool       `gorm:"not null;default:false" json:"block_mainland"`
	MainlandBlockedTotal int64      `gorm:"not null;default:0" json:"mainland_blocked_total"`
	MainlandBlockedAt    *time.Time `json:"mainland_blocked_at"`
	UpdatedAt            time.Time  `gorm:"not null" json:"updated_at"`
}

func (NodeExitPolicy) TableName() string { return "v5_node_exit_policy" }
