package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/router"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The administrator's invite codes are registration control: in both
// editions identity-platform serves them through the identity bridge, and a
// newly generated code admits a registration that requires one. The
// commissions, withdrawals and invite statistics stay hidden in community.
func TestAdminInviteCodesAdmitRegistrationInEveryEdition(t *testing.T) {
	for index, edition := range []string{config.EditionCommunity, config.EditionCommercial} {
		// Registration is rate limited per client address, across subtests.
		remoteAddr := fmt.Sprintf("192.0.2.%d:1234", 10+index)
		t.Run(edition, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cache.InitMemory()
			cfg := &config.Config{
				Env:      "test",
				Database: config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"},
				JWT:      config.JWTConfig{Secret: "invite-codes-e2e-secret", Expire: 3600},
				App:      config.AppConfig{APIToken: "invite-codes-e2e-api-token", SubscribePath: "s", Edition: edition},
			}
			cfg.Auth.Registration.RequireInvite = true
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
			token, err := utils.GenerateToken(admin.ID, admin.Email, true, cfg.JWT.Secret, cfg.JWT.Expire)
			require.NoError(t, err)
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
				request.RemoteAddr = remoteAddr
				request.Header.Set("Content-Type", "application/json")
				if bearer != "" {
					request.Header.Set("Authorization", "Bearer "+bearer)
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				return response
			}

			// Registration needs a code.
			refused := serve(http.MethodPost, "/api/v2/register", map[string]string{"email": "nocode@example.test", "password": "password123"}, "")
			requirePanelErrorResponse(t, refused.Code, refused.Body.Bytes(), "invite code is required")

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
			userToken, err := utils.GenerateToken(admin.ID, admin.Email, false, cfg.JWT.Secret, cfg.JWT.Expire)
			require.NoError(t, err)
			require.Equal(t, http.StatusForbidden, serve(http.MethodPost, "/api/v2/admin/invite/codes", nil, userToken).Code)

			// The new code admits a registration and is then used.
			registered := serve(http.MethodPost, "/api/v2/register", map[string]string{
				"email": "invited@example.test", "password": "password123", "invite_code": first["code"].(string),
			}, "")
			require.Equal(t, http.StatusOK, registered.Code, registered.Body.String())
			require.Equal(t, "invited@example.test", requirePanelDataMap(t, registered.Body.Bytes())["email"])
			again := serve(http.MethodPost, "/api/v2/register", map[string]string{
				"email": "again@example.test", "password": "password123", "invite_code": first["code"].(string),
			}, "")
			requirePanelErrorResponse(t, again.Code, again.Body.Bytes(), "invalid or used invite code")

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
			late := serve(http.MethodPost, "/api/v2/register", map[string]string{
				"email": "late@example.test", "password": "password123", "invite_code": second["code"].(string),
			}, "")
			requirePanelErrorResponse(t, late.Code, late.Body.Bytes(), "invalid or used invite code")

			// Commissions, withdrawals, invite statistics and configuration
			// stay commercial: community answers them as undeclared routes.
			if edition == config.EditionCommunity {
				unknown := serve(http.MethodGet, "/api/v2/no/such/route", nil, token)
				require.Equal(t, http.StatusNotFound, unknown.Code)
				for _, route := range []struct{ method, path string }{
					{http.MethodGet, "/api/v2/admin/invite/stats"},
					{http.MethodGet, "/api/v2/admin/invite/withdrawals"},
					{http.MethodPost, "/api/v2/admin/invite/withdrawals/1/process"},
					{http.MethodGet, "/api/v2/admin/invite/config"},
					{http.MethodPut, "/api/v2/admin/invite/config"},
					{http.MethodGet, "/api/v2/user/invite"},
					{http.MethodPost, "/api/v2/user/invite/generate"},
					{http.MethodGet, "/api/v2/user/invite/commissions"},
				} {
					response := serve(route.method, route.path, nil, token)
					require.Equal(t, unknown.Code, response.Code, route.path)
					require.Equal(t, unknown.Body.String(), response.Body.String(), route.path)
				}
			}
		})
	}
}
