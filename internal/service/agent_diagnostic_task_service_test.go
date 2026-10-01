package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDiagnosticTaskDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AgentDiagnosticTask{}))
	return db
}

// The protocol-runtime package adopts v2_agent_diagnostic_task, so the
// agents' poll checks a pending task against the whitelist again: an action
// off it fails instead of going out, and only the action's own params go
// out, normalized.
func TestPullPendingTasksDispatchesOnlyWhitelistedActions(t *testing.T) {
	db := openDiagnosticTaskDB(t)
	svc := NewAgentDiagnosticTaskService(db)
	rows := []model.AgentDiagnosticTask{
		{TaskID: "task-ok", NodeID: 7, Action: "log_tail", Params: `{"service":"gost","lines":5000,"path":"/etc/shadow"}`, Status: model.AgentDiagnosticTaskStatusPending},
		{TaskID: "task-shell", NodeID: 7, Action: "shell", Params: `{"command":"id"}`, Status: model.AgentDiagnosticTaskStatusPending},
		{TaskID: "task-service", NodeID: 7, Action: "service_restart", Params: `{"service":"sshd"}`, Status: model.AgentDiagnosticTaskStatusPending},
		{TaskID: "task-params", NodeID: 7, Action: "service_status", Params: `not json`, Status: model.AgentDiagnosticTaskStatusPending},
		{TaskID: "task-other-node", NodeID: 8, Action: "shell", Status: model.AgentDiagnosticTaskStatusPending},
	}
	require.NoError(t, db.Create(&rows).Error)

	dispatched, err := svc.PullPendingTasks(7)
	require.NoError(t, err)
	require.Len(t, dispatched, 1)
	require.Equal(t, "task-ok", dispatched[0].TaskID)
	require.JSONEq(t, `{"service":"gost","lines":1000}`, dispatched[0].Params)

	statuses := map[string]string{}
	var stored []model.AgentDiagnosticTask
	require.NoError(t, db.Order("id").Find(&stored).Error)
	for _, task := range stored {
		statuses[task.TaskID] = task.Status
		if task.Status == model.AgentDiagnosticTaskStatusFailed {
			require.Contains(t, task.Error, "rejected before dispatch")
		}
	}
	require.Equal(t, map[string]string{
		"task-ok":         model.AgentDiagnosticTaskStatusDispatched,
		"task-shell":      model.AgentDiagnosticTaskStatusFailed,
		"task-service":    model.AgentDiagnosticTaskStatusFailed,
		"task-params":     model.AgentDiagnosticTaskStatusFailed,
		"task-other-node": model.AgentDiagnosticTaskStatusPending,
	}, statuses)
}
