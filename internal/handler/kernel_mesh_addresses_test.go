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

func TestMeshAddressesAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(model.MeshModels()...))
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	handler := &MeshHandler{db: func() *gorm.DB { return env.db }, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.POST("/mesh/regions", handler.CreateRegion)
	router.POST("/mesh/regions/:id/members", handler.AddMember)
	router.GET("/mesh/summary", handler.MeshSummary)
	router.PUT("/mesh/settings", handler.SaveMeshSettings)
	router.GET("/mesh/devices", handler.ListMeshDevices)
	router.POST("/mesh/regions/:id/devices", handler.CreateMeshDevice)
	router.PUT("/mesh/devices/:device_id", handler.UpdateMeshDevice)
	router.DELETE("/mesh/devices/:device_id", handler.DeleteMeshDevice)
	router.POST("/mesh/devices/:device_id/move", handler.MoveMeshDevice)
	router.GET("/mesh/host-groups", handler.ListMeshHostGroups)
	router.POST("/mesh/host-groups", handler.CreateMeshHostGroup)
	router.DELETE("/mesh/host-groups/:group_id", handler.DeleteMeshHostGroup)
	router.POST("/mesh/device-names", handler.CreateMeshDeviceName)
	router.GET("/mesh/names", handler.ListMeshNames)
	router.GET("/mesh/changes", handler.ListMeshChanges)
	router.POST("/mesh/changes", handler.CreateMeshChange)
	router.POST("/mesh/changes/:change_id/approve", handler.ApproveMeshChange)
	router.POST("/mesh/changes/:change_id/reject", handler.RejectMeshChange)

	features.SetVersion("5.0.13")
	response := serveJSON(router, http.MethodGet, "/mesh/summary", "")
	assert.Equal(t, "feature_disabled", errorCode(t, response))
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })

	regionID := func(body string) string {
		response := serveJSON(router, http.MethodPost, "/mesh/regions", body)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		var region model.MeshRegion
		require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &region))
		return strconv.FormatUint(uint64(region.ID), 10)
	}
	cn := regionID(`{"code":"cn","name":"中国大陆","cidr":"10.150.0.0/16","enabled":true}`)
	eu := regionID(`{"code":"eu","name":"欧洲","cidr":"10.152.0.0/16","enabled":true}`)
	response = serveJSON(router, http.MethodPost, "/mesh/regions/"+cn+"/members", `{"node":"forward-5"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())

	response = serveJSON(router, http.MethodPut, "/mesh/settings", `{"name_suffix":"mesh.internal","host_group_supernet":"10.150.0.0/16","host_group_prefix":24}`)
	assert.Equal(t, "mesh_conflict", errorCode(t, response))
	response = serveJSON(router, http.MethodPut, "/mesh/settings", `{"name_suffix":"mesh.internal","host_group_supernet":"10.127.0.0/16","host_group_prefix":24,"reserved_ranges":["10.126.0.0/16"]}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = serveJSON(router, http.MethodGet, "/mesh/summary", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"reserved_ranges":["10.126.0.0/16"]`)

	response = serveJSON(router, http.MethodPost, "/mesh/regions/"+cn+"/devices", `{"hostname":"phone"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var device model.MeshDevice
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &device))
	assert.Equal(t, "10.150.128.0", *device.Address)
	devicePath := "/mesh/devices/" + strconv.FormatUint(uint64(device.ID), 10)
	response = serveJSON(router, http.MethodPost, "/mesh/regions/"+cn+"/devices", `{"hostname":"no good"}`)
	assert.Equal(t, "invalid_request", errorCode(t, response))
	response = serveJSON(router, http.MethodPut, devicePath, `{"hostname":"phone-2"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = serveJSON(router, http.MethodGet, "/mesh/devices?region_id="+cn, "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"fqdn":"phone-2.cn.mesh.internal"`)

	response = serveJSON(router, http.MethodPost, "/mesh/host-groups", `{"name":"vms","gateway":"forward-5"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var group model.MeshHostGroup
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &group))
	assert.Equal(t, "10.127.0.0/24", group.Segment)
	response = serveJSON(router, http.MethodPost, "/mesh/device-names", `{"host_group_id":`+strconv.FormatUint(uint64(group.ID), 10)+`,"name":"db","address":"10.127.0.9"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	response = serveJSON(router, http.MethodGet, "/mesh/names", "")
	require.Equal(t, http.StatusOK, response.Code)
	for _, fqdn := range []string{"relay.cn.mesh.internal", "phone-2.cn.mesh.internal", "db.cn.mesh.internal"} {
		assert.Contains(t, response.Body.String(), `"fqdn":"`+fqdn+`"`)
	}

	response = serveJSON(router, http.MethodPost, "/mesh/changes", `{"device_id":`+strconv.FormatUint(uint64(device.ID), 10)+`,"to_region_id":`+eu+`,"reason":"relocated"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var request model.MeshRegionChange
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &request))
	changePath := "/mesh/changes/" + strconv.FormatUint(uint64(request.ID), 10)
	response = serveJSON(router, http.MethodPost, changePath+"/approve", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"new_address":"10.152.128.0"`)
	assert.Contains(t, response.Body.String(), `"decided_by":7`)
	response = serveJSON(router, http.MethodPost, changePath+"/reject", `{"note":"late"}`)
	assert.Equal(t, "mesh_conflict", errorCode(t, response))
	response = serveJSON(router, http.MethodGet, "/mesh/changes?status=approved", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"to_region_code":"eu"`)
	response = serveJSON(router, http.MethodPost, devicePath+"/move", `{"region_id":`+cn+`}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodDelete, "/mesh/host-groups/"+strconv.FormatUint(uint64(group.ID), 10), "").Code)
	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodDelete, devicePath, "").Code)

	var actions []string
	require.NoError(t, env.db.Model(&model.OperationLog{}).Where("module = ?", "mesh").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{
		"mesh_region_create", "mesh_region_create", "mesh_member_add", "mesh_settings_update", "mesh_device_create", "mesh_device_update",
		"mesh_host_group_create", "mesh_device_name_create", "mesh_change_request", "mesh_change_approve", "mesh_device_move",
		"mesh_host_group_delete", "mesh_device_delete",
	}, actions)
}
