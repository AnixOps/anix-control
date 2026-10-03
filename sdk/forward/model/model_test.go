package model

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func fullRoute() *forwardv1.Route {
	return &forwardv1.Route{
		Id:    "01JF1B000000000000000000B1",
		Owner: "user:1042",
		Name:  "sh-to-jp",
		Listen: &forwardv1.Listen{
			Address: "192.0.2.21", Port: 40001, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP,
			EntryHostname: "r1042.fwd.example.com",
		},
		Hops: []*forwardv1.Hop{
			{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{"forward-21"}},
			{
				Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_GOST, NodeRefs: []string{"forward-41", "forward-42"},
				Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_WSS, Mux: true, ServerName: "x.example", Path: "/t"},
				Port:    20000, DialAddress: "203.0.113.41",
			},
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.7", Port: 8443, Weight: 3, Priority: 10}},
		Policy: &forwardv1.Policy{
			NextHop: forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER, Target: forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH,
			Health:         &forwardv1.HealthCheck{IntervalMs: 7000, TimeoutMs: 1500, Disabled: true},
			CircuitBreaker: &forwardv1.CircuitBreaker{FailureThreshold: 5, OpenMs: 60000},
			Direct:         forwardv1.DirectMode_DIRECT_MODE_PREFERRED, TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		},
		Limits:          &forwardv1.Limits{BandwidthBps: 50_000_000, QuotaBytes: 1 << 40, MaxConns: 500, ExpiresAtUnixMs: 1798761600000},
		Labels:          map[string]string{"link": "iepl"},
		Paused:          true,
		Revision:        7,
		CreatedAtUnixMs: 1790000000000,
		UpdatedAtUnixMs: 1790000000123,
	}
}

func TestRouteRoundTrip(t *testing.T) {
	in := fullRoute()
	r := FromProto(in)
	if got := r.ToProto(); !proto.Equal(in, got) {
		t.Fatalf("round trip changed the route:\n in: %v\nout: %v", in, got)
	}
	if r.Policy.Health.Interval != 7*time.Second || r.Limits.ExpiresAt.UnixMilli() != 1798761600000 {
		t.Fatalf("unexpected conversion: %+v", r)
	}
	if !r.Listen.Protocol.HasUDP() || r.Hops[1].Ingress.Security != LinkSecurityWSS || r.IsAdmin() {
		t.Fatalf("unexpected conversion: %+v", r)
	}
}

func TestNilAndEmpty(t *testing.T) {
	if got := FromProto(nil); !reflect.DeepEqual(got, Route{}) {
		t.Fatalf("nil route: %+v", got)
	}
	empty := Route{}
	p := empty.ToProto()
	if p.Listen != nil || p.Policy != nil || p.Limits != nil || p.Hops != nil || p.Targets != nil {
		t.Fatalf("an empty route must convert without sub-messages: %v", p)
	}
	if proto.Size(p) != 0 {
		t.Fatalf("an empty route must encode to nothing")
	}
	// A present but empty sub-message reads as absent.
	in := &forwardv1.Route{Listen: &forwardv1.Listen{}, Policy: &forwardv1.Policy{Health: &forwardv1.HealthCheck{}}}
	r := FromProto(in)
	if out := r.ToProto(); out.Listen != nil || out.Policy != nil {
		t.Fatalf("empty sub-messages must convert to nil: %v", out)
	}
	// Hops and targets keep their places even when empty.
	r = FromProto(&forwardv1.Route{Hops: []*forwardv1.Hop{{}}, Targets: []*forwardv1.Target{{}}})
	if out := r.ToProto(); len(out.Hops) != 1 || out.Hops[0] == nil || len(out.Targets) != 1 || out.Targets[0] == nil {
		t.Fatalf("empty list entries must keep their place: %v", out)
	}
}

func TestUnknownEnumsSurvive(t *testing.T) {
	in := &forwardv1.Route{
		Listen: &forwardv1.Listen{Protocol: 42},
		Hops:   []*forwardv1.Hop{{Role: 9, Engine: 77, Ingress: &forwardv1.LinkTransport{Security: 99}}},
		Policy: &forwardv1.Policy{NextHop: 12, Target: 13, Direct: 14, TargetPolicy: 15},
	}
	r := FromProto(in)
	if r.Hops[0].Engine.IsKnown() || r.Hops[0].Role.IsKnown() || r.Listen.Protocol.IsKnown() ||
		r.Hops[0].Ingress.Security.IsKnown() || r.Policy.NextHop.IsKnown() || r.Policy.Direct.IsKnown() ||
		r.Policy.TargetPolicy.IsKnown() {
		t.Fatalf("unknown values must not be known")
	}
	if r.Hops[0].Engine.String() != "77" {
		t.Fatalf("an unknown engine prints its number, got %q", r.Hops[0].Engine)
	}
	if out := r.ToProto(); !proto.Equal(in, out) {
		t.Fatalf("unknown enum values must survive:\n in: %v\nout: %v", in, out)
	}
	if EngineGost.String() != "ENGINE_GOST" || !EngineNFTables.IsKnown() {
		t.Fatalf("known engine")
	}
}

func TestDefaults(t *testing.T) {
	p := Policy{}.WithDefaults()
	want := Policy{
		NextHop: BalanceRoundRobin, Target: BalanceRoundRobin, Direct: DirectOff, TargetPolicy: TargetPolicyPublicOnly,
		Health:         HealthCheck{Interval: 5 * time.Second, Timeout: 2 * time.Second},
		CircuitBreaker: CircuitBreaker{FailureThreshold: 3, OpenFor: 30 * time.Second},
	}
	if p != want {
		t.Fatalf("defaults: got %+v, want %+v", p, want)
	}
	set := Policy{
		NextHop: BalanceFailover, Target: BalanceIPHash, Direct: DirectForced, TargetPolicy: TargetPolicyAllowPrivate,
		Health:         HealthCheck{Interval: time.Second, Timeout: time.Millisecond, Disabled: true},
		CircuitBreaker: CircuitBreaker{FailureThreshold: 9, OpenFor: time.Minute},
	}
	if got := set.WithDefaults(); got != set {
		t.Fatalf("set values must be kept: %+v", got)
	}
	if (Target{}).EffectiveWeight() != 1 || (Target{Weight: 4}).EffectiveWeight() != 4 {
		t.Fatalf("effective weight")
	}
	// H21 (decided): least-connections re-weights every 10 seconds.
	if DefaultLeastConnReweight != 10*time.Second {
		t.Fatalf("least-conn re-weighting: %v", DefaultLeastConnReweight)
	}
}

func TestMillisFromDuration(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want uint32
	}{
		{0, 0}, {-time.Second, 0}, {1500 * time.Microsecond, 1}, {time.Second, 1000},
		{time.Duration(math.MaxUint32) * time.Millisecond, math.MaxUint32},
		{time.Duration(math.MaxInt64), math.MaxUint32},
	} {
		if got := millisFromDuration(tc.in); got != tc.want {
			t.Errorf("millisFromDuration(%s) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestNodeAndCountersRoundTrip(t *testing.T) {
	node := &forwardv1.NodeInfo{
		NodeRef: "forward-21", Addresses: []string{"192.0.2.21", "172.16.5.21"},
		PortRange: &forwardv1.PortRange{First: 40000, Last: 49999},
		Engines: []*forwardv1.EngineCapabilities{{
			Engine: forwardv1.Engine_ENGINE_NFTABLES, Version: "nft 1.0.9", Available: true, UnavailableReason: "x",
			Ipv6: true, Udp: true, BandwidthLimit: true, Quota: true, MaxConns: true,
			Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER, 40},
			LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		}},
		Labels: map[string]string{"link": "iepl"},
	}
	n := NodeInfoFromProto(node)
	if out := n.ToProto(); !proto.Equal(node, out) {
		t.Fatalf("node round trip:\n in: %v\nout: %v", node, out)
	}
	caps, ok := n.Engine(EngineNFTables)
	if !ok || !caps.Supports(BalanceFailover) || caps.Supports(BalanceRandom) || !caps.SupportsLink(LinkSecurityRaw) || caps.SupportsLink(LinkSecurityTLS) {
		t.Fatalf("capabilities: %+v", caps)
	}
	if _, ok := n.Engine(EngineGost); ok {
		t.Fatalf("gost is not advertised")
	}
	if !n.PortRange.Contains(40000) || n.PortRange.Contains(50000) || n.PortRange.IsZero() {
		t.Fatalf("port range")
	}
	if got := NodesFromProto([]*forwardv1.NodeInfo{node}); len(got) != 1 || got[0].NodeRef != "forward-21" {
		t.Fatalf("nodes: %+v", got)
	}

	counters := &forwardv1.Counters{
		RouteId: "r", HopIndex: 2, NodeRef: "forward-41", UpBytes: 1, DownBytes: 2, UpPackets: 3, DownPackets: 4,
		ActiveConns: 5, TotalConns: 6, CounterEpoch: "e1", ObservedAtUnixMs: 1790000000000,
	}
	cm := CountersFromProto(counters)
	if out := cm.ToProto(); !proto.Equal(counters, out) {
		t.Fatalf("counters round trip:\n in: %v\nout: %v", counters, out)
	}
}

// normalize clears every set but empty singular sub-message, the one
// difference a round trip may make (doc.go).
func normalize(m protoreflect.Message) {
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList() && fd.Message() != nil:
			for i := 0; i < v.List().Len(); i++ {
				normalize(v.List().Get(i).Message())
			}
		case !fd.IsMap() && !fd.IsList() && fd.Message() != nil:
			normalize(v.Message())
			if proto.Size(v.Message().Interface()) == 0 {
				m.Clear(fd)
			}
		}
		return true
	})
}

// randomRoute builds a route with random values: unknown enum numbers,
// extreme numbers, nil and empty sub-messages, nil and empty lists.
func randomRoute(rng *rand.Rand) *forwardv1.Route {
	str := func() string {
		return []string{"", "a", "forward-1", "198.51.100.7", "::1", "ü", "x.example"}[rng.Intn(7)]
	}
	u32 := func() uint32 { return []uint32{0, 1, 65535, 65536, math.MaxUint32, rng.Uint32()}[rng.Intn(6)] }
	u64 := func() uint64 { return []uint64{0, 1, math.MaxUint64, rng.Uint64()}[rng.Intn(4)] }
	i64 := func() int64 { return []int64{0, 1, -1, math.MaxInt64, math.MinInt64, rng.Int63()}[rng.Intn(6)] }
	enum := func() int32 { return int32(rng.Intn(9)) - 1 }
	maybe := func() bool { return rng.Intn(3) > 0 }

	r := &forwardv1.Route{
		Id: str(), Owner: str(), Name: str(), Paused: maybe(), Revision: u64(),
		CreatedAtUnixMs: i64(), UpdatedAtUnixMs: i64(),
	}
	if maybe() {
		r.Listen = &forwardv1.Listen{Address: str(), Port: u32(), Protocol: forwardv1.L4Protocol(enum()), EntryHostname: str()}
	}
	for i := rng.Intn(4); i > 0; i-- {
		h := &forwardv1.Hop{Role: forwardv1.HopRole(enum()), Engine: forwardv1.Engine(enum()), Port: u32(), DialAddress: str()}
		for j := rng.Intn(3); j > 0; j-- {
			h.NodeRefs = append(h.NodeRefs, str())
		}
		if maybe() {
			h.Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity(enum()), Mux: maybe(), ServerName: str(), Path: str()}
		}
		r.Hops = append(r.Hops, h)
	}
	for i := rng.Intn(4); i > 0; i-- {
		r.Targets = append(r.Targets, &forwardv1.Target{Host: str(), Port: u32(), Weight: u32(), Priority: u32()})
	}
	if maybe() {
		r.Policy = &forwardv1.Policy{
			NextHop: forwardv1.BalanceStrategy(enum()), Target: forwardv1.BalanceStrategy(enum()),
			Direct: forwardv1.DirectMode(enum()), TargetPolicy: forwardv1.TargetPolicy(enum()),
		}
		if maybe() {
			r.Policy.Health = &forwardv1.HealthCheck{IntervalMs: u32(), TimeoutMs: u32(), Disabled: maybe()}
		}
		if maybe() {
			r.Policy.CircuitBreaker = &forwardv1.CircuitBreaker{FailureThreshold: u32(), OpenMs: u32()}
		}
	}
	if maybe() {
		r.Limits = &forwardv1.Limits{BandwidthBps: u64(), QuotaBytes: u64(), MaxConns: u32(), ExpiresAtUnixMs: i64()}
	}
	switch rng.Intn(3) {
	case 0:
		r.Labels = map[string]string{}
	case 1:
		r.Labels = map[string]string{str(): str(), "k": str()}
	}
	return r
}

// TestRoundTripProperty: for any contract route, ToProto(FromProto(r))
// equals r up to empty sub-messages.
func TestRoundTripProperty(t *testing.T) {
	property := func(seed int64) bool {
		in := randomRoute(rand.New(rand.NewSource(seed))) // #nosec G404 -- deterministic test data
		r := FromProto(in)
		out := r.ToProto()
		normalize(in.ProtoReflect())
		if !proto.Equal(in, out) {
			t.Logf("seed %d:\n in: %v\nout: %v", seed, in, out)
			return false
		}
		return true
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatal(err)
	}
}
