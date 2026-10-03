package grpc

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Maintenance events on the Agent Control stream (maintenance.v1;
// PROTOCOL.md, "Maintenance events"). The Agent keeps plugin health
// incidents and recoveries in a durable outbox until Control has stored
// them; before maintenance.v1 it sent them only on the legacy agent
// WebSocket (maintenance_events, answered by maintenance_ack), which
// agent_control.mtls: required refuses. When an agent's Hello lists
// maintenance.v1, the HelloAck advertises it back (proxy nodes, whose node
// log stores the events) and the stream accepts MaintenanceEvents.
//
// Each event is stored once per node and event id, as a node log row of
// source "maintenance" (NodeLogService.RecordAgentMaintenanceEvent), and
// every batch is answered by one MaintenanceAck with a result per event,
// in the batch's order:
//
//   - persisted: stored, by this delivery or an earlier one; the Agent
//     removes it from its outbox;
//   - an error: refused for good (malformed, another node's, a batch over
//     the bounds, a node that no longer exists); refused again on every
//     delivery;
//   - neither: Control could not store it now (its database failed); the
//     Agent sends it again.
//
// A bad event never ends the stream; only a batch sent without the
// capability negotiated does, as every data-plane payload does.

// servesMaintenance tells whether the HelloAck advertises maintenance.v1:
// when the agent lists it and the stream's node is a proxy node, whose
// node log stores the events.
func (s *AgentControlGRPCServer) servesMaintenance(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	return node.Kind == agentcontrol.NodeKindProxy &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityMaintenance, agentcontrol.CapabilityVersionV1)
}

// handleMaintenanceEvents stores a batch of maintenance events for the
// stream's node and answers it. It returns an error only when the stream
// must end.
func (s *AgentControlGRPCServer) handleMaintenanceEvents(manager *AgentControlManager, connection *AgentControlConnection, node agentcontrol.AgentNode, message *agentv1pb.AgentToControl) error {
	if !agentcontrol.Negotiated(connection.Capabilities, connection.ServerCapabilities, agentcontrol.CapabilityMaintenance) {
		return unnegotiatedPayload("maintenance_events", agentcontrol.CapabilityMaintenance)
	}
	batch := message.GetMaintenanceEvents()
	if batch == nil {
		return status.Error(codes.InvalidArgument, "maintenance_events payload is required")
	}
	refusal := maintenanceBatchRefusal(batch)
	now := time.Now()
	ack := &agentv1pb.MaintenanceAck{Version: batch.Version, Events: make([]*agentv1pb.MaintenanceEventResult, 0, len(batch.EventsJson))}
	for _, raw := range batch.EventsJson {
		result := &agentv1pb.MaintenanceEventResult{EventId: agentcontrol.MaintenanceEventID(raw)}
		ack.Events = append(ack.Events, result)
		if refusal != "" {
			result.Error = refusal
			agentMaintenanceMetrics.result(maintenanceRefused)
			continue
		}
		event, err := agentcontrol.ParseMaintenanceEvent(raw, node, now)
		if err != nil {
			result.Error = err.Error()
			agentMaintenanceMetrics.result(maintenanceRefused)
			slog.Warn("refused agent maintenance event", "component", "agent-control", "node", node.String(), "event_id", logValue(result.EventId), "error", err)
			continue
		}
		recorded, err := s.reports.logs.RecordAgentMaintenanceEvent(node.Kind, uint(node.ID), event)
		switch {
		case err == nil:
			result.Persisted = true
			if recorded {
				agentMaintenanceMetrics.result(maintenancePersisted)
			} else {
				agentMaintenanceMetrics.result(maintenanceDuplicate)
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			result.Error = "the node no longer exists"
			agentMaintenanceMetrics.result(maintenanceRefused)
		default:
			// Transient: neither persisted nor refused, so the Agent keeps
			// the event and sends it again.
			agentMaintenanceMetrics.result(maintenanceUnrecorded)
			slog.Warn("failed to record agent maintenance event", "component", "agent-control", "node", node.String(), "event_id", logValue(event.EventID), "error", err)
		}
	}
	return connection.send(&agentv1pb.ControlToAgent{
		RequestId:    message.RequestId,
		NodeId:       node.ID,
		Revision:     manager.DesiredRevision(node.ID),
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload:      &agentv1pb.ControlToAgent_MaintenanceAck{MaintenanceAck: ack},
	})
}

// maintenanceBatchRefusal is why a whole batch is refused, empty when its
// events are read one by one.
func maintenanceBatchRefusal(batch *agentv1pb.MaintenanceEvents) string {
	if batch.Version != agentcontrol.MaintenanceSchemaV1 {
		return fmt.Sprintf("unsupported maintenance schema %q, want %s", logValue(batch.Version), agentcontrol.MaintenanceSchemaV1)
	}
	if len(batch.EventsJson) > agentcontrol.MaxMaintenanceBatchEvents {
		return fmt.Sprintf("the batch has %d events, more than %d", len(batch.EventsJson), agentcontrol.MaxMaintenanceBatchEvents)
	}
	size := 0
	for _, raw := range batch.EventsJson {
		size += len(raw)
	}
	if size > agentcontrol.MaxMaintenanceBatchBytes {
		return fmt.Sprintf("the batch's events exceed %d bytes", agentcontrol.MaxMaintenanceBatchBytes)
	}
	return ""
}

// The result label of anixops_agent_maintenance_events_total.
const (
	maintenancePersisted  = "persisted"
	maintenanceDuplicate  = "duplicate"
	maintenanceRefused    = "refused"
	maintenanceUnrecorded = "unrecorded"
)

var maintenanceResults = []string{maintenancePersisted, maintenanceDuplicate, maintenanceRefused, maintenanceUnrecorded}

// maintenanceMetrics counts the maintenance events the stream received, by
// a closed set of results.
type maintenanceMetrics struct {
	results map[string]*atomic.Uint64
}

var agentMaintenanceMetrics = func() *maintenanceMetrics {
	metrics := &maintenanceMetrics{results: map[string]*atomic.Uint64{}}
	for _, result := range maintenanceResults {
		metrics.results[result] = &atomic.Uint64{}
	}
	return metrics
}()

func (m *maintenanceMetrics) result(result string) {
	if counter := m.results[result]; counter != nil {
		counter.Add(1)
	}
}

// WriteAgentMaintenancePrometheus renders the maintenance event counter in
// the Prometheus text format.
func WriteAgentMaintenancePrometheus(body *strings.Builder) {
	body.WriteString("# HELP anixops_agent_maintenance_events_total Maintenance events received on the Agent Control stream (maintenance.v1), by result: persisted (stored now), duplicate (stored before), refused or unrecorded (the database failed; the Agent resends).\n")
	body.WriteString("# TYPE anixops_agent_maintenance_events_total counter\n")
	for _, result := range maintenanceResults {
		body.WriteString("anixops_agent_maintenance_events_total{result=\"" + result + "\"} " + strconv.FormatUint(agentMaintenanceMetrics.results[result].Load(), 10) + "\n")
	}
}
