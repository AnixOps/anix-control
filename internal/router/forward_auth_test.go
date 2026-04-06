package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anixops/v2board/internal/utils"
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
	defer teardownTestRouter()

	userToken := testRouterJWT(t, cfg.JWT.Secret, false)
	adminToken := testRouterJWT(t, cfg.JWT.Secret, true)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v2/user/reset", body: "{}"},
		{method: http.MethodPost, path: "/api/v2/tunnel/user/list", body: "{}"},
		{method: http.MethodGet, path: "/api/v2/admin/forward/runtime/jobs"},
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

				assert.Equal(t, http.StatusOK, w.Code)
			})
		})
	}
}

func TestSetup_ForwardUserEndpoints_RequireJWT(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter()

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

				assert.Equal(t, http.StatusOK, w.Code)
			})

			t.Run("admin jwt", func(t *testing.T) {
				req, err := http.NewRequest(ep.method, ep.path, nil)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+adminToken)

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code)
			})
		})
	}
}
