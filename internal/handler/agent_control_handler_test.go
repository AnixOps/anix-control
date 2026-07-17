package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/v4/api/grpc/agent/v1"
	controlgrpc "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeNodeAgentControl struct {
	connected bool
	snapshot  controlgrpc.AgentControlSnapshot
	observed  *agentv1pb.ObservedState
	ack       *agentv1pb.OperationAck
	err       error
	received  *agentv1pb.DesiredOperation
}

func (f *fakeNodeAgentControl) DispatchOperation(_ context.Context, _ uint32, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	f.received = operation
	if f.ack == nil && f.err == nil {
		f.ack = &agentv1pb.OperationAck{OperationId: operation.OperationId, Accepted: true, Revision: 7}
	}
	return f.ack, f.err
}

func (f *fakeNodeAgentControl) Connection(uint32) (controlgrpc.AgentControlSnapshot, bool) {
	return f.snapshot, f.connected
}

func (f *fakeNodeAgentControl) ObservedState(uint32) (*agentv1pb.ObservedState, bool) {
	return f.observed, f.observed != nil
}

func createAgentControlHandlerTestNode(t *testing.T) *model.Node {
	t.Helper()
	db := initTestDB()
	now := time.Now().UnixNano()
	node := &model.Node{
		Name:       fmt.Sprintf("Agent Control Node %d", now),
		Host:       "127.0.0.1",
		Port:       443,
		APIKey:     fmt.Sprintf("agent-control-key-%d", now),
		APIKeyHash: fmt.Sprintf("agent-control-hash-%d", now),
		Secret:     fmt.Sprintf("agent-control-secret-%d", now),
		Status:     model.NodeStatusOnline,
	}
	require.NoError(t, db.Create(node).Error)
	t.Cleanup(func() {
		_ = db.Delete(&model.Node{}, node.ID).Error
	})
	return node
}

func TestDispatchAgentControlOperationUsesLiveGRPCStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := createAgentControlHandlerTestNode(t)
	fake := &fakeNodeAgentControl{
		connected: true,
		snapshot: controlgrpc.AgentControlSnapshot{
			NodeID:    uint32(node.ID),
			SessionID: "session-test",
		},
	}
	handler := NewNodeHandler()
	handler.agentControl = fake
	router := gin.New()
	router.POST("/nodes/:id/agent-control/operations", handler.DispatchAgentControlOperation)

	body := `{"operation_id":"manual-ping-1","kind":"agent.ping","payload":{"source":"test"},"timeout_seconds":2}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/nodes/%d/agent-control/operations", node.ID), strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, fake.received)
	assert.Equal(t, "manual-ping-1", fake.received.OperationId)
	assert.Equal(t, "agent.ping", fake.received.Kind)
	assert.JSONEq(t, `{"source":"test"}`, string(fake.received.PayloadJson))
}

func TestSyncProtocolDispatchesNodeReloadWhenAgentControlConnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := createAgentControlHandlerTestNode(t)
	fake := &fakeNodeAgentControl{connected: true}
	handler := NewNodeHandler()
	handler.agentControl = fake
	router := gin.New()
	router.POST("/nodes/:id/sync", handler.SyncProtocol)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/nodes/%d/sync", node.ID), nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, fake.received)
	assert.Equal(t, "node.reload", fake.received.Kind)
	assert.Contains(t, recorder.Body.String(), "agent-control-grpc")
}

func TestAgentControlOperationRequiresConnectedNode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := createAgentControlHandlerTestNode(t)
	handler := NewNodeHandler()
	handler.agentControl = &fakeNodeAgentControl{}
	router := gin.New()
	router.POST("/nodes/:id/agent-control/operations", handler.DispatchAgentControlOperation)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/nodes/%d/agent-control/operations", node.ID), strings.NewReader(`{"kind":"agent.ping"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusConflict, recorder.Code)
}
