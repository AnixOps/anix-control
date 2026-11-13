package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/netline"
	"github.com/AnixOps/anix-control/v5/internal/routesubscription"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NetLinesHandler writes lines (docs/design/network-lines, step 3): each
// is stored as a forward route, validated and planned as one, and offered
// in subscriptions when shown. GET /api/v4/kernel/network/lines reads them.
type NetLinesHandler struct {
	db func() *gorm.DB
}

// NewNetLinesHandler reads the database on each request.
func NewNetLinesHandler() *NetLinesHandler { return &NetLinesHandler{db: database.Get} }

func (h *NetLinesHandler) writer(c *gin.Context) *netline.Writer {
	db := h.db()
	return &netline.Writer{DB: db, Forward: kernelforward.New(db), Offers: routesubscription.New(db),
		Actor: routesubscription.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}}
}

// requestID is the Idempotency-Key header, or a new id: a retried request
// with the same key is answered once.
func requestID(c *gin.Context) string {
	if key := c.GetHeader("Idempotency-Key"); key != "" {
		return key
	}
	return uuid.NewString()
}

func (h *NetLinesHandler) fail(c *gin.Context, err error) {
	var refusal *kernelforward.RefusedError
	switch {
	case errors.As(err, &refusal):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{"code": "refused", "message": err.Error(), "violations": refusal.ProtoViolations()}})
	case errors.Is(err, kernelforward.ErrInvalidRequest), errors.Is(err, routesubscription.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, kernelforward.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, kernelforward.ErrRevisionMismatch):
		kernelError(c, http.StatusConflict, "revision_conflict", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// Create godoc
// @Summary Create a line
// @Description Stores the line: chain is its hops (devices; the first hop's are the entries, the last hop's device is the exit), and protocol_id names the exit device's protocol it serves, or targets are fixed addresses the exit connects to. A device on a cloud that scans its machines (its network settings, scan_cloud) forwards in the kernel. show offers the line in subscriptions. Validated and planned as a forward route: 422 with violations when refused. Idempotency-Key makes a retry safe. Answers the new line's id.
// @Tags Network
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body netline.Draft true "the line"
// @Success 201 {object} map[string]any
// @Router /api/v5/net/lines [post]
func (h *NetLinesHandler) Create(c *gin.Context) {
	var draft netline.Draft
	if err := c.ShouldBindJSON(&draft); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	id, err := h.writer(c).Create(c.Request.Context(), requestID(c), draft)
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusCreated, gin.H{"id": id})
}

// Update godoc
// @Summary Update a line
// @Description Replaces the line at revision (409 revision_conflict when it moved on). The draft's policy, hop_balance, quota_bytes and expires_at_unix_ms replace the line's (send what the line API read to keep them); its labels stay, and so does a hop's transport while its devices do. As create otherwise.
// @Tags Network
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "line id"
// @Param request body netline.Draft true "the line"
// @Success 200 {object} map[string]any
// @Router /api/v5/net/lines/{id} [put]
func (h *NetLinesHandler) Update(c *gin.Context) {
	var draft netline.Draft
	if err := c.ShouldBindJSON(&draft); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := h.writer(c).Update(c.Request.Context(), requestID(c), c.Param("id"), draft); err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": c.Param("id")})
}

// Delete godoc
// @Summary Delete a line
// @Description Removes the line and its subscription offer; its devices drop it with the next plan. Needs a super administrator, as deleting a forward route does.
// @Tags Network
// @Produce json
// @Security BearerAuth
// @Param id path string true "line id"
// @Success 200 {object} map[string]any
// @Router /api/v5/net/lines/{id} [delete]
func (h *NetLinesHandler) Delete(c *gin.Context) {
	allowed, err := service.IsSuperAdmin(h.db(), kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !allowed {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may delete lines")
		return
	}
	if err := h.writer(c).Delete(c.Request.Context(), requestID(c), c.Param("id")); err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": c.Param("id")})
}
