package planner

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"google.golang.org/protobuf/proto"
)

// propertyReserved are reserved ports of the property tests' nodes.
var propertyReserved = map[string][]uint32{"forward-41": {20000, 20002}, "forward-21": {40001}}

// randomRoutes builds up to 12 valid routes over testNodes: nftables or
// gost entries (one or two nodes), optional gost relays and exits with
// RAW or encrypted links, allocated or explicit ports (explicit ones
// distinct by construction), one to three targets.
func randomRoutes(rng *rand.Rand) []*forwardv1.Route {
	n := 1 + rng.Intn(12)
	routes := make([]*forwardv1.Route, 0, n)
	for i := 0; i < n; i++ {
		r := &forwardv1.Route{
			Id: routeID(rng.Intn(1000)*100 + i), Owner: "admin", Paused: rng.Intn(5) == 0,
			Listen: &forwardv1.Listen{Protocol: forwardv1.L4Protocol(1 + rng.Intn(3))},
		}
		nft := rng.Intn(2) == 0
		if rng.Intn(4) == 0 {
			r.Listen.Port = uint32(20020 + i) // #nosec G115 -- i < 12
			if nft {
				r.Listen.Port = uint32(40040 + i) // #nosec G115 -- i < 12
			}
		}
		entry := hop(forwardv1.HopRole_HOP_ROLE_ENTRY, forwardv1.Engine_ENGINE_GOST, 0, "forward-31")
		if nft {
			entry = hop(forwardv1.HopRole_HOP_ROLE_ENTRY, forwardv1.Engine_ENGINE_NFTABLES, 0, "forward-21")
			if rng.Intn(3) == 0 {
				entry.NodeRefs = []string{"forward-21", "forward-22"}
				r.Listen.EntryHostname = "edge.example.com"
			}
		}
		r.Hops = []*forwardv1.Hop{entry}
		secs := []forwardv1.LinkSecurity{
			forwardv1.LinkSecurity_LINK_SECURITY_RAW, forwardv1.LinkSecurity_LINK_SECURITY_TLS, forwardv1.LinkSecurity_LINK_SECURITY_QUIC,
		}
		pick := func(prevNFT bool) forwardv1.LinkSecurity {
			if prevNFT {
				return forwardv1.LinkSecurity_LINK_SECURITY_RAW
			}
			return secs[rng.Intn(len(secs))]
		}
		prevNFT := nft
		if rng.Intn(3) == 0 {
			relay := hop(forwardv1.HopRole_HOP_ROLE_RELAY, forwardv1.Engine_ENGINE_GOST, pick(prevNFT), "forward-31")
			if rng.Intn(3) == 0 {
				relay.Port = uint32(20040 + i) // #nosec G115 -- i < 12
			}
			r.Hops = append(r.Hops, relay)
			prevNFT = false
		}
		if len(r.Hops) > 1 || rng.Intn(3) > 0 {
			exits := [][]string{{"forward-41"}, {"forward-42"}, {"forward-41", "forward-42"}}[rng.Intn(3)]
			r.Hops = append(r.Hops, hop(forwardv1.HopRole_HOP_ROLE_EXIT, forwardv1.Engine_ENGINE_GOST, pick(prevNFT), exits...))
		}
		for t := 0; t <= rng.Intn(3); t++ {
			r.Targets = append(r.Targets, &forwardv1.Target{
				Host: fmt.Sprintf("198.51.100.%d", 1+t), Port: uint32(1 + rng.Intn(65535)), // #nosec G115 -- < 65536
				Weight: uint32(rng.Intn(4)), Priority: uint32(rng.Intn(3)), // #nosec G115 -- small
			})
		}
		routes = append(routes, r)
	}
	return routes
}

func propertyPlan(t *testing.T, routes []*forwardv1.Route, previous Allocations) Result {
	t.Helper()
	res, err := Plan(routes, testNodes(), previous, Options{Cluster: goldenCluster, ReservedPorts: propertyReserved})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Violations) > 0 {
		// The generator makes valid routes (TestPropertyValidInputs), so a
		// refusal is a planner bug, not a skipped case.
		t.Fatalf("violations: %v", res.Violations)
	}
	return res
}

func quickCheck(t *testing.T, property func(seed int64) bool) {
	t.Helper()
	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
}

func newRNG(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic test data
}

// TestPropertyValidInputs keeps the generator honest: its routes are
// valid, so the other properties test plans, not refusals.
func TestPropertyValidInputs(t *testing.T) {
	quickCheck(t, func(seed int64) bool {
		for _, r := range randomRoutes(newRNG(seed)) {
			if vs := validate.Proto(r, validate.Options{}); len(vs) > 0 {
				t.Logf("seed %d: %v", seed, vs)
				return false
			}
		}
		propertyPlan(t, randomRoutes(newRNG(seed)), nil) // fails on violations
		return true
	})
}

// TestPropertyIdempotent: re-planning with the answered allocations
// answers the same plan, and the order of the routes does not matter.
func TestPropertyIdempotent(t *testing.T) {
	quickCheck(t, func(seed int64) bool {
		routes := randomRoutes(newRNG(seed))
		first := propertyPlan(t, routes, nil)
		reversed := make([]*forwardv1.Route, len(routes))
		for i, r := range routes {
			reversed[len(routes)-1-i] = r
		}
		for _, again := range []func() Result{
			func() Result { r := propertyPlan(t, routes, first.Allocations); return r },
			func() Result { r := propertyPlan(t, reversed, nil); return r },
		} {
			second := again()
			if fmt.Sprint(first.Allocations.Sorted()) != fmt.Sprint(second.Allocations.Sorted()) || len(first.States) != len(second.States) {
				t.Logf("seed %d: allocations differ", seed)
				return false
			}
			for ref, s := range first.States {
				if !proto.Equal(s, second.States[ref]) {
					t.Logf("seed %d: %s differs", seed, ref)
					return false
				}
			}
		}
		return true
	})
}

// TestPropertyAllocations: no port or mark is used twice on a node, and
// every port is inside the node's range, not reserved, and the one the
// hop listens on and the previous hop dials.
func TestPropertyAllocations(t *testing.T) {
	nodes := map[string]*forwardv1.NodeInfo{}
	for _, n := range testNodes() {
		nodes[n.GetNodeRef()] = n
	}
	quickCheck(t, func(seed int64) bool {
		res := propertyPlan(t, randomRoutes(newRNG(seed)), nil)
		ports, marks := map[string]bool{}, map[string]bool{}
		for _, a := range res.Allocations.Sorted() {
			pk, mk := fmt.Sprintf("%s:%d", a.NodeRef, a.Port), fmt.Sprintf("%s#%d", a.NodeRef, a.Mark)
			pr := nodes[a.NodeRef].GetPortRange()
			if ports[pk] || marks[mk] || a.Port < pr.GetFirst() || a.Port > pr.GetLast() || a.Mark < 1 || a.Mark > MaxMark {
				t.Logf("seed %d: bad allocation %v", seed, a)
				return false
			}
			for _, reserved := range propertyReserved[a.NodeRef] {
				if a.Port == reserved {
					t.Logf("seed %d: reserved port %v", seed, a)
					return false
				}
			}
			ports[pk], marks[mk] = true, true
		}
		for ref, s := range res.States {
			for _, h := range s.GetHops() {
				a := res.Allocations[Key{RouteID: h.GetRouteId(), HopIndex: h.GetHopIndex(), NodeRef: ref}]
				if h.GetListen().GetPort() != a.Port || h.GetMark() != a.Mark {
					t.Logf("seed %d: %s hop does not use its allocation", seed, ref)
					return false
				}
				for _, u := range h.GetUpstreams() {
					if u.GetNodeRef() == "" {
						continue
					}
					next := res.Allocations[Key{RouteID: h.GetRouteId(), HopIndex: h.GetHopIndex() + 1, NodeRef: u.GetNodeRef()}]
					if u.GetPort() != next.Port {
						t.Logf("seed %d: upstream port %d, next hop listens on %d", seed, u.GetPort(), next.Port)
						return false
					}
				}
			}
		}
		return true
	})
}

// TestPropertyRemovingARouteFreesItsPorts: without a route, its keys are
// gone, its ports and marks are free, and every other allocation stays.
func TestPropertyRemovingARouteFreesItsPorts(t *testing.T) {
	quickCheck(t, func(seed int64) bool {
		rng := newRNG(seed)
		routes := randomRoutes(rng)
		before := propertyPlan(t, routes, nil)
		gone := routes[rng.Intn(len(routes))]
		var rest []*forwardv1.Route
		for _, r := range routes {
			if r.GetId() != gone.GetId() {
				rest = append(rest, r)
			}
		}
		after := propertyPlan(t, rest, before.Allocations)
		used := map[string]bool{}
		for k, s := range after.Allocations {
			if k.RouteID == gone.GetId() {
				t.Logf("seed %d: removed route still allocated", seed)
				return false
			}
			if before.Allocations[k] != s {
				t.Logf("seed %d: %v moved", seed, k)
				return false
			}
			used[fmt.Sprintf("%s:%d", k.NodeRef, s.Port)] = true
		}
		for k, s := range before.Allocations {
			if k.RouteID == gone.GetId() && used[fmt.Sprintf("%s:%d", k.NodeRef, s.Port)] {
				t.Logf("seed %d: freed port %v still used", seed, k)
				return false
			}
		}
		for _, s := range after.States {
			for _, h := range s.GetHops() {
				if h.GetRouteId() == gone.GetId() {
					return false
				}
			}
		}
		return true
	})
}

// TestPropertyGenerations: after any change to the routes, a node's
// generation bumps exactly when its state's bytes change.
func TestPropertyGenerations(t *testing.T) {
	quickCheck(t, func(seed int64) bool {
		rng := newRNG(seed)
		routes := randomRoutes(rng)
		first := propertyPlan(t, routes, nil)
		gens := Stamp(first.States, nil)
		before := map[string][]byte{}
		for ref, s := range first.States {
			before[ref] = hopBytes(s)
		}
		// Change one route: drop it, edit a target, or pause it.
		i := rng.Intn(len(routes))
		changed := make([]*forwardv1.Route, 0, len(routes))
		for j, r := range routes {
			r = proto.Clone(r).(*forwardv1.Route)
			if j == i {
				switch rng.Intn(3) {
				case 0:
					continue
				case 1:
					r.Targets[0].Port = r.Targets[0].Port%65535 + 1
				case 2:
					r.Paused = !r.Paused
				}
			}
			changed = append(changed, r)
		}
		second := propertyPlan(t, changed, first.Allocations)
		next := Stamp(second.States, gens)
		for ref, s := range second.States {
			bumped := next[ref].Generation != gens[ref].Generation
			if bumped && next[ref].Generation != gens[ref].Generation+1 {
				return false
			}
			if bumped != (string(hopBytes(s)) != string(before[ref])) {
				t.Logf("seed %d: %s bumped=%v", seed, ref, bumped)
				return false
			}
		}
		return true
	})
}

func hopBytes(s *forwardv1.NodeForwardState) []byte {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(&forwardv1.NodeForwardState{Hops: s.GetHops()})
	if err != nil {
		panic(err)
	}
	return b
}
