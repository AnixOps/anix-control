package planner

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"google.golang.org/protobuf/proto"
)

var allStrategies = []forwardv1.BalanceStrategy{
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM,
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH, forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN,
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
}

func nftNode(ref string, first, last uint32, addrs ...string) *forwardv1.NodeInfo {
	return &forwardv1.NodeInfo{
		NodeRef: ref, Addresses: addrs, PortRange: &forwardv1.PortRange{First: first, Last: last},
		Engines: []*forwardv1.EngineCapabilities{{
			Engine: forwardv1.Engine_ENGINE_NFTABLES, Available: true, Udp: true, Ipv6: true, Strategies: allStrategies,
			LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
			BandwidthLimit: true, Quota: true, MaxConns: true,
		}},
	}
}

func gostNode(ref string, first, last uint32, addrs ...string) *forwardv1.NodeInfo {
	return &forwardv1.NodeInfo{
		NodeRef: ref, Addresses: addrs, PortRange: &forwardv1.PortRange{First: first, Last: last},
		Engines: []*forwardv1.EngineCapabilities{{
			Engine: forwardv1.Engine_ENGINE_GOST, Available: true, Udp: true, Ipv6: true, Strategies: allStrategies,
			LinkSecurities: []forwardv1.LinkSecurity{
				forwardv1.LinkSecurity_LINK_SECURITY_RAW, forwardv1.LinkSecurity_LINK_SECURITY_TLS,
				forwardv1.LinkSecurity_LINK_SECURITY_WSS, forwardv1.LinkSecurity_LINK_SECURITY_QUIC,
				forwardv1.LinkSecurity_LINK_SECURITY_GRPC,
			},
			BandwidthLimit: true, MaxConns: true,
		}},
	}
}

func testNodes() []*forwardv1.NodeInfo {
	return []*forwardv1.NodeInfo{
		nftNode("forward-21", 40000, 40063, "192.0.2.21"),
		nftNode("forward-22", 40000, 40063, "192.0.2.22"),
		gostNode("forward-31", 20000, 20063, "198.51.100.31"),
		gostNode("forward-41", 20000, 20063, "203.0.113.41"),
		gostNode("forward-42", 20000, 20063, "203.0.113.42"),
	}
}

func hop(role forwardv1.HopRole, engine forwardv1.Engine, sec forwardv1.LinkSecurity, nodes ...string) *forwardv1.Hop {
	h := &forwardv1.Hop{Role: role, Engine: engine, NodeRefs: nodes}
	if role != forwardv1.HopRole_HOP_ROLE_ENTRY {
		h.Ingress = &forwardv1.LinkTransport{Security: sec}
	}
	return h
}

// twoHop is an nftables entry on forward-21 and a gost exit on forward-41.
func twoHop(id string, listenPort uint32) *forwardv1.Route {
	return &forwardv1.Route{
		Id: id, Owner: "admin",
		Listen: &forwardv1.Listen{Port: listenPort, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			hop(forwardv1.HopRole_HOP_ROLE_ENTRY, forwardv1.Engine_ENGINE_NFTABLES, 0, "forward-21"),
			hop(forwardv1.HopRole_HOP_ROLE_EXIT, forwardv1.Engine_ENGINE_GOST, forwardv1.LinkSecurity_LINK_SECURITY_RAW, "forward-41"),
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.7", Port: 443}},
	}
}

func routeID(n int) string { return fmt.Sprintf("01JF1T%020d", n) }

func mustPlan(t *testing.T, routes []*forwardv1.Route, previous Allocations, opts Options) Result {
	t.Helper()
	if opts.Cluster == "" {
		opts.Cluster = goldenCluster
	}
	res, err := Plan(routes, testNodes(), previous, opts)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func hasViolation(res Result, routeID, field string, code validate.Code) bool {
	for _, v := range res.Violations {
		if v.RouteID == routeID && v.Field == field && v.Code == code {
			return true
		}
	}
	return false
}

func TestPlanMergesRoutesPerNode(t *testing.T) {
	a, b := twoHop(routeID(1), 0), twoHop(routeID(2), 0)
	res := mustPlan(t, []*forwardv1.Route{b, a}, nil, Options{})
	if len(res.Violations) > 0 {
		t.Fatal(res.Violations)
	}
	// Every inventory node has a state; the unused ones are empty.
	if len(res.States) != len(testNodes()) || len(res.States["forward-31"].GetHops()) != 0 {
		t.Fatalf("states: %v", res.States)
	}
	entry := res.States["forward-21"].GetHops()
	if len(entry) != 2 || entry[0].GetRouteId() != a.GetId() || entry[1].GetRouteId() != b.GetId() {
		t.Fatalf("hops are sorted by route: %v", entry)
	}
	// The lowest free ports and marks, in route order.
	if entry[0].GetListen().GetPort() != 40000 || entry[1].GetListen().GetPort() != 40001 ||
		entry[0].GetMark() != 1 || entry[1].GetMark() != 2 {
		t.Fatalf("allocation: %v", entry)
	}
	exit := res.States["forward-41"].GetHops()
	if exit[0].GetListen().GetPort() != 20000 || exit[1].GetListen().GetPort() != 20001 ||
		entry[1].GetUpstreams()[0].GetPort() != 20001 {
		t.Fatalf("wiring: %v %v", entry, exit)
	}
	for _, s := range res.States {
		if s.GetGeneration() != 0 || s.GetStateHash() != "" {
			t.Fatalf("Plan leaves generations to Stamp: %v", s)
		}
	}
}

func TestPortInUse(t *testing.T) {
	// A stored route holds 40005; a new route asking for it is refused,
	// whatever the order of the routes.
	stored, fresh := twoHop(routeID(2), 0), twoHop(routeID(1), 40005)
	previous := Allocations{{RouteID: stored.GetId(), HopIndex: 0, NodeRef: "forward-21"}: {Port: 40005, Mark: 1}}
	res := mustPlan(t, []*forwardv1.Route{stored, fresh}, previous, Options{})
	if !hasViolation(res, fresh.GetId(), "listen.port", validate.CodePortInUse) || len(res.Violations) != 1 {
		t.Fatalf("violations: %v", res.Violations)
	}
	if res.States != nil || res.Allocations != nil {
		t.Fatal("a refused plan plans nothing")
	}

	// Two new routes asking for one port: the first by id gets it.
	res = mustPlan(t, []*forwardv1.Route{twoHop(routeID(4), 40007), twoHop(routeID(3), 40007)}, nil, Options{})
	if !hasViolation(res, routeID(4), "listen.port", validate.CodePortInUse) || len(res.Violations) != 1 {
		t.Fatalf("violations: %v", res.Violations)
	}

	// A port taken by a route in its grace period.
	taken := Allocations{{RouteID: routeID(9), NodeRef: "forward-21"}: {Port: 40007}}
	res = mustPlan(t, []*forwardv1.Route{twoHop(routeID(3), 40007)}, nil, Options{Taken: taken})
	if !hasViolation(res, routeID(3), "listen.port", validate.CodePortInUse) {
		t.Fatalf("violations: %v", res.Violations)
	}

	// One node serving two hops of a route on one port.
	r := twoHop(routeID(5), 20003)
	r.Hops[0] = hop(forwardv1.HopRole_HOP_ROLE_ENTRY, forwardv1.Engine_ENGINE_GOST, 0, "forward-41")
	r.Hops[1].Port = 20003
	res = mustPlan(t, []*forwardv1.Route{r}, nil, Options{})
	if !hasViolation(res, r.GetId(), "hops[1].port", validate.CodePortInUse) {
		t.Fatalf("violations: %v", res.Violations)
	}
	if !strings.Contains(res.Violations[0].Message, "hop 0 of this route") {
		t.Fatalf("message: %s", res.Violations[0].Message)
	}
}

func TestStickyAllocations(t *testing.T) {
	r := twoHop(routeID(1), 0)
	previous := Allocations{
		{RouteID: r.GetId(), HopIndex: 0, NodeRef: "forward-21"}: {Port: 40009, Mark: 7},
		{RouteID: r.GetId(), HopIndex: 1, NodeRef: "forward-41"}: {Port: 20011, Mark: 4},
	}
	res := mustPlan(t, []*forwardv1.Route{r}, previous, Options{})
	if fmt.Sprint(res.Allocations.Sorted()) != fmt.Sprint(previous.Sorted()) || len(res.Warnings) != 0 {
		t.Fatalf("allocations must stick: %v %v", res.Allocations.Sorted(), res.Warnings)
	}

	// A sticky port that became reserved moves, with a warning; the mark
	// stays.
	res = mustPlan(t, []*forwardv1.Route{r}, previous, Options{ReservedPorts: map[string][]uint32{"forward-41": {20011}}})
	got := res.Allocations[Key{RouteID: r.GetId(), HopIndex: 1, NodeRef: "forward-41"}]
	if got.Port != 20000 || got.Mark != 4 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "moved to 20000") {
		t.Fatalf("moved: %v %v", got, res.Warnings)
	}

	// The route's own allocations in Options.Taken do not block it.
	res = mustPlan(t, []*forwardv1.Route{r}, previous, Options{Taken: previous})
	if fmt.Sprint(res.Allocations.Sorted()) != fmt.Sprint(previous.Sorted()) {
		t.Fatalf("own taken: %v", res.Allocations.Sorted())
	}

	// An explicit port replaces the sticky one.
	r.Hops[1].Port = 20030
	res = mustPlan(t, []*forwardv1.Route{r}, previous, Options{})
	if got := res.Allocations[Key{RouteID: r.GetId(), HopIndex: 1, NodeRef: "forward-41"}]; got.Port != 20030 {
		t.Fatalf("explicit: %v", got)
	}
}

func TestEntryNodesShareAPort(t *testing.T) {
	r := twoHop(routeID(1), 0)
	r.Listen.EntryHostname = "edge.example.com"
	r.Hops[0].NodeRefs = []string{"forward-21", "forward-22"}
	// forward-21 already ran the entry on 40002; forward-22 joins and gets
	// the same port.
	previous := Allocations{{RouteID: r.GetId(), NodeRef: "forward-21"}: {Port: 40002, Mark: 1}}
	res := mustPlan(t, []*forwardv1.Route{r}, previous, Options{})
	for _, ref := range []string{"forward-21", "forward-22"} {
		if got := res.Allocations[Key{RouteID: r.GetId(), NodeRef: ref}]; got.Port != 40002 {
			t.Fatalf("%s: %v", ref, got)
		}
	}
	// When forward-22 holds 40002, both move to the lowest port free on
	// both.
	taken := Allocations{{RouteID: routeID(9), NodeRef: "forward-22"}: {Port: 40002}, {RouteID: routeID(9), HopIndex: 1, NodeRef: "forward-22"}: {Port: 40000}}
	res = mustPlan(t, []*forwardv1.Route{r}, previous, Options{Taken: taken})
	for _, ref := range []string{"forward-21", "forward-22"} {
		if got := res.Allocations[Key{RouteID: r.GetId(), NodeRef: ref}]; got.Port != 40001 {
			t.Fatalf("%s: %v", ref, got)
		}
	}
}

func TestAllocationFailures(t *testing.T) {
	nodes := testNodes()
	nodes[3].PortRange = nil // forward-41
	res, err := Plan([]*forwardv1.Route{twoHop(routeID(1), 0)}, nodes, nil, Options{Cluster: goldenCluster})
	if err != nil || !hasViolation(res, routeID(1), "hops[1].port", validate.CodeNoPortRange) {
		t.Fatalf("no range: %v %v", err, res.Violations)
	}

	reserved := map[string][]uint32{}
	for p := uint32(40000); p <= 40063; p++ {
		reserved["forward-21"] = append(reserved["forward-21"], p)
	}
	res = mustPlan(t, []*forwardv1.Route{twoHop(routeID(1), 0)}, nil, Options{ReservedPorts: reserved})
	if !hasViolation(res, routeID(1), "listen.port", validate.CodePortExhausted) {
		t.Fatalf("exhausted: %v", res.Violations)
	}

	taken := Allocations{}
	for m := uint32(1); m <= MaxMark; m++ {
		taken[Key{RouteID: routeID(9), HopIndex: m, NodeRef: "forward-41"}] = Slot{Mark: m}
	}
	res = mustPlan(t, []*forwardv1.Route{twoHop(routeID(1), 0)}, nil, Options{Taken: taken})
	if !hasViolation(res, routeID(1), "hops[1].node_refs", validate.CodeMarkExhausted) {
		t.Fatalf("marks: %v", res.Violations)
	}

	nodes = testNodes()
	nodes[3].Addresses = nil
	res, err = Plan([]*forwardv1.Route{twoHop(routeID(1), 0)}, nodes, nil, Options{Cluster: goldenCluster})
	if err != nil || !hasViolation(res, routeID(1), "hops[1].node_refs[0]", validate.CodeNoAddress) {
		t.Fatalf("no address: %v %v", err, res.Violations)
	}
}

func TestPlanRefusesInvalidInput(t *testing.T) {
	noID := twoHop("", 0)
	res := mustPlan(t, []*forwardv1.Route{noID, twoHop(routeID(1), 0), twoHop(routeID(1), 0)}, nil, Options{})
	if !hasViolation(res, "", "id", validate.CodeRequired) || !hasViolation(res, routeID(1), "id", validate.CodeDuplicate) {
		t.Fatalf("ids: %v", res.Violations)
	}

	// Validation runs first, with its codes.
	bad := twoHop(routeID(2), 0)
	bad.Hops[1].Ingress.Security = forwardv1.LinkSecurity_LINK_SECURITY_TLS
	res = mustPlan(t, []*forwardv1.Route{bad}, nil, Options{})
	if !hasViolation(res, bad.GetId(), "hops[1].ingress.security", validate.CodeLinkUnsupported) || res.States != nil {
		t.Fatalf("validation: %v", res.Violations)
	}

	// A duplicated inventory node: the last wins, as in validation.
	nodes := append(testNodes(), gostNode("forward-41", 21000, 21009, "203.0.113.141"))
	res, err := Plan([]*forwardv1.Route{twoHop(routeID(1), 0)}, nodes, nil, Options{})
	if err != nil || res.Allocations[Key{RouteID: routeID(1), HopIndex: 1, NodeRef: "forward-41"}].Port != 21000 {
		t.Fatalf("duplicate node: %v %v", err, res.Allocations)
	}

	// Without an inventory nothing can be planned.
	res, err = Plan([]*forwardv1.Route{twoHop(routeID(1), 0)}, nil, nil, Options{})
	if err != nil || !hasViolation(res, routeID(1), "hops[0].node_refs[0]", validate.CodeUnknownNode) {
		t.Fatalf("no inventory: %v %v", err, res.Violations)
	}
}

func TestEncryptedLinkNeedsCluster(t *testing.T) {
	r := twoHop(routeID(1), 0)
	r.Hops[0] = hop(forwardv1.HopRole_HOP_ROLE_ENTRY, forwardv1.Engine_ENGINE_GOST, 0, "forward-31")
	r.Hops[1].Ingress.Security = forwardv1.LinkSecurity_LINK_SECURITY_TLS
	if _, err := Plan([]*forwardv1.Route{r}, testNodes(), nil, Options{}); !errors.Is(err, ErrNoCluster) {
		t.Fatalf("err = %v", err)
	}
	if _, err := Plan([]*forwardv1.Route{r}, testNodes(), nil, Options{Cluster: "Not A Cluster"}); err == nil {
		t.Fatal("an invalid cluster is a misuse")
	}
	// A RAW route needs no cluster.
	if res, err := Plan([]*forwardv1.Route{twoHop(routeID(1), 0)}, testNodes(), nil, Options{}); err != nil || len(res.Violations) > 0 {
		t.Fatalf("raw: %v %v", err, res.Violations)
	}
	// A node id beyond the identity's range cannot be pinned.
	nodes := append(testNodes(), gostNode("forward-99999999999", 20000, 20063, "203.0.113.99"))
	r.Hops[1].NodeRefs = []string{"forward-99999999999"}
	res, err := Plan([]*forwardv1.Route{r}, nodes, nil, Options{Cluster: goldenCluster})
	if err != nil || !hasViolation(res, r.GetId(), "hops[1].node_refs[0]", validate.CodeInvalidFormat) {
		t.Fatalf("identity: %v %v", err, res.Violations)
	}
}

func TestWiring(t *testing.T) {
	r := &forwardv1.Route{
		Id: routeID(1), Owner: "admin", Paused: true,
		Listen: &forwardv1.Listen{Port: 40001, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			hop(forwardv1.HopRole_HOP_ROLE_ENTRY, forwardv1.Engine_ENGINE_NFTABLES, 0, "forward-21"),
			hop(forwardv1.HopRole_HOP_ROLE_RELAY, forwardv1.Engine_ENGINE_GOST, forwardv1.LinkSecurity_LINK_SECURITY_RAW, "forward-31"),
			hop(forwardv1.HopRole_HOP_ROLE_EXIT, forwardv1.Engine_ENGINE_GOST, forwardv1.LinkSecurity_LINK_SECURITY_GRPC, "forward-41", "forward-42"),
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.7", Port: 443, Priority: 4}, {Host: "198.51.100.8", Port: 443, Priority: 2}},
		Policy: &forwardv1.Policy{
			NextHop: forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH, Target: forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
			Health: &forwardv1.HealthCheck{Disabled: true},
		},
		Limits: &forwardv1.Limits{QuotaBytes: 1 << 30},
	}
	r.Hops[2].Ingress.Path = "anixops.Tunnel"
	res := mustPlan(t, []*forwardv1.Route{r}, nil, Options{})
	if len(res.Violations) > 0 {
		t.Fatal(res.Violations)
	}
	entry, relay := res.States["forward-21"].GetHops()[0], res.States["forward-31"].GetHops()[0]
	exit := res.States["forward-42"].GetHops()[0]
	for _, h := range []*forwardv1.NodeHop{entry, relay, exit} {
		if !h.GetPaused() || !h.GetHealth().GetDisabled() || h.GetHealth().GetIntervalMs() != 5000 {
			t.Fatalf("paused route, health defaults: %v", h)
		}
	}
	if !proto.Equal(entry.GetLimits(), r.GetLimits()) || relay.GetLimits() != nil || exit.GetLimits() != nil {
		t.Fatal("limits land on the entry only")
	}
	if entry.GetBalance() != forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH ||
		exit.GetBalance() != forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER {
		t.Fatal("balance: next_hop towards nodes, target towards targets")
	}
	if len(relay.GetUpstreams()) != 2 || relay.GetUpstreams()[1].GetPriority() != 1 ||
		relay.GetUpstreams()[1].GetEgress().GetServerName() != "forward-42" ||
		relay.GetUpstreams()[1].GetEgress().GetPath() != "anixops.Tunnel" ||
		relay.GetUpstreams()[1].GetPeerIdentity() != "spiffe://anixops/example/agent/forward-42" {
		t.Fatalf("relay upstreams: %v", relay.GetUpstreams())
	}
	if len(entry.GetUpstreams()) != 1 || entry.GetUpstreams()[0].GetPeerIdentity() != "" || len(relay.GetIngressPeers()) != 0 {
		t.Fatal("RAW links pin no identity")
	}
	if fmt.Sprint(exit.GetIngressPeers()) != "[spiffe://anixops/example/agent/forward-31]" ||
		fmt.Sprint(exit.GetIngressSources()) != "[198.51.100.31]" || len(entry.GetIngressSources()) != 0 {
		t.Fatalf("ingress admission: %v", exit)
	}
	if exit.GetUpstreams()[0].GetPriority() != 4 || exit.GetUpstreams()[1].GetAddress() != "198.51.100.8" {
		t.Fatal("targets keep their order and priorities")
	}
}

func TestDirectPreferred(t *testing.T) {
	r := twoHop(routeID(1), 0)
	r.Targets = []*forwardv1.Target{{Host: "198.51.100.7", Port: 443, Priority: 3}}
	r.Policy = &forwardv1.Policy{Direct: forwardv1.DirectMode_DIRECT_MODE_PREFERRED, Target: forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER}
	res := mustPlan(t, []*forwardv1.Route{r}, nil, Options{})
	if len(res.Violations) > 0 {
		t.Fatal(res.Violations)
	}
	ups := res.States["forward-21"].GetHops()[0].GetUpstreams()
	if len(ups) != 2 || ups[0].GetAddress() != "198.51.100.7" || ups[0].GetPriority() != 3 ||
		ups[1].GetNodeRef() != "forward-41" || ups[1].GetPriority() != 4 {
		t.Fatalf("direct preferred: %v", ups)
	}
	if res.States["forward-21"].GetHops()[0].GetBalance() != forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER {
		t.Fatal("the entry balances with policy.target")
	}
}

func TestPlanRoute(t *testing.T) {
	// The request's nodes replace inventory nodes; the route's id may be
	// empty; other routes' ports are taken.
	inventory := testNodes()
	req := &forwardv1.PlanRouteRequest{Route: twoHop("", 0), Nodes: []*forwardv1.NodeInfo{nftNode("forward-21", 41000, 41009, "192.0.2.121")}}
	taken := Allocations{{RouteID: routeID(9), NodeRef: "forward-21"}: {Port: 41000, Mark: 1}}
	resp, err := PlanRoute(req, inventory, nil, Options{Taken: taken})
	if err != nil || len(resp.GetViolations()) > 0 {
		t.Fatalf("%v %v", err, resp.GetViolations())
	}
	states := resp.GetStates()
	if len(states) != 2 || states[0].GetNodeRef() != "forward-21" || states[0].GetHops()[0].GetListen().GetPort() != 41001 ||
		states[0].GetHops()[0].GetMark() != 2 || len(resp.GetAllocations()) != 2 {
		t.Fatalf("plan: %v", resp)
	}
	if fmt.Sprint(states[1].GetHops()[0].GetIngressSources()) != "[192.0.2.121]" {
		t.Fatal("the request's node replaces the inventory's")
	}
}

func TestStamp(t *testing.T) {
	res := mustPlan(t, []*forwardv1.Route{twoHop(routeID(1), 0), twoHop(routeID(2), 0)}, nil, Options{})
	gens := Stamp(res.States, nil)
	for ref, g := range gens {
		if g.Generation != 1 || res.States[ref].GetGeneration() != 1 || res.States[ref].GetStateHash() != g.StateHash || len(g.StateHash) != 64 {
			t.Fatalf("%s: %v", ref, g)
		}
	}
	// Changing route 2's target changes only its exit's state... which is
	// forward-41 for both routes, so forward-41 bumps and forward-21 does
	// not.
	r2 := twoHop(routeID(2), 0)
	r2.Targets[0].Port = 8443
	again := mustPlan(t, []*forwardv1.Route{twoHop(routeID(1), 0), r2}, res.Allocations, Options{})
	next := Stamp(again.States, gens)
	if next["forward-21"] != gens["forward-21"] || next["forward-41"].Generation != 2 || next["forward-31"] != gens["forward-31"] {
		t.Fatalf("generations: %v -> %v", gens, next)
	}
	// Generations of nodes without a state are kept.
	kept := Stamp(map[string]*forwardv1.NodeForwardState{}, map[string]Generation{"forward-9": {Generation: 4, StateHash: "x"}})
	if kept["forward-9"].Generation != 4 {
		t.Fatal(kept)
	}
	if StateHash(nil) != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal("an empty state hashes as empty bytes")
	}
}
