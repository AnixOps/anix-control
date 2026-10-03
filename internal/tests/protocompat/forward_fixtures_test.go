package protocompat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The forward fixtures (contracts/forward/v1) are the draft planner goldens
// of docs/architecture/forward-sdk.md. Until the planner exists (F1c),
// these tests keep them parseable as the draft contract, internally
// consistent, and in agreement with the shared validation
// (sdk/forward/validate); the planner's own tests will compare its output
// with them.

func forwardFixturePath(name string) string {
	return filepath.Join("..", "..", "..", "contracts", "forward", "v1", name)
}

func readForwardFixture(t *testing.T, name string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(forwardFixturePath(name))
	require.NoError(t, err)
	var fixture map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fixture), name)
	var status string
	require.NoError(t, json.Unmarshal(fixture["status"], &status), name)
	require.Equal(t, "draft", status, "%s: forward fixtures stay drafts until H11", name)
	return fixture
}

func unmarshalForward(t *testing.T, raw json.RawMessage, message proto.Message, what string) {
	t.Helper()
	require.NotEmpty(t, raw, what)
	require.NoError(t, protojson.UnmarshalOptions{}.Unmarshal(raw, message), what)
}

func TestForwardPlanFixturesAreConsistent(t *testing.T) {
	for _, name := range []string{
		"plan-single-hop-nftables-iepl.json",
		"plan-nft-entry-gost-relay-exit-failover.json",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := readForwardFixture(t, name)
			request := &forwardv1.PlanRouteRequest{}
			response := &forwardv1.PlanRouteResponse{}
			unmarshalForward(t, fixture["request"], request, name+": request")
			unmarshalForward(t, fixture["response"], response, name+": response")
			checkForwardPlan(t, request, response)
			// The shared validation (sdk/forward/validate) accepts the
			// fixture's route with the fixture's nodes.
			require.Empty(t, validate.PlanRequest(request, validate.Options{}), name)
		})
	}
}

func checkForwardPlan(t *testing.T, request *forwardv1.PlanRouteRequest, response *forwardv1.PlanRouteResponse) {
	t.Helper()
	route := request.GetRoute()
	require.NotEmpty(t, route.GetId())
	require.NotEmpty(t, route.GetHops())
	require.Empty(t, response.GetViolations())

	nodes := map[string]*forwardv1.NodeInfo{}
	for _, node := range request.GetNodes() {
		nodes[node.GetNodeRef()] = node
	}
	allocated := map[string]uint32{}
	for _, allocation := range response.GetAllocations() {
		require.Equal(t, route.GetId(), allocation.GetRouteId())
		allocated[forwardHopKey(allocation.GetHopIndex(), allocation.GetNodeRef())] = allocation.GetPort()
	}

	hops := route.GetHops()
	planned := map[string]*forwardv1.NodeHop{}
	for _, state := range response.GetStates() {
		require.Contains(t, nodes, state.GetNodeRef())
		require.Zero(t, state.GetGeneration(), "a plan carries no generation")
		require.Empty(t, state.GetStateHash(), "a plan carries no state hash")
		for _, hop := range state.GetHops() {
			require.Equal(t, route.GetId(), hop.GetRouteId())
			require.Less(t, int(hop.GetHopIndex()), len(hops))
			spec := hops[hop.GetHopIndex()]
			require.Equal(t, spec.GetRole(), hop.GetRole())
			require.Equal(t, spec.GetEngine(), hop.GetEngine())
			require.Contains(t, spec.GetNodeRefs(), state.GetNodeRef())
			key := forwardHopKey(hop.GetHopIndex(), state.GetNodeRef())
			planned[key] = hop

			port := hop.GetListen().GetPort()
			require.Equal(t, allocated[key], port, "allocation of %s", key)
			portRange := nodes[state.GetNodeRef()].GetPortRange()
			require.GreaterOrEqual(t, port, portRange.GetFirst(), key)
			require.LessOrEqual(t, port, portRange.GetLast(), key)

			// Limits land on the entry only (section 5.3).
			if hop.GetHopIndex() == 0 {
				require.Empty(t, hop.GetIngressSources(), key)
				require.True(t, proto.Equal(route.GetLimits(), hop.GetLimits()), key)
			} else {
				require.Nil(t, hop.GetLimits(), key)
			}
			// nftables neither terminates nor originates an encrypted link.
			if hop.GetEngine() == forwardv1.Engine_ENGINE_NFTABLES {
				require.Equal(t, forwardv1.LinkSecurity_LINK_SECURITY_RAW, hop.GetIngress().GetSecurity(), key)
			}
			for _, upstream := range hop.GetUpstreams() {
				if hop.GetEngine() == forwardv1.Engine_ENGINE_NFTABLES {
					require.Equal(t, forwardv1.LinkSecurity_LINK_SECURITY_RAW, upstream.GetEgress().GetSecurity(), key)
				}
			}
		}
	}

	// Every node of every hop runs it, and each hop dials the next hop's
	// nodes (or the targets) on the port and transport the next hop listens
	// with.
	for index, spec := range hops {
		for _, nodeRef := range spec.GetNodeRefs() {
			hop := planned[forwardHopKey(index, nodeRef)]
			require.NotNil(t, hop, "hop %d on %s is not planned", index, nodeRef)
			if index == len(hops)-1 {
				require.Len(t, hop.GetUpstreams(), len(route.GetTargets()))
				for i, upstream := range hop.GetUpstreams() {
					require.Equal(t, route.GetTargets()[i].GetHost(), upstream.GetAddress())
					require.Equal(t, route.GetTargets()[i].GetPort(), upstream.GetPort())
					require.Empty(t, upstream.GetNodeRef())
				}
				continue
			}
			next := hops[index+1]
			var sources []string
			for _, ref := range spec.GetNodeRefs() {
				sources = append(sources, nodes[ref].GetAddresses()...)
			}
			for _, nextRef := range next.GetNodeRefs() {
				nextHop := planned[forwardHopKey(index+1, nextRef)]
				require.NotNil(t, nextHop)
				require.ElementsMatch(t, sources, nextHop.GetIngressSources(),
					"hop %d on %s admits only the previous hop's nodes", index+1, nextRef)
				if nextHop.GetIngress().GetSecurity() != forwardv1.LinkSecurity_LINK_SECURITY_RAW {
					require.Len(t, nextHop.GetIngressPeers(), len(spec.GetNodeRefs()))
				}
			}
			require.Len(t, hop.GetUpstreams(), len(next.GetNodeRefs()))
			for i, upstream := range hop.GetUpstreams() {
				require.Equal(t, next.GetNodeRefs()[i], upstream.GetNodeRef())
				nextHop := planned[forwardHopKey(index+1, upstream.GetNodeRef())]
				require.NotNil(t, nextHop)
				require.Equal(t, nextHop.GetListen().GetPort(), upstream.GetPort())
				require.Equal(t, next.GetIngress().GetSecurity(), upstream.GetEgress().GetSecurity())
				require.Equal(t, nextHop.GetIngress().GetSecurity(), upstream.GetEgress().GetSecurity())
				if upstream.GetEgress().GetSecurity() != forwardv1.LinkSecurity_LINK_SECURITY_RAW {
					require.True(t, strings.HasSuffix(upstream.GetPeerIdentity(), "/agent/"+upstream.GetNodeRef()),
						"an encrypted link pins the next node's Agent identity")
				}
			}
		}
	}
}

func forwardHopKey[I int | uint32](index I, nodeRef string) string {
	return fmt.Sprintf("%s#%d", nodeRef, index)
}

func TestForwardNegativeFixturesAreWellFormed(t *testing.T) {
	fixture := readForwardFixture(t, "plan-negative.json")
	var nodesFrom string
	require.NoError(t, json.Unmarshal(fixture["nodes_from"], &nodesFrom))
	_, err := os.Stat(forwardFixturePath(nodesFrom))
	require.NoError(t, err)

	var cases []struct {
		Name   string          `json:"name"`
		Route  json.RawMessage `json:"route"`
		Field  string          `json:"field"`
		Reason string          `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(fixture["cases"], &cases))
	require.NotEmpty(t, cases)

	// The cases run against the nodes of the fixture they name.
	nodesFixture := readForwardFixture(t, nodesFrom)
	nodesRequest := &forwardv1.PlanRouteRequest{}
	unmarshalForward(t, nodesFixture["request"], nodesRequest, nodesFrom)
	require.NotEmpty(t, nodesRequest.GetNodes())

	// The code each documented reason maps to in sdk/forward/validate.
	wantCodes := map[string]validate.Code{
		"nftables hop cannot originate TLS":                    validate.CodeLinkUnsupported,
		"user route to a private target":                       validate.CodeForbidden,
		"loopback target is refused even for an administrator": validate.CodeTargetNotAllowed,
		"engine the node does not advertise":                   validate.CodeEngineNotAdvertised,
		"dial_address with several nodes":                      validate.CodeRequiresSingleNode,
	}
	seen := map[string]bool{}
	for _, testCase := range cases {
		require.False(t, seen[testCase.Name], "duplicate case %s", testCase.Name)
		seen[testCase.Name] = true
		route := &forwardv1.Route{}
		unmarshalForward(t, testCase.Route, route, testCase.Name)
		require.NotEmpty(t, testCase.Field, testCase.Name)
		require.NotEmpty(t, testCase.Reason, testCase.Name)

		code, ok := wantCodes[testCase.Name]
		require.True(t, ok, "%s: map the case to its validation code", testCase.Name)
		violations := validate.PlanRequest(&forwardv1.PlanRouteRequest{Route: route, Nodes: nodesRequest.GetNodes()}, validate.Options{})
		require.True(t, violations.Has(testCase.Field, code),
			"%s: want %s at %s (%s), got %v", testCase.Name, code, testCase.Field, testCase.Reason, violations)
		// Each case breaks one rule. The user's private target is also
		// refused as a target, since a user's route is checked as
		// PUBLIC_ONLY.
		for _, violation := range violations {
			if testCase.Name == "user route to a private target" && violation.Field == "targets[0].host" {
				continue
			}
			require.Equal(t, testCase.Field, violation.Field, "%s: unexpected %v", testCase.Name, violation)
		}
	}
	require.Len(t, seen, len(wantCodes))
}
