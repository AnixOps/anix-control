package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/AnixOps/anix-control/v5/internal/authn/authntest"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The administrator's invite codes: identity-platform serves them through
// the identity bridge.
func TestAdminInviteCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	cfg := &config.Config{
		Env:      "test",
		Database: config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"},
		JWT:      config.JWTConfig{Secret: "invite-codes-e2e-secret", Expire: 3600},
		App:      config.AppConfig{SubscribePath: "s"},
	}
	config.Set(cfg)
	require.NoError(t, database.Init(&cfg.Database))
	t.Cleanup(func() { requireDatabaseClosed(t) })
	require.NoError(t, database.Get().AutoMigrate(&model.User{}, &model.InviteCode{}, &model.InviteConfig{}))
	admin := model.User{Email: "admin@example.test", Password: "unused", Token: "admin-token", UUID: "admin-uuid", IsAdmin: 1}
	require.NoError(t, database.Get().Create(&admin).Error)

	restoreHost := installIdentityPlatformE2EPackage(t, cfg)
	t.Cleanup(restoreHost)
	engine := gin.New()
	router.Setup(engine, cfg)
	router.SetupNotFound(engine)
	token := authntest.Token(t, admin.ID, admin.Email, true)
	serve := func(method, path string, body any, bearer string) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(raw)
		}
		request := httptest.NewRequest(method, path, reader)
		request.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}

	// Generate two codes that never expire.
	generated := serve(http.MethodPost, "/api/v2/admin/invite/codes", map[string]int{"count": 2, "expire_days": 0}, token)
	require.Equal(t, http.StatusOK, generated.Code, generated.Body.String())
	codes, ok := requirePanelDataMap(t, generated.Body.Bytes())["codes"].([]any)
	require.True(t, ok)
	require.Len(t, codes, 2)
	first := codes[0].(map[string]any)
	second := codes[1].(map[string]any)
	require.Len(t, first["code"], 8)
	require.Nil(t, first["user_id"])
	require.Nil(t, first["expired_at"])

	// Bad requests answer a panel error.
	tooMany := serve(http.MethodPost, "/api/v2/admin/invite/codes", map[string]int{"count": 51}, token)
	requirePanelErrorResponse(t, tooMany.Code, tooMany.Body.Bytes(), "count must be between 1 and 50")
	badFilter := serve(http.MethodGet, "/api/v2/admin/invite/codes?status=bogus", nil, token)
	requirePanelErrorResponse(t, badFilter.Code, badFilter.Body.Bytes(), "invalid invite code status filter")
	badID := serve(http.MethodDelete, "/api/v2/admin/invite/codes/abc", nil, token)
	requirePanelErrorResponse(t, badID.Code, badID.Body.Bytes(), "invalid invite code id")

	// An unauthenticated or non-admin caller cannot generate.
	require.Equal(t, http.StatusUnauthorized, serve(http.MethodPost, "/api/v2/admin/invite/codes", nil, "").Code)
	userToken := authntest.Token(t, admin.ID, admin.Email, false)
	require.Equal(t, http.StatusForbidden, serve(http.MethodPost, "/api/v2/admin/invite/codes", nil, userToken).Code)

	// A registration uses the first code (identitycompat covers the
	// registration itself).
	require.NoError(t, database.Get().Model(&model.InviteCode{}).Where("code = ?", first["code"]).Updates(map[string]any{"status": 1, "used_by": admin.ID}).Error)

	// The list shows the used and the unused code.
	used := serve(http.MethodGet, "/api/v2/admin/invite/codes?status=used", nil, token)
	require.Equal(t, http.StatusOK, used.Code, used.Body.String())
	usedData := requirePanelDataMap(t, used.Body.Bytes())
	require.EqualValues(t, 1, usedData["total"])
	usedCode := usedData["list"].([]any)[0].(map[string]any)
	require.Equal(t, first["code"], usedCode["code"])
	require.EqualValues(t, 1, usedCode["status"])
	require.NotNil(t, usedCode["used_by"])
	all := requirePanelDataMap(t, serve(http.MethodGet, "/api/v2/admin/invite/codes", nil, token).Body.Bytes())
	require.EqualValues(t, 2, all["total"])

	// A used code cannot be revoked; an unused one can, and then no
	// longer admits a registration.
	usedID := fmt.Sprintf("%v", usedCode["id"])
	revokeUsed := serve(http.MethodDelete, "/api/v2/admin/invite/codes/"+usedID, nil, token)
	requirePanelErrorResponse(t, revokeUsed.Code, revokeUsed.Body.Bytes(), "invite code already used")
	revoked := serve(http.MethodDelete, fmt.Sprintf("/api/v2/admin/invite/codes/%v", second["id"]), nil, token)
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	requirePanelDataMap(t, revoked.Body.Bytes())
}
