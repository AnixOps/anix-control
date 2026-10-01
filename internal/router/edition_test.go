package router

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/edition"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var routeParameter = regexp.MustCompile(`:[A-Za-z_]+`)

func concreteV2Path(pattern string) string {
	return routeParameter.ReplaceAllString(pattern, "1")
}

func serveEditionRequest(t *testing.T, router *gin.Engine, cfg *config.Config, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if method == http.MethodPost || method == http.MethodPut {
		body = strings.NewReader("{}")
	} else {
		body = strings.NewReader("")
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	_ = cfg
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// The routes the community edition hides: every route of the commercial
// packages and the user's plan list, and nothing else.
func TestCommunityEditionHidesExactlyTheCommercialRoutes(t *testing.T) {
	community := edition.New(config.EditionCommunity)
	hidden := community.HiddenRoutes()
	require.NotEmpty(t, hidden)
	require.Equal(t, []string{"affiliate", "order", "payment"}, community.HiddenPackages())
	require.Contains(t, hidden, "GET /api/v2/user/plan")
	require.Contains(t, hidden, "POST /api/v2/payment/stripe/webhook")
	require.Contains(t, hidden, "POST /api/v2/payment/x402/callback")
	require.Contains(t, hidden, "POST /api/v2/payment/callback/:type")
	for _, kept := range []string{
		"GET /api/v2/admin/plans", "POST /api/v2/admin/plans", "GET /api/v2/admin/plans/:id",
		"PUT /api/v2/admin/plans/:id", "DELETE /api/v2/admin/plans/:id", "POST /api/v2/admin/plans/:id/assign",
		"GET /api/v2/user/subscription", "GET /api/v2/user/profile",
		// Registration keeps its invite codes (auth.registration.require_invite).
		"POST /api/v2/register", "POST /api/v2/login",
	} {
		require.NotContains(t, hidden, kept)
	}
	for _, key := range hidden {
		method, path, _ := strings.Cut(key, " ")
		require.True(t, community.HidesRequest(method, path), key)
	}

	commercial := edition.New(config.EditionCommercial)
	require.Empty(t, commercial.HiddenRoutes())
	require.Empty(t, commercial.HiddenPackages())
}

// In the community edition every commercial route answers exactly as an
// /api/v2 route that does not exist, with or without credentials; in the
// commercial edition none of them does.
func TestCommunityEditionAnswersCommercialRoutesAsUnknown(t *testing.T) {
	hidden := edition.New(config.EditionCommunity).HiddenRoutes()

	router, cfg := setupTestRouterWithEdition(t, config.EditionCommunity)
	unknown := serveEditionRequest(t, router, cfg, http.MethodGet, "/api/v2/no/such/route")
	require.Equal(t, http.StatusNotFound, unknown.Code)
	require.JSONEq(t, `{"error":{"code":"package_route_not_found","message":"package route is not declared"}}`, unknown.Body.String())

	for _, key := range hidden {
		method, pattern, _ := strings.Cut(key, " ")
		path := concreteV2Path(pattern)
		response := serveEditionRequest(t, router, cfg, method, path)
		require.Equal(t, unknown.Code, response.Code, key)
		require.Equal(t, unknown.Body.String(), response.Body.String(), key)
		require.Equal(t, unknown.Header().Get("Content-Type"), response.Header().Get("Content-Type"), key)

		withToken := requestV2(t, router, cfg, method, path)
		require.Equal(t, unknown.Body.String(), withToken.Body.String(), key)
	}

	// Subscription templates stay: the plan administration routes are
	// served (here they stop at authentication).
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v2/admin/plans"},
		{http.MethodPost, "/api/v2/admin/plans"},
		{http.MethodPut, "/api/v2/admin/plans/1"},
		{http.MethodPost, "/api/v2/admin/plans/1/assign"},
	} {
		response := serveEditionRequest(t, router, cfg, route.method, route.path)
		require.NotEqual(t, unknown.Body.String(), response.Body.String(), route.path)
	}
	teardownTestRouter(t)

	router, cfg = setupTestRouterWithEdition(t, config.EditionCommercial)
	defer teardownTestRouter(t)
	for _, key := range hidden {
		method, pattern, _ := strings.Cut(key, " ")
		response := serveEditionRequest(t, router, cfg, method, concreteV2Path(pattern))
		require.NotEqual(t, unknown.Body.String(), response.Body.String(), key)
	}
}

// Through the package hosts: community hides the user's plan list, order
// and payment routes before they reach a package; commercial serves them.
func TestEditionDecidesWhetherCommercialRoutesReachTheirPackages(t *testing.T) {
	for _, test := range []struct {
		path    string
		routeID string
	}{
		{path: "/api/v2/user/plan", routeID: "plan.user.plan.get"},
		{path: "/api/v2/user/order", routeID: "order.user.order.get"},
		{path: "/api/v2/payment/methods", routeID: "payment.payment.methods.get"},
	} {
		t.Run(test.routeID, func(t *testing.T) {
			router, cfg, host := setupV2PackageRouterWithEdition(t, config.EditionCommunity)
			response := requestV2(t, router, cfg, http.MethodGet, test.path)
			require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "package_route_not_found")
			require.Empty(t, host.lastRouteID)
			// A community route next to them is still served.
			require.Equal(t, http.StatusOK, requestV2(t, router, cfg, http.MethodGet, "/api/v2/user/subscription").Code)

			router, cfg, host = setupV2PackageRouterWithEdition(t, config.EditionCommercial)
			response = requestV2(t, router, cfg, http.MethodGet, test.path)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, test.routeID, host.lastRouteID)
		})
	}
}

func TestPublicConfigReportsTheEdition(t *testing.T) {
	for _, test := range []struct {
		edition string
		want    string
	}{
		{config.EditionCommunity, `{"data":{"edition":"community","hidden_packages":["affiliate","order","payment"],"registration":{"enabled":true,"require_invite":false}}}`},
		{config.EditionCommercial, `{"data":{"edition":"commercial","hidden_packages":[],"registration":{"enabled":true,"require_invite":false}}}`},
	} {
		router, cfg := setupTestRouterWithEdition(t, test.edition)
		response := serveEditionRequest(t, router, cfg, http.MethodGet, "/api/v4/public/config")
		require.Equal(t, http.StatusOK, response.Code)
		require.JSONEq(t, test.want, response.Body.String())
		teardownTestRouter(t)
	}

	router, cfg := setupTestRouterWithEdition(t, config.EditionCommunity)
	defer teardownTestRouter(t)
	cfg.Auth.Registration.RequireInvite = true
	response := serveEditionRequest(t, router, cfg, http.MethodGet, "/api/v4/public/config")
	require.Contains(t, response.Body.String(), `"require_invite":true`)
}
