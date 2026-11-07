package handler

import (
	"fmt"
	"net/http"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/exitpolicy"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ExitPolicyHandler serves what a proxy node refuses as an exit
// (internal/exitpolicy): the mainland-China block, off by default.
type ExitPolicyHandler struct {
	db func() *gorm.DB
}

// NewExitPolicyHandler reads the loaded database on each request.
func NewExitPolicyHandler() *ExitPolicyHandler { return &ExitPolicyHandler{db: database.Get} }

type exitPolicyRequest struct {
	BlockMainland *bool `json:"block_mainland"`
}

// node answers the proxy node of the request and the database, after
// answering an error itself.
func (h *ExitPolicyHandler) node(c *gin.Context) (agentcontrol.AgentNode, *gorm.DB, bool) {
	node, ok := parseNodeRef(c)
	if !ok {
		return node, nil, false
	}
	if node.Kind != agentcontrol.NodeKindProxy {
		kernelError(c, http.StatusBadRequest, "invalid_request", "an exit policy belongs to a proxy node (proxy-<id>)")
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

// Get godoc
// @Summary A proxy node's exit policy
// @Description Whether the node refuses mainland-China destinations as an exit (block_mainland, off by default), and how many connections its Agent reported refusing (mainland_blocked_total) and when last (mainland_blocked_at).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Success 200 {object} model.NodeExitPolicy
// @Router /api/v4/kernel/nodes/{id}/exit-policy [get]
func (h *ExitPolicyHandler) Get(c *gin.Context) {
	node, db, ok := h.node(c)
	if !ok {
		return
	}
	row, err := exitpolicy.Get(db, uint(node.ID))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, row)
}

// Set godoc
// @Summary Turn a proxy node's mainland-China block on or off
// @Description With block_mainland on, the node's Agent has its kernel refuse every connection the node dials to a mainland-China address (the list ships with the Agent), and reports the refusals on its heartbeat; a refusal raises the alert exit_mainland_destinations. It needs an Agent and anx-node kernel of 5.0.2 or later; older ones ignore it. The node gets it with its next configuration. Off by default. Audited (module exit_policy, action exit_policy_set).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Param request body exitPolicyRequest true "block_mainland"
// @Success 200 {object} model.NodeExitPolicy
// @Router /api/v4/kernel/nodes/{id}/exit-policy [put]
func (h *ExitPolicyHandler) Set(c *gin.Context) {
	node, db, ok := h.node(c)
	if !ok {
		return
	}
	var request exitPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.BlockMainland == nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "block_mainland is required")
		return
	}
	var row model.NodeExitPolicy
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		if row, err = exitpolicy.Set(tx, uint(node.ID), *request.BlockMainland); err != nil {
			return err
		}
		var userID *uint
		if id := kernelActorID(c); id != 0 {
			userID = &id
		}
		targetID := uint(node.ID)
		content := fmt.Sprintf(`{"node":%q,"block_mainland":%t}`, node.String(), *request.BlockMainland)
		return tx.Create(&model.OperationLog{
			UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: "exit_policy_set", Module: "exit_policy",
			TargetType: "proxy_node", TargetID: &targetID, Content: content, IP: truncateText(c.ClientIP(), 45), Status: 1,
		}).Error
	})
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, row)
}
