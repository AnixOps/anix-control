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
