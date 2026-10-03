package grpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentreports"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Reports on the Agent Control stream (A2-5; PROTOCOL.md, "Reports"). When
// an agent's Hello lists reports.v1, the HelloAck advertises it back and the
// stream accepts TrafficReport, LogBatch and NodeStatus for the stream's
// node. Each report goes where the legacy path of the same node puts it:
//
//   - traffic through RecordNodeTrafficReport's transaction, as ReportTraffic
//     and the UniProxy push, so a byte is counted once whichever path
//     carried it;
//   - online IPs into the alive set ReportOnline feeds;
//   - logs into v2_node_log, as NodeLogService.ReportLogs;
//   - status into the node's heartbeat and runtime-health columns, as
//     ReportStatus.
//
// A TrafficReport or LogBatch is applied at most once per node and batch
// id (internal/agentreports) and answered with a ReportAck:
//
//   - applied true: this delivery recorded it;
//   - applied false, no error: a committed delivery recorded it before, and
//     nothing was counted again;
//   - applied false with an error and its error_code: refused for good (a
//     malformed batch, a node that no longer exists); the agent drops it.
//
// A batch the kernel cannot record for now (the database failed) gets no
// ReportAck and the stream stays open: the agent's spool resends it. An
// agent whose reports.v1 negotiated the transient_ack attribute gets
// applied false, no error, error_code report_unavailable and a retry hint
// instead. A NodeStatus is never acknowledged.

// agentReportSinks are the legacy sinks the stream's reports feed.
type agentReportSinks struct {
	server *service.ServerService
	logs   *service.NodeLogService
}

func newAgentReportSinks() *agentReportSinks {
	return &agentReportSinks{server: service.NewServerService(), logs: service.NewNodeLogService()}
}

// reportRetryAfterMs is ReportAck.retry_after_ms of a batch Control could
// not record now (report_unavailable): 15 s.
const reportRetryAfterMs uint32 = 15000

// reportRefused is a permanent refusal of a batch: the agent drops it. code
// is its ReportAck.error_code (agentcontrol.ReportErrorCode*).
type reportRefused struct{ code, reason string }

func (e reportRefused) Error() string { return e.reason }

func refuseReport(code, format string, args ...any) error {
	return reportRefused{code: code, reason: fmt.Sprintf(format, args...)}
}

// reportRefusalCode tells a permanent failure from a transient one: the
// ReportAck.error_code of a refusal, empty for a transient failure.
func reportRefusalCode(err error) string {
	var refused reportRefused
	switch {
	case errors.As(err, &refused):
		return refused.code
	case errors.Is(err, gorm.ErrRecordNotFound):
		return agentcontrol.ReportErrorCodeNodeGone
	case errors.Is(err, agentreports.ErrInvalidBatchID):
		return agentcontrol.ReportErrorCodeBatchIDInvalid
	case errors.Is(err, service.ErrNegativeTraffic):
		return agentcontrol.ReportErrorCodeInvalid
	}
	return ""
}

// reportsServerCapability is reports.v1 as the HelloAck lists it: with the
// transient_ack attribute when the agent's reports.v1 asks for it, since
// this Control answers a batch it cannot record now with
// report_unavailable to such an agent.
func reportsServerCapability(agent []*agentv1pb.Capability) *agentv1pb.Capability {
	capability := &agentv1pb.Capability{Name: agentcontrol.CapabilityReports, Version: agentcontrol.CapabilityVersionV1}
	if agentcontrol.TransientReportAcks(agent, agent) {
		capability.Attributes = map[string]string{agentcontrol.ReportsAttributeTransientAck: agentcontrol.ReportsTransientAckV1}
	}
	return capability
}

// servesReports tells whether the HelloAck advertises reports.v1 to an
// agent: when the agent lists it and the stream's node is a proxy node
// (v2_node), whose legacy report sinks exist. Forward-node reports join
// with A5.
func (s *AgentControlGRPCServer) servesReports(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	return node.Kind == agentcontrol.NodeKindProxy &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityReports, agentcontrol.CapabilityVersionV1)
}

// handleReport applies one traffic, logs or status payload for the stream's
// node and answers it as described above. It returns an error only when the
// stream must end.
func (s *AgentControlGRPCServer) handleReport(manager *AgentControlManager, connection *AgentControlConnection, node agentcontrol.AgentNode, message *agentv1pb.AgentToControl) error {
	negotiated := agentcontrol.Negotiated(connection.Capabilities, connection.ServerCapabilities, agentcontrol.CapabilityReports)
	var (
		batchID string
		applied bool
		err     error
	)
	switch payload := message.Payload.(type) {
	case *agentv1pb.AgentToControl_Traffic:
		if !negotiated {
			return unnegotiatedPayload("traffic", agentcontrol.CapabilityReports)
		}
		if payload.Traffic == nil {
			return status.Error(codes.InvalidArgument, "traffic payload is required")
		}
		batchID = payload.Traffic.BatchId
		applied, err = s.applyTrafficReport(node, payload.Traffic)
	case *agentv1pb.AgentToControl_Logs:
		if !negotiated {
			return unnegotiatedPayload("logs", agentcontrol.CapabilityReports)
		}
		if payload.Logs == nil {
			return status.Error(codes.InvalidArgument, "logs payload is required")
		}
		batchID = payload.Logs.BatchId
		applied, err = s.applyLogBatch(node, payload.Logs)
	case *agentv1pb.AgentToControl_Status:
		if !negotiated {
			return unnegotiatedPayload("status", agentcontrol.CapabilityReports)
		}
		if payload.Status == nil {
			return status.Error(codes.InvalidArgument, "status payload is required")
		}
		if err := s.applyNodeStatus(node, payload.Status); err != nil {
			slog.Warn("failed to record agent node status", "component", "agent-control", "node", node.String(), "error", err)
		}
		// As the legacy heartbeat and runtime-health requests are, the
		// status is a sighting of the session's transport (mtls-stream or
		// apikey-stream); the recorder writes at most once a minute.
		agenttransport.Seen(connection.stream.Context(), connection.principal.sighting(connection.AgentVersion))
		return nil
	default:
		return status.Error(codes.InvalidArgument, "control message payload is required")
	}

	ack := &agentv1pb.ReportAck{BatchId: batchID, Applied: applied}
	switch code := reportRefusalCode(err); {
	case err == nil:
	case code != "":
		ack.Applied = false
		ack.Error = err.Error()
		ack.ErrorCode = code
		slog.Warn("refused agent report batch", "component", "agent-control", "node", node.String(), "batch_id", batchID, "error_code", code, "error", err)
	default:
		// Transient: the agent keeps the batch and resends it. An agent
		// that negotiated transient_ack is told so with a retry hint;
		// any other gets no acknowledgement, as an acknowledgement
		// would make it drop the batch.
		slog.Warn("failed to record agent report batch", "component", "agent-control", "node", node.String(), "batch_id", batchID, "error", err)
		if !agentcontrol.TransientReportAcks(connection.Capabilities, connection.ServerCapabilities) {
			return nil
		}
		ack.Applied = false
		ack.ErrorCode = agentcontrol.ReportErrorCodeUnavailable
		ack.RetryAfterMs = reportRetryAfterMs
	}
	return connection.send(&agentv1pb.ControlToAgent{
		RequestId:    message.RequestId,
		NodeId:       node.ID,
		Revision:     manager.DesiredRevision(node.ID),
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload:      &agentv1pb.ControlToAgent_ReportAck{ReportAck: ack},
	})
}

// applyTrafficReport counts a traffic batch once, as ReportTraffic counts the
// legacy report, and replaces the node's online set, as ReportOnline does.
func (s *AgentControlGRPCServer) applyTrafficReport(node agentcontrol.AgentNode, report *agentv1pb.TrafficReport) (bool, error) {
	if err := agentreports.ValidateBatchID(report.BatchId); err != nil {
		return false, err
	}
	traffics := make(map[uint][2]int64, len(report.Users))
	for index, user := range report.Users {
		if user == nil || user.UserId == 0 {
			return false, refuseReport(agentcontrol.ReportErrorCodeInvalid, "traffic entry %d: user_id is required", index)
		}
		if user.UploadBytes > math.MaxInt64 || user.DownloadBytes > math.MaxInt64 {
			return false, refuseReport(agentcontrol.ReportErrorCodeCounterOverflow, "traffic entry %d: bytes exceed the counter range", index)
		}
		userID := uint(user.UserId)
		total := traffics[userID]
		upload, download := int64(user.UploadBytes), int64(user.DownloadBytes)
		if upload > math.MaxInt64-total[0] || download > math.MaxInt64-total[1] {
			return false, refuseReport(agentcontrol.ReportErrorCodeCounterOverflow, "traffic entry %d: bytes exceed the counter range", index)
		}
		traffics[userID] = [2]int64{total[0] + upload, total[1] + download}
	}
	userIPs := make(map[uint][]string, len(report.Online))
	for index, online := range report.Online {
		if online == nil || online.UserId == 0 {
			return false, refuseReport(agentcontrol.ReportErrorCodeInvalid, "online entry %d: user_id is required", index)
		}
		ips := make([]string, 0, len(online.Ips))
		for _, ip := range online.Ips {
			if ip = strings.TrimSpace(ip); ip != "" {
				ips = append(ips, ip)
			}
		}
		userIPs[uint(online.UserId)] = append(userIPs[uint(online.UserId)], ips...)
	}

	if err := s.touchNode(node); err != nil {
		slog.Warn("failed to update node heartbeat before traffic report", "component", "agent-control", "node", node.String(), "error", err)
	}
	rate := 1.0
	if current, err := s.nodeService.GetNode(uint(node.ID)); err == nil {
		rate = current.Rate
	}
	applied, err := s.reports.server.RecordAgentTrafficReport(node.Kind, uint(node.ID), report.BatchId, traffics, rate)
	if err != nil || !applied {
		return applied, err
	}
	// The online set is a cache; the batch is committed whatever happens to
	// it, and the next report replaces it.
	if err := s.reports.server.UpdateOnlineStatus(model.ServerType(""), uint(node.ID), userIPs); err != nil {
		slog.Warn("failed to update online status from traffic report", "component", "agent-control", "node", node.String(), "error", err)
	}
	onlineUsers := 0
	for _, ips := range userIPs {
		if len(ips) > 0 {
			onlineUsers++
		}
	}
	if err := s.nodeService.UpdateOnlineUsers(uint(node.ID), onlineUsers); err != nil {
		slog.Warn("failed to update online user count from traffic report", "component", "agent-control", "node", node.String(), "error", err)
	}
	return true, nil
}

// applyLogBatch records a log batch once, as ReportLogs records the legacy
// batch, with the runtime health a WireGuard entry may carry.
func (s *AgentControlGRPCServer) applyLogBatch(node agentcontrol.AgentNode, batch *agentv1pb.LogBatch) (bool, error) {
	if err := agentreports.ValidateBatchID(batch.BatchId); err != nil {
		return false, err
	}
	inputs := make([]service.NodeLogInput, 0, len(batch.Entries))
	for index, entry := range batch.Entries {
		if entry == nil {
			return false, refuseReport(agentcontrol.ReportErrorCodeInvalid, "log entry %d is empty", index)
		}
		message := strings.TrimSpace(entry.Message)
		if message == "" {
			continue
		}
		fields := strings.TrimSpace(string(entry.FieldsJson))
		if fields != "" && !json.Valid([]byte(fields)) {
			return false, refuseReport(agentcontrol.ReportErrorCodeInvalid, "log entry %d: fields_json is not JSON", index)
		}
		inputs = append(inputs, service.NodeLogInput{
			Level:      entry.Level,
			Source:     entry.Source,
			Message:    message,
			TraceID:    entry.TraceId,
			FieldsJSON: fields,
			LoggedAt:   normalizeLoggedAt(entry.LoggedAtUnixMs),
		})
	}
	applied, err := s.reports.logs.RecordAgentLogBatch(node.Kind, uint(node.ID), batch.BatchId, inputs)
	if err != nil || !applied {
		return applied, err
	}
	if healthy, runtimeError := nodeLogRuntimeHealth(inputs); healthy != nil {
		if err := s.nodeService.UpdateRuntimeHealth(uint(node.ID), *healthy, runtimeError); err != nil {
			slog.Warn("failed to update runtime health from log batch", "component", "agent-control", "node", node.String(), "error", err)
		}
	}
	if err := s.touchNode(node); err != nil {
		slog.Warn("failed to update node heartbeat after log batch", "component", "agent-control", "node", node.String(), "error", err)
	}
	return true, nil
}

// applyNodeStatus writes a NodeStatus as ReportStatus and the runtime-health
// report write theirs. It is never acknowledged.
func (s *AgentControlGRPCServer) applyNodeStatus(node agentcontrol.AgentNode, report *agentv1pb.NodeStatus) error {
	return s.nodeService.RecordAgentNodeStatus(uint(node.ID), service.AgentNodeStatus{
		CPUUsage:       report.CpuUsagePercent,
		MemoryUsage:    report.MemoryUsagePercent,
		DiskUsage:      report.DiskUsagePercent,
		Uptime:         report.UptimeSeconds,
		RuntimeHealthy: report.RuntimeHealthy,
		RuntimeError:   report.RuntimeError,
	})
}
