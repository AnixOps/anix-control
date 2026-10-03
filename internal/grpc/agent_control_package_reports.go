package grpc

import (
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Package reports on the Agent Control stream (package-reports.v1;
// PROTOCOL.md, "Package reports"). When an agent's Hello lists
// package-reports.v1, the HelloAck advertises it back and the stream accepts
// PackageReport: the latest observation of one kind that a plugin package
// makes on the node, such as the systemd services table of
// machine-telemetry. NodeService.RecordPackageReport authorizes the
// reporting release, sanitizes the payload and keeps the newest report per
// node, plugin and kind.
//
// A PackageReport is never acknowledged. A refused report (malformed,
// oversize, unauthorized or a payload its kind's sanitizer refuses) and one
// the kernel cannot record now are logged, counted and dropped; the stream
// stays open, since the next report replaces it anyway. Only a report sent
// without the capability negotiated ends the stream, as every data-plane
// payload does.

// servesPackageReports tells whether the HelloAck advertises
// package-reports.v1: when the agent lists it and the stream's node is a
// proxy node, whose plugin assignments authorize the reports. Forward-node
// package reports join with the forward plugins.
func (s *AgentControlGRPCServer) servesPackageReports(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	return node.Kind == agentcontrol.NodeKindProxy &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityPackageReports, agentcontrol.CapabilityVersionV1)
}

// handlePackageReport records one PackageReport for the stream's node. It
// returns an error only when the stream must end.
func (s *AgentControlGRPCServer) handlePackageReport(connection *AgentControlConnection, node agentcontrol.AgentNode, report *agentv1pb.PackageReport) error {
	if !agentcontrol.Negotiated(connection.Capabilities, connection.ServerCapabilities, agentcontrol.CapabilityPackageReports) {
		return unnegotiatedPayload("package_report", agentcontrol.CapabilityPackageReports)
	}
	if report == nil {
		return status.Error(codes.InvalidArgument, "package_report payload is required")
	}
	input := service.PackageReportInput{
		NodeKind: node.Kind, NodeID: uint(node.ID),
		PluginID: report.PluginId, Kind: report.Kind, Version: report.Version, Payload: report.PayloadJson,
	}
	if report.ObservedAtUnixMs > 0 {
		input.ObservedAt = time.UnixMilli(report.ObservedAtUnixMs)
	}
	stored, err := s.nodeService.RecordPackageReport(input, time.Now())
	switch reason := service.PackageReportRefusal(err); {
	case err == nil && stored:
		agentPackageReportMetrics.result(packageReportAccepted)
	case err == nil:
		agentPackageReportMetrics.result(packageReportSuperseded)
	case reason != "":
		agentPackageReportMetrics.refused(reason)
		slog.Warn("refused agent package report", "component", "agent-control", "node", node.String(),
			"plugin_id", logValue(report.PluginId), "kind", logValue(report.Kind), "reason", reason, "error", err)
	default:
		agentPackageReportMetrics.result(packageReportUnrecorded)
		slog.Warn("failed to record agent package report", "component", "agent-control", "node", node.String(),
			"plugin_id", logValue(report.PluginId), "kind", logValue(report.Kind), "error", err)
	}
	return nil
}

// logValue bounds an agent-chosen string before it reaches the log.
func logValue(value string) string {
	const limit = 128
	if len(value) > limit {
		value = value[:limit] + "..."
	}
	return strconv.Quote(value)
}

// The result label of anixops_agent_package_reports_total.
const (
	packageReportAccepted   = "accepted"
	packageReportSuperseded = "superseded"
	packageReportRefused    = "refused"
	packageReportUnrecorded = "unrecorded"
)

var packageReportResults = []string{packageReportAccepted, packageReportSuperseded, packageReportRefused, packageReportUnrecorded}

// packageReportMetrics counts the package reports the stream received. The
// labels are closed sets: never a plugin id or kind, which agents choose.
type packageReportMetrics struct {
	results map[string]*atomic.Uint64
	reasons map[string]*atomic.Uint64
}

var agentPackageReportMetrics = newPackageReportMetrics()

func newPackageReportMetrics() *packageReportMetrics {
	metrics := &packageReportMetrics{results: map[string]*atomic.Uint64{}, reasons: map[string]*atomic.Uint64{}}
	for _, result := range packageReportResults {
		metrics.results[result] = &atomic.Uint64{}
	}
	for _, reason := range service.PackageReportRefusalReasons {
		metrics.reasons[reason] = &atomic.Uint64{}
	}
	return metrics
}

func (m *packageReportMetrics) result(result string) {
	if counter := m.results[result]; counter != nil {
		counter.Add(1)
	}
}

func (m *packageReportMetrics) refused(reason string) {
	m.result(packageReportRefused)
	if counter := m.reasons[reason]; counter != nil {
		counter.Add(1)
	}
}

// WriteAgentPackageReportPrometheus renders the package report counters in
// the Prometheus text format.
func WriteAgentPackageReportPrometheus(body *strings.Builder) {
	body.WriteString("# HELP anixops_agent_package_reports_total PackageReport messages received on the Agent Control stream, by result: accepted (stored), superseded (a newer report was stored), refused or unrecorded (the database failed).\n")
	body.WriteString("# TYPE anixops_agent_package_reports_total counter\n")
	for _, result := range packageReportResults {
		body.WriteString("anixops_agent_package_reports_total{result=\"" + result + "\"} " + strconv.FormatUint(agentPackageReportMetrics.results[result].Load(), 10) + "\n")
	}
	body.WriteString("# HELP anixops_agent_package_reports_refused_total PackageReport messages refused, by reason.\n")
	body.WriteString("# TYPE anixops_agent_package_reports_refused_total counter\n")
	for _, reason := range service.PackageReportRefusalReasons {
		body.WriteString("anixops_agent_package_reports_refused_total{reason=\"" + reason + "\"} " + strconv.FormatUint(agentPackageReportMetrics.reasons[reason].Load(), 10) + "\n")
	}
}
