package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/tempproxy"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TemporaryProxyHandler serves the SOCKS5 proxies a proxy node serves for a
// limited time on an internal address (internal/tempproxy).
type TemporaryProxyHandler struct {
	db  func() *gorm.DB
	now func() time.Time
}

// NewTemporaryProxyHandler reads the loaded database on each request.
func NewTemporaryProxyHandler() *TemporaryProxyHandler {
	return &TemporaryProxyHandler{db: database.Get, now: time.Now}
}

type temporaryProxyRequest struct {
	// Listen is the node's internal address, such as its mesh address.
	Listen string `json:"listen"`
	Port   int    `json:"port"`
	// Minutes the proxy stays open: 1 to 1440, 15 when 0.
	Minutes int `json:"minutes"`
}

// node answers the proxy node of the request and the database, after
// answering an error itself.
func (h *TemporaryProxyHandler) node(c *gin.Context) (agentcontrol.AgentNode, *gorm.DB, bool) {
	node, ok := parseNodeRef(c)
	if !ok {
		return node, nil, false
	}
	if node.Kind != agentcontrol.NodeKindProxy {
		kernelError(c, http.StatusBadRequest, "invalid_request", "a temporary proxy belongs to a proxy node (proxy-<id>)")
		return node, nil, false
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return node, nil, false
	}
	exists, err := nodeExists(db, node)
	if err != nil {
		kernelDBError(c, err)
		return node, nil, false
	}
	if !exists {
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist")
		return node, nil, false
	}
	return node, db.WithContext(c.Request.Context()), true
}

// audit records action on row in the operation log; never the password.
func (h *TemporaryProxyHandler) audit(c *gin.Context, node agentcontrol.AgentNode, action string) tempproxy.Audit {
	return func(tx *gorm.DB, row model.NodeTemporaryProxy) error {
		var userID *uint
		if id := kernelActorID(c); id != 0 {
			userID = &id
		}
		targetID := uint(node.ID)
		content := fmt.Sprintf(`{"node":%q,"proxy_id":%d,"listen":%q,"port":%d,"expires_at":%q}`,
			node.String(), row.ID, row.Listen, row.Port, row.ExpiresAt.UTC().Format(time.RFC3339))
		return tx.Create(&model.OperationLog{
			UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: action, Module: "temporary_proxy",
			TargetType: "proxy_node", TargetID: &targetID, Content: content, IP: truncateText(c.ClientIP(), 45), Status: 1,
		}).Error
	}
}

func temporaryProxyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tempproxy.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, tempproxy.ErrPortTaken):
		kernelError(c, http.StatusConflict, "port_taken", err.Error())
	case errors.Is(err, tempproxy.ErrTooMany):
		kernelError(c, http.StatusConflict, "too_many_temporary_proxies", err.Error())
	case errors.Is(err, tempproxy.ErrNotFound):
		kernelError(c, http.StatusNotFound, "temporary_proxy_not_found", "the node has no open temporary proxy with this id")
	default:
		kernelDBError(c, err)
	}
}

// List godoc
// @Summary A proxy node's open temporary proxies
// @Description The SOCKS5 proxies the node serves now for a limited time on an internal address, the soonest to end first: id, listen, port, username, expires_at, created_by and created_at. Passwords are never answered here.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Success 200 {array} model.NodeTemporaryProxy
// @Router /api/v4/kernel/nodes/{id}/temporary-proxies [get]
func (h *TemporaryProxyHandler) List(c *gin.Context) {
	node, db, ok := h.node(c)
	if !ok {
		return
	}
	rows, err := tempproxy.List(c.Request.Context(), db, uint(node.ID), h.now())
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// Open godoc
// @Summary Open a temporary SOCKS5 proxy on a proxy node
// @Description The node serves a SOCKS5 proxy on listen:port for minutes (1 to 1440, 15 when 0), for devices of its internal network (a mesh address or a LAN address) to send traffic out through the node. listen must be an internal address of the node (10/8, 172.16/12, 192.168/16, 100.64/10, link-local, loopback or fc00::/7), never a wildcard or public address. Control generates the username and password; the password is answered only here. The node's Agent gets the proxy at once, and the node's kernel closes it at expires_at by itself. It needs an Agent and anx-node kernel that run temporary proxies. At most 8 open proxies per node; 409 port_taken when a protocol or another proxy of the node uses the port. Audited (module temporary_proxy, action temporary_proxy_open).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Param request body temporaryProxyRequest true "listen, port and minutes"
// @Success 201 {object} tempproxy.Opened
// @Router /api/v4/kernel/nodes/{id}/temporary-proxies [post]
func (h *TemporaryProxyHandler) Open(c *gin.Context) {
	node, db, ok := h.node(c)
	if !ok {
		return
	}
	var request temporaryProxyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "listen, port and minutes are expected")
		return
	}
	if request.Minutes < 0 || request.Minutes > int(tempproxy.MaxDuration/time.Minute) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "minutes: 1 to 1440, or 0 for 15")
		return
	}
	var createdBy *uint
	if id := kernelActorID(c); id != 0 {
		createdBy = &id
	}
	opened, err := tempproxy.Open(c.Request.Context(), db, uint(node.ID), tempproxy.Request{
		Listen: request.Listen, Port: request.Port, Duration: time.Duration(request.Minutes) * time.Minute, CreatedBy: createdBy,
	}, h.now(), h.audit(c, node, "temporary_proxy_open"))
	if err != nil {
		temporaryProxyError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, opened)
}

// Close godoc
// @Summary Close a temporary proxy before its end
// @Description The node's Agent stops the proxy at once and its open connections end. 404 when the node has no open proxy with this id (also after it ended). Audited (module temporary_proxy, action temporary_proxy_close).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Param proxy_id path int true "temporary proxy id"
// @Success 200 {object} model.NodeTemporaryProxy
// @Router /api/v4/kernel/nodes/{id}/temporary-proxies/{proxy_id} [delete]
func (h *TemporaryProxyHandler) Close(c *gin.Context) {
	node, db, ok := h.node(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("proxy_id"), 10, 32)
	if err != nil || id == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "proxy_id must be a positive integer")
		return
	}
	row, err := tempproxy.Close(c.Request.Context(), db, uint(node.ID), uint(id), h.now(), h.audit(c, node, "temporary_proxy_close"))
	if err != nil {
		temporaryProxyError(c, err)
		return
	}
	kernelData(c, http.StatusOK, row)
}
