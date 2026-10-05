package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GET /api/v4/admin/users/activity is for administrators, answers the last
// time each asked-for user was seen online (null for one never seen), in the
// order asked, and refuses ids it cannot read.
func TestSetup_AdminV4UserActivity(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	require.NoError(t, service.EnsureKernelSchema(db))
	for _, id := range []uint{1, 2, 3} {
		require.NoError(t, db.Create(&model.User{ID: id, Email: fmt.Sprintf("activity%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("u%d", id)}).Error)
	}
	seen := time.Unix(1_800_000_000, 0)
	subscriber.RecordOnline(db, []uint{2}, seen)

	get := func(token, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	admin, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	member, err := utils.GenerateToken(3, "member@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, get("", "/api/v4/admin/users/activity?ids=2").Code)
	assert.Equal(t, http.StatusForbidden, get(member, "/api/v4/admin/users/activity?ids=2").Code)

	answer := get(admin, "/api/v4/admin/users/activity?ids=3,2,2,1")
	require.Equal(t, http.StatusOK, answer.Code, answer.Body.String())
	assert.Equal(t, "no-store", answer.Header().Get("Cache-Control"))
	var body struct {
		Data struct {
			Users []map[string]any `json:"users"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(answer.Body.Bytes(), &body))
	assert.Equal(t, []map[string]any{
		{"user_id": float64(3), "last_online_at": nil},
		{"user_id": float64(2), "last_online_at": float64(seen.Unix())},
		{"user_id": float64(1), "last_online_at": nil},
	}, body.Data.Users)

	for _, path := range []string{
		"/api/v4/admin/users/activity", "/api/v4/admin/users/activity?ids=", "/api/v4/admin/users/activity?ids=1,x",
		"/api/v4/admin/users/activity?ids=0", "/api/v4/admin/users/activity?ids=-1",
	} {
		refused := get(admin, path)
		assert.Equal(t, http.StatusBadRequest, refused.Code, path)
		assert.Contains(t, refused.Body.String(), "invalid_request", path)
	}
}
