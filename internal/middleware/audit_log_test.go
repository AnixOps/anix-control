package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
