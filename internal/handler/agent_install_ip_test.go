package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /install/ip answers the caller's address as gin's ClientIP sees it: the
// connection's, or the forwarded one from a trusted proxy only.
func TestInstallClientIP(t *testing.T) {
	handler := &AgentInstallHandler{}
	serve := func(trusted []string, remote, forwarded string) *httptest.ResponseRecorder {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		policy, err := requestorigin.NewPolicy(trusted)
		require.NoError(t, err)
		require.NoError(t, policy.ApplyTo(router))
		router.GET("/install/ip", handler.ClientIP)
		request := httptest.NewRequest(http.MethodGet, "/install/ip", nil)
		request.RemoteAddr = remote
		if forwarded != "" {
			request.Header.Set("X-Forwarded-For", forwarded)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	direct := serve([]string{}, "203.0.113.9:40000", "")
	require.Equal(t, http.StatusOK, direct.Code)
	assert.Equal(t, "203.0.113.9\n", direct.Body.String())
	assert.Equal(t, "no-store", direct.Header().Get("Cache-Control"))
	assert.True(t, strings.HasPrefix(direct.Header().Get("Content-Type"), "text/plain"))

	assert.Equal(t, "2001:db8::9\n", serve([]string{}, "[2001:db8::9]:40000", "").Body.String())
	assert.Equal(t, "203.0.113.9\n", serve([]string{}, "203.0.113.9:40000", "198.51.100.1").Body.String(), "an untrusted peer cannot claim another address")
	assert.Equal(t, "198.51.100.1\n", serve([]string{"10.0.0.0/8"}, "10.0.0.2:40000", "198.51.100.1").Body.String(), "a trusted proxy's forwarded address")
}
