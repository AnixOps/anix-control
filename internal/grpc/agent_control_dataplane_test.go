package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// dataPlaneCapabilities is what an Agent with the A2 data plane advertises.
func dataPlaneCapabilities() []*agentv1pb.Capability {
	return []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityConfig, Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityUsers, Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityReports, Version: agentcontrol.CapabilityVersionV1},
	}
}

// openDataPlaneSession sends a Hello advertising the data plane, with a
// configuration revision and a user cursor, and returns the HelloAck.
func openDataPlaneSession(t *testing.T, environment *agentControlTestEnvironment) (agentv1pb.AgentControlService_ControlStreamClient, *agentv1pb.HelloAck) {
	t.Helper()
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(uint32(environment.node.ID))
	hello.GetHello().Capabilities = dataPlaneCapabilities()
	hello.GetHello().ConfigRevision = 41
	hello.GetHello().UsersCursor = 1234
	require.NoError(t, stream.Send(hello))
	message, err := stream.Recv()
	require.NoError(t, err)
	helloAck := message.GetHelloAck()
	require.NotNil(t, helloAck)
	return stream, helloAck
}

// The server does not serve the data plane yet: an Agent that advertises it
// gets an ordinary session, no server capabilities and nothing but the
// existing payloads.
func TestAgentControlServerAdvertisesNoDataPlane(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	stream, helloAck := openDataPlaneSession(t, environment)
	require.NotEmpty(t, helloAck.SessionId)
	assert.Empty(t, helloAck.ServerCapabilities)
	for _, capability := range []string{agentcontrol.CapabilityConfig, agentcontrol.CapabilityUsers, agentcontrol.CapabilityReports} {
		assert.False(t, agentcontrol.Negotiated(dataPlaneCapabilities(), helloAck.ServerCapabilities, capability), capability)
	}

	snapshot, connected := environment.manager.Connection(uint32(environment.node.ID))
	require.True(t, connected)
	assert.Contains(t, snapshot.Capabilities, agentcontrol.CapabilityReports)

	// The next message after the HelloAck answers this heartbeat: no
	// ConfigSnapshot or UserDelta was pushed for the Hello's revision and
	// cursor.
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId:    "heartbeat-request",
		NodeId:       uint32(environment.node.ID),
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{
			Heartbeat: &agentv1pb.Heartbeat{SessionId: helloAck.SessionId, UptimeSeconds: 1},
		},
	}))
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetHeartbeatAck(), "got %T", message.Payload)
	assert.Equal(t, "heartbeat-request", message.RequestId)
	require.NoError(t, stream.CloseSend())
}

// An Agent that sends a data-plane report anyway gets InvalidArgument and the
// stream ends, as from a Control built before the payloads existed.
func TestAgentControlStreamRejectsUnnegotiatedDataPlanePayloads(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	tests := []struct {
		name    string
		payload func() *agentv1pb.AgentToControl
		want    string
	}{
		{
			name: "config_status",
			payload: func() *agentv1pb.AgentToControl {
				return &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_ConfigStatus{
					ConfigStatus: &agentv1pb.ConfigStatus{ConfigRevision: 41, ConfigHash: "hash", Applied: true},
				}}
			},
			want: "control message payload config_status requires the config.v1 server capability",
		},
		{
			name: "traffic",
			payload: func() *agentv1pb.AgentToControl {
				return &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Traffic{
					Traffic: &agentv1pb.TrafficReport{
						BatchId: "node:proxy-1:boot:1",
						Users:   []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 10, DownloadBytes: 20}},
						Online:  []*agentv1pb.OnlineUser{{UserId: 7, Ips: []string{"192.0.2.1"}}},
					},
				}}
			},
			want: "control message payload traffic requires the reports.v1 server capability",
		},
		{
			name: "logs",
			payload: func() *agentv1pb.AgentToControl {
				return &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Logs{
					Logs: &agentv1pb.LogBatch{BatchId: "node:proxy-1:boot:2", Entries: []*agentv1pb.LogEntry{{Level: "info", Message: "started"}}},
				}}
			},
			want: "control message payload logs requires the reports.v1 server capability",
		},
		{
			name: "status",
			payload: func() *agentv1pb.AgentToControl {
				return &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Status{
					Status: &agentv1pb.NodeStatus{CpuUsagePercent: 12.5, RuntimeHealthy: true},
				}}
			},
			want: "control message payload status requires the reports.v1 server capability",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream, _ := openDataPlaneSession(t, environment)
			message := test.payload()
			message.RequestId = test.name + "-request"
			message.NodeId = uint32(environment.node.ID)
			message.SentAtUnixMs = time.Now().UnixMilli()
			require.NoError(t, stream.Send(message))
			_, err := stream.Recv()
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
			assert.Contains(t, status.Convert(err).Message(), test.want)
		})
	}
}
