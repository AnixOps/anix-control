package model

import "time"

// AgentReportBatch records a report batch an agent delivered on the Agent
// Control stream (reports.v1): a TrafficReport or a LogBatch, by the node's
// kind and id and the batch id the agent chose. The kernel applies a batch
// in the transaction that creates its row, so a batch counts once however
// often the agent resends it; a replay is answered from the row. Rows older
// than the retention are pruned (internal/agentreports).
type AgentReportBatch struct {
	NodeKind  string    `gorm:"primaryKey;size:16" json:"node_kind"`
	NodeID    uint      `gorm:"primaryKey" json:"node_id"`
	BatchID   string    `gorm:"primaryKey;size:128" json:"batch_id"`
	Kind      string    `gorm:"size:16;not null" json:"kind"`
	CreatedAt time.Time `gorm:"not null;index" json:"created_at"`
}

func (AgentReportBatch) TableName() string { return "v4_kernel_agent_report_batch" }
