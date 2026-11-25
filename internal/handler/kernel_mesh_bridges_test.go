package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMeshBridgesAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(model.MeshModels()...))
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	handler := &MeshHandler{db: func() *gorm.DB { return env.db }, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.POST("/mesh/regions", handler.CreateRegion)
	router.POST("/mesh/regions/:id/members", handler.AddMember)
	router.GET("/mesh/bridges", handler.ListMeshBridges)
	router.POST("/mesh/bridges", handler.CreateMeshBridge)
	router.PUT("/mesh/bridges/:bridge_id", handler.UpdateMeshBridge)
	router.DELETE("/mesh/bridges/:bridge_id", handler.DeleteMeshBridge)

	features.SetVersion("5.0.13")
	response := serveJSON(router, http.MethodGet, "/mesh/bridges", "")
	assert.Equal(t, "feature_disabled", errorCode(t, response))
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })

	response = serveJSON(router, http.MethodPost, "/mesh/regions", `{"code":"cn","name":"中国大陆","cidr":"10.150.0.0/16","enabled":true}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var region model.MeshRegion
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &region))
	response = serveJSON(router, http.MethodPost, "/mesh/regions/"+strconv.FormatUint(uint64(region.ID), 10)+"/members", `{"node":"forward-5"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())

	response = serveJSON(router, http.MethodPost, "/mesh/bridges", `{"node":"forward-5","interface":"eth1","routes":["8.8.8.8"]}`)
	assert.Equal(t, "invalid_request", errorCode(t, response))
	response = serveJSON(router, http.MethodPost, "/mesh/bridges", `{"node":"forward-5","interface":"eth1","routes":["10.150.1.0/24"]}`)
	assert.Equal(t, "mesh_conflict", errorCode(t, response))
	response = serveJSON(router, http.MethodPost, "/mesh/bridges", `{"node":"forward-5","interface":"eth1","routes":"x"}`)
	assert.Equal(t, "invalid_request", errorCode(t, response))
	response = serveJSON(router, http.MethodPost, "/mesh/bridges", `{"node":"forward-5","interface":"ext0","routes":["100.100.1.2","192.168.50.0/24"],"allow":["node:forward-5"]}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var bridge model.MeshExternalBridge
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &bridge))
	path := "/mesh/bridges/" + strconv.FormatUint(uint64(bridge.ID), 10)

	response = serveJSON(router, http.MethodGet, "/mesh/bridges", "")
	require.Equal(t, http.StatusOK, response.Code)
	var views []meshregion.BridgeView
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &views))
	require.Len(t, views, 1)
	assert.Equal(t, []string{"100.100.1.2/32", "192.168.50.0/24"}, views[0].Routes)
	assert.Equal(t, []string{"10.150.0.1/32"}, views[0].Expanded)
	assert.Equal(t, "cn", views[0].RegionCode)
	assert.Equal(t, []string{meshregion.WarningBridgeUnconfirmed}, views[0].Warnings)

	response = serveJSON(router, http.MethodPut, path, `{"interface":"eth2","routes":["192.168.60.0/24"],"allow":[]}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"interface":"eth2"`)
	response = serveJSON(router, http.MethodPut, "/mesh/bridges/99", `{"interface":"eth2","routes":["192.168.60.0/24"]}`)
	assert.Equal(t, "not_found", errorCode(t, response))
	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodDelete, path, "").Code)
	assert.Equal(t, "not_found", errorCode(t, serveJSON(router, http.MethodDelete, path, "")))

	var actions []string
	require.NoError(t, env.db.Model(&model.OperationLog{}).Where("module = ? AND action LIKE ?", "mesh", "mesh_bridge_%").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"mesh_bridge_create", "mesh_bridge_update", "mesh_bridge_delete"}, actions)
}
