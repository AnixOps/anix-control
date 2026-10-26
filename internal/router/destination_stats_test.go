package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The administrators read every user's traffic by destination and change the
// settings; a user reads only their own.
func TestSetup_DestinationStats(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&model.DestinationStat{}, &model.SystemConfig{}))
	hour := time.Now().UTC().Truncate(time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.DestinationStat{
		{HourAt: hour, NodeID: 1, UserID: 2, Domain: "youtube.com", Protocol: "tls", Upload: 10, Download: 1000, Connections: 1},
		{HourAt: hour, NodeID: 1, UserID: 3, Domain: "telegram.org", Protocol: "tls", Upload: 5, Download: 5, Connections: 1},
		{HourAt: hour - 48*3600, NodeID: 1, UserID: 3, Domain: "netflix.com", Protocol: "tls", Upload: 1, Download: 1},
	}).Error)

	do := func(method, token, path string, body any) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(raw)
		} else {
			reader = bytes.NewReader(nil)
		}
		request := httptest.NewRequest(method, path, reader)
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	type answer struct {
		Data struct {
			destinationstats.Summary
			Settings destinationstats.Settings `json:"settings"`
		} `json:"data"`
	}
	read := func(recorder *httptest.ResponseRecorder) answer {
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var body answer
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		return body
	}
	admin, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	member, err := utils.GenerateToken(3, "member@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, do(http.MethodGet, "", "/api/v4/admin/traffic/destinations", nil).Code)
	assert.Equal(t, http.StatusForbidden, do(http.MethodGet, member, "/api/v4/admin/traffic/destinations", nil).Code)
	assert.Equal(t, http.StatusForbidden, do(http.MethodPut, member, "/api/v4/admin/traffic/destinations/settings", map[string]any{"enabled": true, "retention_days": 30}).Code)
	assert.Equal(t, http.StatusUnauthorized, do(http.MethodGet, "", "/api/v4/user/traffic/destinations", nil).Code)

	all := read(do(http.MethodGet, admin, "/api/v4/admin/traffic/destinations", nil))
	assert.Equal(t, int64(15), all.Data.Upload)
	assert.Equal(t, 2, all.Data.DomainCount)
	assert.Equal(t, destinationstats.Settings{Enabled: false, RetentionDays: 30}, all.Data.Settings)
	one := read(do(http.MethodGet, admin, "/api/v4/admin/traffic/destinations?user_id=2&node_id=1&limit=1", nil))
	require.Len(t, one.Data.Domains, 1)
	assert.Equal(t, "youtube.com", one.Data.Domains[0].Domain)
	assert.Equal(t, destinationstats.CategoryVideo, one.Data.Domains[0].Category)
	week := read(do(http.MethodGet, admin, "/api/v4/admin/traffic/destinations?hours=168&user_id=3", nil))
	assert.Equal(t, 2, week.Data.DomainCount)
	for _, path := range []string{"?hours=x", "?hours=-1", "?hours=4321", "?limit=501", "?user_id=-2"} {
		refused := do(http.MethodGet, admin, "/api/v4/admin/traffic/destinations"+path, nil)
		assert.Equal(t, http.StatusBadRequest, refused.Code, path)
	}

	own := read(do(http.MethodGet, member, "/api/v4/user/traffic/destinations?user_id=2", nil))
	require.Len(t, own.Data.Domains, 1, "user_id is ignored: a user reads their own")
	assert.Equal(t, "telegram.org", own.Data.Domains[0].Domain)
	assert.Equal(t, destinationstats.CategoryMessaging, own.Data.Domains[0].Category)

	settings := do(http.MethodPut, admin, "/api/v4/admin/traffic/destinations/settings", map[string]any{"enabled": true, "retention_days": 90})
	require.Equal(t, http.StatusOK, settings.Code, settings.Body.String())
	stored, err := destinationstats.LoadSettings(db)
	require.NoError(t, err)
	assert.Equal(t, destinationstats.Settings{Enabled: true, RetentionDays: 90}, stored)
	got := do(http.MethodGet, admin, "/api/v4/admin/traffic/destinations/settings", nil)
	require.Equal(t, http.StatusOK, got.Code)
	assert.JSONEq(t, `{"data":{"enabled":true,"retention_days":90}}`, got.Body.String())
	for _, body := range []any{map[string]any{"enabled": true}, map[string]any{"enabled": true, "retention_days": 3}, "x"} {
		assert.Equal(t, http.StatusBadRequest, do(http.MethodPut, admin, "/api/v4/admin/traffic/destinations/settings", body).Code, body)
	}
}
