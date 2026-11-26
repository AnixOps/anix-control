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

func TestMeshPathsAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(model.MeshModels()...))
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	handler := &MeshHandler{db: func() *gorm.DB { return env.db }, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.POST("/mesh/regions", handler.CreateRegion)
	router.POST("/mesh/regions/:id/members", handler.AddMember)
	router.GET("/mesh/paths", handler.ListMeshPaths)
	router.POST("/mesh/paths", handler.CreateMeshPath)
	router.PUT("/mesh/paths/:path_id", handler.UpdateMeshPath)
	router.DELETE("/mesh/paths/:path_id", handler.DeleteMeshPath)

	features.SetVersion("5.0.13")
	response := serveJSON(router, http.MethodGet, "/mesh/paths", "")
	assert.Equal(t, "feature_disabled", errorCode(t, response))
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })

	response = serveJSON(router, http.MethodPost, "/mesh/regions", `{"code":"cn","name":"中国大陆","cidr":"10.150.0.0/16","enabled":true}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var region model.MeshRegion
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &region))
	for _, node := range []string{"forward-5", "proxy-5"} {
		response = serveJSON(router, http.MethodPost, "/mesh/regions/"+strconv.FormatUint(uint64(region.ID), 10)+"/members", `{"node":"`+node+`"}`)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	}

	response = serveJSON(router, http.MethodPost, "/mesh/paths", `{"source":"node:forward-5","target":"node:proxy-5","via":[]}`)
	assert.Equal(t, "invalid_request", errorCode(t, response))
	response = serveJSON(router, http.MethodPost, "/mesh/paths", `{"source":"node:forward-5","target":"node:proxy-5","via":"x"}`)
	assert.Equal(t, "invalid_request", errorCode(t, response))
	response = serveJSON(router, http.MethodPost, "/mesh/paths", `{"source":"node:forward-5","target":"node:proxy-5","via":["node:forward-5"]}`)
	assert.Equal(t, "invalid_request", errorCode(t, response))

	response = serveJSON(router, http.MethodPost, "/mesh/paths", `{"source":"node:forward-5","target":"192.168.7.0/24","via":["node:proxy-5"]}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var created model.MeshPath
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &created))
	path := "/mesh/paths/" + strconv.FormatUint(uint64(created.ID), 10)
	response = serveJSON(router, http.MethodPost, "/mesh/paths", `{"source":"node:forward-5","target":"192.168.7.0/24","via":["node:proxy-5"]}`)
	assert.Equal(t, "mesh_conflict", errorCode(t, response))

	response = serveJSON(router, http.MethodGet, "/mesh/paths", "")
	require.Equal(t, http.StatusOK, response.Code)
	var views []meshregion.PathView
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &views))
	require.Len(t, views, 1)
	assert.Equal(t, "node:forward-5", views[0].Source)
	assert.Equal(t, "cn", views[0].RegionCode)
	assert.Equal(t, meshregion.PathTargetSubnet, views[0].TargetKind)
	require.Len(t, views[0].Via, 1)
	assert.Equal(t, "10.150.0.2", views[0].Via[0].Address)
	assert.Empty(t, views[0].Warnings)
	assert.Nil(t, views[0].Status)

	response = serveJSON(router, http.MethodPut, path, `{"source":"node:proxy-5","target":"192.168.7.0/24","via":["node:proxy-5"]}`)
	assert.Equal(t, "invalid_request", errorCode(t, response), "the source never changes")
	response = serveJSON(router, http.MethodPut, path, `{"target":"192.168.8.0/24","via":["node:proxy-5"]}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"target":"192.168.8.0/24"`)
	response = serveJSON(router, http.MethodPut, "/mesh/paths/99", `{"target":"192.168.8.0/24","via":["node:proxy-5"]}`)
	assert.Equal(t, "not_found", errorCode(t, response))
	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodDelete, path, "").Code)
	assert.Equal(t, "not_found", errorCode(t, serveJSON(router, http.MethodDelete, path, "")))

	var actions []string
	require.NoError(t, env.db.Model(&model.OperationLog{}).Where("module = ? AND action LIKE ?", "mesh", "mesh_path_%").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"mesh_path_create", "mesh_path_update", "mesh_path_delete"}, actions)
}
