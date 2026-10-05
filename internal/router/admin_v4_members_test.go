package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GET /api/v4/admin/subscription-groups/:id/members is for administrators,
// pages the users granted the group directly, and refuses what it cannot
// read: an id that is not one (400), an unknown group (404), a status other
// than active or expired (400).
func TestSetup_AdminV4SubscriptionGroupMembers(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&model.SubscriptionGroup{}, &model.UserSubscriptionGroup{}, &model.Plan{}))
	require.NoError(t, db.Create(&model.SubscriptionGroup{ID: 1, Name: "premium"}).Error)
	granted := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for id, email := range map[uint]string{1: "ann@example.test", 2: "bob@example.test"} {
		require.NoError(t, db.Create(&model.User{ID: id, Email: email, Token: "secret-token-" + email, UUID: "secret-uuid-" + email}).Error)
		require.NoError(t, db.Create(&model.UserSubscriptionGroup{UserID: id, GroupID: 1, CreatedAt: granted.Add(time.Duration(id) * time.Hour)}).Error)
	}

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
	member, err := utils.GenerateToken(2, "member@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	const path = "/api/v4/admin/subscription-groups/1/members"
	assert.Equal(t, http.StatusUnauthorized, get("", path).Code)
	assert.Equal(t, http.StatusForbidden, get(member, path).Code)

	answer := get(admin, path+"?page_size=1&page=1&status=active")
	require.Equal(t, http.StatusOK, answer.Code, answer.Body.String())
	assert.Equal(t, "no-store", answer.Header().Get("Cache-Control"))
	assert.NotContains(t, answer.Body.String(), "secret-")
	var body struct {
		Data struct {
			Total    int64            `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
			Members  []map[string]any `json:"members"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(answer.Body.Bytes(), &body))
	assert.EqualValues(t, 2, body.Data.Total)
	assert.Equal(t, 1, body.Data.Page)
	assert.Equal(t, 1, body.Data.PageSize)
	require.Len(t, body.Data.Members, 1)
	assert.Equal(t, "bob@example.test", body.Data.Members[0]["email"])
	assert.Equal(t, true, body.Data.Members[0]["active"])

	searched := get(admin, path+"?q=ann")
	require.Equal(t, http.StatusOK, searched.Code)
	require.NoError(t, json.Unmarshal(searched.Body.Bytes(), &body))
	require.Len(t, body.Data.Members, 1)
	assert.Equal(t, "ann@example.test", body.Data.Members[0]["email"])

	for path, want := range map[string]int{
		"/api/v4/admin/subscription-groups/x/members":                http.StatusBadRequest,
		"/api/v4/admin/subscription-groups/0/members":                http.StatusBadRequest,
		"/api/v4/admin/subscription-groups/99/members":               http.StatusNotFound,
		"/api/v4/admin/subscription-groups/1/members?status=enabled": http.StatusBadRequest,
	} {
		assert.Equal(t, want, get(admin, path).Code, path)
	}
	assert.Contains(t, get(admin, "/api/v4/admin/subscription-groups/99/members").Body.String(), "not_found")
	assert.Contains(t, get(admin, "/api/v4/admin/subscription-groups/1/members?status=enabled").Body.String(), "invalid_request")
}
