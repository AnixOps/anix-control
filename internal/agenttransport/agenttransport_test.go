package agenttransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/agentws"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/gin-gonic/gin"
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

func TestPolicy(t *testing.T) {
	for mode, expected := range map[string][2]bool{
		config.AgentMTLSOff: {false, false}, config.AgentMTLSOptional: {false, false},
		config.AgentMTLSPreferred: {true, false}, config.AgentMTLSRequired: {true, true},
	} {
		policy := PolicyFrom(config.AgentControlConfig{MTLS: mode})
		require.Equal(t, mode, policy.Mode)
		require.Equal(t, expected[0], policy.Signals(), mode)
		require.Equal(t, expected[1], policy.RefusesLegacy(), mode)
	}
	require.Equal(t, config.AgentMTLSRequired, PolicyFrom(config.AgentControlConfig{}).Mode, "the 4.2 default")
	require.True(t, PolicyFrom(config.AgentControlConfig{}).RefusesLegacy())
	require.False(t, PolicyFrom(config.AgentControlConfig{MTLS: "preferred"}).RefusesLegacy(), "preferred stays selectable")

	header := http.Header{}
	PolicyFrom(config.AgentControlConfig{}).SetDeprecationHeaders(header)
	require.Equal(t, "true", header.Get("Deprecation"))
	require.Empty(t, header.Get("Sunset"), "no sunset unless configured")
	require.Equal(t, "<"+UpgradeGuideURL+`>; rel="deprecation"; type="text/html"`, header.Get("Link"))

	header = http.Header{}
	policy := PolicyFrom(config.AgentControlConfig{LegacySunset: "2027-03-31"})
	policy.SetDeprecationHeaders(header)
	require.Equal(t, "Wed, 31 Mar 2027 00:00:00 GMT", header.Get("Sunset"))
	require.Contains(t, header.Get("Link"), `rel="deprecation sunset"`)
	require.True(t, PolicyFrom(config.AgentControlConfig{LegacySunset: "never"}).Sunset.IsZero(), "an invalid date is ignored")
}

func TestMetrics(t *testing.T) {
	before := LegacyRequests("/test/metrics")
	CountLegacy("/test/metrics")
	CountLegacy("/test/metrics")
	CountRefused("/test/metrics")
	CountLegacy("")
	require.Equal(t, before+2, LegacyRequests("/test/metrics"))
	require.Positive(t, RefusedRequests("/test/metrics"))
	require.Positive(t, LegacyRequests(otherPath))

	SetMode(config.AgentMTLSPreferred)
	var body strings.Builder
	WritePrometheus(&body)
	text := body.String()
	require.Contains(t, text, "# TYPE anixops_agent_legacy_requests_total counter")
	require.Contains(t, text, `anixops_agent_legacy_requests_total{path="/test/metrics"}`)
	require.Contains(t, text, `anixops_agent_legacy_refused_total{path="/test/metrics"}`)
	require.Contains(t, text, `anixops_agent_mtls_mode{mode="preferred"} 1`)
	require.Contains(t, text, `anixops_agent_mtls_mode{mode="required"} 0`)

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

	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportAPIKeyStream, AgentVersion: "1.1.0", Identity: "api-key"})
	row, ok := loadRow(t, db, proxy7, model.AgentTransportAPIKeyStream)
	require.True(t, ok, "the first sighting is written at once")
	require.Equal(t, "1.1.0", row.AgentVersion)
	first := row.LastSeenAt

	// Within the interval: memory only.
	c.now = c.now.Add(30 * time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportAPIKeyStream})
	row, _ = loadRow(t, db, proxy7, model.AgentTransportAPIKeyStream)
	require.True(t, row.LastSeenAt.Equal(first), "throttled")
	snapshot := r.Snapshot()
	require.Len(t, snapshot, 1)
	require.True(t, snapshot[0].LastSeenAt.Equal(c.now), "memory holds the live value")
	require.Equal(t, "1.1.0", snapshot[0].AgentVersion, "an empty version keeps the known one")

	// A new version is written at once.
	c.now = c.now.Add(time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportAPIKeyStream, AgentVersion: "1.2.0"})
	row, _ = loadRow(t, db, proxy7, model.AgentTransportAPIKeyStream)
	require.Equal(t, "1.2.0", row.AgentVersion)
	require.True(t, row.LastSeenAt.Equal(c.now))
	require.True(t, row.FirstSeenAt.Equal(first), "first seen stays")

	// After the interval the next sighting is written.
	c.now = c.now.Add(10 * time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportAPIKeyStream})
	row, _ = loadRow(t, db, proxy7, model.AgentTransportAPIKeyStream)
	require.False(t, row.LastSeenAt.Equal(c.now), "still throttled")
	c.now = c.now.Add(PersistInterval)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportAPIKeyStream})
	row, _ = loadRow(t, db, proxy7, model.AgentTransportAPIKeyStream)
	require.True(t, row.LastSeenAt.Equal(c.now))

	// Flush writes what is pending.
	c.now = c.now.Add(5 * time.Second)
	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportAPIKeyStream})
	require.NoError(t, r.Flush(ctx))
	row, _ = loadRow(t, db, proxy7, model.AgentTransportAPIKeyStream)
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
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: "edge", ID: 1}, Transport: model.AgentTransportWebSocket})
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy}, Transport: model.AgentTransportWebSocket})
	r.Seen(ctx, Sighting{Node: proxy7})
	require.Len(t, r.Snapshot(), 2)

	// A nil recorder, and one without a database, record nothing and fail
	// nothing.
	var none *Recorder
	none.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportWebSocket})
	require.Nil(t, none.Snapshot())
	require.NoError(t, none.Flush(ctx))
	NewRecorder(nil).Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportWebSocket})
	NewRecorder(func() *gorm.DB { return nil }).Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportWebSocket})
}

func TestRecorderRetriesAFailedWrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), gormConfig)
	require.NoError(t, err)
	c := &clock{now: time.Now().UTC()}
	r := newTestRecorder(db, c)
	ctx := context.Background()
	r.Seen(ctx, Sighting{Node: proxy7, Transport: model.AgentTransportWebSocket})
	require.Error(t, r.Flush(ctx), "the table is missing: the sighting stays pending")
	migrate(t, db)
	require.NoError(t, r.Flush(ctx))
	_, ok := loadRow(t, db, proxy7, model.AgentTransportWebSocket)
	require.True(t, ok)
}

func TestDefaultRecorder(t *testing.T) {
	db := openSQLite(t)
	restore := SetDefault(newTestRecorder(db, &clock{now: time.Now().UTC()}))
	defer restore()
	Seen(context.Background(), Sighting{Node: proxy7, Transport: model.AgentTransportUniProxy})
	_, ok := loadRow(t, db, proxy7, model.AgentTransportUniProxy)
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
		// Node 1 moved to the mTLS stream; it still pulls UniProxy.
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportAPIKeyStream, AgentVersion: "1.1.0", FirstSeenAt: at(600), LastSeenAt: at(300)},
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0", FirstSeenAt: at(200), LastSeenAt: at(5)},
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportUniProxy, FirstSeenAt: at(600), LastSeenAt: at(1)},
		// Node 2 is a legacy agent: WebSocket and REST.
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportWebSocket, FirstSeenAt: at(600), LastSeenAt: at(3)},
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportHTTPLegacy, FirstSeenAt: at(600), LastSeenAt: at(2)},
		// Node 3 is third-party software on UniProxy only.
		{NodeKind: "proxy", NodeID: 3, Transport: model.AgentTransportUniProxy, FirstSeenAt: at(600), LastSeenAt: at(1)},
		// A deleted node's row is not listed.
		{NodeKind: "proxy", NodeID: 99, Transport: model.AgentTransportWebSocket, FirstSeenAt: at(600), LastSeenAt: at(1)},
	}).Error)
}

func TestInventory(t *testing.T) {
	db := openSQLite(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	seedInventory(t, db, now)
	policy := PolicyFrom(config.AgentControlConfig{LegacySunset: "2027-03-31"})

	inventory, err := Build(context.Background(), db, policy, Options{Now: now})
	require.NoError(t, err)
	require.Equal(t, config.AgentMTLSRequired, inventory.Mode, "the 4.2 default")
	require.NotNil(t, inventory.Sunset)
	require.Equal(t, UpgradeGuideURL, inventory.UpgradeGuide)
	counts := inventory.Summary
	counts.ReadyForRequired, counts.RequiredReasons, counts.RequiredBlockers = false, nil, nil
	require.Equal(t, Summary{Total: 5, MTLS: 1, Legacy: 2, ThirdParty: 1, Unseen: 1}, counts)
	require.False(t, inventory.Summary.ReadyForRequired)
	require.Len(t, inventory.Summary.RequiredBlockers, 2, "the disabled unseen node does not count")
	require.Equal(t, "proxy-2", inventory.Summary.RequiredBlockers[0].Node)
	require.Equal(t, model.AgentTransportHTTPLegacy, inventory.Summary.RequiredBlockers[0].Transport)
	require.Equal(t, "forward-1", inventory.Summary.RequiredBlockers[1].Node)
	require.Equal(t, model.AgentTransportCleanAgent, inventory.Summary.RequiredBlockers[1].Transport)
	byNode := map[string]NodeTransports{}
	for _, node := range inventory.Nodes {
		byNode[node.Node] = node
	}
	enrolled := byNode["proxy-1"]
	require.Equal(t, StatusMTLS, enrolled.Status, "third-party protocols do not decide")
	require.Equal(t, model.AgentTransportUniProxy, enrolled.Transport, "the latest sighting")
	require.Equal(t, "2.0.0", enrolled.AgentVersion)
	require.NotNil(t, enrolled.Certificate)
	require.Equal(t, "new", enrolled.Certificate.Serial)
	require.Equal(t, "spiffe://anixops/prod/agent/proxy-1", enrolled.Certificate.SPIFFEID)
	require.Len(t, enrolled.Transports, 3)
	require.True(t, enrolled.Transports[2].Legacy)

	legacy := byNode["proxy-2"]
	require.Equal(t, StatusLegacy, legacy.Status)
	require.Equal(t, model.AgentTransportHTTPLegacy, legacy.Transport)
	require.Equal(t, "1.1.0-node", legacy.AgentVersion, "the node row's version")
	require.Nil(t, legacy.Certificate, "a revoked certificate is no identity")

	require.Equal(t, StatusThirdParty, byNode["proxy-3"].Status)
	never := byNode["proxy-4"]
	require.Equal(t, StatusUnseen, never.Status)
	require.False(t, never.Enabled)
	require.Nil(t, never.LastSeenAt)
	require.NotNil(t, never.Transports)

	relay := byNode["forward-1"]
	require.Equal(t, StatusLegacy, relay.Status)
	require.Equal(t, model.AgentTransportCleanAgent, relay.Transport)
	require.Equal(t, "0.9", relay.AgentVersion)
	_, listed := byNode["proxy-99"]
	require.False(t, listed)

	// The checklist: legacy nodes only.
	inventory, err = Build(context.Background(), db, policy, Options{Now: now, LegacyOnly: true})
	require.NoError(t, err)
	require.Len(t, inventory.Nodes, 2)
	require.Equal(t, 2, inventory.Summary.Legacy)
	require.Len(t, inventory.Summary.RequiredBlockers, 2, "readiness covers every node, also with legacy_only")

	// The live overlay: node 2 enrolled a second ago, not written yet.
	live := newTestRecorder(nil, &clock{now: now})
	live.Seen(context.Background(), Sighting{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0"})
	live.Seen(context.Background(), Sighting{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 3}, Transport: model.AgentTransportUniProxy})
	inventory, err = Build(context.Background(), db, policy, Options{Now: now, Live: live, LegacyOnly: true})
	require.NoError(t, err)
	require.Len(t, inventory.Nodes, 1)
	require.Equal(t, "forward-1", inventory.Nodes[0].Node)

	// Without a sunset none is reported.
	inventory, err = Build(context.Background(), db, Policy{Mode: config.AgentMTLSRequired}, Options{})
	require.NoError(t, err)
	require.Nil(t, inventory.Sunset)
	for _, node := range inventory.Nodes {
		require.Nil(t, node.Session, "no sessions are read without a provider")
	}

	// The live sessions: each node's Agent Control stream session, with
	// how it authenticated and what it negotiated; WebSocket sessions are
	// not shown.
	notAfter := now.Add(48 * time.Hour)
	inventory, err = Build(context.Background(), db, policy, Options{Now: now, Sessions: func() []agentstreams.Session {
		return []agentstreams.Session{
			{
				Node: agentcontrol.AgentNode{Kind: "proxy", ID: 1}, Transport: agentstreams.TransportControlStream, SessionID: "session-1",
				AgentVersion: "2.0.0", ConnectedAt: now.Add(-time.Hour), LastSeen: now, Identity: "spiffe://anixops/prod/agent/proxy-1",
				Authentication: agentstreams.AuthenticationMTLS, NegotiatedCapabilities: []string{"config.v1", "reports.v1"},
				Certificate: &agentstreams.SessionCertificate{Serial: "new", NotAfter: notAfter, SPIFFEID: "spiffe://anixops/prod/agent/proxy-1"},
			},
			{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: agentstreams.TransportWebSocket, Identity: agentstreams.IdentityAPIKey},
			{
				Node: agentcontrol.AgentNode{Kind: "forward", ID: 1}, Transport: agentstreams.TransportControlStream, SessionID: "session-f",
				Identity: agentstreams.IdentityAPIKey, Authentication: agentstreams.AuthenticationAPIKey,
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
	require.Equal(t, agentstreams.AuthenticationAPIKey, forward.Authentication)
	require.Nil(t, forward.Certificate)
	require.NotNil(t, forward.NegotiatedCapabilities)
}

// TestRequiredReadiness: ready_for_required and its reasons from an
// inventory: enabled legacy nodes (recent or not) and enabled nodes that
// never enrolled block; mTLS, third-party, enrolled-but-unseen and
// disabled nodes do not.
func TestRequiredReadiness(t *testing.T) {
	db := openSQLite(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	inventory, err := Build(context.Background(), db, Policy{Mode: config.AgentMTLSRequired}, Options{Now: now})
	require.NoError(t, err)
	require.True(t, inventory.Summary.ReadyForRequired, "no node, nothing to refuse")
	require.Empty(t, inventory.Summary.RequiredReasons)
	require.NotNil(t, inventory.Summary.RequiredReasons, "an empty list, not null")
	require.NotNil(t, inventory.Summary.RequiredBlockers)

	require.NoError(t, db.Create(&[]model.Node{
		{ID: 1, Name: "mtls", APIKey: "k1", Status: model.NodeStatusOnline},
		{ID: 2, Name: "recent-legacy", APIKey: "k2", Status: model.NodeStatusOnline},
		{ID: 3, Name: "stale-legacy", APIKey: "k3", Status: model.NodeStatusOffline},
		{ID: 4, Name: "disabled-legacy", APIKey: "k4", Status: model.NodeStatusDisabled},
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
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportAPIKeyStream, FirstSeenAt: days(30), LastSeenAt: days(1)},
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportUniProxy, FirstSeenAt: days(30), LastSeenAt: days(0)},
		{NodeKind: "proxy", NodeID: 3, Transport: model.AgentTransportHTTPLegacy, FirstSeenAt: days(30), LastSeenAt: days(10)},
		{NodeKind: "proxy", NodeID: 4, Transport: model.AgentTransportWebSocket, FirstSeenAt: days(30), LastSeenAt: days(1)},
		{NodeKind: "proxy", NodeID: 5, Transport: model.AgentTransportUniProxy, FirstSeenAt: days(30), LastSeenAt: days(0)},
	}).Error)

	inventory, err = Build(context.Background(), db, Policy{Mode: config.AgentMTLSRequired}, Options{Now: now, LegacyOnly: true})
	require.NoError(t, err)
	summary := inventory.Summary
	require.False(t, summary.ReadyForRequired)
	blockers := map[string]RequiredBlocker{}
	for _, blocker := range summary.RequiredBlockers {
		blockers[blocker.Node] = blocker
	}
	require.Len(t, blockers, 4, "%+v", summary.RequiredBlockers)
	require.Equal(t, RequiredBlocker{Node: "proxy-2", Name: "recent-legacy", Reason: BlockerLegacy, Transport: model.AgentTransportAPIKeyStream,
		LastSeenAt: blockers["proxy-2"].LastSeenAt, Recent: true}, blockers["proxy-2"])
	require.True(t, blockers["proxy-2"].LastSeenAt.Equal(days(1)), "the legacy sighting's time, not the later UniProxy one")
	require.Equal(t, BlockerLegacy, blockers["proxy-3"].Reason)
	require.False(t, blockers["proxy-3"].Recent, "seen 10 days ago")
	require.Equal(t, BlockerNeverEnrolled, blockers["proxy-7"].Reason)
	require.Nil(t, blockers["proxy-7"].LastSeenAt)
	require.Equal(t, BlockerNeverEnrolled, blockers["forward-1"].Reason)
	legacy, recent, never := summary.RequiredBlockerCounts()
	require.Equal(t, [3]int{2, 1, 2}, [3]int{legacy, recent, never})
	require.Equal(t, []string{
		"2 enabled node(s) still on a legacy AnixOps Agent channel (1 seen within the last 7 days): upgrade their Agents and let them enroll",
		"2 enabled node(s) never enrolled (no agent certificate, never seen): install or enroll their Agents, or disable the nodes",
	}, summary.RequiredReasons)

	// Moving the nodes clears the blockers.
	require.NoError(t, db.Model(&model.Node{}).Where("id IN ?", []uint{2, 3, 7}).Update("status", model.NodeStatusDisabled).Error)
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 1).Update("enabled", false).Error)
	inventory, err = Build(context.Background(), db, Policy{Mode: config.AgentMTLSPreferred}, Options{Now: now})
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
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: model.AgentTransportWebSocket, AgentVersion: "1.1.1"})
	row, ok := loadRow(t, db, agentcontrol.AgentNode{Kind: "proxy", ID: 2}, model.AgentTransportWebSocket)
	require.True(t, ok)
	require.Equal(t, "1.1.1", row.AgentVersion)
	require.True(t, row.LastSeenAt.Equal(now))
	require.True(t, row.FirstSeenAt.Before(now), "the upsert keeps first_seen_at")
	c.now = now.Add(PersistInterval)
	r.Seen(ctx, Sighting{Node: agentcontrol.AgentNode{Kind: "proxy", ID: 2}, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0"})
	inventory, err := Build(ctx, db, Policy{Mode: config.AgentMTLSRequired}, Options{Now: c.now})
	require.NoError(t, err)
	require.Equal(t, 1, inventory.Summary.Legacy, "only the clean agent's forward node is left")
	require.Equal(t, 2, inventory.Summary.MTLS)
}

func TestLegacyHTTPModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openSQLite(t)
	restore := SetDefault(newTestRecorder(db, &clock{now: time.Now().UTC()}))
	defer restore()

	authenticate := func(forward bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			if c.GetHeader("X-API-Key") != "key" {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Set("node_id", uint(11))
			c.Set(agentws.ForwardNodeContextKey, forward)
			c.Next()
		}
	}
	serve := func(c *gin.Context) { c.Status(http.StatusNoContent) }

	for _, mode := range config.AgentMTLSModes {
		t.Run(mode, func(t *testing.T) {
			policy := PolicyFrom(config.AgentControlConfig{MTLS: mode})
			router := gin.New()
			path := "/api/v2/agent/heartbeat-" + mode
			router.POST(path, LegacyHTTP(policy, model.AgentTransportHTTPLegacy), authenticate(false), serve)
			router.GET("/api/v2/server/UniProxy/config-"+mode, ThirdPartyHTTP(model.AgentTransportUniProxy), authenticate(false), serve)
			router.POST("/api/v2/forward/agent/rules-"+mode, LegacyHTTP(policy, model.AgentTransportHTTPLegacy), authenticate(true), serve)

			request := func(method, target string) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest(method, target, nil)
				req.Header.Set("X-API-Key", "key")
				router.ServeHTTP(recorder, req)
				return recorder
			}
			before, refusedBefore := LegacyRequests(path), RefusedRequests(path)
			answer := request(http.MethodPost, path)
			switch mode {
			case config.AgentMTLSRequired:
				require.Equal(t, http.StatusForbidden, answer.Code)
				require.Contains(t, answer.Body.String(), `"code":"agent_mtls_required"`)
				require.Equal(t, "true", answer.Header().Get("Deprecation"), "the refusal says why")
				require.Equal(t, refusedBefore+1, RefusedRequests(path))
				require.Equal(t, before, LegacyRequests(path))
				_, ok := loadRow(t, db, agentcontrol.AgentNode{Kind: "proxy", ID: 11}, model.AgentTransportHTTPLegacy)
				require.False(t, ok, "a refused request is not a sighting")
			case config.AgentMTLSPreferred:
				require.Equal(t, http.StatusNoContent, answer.Code)
				require.Equal(t, "true", answer.Header().Get("Deprecation"))
				require.Contains(t, answer.Header().Get("Link"), UpgradeGuideURL)
				require.Equal(t, before+1, LegacyRequests(path))
			default:
				require.Equal(t, http.StatusNoContent, answer.Code)
				require.Empty(t, answer.Header().Get("Deprecation"), "optional and off are silent")
				require.Empty(t, answer.Header().Get("Link"))
				require.Equal(t, before+1, LegacyRequests(path), "counted in every mode")
			}
			if mode != config.AgentMTLSRequired {
				row, ok := loadRow(t, db, agentcontrol.AgentNode{Kind: "proxy", ID: 11}, model.AgentTransportHTTPLegacy)
				require.True(t, ok)
				require.Equal(t, "api-key", row.Identity)
				answer = request(http.MethodPost, "/api/v2/forward/agent/rules-"+mode)
				require.Equal(t, http.StatusNoContent, answer.Code)
				_, ok = loadRow(t, db, agentcontrol.AgentNode{Kind: "forward", ID: 11}, model.AgentTransportHTTPLegacy)
				require.True(t, ok, "the forward flag picks the node kind")
				require.NoError(t, db.Where("1 = 1").Delete(&model.AgentTransport{}).Error)
				SetDefault(newTestRecorder(db, &clock{now: time.Now().UTC()}))
			}

			// UniProxy is never signalled or refused, whatever the mode.
			answer = request(http.MethodGet, "/api/v2/server/UniProxy/config-"+mode)
			require.Equal(t, http.StatusNoContent, answer.Code)
			require.Empty(t, answer.Header().Get("Deprecation"))
			_, ok := loadRow(t, db, agentcontrol.AgentNode{Kind: "proxy", ID: 11}, model.AgentTransportUniProxy)
			require.True(t, ok)
		})
	}

	// An unauthenticated request is served (and answered by its handler)
	// but not recorded.
	router := gin.New()
	router.GET("/ws", LegacyHTTP(Policy{Mode: config.AgentMTLSPreferred}, model.AgentTransportWebSocket), authenticate(false), serve)
	router.GET("/plain", LegacyHTTP(Policy{Mode: config.AgentMTLSPreferred}, model.AgentTransportHTTPLegacy), func(c *gin.Context) {
		c.Set("node_id", "not-a-number")
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ws", nil))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Equal(t, "true", recorder.Header().Get("Deprecation"))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/plain", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	// WebSocket sessions are recorded by their handler, not here.
	recorder = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("X-API-Key", "key")
	router.ServeHTTP(recorder, req)
	_, ok := loadRow(t, db, agentcontrol.AgentNode{Kind: "proxy", ID: 11}, model.AgentTransportWebSocket)
	require.False(t, ok)
}

func TestContextNodeID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Zero(t, contextNodeID(c))
	for _, value := range []any{uint(5), uint32(5), uint64(5), 5} {
		c.Set("node_id", value)
		require.Equal(t, uint32(5), contextNodeID(c))
	}
	c.Set("node_id", -1)
	require.Zero(t, contextNodeID(c))
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

			inventory, err := Build(context.Background(), db, Policy{Mode: config.AgentMTLSRequired}, Options{Now: now})
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
			// and is disabled (not a blocker); revoked proxy-2 is on a legacy channel.
			blockers := map[string]string{}
			for _, blocker := range inventory.Summary.RequiredBlockers {
				blockers[blocker.Node] = blocker.Reason
			}
			require.Equal(t, BlockerLegacy, blockers["proxy-2"])
			require.NotContains(t, blockers, "proxy-1")
		})
	}
}

// TestInventoryNodeFilter: Options.Nodes narrows the list, not the summary.
func TestInventoryNodeFilter(t *testing.T) {
	db := openSQLite(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	seedInventory(t, db, now)
	policy := PolicyFrom(config.AgentControlConfig{})
	all, err := Build(context.Background(), db, policy, Options{Now: now})
	require.NoError(t, err)

	one, err := Build(context.Background(), db, policy, Options{Now: now, Nodes: []agentcontrol.AgentNode{{Kind: "proxy", ID: 2}}})
	require.NoError(t, err)
	require.Len(t, one.Nodes, 1)
	require.Equal(t, "proxy-2", one.Nodes[0].Node)
	require.Equal(t, Summary{Total: 1, Legacy: 1}, Summary{Total: one.Summary.Total, MTLS: one.Summary.MTLS, Legacy: one.Summary.Legacy,
		ThirdParty: one.Summary.ThirdParty, Unseen: one.Summary.Unseen}, "the counts follow the list")
	require.Equal(t, all.Summary.RequiredBlockers, one.Summary.RequiredBlockers, "readiness covers every node, also for one")
	require.Equal(t, all.Summary.ReadyForRequired, one.Summary.ReadyForRequired)
	require.Equal(t, all.Summary.RequiredReasons, one.Summary.RequiredReasons)

	several, err := Build(context.Background(), db, policy, Options{Now: now, LegacyOnly: true, Nodes: []agentcontrol.AgentNode{
		{Kind: "forward", ID: 1}, {Kind: "proxy", ID: 1}, {Kind: "proxy", ID: 77},
	}})
	require.NoError(t, err)
	require.Len(t, several.Nodes, 1, "legacy_only and node both narrow; an unknown node is not listed")
	require.Equal(t, "forward-1", several.Nodes[0].Node)
}

// TestConnectionOf: the connection type from a live session, else from the
// transports seen within the window.
func TestConnectionOf(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	seen := func(transport string, d time.Duration) TransportSeen {
		return TransportSeen{
			Transport: transport, Legacy: model.AgentTransportLegacy(transport),
			ThirdParty: model.AgentTransportThirdParty(transport), LastSeenAt: ago(d),
		}
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
		{name: "an API key stream session", node: NodeTransports{Session: live(agentstreams.AuthenticationAPIKey)}, sessionsRead: true, want: ConnectionAPIKeyStream, transport: model.AgentTransportAPIKeyStream},
		{name: "a session without an authentication is an API key one", node: NodeTransports{Session: live("")}, sessionsRead: true, want: ConnectionAPIKeyStream, transport: model.AgentTransportAPIKeyStream},
		{name: "the session beats a newer legacy sighting", node: NodeTransports{Session: live(agentstreams.AuthenticationMTLS), Transports: []TransportSeen{seen(model.AgentTransportHTTPLegacy, time.Second)}}, sessionsRead: true, want: ConnectionMTLSStream, transport: model.AgentTransportMTLSStream},
		{name: "a legacy REST sighting", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportHTTPLegacy, time.Minute)}}, sessionsRead: true, want: ConnectionLegacy, transport: model.AgentTransportHTTPLegacy},
		{name: "a legacy WebSocket sighting", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportWebSocket, time.Minute)}}, sessionsRead: true, want: ConnectionLegacy, transport: model.AgentTransportWebSocket},
		{name: "a clean agent", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportCleanAgent, 2*time.Minute)}}, sessionsRead: true, want: ConnectionLegacy, transport: model.AgentTransportCleanAgent},
		{name: "the window is inclusive", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportWebSocket, ConnectionWindow)}}, sessionsRead: true, want: ConnectionLegacy, transport: model.AgentTransportWebSocket},
		{name: "a stale sighting is offline", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportWebSocket, ConnectionWindow+time.Second)}}, sessionsRead: true, want: ConnectionOffline},
		{name: "never seen", node: NodeTransports{}, sessionsRead: true, want: ConnectionOffline},
		{name: "a closed stream is not connected when sessions were read", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportMTLSStream, time.Minute)}}, sessionsRead: true, want: ConnectionOffline},
		{name: "a stream sighting counts without sessions (the CLI)", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportMTLSStream, time.Minute)}}, want: ConnectionMTLSStream, transport: model.AgentTransportMTLSStream},
		{name: "an API key stream sighting without sessions", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportAPIKeyStream, time.Minute)}}, want: ConnectionAPIKeyStream, transport: model.AgentTransportAPIKeyStream},
		{name: "UniProxy only is third party", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportUniProxy, time.Minute)}}, sessionsRead: true, want: ConnectionThirdParty, transport: model.AgentTransportUniProxy},
		{name: "an AnixOps channel beats a newer UniProxy sighting", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportUniProxy, time.Second), seen(model.AgentTransportHTTPLegacy, time.Minute)}}, sessionsRead: true, want: ConnectionLegacy, transport: model.AgentTransportHTTPLegacy},
		{name: "a stale UniProxy sighting is offline", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportV2boardGRPC, time.Hour)}}, sessionsRead: true, want: ConnectionOffline},
		{name: "the newest recent legacy transport is named", node: NodeTransports{Transports: []TransportSeen{seen(model.AgentTransportHTTPLegacy, time.Minute), seen(model.AgentTransportWebSocket, 2*time.Minute)}}, sessionsRead: true, want: ConnectionLegacy, transport: model.AgentTransportHTTPLegacy},
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
	policy := PolicyFrom(config.AgentControlConfig{})

	// The CLI reads no sessions: node 1's stream sighting (five minutes old,
	// inside the window) decides, over its newer UniProxy pulls.
	inventory, err := Build(context.Background(), db, policy, Options{Now: now})
	require.NoError(t, err)
	byNode := inventoryByNode(inventory)
	require.Equal(t, ConnectionMTLSStream, byNode["proxy-1"].Connection.Type)
	require.Equal(t, ConnectionLegacy, byNode["proxy-2"].Connection.Type)
	require.Equal(t, model.AgentTransportHTTPLegacy, byNode["proxy-2"].Connection.Transport, "the newest legacy transport")
	require.Equal(t, ConnectionThirdParty, byNode["proxy-3"].Connection.Type)
	require.Equal(t, ConnectionOffline, byNode["proxy-4"].Connection.Type)
	require.Equal(t, ConnectionLegacy, byNode["forward-1"].Connection.Type, "a clean agent seen two minutes ago")

	// The API reads this process's sessions: with none, proxy-1's closed
	// stream is no connection (it still pulls UniProxy); a live API key
	// stream on the forward node beats its clean agent.
	inventory, err = Build(context.Background(), db, policy, Options{Now: now, Sessions: func() []agentstreams.Session {
		return []agentstreams.Session{{
			Node: agentcontrol.AgentNode{Kind: "forward", ID: 1}, Transport: agentstreams.TransportControlStream,
			Identity: agentstreams.IdentityAPIKey, Authentication: agentstreams.AuthenticationAPIKey, LastSeen: now.Add(-time.Second),
		}}
	}})
	require.NoError(t, err)
	byNode = inventoryByNode(inventory)
	require.Equal(t, ConnectionThirdParty, byNode["proxy-1"].Connection.Type)
	require.Equal(t, ConnectionAPIKeyStream, byNode["forward-1"].Connection.Type)
	require.Equal(t, model.AgentTransportAPIKeyStream, byNode["forward-1"].Connection.Transport)
	require.True(t, byNode["forward-1"].Connection.LastSeenAt.Equal(now.Add(-time.Second)))
	require.Equal(t, ConnectionOffline, byNode["proxy-4"].Connection.Type)

	// An mTLS session.
	inventory, err = Build(context.Background(), db, policy, Options{Now: now, Sessions: func() []agentstreams.Session {
		return []agentstreams.Session{{
			Node: agentcontrol.AgentNode{Kind: "proxy", ID: 1}, Transport: agentstreams.TransportControlStream,
			Identity: "spiffe://anixops/prod/agent/proxy-1", Authentication: agentstreams.AuthenticationMTLS, LastSeen: now,
		}}
	}})
	require.NoError(t, err)
	require.Equal(t, ConnectionMTLSStream, inventoryByNode(inventory)["proxy-1"].Connection.Type)
}
