package planner

import (
	"fmt"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

func testNodes() []*forwardv1.NodeInfo {
	return []*forwardv1.NodeInfo{
		{NodeRef: "proxy-21", Addresses: []string{"192.0.2.21"}, PortRange: &forwardv1.PortRange{First: 40000, Last: 49999}},
		{NodeRef: "proxy-41", Addresses: []string{"203.0.113.41"}},
	}
}

func routeID(n int) string { return fmt.Sprintf("01JF1T%020d", n) }

// networkRoute is a two-hop route on the network model.
func networkRoute(id string, port uint32) *forwardv1.Route {
	return &forwardv1.Route{
		Id: id, Owner: "admin",
		Listen: &forwardv1.Listen{Port: port, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NETWORK, NodeRefs: []string{"proxy-21"}},
			{
				Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_NETWORK, NodeRefs: []string{"proxy-41"},
				Ingress:         &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_NETWORK},
				NetworkEgressId: "eg-hk",
			},
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.7", Port: 443}},
	}
}

func hasViolation(violations []RouteViolation, routeID, field string, code validate.Code) bool {
	for _, v := range violations {
		if v.RouteID == routeID && v.Field == field && v.Code == code {
			return true
		}
	}
	return false
}

func TestPlanAnswersAnEmptyStatePerNode(t *testing.T) {
	res := Plan([]*forwardv1.Route{networkRoute(routeID(1), 40001), networkRoute(routeID(2), 40002)}, testNodes(), Options{})
	if len(res.Violations) > 0 {
		t.Fatal(res.Violations)
	}
	if len(res.States) != 2 {
		t.Fatalf("states: %v", res.States)
	}
	for ref, s := range res.States {
		if s.GetNodeRef() != ref || len(s.GetHops()) != 0 {
			t.Fatalf("%s: %v", ref, s)
		}
	}
	if got := SortedStates(res.States); got[0].GetNodeRef() != "proxy-21" || got[1].GetNodeRef() != "proxy-41" {
		t.Fatalf("sorted: %v", got)
	}
}

func TestPlanRefusesInvalidInput(t *testing.T) {
	removed := networkRoute(routeID(2), 40002)
	removed.Hops[0].Engine = forwardv1.Engine_ENGINE_NFTABLES
	reserved := networkRoute(routeID(3), 40022)
	routes := []*forwardv1.Route{networkRoute(routeID(1), 40001), networkRoute(routeID(1), 40001), removed, reserved, networkRoute("", 40004)}
	res := Plan(routes, testNodes(), Options{ReservedPorts: map[string][]uint32{"proxy-21": {40022}}})
	if res.States != nil {
		t.Fatal("a refused plan has no states")
	}
	for _, want := range []struct {
		route, field string
		code         validate.Code
	}{
		{routeID(1), "id", validate.CodeDuplicate},
		{"", "id", validate.CodeRequired},
		{routeID(2), "hops[0].engine", validate.CodeEngineNotEnabled},
		{routeID(3), "listen.port", validate.CodePortReserved},
	} {
		if !hasViolation(res.Violations, want.route, want.field, want.code) {
			t.Errorf("missing %v in %v", want, res.Violations)
		}
	}
}

func TestPlanRoute(t *testing.T) {
	req := &forwardv1.PlanRouteRequest{Route: networkRoute("", 40001)}
	if resp := PlanRoute(req, testNodes(), Options{}); len(resp.GetViolations()) > 0 || len(resp.GetStates()) > 0 {
		t.Fatal(resp)
	}
	// Request nodes replace the inventory's: proxy-21 now ends at 40000.
	req.Nodes = []*forwardv1.NodeInfo{{NodeRef: "proxy-21", PortRange: &forwardv1.PortRange{First: 30000, Last: 40000}}}
	resp := PlanRoute(req, testNodes(), Options{})
	if len(resp.GetViolations()) != 1 || resp.GetViolations()[0].GetField() != "listen.port" {
		t.Fatal(resp.GetViolations())
	}
}

func TestStamp(t *testing.T) {
	res := Plan([]*forwardv1.Route{networkRoute(routeID(1), 40001)}, testNodes(), Options{})
	gens := Stamp(res.States, nil)
	for ref, g := range gens {
		if g.Generation != 1 || res.States[ref].GetGeneration() != 1 || res.States[ref].GetStateHash() != g.StateHash || len(g.StateHash) != 64 {
			t.Fatalf("%s: %v", ref, g)
		}
	}
	// Re-planning identical input bumps nothing; a vantage probe bumps only
	// its node.
	again := Plan([]*forwardv1.Route{networkRoute(routeID(1), 40001)}, testNodes(), Options{})
	again.States["proxy-21"].VantageProbes = []*forwardv1.VantageProbe{{BindingId: 1, Address: "192.0.2.1", Port: 443}}
	next := Stamp(again.States, gens)
	if next["proxy-41"] != gens["proxy-41"] || next["proxy-21"].Generation != 2 {
		t.Fatalf("generations: %v -> %v", gens, next)
	}
	// Generations of nodes without a state are kept.
	kept := Stamp(map[string]*forwardv1.NodeForwardState{}, map[string]Generation{"proxy-9": {Generation: 4, StateHash: "x"}})
	if kept["proxy-9"].Generation != 4 {
		t.Fatal(kept)
	}
	if StateHash(nil) != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal("an empty state hashes as empty bytes")
	}
	if StateHashOf(&forwardv1.NodeForwardState{}) != StateHash(nil) {
		t.Fatal("a state without probes hashes as its hops")
	}
}
