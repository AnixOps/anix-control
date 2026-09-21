package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MaintenanceOperationHandler struct{ db *gorm.DB }

func NewMaintenanceOperationHandler() *MaintenanceOperationHandler {
	return &MaintenanceOperationHandler{db: database.Get()}
}

func (h *MaintenanceOperationHandler) authorized(c *gin.Context) bool {
	role, err := service.MaintenanceActorRole(h.db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return false
	}
	if role != "owner" && role != "technician" {
		kernelError(c, 403, "maintenance_forbidden", "maintenance role required")
		return false
	}
	return true
}

func maintenanceOperationError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrMaintenanceOperationDenied) {
		kernelError(c, 403, "maintenance_forbidden", "owner approval or maintenance role required")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		kernelError(c, 404, "not_found", "node, verified release or change not found")
		return
	}
	kernelError(c, 409, "maintenance_change_rejected", err.Error())
}

func (h *MaintenanceOperationHandler) List(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	var changes []model.MaintenanceChange
	if err := h.db.Order("id DESC").Limit(200).Find(&changes).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	data := make([]gin.H, 0, len(changes))
	for _, change := range changes {
		var audit []model.MaintenanceChangeAudit
		if err := h.db.Where("change_id = ?", change.ID).Order("id").Find(&audit).Error; err != nil {
			kernelDBError(c, err)
			return
		}
		var operationIDs []string
		if change.OperationIDsJSON != "" {
			_ = json.Unmarshal([]byte(change.OperationIDsJSON), &operationIDs)
		}
		var stored []model.KernelOperation
		operations := []service.KernelOperationStatus{}
		if len(operationIDs) > 0 {
			if err := h.db.Where("id IN ?", operationIDs).Order("node_id, revision").Find(&stored).Error; err != nil {
				kernelDBError(c, err)
				return
			}
			for _, operation := range stored {
				operations = append(operations, service.PublicKernelOperation(operation))
			}
		}
		data = append(data, gin.H{"change": change, "config": service.MaintenanceChangePublicConfig(change), "audit": audit, "operations": operations})
	}
	kernelData(c, 200, data)
}

func (h *MaintenanceOperationHandler) Create(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var input service.MaintenanceChangeInput
	if err := decodeMaintenanceChangeRequest(c, &input); err != nil {
		kernelError(c, 400, "invalid_request", "invalid bounded change request")
		return
	}
	change, err := service.CreateMaintenanceChange(h.db, kernelActorID(c), input)
	if err != nil {
		maintenanceOperationError(c, err)
		return
	}
	kernelData(c, 201, change)
}

func (h *MaintenanceOperationHandler) Approve(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var input struct {
		BindingHash string `json:"binding_hash"`
	}
	if err := decodeMaintenanceChangeRequest(c, &input); err != nil || len(input.BindingHash) != 64 {
		kernelError(c, 400, "invalid_request", "exact binding_hash required")
		return
	}
	change, err := service.ApproveMaintenanceChange(h.db, kernelActorID(c), id, input.BindingHash)
	if err != nil {
		maintenanceOperationError(c, err)
		return
	}
	kernelData(c, 200, change)
}

func (h *MaintenanceOperationHandler) Execute(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	change, err := service.ExecuteMaintenanceChange(h.db, kernelActorID(c), id)
	if err != nil {
		maintenanceOperationError(c, err)
		return
	}
	kernelData(c, 202, change)
}

// MaintenanceLegacyMutationGuard also protects old plugin, topology, system,
// and database APIs when a technician happens to carry the legacy admin flag.
func MaintenanceLegacyMutationGuard() gin.HandlerFunc {
	return maintenanceLegacyMutationGuard(database.Get())
}

func maintenanceLegacyMutationGuard(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maintenanceLegacyRead(c) {
			c.Next()
			return
		}
		denied, err := service.BlockLegacyMaintenanceMutation(db, kernelActorID(c))
		if err != nil {
			kernelDBError(c, err)
			c.Abort()
			return
		}
		if denied {
			kernelError(c, 403, "maintenance_approval_required", "use the maintenance change workflow for operational mutations")
			c.Abort()
			return
		}
		c.Next()
	}
}

func decodeMaintenanceChangeRequest(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func (h *MaintenanceOperationHandler) Catalog(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	var afterID uint64
	var err error
	if after := c.Query("after_id"); after != "" {
		afterID, err = strconv.ParseUint(after, 10, 32)
		if err != nil {
			kernelError(c, 400, "invalid_id", "after_id must be an integer")
			return
		}
	}
	catalog, err := service.GetMaintenanceOperationCatalog(h.db, uint(afterID))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, 200, catalog)
}

func maintenanceLegacyRead(c *gin.Context) bool {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
		return true
	}
	if c.Request.Method != http.MethodPost {
		return false
	}
	switch c.Request.URL.Path {
	case "/api/v2/speed-limit/list", "/api/v2/speed-limit/tunnels", "/api/v2/tunnel/user/list", "/api/v2/admin/forward/list", "/api/v2/admin/forward/diagnose", "/api/v2/admin/tunnel/list", "/api/v2/admin/tunnel/diagnose", "/api/v2/admin/tunnel/user/list", "/api/v2/admin/subscription/preview":
		return true
	}
	return false
}

// Customer forward APIs also permit administrators to operate other users'
// resources. Guard that elevated path and designated technicians, while normal
// customer requests retain the existing resource-scoped authorization.
func MaintenanceForwardMutationGuard() gin.HandlerFunc {
	return maintenanceForwardMutationGuard(database.Get())
}
func maintenanceForwardMutationGuard(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		switch c.Request.URL.Path {
		case "/api/v2/user/forward/rules", "/api/v2/forward/create", "/api/v2/forward/update", "/api/v2/forward/delete", "/api/v2/forward/force-delete", "/api/v2/forward/pause", "/api/v2/forward/resume", "/api/v2/forward/update-order":
		default:
			c.Next()
			return
		}
		exists, err := service.MaintenanceSettingsAvailable(db)
		if err != nil {
			kernelDBError(c, err)
			c.Abort()
			return
		}
		if !exists {
			c.Next()
			return
		}
		role, err := service.MaintenanceActorRole(db, kernelActorID(c))
		if err != nil {
			kernelDBError(c, err)
			c.Abort()
			return
		}
		if role != "technician" && !kernelActorIsAdmin(c) {
			c.Next()
			return
		}
		denied, err := service.BlockLegacyMaintenanceMutation(db, kernelActorID(c))
		if err != nil {
			kernelDBError(c, err)
			c.Abort()
			return
		}
		if denied {
			kernelError(c, 403, "maintenance_approval_required", "owner authorization required for operator network changes")
			c.Abort()
			return
		}
		c.Next()
	}
}
