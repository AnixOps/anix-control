package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/routesubscription"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RouteSubscriptionHandler is the administrator API that publishes a
// forward route in subscriptions (internal/routesubscription, 加入订阅):
// the route's target protocol is offered again at each entry node. Every
// write is in the operation log (module forward_route_subscription).
type RouteSubscriptionHandler struct {
	db func() *gorm.DB
}

// NewRouteSubscriptionHandler reads the database on each request.
func NewRouteSubscriptionHandler() *RouteSubscriptionHandler {
	return &RouteSubscriptionHandler{db: database.Get}
}

type routeSubscriptionRequest struct {
	Published   bool   `json:"published"`
	DisplayName string `json:"display_name"`
}

func (h *RouteSubscriptionHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, routesubscription.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "route not found")
	case errors.Is(err, routesubscription.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// Get godoc
// @Summary A forward route's subscription state
// @Description Whether the route is published in subscriptions (加入订阅), the proxy node protocol its targets reach (a target on the node's host or a protocol's own host and the protocol's port, or a loopback target of a last hop on the node), the entries subscribers dial (each entry node's address, or the route's listen address, and its allocated port), and reason/message when the route cannot be offered: no_protocol, paused, l4_mismatch (the route does not forward the protocol's TCP or UDP) or no_entry.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "route id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/forward-routes/{id}/subscription [get]
func (h *RouteSubscriptionHandler) Get(c *gin.Context) {
	status, err := routesubscription.New(h.db()).Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, status)
}

// Set godoc
// @Summary Publish a forward route in subscriptions
// @Description published true offers the route's target protocol in the protocol's subscription groups once per entry node, at the entry's address and port, named "<protocol's name> | <display_name or route name>" (the entry node's name added when the route has several); false withdraws it. Publishing needs a target that is an enabled protocol of a proxy node. A paused route, or one that does not forward the protocol's transport, stays published and is offered once fixed. display_name is at most 128 characters, without | or control characters. Audited.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "route id"
// @Param request body routeSubscriptionRequest true "published and display name"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/forward-routes/{id}/subscription [put]
func (h *RouteSubscriptionHandler) Set(c *gin.Context) {
	var request routeSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	actor := routesubscription.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
	status, err := routesubscription.New(h.db()).Set(c.Request.Context(), c.Param("id"), request.Published, request.DisplayName, actor)
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, status)
}
