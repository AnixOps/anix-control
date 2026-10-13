package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PanelStatusHandler answers the panel's own state for the console: the
// certificates Control serves (the banner reads it) and a retry of the
// ones that fail.
type PanelStatusHandler struct {
	manager func() *panelcert.Manager
	db      func() *gorm.DB
}

// NewPanelStatusHandler reads the process's certificate manager, and
// without one (a process that serves no panel) the status the running
// Control last stored.
func NewPanelStatusHandler() *PanelStatusHandler {
	return &PanelStatusHandler{manager: panelcert.Default, db: database.Get}
}

// panelStatusAnswer is GET /api/v5/panel/status.
type panelStatusAnswer struct {
	Certificate panelcert.Status `json:"certificate"`
}

// Status answers the panel certificate status: the mode, the overall state
// (not_configured, pending, valid, failed) and per certificate its name,
// roles, source, state, what is served, whether it is publicly trusted, its
// expiry and renewal, the last error with its problem class and fix.
// Administrators only; it holds no secret.
func (h *PanelStatusHandler) Status(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if manager := h.manager(); manager != nil {
		kernelData(c, http.StatusOK, panelStatusAnswer{Certificate: manager.Status()})
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "panel_status_unavailable", "the panel certificate status is not available")
		return
	}
	status, err := panelcert.LoadStatus(c.Request.Context(), db)
	if errors.Is(err, panelcert.ErrNoStatus) {
		kernelError(c, http.StatusServiceUnavailable, "panel_status_unavailable", err.Error())
		return
	}
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, panelStatusAnswer{Certificate: status})
}

// RetryCertificates tries the failing certificates again now (within the
// hourly limit Control keeps per name): POST
// /api/v5/panel/certificates/retry. The answer lists the names retried,
// the names waiting for the limit and the healthy ones, and the status.
func (h *PanelStatusHandler) RetryCertificates(c *gin.Context) {
	manager := h.manager()
	if manager == nil {
		db := h.db()
		if db == nil {
			kernelError(c, http.StatusServiceUnavailable, "panel_status_unavailable", "the panel certificates are not managed by this process")
			return
		}
		if err := panelcert.RequestRetry(context.WithoutCancel(c.Request.Context()), db, time.Now()); err != nil {
			kernelDBError(c, err)
			return
		}
		kernelData(c, http.StatusAccepted, gin.H{"requested": true})
		return
	}
	result := manager.Retry()
	kernelData(c, http.StatusAccepted, gin.H{"retry": result, "certificate": manager.Status()})
}
