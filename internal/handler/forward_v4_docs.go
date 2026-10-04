package handler

import "github.com/gin-gonic/gin"

// ForwardV4Docs documents the forward package's /api/v4/forward/* for
// Swagger. The package answers every route (packages/forward/v4api); the
// kernel's ForwardGateway dispatches them, so these methods are never
// routed. Answers are {"data": ...}; refusals {"error": {"code", "message",
// "violations"}}; contract messages are protojson with the proto field
// names, and their 64-bit integers are JSON strings.
type ForwardV4Docs struct{}

// ListRoutes godoc
// @Summary List routes
// @Description Routes with Control's enforcement reason. Query: owner, node_ref, page_size, page_token.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param owner query string false "Owner filter (admin)"
// @Param node_ref query string false "Routes with a hop on this node"
// @Param page_size query int false "Page size (100, at most 1000)"
// @Param page_token query string false "next_page_token of the previous page"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes [get]
func (ForwardV4Docs) ListRoutes(*gin.Context) {}

// CreateRoute godoc
// @Summary Create a route
// @Description Body: a Route (protojson, proto field names). Control assigns id, revision and times. 400 invalid_route or 409 refused carry violations with sdk/forward/validate codes. Idempotency-Key makes a retry apply once.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "Route (protojson)"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes [post]
func (ForwardV4Docs) CreateRoute(*gin.Context) {}

// PreviewRoute godoc
// @Summary Plan a route without storing it
// @Description Body: a PlanRouteRequest. Answers the per-node states, allocations, violations and warnings.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "PlanRouteRequest (protojson)"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/preview [post]
func (ForwardV4Docs) PreviewRoute(*gin.Context) {}

// GetRoute godoc
// @Summary Get a route
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Route id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/{id} [get]
func (ForwardV4Docs) GetRoute(*gin.Context) {}

// UpdateRoute godoc
// @Summary Replace a route
// @Description Body: the whole Route; its revision is the revision it replaces (409 revision_conflict when stale).
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Route id"
// @Param body body object true "Route (protojson)"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/{id} [put]
func (ForwardV4Docs) UpdateRoute(*gin.Context) {}

// DeleteRoute godoc
// @Summary Delete a route
// @Description Super administrators only.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Route id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/{id} [delete]
func (ForwardV4Docs) DeleteRoute(*gin.Context) {}

// PauseRoute godoc
// @Summary Pause a route
// @Description Keeps the route's ports, marks, counters and quota; the drivers drop its traffic.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Route id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/{id}/pause [post]
func (ForwardV4Docs) PauseRoute(*gin.Context) {}

// ResumeRoute godoc
// @Summary Resume a route
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Route id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/{id}/resume [post]
func (ForwardV4Docs) ResumeRoute(*gin.Context) {}

// RouteStats godoc
// @Summary A route's traffic
// @Description Ledger totals per hop and node, and the hourly series over since/until (Unix milliseconds).
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Route id"
// @Param since query int false "Unix milliseconds"
// @Param until query int false "Unix milliseconds"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/{id}/stats [get]
func (ForwardV4Docs) RouteStats(*gin.Context) {}

// RouteHealth godoc
// @Summary A route's upstream health
// @Description The latest health and latency probe of every upstream, from the nodes' reports.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Route id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/routes/{id}/health [get]
func (ForwardV4Docs) RouteHealth(*gin.Context) {}

// ListNodes godoc
// @Summary List the node inventory
// @Description Every forward node and the proxy nodes in the forwarding inventory, with settings, the planner's view and desired and reported generations. Replaces GET /api/v2/admin/forward/nodes.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param kind query string false "forward or proxy"
// @Param transport query string false "agent or ansible"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/nodes [get]
func (ForwardV4Docs) ListNodes(*gin.Context) {}

// CreateNode godoc
// @Summary Add a forward node
// @Description Body: {node: ForwardNodeRecord, settings?: NodeSettings}. No answer carries a node credential; the node enrolls its Agent with an install token.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "CreateForwardNodeRequest (protojson)"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/nodes [post]
func (ForwardV4Docs) CreateNode(*gin.Context) {}

// GetNode godoc
// @Summary A node with its desired state and latest report
// @Description The applied generation, hop errors and upstream health. Replaces the v2 node detail and check.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param ref path string true "forward-<id> or proxy-<id>"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/nodes/{ref} [get]
func (ForwardV4Docs) GetNode(*gin.Context) {}

// UpdateNode godoc
// @Summary Replace a forward node's fields
// @Description Body: ForwardNodeRecord. Disabling a node a route uses is 409 refused (node_in_use).
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param ref path string true "forward-<id>"
// @Param body body object true "ForwardNodeRecord (protojson)"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/nodes/{ref} [put]
func (ForwardV4Docs) UpdateNode(*gin.Context) {}

// DeleteNode godoc
// @Summary Delete a forward node
// @Description Super administrators only. 409 refused (node_in_use) while a route uses it.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param ref path string true "forward-<id>"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/nodes/{ref} [delete]
func (ForwardV4Docs) DeleteNode(*gin.Context) {}

// SetNodeSettings godoc
// @Summary Replace a node's forwarding settings
// @Description Body: NodeSettings (port range, reserved ports, addresses, labels). A replan the settings make impossible still stores them; violations say why.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param ref path string true "forward-<id> or proxy-<id>"
// @Param body body object true "NodeSettings (protojson)"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/nodes/{ref}/settings [put]
func (ForwardV4Docs) SetNodeSettings(*gin.Context) {}

// ToggleNode godoc
// @Summary Enable or disable a forward node
// @Description Body: {enabled: bool}.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param ref path string true "forward-<id>"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/nodes/{ref}/toggle [post]
func (ForwardV4Docs) ToggleNode(*gin.Context) {}

// ListAnsibleMachines godoc
// @Summary List the Ansible machines
// @Description Forward nodes on the Ansible transport. Replaces GET /api/v2/admin/forward/ansible-machines.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/ansible-machines [get]
func (ForwardV4Docs) ListAnsibleMachines(*gin.Context) {}

// CreateAnsibleMachine godoc
// @Summary Add an Ansible machine
// @Description Body as POST /nodes; the transport is NODE_TRANSPORT_ANSIBLE.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "CreateForwardNodeRequest (protojson)"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/ansible-machines [post]
func (ForwardV4Docs) CreateAnsibleMachine(*gin.Context) {}

// GetAnsibleMachine godoc
// @Summary An Ansible machine
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Forward node id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/ansible-machines/{id} [get]
func (ForwardV4Docs) GetAnsibleMachine(*gin.Context) {}

// UpdateAnsibleMachine godoc
// @Summary Replace an Ansible machine's fields
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Forward node id"
// @Param body body object true "ForwardNodeRecord (protojson)"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/ansible-machines/{id} [put]
func (ForwardV4Docs) UpdateAnsibleMachine(*gin.Context) {}

// DeleteAnsibleMachine godoc
// @Summary Delete an Ansible machine
// @Description Super administrators only.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Forward node id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/ansible-machines/{id} [delete]
func (ForwardV4Docs) DeleteAnsibleMachine(*gin.Context) {}

// ToggleAnsibleMachine godoc
// @Summary Enable or disable an Ansible machine
// @Description Body: {enabled: bool}.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Forward node id"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/ansible-machines/{id}/toggle [post]
func (ForwardV4Docs) ToggleAnsibleMachine(*gin.Context) {}

// Stats godoc
// @Summary Traffic totals and the hourly series
// @Description Raw metered bytes (no multiplier) per route, hop and node, per node, and the hourly buckets. Replaces the v2 sync-stats.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param route_id query string false "Route filter"
// @Param node_ref query string false "Node filter"
// @Param since query int false "Unix milliseconds (default: 24 hours ago)"
// @Param until query int false "Unix milliseconds"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/stats [get]
func (ForwardV4Docs) Stats(*gin.Context) {}

// ObservabilityTargets godoc
// @Summary Every route target with its health
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param node_ref query string false "Routes with a hop on this node"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/observability/targets [get]
func (ForwardV4Docs) ObservabilityTargets(*gin.Context) {}

// ObservabilityTopology godoc
// @Summary Nodes, hops and targets as a graph
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/observability/topology [get]
func (ForwardV4Docs) ObservabilityTopology(*gin.Context) {}

// ObservabilityTrend godoc
// @Summary Hourly traffic trend
// @Description The kernel keeps no latency history: the trend is traffic.
// @Tags Forward v4
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param route_id query string false "Route filter"
// @Param node_ref query string false "Node filter"
// @Param since query int false "Unix milliseconds"
// @Param until query int false "Unix milliseconds"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v4/forward/observability/trend [get]
func (ForwardV4Docs) ObservabilityTrend(*gin.Context) {}
