package planner

import (
	"fmt"
	"slices"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// Options are what the planner knows beyond the routes and the nodes.
type Options struct {
	// ReservedPorts are the ports each node keeps for itself (SSH, the
	// Agent's ports, a per-node list), by node reference. Validation
	// refuses a listen port among them.
	ReservedPorts map[string][]uint32
	// OnCreate and Now are passed to validation (validate.Options).
	OnCreate bool
	Now      time.Time
}

// RouteViolation is a violation of one route of a plan.
type RouteViolation struct {
	RouteID string
	validate.Violation
}

// Result is a plan.
type Result struct {
	// States are the desired states by node reference: an empty one for
	// every node of the inventory. Routes run on the network model, which
	// Control plans outside NodeForwardState, so a state carries no hops;
	// Control adds the vantage probes. Their generation and state_hash are
	// unset; Stamp sets them. Nil when there are violations.
	States map[string]*forwardv1.NodeForwardState
	// Violations are every reason the routes cannot be planned, by route
	// and then in rule order. Any violation refuses the whole plan.
	Violations []RouteViolation
}

// Plan validates every route (sdk/forward/validate, with the inventory)
// and answers one empty state per inventory node, or the violations when
// any route is refused. It reads nothing but its arguments. Routes need
// distinct, non-empty ids.
func Plan(routes []*forwardv1.Route, nodes []*forwardv1.NodeInfo, opts Options) Result {
	inventory := model.NodesFromProto(nodes)
	var violations []RouteViolation
	report := func(routeID string, code validate.Code, format string, args ...any) {
		violations = append(violations, RouteViolation{
			RouteID: routeID, Violation: validate.Violation{Field: "id", Code: code, Message: fmt.Sprintf(format, args...)},
		})
	}
	ms := make([]model.Route, 0, len(routes))
	seen := map[string]bool{}
	for _, r := range routes {
		m := model.FromProto(r)
		if m.ID == "" {
			report(m.ID, validate.CodeRequired, "a planned route needs its id")
		} else if seen[m.ID] {
			report(m.ID, validate.CodeDuplicate, "route %s is listed twice", m.ID)
			continue
		}
		seen[m.ID] = true
		ms = append(ms, m)
	}
	slices.SortStableFunc(ms, func(a, b model.Route) int { return strings.Compare(a.ID, b.ID) })
	violations = append(violations, validateRoutes(ms, inventory, opts)...)
	if len(violations) > 0 {
		return Result{Violations: violations}
	}
	states := make(map[string]*forwardv1.NodeForwardState, len(inventory))
	for _, n := range inventory {
		states[n.NodeRef] = &forwardv1.NodeForwardState{NodeRef: n.NodeRef}
	}
	return Result{States: states}
}

// PlanRoute is the single-route check of ForwardControl.PlanRoute:
// req.nodes replace inventory nodes of the same reference, and the answer
// holds the route's violations. A route on the network model has no node
// states, so a valid route answers an empty response. The route's id may
// be empty (a preview before the route is stored).
func PlanRoute(req *forwardv1.PlanRouteRequest, inventory []*forwardv1.NodeInfo, opts Options) *forwardv1.PlanRouteResponse {
	nodes := slices.Clone(req.GetNodes())
	listed := map[string]bool{}
	for _, n := range nodes {
		listed[n.GetNodeRef()] = true
	}
	for _, n := range inventory {
		if !listed[n.GetNodeRef()] {
			nodes = append(nodes, n)
		}
	}
	resp := &forwardv1.PlanRouteResponse{}
	for _, v := range validateRoutes([]model.Route{model.FromProto(req.GetRoute())}, model.NodesFromProto(nodes), opts) {
		resp.Violations = append(resp.Violations, v.ToProto())
	}
	return resp
}

// SortedStates answers the states sorted by node reference.
func SortedStates(states map[string]*forwardv1.NodeForwardState) []*forwardv1.NodeForwardState {
	out := make([]*forwardv1.NodeForwardState, 0, len(states))
	for _, s := range states {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b *forwardv1.NodeForwardState) int { return strings.Compare(a.GetNodeRef(), b.GetNodeRef()) })
	return out
}

func validateRoutes(routes []model.Route, inventory []model.NodeInfo, opts Options) []RouteViolation {
	vopts := validate.Options{Nodes: inventory, ReservedPorts: opts.ReservedPorts, OnCreate: opts.OnCreate, Now: opts.Now}
	var out []RouteViolation
	for _, route := range routes {
		for _, v := range validate.Route(&route, vopts) {
			out = append(out, RouteViolation{RouteID: route.ID, Violation: v})
		}
	}
	return out
}
