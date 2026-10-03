package planner

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// update rewrites the golden fixtures from the planner's output:
//
//	go -C sdk test ./forward/planner -run Golden -update
var update = flag.Bool("update", false, "rewrite the golden fixtures in contracts/forward/v1")

// goldenCluster is the cluster of the Agent identities in the fixtures.
const goldenCluster = "example"

// planRouteFixtures are PlanRoute goldens: a PlanRouteRequest, optional
// planner inputs the contract's request does not carry, and the
// PlanRouteResponse.
var planRouteFixtures = []string{
	"plan-single-hop-nftables-iepl.json",
	"plan-nft-entry-gost-relay-exit-failover.json",
	"plan-gost-udp-ipv6-targets.json",
	"plan-multi-entry-hostname-failover.json",
	"plan-sticky-replan-added-target.json",
	"plan-port-exhausted.json",
}

// planFixtures are Plan goldens: several routes merged into node states,
// stamped with generations.
var planFixtures = []string{
	"plan-node-states-generations.json",
}

// allocationJSON is an allocation in a fixture: the contract's
// PortAllocation plus the mark, which the contract carries in NodeHop.
type allocationJSON struct {
	RouteID  string `json:"route_id"`
	HopIndex uint32 `json:"hop_index"`
	NodeRef  string `json:"node_ref"`
	Port     uint32 `json:"port"`
	Mark     uint32 `json:"mark,omitempty"`
}

type generationJSON struct {
	Generation uint64 `json:"generation"`
	StateHash  string `json:"state_hash"`
}

// fixture is a golden file. Field order is the file's key order.
type fixture struct {
	Comment string `json:"$comment"`
	Name    string `json:"name"`
	Status  string `json:"status"`

	// PlanRoute inputs and output.
	Request json.RawMessage `json:"request,omitempty"`

	// Plan inputs.
	Routes []json.RawMessage `json:"routes,omitempty"`
	Nodes  []json.RawMessage `json:"nodes,omitempty"`

	// Planner inputs the contract's request does not carry.
	PreviousAllocations []allocationJSON          `json:"previous_allocations,omitempty"`
	TakenAllocations    []allocationJSON          `json:"taken_allocations,omitempty"`
	ReservedPorts       map[string][]uint32       `json:"reserved_ports,omitempty"`
	PreviousGenerations map[string]generationJSON `json:"previous_generations,omitempty"`

	// PlanRoute output.
	Response json.RawMessage `json:"response,omitempty"`

	// Plan output.
	States      []json.RawMessage         `json:"states,omitempty"`
	Allocations []allocationJSON          `json:"allocations,omitempty"`
	Warnings    []string                  `json:"warnings,omitempty"`
	Generations map[string]generationJSON `json:"generations,omitempty"`
}

func fixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "contracts", "forward", "v1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the forward fixtures are not at %s: %v", dir, err)
	}
	return dir
}

func readFixture(t *testing.T, name string) (fixture, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir(t), name)) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if f.Status != "draft" {
		t.Fatalf("%s: forward fixtures stay drafts until the contract is served (F3a)", name)
	}
	return f, raw
}

// encodeProto answers m as compact protojson with the proto field names.
func encodeProto(t *testing.T, m proto.Message) json.RawMessage {
	t.Helper()
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, b); err != nil {
		t.Fatal(err)
	}
	return compact.Bytes()
}

func decodeProto(t *testing.T, raw json.RawMessage, m proto.Message, what string) {
	t.Helper()
	if err := protojson.Unmarshal(raw, m); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// encodeFixture answers the canonical bytes of a fixture: two-space
// indentation, keys in fixture order (maps sorted), no HTML escaping, a
// final newline.
func encodeFixture(t *testing.T, f fixture) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func allocationsFromJSON(in []allocationJSON) Allocations {
	if len(in) == 0 {
		return nil
	}
	out := Allocations{}
	for _, a := range in {
		out[Key{RouteID: a.RouteID, HopIndex: a.HopIndex, NodeRef: a.NodeRef}] = Slot{Port: a.Port, Mark: a.Mark}
	}
	return out
}

func allocationsToJSON(a Allocations) []allocationJSON {
	var out []allocationJSON
	for _, s := range a.Sorted() {
		out = append(out, allocationJSON{RouteID: s.RouteID, HopIndex: s.HopIndex, NodeRef: s.NodeRef, Port: s.Port, Mark: s.Mark})
	}
	return out
}

func generationsFromJSON(in map[string]generationJSON) map[string]Generation {
	out := map[string]Generation{}
	for ref, g := range in {
		out[ref] = Generation(g)
	}
	return out
}

func generationsToJSON(in map[string]Generation) map[string]generationJSON {
	out := map[string]generationJSON{}
	for ref, g := range in {
		out[ref] = generationJSON(g)
	}
	return out
}

// checkGolden compares want with the file, or rewrites the file with
// -update.
func checkGolden(t *testing.T, name string, got, file []byte) {
	t.Helper()
	if bytes.Equal(got, file) {
		return
	}
	if *update {
		if err := os.WriteFile(filepath.Join(fixtureDir(t), name), got, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", name)
		return
	}
	t.Errorf("%s differs from the planner's output (rerun with -update to accept it):\n%s", name, firstDifference(file, got))
}

func firstDifference(want, got []byte) string {
	w, g := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d:\n file:    %s\n planner: %s", i+1, wl, gl)
		}
	}
	return "(trailing bytes differ)"
}

func goldenOptions(f fixture) Options {
	return Options{Cluster: goldenCluster, ReservedPorts: f.ReservedPorts, Taken: allocationsFromJSON(f.TakenAllocations)}
}

func TestGoldenPlanRoute(t *testing.T) {
	for _, name := range planRouteFixtures {
		t.Run(name, func(t *testing.T) {
			f, file := readFixture(t, name)
			req := &forwardv1.PlanRouteRequest{}
			decodeProto(t, f.Request, req, name+": request")
			resp, err := PlanRoute(req, nil, allocationsFromJSON(f.PreviousAllocations), goldenOptions(f))
			if err != nil {
				t.Fatal(err)
			}
			// The plan is deterministic: a second run answers the same.
			again, err := PlanRoute(req, nil, allocationsFromJSON(f.PreviousAllocations), goldenOptions(f))
			if err != nil || !proto.Equal(resp, again) {
				t.Fatalf("a second plan differs: %v", err)
			}
			f.Request = encodeProto(t, req)
			f.Response = encodeProto(t, resp)
			checkGolden(t, name, encodeFixture(t, f), file)
		})
	}
}

func TestGoldenPlan(t *testing.T) {
	for _, name := range planFixtures {
		t.Run(name, func(t *testing.T) {
			f, file := readFixture(t, name)
			routes := make([]*forwardv1.Route, len(f.Routes))
			for i, raw := range f.Routes {
				routes[i] = &forwardv1.Route{}
				decodeProto(t, raw, routes[i], fmt.Sprintf("%s: routes[%d]", name, i))
				f.Routes[i] = encodeProto(t, routes[i])
			}
			nodes := make([]*forwardv1.NodeInfo, len(f.Nodes))
			for i, raw := range f.Nodes {
				nodes[i] = &forwardv1.NodeInfo{}
				decodeProto(t, raw, nodes[i], fmt.Sprintf("%s: nodes[%d]", name, i))
				f.Nodes[i] = encodeProto(t, nodes[i])
			}
			previous := allocationsFromJSON(f.PreviousAllocations)
			res, err := Plan(routes, nodes, previous, goldenOptions(f))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Violations) > 0 {
				t.Fatalf("violations: %v", res.Violations)
			}
			generations := Stamp(res.States, generationsFromJSON(f.PreviousGenerations))
			f.States = nil
			for _, s := range SortedStates(res.States) {
				f.States = append(f.States, encodeProto(t, s))
			}
			f.Allocations = allocationsToJSON(res.Allocations)
			f.Warnings = res.Warnings
			f.Generations = generationsToJSON(generations)
			checkGolden(t, name, encodeFixture(t, f), file)

			// Re-planning the same routes with what this plan answered
			// changes nothing: same allocations, same generations.
			again, err := Plan(routes, nodes, res.Allocations, goldenOptions(f))
			if err != nil || len(again.Violations) > 0 {
				t.Fatalf("re-plan: %v %v", err, again.Violations)
			}
			regenerations := Stamp(again.States, generations)
			if fmt.Sprint(again.Allocations.Sorted()) != fmt.Sprint(res.Allocations.Sorted()) {
				t.Fatalf("re-plan moved allocations:\n%v\n%v", res.Allocations.Sorted(), again.Allocations.Sorted())
			}
			for ref, g := range generations {
				if regenerations[ref] != g {
					t.Fatalf("re-plan bumped %s: %v -> %v", ref, g, regenerations[ref])
				}
			}
		})
	}
}

// TestGoldenNegative runs the refused routes of plan-negative.json through
// the planner: each is refused with its violation and nothing is planned.
func TestGoldenNegative(t *testing.T) {
	var negative struct {
		NodesFrom string `json:"nodes_from"`
		Cases     []struct {
			Name  string          `json:"name"`
			Route json.RawMessage `json:"route"`
			Field string          `json:"field"`
			Code  string          `json:"code"`
		} `json:"cases"`
	}
	raw, err := os.ReadFile(filepath.Join(fixtureDir(t), "plan-negative.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &negative); err != nil {
		t.Fatal(err)
	}
	f, _ := readFixture(t, negative.NodesFrom)
	nodesReq := &forwardv1.PlanRouteRequest{}
	decodeProto(t, f.Request, nodesReq, negative.NodesFrom)
	for _, c := range negative.Cases {
		t.Run(c.Name, func(t *testing.T) {
			route := &forwardv1.Route{}
			decodeProto(t, c.Route, route, c.Name)
			resp, err := PlanRoute(&forwardv1.PlanRouteRequest{Route: route, Nodes: nodesReq.GetNodes()}, nil, nil, Options{Cluster: goldenCluster})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.GetStates()) > 0 || len(resp.GetAllocations()) > 0 {
				t.Fatalf("a refused route is not planned: %v", resp)
			}
			found := false
			for _, v := range resp.GetViolations() {
				found = found || (v.GetField() == c.Field && v.GetCode() == c.Code)
			}
			if !found {
				t.Fatalf("want %s at %s, got %v", c.Code, c.Field, resp.GetViolations())
			}
		})
	}
}
