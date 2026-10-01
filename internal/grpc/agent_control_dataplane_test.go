package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
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

// dataPlaneCapabilitiesWithout is dataPlaneCapabilities less one feature.
func dataPlaneCapabilitiesWithout(name string) []*agentv1pb.Capability {
	var capabilities []*agentv1pb.Capability
	for _, capability := range dataPlaneCapabilities() {
		if capability.Name != name {
			capabilities = append(capabilities, capability)
		}
	}
	return capabilities
}

// openDataPlaneSession sends a Hello advertising the data plane, with a
// configuration revision and a user cursor, and returns the HelloAck.
func openDataPlaneSession(t *testing.T, environment *agentControlTestEnvironment, capabilities ...*agentv1pb.Capability) (agentv1pb.AgentControlService_ControlStreamClient, *agentv1pb.HelloAck) {
	t.Helper()
	if capabilities == nil {
		capabilities = dataPlaneCapabilities()
	}
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(uint32(environment.node.ID))
	hello.GetHello().Capabilities = capabilities
	hello.GetHello().ConfigRevision = 41
	hello.GetHello().UsersCursor = 1234
	require.NoError(t, stream.Send(hello))
	message, err := stream.Recv()
	require.NoError(t, err)
	helloAck := message.GetHelloAck()
	require.NotNil(t, helloAck)
	return stream, helloAck
}

// The server serves users.v1 (A2-4) and reports.v1 (A2-5) of the data plane
// and nothing else yet: an Agent that advertises the whole data plane
// negotiates those two, and no ConfigSnapshot is pushed for its revision.
func TestAgentControlServerAdvertisesUsersAndReports(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	requireAutoMigrate(t, &model.User{}, &model.SubscriberChange{})
	stream, helloAck := openDataPlaneSession(t, environment)
	require.NotEmpty(t, helloAck.SessionId)
	for _, capability := range []string{agentcontrol.CapabilityUsers, agentcontrol.CapabilityReports} {
		assert.True(t, agentcontrol.Negotiated(dataPlaneCapabilities(), helloAck.ServerCapabilities, capability), capability)
	}
	assert.False(t, agentcontrol.Negotiated(dataPlaneCapabilities(), helloAck.ServerCapabilities, agentcontrol.CapabilityConfig))

	snapshot, connected := environment.manager.Connection(uint32(environment.node.ID))
	require.True(t, connected)
	assert.Contains(t, snapshot.Capabilities, agentcontrol.CapabilityReports)

	// The Hello's cursor (1234) is ahead of the empty change log, so the
	// node's (empty) user set is sent as a full resync; the heartbeat ack
	// and that one page are the only messages, in either order.
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId:    "heartbeat-request",
		NodeId:       uint32(environment.node.ID),
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{
			Heartbeat: &agentv1pb.Heartbeat{SessionId: helloAck.SessionId, UptimeSeconds: 1},
		},
	}))
	var heartbeatAcks, userDeltas int
	for i := 0; i < 2; i++ {
		message, err := stream.Recv()
		require.NoError(t, err)
		switch payload := message.Payload.(type) {
		case *agentv1pb.ControlToAgent_HeartbeatAck:
			heartbeatAcks++
			assert.Equal(t, "heartbeat-request", message.RequestId)
		case *agentv1pb.ControlToAgent_Users:
			userDeltas++
			assert.True(t, payload.Users.Full)
			assert.True(t, payload.Users.LastPage)
			assert.Zero(t, payload.Users.Cursor)
			assert.Empty(t, payload.Users.Upserts)
		default:
			t.Fatalf("unexpected payload %T", message.Payload)
		}
	}
	assert.Equal(t, 1, heartbeatAcks)
	assert.Equal(t, 1, userDeltas)
	require.NoError(t, stream.CloseSend())
}

// An Agent that sends a data-plane payload the session did not negotiate
// gets InvalidArgument and the stream ends, as from a Control built before
// the payloads existed: config_status, which the server does not serve yet,
// and the reports from an Agent whose Hello did not list reports.v1.
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
			stream, _ := openDataPlaneSession(t, environment, dataPlaneCapabilitiesWithout(agentcontrol.CapabilityReports)...)
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
