package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/iwanline"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IWANLinesHandler is the administrator API of the Panabit iWAN lines
// (internal/iwanline): third-party iWAN ingresses that AnixOps Net logs into
// with each line's shared account to reach proxy nodes through the line
// (node entries of kind iwan). Every write is in the operation log (module
// iwan_line); no answer carries a password.
type IWANLinesHandler struct {
	db func() *gorm.DB
}

// NewIWANLinesHandler reads the database on each request.
func NewIWANLinesHandler() *IWANLinesHandler {
	return &IWANLinesHandler{db: database.Get}
}

func (h *IWANLinesHandler) lineID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "invalid line id")
		return 0, false
	}
	return uint(id), true
}

func (h *IWANLinesHandler) actor(c *gin.Context) iwanline.Actor {
	return iwanline.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

func (h *IWANLinesHandler) lineError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, iwanline.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "line not found")
	case errors.Is(err, iwanline.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, iwanline.ErrInUse):
		kernelError(c, http.StatusConflict, "in_use", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// List godoc
// @Summary The Panabit iWAN lines
// @Description Every line by name: id, name, host, port, username, mtu (0 is the client's default), encrypt (XOR masking, not confidentiality) and entries (how many node entries reach their nodes through it). The password is never answered.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/iwan-lines [get]
func (h *IWANLinesHandler) List(c *gin.Context) {
	lines, err := iwanline.New(h.db()).List(c.Request.Context())
	if err != nil {
		h.lineError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"lines": lines})
}

// Create godoc
// @Summary Add a Panabit iWAN line
// @Description Adds a line with its shared login (name unique, without |; host a domain or an IP address; port 1-65535; username and password up to 128 characters; mtu 0 or 576-1600). At most 64 lines. Audited without the login.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body iwanline.Input true "the line"
// @Success 201 {object} map[string]any
// @Router /api/v4/kernel/iwan-lines [post]
func (h *IWANLinesHandler) Create(c *gin.Context) {
	var input iwanline.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	line, err := iwanline.New(h.db()).Create(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.lineError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, gin.H{"line": line})
}

// Update godoc
// @Summary Change a Panabit iWAN line
// @Description Replaces the line's fields as Create checks them; an empty password keeps the stored one. Subscriptions change at once. Audited without the login.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "line id"
// @Param request body iwanline.Input true "the line"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/iwan-lines/{id} [put]
func (h *IWANLinesHandler) Update(c *gin.Context) {
	id, ok := h.lineID(c)
	if !ok {
		return
	}
	var input iwanline.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	line, err := iwanline.New(h.db()).Update(c.Request.Context(), id, input, h.actor(c))
	if err != nil {
		h.lineError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"line": line})
}

// Delete godoc
// @Summary Delete a Panabit iWAN line
// @Description Deletes a line that no node entry uses (409 in_use otherwise). Audited.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "line id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/iwan-lines/{id} [delete]
func (h *IWANLinesHandler) Delete(c *gin.Context) {
	id, ok := h.lineID(c)
	if !ok {
		return
	}
	if err := iwanline.New(h.db()).Delete(c.Request.Context(), id, h.actor(c)); err != nil {
		h.lineError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id})
}
