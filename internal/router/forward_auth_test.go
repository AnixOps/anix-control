package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRouterJWT(t *testing.T, secret string, isAdmin bool) string {
	t.Helper()

	token, err := utils.GenerateToken(1, "auth-test@example.com", isAdmin, secret, 3600)
	require.NoError(t, err)
	return token
}

func TestSetup_ForwardCompatAdminEndpoints_RequireAdmin(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)

	userToken := testRouterJWT(t, cfg.JWT.Secret, false)
	adminToken := testRouterJWT(t, cfg.JWT.Secret, true)

	endpoints := []struct {
		method      string
		path        string
		body        string
		adminStatus int
	}{
		{method: http.MethodPost, path: "/api/v2/user/reset", body: "{}", adminStatus: http.StatusServiceUnavailable},
		{method: http.MethodPost, path: "/api/v2/tunnel/user/list", body: "{}", adminStatus: http.StatusServiceUnavailable},
		{method: http.MethodGet, path: "/api/v2/admin/forward/runtime/status", adminStatus: http.StatusServiceUnavailable},
		{method: http.MethodGet, path: "/api/v2/admin/forward/runtime/doctor", adminStatus: http.StatusServiceUnavailable},
		{method: http.MethodGet, path: "/api/v2/admin/forward/local/status", adminStatus: http.StatusServiceUnavailable},
		{method: http.MethodGet, path: "/api/v2/admin/forward/local/doctor", adminStatus: http.StatusServiceUnavailable},
		{method: http.MethodGet, path: "/api/v2/admin/forward/nodex/status", adminStatus: http.StatusServiceUnavailable},
		{method: http.MethodGet, path: "/api/v2/admin/forward/nodex/doctor", adminStatus: http.StatusServiceUnavailable},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			t.Run("no auth", func(t *testing.T) {
				req, err := http.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
				require.NoError(t, err)
				if ep.method == http.MethodPost {
					req.Header.Set("Content-Type", "application/json")
				}

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("non admin jwt", func(t *testing.T) {
				req, err := http.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+userToken)
				if ep.method == http.MethodPost {
					req.Header.Set("Content-Type", "application/json")
				}

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("admin jwt", func(t *testing.T) {
				req, err := http.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				if ep.method == http.MethodPost {
					req.Header.Set("Content-Type", "application/json")
				}

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				expected := ep.adminStatus
				if expected == 0 {
					expected = http.StatusOK
				}
				assert.Equal(t, expected, w.Code)
			})
		})
	}
}

func TestSetup_ForwardUserEndpoints_RequireJWT(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)

	userToken := testRouterJWT(t, cfg.JWT.Secret, false)
	adminToken := testRouterJWT(t, cfg.JWT.Secret, true)

	endpoints := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v2/forward/list"},
		{method: http.MethodPost, path: "/api/v2/tunnel/user/tunnel"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			t.Run("no auth", func(t *testing.T) {
				req, err := http.NewRequest(ep.method, ep.path, nil)
				require.NoError(t, err)

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("user jwt", func(t *testing.T) {
				req, err := http.NewRequest(ep.method, ep.path, nil)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+userToken)

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusServiceUnavailable, w.Code)
			})

			t.Run("admin jwt", func(t *testing.T) {
				req, err := http.NewRequest(ep.method, ep.path, nil)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+adminToken)

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusServiceUnavailable, w.Code)
			})
		})
	}
}

// The flux forwarding API was removed in v4.2 (F5d): its v2 routes are not
// registered, and /api/v4/forward/* replaces them.
func TestFluxForwardRoutesAreRemoved(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	adminToken := testRouterJWT(t, cfg.JWT.Secret, true)

	for _, ep := range []struct{ method, path string }{
		{http.MethodPost, "/api/v2/admin/forward/create"},
		{http.MethodGet, "/api/v2/admin/forward/rules"},
		{http.MethodPost, "/api/v2/admin/forward/sync-backend"},
		{http.MethodGet, "/api/v2/admin/forward/runtime/jobs"},
		{http.MethodPost, "/api/v2/admin/tunnel/update"},
		{http.MethodPost, "/api/v2/forward/create"},
		{http.MethodPost, "/api/v2/speed-limit/update"},
		{http.MethodPost, "/api/v2/tunnel/user/remove"},
		{http.MethodPost, "/api/v2/user/forward/rules"},
		{http.MethodGet, "/api/v2/admin/forward/nodes"},
		{http.MethodGet, "/api/v2/admin/forward/ansible-machines"},
		{http.MethodGet, "/api/v2/admin/forward/observability/topology"},
		{http.MethodGet, "/api/v2/admin/forward/agents"},
		{http.MethodGet, "/api/v2/forward-agent/install.sh"},
	} {
		req, err := http.NewRequest(ep.method, ep.path, strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code, "%s %s", ep.method, ep.path)
	}
}
