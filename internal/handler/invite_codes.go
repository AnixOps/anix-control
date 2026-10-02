package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// InviteCodeAdminHandler serves the administrator's invite codes
// (registration control): list, generate and revoke. identity-platform
// declares the routes and the identity bridge relays them here, in every
// edition; commissions, withdrawals and invite statistics stay in the
// affiliate package (InviteHandler).
type InviteCodeAdminHandler struct {
	inviteService *service.InviteService
	now           func() time.Time
}

// NewInviteCodeAdminHandler returns the handler on the kernel database.
func NewInviteCodeAdminHandler() *InviteCodeAdminHandler {
	return &InviteCodeAdminHandler{inviteService: service.NewInviteService(database.Get()), now: time.Now}
}

type generateInviteCodesRequest struct {
	Count      *int `json:"count"`
	ExpireDays *int `json:"expire_days"`
}

// ListCodes godoc
// @Summary List invite codes
// @Description Every invite code, newest first; status filters unused, used or expired codes.
// @Tags admin-invite-codes
// @Produce json
// @Security BearerAuth
// @Param status query string false "unused, used or expired"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} map[string]any
// @Router /admin/invite/codes [get]
func (h *InviteCodeAdminHandler) ListCodes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	codes, total, err := h.inviteService.ListInviteCodes(service.InviteCodeListQuery{
		Status: c.Query("status"), Page: page, PageSize: pageSize,
	}, h.now())
	if errors.Is(err, service.ErrInviteCodeFilter) {
		panelError(c, err.Error())
		return
	}
	if err != nil {
		panelError(c, "failed to load invite codes")
		return
	}
	panelSuccess(c, gin.H{
		"list":      codes,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GenerateCodes godoc
// @Summary Generate invite codes
// @Description Generates 1 to 50 codes that belong to no user. expire_days overrides the configured expiry (0: never).
// @Tags admin-invite-codes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]any false "{count, expire_days}"
// @Success 200 {object} map[string]any
// @Router /admin/invite/codes [post]
func (h *InviteCodeAdminHandler) GenerateCodes(c *gin.Context) {
	var req generateInviteCodesRequest
	var body []byte
	if c.Request.Body != nil {
		read, err := io.ReadAll(io.LimitReader(c.Request.Body, 4096))
		if err != nil {
			panelError(c, "invalid request body")
			return
		}
		body = read
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			panelError(c, "invalid request body")
			return
		}
	}
	count := 1
	if req.Count != nil {
		count = *req.Count
	}
	codes, err := h.inviteService.GenerateAdminInviteCodes(count, req.ExpireDays, h.now())
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, gin.H{"codes": codes})
}

// RevokeCode godoc
// @Summary Revoke an invite code
// @Description Deletes an unused invite code; a used code is kept.
// @Tags admin-invite-codes
// @Produce json
// @Security BearerAuth
// @Param id path int true "invite code id"
// @Success 200 {object} map[string]any
// @Router /admin/invite/codes/{id} [delete]
func (h *InviteCodeAdminHandler) RevokeCode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		panelError(c, "invalid invite code id")
		return
	}
	switch err := h.inviteService.RevokeInviteCode(uint(id)); {
	case errors.Is(err, service.ErrInviteCodeNotFound), errors.Is(err, service.ErrInviteCodeUsed):
		panelError(c, err.Error())
	case err != nil:
		panelError(c, "failed to revoke invite code")
	default:
		panelSuccess(c, gin.H{"message": "invite code revoked"})
	}
}
