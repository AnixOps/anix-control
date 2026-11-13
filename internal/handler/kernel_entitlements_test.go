package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/entitlement"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEntitlementsAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.SystemConfig{}, &model.EntitlementDevice{}, &model.Plan{}))
	require.NoError(t, env.db.Create(&model.Plan{ID: 3, Name: "pro"}).Error)
	now := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	kek := ""
	handler := &EntitlementsHandler{db: func() *gorm.DB { return env.db }, kek: func() string { return kek }, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.GET("/entitlements/:feature", handler.Get)
	router.PUT("/entitlements/:feature", handler.Set)
	router.GET("/entitlements/:feature/devices", handler.Devices)
	router.POST("/entitlements/:feature/devices/:id/revoke", handler.Revoke)
	router.POST("/entitlements/:feature/devices/:id/restore", handler.Restore)

	type view struct {
		Enabled   bool                     `json:"enabled"`
		Require   bool                     `json:"require"`
		TrialDays int                      `json:"trial_days"`
		PlanIDs   []uint                   `json:"plan_ids"`
		CA        string                   `json:"ca"`
		RootKey   bool                     `json:"root_key"`
		Devices   entitlement.DeviceCounts `json:"devices"`
	}
	read := func(body string) view {
		var answer struct{ Data view }
		require.NoError(t, json.Unmarshal([]byte(body), &answer), body)
		return answer.Data
	}

	// Off by default, no root key.
	response := serveJSON(router, http.MethodGet, "/entitlements/anx", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	got := read(response.Body.String())
	assert.False(t, got.Enabled)
	assert.Equal(t, entitlement.DefaultTrialDays, got.TrialDays)
	assert.Equal(t, []uint{}, got.PlanIDs)
	assert.False(t, got.RootKey)
	assert.Empty(t, got.CA)
	require.Equal(t, http.StatusNotFound, serveJSON(router, http.MethodGet, "/entitlements/other", "").Code)

	for _, bad := range []string{`{}`, `[]`, `{"trial_days":31}`, `{"trial_days":-1}`, `{"plan_ids":[0]}`} {
		response = serveJSON(router, http.MethodPut, "/entitlements/anx", bad)
		require.Equal(t, http.StatusBadRequest, response.Code, bad)
	}
	response = serveJSON(router, http.MethodPut, "/entitlements/anx", `{"plan_ids":[3,404]}`)
	require.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "plan_not_found")

	kek = strings.Repeat("42", 32)
	response = serveJSON(router, http.MethodPut, "/entitlements/anx", `{"enabled":true,"trial_days":0,"plan_ids":[3,3]}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	got = read(response.Body.String())
	assert.True(t, got.Enabled)
	assert.False(t, got.Require)
	assert.Equal(t, 0, got.TrialDays)
	assert.Equal(t, []uint{3}, got.PlanIDs)
	assert.True(t, got.RootKey)
	assert.Equal(t, "c83c22218d9d871e9eb6196777b1ee9715a34420d42646c09e6bbedf3687de26", got.CA)
	settings, err := entitlement.LoadSettings(env.db)
	require.NoError(t, err)
	assert.Equal(t, entitlement.Settings{Enabled: true, TrialDays: 0, PlanIDs: []uint{3}}, settings)

	// A partial change keeps the rest; [] is every plan again.
	response = serveJSON(router, http.MethodPut, "/entitlements/anx", `{"require":true,"plan_ids":[]}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	got = read(response.Body.String())
	assert.True(t, got.Enabled)
	assert.True(t, got.Require)
	assert.Equal(t, []uint{}, got.PlanIDs)

	var audits []model.OperationLog
	require.NoError(t, env.db.Where("action = ?", "entitlement_settings_set").Order("id").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.JSONEq(t, `{"feature":"anx","enabled":true,"require":false,"trial_days":0,"plan_ids":[3]}`, audits[0].Content)

	// Devices: one seated, one gone quiet, one of another user.
	devices := []model.EntitlementDevice{
		{UserID: 7, Feature: "anx", DeviceID: "0101010101010101", Alg: "ed25519", PublicKey: "aa", NotAfter: now.Add(7 * 24 * time.Hour).Unix(), CreatedAt: now.Add(-time.Hour), LastSeenAt: now.Add(-time.Hour)},
		{UserID: 7, Feature: "anx", DeviceID: "0202020202020202", Alg: "p256", PublicKey: "bb", Trial: true, CreatedAt: now.Add(-60 * 24 * time.Hour), LastSeenAt: now.Add(-40 * 24 * time.Hour)},
		{UserID: 8, Feature: "anx", DeviceID: "0303030303030303", Alg: "ed25519", PublicKey: "cc", CreatedAt: now.Add(-2 * time.Hour), LastSeenAt: now.Add(-2 * time.Hour)},
	}
	require.NoError(t, env.db.Create(&devices).Error)

	type device struct {
		ID       uint   `json:"id"`
		UserID   uint   `json:"user_id"`
		Email    string `json:"email"`
		DeviceID string `json:"device_id"`
		Revoked  bool   `json:"revoked"`
		Seated   bool   `json:"seated"`
	}
	type page struct {
		Items []device `json:"items"`
		Total int64    `json:"total"`
	}
	list := func(query string) page {
		response := serveJSON(router, http.MethodGet, "/entitlements/anx/devices"+query, "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var answer struct{ Data page }
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer))
		return answer.Data
	}
	all := list("")
	require.Equal(t, int64(3), all.Total)
	require.Len(t, all.Items, 3)
	assert.Equal(t, "0101010101010101", all.Items[0].DeviceID)
	assert.Equal(t, "root@example.com", all.Items[0].Email)
	assert.True(t, all.Items[0].Seated)
	assert.Equal(t, "staff@example.com", all.Items[1].Email)
	assert.False(t, all.Items[2].Seated)
	mine := list("?user_id=7&page=2&page_size=1")
	require.Equal(t, int64(2), mine.Total)
	require.Len(t, mine.Items, 1)
	assert.Equal(t, "0202020202020202", mine.Items[0].DeviceID)
	for _, bad := range []string{"?user_id=x", "?page=0", "?page_size=101"} {
		require.Equal(t, http.StatusBadRequest, serveJSON(router, http.MethodGet, "/entitlements/anx/devices"+bad, "").Code, bad)
	}

	response = serveJSON(router, http.MethodGet, "/entitlements/anx", "")
	assert.Equal(t, entitlement.DeviceCounts{Total: 3, Seated: 2, Revoked: 0}, read(response.Body.String()).Devices)

	// Revoke frees the seat; a second revoke changes nothing; restore lifts it.
	id := devices[0].ID
	response = serveJSON(router, http.MethodPost, "/entitlements/anx/devices/"+strconv.FormatUint(uint64(id), 10)+"/revoke", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"changed":true`)
	assert.Contains(t, response.Body.String(), `"revoked":true`)
	response = serveJSON(router, http.MethodPost, "/entitlements/anx/devices/"+strconv.FormatUint(uint64(id), 10)+"/revoke", "")
	assert.Contains(t, response.Body.String(), `"changed":false`)
	response = serveJSON(router, http.MethodGet, "/entitlements/anx", "")
	assert.Equal(t, entitlement.DeviceCounts{Total: 3, Seated: 1, Revoked: 1}, read(response.Body.String()).Devices)

	response = serveJSON(router, http.MethodPost, "/entitlements/anx/devices/"+strconv.FormatUint(uint64(id), 10)+"/restore", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"revoked":false`)
	require.Equal(t, http.StatusNotFound, serveJSON(router, http.MethodPost, "/entitlements/anx/devices/999/revoke", "").Code)
	require.Equal(t, http.StatusBadRequest, serveJSON(router, http.MethodPost, "/entitlements/anx/devices/x/revoke", "").Code)

	require.NoError(t, env.db.Where("module = ? AND action LIKE ?", "entitlement", "entitlement_device_%").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, "entitlement_device_revoke", audits[0].Action)
	assert.JSONEq(t, `{"feature":"anx","device_id":"0101010101010101","user_id":7}`, audits[0].Content)
}
