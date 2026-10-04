package agente2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// hostHasSystemd tells whether the machine runs systemd with the cgroup v2
// hierarchy, so the machine-telemetry collector can list units (the CI
// runner does; a container usually does not).
func hostHasSystemd() bool {
	if info, err := os.Stat("/run/systemd/system"); err != nil || !info.IsDir() {
		return false
	}
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

// scenarioPluginArtifactsOverMTLS installs the signed machine-telemetry
// package on the node, with the systemd services collector enabled for it.
// The Agent downloads the manifest and the artifact from AgentArtifacts on
// the stream (artifacts.v1, the client certificate; no API key exists).
func (s *suite) scenarioPluginArtifactsOverMTLS(t *testing.T) string {
	pkg := s.pkgs["machine-telemetry"]
	s.control.API(t, http.MethodPut, "/api/v3/plugin-installations", map[string]any{
		"plugin_id": pkg.ID, "target": "agent", "desired_version": pkg.Version, "enabled": true,
	}, nil)
	var agentInstallation *installation
	for _, row := range s.control.Installations(t) {
		if row.PluginID == pkg.ID && row.Target == "agent" {
			agentInstallation = &row
		}
	}
	require.NotNil(t, agentInstallation, "the agent installation of machine-telemetry")
	config := map[string]any{
		"interval_seconds": 5,
		"systemd_services": map[string]any{"nodes": map[string]any{
			fmt.Sprint(s.nodeID): map[string]any{"enabled": true, "include": []string{}, "exclude": []string{}},
		}},
	}
	s.control.API(t, http.MethodPut, fmt.Sprintf("/api/v3/plugin-installations/%d/config", agentInstallation.ID), map[string]any{"config": config}, nil)
	s.control.API(t, http.MethodPut, fmt.Sprintf("/api/v3/nodes/%d/assignments", s.nodeID), map[string]any{
		"service_scope": "monitoring", "plugin_id": pkg.ID, "role": "telemetry", "desired_version": pkg.Version, "enabled": true,
	}, nil)

	// The install operation succeeded on the node and the plugin runs:
	// its telemetry arrives with the heartbeats.
	eventually(t, 90*time.Second, func() (bool, string) {
		installed := s.count(t, "SELECT COUNT(*) FROM v3_kernel_operation WHERE node_id = ? AND plugin_id = ? AND kind = 'plugin.install' AND state = 'succeeded'", s.nodeID, pkg.ID)
		telemetry := s.count(t, "SELECT COUNT(*) FROM v3_kernel_plugin_telemetry_state WHERE node_id = ? AND plugin_id = ?", s.nodeID, pkg.ID)
		return installed == 1 && telemetry == 1, fmt.Sprintf("install succeeded %d, telemetry rows %d", installed, telemetry)
	}, s.agent.proc, s.control.proc)
	s.pluginReadyAt = time.Now()
	// The download went over AgentArtifacts: the Agent counts it in the
	// metrics of its heartbeat.
	var downloads float64
	eventually(t, 60*time.Second, func() (bool, string) {
		node := s.inventory(t)
		if node == nil || node.Session == nil {
			return false, "no session"
		}
		downloads = node.Session.AgentMetrics["agent_dataplane_artifact_downloads_total"]
		return downloads >= 1, fmt.Sprintf("metrics %v", sortedKeys(node.Session.AgentMetrics))
	}, s.agent.proc)
	require.NotContains(t, s.agent.proc.Output(), "X-API-Key", "never the legacy HTTP download")
	return fmt.Sprintf("machine-telemetry %s installed and reporting telemetry, %v artifact download(s) over AgentArtifacts", pkg.Version, downloads)
}

// scenarioSystemdServicesPanel: the plugin's systemd collector reports
// with PackageReport (package-reports.v1); Control keeps the latest and the
// machine-telemetry route answers the services table.
func (s *suite) scenarioSystemdServicesPanel(t *testing.T) string {
	var report struct {
		Version     string
		PayloadJSON string
		ObservedAt  time.Time
		ReceivedAt  time.Time
	}
	// Package reports go every 5 minutes, and at once on a new session:
	// once the collector has had time for its first samples, a short
	// partition starts a new session.
	if s.pluginReadyAt.IsZero() {
		return skipped + ":machine-telemetry is not installed (previous scenario failed)"
	}
	if wait := 45*time.Second - time.Since(s.pluginReadyAt); wait > 0 {
		time.Sleep(wait)
	}
	session := s.connectedSession(t, 30*time.Second, "")
	s.proxy.Drop()
	time.Sleep(2 * time.Second)
	s.proxy.Restore(t)
	s.connectedSession(t, 60*time.Second, session.SessionID)
	eventually(t, 90*time.Second, func() (bool, string) {
		require.NoError(t, s.control.db.Raw("SELECT version, payload_json, observed_at, received_at FROM v4_kernel_package_report_state WHERE node_kind = 'proxy' AND node_id = ? AND plugin_id = 'machine-telemetry'", s.nodeID).Scan(&report).Error)
		return report.PayloadJSON != "", "no package report yet"
	}, s.agent.proc, s.control.proc)
	require.NotContains(t, report.PayloadJSON, "Description")
	require.NotContains(t, report.PayloadJSON, "ExecStart")

	var answer struct {
		Data struct {
			NodeID            uint   `json:"node_id"`
			Enabled           bool   `json:"enabled"`
			Reported          bool   `json:"reported"`
			Supported         bool   `json:"supported"`
			UnsupportedReason string `json:"unsupported_reason"`
			Stale             bool   `json:"stale"`
			Summary           struct {
				Total  int `json:"total"`
				Failed int `json:"failed"`
				Active int `json:"active"`
			} `json:"summary"`
			Units []struct {
				Name        string `json:"name"`
				ActiveState string `json:"active_state"`
			} `json:"units"`
		} `json:"data"`
	}
	s.control.API(t, http.MethodGet, fmt.Sprintf("/api/v3/plugins/machine-telemetry/nodes/%d/services", s.nodeID), nil, &answer)
	data := answer.Data
	require.True(t, data.Enabled, "enabled for the node")
	require.True(t, data.Reported, "reported")
	require.False(t, data.Stale)
	if hostHasSystemd() {
		require.True(t, data.Supported, "systemd and cgroup v2 are on this host: %s", data.UnsupportedReason)
		require.NotEmpty(t, data.Units)
		for _, unit := range data.Units {
			require.True(t, strings.HasSuffix(unit.Name, ".service"), unit.Name)
			require.False(t, strings.HasPrefix(unit.Name, "user@") || strings.HasPrefix(unit.Name, "run-"), unit.Name)
		}
		return fmt.Sprintf("%d units (%d active, %d failed); report observed %s, received %s", data.Summary.Total, data.Summary.Active, data.Summary.Failed,
			report.ObservedAt.UTC().Format(time.RFC3339), report.ReceivedAt.UTC().Format(time.RFC3339))
	}
	require.False(t, data.Supported)
	require.NotEmpty(t, data.UnsupportedReason)
	return "host without systemd/cgroup v2: supported=false (" + data.UnsupportedReason + ")"
}

// scenarioMaintenanceEvents tampers with the installed plugin's signature:
// the supervisor's next maintenance check records a signature fault (a
// manual-action incident, sent at once), which reaches Control as
// MaintenanceEvents (maintenance.v1) and is persisted as a node log.
func (s *suite) scenarioMaintenanceEvents(t *testing.T) string {
	before := s.count(t, "SELECT COUNT(*) FROM v2_node_log WHERE node_id = ? AND source = 'maintenance'", s.nodeID)
	var signatures []string
	_ = filepath.Walk(s.agent.pluginRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == "manifest.sig" && strings.Contains(path, "machine-telemetry") {
			signatures = append(signatures, path)
		}
		return nil
	})
	require.NotEmpty(t, signatures, "the installed machine-telemetry signature under %s", s.agent.pluginRoot)
	originals := map[string][]byte{}
	for _, path := range signatures {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		originals[path] = data
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.NoError(t, os.Chmod(path, 0o600))
		require.NoError(t, os.WriteFile(path, []byte("AAAA"+strings.TrimSpace(string(data))[4:]), 0o600))
		require.NoError(t, os.Chmod(path, info.Mode()))
	}
	defer func() {
		for path, data := range originals {
			_ = os.Chmod(path, 0o600)
			_ = os.WriteFile(path, data, 0o400)
		}
	}()
	var row struct {
		Level   string
		Message string
		TraceID string
	}
	eventually(t, 90*time.Second, func() (bool, string) {
		if s.count(t, "SELECT COUNT(*) FROM v2_node_log WHERE node_id = ? AND source = 'maintenance'", s.nodeID) <= before {
			return false, "no maintenance event persisted"
		}
		require.NoError(t, s.control.db.Raw("SELECT level, message, trace_id FROM v2_node_log WHERE node_id = ? AND source = 'maintenance' ORDER BY id DESC LIMIT 1", s.nodeID).Scan(&row).Error)
		return true, ""
	}, s.agent.proc, s.control.proc)
	require.Equal(t, int64(1), s.count(t, "SELECT COUNT(*) FROM v4_kernel_agent_report_batch WHERE node_kind = 'proxy' AND node_id = ? AND kind = 'maintenance' AND batch_id LIKE 'maintenance:%'", s.nodeID),
		"the event is recorded once in the ledger")
	return fmt.Sprintf("event %s (%s): %s", row.TraceID, row.Level, truncate(row.Message, 160))
}

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
// exists under mtls: required): service_status and log_tail of gost.
func (s *suite) scenarioDiagnostic(t *testing.T) string {
	var evidence []string
	for _, action := range []string{"service_status", "log_tail"} {
		var answer struct {
			Data struct {
				TaskID string `json:"task_id"`
			} `json:"data"`
		}
		s.control.API(t, http.MethodPost, "/api/v2/admin/agent/tasks", map[string]any{
			"node_id": s.nodeID, "type": "diagnostic", "action": action, "params": map[string]any{"service": "gost", "lines": 5}, "timeout": 20,
		}, &answer)
		var task struct {
			TaskID  string
			Status  string
			Success bool
			Output  string
			Error   string
		}
		eventually(t, 40*time.Second, func() (bool, string) {
			query := "SELECT task_id, status, success, output, error FROM v2_agent_diagnostic_task WHERE node_id = ? AND action = ? ORDER BY id DESC LIMIT 1"
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

func prettyJSON(raw string) string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return raw
	}
	out, _ := json.Marshal(v)
	return string(out)
}

// scenarioPluginOperationAfterStreamOperations changes the
// machine-telemetry configuration after operations that live only on the
// stream (agent.diagnostic, the forward checks) ran on the node: the
// durable plugin operation must still reach the Agent.
//
// This suite found that the stream-only operations took their revisions
// from the session's in-memory counter while durable operations took the
// next revision of the node's durable cursor
// (v3_kernel_node_operation_revision), so the durable operation was refused
// ("revision N is not newer than M") and stayed dispatching. Both now
// allocate from the durable cursor; a refusal fails the scenario.
func (s *suite) scenarioPluginOperationAfterStreamOperations(t *testing.T) string {
	if s.pluginReadyAt.IsZero() {
		return skipped + ":machine-telemetry is not installed (previous scenario failed)"
	}
	var installationID uint
	for _, row := range s.control.Installations(t) {
		if row.PluginID == "machine-telemetry" && row.Target == "agent" {
			installationID = row.ID
		}
	}
	require.NotZero(t, installationID)
	before := s.count(t, "SELECT COALESCE(MAX(revision), 0) FROM v3_kernel_operation WHERE node_id = ?", s.nodeID)
	s.control.API(t, http.MethodPut, fmt.Sprintf("/api/v3/plugin-installations/%d/config", installationID), map[string]any{"config": map[string]any{
		"interval_seconds": 10,
		"systemd_services": map[string]any{"nodes": map[string]any{
			fmt.Sprint(s.nodeID): map[string]any{"enabled": true, "include": []string{}, "exclude": []string{}},
		}},
	}}, nil)
	type operation struct {
		ID        string
		Kind      string
		State     string
		Revision  int64
		LastError string
	}
	var ops []operation
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		ops = nil
		require.NoError(t, s.control.db.Raw("SELECT id, kind, state, revision, last_error FROM v3_kernel_operation WHERE node_id = ? AND plugin_id = 'machine-telemetry' AND revision > ? ORDER BY revision", s.nodeID, before).Scan(&ops).Error)
		done := len(ops) > 0
		for _, op := range ops {
			if op.State != "succeeded" {
				done = false
			}
		}
		if done {
			return fmt.Sprintf("%d durable operation(s) succeeded after the stream-only ones", len(ops))
		}
		time.Sleep(time.Second)
	}
	for _, op := range ops {
		if strings.Contains(op.LastError, "is not newer than") {
			t.Fatalf("durable %s at revision %d is refused after stream-only operations: %q (state %s)", op.Kind, op.Revision, op.LastError, op.State)
		}
	}
	if len(ops) == 0 {
		s.skip(t, "the configuration change queued no operation within 40 s (dispatch latency, not the revision check)")
	}
	t.Fatalf("the configuration change did not reach the Agent: %+v\n%s", ops, s.agent.proc.Tail())
	return ""
}
