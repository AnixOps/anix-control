package model

import "time"

// RealityTargetProbe is the latest probe of one candidate REALITY target
// (dest and SNI) from one proxy node (internal/realitytarget): the node's
// Agent ran reality.probe on it, and Control scored the answer. A node keeps
// one row per candidate; a new probe replaces it.
type RealityTargetProbe struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	NodeID     uint   `gorm:"not null;uniqueIndex:idx_v5_reality_target_probe_target,priority:1" json:"node_id"`
	ServerName string `gorm:"size:253;not null;uniqueIndex:idx_v5_reality_target_probe_target,priority:2" json:"server_name"`
	Port       int    `gorm:"not null;uniqueIndex:idx_v5_reality_target_probe_target,priority:3" json:"port"`
	// Verdict is the Agent's: ok when the target passes every hard
	// requirement, failed otherwise.
	Verdict string `gorm:"size:16;not null" json:"verdict"`
	// Score ranks the targets that passed (0 to 100); 0 for a failed one.
	Score int `gorm:"not null;default:0" json:"score"`
	// Result is the Agent's answer as JSON (realitytarget.ProbeResult).
	Result    string    `gorm:"type:text;not null" json:"-"`
	ProbedAt  time.Time `gorm:"not null" json:"probed_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (RealityTargetProbe) TableName() string { return "v5_reality_target_probe" }
