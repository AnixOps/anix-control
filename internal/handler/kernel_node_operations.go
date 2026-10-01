package handler

import (
	"net/http"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// KernelNodeOperationsHandler lists the KernelNodeOps ledger for
// administrators (docs/architecture/node-ops-service.md section 3.9). It is
// read-only.
type KernelNodeOperationsHandler struct {
	db func() *gorm.DB
}

// NewKernelNodeOperationsHandler reads the kernel database.
func NewKernelNodeOperationsHandler() *KernelNodeOperationsHandler {
	return &KernelNodeOperationsHandler{db: database.Get}
}

// List answers GET /api/v4/kernel/node-operations: every package's node
// operations, newest first, filtered by package_id, family, kind, state,
// request_id, operation_id, parent_operation_id, or target_kind with
// target_id, and paged by before (a cursor) and limit (at most 500).
func (h *KernelNodeOperationsHandler) List(c *gin.Context) {
	query, err := kernelnodeops.ParseAdminQuery(c.Query)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	page, err := kernelnodeops.AdminList(c.Request.Context(), db, query)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, page)
}
