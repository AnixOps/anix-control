package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentinstall"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/nodegroup"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeGroupsHandler is the administrator API of node groups (5.0 node
// onboarding, docs/design/node-onboarding): the groups, their install
// commands and join tokens, and the approval of the nodes that joined a
// group requiring it. Every write and every token read is in the operation
// log (module node_group); no answer but the command's carries the token.
type NodeGroupsHandler struct {
	groups  func() *nodegroup.Service
	db      func() *gorm.DB
	install *AgentInstallHandler
}

// NewNodeGroupsHandler reads the loaded configuration and database on each
// request; install renders the commands.
func NewNodeGroupsHandler(install *AgentInstallHandler) *NodeGroupsHandler {
	return &NodeGroupsHandler{
		groups:  func() *nodegroup.Service { return nodegroup.New(database.Get()) },
		db:      database.Get,
		install: install,
	}
}

// nodeGroupCommandResponse is the install command of a group.
type nodeGroupCommandResponse struct {
	Group nodegroup.GroupView `json:"group"`
	// Token is the group's join token; it joins any number of machines
	// until it expires or is reset.
	Token          string                 `json:"token"`
	TokenExpiresAt *time.Time             `json:"token_expires_at"`
	AgentVersion   string                 `json:"agent_version"`
	Commands       []agentinstall.Command `json:"commands"`
	// UninstallCommand removes the Agent from a machine (add --purge to
	// remove its identity and state too).
	UninstallCommand string            `json:"uninstall_command"`
	Script           installScriptInfo `json:"script"`
}

func (h *NodeGroupsHandler) actor(c *gin.Context) nodegroup.Actor {
	return nodegroup.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

// requireSuperAdmin answers 403 unless the caller is a super administrator
// (an administrator who is not staff and not banned): the routes that show
// or replace a join token.
func (h *NodeGroupsHandler) requireSuperAdmin(c *gin.Context, what string) bool {
	ok, err := service.IsSuperAdmin(h.db(), kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return false
	}
	if !ok {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may "+what)
		return false
	}
	return true
}

// groupError answers a node group error.
func (h *NodeGroupsHandler) groupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, nodegroup.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "node group not found")
	case errors.Is(err, nodegroup.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, nodegroup.ErrNameTaken):
		kernelError(c, http.StatusConflict, "name_taken", err.Error())
	case errors.Is(err, nodegroup.ErrInUse):
		kernelError(c, http.StatusConflict, "node_group_in_use", "the node group has live nodes: delete or move them first")
	case errors.Is(err, nodegroup.ErrNoKEK):
		kernelError(c, http.StatusConflict, "ca_kek_missing", err.Error())
	case errors.Is(err, nodegroup.ErrNodeNotFound):
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist")
	case errors.Is(err, nodegroup.ErrNotPending):
		kernelError(c, http.StatusConflict, "node_not_pending", "the node is not pending approval")
	case errors.Is(err, nodegroup.ErrSealedToken):
		kernelError(c, http.StatusConflict, "join_token_unsealable", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// List godoc
// @Summary List node groups
// @Description The live node groups by sort and id, each with its defaults, protocol template, subscription groups, token reset and expiry times, and its live and pending nodes counted. Never the join token.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/node-groups [get]
func (h *NodeGroupsHandler) List(c *gin.Context) {
	groups, err := h.groups().List(c.Request.Context())
	if err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"groups": groups})
}

// Get godoc
// @Summary Get a node group
// @Description One live node group, as List shows it.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "node group id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/node-groups/{id} [get]
func (h *NodeGroupsHandler) Get(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	group, err := h.groups().Get(c.Request.Context(), id)
	if err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusOK, group)
}

// EnsureDefault godoc
// @Summary Make sure the panel has a node group
// @Description Creates the default node group when no live node group exists, and answers the first group otherwise (created tells which): a proxy group named "默认" whose template is one VLESS Reality protocol on a random port from 20000 to 40000, attached to the default subscription group (created when the panel has none), so the first node needs no input. Idempotent and safe to call concurrently. 409 ca_kek_missing without module_runtime.ca_kek. Audited as a node group create when it creates.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Success 201 {object} map[string]any
// @Router /api/v4/kernel/node-groups/default [post]
func (h *NodeGroupsHandler) EnsureDefault(c *gin.Context) {
	group, created, err := h.groups().EnsureDefault(c.Request.Context(), h.actor(c))
	if err != nil {
		h.groupError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	kernelData(c, status, gin.H{"group": group, "created": created})
}

// Nodes godoc
// @Summary List a node group's nodes
// @Description The group's live nodes (every node of the group but the rejected ones), newest first, for the join dialog that waits for machines: id, node (proxy-<id> or forward-<id>), kind, name, host, approval (approved or pending), online (enabled and checked in within five minutes, as the node list computes it), last_seen, agent_version and created_at. Never a credential. Any administrator.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "node group id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/node-groups/{id}/nodes [get]
func (h *NodeGroupsHandler) Nodes(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	nodes, err := h.groups().Nodes(c.Request.Context(), id)
	if err != nil {
		h.groupError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, gin.H{"nodes": nodes})
}

// Create godoc
// @Summary Create a node group
// @Description A node group with a new join token (sealed under module_runtime.ca_kek, which must be set). Its defaults (rate, tags, monthly_limit_bytes, reset_day, sort) are copied to every node the token creates; protocol_template lists the protocols each new node gets (empty: the node only runs the network model, ANX) (tls none, or reality for vless; port or port_range) and subscription_group_ids the subscription groups they join. The answer does not carry the token: GET .../command shows the install command. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body nodegroup.GroupInput true "the group"
// @Success 201 {object} map[string]any
// @Router /api/v4/kernel/node-groups [post]
func (h *NodeGroupsHandler) Create(c *gin.Context) {
	var input nodegroup.GroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	group, err := h.groups().Create(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, group)
}

// Update godoc
// @Summary Update a node group
// @Description Replaces every field of a group but its token. Nodes that joined keep what they got; the defaults and the template apply to later joins. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "node group id"
// @Param request body nodegroup.GroupInput true "the group"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/node-groups/{id} [put]
func (h *NodeGroupsHandler) Update(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var input nodegroup.GroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	group, err := h.groups().Update(c.Request.Context(), id, input, h.actor(c))
	if err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusOK, group)
}

// Delete godoc
// @Summary Delete a node group
// @Description Soft-deletes a group: its token joins nothing from then on and its name can be used again. Refused (409 node_group_in_use) while the group has live nodes. Audited.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "node group id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/node-groups/{id} [delete]
func (h *NodeGroupsHandler) Delete(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	if err := h.groups().Delete(c.Request.Context(), id, h.actor(c)); err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id, "deleted": true})
}

// Command godoc
// @Summary Show a node group's install command
// @Description The install command for every mirror (curl -fsSL <control>/install.sh | sudo bash -s -- -u <control> -t <anixjoin_...>), the join token, its expiry and the uninstall command. The same command joins any number of machines until the token expires or is reset. Super administrators only; every read is audited as a token reveal (node_group_join_token_reveal).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "node group id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/node-groups/{id}/command [get]
func (h *NodeGroupsHandler) Command(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	if !h.requireSuperAdmin(c, "show a node group's join token") {
		return
	}
	cfg := h.install.config()
	settings, err := h.install.settings(c, cfg)
	if err != nil {
		kernelError(c, http.StatusConflict, "agent_install_unconfigured", err.Error())
		return
	}
	revealed, err := h.groups().RevealToken(c.Request.Context(), id, h.actor(c))
	if err != nil {
		h.groupError(c, err)
		return
	}
	_, signatureErr := agentinstall.LoadSignature(cfg.AgentInstall.SignatureFile, cfg.Plugins.OfficialPublicKey)
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, nodeGroupCommandResponse{
		Group: revealed.Group, Token: revealed.Token, TokenExpiresAt: revealed.Group.TokenExpiresAt,
		// A group without a protocol template makes relays.
		AgentVersion: settings.AgentVersion, Commands: settings.CommandsFor(revealed.Token, len(revealed.Group.ProtocolTemplate) == 0),
		UninstallCommand: agentinstall.UninstallCommand,
		Script: installScriptInfo{
			URL: settings.ControlURL + "/install.sh", SignatureURL: settings.ControlURL + "/install.sh.sig", Signed: signatureErr == nil,
		},
	})
}

// ResetToken godoc
// @Summary Reset a node group's join token
// @Description Replaces the group's join token: every earlier install command stops working; nodes that joined keep their certificates. GET .../command shows the new command. Super administrators only; audited (node_group_join_token_reset).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "node group id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/node-groups/{id}/reset-token [post]
func (h *NodeGroupsHandler) ResetToken(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	if !h.requireSuperAdmin(c, "reset a node group's join token") {
		return
	}
	group, err := h.groups().ResetToken(c.Request.Context(), id, h.actor(c))
	if err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusOK, group)
}

// parseNodeRef reads the node of the path, proxy-<id> or forward-<id>. The
// router names the segment :id, as /kernel/nodes/:id/traffic does.
func parseNodeRef(c *gin.Context) (agentcontrol.AgentNode, bool) {
	node, err := agentcontrol.ParseAgentNode(c.Param("id"))
	if err != nil || !agentinstall.ValidNode(node.String()) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node must be proxy-<id> or forward-<id>")
		return agentcontrol.AgentNode{}, false
	}
	return node, true
}

// ApproveNode godoc
// @Summary Approve a pending node
// @Description Approves a node that joined a group requiring approval: Control starts sending it its configuration (config.v1) and its users (users.v1). 409 node_not_pending for any other node. Audited (node_approve).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id> or forward-<id>"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/approve [post]
func (h *NodeGroupsHandler) ApproveNode(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	decision, err := h.groups().Approve(c.Request.Context(), node, h.actor(c))
	if err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusOK, decision)
}

// RejectNode godoc
// @Summary Reject a pending node
// @Description Rejects a node that joined a group requiring approval: the node is disabled and marked rejected (it no longer counts toward its group), and its Agent certificates are revoked, which ends its stream at the next heartbeat. 409 node_not_pending for any other node. Audited (node_reject).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id> or forward-<id>"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/reject [post]
func (h *NodeGroupsHandler) RejectNode(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	decision, err := h.groups().Reject(c.Request.Context(), node, h.actor(c))
	if err != nil {
		h.groupError(c, err)
		return
	}
	kernelData(c, http.StatusOK, decision)
}
