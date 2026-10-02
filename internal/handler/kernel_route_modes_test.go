package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/routemodefixture"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newRouteModeHandlerFixture(t *testing.T) (*RouteModeHandler, *gorm.DB) {
	t.Helper()
	db := newKernelHandlerTestDB(t, &model.User{}, &model.OperationLog{})
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "root@example.com", Token: "t1", UUID: "u1", IsAdmin: 1},
		{ID: 2, Email: "staff@example.com", Token: "t2", UUID: "u2", IsAdmin: 1, IsStaff: 1},
	}).Error)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	routemodefixture.Install(t, db, privateKey, "knowledge", []routemodefixture.Route{
		routemodefixture.HTTP("GET", "/api/v2/user/knowledge", "knowledge.article.list"),
		routemodefixture.HTTP("POST", "/api/v2/admin/knowledge", "knowledge.admin.knowledge.post"),
		routemodefixture.HTTP("GET", "/api/v2/admin/knowledge/bridged", "knowledge.bridged.get"),
		routemodefixture.HTTP("GET", "/api/v2/admin/knowledge/kernel", "knowledge.kernel.get"),
	})
	groupA := []routemodefixture.Route{routemodefixture.HTTP("GET", "/api/v2/admin/invite-codes", "identity.admin.invite_codes.get")}
	catalog := map[string]service.RouteCatalogEntry{
		"knowledge.article.list":          {PackageID: "knowledge", Mode: service.RouteCatalogNativeFlagged},
		"knowledge.admin.knowledge.post":  {PackageID: "knowledge", Mode: service.RouteCatalogNativeFlagged},
		"knowledge.bridged.get":           {PackageID: "knowledge", Mode: service.RouteCatalogBridged},
		"knowledge.kernel.get":            {PackageID: "knowledge", Mode: service.RouteCatalogKernelOwned},
		"identity.admin.invite_codes.get": {PackageID: service.IdentityPlatformPackageID, Mode: service.RouteCatalogNativeFlagged},
	}
	for i, route := range service.IdentityGroupARoutes {
		groupA = append(groupA, routemodefixture.HTTP("POST", "/api/v2/group-a/"+string(rune('a'+i)), route))
		catalog[route] = service.RouteCatalogEntry{PackageID: service.IdentityPlatformPackageID, Mode: service.RouteCatalogNativeFlagged}
	}
	routemodefixture.Install(t, db, privateKey, service.IdentityPlatformPackageID, groupA)
	handler := &RouteModeHandler{
		db:        func() *gorm.DB { return db },
		publicKey: func() (ed25519.PublicKey, error) { return publicKey, nil },
		hosts: func() []pluginhost.HostStats {
			return []pluginhost.HostStats{{PackageID: "knowledge", HealthDetailsJSON: `{"routes":{"knowledge.article.list":{"mode":"shadow","effective":"shadow","shadow_total":4,"shadow_mismatch":1}}}`}}
		},
		catalog: catalog,
	}
	return handler, db
}

func routeModeRequest(t *testing.T, handler gin.HandlerFunc, method, path, route, body string, userID uint) (int, map[string]any) {
	t.Helper()
	recorder := performKernelHandlerRequestWithSetup(t, method, path, body, route, handler, func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("is_admin", true)
	})
	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return recorder.Code, response
}

func routeModeErrorCode(response map[string]any) string {
	if failure, ok := response["error"].(map[string]any); ok {
		code, _ := failure["code"].(string)
		return code
	}
	return ""
}

func TestRouteModeHandlerSwitchesOnlyForSuperAdmins(t *testing.T) {
	handler, db := newRouteModeHandlerFixture(t)
	set := func(body string, userID uint) (int, map[string]any) {
		return routeModeRequest(t, handler.Set, http.MethodPost, "/route-modes", "/route-modes", body, userID)
	}
	rollback := func(body string, userID uint) (int, map[string]any) {
		return routeModeRequest(t, handler.Rollback, http.MethodPost, "/route-modes/rollback", "/route-modes/rollback", body, userID)
	}

	// Any administrator reads; can_switch tells the page whether it may switch.
	code, response := routeModeRequest(t, handler.List, http.MethodGet, "/route-modes?package_id=knowledge", "/route-modes", "", 2)
	require.Equal(t, http.StatusOK, code, response)
	data := response["data"].(map[string]any)
	require.Equal(t, false, data["can_switch"])
	packages := data["packages"].([]any)
	require.Len(t, packages, 1)
	routes := packages[0].(map[string]any)["routes"].([]any)
	require.Len(t, routes, 4)
	var article map[string]any
	for _, route := range routes {
		if entry := route.(map[string]any); entry["route_id"] == "knowledge.article.list" {
			article = entry
		}
	}
	require.NotNil(t, article)
	require.EqualValues(t, 1, article["host"].(map[string]any)["shadow_mismatch"])
	code, response = routeModeRequest(t, handler.List, http.MethodGet, "/route-modes", "/route-modes", "", 1)
	require.Equal(t, http.StatusOK, code, response)
	require.Equal(t, true, response["data"].(map[string]any)["can_switch"])
	code, response = routeModeRequest(t, handler.List, http.MethodGet, "/route-modes?package_id=ticket", "/route-modes", "", 1)
	require.Equal(t, http.StatusNotFound, code, response)

	// A staff administrator, or an unknown user, may not switch.
	for _, userID := range []uint{2, 99} {
		code, response = set(`{"package_id":"knowledge","mode":"shadow","routes":["knowledge.article.list"]}`, userID)
		require.Equal(t, http.StatusForbidden, code, response)
		require.Equal(t, "super_admin_required", routeModeErrorCode(response))
		code, response = rollback(`{"package_id":"knowledge"}`, userID)
		require.Equal(t, http.StatusForbidden, code, response)
	}

	// Native needs confirm and a reason.
	for _, body := range []string{
		`{"package_id":"knowledge","mode":"native","routes":["knowledge.article.list"]}`,
		`{"package_id":"knowledge","mode":"native","routes":["knowledge.article.list"],"confirm":true}`,
		`{"package_id":"knowledge","mode":"native","routes":["knowledge.article.list"],"reason":"batch 1"}`,
	} {
		code, response = set(body, 1)
		require.Equal(t, http.StatusBadRequest, code, response)
		require.Equal(t, "route_mode_confirmation_required", routeModeErrorCode(response))
	}

	rejected := []string{
		`{"package_id":"knowledge","mode":"native","routes":["knowledge.bridged.get"],"confirm":true,"reason":"x"}`,
		`{"package_id":"knowledge","mode":"legacy","routes":["knowledge.kernel.get"]}`,
		`{"package_id":"knowledge","mode":"shadow","routes":["knowledge.admin.knowledge.post"]}`,
	}
	for _, body := range rejected {
		code, response = set(body, 1)
		require.Equal(t, http.StatusBadRequest, code, response)
		require.Equal(t, "route_mode_rejected", routeModeErrorCode(response), body)
	}
	code, response = set(`{"mode":"legacy"}`, 1)
	require.Equal(t, http.StatusBadRequest, code, response)
	require.Equal(t, "invalid_request", routeModeErrorCode(response))

	code, response = set(`{"package_id":"knowledge","mode":"native","routes":["knowledge.article.list"],"confirm":true,"reason":"shadow clean for 48h"}`, 1)
	require.Equal(t, http.StatusOK, code, response)
	require.Len(t, response["data"].(map[string]any)["changes"], 1)
	code, response = set(`{"package_id":"knowledge","mode":"native","confirm":true,"reason":"whole package"}`, 1)
	require.Equal(t, http.StatusOK, code, response)
	require.Len(t, response["data"].(map[string]any)["changes"], 1, "knowledge.admin.knowledge.post")

	code, response = rollback(`{"package_id":"knowledge"}`, 1)
	require.Equal(t, http.StatusOK, code, response)
	require.Len(t, response["data"].(map[string]any)["changes"], 2)
	code, response = rollback(`{"package_id":"ticket"}`, 1)
	require.Equal(t, http.StatusNotFound, code, response)

	code, response = routeModeRequest(t, handler.Revisions, http.MethodGet, "/route-modes/revisions?package_id=knowledge", "/route-modes/revisions", "", 2)
	require.Equal(t, http.StatusOK, code, response)
	revisions := response["data"].(map[string]any)["revisions"].([]any)
	require.Len(t, revisions, 4)
	latest := revisions[0].(map[string]any)
	require.Equal(t, "rollback", latest["action"])
	require.Equal(t, "root@example.com", latest["actor"])
	require.EqualValues(t, 1, latest["actor_user_id"])
	code, _ = routeModeRequest(t, handler.Revisions, http.MethodGet, "/route-modes/revisions?limit=x", "/route-modes/revisions", "", 1)
	require.Equal(t, http.StatusBadRequest, code)

	var audits []model.OperationLog
	require.NoError(t, db.Order("id").Find(&audits).Error)
	require.Len(t, audits, 3)
	for _, audit := range audits {
		require.Equal(t, "kernel", audit.Module)
		require.Equal(t, "root@example.com", audit.Username)
	}
	require.Equal(t, "route_mode_set", audits[0].Action)
	require.Contains(t, audits[0].Content, "shadow clean for 48h")
	require.Equal(t, "route_mode_rollback", audits[2].Action)
}

func TestRouteModeHandlerRejectsPartialIdentityGroupA(t *testing.T) {
	handler, _ := newRouteModeHandlerFixture(t)
	code, response := routeModeRequest(t, handler.Set, http.MethodPost, "/route-modes", "/route-modes",
		`{"package_id":"identity-platform","mode":"native","routes":["`+service.IdentityGroupARoutes[0]+`"],"confirm":true,"reason":"x"}`, 1)
	require.Equal(t, http.StatusBadRequest, code, response)
	require.Equal(t, "route_mode_rejected", routeModeErrorCode(response))
	require.Contains(t, response["error"].(map[string]any)["message"], "identity cutover")

	// The whole package switches everything but group A.
	code, response = routeModeRequest(t, handler.Set, http.MethodPost, "/route-modes", "/route-modes",
		`{"package_id":"identity-platform","mode":"native","confirm":true,"reason":"x"}`, 1)
	require.Equal(t, http.StatusOK, code, response)
	data := response["data"].(map[string]any)
	require.Len(t, data["changes"], 1)
	require.Len(t, data["skipped"], len(service.IdentityGroupARoutes))
}

func TestRouteModeHandlerNeedsTheTrustRootToSwitch(t *testing.T) {
	handler, _ := newRouteModeHandlerFixture(t)
	handler.publicKey = func() (ed25519.PublicKey, error) { return nil, service.ErrPluginTrustRootRequired }
	code, response := routeModeRequest(t, handler.Rollback, http.MethodPost, "/route-modes/rollback", "/route-modes/rollback", `{"package_id":"knowledge"}`, 1)
	require.Equal(t, http.StatusServiceUnavailable, code, response)
	require.Equal(t, "plugin_trust_root_unconfigured", routeModeErrorCode(response))
}
