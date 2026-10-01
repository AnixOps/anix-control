package handler

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GET /api/v4/kernel/node-operations lists the ledger read-only, newest
// first, and refuses filters it does not know.
func TestKernelNodeOperationsListing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model.KernelNodeOperationModels()...))
	now := time.Now().UTC()
	for _, row := range []model.KernelNodeOperation{
		{OperationID: "op-1", RequestID: "node.sync:proxy-1:a", Digest: "d1", PackageID: "proxy-node", PackageGeneration: 2,
			SubmittedBy: "proxy-node@2", Family: "nodeconfig", Kind: "node.sync", ResourceKey: "nodeconfig:proxy-1", State: "succeeded",
			Operation: `{"sync_node":{"node":{"kind":"NODE_KIND_PROXY","id":"1"}}}`, CreatedAt: now, UpdatedAt: now, DeadlineAt: now, FinishedAt: &now},
		{OperationID: "op-2", RequestID: "forward.apply:40:update:b", Digest: "d2", PackageID: "forward", PackageGeneration: 4,
			SubmittedBy: "forward@4", Family: "forward", Kind: "forward.apply", ResourceKey: "forward:40", State: "failed",
			Operation: `{"apply_forward":{"forward_id":"40"}}`, ErrorCode: "backend_failed", ErrorMessage: "nodex refused",
			CreatedAt: now, UpdatedAt: now, DeadlineAt: now},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	require.NoError(t, db.Create(&model.KernelNodeOperationTarget{OperationID: "op-2", NodeKind: "forward", NodeID: 10}).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/node-operations", (&KernelNodeOperationsHandler{db: func() *gorm.DB { return db }}).List)

	response := serveModuleRequest(router, http.MethodGet, "/node-operations", "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var page struct {
		Data struct {
			Operations []map[string]any `json:"operations"`
			NextBefore uint64           `json:"next_before"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Data.Operations, 2)
	newest := page.Data.Operations[0]
	require.Equal(t, "op-2", newest["operation_id"])
	require.Equal(t, "forward@4", newest["submitted_by"])
	require.Equal(t, map[string]any{"code": "backend_failed", "message": "nodex refused", "retryable": false}, newest["error"])
	require.Equal(t, []any{map[string]any{"kind": "forward", "id": float64(10)}}, newest["targets"])
	require.Equal(t, map[string]any{"apply_forward": map[string]any{"forward_id": "40"}}, newest["operation"])

	filtered := serveModuleRequest(router, http.MethodGet, "/node-operations?package_id=proxy-node&state=succeeded&limit=1", "")
	require.Equal(t, http.StatusOK, filtered.Code)
	require.NoError(t, json.Unmarshal(filtered.Body.Bytes(), &page))
	require.Len(t, page.Data.Operations, 1)
	require.Equal(t, "op-1", page.Data.Operations[0]["operation_id"])
	require.Equal(t, true, page.Data.Operations[0]["terminal"])

	refused := serveModuleRequest(router, http.MethodGet, "/node-operations?state=finished", "")
	require.Equal(t, http.StatusBadRequest, refused.Code)
	require.Contains(t, refused.Body.String(), "invalid_request")
}
