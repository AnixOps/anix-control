package agentupgrade

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSN enables the PostgreSQL runs (TestPostgres*); each gets a
// throwaway schema.
const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

var start = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(postgresDSN))
	if base == "" {
		t.Skip(postgresDSN + " is not set")
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
	schema := "agent_upgrade_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

func migrate(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(append(model.AgentUpgradeModels(),
		&model.AgentTransport{}, &model.Node{}, &model.ForwardNode{}, &model.KernelNodeConfigStatus{}, &model.OperationLog{})...))
}

// clock is a settable time.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fakeStreams is the Agent Control streams of the test: one session per
// node, dispatches recorded, acknowledgements chosen per node.
type fakeStreams struct {
	mu       sync.Mutex
	sessions map[agentcontrol.AgentNode]agentstreams.Session
	sent     []sentOperation
	refuse   map[agentcontrol.AgentNode]string
	handler  agentstreams.ObservedHandler
	sequence int
}

type sentOperation struct {
	node      agentcontrol.AgentNode
	operation *agentv1pb.DesiredOperation
	request   agentcontrol.UpgradeRequest
}

func newFakeStreams() *fakeStreams {
	return &fakeStreams{sessions: map[agentcontrol.AgentNode]agentstreams.Session{}, refuse: map[agentcontrol.AgentNode]string{}}
}

// connect opens a new session of node with version; upgrade says whether
// it negotiated upgrade.v1 (config.v1 is never negotiated unless config).
func (f *fakeStreams) connect(node agentcontrol.AgentNode, version string, at time.Time, capabilities ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sequence++
	f.sessions[node] = agentstreams.Session{
		Node: node, Transport: agentstreams.TransportControlStream, SessionID: fmt.Sprintf("s-%d", f.sequence), AgentVersion: version,
		ConnectedAt: at, LastSeen: at, NegotiatedCapabilities: capabilities,
	}
}

func (f *fakeStreams) disconnect(node agentcontrol.AgentNode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, node)
}

func (f *fakeStreams) operations() []sentOperation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentOperation(nil), f.sent...)
}

func (f *fakeStreams) Session(node agentcontrol.AgentNode) (agentstreams.Session, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	session, ok := f.sessions[node]
	return session, ok
}

func (f *fakeStreams) Sessions() []agentstreams.Session { return nil }

func (f *fakeStreams) Observed(agentcontrol.AgentNode) (*agentv1pb.ObservedState, bool) {
	return nil, false
}

func (f *fakeStreams) Dispatch(_ context.Context, node agentcontrol.AgentNode, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	session, ok := f.sessions[node]
	if !ok {
		return nil, agentstreams.ErrNotConnected
	}
	upgrade := false
	for _, name := range session.NegotiatedCapabilities {
		upgrade = upgrade || name == "upgrade.v1"
	}
	if operation.GetKind() == agentcontrol.OperationKindAgentUpgrade && !upgrade {
		return nil, fmt.Errorf("agent node %s does not advertise capability %q: %w", node, operation.GetKind(), agentstreams.ErrCapabilityMissing)
	}
	request, err := agentcontrol.ParseUpgradeRequest(operation.GetPayloadJson())
	if err != nil {
		return nil, err
	}
	f.sent = append(f.sent, sentOperation{node: node, operation: operation, request: request})
	if reason, refused := f.refuse[node]; refused {
		return &agentv1pb.OperationAck{OperationId: operation.GetOperationId(), Accepted: false, Error: reason, SessionId: session.SessionID}, nil
	}
	return &agentv1pb.OperationAck{OperationId: operation.GetOperationId(), Accepted: true, SessionId: session.SessionID, Revision: 1}, nil
}

func (f *fakeStreams) Cancel(context.Context, agentcontrol.AgentNode, string, uint64) error {
	return nil
}

func (f *fakeStreams) OnObserved(handler agentstreams.ObservedHandler) { f.handler = handler }

// fixture is a database with stream-seen nodes, the fake streams and a
// worker on the test clock.
type fixture struct {
	t       *testing.T
	db      *gorm.DB
	clock   *clock
	streams *fakeStreams
	service *Service
	worker  *Worker
}

func newFixture(t *testing.T, db *gorm.DB) *fixture {
	c := &clock{now: start}
	streams := newFakeStreams()
	service := &Service{DB: db, Now: c.Now}
	return &fixture{t: t, db: db, clock: c, streams: streams, service: service, worker: &Worker{Service: service, Streams: func() agentstreams.Streams { return streams }}}
}

// addProxy creates an enabled proxy node seen on the mTLS stream.
func (f *fixture) addProxy(id uint, tags string) agentcontrol.AgentNode {
	f.t.Helper()
	node := model.Node{ID: id, Name: fmt.Sprintf("proxy-%d", id), APIKey: fmt.Sprintf("key-%d", id), Status: model.NodeStatusOnline}
	if tags != "" {
		node.Tags = &tags
	}
	require.NoError(f.t, f.db.Create(&node).Error)
	f.seen(agentcontrol.NodeKindProxy, id, model.AgentTransportMTLSStream)
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(id)}
}

// addForward creates an enabled forward node seen on the mTLS stream.
func (f *fixture) addForward(id uint, tags string) agentcontrol.AgentNode {
	f.t.Helper()
	require.NoError(f.t, f.db.Create(&model.ForwardNode{ID: id, Name: fmt.Sprintf("forward-%d", id), Host: "192.0.2.1", Port: 1, Enabled: true, Tags: tags}).Error)
	f.seen(agentcontrol.NodeKindForward, id, model.AgentTransportMTLSStream)
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(id)}
}

func (f *fixture) seen(kind string, id uint, transport string) {
	f.t.Helper()
	require.NoError(f.t, f.db.Create(&model.AgentTransport{
		NodeKind: kind, NodeID: id, Transport: transport, AgentVersion: "v4.1.0", FirstSeenAt: start, LastSeenAt: start,
	}).Error)
}

func (f *fixture) start(request StartRequest) model.AgentUpgradeCampaign {
	f.t.Helper()
	if request.TargetVersion == "" {
		request.TargetVersion = "v4.2.0"
	}
	if request.ControlVersion == "" {
		request.ControlVersion = "4.2.0"
	}
	if request.Artifacts == nil {
		request.Artifacts = testArtifacts()
	}
	campaign, err := f.service.Start(context.Background(), request)
	require.NoError(f.t, err)
	return campaign
}

func (f *fixture) tick() {
	f.t.Helper()
	require.NoError(f.t, f.worker.Tick(context.Background()))
}

func (f *fixture) campaign(id string) CampaignView {
	f.t.Helper()
	view, err := f.service.Get(context.Background(), id)
	require.NoError(f.t, err)
	return view
}

func (f *fixture) nodeState(id string, node agentcontrol.AgentNode) model.AgentUpgradeNode {
	f.t.Helper()
	var row model.AgentUpgradeNode
	require.NoError(f.t, f.db.Where("campaign_id = ? AND node_kind = ? AND node_id = ?", id, node.Kind, node.ID).First(&row).Error)
	return row
}

func (f *fixture) batchNodes(id string, batch int) []agentcontrol.AgentNode {
	f.t.Helper()
	var rows []model.AgentUpgradeNode
	require.NoError(f.t, f.db.Where("campaign_id = ? AND batch = ?", id, batch).Order("order_key").Find(&rows).Error)
	nodes := make([]agentcontrol.AgentNode, 0, len(rows))
	for _, row := range rows {
		nodes = append(nodes, agentcontrol.AgentNode{Kind: row.NodeKind, ID: uint32(row.NodeID)})
	}
	return nodes
}

// upgrade simulates a node's Agent applying the upgrade: it reports the
// hand-off, restarts and reconnects with version.
func (f *fixture) upgrade(node agentcontrol.AgentNode, version string) {
	f.t.Helper()
	row := f.lastOperation(node)
	f.worker.HandleObserved(node, &agentv1pb.ObservedState{
		OperationId: row.operation.GetOperationId(), Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		StateJson: []byte(`{"phase":"handed_off"}`),
	})
	f.clock.Advance(time.Second)
	capabilities := []string{"upgrade.v1"}
	if session, ok := f.streams.Session(node); ok {
		capabilities = session.NegotiatedCapabilities
	}
	f.streams.connect(node, version, f.clock.Now(), capabilities...)
}

func (f *fixture) lastOperation(node agentcontrol.AgentNode) sentOperation {
	f.t.Helper()
	operations := f.streams.operations()
	for index := len(operations) - 1; index >= 0; index-- {
		if operations[index].node == node {
			return operations[index]
		}
	}
	f.t.Fatalf("no operation sent to %s", node)
	return sentOperation{}
}

func testArtifacts() []agentcontrol.UpgradeArtifact {
	return []agentcontrol.UpgradeArtifact{
		{Arch: "amd64", Asset: "anix-agent-linux-64.zip", URL: "https://panel.example.com/install/agent/v4.2.0/anix-agent-linux-64.zip",
			SHA256: strings.Repeat("a", 64), Size: 10, Signature: base64.StdEncoding.EncodeToString(make([]byte, 64))},
		{Arch: "arm64", Asset: "anix-agent-linux-arm64-v8a.zip", URL: "https://panel.example.com/install/agent/v4.2.0/anix-agent-linux-arm64-v8a.zip",
			SHA256: strings.Repeat("b", 64), Size: 10, Signature: base64.StdEncoding.EncodeToString(make([]byte, 64))},
	}
}
