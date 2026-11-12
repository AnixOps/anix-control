package e2e

import (
	"bytes"
	"encoding/json"
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

// bulkResult is the data of a bulk answer.
type bulkResult struct {
	Action    string `json:"action"`
	Requested int    `json:"requested"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Results   []struct {
		ID    uint `json:"id"`
		OK    bool `json:"ok"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"results"`
}

// The invite code bulk endpoint repeats the single-item /api/v2 route per id
// through the real package gateway and the identity package's bridge, as the
// route's own request does.
func TestBulkActionsRunTheSingleItemRoutesThroughThePackageGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	cfg := &config.Config{
		Env:      "test",
		Database: config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"},
		JWT:      config.JWTConfig{Secret: "bulk-e2e-test-secret", Expire: 3600},
		App:      config.AppConfig{SubscribePath: "s"},
	}
	config.Set(cfg)
	require.NoError(t, database.Init(&cfg.Database))
	t.Cleanup(func() { requireDatabaseClosed(t) })
	db := database.Get()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.InviteCode{}, &model.AuditLog{}))
	restoreHost := installIdentityPlatformE2EPackage(t, cfg)
	t.Cleanup(restoreHost)

	require.NoError(t, db.Create(&model.User{ID: 1, Email: "bulk1@example.test", Token: "tok1", UUID: "uuid1", IsAdmin: 1}).Error)
	engine := gin.New()
	router.Setup(engine, cfg)
	admin := authntest.Token(t, 1, "bulk1@example.test", true)

	post := func(token, path, body string) (*httptest.ResponseRecorder, bulkResult) {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		var answer struct {
			Data bulkResult `json:"data"`
		}
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
		return recorder, answer.Data
	}

	recorder, _ := post("", "/api/v4/admin/invite-codes/bulk", `{"action":"revoke","ids":[1]}`)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	// Invite codes: an unused code is revoked, a used one kept, a missing one
	// reported.
	require.NoError(t, db.Create(&[]model.InviteCode{{ID: 1, Code: "FREE0001"}, {ID: 2, Code: "USED0002", Status: 1}}).Error)
	recorder, result := post(admin, "/api/v4/admin/invite-codes/bulk", `{"action":"revoke","ids":[1,2,3]}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, result.Succeeded)
	require.Equal(t, "conflict", result.Results[1].Error.Code)
	require.Equal(t, "not_found", result.Results[2].Error.Code)
	var remaining []string
	require.NoError(t, db.Model(&model.InviteCode{}).Order("id").Pluck("code", &remaining).Error)
	require.Equal(t, []string{"USED0002"}, remaining)
}
