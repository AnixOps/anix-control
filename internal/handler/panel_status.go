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

// Status godoc
// @Summary The certificates the panel serves itself
// @Description The panel certificate status (internal/panelcert) in data.certificate: mode (acme, files, external), state (not_configured, pending, valid, failed; the worst certificate's), domain, grpc_name, directory (an ACME directory other than the public CA's), persistent (whether keys survive a restart), listeners (ACME challenge listeners that could not bind) and certificates, each with name, names, roles (web, grpc), source (acme, files), state, serving (acme, file, self_signed), publicly_trusted, issuer, not_before, not_after, renew_at, renewal_failing, last_attempt_at, last_success_at, next_attempt_at, last_error, problem and fix. A process without a certificate manager answers the status the running Control stored; 503 panel_status_unavailable when there is none. Any administrator; no secret.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v5/panel/status [get]
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

// RetryCertificates godoc
// @Summary Request the failing panel certificates again
// @Description Tries the certificates that are failing (no usable certificate, a renewal that is due or failed, a certificate that does not verify) again now, within the limit of three requests per name and hour; certificate files are read again. 202 with data.retry {retrying, waiting (name: the time the limit allows the next request), healthy} and data.certificate (the status). A process without a certificate manager stores the request for the running Control (data.requested). Any administrator.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 202 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v5/panel/certificates/retry [post]
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
