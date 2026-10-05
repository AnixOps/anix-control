package kernelalerts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSN enables the PostgreSQL runs; each gets a throwaway schema.
const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

var start = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func migrate(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&model.KernelAlert{}, &model.AgentCertificate{}, &model.ForwardLinkCertificate{}, &model.ModuleCertificate{},
		&model.ServiceCA{}, &model.ForwardLinkCA{}, &model.Node{}, &model.ForwardNode{},
		&model.NodeSecretSplit{}, &model.IdentityAuthority{}, &model.IdentityCutoverEvent{},
	))
}

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

// openPostgres creates a throwaway schema in the test database and returns a
// migrated connection whose search_path is that schema.
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
	schema := "kernel_alerts_" + hex.EncodeToString(suffix)
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

// forEachDatabase runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN is
// set, on PostgreSQL.
func forEachDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { body(t, openSQLite(t)) })
	t.Run("postgres", func(t *testing.T) { body(t, openPostgres(t)) })
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
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// recorder is a Notifier that keeps the digests it was given.
type recorder struct {
	mu      sync.Mutex
	digests []Notification
	err     error
}

func (r *recorder) Notify(_ context.Context, notification Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.digests = append(r.digests, notification)
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.digests)
}

func (r *recorder) last() Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.digests[len(r.digests)-1]
}

func defaultSettings() config.AlertSettings {
	return config.AlertSettings{
		Enabled: true, CheckInterval: 15 * time.Minute, LeafExpiryDays: 14, CAExpiryDays: 60,
		RenotifyInterval: 24 * time.Hour, PhaseStuckAfter: 72 * time.Hour,
	}
}

func hours(n int) time.Duration { return time.Duration(n) * time.Hour }

// addAgentCert records an Agent certificate that lives lifetime and was
// issued issuedAgo before start.
func addAgentCert(t *testing.T, db *gorm.DB, serial, kind string, nodeID uint, lifetime, issuedAgo time.Duration) {
	t.Helper()
	issued := start.Add(-issuedAgo)
	require.NoError(t, db.Create(&model.AgentCertificate{
		Serial: serial, NodeKind: kind, NodeID: nodeID, Cluster: "prod", EnrollmentID: "enr-" + serial, IssuerKeyID: "k",
		NotAfter: issued.Add(lifetime), CreatedAt: issued,
	}).Error)
}

func addProxyNode(t *testing.T, db *gorm.DB, name string, status model.NodeStatus) model.Node {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	node := model.Node{Name: name, Host: "198.51.100.10", APIKey: "key-" + name, APIKeyHash: hex.EncodeToString(sum[:]), Status: status}
	require.NoError(t, db.Create(&node).Error)
	return node
}

func addForwardNode(t *testing.T, db *gorm.DB, name string, enabled bool) model.ForwardNode {
	t.Helper()
	node := model.ForwardNode{Name: name, Host: "198.51.100.20", Port: 8443, APIToken: "t-" + name, Enabled: true}
	require.NoError(t, db.Create(&node).Error)
	if !enabled {
		require.NoError(t, db.Model(&node).Update("enabled", false).Error)
	}
	return node
}

func scan(t *testing.T, db *gorm.DB, settings config.AlertSettings, now time.Time) ScanResult {
	t.Helper()
	result, err := (&Scanner{DB: db, Settings: settings, Now: func() time.Time { return now }}).Scan(t.Context())
	require.NoError(t, err)
	require.Empty(t, result.Unevaluated)
	return result
}

func kinds(result ScanResult) map[string]Finding {
	out := map[string]Finding{}
	for _, finding := range result.Findings {
		out[finding.Key()] = finding
	}
	return out
}

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func toJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// openBare is a SQLite database without any table.
func openBare(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
