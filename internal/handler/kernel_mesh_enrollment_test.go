package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// An administrator issues a credential; the device enrolls once, reads its
// network, reports and asks for another region; the administrator approves
// and finally revokes the device's token.
func TestMeshEnrollmentAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(model.MeshModels()...))
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	handler := &MeshHandler{db: func() *gorm.DB { return env.db }, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)) })
	router.POST("/mesh/regions", handler.CreateRegion)
	router.GET("/mesh/enrollments", handler.ListMeshEnrollments)
	router.POST("/mesh/enrollments", handler.CreateMeshEnrollment)
	router.DELETE("/mesh/enrollments/:enrollment_id", handler.DeleteMeshEnrollment)
	router.POST("/mesh/devices/:device_id/revoke", handler.RevokeMeshDevice)
	router.POST("/mesh/changes/:change_id/approve", handler.ApproveMeshChange)
	router.POST("/device/enroll", handler.EnrollMeshDevice)
	router.GET("/device/config", handler.MeshDeviceConfig)
	router.POST("/device/report", handler.MeshDeviceReport)
	router.POST("/device/region-change", handler.MeshDeviceChange)
	device := func(method, path, token, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(recorder, request)
		return recorder
	}

	for _, body := range []string{`{"code":"cn","name":"cn","cidr":"10.150.0.0/16","enabled":true}`, `{"code":"eu","name":"eu","cidr":"10.152.0.0/16","enabled":true}`} {
		response := serveJSON(router, http.MethodPost, "/mesh/regions", body)
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	}

	assert.Equal(t, "invalid_request", errorCode(t, serveJSON(router, http.MethodPost, "/mesh/enrollments", `{"hostname":"","hours":1}`)))
	response := serveJSON(router, http.MethodPost, "/mesh/enrollments", `{"hostname":"laptop"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var issued meshregion.IssuedEnrollment
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &issued))
	require.NotEmpty(t, issued.Token)
	response = serveJSON(router, http.MethodGet, "/mesh/enrollments", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), issued.Token)

	body := `{"token":"` + issued.Token + `","device_key":"` + strings.Repeat("k", 32) + `","latencies":{"eu":20,"cn":90},"features":["mesh","mesh_native_keys"]}`
	response = serveJSON(router, http.MethodPost, "/device/enroll", body)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var enrolled meshregion.Enrolled
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &enrolled))
	assert.Contains(t, response.Body.String(), `"address":"10.152.128.0/16"`)
	other := `{"token":"` + issued.Token + `","device_key":"` + strings.Repeat("o", 32) + `","latencies":{"eu":20}}`
	assert.Equal(t, http.StatusUnauthorized, serveJSON(router, http.MethodPost, "/device/enroll", other).Code, "used once")
	assert.Equal(t, http.StatusRequestEntityTooLarge, serveJSON(router, http.MethodPost, "/device/enroll", `{"token":"`+strings.Repeat("x", 20<<10)+`"}`).Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, device(http.MethodPost, "/device/region-change", enrolled.DeviceToken, `{"reason":"`+strings.Repeat("x", 20<<10)+`"}`).Code)

	assert.Equal(t, http.StatusUnauthorized, device(http.MethodGet, "/device/config", "msd_unknown", "").Code)
	response = device(http.MethodGet, "/device/config", enrolled.DeviceToken, "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"networks":[]`, "no network before the kernel names mesh_native_keys")
	response = device(http.MethodGet, "/device/config?features=mesh,+mesh_native_keys", enrolled.DeviceToken, "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"role":"access"`)
	assert.Equal(t, "invalid_request", errorCode(t, device(http.MethodPost, "/device/report", enrolled.DeviceToken, `[]`)))
	assert.Equal(t, http.StatusRequestEntityTooLarge, device(http.MethodPost, "/device/report", enrolled.DeviceToken, `{"networks":["`+strings.Repeat("x", 1<<20)+`"]}`).Code)
	assert.Equal(t, http.StatusNoContent, device(http.MethodPost, "/device/report", enrolled.DeviceToken, `{"networks":[]}`).Code)

	response = device(http.MethodPost, "/device/region-change", enrolled.DeviceToken, `{"region":"cn","reason":"moved"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var change model.MeshRegionChange
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &change))
	response = serveJSON(router, http.MethodPost, "/mesh/changes/"+strconv.FormatUint(uint64(change.ID), 10)+"/approve", `{}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, device(http.MethodGet, "/device/config?features=mesh,mesh_native_keys", enrolled.DeviceToken, "").Body.String(), `"address":"10.150.128.0/16"`)

	devicePath := "/mesh/devices/" + strconv.FormatUint(uint64(enrolled.DeviceID), 10)
	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodPost, devicePath+"/revoke", "").Code)
	assert.Equal(t, http.StatusNotFound, serveJSON(router, http.MethodPost, devicePath+"/revoke", "").Code)
	assert.Equal(t, http.StatusUnauthorized, device(http.MethodGet, "/device/config", enrolled.DeviceToken, "").Code)
	assert.Equal(t, "mesh_conflict", errorCode(t, serveJSON(router, http.MethodDelete, "/mesh/enrollments/"+strconv.FormatUint(uint64(issued.ID), 10), "")), "a used credential stays")
	response = serveJSON(router, http.MethodPost, "/mesh/enrollments", `{"device_id":`+strconv.FormatUint(uint64(enrolled.DeviceID), 10)+`}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var again meshregion.IssuedEnrollment
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &again))
	assert.Equal(t, http.StatusNoContent, serveJSON(router, http.MethodDelete, "/mesh/enrollments/"+strconv.FormatUint(uint64(again.ID), 10), "").Code)

	var actions []string
	require.NoError(t, env.db.Model(&model.OperationLog{}).Where("module = ?", "mesh").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"mesh_region_create", "mesh_region_create", "mesh_enrollment_create", "mesh_change_approve", "mesh_device_revoke", "mesh_enrollment_create", "mesh_enrollment_delete"}, actions)

	features.SetVersion("5.0.13")
	assert.Equal(t, http.StatusNotFound, serveJSON(router, http.MethodPost, "/device/enroll", body).Code)
}

func TestMeshFullNodeAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(model.MeshModels()...))
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	handler := &MeshHandler{db: func() *gorm.DB { return env.db }, now: func() time.Time { return now }}
	router := gin.New()
	router.POST("/mesh/regions", handler.CreateRegion)
	router.GET("/mesh/enrollments", handler.ListMeshEnrollments)
	router.POST("/mesh/enrollments", handler.CreateMeshEnrollment)
	router.PUT("/mesh/devices/:device_id/subnets", handler.SetMeshDeviceSubnets)
	router.POST("/device/enroll", handler.EnrollMeshDevice)
	router.GET("/device/config", handler.MeshDeviceConfig)

	response := serveJSON(router, http.MethodPost, "/mesh/regions", `{"code":"cn","name":"cn","cidr":"10.150.0.0/16","enabled":true}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var region model.MeshRegion
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &region))
	regionID := strconv.FormatUint(uint64(region.ID), 10)

	assert.Equal(t, "invalid_request", errorCode(t, serveJSON(router, http.MethodPost, "/mesh/enrollments", `{"hostname":"router","role":"full"}`)), "a full node needs a region")
	assert.Equal(t, "invalid_request", errorCode(t, serveJSON(router, http.MethodPost, "/mesh/enrollments", `{"hostname":"router","region_id":`+regionID+`,"role":"full","address":"10.150.200.1"}`)))
	response = serveJSON(router, http.MethodPost, "/mesh/enrollments", `{"hostname":"router","region_id":`+regionID+`,"role":"full","address":"10.150.0.7"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var issued meshregion.IssuedEnrollment
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &issued))
	assert.Equal(t, "full", issued.Role)
	assert.Equal(t, "mesh_conflict", errorCode(t, serveJSON(router, http.MethodPost, "/mesh/enrollments", `{"hostname":"nas","region_id":`+regionID+`,"role":"full","address":"10.150.0.7"}`)))
	assert.Contains(t, serveJSON(router, http.MethodGet, "/mesh/enrollments", "").Body.String(), `"role":"full","address":"10.150.0.7"`)

	response = serveJSON(router, http.MethodPost, "/device/enroll", `{"token":"`+issued.Token+`","device_key":"`+strings.Repeat("r", 32)+`","features":["mesh_native_keys"]}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var enrolled meshregion.Enrolled
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &enrolled))
	assert.Contains(t, response.Body.String(), `"address":"10.150.0.7/16"`)
	assert.Contains(t, response.Body.String(), `"role":"full"`)

	path := "/mesh/devices/" + strconv.FormatUint(uint64(enrolled.DeviceID), 10) + "/subnets"
	assert.Equal(t, "invalid_request", errorCode(t, serveJSON(router, http.MethodPut, path, `{"subnets":"192.168.1.0/24"}`)))
	assert.Equal(t, "invalid_request", errorCode(t, serveJSON(router, http.MethodPut, path, `{"subnets":["8.8.8.0/24"]}`)))
	assert.Equal(t, "mesh_conflict", errorCode(t, serveJSON(router, http.MethodPut, path, `{"subnets":["10.150.9.0/24"]}`)))
	tooLarge := serveJSON(router, http.MethodPut, path, `{"subnets":["`+strings.Repeat("x", 20<<10)+`"]}`)
	assert.Equal(t, http.StatusRequestEntityTooLarge, tooLarge.Code)
	assert.Equal(t, "payload_too_large", errorCode(t, tooLarge))
	response = serveJSON(router, http.MethodPut, path, `{"subnets":["192.168.1.0/24"]}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.JSONEq(t, `["192.168.1.0/24"]`, string(dataOf(t, response.Body.Bytes())))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/device/config?features=mesh_native_keys", nil)
	request.Header.Set("Authorization", "Bearer "+enrolled.DeviceToken)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"advertise":["192.168.1.0/24"]`)

	var actions []string
	require.NoError(t, env.db.Model(&model.OperationLog{}).Where("module = ?", "mesh").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"mesh_region_create", "mesh_enrollment_create", "mesh_device_subnets"}, actions)
}
