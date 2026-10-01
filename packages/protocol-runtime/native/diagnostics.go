package native

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
)

// Limits of the kernel's AgentDiagnosticTaskService.ListTasks.
const (
	defaultTaskLimit = 50
	maxTaskLimit     = 200
)

// AgentDiagnosticTask is a v2_agent_diagnostic_task row, as the kernel model
// declares it: a whitelisted diagnostic action an administrator sent to a
// node's agent, and the result the agent reported.
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

// TableName is the adopted kernel table.
func (AgentDiagnosticTask) TableName() string { return "v2_agent_diagnostic_task" }

// ListDiagnosticTasks is GET /api/v2/admin/agent/tasks: the newest tasks,
// optionally of one node. node_id and limit are read as the legacy handler
// reads them, strconv.Atoi with errors as 0.
func (s *Service) ListDiagnosticTasks(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	nodeID, _ := strconv.Atoi(query(request, "node_id"))
	limit, _ := strconv.Atoi(query(request, "limit"))
	db, err := s.Open(ctx)
	if err != nil {
		return internalError(err)
	}
	if limit <= 0 || limit > maxTaskLimit {
		limit = defaultTaskLimit
	}
	tasks := db.Model(&AgentDiagnosticTask{}).Order("id DESC").Limit(limit)
	if nodeID != 0 {
		// #nosec G115 -- the legacy handler's conversion: a negative id
		// wraps as it does there, and matches no node.
		tasks = tasks.Where("node_id = ?", uint(nodeID))
	}
	var rows []AgentDiagnosticTask
	if err := tasks.Find(&rows).Error; err != nil {
		return internalError(err)
	}
	return s.panel(rows)
}

// GetDiagnosticTask is GET /api/v2/admin/agent/tasks/:task_id.
func (s *Service) GetDiagnosticTask(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	taskID := request.Metadata.PathParams["task_id"]
	if taskID == "" {
		return errorAnswer(http.StatusBadRequest, "task_id is required")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return internalError(err)
	}
	var task AgentDiagnosticTask
	if err := db.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errorAnswer(http.StatusNotFound, "task result not found")
		}
		return internalError(err)
	}
	return s.panel(task)
}
