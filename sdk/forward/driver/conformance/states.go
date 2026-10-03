package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Topology is what the generated states use: the node's name, the
// addresses its hops dial and the ports they listen on. A real harness
// sets them to its namespaces' addresses.
type Topology struct {
	NodeRef string
	// UpstreamsV4 and UpstreamsV6 are dialled by the hops, three of each
	// (the targets of a netns harness). UpstreamPort is their port.
	UpstreamsV4  []string
	UpstreamsV6  []string
	UpstreamPort uint32
	// ListenPorts are ports the driver may listen on, at least four.
	ListenPorts []uint32
	// IngressSources is the previous hop's address for RELAY hops.
	IngressSources []string
}

// DefaultTopology uses documentation addresses (RFC 5737, RFC 3849).
func DefaultTopology() Topology {
	return Topology{
		NodeRef:        "forward-11",
		UpstreamsV4:    []string{"192.0.2.20", "192.0.2.21", "192.0.2.22"},
		UpstreamsV6:    []string{"2001:db8::20", "2001:db8::21", "2001:db8::22"},
		UpstreamPort:   443,
		ListenPorts:    []uint32{30001, 30002, 30003, 30004},
		IngressSources: []string{"198.51.100.7"},
	}
}

func (t Topology) check() error {
	switch {
	case t.NodeRef == "":
		return fmt.Errorf("topology: empty NodeRef")
	case len(t.UpstreamsV4) < 3, len(t.UpstreamsV6) < 3:
		return fmt.Errorf("topology: need three IPv4 and three IPv6 upstreams")
	case t.UpstreamPort == 0:
		return fmt.Errorf("topology: UpstreamPort unset")
	case len(t.ListenPorts) < 4:
		return fmt.Errorf("topology: need four listen ports")
	}
	return nil
}

// Route ids of the generated states.
const (
	RouteA = "01JF2A000000000000000000A1"
	RouteB = "01JF2A000000000000000000B1"
	RouteC = "01JF2A000000000000000000C1"
)

// StateCase is one data-driven state the suite renders, applies, observes
// and removes on every driver whose capabilities cover it (forward-sdk.md
// section 13: single target, each strategy, failover, limits, IPv4 and
// IPv6, TCP and UDP, several routes).
type StateCase struct {
	Name string
	// Requires answers why the capabilities cannot run the case, or "".
	Requires func(c *forwardv1.EngineCapabilities) string
	// Hops builds the case's hops for the engine.
	Hops func(b Builder) []*forwardv1.NodeHop
}

// Builder makes hops for one engine on one topology.
type Builder struct {
	Engine forwardv1.Engine
	Caps   *forwardv1.EngineCapabilities
	Top    Topology
}

// Strategy answers s when the engine supports it, else the first strategy
// it does.
func (b Builder) Strategy(s forwardv1.BalanceStrategy) forwardv1.BalanceStrategy {
	if slices.Contains(b.Caps.GetStrategies(), s) || len(b.Caps.GetStrategies()) == 0 {
		return s
	}
	return b.Caps.GetStrategies()[0]
}

// Link answers RAW when supported, else the engine's first link security.
func (b Builder) Link() *forwardv1.LinkTransport {
	s := forwardv1.LinkSecurity_LINK_SECURITY_RAW
	if ls := b.Caps.GetLinkSecurities(); len(ls) > 0 && !slices.Contains(ls, s) {
		s = ls[0]
	}
	return &forwardv1.LinkTransport{Security: s}
}

// Upstreams answers n upstreams from addrs with weights 1, 2, 3... and
// priorities 0, 10, 20...
func (b Builder) Upstreams(addrs []string, n int) []*forwardv1.Upstream {
	out := make([]*forwardv1.Upstream, 0, n)
	for i := range n {
		out = append(out, &forwardv1.Upstream{
			Address:  addrs[i],
			Port:     b.Top.UpstreamPort,
			Weight:   uint32(i + 1),  // #nosec G115 -- i < 3
			Priority: uint32(i * 10), // #nosec G115 -- i < 3
			Egress:   b.Link(),
		})
	}
	return out
}

// Entry answers an ENTRY hop of route on the listen port with index port.
func (b Builder) Entry(route string, port int, p forwardv1.L4Protocol, s forwardv1.BalanceStrategy, ups []*forwardv1.Upstream) *forwardv1.NodeHop {
	return &forwardv1.NodeHop{
		RouteId:        route,
		HopIndex:       0,
		Role:           forwardv1.HopRole_HOP_ROLE_ENTRY,
		Engine:         b.Engine,
		Listen:         &forwardv1.Listen{Port: b.Top.ListenPorts[port], Protocol: p},
		Ingress:        b.Link(),
		Upstreams:      ups,
		Balance:        b.Strategy(s),
		Health:         &forwardv1.HealthCheck{IntervalMs: 5000, TimeoutMs: 2000},
		CircuitBreaker: &forwardv1.CircuitBreaker{FailureThreshold: 3, OpenMs: 30000},
		TargetPolicy:   forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		// The planner's mark is an index (1..4095) that the driver shifts
		// into its mark mask (forward-sdk.md section 6.1).
		Mark: uint32(port) + 1, // #nosec G115 -- port < 4
	}
}

// Relay answers a RELAY hop (hop index 1) of route.
func (b Builder) Relay(route string, port int, ups []*forwardv1.Upstream) *forwardv1.NodeHop {
	h := b.Entry(route, port, forwardv1.L4Protocol_L4_PROTOCOL_TCP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, ups)
	h.HopIndex = 1
	h.Role = forwardv1.HopRole_HOP_ROLE_RELAY
	h.IngressSources = slices.Clone(b.Top.IngressSources)
	return h
}

// Simple answers the base hop most scenarios use: one route, entry on
// listen port index port, TCP, three IPv4 upstreams.
func (b Builder) Simple(route string, port int) *forwardv1.NodeHop {
	return b.Entry(route, port, forwardv1.L4Protocol_L4_PROTOCOL_TCP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER, b.Upstreams(b.Top.UpstreamsV4, 3))
}

func needStrategy(s forwardv1.BalanceStrategy) func(*forwardv1.EngineCapabilities) string {
	return func(c *forwardv1.EngineCapabilities) string {
		if !slices.Contains(c.GetStrategies(), s) {
			return "strategy " + s.String() + " not supported"
		}
		return ""
	}
}

func needUDP(c *forwardv1.EngineCapabilities) string {
	if !c.GetUdp() {
		return "udp not supported"
	}
	return ""
}

func needIPv6(c *forwardv1.EngineCapabilities) string {
	if !c.GetIpv6() {
		return "ipv6 not supported"
	}
	return ""
}

func anyStrategy(c *forwardv1.EngineCapabilities) string {
	if len(c.GetStrategies()) == 0 {
		return "no strategy supported"
	}
	return ""
}

// StateCases answers the data-driven states.
func StateCases() []StateCase {
	cases := []StateCase{{
		Name:     "single-target",
		Requires: anyStrategy,
		Hops: func(b Builder) []*forwardv1.NodeHop {
			return []*forwardv1.NodeHop{b.Entry(RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_TCP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, b.Upstreams(b.Top.UpstreamsV4, 1))}
		},
	}}
	for _, s := range []forwardv1.BalanceStrategy{
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
	} {
		cases = append(cases, StateCase{
			Name:     "strategy-" + s.String(),
			Requires: needStrategy(s),
			Hops: func(b Builder) []*forwardv1.NodeHop {
				return []*forwardv1.NodeHop{b.Entry(RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_TCP, s, b.Upstreams(b.Top.UpstreamsV4, 3))}
			},
		})
	}
	cases = append(cases,
		StateCase{
			Name:     "tcp-udp",
			Requires: needUDP,
			Hops: func(b Builder) []*forwardv1.NodeHop {
				return []*forwardv1.NodeHop{b.Entry(RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, b.Upstreams(b.Top.UpstreamsV4, 2))}
			},
		},
		StateCase{
			Name:     "udp",
			Requires: needUDP,
			Hops: func(b Builder) []*forwardv1.NodeHop {
				return []*forwardv1.NodeHop{b.Entry(RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_UDP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, b.Upstreams(b.Top.UpstreamsV4, 2))}
			},
		},
		StateCase{
			Name:     "ipv6",
			Requires: needIPv6,
			Hops: func(b Builder) []*forwardv1.NodeHop {
				return []*forwardv1.NodeHop{b.Entry(RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_TCP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, b.Upstreams(b.Top.UpstreamsV6, 2))}
			},
		},
		StateCase{
			Name:     "dual-stack",
			Requires: needIPv6,
			Hops: func(b Builder) []*forwardv1.NodeHop {
				ups := append(b.Upstreams(b.Top.UpstreamsV4, 2), b.Upstreams(b.Top.UpstreamsV6, 2)...)
				return []*forwardv1.NodeHop{b.Entry(RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_TCP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, ups)}
			},
		},
		StateCase{
			Name: "limits",
			Requires: func(c *forwardv1.EngineCapabilities) string {
				if !c.GetBandwidthLimit() && !c.GetQuota() && !c.GetMaxConns() {
					return "no limit supported"
				}
				return anyStrategy(c)
			},
			Hops: func(b Builder) []*forwardv1.NodeHop {
				h := b.Simple(RouteA, 0)
				h.Limits = &forwardv1.Limits{}
				if b.Caps.GetBandwidthLimit() {
					h.Limits.BandwidthBps = 100_000_000
				}
				if b.Caps.GetQuota() {
					h.Limits.QuotaBytes = 1 << 40
				}
				if b.Caps.GetMaxConns() {
					h.Limits.MaxConns = 2000
				}
				return []*forwardv1.NodeHop{h}
			},
		},
		StateCase{
			Name:     "several-routes",
			Requires: anyStrategy,
			Hops: func(b Builder) []*forwardv1.NodeHop {
				return []*forwardv1.NodeHop{b.Simple(RouteA, 0), b.Relay(RouteB, 1, b.Upstreams(b.Top.UpstreamsV4, 2)), b.Simple(RouteC, 2)}
			},
		},
		StateCase{
			Name:     "paused",
			Requires: anyStrategy,
			Hops: func(b Builder) []*forwardv1.NodeHop {
				h := b.Simple(RouteA, 0)
				h.Paused = true
				return []*forwardv1.NodeHop{h, b.Simple(RouteB, 1)}
			},
		},
	)
	return cases
}

// State wraps hops into a node state with a generation and a state_hash
// computed as the planner does (SHA-256 of the deterministic encoding).
func State(nodeRef string, generation uint64, hops ...*forwardv1.NodeHop) *forwardv1.NodeForwardState {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(&forwardv1.NodeForwardState{Hops: hops})
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(b)
	return &forwardv1.NodeForwardState{NodeRef: nodeRef, Generation: generation, StateHash: hex.EncodeToString(sum[:]), Hops: hops}
}
