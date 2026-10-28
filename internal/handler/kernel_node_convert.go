package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeConvertHandler turns a forward node into a regular node
// (docs/design/node-unification, kernelforward.ConvertForwardNode).
type NodeConvertHandler struct {
	db func() *gorm.DB
}

// NewNodeConvertHandler reads the database on each request.
func NewNodeConvertHandler() *NodeConvertHandler {
	return &NodeConvertHandler{db: database.Get}
}

type nodeConvertRequest struct {
	DryRun bool `json:"dry_run"`
}

// Convert godoc
// @Summary Turn a forward node into a regular node
// @Description One machine is one node: a relay is a regular (proxy) node without user protocols, which runs the network model and ANX relays. This creates that node from forward-<id> (name, host, group, tags, approval; hidden from subscribers; no protocols), copies its forwarding settings and last reported engines, moves every route hop, port allocation and DNS node row to it (routes keep their ports) and deletes the forward node, which revokes its Agent's certificates. Every route's plan must accept the change (409 plan_refused otherwise). 409 not_convertible for an Ansible machine or a forward node the network model names. With dry_run nothing changes and to names the node it would be. Then reinstall the Agent on the machine with the new node's install command; its hops are down until it connects. Super administrators only. Audited (module forward, node_convert).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "forward-<id>"
// @Param request body nodeConvertRequest false "dry_run"
// @Success 200 {object} kernelforward.Conversion
// @Router /api/v4/kernel/nodes/{id}/convert [post]
func (h *NodeConvertHandler) Convert(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	if node.Kind != agentcontrol.NodeKindForward {
		kernelError(c, http.StatusBadRequest, "invalid_request", "only a forward node (forward-<id>) is converted")
		return
	}
	var request nodeConvertRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	db := h.db()
	admin, err := service.IsSuperAdmin(db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !admin {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may convert a node")
		return
	}
	actor := networkmodel.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
	answer, err := kernelforward.New(db).ConvertForwardNode(c.Request.Context(), uint64(node.ID), request.DryRun, actor)
	var refused *kernelforward.RefusedError
	switch {
	case errors.Is(err, kernelforward.ErrNodeNotFound):
		kernelError(c, http.StatusNotFound, "node_not_found", err.Error())
	case errors.Is(err, kernelforward.ErrNotConvertible):
		kernelError(c, http.StatusConflict, "not_convertible", err.Error())
	case errors.As(err, &refused):
		kernelError(c, http.StatusConflict, "plan_refused", err.Error())
	case errors.Is(err, kernelforward.ErrInvalidRequest):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case err != nil:
		kernelDBError(c, err)
	default:
		kernelData(c, http.StatusOK, answer)
	}
}
