package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/gin-gonic/gin"
)

// ModuleHandler rotates the kernel CA, which signs the Agent certificates.
// The route sits behind admin authentication and the audit log.
type ModuleHandler struct {
	authority func() (*modulepki.Authority, error)
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
	}
}

// RotateCA creates the next CA; it signs after one certificate lifetime.
func (h *ModuleHandler) RotateCA(c *gin.Context) {
	authority, err := h.authority()
	switch {
	case errors.Is(err, modulepki.ErrBuiltinPKIDisabled):
		kernelError(c, http.StatusConflict, "module_pki_disabled", "the built-in CA is not enabled (module_runtime.ca_kek with pki=builtin)")
		return
	case err != nil:
		kernelError(c, http.StatusServiceUnavailable, "module_pki_unavailable", err.Error())
		return
	}
	next, err := authority.Rotate(c.Request.Context())
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, next)
}
