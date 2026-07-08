package model

import "time"

const (
	ForwardAgentBridgeTaskStatusPending    = "pending"
	ForwardAgentBridgeTaskStatusDispatched = "dispatched"
	ForwardAgentBridgeTaskStatusCompleted  = "completed"
	ForwardAgentBridgeTaskStatusFailed     = "failed"
)

// ForwardAgentBridgeTask 持久化 clean_agent 转发任务的 task_id ↔ runtime_job 映射。
// 由于 /api/v2/agent/tasks 的内存任务表重启即丢，这张表用于让 bridge worker 下发的
// 任务在重启后仍可回溯，从而把 agent 上报结果准确回写到对应 runtime job 与 forward。
type ForwardAgentBridgeTask struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TaskID       string    `gorm:"size:80;uniqueIndex" json:"taskId"`
	RuntimeJobID uint      `gorm:"uniqueIndex" json:"runtimeJobId"`
	NodeID       uint      `gorm:"index" json:"nodeId"`
	ForwardID    *uint     `gorm:"index" json:"forwardId"`
	Action       string    `gorm:"size:30" json:"action"`
	Type         string    `gorm:"size:30" json:"type"`
	Params       string    `gorm:"type:text" json:"params"`
	Status       string    `gorm:"size:20;index" json:"status"`
	Dispatched   bool      `gorm:"default:false" json:"dispatched"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ForwardAgentBridgeTask) TableName() string {
	return "v2_forward_agent_bridge_task"
}
