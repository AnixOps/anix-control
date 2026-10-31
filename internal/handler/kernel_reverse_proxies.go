package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/reverseproxy"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ReverseProxiesHandler is the administrator API of the panel's HTTP(S)
// reverse proxies (internal/reverseproxy): sites by host name, routes by
// path to upstream servers, run on entry nodes and dialed from the entry
// or from the last via node over Control's links. Every write is in the
// operation log (module reverse_proxy) and replans the devices; no answer
// carries a private key.
type ReverseProxiesHandler struct {
	db func() *gorm.DB
}

// NewReverseProxiesHandler reads the database on each request.
func NewReverseProxiesHandler() *ReverseProxiesHandler {
	return &ReverseProxiesHandler{db: database.Get}
}

func (h *ReverseProxiesHandler) proxyID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "invalid reverse proxy id")
		return 0, false
	}
	return uint(id), true
}

func (h *ReverseProxiesHandler) actor(c *gin.Context) reverseproxy.Actor {
	return reverseproxy.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

func (h *ReverseProxiesHandler) proxyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, reverseproxy.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "reverse proxy not found")
	case errors.Is(err, reverseproxy.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, reverseproxy.ErrNoKEK):
		kernelError(c, http.StatusConflict, "kek_required", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// List godoc
// @Summary The reverse proxies
// @Description Every reverse proxy by name: entries and via nodes, listen_port, http_port (the plain redirect listener), sites (hosts and routes to upstreams), tls_mode, whether it has a certificate with its names and expiry, pending (why entries do not run it now) and status (what each entry last reported: state, bytes and upstream health). The private key is never answered.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/reverse-proxies [get]
func (h *ReverseProxiesHandler) List(c *gin.Context) {
	proxies, err := reverseproxy.New(h.db()).List(c.Request.Context())
	if err != nil {
		h.proxyError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"reverse_proxies": proxies})
}

// Create godoc
// @Summary Add a reverse proxy
// @Description Adds a reverse proxy: name unique; 1-8 entries and 0-4 via nodes (proxy-<id>); listen_port; sites with hosts (example.com, *.example.com or *) and routes (path prefix, strip_prefix, host_header client or upstream or a fixed host, headers, upstreams, balance, health); tls_mode none or manual (certificate_pem and private_key_pem, a pair covering every host; http_port then redirects to https). The private key is sealed under module_runtime.ca_kek (409 kek_required without it). Audited without the certificate; replans the devices.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body reverseproxy.Input true "the reverse proxy"
// @Success 201 {object} map[string]any
// @Router /api/v4/kernel/reverse-proxies [post]
func (h *ReverseProxiesHandler) Create(c *gin.Context) {
	var input reverseproxy.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	proxy, err := reverseproxy.New(h.db()).Create(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.proxyError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, gin.H{"reverse_proxy": proxy})
}

// Update godoc
// @Summary Change a reverse proxy
// @Description Replaces the reverse proxy's fields as Create checks them; with tls_mode manual, empty certificate_pem and private_key_pem keep the stored certificate. Audited without the certificate; replans the devices.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "reverse proxy id"
// @Param request body reverseproxy.Input true "the reverse proxy"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/reverse-proxies/{id} [put]
func (h *ReverseProxiesHandler) Update(c *gin.Context) {
	id, ok := h.proxyID(c)
	if !ok {
		return
	}
	var input reverseproxy.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	proxy, err := reverseproxy.New(h.db()).Update(c.Request.Context(), id, input, h.actor(c))
	if err != nil {
		h.proxyError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"reverse_proxy": proxy})
}

// Delete godoc
// @Summary Delete a reverse proxy
// @Description Deletes the reverse proxy; its entries stop running it. Audited; replans the devices.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "reverse proxy id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/reverse-proxies/{id} [delete]
func (h *ReverseProxiesHandler) Delete(c *gin.Context) {
	id, ok := h.proxyID(c)
	if !ok {
		return
	}
	if err := reverseproxy.New(h.db()).Delete(c.Request.Context(), id, h.actor(c)); err != nil {
		h.proxyError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id})
}
