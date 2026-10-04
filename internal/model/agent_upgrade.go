package model

import "time"

// Agent upgrade campaign statuses (forward-sdk.md section 9, O4; H19).
const (
	// AgentUpgradeRunning: batches are offered and evaluated.
	AgentUpgradeRunning = "running"
	// AgentUpgradePaused: no node is offered the upgrade and no batch
	// starts; nodes already offered are still evaluated, and a failed
	// batch still rolls back.
	AgentUpgradePaused = "paused"
	// AgentUpgradeRollingBack: a batch failed (or an administrator aborted
	// with rollback); its upgraded nodes are told to roll back.
	AgentUpgradeRollingBack = "rolling_back"
	// AgentUpgradeSucceeded: every batch passed (terminal).
	AgentUpgradeSucceeded = "succeeded"
	// AgentUpgradeRolledBack: a batch failed and was rolled back (terminal).
	AgentUpgradeRolledBack = "rolled_back"
	// AgentUpgradeAborted: an administrator stopped it (terminal).
	AgentUpgradeAborted = "aborted"
)

// AgentUpgradeActiveSlot is AgentUpgradeCampaign.ActiveSlot of the one
// campaign that is not terminal.
const AgentUpgradeActiveSlot = "active"

// Per-node states of a campaign.
const (
	// AgentUpgradeNodePending: not offered yet (its batch has not started,
	// or the node is offline or busy).
	AgentUpgradeNodePending = "pending"
	// AgentUpgradeNodeOffered: the agent.upgrade operation was sent and
	// acknowledged.
	AgentUpgradeNodeOffered = "offered"
	// AgentUpgradeNodeUpgrading: the Agent reported progress or handed the
	// release to its updater; Control waits for the new version in Hello.
	AgentUpgradeNodeUpgrading = "upgrading"
	// AgentUpgradeNodeSucceeded: the Agent reconnected with the target
	// version (or already ran it) and its configuration is healthy.
	AgentUpgradeNodeSucceeded = "succeeded"
	// AgentUpgradeNodeFailed: refused, failed to apply, did not reconnect
	// with the target version in time, or failed its configuration.
	AgentUpgradeNodeFailed = "failed"
	// AgentUpgradeNodeRolledBack: told to roll back after its batch failed.
	AgentUpgradeNodeRolledBack = "rolled_back"
	// AgentUpgradeNodeSkipped: not offered: its Agent does not negotiate
	// upgrade.v1, or it stayed offline for its whole batch. Not counted
	// in the batch's failure ratio.
	AgentUpgradeNodeSkipped = "skipped"
)

// AgentUpgradeCampaign is one staged Agent upgrade: the target release,
// its verified artifacts, the batches and where the campaign is. At most
// one campaign is not terminal (ActiveSlot is unique and NULL once it
// ends). Protected (service.protectedTables).
type AgentUpgradeCampaign struct {
	ID            string `gorm:"primaryKey;size:36" json:"id"`
	TargetVersion string `gorm:"size:64;not null" json:"target_version"`
	// ArtifactsJSON is the per-architecture release assets the operation
	// carries ([]agentcontrol.UpgradeArtifact: asset, url, sha256, size,
	// signature), verified with the official key when the campaign was
	// created.
	ArtifactsJSON string `gorm:"type:text;not null" json:"-"`
	// BatchesJSON is the batches ([]agentupgrade.Batch: cumulative
	// percent, minimum duration in seconds).
	BatchesJSON string `gorm:"type:text;not null" json:"-"`
	// FailureThresholdPercent: a batch rolls back when more than this
	// share of its offered nodes fail (H19: 5).
	FailureThresholdPercent int `gorm:"not null" json:"failure_threshold_percent"`
	// ReconnectTimeoutSeconds: an offered node must reconnect with the
	// target version within this time (H19: 600).
	ReconnectTimeoutSeconds int `gorm:"not null" json:"reconnect_timeout_seconds"`
	// ExcludeJSON lists the excluded nodes and tags
	// ({"nodes":["forward-3"],"tags":["edge"]}).
	ExcludeJSON string `gorm:"type:text;not null" json:"-"`
	Status      string `gorm:"size:16;not null;index" json:"status"`
	// ActiveSlot is AgentUpgradeActiveSlot while the campaign is not
	// terminal, NULL afterwards: its unique index admits one active
	// campaign on SQLite and PostgreSQL alike.
	ActiveSlot *string `gorm:"size:8;uniqueIndex:ux_v4_kernel_agent_upgrade_active" json:"-"`
	// CurrentBatch is the index of the running batch.
	CurrentBatch   int        `gorm:"not null;default:0" json:"current_batch"`
	BatchStartedAt *time.Time `json:"batch_started_at"`
	// PausedAt is when the campaign was paused; nil otherwise.
	PausedAt *time.Time `json:"paused_at"`
	// RollbackStartedAt is when the current batch started rolling back.
	RollbackStartedAt *time.Time `json:"rollback_started_at"`
	// StatusReason and ErrorCode explain a terminal status or rollback.
	StatusReason string     `gorm:"size:1024;not null;default:''" json:"status_reason"`
	ErrorCode    string     `gorm:"size:64;not null;default:''" json:"error_code"`
	Reason       string     `gorm:"size:512;not null;default:''" json:"reason"`
	CreatedBy    uint       `gorm:"not null;default:0" json:"created_by"`
	Actor        string     `gorm:"size:100;not null;default:''" json:"actor"`
	CreatedAt    time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"not null" json:"updated_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

func (AgentUpgradeCampaign) TableName() string { return "v4_kernel_agent_upgrade_campaign" }

// AgentUpgradeNode is one node of a campaign: its batch, its place in the
// canary order and how its upgrade went.
type AgentUpgradeNode struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"-"`
	CampaignID string `gorm:"size:36;not null;uniqueIndex:ux_v4_kernel_agent_upgrade_node,priority:1;index:idx_v4_kernel_agent_upgrade_node_batch,priority:1" json:"campaign_id"`
	// NodeKind is proxy or forward; ids of the two overlap.
	NodeKind string `gorm:"size:16;not null;uniqueIndex:ux_v4_kernel_agent_upgrade_node,priority:2" json:"node_kind"`
	NodeID   uint64 `gorm:"not null;uniqueIndex:ux_v4_kernel_agent_upgrade_node,priority:3" json:"node_id"`
	// Batch is the node's batch; OrderKey its canary order (the
	// SHA-256 of its name, hex), lowest first.
	Batch    int    `gorm:"not null;index:idx_v4_kernel_agent_upgrade_node_batch,priority:2" json:"batch"`
	OrderKey string `gorm:"size:64;not null" json:"order_key"`
	State    string `gorm:"size:16;not null" json:"state"`
	// FromVersion is the version the node's Agent ran when it was offered
	// the upgrade (the rollback target).
	FromVersion string `gorm:"size:64;not null;default:''" json:"from_version"`
	// OperationID is the agent.upgrade operation sent; SessionID the
	// stream session it was sent on.
	OperationID string `gorm:"size:64;not null;default:''" json:"operation_id"`
	SessionID   string `gorm:"size:64;not null;default:''" json:"session_id"`
	// RollbackOperationID is the rollback operation sent, empty if none.
	RollbackOperationID string     `gorm:"size:64;not null;default:''" json:"rollback_operation_id"`
	ErrorCode           string     `gorm:"size:64;not null;default:''" json:"error_code"`
	Error               string     `gorm:"size:1024;not null;default:''" json:"error"`
	OfferedAt           *time.Time `json:"offered_at"`
	HandedOffAt         *time.Time `json:"handed_off_at"`
	ReconnectedAt       *time.Time `json:"reconnected_at"`
	FinishedAt          *time.Time `json:"finished_at"`
	RollbackSentAt      *time.Time `json:"rollback_sent_at"`
	CreatedAt           time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"not null" json:"updated_at"`
}

func (AgentUpgradeNode) TableName() string { return "v4_kernel_agent_upgrade_node" }

// AgentUpgradeModels are the tables of Agent upgrade campaigns.
func AgentUpgradeModels() []any {
	return []any{&AgentUpgradeCampaign{}, &AgentUpgradeNode{}}
}
