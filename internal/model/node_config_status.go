package model

import "time"

// Verdicts of an agent's ConfigStatus, as KernelNodeConfigStatus.Verdict
// records them (node-ops-service.md section 5.5).
const (
	// ConfigVerdictApplied: the agent applied the node's desired
	// configuration, at its revision and hash.
	ConfigVerdictApplied = "applied"
	// ConfigVerdictFailed: the agent could not apply the node's desired
	// configuration, at its revision and hash.
	ConfigVerdictFailed = "failed"
	// ConfigVerdictStale: the status names a revision older than the
	// node's desired one.
	ConfigVerdictStale = "stale"
	// ConfigVerdictMismatch: the status names the desired revision with
	// another hash, a revision the kernel never stored, or a node without
	// a desired configuration.
	ConfigVerdictMismatch = "mismatch"
)

// KernelNodeConfigStatus is what a node's agent last reported about its
// configuration on the Agent Control stream (config.v1, A2-3): its last
// ConfigStatus as reported, and the last revision the kernel verified as
// applied. One row per node kind and id. A status is verified when its
// revision and hash are the node's desired ones
// (v4_kernel_node_desired_config) at the time it arrives; a stale or
// mismatched status is recorded in the reported columns and never moves
// the applied ones.
//
// The table is protected (service.protectedTables): a package that could
// write it could make a node look converged.
type KernelNodeConfigStatus struct {
	ID uint64 `gorm:"primaryKey;autoIncrement" json:"-"`
	// NodeKind is proxy (v2_node) or forward (v2_forward_node); ids of the
	// two tables overlap.
	NodeKind string `gorm:"size:16;not null;uniqueIndex:idx_kernel_node_config_status_node,priority:1" json:"node_kind"`
	NodeID   uint64 `gorm:"not null;uniqueIndex:idx_kernel_node_config_status_node,priority:2" json:"node_id"`
	// SessionID is the stream session that sent the last status.
	SessionID string `gorm:"size:64;not null" json:"session_id"`
	// ReportedRevision, ReportedHash, ReportedApplied and ReportedError are
	// the last ConfigStatus as the agent sent it (the error cut to 1024
	// bytes); Verdict is how the kernel judged it (ConfigVerdict*).
	ReportedRevision uint64    `gorm:"not null;default:0" json:"reported_revision"`
	ReportedHash     string    `gorm:"size:64;not null" json:"reported_hash"`
	ReportedApplied  bool      `gorm:"not null;default:false" json:"reported_applied"`
	ReportedError    string    `gorm:"size:1024;not null" json:"reported_error"`
	Verdict          string    `gorm:"size:16;not null" json:"verdict"`
	ReportedAt       time.Time `gorm:"not null" json:"reported_at"`
	// AppliedRevision and AppliedHash are the last configuration the kernel
	// verified as applied; zero and empty until one is.
	AppliedRevision uint64     `gorm:"not null;default:0" json:"applied_revision"`
	AppliedHash     string     `gorm:"size:64;not null" json:"applied_hash"`
	AppliedAt       *time.Time `json:"applied_at"`
	CreatedAt       time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"not null" json:"updated_at"`
}

func (KernelNodeConfigStatus) TableName() string { return "v4_kernel_node_config_status" }
