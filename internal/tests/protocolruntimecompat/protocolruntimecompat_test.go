// Package protocolruntimecompat proves the protocol-runtime package's native
// routes answer exactly as the kernel's legacy handlers, on SQLite and
// PostgreSQL: the protocol templates, and the administrator's diagnostic
// task history and task detail on v2_agent_diagnostic_task, which the
// package adopts in place.
package protocolruntimecompat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.AgentDiagnosticTask{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// agent builds the legacy handler per request: it keeps the database it was
// built with, and the harness sets up a database per case.
func agent(method func(*handler.AgentHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewAgentHandler(), c) }
}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// seedTasks writes 57 tasks of three nodes, in every state, some with
// output and errors and some without params (omitted from the answer).
func seedTasks(t testing.TB, db *gorm.DB) {
	statuses := []string{model.AgentDiagnosticTaskStatusPending, model.AgentDiagnosticTaskStatusDispatched,
		model.AgentDiagnosticTaskStatusCompleted, model.AgentDiagnosticTaskStatusFailed}
	tasks := make([]model.AgentDiagnosticTask, 0, 57)
	for i := 1; i <= 57; i++ {
		task := model.AgentDiagnosticTask{
			ID: uint(i), TaskID: fmt.Sprintf("task-%03d", i), NodeID: uint(i%3 + 1), Action: "service_status",
			Params: `{"service":"gost"}`, Status: statuses[i%4], DurationMS: int64(i * 7),
			CreatedAt: seeded.Add(time.Duration(i) * time.Minute), UpdatedAt: seeded.Add(time.Duration(i)*time.Minute + time.Second),
		}
		switch i % 4 {
		case 2:
			task.Success, task.Output = true, fmt.Sprintf("gost.service active (running) <%d>", i)
		case 3:
			task.Error = "exit status 3: \"gost\" & friends"
		case 0:
			task.Action, task.Params = "log_tail", ""
		}
		tasks = append(tasks, task)
	}
	require.NoError(t, db.Create(&tasks).Error)
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		packagecompat.RunRead(t, r, c)
	}
}

func TestProtocolTemplatesRouteParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/protocol-templates", "protocol.admin.protocol_templates.get",
		func(c *gin.Context) { handler.NewNodeHandler().GetProtocolTemplates(c) }), []packagecompat.Case{
		{Name: "every template", Path: "/api/v2/admin/protocol-templates"},
	})
}

func TestDiagnosticTaskListRouteParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/agent/tasks", "protocol.admin.agent.tasks.get", agent((*handler.AgentHandler).ListDiagnosticTasks)), []packagecompat.Case{
		{Name: "newest 50", Path: "/api/v2/admin/agent/tasks", Seed: seedTasks},
		{Name: "one node", Path: "/api/v2/admin/agent/tasks?node_id=2", Seed: seedTasks},
		{Name: "one node and a limit", Path: "/api/v2/admin/agent/tasks?node_id=3&limit=4", Seed: seedTasks},
		{Name: "a node without tasks", Path: "/api/v2/admin/agent/tasks?node_id=9", Seed: seedTasks},
		{Name: "a node id that is not a number", Path: "/api/v2/admin/agent/tasks?node_id=two", Seed: seedTasks},
		{Name: "a negative node id", Path: "/api/v2/admin/agent/tasks?node_id=-1", Seed: seedTasks},
		{Name: "the first of repeated node ids", Path: "/api/v2/admin/agent/tasks?node_id=1&node_id=2", Seed: seedTasks},
		{Name: "limit", Path: "/api/v2/admin/agent/tasks?limit=3", Seed: seedTasks},
		{Name: "the largest limit", Path: "/api/v2/admin/agent/tasks?limit=200", Seed: seedTasks},
		{Name: "a limit above 200", Path: "/api/v2/admin/agent/tasks?limit=201", Seed: seedTasks},
		{Name: "limit zero", Path: "/api/v2/admin/agent/tasks?limit=0", Seed: seedTasks},
		{Name: "a negative limit", Path: "/api/v2/admin/agent/tasks?limit=-5", Seed: seedTasks},
		{Name: "a limit that is not a number", Path: "/api/v2/admin/agent/tasks?limit=ten", Seed: seedTasks},
		{Name: "no tasks", Path: "/api/v2/admin/agent/tasks"},
	})
}

func TestDiagnosticTaskDetailRouteParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/agent/tasks/:task_id", "protocol.admin.agent.tasks.task_id.get", agent((*handler.AgentHandler).GetTaskResult)), []packagecompat.Case{
		{Name: "completed with output", Path: "/api/v2/admin/agent/tasks/task-002", Seed: seedTasks},
		{Name: "failed with an error", Path: "/api/v2/admin/agent/tasks/task-003", Seed: seedTasks},
		{Name: "without params", Path: "/api/v2/admin/agent/tasks/task-004", Seed: seedTasks},
		{Name: "pending", Path: "/api/v2/admin/agent/tasks/task-005", Seed: seedTasks},
		{Name: "unknown", Path: "/api/v2/admin/agent/tasks/task-999", Seed: seedTasks},
		{Name: "an id with SQL in it", Path: "/api/v2/admin/agent/tasks/task-001'%20OR%20'1'='1", Seed: seedTasks},
		{Name: "no tasks", Path: "/api/v2/admin/agent/tasks/task-001"},
	})
}
