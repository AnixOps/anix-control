package agente2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The A2-7 suite (node-ops-service.md section 9): the real Control binary
// and the real Agent binary, built from the pinned anix-agent commit,
// against each other under agent_control.mtls: required, with chaos.
//
// It is opt-in: ANIXOPS_AGENT_E2E=1 and an anix-agent checkout
// (ANIXOPS_AGENT_ROOT, or a sibling V2bX_AnixOps / anix-agent directory).
// ANIXOPS_AGENT_GO is the go command for the Agent's module (its
// toolchain line), ANIXOPS_AGENT_E2E_POSTGRES_DSN runs Control on
// PostgreSQL (SQLite otherwise), ANIXOPS_AGENT_E2E_LOGDIR keeps every log
// and the scenario summary, and ANIXOPS_AGENT_E2E_NETNS=1 is the privileged
// lane: the whole run inside a throwaway network namespace (the test
// refuses the host's), with the real nftables forward driver.

func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("ANIXOPS_AGENT_E2E") != "1" {
		t.Skip("set ANIXOPS_AGENT_E2E=1 (and ANIXOPS_AGENT_ROOT) to run the cross-repository real Agent suite")
	}
}

type suite struct {
	t       *testing.T
	dir     string
	logDir  string
	keys    testPKI
	bins    binaries
	pkgs    map[string]signedPackage
	control *control
	proxy   *tcpProxy
	agent   *agentNode
	sink    *trafficSink
	netns   bool

	nodeID  uint
	routeID string
	// pluginReadyAt is when machine-telemetry ran on the node.
	pluginReadyAt time.Time
	protocolPt    int
	users         []testUser

	mu         sync.Mutex
	results    []scenarioResult
	skipReason string
}

// skip records why a scenario skips (for the summary) and skips it.
func (s *suite) skip(t *testing.T, format string, args ...any) {
	t.Helper()
	s.mu.Lock()
	s.skipReason = fmt.Sprintf(format, args...)
	s.mu.Unlock()
	t.Skip(s.skipReason)
}

func (s *suite) takeSkipReason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	reason := s.skipReason
	s.skipReason = ""
	if reason == "" {
		reason = "see the test log"
	}
	return reason
}

type testUser struct {
	ID   uint
	UUID string
}

type scenarioResult struct {
	Name     string  `json:"name"`
	Result   string  `json:"result"`
	Seconds  float64 `json:"seconds"`
	Detail   string  `json:"detail,omitempty"`
	Evidence string  `json:"evidence,omitempty"`
}

func newSuite(t *testing.T) *suite {
	t.Helper()
	requireE2E(t)
	started := time.Now()
	s := &suite{t: t, netns: os.Getenv("ANIXOPS_AGENT_E2E_NETNS") == "1"}
	if s.netns {
		requireThrowawayNetns(t)
	}
	s.dir = t.TempDir()
	if keep := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_E2E_WORKDIR")); keep != "" {
		// Kept after the run, for debugging.
		s.dir = filepath.Join(keep, time.Now().UTC().Format("20060102T150405"))
		require.NoError(t, os.MkdirAll(s.dir, 0o700))
	}
	s.logDir = strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_E2E_LOGDIR"))
	if s.logDir == "" {
		s.logDir = filepath.Join(s.dir, "logs")
	}
	require.NoError(t, os.MkdirAll(s.logDir, 0o755))
	t.Cleanup(s.writeSummary)

	s.keys = newTestPKI(t, filepath.Join(s.dir, "pki"))
	s.bins = buildBinaries(t, filepath.Join(s.dir, "bin"))
	t.Logf("Agent %s at %s (commit %s)", s.bins.AgentVersion, s.bins.AgentRoot, s.bins.AgentCommit)
	s.pkgs = map[string]signedPackage{}
	for _, id := range []string{"identity-platform", "protocol-runtime", "forward", "machine-telemetry"} {
		s.pkgs[id] = buildSignedPackage(t, s.bins, s.keys, filepath.Join(s.dir, "packages"), id)
	}

	postgresDSN := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_E2E_POSTGRES_DSN"))
	if postgresDSN != "" {
		postgresDSN = freshPostgresDatabase(t, postgresDSN)
	}
	c := &control{apiPort: freePort(t), grpcPort: freePort(t)}
	if postgresDSN != "" {
		c.dbDriver = "postgres"
	} else {
		c.dbDriver = "sqlite"
	}
	controlDir := filepath.Join(s.dir, "control")
	require.NoError(t, os.MkdirAll(controlDir, 0o700))
	c.configPath, c.sqlitePath = writeControlConfig(t, controlDir, s.keys, s.pkgs["identity-platform"].Dir, c.apiPort, c.grpcPort, postgresDSN)
	controlEnv := append(os.Environ(), "ANIX_CONTROL_CONFIG="+c.configPath, "TMPDIR="+controlDir)
	c.proc = newProcess("control", s.bins.Control, controlDir, filepath.Join(s.logDir, "control.log"), controlEnv, "-config", c.configPath)
	s.control = c
	c.Start(t)
	t.Cleanup(func() { c.proc.Stop(t, 10*time.Second) })
	c.db = openTestDB(t, postgresDSN, c.sqlitePath)
	c.Login(t)
	for _, id := range []string{"protocol-runtime", "forward", "machine-telemetry"} {
		c.InstallPackage(t, s.pkgs[id])
	}
	s.sink = newTrafficSink(t)
	s.proxy = newTCPProxy(t, c.GRPCAddr())
	t.Logf("environment ready in %s (database %s, netns %t)", time.Since(started).Round(time.Second), c.dbDriver, s.netns)
	return s
}

// requireThrowawayNetns refuses to run the privileged lane in the host's
// network namespace: the real nftables driver writes rules.
func requireThrowawayNetns(t *testing.T) {
	t.Helper()
	self, err := os.Readlink("/proc/self/ns/net")
	require.NoError(t, err)
	host, err := os.Readlink("/proc/1/ns/net")
	if err != nil {
		t.Fatalf("cannot read the host network namespace (%v): run the netns lane as root under unshare --net", err)
	}
	if self == host {
		t.Fatalf("ANIXOPS_AGENT_E2E_NETNS=1 needs a throwaway network namespace (unshare --net); refusing to touch the host's (%s)", self)
	}
}

// run runs one scenario and records its outcome for the summary.
func (s *suite) run(name string, scenario func(t *testing.T) string) bool {
	started := time.Now()
	evidence := ""
	ran := false
	var knownIssue string
	ok := s.t.Run(name, func(t *testing.T) {
		ran = true
		t.Cleanup(func() {
			if t.Skipped() {
				knownIssue = "skipped"
			}
		})
		evidence = scenario(t)
	})
	if !ran {
		return ok
	}
	result := "pass"
	detail := ""
	switch {
	case !ok:
		result = "fail"
	case evidence == skipped:
		result = "skip"
		evidence = ""
	}
	if knownIssue != "" {
		result, detail = "skip", s.takeSkipReason()
	}
	if strings.HasPrefix(evidence, skipped+":") {
		result, detail, evidence = "skip", strings.TrimPrefix(evidence, skipped+":"), ""
	}
	s.mu.Lock()
	s.results = append(s.results, scenarioResult{Name: name, Result: result, Seconds: time.Since(started).Seconds(), Detail: detail, Evidence: evidence})
	s.mu.Unlock()
	return ok
}

const skipped = "skipped"

func (s *suite) writeSummary() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		return
	}
	encoded, _ := json.MarshalIndent(map[string]any{
		"agent_commit": s.bins.AgentCommit, "database": s.control.dbDriver, "netns": s.netns, "scenarios": s.results,
	}, "", "  ")
	_ = os.WriteFile(filepath.Join(s.logDir, fmt.Sprintf("summary-%s.json", map[bool]string{true: "netns", false: "main"}[s.netns])), encoded, 0o644)
	var lines []string
	for _, r := range s.results {
		line := fmt.Sprintf("%-4s %6.1fs  %s", r.Result, r.Seconds, r.Name)
		if r.Detail != "" {
			line += "  (" + r.Detail + ")"
		}
		lines = append(lines, line)
	}
	s.t.Logf("scenario summary:\n%s", strings.Join(lines, "\n"))
}

// seedNode creates the proxy node and its VLESS protocol the way the
// admin routes store them; Control builds the node's configuration from
// these rows when the Agent says Hello.
func (s *suite) seedNode(t *testing.T) {
	t.Helper()
	db := s.control.db
	node := model.Node{
		Name: "agent-e2e-proxy", Host: "127.0.0.1", Port: 443, Status: model.NodeStatusOnline,
		APIKey: "unused-" + uuid.NewString()[:16], Rate: 1, TrafficRate: 1,
	}
	require.NoError(t, db.Create(&node).Error)
	s.nodeID = node.ID
	s.protocolPt = freePort(t)
	settings := `{"flow":""}`
	transport := "tcp"
	protocol := model.NodeProtocol{
		NodeID: node.ID, Name: "vless-e2e", Type: model.ProtocolVLESS, Port: s.protocolPt, Enable: 1, Show: 1,
		Settings: &settings, Transport: &transport,
	}
	require.NoError(t, db.Create(&protocol).Error)
}

// createUser creates a user through the identity-platform admin route
// (which records the subscriber change the users delta follows).
func (s *suite) createUser(t *testing.T, email string) testUser {
	t.Helper()
	transfer := int64(50 << 30)
	s.control.API(t, http.MethodPost, "/api/v2/admin/users", map[string]any{
		"email": email, "password": "agent-e2e-user", "transfer_enable": transfer,
	}, nil)
	var row struct {
		ID   uint
		UUID string
	}
	require.NoError(t, s.control.db.Raw("SELECT id, uuid FROM v2_user WHERE email = ?", email).Scan(&row).Error)
	require.NotZero(t, row.ID)
	require.NotEmpty(t, row.UUID)
	user := testUser{ID: row.ID, UUID: row.UUID}
	s.users = append(s.users, user)
	return user
}

// enrollmentCredential issues a one-time anixagt_ credential for the node.
func (s *suite) enrollmentCredential(t *testing.T) string {
	t.Helper()
	var answer struct {
		Data struct {
			Credential string `json:"credential"`
		} `json:"data"`
	}
	s.control.API(t, http.MethodPost, "/api/v4/kernel/agents/enrollment-tokens", map[string]any{
		"node": fmt.Sprintf("proxy-%d", s.nodeID), "ttl_seconds": 3600,
	}, &answer)
	require.True(t, strings.HasPrefix(answer.Data.Credential, "anixagt_"), "credential %q", answer.Data.Credential)
	return answer.Data.Credential
}

func (s *suite) startAgent(t *testing.T) {
	t.Helper()
	s.agent.proc.Start(t)
}

// userTraffic is u+d of the user as Control counted it.
func (s *suite) userTraffic(t *testing.T, user testUser) int64 {
	t.Helper()
	var total int64
	require.NoError(t, s.control.db.Raw("SELECT u + d FROM v2_user WHERE id = ?", user.ID).Scan(&total).Error)
	return total
}

func (s *suite) count(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.control.db.Raw(query, args...).Scan(&n).Error)
	return n
}

func (s *suite) trafficBatches(t *testing.T) int64 {
	return s.count(t, "SELECT COUNT(*) FROM v4_kernel_agent_report_batch WHERE node_kind = 'proxy' AND node_id = ? AND kind = 'traffic'", s.nodeID)
}

// session is the node's row of the transport inventory.
type inventorySession struct {
	SessionID      string `json:"session_id"`
	Authentication string `json:"authentication"`
	Identity       string `json:"identity"`
	Certificate    *struct {
		Serial   string `json:"serial"`
		SPIFFEID string `json:"spiffe_id"`
	} `json:"certificate"`
	AgentVersion           string             `json:"agent_version"`
	NegotiatedCapabilities []string           `json:"negotiated_capabilities"`
	AgentMetrics           map[string]float64 `json:"agent_metrics"`
}

type inventoryNode struct {
	Node      string            `json:"node"`
	Transport string            `json:"transport"`
	Session   *inventorySession `json:"session"`
}

func (s *suite) inventory(t *testing.T) *inventoryNode {
	t.Helper()
	var answer struct {
		Data struct {
			Nodes []inventoryNode `json:"nodes"`
		} `json:"data"`
	}
	status, body := s.control.TryAPI(t, http.MethodGet, "/api/v4/kernel/agents/transports", nil)
	if status != http.StatusOK {
		return nil
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil
	}
	want := fmt.Sprintf("proxy-%d", s.nodeID)
	for i := range answer.Data.Nodes {
		if answer.Data.Nodes[i].Node == want {
			return &answer.Data.Nodes[i]
		}
	}
	return nil
}

// connectedSession waits for a live stream session of the node and
// answers it.
func (s *suite) connectedSession(t *testing.T, timeout time.Duration, differentFrom string) *inventorySession {
	t.Helper()
	var session *inventorySession
	eventually(t, timeout, func() (bool, string) {
		node := s.inventory(t)
		if node == nil || node.Session == nil {
			return false, "no session"
		}
		if node.Session.SessionID == differentFrom {
			return false, "still the old session " + differentFrom
		}
		session = node.Session
		return true, ""
	}, s.control.proc, s.agent.proc)
	return session
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
