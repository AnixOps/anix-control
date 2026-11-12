package router

import (
	"github.com/AnixOps/anix-control/v5/internal/authn/authntest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every bulk action repeats a single-item route the router serves, and both
// bulk endpoints are administrator-only.
func TestSetup_AdminV4BulkEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	registered := map[string]bool{}
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	routes := handler.BulkActionRoutes()
	require.NotEmpty(t, routes)
	for _, route := range routes {
		assert.True(t, registered[route.Method+" "+route.Pattern], "%s %s is the route of %s on %s", route.Method, route.Pattern, route.Action, route.Endpoint)
		assert.True(t, registered["POST "+route.Endpoint], route.Endpoint)
	}

	member := authntest.Token(t, 2, "member@example.com", false)
	for _, endpoint := range []string{"/api/v4/admin/users/bulk", "/api/v4/admin/invite-codes/bulk"} {
		request := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"action":"ban","ids":[1]}`))
		unauthenticated := httptest.NewRecorder()
		r.ServeHTTP(unauthenticated, request)
		assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code, endpoint)

		request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"action":"ban","ids":[1]}`))
		request.Header.Set("Authorization", "Bearer "+member)
		forbidden := httptest.NewRecorder()
		r.ServeHTTP(forbidden, request)
		assert.Equal(t, http.StatusForbidden, forbidden.Code, endpoint)
	}
}
