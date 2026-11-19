package model

import "time"

// ReachVantage marks a proxy node whose Agent probes the other nodes'
// reachability from its region (internal/reachprobe): domestic or overseas.
type ReachVantage struct {
	NodeID    uint      `gorm:"primaryKey;autoIncrement:false" json:"node_id"`
	Region    string    `gorm:"size:16;not null" json:"region"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (ReachVantage) TableName() string { return "v5_reach_vantage" }

// ReachProbe is the latest reach.probe answer of one vantage about one
// target node; a new probe replaces it.
type ReachProbe struct {
	VantageNodeID uint   `gorm:"primaryKey;autoIncrement:false" json:"vantage_node_id"`
	TargetNodeID  uint   `gorm:"primaryKey;autoIncrement:false" json:"target_node_id"`
	Region        string `gorm:"size:16;not null" json:"region"`
	Reachable     bool   `gorm:"not null;default:false" json:"reachable"`
	// Result is the Agent's answer for the target as JSON
	// (reachprobe.Target).
	Result   string    `gorm:"type:text;not null" json:"-"`
	ProbedAt time.Time `gorm:"not null" json:"probed_at"`
}

func (ReachProbe) TableName() string { return "v5_reach_probe" }
