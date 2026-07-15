package service

import (
	"encoding/json"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

// AgentDiagnosticTaskService 负责白名单诊断任务的持久化，取代 AgentHandler 里原来的
// taskResults sync.Map，使任务结果在面板进程重启后仍可查询。
type AgentDiagnosticTaskService struct {
	db *gorm.DB
}

func NewAgentDiagnosticTaskService(db *gorm.DB) *AgentDiagnosticTaskService {
	if db == nil {
		db = database.Get()
	}
	return &AgentDiagnosticTaskService{db: db}
}

// CreateTask 插入一条 pending 状态的诊断任务记录，params 必须已经过
// ValidateAgentDiagnosticTask 归一化。
func (s *AgentDiagnosticTaskService) CreateTask(taskID string, nodeID uint, action string, params map[string]any) (*model.AgentDiagnosticTask, error) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	task := &model.AgentDiagnosticTask{
		TaskID: taskID,
		NodeID: nodeID,
		Action: action,
		Params: string(paramsJSON),
		Status: model.AgentDiagnosticTaskStatusPending,
	}
	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}

// MarkStatus 更新任务的投递状态（如 dispatched），不改变最终结果字段。
func (s *AgentDiagnosticTaskService) MarkStatus(taskID string, status string) error {
	return s.db.Model(&model.AgentDiagnosticTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]any{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

// PullPendingTasks 取出该节点尚未下发 (status=pending) 的诊断任务，并原子标记为
// dispatched，供 HTTP 轮询降级路径 (AgentGetTasks) 使用，避免并发重复下发。
func (s *AgentDiagnosticTaskService) PullPendingTasks(nodeID uint) ([]model.AgentDiagnosticTask, error) {
	if nodeID == 0 {
		return nil, nil
	}

	var pending []model.AgentDiagnosticTask
	if err := s.db.
		Where("node_id = ? AND status = ?", nodeID, model.AgentDiagnosticTaskStatusPending).
		Order("id ASC").
		Find(&pending).Error; err != nil {
		return nil, err
	}

	dispatched := make([]model.AgentDiagnosticTask, 0, len(pending))
	for i := range pending {
		result := s.db.Model(&model.AgentDiagnosticTask{}).
			Where("id = ? AND status = ?", pending[i].ID, model.AgentDiagnosticTaskStatusPending).
			Updates(map[string]any{
				"status":     model.AgentDiagnosticTaskStatusDispatched,
				"updated_at": time.Now(),
			})
		if result.Error != nil {
			return dispatched, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		dispatched = append(dispatched, pending[i])
	}
	return dispatched, nil
}

// CompleteTask 写入任务的最终执行结果。如果 task_id 尚无记录（例如 agent 直接
// 上报、面板端没有先建任务），则补建一条 completed/failed 记录，保持幂等可查询。
func (s *AgentDiagnosticTaskService) CompleteTask(taskID string, nodeID uint, action string, success bool, output string, errMsg string, durationMS int64) error {
	status := model.AgentDiagnosticTaskStatusCompleted
	if !success {
		status = model.AgentDiagnosticTaskStatusFailed
	}

	updates := map[string]any{
		"status":      status,
		"success":     success,
		"output":      output,
		"error":       errMsg,
		"duration_ms": durationMS,
		"updated_at":  time.Now(),
	}
	if nodeID != 0 {
		updates["node_id"] = nodeID
	}
	if action != "" {
		updates["action"] = action
	}

	result := s.db.Model(&model.AgentDiagnosticTask{}).
		Where("task_id = ?", taskID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	return s.db.Create(&model.AgentDiagnosticTask{
		TaskID:     taskID,
		NodeID:     nodeID,
		Action:     action,
		Status:     status,
		Success:    success,
		Output:     output,
		Error:      errMsg,
		DurationMS: durationMS,
	}).Error
}

// GetTask 按 task_id 查询单条任务。
func (s *AgentDiagnosticTaskService) GetTask(taskID string) (*model.AgentDiagnosticTask, error) {
	var task model.AgentDiagnosticTask
	if err := s.db.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// ListTasks 按可选的 node_id 过滤，返回最近的诊断任务历史（供管理页任务历史表使用）。
func (s *AgentDiagnosticTaskService) ListTasks(nodeID uint, limit int) ([]model.AgentDiagnosticTask, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := s.db.Model(&model.AgentDiagnosticTask{}).Order("id DESC").Limit(limit)
	if nodeID != 0 {
		query = query.Where("node_id = ?", nodeID)
	}
	var tasks []model.AgentDiagnosticTask
	if err := query.Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}
