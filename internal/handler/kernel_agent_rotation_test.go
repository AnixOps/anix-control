package handler

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const rotationPostgresDSN = "ANIX_TEST_POSTGRES_DSN"

// rotationFixture is the agent PKI, one proxy node, one forward node and
// the administrators the rotation route tells apart.
type rotationFixture struct {
	db      *gorm.DB
	pki     *agentpki.Service
	proxy   model.Node
	forward model.ForwardNode
	apiKey  string
	users   map[string]uint
}

// forEachRotationDatabase runs body on SQLite and, when
// ANIX_TEST_POSTGRES_DSN is set, on PostgreSQL (a throwaway schema).
func forEachRotationDatabase(t *testing.T, body func(t *testing.T, f *rotationFixture)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		body(t, newRotationFixture(t, db))
	})
	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv(rotationPostgresDSN))
		if base == "" {
			t.Skip(rotationPostgresDSN + " is not set")
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
		schema := "handler_rotation_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		body(t, newRotationFixture(t, db))
	})
}

func newRotationFixture(t *testing.T, db *gorm.DB) *rotationFixture {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.AgentEnrollment{}, &model.AgentCertificate{}, &model.Node{},
		&model.ForwardNode{}, &model.OperationLog{}, &model.User{}))
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "prod", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(t.Context()))
	pki, err := agentpki.New(agentpki.Options{DB: db, Authority: authority})
	require.NoError(t, err)

	raw := make([]byte, 24)
	_, err = rand.Read(raw)
	require.NoError(t, err)
	apiKey := hex.EncodeToString(raw)
	f := &rotationFixture{db: db, pki: pki, apiKey: apiKey, users: map[string]uint{}}
	f.proxy = model.Node{Name: "proxy", APIKey: apiKey, APIKeyHash: hashForTest(apiKey), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&f.proxy).Error)
	f.forward = model.ForwardNode{ID: f.proxy.ID, Name: "forward", Host: "198.51.100.20", Port: 8443, APIToken: "forward-token-" + apiKey, Enabled: true}
	require.NoError(t, db.Create(&f.forward).Error)
	for name, user := range map[string]model.User{
		"super":  {IsAdmin: 1},
		"staff":  {IsAdmin: 1, IsStaff: 1},
		"banned": {IsAdmin: 1, Banned: 1},
		"user":   {},
	} {
		user.Email = name + "@example.com"
		user.Token = "token-" + name
		user.UUID = "uuid-" + name
		require.NoError(t, db.Create(&user).Error)
		f.users[name] = user.ID
	}
	return f
}

func hashForTest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (f *rotationFixture) router(actor string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if id, ok := f.users[actor]; ok {
			c.Set("user_id", id)
		}
		c.Set("email", actor+"@example.com")
	})
	agents := &AgentPKIHandler{pki: func() (*agentpki.Service, error) { return f.pki, nil }, db: func() *gorm.DB { return f.db }}
	router.POST("/rotate-credentials", agents.RotateCredentials)
	return router
}

func (f *rotationFixture) proxyName() string {
	return "proxy-" + strconv.FormatUint(uint64(f.proxy.ID), 10)
}

func (f *rotationFixture) forwardName() string {
	return "forward-" + strconv.FormatUint(uint64(f.forward.ID), 10)
}

type rotationAnswer struct {
	Data struct {
		Node    string `json:"node"`
		Revoked struct {
			Certificates     int64 `json:"certificates"`
			Enrollments      int64 `json:"enrollments"`
			LinkCertificates int64 `json:"link_certificates"`
		} `json:"revoked"`
		APIKeyRotated bool           `json:"api_key_rotated"`
		Enrollment    map[string]any `json:"enrollment"`
		ExpiresAt     time.Time      `json:"expires_at"`
		Credential    string         `json:"credential"`
	} `json:"data"`
}

func (f *rotationFixture) enrollCertificate(t *testing.T, bootstrap agentpki.Bootstrap) (agentpki.Issued, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	require.NoError(t, err)
	issued, err := f.pki.Enroll(t.Context(), agentpki.EnrollRequest{Bootstrap: bootstrap, CSRDER: csr})
	if err != nil {
		require.Failf(t, "enrollment failed", "%v", err)
	}
	leaf, err := x509.ParseCertificate(issued.CertificateDER)
	require.NoError(t, err)
	return issued, leaf
}

func (f *rotationFixture) proxyBootstrap(secret string) agentpki.Bootstrap {
	return agentpki.Bootstrap{
		Method: model.AgentEnrollmentMethodNodeAPIKey, Secret: secret,
		Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(f.proxy.ID)},
	}
}

func TestRotateCredentialsNeedsASuperAdministrator(t *testing.T) {
	forEachRotationDatabase(t, func(t *testing.T, f *rotationFixture) {
		issued, _ := f.enrollCertificate(t, f.proxyBootstrap(f.apiKey))
		body := `{"node":"` + f.proxyName() + `"}`
		for actor, code := range map[string]int{"staff": http.StatusForbidden, "banned": http.StatusForbidden, "user": http.StatusForbidden, "nobody": http.StatusForbidden} {
			response := serveModuleRequest(f.router(actor), http.MethodPost, "/rotate-credentials", body)
			require.Equal(t, code, response.Code, actor)
			assert.Contains(t, response.Body.String(), "super_admin_required", actor)
		}
		// Nothing changed.
		_, _, err := f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.NoError(t, err)
		var audits int64
		require.NoError(t, f.db.Model(&model.OperationLog{}).Where("action = ?", agentpki.AuditActionCredentialsRotate).Count(&audits).Error)
		assert.Zero(t, audits)

		// A demoted administrator loses the right at once.
		require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", f.users["super"]).Update("is_admin", 0).Error)
		response := serveModuleRequest(f.router("super"), http.MethodPost, "/rotate-credentials", body)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})
}

func TestRotateCredentialsEndToEnd(t *testing.T) {
	forEachRotationDatabase(t, func(t *testing.T, f *rotationFixture) {
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		issued, leaf := f.enrollCertificate(t, f.proxyBootstrap(f.apiKey))
		router := f.router("super")
		response := serveModuleRequest(router, http.MethodPost, "/rotate-credentials",
			`{"node":"`+f.proxyName()+`","ttl_seconds":1800,"reason":"  laptop stolen  "}`)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		var answer rotationAnswer
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer))
		assert.Equal(t, f.proxyName(), answer.Data.Node)
		assert.True(t, strings.HasPrefix(answer.Data.Credential, agentcontrol.EnrollmentCredentialPrefix))
		assert.EqualValues(t, 1, answer.Data.Revoked.Certificates)
		assert.EqualValues(t, 1, answer.Data.Revoked.Enrollments)
		assert.Zero(t, answer.Data.Revoked.LinkCertificates)
		assert.False(t, answer.Data.APIKeyRotated)
		assert.WithinDuration(t, time.Now().Add(30*time.Minute), answer.Data.ExpiresAt, time.Minute)
		assert.NotContains(t, answer.Data.Enrollment, "credential_hash")
		assert.EqualValues(t, f.users["super"], answer.Data.Enrollment["created_by"])
		assert.NotContains(t, response.Body.String(), "api_key\"")

		// The old identity is revoked on every path; the new credential
		// enrolls and the old API key still works (not rotated).
		_, _, err := f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
		require.NoError(t, err)
		_, err = f.pki.Renew(t.Context(), leaf, csr)
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)
		fresh, _ := f.enrollCertificate(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: answer.Data.Credential})
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{fresh.CertificateDER})
		require.NoError(t, err)
		_, err = nodesecrets.NodeByAPIKey(f.db, f.apiKey, true)
		require.NoError(t, err, "the API key is not rotated unless asked")

		// Audit entries and logs carry the reason and no secret.
		var entries []model.OperationLog
		require.NoError(t, f.db.Where("action = ?", agentpki.AuditActionCredentialsRotate).Find(&entries).Error)
		require.Len(t, entries, 1)
		assert.Equal(t, "super@example.com", entries[0].Username)
		assert.Contains(t, entries[0].Content, `"reason":"laptop stolen"`)
		var all []model.OperationLog
		require.NoError(t, f.db.Find(&all).Error)
		for _, entry := range all {
			for _, secret := range []string{answer.Data.Credential, f.apiKey} {
				assert.NotContains(t, entry.Content, secret)
			}
		}
		for _, secret := range []string{answer.Data.Credential, f.apiKey} {
			assert.NotContains(t, logs.String(), secret)
		}
	})
}

func TestRotateCredentialsReplacesTheAPIKeyOnRequest(t *testing.T) {
	forEachRotationDatabase(t, func(t *testing.T, f *rotationFixture) {
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		issued, _ := f.enrollCertificate(t, f.proxyBootstrap(f.apiKey))
		response := serveModuleRequest(f.router("super"), http.MethodPost, "/rotate-credentials",
			`{"node":"`+f.proxyName()+`","rotate_api_key":true}`)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		var answer rotationAnswer
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer))
		assert.True(t, answer.Data.APIKeyRotated)

		_, err := nodesecrets.NodeByAPIKey(f.db, f.apiKey, true)
		require.Error(t, err, "the old API key stops working")
		_, err = f.pki.Enroll(t.Context(), agentpki.EnrollRequest{Bootstrap: f.proxyBootstrap(f.apiKey), CSRDER: []byte("x")})
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)

		var node model.Node
		require.NoError(t, f.db.First(&node, f.proxy.ID).Error)
		newKey := nodesecrets.NodeAPIKey(f.db, &node)
		require.False(t, nodesecrets.Unusable(newKey))
		require.NotEqual(t, f.apiKey, newKey)
		found, err := nodesecrets.NodeByAPIKey(f.db, newKey, true)
		require.NoError(t, err)
		assert.Equal(t, f.proxy.ID, found.ID)
		// The new key is read through the audited credentials route, never
		// from this answer, the audit log or the logs.
		assert.NotContains(t, response.Body.String(), newKey)
		var all []model.OperationLog
		require.NoError(t, f.db.Order("id").Find(&all).Error)
		for _, entry := range all {
			assert.NotContains(t, entry.Content, newKey)
			assert.NotContains(t, entry.Content, f.apiKey)
		}
		assert.NotContains(t, logs.String(), newKey)
		assert.Contains(t, all[len(all)-1].Content, `"api_key_rotated":true`)

		fresh, _ := f.enrollCertificate(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: answer.Data.Credential})
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{fresh.CertificateDER})
		require.NoError(t, err)
	})
}

func TestRotateCredentialsValidation(t *testing.T) {
	forEachRotationDatabase(t, func(t *testing.T, f *rotationFixture) {
		router := f.router("super")
		long := strings.Repeat("a", 201)
		cases := []struct {
			name string
			body string
			code int
			want string
		}{
			{"missing node", `{}`, http.StatusBadRequest, "invalid_request"},
			{"not json", `nope`, http.StatusBadRequest, "invalid_request"},
			{"module node", `{"node":"module-1"}`, http.StatusBadRequest, "node must be proxy-"},
			{"unknown proxy", `{"node":"proxy-424242"}`, http.StatusNotFound, "node_not_found"},
			{"unknown forward", `{"node":"forward-424242"}`, http.StatusNotFound, "node_not_found"},
			{"ttl too short", `{"node":"` + f.proxyName() + `","ttl_seconds":59}`, http.StatusBadRequest, "ttl_seconds"},
			{"ttl too long", `{"node":"` + f.proxyName() + `","ttl_seconds":604801}`, http.StatusBadRequest, "ttl_seconds"},
			{"ttl negative", `{"node":"` + f.proxyName() + `","ttl_seconds":-5}`, http.StatusBadRequest, "ttl_seconds"},
			{"ttl text", `{"node":"` + f.proxyName() + `","ttl_seconds":"x"}`, http.StatusBadRequest, "invalid_request"},
			{"reason too long", `{"node":"` + f.proxyName() + `","reason":"` + long + `"}`, http.StatusBadRequest, "reason"},
			{"forward token rotation", `{"node":"` + f.forwardName() + `","rotate_api_key":true}`, http.StatusBadRequest, "rotate_api_key applies to proxy nodes only"},
			{"forward", `{"node":"` + f.forwardName() + `"}`, http.StatusCreated, "credential"},
			{"longest ttl", `{"node":"` + f.proxyName() + `","ttl_seconds":604800}`, http.StatusCreated, "credential"},
		}
		for _, tc := range cases {
			response := serveModuleRequest(router, http.MethodPost, "/rotate-credentials", tc.body)
			assert.Equal(t, tc.code, response.Code, tc.name+": "+response.Body.String())
			assert.Contains(t, response.Body.String(), tc.want, tc.name)
		}
		// The forward node's token was not touched by the refused request.
		var forward model.ForwardNode
		require.NoError(t, f.db.First(&forward, f.forward.ID).Error)
		assert.True(t, strings.HasPrefix(forward.APIToken, "forward-token-"))
	})
}

func TestRotateCredentialsDisabledAndOfflineNodes(t *testing.T) {
	forEachRotationDatabase(t, func(t *testing.T, f *rotationFixture) {
		router := f.router("super")
		// An offline node (no heartbeat) is rotated like any other: nothing
		// is dialled.
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).Update("status", model.NodeStatusOffline).Error)
		response := serveModuleRequest(router, http.MethodPost, "/rotate-credentials", `{"node":"`+f.proxyName()+`"}`)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		// A repeat voids the first credential and issues the second.
		var first, second rotationAnswer
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &first))
		response = serveModuleRequest(router, http.MethodPost, "/rotate-credentials", `{"node":"`+f.proxyName()+`"}`)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &second))
		assert.NotEqual(t, first.Data.Credential, second.Data.Credential)
		assert.EqualValues(t, 1, second.Data.Revoked.Enrollments)
		_, err := f.pki.Enroll(t.Context(), agentpki.EnrollRequest{
			Bootstrap: agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: first.Data.Credential}, CSRDER: []byte("x"),
		})
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		f.enrollCertificate(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: second.Data.Credential})

		// A disabled node is refused with its own code, and nothing changes.
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).Update("status", model.NodeStatusDisabled).Error)
		response = serveModuleRequest(router, http.MethodPost, "/rotate-credentials", `{"node":"`+f.proxyName()+`"}`)
		assert.Equal(t, http.StatusConflict, response.Code)
		assert.Contains(t, response.Body.String(), "node_disabled")
		require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", f.forward.ID).Update("enabled", false).Error)
		response = serveModuleRequest(router, http.MethodPost, "/rotate-credentials", `{"node":"`+f.forwardName()+`"}`)
		assert.Equal(t, http.StatusConflict, response.Code)
	})
}

func TestRotateCredentialsWithoutBuiltinPKI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	forEachRotationDatabase(t, func(t *testing.T, f *rotationFixture) {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("user_id", f.users["super"]) })
		agents := &AgentPKIHandler{pki: func() (*agentpki.Service, error) { return nil, agentpki.ErrDisabled }, db: func() *gorm.DB { return f.db }}
		router.POST("/rotate-credentials", agents.RotateCredentials)
		response := serveModuleRequest(router, http.MethodPost, "/rotate-credentials", `{"node":"proxy-1"}`)
		assert.Equal(t, http.StatusConflict, response.Code)
		assert.Contains(t, response.Body.String(), "agent_pki_disabled")
	})
}
