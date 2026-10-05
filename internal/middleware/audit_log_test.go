package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuditLogDoesNotTruncateLargeRequestBody(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	payload := strings.Repeat("x", maxBodyDBLength+1024)
	router := gin.New()
	router.Use(AuditLog())
	router.POST("/api/v2/admin/large", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.String(http.StatusOK, "%d", len(body))
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/admin/large", bytes.NewBufferString(payload))
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "5120", recorder.Body.String())
}

func TestAuditLogV3DoesNotWrapConfigurationBody(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	payload := strings.Repeat("x", maxBodyDBLength+1024)
	router := gin.New()
	router.Use(AuditLog())
	router.POST("/api/v3/topologies", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.String(http.StatusOK, "%d", len(body))
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v3/topologies", bytes.NewBufferString(payload))
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "5120", recorder.Body.String())
}

// Node secrets are masked in every administrator answer but the per-node
// credentials read, which is recorded like a write, as "reveal". Other reads
// are not recorded.
func TestAuditLogRecordsCredentialReveals(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := gin.New()
	router.Use(AuditLog())
	router.GET("/api/v2/admin/nodes/:id/credentials", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"api_key": "node-api-key"})
	})
	router.GET("/api/v2/admin/nodes/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v2/admin/nodes/7", nil))
	require.Empty(t, logged.String(), "an ordinary read is not recorded")

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v2/admin/nodes/7/credentials", nil))
	require.Contains(t, logged.String(), `"path":"/api/v2/admin/nodes/7/credentials"`)
	require.Contains(t, logged.String(), `"action":"reveal"`)
	require.Contains(t, logged.String(), `"module":"nodes"`)
	require.NotContains(t, logged.String(), "node-api-key", "the record never holds the answer")

	require.True(t, auditedRead(http.MethodGet, "/api/v2/admin/nodes/7/credentials"))
	require.False(t, auditedRead(http.MethodGet, "/api/v2/admin/nodes/7/credentials/extra"))
	require.False(t, auditedRead(http.MethodGet, "/api/v2/user/nodes/7/credentials"))
	require.False(t, auditedRead(http.MethodHead, "/api/v2/admin/nodes/7/credentials"))
}

// A user's reset of their own subscription link is recorded like the
// administrator's reset, as user/reset_subscribe, without its body: the
// body is the user's password or one-time code. Other user requests are
// not recorded.
func TestAuditLogRecordsAUsersSubscriptionReset(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "member@example.test") })
	router.POST("/api/v2/user/subscription/reset", AuditLog(), func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"password":"hunter2-secret"}`, string(body), "the handler still reads the whole body")
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})
	router.POST("/api/v2/user/ticket", AuditLog(), func(c *gin.Context) { c.Status(http.StatusOK) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v2/user/ticket", strings.NewReader(`{"subject":"x"}`)))
	require.Empty(t, logged.String(), "other user writes are not recorded")

	request := httptest.NewRequest(http.MethodPost, "/api/v2/user/subscription/reset", strings.NewReader(`{"password":"hunter2-secret"}`))
	router.ServeHTTP(httptest.NewRecorder(), request)
	require.Contains(t, logged.String(), `"path":"/api/v2/user/subscription/reset"`)
	require.Contains(t, logged.String(), `"module":"user"`)
	require.Contains(t, logged.String(), `"action":"reset_subscribe"`)
	require.Contains(t, logged.String(), `"user_id":"7"`)
	require.NotContains(t, logged.String(), "hunter2-secret")
	require.NotContains(t, logged.String(), "request_body")

	require.True(t, auditedUserWrite(http.MethodPost, "/api/v2/user/subscription/reset/"))
	require.False(t, auditedUserWrite(http.MethodGet, "/api/v2/user/subscription/reset"))
	require.False(t, auditedUserWrite(http.MethodPost, "/api/v2/user/subscription"))
}

// The forward package's v4 administrator API is recorded like v2's
// administrator writes, as module forward; its reads are not.
func TestAuditLogRecordsForwardV4Writes(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := gin.New()
	router.Use(AuditLog())
	router.Any("/api/v4/forward/*route", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v4/forward/routes", nil))
	require.Empty(t, logged.String(), "a read is not recorded")

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v4/forward/routes/01J/pause", strings.NewReader(`{}`)))
	require.Contains(t, logged.String(), `"module":"forward"`)
	require.Contains(t, logged.String(), `"action":"pause"`)

	logged.Reset()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v4/forward/nodes/forward-7", nil))
	require.Contains(t, logged.String(), `"path":"/api/v4/forward/nodes/forward-7"`)
	require.Contains(t, logged.String(), `"action":"delete"`)

	logged.Reset()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v4/forward/routes/01J/diagnose", nil))
	require.Contains(t, logged.String(), `"module":"forward"`)
	require.Contains(t, logged.String(), `"action":"diagnose"`)
}

// A forward route preview stores nothing, and the route editor sends one
// after every pause in typing: it is not audited (F5b D13). Creating the
// route still is.
func TestAuditLogExemptsForwardRoutePreview(t *testing.T) {
	require.True(t, auditExempt(http.MethodPost, "/api/v4/forward/routes/preview"))
	require.True(t, auditExempt(http.MethodPost, "/api/v4/forward/routes/preview/"))
	require.False(t, auditExempt(http.MethodPost, "/api/v4/forward/routes"))
	require.False(t, auditExempt(http.MethodPut, "/api/v4/forward/routes/preview"))
	require.False(t, auditExempt(http.MethodPost, "/api/v4/forward/routes/01J/diagnose"))

	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := gin.New()
	router.Use(AuditLog())
	router.Any("/api/v4/forward/*route", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v4/forward/routes/preview", strings.NewReader(`{"route":{}}`)))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, logged.String(), "a preview is logged at debug level only")

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v4/forward/routes", strings.NewReader(`{}`)))
	require.Contains(t, logged.String(), `"action":"create"`)
}

// The console's kernel API (/api/v4/admin/*) is audited like the /api/v2
// admin routes: its writes are recorded under the first path segment as the
// module, with the action a handler names (a bulk request's bulk_<action>)
// or the one the method gives; its reads are not recorded.
func TestAuditLogRecordsV4AdminWrites(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := gin.New()
	router.Use(AuditLog())
	router.POST("/api/v4/admin/users/bulk", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Set(AuditActionKey, "bulk_ban")
		c.Status(http.StatusOK)
	})
	router.POST("/api/v4/admin/invite-codes/bulk", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/v4/admin/users/activity", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.POST("/api/v4/adminx/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v4/admin/users/activity?ids=1", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v4/adminx/users", nil))
	require.Empty(t, logged.String(), "a read, and a path outside the prefix, are not recorded")

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v4/admin/users/bulk", strings.NewReader(`{"action":"ban","ids":[3,4]}`)))
	require.Contains(t, logged.String(), `"module":"users"`)
	require.Contains(t, logged.String(), `"action":"bulk_ban"`)
	require.Contains(t, logged.String(), `"request_body":"{\"action\":\"ban\",\"ids\":[3,4]}"`)

	logged.Reset()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v4/admin/invite-codes/bulk", strings.NewReader(`{"action":"revoke","ids":[1]}`)))
	require.Contains(t, logged.String(), `"module":"invite-codes"`)
	require.Contains(t, logged.String(), `"action":"create"`, "without a named action the method's applies")
}

// RecordAuditItems writes the rows a bulk request's items would have left had
// they been requested alone, with the administrator, address and request id
// of the bulk request.
func TestRecordAuditItems(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	require.NoError(t, database.GetDB().AutoMigrate(&model.AuditLog{}))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v4/admin/users/bulk", nil)
	c.Request.RemoteAddr = "198.51.100.7:4711"
	c.Request.Header.Set("User-Agent", "console/1")
	c.Request.Header.Set("X-Request-ID", "req-1")
	c.Set("user_id", uint(1))
	c.Set("email", "admin@example.test")

	RecordAuditItems(c, nil)
	RecordAuditItems(c, []AuditItem{
		{Method: http.MethodPost, Path: "/api/v2/admin/users/12/ban", Status: http.StatusOK},
		{Method: http.MethodPost, Path: "/api/v2/admin/users/13/ban", Status: http.StatusNotFound, Error: "not_found: 用户不存在"},
		{Method: http.MethodDelete, Path: "/api/v2/admin/invite/codes/5", Status: http.StatusOK},
	})

	var rows []model.AuditLog
	require.NoError(t, database.GetDB().Order("id").Find(&rows).Error)
	require.Len(t, rows, 3)
	require.NotNil(t, rows[0].UserID)
	require.Equal(t, uint(1), *rows[0].UserID)
	for _, row := range rows {
		require.Equal(t, "admin@example.test", row.Email)
		require.Equal(t, "198.51.100.7", row.IP)
		require.Equal(t, "console/1", row.UserAgent)
		require.Equal(t, "req-1", row.RequestID)
		require.Empty(t, row.RequestBody)
	}
	require.Equal(t, "POST", rows[0].Method)
	require.Equal(t, "/api/v2/admin/users/12/ban", rows[0].Path)
	require.Equal(t, "users", rows[0].Module)
	require.Equal(t, "ban", rows[0].Action)
	require.Equal(t, 200, rows[0].StatusCode)
	require.Empty(t, rows[0].ErrorMessage)
	require.Equal(t, 404, rows[1].StatusCode)
	require.Equal(t, "not_found: 用户不存在", rows[1].ErrorMessage)
	require.Equal(t, "invite", rows[2].Module)
	require.Equal(t, "delete", rows[2].Action)
}
