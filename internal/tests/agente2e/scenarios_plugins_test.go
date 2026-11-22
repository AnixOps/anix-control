package agente2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// scenarioAliveList: Control pushes the alive list (alive.v1) from the
// online IPs the traffic reports carried; the Agent records its revision.
func (s *suite) scenarioAliveList(t *testing.T) string {
	var revision, users float64
	eventually(t, 150*time.Second, func() (bool, string) {
		node := s.inventory(t)
		if node == nil || node.Session == nil {
			return false, "no session"
		}
		revision = node.Session.AgentMetrics["agent_dataplane_alive_revision"]
		users = node.Session.AgentMetrics["agent_dataplane_alive_users"]
		return revision > 0, fmt.Sprintf("alive revision %v", revision)
	}, s.agent.proc)
	return fmt.Sprintf("alive list revision %v with %v user(s) applied by the Agent", revision, users)
}

// scenarioDiagnostic runs agent.diagnostic on the stream (no WebSocket
// exists under mtls: required): service_status and log_tail of the Agent.
func (s *suite) scenarioDiagnostic(t *testing.T) string {
	var evidence []string
	for _, action := range []string{"service_status", "log_tail"} {
		var answer struct {
			Data struct {
				TaskID string `json:"task_id"`
			} `json:"data"`
		}
		s.control.API(t, http.MethodPost, "/api/v2/admin/agent/tasks", map[string]any{
			"node_id": s.nodeID, "type": "diagnostic", "action": action, "params": map[string]any{"service": "anix-agent", "lines": 5}, "timeout": 20,
		}, &answer)
		var task struct {
			TaskID  string
			Status  string
			Success bool
			Output  string
			Error   string
		}
		eventually(t, 40*time.Second, func() (bool, string) {
			query := "SELECT task_id, status, success, output, error FROM v5_agent_diagnostic_task WHERE node_id = ? AND action = ? ORDER BY id DESC LIMIT 1"
			require.NoError(t, s.control.db.Raw(query, s.nodeID, action).Scan(&task).Error)
			return task.Status == "completed" || task.Status == "failed", fmt.Sprintf("%+v", task)
		}, s.agent.proc, s.control.proc)
		require.Equal(t, "completed", task.Status, "%+v", task)
		evidence = append(evidence, fmt.Sprintf("%s %s: %s", action, task.Status, truncate(strings.TrimSpace(task.Output), 60)))
	}
	require.Contains(t, s.agent.ShimCalls(), "systemctl status")
	return strings.Join(evidence, "; ")
}

func truncate(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[:n] + "..."
}
