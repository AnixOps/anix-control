package native

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The node synchronization, Agent Control and agent routes act on the
// agents' live connections, which only the kernel holds: the package asks
// the kernel through KernelNodeOps (node.sync, agent.operation,
// agent.diagnostic and the agent session RPCs) and answers what the kernel
// reports, as the legacy routes answer it. The kernel renders what those
// routes show of a session, a monitor snapshot, an acknowledgement and a
// task row, so both answer the same bytes (docs/architecture/
// node-ops-service.md section 6.3).

// proxyNode is a proxy node reference.
func proxyNode(id uint) *kernelnodeopsv1.NodeRef {
	return &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: uint64(id)}
}

// rawJSON is a JSON document the kernel rendered, or null.
func rawJSON(document []byte) json.RawMessage {
	if len(document) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(document)
}

// operationAck is the agent's acknowledgement as the legacy routes show it
// (agentv1.OperationAck, encoded by encoding/json).
func operationAck(operationID string, ack *kernelnodeopsv1.AgentAck) *agentv1pb.OperationAck {
	if ack == nil {
		return nil
	}
	return &agentv1pb.OperationAck{
		OperationId: operationID, Accepted: ack.GetAccepted(), Error: ack.GetError(), AcceptedAtUnixMs: ack.GetAcceptedAtUnixMs(),
		SessionId: ack.GetSessionId(), Revision: ack.GetRevision(),
	}
}

// SyncNode is POST /api/v2/admin/nodes/:id/sync: the kernel rebuilds the
// node's desired configuration and pushes it to the node's agent on the
// Agent Control stream (node.sync, forced); the answer is the agent's
// acknowledgement, or, for a node on the legacy transports, that its next
// pull reads the configuration.
func (s *Service) SyncNode(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	nodeID, ok := pathUint(request, "id")
	if !ok {
		return message(http.StatusBadRequest, "无效的节点ID")
	}
	operation, err := s.submit(ctx, request, s.requestID(request, fmt.Sprintf("node.sync:proxy-%d", nodeID), false),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_SyncNode{SyncNode: &kernelnodeopsv1.SyncNode{Node: proxyNode(nodeID), Force: true}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED, defaultAckTimeout+ackWait)
	if status.Code(err) == codes.NotFound {
		return message(http.StatusNotFound, "节点不存在")
	}
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	result := operation.GetResult().GetNodeSync()
	switch {
	case operation.GetError().GetCode() == kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE:
		return message(http.StatusNotFound, "节点不存在")
	case result == nil:
		if state(operation) == kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED {
			return message(http.StatusInternalServerError, "同步失败")
		}
		return messageError(http.StatusBadGateway, "Agent Control 同步下发失败", "the agent did not acknowledge in time")
	case result.GetChannel() != kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL:
		return s.panel(map[string]any{"message": "节点未连接 Agent Control，将保留旧版周期拉取同步", "transport": "legacy-poll"})
	case result.GetAck() == nil && state(operation) == kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED:
		return messageError(http.StatusBadGateway, "Agent Control 同步下发失败", dispatchError(operation))
	case result.GetSnapshot():
		return s.panel(map[string]any{
			"message": "配置快照已通过 AnixOps Agent Control 下发", "transport": "agent-control-grpc",
			"config_revision": result.GetConfigRevision(), "config_hash": result.GetConfigHash(),
		})
	}
	return s.panel(map[string]any{
		"message": "同步操作已由 AnixOps Agent 接收", "transport": "agent-control-grpc",
		"operation_id": result.GetAgentOperationId(), "revision": result.GetRevision(),
		"ack": operationAck(result.GetAgentOperationId(), result.GetAck()),
	})
}

// GetAgentControl is GET /api/v2/admin/nodes/:id/agent-control: the node's
// Agent Control session and the last state its agent reported.
func (s *Service) GetAgentControl(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	nodeID, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
	if err != nil {
		return message(http.StatusBadRequest, "无效的节点ID")
	}
	session, err := s.NodeOps.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: nodeID})
	if status.Code(err) == codes.NotFound {
		return message(http.StatusNotFound, "节点不存在")
	}
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	data := map[string]any{"connected": false, "node_id": nodeID}
	if len(session.GetConnectionJson()) > 0 {
		data["connected"] = true
		data["connection"] = rawJSON(session.GetConnectionJson())
	}
	if len(session.GetObservedStateJson()) > 0 {
		data["observed_state"] = rawJSON(session.GetObservedStateJson())
	}
	return s.panel(data)
}

// agentControlOperationRequest is the kernel's: what the Agent Control
// operation route binds.
type agentControlOperationRequest struct {
	OperationID   string `json:"operation_id"`
	Kind          string `json:"kind" binding:"required"`
	Payload       any    `json:"payload"`
	TimeoutSecond int    `json:"timeout_seconds"`
}

// agentControlOperations are the operations the route sends.
var agentControlOperations = map[string]bool{"agent.ping": true, "node.reload": true, "users.reload": true}

// DispatchAgentControlOperation is POST
// /api/v2/admin/nodes/:id/agent-control/operations: one bounded operation
// on the node's Agent Control stream (agent.operation); the answer is the
// operation sent and the agent's acknowledgement.
func (s *Service) DispatchAgentControlOperation(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	nodeID, ok := pathUint(request, "id")
	if !ok {
		return message(http.StatusBadRequest, "无效的节点ID")
	}
	session, err := s.NodeOps.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: uint64(nodeID)})
	if status.Code(err) == codes.NotFound {
		return message(http.StatusNotFound, "节点不存在")
	}
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var req agentControlOperationRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return messageError(http.StatusBadRequest, "参数错误", err.Error())
	}
	req.Kind = strings.TrimSpace(req.Kind)
	if !agentControlOperations[req.Kind] {
		return message(http.StatusBadRequest, "不支持的 Agent Control 操作")
	}
	if !session.GetConnected() {
		return message(http.StatusConflict, "节点未连接 Agent Control")
	}
	var payload []byte
	if req.Payload != nil {
		if payload, err = json.Marshal(req.Payload); err != nil {
			return messageError(http.StatusBadGateway, "Agent Control 操作下发失败", fmt.Sprintf("encode operation payload: %v", err))
		}
	}
	seconds := uint32(0)
	if req.TimeoutSecond > 0 {
		seconds = uint32(min(req.TimeoutSecond, int(maxAckTimeout/time.Second))) // #nosec G115 -- capped at 60.
	}
	operation, err := s.submit(ctx, request, s.requestID(request, fmt.Sprintf("agent.op:%d", nodeID), false),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_AgentControlOperation{AgentControlOperation: &kernelnodeopsv1.AgentControlOperation{
			NodeId: uint64(nodeID), Kind: req.Kind, PayloadJson: payload, TimeoutSeconds: seconds, AgentOperationId: req.OperationID,
		}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED, ackTimeout(seconds)+ackWait)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	result := operation.GetResult().GetAgentOperation()
	if result == nil || result.GetAck() == nil {
		code := http.StatusBadGateway
		errorCode := operation.GetError().GetCode()
		if errorCode == kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED || errorCode == kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED ||
			state(operation) == kernelnodeopsv1.OperationState_OPERATION_STATE_CANCELLED {
			code = http.StatusGatewayTimeout
		}
		detail := dispatchError(operation)
		if operation.GetError() == nil {
			detail = "the agent did not acknowledge in time"
		}
		return messageError(code, "Agent Control 操作下发失败", detail)
	}
	desired := &agentv1pb.DesiredOperation{
		OperationId: result.GetAgentOperationId(), Kind: req.Kind, Revision: result.GetAck().GetRevision(), PayloadJson: payload,
		DeadlineUnixMs: result.GetDeadlineUnixMs(),
	}
	return s.panel(map[string]any{"operation": desired, "ack": operationAck(result.GetAgentOperationId(), result.GetAck())})
}

// ListAgents is GET /api/v2/admin/agent/list: the agents connected on the
// legacy WebSocket, as the kernel renders them.
func (s *Service) ListAgents(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	sessions, err := s.NodeOps.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{Transport: kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_WEBSOCKET})
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	agents := make([]json.RawMessage, 0, len(sessions.GetSessions()))
	for _, session := range sessions.GetSessions() {
		if len(session.GetAdminJson()) == 0 {
			// A kernel that does not render the list (before M3-1).
			return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
		}
		agents = append(agents, session.GetAdminJson())
	}
	return s.panel(map[string]any{"agents": agents})
}

// GetMonitor is GET /api/v2/admin/agent/monitor: the last monitor snapshot
// a node's WebSocket agent posted.
func (s *Service) GetMonitor(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	nodeID, err := strconv.ParseUint(query(request, "node_id"), 10, 32)
	if err != nil || nodeID == 0 {
		return errorAnswer(http.StatusBadRequest, "valid node_id is required")
	}
	monitor, err := s.NodeOps.GetAgentMonitor(ctx, &kernelnodeopsv1.GetAgentMonitorRequest{NodeId: nodeID})
	if status.Code(err) == codes.NotFound {
		return errorAnswer(http.StatusNotFound, "monitor data not found")
	}
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if !monitor.GetFound() {
		return errorAnswer(http.StatusNotFound, "monitor data not found")
	}
	if len(monitor.GetMonitorJson()) == 0 {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	return s.panel(rawJSON(monitor.GetMonitorJson()))
}

// agentDiagnosticTaskType is the only task type sent to agents.
const agentDiagnosticTaskType = "diagnostic"

// CreateTaskRequest is the kernel's handler.CreateTaskRequest; its name
// appears in binding errors.
type CreateTaskRequest struct {
	NodeID  uint           `json:"node_id" binding:"required"`
	Type    string         `json:"type" binding:"required"`
	Action  string         `json:"action" binding:"required"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// ExecuteCommandRequest is the kernel's handler.ExecuteCommandRequest.
type ExecuteCommandRequest struct {
	NodeID  uint           `json:"node_id" binding:"required"`
	Action  string         `json:"action" binding:"required"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// CreateTask is POST /api/v2/admin/agent/tasks: one whitelisted diagnostic
// action for a node's agent (agent.diagnostic); the answer is the agent's
// acknowledgement and the task's row.
func (s *Service) CreateTask(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req CreateTaskRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return errorAnswer(http.StatusBadRequest, err.Error())
	}
	return s.diagnose(ctx, request, req)
}

// ExecuteCommand is POST /api/v2/admin/agent/execute: CreateTask for a
// diagnostic action, without a type.
func (s *Service) ExecuteCommand(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req ExecuteCommandRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return errorAnswer(http.StatusBadRequest, err.Error())
	}
	return s.diagnose(ctx, request, CreateTaskRequest{NodeID: req.NodeID, Type: agentDiagnosticTaskType, Action: req.Action, Params: req.Params, Timeout: req.Timeout})
}

func (s *Service) diagnose(ctx context.Context, request pluginhostsdk.NativeRequest, req CreateTaskRequest) (pluginhostsdk.NativeResponse, error) {
	if strings.TrimSpace(req.Type) != agentDiagnosticTaskType {
		return errorAnswer(http.StatusBadRequest, fmt.Sprintf("task type %q is not allowed: only %q tasks are sent to agents", req.Type, agentDiagnosticTaskType))
	}
	if req.Timeout < 0 || uint64(req.Timeout) > uint64(^uint32(0)) {
		// The kernel's operation takes no negative timeout; the legacy
		// handler sends the task as asked.
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	params, err := json.Marshal(req.Params)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	seconds := uint32(req.Timeout) // #nosec G115 -- checked above.
	operation, err := s.submit(ctx, request, s.requestID(request, fmt.Sprintf("agent.diag:%d", req.NodeID), false),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RunAgentDiagnostic{RunAgentDiagnostic: &kernelnodeopsv1.RunAgentDiagnostic{
			NodeId: uint64(req.NodeID), Action: req.Action, ParamsJson: params, TimeoutSeconds: seconds,
		}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED, ackTimeout(seconds)+ackWait)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	result := operation.GetResult().GetAgentDiagnostic()
	if result == nil {
		switch operation.GetError().GetCode() {
		case kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE:
			return errorAnswer(http.StatusBadRequest, "node offline")
		case kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED:
			return errorAnswer(http.StatusBadRequest, operation.GetError().GetMessage())
		}
		return errorAnswer(http.StatusInternalServerError, operation.GetError().GetMessage())
	}
	onStream := operation.GetChannel() == kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL
	sent := result.GetDispatchError() == "" || result.GetLegacyFallback()
	if !sent {
		answer := map[string]any{
			"error": "send failed", "task_id": result.GetTaskId(), "message_id": result.GetMessageId(),
			"dispatch_err": result.GetDispatchError(), "data": rawJSON(result.GetTaskJson()),
		}
		if onStream {
			answer["channel"] = "agent_control"
		}
		return jsonAnswer(http.StatusInternalServerError, answer)
	}
	if result.GetLegacyFallback() {
		return s.panel(map[string]any{
			"message": "task sent with legacy fallback", "task_id": result.GetTaskId(), "node_id": req.NodeID, "success": true,
			"output": "task dispatched (legacy fallback)", "duration_ms": int64(0), "message_id": result.GetMessageId(),
			"ack_received": false, "dispatch_err": result.GetDispatchError(), "data": rawJSON(result.GetTaskJson()),
		})
	}
	answer := map[string]any{
		"message": "task sent", "task_id": result.GetTaskId(), "node_id": req.NodeID, "success": true, "output": "task dispatched",
		"duration_ms": int64(0), "message_id": result.GetMessageId(), "ack_received": true, "ack": rawJSON(result.GetAckJson()),
		"data": rawJSON(result.GetTaskJson()),
	}
	if onStream {
		answer["channel"] = "agent_control"
	}
	return s.panel(answer)
}
