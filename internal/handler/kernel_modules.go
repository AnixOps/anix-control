package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ModuleHandler administers the module PKI: enrollment credentials for
// network modules and CA rotation. Routes sit behind admin authentication
// and the audit log.
type ModuleHandler struct {
	authority func() (*modulepki.Authority, error)
	db        func() *gorm.DB
	remote    func() bool
}

// NewModuleHandler builds the authority from the loaded configuration on
// each request, so the handler follows configuration reloads in tests.
func NewModuleHandler() *ModuleHandler {
	return &ModuleHandler{
		authority: func() (*modulepki.Authority, error) {
			cfg := config.Get()
			if cfg == nil {
				return nil, modulepki.ErrBuiltinPKIDisabled
			}
			return modulepki.FromConfig(cfg.ModuleRuntime, database.Get())
		},
		db: database.Get,
		remote: func() bool {
			cfg := config.Get()
			return cfg != nil && cfg.ModuleRuntime.Enabled
		},
	}
}

type createEnrollmentRequest struct {
	PackageID  string `json:"package_id" binding:"required"`
	TTLSeconds int64  `json:"ttl_seconds" binding:"required,min=1"`
	Reusable   bool   `json:"reusable"`
}

type createdEnrollment struct {
	Enrollment model.ModuleEnrollment `json:"enrollment"`
	// Credential is returned once; the kernel stores only its hash.
	Credential string `json:"credential"`
}

func (h *ModuleHandler) authorityOrError(c *gin.Context) *modulepki.Authority {
	authority, err := h.authority()
	switch {
	case errors.Is(err, modulepki.ErrBuiltinPKIDisabled):
		kernelError(c, http.StatusConflict, "module_pki_disabled", "the built-in module PKI is not enabled (module_runtime.ca_kek with pki=builtin)")
		return nil
	case err != nil:
		kernelError(c, http.StatusServiceUnavailable, "module_pki_unavailable", err.Error())
		return nil
	}
	return authority
}

// CreateEnrollment issues an enrollment credential for a module.
func (h *ModuleHandler) CreateEnrollment(c *gin.Context) {
	var request createEnrollmentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	authority := h.authorityOrError(c)
	if authority == nil {
		return
	}
	credential, row, err := authority.CreateEnrollment(c.Request.Context(), modulepki.EnrollmentRequest{
		PackageID: request.PackageID, TTL: time.Duration(request.TTLSeconds) * time.Second,
		Reusable: request.Reusable, CreatedBy: kernelActorID(c),
	})
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusCreated, createdEnrollment{Enrollment: row, Credential: credential})
}

// ListEnrollments lists enrollment credentials without their secrets.
func (h *ModuleHandler) ListEnrollments(c *gin.Context) {
	authority := h.authorityOrError(c)
	if authority == nil {
		return
	}
	rows, err := authority.ListEnrollments(c.Request.Context())
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// RevokeEnrollment revokes a credential and the certificates issued with it.
func (h *ModuleHandler) RevokeEnrollment(c *gin.Context) {
	authority := h.authorityOrError(c)
	if authority == nil {
		return
	}
	if err := authority.RevokeEnrollment(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			kernelError(c, http.StatusNotFound, "not_found", "enrollment not found")
			return
		}
		kernelDBError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// RotateCA creates the next module CA; it signs after one certificate
// lifetime.
func (h *ModuleHandler) RotateCA(c *gin.Context) {
	authority := h.authorityOrError(c)
	if authority == nil {
		return
	}
	next, err := authority.Rotate(c.Request.Context())
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, next)
}

type setRuntimeRequest struct {
	Runtime string `json:"runtime" binding:"required"`
}

// ListRuntimes lists the packages whose runtime was chosen explicitly; all
// others run locally.
func (h *ModuleHandler) ListRuntimes(c *gin.Context) {
	var rows []model.PluginRuntime
	if err := h.db().WithContext(c.Request.Context()).Order("plugin_id").Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// SetRuntime selects local or remote for a package. It applies at the
// package's next lifecycle operation.
func (h *ModuleHandler) SetRuntime(c *gin.Context) {
	var request setRuntimeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	row, err := service.SetPluginRuntime(c.Request.Context(), h.db(), c.Param("plugin_id"), request.Runtime, h.remote(), kernelActorID(c))
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "plugin not found")
	case errors.Is(err, service.ErrRemoteRuntimeDisabled):
		kernelError(c, http.StatusConflict, "module_runtime_disabled", err.Error())
	case err != nil:
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		kernelData(c, http.StatusOK, row)
	}
}

// TrustBundle returns the module CA bundle as PEM for module deployments.
func (h *ModuleHandler) TrustBundle(c *gin.Context) {
	authority := h.authorityOrError(c)
	if authority == nil {
		return
	}
	bundle, err := authority.TrustBundlePEM(c.Request.Context())
	if err != nil {
		kernelError(c, http.StatusServiceUnavailable, "module_pki_unavailable", err.Error())
		return
	}
	c.Data(http.StatusOK, "application/x-pem-file", bundle)
}
