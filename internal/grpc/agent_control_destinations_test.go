package grpc

import (
	"math"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newDestinationsTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newReportsTestEnvironment(t)
	requireAutoMigrate(t, &model.DestinationStat{}, &model.SystemConfig{})
	return environment
}

func destinationsMessage(requestID string, nodeID uint32, report *agentv1pb.DestinationReport) *agentv1pb.AgentToControl {
	return &agentv1pb.AgentToControl{
		RequestId: requestID, NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Destinations{Destinations: report},
	}
}

func destinationRows(t *testing.T) []model.DestinationStat {
	t.Helper()
	var rows []model.DestinationStat
	require.NoError(t, database.GetDB().Order("user_id, domain").Find(&rows).Error)
	return rows
}

// destinations.v1 is offered to a proxy node's Agent that lists it,
// whatever the setting, and never to a forward node.
func TestAgentControlDestinationsNegotiation(t *testing.T) {
	server := &AgentControlGRPCServer{}
	offered := func(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
		return agentcontrol.HasCapabilityVersion(server.serverCapabilities(node, agent), agentcontrol.CapabilityDestinations, agentcontrol.CapabilityVersionV1)
	}
	proxy := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	assert.True(t, offered(proxy, reportCapabilities(agentcontrol.CapabilityDestinations)))
	assert.False(t, offered(proxy, reportCapabilities()), "not listed by the agent")
	assert.False(t, offered(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, reportCapabilities(agentcontrol.CapabilityDestinations)))
}

// A batch is recorded once while the statistics are on, dropped (and
// acknowledged) while they are off, and refused for good when invalid.
func TestAgentControlDestinationReport(t *testing.T) {
	environment := newDestinationsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities(agentcontrol.CapabilityDestinations))
	require.True(t, agentcontrol.Negotiated(reportCapabilities(agentcontrol.CapabilityDestinations), helloAck.ServerCapabilities, agentcontrol.CapabilityDestinations))

	report := &agentv1pb.DestinationReport{
		BatchId: "node:proxy-1:boot:destinations-1", WindowEndUnixMs: time.Now().UnixMilli(),
		Entries: []*agentv1pb.DestinationTraffic{
			{UserId: 7, Domain: "youtube.com", Protocol: "tls", UploadBytes: 10, DownloadBytes: 1000, Connections: 2},
			{UserId: 8, Domain: "telegram.org", Protocol: "tls", UploadBytes: 5, DownloadBytes: 6, Connections: 1},
		},
	}

	// Off: acknowledged, nothing stored.
	require.NoError(t, stream.Send(destinationsMessage("off", nodeID, report)))
	ack := expectReportAck(t, stream, "off")
	assert.True(t, ack.Applied)
	assert.Empty(t, ack.Error)
	assert.Empty(t, destinationRows(t))

	// On: stored once.
	require.NoError(t, destinationstats.SaveSettings(database.GetDB(), destinationstats.Settings{Enabled: true, RetentionDays: 30}))
	report.BatchId = "node:proxy-1:boot:destinations-2"
	require.NoError(t, stream.Send(destinationsMessage("on", nodeID, report)))
	ack = expectReportAck(t, stream, "on")
	assert.Equal(t, report.BatchId, ack.BatchId)
	assert.True(t, ack.Applied)
	require.NoError(t, stream.Send(destinationsMessage("replay", nodeID, report)))
	ack = expectReportAck(t, stream, "replay")
	assert.False(t, ack.Applied)
	assert.Empty(t, ack.Error)
	rows := destinationRows(t)
	require.Len(t, rows, 2)
	assert.Equal(t, uint(nodeID), rows[0].NodeID)
	assert.Equal(t, "youtube.com", rows[0].Domain)
	assert.Equal(t, int64(1000), rows[0].Download)
	assert.Equal(t, int64(2), rows[0].Connections)
	assert.Equal(t, "telegram.org", rows[1].Domain)

	for _, test := range []struct {
		name   string
		report *agentv1pb.DestinationReport
		code   string
	}{
		{name: "no batch id", report: &agentv1pb.DestinationReport{Entries: []*agentv1pb.DestinationTraffic{{UserId: 7}}}, code: agentcontrol.ReportErrorCodeBatchIDInvalid},
		{name: "no user", report: &agentv1pb.DestinationReport{BatchId: "d1", Entries: []*agentv1pb.DestinationTraffic{{Domain: "example.com"}}}, code: agentcontrol.ReportErrorCodeInvalid},
		{name: "empty entry", report: &agentv1pb.DestinationReport{BatchId: "d2", Entries: []*agentv1pb.DestinationTraffic{nil}}, code: agentcontrol.ReportErrorCodeInvalid},
		{name: "overflow", report: &agentv1pb.DestinationReport{BatchId: "d3", Entries: []*agentv1pb.DestinationTraffic{{UserId: 7, UploadBytes: math.MaxUint64}}}, code: agentcontrol.ReportErrorCodeCounterOverflow},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, stream.Send(destinationsMessage(test.name, nodeID, test.report)))
			ack := expectReportAck(t, stream, test.name)
			assert.False(t, ack.Applied)
			assert.NotEmpty(t, ack.Error)
			assert.Equal(t, test.code, ack.ErrorCode)
		})
	}
	assert.Len(t, destinationRows(t), 2)
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-end")
	require.NoError(t, stream.CloseSend())
}

// Without destinations.v1 negotiated the payload ends the stream.
func TestAgentControlDestinationReportWithoutCapability(t *testing.T) {
	environment := newDestinationsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, _ := openReportsSession(t, environment, reportCapabilities())
	require.NoError(t, stream.Send(destinationsMessage("unnegotiated", nodeID, &agentv1pb.DestinationReport{
		BatchId: "node:proxy-1:boot:destinations-1", Entries: []*agentv1pb.DestinationTraffic{{UserId: 7, Domain: "example.com"}},
	})))
	_, err := stream.Recv()
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "requires the destinations.v1 server capability")
	assert.Empty(t, destinationRows(t))
}
