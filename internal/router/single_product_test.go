package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/stretchr/testify/require"
)

func serveV2Request(router http.Handler, method, path string) *httptest.ResponseRecorder {
	body := strings.NewReader("")
	if method == http.MethodPost || method == http.MethodPut {
		body = strings.NewReader("{}")
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// An /api/v2 path that matches no route answers with the gateway's
// unknown-route body.
func TestUnknownV2RouteAnswersAsUndeclared(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer teardownTestRouter(t)
	unknown := serveV2Request(router, http.MethodGet, "/api/v2/no/such/route")
	require.Equal(t, http.StatusNotFound, unknown.Code)
	require.JSONEq(t, `{"error":{"code":"package_route_not_found","message":"package route is not declared"}}`, unknown.Body.String())
}

// There are no product editions: the plan purchase list, order and payment
// routes reach their packages like every other route.
func TestOrderPaymentAndAffiliateRoutesReachTheirPackages(t *testing.T) {
	for _, test := range []struct {
		path    string
		routeID string
	}{
		{path: "/api/v2/user/plan", routeID: "plan.user.plan.get"},
	} {
		t.Run(test.routeID, func(t *testing.T) {
			router, cfg, host := setupV2PackageRouter(t)
			response := requestV2(t, router, cfg, http.MethodGet, test.path)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, test.routeID, host.lastRouteID)
		})
	}
}

// The administrator's invite codes reach identity-platform.
func TestAdministratorInviteCodesReachIdentityPlatform(t *testing.T) {
	router, cfg, host := setupV2PackageRouter(t)
	for _, route := range []struct{ method, path, routeID string }{
		{http.MethodGet, "/api/v2/admin/invite/codes", "identity.admin.invite.codes.get"},
		{http.MethodPost, "/api/v2/admin/invite/codes", "identity.admin.invite.codes.post"},
		{http.MethodDelete, "/api/v2/admin/invite/codes/3", "identity.admin.invite.codes.id.delete"},
	} {
		host.lastRouteID = ""
		response := requestV2Admin(t, router, cfg, route.method, route.path)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Equal(t, route.routeID, host.lastRouteID)
	}
}

// A user's reset of their own subscription link reaches identity-platform.
func TestUsersSubscriptionResetReachesIdentityPlatform(t *testing.T) {
	router, cfg, host := setupV2PackageRouter(t)
	token, err := utils.GenerateToken(2, "member@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	response := requestV2WithToken(t, router, http.MethodPost, "/api/v2/user/subscription/reset", token)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "identity.user.subscription.reset.post", host.lastRouteID)
}

// The public configuration reports the community edition and hides the
// commercial packages.
func TestPublicConfigHidesTheCommercialSet(t *testing.T) {
	router, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	response := serveV2Request(router, http.MethodGet, "/api/v4/public/config")
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"data":{"edition":"community","hidden_packages":["affiliate","order","payment"],"registration":{"enabled":true,"require_invite":false}}}`, response.Body.String())

	cfg.Auth.Registration.RequireInvite = true
	response = serveV2Request(router, http.MethodGet, "/api/v4/public/config")
	require.Contains(t, response.Body.String(), `"require_invite":true`)
}
