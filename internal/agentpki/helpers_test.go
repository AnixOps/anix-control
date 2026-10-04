package agentpki_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSN enables the PostgreSQL runs; each run gets a throwaway schema.
const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

const testCluster = "test"

var testModels = []any{
	&model.ServiceCA{}, &model.AgentEnrollment{}, &model.AgentCertificate{}, &model.Node{}, &model.ForwardNode{},
	&model.OperationLog{}, &model.ForwardLinkCA{}, &model.ForwardLinkCertificate{}, &model.KernelForwardNode{},
}

// clock is a settable time source shared by the authority and the service.
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

// fixture is an agent PKI over a fresh database with one proxy node and one
// forward node of the same id, so kind confusion would show.
type fixture struct {
	db           *gorm.DB
	clock        *clock
	authority    *modulepki.Authority
	link         *agentpki.LinkAuthority
	pki          *agentpki.Service
	proxy        model.Node
	proxyKey     string
	forward      model.ForwardNode
	forwardToken string
}

// forEachDatabase runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN is
// set, on PostgreSQL.
func forEachDatabase(t *testing.T, body func(t *testing.T, f *fixture)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		body(t, newFixture(t, db))
	})
	t.Run("postgres", func(t *testing.T) {
		body(t, newFixture(t, openPostgres(t)))
	})
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
	schema := "agent_pki_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// openSQLiteForConfig opens an empty in-memory database.
func openSQLiteForConfig(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func newFixture(t *testing.T, db *gorm.DB) *fixture {
	t.Helper()
	require.NoError(t, db.AutoMigrate(testModels...))
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	now := &clock{now: time.Now().UTC().Truncate(time.Second)}
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: testCluster, KEK: kek, Now: now.Now})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(t.Context()))
	link, err := agentpki.NewLinkAuthority(agentpki.LinkAuthorityOptions{DB: db, Cluster: testCluster, KEK: kek, Now: now.Now})
	require.NoError(t, err)
	require.NoError(t, link.Ensure(t.Context()))
	pki, err := agentpki.New(agentpki.Options{DB: db, Authority: authority, Now: now.Now, Link: link})
	require.NoError(t, err)

	f := &fixture{db: db, clock: now, authority: authority, link: link, pki: pki, proxyKey: randomSecret(t), forwardToken: randomSecret(t)}
	f.proxy = model.Node{Name: "proxy", Host: "198.51.100.10", APIKey: f.proxyKey, APIKeyHash: sha256Hex(f.proxyKey), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&f.proxy).Error)
	f.forward = model.ForwardNode{ID: f.proxy.ID, Name: "forward", Host: "198.51.100.20", Port: 8443, APIToken: f.forwardToken, Enabled: true}
	require.NoError(t, db.Create(&f.forward).Error)
	return f
}

func randomSecret(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 24)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return hex.EncodeToString(raw)
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// newCSR returns a throwaway key and a CSR whose subject and SANs claim
// another node, which the kernel must ignore.
func newCSR(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	claimed, err := url.Parse("spiffe://anixops/" + testCluster + "/agent/proxy-999")
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "proxy-999", Organization: []string{"attacker"}},
		DNSNames: []string{"control.example"},
		URIs:     []*url.URL{claimed},
	}, key)
	require.NoError(t, err)
	return key, der
}
