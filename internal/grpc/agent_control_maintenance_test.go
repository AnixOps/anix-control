package grpc

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentreports"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maintenanceEventJSON is an anixops.maintenance/v1 incident of nodeID, as
// the Agent's outbox encodes it.
func maintenanceEventJSON(t *testing.T, nodeID uint32, eventID string) []byte {
	t.Helper()
	occurred := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	firstFailed := occurred.Add(-2 * time.Minute)
	raw, err := json.Marshal(agentcontrol.MaintenanceEvent{
		SchemaVersion: 1, EventID: eventID, OccurredAt: occurred, Environment: "production", Source: "agent",
		NodeID: strconv.FormatUint(uint64(nodeID), 10), AgentVersion: "4.2.0", PluginID: "machine-telemetry",
		PluginVersion: "1.1.0", InstanceID: "machine-telemetry", ErrorCode: "PLUGIN_PROCESS_EXITED", Severity: "P1",
		Status: "open", FirstFailedAt: &firstFailed, ConsecutiveFailures: 3, SelfHealAction: "restart",
		SelfHealResult: "failed", RedactedSummary: "exited with status 1",
	})
	require.NoError(t, err)
	return raw
}

func maintenanceMessage(requestID string, nodeID uint32, version string, events ...[]byte) *agentv1pb.AgentToControl {
	return &agentv1pb.AgentToControl{
		RequestId: requestID, NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_MaintenanceEvents{MaintenanceEvents: &agentv1pb.MaintenanceEvents{Version: version, EventsJson: events}},
	}
}

// expectMaintenanceAck reads the next message and requires it to be the
// MaintenanceAck answering requestID.
func expectMaintenanceAck(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient, requestID string) *agentv1pb.MaintenanceAck {
	t.Helper()
	message, err := stream.Recv()
	require.NoError(t, err)
	ack := message.GetMaintenanceAck()
	require.NotNil(t, ack, "got %T", message.Payload)
	assert.Equal(t, requestID, message.RequestId)
	return ack
}

func maintenanceLogRows(t *testing.T, nodeID uint) []model.NodeLog {
	t.Helper()
	var rows []model.NodeLog
	require.NoError(t, database.GetDB().Where("node_id = ? AND source = ?", nodeID, service.NodeLogSourceMaintenance).Order("id").Find(&rows).Error)
	return rows
}

// A batch is answered event by event, in order: stored events are
// persisted (once, whatever the number of deliveries), and malformed ones,
// another node's, and events of a batch over the bounds are refused with
// the reason. The stream stays open.
func TestAgentControlMaintenanceEventsStoredOncePerEvent(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities(agentcontrol.CapabilityMaintenance))
	require.True(t, agentcontrol.Negotiated(reportCapabilities(agentcontrol.CapabilityMaintenance), helloAck.ServerCapabilities, agentcontrol.CapabilityMaintenance))
	persisted := agentMaintenanceMetrics.results[maintenancePersisted].Load()
	duplicates := agentMaintenanceMetrics.results[maintenanceDuplicate].Load()
	refused := agentMaintenanceMetrics.results[maintenanceRefused].Load()

	first := maintenanceEventJSON(t, nodeID, "event-1")
	require.NoError(t, stream.Send(maintenanceMessage("batch-1", nodeID, agentcontrol.MaintenanceSchemaV1,
		first, maintenanceEventJSON(t, nodeID+1, "event-other-node"), []byte(`{"event_id":"event-bad","schema_version":2}`), []byte("{"))))
	ack := expectMaintenanceAck(t, stream, "batch-1")
	assert.Equal(t, agentcontrol.MaintenanceSchemaV1, ack.Version)
	require.Len(t, ack.Events, 4)
	assert.Equal(t, &agentv1pb.MaintenanceEventResult{EventId: "event-1", Persisted: true}, stripResult(ack.Events[0]))
	assert.Equal(t, "event-other-node", ack.Events[1].EventId)
	assert.False(t, ack.Events[1].Persisted)
	assert.Contains(t, ack.Events[1].Error, "not the stream's node")
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeWrongNode, ack.Events[1].ErrorCode)
	assert.Equal(t, "event-bad", ack.Events[2].EventId)
	assert.Contains(t, ack.Events[2].Error, "schema_version")
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeEventInvalid, ack.Events[2].ErrorCode)
	assert.Empty(t, ack.Events[3].EventId, "an unreadable event has no id to echo")
	assert.NotEmpty(t, ack.Events[3].Error)
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeEventInvalid, ack.Events[3].ErrorCode)
	assert.Empty(t, ack.Events[0].ErrorCode, "a stored event has no code")
	assert.Zero(t, ack.Events[0].RetryAfterMs)

	rows := maintenanceLogRows(t, environment.node.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, service.NodeLogLevelError, rows[0].Level, "a P1 incident")
	assert.Equal(t, "event-1", rows[0].TraceID)
	assert.Contains(t, rows[0].Message, "maintenance open: plugin machine-telemetry instance machine-telemetry PLUGIN_PROCESS_EXITED (P1): exited with status 1")
	var stored agentcontrol.MaintenanceEvent
	require.NoError(t, json.Unmarshal([]byte(rows[0].FieldsJSON), &stored))
	assert.Equal(t, "event-1", stored.EventID)
	require.NotNil(t, rows[0].LoggedAt)

	// The agent lost the acknowledgement and resends: persisted again,
	// stored once.
	require.NoError(t, stream.Send(maintenanceMessage("batch-2", nodeID, agentcontrol.MaintenanceSchemaV1, first, first)))
	ack = expectMaintenanceAck(t, stream, "batch-2")
	require.Len(t, ack.Events, 2)
	for _, result := range ack.Events {
		assert.True(t, result.Persisted)
		assert.Empty(t, result.Error)
	}
	assert.Len(t, maintenanceLogRows(t, environment.node.ID), 1)

	// A batch over the bounds, or of another schema, is refused event by
	// event with the reason.
	var tooMany [][]byte
	for index := 0; index <= agentcontrol.MaxMaintenanceBatchEvents; index++ {
		tooMany = append(tooMany, maintenanceEventJSON(t, nodeID, "event-many-"+strconv.Itoa(index)))
	}
	require.NoError(t, stream.Send(maintenanceMessage("batch-3", nodeID, agentcontrol.MaintenanceSchemaV1, tooMany...)))
	ack = expectMaintenanceAck(t, stream, "batch-3")
	require.Len(t, ack.Events, agentcontrol.MaxMaintenanceBatchEvents+1)
	assert.Equal(t, "event-many-0", ack.Events[0].EventId)
	assert.Contains(t, ack.Events[0].Error, "more than 50")
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeBatchTooLarge, ack.Events[0].ErrorCode)
	require.NoError(t, stream.Send(maintenanceMessage("batch-4", nodeID, "anixops.maintenance/v2", maintenanceEventJSON(t, nodeID, "event-v2"))))
	ack = expectMaintenanceAck(t, stream, "batch-4")
	assert.Equal(t, "anixops.maintenance/v2", ack.Version)
	require.Len(t, ack.Events, 1)
	assert.Contains(t, ack.Events[0].Error, "unsupported maintenance schema")
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeSchemaUnsupported, ack.Events[0].ErrorCode)
	large := make([][]byte, 0, 20)
	for index := 0; index < 20; index++ {
		large = append(large, []byte(`{"event_id":"large-`+strconv.Itoa(index)+`","pad":"`+strings.Repeat("x", 15<<10)+`"}`))
	}
	require.NoError(t, stream.Send(maintenanceMessage("batch-5", nodeID, agentcontrol.MaintenanceSchemaV1, large...)))
	ack = expectMaintenanceAck(t, stream, "batch-5")
	assert.Contains(t, ack.Events[0].Error, "exceed")
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeBatchTooLarge, ack.Events[0].ErrorCode)
	assert.Len(t, maintenanceLogRows(t, environment.node.ID), 1)

	assert.Equal(t, persisted+1, agentMaintenanceMetrics.results[maintenancePersisted].Load())
	assert.Equal(t, duplicates+2, agentMaintenanceMetrics.results[maintenanceDuplicate].Load())
	assert.Equal(t, refused+3+uint64(agentcontrol.MaxMaintenanceBatchEvents)+1+1+20, agentMaintenanceMetrics.results[maintenanceRefused].Load())
	var body strings.Builder
	WriteAgentMaintenancePrometheus(&body)
	assert.Contains(t, body.String(), `anixops_agent_maintenance_events_total{result="persisted"}`)

	// The stream is still open.
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-after")
	require.NoError(t, stream.CloseSend())
}

func stripResult(result *agentv1pb.MaintenanceEventResult) *agentv1pb.MaintenanceEventResult {
	return &agentv1pb.MaintenanceEventResult{EventId: result.EventId, Persisted: result.Persisted, Error: result.Error}
}

// An event Control cannot store now is neither persisted nor refused: the
// agent keeps it, and its next delivery stores it. An event of a node that
// no longer exists is refused.
func TestAgentControlMaintenanceEventsUnrecordedAndGoneNode(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, _ := openReportsSession(t, environment, reportCapabilities(agentcontrol.CapabilityMaintenance))
	unrecorded := agentMaintenanceMetrics.results[maintenanceUnrecorded].Load()

	require.NoError(t, database.GetDB().Migrator().DropTable(&model.NodeLog{}))
	event := maintenanceEventJSON(t, nodeID, "event-transient")
	require.NoError(t, stream.Send(maintenanceMessage("batch-1", nodeID, agentcontrol.MaintenanceSchemaV1, event)))
	ack := expectMaintenanceAck(t, stream, "batch-1")
	require.Len(t, ack.Events, 1)
	assert.Equal(t, &agentv1pb.MaintenanceEventResult{EventId: "event-transient"}, stripResult(ack.Events[0]))
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeUnavailable, ack.Events[0].ErrorCode, "transient: a code, no error")
	assert.Equal(t, maintenanceRetryAfterMs, ack.Events[0].RetryAfterMs)
	assert.Equal(t, unrecorded+1, agentMaintenanceMetrics.results[maintenanceUnrecorded].Load())
	var claims int64
	require.NoError(t, database.GetDB().Model(&model.AgentReportBatch{}).Where("batch_id = ?", agentreports.MaintenanceBatchID("event-transient")).Count(&claims).Error)
	assert.Zero(t, claims, "the claim rolled back with the row")

	requireAutoMigrate(t, &model.NodeLog{})
	require.NoError(t, stream.Send(maintenanceMessage("batch-2", nodeID, agentcontrol.MaintenanceSchemaV1, event)))
	ack = expectMaintenanceAck(t, stream, "batch-2")
	assert.True(t, ack.Events[0].Persisted)
	assert.Empty(t, ack.Events[0].ErrorCode)
	assert.Zero(t, ack.Events[0].RetryAfterMs)
	assert.Len(t, maintenanceLogRows(t, environment.node.ID), 1)

	require.NoError(t, database.GetDB().Delete(&model.Node{}, environment.node.ID).Error)
	require.NoError(t, stream.Send(maintenanceMessage("batch-3", nodeID, agentcontrol.MaintenanceSchemaV1, maintenanceEventJSON(t, nodeID, "event-gone"))))
	ack = expectMaintenanceAck(t, stream, "batch-3")
	assert.False(t, ack.Events[0].Persisted)
	assert.Equal(t, "the node no longer exists", ack.Events[0].Error)
	assert.Equal(t, agentcontrol.MaintenanceErrorCodeNodeGone, ack.Events[0].ErrorCode)
	assert.Zero(t, ack.Events[0].RetryAfterMs)
	require.NoError(t, stream.CloseSend())
}

// maintenance.v1 is offered only to an agent that lists it, for proxy
// nodes; a batch on a session that did not negotiate it ends the stream.
// An empty batch is answered with an empty acknowledgement.
func TestAgentControlMaintenanceNegotiation(t *testing.T) {
	server := &AgentControlGRPCServer{}
	with := reportCapabilities(agentcontrol.CapabilityMaintenance)
	assert.True(t, agentcontrol.HasCapabilityVersion(server.serverCapabilities(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, with),
		agentcontrol.CapabilityMaintenance, agentcontrol.CapabilityVersionV1))
	assert.False(t, agentcontrol.HasCapabilityVersion(server.serverCapabilities(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, with),
		agentcontrol.CapabilityMaintenance, agentcontrol.CapabilityVersionV1), "forward nodes have no node log")
	assert.False(t, agentcontrol.HasCapabilityVersion(server.serverCapabilities(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, reportCapabilities()),
		agentcontrol.CapabilityMaintenance, agentcontrol.CapabilityVersionV1))

	environment := newReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, _ := openReportsSession(t, environment, reportCapabilities())
	require.NoError(t, stream.Send(maintenanceMessage("batch", nodeID, agentcontrol.MaintenanceSchemaV1, maintenanceEventJSON(t, nodeID, "event-1"))))
	_, err := stream.Recv()
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "maintenance.v1")

	stream, _ = openReportsSession(t, environment, reportCapabilities(agentcontrol.CapabilityMaintenance))
	require.NoError(t, stream.Send(maintenanceMessage("empty", nodeID, agentcontrol.MaintenanceSchemaV1)))
	ack := expectMaintenanceAck(t, stream, "empty")
	assert.Empty(t, ack.Events)
	assert.Empty(t, maintenanceLogRows(t, environment.node.ID))
	require.NoError(t, stream.CloseSend())
}

// The handler refuses a maintenance payload without its message, as the
// other data-plane payloads do.
func TestAgentControlMaintenanceRequiresThePayload(t *testing.T) {
	server := &AgentControlGRPCServer{}
	capabilities := reportCapabilities(agentcontrol.CapabilityMaintenance)
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	connection := &AgentControlConnection{NodeID: 1, Capabilities: capabilities, ServerCapabilities: server.serverCapabilities(node, capabilities)}
	err := server.handleMaintenanceEvents(NewAgentControlManager(), connection, node, &agentv1pb.AgentToControl{RequestId: "nil"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
