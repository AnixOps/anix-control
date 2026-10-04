// Package packagecompat compares a route's legacy kernel handler with its
// package-native implementation on identical seeded data. Extraction PRs use
// it to prove that a native route answers byte-compatibly (after
// v2compat.NormalizeForCompare) before its mode moves past shadow.
//
// Every run seeds two independent databases: the legacy handler reads the
// kernel's global database, the native handler gets its own connection. They
// are SQLite files by default; with ANIX_TEST_POSTGRES_DSN set, RunRead and
// RunWrite also run on two PostgreSQL schemas. Each case starts from empty
// databases with the route's Models migrated; the cases a test runs share
// the migrated databases, emptied between cases (shared.go).
package packagecompat

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgresDSNEnvironment enables the PostgreSQL runs.
const PostgresDSNEnvironment = "ANIX_TEST_POSTGRES_DSN"

// Route is one v2 route with both implementations.
type Route struct {
	// Method and Pattern are the gin registration, for example GET and
	// "/api/v2/user/knowledge/:id".
	Method  string
	Pattern string
	RouteID string
	// Legacy is the kernel's gin handler. It reads database.GetDB().
	Legacy gin.HandlerFunc
	// Native builds the package implementation on a database connection.
	Native func(db *gorm.DB) pluginhostsdk.NativeHandler
	// Models are migrated into both databases before Seed runs.
	Models []any
}

// Case is one request.
type Case struct {
	Name string
	// Path is the concrete request path, with query string if any.
	Path      string
	Body      []byte
	Principal pluginhostsdk.Principal
	// Seed writes the same data into each database.
	Seed func(t testing.TB, db *gorm.DB)
	// Snapshot returns the database state RunWrite compares after the
	// request; any JSON-encodable value.
	Snapshot func(t testing.TB, db *gorm.DB) any
	// Mask lists dotted JSON paths (for example "data.token") whose values
	// differ by design, such as tokens and secrets: they must exist on both
	// sides and are replaced before comparing.
	Mask []string
	// Headers lists response headers both sides must send.
	Headers []string
	// RequestHeaders are sent with the request: the legacy handler reads
	// them from it, the native one from the metadata the kernel forwards.
	RequestHeaders map[string]string
	// Host is the request's Host (default example.com) and TLS whether it
	// arrived over TLS: the legacy handler reads them from the request, the
	// native one from the request scheme and host the kernel sends.
	Host string
	TLS  bool
	// Warmup bodies are sent, in order, before the compared request.
	Warmup [][]byte
	// Fallback is set when the native side must answer from the legacy
	// handler (RunKernelRead, RunKernelWrite): the route cannot serve the
	// request natively, for example before the node credential split is
	// finalized.
	Fallback bool
}

// noRouteHeader marks the harness's own answer to a path no route matches.
const noRouteHeader = "X-Packagecompat-No-Route"

// clientIP is the address both sides see: the legacy handler through the
// request, the native one through the metadata the kernel sends.
const clientIP = "192.0.2.1"

// Result is one side's answer.
type Result struct {
	StatusCode int
	Body       []byte
	Header     http.Header
	State      any
}

// RunRead runs c against both implementations and requires equal status
// codes and normalized bodies.
func RunRead(t *testing.T, route Route, c Case) {
	t.Helper()
	forEachBackend(t, c.Name, func(t *testing.T, open opener) {
		legacy, native := run(t, open, route, c)
		requireSame(t, c, legacy, native)
	})
}

// RunWrite is RunRead plus an equal Snapshot of each database afterwards.
func RunWrite(t *testing.T, route Route, c Case) {
	t.Helper()
	require.NotNil(t, c.Snapshot, "RunWrite needs Case.Snapshot")
	forEachBackend(t, c.Name, func(t *testing.T, open opener) {
		legacy, native := run(t, open, route, c)
		requireSame(t, c, legacy, native)
		legacyState, err := json.Marshal(legacy.State)
		require.NoError(t, err)
		nativeState, err := json.Marshal(native.State)
		require.NoError(t, err)
		require.JSONEq(t, string(legacyState), string(nativeState), "database state after the request differs")
	})
}

func requireSameAnswer(t *testing.T, legacy, native Result) {
	t.Helper()
	require.NoError(t, CompareAnswers(legacy, native))
}

func requireSame(t *testing.T, c Case, legacy, native Result) {
	t.Helper()
	for _, name := range c.Headers {
		require.NotEmpty(t, legacy.Header.Get(name), "legacy %s header", name)
		require.NotEmpty(t, native.Header.Get(name), "native %s header", name)
	}
	if len(c.Mask) > 0 {
		legacy.Body, native.Body = mask(t, legacy.Body, c.Mask), mask(t, native.Body, c.Mask)
	}
	requireSameAnswer(t, legacy, native)
}

func mask(t *testing.T, body []byte, paths []string) []byte {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded), "masked body must be a JSON object: %s", body)
	for _, path := range paths {
		parts := strings.Split(path, ".")
		node := decoded
		for _, part := range parts[:len(parts)-1] {
			next, ok := node[part].(map[string]any)
			require.True(t, ok, "masked path %s is missing in %s", path, body)
			node = next
		}
		_, ok := node[parts[len(parts)-1]]
		require.True(t, ok, "masked path %s is missing in %s", path, body)
		node[parts[len(parts)-1]] = "<masked>"
	}
	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)
	return encoded
}

// CompareAnswers reports how two answers differ: status code, or body after
// v2compat.NormalizeForCompare. A sealed handle in the native answer
// matches the secret the legacy answer shows there, as in the router's
// shadow comparison (v2compat.EqualForCompare).
func CompareAnswers(legacy, native Result) error {
	if legacy.StatusCode != native.StatusCode {
		return fmt.Errorf("status code differs: legacy %d %s, native %d %s", legacy.StatusCode, legacy.Body, native.StatusCode, native.Body)
	}
	if !v2compat.EqualForCompare(legacy.Body, native.Body) {
		return fmt.Errorf("normalized body differs:\nlegacy %s\nnative %s", v2compat.NormalizeForCompare(legacy.Body), v2compat.NormalizeForCompare(native.Body))
	}
	return nil
}

// opener returns an empty database for one side of a case: its config for
// database.Init, a connection for the native side, and whether the route's
// Models are already migrated into it.
type opener func(t *testing.T, label string, models []any) (*config.DatabaseConfig, *gorm.DB, bool)

// forEachBackend runs body on SQLite and, with ANIX_TEST_POSTGRES_DSN, on
// PostgreSQL. The databases are shared by the cases a test runs through
// t: migrated once per side and backend, and emptied before each case
// (shared.go).
func forEachBackend(t *testing.T, name string, body func(*testing.T, opener)) {
	t.Helper()
	pool := poolFor(t)
	t.Run(name+"/sqlite", func(t *testing.T) { body(t, pool.opener(backendSQLite)) })
	if strings.TrimSpace(os.Getenv(PostgresDSNEnvironment)) != "" {
		t.Run(name+"/postgres", func(t *testing.T) { body(t, pool.opener(backendPostgres)) })
	}
}

func run(t *testing.T, open opener, route Route, c Case) (Result, Result) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	legacyConfig, _, migrated := open(t, "legacy", route.Models)
	database.Reset()
	require.NoError(t, database.Init(legacyConfig))
	t.Cleanup(func() {
		_ = database.Close()
		database.Reset()
	})
	legacyDB := database.GetDB()
	prepare(t, legacyDB, route, c, migrated)
	legacy := serve(t, route.Method, route.Pattern, c, func(ctx *gin.Context) {
		ctx.Set("user_id", c.Principal.ActorID)
		ctx.Set("is_admin", c.Principal.Admin)
		route.Legacy(ctx)
	})
	if c.Snapshot != nil {
		legacy.State = c.Snapshot(t, legacyDB)
	}

	_, nativeDB, migrated := open(t, "native", route.Models)
	prepare(t, nativeDB, route, c, migrated)
	handler := route.Native(nativeDB)
	native := serve(t, route.Method, route.Pattern, c, func(ctx *gin.Context) {
		params := make(map[string]string, len(ctx.Params))
		for _, param := range ctx.Params {
			params[param.Key] = param.Value
		}
		body, err := io.ReadAll(ctx.Request.Body)
		require.NoError(t, err)
		response, err := handler(ctx.Request.Context(), pluginhostsdk.NativeRequest{
			RouteID: route.RouteID, Method: route.Method, Body: body, Principal: c.Principal,
			Metadata: pluginhostsdk.RequestMetadata{
				Path: ctx.Request.URL.Path, Query: ctx.Request.URL.Query(), PathParams: params,
				ClientIP: clientIP, UserAgent: ctx.Request.UserAgent(), Headers: forwardedHeaders(c),
				Scheme: requestScheme(ctx.Request), Host: ctx.Request.Host,
			},
		})
		require.NoError(t, err, "native handler failed")
		for _, header := range response.Headers {
			ctx.Header(header.Name, header.Value)
		}
		ctx.Data(int(response.StatusCode), ctx.Writer.Header().Get("Content-Type"), response.Body)
	})
	if c.Snapshot != nil {
		native.State = c.Snapshot(t, nativeDB)
	}
	return legacy, native
}

// requestScheme is the scheme the kernel sends: from the connection's TLS
// state only.
func requestScheme(request *http.Request) string {
	if request.TLS != nil {
		return "https"
	}
	return "http"
}

// forwardedHeaders are the case's request headers as the kernel forwards
// them to a package: canonical names, nil when there are none.
func forwardedHeaders(c Case) map[string][]string {
	if len(c.RequestHeaders) == 0 {
		return nil
	}
	headers := make(map[string][]string, len(c.RequestHeaders))
	for name, value := range c.RequestHeaders {
		headers[http.CanonicalHeaderKey(name)] = []string{value}
	}
	return headers
}

func prepare(t *testing.T, db *gorm.DB, route Route, c Case, migrated bool) {
	t.Helper()
	if len(route.Models) > 0 && !migrated {
		require.NoError(t, db.AutoMigrate(route.Models...))
	}
	if c.Seed != nil {
		c.Seed(t, db)
	}
}

func serve(t *testing.T, method, pattern string, c Case, handler gin.HandlerFunc) Result {
	t.Helper()
	engine := gin.New()
	engine.Handle(method, pattern, handler)
	// A path the pattern does not match is a broken case; a 404 the handler
	// answers itself (an unknown record) is not.
	engine.NoRoute(func(ctx *gin.Context) { ctx.Header(noRouteHeader, "1"); ctx.Status(http.StatusNotFound) })
	send := func(body []byte) Result {
		request := httptest.NewRequestWithContext(context.Background(), method, c.Path, bytes.NewReader(body))
		request.RemoteAddr = clientIP + ":1234"
		if c.Host != "" {
			request.Host = c.Host
		}
		if c.TLS {
			request.TLS = &tls.ConnectionState{}
		} else {
			request.TLS = nil
		}
		if len(body) > 0 {
			request.Header.Set("Content-Type", "application/json")
		}
		for name, value := range c.RequestHeaders {
			request.Header.Set(name, value)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		require.Empty(t, recorder.Header().Get(noRouteHeader), "case path %s does not match pattern %s", c.Path, pattern)
		return Result{StatusCode: recorder.Code, Body: recorder.Body.Bytes(), Header: recorder.Header()}
	}
	for _, body := range c.Warmup {
		send(body)
	}
	return send(c.Body)
}

// openSQLite returns a fresh SQLite file.
func openSQLite(t *testing.T, label string, _ []any) (*config.DatabaseConfig, *gorm.DB, bool) {
	t.Helper()
	cfg, db, err := createSQLite(t.TempDir(), label)
	require.NoError(t, err)
	closeOnCleanup(t, db)
	return cfg, db, false
}

func createSQLite(dir, label string) (*config.DatabaseConfig, *gorm.DB, error) {
	path := filepath.Join(dir, label+".db")
	cfg := &config.DatabaseConfig{Driver: "sqlite", Database: path, LogLevel: "silent"}
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	return cfg, db, err
}

// errUnsafePostgres refuses a PostgreSQL database whose name does not say
// it is for tests.
var errUnsafePostgres = errors.New("refusing to run destructive postgres test")

// createPostgresSchema creates a throwaway schema and points a connection's
// search_path at it; drop removes the schema and closes the connections.
func createPostgresSchema(label string) (cfg *config.DatabaseConfig, db *gorm.DB, drop func(), err error) {
	base := strings.TrimSpace(os.Getenv(PostgresDSNEnvironment))
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, nil, nil, err
	}
	closeAdmin := func() {
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	var databaseName string
	if err := admin.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		closeAdmin()
		return nil, nil, nil, err
	}
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		closeAdmin()
		return nil, nil, nil, fmt.Errorf("%w against database %q", errUnsafePostgres, databaseName)
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		closeAdmin()
		return nil, nil, nil, err
	}
	schema := "packagecompat_" + label + "_" + hex.EncodeToString(suffix)
	if err := admin.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		closeAdmin()
		return nil, nil, nil, err
	}
	dropSchema := func() {
		_ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		closeAdmin()
	}
	dsn := base + " search_path=" + schema
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		dropSchema()
		return nil, nil, nil, err
	}
	drop = func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		dropSchema()
	}
	return &config.DatabaseConfig{Driver: "postgres", DSN: dsn, LogLevel: "silent"}, db, drop, nil
}

func closeOnCleanup(t *testing.T, db *gorm.DB) {
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}
