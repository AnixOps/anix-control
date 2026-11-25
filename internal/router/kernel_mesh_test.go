package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/authn/authntest"
	"github.com/stretchr/testify/assert"
)

// Every mesh route is an administrator's: a request without a session is
// unauthorized and a user's is forbidden, before the feature gate.
func TestKernelMeshRoutesAreAdminOnly(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)
	user := authntest.Token(t, 2, "user@example.com", false)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v4/kernel/mesh/regions"},
		{http.MethodPost, "/api/v4/kernel/mesh/regions"},
		{http.MethodPut, "/api/v4/kernel/mesh/settings"},
		{http.MethodGet, "/api/v4/kernel/mesh/devices"},
		{http.MethodPost, "/api/v4/kernel/mesh/devices/1/address"},
		{http.MethodPut, "/api/v4/kernel/mesh/devices/1/subnets"},
		{http.MethodPost, "/api/v4/kernel/mesh/changes"},
		{http.MethodPost, "/api/v4/kernel/mesh/changes/1/approve"},
		{http.MethodGet, "/api/v4/kernel/mesh/names"},
		{http.MethodPost, "/api/v4/kernel/mesh/enrollments"},
		{http.MethodPost, "/api/v4/kernel/mesh/regions/1/rotate-secret"},
		{http.MethodPost, "/api/v4/kernel/mesh/devices/1/revoke"},
	} {
		for token, want := range map[string]int{"": http.StatusUnauthorized, user: http.StatusForbidden} {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			if token != "" {
				request.Header.Set("Authorization", "Bearer "+token)
			}
			recorder := httptest.NewRecorder()
			r.ServeHTTP(recorder, request)
			assert.Equal(t, want, recorder.Code, "%s %s", route.method, route.path)
		}
	}
}
