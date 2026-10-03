package planner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// Options are what the planner knows beyond the routes, the nodes and the
// previous allocations.
type Options struct {
	// Cluster names the Agent identities the planner pins on encrypted
	// links: spiffe://anixops/<cluster>/agent/<node>
	// (sdk/agentcontrol.AgentIdentity). Required when a route has an
	// encrypted link.
	Cluster string
	// ReservedPorts are the ports each node keeps for itself (SSH, the
	// Agent's ports, a per-node list), by node reference. The planner never
	// allocates them and validation refuses them.
	ReservedPorts map[string][]uint32
	// Taken are ports and marks held by hops outside this plan: routes
	// deleted within the grace period (forward-sdk.md section 5.2), and for
	// PlanRoute every other stored route. The planner never hands them out
	// and refuses a route that asks for one of their ports.
	Taken Allocations
	// EnableAnixOps, OnCreate and Now are passed to validation
	// (validate.Options).
	EnableAnixOps bool
	OnCreate      bool
	Now           time.Time
}

// RouteViolation is a violation of one route of a plan.
type RouteViolation struct {
	RouteID string
	validate.Violation
}

// Result is a plan.
type Result struct {
	// States are the desired states by node reference: one for every node
	// of the inventory and every node a route uses, with the node's hops
	// sorted by route and hop. Their generation and state_hash are unset;
	// Stamp sets them. Nil when there are violations.
	States map[string]*forwardv1.NodeForwardState
	// Allocations are the ports and marks of every planned hop. Nil when
	// there are violations.
	Allocations Allocations
	// Violations are every reason the routes cannot be planned, by route
	// and then in rule order. Any violation refuses the whole plan, so a
	// node is never left with part of a change.
	Violations []RouteViolation
	// Warnings are for people: ports that had to move, for example.
	Warnings []string
}

// ErrNoCluster is answered when a route has an encrypted link and
// Options.Cluster is empty, so the planner cannot name the identities to
// pin.
var ErrNoCluster = errors.New("planner: Options.Cluster is required to pin Agent identities on encrypted links")

// Plan renders every route into one state per node. It validates every
// route first (sdk/forward/validate, with the inventory) and plans nothing
// when any route is refused. It reads nothing but its arguments, and the
// same arguments give the same result. Routes need distinct, non-empty ids;
// a paused route keeps its hops, allocations and marks, rendered paused.
//
// The error is for a misuse of the planner (ErrNoCluster), never for a
// route: those are Violations.
func Plan(routes []*forwardv1.Route, nodes []*forwardv1.NodeInfo, previous Allocations, opts Options) (Result, error) {
	p := newPlan(nodes, previous, opts)
	ms := make([]model.Route, 0, len(routes))
	seen := map[string]bool{}
	for _, r := range routes {
		m := model.FromProto(r)
		if m.ID == "" {
			p.report(m.ID, "id", validate.CodeRequired, "a planned route needs its id")
		} else if seen[m.ID] {
			p.report(m.ID, "id", validate.CodeDuplicate, "route %s is listed twice", m.ID)
			continue
		}
		seen[m.ID] = true
		ms = append(ms, m)
	}
	slices.SortStableFunc(ms, func(a, b model.Route) int { return compareStrings(a.ID, b.ID) })
	res, err := p.run(ms)
	if err != nil || len(res.Violations) > 0 {
		return res, err
	}
	for ref := range p.nodes {
		if _, ok := res.States[ref]; !ok {
			res.States[ref] = &forwardv1.NodeForwardState{NodeRef: ref}
		}
	}
	return res, nil
}

// PlanRoute is the single-route plan of ForwardControl.PlanRoute and the
// golden fixtures in contracts/forward/v1. req.nodes replace inventory
// nodes of the same reference. previous holds the route's own allocations
// (an update keeps them); opts.Taken holds every other route's. The states
// hold only this route's hops, only on the nodes it uses, at generation 0
// and without a state_hash. The route's id may be empty (a preview before
// the route is stored).
func PlanRoute(req *forwardv1.PlanRouteRequest, inventory []*forwardv1.NodeInfo, previous Allocations, opts Options) (*forwardv1.PlanRouteResponse, error) {
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
	p := newPlan(nodes, previous, opts)
	res, err := p.run([]model.Route{model.FromProto(req.GetRoute())})
	if err != nil {
		return nil, err
	}
	resp := &forwardv1.PlanRouteResponse{Warnings: res.Warnings}
	if len(res.Violations) > 0 {
		for _, v := range res.Violations {
			resp.Violations = append(resp.Violations, v.ToProto())
		}
		return resp, nil
	}
	resp.States = SortedStates(res.States)
	resp.Allocations = res.Allocations.ToProto()
	return resp, nil
}

// SortedStates answers the states sorted by node reference.
func SortedStates(states map[string]*forwardv1.NodeForwardState) []*forwardv1.NodeForwardState {
	out := make([]*forwardv1.NodeForwardState, 0, len(states))
	for _, s := range states {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b *forwardv1.NodeForwardState) int { return compareStrings(a.GetNodeRef(), b.GetNodeRef()) })
	return out
}

// plan is one run of the planner.
type plan struct {
	nodes      map[string]*model.NodeInfo
	inventory  []model.NodeInfo
	previous   Allocations
	opts       Options
	violations []RouteViolation
}

func newPlan(nodes []*forwardv1.NodeInfo, previous Allocations, opts Options) *plan {
	inventory := model.NodesFromProto(nodes)
	index := make(map[string]*model.NodeInfo, len(inventory))
	for i := range inventory {
		if _, dup := index[inventory[i].NodeRef]; !dup {
			index[inventory[i].NodeRef] = &inventory[i]
		}
	}
	return &plan{nodes: index, inventory: inventory, previous: previous, opts: opts}
}

func (p *plan) report(routeID, field string, code validate.Code, format string, args ...any) {
	p.violations = append(p.violations, RouteViolation{
		RouteID:   routeID,
		Violation: validate.Violation{Field: field, Code: code, Message: fmt.Sprintf(format, args...)},
	})
}

// run validates, allocates and wires routes, which are sorted by id.
func (p *plan) run(routes []model.Route) (Result, error) {
	vopts := validate.Options{
		Nodes: p.inventory, ReservedPorts: p.opts.ReservedPorts, EnableAnixOps: p.opts.EnableAnixOps,
		OnCreate: p.opts.OnCreate, Now: p.opts.Now,
	}
	for i := range routes {
		r := &routes[i]
		for _, v := range validate.Route(r, vopts) {
			p.violations = append(p.violations, RouteViolation{RouteID: r.ID, Violation: v})
		}
		if len(p.inventory) == 0 {
			// Validation skips the inventory rules without an inventory;
			// the planner cannot plan without one.
			for hi, h := range r.Hops {
				for ni, ref := range h.NodeRefs {
					p.report(r.ID, fmt.Sprintf("hops[%d].node_refs[%d]", hi, ni), validate.CodeUnknownNode, "%s is not in the node inventory", ref)
				}
			}
		}
	}
	if len(p.violations) > 0 {
		return Result{Violations: p.violations}, nil
	}

	alloc := newAllocator(p.nodes, p.opts, p.previous, p.report)
	var slots []*slot
	for i := range routes {
		slots = append(slots, routeSlots(&routes[i])...)
	}
	alloc.ports(slots)
	if len(p.violations) == 0 {
		alloc.marks()
	}
	if len(p.violations) > 0 {
		return Result{Violations: p.violations, Warnings: alloc.warnings}, nil
	}

	w := wiring{nodes: p.nodes, alloc: alloc.out, cluster: p.opts.Cluster, report: p.report}
	states := map[string]*forwardv1.NodeForwardState{}
	for i := range routes {
		hops, err := w.route(&routes[i])
		if err != nil {
			return Result{}, err
		}
		for _, h := range hops {
			s, ok := states[h.node]
			if !ok {
				s = &forwardv1.NodeForwardState{NodeRef: h.node}
				states[h.node] = s
			}
			s.Hops = append(s.Hops, h.hop)
		}
	}
	warnings := append(alloc.warnings, w.warnings...)
	if len(p.violations) > 0 {
		return Result{Violations: p.violations, Warnings: warnings}, nil
	}
	for _, s := range states {
		sortHops(s.Hops)
	}
	return Result{States: states, Allocations: alloc.out, Warnings: warnings}, nil
}

// routeSlots answers the port decisions of a route: one for the entry hop
// (a shared port on every entry node), one per node for the others.
func routeSlots(r *model.Route) []*slot {
	var slots []*slot
	for i, h := range r.Hops {
		if i == 0 {
			slots = append(slots, &slot{
				routeID: r.ID, nodes: slices.Clone(h.NodeRefs), explicit: r.Listen.Port, field: "listen.port",
			})
			continue
		}
		for _, ref := range h.NodeRefs {
			slots = append(slots, &slot{
				routeID: r.ID, hopIndex: uint32(i), nodes: []string{ref}, explicit: h.Port, // #nosec G115 -- i < validate.MaxHops
				field: fmt.Sprintf("hops[%d].port", i),
			})
		}
	}
	return slots
}

func sortHops(hops []*forwardv1.NodeHop) {
	slices.SortFunc(hops, func(a, b *forwardv1.NodeHop) int {
		if c := compareStrings(a.GetRouteId(), b.GetRouteId()); c != 0 {
			return c
		}
		return int(a.GetHopIndex()) - int(b.GetHopIndex())
	})
}

func compareStrings(a, b string) int { return strings.Compare(a, b) }
