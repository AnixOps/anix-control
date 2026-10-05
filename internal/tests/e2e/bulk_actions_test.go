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

// The bulk endpoints repeat the single-item /api/v2 route per id through the
// real package gateway and the identity package's bridge, as the route's own
// request does: the users are banned and reset in the database, a retry with
// the same Idempotency-Key resets once, a missing id is reported, the caller
// is not banned, and each item leaves the audit row of its single-item route.
func TestBulkActionsRunTheSingleItemRoutesThroughThePackageGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	cfg := &config.Config{
		Env:      "test",
		Database: config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"},
		JWT:      config.JWTConfig{Secret: "bulk-e2e-test-secret", Expire: 3600},
		App:      config.AppConfig{APIToken: "bulk-e2e-api-token", SubscribePath: "s"},
	}
	config.Set(cfg)
	require.NoError(t, database.Init(&cfg.Database))
	t.Cleanup(func() { requireDatabaseClosed(t) })
	db := database.Get()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.InviteCode{}, &model.AuditLog{}, &model.WireGuardPeer{}))
	restoreHost := installIdentityPlatformE2EPackage(t, cfg)
	t.Cleanup(restoreHost)

	for id := uint(1); id <= 4; id++ {
		require.NoError(t, db.Create(&model.User{ID: id, Email: fmt.Sprintf("bulk%d@example.test", id), Token: fmt.Sprintf("tok%d", id), UUID: fmt.Sprintf("uuid%d", id), U: 50, D: 70, IsAdmin: int(boolToInt(id == 1))}).Error)
	}
	engine := gin.New()
	router.Setup(engine, cfg)
	admin, err := utils.GenerateToken(1, "bulk1@example.test", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	member, err := utils.GenerateToken(2, "bulk2@example.test", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	post := func(token, path, body string, headers ...string) (*httptest.ResponseRecorder, bulkResult) {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		for i := 0; i+1 < len(headers); i += 2 {
			request.Header.Set(headers[i], headers[i+1])
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		var answer struct {
			Data bulkResult `json:"data"`
		}
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
		return recorder, answer.Data
	}
	banned := func(id uint) int {
		var user model.User
		require.NoError(t, db.First(&user, id).Error)
		return user.Banned
	}
	used := func(id uint) int64 {
		var user model.User
		require.NoError(t, db.First(&user, id).Error)
		return user.U + user.D
	}

	recorder, _ := post("", "/api/v4/admin/users/bulk", `{"action":"ban","ids":[2]}`)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	recorder, _ = post(member, "/api/v4/admin/users/bulk", `{"action":"ban","ids":[3]}`)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Zero(t, banned(3))

	// Ban: users 2 and 3 are banned, 99 is not a user, 1 is the caller.
	recorder, result := post(admin, "/api/v4/admin/users/bulk", `{"action":"ban","ids":[2,3,99,1]}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 4, result.Requested)
	require.Equal(t, 2, result.Succeeded)
	require.Equal(t, 2, result.Failed)
	require.Equal(t, 1, banned(2))
	require.Equal(t, 1, banned(3))
	require.Zero(t, banned(1))
	require.True(t, result.Results[0].OK)
	require.Equal(t, uint(99), result.Results[2].ID)
	require.Equal(t, "not_found", result.Results[2].Error.Code, recorder.Body.String())
	require.Equal(t, "forbidden_self", result.Results[3].Error.Code)

	// Banning again is not a failure.
	_, result = post(admin, "/api/v4/admin/users/bulk", `{"action":"ban","ids":[2,3]}`)
	require.Equal(t, 2, result.Succeeded)

	_, result = post(admin, "/api/v4/admin/users/bulk", `{"action":"unban","ids":[2]}`)
	require.Equal(t, 1, result.Succeeded)
	require.Zero(t, banned(2))
	require.Equal(t, 1, banned(3))

	// Reset traffic: applied once per Idempotency-Key and user.
	_, result = post(admin, "/api/v4/admin/users/bulk", `{"action":"reset_traffic","ids":[3,4]}`, "Idempotency-Key", "reset-1")
	require.Equal(t, 2, result.Succeeded, "%+v", result)
	require.Zero(t, used(3))
	require.Zero(t, used(4))
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", []uint{3, 4}).Updates(map[string]any{"u": 5, "d": 6}).Error)
	_, result = post(admin, "/api/v4/admin/users/bulk", `{"action":"reset_traffic","ids":[3,4]}`, "Idempotency-Key", "reset-1")
	require.Equal(t, 2, result.Succeeded)
	require.EqualValues(t, 11, used(3), "a retry with the same key resets nothing again")
	_, result = post(admin, "/api/v4/admin/users/bulk", `{"action":"reset_traffic","ids":[3]}`, "Idempotency-Key", "reset-2")
	require.Equal(t, 1, result.Succeeded)
	require.Zero(t, used(3), "a new key is a new reset")
	require.EqualValues(t, 11, used(4))

	// Each item left the audit row of its single-item route, tied to the
	// bulk request by the request id.
	var rows []model.AuditLog
	require.NoError(t, db.Where("path = ?", "/api/v2/admin/users/99/ban").Find(&rows).Error)
	require.NotEmpty(t, rows)
	require.Equal(t, "users", rows[0].Module)
	require.Equal(t, "ban", rows[0].Action)
	require.Equal(t, http.StatusNotFound, rows[0].StatusCode)
	require.Contains(t, rows[0].ErrorMessage, "not_found")
	require.NoError(t, db.Where("path = ? AND status_code = ?", "/api/v2/admin/users/2/ban", http.StatusOK).Find(&rows).Error)
	require.NotEmpty(t, rows)
	require.Equal(t, uint(1), *rows[0].UserID)

	// Invite codes: an unused code is revoked, a used one kept, a missing one
	// reported.
	require.NoError(t, db.Create(&[]model.InviteCode{{ID: 1, Code: "FREE0001"}, {ID: 2, Code: "USED0002", Status: 1}}).Error)
	_, result = post(admin, "/api/v4/admin/invite-codes/bulk", `{"action":"revoke","ids":[1,2,3]}`)
	require.Equal(t, 1, result.Succeeded)
	require.Equal(t, "conflict", result.Results[1].Error.Code)
	require.Equal(t, "not_found", result.Results[2].Error.Code)
	var remaining []string
	require.NoError(t, db.Model(&model.InviteCode{}).Order("id").Pluck("code", &remaining).Error)
	require.Equal(t, []string{"USED0002"}, remaining)
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
