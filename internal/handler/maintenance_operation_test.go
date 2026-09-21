package handler

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMaintenanceTechnicianCannotBypassThroughLegacyAdminMutations(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.MaintenanceSettings{})
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 1, TechnicianIDs: []uint{2}}).Error)
	for _, path := range []string{"/api/v3/operations", "/api/v3/plugin-installations/1/actions", "/api/v3/nodes/3/assignments", "/api/v3/deployments/4/apply", "/api/v2/admin/system/backup/restore"} {
		t.Run(path, func(t *testing.T) {
			reached := false
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("user_id", uint(2)); c.Set("is_admin", true) })
			router.Use(maintenanceLegacyMutationGuard(db))
			router.POST(path, func(c *gin.Context) { reached = true; c.Status(204) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("POST", path, bytes.NewBufferString(`{"action":"update","target_version":"2.0.0"}`)))
			require.Equal(t, 403, recorder.Code)
			require.False(t, reached)
		})
	}
}

func TestMaintenanceChangeHandlerRejectsForgedActorAndOversizedBody(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.MaintenanceSettings{})
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 1, TechnicianIDs: []uint{2}}).Error)
	h := &MaintenanceOperationHandler{db: db}
	response := performKernelHandlerRequestWithSetup(t, "POST", "/changes", `{"requested_by":1}`, "/changes", h.Create, func(c *gin.Context) { c.Set("user_id", uint(3)); c.Set("is_admin", true) })
	require.Equal(t, 403, response.Code)
	oversized := `{"config":"` + string(bytes.Repeat([]byte("x"), 33<<10)) + `"}`
	response = performKernelHandlerRequestWithSetup(t, "POST", "/changes", oversized, "/changes", h.Create, func(c *gin.Context) { c.Set("user_id", uint(2)) })
	require.Equal(t, 400, response.Code)
}

func TestMaintenanceForwardGuardKeepsCustomerActionsAndBlocksOperatorBypasses(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.MaintenanceSettings{})
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 1, TechnicianIDs: []uint{2}}).Error)
	cases := []struct {
		name, path string
		actor      uint
		admin      bool
		status     int
	}{
		{"customer forward", "/api/v2/forward/create", 3, false, 204},
		{"technician forward", "/api/v2/forward/create", 2, false, 403},
		{"other administrator", "/api/v2/forward/delete", 3, true, 403},
		{"owner network change", "/api/v2/forward/update", 1, true, 204},
		{"technician customer ticket", "/api/v2/user/ticket", 2, false, 204},
		{"technician read list", "/api/v2/forward/list", 2, true, 204},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("user_id", tc.actor); c.Set("is_admin", tc.admin) })
			router.Use(maintenanceForwardMutationGuard(db))
			router.POST(tc.path, func(c *gin.Context) { c.Status(204) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("POST", tc.path, nil))
			require.Equal(t, tc.status, recorder.Code)
		})
	}
}

func TestMaintenanceChangeListReturnsExecutionStatusWithoutRawResults(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.MaintenanceSettings{})
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 1, TechnicianIDs: []uint{2}}).Error)
	require.NoError(t, db.Create(&model.KernelOperation{ID: "operation-visible", IdempotencyKey: "status-test", PluginID: "machine-telemetry", Kind: "plugin.enable", State: "failed", ConfigJSON: `{"secret":"private-config"}`, ResultJSON: `{"password":"private-result"}`, LastError: "private-error"}).Error)
	require.NoError(t, db.Create(&model.MaintenanceChange{RequestKey: "status-request", RequestedBy: 2, NodeIDsJSON: "[1]", PluginID: "machine-telemetry", Kind: "restart", ConfigJSON: "{}", Status: "queued", OperationIDsJSON: `["operation-visible"]`}).Error)
	h := &MaintenanceOperationHandler{db: db}
	response := performKernelHandlerRequestWithSetup(t, "GET", "/changes", "", "/changes", h.List, func(c *gin.Context) { c.Set("user_id", uint(2)) })
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"state":"failed"`)
	require.Contains(t, response.Body.String(), `"has_error":true`)
	require.NotContains(t, response.Body.String(), "private-")
}
