package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	controlgrpc "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/gin-gonic/gin"
)

const (
	defaultAgentControlOperationTimeout = 10 * time.Second
	maxAgentControlOperationTimeout     = 60 * time.Second
)

var allowedAgentControlOperations = map[string]struct{}{
	"agent.ping":   {},
	"node.reload":  {},
	"users.reload": {},
}

type nodeAgentControl interface {
	DispatchOperation(context.Context, uint32, *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error)
	Connection(uint32) (controlgrpc.AgentControlSnapshot, bool)
	ObservedState(uint32) (*agentv1pb.ObservedState, bool)
}

func defaultNodeAgentControl() nodeAgentControl {
	return controlgrpc.GetAgentControlManager()
}

type agentControlOperationRequest struct {
	OperationID   string `json:"operation_id"`
	Kind          string `json:"kind" binding:"required"`
	Payload       any    `json:"payload"`
	TimeoutSecond int    `json:"timeout_seconds"`
}

// proxyNodeStreams is the handler's Agent Control manager as the
// KernelNodeOps functions read it (agentstreams.Streams): the proxy nodes'
// streams, which is all this handler reaches. The manager's errors are
// shown as they are, so the routes' answers do not change. config is the
// process's streams when the manager is the process's, so the sync route
// pushes a ConfigSnapshot to an agent that negotiated config.v1
// (agentstreams.ConfigStreams); nil otherwise.
type proxyNodeStreams struct {
	control nodeAgentControl
	config  agentstreams.ConfigStreams
}

func (h *NodeHandler) streams() agentstreams.Streams {
	if h.agentControl == nil {
		return nil
	}
	if streams, ok := h.agentControl.(agentstreams.Streams); ok {
		return streams
	}
	streams := proxyNodeStreams{control: h.agentControl}
	if manager, ok := h.agentControl.(*controlgrpc.AgentControlManager); ok && manager == controlgrpc.GetAgentControlManager() {
		streams.config = controlgrpc.GetAgentStreams()
	}
	return streams
}

// ConfigNegotiated reports whether the proxy node's session negotiated
// config.v1.
func (s proxyNodeStreams) ConfigNegotiated(node agentcontrol.AgentNode) bool {
	return s.config != nil && node.Kind == agentcontrol.NodeKindProxy && s.config.ConfigNegotiated(node)
}

// PushConfig sends a snapshot to the proxy node's session.
func (s proxyNodeStreams) PushConfig(ctx context.Context, node agentcontrol.AgentNode, snapshot *agentv1pb.ConfigSnapshot) (string, bool, error) {
	if s.config == nil || node.Kind != agentcontrol.NodeKindProxy {
		return "", false, fmt.Errorf("agent node %s does not advertise capability %q: %w", node, agentcontrol.CapabilityConfig, agentstreams.ErrCapabilityMissing)
	}
	return s.config.PushConfig(ctx, node, snapshot)
}

// OnConfigStatus registers handler with the process's streams.
func (s proxyNodeStreams) OnConfigStatus(handler agentstreams.ConfigStatusHandler) {
	if s.config != nil {
		s.config.OnConfigStatus(handler)
	}
}

func (s proxyNodeStreams) Session(node agentcontrol.AgentNode) (agentstreams.Session, bool) {
	if node.Kind != agentcontrol.NodeKindProxy {
		return agentstreams.Session{}, false
	}
	snapshot, ok := s.control.Connection(node.ID)
	if !ok {
		return agentstreams.Session{}, false
	}
	identity := snapshot.Identity
	if identity == "" {
		identity = agentstreams.IdentityAPIKey
	}
	return agentstreams.Session{
		Node: node, Transport: agentstreams.TransportControlStream, SessionID: snapshot.SessionID, AgentVersion: snapshot.AgentVersion,
		InstanceID: snapshot.InstanceID, Capabilities: snapshot.Capabilities, ConnectedAt: snapshot.ConnectedAt, LastSeen: snapshot.LastSeen,
		DesiredRevision: snapshot.DesiredRev, ObservedRevision: snapshot.ObservedRev, Identity: identity,
		Authentication: snapshot.Authentication, Certificate: snapshot.Certificate, NegotiatedCapabilities: snapshot.NegotiatedCapabilities,
		AgentMetrics: snapshot.AgentMetrics, AgentMetricsAt: snapshot.AgentMetricsAt,
	}, true
}

func (s proxyNodeStreams) Sessions() []agentstreams.Session { return nil }

func (s proxyNodeStreams) Observed(node agentcontrol.AgentNode) (*agentv1pb.ObservedState, bool) {
	if node.Kind != agentcontrol.NodeKindProxy {
		return nil, false
	}
	return s.control.ObservedState(node.ID)
}

func (s proxyNodeStreams) Dispatch(ctx context.Context, node agentcontrol.AgentNode, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	if node.Kind != agentcontrol.NodeKindProxy {
		return nil, fmt.Errorf("agent node %s is not connected", node)
	}
	return s.control.DispatchOperation(ctx, node.ID, operation)
}

func (s proxyNodeStreams) Cancel(context.Context, agentcontrol.AgentNode, string, uint64) error {
	return errors.New("the legacy routes do not cancel operations")
}

func (s proxyNodeStreams) OnObserved(agentstreams.ObservedHandler) {}

// dispatchAgentControlOperation sends one Agent Control operation to a
// proxy node through kernelnodeops.DispatchAgentOperation, the function
// the agent.operation executor runs.
func (h *NodeHandler) dispatchAgentControlOperation(
	ctx context.Context,
	nodeID uint32,
	operationID string,
	kind string,
	payload any,
	timeout time.Duration,
) (*agentv1pb.OperationAck, *agentv1pb.DesiredOperation, error) {
	streams := h.streams()
	if streams == nil {
		return nil, nil, errors.New("agent control manager is unavailable")
	}
	var payloadJSON []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("encode operation payload: %w", err)
		}
		payloadJSON = encoded
	}
	operation, err := kernelnodeops.DispatchAgentOperation(ctx, streams, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: nodeID},
		kernelnodeops.AgentOperationRequest{OperationID: operationID, Kind: kind, PayloadJSON: payloadJSON, Timeout: timeout})
	if operation == nil {
		return nil, nil, err
	}
	operation.Release()
	if err != nil {
		return nil, operation.Desired, err
	}
	return operation.Ack, operation.Desired, nil
}

// GetAgentControlStatus returns the live v3 control-stream state for one node.
func (h *NodeHandler) GetAgentControlStatus(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}
	if _, err := h.nodeService.GetNode(uint(nodeID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}

	data := gin.H{"connected": false, "node_id": nodeID}
	if h.agentControl != nil {
		if connection, ok := h.agentControl.Connection(uint32(nodeID)); ok {
			data["connected"] = true
			data["connection"] = connection
		}
		if observed, ok := h.agentControl.ObservedState(uint32(nodeID)); ok {
			data["observed_state"] = observed
		}
	}
	panelSuccess(c, data)
}

// DispatchAgentControlOperation sends a bounded, capability-backed operation.
func (h *NodeHandler) DispatchAgentControlOperation(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}
	if _, err := h.nodeService.GetNode(uint(nodeID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}

	var req agentControlOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	if _, allowed := allowedAgentControlOperations[req.Kind]; !allowed {
		c.JSON(http.StatusBadRequest, gin.H{"message": "不支持的 Agent Control 操作"})
		return
	}
	if h.agentControl == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Agent Control 服务不可用"})
		return
	}
	if _, connected := h.agentControl.Connection(uint32(nodeID)); !connected {
		c.JSON(http.StatusConflict, gin.H{"message": "节点未连接 Agent Control"})
		return
	}

	timeout := time.Duration(req.TimeoutSecond) * time.Second
	if timeout <= 0 {
		timeout = defaultAgentControlOperationTimeout
	}
	ack, operation, err := h.dispatchAgentControlOperation(c.Request.Context(), uint32(nodeID), req.OperationID, req.Kind, req.Payload, timeout)
	if err != nil {
		statusCode := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			statusCode = http.StatusGatewayTimeout
		}
		c.JSON(statusCode, gin.H{"message": "Agent Control 操作下发失败", "error": err.Error()})
		return
	}
	panelSuccess(c, gin.H{
		"operation": operation,
		"ack":       ack,
	})
}
