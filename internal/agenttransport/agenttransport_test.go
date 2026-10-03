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
	require.Equal(t, config.AgentMTLSPreferred, PolicyFrom(config.AgentControlConfig{}).Mode, "4.1 default")

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
	require.Equal(t, config.AgentMTLSPreferred, inventory.Mode)
	require.NotNil(t, inventory.Sunset)
	require.Equal(t, UpgradeGuideURL, inventory.UpgradeGuide)
	require.Equal(t, Summary{Total: 5, MTLS: 1, Legacy: 2, ThirdParty: 1, Unseen: 1}, inventory.Summary)
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
