package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelalerts"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// KernelAlertsHandler lists the kernel's alerts for administrators
// (certificates that were not renewed, CAs close to their end, phased
// processes left in a non-final phase; internal/kernelalerts). It is
// read-only: alerts are raised and resolved by the monitor, and nothing
// here touches a certificate or a phase.
type KernelAlertsHandler struct {
	db func() *gorm.DB
}

// NewKernelAlertsHandler reads the kernel database.
func NewKernelAlertsHandler() *KernelAlertsHandler {
	return &KernelAlertsHandler{db: database.Get}
}

// List godoc
// @Summary List the kernel alerts
// @Description The alerts of the kernel's alert monitor: Agent, forward link and module certificates that were not renewed in time (leaf certificates alert inside the smaller of alerts.leaf_expiry_days and a sixth of their lifetime), the current module and forward link CAs inside alerts.ca_expiry_days, and phased processes left untouched in a non-final phase for alerts.phase_stuck_after (the node credential split, an interrupted finalize, the identity import and cutover). Critical alerts come first, then the soonest to end. The summary counts every active alert whatever the filters. Alerts hold no secret: kinds, node names, dates and counts only.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param status query string false "active (default), resolved or all"
// @Param kind query string false "one alert kind, for example agent_certificate_expiring"
// @Param severity query string false "warning or critical"
// @Param limit query int false "1 to 500, default 100"
// @Success 200 {object} map[string]any "data.alerts and data.summary {active, critical, warning}"
// @Failure 400 {object} map[string]any
// @Router /api/v4/kernel/alerts [get]
func (h *KernelAlertsHandler) List(c *gin.Context) {
	query, err := kernelalerts.ParseQuery(c.Query("status"), c.Query("kind"), c.Query("severity"), c.Query("limit"))
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, kernelalerts.ErrInvalidQuery) {
			code = http.StatusBadRequest
		}
		kernelError(c, code, "invalid_request", err.Error())
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	page, err := kernelalerts.List(c.Request.Context(), db, query)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, page)
}

// LoadNotificationEmailConfig reads the administrators' e-mail settings
// (the notification e-mail configuration) for senders outside a request,
// such as the alert monitor's digest. A configuration without a host or a
// from address is not usable: it answers nil.
func LoadNotificationEmailConfig(db *gorm.DB) (*model.EmailConfig, error) {
	handler := &NotificationHandler{systemConfigService: service.NewSystemConfigService(db)}
	cfg, err := handler.loadEmailConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Host == "" || cfg.FromAddress == "" {
		return nil, nil
	}
	return cfg, nil
}
