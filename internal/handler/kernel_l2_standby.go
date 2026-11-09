package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/l2standby"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// L2StandbyHandler is the administrator API of the cross-border standby
// channels (internal/l2standby): a public-protocol channel of each tier
// (VLESS+REALITY+Vision, VLESS+XHTTP behind our own TLS front) for every
// cross-border transport, switched on and off in one call. Every write is
// in the operation log (module l2_standby); no answer carries a uuid or a
// private key. The drill manual is docs/guide/cross-border-standby.md.
type L2StandbyHandler struct {
	db func() *gorm.DB
}

// NewL2StandbyHandler reads the database on each request.
func NewL2StandbyHandler() *L2StandbyHandler {
	return &L2StandbyHandler{db: database.Get}
}

func (h *L2StandbyHandler) service() *l2standby.Service { return l2standby.New(h.db()) }

func (h *L2StandbyHandler) actor(c *gin.Context) l2standby.Actor {
	return l2standby.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

func (h *L2StandbyHandler) standbyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, l2standby.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, l2standby.ErrNotReady):
		kernelError(c, http.StatusConflict, "not_ready", err.Error())
	case errors.Is(err, l2standby.ErrNoPort):
		kernelError(c, http.StatusConflict, "port_taken", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// Overview godoc
// @Summary Cross-border standby channels
// @Description Every cross-border transport of the network model (kind anx, not trusted) with the node pairs it serves, the tier its traffic takes now (reality, xhttp or off), the tiers ready for every pair, and its channels (relay port, borrowed site, own domain and path, REALITY public key); channels of pairs that are gone under stale. Never a uuid or private key.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/network/standby [get]
func (h *L2StandbyHandler) Overview(c *gin.Context) {
	view, err := h.service().Overview(c.Request.Context())
	if err != nil {
		h.standbyError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// Prepare godoc
// @Summary Prepare cross-border standby channels
// @Description Makes sure every pair of the cross-border transports (or of transports) has a channel of each tier it can have: reality once reality_server_name is given (reality_dest defaults to it on 443), xhttp once xhttp_domains names the exit node's own domain. New channels get a fresh uuid, REALITY key pair, short id, path and a relay port of 31000-31999; existing ones keep them. Inactive channels of pairs that are gone are removed when transports is empty. Changes nothing on the nodes until a switch. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body l2standby.PrepareInput true "what to prepare"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/network/standby/prepare [post]
func (h *L2StandbyHandler) Prepare(c *gin.Context) {
	var input l2standby.PrepareInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.service().Prepare(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.standbyError(c, err)
		return
	}
	kernelData(c, http.StatusOK, result)
}

// Switch godoc
// @Summary Switch cross-border transports to a standby tier
// @Description Moves the traffic of transports (or all) to their reality or xhttp channels, or back to the transport with tier off: the source node routes every domain to the exit node's VLESS relay, the exit node serves it, both within a minute through their Agents. All or nothing: 409 not_ready when a transport lacks a ready channel of the tier for one of its pairs. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body l2standby.SwitchInput true "transports and tier"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/network/standby/switch [post]
func (h *L2StandbyHandler) Switch(c *gin.Context) {
	var input l2standby.SwitchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.service().Switch(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.standbyError(c, err)
		return
	}
	kernelData(c, http.StatusOK, result)
}
