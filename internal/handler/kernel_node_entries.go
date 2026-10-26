package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeEntriesHandler is the administrator API of a proxy node's entries
// (internal/nodeentry): the addresses subscribers reach it at beyond its
// host, such as each line's native IPv6 address and a provider NAT address
// with its port map. Every write is in the operation log (module
// node_entry).
type NodeEntriesHandler struct {
	db func() *gorm.DB
}

// NewNodeEntriesHandler reads the database on each request.
func NewNodeEntriesHandler() *NodeEntriesHandler {
	return &NodeEntriesHandler{db: database.Get}
}

type nodeEntriesRequest struct {
	Entries []nodeentry.Input `json:"entries"`
}

func (h *NodeEntriesHandler) nodeID(c *gin.Context) (uint, bool) {
	node, ok := parseNodeRef(c)
	if !ok {
		return 0, false
	}
	if node.Kind != agentcontrol.NodeKindProxy || node.ID == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "entries belong to a proxy node (proxy-<id>)")
		return 0, false
	}
	return uint(node.ID), true
}

func (h *NodeEntriesHandler) entryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, nodeentry.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "node not found")
	case errors.Is(err, nodeentry.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// List godoc
// @Summary A proxy node's entries
// @Description The addresses subscribers reach the node at beyond its host, in order: label, kind (direct: the protocols' own ports; nat: a public address whose port ranges map to protocol ports), host, family (ipv4, ipv6 or domain), port map and whether the entry is enabled. A node with enabled entries is offered in subscriptions once per entry and protocol, in place of its host.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/entries [get]
func (h *NodeEntriesHandler) List(c *gin.Context) {
	id, ok := h.nodeID(c)
	if !ok {
		return
	}
	entries, err := nodeentry.New(h.db()).List(c.Request.Context(), id)
	if err != nil {
		h.entryError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"node_id": id, "entries": entries})
}

// Replace godoc
// @Summary Replace a proxy node's entries
// @Description Makes the list the node's entries, in that order (at most 32; labels unique, without |). A direct entry has no port map; a NAT entry has 1 to 32 port ranges {from, to, external} that do not overlap: protocol ports from..to are reached at external..external+(to-from). An empty list removes every entry, so the node is reached at its host again. Subscriptions change at once; the node's configuration does not. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Param request body nodeEntriesRequest true "the entries"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/entries [put]
func (h *NodeEntriesHandler) Replace(c *gin.Context) {
	id, ok := h.nodeID(c)
	if !ok {
		return
	}
	var request nodeEntriesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	actor := nodeentry.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
	entries, err := nodeentry.New(h.db()).Replace(c.Request.Context(), id, request.Entries, actor)
	if err != nil {
		h.entryError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"node_id": id, "entries": entries})
}
