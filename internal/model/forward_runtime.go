package model

import "time"

const (
	ForwardRuntimeBackendGost            = "gost"
	ForwardRuntimeBackendIptablesAnsible = "iptables_ansible"
)

const (
	// These status values are also used by Forward.RuntimeStatus as the latest runtime sync state.
	ForwardRuntimeJobStatusPending = 0
	ForwardRuntimeJobStatusRunning = 1
	ForwardRuntimeJobStatusSuccess = 2
	ForwardRuntimeJobStatusFailed  = 3
)

const (
	ForwardRuntimeJobActionCreate = "create"
	ForwardRuntimeJobActionUpdate = "update"
	ForwardRuntimeJobActionDelete = "delete"
	ForwardRuntimeJobActionPause  = "pause"
	ForwardRuntimeJobActionResume = "resume"
	ForwardRuntimeJobActionSync   = "sync"
)

// ForwardRuntimeJob records non-DB runtime actions for forward-compatible runtimes.
type ForwardRuntimeJob struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Backend      string     `gorm:"size:50;not null;index" json:"backend"`
	Action       string     `gorm:"size:30;not null;index" json:"action"`
	ResourceType string     `gorm:"size:30;index" json:"resourceType"`
	ResourceID   *uint      `json:"resourceId"`
	ForwardID    *uint      `gorm:"index" json:"forwardId"`
	TunnelID     *uint      `gorm:"index" json:"tunnelId"`
	NodeID       *uint      `gorm:"index" json:"nodeId"`
	Status       int        `gorm:"default:0;index" json:"status"`
	Payload      string     `gorm:"type:text" json:"payload"`
	Result       string     `gorm:"type:text" json:"result"`
	Error        string     `gorm:"type:text" json:"error"`
	StartedAt    *time.Time `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (ForwardRuntimeJob) TableName() string {
	return "v2_forward_runtime_job"
}
