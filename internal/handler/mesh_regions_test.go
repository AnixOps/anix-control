package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newMeshRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mesh.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model.MeshModels()...))
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	mesh := &MeshHandler{db: func() *gorm.DB { return db }, now: func() time.Time { return now }}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)) })
	router.GET("/mesh", mesh.Overview)
	router.PUT("/mesh/settings", mesh.UpdateSettings)
	router.POST("/mesh/regions", mesh.CreateRegion)
	router.PUT("/mesh/regions/:id", mesh.UpdateRegion)
	router.DELETE("/mesh/regions/:id", mesh.DeleteRegion)
	router.GET("/mesh/members", mesh.Members)
	router.POST("/mesh/members", mesh.CreateMember)
	router.PUT("/mesh/members/:id", mesh.UpdateMember)
	router.DELETE("/mesh/members/:id", mesh.DeleteMember)
	router.POST("/mesh/members/:id/move", mesh.MoveMember)
	router.GET("/mesh/host-groups", mesh.HostGroups)
	router.POST("/mesh/host-groups", mesh.CreateHostGroup)
	router.POST("/mesh/device-names", mesh.CreateDeviceName)
	router.GET("/mesh/names", mesh.Names)
	router.GET("/mesh/region-changes", mesh.Changes)
	router.POST("/mesh/region-changes/:id/approve", mesh.ApproveChange)
	router.POST("/mesh/region-changes/:id/reject", mesh.RejectChange)
	return router
}

func TestMeshAPI(t *testing.T) {
	router := newMeshRouter(t)

	response := serveJSON(router, http.MethodGet, "/mesh", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var overview struct {
		Settings struct {
			NameSuffix string `json:"name_suffix"`
		} `json:"settings"`
		Regions []struct {
			ID       uint     `json:"id"`
			Code     string   `json:"code"`
			Security string   `json:"security"`
			Warnings []string `json:"warnings"`
		} `json:"regions"`
		PendingChanges int `json:"pending_changes"`
	}
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &overview))
	assert.Equal(t, "mesh.internal", overview.Settings.NameSuffix)
	require.Len(t, overview.Regions, 4)
	assert.Equal(t, "forward_secrecy", overview.Regions[0].Security)
	assert.NotContains(t, response.Body.String(), "network_key", "the network key is never answered")
	eu := overview.Regions[2].ID

	for body, want := range map[string]struct {
		status int
		code   string
	}{
		`{"code":"cn","name":"x","subnet":"10.125.0.0/16","server_range":"10.125.0.0/20","device_pool":"10.125.64.0/18"}`: {http.StatusConflict, "conflict"},
		`{"code":"me","name":"x","subnet":"8.8.0.0/16","server_range":"8.8.0.0/20","device_pool":"8.8.64.0/18"}`:          {http.StatusBadRequest, "invalid_request"},
		`{"code":5}`: {http.StatusBadRequest, "invalid_request"},
	} {
		response = serveJSON(router, http.MethodPost, "/mesh/regions", body)
		assert.Equal(t, want.status, response.Code, body)
		assert.Equal(t, want.code, errorCode(t, response), body)
	}
	response = serveJSON(router, http.MethodPut, "/mesh/regions/404", `{"name":"x","subnet":"10.125.0.0/16","server_range":"10.125.0.0/20","device_pool":"10.125.64.0/18"}`)
	assert.Equal(t, http.StatusNotFound, response.Code)
	response = serveJSON(router, http.MethodDelete, "/mesh/regions/x", "")
	assert.Equal(t, http.StatusBadRequest, response.Code)

	response = serveJSON(router, http.MethodPost, "/mesh/members", fmt.Sprintf(`{"region_id":%d,"hostname":"gw","role":"gateway","node_ref":"proxy-5"}`, eu))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var gateway struct {
		ID      uint   `json:"id"`
		Address string `json:"address"`
	}
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &gateway))
	assert.Equal(t, "10.123.0.1", gateway.Address)
	response = serveJSON(router, http.MethodPost, "/mesh/members", fmt.Sprintf(`{"region_id":%d,"hostname":"phone","role":"device"}`, eu))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var phone struct {
		ID uint `json:"id"`
	}
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &phone))

	response = serveJSON(router, http.MethodGet, fmt.Sprintf("/mesh/members?region_id=%d", eu), "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"fqdn":"gw.eu.mesh.internal"`)
	response = serveJSON(router, http.MethodGet, "/mesh/members?region_id=x", "")
	assert.Equal(t, http.StatusBadRequest, response.Code)

	response = serveJSON(router, http.MethodPost, "/mesh/host-groups", fmt.Sprintf(`{"name":"宿主 A","gateway_member_id":%d}`, gateway.ID))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var group struct {
		ID      uint   `json:"id"`
		Segment string `json:"segment"`
	}
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &group))
	assert.Equal(t, "10.127.0.0/24", group.Segment)
	response = serveJSON(router, http.MethodPost, "/mesh/device-names", fmt.Sprintf(`{"host_group_id":%d,"name":"vm-1","address":"10.127.0.10"}`, group.ID))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	response = serveJSON(router, http.MethodGet, "/mesh/names", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"fqdn":"vm-1.eu.mesh.internal"`)

	response = serveJSON(router, http.MethodDelete, fmt.Sprintf("/mesh/members/%d", gateway.ID), "")
	assert.Equal(t, http.StatusConflict, response.Code, "a host group's gateway stays")

	response = serveJSON(router, http.MethodPost, fmt.Sprintf("/mesh/members/%d/move", phone.ID), fmt.Sprintf(`{"region_id":%d}`, overview.Regions[1].ID))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var moved struct {
		Status      string `json:"status"`
		Source      string `json:"source"`
		NewAddress  string `json:"new_address"`
		DecidedBy   uint   `json:"decided_by"`
		RequestedBy uint   `json:"requested_by"`
	}
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &moved))
	assert.Equal(t, "approved", moved.Status)
	assert.Equal(t, "admin", moved.Source)
	assert.Equal(t, "10.122.64.0", moved.NewAddress)
	assert.EqualValues(t, 7, moved.DecidedBy)

	response = serveJSON(router, http.MethodGet, "/mesh/region-changes?status=approved", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"hostname":"phone"`)
	response = serveJSON(router, http.MethodPost, "/mesh/region-changes/1/approve", "")
	assert.Equal(t, http.StatusConflict, response.Code, "a decided change stays decided")
	response = serveJSON(router, http.MethodPost, "/mesh/region-changes/99/reject", `{"note":"x"}`)
	assert.Equal(t, http.StatusNotFound, response.Code)

	response = serveJSON(router, http.MethodPut, "/mesh/settings", `{"name_suffix":"corp.internal","host_group_supernet":"10.127.0.0/16","host_group_prefix":24}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = serveJSON(router, http.MethodPut, "/mesh/settings", `{"name_suffix":"bad suffix","host_group_supernet":"10.127.0.0/16","host_group_prefix":24}`)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}
