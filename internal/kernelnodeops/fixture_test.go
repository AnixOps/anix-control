package kernelnodeops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSN enables the PostgreSQL runs; each run gets a throwaway
// schema.
const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

var gormConfig = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}

// forEachDatabase runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN is
// set, on PostgreSQL, each with the ledger and the seeded node tables.
func forEachDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		// _txlock=immediate: every transaction takes the write lock when it
		// begins, and waits for it under the busy timeout. With the default
		// deferred BEGIN, a transaction that read and then writes while
		// another connection (the engine's dispatcher, an executor) has
		// committed gets SQLITE_BUSY at once in WAL mode: the busy timeout
		// does not apply to that upgrade.
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_txlock=immediate"), gormConfig)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		seed(t, db)
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := openPostgres(t)
		seed(t, db)
		body(t, db)
	})
}

func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(postgresDSN))
	if base == "" {
		t.Skip(postgresDSN + " is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), gormConfig)
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
	schema := "kernel_nodeops_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), gormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// The seeded nodes: proxy nodes 1 and 2 (protocol 5 on node 1), forward
// nodes 10, 11 and 12, clean agent 20 on forward node 10, tunnel 30 from 10
// to 11 with forwards 40 and 41, tunnel 31 on 12 with no forward, legacy
// rule 50 from 10 to 12, registration key 60.
func seed(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(append(model.KernelNodeOperationModels(),
		&model.Node{}, &model.NodeProtocol{}, &model.ForwardNode{}, &model.ForwardCleanAgent{},
		&model.ForwardTunnel{}, &model.Forward{}, &model.ForwardRule{}, &model.AuthorizedKey{})...))
	exit := uint(11)
	agentNode := uint(10)
	require.NoError(t, db.Create(&[]model.Node{{ID: 1, Name: "proxy-1", APIKey: "node-key-1"}, {ID: 2, Name: "proxy-2", APIKey: "node-key-2"}}).Error)
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 5, NodeID: 1, Name: "vless", Type: model.ProtocolVLESS, Port: 443}).Error)
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 10, Name: "relay", Host: "198.51.100.10", Port: 22}, {ID: 11, Name: "exit", Host: "198.51.100.11", Port: 22},
		{ID: 12, Name: "spare", Host: "198.51.100.12", Port: 22},
	}).Error)
	require.NoError(t, db.Create(&model.ForwardCleanAgent{ID: 20, NodeID: &agentNode, Name: "agent", Token: "clean-agent-token"}).Error)
	require.NoError(t, db.Create(&[]model.ForwardTunnel{
		{ID: 30, Name: "tunnel", InNodeID: 10, OutNodeID: &exit}, {ID: 31, Name: "empty", InNodeID: 12},
	}).Error)
	require.NoError(t, db.Create(&[]model.Forward{
		{ID: 40, UserID: 1, Name: "a", TunnelID: 30, InPort: 1000, RemoteAddr: "203.0.113.1:80"},
		{ID: 41, UserID: 1, Name: "b", TunnelID: 30, InPort: 1001, RemoteAddr: "203.0.113.2:80"},
	}).Error)
	require.NoError(t, db.Create(&model.ForwardRule{ID: 50, Name: "rule", RelayNodeID: 10, ListenPort: 2000, ExitNodeID: 12, TargetHost: "203.0.113.3", TargetPort: 80}).Error)
	require.NoError(t, db.Create(&model.AuthorizedKey{ID: 60, Name: "key", Key: "registration-key"}).Error)
}

// grants authorizes the capabilities it holds; "fenced" refuses every call
// as a fenced generation. It is safe for concurrent use and can change.
type grants struct {
	mu   sync.Mutex
	held map[string]bool
}

func allow(capabilities ...string) *grants {
	g := &grants{held: map[string]bool{}}
	for _, capability := range capabilities {
		g.held[capability] = true
	}
	return g
}

func allFamilies() *grants { return allow(service.NodeOpsCapabilities()...) }

func (g *grants) set(capability string, held bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.held[capability] = held
}

func (g *grants) AuthorizeCapability(_ context.Context, _ packagebridge.HostIdentity, capability string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.held["fenced"] {
		return packagebridge.ErrHostFenced
	}
	if g.held[capability] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

var (
	forwardHost  = packagebridge.HostIdentity{PackageID: "forward", Version: "4.1.0", Generation: 3}
	proxyHost    = packagebridge.HostIdentity{PackageID: "proxy-node", Version: "4.1.0", Generation: 5}
	protocolHost = packagebridge.HostIdentity{PackageID: "protocol-runtime", Version: "4.1.0", Generation: 2}
)

// harness is an engine on a seeded database with its executors.
type harness struct {
	db       *gorm.DB
	engine   *Engine
	registry *Registry
}

// newHarness builds an engine; options adjust it before it runs.
func newHarness(t *testing.T, db *gorm.DB, options ...func(*Engine)) *harness {
	t.Helper()
	registry := NewRegistry()
	engine := &Engine{DB: db, Executors: registry, PollInterval: 20 * time.Millisecond}
	for _, option := range options {
		option(engine)
	}
	return &harness{db: db, engine: engine, registry: registry}
}

// start runs the dispatcher until the test ends.
func (h *harness) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.engine.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func (h *harness) serve(kindName string, executor Executor) {
	if err := h.registry.Register(kindName, executor); err != nil {
		panic(err)
	}
}

func (h *harness) client(host packagebridge.HostIdentity, authorizer Authorizer) kernelnodeopsv1.KernelNodeOpsServer {
	return (&Server{Engine: h.engine, Authorizer: authorizer}).For(host)
}

// rows returns the ledger, oldest first.
func (h *harness) rows(t *testing.T) []model.KernelNodeOperation {
	t.Helper()
	var rows []model.KernelNodeOperation
	require.NoError(t, h.db.Order("id").Find(&rows).Error)
	return rows
}

// states returns the states an operation's events recorded, in order.
func (h *harness) states(t *testing.T, operationID string) []string {
	t.Helper()
	var states []string
	require.NoError(t, h.db.Model(&model.KernelNodeOperationEvent{}).Where("operation_id = ?", operationID).Order("id").Pluck("state", &states).Error)
	return states
}

// eventually polls get until done or 10 seconds passed.
func eventually(t *testing.T, get func() *kernelnodeopsv1.Operation, done func(*kernelnodeopsv1.Operation) bool) *kernelnodeopsv1.Operation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		operation := get()
		if done(operation) {
			return operation
		}
		if time.Now().After(deadline) {
			require.FailNowf(t, "operation did not reach the expected state", "last: %v", operation)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func inState(states ...kernelnodeopsv1.OperationState) func(*kernelnodeopsv1.Operation) bool {
	return func(operation *kernelnodeopsv1.Operation) bool {
		for _, state := range states {
			if operation.GetState() == state {
				return true
			}
		}
		return false
	}
}

func get(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, operationID string) func() *kernelnodeopsv1.Operation {
	return func() *kernelnodeopsv1.Operation {
		response, err := client.GetOperation(context.Background(), &kernelnodeopsv1.GetOperationRequest{
			Selector: &kernelnodeopsv1.GetOperationRequest_OperationId{OperationId: operationID},
		})
		require.NoError(t, err)
		return response.GetOperation()
	}
}

func submit(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, requestID string, spec *kernelnodeopsv1.OperationSpec) *kernelnodeopsv1.SubmitOperationResponse {
	t.Helper()
	response, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: requestID, Operation: spec})
	require.NoError(t, err)
	return response
}

// Operation builders.

func applyForward(id uint64, action kernelnodeopsv1.ForwardAction) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_ApplyForward{
		ApplyForward: &kernelnodeopsv1.ApplyForward{ForwardId: id, Action: action},
	}}
}

func applyTunnel(id uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_ApplyTunnel{
		ApplyTunnel: &kernelnodeopsv1.ApplyTunnel{TunnelId: id, Reasons: []string{"protocol"}},
	}}
}

func syncNode(kind kernelnodeopsv1.NodeKind, id uint64, force bool) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_SyncNode{
		SyncNode: &kernelnodeopsv1.SyncNode{Node: nodeRef(kind, id), Force: force},
	}}
}

func retireNode(kind kernelnodeopsv1.NodeKind, id uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RetireNode{
		RetireNode: &kernelnodeopsv1.RetireNode{Node: nodeRef(kind, id)},
	}}
}

func checkEndpoints(nodes ...*kernelnodeopsv1.NodeRef) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_CheckEndpoints{
		CheckEndpoints: &kernelnodeopsv1.CheckEndpoints{Nodes: nodes},
	}}
}

func agentOperation(node uint64, kind string) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_AgentControlOperation{
		AgentControlOperation: &kernelnodeopsv1.AgentControlOperation{NodeId: node, Kind: kind},
	}}
}

func issueCredential(subject *kernelnodeopsv1.NodeRef, kind kernelnodeopsv1.CredentialKind, handle string) *kernelnodeopsv1.OperationSpec {
	operation := &kernelnodeopsv1.IssueCredential{Subject: subject, Kind: kind, Replace: true}
	if handle != "" {
		operation.Value = &kernelnodeopsv1.SecretRef{Handle: handle}
	}
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueCredential{IssueCredential: operation}}
}

func putSecretDocument(scope kernelnodeopsv1.SecretScope, owner uint64, column, document string) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_PutSecretDocument{
		PutSecretDocument: &kernelnodeopsv1.PutSecretDocument{Scope: scope, OwnerId: owner, Column: column, DocumentJson: []byte(document)},
	}}
}

// script is a fake executor: each dispatch is reported on started and runs
// behave.
type script struct {
	started chan *Run
	behave  func(ctx context.Context, run *Run) Outcome
}

func newScript(behave func(ctx context.Context, run *Run) Outcome) *script {
	return &script{started: make(chan *Run, 64), behave: behave}
}

func (s *script) Execute(ctx context.Context, run *Run) Outcome {
	s.started <- run
	return s.behave(ctx, run)
}

func succeed(context.Context, *Run) Outcome { return Succeeded(nil) }

// held runs until release answers or its context ends.
type held struct {
	*script
	release chan Outcome
}

func newHeld() *held {
	h := &held{release: make(chan Outcome, 64)}
	h.script = newScript(func(ctx context.Context, run *Run) Outcome {
		if err := run.Accept(ctx, Acceptance{Channel: kernelnodeopsv1.Channel_CHANNEL_KERNEL}); err != nil {
			return Cancelled("ended before it was accepted")
		}
		select {
		case outcome := <-h.release:
			return outcome
		case <-ctx.Done():
			return Cancelled("")
		}
	})
	return h
}

func (h *held) next(t *testing.T) *Run {
	t.Helper()
	select {
	case run := <-h.started:
		return run
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the executor was not started")
		return nil
	}
}
