package agente2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRealAgentEndToEnd runs the scenarios in order against one Control and
// one Agent; each records pass, fail or skip in the summary.
func TestRealAgentEndToEnd(t *testing.T) {
	s := newSuite(t)
	s.seedNode(t)
	s.createUser(t, "e2e-user-1@anixops.test")

	currentAgentSettings = agentSettings{GRPCHost: s.proxy.Addr(), SigningPublicKey: s.keys.SigningPublicKey, RealNftables: s.netns}
	s.agent = newAgentNode(t, filepath.Join(s.dir, "agent"), s.nodeID)
	s.agent.proc = newProcess("agent", s.bins.Agent, s.agent.dir, filepath.Join(s.logDir, "agent.log"),
		agentEnv(s.agent.shimDir, s.keys.ServerCAFile), "server", "-c", s.agent.configPath, "--watch=false")
	t.Cleanup(func() { s.agent.proc.Stop(t, 15*time.Second) })

	if !s.run("enroll_with_one_time_credential", s.scenarioEnroll) {
		t.Fatal("the Agent did not enroll; the other scenarios need it")
	}
	s.run("config_snapshot_applied", s.scenarioConfigApplied)
	s.run("users_delta", s.scenarioUsersDelta)
	s.run("node_status_heartbeat_and_runtime_health", s.scenarioNodeStatus)
	s.run("plugin_artifacts_over_mtls", s.scenarioPluginArtifactsOverMTLS)
	s.run("systemd_package_report_and_services_panel", s.scenarioSystemdServicesPanel)
	s.run("link_certificate_issued", s.scenarioLinkCertificate)
	s.run("forward_state_push_and_report", s.scenarioForwardStatePushAndReport)
	s.run("traffic_counted_once", s.scenarioTrafficCountedOnce)
	s.run("alive_list", s.scenarioAliveList)
	s.run("agent_diagnostic_on_stream", s.scenarioDiagnostic)
	s.run("forward_diagnose_node_checks", s.scenarioForwardDiagnose)
	s.run("plugin_operation_after_stream_operations", s.scenarioPluginOperationAfterStreamOperations)
	s.run("maintenance_events", s.scenarioMaintenanceEvents)
	s.run("chaos_agent_graceful_restart", s.scenarioAgentGracefulRestart)
	s.run("chaos_control_killed_mid_stream", s.scenarioControlKilledMidStream)
	s.run("chaos_partition_spool_and_agent_crash", s.scenarioPartitionSpoolAndAgentCrash)
	s.run("chaos_control_db_generation_reset", s.scenarioGenerationRecovery)
	s.run("chaos_certificate_revoked", s.scenarioCertificateRevoked)
	s.run("clock_skew_within_bounds", s.scenarioClockSkewBounds)
	s.run("proxy_inbound_after_reloads", s.scenarioProxyInboundAfterReloads)
}

func (s *suite) scenarioEnroll(t *testing.T) string {
	s.agent.WriteCredential(t, s.enrollmentCredential(t))
	s.startAgent(t)
	session := s.connectedSession(t, 60*time.Second, "")
	require.Equal(t, "mtls", strings.ToLower(session.Authentication), "session %+v", session)
	require.NotNil(t, session.Certificate)
	require.Equal(t, fmt.Sprintf("spiffe://anixops/default/agent/proxy-%d", s.nodeID), session.Certificate.SPIFFEID)
	eventually(t, 10*time.Second, func() (bool, string) {
		return s.count(t, "SELECT COUNT(*) FROM v4_kernel_agent_enrollment WHERE node_kind = 'proxy' AND node_id = ? AND method = 'enrollment_credential' AND used_at IS NOT NULL", s.nodeID) == 1, "enrollment not used"
	})
	require.Equal(t, int64(1), s.count(t, "SELECT COUNT(*) FROM v4_kernel_agent_certificate WHERE node_kind = 'proxy' AND node_id = ? AND revoked_at IS NULL", s.nodeID))
	require.NoFileExists(t, s.agent.credentialPath, "the Agent removes the one-time credential")
	return fmt.Sprintf("session %s, certificate %s, capabilities %v", session.SessionID, session.Certificate.Serial, session.NegotiatedCapabilities)
}

func (s *suite) scenarioConfigApplied(t *testing.T) string {
	var status struct {
		Verdict         string
		AppliedRevision uint64
		ReportedError   string
	}
	eventually(t, 60*time.Second, func() (bool, string) {
		require.NoError(t, s.control.db.Raw("SELECT verdict, applied_revision, reported_error FROM v4_kernel_node_config_status WHERE node_kind = 'proxy' AND node_id = ?", s.nodeID).Scan(&status).Error)
		return status.Verdict == "applied" && status.AppliedRevision > 0, fmt.Sprintf("%+v", status)
	}, s.agent.proc)
	// The VLESS inbound runs: a user's transfer goes through it.
	eventually(t, 30*time.Second, func() (bool, string) {
		err := vlessTransfer(fmt.Sprintf("127.0.0.1:%d", s.protocolPt), s.users[0].UUID, s.sink.Port(), 1024, 1024)
		return err == nil, fmt.Sprint(err)
	}, s.agent.proc)
	return fmt.Sprintf("verdict %s at revision %d", status.Verdict, status.AppliedRevision)
}

func (s *suite) scenarioUsersDelta(t *testing.T) string {
	user := s.createUser(t, "e2e-user-2@anixops.test")
	// The new user arrives as a delta on the open stream: its UUID is
	// accepted by the inbound without a restart.
	eventually(t, 30*time.Second, func() (bool, string) {
		err := vlessTransfer(fmt.Sprintf("127.0.0.1:%d", s.protocolPt), user.UUID, s.sink.Port(), 512, 512)
		return err == nil, fmt.Sprint(err)
	}, s.agent.proc)
	return "user " + user.UUID + " served after its delta"
}

func (s *suite) scenarioNodeStatus(t *testing.T) string {
	var node struct {
		LastCheckAt      *int64
		RuntimeHealthy   bool
		RuntimeCheckedAt *int64
		MemoryUsage      float64
		Uptime           int64
	}
	eventually(t, 45*time.Second, func() (bool, string) {
		require.NoError(t, s.control.db.Raw("SELECT last_check_at, runtime_healthy, runtime_checked_at, memory_usage, uptime FROM v2_node WHERE id = ?", s.nodeID).Scan(&node).Error)
		fresh := node.LastCheckAt != nil && time.Now().Unix()-*node.LastCheckAt < 60
		return fresh && node.RuntimeCheckedAt != nil && node.RuntimeHealthy && node.MemoryUsage > 0, fmt.Sprintf("%+v", node)
	}, s.agent.proc)
	return fmt.Sprintf("last_check_at %d, runtime healthy, memory %.1f%%", *node.LastCheckAt, node.MemoryUsage)
}

// Traffic is reported every push interval (60 s, Control's base_config):
// each traffic scenario waits for one report.
const reportWait = 100 * time.Second

func (s *suite) scenarioTrafficCountedOnce(t *testing.T) string {
	user := s.users[0]
	before := s.userTraffic(t, user)
	batches := s.trafficBatches(t)
	const up, down = 1 << 20, 2 << 20
	s.pushTraffic(t, user, up, down)
	eventually(t, reportWait, func() (bool, string) {
		delta := s.userTraffic(t, user) - before
		return delta >= up+down, fmt.Sprintf("counted %d of %d", delta, up+down)
	}, s.agent.proc)
	// Settle one more report interval: nothing is counted twice.
	time.Sleep(5 * time.Second)
	delta := s.userTraffic(t, user) - before
	requireCountedOnce(t, delta, up+down)
	return fmt.Sprintf("counted %d bytes for %d transferred, %d new batches", delta, up+down, s.trafficBatches(t)-batches)
}

// requireCountedOnce: the counted bytes cover the transfer (plus the
// earlier probes and protocol overhead) but are far from twice it.
func requireCountedOnce(t *testing.T, counted, transferred int64) {
	t.Helper()
	require.GreaterOrEqual(t, counted, transferred)
	require.Less(t, counted, transferred+transferred/2, "counted %d for %d transferred: counted twice", counted, transferred)
}
