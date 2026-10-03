package planner

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// MaxMark is the largest connection mark the planner hands out on a node:
// the nftables driver's mark mask 0x0fff0000 holds 4095 hops per node
// (forward-sdk.md section 6.1). Marks are 1..MaxMark; the driver shifts
// them into its mask.
const MaxMark = 4095

// Key names one hop of one route on one node.
type Key struct {
	RouteID  string
	HopIndex uint32
	NodeRef  string
}

func (k Key) String() string { return fmt.Sprintf("%s/hops[%d]/%s", k.RouteID, k.HopIndex, k.NodeRef) }

func compareKeys(a, b Key) int {
	return cmp.Or(cmp.Compare(a.RouteID, b.RouteID), cmp.Compare(a.HopIndex, b.HopIndex), cmp.Compare(a.NodeRef, b.NodeRef))
}

// Slot is what a hop holds on a node: its listening port and its
// connection mark.
type Slot struct {
	Port uint32
	Mark uint32
}

// Allocations are the ports and marks of hops on nodes. Control stores
// them and passes them back as the previous allocations of the next plan,
// which keeps them (forward-sdk.md section 5.2).
type Allocations map[Key]Slot

// Allocation is one entry of Allocations.
type Allocation struct {
	Key
	Slot
}

// Sorted answers the allocations by route, hop and node.
func (a Allocations) Sorted() []Allocation {
	out := make([]Allocation, 0, len(a))
	for k, s := range a {
		out = append(out, Allocation{Key: k, Slot: s})
	}
	slices.SortFunc(out, func(x, y Allocation) int { return compareKeys(x.Key, y.Key) })
	return out
}

// ToProto answers the allocations as the contract's PortAllocation (port
// and mark), sorted.
func (a Allocations) ToProto() []*forwardv1.PortAllocation {
	sorted := a.Sorted()
	out := make([]*forwardv1.PortAllocation, len(sorted))
	for i, s := range sorted {
		out[i] = &forwardv1.PortAllocation{RouteId: s.RouteID, HopIndex: s.HopIndex, NodeRef: s.NodeRef, Port: s.Port, Mark: s.Mark}
	}
	return out
}

// AllocationsFromProto answers the contract's allocations (what Control
// stored from PlanRouteResponse.allocations) as Allocations, nil for none.
// A later entry for the same (route, hop, node) wins.
func AllocationsFromProto(in []*forwardv1.PortAllocation) Allocations {
	if len(in) == 0 {
		return nil
	}
	out := make(Allocations, len(in))
	for _, p := range in {
		out[Key{RouteID: p.GetRouteId(), HopIndex: p.GetHopIndex(), NodeRef: p.GetNodeRef()}] = Slot{Port: p.GetPort(), Mark: p.GetMark()}
	}
	return out
}

// slot is one port decision: the entry hop's nodes share one port (so DNS
// based entry HA works), every other hop decides per node.
type slot struct {
	routeID  string
	hopIndex uint32
	nodes    []string
	// explicit is the port the route asks for; 0 allocates.
	explicit uint32
	field    string
	port     uint32
	done     bool
}

func (s *slot) key(node string) Key {
	return Key{RouteID: s.routeID, HopIndex: s.hopIndex, NodeRef: node}
}

// nodeUse is what is taken on one node.
type nodeUse struct {
	ports map[uint32]Key
	marks map[uint32]Key
}

// allocator hands out ports and marks over every route of a plan.
type allocator struct {
	nodes    map[string]*model.NodeInfo
	reserved map[string]map[uint32]bool
	use      map[string]*nodeUse
	previous Allocations
	out      Allocations
	report   func(routeID, field string, code validate.Code, format string, args ...any)
	warnings []string
}

// newAllocator starts from opts.Taken, without the entries of the routes
// being planned (planned), whose own previous allocations stick instead.
func newAllocator(nodes map[string]*model.NodeInfo, opts Options, previous Allocations, planned map[string]bool,
	report func(routeID, field string, code validate.Code, format string, args ...any),
) *allocator {
	a := &allocator{
		nodes:    nodes,
		reserved: map[string]map[uint32]bool{},
		use:      map[string]*nodeUse{},
		previous: previous,
		out:      Allocations{},
		report:   report,
	}
	for ref, ports := range opts.ReservedPorts {
		set := map[uint32]bool{}
		for _, p := range ports {
			set[p] = true
		}
		a.reserved[ref] = set
	}
	for _, t := range opts.Taken.Sorted() {
		if t.RouteID != "" && planned[t.RouteID] {
			continue
		}
		u := a.nodeUse(t.NodeRef)
		if t.Port != 0 {
			if _, ok := u.ports[t.Port]; !ok {
				u.ports[t.Port] = t.Key
			}
		}
		if t.Mark != 0 {
			if _, ok := u.marks[t.Mark]; !ok {
				u.marks[t.Mark] = t.Key
			}
		}
	}
	return a
}

func (a *allocator) nodeUse(ref string) *nodeUse {
	u, ok := a.use[ref]
	if !ok {
		u = &nodeUse{ports: map[uint32]Key{}, marks: map[uint32]Key{}}
		a.use[ref] = u
	}
	return u
}

// free reports whether port may listen on node: unused and, for an
// allocated port, inside the node's range and not reserved.
func (a *allocator) free(node string, port uint32, allocated bool) bool {
	if port == 0 || port > validate.MaxPort {
		return false
	}
	if _, taken := a.nodeUse(node).ports[port]; taken {
		return false
	}
	if !allocated {
		return true
	}
	n := a.nodes[node]
	return n != nil && !n.PortRange.IsZero() && n.PortRange.Contains(port) && !a.reserved[node][port]
}

func (a *allocator) freeOnAll(s *slot, port uint32) bool {
	for _, node := range s.nodes {
		if !a.free(node, port, s.explicit == 0) {
			return false
		}
	}
	return true
}

func (a *allocator) claim(s *slot, port uint32) {
	s.port, s.done = port, true
	for _, node := range s.nodes {
		a.nodeUse(node).ports[port] = s.key(node)
	}
}

// ports gives every slot its port in three passes, each in slot order:
//  1. a slot keeps its previous port while that is still legal (for an
//     explicit slot, while the route still asks for it), so allocations
//     stick across replans;
//  2. explicit ports are claimed; one already held is refused;
//  3. the remaining slots get the lowest port free on all their nodes.
func (a *allocator) ports(slots []*slot) {
	for _, s := range slots {
		for _, port := range a.previousPorts(s) {
			if (s.explicit == 0 || port == s.explicit) && a.freeOnAll(s, port) {
				a.claim(s, port)
				break
			}
		}
	}
	for _, s := range slots {
		if s.done || s.explicit == 0 {
			continue
		}
		ok := true
		for _, node := range s.nodes {
			if holder, taken := a.nodeUse(node).ports[s.explicit]; taken {
				ok = false
				a.report(s.routeID, s.field, validate.CodePortInUse, "%d is in use on %s by %s", s.explicit, node, holderName(holder, s.routeID))
			}
		}
		if ok {
			a.claim(s, s.explicit)
		}
	}
	for _, s := range slots {
		if s.done || s.explicit != 0 {
			continue
		}
		if port, ok := a.lowestFree(s); ok {
			a.claim(s, port)
			if prev := a.previousPorts(s); len(prev) > 0 {
				a.warnings = append(a.warnings, fmt.Sprintf("route %s hop %d: port %d is no longer available on %s, moved to %d",
					s.routeID, s.hopIndex, prev[0], joinNodes(s.nodes), port))
			}
		}
	}
	for _, s := range slots {
		if !s.done {
			continue
		}
		for _, node := range s.nodes {
			a.out[s.key(node)] = Slot{Port: s.port}
		}
	}
}

// previousPorts answers the distinct ports the slot's nodes held, lowest
// first.
func (a *allocator) previousPorts(s *slot) []uint32 {
	var ports []uint32
	for _, node := range s.nodes {
		if prev, ok := a.previous[s.key(node)]; ok && prev.Port != 0 && !slices.Contains(ports, prev.Port) {
			ports = append(ports, prev.Port)
		}
	}
	slices.Sort(ports)
	return ports
}

// lowestFree answers the lowest port free on every node of the slot, and
// reports why there is none.
func (a *allocator) lowestFree(s *slot) (uint32, bool) {
	first, last := uint32(1), uint32(validate.MaxPort)
	for _, node := range s.nodes {
		n := a.nodes[node]
		if n == nil {
			return 0, false // reported as an unknown node
		}
		if n.PortRange.IsZero() {
			a.report(s.routeID, s.field, validate.CodeNoPortRange, "%s has no port range to allocate from", node)
			return 0, false
		}
		first, last = max(first, n.PortRange.First), min(last, n.PortRange.Last)
	}
	for port := first; port <= last && port != 0; port++ {
		if a.freeOnAll(s, port) {
			return port, true
		}
	}
	if len(s.nodes) == 1 {
		n := a.nodes[s.nodes[0]]
		a.report(s.routeID, s.field, validate.CodePortExhausted, "no free port in %s's range %d-%d",
			s.nodes[0], n.PortRange.First, n.PortRange.Last)
	} else {
		a.report(s.routeID, s.field, validate.CodePortExhausted, "no port is free on every entry node (%s)", joinNodes(s.nodes))
	}
	return 0, false
}

// marks gives every allocated hop its connection mark: the previous one
// while it is free, else the lowest free one.
func (a *allocator) marks() {
	keys := make([]Key, 0, len(a.out))
	for k := range a.out {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareKeys)
	var pending []Key
	for _, k := range keys {
		u := a.nodeUse(k.NodeRef)
		prev := a.previous[k].Mark
		if _, taken := u.marks[prev]; prev != 0 && prev <= MaxMark && !taken {
			u.marks[prev] = k
			a.setMark(k, prev)
			continue
		}
		pending = append(pending, k)
	}
	for _, k := range pending {
		u := a.nodeUse(k.NodeRef)
		mark := uint32(1)
		for ; mark <= MaxMark; mark++ {
			if _, taken := u.marks[mark]; !taken {
				break
			}
		}
		if mark > MaxMark {
			a.report(k.RouteID, fmt.Sprintf("hops[%d].node_refs", k.HopIndex), validate.CodeMarkExhausted, "%s already runs %d hops", k.NodeRef, MaxMark)
			continue
		}
		u.marks[mark] = k
		a.setMark(k, mark)
	}
}

func (a *allocator) setMark(k Key, mark uint32) {
	s := a.out[k]
	s.Mark = mark
	a.out[k] = s
}

func holderName(holder Key, routeID string) string {
	if holder.RouteID == routeID {
		return fmt.Sprintf("hop %d of this route", holder.HopIndex)
	}
	return fmt.Sprintf("route %s hop %d", holder.RouteID, holder.HopIndex)
}

func joinNodes(nodes []string) string { return strings.Join(nodes, ", ") }
