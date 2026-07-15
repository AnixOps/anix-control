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

	agentv1pb "github.com/AnixOps/anix-control/v3/api/grpc/agent/v1"
	controlgrpc "github.com/AnixOps/anix-control/v3/internal/grpc"
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

func (h *NodeHandler) dispatchAgentControlOperation(
	ctx context.Context,
	nodeID uint32,
	operationID string,
	kind string,
	payload any,
	timeout time.Duration,
) (*agentv1pb.OperationAck, *agentv1pb.DesiredOperation, error) {
	if h.agentControl == nil {
		return nil, nil, errors.New("agent control manager is unavailable")
	}
	kind = strings.TrimSpace(kind)
	if _, allowed := allowedAgentControlOperations[kind]; !allowed {
		return nil, nil, fmt.Errorf("unsupported Agent Control operation %q", kind)
	}
	if timeout <= 0 {
		timeout = defaultAgentControlOperationTimeout
	}
	if timeout > maxAgentControlOperationTimeout {
		timeout = maxAgentControlOperationTimeout
	}

	var payloadJSON []byte
	var err error
	if payload != nil {
		payloadJSON, err = json.Marshal(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("encode operation payload: %w", err)
		}
	}

	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		operationID = generateTaskID()
	}
	operation := &agentv1pb.DesiredOperation{
		OperationId:    operationID,
		Kind:           kind,
		PayloadJson:    payloadJSON,
		DeadlineUnixMs: time.Now().Add(timeout).UnixMilli(),
	}
	dispatchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ack, err := h.agentControl.DispatchOperation(dispatchCtx, nodeID, operation)
	if err != nil {
		return nil, operation, err
	}
	if ack != nil && ack.Revision > 0 {
		operation.Revision = ack.Revision
	}
	return ack, operation, nil
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
