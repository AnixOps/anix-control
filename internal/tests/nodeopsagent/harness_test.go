// Package nodeopsagent tests the node configuration and agent operations
// of KernelNodeOps (docs/architecture/node-ops-service.md sections 3.3,
// 3.11 and 5.5, NO-6) against the real Agent Control listener and the
// scripted agent of internal/tests/fakeagent: node.sync with
// v4_kernel_node_desired_config, agent.operation, agent.diagnostic and
// the agent session RPCs. It runs on SQLite and, with
// ANIX_TEST_POSTGRES_DSN set, on PostgreSQL. Every secret is fake.
package nodeopsagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const postgresDSNEnvironment = "ANIX_TEST_POSTGRES_DSN"

// Fake credentials: a proxy node's API key, a forward node's token, and a
// Reality private key in the proxy node's protocol. No answer may carry
// them.
const (
	proxyKey       = "fake-proxy-api-key-0123456789"
	forwardToken   = "fake-forward-token-0123456789"
	realityKey     = "fake-reality-private-key-0123456789"
	awaitTimeout   = 10 * time.Second
	settleInterval = 20 * time.Millisecond
	protocolHost   = "node.example.test"
)

// fixture is one backend with the kernel database, the Control listener,
// a NodeOps engine serving the NO-6 kinds, and two nodes of the same id:
// proxy node and forward node (their ids overlap by design).
type fixture struct {
	t          *testing.T
	db         *gorm.DB
	control    *fakeagent.Control
	engine     *kernelnodeops.Engine
	registry   *kernelnodeops.Registry
	websockets *fakeWebSockets
	proxy      model.Node
	forward    model.ForwardNode
	protocol   model.NodeProtocol
	timeout    time.Duration
	started    bool
}

// forEachBackend runs body on SQLite and, with ANIX_TEST_POSTGRES_DSN, on
// a throwaway PostgreSQL schema.
func forEachBackend(t *testing.T, body func(t *testing.T, f *fixture)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "control.db")
		body(t, newFixture(t, &config.DatabaseConfig{Driver: "sqlite", Database: path, LogLevel: "silent"}))
	})
	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv(postgresDSNEnvironment))
		if base == "" {
			t.Skip(postgresDSNEnvironment + " is not set")
		}
		admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		adminDB, err := admin.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = adminDB.Close() })
		var databaseName string
		require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
		if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
			t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
		}
		suffix := make([]byte, 4)
		_, err = rand.Read(suffix)
		require.NoError(t, err)
		schema := "nodeopsagent_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		body(t, newFixture(t, &config.DatabaseConfig{Driver: "postgres", DSN: base + " search_path=" + schema, LogLevel: "silent"}))
	})
}

func newFixture(t *testing.T, cfg *config.DatabaseConfig) *fixture {
	t.Helper()
	cache.InitMemory()
	database.Reset()
	require.NoError(t, database.Init(cfg))
	t.Cleanup(func() {
		_ = database.Close()
		database.Reset()
	})
	db := database.Get()
	require.NoError(t, db.AutoMigrate(
		&model.Node{}, &model.NodeProtocol{}, &model.AuthorizedKey{}, &model.ForwardNode{}, &model.ForwardRule{}, &model.ForwardTunnel{},
		&model.AgentDiagnosticTask{}, &model.OperationLog{},
	))
	require.NoError(t, service.EnsureKernelSchema(db))

	f := &fixture{t: t, db: db, websockets: newFakeWebSockets(), timeout: 20 * time.Second}
	// The Agent Control managers are the process's: a node id of its own
	// keeps this fixture's revisions and retained operations apart from
	// the other tests' nodes.
	var idBytes [2]byte
	_, err := rand.Read(idBytes[:])
	require.NoError(t, err)
	nodeID := uint(1000) + uint(idBytes[0])<<8 + uint(idBytes[1])
	f.proxy = model.Node{ID: nodeID, Name: "proxy", Host: "198.51.100.1", Port: 443, APIKey: proxyKey, APIKeyHash: fakeagent.APIKeyHash(proxyKey), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&f.proxy).Error)
	settings, reality := `{"flow":"xtls-rprx-vision"}`, `{"private_key":"`+realityKey+`","short_id":"abcd"}`
	host := protocolHost
	f.protocol = model.NodeProtocol{NodeID: f.proxy.ID, Name: "vless", Type: model.ProtocolVLESS, Port: 443, Enable: 1, TLS: 2, Host: &host, Settings: &settings, RealitySettings: &reality}
	require.NoError(t, db.Create(&f.protocol).Error)
	f.forward = model.ForwardNode{ID: f.proxy.ID, Name: "relay", Host: "198.51.100.2", Port: 22, APIPort: 8443, APIToken: forwardToken, Enabled: true}
	require.NoError(t, db.Create(&f.forward).Error)
	require.NoError(t, db.Create(&model.ForwardRule{Name: "rule", RelayNodeID: f.forward.ID, ListenPort: 2000, Protocol: "tcp", ExitNodeID: f.forward.ID, TargetHost: "203.0.113.3", TargetPort: 80, Enabled: true}).Error)

	f.control = fakeagent.StartControl(t, db)
	f.registry = kernelnodeops.NewRegistry()
	require.NoError(t, kernelnodeops.RegisterNodeOperationExecutors(f.registry, func() kernelnodeops.AgentSources {
		return kernelnodeops.AgentSources{Streams: f.control.Streams, WebSockets: f.websockets}
	}))
	return f
}

// start runs the engine with the fixture's operation timeout.
func (f *fixture) start() {
	f.t.Helper()
	if f.started {
		return
	}
	f.started = true
	f.engine = &kernelnodeops.Engine{DB: f.db, Executors: f.registry, PollInterval: settleInterval, Timeout: f.timeout}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		f.engine.Run(ctx)
		close(done)
	}()
	f.t.Cleanup(func() {
		cancel()
		<-done
	})
}

func (f *fixture) proxyNode() agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(f.proxy.ID)} // #nosec G115 -- a test id.
}

func (f *fixture) forwardNode() agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(f.forward.ID)} // #nosec G115 -- a test id.
}

// proxyAgent connects a scripted agent for the proxy node by API key.
func (f *fixture) proxyAgent(script fakeagent.Script) *fakeagent.Agent {
	f.t.Helper()
	return fakeagent.ConnectWithKey(f.t, f.control.Dial(f.t, nil), f.proxyNode().ID, proxyKey, script)
}

// forwardAgent enrolls the forward node with its token and connects its
// agent by certificate.
func (f *fixture) forwardAgent(script fakeagent.Script) *fakeagent.Agent {
	f.t.Helper()
	certificate := f.control.Enroll(f.t, f.forwardNode(), forwardToken)
	return fakeagent.ConnectWithCertificate(f.t, f.control.Dial(f.t, certificate), f.forwardNode(), script)
}

// grants allows every NodeOps family; "none" allows nothing.
type grants struct{ families map[string]bool }

func allFamilies() grants {
	g := grants{families: map[string]bool{}}
	for _, capability := range service.NodeOpsCapabilities() {
		g.families[capability] = true
	}
	return g
}

func (g grants) AuthorizeCapability(_ context.Context, _ packagebridge.HostIdentity, capability string) error {
	if g.families[capability] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

var protocolHost2 = packagebridge.HostIdentity{PackageID: "protocol-runtime", Version: "4.1.0", Generation: 2}

// client returns an in-process KernelNodeOps server for the
// protocol-runtime package, with the fixture's engine running.
func (f *fixture) client() kernelnodeopsv1.KernelNodeOpsServer {
	f.t.Helper()
	f.start()
	return f.clientWithout(allFamilies())
}

func (f *fixture) clientWithout(authorizer kernelnodeops.Authorizer) kernelnodeopsv1.KernelNodeOpsServer {
	f.t.Helper()
	f.start()
	sources := &kernelnodeops.AgentSources{Streams: f.control.Streams, WebSockets: f.websockets}
	return (&kernelnodeops.Server{Engine: f.engine, Authorizer: authorizer, Agents: sources}).For(protocolHost2)
}

func syncSpec(node agentcontrol.AgentNode, force bool) *kernelnodeopsv1.OperationSpec {
	kind := kernelnodeopsv1.NodeKind_NODE_KIND_PROXY
	if node.Kind == agentcontrol.NodeKindForward {
		kind = kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD
	}
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_SyncNode{SyncNode: &kernelnodeopsv1.SyncNode{
		Node: &kernelnodeopsv1.NodeRef{Kind: kind, Id: uint64(node.ID)}, Force: force,
	}}}
}

func agentOperationSpec(nodeID uint, kind, operationID string, timeoutSeconds uint32) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_AgentControlOperation{AgentControlOperation: &kernelnodeopsv1.AgentControlOperation{
		NodeId: uint64(nodeID), Kind: kind, AgentOperationId: operationID, PayloadJson: []byte(`{"source":"test"}`), TimeoutSeconds: timeoutSeconds,
	}}}
}

func diagnosticSpec(nodeID uint, action string, params string) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RunAgentDiagnostic{RunAgentDiagnostic: &kernelnodeopsv1.RunAgentDiagnostic{
		NodeId: uint64(nodeID), Action: action, ParamsJson: []byte(params), TimeoutSeconds: 30,
	}}}
}

// submit submits spec and returns the answer; wait is the wait mode.
func submit(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, requestID string, spec *kernelnodeopsv1.OperationSpec, wait kernelnodeopsv1.WaitMode) (*kernelnodeopsv1.SubmitOperationResponse, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), awaitTimeout+5*time.Second)
	defer cancel()
	return client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: requestID, Operation: spec, Wait: wait, WaitTimeoutMs: uint32(awaitTimeout / time.Millisecond),
	})
}

// terminal submits with wait TERMINAL and requires the operation ended.
func terminal(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, requestID string, spec *kernelnodeopsv1.OperationSpec) *kernelnodeopsv1.Operation {
	t.Helper()
	response, err := submit(t, client, requestID, spec, kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL)
	require.NoError(t, err)
	operation := response.GetOperation()
	require.True(t, kernelnodeops.Terminal(strings.ToLower(strings.TrimPrefix(operation.GetState().String(), "OPERATION_STATE_"))),
		"operation %s did not end: %s", operation.GetOperationId(), operation.GetState())
	return operation
}

// awaitState polls GetOperation until the operation is in state.
func awaitState(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, operationID string, state kernelnodeopsv1.OperationState) *kernelnodeopsv1.Operation {
	t.Helper()
	deadline := time.Now().Add(awaitTimeout)
	var last *kernelnodeopsv1.Operation
	for time.Now().Before(deadline) {
		response, err := client.GetOperation(context.Background(), &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_OperationId{OperationId: operationID}})
		require.NoError(t, err)
		last = response.GetOperation()
		if last.GetState() == state {
			return last
		}
		time.Sleep(settleInterval)
	}
	require.Failf(t, "state not reached", "operation %s is %s, wanted %s: %v", operationID, last.GetState(), state, last.GetError())
	return nil
}

// desired returns the node's desired configuration row.
func (f *fixture) desired(node agentcontrol.AgentNode) (model.KernelNodeDesiredConfig, bool) {
	f.t.Helper()
	row, found, err := kernelnodeops.LoadDesiredConfig(context.Background(), f.db, node)
	require.NoError(f.t, err)
	return row, found
}

// fakeWebSockets is an in-memory agentstreams.WebSockets: the legacy
// WebSocket agents, with a scripted diagnostic transport per node.
type fakeWebSockets struct {
	mu         sync.Mutex
	sessions   []agentstreams.Session
	monitors   map[uint]fakeMonitor
	transports map[uint]*fakeTransport
}

type fakeMonitor struct {
	system     map[string]any
	receivedAt time.Time
}

// fakeTransport answers a dispatch per its script and records the tasks.
type fakeTransport struct {
	mu       sync.Mutex
	tasks    []agentstreams.DiagnosticTask
	dispatch func(task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch
}

func newFakeWebSockets() *fakeWebSockets {
	return &fakeWebSockets{monitors: map[uint]fakeMonitor{}, transports: map[uint]*fakeTransport{}}
}

func (w *fakeWebSockets) Sessions() []agentstreams.Session {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]agentstreams.Session(nil), w.sessions...)
}

func (w *fakeWebSockets) Monitor(nodeID uint) (map[string]any, time.Time, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	monitor, ok := w.monitors[nodeID]
	return monitor.system, monitor.receivedAt, ok
}

func (w *fakeWebSockets) DiagnosticTransport(nodeID uint) (agentstreams.DiagnosticTransport, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	transport, ok := w.transports[nodeID]
	return transport, ok
}

func (w *fakeWebSockets) connect(nodeID uint, dispatch func(task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch) *fakeTransport {
	w.mu.Lock()
	defer w.mu.Unlock()
	transport := &fakeTransport{dispatch: dispatch}
	w.transports[nodeID] = transport
	w.sessions = append(w.sessions, agentstreams.Session{
		Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(nodeID)}, Transport: agentstreams.TransportWebSocket, // #nosec G115 -- a test id.
		AgentVersion: "ws-agent", Capabilities: []string{"diagnostic"}, LastSeen: time.Now(),
		System: map[string]any{"os": "linux", "password": "ws-secret-value-0123456789"}, Identity: agentstreams.IdentityAPIKey,
	})
	return transport
}

func (w *fakeWebSockets) disconnect(nodeID uint) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.transports, nodeID)
	w.sessions = nil
}

func (w *fakeWebSockets) monitor(nodeID uint, system map[string]any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.monitors[nodeID] = fakeMonitor{system: system, receivedAt: time.Now()}
}

func (f *fakeTransport) DispatchDiagnostic(_ context.Context, task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch {
	f.mu.Lock()
	f.tasks = append(f.tasks, task)
	f.mu.Unlock()
	return f.dispatch(task)
}

func (f *fakeTransport) received() []agentstreams.DiagnosticTask {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agentstreams.DiagnosticTask(nil), f.tasks...)
}

// acknowledged is a WebSocket dispatch the agent acknowledged.
func acknowledged(task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch {
	return agentstreams.DiagnosticDispatch{MessageID: "msg-" + task.ID, AckReceived: true, Ack: agentstreams.Ack{Accepted: true, AcceptedAt: time.Now()}}
}

// fallenBack is a WebSocket dispatch whose acknowledgement timed out and
// that went out in the legacy task message.
func fallenBack(task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch {
	return agentstreams.DiagnosticDispatch{MessageID: "msg-" + task.ID, DispatchError: errors.New("ack timeout after 3s"), LegacyFallback: true}
}

// undeliverable is a WebSocket dispatch that failed on both paths.
func undeliverable(task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch {
	return agentstreams.DiagnosticDispatch{MessageID: "msg-" + task.ID, DispatchError: errors.New("ack timeout after 3s"), FallbackError: errors.New("websocket: close sent")}
}
