package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMeshAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.MeshRegion{}, &model.MeshMember{}, &model.MeshStatus{}))
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	handler := &MeshHandler{db: func() *gorm.DB { return env.db }, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.GET("/mesh/regions", handler.ListRegions)
	router.POST("/mesh/regions", handler.CreateRegion)
	router.PUT("/mesh/regions/:id", handler.UpdateRegion)
	router.DELETE("/mesh/regions/:id", handler.DeleteRegion)
	router.POST("/mesh/regions/:id/members", handler.AddMember)
	router.PUT("/mesh/members/:member_id", handler.UpdateMember)
	router.DELETE("/mesh/members/:member_id", handler.RemoveMember)

	features.SetVersion("5.0.13")
	response := serveJSON(router, http.MethodGet, "/mesh/regions", "")
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, "feature_disabled", errorCode(t, response))
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })

	response = serveJSON(router, http.MethodPost, "/mesh/regions", `{"code":"cn","name":"中国大陆","cidr":"10.150.0.0/16","enabled":true}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "secret")
	var region model.MeshRegion
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &region))
	path := "/mesh/regions/" + strconv.FormatUint(uint64(region.ID), 10)

	for body, code := range map[string]string{
		`{"code":"eu","name":"欧洲","cidr":"10.150.0.0/17"}`: "mesh_conflict",
		`{"code":"eu","name":"欧洲","cidr":"8.8.8.0/24"}`:    "invalid_request",
	} {
		response = serveJSON(router, http.MethodPost, "/mesh/regions", body)
		assert.Equal(t, code, errorCode(t, response), body)
	}
	response = serveJSON(router, http.MethodPost, path+"/members", `{"node":"forward-5","boundary":true}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var member model.MeshMember
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &member))
	assert.Equal(t, "10.150.0.1", member.Address)
	response = serveJSON(router, http.MethodPost, path+"/members", `{"node":"forward-5"}`)
	assert.Equal(t, "mesh_conflict", errorCode(t, response))
	response = serveJSON(router, http.MethodPost, path+"/members", `{"node":"proxy-404"}`)
	assert.Equal(t, http.StatusNotFound, response.Code)

	response = serveJSON(router, http.MethodGet, "/mesh/regions", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"node_name":"relay"`)

	memberPath := "/mesh/members/" + strconv.FormatUint(uint64(member.ID), 10)
	response = serveJSON(router, http.MethodPut, memberPath, `{"boundary":false}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "mesh_conflict", errorCode(t, serveJSON(router, http.MethodDelete, path, "")))
	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodDelete, memberPath, "").Code)
	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodDelete, path, "").Code)

	var actions []string
	require.NoError(t, env.db.Model(&model.OperationLog{}).Where("module = ?", "mesh").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"mesh_region_create", "mesh_member_add", "mesh_member_update", "mesh_member_remove", "mesh_region_delete"}, actions)
}
