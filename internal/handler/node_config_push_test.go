package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	controlgrpc "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConfigAgentControl is a connected agent that negotiated config.v1:
// the handler's manager and the kernel's streams (agentstreams.Streams and
// agentstreams.ConfigStreams).
type fakeConfigAgentControl struct {
	*fakeNodeAgentControl
	pushed []*agentv1pb.ConfigSnapshot
}

func (f *fakeConfigAgentControl) Session(node agentcontrol.AgentNode) (agentstreams.Session, bool) {
	return agentstreams.Session{Node: node, SessionID: "session-config"}, true
}
func (f *fakeConfigAgentControl) Sessions() []agentstreams.Session { return nil }
func (f *fakeConfigAgentControl) Observed(agentcontrol.AgentNode) (*agentv1pb.ObservedState, bool) {
	return nil, false
}
func (f *fakeConfigAgentControl) Dispatch(ctx context.Context, node agentcontrol.AgentNode, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	return f.DispatchOperation(ctx, node.ID, operation)
}
func (f *fakeConfigAgentControl) Cancel(context.Context, agentcontrol.AgentNode, string, uint64) error {
	return nil
}
func (f *fakeConfigAgentControl) OnObserved(agentstreams.ObservedHandler)         {}
func (f *fakeConfigAgentControl) ConfigNegotiated(agentcontrol.AgentNode) bool    { return true }
func (f *fakeConfigAgentControl) OnConfigStatus(agentstreams.ConfigStatusHandler) {}
func (f *fakeConfigAgentControl) PushConfig(_ context.Context, _ agentcontrol.AgentNode, snapshot *agentv1pb.ConfigSnapshot) (string, bool, error) {
	f.pushed = append(f.pushed, snapshot)
	return "session-config", true, nil
}

// The sync route pushes a ConfigSnapshot, not node.reload, to an agent that
// negotiated config.v1, and answers with the snapshot's revision and hash.
func TestSyncProtocolPushesASnapshotToAConfigAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := createAgentControlHandlerTestNode(t)
	fake := &fakeConfigAgentControl{fakeNodeAgentControl: &fakeNodeAgentControl{connected: true}}
	h := NewNodeHandler()
	h.agentControl = fake
	router := gin.New()
	router.POST("/nodes/:id/sync", h.SyncProtocol)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/nodes/%d/sync", node.ID), nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Len(t, fake.pushed, 1)
	assert.Nil(t, fake.received, "no node.reload")
	expected := fmt.Sprintf(`{"config_hash":%q,"config_revision":%d,"message":"配置快照已通过 AnixOps Agent Control 下发","transport":"agent-control-grpc"}`,
		fake.pushed[0].ConfigHash, fake.pushed[0].ConfigRevision)
	assert.JSONEq(t, expected, string(rawData(t, recorder)))
}

// The handler's streams carry configurations only over the process's
// manager; a test manager carries none.
func TestProxyNodeStreamsConfig(t *testing.T) {
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 4242}
	plain := proxyNodeStreams{control: &fakeNodeAgentControl{}}
	assert.False(t, plain.ConfigNegotiated(node))
	_, _, err := plain.PushConfig(context.Background(), node, &agentv1pb.ConfigSnapshot{ConfigRevision: 1})
	assert.ErrorIs(t, err, agentstreams.ErrCapabilityMissing)
	plain.OnConfigStatus(func(agentstreams.ConfigStatusReport) {})

	h := NewNodeHandler()
	h.agentControl = controlgrpc.GetAgentControlManager()
	process, ok := h.streams().(proxyNodeStreams)
	require.True(t, ok)
	require.NotNil(t, process.config)
	assert.False(t, process.ConfigNegotiated(node), "not connected")
	_, _, err = process.PushConfig(context.Background(), node, &agentv1pb.ConfigSnapshot{ConfigRevision: 1})
	assert.ErrorIs(t, err, agentstreams.ErrNotConnected)
	_, _, err = process.PushConfig(context.Background(), agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, &agentv1pb.ConfigSnapshot{ConfigRevision: 1})
	assert.ErrorIs(t, err, agentstreams.ErrCapabilityMissing, "the handler reaches proxy nodes only")
	process.OnConfigStatus(func(agentstreams.ConfigStatusReport) {})
}
