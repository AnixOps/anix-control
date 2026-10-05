package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var apiTokenTestGorm = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

func migrateAPITokenTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserMFA{}, &model.AdminAPIToken{}, &model.OperationLog{}))
}

func openAPITokenSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)"), apiTokenTestGorm)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrateAPITokenTables(t, db)
	return db
}

// openAPITokenPostgres opens a throwaway schema of ANIX_TEST_POSTGRES_DSN.
func openAPITokenPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), apiTokenTestGorm)
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
	schema := "admin_token_handler_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), apiTokenTestGorm)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrateAPITokenTables(t, db)
	return db
}

func forEachAPITokenDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { body(t, openAPITokenSQLite(t)) })
	t.Run("postgres", func(t *testing.T) { body(t, openAPITokenPostgres(t)) })
}

const apiTokenTestPassword = "correct horse battery staple"

type apiTokenEnv struct {
	router *gin.Engine
	db     *gorm.DB
	clock  *time.Time
}

func newAPITokenEnv(t *testing.T, db *gorm.DB) *apiTokenEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	hash, err := bcrypt.GenerateFromPassword([]byte(apiTokenTestPassword), bcrypt.MinCost)
	require.NoError(t, err)
	for _, user := range []model.User{
		{ID: 7, Email: "root@example.com", IsAdmin: 1, Password: string(hash)},
		{ID: 8, Email: "staff@example.com", IsAdmin: 1, IsStaff: 1, Password: string(hash)},
		{ID: 9, Email: "member@example.com", Password: string(hash)},
		{ID: 10, Email: "identity@example.com", IsAdmin: 1, Password: model.UnusableLegacyPassword},
		{ID: 11, Email: "mfa@example.com", IsAdmin: 1, Password: string(hash)},
	} {
		user.Token, user.UUID = "token-"+user.Email, "uuid-"+user.Email
		require.NoError(t, db.Create(&user).Error)
	}
	now := time.Now()
	env := &apiTokenEnv{db: db, clock: &now}
	handler := &AdminAPITokensHandler{
		db: func() *gorm.DB { return db }, limiter: service.NewLoginRateLimiter(),
		now: func() time.Time { return *env.clock },
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		// What the authentication middleware sets: the user, and for an API
		// token its marker.
		id, _ := strconv.ParseUint(c.GetHeader("X-Test-User"), 10, 32)
		c.Set("user_id", uint(id))
		c.Set("email", c.GetHeader("X-Test-Email"))
		if c.GetHeader("X-Test-Token") != "" {
			c.Set(adminapitoken.ContextKeyAuthMethod, adminapitoken.AuthMethodAPIToken)
		}
		if issued := c.GetHeader("X-Test-Issued-Ago"); issued != "" {
			seconds, _ := strconv.Atoi(issued)
			c.Set("token_issued_at", env.clock.Add(-time.Duration(seconds)*time.Second))
		}
		c.Next()
	})
	router.POST("/api-tokens", handler.Create)
	router.GET("/api-tokens", handler.List)
	router.DELETE("/api-tokens/:id", handler.Revoke)
	env.router = router
	return env
}

func (e *apiTokenEnv) call(method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-User", "7")
	request.Header.Set("X-Test-Email", "root@example.com")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	e.router.ServeHTTP(recorder, request)
	return recorder
}

func (e *apiTokenEnv) as(user, email string) map[string]string {
	return map[string]string{"X-Test-User": user, "X-Test-Email": email}
}

type apiTokenAnswer struct {
	Data struct {
		Token    string         `json:"token"`
		APIToken map[string]any `json:"api_token"`
	} `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeAPITokenAnswer(t *testing.T, response *httptest.ResponseRecorder) apiTokenAnswer {
	t.Helper()
	var answer apiTokenAnswer
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer), response.Body.String())
	return answer
}

func createBody(extra string) string {
	body := `{"name":"deploy","scope":"read","password":"` + apiTokenTestPassword + `"`
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

func TestAdminAPITokenCreate(t *testing.T) {
	forEachAPITokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newAPITokenEnv(t, db)
		response := env.call(http.MethodPost, "/api-tokens", createBody(`"expires_in_days":30`), nil)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		answer := decodeAPITokenAnswer(t, response)
		token := answer.Data.Token
		assert.True(t, adminapitoken.WellFormed(token))
		assert.Equal(t, "read", answer.Data.APIToken["scope"])
		assert.Equal(t, "deploy", answer.Data.APIToken["name"])
		assert.EqualValues(t, 7, answer.Data.APIToken["user_id"])
		assert.NotContains(t, answer.Data.APIToken, "token_hash")
		assert.NotContains(t, answer.Data.APIToken, "token")
		expires, err := time.Parse(time.RFC3339, answer.Data.APIToken["expires_at"].(string))
		require.NoError(t, err)
		assert.WithinDuration(t, env.clock.Add(30*24*time.Hour), expires, time.Second)

		// The token works as stored, and is nowhere in clear, the audit
		// entry and the request password included.
		principal, err := adminapitoken.New(db).Authenticate(context.Background(), token, "")
		require.NoError(t, err)
		assert.EqualValues(t, 7, principal.UserID)
		var raw []map[string]any
		require.NoError(t, db.Table("v4_kernel_admin_api_token").Find(&raw).Error)
		stored, err := json.Marshal(raw)
		require.NoError(t, err)
		assert.NotContains(t, string(stored), token)
		var logs []model.OperationLog
		require.NoError(t, db.Find(&logs).Error)
		require.Len(t, logs, 1)
		assert.Equal(t, adminapitoken.AuditActionCreate, logs[0].Action)
		assert.Equal(t, "root@example.com", logs[0].Username)
		assert.NotContains(t, logs[0].Content, token)
		assert.NotContains(t, logs[0].Content, apiTokenTestPassword)

		// Invalid requests.
		for name, body := range map[string]string{
			"no scope":         `{"name":"x","password":"` + apiTokenTestPassword + `"}`,
			"no name":          `{"scope":"read","password":"` + apiTokenTestPassword + `"}`,
			"unknown scope":    createBody(`"scope":"write"`),
			"too long expiry":  createBody(`"expires_in_days":731`),
			"negative expiry":  createBody(`"expires_in_days":-1`),
			"not json":         `nope`,
			"name too long":    `{"name":"` + strings.Repeat("n", 101) + `","scope":"read","password":"` + apiTokenTestPassword + `"}`,
			"expiry not a num": createBody(`"expires_in_days":"soon"`),
		} {
			assert.Equal(t, http.StatusBadRequest, env.call(http.MethodPost, "/api-tokens", body, nil).Code, name)
		}
		// A scope duplicated in the body is the last one: not a bypass of anything.
		assert.Equal(t, http.StatusCreated, env.call(http.MethodPost, "/api-tokens", createBody(`"scope":"admin"`), nil).Code)
	})
}

func TestAdminAPITokenCreateNeedsAnAdministratorAndASession(t *testing.T) {
	forEachAPITokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newAPITokenEnv(t, db)
		// A request an API token authenticated never reaches the handlers.
		for method, target := range map[string]string{http.MethodPost: "/api-tokens", http.MethodGet: "/api-tokens", http.MethodDelete: "/api-tokens/x"} {
			response := env.call(method, target, createBody(""), map[string]string{"X-Test-Token": "1"})
			assert.Equal(t, http.StatusForbidden, response.Code, method)
			assert.Equal(t, "session_required", decodeAPITokenAnswer(t, response).Error.Code)
		}
		// No signed-in user.
		assert.Equal(t, http.StatusUnauthorized, env.call(http.MethodPost, "/api-tokens", createBody(""), map[string]string{"X-Test-User": "0"}).Code)
		// A member who somehow got here.
		response := env.call(http.MethodPost, "/api-tokens", createBody(""), env.as("9", "member@example.com"))
		assert.Equal(t, http.StatusForbidden, response.Code)
		var count int64
		require.NoError(t, db.Model(&model.AdminAPIToken{}).Count(&count).Error)
		assert.Zero(t, count)
		// A banned administrator, even with their password.
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 8).Update("banned", 1).Error)
		response = env.call(http.MethodPost, "/api-tokens", createBody(""), env.as("8", "staff@example.com"))
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Equal(t, "not_an_administrator", decodeAPITokenAnswer(t, response).Error.Code)
	})
}

func TestAdminAPITokenStepUp(t *testing.T) {
	forEachAPITokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newAPITokenEnv(t, db)
		denied := func(body, wantCode string) {
			t.Helper()
			response := env.call(http.MethodPost, "/api-tokens", body, nil)
			assert.Equal(t, http.StatusForbidden, response.Code, body)
			assert.Equal(t, wantCode, decodeAPITokenAnswer(t, response).Error.Code, body)
			assert.NotContains(t, response.Body.String(), apiTokenTestPassword)
		}
		denied(`{"name":"x","scope":"read"}`, "step_up_required")
		denied(`{"name":"x","scope":"read","password":"wrong"}`, "step_up_failed")
		denied(`{"name":"x","scope":"read","code":"123456"}`, "step_up_required")
		var count int64
		require.NoError(t, db.Model(&model.AdminAPIToken{}).Count(&count).Error)
		assert.Zero(t, count, "nothing is created without the re-authentication")

		// The refusals are audited, without the password.
		var entries []model.OperationLog
		require.NoError(t, db.Where("action = ?", adminapitoken.AuditActionCreateDenied).Find(&entries).Error)
		require.NotEmpty(t, entries)
		for _, entry := range entries {
			assert.Equal(t, "root@example.com", entry.Username)
			assert.NotContains(t, entry.Content, "wrong")
			assert.NotContains(t, entry.Content, "123456")
		}

		// Repeated failures lock creation out, even for the right password.
		for range 6 {
			env.call(http.MethodPost, "/api-tokens", `{"name":"x","scope":"read","password":"wrong"}`, nil)
		}
		response := env.call(http.MethodPost, "/api-tokens", createBody(""), nil)
		assert.Equal(t, http.StatusTooManyRequests, response.Code)
		assert.NotEmpty(t, response.Header().Get("Retry-After"))
		// Another administrator is not locked out.
		assert.Equal(t, http.StatusCreated, env.call(http.MethodPost, "/api-tokens", createBody(""), env.as("8", "staff@example.com")).Code)
	})
}

func TestAdminAPITokenStepUpWithMFA(t *testing.T) {
	forEachAPITokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newAPITokenEnv(t, db)
		key, err := totp.Generate(totp.GenerateOpts{Issuer: "test", AccountName: "mfa@example.com"})
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.UserMFA{UserID: 11, Enabled: true, TOTPSecret: key.Secret(), BackupCodes: `["backup-one"]`}).Error)
		as := env.as("11", "mfa@example.com")
		call := func(body string) *httptest.ResponseRecorder {
			return env.call(http.MethodPost, "/api-tokens", body, as)
		}

		// The password alone is not enough once a second factor is on.
		response := call(createBody(""))
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Equal(t, "step_up_required", decodeAPITokenAnswer(t, response).Error.Code)
		response = call(`{"name":"x","scope":"read","code":"000000","method":"totp"}`)
		assert.Equal(t, "step_up_failed", decodeAPITokenAnswer(t, response).Error.Code)

		code, err := totp.GenerateCode(key.Secret(), time.Now())
		require.NoError(t, err)
		response = call(`{"name":"x","scope":"admin","code":"` + code + `","method":"totp"}`)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())

		// A recovery code works once.
		response = call(`{"name":"y","scope":"read","code":"backup-one","method":"backup"}`)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		response = call(`{"name":"z","scope":"read","code":"backup-one","method":"backup"}`)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})
}

func TestAdminAPITokenStepUpWhenIdentityHoldsTheCredentials(t *testing.T) {
	forEachAPITokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newAPITokenEnv(t, db)
		body := `{"name":"x","scope":"read"}`
		headers := func(issuedAgo string) map[string]string {
			h := env.as("10", "identity@example.com")
			if issuedAgo != "" {
				h["X-Test-Issued-Ago"] = issuedAgo
			}
			return h
		}
		// The kernel cannot check a password it does not hold; the sign-in
		// that identity just checked stands in, when it is recent.
		assert.Equal(t, http.StatusForbidden, env.call(http.MethodPost, "/api-tokens", body, headers("")).Code, "no issue time")
		assert.Equal(t, http.StatusForbidden, env.call(http.MethodPost, "/api-tokens", body, headers("601")).Code, "a stale sign-in")
		assert.Equal(t, http.StatusCreated, env.call(http.MethodPost, "/api-tokens", body, headers("120")).Code)
		// A password in the body does not matter for such an account.
		assert.Equal(t, http.StatusForbidden, env.call(http.MethodPost, "/api-tokens", createBody(""), headers("601")).Code)
	})
}

func TestAdminAPITokenLimit(t *testing.T) {
	forEachAPITokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newAPITokenEnv(t, db)
		for range adminapitoken.MaxActivePerUser {
			require.Equal(t, http.StatusCreated, env.call(http.MethodPost, "/api-tokens", createBody(""), nil).Code)
		}
		response := env.call(http.MethodPost, "/api-tokens", createBody(""), nil)
		assert.Equal(t, http.StatusConflict, response.Code)
		assert.Equal(t, "too_many_tokens", decodeAPITokenAnswer(t, response).Error.Code)
	})
}

type listAnswer struct {
	Data []map[string]any `json:"data"`
}

func listIDs(t *testing.T, response *httptest.ResponseRecorder) []string {
	t.Helper()
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var answer listAnswer
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer))
	ids := []string{}
	for _, row := range answer.Data {
		ids = append(ids, row["id"].(string))
		assert.NotContains(t, row, "token_hash")
		assert.NotContains(t, row, "token")
		assert.NotEmpty(t, row["hint"])
	}
	return ids
}

func TestAdminAPITokenListAndRevoke(t *testing.T) {
	forEachAPITokenDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newAPITokenEnv(t, db)
		create := func(headers map[string]string) (string, string) {
			response := env.call(http.MethodPost, "/api-tokens", createBody(""), headers)
			require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
			answer := decodeAPITokenAnswer(t, response)
			return answer.Data.APIToken["id"].(string), answer.Data.Token
		}
		rootToken, rootSecret := create(nil)
		staffToken, staffSecret := create(env.as("8", "staff@example.com"))

		// Lists are the caller's own.
		body := listIDs(t, env.call(http.MethodGet, "/api-tokens", "", nil))
		assert.Equal(t, []string{rootToken}, body)
		assert.Equal(t, []string{staffToken}, listIDs(t, env.call(http.MethodGet, "/api-tokens", "", env.as("8", "staff@example.com"))))
		raw := env.call(http.MethodGet, "/api-tokens?include_inactive=true", "", nil).Body.String()
		assert.NotContains(t, raw, rootSecret)
		assert.NotContains(t, raw, staffSecret)

		// Only a super administrator reads another's: root (not staff) may,
		// the staff administrator may not, and a member is no administrator.
		assert.Equal(t, []string{staffToken}, listIDs(t, env.call(http.MethodGet, "/api-tokens?user_id=8", "", nil)))
		assert.ElementsMatch(t, []string{rootToken, staffToken}, listIDs(t, env.call(http.MethodGet, "/api-tokens?all=true", "", nil)))
		staff := env.as("8", "staff@example.com")
		assert.Equal(t, http.StatusForbidden, env.call(http.MethodGet, "/api-tokens?user_id=7", "", staff).Code)
		assert.Equal(t, http.StatusForbidden, env.call(http.MethodGet, "/api-tokens?all=true", "", staff).Code)
		assert.Equal(t, http.StatusOK, env.call(http.MethodGet, "/api-tokens?user_id=8", "", staff).Code, "their own id is fine")
		assert.Equal(t, http.StatusBadRequest, env.call(http.MethodGet, "/api-tokens?user_id=abc", "", nil).Code)

		// The staff administrator cannot revoke root's token, and is told
		// nothing about whether it exists.
		response := env.call(http.MethodDelete, "/api-tokens/"+rootToken, "", staff)
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(t, http.StatusNotFound, env.call(http.MethodDelete, "/api-tokens/no-such-token", "", staff).Code)
		_, err := adminapitoken.New(db).Authenticate(context.Background(), rootSecret, "")
		require.NoError(t, err)

		// They revoke their own; root, a super administrator, revokes anyone's.
		response = env.call(http.MethodDelete, "/api-tokens/"+staffToken, "", staff)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Contains(t, response.Body.String(), `"changed":true`)
		assert.Empty(t, listIDs(t, env.call(http.MethodGet, "/api-tokens", "", staff)))
		assert.Equal(t, []string{staffToken}, listIDs(t, env.call(http.MethodGet, "/api-tokens?include_inactive=true", "", staff)))
		_, err = adminapitoken.New(db).Authenticate(context.Background(), staffSecret, "")
		assert.ErrorIs(t, err, adminapitoken.ErrRevoked)
		response = env.call(http.MethodDelete, "/api-tokens/"+staffToken, "", staff)
		assert.Equal(t, http.StatusOK, response.Code, "revoking again is not an error")
		assert.Contains(t, response.Body.String(), `"changed":false`)

		response = env.call(http.MethodDelete, "/api-tokens/"+rootToken, "", env.as("8", "staff@example.com"))
		assert.Equal(t, http.StatusNotFound, response.Code)
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 8).Update("is_staff", 0).Error)
		response = env.call(http.MethodDelete, "/api-tokens/"+rootToken, "", staff)
		require.Equal(t, http.StatusOK, response.Code, "an administrator who is not staff is a super administrator: "+response.Body.String())
		_, err = adminapitoken.New(db).Authenticate(context.Background(), rootSecret, "")
		assert.ErrorIs(t, err, adminapitoken.ErrRevoked)

		var entries []model.OperationLog
		require.NoError(t, db.Where("action = ?", adminapitoken.AuditActionRevoke).Find(&entries).Error)
		assert.Len(t, entries, 2)
		for _, entry := range entries {
			assert.NotContains(t, entry.Content, rootSecret)
			assert.NotContains(t, entry.Content, staffSecret)
		}
	})
}
