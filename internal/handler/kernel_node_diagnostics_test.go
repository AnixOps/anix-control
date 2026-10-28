package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// diagnosticStreams is the Agent Control streams of a test: sessions by
// node, and a script that answers each dispatched operation with an
// acknowledgement and, optionally, a terminal observed state.
type diagnosticStreams struct {
	mu       sync.Mutex
	sessions map[agentcontrol.AgentNode]agentstreams.Session
	handlers []agentstreams.ObservedHandler
	sent     []*agentv1pb.DesiredOperation
	// answer returns the state the agent reports for an operation; nil
	// reports nothing (the agent hangs). refuse, when set, is the nack.
	answer func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState
	refuse string
	// gate, when set, holds every dispatch until it is closed.
	gate chan struct{}
}

func newDiagnosticStreams() *diagnosticStreams {
	return &diagnosticStreams{sessions: map[agentcontrol.AgentNode]agentstreams.Session{}}
}

func (s *diagnosticStreams) connect(node agentcontrol.AgentNode, capabilities ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[node] = agentstreams.Session{
		Node: node, Transport: agentstreams.TransportControlStream, SessionID: "s-" + node.String(), AgentVersion: "4.2.0",
		Capabilities: capabilities,
	}
}

func (s *diagnosticStreams) operations() []*agentv1pb.DesiredOperation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*agentv1pb.DesiredOperation(nil), s.sent...)
}

func (s *diagnosticStreams) Session(node agentcontrol.AgentNode) (agentstreams.Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[node]
	return session, ok
}

func (s *diagnosticStreams) Sessions() []agentstreams.Session { return nil }

func (s *diagnosticStreams) Observed(agentcontrol.AgentNode) (*agentv1pb.ObservedState, bool) {
	return nil, false
}

func (s *diagnosticStreams) Dispatch(ctx context.Context, node agentcontrol.AgentNode, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	if _, ok := s.sessions[node]; !ok {
		s.mu.Unlock()
		return nil, agentstreams.ErrNotConnected
	}
	s.sent = append(s.sent, operation)
	handlers := append([]agentstreams.ObservedHandler(nil), s.handlers...)
	refuse, answer := s.refuse, s.answer
	s.mu.Unlock()
	if refuse != "" {
		return &agentv1pb.OperationAck{OperationId: operation.GetOperationId(), Accepted: false, Error: refuse}, nil
	}
	if answer != nil {
		if observed := answer(operation); observed != nil {
			observed.OperationId, observed.Revision = operation.GetOperationId(), 1
			go func() {
				time.Sleep(20 * time.Millisecond)
				for _, handler := range handlers {
					handler(node, observed)
				}
			}()
		}
	}
	return &agentv1pb.OperationAck{OperationId: operation.GetOperationId(), Accepted: true, Revision: 1}, nil
}

func (s *diagnosticStreams) Cancel(context.Context, agentcontrol.AgentNode, string, uint64) error {
	return nil
}

func (s *diagnosticStreams) OnObserved(handler agentstreams.ObservedHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, handler)
}

func succeeded(state string) *agentv1pb.ObservedState {
	return &agentv1pb.ObservedState{Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, StateJson: []byte(state)}
}

type diagnosticsEnv struct {
	router  *gin.Engine
	db      *gorm.DB
	streams *diagnosticStreams
}

const diagnosticsCapabilities = kernelnodeops.AgentDiagnosticOperation

func newDiagnosticsEnv(t *testing.T) *diagnosticsEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "diag.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Node{}, &model.ForwardNode{}, &model.AgentDiagnosticTask{}, &model.OperationLog{}))
	for _, user := range []model.User{{ID: 7, Email: "root@example.com", IsAdmin: 1}, {ID: 8, Email: "staff@example.com", IsAdmin: 1, IsStaff: 1}} {
		user.Token = "token-" + strconv.FormatUint(uint64(user.ID), 10)
		user.UUID = "uuid-" + strconv.FormatUint(uint64(user.ID), 10)
		require.NoError(t, db.Create(&user).Error)
	}
	require.NoError(t, db.Create(&model.Node{ID: 5, Name: "hk-01", APIKey: "key-5", Status: model.NodeStatusOnline}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 5, Name: "relay", Host: "192.0.2.1", Port: 22, Enabled: true}).Error)

	env := &diagnosticsEnv{db: db, streams: newDiagnosticStreams()}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID := uint(7)
		if raw := c.GetHeader("X-Test-User"); raw != "" {
			parsed, _ := strconv.ParseUint(raw, 10, 32)
			userID = uint(parsed)
		}
		c.Set("user_id", userID)
		c.Set("email", "root@example.com")
	})
	handler := &NodeDiagnosticsHandler{db: func() *gorm.DB { return db }, sources: func() kernelnodeops.AgentSources {
		return kernelnodeops.AgentSources{Streams: env.streams}
	}}
	router.POST("/nodes/:id/diagnostics", handler.Run)
	router.GET("/nodes/:id/diagnostics", handler.List)
	env.router = router
	return env
}

func (e *diagnosticsEnv) call(t *testing.T, user uint, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-User", strconv.FormatUint(uint64(user), 10))
	e.router.ServeHTTP(recorder, request)
	return recorder
}

func (e *diagnosticsEnv) audits(t *testing.T) []model.OperationLog {
	t.Helper()
	var rows []model.OperationLog
	require.NoError(t, e.db.Where("module = ?", "node_diagnostic").Order("id ASC").Find(&rows).Error)
	return rows
}

func proxy(id uint32) agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: id}
}

func forward(id uint32) agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: id}
}

func auditContent(t *testing.T, row model.OperationLog) map[string]any {
	t.Helper()
	var content map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.Content), &content))
	return content
}

// Only the whitelist runs: an unknown action, a service outside it and a
// forward check without its hop are refused before anything is sent, and
// every refusal is in the operation log.
func TestNodeDiagnosticsWhitelist(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(proxy(5), diagnosticsCapabilities, agentcontrol.CapabilityDiag)

	for _, refused := range []struct{ body, code string }{
		{`{"action":"rm -rf /"}`, "action_not_allowed"},
		{`{"action":"exec","params":{"command":"id"}}`, "action_not_allowed"},
		{`{"action":"service_status","params":{"service":"sshd"}}`, "invalid_params"},
		{`{"action":"service_status"}`, "invalid_params"},
		{`{"action":"forward.connect","params":{"hop_index":0}}`, "invalid_params"},
		{`{"action":"forward.listen","params":{"route_id":"r","hop_index":9}}`, "invalid_params"},
	} {
		response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", refused.body)
		require.Equal(t, http.StatusBadRequest, response.Code, refused.body)
		assert.Equal(t, refused.code, errorCode(t, response), refused.body)
	}
	for _, body := range []string{`{}`, `{"action":"  "}`, `not json`} {
		response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", body)
		assert.Equal(t, http.StatusBadRequest, response.Code, body)
		assert.Equal(t, "invalid_request", errorCode(t, response), body)
	}
	assert.Equal(t, http.StatusBadRequest, env.call(t, 7, http.MethodPost, "/nodes/gateway-5/diagnostics", `{"action":"service_status"}`).Code)
	assert.Empty(t, env.streams.operations(), "nothing reached the node")

	rows := env.audits(t)
	require.Len(t, rows, 6, "each refused call is audited; bad bodies and node names are not calls")
	assert.Equal(t, "node_diagnostic_refused", rows[0].Action)
	assert.Equal(t, "node_diagnostic_refused", rows[1].Action)
	assert.Equal(t, "node_diagnostic_service_status", rows[2].Action)
	assert.Equal(t, "node_diagnostic_forward_connect", rows[4].Action)
	assert.Equal(t, "node_diagnostic_forward_listen", rows[5].Action)
	for _, row := range rows {
		assert.Equal(t, 2, row.Status)
		assert.Equal(t, "refused", auditContent(t, row)["outcome"])
	}
}

// A node whose Agent is offline, or does not advertise agent.diagnostic
// (or diag.v1 for the forward checks), answers 409 with a code per cause.
func TestNodeDiagnosticsUnavailable(t *testing.T) {
	env := newDiagnosticsEnv(t)
	status := `{"action":"service_status","params":{"service":"anix-agent"}}`
	check := `{"action":"forward.listen","params":{"route_id":"r1","hop_index":0}}`

	response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status)
	require.Equal(t, http.StatusConflict, response.Code)
	assert.Equal(t, "node_offline", errorCode(t, response))

	env.streams.connect(proxy(5), "agent.ping")
	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status)
	require.Equal(t, http.StatusConflict, response.Code)
	assert.Equal(t, "diagnostics_unsupported", errorCode(t, response))

	env.streams.connect(proxy(5), diagnosticsCapabilities)
	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", check)
	require.Equal(t, http.StatusConflict, response.Code)
	assert.Equal(t, "diagnostics_unsupported", errorCode(t, response))
	assert.Contains(t, response.Body.String(), "diag.v1")

	assert.Equal(t, http.StatusNotFound, env.call(t, 7, http.MethodPost, "/nodes/proxy-99/diagnostics", status).Code)
	assert.Empty(t, env.streams.operations())

	rows := env.audits(t)
	require.Len(t, rows, 4)
	assert.Equal(t, "node_offline", auditContent(t, rows[0])["outcome"])
	assert.Equal(t, "diagnostics_unsupported", auditContent(t, rows[1])["outcome"])
	assert.Equal(t, "diagnostics_unsupported", auditContent(t, rows[2])["outcome"])
	assert.Equal(t, "node_not_found", auditContent(t, rows[3])["outcome"])
	var tasks int64
	require.NoError(t, env.db.Model(&model.AgentDiagnosticTask{}).Count(&tasks).Error)
	assert.Zero(t, tasks, "a task that could not be sent leaves no row")
}

// Reads are for any administrator; a restart needs a super administrator,
// and a refusal sends nothing but is audited.
func TestNodeDiagnosticsRestartNeedsSuperAdmin(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(proxy(5), diagnosticsCapabilities)
	env.streams.answer = func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return succeeded(`{"success":true,"output":"restarted","duration_ms":9}`)
	}
	restart := `{"action":"service_restart","params":{"service":"anix-agent"}}`

	response := env.call(t, 8, http.MethodPost, "/nodes/proxy-5/diagnostics", restart)
	require.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "super_admin_required", errorCode(t, response))
	assert.Empty(t, env.streams.operations())

	response = env.call(t, 8, http.MethodPost, "/nodes/proxy-5/diagnostics", `{"action":"service_status","params":{"service":"anix-agent"}}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", restart)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	result := decodeData[kernelnodeops.NodeDiagnosticResult](t, response)
	assert.True(t, result.Success)
	assert.Equal(t, "restarted", result.Output)
	assert.Equal(t, "proxy-5", result.Node)

	rows := env.audits(t)
	require.Len(t, rows, 3)
	assert.Equal(t, "node_diagnostic_service_restart", rows[0].Action)
	assert.Equal(t, "forbidden", auditContent(t, rows[0])["outcome"])
	assert.Equal(t, 2, rows[0].Status)
	assert.Equal(t, "completed", auditContent(t, rows[2])["outcome"])
	assert.Equal(t, 1, rows[2].Status)
}

// A task is sent on the stream with its parameters normalized and its time
// bound, the result is answered in the request, a proxy node's task row is
// completed, and the operation log row ends as completed.
func TestNodeDiagnosticsRunOnTheStream(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(proxy(5), diagnosticsCapabilities)
	env.streams.answer = func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return succeeded(`{"success":true,"output":"line 1\napi_key=abcdef0123456789\nline 3","duration_ms":12}`)
	}

	response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics",
		`{"action":"log_tail","params":{"service":"anix-agent","lines":5000,"extra":"dropped"},"timeout_seconds":20}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	result := decodeData[kernelnodeops.NodeDiagnosticResult](t, response)
	assert.True(t, result.Success)
	assert.EqualValues(t, 12, result.DurationMS)
	assert.Equal(t, "log_tail", result.Action)
	assert.EqualValues(t, 1000, result.Params["lines"], "the whitelist caps the lines")
	assert.NotContains(t, result.Output, "abcdef0123456789", "node text is scrubbed")
	assert.Contains(t, result.Output, "line 1")

	sent := env.streams.operations()
	require.Len(t, sent, 1)
	assert.Equal(t, kernelnodeops.AgentDiagnosticOperation, sent[0].GetKind())
	assert.Equal(t, result.TaskID, sent[0].GetOperationId())
	assert.NotZero(t, sent[0].GetDeadlineUnixMs())
	var payload struct {
		Task struct {
			Type    string         `json:"type"`
			Action  string         `json:"action"`
			Params  map[string]any `json:"params"`
			Timeout int            `json:"timeout"`
		} `json:"task"`
	}
	require.NoError(t, json.Unmarshal(sent[0].GetPayloadJson(), &payload))
	assert.Equal(t, "diagnostic", payload.Task.Type)
	assert.Equal(t, 20, payload.Task.Timeout)
	assert.Equal(t, map[string]any{"service": "anix-agent", "lines": float64(1000)}, payload.Task.Params)

	var row model.AgentDiagnosticTask
	require.NoError(t, env.db.Where("task_id = ?", result.TaskID).First(&row).Error)
	assert.Equal(t, uint(5), row.NodeID)
	assert.Equal(t, model.AgentDiagnosticTaskStatusCompleted, row.Status)
	assert.True(t, row.Success)

	rows := env.audits(t)
	require.Len(t, rows, 1)
	content := auditContent(t, rows[0])
	assert.Equal(t, "completed", content["outcome"])
	assert.Equal(t, result.TaskID, content["task_id"])
	assert.Equal(t, "proxy-5", content["node"])
	assert.NotContains(t, rows[0].Content, "line 1", "the log row never carries the node's output")
	assert.Equal(t, "proxy_node", rows[0].TargetType)
	assert.Equal(t, 1, rows[0].Status)

	// The default is 30 seconds.
	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", `{"action":"service_status","params":{"service":"anix-agent"}}`)
	require.Equal(t, http.StatusOK, response.Code)
	sent = env.streams.operations()
	require.Len(t, sent, 2)
	require.NoError(t, json.Unmarshal(sent[1].GetPayloadJson(), &payload))
	assert.Equal(t, 30, payload.Task.Timeout)
}

// A task the agent ran and that failed is an answer (200, success false);
// an agent that refuses it is 502, and one that never answers is 504 after
// the request's time.
func TestNodeDiagnosticsAgentOutcomes(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(proxy(5), diagnosticsCapabilities)
	status := `{"action":"service_status","params":{"service":"anix-agent"}}`

	env.streams.answer = func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return succeeded(`{"success":false,"output":"inactive (dead)","error":"exit status 3","duration_ms":4}`)
	}
	response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status)
	require.Equal(t, http.StatusOK, response.Code)
	result := decodeData[kernelnodeops.NodeDiagnosticResult](t, response)
	assert.False(t, result.Success)
	assert.Equal(t, "exit status 3", result.Error)
	assert.Equal(t, "inactive (dead)", result.Output)

	env.streams.answer = func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return &agentv1pb.ObservedState{Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, Message: `agent.diagnostic: service "gost" is not allowed`}
	}
	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status)
	require.Equal(t, http.StatusOK, response.Code)
	result = decodeData[kernelnodeops.NodeDiagnosticResult](t, response)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "is not allowed")

	env.streams.answer = nil
	env.streams.refuse = "busy"
	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status)
	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.Equal(t, "agent_error", errorCode(t, response))
	assert.Contains(t, response.Body.String(), "busy")

	env.streams.refuse = ""
	started := time.Now()
	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", `{"action":"service_status","params":{"service":"anix-agent"},"timeout_seconds":1}`)
	require.Equal(t, http.StatusGatewayTimeout, response.Code, response.Body.String())
	assert.Equal(t, "diagnostic_timeout", errorCode(t, response))
	assert.Less(t, time.Since(started), 5*time.Second, "the answer comes at the request's own time")

	var failed int64
	require.NoError(t, env.db.Model(&model.AgentDiagnosticTask{}).Where("status = ?", model.AgentDiagnosticTaskStatusFailed).Count(&failed).Error)
	assert.EqualValues(t, 4, failed, "a failed run, a failed state, the nack and the timeout are failed rows")
	rows := env.audits(t)
	require.Len(t, rows, 4)
	assert.Equal(t, "failed", auditContent(t, rows[0])["outcome"])
	assert.Equal(t, "agent_error", auditContent(t, rows[2])["outcome"])
	assert.Equal(t, "timeout", auditContent(t, rows[3])["outcome"])
}

// The time of a request is 1 to 120 seconds; anything else is refused.
func TestNodeDiagnosticsTimeoutBounds(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(proxy(5), diagnosticsCapabilities)
	env.streams.answer = func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return succeeded(`{"success":true,"output":"ok"}`)
	}
	for _, seconds := range []string{"0", "-1", "121", "3600"} {
		response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics",
			`{"action":"service_status","params":{"service":"anix-agent"},"timeout_seconds":`+seconds+`}`)
		require.Equal(t, http.StatusBadRequest, response.Code, seconds)
		assert.Equal(t, "invalid_request", errorCode(t, response))
	}
	response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics",
		`{"action":"service_status","params":{"service":"anix-agent"},"timeout_seconds":120}`)
	require.Equal(t, http.StatusOK, response.Code)
	sent := env.streams.operations()
	require.Len(t, sent, 1)
	assert.Less(t, time.Until(time.UnixMilli(sent[0].GetDeadlineUnixMs())), 121*time.Second)

	assert.Equal(t, kernelnodeops.DefaultNodeDiagnosticTimeout, kernelnodeops.ClampNodeDiagnosticTimeout(0))
	assert.Equal(t, 5*time.Second, kernelnodeops.ClampNodeDiagnosticTimeout(5*time.Second))
	assert.Equal(t, kernelnodeops.MaxNodeDiagnosticTimeout, kernelnodeops.ClampNodeDiagnosticTimeout(time.Hour))
}

// A forward node runs the generic actions and the forward checks; its
// checks answer the structured result, and none of it is a proxy node's
// task row.
func TestNodeDiagnosticsForwardNode(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(forward(5), diagnosticsCapabilities, agentcontrol.CapabilityDiag)
	env.streams.answer = func(operation *agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return succeeded(`{"success":true,"output":"1 upstream reachable","duration_ms":4,"result":{"check":"forward.connect","route_id":"r1","hop_index":1,"status":"ok","items":[{"target":"198.51.100.10:443","protocol":"tcp","status":"ok","rtt_us":900}]}}`)
	}

	response := env.call(t, 7, http.MethodPost, "/nodes/forward-5/diagnostics",
		`{"action":"forward.connect","params":{"route_id":"r1","hop_index":1,"timeout_ms":99999,"extra":"dropped"}}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Data struct {
			Node    string         `json:"node"`
			Success bool           `json:"success"`
			Params  map[string]any `json:"params"`
			Result  struct {
				Status string `json:"status"`
				Items  []struct {
					Target string `json:"target"`
					RTT    int    `json:"rtt_us"`
				} `json:"items"`
			} `json:"result"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "forward-5", body.Data.Node)
	assert.True(t, body.Data.Success)
	assert.Equal(t, "ok", body.Data.Result.Status)
	require.Len(t, body.Data.Result.Items, 1)
	assert.Equal(t, 900, body.Data.Result.Items[0].RTT)
	assert.EqualValues(t, 10000, body.Data.Params["timeout_ms"], "the check's timeout is clamped")

	var tasks int64
	require.NoError(t, env.db.Model(&model.AgentDiagnosticTask{}).Count(&tasks).Error)
	assert.Zero(t, tasks, "the table is keyed by proxy node: forward nodes and checks keep no row")
	rows := env.audits(t)
	require.Len(t, rows, 1)
	assert.Equal(t, "forward_node", rows[0].TargetType)
	assert.Equal(t, "node_diagnostic_forward_connect", rows[0].Action)

	// The proxy node of the same id has no Agent here.
	response = env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", `{"action":"service_status","params":{"service":"anix-agent"}}`)
	assert.Equal(t, http.StatusConflict, response.Code)
}

// One diagnostic of a node runs at a time.
func TestNodeDiagnosticsOnePerNode(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(proxy(5), diagnosticsCapabilities)
	env.streams.gate = make(chan struct{})
	env.streams.answer = func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return succeeded(`{"success":true,"output":"ok"}`)
	}
	status := `{"action":"service_status","params":{"service":"anix-agent"}}`
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status) }()
	require.Eventually(t, func() bool { return len(env.audits(t)) == 1 }, 5*time.Second, 5*time.Millisecond)
	// Give the first request time to take the node.
	time.Sleep(50 * time.Millisecond)

	response := env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status)
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	assert.Equal(t, "diagnostic_busy", errorCode(t, response))

	close(env.streams.gate)
	assert.Equal(t, http.StatusOK, (<-first).Code)
	assert.Equal(t, http.StatusOK, env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", status).Code, "the node is free again")
}

// The listing says what the node's session runs and gives the newest
// tasks; only a super administrator can restart.
func TestNodeDiagnosticsList(t *testing.T) {
	env := newDiagnosticsEnv(t)
	env.streams.connect(proxy(5), diagnosticsCapabilities)
	env.streams.answer = func(*agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		return succeeded(`{"success":true,"output":"active (running)","duration_ms":3}`)
	}
	type listing struct {
		Node         string `json:"node"`
		Connected    bool   `json:"connected"`
		AgentVersion string `json:"agent_version"`
		Diagnostics  bool   `json:"diagnostics"`
		Forward      bool   `json:"forward_checks"`
		Actions      []struct {
			Action   string `json:"action"`
			Mutating bool   `json:"mutating"`
			Forward  bool   `json:"forward"`
		} `json:"actions"`
		Services   []string `json:"services"`
		LogTailMax int      `json:"log_tail_max"`
		Timeout    struct {
			Default int `json:"default"`
			Max     int `json:"max"`
		} `json:"timeout_seconds"`
		CanRestart bool `json:"can_restart"`
		History    bool `json:"history"`
		Tasks      []struct {
			TaskID  string `json:"task_id"`
			Action  string `json:"action"`
			Status  string `json:"status"`
			Success bool   `json:"success"`
			Output  string `json:"output"`
		} `json:"tasks"`
	}

	idle := decodeData[listing](t, env.call(t, 7, http.MethodGet, "/nodes/proxy-5/diagnostics", ""))
	assert.True(t, idle.Connected)
	assert.True(t, idle.Diagnostics)
	assert.False(t, idle.Forward)
	assert.Equal(t, "4.2.0", idle.AgentVersion)
	var names []string
	for _, action := range idle.Actions {
		names = append(names, action.Action)
		assert.False(t, action.Forward)
		assert.Equal(t, action.Action == "service_restart", action.Mutating)
	}
	assert.Equal(t, []string{"log_tail", "service_restart", "service_status"}, names)
	assert.Equal(t, []string{"anix-agent"}, idle.Services, "a proxy node that does not forward runs its Agent's unit")
	assert.Equal(t, 1000, idle.LogTailMax)
	assert.Equal(t, 30, idle.Timeout.Default)
	assert.Equal(t, 120, idle.Timeout.Max)
	assert.True(t, idle.CanRestart)
	assert.True(t, idle.History)
	assert.Empty(t, idle.Tasks)

	for range 3 {
		require.Equal(t, http.StatusOK, env.call(t, 7, http.MethodPost, "/nodes/proxy-5/diagnostics", `{"action":"service_status","params":{"service":"anix-agent"}}`).Code)
	}
	listed := decodeData[listing](t, env.call(t, 8, http.MethodGet, "/nodes/proxy-5/diagnostics?limit=2", ""))
	assert.False(t, listed.CanRestart, "staff read, a super administrator restarts")
	require.Len(t, listed.Tasks, 2)
	assert.Equal(t, "service_status", listed.Tasks[0].Action)
	assert.Equal(t, "completed", listed.Tasks[0].Status)
	assert.True(t, listed.Tasks[0].Success)
	assert.Equal(t, "active (running)", listed.Tasks[0].Output)

	assert.Equal(t, http.StatusBadRequest, env.call(t, 7, http.MethodGet, "/nodes/proxy-5/diagnostics?limit=500", "").Code)
	assert.Equal(t, http.StatusNotFound, env.call(t, 7, http.MethodGet, "/nodes/proxy-99/diagnostics", "").Code)

	// A forward node keeps no history; an offline node lists nothing to run.
	other := decodeData[listing](t, env.call(t, 7, http.MethodGet, "/nodes/forward-5/diagnostics", ""))
	assert.False(t, other.Connected)
	assert.False(t, other.History)
	assert.Empty(t, other.Actions)
	assert.Empty(t, other.Tasks)

	env.streams.connect(forward(5), diagnosticsCapabilities, agentcontrol.CapabilityDiag)
	other = decodeData[listing](t, env.call(t, 7, http.MethodGet, "/nodes/forward-5/diagnostics", ""))
	assert.True(t, other.Forward)
	var forwardChecks int
	for _, action := range other.Actions {
		if action.Forward {
			forwardChecks++
		}
	}
	assert.Equal(t, 4, forwardChecks)
	assert.Empty(t, other.Services, "a forward node runs no service of its own since gost went")

	// A forwarding proxy node runs only its Agent.
	env.streams.connect(proxy(5), diagnosticsCapabilities, agentcontrol.CapabilityDiag)
	forwarding := decodeData[listing](t, env.call(t, 7, http.MethodGet, "/nodes/proxy-5/diagnostics", ""))
	assert.Equal(t, []string{"anix-agent"}, forwarding.Services)
}
