package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var tokenTestGormConfig = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

func migrateTokenTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdminAPIToken{}, &model.OperationLog{}))
}

func openTokenSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)"), tokenTestGormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrateTokenTables(t, db)
	return db
}

// openTokenPostgres opens a throwaway schema of ANIX_TEST_POSTGRES_DSN.
func openTokenPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), tokenTestGormConfig)
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
	schema := "admin_token_mw_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), tokenTestGormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrateTokenTables(t, db)
	return db
}

func forEachTokenDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { body(t, openTokenSQLite(t)) })
	t.Run("postgres", func(t *testing.T) { body(t, openTokenPostgres(t)) })
}

const tokenTestSecret = "token-test-jwt-secret"

type tokenEnv struct {
	router *gin.Engine
	db     *gorm.DB
	tokens *adminapitoken.Service
	logs   *bytes.Buffer
}

func newTokenEnv(t *testing.T, db *gorm.DB) *tokenEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	config.Set(&config.Config{JWT: config.JWTConfig{Secret: tokenTestSecret}})
	for _, user := range []model.User{
		{ID: 7, Email: "root@example.com", IsAdmin: 1},
		{ID: 8, Email: "staff@example.com", IsAdmin: 1, IsStaff: 1},
		{ID: 9, Email: "member@example.com"},
	} {
		user.Token, user.UUID = "token-"+user.Email, "uuid-"+user.Email
		require.NoError(t, db.Create(&user).Error)
	}
	env := &tokenEnv{db: db, tokens: adminapitoken.New(db), logs: &bytes.Buffer{}}

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	cache := &tokenServiceCache{source: func() *gorm.DB { return db }}
	failures := service.NewLoginRateLimiter()
	router := gin.New()
	router.Use(gin.LoggerWithWriter(env.logs))
	whoami := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": c.GetUint("user_id"), "email": c.GetString("email"), "is_admin": c.GetBool("is_admin"),
			"auth_method": c.GetString(adminapitoken.ContextKeyAuthMethod), "scope": c.GetString(adminapitoken.ContextKeyTokenScope),
		})
	}
	for _, prefix := range []string{"/api/v2/admin", "/api/v3", "/api/v4"} {
		group := router.Group(prefix)
		group.Use(adminAPIToken(cache, failures), JWTAuth(), AdminAuth())
		group.GET("/things", whoami)
		group.HEAD("/things", whoami)
		group.POST("/things", whoami)
		group.PUT("/things/:id", whoami)
		group.DELETE("/things/:id", whoami)
	}
	admin := router.Group("/api/v2/admin")
	admin.Use(adminAPIToken(cache, failures), JWTAuth(), AdminAuth())
	admin.GET("/nodes/:id/credentials", whoami)
	admin.GET("/telegram/bot", whoami)
	v4 := router.Group("/api/v4")
	v4.Use(adminAPIToken(cache, failures), JWTAuth(), AdminAuth())
	v4.GET("/kernel/api-tokens", whoami)
	v4.POST("/kernel/api-tokens", whoami)
	v4.DELETE("/kernel/api-tokens/:id", whoami)
	// User routes have no token middleware.
	user := router.Group("/api/v2/user")
	user.Use(JWTAuth())
	user.GET("/profile", whoami)
	user.POST("/mfa/disable", whoami)
	env.router = router
	return env
}

func (e *tokenEnv) issue(t *testing.T, owner uint, scope string, expires *time.Time) (string, model.AdminAPIToken) {
	t.Helper()
	token, row, err := e.tokens.Create(context.Background(), adminapitoken.CreateInput{UserID: owner, Name: "ci", Scope: scope, ExpiresAt: expires})
	require.NoError(t, err)
	return token, row
}

func (e *tokenEnv) do(method, target, authorization, remote string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if remote != "" {
		request.RemoteAddr = remote + ":4000"
	}
	recorder := httptest.NewRecorder()
	e.router.ServeHTTP(recorder, request)
	return recorder
}

func jwtFor(t *testing.T, userID uint, admin bool) string {
	t.Helper()
	token, err := utils.GenerateToken(userID, "jwt@example.com", admin, tokenTestSecret, 3600)
	require.NoError(t, err)
	return "Bearer " + token
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
	return body
}

func auditActions(t *testing.T, db *gorm.DB, action string) []model.OperationLog {
	t.Helper()
	var rows []model.OperationLog
	require.NoError(t, db.Where("action = ?", action).Order("id").Find(&rows).Error)
	return rows
}

func TestAdminAPITokenAuthenticatesTheAdminAPIs(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		token, row := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
		for _, prefix := range []string{"/api/v2/admin", "/api/v3", "/api/v4"} {
			for method, path := range map[string]string{
				http.MethodGet: "/things", http.MethodPost: "/things", http.MethodPut: "/things/1", http.MethodDelete: "/things/1",
			} {
				response := env.do(method, prefix+path, "Bearer "+token, "")
				require.Equal(t, http.StatusOK, response.Code, method+" "+prefix+path+" "+response.Body.String())
				body := decodeBody(t, response)
				assert.EqualValues(t, 7, body["user_id"])
				assert.Equal(t, "root@example.com", body["email"])
				assert.Equal(t, true, body["is_admin"])
				assert.Equal(t, adminapitoken.AuthMethodAPIToken, body["auth_method"])
				assert.Equal(t, model.AdminAPITokenScopeAdmin, body["scope"])
			}
		}
		// The scheme is case-insensitive, as RFC 7235 has it.
		assert.Equal(t, http.StatusOK, env.do(http.MethodGet, "/api/v3/things", "bearer "+token, "").Code)

		// The JWT is untouched: same routes, no token context.
		response := env.do(http.MethodPost, "/api/v4/things", jwtFor(t, 7, true), "")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Empty(t, decodeBody(t, response)["auth_method"])
		assert.Equal(t, http.StatusForbidden, env.do(http.MethodGet, "/api/v4/things", jwtFor(t, 9, false), "").Code, "a non-admin JWT is still refused")
		assert.Equal(t, http.StatusUnauthorized, env.do(http.MethodGet, "/api/v4/things", "", "").Code)

		var stored model.AdminAPIToken
		require.NoError(t, db.First(&stored, "id = ?", row.ID).Error)
		require.NotNil(t, stored.LastUsedAt, "a use is recorded")
	})
}

func TestAdminAPITokenIsRefusedOnUserRoutes(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		token, _ := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
		for method, path := range map[string]string{http.MethodGet: "/api/v2/user/profile", http.MethodPost: "/api/v2/user/mfa/disable"} {
			response := env.do(method, path, "Bearer "+token, "")
			assert.Equal(t, http.StatusUnauthorized, response.Code, path)
			assert.NotContains(t, response.Body.String(), "api_token", "the user route does not know tokens")
		}
		// The raw token without a scheme is no JWT either.
		assert.Equal(t, http.StatusUnauthorized, env.do(http.MethodGet, "/api/v4/things", token, "").Code)
	})
}

func TestAdminAPITokenReadScope(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		token, row := env.issue(t, 7, model.AdminAPITokenScopeRead, nil)
		bearer := "Bearer " + token
		for _, prefix := range []string{"/api/v2/admin", "/api/v3", "/api/v4"} {
			assert.Equal(t, http.StatusOK, env.do(http.MethodGet, prefix+"/things", bearer, "").Code, prefix)
			assert.Equal(t, http.StatusOK, env.do(http.MethodHead, prefix+"/things", bearer, "").Code, prefix)
			for method, path := range map[string]string{http.MethodPost: "/things", http.MethodPut: "/things/1", http.MethodDelete: "/things/1"} {
				response := env.do(method, prefix+path, bearer, "")
				assert.Equal(t, http.StatusForbidden, response.Code, method+" "+prefix)
				assert.Equal(t, "api_token_forbidden", decodeBody(t, response)["code"])
			}
		}
		// A read that answers a secret in clear is not a read for this scope.
		for _, path := range []string{"/api/v2/admin/nodes/3/credentials", "/api/v2/admin/telegram/bot"} {
			response := env.do(http.MethodGet, path, bearer, "")
			assert.Equal(t, http.StatusForbidden, response.Code, path)
			assert.Equal(t, http.StatusOK, env.do(http.MethodGet, path, "Bearer "+adminToken(t, env), "").Code, path+
				": the admin scope may read it, as its owner may (the credentials read is audited as reveal)")
		}

		reasons := map[string]bool{}
		for _, entry := range auditActions(t, db, adminapitoken.AuditActionUseDenied) {
			assert.Equal(t, 2, entry.Status)
			assert.NotContains(t, entry.Content, token)
			assert.Contains(t, entry.Content, row.ID)
			for _, reason := range []string{"scope_read_only", "scope_secret_read"} {
				if strings.Contains(entry.Content, reason) {
					reasons[reason] = true
				}
			}
		}
		assert.Equal(t, map[string]bool{"scope_read_only": true, "scope_secret_read": true}, reasons)
	})
}

func adminToken(t *testing.T, env *tokenEnv) string {
	t.Helper()
	token, _ := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
	return token
}

func TestAdminAPITokenCannotManageTokens(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		for _, scope := range []string{model.AdminAPITokenScopeAdmin, model.AdminAPITokenScopeRead} {
			token, _ := env.issue(t, 7, scope, nil)
			for method, path := range map[string]string{
				http.MethodGet: "/api/v4/kernel/api-tokens", http.MethodPost: "/api/v4/kernel/api-tokens", http.MethodDelete: "/api/v4/kernel/api-tokens/x",
			} {
				response := env.do(method, path, "Bearer "+token, "")
				assert.Equal(t, http.StatusForbidden, response.Code, scope+" "+method)
			}
		}
		for method, path := range map[string]string{
			http.MethodGet: "/api/v4/kernel/api-tokens", http.MethodPost: "/api/v4/kernel/api-tokens", http.MethodDelete: "/api/v4/kernel/api-tokens/x",
		} {
			assert.Equal(t, http.StatusOK, env.do(method, path, jwtFor(t, 7, true), "").Code, "a session reaches it: "+method)
		}
		var interactive int
		for _, entry := range auditActions(t, db, adminapitoken.AuditActionUseDenied) {
			if strings.Contains(entry.Content, "interactive_only") {
				interactive++
			}
		}
		assert.Equal(t, 2, interactive, "one entry per token and reason")
	})
}

func TestAdminAPITokenNeverInAURL(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		token, row := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
		for _, target := range []string{
			"/api/v4/things?token=" + token, "/api/v4/things?api_key=" + token, "/api/v3/things?access_token=" + token,
			"/api/v2/admin/things?x=1&token=" + token,
		} {
			for _, authorization := range []string{"", "Bearer " + token, jwtFor(t, 7, true)} {
				response := env.do(http.MethodGet, target, authorization, "")
				assert.Equal(t, http.StatusUnauthorized, response.Code, target)
				assert.Equal(t, "api_token_in_url", decodeBody(t, response)["code"])
				assert.NotContains(t, response.Body.String(), token, "the answer does not echo it")
			}
		}
		var stored model.AdminAPIToken
		require.NoError(t, db.First(&stored, "id = ?", row.ID).Error)
		assert.Nil(t, stored.LastUsedAt, "a token in a URL is never used")
		assert.Nil(t, stored.RevokedAt)
		denied := auditActions(t, db, adminapitoken.AuditActionUseDenied)
		require.NotEmpty(t, denied)
		for _, entry := range denied {
			assert.Contains(t, entry.Content, "token_in_url")
			assert.NotContains(t, entry.Content, token)
			assert.NotContains(t, entry.Content, "?", "the audited path has no query")
		}
	})
}

func TestAdminAPITokenRefusalsAreAllTheSame(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		past := time.Now().Add(-time.Hour)
		// An already expired token cannot be created: age it in the table.
		expired, expiredRow := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
		require.NoError(t, db.Model(&model.AdminAPIToken{}).Where("id = ?", expiredRow.ID).Update("expires_at", past).Error)
		revoked, revokedRow := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
		_, _, err := env.tokens.Revoke(context.Background(), revokedRow.ID, adminapitoken.Actor{UserID: 7})
		require.NoError(t, err)
		banned, _ := env.issue(t, 8, model.AdminAPITokenScopeAdmin, nil)
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 8).Update("banned", 1).Error)
		require.NoError(t, db.Create(&model.User{ID: 20, Email: "demoted@example.com", Token: "t20", UUID: "u20", IsAdmin: 1}).Error)
		demoted, _ := env.issue(t, 20, model.AdminAPITokenScopeAdmin, nil)
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 20).Update("is_admin", 0).Error)
		require.NoError(t, db.Create(&model.User{ID: 21, Email: "deleted@example.com", Token: "t21", UUID: "u21", IsAdmin: 1}).Error)
		deleted, _ := env.issue(t, 21, model.AdminAPITokenScopeAdmin, nil)
		require.NoError(t, db.Delete(&model.User{}, 21).Error)
		unknown := adminapitoken.Prefix + strings.Repeat("A", 43)

		var want string
		for name, token := range map[string]string{
			"expired": expired, "revoked": revoked, "banned owner": banned, "demoted owner": demoted, "deleted owner": deleted,
			"unknown": unknown, "malformed": adminapitoken.Prefix + "short",
		} {
			// Each from its own address, so the lockout of the next test is not met here.
			response := env.do(http.MethodGet, "/api/v4/things", "Bearer "+token, "198.51.100.77")
			assert.Equal(t, http.StatusUnauthorized, response.Code, name)
			if want == "" {
				want = response.Body.String()
			}
			assert.Equal(t, want, response.Body.String(), name+": one answer for every refusal")
		}
		assert.Contains(t, want, "api_token_invalid")

		// The audit log says which, without the token.
		var details []string
		for _, entry := range auditActions(t, db, adminapitoken.AuditActionUseDenied) {
			for _, token := range []string{expired, revoked, banned, demoted, deleted, unknown} {
				assert.NotContains(t, entry.Content, token)
			}
			details = append(details, entry.Content)
		}
		joined := strings.Join(details, "\n")
		for _, reason := range []string{"expired", "revoked", "owner banned", "owner not an administrator", "owner deleted", "unknown", "malformed"} {
			assert.Contains(t, joined, reason)
		}
	})
}

func TestAdminAPITokenFailuresAreRateLimited(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		good, _ := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
		bad := "Bearer " + adminapitoken.Prefix + strings.Repeat("B", 43)
		for i := range apiTokenFailureLimit.MaxAttempts {
			response := env.do(http.MethodGet, "/api/v4/things", bad, "203.0.113.9")
			require.Equal(t, http.StatusUnauthorized, response.Code, "attempt %d", i+1)
		}
		// The address is locked out of token authentication, valid token or
		// not, and told for how long.
		response := env.do(http.MethodGet, "/api/v4/things", "Bearer "+good, "203.0.113.9")
		assert.Equal(t, http.StatusTooManyRequests, response.Code)
		assert.NotEmpty(t, response.Header().Get("Retry-After"))
		assert.Equal(t, "api_token_rate_limited", decodeBody(t, response)["code"])
		// Other addresses and JWT sessions are unaffected.
		assert.Equal(t, http.StatusOK, env.do(http.MethodGet, "/api/v4/things", "Bearer "+good, "203.0.113.10").Code)
		assert.Equal(t, http.StatusOK, env.do(http.MethodGet, "/api/v4/things", jwtFor(t, 7, true), "203.0.113.9").Code)
	})
}

func TestAdminAPITokenWritesAreAuditedAsTokenUse(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		token, row := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
		require.Equal(t, http.StatusOK, env.do(http.MethodGet, "/api/v3/things", "Bearer "+token, "").Code)
		assert.Empty(t, auditActions(t, db, adminapitoken.AuditActionUse), "reads are not logged one by one")
		require.Equal(t, http.StatusOK, env.do(http.MethodDelete, "/api/v3/things/5", "Bearer "+token, "192.0.2.44").Code)
		uses := auditActions(t, db, adminapitoken.AuditActionUse)
		require.Len(t, uses, 1)
		assert.Equal(t, "root@example.com", uses[0].Username)
		assert.Equal(t, "192.0.2.44", uses[0].IP)
		assert.Contains(t, uses[0].Content, row.ID)
		assert.Contains(t, uses[0].Content, "/api/v3/things/5")
		assert.NotContains(t, uses[0].Content, token)
	})
}

func TestAdminAPITokenNeverInLogs(t *testing.T) {
	forEachTokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTokenEnv(t, db)
		token, _ := env.issue(t, 7, model.AdminAPITokenScopeRead, nil)
		secret := token[len(adminapitoken.Prefix):]
		env.do(http.MethodGet, "/api/v4/things", "Bearer "+token, "")
		env.do(http.MethodPost, "/api/v4/things", "Bearer "+token, "")
		env.do(http.MethodGet, "/api/v4/things", "Bearer "+token[:len(token)-1]+"x", "")
		env.do(http.MethodGet, "/api/v2/user/profile", "Bearer "+token, "")
		assert.NotEmpty(t, env.logs.String(), "requests were logged")
		assert.NotContains(t, env.logs.String(), token)
		assert.NotContains(t, env.logs.String(), secret)
		assert.NotContains(t, env.logs.String(), adminapitoken.Hash(token))
	})
}

func TestAdminAPITokenStorageFailureIs503(t *testing.T) {
	env := newTokenEnv(t, openTokenSQLite(t))
	token, _ := env.issue(t, 7, model.AdminAPITokenScopeAdmin, nil)
	require.NoError(t, env.db.Migrator().DropTable(&model.AdminAPIToken{}))
	response := env.do(http.MethodGet, "/api/v4/things", "Bearer "+token, "")
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.NotContains(t, env.logs.String(), token)
	// No database at all.
	cache := &tokenServiceCache{source: func() *gorm.DB { return nil }}
	router := gin.New()
	router.Use(adminAPIToken(cache, service.NewLoginRateLimiter()))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodGet, "/x", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestBearerAPIToken(t *testing.T) {
	token := adminapitoken.Prefix + strings.Repeat("a", 43)
	for header, want := range map[string]bool{
		"Bearer " + token: true, "bearer " + token: true, "BEARER   " + token + "  ": true,
		token: false, "Basic " + token: false, "Bearer eyJhbGciOiJIUzI1NiJ9.e30.x": false, "": false, "Bearer": false,
		"Bearer x" + token: false,
	} {
		got, ok := bearerAPIToken(header)
		assert.Equal(t, want, ok, header)
		if ok {
			assert.Equal(t, token, got)
		}
	}
}
