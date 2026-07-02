package model

import "time"

const (
	AgentDiagnosticTaskStatusPending    = "pending"
	AgentDiagnosticTaskStatusDispatched = "dispatched"
	AgentDiagnosticTaskStatusCompleted  = "completed"
	AgentDiagnosticTaskStatusFailed     = "failed"
)

// AgentDiagnosticTask 持久化白名单诊断任务（NodeX Agent 终端功能），使任务结果在
// 面板进程重启后仍可查询，不再依赖 AgentHandler.taskResults 这张纯内存表。
// JSON 字段沿用原 AgentTaskStatus 的 snake_case 命名，保持与前端 Agent.vue 的契约不变。
type AgentDiagnosticTask struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TaskID     string    `gorm:"size:80;uniqueIndex" json:"task_id"`
	NodeID     uint      `gorm:"index" json:"node_id"`
	Action     string    `gorm:"size:40" json:"action"`
	Params     string    `gorm:"type:text" json:"params,omitempty"`
	Status     string    `gorm:"size:20;index" json:"status"`
	Success    bool      `json:"success"`
	Output     string    `gorm:"type:text" json:"output,omitempty"`
	Error      string    `gorm:"type:text" json:"error,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"timestamp"`
}

func (AgentDiagnosticTask) TableName() string {
	return "v2_agent_diagnostic_task"
}
