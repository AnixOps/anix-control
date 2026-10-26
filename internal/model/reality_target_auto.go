package model

import "time"

// RealityTargetAuto is one proxy node's automatic REALITY target selection
// (internal/realitytarget, AutoWorker): when Enabled, Control probes the
// node's targets every few hours and moves a Reality protocol whose target
// failed twice in a row to the recommended one.
type RealityTargetAuto struct {
	NodeID  uint `gorm:"primaryKey;autoIncrement:false" json:"node_id"`
	Enabled bool `gorm:"not null;default:false" json:"enabled"`
	// NextRunAt is when the worker probes the node next; nil is now.
	NextRunAt *time.Time `json:"next_run_at"`
	LastRunAt *time.Time `json:"last_run_at"`
	// LastSwitchAt is the worker's last change of a protocol's target.
	LastSwitchAt *time.Time `json:"last_switch_at"`
	// FailStreak counts the runs in a row in which a current target failed.
	FailStreak int `gorm:"not null;default:0" json:"fail_streak"`
	// LastOutcome says what the last run found or did.
	LastOutcome string    `gorm:"size:500" json:"last_outcome"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (RealityTargetAuto) TableName() string { return "v5_reality_target_auto" }
