package middleware

import (
	"bytes"
	"io"
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
