package grpc

import (
	"crypto/ed25519"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	packageReportVersion = "4.1.0"
	systemdPayload       = `{"supported": true, "window_seconds": 600, "units": [
		{"name": "sshd.service", "active_state": "active", "sub_state": "running", "cpu_avg_percent": 0.5, "cpu_peak_percent": 4, "memory_bytes": 1024, "memory_peak_bytes": 2048, "main_pid": 1}]}`
)

// newPackageReportsTestEnvironment is the agent control environment with
// the package report table and three Agent releases enabled on the node:
// machine-telemetry 4.1.0 declaring telemetry.systemd.read, lite-telemetry
// 1.0.0 declaring only telemetry.read, and unsigned-telemetry 1.0.0, whose
// stored signature no longer verifies.
func newPackageReportsTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newAgentControlTestEnvironment(t)
	requireAutoMigrate(t, &model.PackageReportState{})
	for _, release := range []struct {
		id, version  string
		capabilities []string
	}{
		{"machine-telemetry", packageReportVersion, []string{"telemetry.read", systemdreport.Capability}},
		{"lite-telemetry", "1.0.0", []string{"telemetry.read"}},
		{"unsigned-telemetry", "1.0.0", []string{systemdreport.Capability}},
	} {
		manifest := service.PluginManifest{
			ID: release.id, Name: release.id, Version: release.version, APIVersion: "v1", Publisher: "AnixOps",
			Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: release.capabilities,
		}
		canonical, err := service.CanonicalPluginManifest(manifest)
		require.NoError(t, err)
		_, err = service.RegisterPluginRelease(database.GetDB(), string(canonical),
			base64.StdEncoding.EncodeToString(ed25519.Sign(environment.signer, canonical)), environment.signer.Public().(ed25519.PublicKey))
		require.NoError(t, err)
		require.NoError(t, database.GetDB().Create(&model.NodeServiceAssignment{
			NodeID: environment.node.ID, ServiceScope: "telemetry", PluginID: release.id, Role: "agent", DesiredVersion: release.version, Enabled: true,
		}).Error)
	}
	require.NoError(t, database.GetDB().Model(&model.PluginRelease{}).Where("plugin_id = ?", "unsigned-telemetry").Update("signature", "").Error)
	return environment
}

func packageReportCapabilities() []*agentv1pb.Capability {
	return []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityPackageReports, Version: agentcontrol.CapabilityVersionV1},
	}
}

func packageReportMessage(requestID string, nodeID uint32, report *agentv1pb.PackageReport) *agentv1pb.AgentToControl {
	return &agentv1pb.AgentToControl{
		RequestId: requestID, NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_PackageReport{PackageReport: report},
	}
}

func systemdReport(observedAt time.Time) *agentv1pb.PackageReport {
	return &agentv1pb.PackageReport{
		PluginId: "machine-telemetry", Kind: systemdreport.Kind, Version: packageReportVersion,
		PayloadJson: []byte(systemdPayload), ObservedAtUnixMs: observedAt.UnixMilli(),
	}
}

type packageReportCounts struct {
	results map[string]uint64
	reasons map[string]uint64
}

func readPackageReportCounts() packageReportCounts {
	counts := packageReportCounts{results: map[string]uint64{}, reasons: map[string]uint64{}}
	for result, counter := range agentPackageReportMetrics.results {
		counts.results[result] = counter.Load()
	}
	for reason, counter := range agentPackageReportMetrics.reasons {
		counts.reasons[reason] = counter.Load()
	}
	return counts
}

func storedPackageReports(t *testing.T) []model.PackageReportState {
	t.Helper()
	var rows []model.PackageReportState
	require.NoError(t, database.GetDB().Order("plugin_id, kind").Find(&rows).Error)
	return rows
}

// With package-reports.v1 in the Hello the HelloAck advertises it and the
// session records it; without it the payload ends the stream.
func TestAgentControlPackageReportsNegotiation(t *testing.T) {
	environment := newPackageReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)

	t.Run("on", func(t *testing.T) {
		stream, helloAck := openReportsSession(t, environment, packageReportCapabilities())
		assert.True(t, agentcontrol.Negotiated(packageReportCapabilities(), helloAck.ServerCapabilities, agentcontrol.CapabilityPackageReports))
		assert.False(t, agentcontrol.HasCapabilityVersion(helloAck.ServerCapabilities, agentcontrol.CapabilityReports, agentcontrol.CapabilityVersionV1), "reports.v1 is separate")
		snapshot, connected := environment.manager.Connection(nodeID)
		require.True(t, connected)
		assert.Contains(t, snapshot.ServerCapabilities, "package-reports.v1")
		require.NoError(t, stream.CloseSend())
	})

	t.Run("off", func(t *testing.T) {
		stream, helloAck := openReportsSession(t, environment, reportCapabilities())
		assert.False(t, agentcontrol.HasCapabilityVersion(helloAck.ServerCapabilities, agentcontrol.CapabilityPackageReports, agentcontrol.CapabilityVersionV1))
		require.NoError(t, stream.Send(packageReportMessage("report-1", nodeID, systemdReport(time.Now()))))
		_, err := stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, status.Convert(err).Message(), "requires the package-reports.v1 server capability")
		assert.Empty(t, storedPackageReports(t))
	})
}

// A forward node's stream is not offered package-reports.v1 yet.
func TestAgentControlPackageReportsNotOfferedToForwardNodes(t *testing.T) {
	server := &AgentControlGRPCServer{}
	offered := func(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
		return agentcontrol.HasCapabilityVersion(server.serverCapabilities(node, agent), agentcontrol.CapabilityPackageReports, agentcontrol.CapabilityVersionV1)
	}
	assert.False(t, offered(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, packageReportCapabilities()))
	assert.True(t, offered(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, packageReportCapabilities()))
	assert.False(t, offered(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, validAgentHello(1).GetHello().Capabilities), "not listed by the agent")
}

// An accepted report is stored sanitized, as the latest of its node, plugin
// and kind, and never acknowledged: the next message is the heartbeat's
// answer. An older report is dropped, a newer one replaces it.
func TestAgentControlPackageReportStoredLatestOnly(t *testing.T) {
	environment := newPackageReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, packageReportCapabilities())
	before := readPackageReportCounts()
	observedAt := time.Now().Add(-time.Minute).Truncate(time.Millisecond)

	require.NoError(t, stream.Send(packageReportMessage("report-1", nodeID, systemdReport(observedAt))))
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-1")
	rows := storedPackageReports(t)
	require.Len(t, rows, 1)
	assert.Equal(t, agentcontrol.NodeKindProxy, rows[0].NodeKind)
	assert.Equal(t, environment.node.ID, rows[0].NodeID)
	assert.Equal(t, packageReportVersion, rows[0].Version)
	assert.NotContains(t, rows[0].PayloadJSON, "main_pid", "unknown fields are dropped")
	assert.True(t, rows[0].ObservedAt.Equal(observedAt))

	older := systemdReport(observedAt.Add(-time.Minute))
	older.PayloadJson = []byte(`{"supported": false, "unsupported_reason": "older"}`)
	require.NoError(t, stream.Send(packageReportMessage("report-2", nodeID, older)))
	newer := systemdReport(time.Time{})
	newer.ObservedAtUnixMs = 0
	newer.PayloadJson = []byte(`{"supported": false, "unsupported_reason": "cgroup v1"}`)
	require.NoError(t, stream.Send(packageReportMessage("report-3", nodeID, newer)))
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-2")

	rows = storedPackageReports(t)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].PayloadJSON, "cgroup v1")
	report, err := service.NewNodeService().LatestPackageReport(agentcontrol.NodeKindProxy, environment.node.ID, "machine-telemetry", systemdreport.Kind, time.Now())
	require.NoError(t, err)
	assert.False(t, report.Stale)
	report, err = service.NewNodeService().LatestPackageReport(agentcontrol.NodeKindProxy, environment.node.ID, "machine-telemetry", systemdreport.Kind, time.Now().Add(26*time.Minute))
	require.NoError(t, err)
	assert.True(t, report.Stale)

	after := readPackageReportCounts()
	assert.Equal(t, uint64(2), after.results[packageReportAccepted]-before.results[packageReportAccepted])
	assert.Equal(t, uint64(1), after.results[packageReportSuperseded]-before.results[packageReportSuperseded])
	require.NoError(t, stream.CloseSend())
}

// A refused report is counted by reason and dropped, and the stream stays
// open.
func TestAgentControlPackageReportRefusals(t *testing.T) {
	environment := newPackageReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, packageReportCapabilities())
	now := time.Now()
	edit := func(change func(*agentv1pb.PackageReport)) *agentv1pb.PackageReport {
		report := systemdReport(now)
		change(report)
		return report
	}
	cases := []struct {
		name   string
		report *agentv1pb.PackageReport
		reason string
	}{
		{"empty", &agentv1pb.PackageReport{}, service.PackageReportRefusedInvalid},
		{"bad kind", edit(func(r *agentv1pb.PackageReport) { r.Kind = "systemd/services" }), service.PackageReportRefusedInvalid},
		{"oversize", edit(func(r *agentv1pb.PackageReport) {
			r.PayloadJson = []byte(`{"supported": true, "pad": "` + strings.Repeat("x", agentcontrol.MaxPackageReportPayloadBytes) + `"}`)
		}), service.PackageReportRefusedOversize},
		{"future", edit(func(r *agentv1pb.PackageReport) { r.ObservedAtUnixMs = now.Add(time.Hour).UnixMilli() }), service.PackageReportRefusedFuture},
		{"unknown kind", edit(func(r *agentv1pb.PackageReport) { r.Kind = "forward.rules" }), service.PackageReportRefusedUnknownKind},
		{"not assigned", edit(func(r *agentv1pb.PackageReport) { r.PluginId = "gost-mesh" }), service.PackageReportRefusedNotAssigned},
		{"version mismatch", edit(func(r *agentv1pb.PackageReport) { r.Version = "4.0.0" }), service.PackageReportRefusedVersionMismatch},
		{"unsigned", edit(func(r *agentv1pb.PackageReport) { r.PluginId, r.Version = "unsigned-telemetry", "1.0.0" }), service.PackageReportRefusedUnsigned},
		{"missing capability", edit(func(r *agentv1pb.PackageReport) { r.PluginId, r.Version = "lite-telemetry", "1.0.0" }), service.PackageReportRefusedMissingCapability},
		{"description", edit(func(r *agentv1pb.PackageReport) {
			r.PayloadJson = []byte(strings.Replace(systemdPayload, `"main_pid"`, `"Description"`, 1))
		}), service.PackageReportRefusedBadPayload},
		{"exec start", edit(func(r *agentv1pb.PackageReport) {
			r.PayloadJson = []byte(strings.Replace(systemdPayload, `"main_pid"`, `"ExecStart"`, 1))
		}), service.PackageReportRefusedBadPayload},
		{"too many units", edit(func(r *agentv1pb.PackageReport) {
			units := make([]string, systemdreport.MaxUnits+1)
			for index := range units {
				units[index] = `{"name": "u` + strconv.Itoa(index) + `.service", "active_state": "active", "sub_state": "running"}`
			}
			r.PayloadJson = []byte(`{"supported": true, "window_seconds": 600, "units": [` + strings.Join(units, ",") + `]}`)
		}), service.PackageReportRefusedBadPayload},
		{"long name", edit(func(r *agentv1pb.PackageReport) {
			r.PayloadJson = []byte(`{"supported": true, "window_seconds": 600, "units": [{"name": "` + strings.Repeat("a", systemdreport.MaxNameLength) + `.service", "active_state": "active", "sub_state": "running"}]}`)
		}), service.PackageReportRefusedBadPayload},
		{"not JSON", edit(func(r *agentv1pb.PackageReport) { r.PayloadJson = []byte("{") }), service.PackageReportRefusedBadPayload},
	}
	for index, testCase := range cases {
		before := readPackageReportCounts()
		require.NoError(t, stream.Send(packageReportMessage("report-"+testCase.name, nodeID, testCase.report)))
		expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-"+strconv.Itoa(index))
		after := readPackageReportCounts()
		assert.Equal(t, uint64(1), after.reasons[testCase.reason]-before.reasons[testCase.reason], testCase.name)
		assert.Equal(t, uint64(1), after.results[packageReportRefused]-before.results[packageReportRefused], testCase.name)
		assert.Zero(t, after.results[packageReportAccepted]-before.results[packageReportAccepted], testCase.name)
	}
	assert.Empty(t, storedPackageReports(t))
	require.NoError(t, stream.CloseSend())
}

// A report the kernel cannot record now is counted as unrecorded and
// dropped; the stream stays open and the next report is stored.
func TestAgentControlPackageReportUnrecorded(t *testing.T) {
	environment := newPackageReportsTestEnvironment(t)
	nodeID := uint32(environment.node.ID)
	stream, helloAck := openReportsSession(t, environment, packageReportCapabilities())
	require.NoError(t, database.GetDB().Exec("DROP TABLE v4_kernel_package_report_state").Error)
	before := readPackageReportCounts()
	require.NoError(t, stream.Send(packageReportMessage("report-1", nodeID, systemdReport(time.Now()))))
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-1")
	after := readPackageReportCounts()
	assert.Equal(t, uint64(1), after.results[packageReportUnrecorded]-before.results[packageReportUnrecorded])

	requireAutoMigrate(t, &model.PackageReportState{})
	require.NoError(t, stream.Send(packageReportMessage("report-2", nodeID, systemdReport(time.Now()))))
	expectHeartbeatAck(t, stream, nodeID, helloAck.SessionId, "heartbeat-2")
	assert.Len(t, storedPackageReports(t), 1)
	require.NoError(t, stream.CloseSend())
}

func TestHandlePackageReportRequiresThePayload(t *testing.T) {
	server := &AgentControlGRPCServer{}
	connection := &AgentControlConnection{Capabilities: packageReportCapabilities(), ServerCapabilities: packageReportCapabilities()}
	err := server.handlePackageReport(connection, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestWriteAgentPackageReportPrometheus(t *testing.T) {
	agentPackageReportMetrics.refused(service.PackageReportRefusedBadPayload)
	agentPackageReportMetrics.refused("not-a-reason")
	agentPackageReportMetrics.result("not-a-result")
	var body strings.Builder
	WriteAgentPackageReportPrometheus(&body)
	text := body.String()
	assert.Contains(t, text, "# TYPE anixops_agent_package_reports_total counter\n")
	for _, result := range packageReportResults {
		assert.Contains(t, text, `anixops_agent_package_reports_total{result="`+result+`"} `)
	}
	for _, reason := range service.PackageReportRefusalReasons {
		assert.Contains(t, text, `anixops_agent_package_reports_refused_total{reason="`+reason+`"} `)
	}
	assert.NotContains(t, text, "not-a-")
	assert.Equal(t, `"short"`, logValue("short"))
	assert.Equal(t, 128+5, len(logValue(strings.Repeat("a", 300))))
}
