package agente2e

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// scenarioLinkCertificate: once forward.v1 is negotiated the Agent asks
// Control's link CA for its forward link certificate (H28) over the agent
// certificate's mTLS and installs it in gost's directory.
func (s *suite) scenarioLinkCertificate(t *testing.T) string {
	var row struct {
		Serial   string
		DNSName  string
		SPIFFEID string
	}
	eventually(t, 60*time.Second, func() (bool, string) {
		require.NoError(t, s.control.db.Raw("SELECT serial, dns_name, spiffe_id FROM v4_kernel_forward_link_certificate WHERE node_kind = 'proxy' AND node_id = ? AND revoked_at IS NULL ORDER BY not_after DESC LIMIT 1", s.nodeID).Scan(&row).Error)
		return row.Serial != "", "no link certificate issued"
	}, s.agent.proc, s.control.proc)
	for _, name := range []string{"link.crt", "link.key", "link-ca.crt"} {
		require.FileExists(t, filepath.Join(s.agent.gostDir, "tls", name))
	}
	return fmt.Sprintf("link certificate %s for %s (%s)", row.Serial, row.DNSName, row.SPIFFEID)
}

type forwardReport struct {
	Generation uint64
	StateHash  string
	Applied    bool
	HopErrors  int
	ReportJSON string
	ObservedAt time.Time
	ReceivedAt time.Time
}

func (s *suite) forwardNodeRef() string { return fmt.Sprintf("proxy-%d", s.nodeID) }

func (s *suite) forwardReport(t *testing.T) forwardReport {
	t.Helper()
	var report forwardReport
	require.NoError(t, s.control.db.Raw("SELECT generation, state_hash, applied, hop_errors, report_json, observed_at, received_at FROM v4_kernel_forward_node_report WHERE node_ref = ?", s.forwardNodeRef()).Scan(&report).Error)
	return report
}

func (s *suite) forwardStateGeneration(t *testing.T) (uint64, string) {
	t.Helper()
	var state struct {
		Generation uint64
		StateHash  string
	}
	require.NoError(t, s.control.db.Raw("SELECT generation, state_hash FROM v4_kernel_forward_node_state WHERE node_ref = ?", s.forwardNodeRef()).Scan(&state).Error)
	return state.Generation, state.StateHash
}

// forwardEngine is the engine the suite's route uses: the real nftables
// driver in the netns lane, gost (behind the systemctl and gost stand-ins)
// otherwise.
func (s *suite) forwardEngine() string {
	if s.netns {
		return "ENGINE_NFTABLES"
	}
	return "ENGINE_GOST"
}

// scenarioForwardStatePushAndReport creates a one-hop route on the node:
// Control plans it, pushes the node's forwarding state (config.v1 in the
// node configuration v2 format, forward.v1) and the Agent applies it and
// reports (forward.report) the generation it holds.
func (s *suite) scenarioForwardStatePushAndReport(t *testing.T) string {
	previous := s.forwardReport(t)
	listenPort := freePortInRange(t, 30000, 39999)
	// Control never forwards to loopback: the target is a sink on another
	// local address.
	target := s.forwardTarget(t)
	route := map[string]any{
		"name":   "agent-e2e-route",
		"listen": map[string]any{"port": listenPort, "protocol": "L4_PROTOCOL_TCP"},
		"hops": []any{map[string]any{
			"role": "HOP_ROLE_ENTRY", "engine": s.forwardEngine(), "node_refs": []string{s.forwardNodeRef()},
		}},
		"targets": []any{map[string]any{"host": target.ip, "port": target.sink.Port()}},
		"policy":  map[string]any{"target": "BALANCE_STRATEGY_FAILOVER", "target_policy": "TARGET_POLICY_ALLOW_PRIVATE"},
	}
	var created map[string]any
	s.control.API(t, http.MethodPost, "/api/v4/forward/routes", route, &created)
	routeID := findString(created, "id")
	require.NotEmpty(t, routeID, "route id in %v", created)
	s.routeID = routeID

	var generation uint64
	eventually(t, 90*time.Second, func() (bool, string) {
		gen, hash := s.forwardStateGeneration(t)
		report := s.forwardReport(t)
		generation = gen
		return gen > 0 && report.Generation == gen && report.StateHash == hash && report.Generation > previous.Generation,
			fmt.Sprintf("state generation %d hash %.12s, report generation %d hash %.12s applied %t hop errors %d", gen, hash, report.Generation, report.StateHash, report.Applied, report.HopErrors)
	}, s.agent.proc, s.control.proc)
	report := s.forwardReport(t)
	require.Contains(t, report.ReportJSON, s.routeID, "the report covers the route's hop")
	if s.netns {
		require.True(t, report.Applied, "the nftables hop applied: %s", prettyJSON(report.ReportJSON))
		require.Zero(t, report.HopErrors)
		require.Contains(t, nftTables(t), "inet anixops_fwd", "the driver's table is in the namespace")
		ruleset, err := exec.Command("nft", "list", "table", "inet", "anixops_fwd").CombinedOutput()
		require.NoError(t, err, "%s", ruleset)
		require.Contains(t, string(ruleset), fmt.Sprint(listenPort), "the hop's rules carry the listen port")
	}
	evidence := fmt.Sprintf("route %s on %s: generation %d pushed and reported (applied %t, hop errors %d)", routeID, s.forwardEngine(), generation, report.Applied, report.HopErrors)
	if !s.netns && RealGost() {
		// gost really forwards: a transfer through the route's listener
		// reaches the target.
		require.True(t, report.Applied, "the gost hop applied: %s", prettyJSON(report.ReportJSON))
		eventually(t, 30*time.Second, func() (bool, string) {
			err := sinkTransfer(fmt.Sprintf("127.0.0.1:%d", listenPort), 4096, 8192)
			return err == nil, fmt.Sprint(err)
		}, s.agent.proc)
		evidence += "; traffic forwarded through gost"
	}
	return evidence
}

type forwardTarget struct {
	ip   string
	sink *trafficSink
}

// forwardTarget answers a sink on a local address other than loopback: a
// dummy interface in the netns lane, the host's first private or global
// IPv4 address otherwise.
func (s *suite) forwardTarget(t *testing.T) forwardTarget {
	t.Helper()
	ip := ""
	if s.netns {
		ip = "10.203.0.1"
		for _, args := range [][]string{
			{"link", "add", "ae2e0", "type", "dummy"},
			{"addr", "add", ip + "/24", "dev", "ae2e0"},
			{"link", "set", "ae2e0", "up"},
		} {
			output, err := exec.Command("ip", args...).CombinedOutput()
			if err != nil && !strings.Contains(string(output), "File exists") {
				require.NoError(t, err, "ip %v: %s", args, output)
			}
		}
	} else {
		addresses, err := net.InterfaceAddrs()
		require.NoError(t, err)
		for _, address := range addresses {
			prefix, ok := address.(*net.IPNet)
			if ok && prefix.IP.To4() != nil && !prefix.IP.IsLoopback() && !prefix.IP.IsLinkLocalUnicast() {
				ip = prefix.IP.String()
				break
			}
		}
		if ip == "" {
			t.Skip("no non-loopback IPv4 address for the forward target")
		}
	}
	// The sink outlives the scenario: the diagnosis dials it later.
	return forwardTarget{ip: ip, sink: newTrafficSinkOn(s.t, ip)}
}

// scenarioForwardDiagnose runs Control's route diagnosis with the node
// vantage: agent.diagnostic forward checks (forward.listen,
// forward.port_conflict, forward.connect, forward.udp_probe) on the stream.
func (s *suite) scenarioForwardDiagnose(t *testing.T) string {
	if s.routeID == "" {
		return skipped + ":no route (forward state scenario failed)"
	}
	var answer struct {
		Data struct {
			Nodes []struct {
				NodeRef     string `json:"node_ref"`
				Connected   bool   `json:"connected"`
				NodeVantage bool   `json:"node_vantage"`
			} `json:"nodes"`
			Steps []struct {
				Kind    string `json:"kind"`
				Vantage string `json:"vantage"`
				Result  struct {
					OK      bool   `json:"ok"`
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"result"`
			} `json:"steps"`
		} `json:"data"`
	}
	s.control.API(t, http.MethodPost, "/api/v4/forward/routes/"+s.routeID+"/diagnose", map[string]any{}, &answer)
	require.Len(t, answer.Data.Nodes, 1)
	require.True(t, answer.Data.Nodes[0].NodeVantage, "the node's Agent runs the checks (diag.v1)")
	var steps []string
	nodeSteps := map[string]bool{}
	for _, step := range answer.Data.Steps {
		steps = append(steps, fmt.Sprintf("%s/%s ok=%t %s", strings.TrimPrefix(step.Kind, "PROBE_KIND_"), strings.TrimPrefix(step.Vantage, "DIAGNOSE_VANTAGE_"), step.Result.OK, step.Result.Code))
		if step.Vantage == "DIAGNOSE_VANTAGE_NODE" {
			// Without root (the main CI lane) the Agent cannot read
			// nftables; the port-conflict check reports that instead of
			// a result.
			nodeSteps[step.Kind] = step.Result.OK || (step.Kind == "PROBE_KIND_PORT_CONFLICT" && step.Result.Code == "nft_unavailable")
		}
	}
	for _, kind := range []string{"PROBE_KIND_LISTEN", "PROBE_KIND_PORT_CONFLICT", "PROBE_KIND_DELIVERY"} {
		ok, ran := nodeSteps[kind]
		require.True(t, ran, "%s from the node: %v", kind, steps)
		if !s.netns && RealGost() {
			require.True(t, ok, "%s: %v", kind, steps)
		}
	}
	return strings.Join(steps, "; ")
}

// findString finds the first string value of key in a decoded JSON
// document.
func findString(document any, key string) string {
	switch value := document.(type) {
	case map[string]any:
		if found, ok := value[key].(string); ok && found != "" {
			return found
		}
		for _, child := range value {
			if found := findString(child, key); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range value {
			if found := findString(child, key); found != "" {
				return found
			}
		}
	}
	return ""
}

// scenarioGenerationRecovery is the Control database reset of #179: the
// stored forwarding generation of the node falls behind the one the Agent
// holds (as after restoring an older database), with Control down. When the
// Agent reports its generation, Control moves the node's generation up to
// it, keeps the hops, and later plans stamp above it, so the Agent applies
// them instead of ignoring an older generation.
func (s *suite) scenarioGenerationRecovery(t *testing.T) string {
	held := s.forwardReport(t)
	if held.Generation < 2 || s.routeID == "" {
		return skipped + ":the Agent holds no forwarding generation above 1"
	}
	recoveriesBefore := s.control.Metric(t, "anixops_forward_generation_recoveries_total")
	s.control.proc.Stop(t, 15*time.Second)
	require.NoError(t, s.control.db.Exec("UPDATE v4_kernel_forward_node_state SET generation = 1 WHERE node_ref = ?", s.forwardNodeRef()).Error)
	s.control.Start(t)
	eventually(t, 120*time.Second, func() (bool, string) {
		gen, _ := s.forwardStateGeneration(t)
		return gen >= held.Generation, fmt.Sprintf("Agent holds %d; Control's state is at %d", held.Generation, gen)
	}, s.agent.proc, s.control.proc)
	recovered, _ := s.forwardStateGeneration(t)
	// A new plan (pausing the route) stamps above the Agent's generation
	// and the Agent applies it.
	s.control.Login(t)
	s.control.API(t, http.MethodPost, "/api/v4/forward/routes/"+s.routeID+"/pause", map[string]any{}, nil)
	var applied uint64
	eventually(t, 90*time.Second, func() (bool, string) {
		gen, hash := s.forwardStateGeneration(t)
		report := s.forwardReport(t)
		applied = report.Generation
		return gen > held.Generation && report.Generation == gen && report.StateHash == hash,
			fmt.Sprintf("plan generation %d, Agent reports %d", gen, report.Generation)
	}, s.agent.proc, s.control.proc)
	s.control.API(t, http.MethodPost, "/api/v4/forward/routes/"+s.routeID+"/resume", map[string]any{}, nil)
	return fmt.Sprintf("Agent held %d, the reset left Control at 1; recovered to %d (recoveries %v -> %v); the next plan %d applied by the Agent",
		held.Generation, recovered, recoveriesBefore, s.control.Metric(t, "anixops_forward_generation_recoveries_total"), applied)
}

// nftTables lists the nftables tables of the (throwaway) namespace.
func nftTables(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("nft", "list", "tables").CombinedOutput()
	require.NoError(t, err, "%s", output)
	return string(output)
}

// freePortInRange answers a TCP port of [first, last] that was free a
// moment ago (a node's default forwarding port range).
func freePortInRange(t *testing.T, first, last int) int {
	t.Helper()
	for port := first + int(time.Now().UnixNano()%1000); port <= last; port += 7 {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = listener.Close()
			return port
		}
	}
	t.Fatalf("no free port in %d-%d", first, last)
	return 0
}
