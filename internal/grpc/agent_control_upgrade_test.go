package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func upgradeCapabilities() []*agentv1pb.Capability {
	return []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityUpgrade, Version: agentcontrol.CapabilityVersionV1},
	}
}

// upgrade.v1 is offered to proxy and forward nodes whose Hello lists it at
// v1, and to no other.
func TestAgentControlUpgradeCapabilityOffer(t *testing.T) {
	server := NewAgentControlGRPCServer(NewAgentControlManager())
	for _, node := range []agentcontrol.AgentNode{{Kind: agentcontrol.NodeKindProxy, ID: 1}, {Kind: agentcontrol.NodeKindForward, ID: 1}} {
		assert.True(t, agentcontrol.HasCapabilityVersion(server.serverCapabilities(node, upgradeCapabilities()), agentcontrol.CapabilityUpgrade, agentcontrol.CapabilityVersionV1), node.String())
		assert.False(t, agentcontrol.HasCapability(server.serverCapabilities(node, reportCapabilities()), agentcontrol.CapabilityUpgrade), "only an agent that lists it")
		assert.False(t, agentcontrol.HasCapability(server.serverCapabilities(node, []*agentv1pb.Capability{{Name: agentcontrol.CapabilityUpgrade, Version: "v2"}}), agentcontrol.CapabilityUpgrade))
	}
	assert.Contains(t, agentcontrol.NegotiatedCapabilities(upgradeCapabilities(), upgradeCapabilities()), "upgrade.v1")
}

// agent.upgrade is sent only on a session that negotiated upgrade.v1: a
// Hello naming the operation kind is not enough. A retained agent.upgrade
// is not replayed to an Agent that reconnects without upgrade.v1.
func TestAgentControlUpgradeDispatchRequiresNegotiation(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	operation := func(id string) *agentv1pb.DesiredOperation {
		return &agentv1pb.DesiredOperation{OperationId: id, Kind: agentcontrol.OperationKindAgentUpgrade, PayloadJson: []byte(`{}`)}
	}

	stream, _ := openReportsSession(t, environment, []*agentv1pb.Capability{{Name: agentcontrol.OperationKindAgentUpgrade, Version: agentcontrol.CapabilityVersionV1}})
	_, err := environment.manager.DispatchOperation(ctx, nodeID, operation("agent-upgrade-refused"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `does not advertise capability "agent.upgrade"`)
	require.NoError(t, stream.CloseSend())

	stream, helloAck := openReportsSession(t, environment, upgradeCapabilities())
	require.True(t, agentcontrol.Negotiated(upgradeCapabilities(), helloAck.ServerCapabilities, agentcontrol.CapabilityUpgrade))
	acknowledged := make(chan error, 1)
	go func() {
		ack, dispatchErr := environment.manager.DispatchOperation(ctx, nodeID, operation("agent-upgrade-1"))
		if dispatchErr == nil && !ack.GetAccepted() {
			dispatchErr = assert.AnError
		}
		acknowledged <- dispatchErr
	}()
	message, err := stream.Recv()
	require.NoError(t, err)
	desired := message.GetDesiredOperation()
	require.NotNil(t, desired, "got %T", message.Payload)
	assert.Equal(t, agentcontrol.OperationKindAgentUpgrade, desired.GetKind())
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "ack", NodeId: nodeID, Revision: desired.Revision, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_OperationAck{OperationAck: &agentv1pb.OperationAck{
			OperationId: desired.OperationId, Accepted: true, AcceptedAtUnixMs: time.Now().UnixMilli(), SessionId: helloAck.SessionId, Revision: desired.Revision,
		}},
	}))
	require.NoError(t, <-acknowledged)
	require.NoError(t, stream.CloseSend())

	// The Agent restarts as a release without upgrade.v1 before reporting
	// a terminal state: the retained operation is dropped, not replayed.
	stream, helloAck = openReportsSession(t, environment, reportCapabilities())
	require.NoError(t, stream.Send(heartbeatMessage("heartbeat-1", nodeID, helloAck.SessionId)))
	message, err = stream.Recv()
	require.NoError(t, err)
	assert.NotNil(t, message.GetHeartbeatAck(), "got %T", message.Payload)
	environment.manager.mu.RLock()
	assert.Empty(t, environment.manager.desired[nodeID])
	environment.manager.mu.RUnlock()
	require.NoError(t, stream.CloseSend())
}
