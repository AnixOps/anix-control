package grpc

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// reportsTestEnvironment is the agent control environment with the tables
// the legacy report sinks write and two subscribers, 7 and 8.
func newReportsTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newAgentControlTestEnvironment(t)
	requireAutoMigrate(t, &model.User{}, &model.TrafficLog{}, &model.StatServer{}, &model.NodeLog{},
		&model.AgentReportBatch{}, &model.SubscriberRequest{}, &model.SubscriberChange{})
	for _, id := range []uint{7, 8} {
		name := "user" + strconv.Itoa(int(id))
		require.NoError(t, database.GetDB().Create(&model.User{
			ID: id, Email: name + "@example.com", Token: name + "-token", UUID: name + "-uuid", TransferEnable: 1 << 40,
		}).Error)
	}
	return environment
}

// reportCapabilities is a Hello capability list with reports.v1 and the
// extras given (diag.v1, for one test).
func reportCapabilities(extra ...string) []*agentv1pb.Capability {
	capabilities := []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityReports, Version: agentcontrol.CapabilityVersionV1},
	}
	for _, name := range extra {
		capabilities = append(capabilities, &agentv1pb.Capability{Name: name, Version: agentcontrol.CapabilityVersionV1})
	}
	return capabilities
}

// openReportsSession opens a stream whose Hello lists capabilities and
// returns it with the HelloAck.
func openReportsSession(t *testing.T, environment *agentControlTestEnvironment, capabilities []*agentv1pb.Capability) (agentv1pb.AgentControlService_ControlStreamClient, *agentv1pb.HelloAck) {
	t.Helper()
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(uint32(environment.node.ID))
	hello.GetHello().Capabilities = capabilities
	require.NoError(t, stream.Send(hello))
	message, err := stream.Recv()
	require.NoError(t, err)
	helloAck := message.GetHelloAck()
	require.NotNil(t, helloAck)
	return stream, helloAck
}

func trafficMessage(requestID string, nodeID uint32, report *agentv1pb.TrafficReport) *agentv1pb.AgentToControl {
	return &agentv1pb.AgentToControl{
		RequestId: requestID, NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Traffic{Traffic: report},
	}
}

func logsMessage(requestID string, nodeID uint32, batch *agentv1pb.LogBatch) *agentv1pb.AgentToControl {
	return &agentv1pb.AgentToControl{
		RequestId: requestID, NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Logs{Logs: batch},
	}
}

func heartbeatMessage(requestID string, nodeID uint32, sessionID string) *agentv1pb.AgentToControl {
	return &agentv1pb.AgentToControl{
		RequestId: requestID, NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: sessionID, UptimeSeconds: 1}},
	}
}

// expectReportAck reads the next message and requires it to be the
// ReportAck answering requestID.
func expectReportAck(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient, requestID string) *agentv1pb.ReportAck {
	t.Helper()
	message, err := stream.Recv()
	require.NoError(t, err)
	ack := message.GetReportAck()
	require.NotNil(t, ack, "got %T", message.Payload)
	assert.Equal(t, requestID, message.RequestId)
	return ack
}

// expectHeartbeatAck sends a heartbeat and requires the next message to be
// its acknowledgement: nothing else was sent before it.
func expectHeartbeatAck(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient, nodeID uint32, sessionID, requestID string) {
	t.Helper()
	require.NoError(t, stream.Send(heartbeatMessage(requestID, nodeID, sessionID)))
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetHeartbeatAck(), "got %T", message.Payload)
	assert.Equal(t, requestID, message.RequestId)
}

func userCounters(t *testing.T, id uint) (int64, int64) {
	t.Helper()
	var user model.User
	require.NoError(t, database.GetDB().First(&user, id).Error)
	return user.U, user.D
}

// With reports.v1 in the Hello the HelloAck advertises reports.v1 and the
// session records it; without it the server advertises nothing and keeps
// refusing the payloads.
func TestAgentControlReportsNegotiation(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)

	t.Run("on", func(t *testing.T) {
		stream, helloAck := openReportsSession(t, environment, reportCapabilities())
		assert.True(t, agentcontrol.HasCapabilityVersion(helloAck.ServerCapabilities, agentcontrol.CapabilityReports, agentcontrol.CapabilityVersionV1))
		assert.True(t, agentcontrol.Negotiated(reportCapabilities(), helloAck.ServerCapabilities, agentcontrol.CapabilityReports))
		snapshot, connected := environment.manager.Connection(nodeID)
		require.True(t, connected)
		assert.Contains(t, snapshot.ServerCapabilities, "reports.v1")
		assert.False(t, snapshot.Diagnostics)
		require.NoError(t, stream.CloseSend())
	})

	t.Run("off", func(t *testing.T) {
		stream, helloAck := openReportsSession(t, environment, []*agentv1pb.Capability{
			{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
			{Name: agentcontrol.CapabilityConfig, Version: agentcontrol.CapabilityVersionV1},
		})
		assert.False(t, agentcontrol.HasCapabilityVersion(helloAck.ServerCapabilities, agentcontrol.CapabilityReports, agentcontrol.CapabilityVersionV1))
		require.NoError(t, stream.Send(trafficMessage("traffic-request", nodeID, &agentv1pb.TrafficReport{
			BatchId: "node:proxy-1:boot:1", Users: []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 10}},
		})))
		_, err := stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, status.Convert(err).Message(), "requires the reports.v1 server capability")
		upload, _ := userCounters(t, 7)
		assert.Zero(t, upload)
	})
}

// A forward node's stream is not offered reports.v1: its sinks join with A5.
func TestAgentControlReportsNotOfferedToForwardNodes(t *testing.T) {
	server := &AgentControlGRPCServer{}
	reportsV1 := func(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
		return agentcontrol.HasCapabilityVersion(server.serverCapabilities(node, agent), agentcontrol.CapabilityReports, agentcontrol.CapabilityVersionV1)
	}
	assert.False(t, reportsV1(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, reportCapabilities()))
	assert.True(t, reportsV1(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, reportCapabilities()))
	assert.False(t, reportsV1(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, validAgentHello(1).GetHello().Capabilities), "not listed by the agent")
}

// A traffic batch is counted once, through the legacy ledger: the stream
// node and a legacy ReportTraffic node end with the same rows for the same
// input, a replay is acknowledged without counting again, and the online
// IPs reach the set ReportOnline feeds.
func TestAgentControlTrafficReportAppliedOnceWithLegacyParity(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	legacy := model.Node{Name: "legacy-node", Host: "127.0.0.2", APIKey: "legacy-key", Status: model.NodeStatusOnline}
	require.NoError(t, database.GetDB().Create(&legacy).Error)

	stream, helloAck := openReportsSession(t, environment, reportCapabilities())
	report := &agentv1pb.TrafficReport{
		BatchId: "node:proxy-1:boot-a:1",
		Users:   []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 1000, DownloadBytes: 2000}, {UserId: 7, UploadBytes: 24, DownloadBytes: 48}},
		Online:  []*agentv1pb.OnlineUser{{UserId: 7, Ips: []string{"192.0.2.1", "192.0.2.2"}}, {UserId: 9, Ips: []string{" 192.0.2.3 "}}},
	}
	require.NoError(t, stream.Send(trafficMessage("traffic-1", nodeID, report)))
	ack := expectReportAck(t, stream, "traffic-1")
	assert.Equal(t, report.BatchId, ack.BatchId)
	assert.True(t, ack.Applied)
	assert.Empty(t, ack.Error)

	// The legacy path, with the same input for subscriber 8 on the other
	// node.
	_, err := NewTrafficGRPCServer().ReportTraffic(withNodeCaller(context.Background(), uint32(legacy.ID)), &pb.TrafficReportRequest{
		NodeId: uint32(legacy.ID), Traffics: map[uint32]*pb.TrafficData{8: {Upload: 1024, Download: 2048}},
	})
	require.NoError(t, err)

	upload, download := userCounters(t, 7)
	assert.Equal(t, int64(1024), upload)
	assert.Equal(t, int64(2048), download)
	legacyUpload, legacyDownload := userCounters(t, 8)
	assert.Equal(t, upload, legacyUpload)
	assert.Equal(t, download, legacyDownload)

	type trafficRow struct {
		UserID     uint
		ServerType string
		U, D       int64
		Rate       float64
	}
	rows := func(serverID uint, userID uint) []trafficRow {
		var logs []model.TrafficLog
		require.NoError(t, database.GetDB().Where("server_id = ? AND user_id = ?", serverID, userID).Find(&logs).Error)
		out := make([]trafficRow, 0, len(logs))
		for _, log := range logs {
			out = append(out, trafficRow{UserID: log.UserID - userID, ServerType: log.ServerType, U: log.U, D: log.D, Rate: log.Rate})
		}
		return out
	}
	assert.Equal(t, rows(legacy.ID, 8), rows(environment.node.ID, 7))
	assert.Equal(t, []trafficRow{{ServerType: "node", U: 1024, D: 2048, Rate: 1}}, rows(environment.node.ID, 7))

	var streamNode, legacyNode model.Node
	require.NoError(t, database.GetDB().First(&streamNode, environment.node.ID).Error)
	require.NoError(t, database.GetDB().First(&legacyNode, legacy.ID).Error)
	assert.Equal(t, [2]int64{1024, 2048}, [2]int64{streamNode.TotalUpload, streamNode.TotalDownload})
	assert.Equal(t, [2]int64{legacyNode.TotalUpload, legacyNode.TotalDownload}, [2]int64{streamNode.TotalUpload, streamNode.TotalDownload})
	assert.Equal(t, 2, streamNode.OnlineUsers)

	type statRow struct {
		RecordType string
		RecordAt   int64
		U, D       int64
	}
	stats := func(serverID uint) []statRow {
		var found []model.StatServer
		require.NoError(t, database.GetDB().Where("server_id = ?", serverID).Order("record_type").Find(&found).Error)
		out := make([]statRow, 0, len(found))
		for _, stat := range found {
			out = append(out, statRow{RecordType: stat.RecordType, RecordAt: stat.RecordAt, U: stat.U, D: stat.D})
		}
		return out
	}
	require.Len(t, stats(environment.node.ID), 2)
	assert.Equal(t, stats(legacy.ID), stats(environment.node.ID))

	// Online IPs are in the set ReportOnline feeds: the same keys, for the
	// legacy node's report of subscriber 8.
	_, err = NewTrafficGRPCServer().ReportOnline(withNodeCaller(context.Background(), uint32(legacy.ID)), &pb.OnlineReportRequest{
		NodeId: uint32(legacy.ID), Online: map[uint32]*pb.OnlineData{8: {Ips: []string{"192.0.2.8"}}},
	})
	require.NoError(t, err)
	onlineKey := func(nodeID uint, userID uint) string { return fmt.Sprintf("online:%s:%d:%d", "", nodeID, userID) }
	legacyKeys, err := cache.Keys(fmt.Sprintf("online:%s:%d:*", "", legacy.ID))
	require.NoError(t, err)
	assert.Equal(t, []string{onlineKey(legacy.ID, 8)}, legacyKeys)
	streamKeys, err := cache.Keys(fmt.Sprintf("online:%s:%d:*", "", environment.node.ID))
	require.NoError(t, err)
	sort.Strings(streamKeys)
	assert.Equal(t, []string{onlineKey(environment.node.ID, 7), onlineKey(environment.node.ID, 9)}, streamKeys)
	count, err := cache.SCard(onlineKey(environment.node.ID, 7))
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// The replay is acknowledged as seen, and nothing is counted again.
	require.NoError(t, stream.Send(trafficMessage("traffic-1-again", nodeID, report)))
	ack = expectReportAck(t, stream, "traffic-1-again")
	assert.Equal(t, report.BatchId, ack.BatchId)
	assert.False(t, ack.Applied)
	assert.Empty(t, ack.Error)
	upload, download = userCounters(t, 7)
	assert.Equal(t, [2]int64{1024, 2048}, [2]int64{upload, download})
	assert.Len(t, rows(environment.node.ID, 7), 1)
	require.NoError(t, database.GetDB().First(&streamNode, environment.node.ID).Error)
	assert.Equal(t, [2]int64{1024, 2048}, [2]int64{streamNode.TotalUpload, streamNode.TotalDownload})

	// The next sequence counts, and an online-only batch replaces the set.
	require.NoError(t, stream.Send(trafficMessage("traffic-2", nodeID, &agentv1pb.TrafficReport{
		BatchId: "node:proxy-1:boot-a:2", Online: []*agentv1pb.OnlineUser{{UserId: 7, Ips: []string{"192.0.2.9"}}},
	})))
	ack = expectReportAck(t, stream, "traffic-2")
	assert.True(t, ack.Applied)
	streamKeys, err = cache.Keys(fmt.Sprintf("online:%s:%d:*", "", environment.node.ID))
	require.NoError(t, err)
	assert.Equal(t, []string{onlineKey(environment.node.ID, 7)}, streamKeys)
	count, err = cache.SCard(onlineKey(environment.node.ID, 7))
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	upload, _ = userCounters(t, 7)
	assert.Equal(t, int64(1024), upload)
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-end")
	require.NoError(t, stream.CloseSend())
}

// A batch the kernel cannot record for now gets no acknowledgement and the
// stream stays open; the resend is applied once the fault is gone.
func TestAgentControlTrafficReportTransientFailureGetsNoAck(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities())

	require.NoError(t, database.GetDB().Exec("DROP TABLE v4_kernel_agent_report_batch").Error)
	report := &agentv1pb.TrafficReport{BatchId: "node:proxy-1:boot-a:1", Users: []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 10, DownloadBytes: 20}}}
	require.NoError(t, stream.Send(trafficMessage("traffic-1", nodeID, report)))
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-1")
	upload, _ := userCounters(t, 7)
	assert.Zero(t, upload, "nothing was counted")

	requireAutoMigrate(t, &model.AgentReportBatch{})
	require.NoError(t, stream.Send(trafficMessage("traffic-1-again", nodeID, report)))
	ack := expectReportAck(t, stream, "traffic-1-again")
	assert.True(t, ack.Applied)
	upload, download := userCounters(t, 7)
	assert.Equal(t, [2]int64{10, 20}, [2]int64{upload, download})
	require.NoError(t, stream.CloseSend())
}

// A batch refused for good is acknowledged with applied false and an error,
// nothing is counted, and the stream stays open.
func TestAgentControlTrafficReportRefusedForGood(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities())
	tests := []struct {
		name   string
		report *agentv1pb.TrafficReport
		want   string
	}{
		{name: "no batch id", report: &agentv1pb.TrafficReport{Users: []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 1}}}, want: "batch_id is required"},
		{name: "bytes out of range", report: &agentv1pb.TrafficReport{BatchId: "b2", Users: []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: math.MaxUint64}}}, want: "exceed the counter range"},
		{name: "no user", report: &agentv1pb.TrafficReport{BatchId: "b3", Users: []*agentv1pb.UserTraffic{{UploadBytes: 1}}}, want: "user_id is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, stream.Send(trafficMessage(test.name, nodeID, test.report)))
			ack := expectReportAck(t, stream, test.name)
			assert.Equal(t, test.report.BatchId, ack.BatchId)
			assert.False(t, ack.Applied)
			assert.Contains(t, ack.Error, test.want)
		})
	}
	upload, _ := userCounters(t, 7)
	assert.Zero(t, upload)
	var batches int64
	require.NoError(t, database.GetDB().Model(&model.AgentReportBatch{}).Count(&batches).Error)
	assert.Zero(t, batches, "a refused batch is not recorded")
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-end")
	require.NoError(t, stream.CloseSend())
}

// A log batch lands in v2_node_log as ReportLogs puts it, with the runtime
// health a WireGuard entry carries; a replay adds nothing.
func TestAgentControlLogBatchReachesNodeLogs(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, _ := openReportsSession(t, environment, reportCapabilities())
	loggedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	batch := &agentv1pb.LogBatch{
		BatchId: "node:proxy-1:boot-a:1",
		Entries: []*agentv1pb.LogEntry{
			{Level: "warning", Source: "xray", Message: "  listener restarted  ", LoggedAtUnixMs: loggedAt.UnixMilli(), FieldsJson: []byte(`{"pid": 42}`), TraceId: "trace-1"},
			{Level: "error", Source: "wireguard", Message: "handshake failed", FieldsJson: []byte(`{"runtime_healthy":false,"runtime_error":"no handshake"}`)},
			{Level: "info", Message: "   "},
		},
	}
	require.NoError(t, stream.Send(logsMessage("logs-1", nodeID, batch)))
	ack := expectReportAck(t, stream, "logs-1")
	assert.True(t, ack.Applied)
	assert.Empty(t, ack.Error)

	var logs []model.NodeLog
	require.NoError(t, database.GetDB().Where("node_id = ?", nodeID).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, "warning", logs[0].Level)
	assert.Equal(t, "xray", logs[0].Source)
	assert.Equal(t, "listener restarted", logs[0].Message)
	assert.Equal(t, `{"pid":42}`, logs[0].FieldsJSON)
	assert.Equal(t, "trace-1", logs[0].TraceID)
	require.NotNil(t, logs[0].LoggedAt)
	assert.Equal(t, loggedAt.UnixMilli(), logs[0].LoggedAt.UnixMilli())
	assert.Nil(t, logs[1].LoggedAt)
	var node model.Node
	require.NoError(t, database.GetDB().First(&node, nodeID).Error)
	assert.False(t, node.RuntimeHealthy)
	assert.Equal(t, "no handshake", node.RuntimeError)

	require.NoError(t, stream.Send(logsMessage("logs-1-again", nodeID, batch)))
	ack = expectReportAck(t, stream, "logs-1-again")
	assert.False(t, ack.Applied)
	assert.Empty(t, ack.Error)
	var count int64
	require.NoError(t, database.GetDB().Model(&model.NodeLog{}).Where("node_id = ?", nodeID).Count(&count).Error)
	assert.Equal(t, int64(2), count)

	require.NoError(t, stream.Send(logsMessage("logs-bad", nodeID, &agentv1pb.LogBatch{
		BatchId: "node:proxy-1:boot-a:2", Entries: []*agentv1pb.LogEntry{{Message: "x", FieldsJson: []byte(`{not json`)}},
	})))
	ack = expectReportAck(t, stream, "logs-bad")
	assert.False(t, ack.Applied)
	assert.Contains(t, ack.Error, "fields_json is not JSON")
	require.NoError(t, stream.CloseSend())
}

// A NodeStatus updates the columns ReportStatus and the runtime-health
// report write, and is never acknowledged.
func TestAgentControlNodeStatusUpdatesNodeWithoutAck(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities())
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "status-1", NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Status{Status: &agentv1pb.NodeStatus{
			CpuUsagePercent: 12.5, MemoryUsagePercent: 40, DiskUsagePercent: 70.25, UptimeSeconds: 3600,
			RuntimeHealthy: false, RuntimeError: "xray exited",
		}},
	}))
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-1")

	var node model.Node
	require.NoError(t, database.GetDB().First(&node, nodeID).Error)
	assert.Equal(t, 12.5, node.CPUUsage)
	assert.Equal(t, float64(40), node.MemoryUsage)
	assert.Equal(t, 70.25, node.DiskUsage)
	assert.Equal(t, int64(3600), node.Uptime)
	assert.False(t, node.RuntimeHealthy)
	assert.Equal(t, "xray exited", node.RuntimeError)
	require.NotNil(t, node.RuntimeCheckedAt)
	require.NotNil(t, node.LastCheckAt)
	assert.Equal(t, model.NodeStatusOnline, node.Status)
	require.NoError(t, stream.CloseSend())
}

// A report whose envelope names another node ends the stream, as every
// other message does, and counts nothing.
func TestAgentControlReportForAnotherNodeIsRefused(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, _ := openReportsSession(t, environment, reportCapabilities())
	require.NoError(t, stream.Send(trafficMessage("traffic-other", nodeID+1, &agentv1pb.TrafficReport{
		BatchId: "node:proxy-2:boot:1", Users: []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 10}},
	})))
	_, err := stream.Recv()
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	upload, _ := userCounters(t, 7)
	assert.Zero(t, upload)
	var batches int64
	require.NoError(t, database.GetDB().Model(&model.AgentReportBatch{}).Count(&batches).Error)
	assert.Zero(t, batches)
}

// diag.v1 in the Hello is recorded on the session for the vantage
// selection; no diagnostic is sent.
func TestAgentControlDiagCapabilityRecordedOnSession(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities(agentcontrol.CapabilityDiag))
	snapshot, connected := environment.manager.Connection(nodeID)
	require.True(t, connected)
	assert.True(t, snapshot.Diagnostics)
	assert.Contains(t, snapshot.Capabilities, agentcontrol.CapabilityDiag)
	assert.NotContains(t, snapshot.ServerCapabilities, "diag.v1", "diag.v1 is the agent's, not advertised back")
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-1")
	require.NoError(t, stream.CloseSend())
}

// An Agent that advertises users.v1 and reports.v1 gets both back, receives
// its UserDelta (a full resync for cursor 0) and sends a TrafficReport on the
// same stream.
func TestAgentControlUsersAndReportsOnOneStream(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	// The user list reads the node's protocols and secrets too.
	requireAutoMigrate(t, &model.NodeProtocol{}, &model.Plan{}, &model.WireGuardPeer{})
	require.NoError(t, nodesecrets.EnsureSchema(database.GetDB()))
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities(agentcontrol.CapabilityUsers))
	for _, capability := range []string{agentcontrol.CapabilityUsers, agentcontrol.CapabilityReports} {
		assert.True(t, agentcontrol.Negotiated(reportCapabilities(agentcontrol.CapabilityUsers), helloAck.ServerCapabilities, capability), capability)
	}
	snapshot, connected := environment.manager.Connection(nodeID)
	require.True(t, connected)
	assert.ElementsMatch(t, []string{"users.v1", "reports.v1"}, snapshot.ServerCapabilities)

	report := &agentv1pb.TrafficReport{BatchId: "node:proxy-1:boot-a:1", Users: []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 10, DownloadBytes: 20}}}
	require.NoError(t, stream.Send(trafficMessage("traffic-1", nodeID, report)))
	// The full resync for cursor 0 and the ReportAck arrive in either order.
	var deltas, acks int
	for i := 0; i < 2; i++ {
		message, err := stream.Recv()
		require.NoError(t, err)
		switch payload := message.Payload.(type) {
		case *agentv1pb.ControlToAgent_Users:
			deltas++
			assert.True(t, payload.Users.Full)
			assert.True(t, payload.Users.LastPage)
		case *agentv1pb.ControlToAgent_ReportAck:
			acks++
			assert.Equal(t, "traffic-1", message.RequestId)
			assert.Equal(t, report.BatchId, payload.ReportAck.BatchId)
			assert.True(t, payload.ReportAck.Applied)
		default:
			t.Fatalf("unexpected payload %T", message.Payload)
		}
	}
	assert.Equal(t, 1, deltas)
	assert.Equal(t, 1, acks)
	upload, download := userCounters(t, 7)
	assert.Equal(t, [2]int64{10, 20}, [2]int64{upload, download})
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-end")
	require.NoError(t, stream.CloseSend())
}
