package agenttransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormConfig = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)"), gormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

func migrate(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.AgentTransport{}, &model.AgentCertificate{}, &model.Node{},
		&model.ForwardNode{}, &model.ForwardCleanAgent{}))
}

// openPostgres opens a throwaway schema of ANIX_TEST_POSTGRES_DSN.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
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
	schema := "agent_transport_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), gormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

// clock is a settable test clock.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newTestRecorder(db *gorm.DB, c *clock) *Recorder {
	r := NewRecorder(func() *gorm.DB { return db })
	r.now = c.Now
	return r
}

var proxy7 = agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 7}

func loadRow(t *testing.T, db *gorm.DB, node agentcontrol.AgentNode, transport string) (model.AgentTransport, bool) {
	t.Helper()
	var rows []model.AgentTransport
	require.NoError(t, db.Where("node_kind = ? AND node_id = ? AND transport = ?", node.Kind, node.ID, transport).Find(&rows).Error)
	if len(rows) == 0 {
		return model.AgentTransport{}, false
	}
	return rows[0], true
}

func TestMetrics(t *testing.T) {
	CountRefused("/test/metrics")
	CountRefused("")
	require.Positive(t, RefusedRequests("/test/metrics"))
	require.Positive(t, RefusedRequests(otherPath))

	var body strings.Builder
	WritePrometheus(&body)
	text := body.String()
	require.Contains(t, text, "# TYPE anixops_agent_legacy_refused_total counter")
	require.Contains(t, text, `anixops_agent_legacy_refused_total{path="/test/metrics"}`)

	// The path label is bounded.
	vec := newCounterVec()
	for i := 0; i < maxPathLabels+10; i++ {
		vec.inc("/p/" + strings.Repeat("x", i))
	}
	require.LessOrEqual(t, len(vec.values), maxPathLabels+1)
	require.Equal(t, uint64(10), vec.get(otherPath))
	require.Equal(t, `a\"b\\c\n`, escapeLabel("a\"b\\c\n"))
}

func TestRecorderThrottlesWrites(t *testing.T) {
	db := openSQLite(t)
	c := &clock{now: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
	r := newTestRecorder(db, c)
	ctx := context.Background()

	r.Seen(ctx, Sighting{Node: proxy7, Transport: legacyAPIKeyStream, AgentVersion: "1.1.0", Identity: "api-key"})
	row, ok := loadRow(t, db, proxy7, legacyAPIKeyStream)
	require.True(t, ok, "the first sighting is written at once")
	require.Equal(t, "1.1.0", row.AgentVersion)
	first := row.LastSeenAt

	// Within the interval: memory only.
	c.now = c.now.Add(30 * time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: legacyAPIKeyStream})
	row, _ = loadRow(t, db, proxy7, legacyAPIKeyStream)
	require.True(t, row.LastSeenAt.Equal(first), "throttled")
	snapshot := r.Snapshot()
	require.Len(t, snapshot, 1)
	require.True(t, snapshot[0].LastSeenAt.Equal(c.now), "memory holds the live value")
	require.Equal(t, "1.1.0", snapshot[0].AgentVersion, "an empty version keeps the known one")

	// A new version is written at once.
	c.now = c.now.Add(time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: legacyAPIKeyStream, AgentVersion: "1.2.0"})
	row, _ = loadRow(t, db, proxy7, legacyAPIKeyStream)
	require.Equal(t, "1.2.0", row.AgentVersion)
	require.True(t, row.LastSeenAt.Equal(c.now))
	require.True(t, row.FirstSeenAt.Equal(first), "first seen stays")

	// After the interval the next sighting is written.
	c.now = c.now.Add(10 * time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: legacyAPIKeyStream})
	row, _ = loadRow(t, db, proxy7, legacyAPIKeyStream)
	require.False(t, row.LastSeenAt.Equal(c.now), "still throttled")
	c.now = c.now.Add(PersistInterval)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: legacyAPIKeyStream})
	row, _ = loadRow(t, db, proxy7, legacyAPIKeyStream)
	require.True(t, row.LastSeenAt.Equal(c.now))

	// Flush writes what is pending.
	c.now = c.now.Add(5 * time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: legacyAPIKeyStream})
	require.NoError(t, r.Flush(ctx))
	row, _ = loadRow(t, db, proxy7, legacyAPIKeyStream)
	require.True(t, row.LastSeenAt.Equal(c.now))
	require.NoError(t, r.Flush(ctx), "nothing pending")

	// Certificate details, and sightings that name no node are dropped.
	notAfter := c.now.Add(7 * 24 * time.Hour)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportMTLSStream, Identity: "spiffe://anixops/prod/agent/proxy-7",
		CertSerial: "abc", CertNotAfter: &notAfter, AgentVersion: strings.Repeat("v", 80)})
	row, ok = loadRow(t, db, proxy7, model.AgentTransportMTLSStream)
	require.True(t, ok)
	require.Equal(t, "abc", row.CertSerial)
	require.Len(t, row.AgentVersion, 64)
	require.NotNil(t, row.CertNotAfter)
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: "edge", ID: 1}, Transport: legacyWebSocket})
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy}, Transport: legacyWebSocket})
	r.Seen(ctx, Sighting{Node: proxy7})
	require.Len(t, r.Snapshot(), 2)

	// A nil recorder, and one without a database, record nothing and fail
	// nothing.
	var none *Recorder
	none.Seen(ctx, Sighting{Node: proxy7, Transport: legacyWebSocket})
	require.Nil(t, none.Snapshot())
	require.NoError(t, none.Flush(ctx))
	NewRecorder(nil).Seen(ctx, Sighting{Node: proxy7, Transport: legacyWebSocket})
	NewRecorder(func() *gorm.DB { return nil }).Seen(ctx, Sighting{Node: proxy7, Transport: legacyWebSocket})
}

func TestRecorderRetriesAFailedWrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), gormConfig)
	require.NoError(t, err)
	c := &clock{now: time.Now().UTC()}
	r := newTestRecorder(db, c)
	ctx := context.Background()
	r.Seen(ctx, Sighting{Node: proxy7, Transport: legacyWebSocket})
	require.Error(t, r.Flush(ctx), "the table is missing: the sighting stays pending")
	migrate(t, db)
	require.NoError(t, r.Flush(ctx))
	_, ok := loadRow(t, db, proxy7, legacyWebSocket)
	require.True(t, ok)
}

func TestDefaultRecorder(t *testing.T) {
	db := openSQLite(t)
	restore := SetDefault(newTestRecorder(db, &clock{now: time.Now().UTC()}))
	defer restore()
	Seen(context.Background(), Sighting{Node: proxy7, Transport: model.AgentTransportMTLSStream})
	_, ok := loadRow(t, db, proxy7, model.AgentTransportMTLSStream)
	require.True(t, ok)
	require.Len(t, Default().Snapshot(), 1)
}

func seedInventory(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	version := "1.1.0-node"
	require.NoError(t, db.Create(&[]model.Node{
		{ID: 1, Name: "enrolled", APIKey: "k1", Status: model.NodeStatusOnline},
		{ID: 2, Name: "legacy", APIKey: "k2", Status: model.NodeStatusOnline, ServerVersion: &version},
		{ID: 3, Name: "v2bx", APIKey: "k3", Status: model.NodeStatusOnline},
		{ID: 4, Name: "never", APIKey: "k4", Status: model.NodeStatusDisabled},
	}).Error)
	require.NoError(t, db.Create(&[]model.ForwardNode{{ID: 1, Name: "relay", Host: "198.51.100.1", Port: 22, Enabled: true}}).Error)
	forwardNode := uint(1)
	seen := now.Add(-2 * time.Minute)
	require.NoError(t, db.Create(&model.ForwardCleanAgent{ID: 9, NodeID: &forwardNode, Name: "clean", Token: "t", Version: "0.9", LastSeen: &seen}).Error)
	require.NoError(t, db.Create(&[]model.AgentCertificate{
		{Serial: "old", NodeKind: "proxy", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: now.Add(time.Hour)},
		{Serial: "new", NodeKind: "proxy", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: now.Add(48 * time.Hour)},
		{Serial: "revoked", NodeKind: "proxy", NodeID: 2, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: now.Add(48 * time.Hour), RevokedAt: &now},
	}).Error)
	at := func(minutes int) time.Time { return now.Add(time.Duration(-minutes) * time.Minute) }
	require.NoError(t, db.Create(&[]model.AgentTransport{
		// Node 1 moved to the mTLS stream; it used to pull UniProxy.
		{NodeKind: "proxy", NodeID: 1, Transport: legacyAPIKeyStream, AgentVersion: "1.1.0", FirstSeenAt: at(600), LastSeenAt: at(300)},
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0", FirstSeenAt: at(200), LastSeenAt: at(5)},
		{NodeKind: "proxy", NodeID: 1, Transport: "uniproxy", FirstSeenAt: at(600), LastSeenAt: at(1)},
		// Node 2 is a legacy agent: WebSocket and REST.
		{NodeKind: "proxy", NodeID: 2, Transport: legacyWebSocket, FirstSeenAt: at(600), LastSeenAt: at(3)},
		{NodeKind: "proxy", NodeID: 2, Transport: legacyHTTP, FirstSeenAt: at(600), LastSeenAt: at(2)},
		// Node 3 was third-party software on UniProxy and v2board gRPC only.
		{NodeKind: "proxy", NodeID: 3, Transport: "uniproxy", FirstSeenAt: at(600), LastSeenAt: at(1)},
		{NodeKind: "proxy", NodeID: 3, Transport: "v2board-grpc", FirstSeenAt: at(600), LastSeenAt: at(1)},
		// A deleted node's row is not listed.
		{NodeKind: "proxy", NodeID: 99, Transport: legacyWebSocket, FirstSeenAt: at(600), LastSeenAt: at(1)},
	}).Error)
}

func TestInventory(t *testing.T) {
	db := openSQLite(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	seedInventory(t, db, now)

	inventory, err := Build(context.Background(), db, Options{Now: now})
	require.NoError(t, err)
	require.Equal(t, UpgradeGuideURL, inventory.UpgradeGuide)
	counts := inventory.Summary
	counts.ReadyForRequired, counts.RequiredReasons, counts.RequiredBlockers = false, nil, nil
	require.Equal(t, Summary{Total: 5, MTLS: 1, Unseen: 4}, counts, "retired channels are not shown")
	require.False(t, inventory.Summary.ReadyForRequired)
	require.Len(t, inventory.Summary.RequiredBlockers, 3, "the disabled unseen node does not count")
	require.Equal(t, "proxy-2", inventory.Summary.RequiredBlockers[0].Node)
	require.Equal(t, BlockerNeverEnrolled, inventory.Summary.RequiredBlockers[0].Reason)
	require.Equal(t, "proxy-3", inventory.Summary.RequiredBlockers[1].Node, "UniProxy software never enrolled")
	require.Equal(t, "forward-1", inventory.Summary.RequiredBlockers[2].Node)
	byNode := map[string]NodeTransports{}
	for _, node := range inventory.Nodes {
		byNode[node.Node] = node
	}
	enrolled := byNode["proxy-1"]
	require.Equal(t, StatusMTLS, enrolled.Status)
	require.Equal(t, model.AgentTransportMTLSStream, enrolled.Transport, "the latest sighting of a channel that is not retired")
	require.Equal(t, "2.0.0", enrolled.AgentVersion)
	require.NotNil(t, enrolled.Certificate)
	require.Equal(t, "new", enrolled.Certificate.Serial)
	require.Equal(t, "spiffe://anixops/prod/agent/proxy-1", enrolled.Certificate.SPIFFEID)
	require.Len(t, enrolled.Transports, 1, "the retired API key stream and UniProxy rows are not listed")

	retired := byNode["proxy-2"]
	require.Equal(t, StatusUnseen, retired.Status, "only retired channels were seen")
	require.Empty(t, retired.Transport)
	require.Equal(t, "1.1.0-node", retired.AgentVersion, "the node row's version")
	require.Nil(t, retired.Certificate, "a revoked certificate is no identity")

	require.Equal(t, StatusUnseen, byNode["proxy-3"].Status, "UniProxy and v2board gRPC are retired")
	never := byNode["proxy-4"]
	require.Equal(t, StatusUnseen, never.Status)
	require.False(t, never.Enabled)
	require.Nil(t, never.LastSeenAt)
	require.NotNil(t, never.Transports)

	require.Equal(t, StatusUnseen, byNode["forward-1"].Status, "a clean agent is a retired channel")
	_, listed := byNode["proxy-99"]
	require.False(t, listed)

	// The live overlay: node 2 enrolled a second ago, not written yet.
	live := newTestRecorder(nil, &clock{now: now})
	live.Seen(context.Background(), Sighting{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0"})
	inventory, err = Build(context.Background(), db, Options{Now: now, Live: live})
	require.NoError(t, err)
	require.Equal(t, StatusMTLS, inventoryByNode(inventory)["proxy-2"].Status)

	inventory, err = Build(context.Background(), db, Options{})
	require.NoError(t, err)
	for _, node := range inventory.Nodes {
		require.Nil(t, node.Session, "no sessions are read without a provider")
	}

	// The live sessions: each node's Agent Control stream session, with
	// how it authenticated and what it negotiated; WebSocket sessions are
	// not shown.
	notAfter := now.Add(48 * time.Hour)
	inventory, err = Build(context.Background(), db, Options{Now: now, Sessions: func() []agentstreams.Session {
		return []agentstreams.Session{
			{
				Node: agentcontrol.AgentNode{Kind: "proxy", ID: 1}, Transport: agentstreams.TransportControlStream, SessionID: "session-1",
				AgentVersion: "2.0.0", ConnectedAt: now.Add(-time.Hour), LastSeen: now, Identity: "spiffe://anixops/prod/agent/proxy-1",
				Authentication: agentstreams.AuthenticationMTLS, NegotiatedCapabilities: []string{"config.v1", "reports.v1"},
				Certificate: &agentstreams.SessionCertificate{Serial: "new", NotAfter: notAfter, SPIFFEID: "spiffe://anixops/prod/agent/proxy-1"},
			},
			{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: agentstreams.Transport("websocket")},
			{
				Node: agentcontrol.AgentNode{Kind: "forward", ID: 1}, Transport: agentstreams.TransportControlStream, SessionID: "session-f",
				Identity: "spiffe://anixops/prod/agent/forward-1", Authentication: agentstreams.AuthenticationMTLS,
			},
		}
	}})
	require.NoError(t, err)
	byNode = map[string]NodeTransports{}
	for _, node := range inventory.Nodes {
		byNode[node.Node] = node
	}
	session := byNode["proxy-1"].Session
	require.NotNil(t, session)
	require.Equal(t, "session-1", session.SessionID)
	require.Equal(t, agentstreams.AuthenticationMTLS, session.Authentication)
	require.Equal(t, "new", session.Certificate.Serial)
	require.Equal(t, []string{"config.v1", "reports.v1"}, session.NegotiatedCapabilities)
	require.Nil(t, byNode["proxy-2"].Session, "a WebSocket session is not a stream session")
	forward := byNode["forward-1"].Session
	require.NotNil(t, forward, "forward and proxy nodes of one id are told apart")
	require.Equal(t, "spiffe://anixops/prod/agent/forward-1", forward.Identity)
	require.NotNil(t, forward.NegotiatedCapabilities)
}

// TestRequiredReadiness: ready_for_required and its reasons from an
// inventory: enabled nodes that never enrolled block; mTLS, third-party,
// enrolled-but-unseen and disabled nodes do not. Sightings of retired
// channels count for nothing.
func TestRequiredReadiness(t *testing.T) {
	db := openSQLite(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	inventory, err := Build(context.Background(), db, Options{Now: now})
	require.NoError(t, err)
	require.True(t, inventory.Summary.ReadyForRequired, "no node, nothing to refuse")
	require.Empty(t, inventory.Summary.RequiredReasons)
	require.NotNil(t, inventory.Summary.RequiredReasons, "an empty list, not null")
	require.NotNil(t, inventory.Summary.RequiredBlockers)

	require.NoError(t, db.Create(&[]model.Node{
		{ID: 1, Name: "mtls", APIKey: "k1", Status: model.NodeStatusOnline},
		{ID: 2, Name: "retired-channel", APIKey: "k2", Status: model.NodeStatusOnline},
		{ID: 4, Name: "disabled-retired", APIKey: "k4", Status: model.NodeStatusDisabled},
		{ID: 5, Name: "v2bx", APIKey: "k5", Status: model.NodeStatusOnline},
		{ID: 6, Name: "enrolled-unseen", APIKey: "k6", Status: model.NodeStatusOnline},
		{ID: 7, Name: "never", APIKey: "k7", Status: model.NodeStatusOnline},
		{ID: 8, Name: "disabled-never", APIKey: "k8", Status: model.NodeStatusDisabled},
	}).Error)
	require.NoError(t, db.Create(&[]model.ForwardNode{{ID: 1, Name: "relay-never", Host: "198.51.100.1", Port: 22, Enabled: true}}).Error)
	require.NoError(t, db.Create(&model.AgentCertificate{Serial: "c6", NodeKind: "proxy", NodeID: 6, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: now.Add(time.Hour)}).Error)
	days := func(n int) time.Time { return now.Add(time.Duration(-n) * 24 * time.Hour) }
	require.NoError(t, db.Create(&[]model.AgentTransport{
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, FirstSeenAt: days(3), LastSeenAt: days(0)},
		{NodeKind: "proxy", NodeID: 2, Transport: legacyAPIKeyStream, FirstSeenAt: days(30), LastSeenAt: days(1)},
		{NodeKind: "proxy", NodeID: 4, Transport: legacyWebSocket, FirstSeenAt: days(30), LastSeenAt: days(1)},
		{NodeKind: "proxy", NodeID: 5, Transport: "uniproxy", FirstSeenAt: days(30), LastSeenAt: days(0)},
	}).Error)

	inventory, err = Build(context.Background(), db, Options{Now: now})
	require.NoError(t, err)
	summary := inventory.Summary
	require.False(t, summary.ReadyForRequired)
	blockers := map[string]RequiredBlocker{}
	for _, blocker := range summary.RequiredBlockers {
		blockers[blocker.Node] = blocker
	}
	require.Len(t, blockers, 4, "%+v", summary.RequiredBlockers)
	for _, node := range []string{"proxy-2", "proxy-5", "proxy-7", "forward-1"} {
		require.Equal(t, BlockerNeverEnrolled, blockers[node].Reason, node)
		require.Nil(t, blockers[node].LastSeenAt, node)
	}
	require.Equal(t, []string{
		"4 enabled node(s) never enrolled (no agent certificate, never seen): install or enroll their Agents, or disable the nodes",
	}, summary.RequiredReasons)

	// Disabling the nodes clears the blockers.
	require.NoError(t, db.Model(&model.Node{}).Where("id IN ?", []uint{2, 5, 7}).Update("status", model.NodeStatusDisabled).Error)
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 1).Update("enabled", false).Error)
	inventory, err = Build(context.Background(), db, Options{Now: now})
	require.NoError(t, err)
	require.True(t, inventory.Summary.ReadyForRequired, "%+v", inventory.Summary)
	require.Empty(t, inventory.Summary.RequiredReasons)
}

func TestOverlayKeepsTheNewestSighting(t *testing.T) {
	now := time.Now().UTC()
	rows := []model.AgentTransport{{NodeKind: "proxy", NodeID: 1, Transport: "websocket", FirstSeenAt: now.Add(-time.Hour), LastSeenAt: now.Add(-time.Minute)}}
	merged := overlay(rows, []model.AgentTransport{
		{NodeKind: "proxy", NodeID: 1, Transport: "websocket", FirstSeenAt: now, LastSeenAt: now},
		{NodeKind: "proxy", NodeID: 2, Transport: "websocket", FirstSeenAt: now, LastSeenAt: now},
	})
	require.Len(t, merged, 2)
	require.True(t, merged[0].LastSeenAt.Equal(now))
	require.True(t, merged[0].FirstSeenAt.Equal(now.Add(-time.Hour)))
	older := overlay(merged, []model.AgentTransport{{NodeKind: "proxy", NodeID: 2, Transport: "websocket", LastSeenAt: now.Add(-time.Hour)}})
	require.True(t, older[1].LastSeenAt.Equal(now), "an older live value loses")
}

// TestPostgresAgentTransportInventory runs the recorder's upsert and the
// inventory's queries on PostgreSQL.
func TestPostgresAgentTransportInventory(t *testing.T) {
	db := openPostgres(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedInventory(t, db, now)
	c := &clock{now: now}
	r := newTestRecorder(db, c)
	ctx := context.Background()
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: legacyWebSocket, AgentVersion: "1.1.1"})
	row, ok := loadRow(t, db, agentcontrol.AgentNode{Kind: "proxy", ID: 2}, legacyWebSocket)
	require.True(t, ok)
	require.Equal(t, "1.1.1", row.AgentVersion)
	require.True(t, row.LastSeenAt.Equal(now))
	require.True(t, row.FirstSeenAt.Before(now), "the upsert keeps first_seen_at")
	c.now = now.Add(PersistInterval)
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0"})
	inventory, err := Build(ctx, db, Options{Now: c.now})
	require.NoError(t, err)
	require.Equal(t, 2, inventory.Summary.MTLS, "the retired websocket row is ignored, the mTLS sighting counts")
	require.Equal(t, 2, inventory.Summary.Unseen)
}

// seedCertificates records the agent certificates of the inventory's nodes:
// proxy-1 holds two valid ones (the newest was issued two days ago, with the
// seven-day lifetime), proxy-2 only a revoked one, proxy-3 only an expired
// one, proxy-4 none; proxy-5 was revoked after a valid one expired; forward-1
// has one valid certificate that its credentials revocation then revoked.
func seedCertificates(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	day := 24 * time.Hour
	revokedAt := now.Add(-time.Hour)
	require.NoError(t, db.Create(&[]model.AgentCertificate{
		{Serial: "old", NodeKind: "proxy", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-6 * day), NotAfter: now.Add(day)},
		{Serial: "new", NodeKind: "proxy", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-2 * day), NotAfter: now.Add(5 * day)},
		{Serial: "revoked", NodeKind: "proxy", NodeID: 2, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-3 * day), NotAfter: now.Add(4 * day), RevokedAt: &revokedAt, RevokeReason: "credentials_replaced"},
		{Serial: "expired", NodeKind: "proxy", NodeID: 3, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-8 * day), NotAfter: now.Add(-time.Hour)},
		{Serial: "gone", NodeKind: "proxy", NodeID: 5, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-9 * day), NotAfter: now.Add(-2 * day)},
		{Serial: "newer-revoked", NodeKind: "proxy", NodeID: 5, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-day), NotAfter: now.Add(6 * day), RevokedAt: &revokedAt, RevokeReason: "node_disabled"},
		{Serial: "relay", NodeKind: "forward", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-day), NotAfter: now.Add(6 * day)},
	}).Error)
}

func inventoryByNode(inventory Inventory) map[string]NodeTransports {
	byNode := map[string]NodeTransports{}
	for _, node := range inventory.Nodes {
		byNode[node.Node] = node
	}
	return byNode
}

// TestInventoryCertificateStateAndRenewal: the inventory tells each node's
// certificate state, the certificate's issue, expiry and renewal times, and
// why a revoked one ended, while Certificate keeps meaning the newest valid
// certificate (the readiness check depends on it).
func TestInventoryCertificateStateAndRenewal(t *testing.T) {
	for name, open := range map[string]func(*testing.T) *gorm.DB{"sqlite": openSQLite, "postgres": openPostgres} {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			now := time.Now().UTC().Truncate(time.Microsecond)
			seedInventory(t, db, now)
			require.NoError(t, db.Where("1 = 1").Delete(&model.AgentCertificate{}).Error)
			seedCertificates(t, db, now)
			require.NoError(t, db.Create(&model.Node{ID: 5, Name: "disabled-after-expiry", APIKey: "k5", Status: model.NodeStatusDisabled}).Error)

			inventory, err := Build(context.Background(), db, Options{Now: now})
			require.NoError(t, err)
			byNode := inventoryByNode(inventory)

			valid := byNode["proxy-1"]
			require.Equal(t, CertificateValid, valid.CertificateState)
			require.NotNil(t, valid.Certificate)
			require.Equal(t, "new", valid.Certificate.Serial, "the newest expiry")
			require.True(t, valid.Certificate.NotAfter.Equal(now.Add(5*24*time.Hour)))
			require.True(t, valid.Certificate.IssuedAt.Equal(now.Add(-2*24*time.Hour)))
			// Seven days of lifetime: renew after four days and sixteen hours.
			require.True(t, valid.Certificate.RenewAfter.Equal(valid.Certificate.IssuedAt.Add(7*24*time.Hour*2/3)), valid.Certificate.RenewAfter)
			require.True(t, valid.Certificate.RenewAfter.Equal(modulepki.RenewAfter(valid.Certificate.IssuedAt, valid.Certificate.NotAfter)))
			require.Nil(t, valid.Certificate.RevokedAt)
			require.Equal(t, valid.Certificate, valid.LastCertificate, "a valid certificate is the latest one")

			revoked := byNode["proxy-2"]
			require.Equal(t, CertificateRevoked, revoked.CertificateState)
			require.Nil(t, revoked.Certificate, "a revoked certificate is no identity")
			require.NotNil(t, revoked.LastCertificate)
			require.Equal(t, "revoked", revoked.LastCertificate.Serial)
			require.NotNil(t, revoked.LastCertificate.RevokedAt)
			require.Equal(t, "credentials_replaced", revoked.LastCertificate.RevokeReason)
			require.True(t, revoked.LastCertificate.NotAfter.Equal(now.Add(4*24*time.Hour)))

			expired := byNode["proxy-3"]
			require.Equal(t, CertificateExpired, expired.CertificateState)
			require.Nil(t, expired.Certificate)
			require.Equal(t, "expired", expired.LastCertificate.Serial)
			require.Nil(t, expired.LastCertificate.RevokedAt)

			none := byNode["proxy-4"]
			require.Equal(t, CertificateNone, none.CertificateState)
			require.Nil(t, none.Certificate)
			require.Nil(t, none.LastCertificate)

			// Revoked after an older certificate expired: the newest expiry wins.
			require.Equal(t, CertificateRevoked, byNode["proxy-5"].CertificateState)
			require.Equal(t, "newer-revoked", byNode["proxy-5"].LastCertificate.Serial)
			require.Equal(t, "node_disabled", byNode["proxy-5"].LastCertificate.RevokeReason)

			require.Equal(t, CertificateValid, byNode["forward-1"].CertificateState, "forward and proxy nodes of one id are told apart")
			require.Equal(t, "relay", byNode["forward-1"].Certificate.Serial)

			// The readiness check still reads Certificate: proxy-4 never enrolled
			// and is disabled (not a blocker); revoked proxy-2 has no valid certificate.
			blockers := map[string]string{}
			for _, blocker := range inventory.Summary.RequiredBlockers {
				blockers[blocker.Node] = blocker.Reason
			}
			require.Equal(t, BlockerNeverEnrolled, blockers["proxy-2"])
			require.NotContains(t, blockers, "proxy-1")
		})
	}
}

// TestInventoryNodeFilter: Options.Nodes narrows the list, not the summary.
func TestInventoryNodeFilter(t *testing.T) {
	db := openSQLite(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	seedInventory(t, db, now)
	all, err := Build(context.Background(), db, Options{Now: now})
	require.NoError(t, err)

	one, err := Build(context.Background(), db, Options{Now: now, Nodes: []agentcontrol.AgentNode{{Kind: "proxy", ID: 2}}})
	require.NoError(t, err)
	require.Len(t, one.Nodes, 1)
	require.Equal(t, "proxy-2", one.Nodes[0].Node)
	require.Equal(t, Summary{Total: 1, Unseen: 1}, Summary{Total: one.Summary.Total, MTLS: one.Summary.MTLS,
		Unseen: one.Summary.Unseen}, "the counts follow the list")
	require.Equal(t, all.Summary.RequiredBlockers, one.Summary.RequiredBlockers, "readiness covers every node, also for one")
	require.Equal(t, all.Summary.ReadyForRequired, one.Summary.ReadyForRequired)
	require.Equal(t, all.Summary.RequiredReasons, one.Summary.RequiredReasons)

	several, err := Build(context.Background(), db, Options{Now: now, Nodes: []agentcontrol.AgentNode{
		{Kind: "forward", ID: 1}, {Kind: "proxy", ID: 1}, {Kind: "proxy", ID: 77},
	}})
	require.NoError(t, err)
	require.Len(t, several.Nodes, 2, "an unknown node is not listed")
	require.Equal(t, "proxy-1", several.Nodes[0].Node)
	require.Equal(t, "forward-1", several.Nodes[1].Node)
}

// TestConnectionOf: the connection type from a live session, else from the
// transports seen within the window.
func TestConnectionOf(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	seen := func(transport string, d time.Duration) TransportSeen {
		return TransportSeen{Transport: transport, LastSeenAt: ago(d)}
	}
	live := func(authentication string) *LiveSession {
		return &LiveSession{Authentication: authentication, LastSeenAt: ago(10 * time.Second)}
	}
	cases := []struct {
		name         string
		node         NodeTransports
		sessionsRead bool
		want         string
		transport    string
	}{
		{name: "an mTLS stream session", node: NodeTransports{Session: live(agentstreams.AuthenticationMTLS)}, sessionsRead: true, want: ConnectionMTLSStream, transport: model.AgentTransportMTLSStream},
		{name: "a session without an authentication is no connection", node: NodeTransports{Session: live("")}, sessionsRead: true, want: ConnectionOffline},
		{name: "the session beats a newer stream sighting", node: NodeTransports{Session: live(agentstreams.AuthenticationMTLS), Transports: []TransportSeen{seen(model.AgentTransportMTLSStream, time.Second)}}, sessionsRead: true, want: ConnectionMTLSStream, transport: model.AgentTransportMTLSStream},
		{name: "a stale sighting is offline", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportMTLSStream, ConnectionWindow+time.Second)}}, want: ConnectionOffline},
		{name: "never seen", node: NodeTransports{}, sessionsRead: true, want: ConnectionOffline},
		{name: "a closed stream is not connected when sessions were read", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportMTLSStream, time.Minute)}}, sessionsRead: true, want: ConnectionOffline},
		{name: "a stream sighting counts without sessions (the CLI)", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportMTLSStream, time.Minute)}}, want: ConnectionMTLSStream, transport: model.AgentTransportMTLSStream},
		{name: "the window is inclusive", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportMTLSStream, ConnectionWindow)}}, want: ConnectionMTLSStream, transport: model.AgentTransportMTLSStream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := connectionOf(tc.node, now, tc.sessionsRead)
			require.Equal(t, tc.want, got.Type)
			require.Equal(t, tc.transport, got.Transport)
			if tc.want == ConnectionOffline {
				require.Nil(t, got.LastSeenAt)
			} else {
				require.NotNil(t, got.LastSeenAt)
			}
		})
	}
}

// TestInventoryConnection: the connection of each node of the seeded
// inventory, read with and without a session provider.
func TestInventoryConnection(t *testing.T) {
	db := openSQLite(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	seedInventory(t, db, now)

	// The CLI reads no sessions: node 1's stream sighting (five minutes old,
	// inside the window) decides.
	inventory, err := Build(context.Background(), db, Options{Now: now})
	require.NoError(t, err)
	byNode := inventoryByNode(inventory)
	require.Equal(t, ConnectionMTLSStream, byNode["proxy-1"].Connection.Type)
	require.Equal(t, ConnectionOffline, byNode["proxy-2"].Connection.Type, "retired channels are not connections")
	require.Equal(t, ConnectionOffline, byNode["proxy-3"].Connection.Type, "UniProxy is retired")
	require.Equal(t, ConnectionOffline, byNode["proxy-4"].Connection.Type)
	require.Equal(t, ConnectionOffline, byNode["forward-1"].Connection.Type)

	// The API reads this process's sessions: with none, proxy-1's closed
	// stream is no connection.
	inventory, err = Build(context.Background(), db, Options{Now: now, Sessions: func() []agentstreams.Session { return nil }})
	require.NoError(t, err)
	byNode = inventoryByNode(inventory)
	require.Equal(t, ConnectionOffline, byNode["proxy-1"].Connection.Type)
	require.Equal(t, ConnectionOffline, byNode["proxy-4"].Connection.Type)

	// An mTLS session.
	inventory, err = Build(context.Background(), db, Options{Now: now, Sessions: func() []agentstreams.Session {
		return []agentstreams.Session{{
			Node: agentcontrol.AgentNode{Kind: "proxy", ID: 1}, Transport: agentstreams.TransportControlStream,
			Identity: "spiffe://anixops/prod/agent/proxy-1", Authentication: agentstreams.AuthenticationMTLS, LastSeen: now,
		}}
	}})
	require.NoError(t, err)
	require.Equal(t, ConnectionMTLSStream, inventoryByNode(inventory)["proxy-1"].Connection.Type)
}

// legacyAPIKeyStream is the transport the removed API-key stream recorded;
// older databases may still hold its rows.
const legacyAPIKeyStream = "apikey-stream"

// legacyWebSocket and legacyHTTP are transports removed Agent channels
// recorded; older databases may still hold their rows.
const (
	legacyWebSocket = "websocket"
	legacyHTTP      = "http-legacy"
)
