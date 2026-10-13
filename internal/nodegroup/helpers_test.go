package nodegroup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSN enables the PostgreSQL runs; each run gets a throwaway schema.
const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

// clock is a settable time source shared by the PKI and the service.
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

// fixture is a node group service with the agent PKI over a fresh
// database holding the kernel schema, the node tables and two subscription
// groups.
type fixture struct {
	db            *gorm.DB
	clock         *clock
	kek           string
	pki           *agentpki.Service
	svc           *Service
	subscriptions []model.SubscriptionGroup
	admin         Actor
}

// forEachDatabase runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN is
// set, on PostgreSQL.
func forEachDatabase(t *testing.T, body func(t *testing.T, f *fixture)) {
	t.Run("sqlite", func(t *testing.T) {
		body(t, newFixture(t, openSQLite(t)))
	})
	t.Run("postgres", func(t *testing.T) {
		body(t, newFixture(t, openPostgres(t)))
	})
}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// openPostgres creates a throwaway schema in the test database and returns
// a connection whose search_path is that schema.
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
	schema := "node_group_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func newFixture(t *testing.T, db *gorm.DB) *fixture {
	t.Helper()
	models := append([]any{
		&model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.ForwardNode{}, &model.OperationLog{},
	}, model.KernelModels()...)
	require.NoError(t, db.AutoMigrate(models...))
	require.NoError(t, nodesecrets.MigrateForTest(db))
	clk := &clock{now: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)}
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "test", KEK: raw, Now: clk.Now})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(t.Context()))
	pki, err := agentpki.New(agentpki.Options{DB: db, Authority: authority, Now: clk.Now})
	require.NoError(t, err)
	kek := base64.StdEncoding.EncodeToString(raw)
	f := &fixture{db: db, clock: clk, kek: kek, pki: pki, admin: Actor{UserID: 7, Name: "root@example.com", IP: "192.0.2.10"}}
	f.svc = &Service{
		DB: db, KEK: func() string { return f.kek }, Now: clk.Now, PKI: pki,
		Forward: &kernelforward.Service{DB: db, Cluster: func() string { return "test" }, AnixOps: func() bool { return false }, Now: clk.Now},
	}
	for _, name := range []string{"Standard", "Premium"} {
		group := model.SubscriptionGroup{Name: name, Enable: 1}
		require.NoError(t, db.Create(&group).Error)
		f.subscriptions = append(f.subscriptions, group)
	}
	return f
}

// csr returns a new PKCS#10 request whose subject Control ignores.
func csr(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "proxy-999"}}, key)
	require.NoError(t, err)
	return der
}

// fingerprint is a machine fingerprint for seed.
func fingerprint(seed string) string {
	return strings.Repeat(hex.EncodeToString([]byte(seed))[:2], 32)
}

// createGroup creates a group from input and returns it with its token.
func (f *fixture) createGroup(t *testing.T, input GroupInput) (GroupView, string) {
	t.Helper()
	group, err := f.svc.Create(t.Context(), input, f.admin)
	require.NoError(t, err)
	revealed, err := f.svc.RevealToken(t.Context(), group.ID, f.admin)
	require.NoError(t, err)
	return group, revealed.Token
}

// join joins a machine with token.
func (f *fixture) join(t *testing.T, request JoinRequest) (Joined, error) {
	t.Helper()
	if request.CSRDER == nil {
		request.CSRDER = csr(t)
	}
	if request.AgentVersion == "" {
		request.AgentVersion = "v5.0.0"
	}
	if request.InstanceID == "" {
		request.InstanceID = "instance"
	}
	if request.PeerAddress == "" {
		request.PeerAddress = "198.51.100.200"
	}
	return f.svc.Join(t.Context(), request)
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func (f *fixture) audits(t *testing.T, action string) []model.OperationLog {
	t.Helper()
	var rows []model.OperationLog
	require.NoError(t, f.db.Where("action = ?", action).Order("id").Find(&rows).Error)
	return rows
}
