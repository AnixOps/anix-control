package handler

import (
	"context"
	"errors"
	"math"
	"net/http"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/egressroute"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EgressRoutesHandler is the administrator API of egress routes
// (internal/egressroute): the domains a proxy node's users open that leave
// through another proxy node. Every write is in the operation log (module
// egress_route); no answer carries a relay key.
type EgressRoutesHandler struct {
	db func() *gorm.DB
}

// NewEgressRoutesHandler reads the database on each request.
func NewEgressRoutesHandler() *EgressRoutesHandler {
	return &EgressRoutesHandler{db: database.Get}
}

// Egress route node states: whether a node's Agent runs what the route
// asks of it.
const (
	egressNodeApplied = "applied"
	// egressNodePending: Control has not sent the node the change yet (it
	// does within a minute of a write while the Agent is connected), or
	// the Agent has not answered.
	egressNodePending = "pending"
	egressNodeFailed  = "failed"
)

// egressRouteView is a route with the state of its two nodes.
type egressRouteView struct {
	egressroute.View
	SourceState egressNodeState `json:"source_state"`
	ExitState   egressNodeState `json:"exit_state"`
}

type egressNodeState struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

func (h *EgressRoutesHandler) service() *egressroute.Service { return egressroute.New(h.db()) }

func (h *EgressRoutesHandler) actor(c *gin.Context) egressroute.Actor {
	return egressroute.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

func (h *EgressRoutesHandler) routeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, egressroute.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "egress route not found")
	case errors.Is(err, egressroute.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, egressroute.ErrPortTaken):
		kernelError(c, http.StatusConflict, "port_taken", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// nodeState tells whether the node's Agent applied the configuration the
// node's rows build now.
func (h *EgressRoutesHandler) nodeState(ctx context.Context, states map[uint]egressNodeState, id uint) egressNodeState {
	if state, ok := states[id]; ok {
		return state
	}
	state := egressNodeState{State: egressNodePending}
	defer func() { states[id] = state }()
	if id > math.MaxUint32 {
		return state
	}
	db := h.db().WithContext(ctx)
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(id)}
	built, err := kernelnodeops.BuildDesiredConfig(db, node)
	if err != nil {
		return state
	}
	stored, found, err := kernelnodeops.LoadDesiredConfig(ctx, db, node)
	if err != nil || !found || stored.ConfigHash != built.Hash {
		return state
	}
	status, found, err := kernelnodeops.LoadConfigStatus(ctx, db, node)
	if err != nil || !found {
		return state
	}
	switch {
	case status.AppliedRevision == stored.Revision && status.AppliedHash == stored.ConfigHash:
		state.State = egressNodeApplied
	case status.Verdict == model.ConfigVerdictFailed && status.ReportedRevision == stored.Revision:
		state = egressNodeState{State: egressNodeFailed, Error: status.ReportedError}
	}
	return state
}

func (h *EgressRoutesHandler) withStates(ctx context.Context, states map[uint]egressNodeState, view egressroute.View) egressRouteView {
	return egressRouteView{
		View:        view,
		SourceState: h.nodeState(ctx, states, view.SourceNode.ID),
		ExitState:   h.nodeState(ctx, states, view.ExitNode.ID),
	}
}

// List godoc
// @Summary List egress routes
// @Description Every egress route by id: its source and exit node, the exit host and relay port, its domains, whether it is enabled, and the state of each node (applied, pending or failed with the Agent's error). Never the relay key.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/egress-routes [get]
func (h *EgressRoutesHandler) List(c *gin.Context) {
	views, err := h.service().List(c.Request.Context())
	if err != nil {
		h.routeError(c, err)
		return
	}
	states := map[uint]egressNodeState{}
	routes := make([]egressRouteView, 0, len(views))
	for _, view := range views {
		routes = append(routes, h.withStates(c.Request.Context(), states, view))
	}
	kernelData(c, http.StatusOK, gin.H{"routes": routes})
}

// Create godoc
// @Summary Create an egress route
// @Description Sends the domains the source node's users open out through the exit node: the exit node's Agent serves a Shadowsocks 2022 relay on relay_port with a key Control generates, the source node's Agent routes the domains to it (exit_host, or the exit node's host). Both nodes get it with their next configuration. A bare domain matches its subdomains too; full:, domain:, keyword: and regexp: entries are kept. 409 port_taken when the exit node uses the port. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body egressroute.Input true "the route"
// @Success 201 {object} map[string]any
// @Router /api/v4/kernel/egress-routes [post]
func (h *EgressRoutesHandler) Create(c *gin.Context) {
	var input egressroute.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := h.service().Create(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.routeError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, h.withStates(c.Request.Context(), map[uint]egressNodeState{}, view))
}

// Update godoc
// @Summary Update an egress route
// @Description Replaces every field of the route; rotate_key replaces its relay key too. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "egress route id"
// @Param request body egressroute.Input true "the route"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/egress-routes/{id} [put]
func (h *EgressRoutesHandler) Update(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var input egressroute.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := h.service().Update(c.Request.Context(), id, input, h.actor(c))
	if err != nil {
		h.routeError(c, err)
		return
	}
	kernelData(c, http.StatusOK, h.withStates(c.Request.Context(), map[uint]egressNodeState{}, view))
}

// Delete godoc
// @Summary Delete an egress route
// @Description Removes the route; both nodes drop it with their next configuration. Audited.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "egress route id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/egress-routes/{id} [delete]
func (h *EgressRoutesHandler) Delete(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	if err := h.service().Delete(c.Request.Context(), id, h.actor(c)); err != nil {
		h.routeError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id, "deleted": true})
}
