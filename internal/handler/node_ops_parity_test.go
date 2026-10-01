package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	controlgrpc "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The legacy node sync, Agent Control operation and agent diagnostic
// routes answer exactly what they answered before NO-6 moved them onto the
// KernelNodeOps functions (node-ops-service.md section 3.3: "the legacy
// handlers move onto the same functions"). These tests pin the answers:
// every key, every static value, and the shape of every generated one.

var (
	taskIDPattern    = regexp.MustCompile(`^task-\d+$`)
	messageIDPattern = regexp.MustCompile(`^msg-\d+$`)
)

// panelData decodes a panel answer and returns its data object.
func panelData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope), recorder.Body.String())
	require.Equal(t, float64(0), envelope["code"])
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok, "data is an object: %s", recorder.Body.String())
	return data
}

// rawData returns the data object's JSON as the route wrote it.
func rawData(t *testing.T, recorder *httptest.ResponseRecorder) json.RawMessage {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return envelope.Data
}

func legacyAck(operationID string) *agentv1pb.OperationAck {
	return &agentv1pb.OperationAck{
		OperationId: operationID, Accepted: true, AcceptedAtUnixMs: 1700000000000, SessionId: "session-test", Revision: 7,
	}
}

func TestSyncProtocolLegacyAnswers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := createAgentControlHandlerTestNode(t)
	post := func(h *NodeHandler) *httptest.ResponseRecorder {
		router := gin.New()
		router.POST("/nodes/:id/sync", h.SyncProtocol)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/nodes/%d/sync", node.ID), nil))
		return recorder
	}

	t.Run("agent on the control stream", func(t *testing.T) {
		fake := &fakeNodeAgentControl{connected: true, snapshot: controlgrpc.AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "session-test"}}
		fake.ack = legacyAck("")
		fake.ackFromOperation = true
		h := NewNodeHandler()
		h.agentControl = fake
		recorder := post(h)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.NotNil(t, fake.received)
		assert.Equal(t, "node.reload", fake.received.Kind)
		assert.Empty(t, fake.received.PayloadJson)
		assert.Regexp(t, taskIDPattern, fake.received.OperationId)
		expected := fmt.Sprintf(`{"ack":{"operation_id":%q,"accepted":true,"accepted_at_unix_ms":1700000000000,"session_id":"session-test","revision":7},`+
			`"message":"同步操作已由 AnixOps Agent 接收","operation_id":%q,"revision":7,"transport":"agent-control-grpc"}`,
			fake.received.OperationId, fake.received.OperationId)
		assert.JSONEq(t, expected, string(rawData(t, recorder)))
		assert.Equal(t, []string{"ack", "message", "operation_id", "revision", "transport"}, sortedKeys(panelData(t, recorder)))
		// The legacy route writes the rows the native operation writes.
		var desired model.KernelNodeDesiredConfig
		require.NoError(t, initTestDB().Where("node_kind = ? AND node_id = ?", "proxy", node.ID).First(&desired).Error)
		assert.Equal(t, uint64(1), desired.Revision)
		assert.Len(t, desired.ConfigHash, 64)
	})

	t.Run("agent not on the stream", func(t *testing.T) {
		h := NewNodeHandler()
		h.agentControl = &fakeNodeAgentControl{}
		recorder := post(h)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.JSONEq(t, `{"message":"节点未连接 Agent Control，将保留旧版周期拉取同步","transport":"legacy-poll"}`, string(rawData(t, recorder)))
	})

	t.Run("dispatch failure", func(t *testing.T) {
		h := NewNodeHandler()
		h.agentControl = &fakeNodeAgentControl{connected: true, err: errors.New("agent node 1 is not connected")}
		recorder := post(h)
		require.Equal(t, http.StatusBadGateway, recorder.Code)
		assert.JSONEq(t, `{"message":"Agent Control 同步下发失败","error":"agent node 1 is not connected"}`, recorder.Body.String())
	})

	t.Run("unknown node", func(t *testing.T) {
		h := NewNodeHandler()
		h.agentControl = &fakeNodeAgentControl{connected: true}
		router := gin.New()
		router.POST("/nodes/:id/sync", h.SyncProtocol)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/nodes/999999/sync", nil))
		require.Equal(t, http.StatusNotFound, recorder.Code)
		assert.JSONEq(t, `{"message":"节点不存在"}`, recorder.Body.String())
	})
}

func TestDispatchAgentControlOperationLegacyAnswers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := createAgentControlHandlerTestNode(t)
	post := func(h *NodeHandler, body string) *httptest.ResponseRecorder {
		router := gin.New()
		router.POST("/nodes/:id/agent-control/operations", h.DispatchAgentControlOperation)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/nodes/%d/agent-control/operations", node.ID), strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		return recorder
	}

	t.Run("accepted", func(t *testing.T) {
		fake := &fakeNodeAgentControl{connected: true, snapshot: controlgrpc.AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "session-test"}, ack: legacyAck("manual-ping-1")}
		h := NewNodeHandler()
		h.agentControl = fake
		before := time.Now()
		recorder := post(h, `{"operation_id":"manual-ping-1","kind":"agent.ping","payload":{"source":"test","n":1},"timeout_seconds":2}`)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		data := panelData(t, recorder)
		assert.Equal(t, []string{"ack", "operation"}, sortedKeys(data))
		operation := data["operation"].(map[string]any)
		deadline := int64(operation["deadline_unix_ms"].(float64))
		assert.InDelta(t, before.Add(2*time.Second).UnixMilli(), deadline, 1500)
		expected := fmt.Sprintf(`{"ack":{"operation_id":"manual-ping-1","accepted":true,"accepted_at_unix_ms":1700000000000,"session_id":"session-test","revision":7},`+
			`"operation":{"operation_id":"manual-ping-1","kind":"agent.ping","revision":7,"payload_json":"eyJuIjoxLCJzb3VyY2UiOiJ0ZXN0In0=","deadline_unix_ms":%d}}`, deadline)
		assert.JSONEq(t, expected, string(rawData(t, recorder)))
		assert.JSONEq(t, `{"n":1,"source":"test"}`, string(fake.received.PayloadJson))
	})

	t.Run("not connected", func(t *testing.T) {
		h := NewNodeHandler()
		h.agentControl = &fakeNodeAgentControl{}
		recorder := post(h, `{"kind":"agent.ping"}`)
		require.Equal(t, http.StatusConflict, recorder.Code)
		assert.JSONEq(t, `{"message":"节点未连接 Agent Control"}`, recorder.Body.String())
	})

	t.Run("unsupported kind", func(t *testing.T) {
		h := NewNodeHandler()
		h.agentControl = &fakeNodeAgentControl{connected: true}
		recorder := post(h, `{"kind":"plugin.install"}`)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.JSONEq(t, `{"message":"不支持的 Agent Control 操作"}`, recorder.Body.String())
	})

	t.Run("dispatch failure", func(t *testing.T) {
		h := NewNodeHandler()
		h.agentControl = &fakeNodeAgentControl{connected: true, err: errors.New("agent node 1 does not advertise capability \"agent.ping\"")}
		recorder := post(h, `{"kind":"agent.ping"}`)
		require.Equal(t, http.StatusBadGateway, recorder.Code)
		assert.JSONEq(t, `{"message":"Agent Control 操作下发失败","error":"agent node 1 does not advertise capability \"agent.ping\""}`, recorder.Body.String())
	})

	t.Run("dispatch timeout", func(t *testing.T) {
		h := NewNodeHandler()
		h.agentControl = &fakeNodeAgentControl{connected: true, err: fmt.Errorf("send: %w", errTimeoutForTest)}
		recorder := post(h, `{"kind":"agent.ping"}`)
		require.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	})
}

var errTimeoutForTest = contextDeadlineExceeded()

func TestCreateTaskLegacyAnswers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initTestDB()
	require.NoError(t, db.AutoMigrate(&model.AgentDiagnosticTask{}))
	node := &model.Node{Name: "diag-parity", Host: "127.0.0.1", Port: 443, APIKey: fmt.Sprintf("diag-parity-%d", time.Now().UnixNano()), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(node).Error)
	t.Cleanup(func() { _ = db.Delete(&model.Node{}, node.ID).Error })

	post := func(h *AgentHandler, path string, body any) *httptest.ResponseRecorder {
		router := gin.New()
		router.POST("/admin/agent/tasks", h.CreateTask)
		router.POST("/admin/agent/execute", h.ExecuteCommand)
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		return recorder
	}
	taskRowKeys := []string{"action", "created_at", "duration_ms", "id", "node_id", "params", "status", "success", "task_id", "timestamp"}

	t.Run("acknowledged", func(t *testing.T) {
		h, ackDone, cleanup := ackingAgentHandler(t, node.ID, true)
		defer cleanup()
		recorder := post(h, "/admin/agent/tasks", map[string]any{"node_id": node.ID, "type": "diagnostic", "action": "service_status", "params": map[string]any{"service": "gost"}, "timeout": 30})
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.NoError(t, <-ackDone)
		data := panelData(t, recorder)
		assert.Equal(t, []string{"ack", "ack_received", "data", "duration_ms", "message", "message_id", "node_id", "output", "success", "task_id"}, sortedKeys(data))
		assert.Equal(t, "task sent", data["message"])
		assert.Equal(t, "task dispatched", data["output"])
		assert.Equal(t, true, data["success"])
		assert.Equal(t, true, data["ack_received"])
		assert.Equal(t, float64(0), data["duration_ms"])
		assert.Equal(t, float64(node.ID), data["node_id"])
		assert.Regexp(t, taskIDPattern, data["task_id"])
		assert.Regexp(t, messageIDPattern, data["message_id"])
		ack := data["ack"].(map[string]any)
		assert.Equal(t, []string{"msg_id", "success", "timestamp"}, sortedKeys(ack))
		assert.Equal(t, data["message_id"], ack["msg_id"])
		assert.Equal(t, true, ack["success"])
		row := data["data"].(map[string]any)
		assert.Equal(t, taskRowKeys, sortedKeys(row))
		assert.Equal(t, data["task_id"], row["task_id"])
		assert.Equal(t, "dispatched", row["status"])
		assert.Equal(t, "service_status", row["action"])
		assert.JSONEq(t, `{"service":"gost"}`, row["params"].(string))
	})

	t.Run("legacy fallback after a missing acknowledgement", func(t *testing.T) {
		h, received, cleanup := ackingAgentHandler(t, node.ID, false)
		defer cleanup()
		recorder := post(h, "/admin/agent/execute", map[string]any{"node_id": node.ID, "action": "service_status", "params": map[string]any{"service": "gost"}, "timeout": 5})
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.NoError(t, <-received)
		data := panelData(t, recorder)
		assert.Equal(t, []string{"ack_received", "data", "dispatch_err", "duration_ms", "message", "message_id", "node_id", "output", "success", "task_id"}, sortedKeys(data))
		assert.Equal(t, "task sent with legacy fallback", data["message"])
		assert.Equal(t, "task dispatched (legacy fallback)", data["output"])
		assert.Equal(t, false, data["ack_received"])
		assert.Equal(t, true, data["success"])
		assert.Equal(t, "ack timeout after 100ms", data["dispatch_err"])
		row := data["data"].(map[string]any)
		assert.Equal(t, taskRowKeys, sortedKeys(row))
		assert.Equal(t, "dispatched", row["status"])
	})

	t.Run("node offline", func(t *testing.T) {
		recorder := post(NewAgentHandler(), "/admin/agent/tasks", map[string]any{"node_id": node.ID, "type": "diagnostic", "action": "service_status"})
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.JSONEq(t, `{"error":"node offline"}`, recorder.Body.String())
	})

	t.Run("action outside the whitelist", func(t *testing.T) {
		h, _, cleanup := ackingAgentHandler(t, node.ID, true)
		defer cleanup()
		recorder := post(h, "/admin/agent/tasks", map[string]any{"node_id": node.ID, "type": "diagnostic", "action": "rm -rf /"})
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.JSONEq(t, `{"error":"action \"rm -rf /\" is not in the diagnostic whitelist"}`, recorder.Body.String())
		var tasks int64
		require.NoError(t, db.Model(&model.AgentDiagnosticTask{}).Where("node_id = ? AND action = ?", node.ID, "rm -rf /").Count(&tasks).Error)
		assert.Zero(t, tasks)
	})

	t.Run("other task types", func(t *testing.T) {
		h, _, cleanup := ackingAgentHandler(t, node.ID, true)
		defer cleanup()
		recorder := post(h, "/admin/agent/tasks", map[string]any{"node_id": node.ID, "type": "shell", "action": "service_status"})
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.JSONEq(t, `{"error":"task type \"shell\" is not allowed: only \"diagnostic\" tasks are sent to agents"}`, recorder.Body.String())
	})
}

// ackingAgentHandler connects a WebSocket agent for nodeID that reads the
// task assignment and, when ack is true, acknowledges it. The channel
// reports what the agent read: nil when it saw a task assignment, and the
// legacy task message after a refused acknowledgement.
func ackingAgentHandler(t *testing.T, nodeID uint, ack bool) (*AgentHandler, <-chan error, func()) {
	t.Helper()
	h := NewAgentHandler()
	h.ackTimeout = 100 * time.Millisecond
	h.maxRetries = 0
	serverConn, clientConn, cleanup := newTestWebSocketPair(t)
	agentConn := &AgentConnection{NodeID: nodeID, WsConn: serverConn, LastSeen: time.Now()}
	h.connections.Store(nodeID, agentConn)
	done := make(chan error, 1)
	go func() {
		_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var outbound wsOutboundEnvelope
		if err := clientConn.ReadJSON(&outbound); err != nil {
			done <- err
			return
		}
		if outbound.Type != "task.assign" || !outbound.RequireAck {
			done <- fmt.Errorf("unexpected first message %q", outbound.Type)
			return
		}
		if !ack {
			var legacy wsOutboundEnvelope
			if err := clientConn.ReadJSON(&legacy); err != nil {
				done <- err
				return
			}
			if legacy.Type != "task" || legacy.RequireAck {
				done <- fmt.Errorf("unexpected legacy message %q", legacy.Type)
				return
			}
			done <- nil
			return
		}
		payload, _ := json.Marshal(wsAckPayload{MessageID: outbound.ID, Success: true, Timestamp: time.Now().Unix()})
		raw, _ := json.Marshal(wsInboundEnvelope{ID: "ack-" + outbound.ID, Type: "ack", NodeID: nodeID, Timestamp: time.Now().Unix(), Payload: payload})
		h.handleWebSocketMessage(agentConn, raw)
		done <- nil
	}()
	return h, done, cleanup
}

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortStrings(values []string) { sort.Strings(values) }

func contextDeadlineExceeded() error { return context.DeadlineExceeded }
