package driver_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/fake"
)

const (
	nft  = forwardv1.Engine_ENGINE_NFTABLES
	gost = forwardv1.Engine_ENGINE_GOST
)

func hop(route string, idx uint32, e forwardv1.Engine) *forwardv1.NodeHop {
	return &forwardv1.NodeHop{
		RouteId:   route,
		HopIndex:  idx,
		Role:      forwardv1.HopRole_HOP_ROLE_ENTRY,
		Engine:    e,
		Listen:    &forwardv1.Listen{Port: 30001 + idx, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Ingress:   &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		Upstreams: []*forwardv1.Upstream{{Address: "192.0.2.20", Port: 443, Weight: 1}},
		Balance:   forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN,
	}
}

func TestArtifact(t *testing.T) {
	s := &forwardv1.NodeForwardState{NodeRef: "forward-1", Generation: 4, StateHash: "ab"}
	a := driver.NewArtifact(nft, s, []driver.HopKey{{RouteID: "b"}, {RouteID: "a", HopIndex: 2}, {RouteID: "a", HopIndex: 1}}, []byte("x"))
	want := []driver.HopKey{{RouteID: "a", HopIndex: 1}, {RouteID: "a", HopIndex: 2}, {RouteID: "b"}}
	if !slices.Equal(a.Hops, want) || a.NodeRef != "forward-1" || a.Generation != 4 || a.StateHash != "ab" {
		t.Fatalf("artifact %+v", a)
	}
	if a.Digest != driver.Digest([]byte("x")) || len(a.Digest) != 64 || a.Empty() {
		t.Fatalf("digest %s", a.Digest)
	}
	if err := a.Verify(nft); err != nil {
		t.Fatal(err)
	}
	if err := a.Verify(gost); !errors.Is(err, driver.ErrEngineMismatch) {
		t.Fatalf("engine mismatch: %v", err)
	}
	b := a
	b.Content = []byte("y")
	if err := b.Verify(nft); !errors.Is(err, driver.ErrInvalidArtifact) {
		t.Fatalf("tampered: %v", err)
	}
	c := a
	c.Hops = []driver.HopKey{{RouteID: "b"}, {RouteID: "a"}}
	if err := c.Verify(nft); !errors.Is(err, driver.ErrInvalidArtifact) {
		t.Fatalf("unsorted: %v", err)
	}
	c.Hops = []driver.HopKey{{RouteID: "a"}, {RouteID: "a"}}
	if err := c.Verify(nft); !errors.Is(err, driver.ErrInvalidArtifact) {
		t.Fatalf("duplicate: %v", err)
	}
	if e := driver.NewArtifact(nft, nil, nil, nil); !e.Empty() || e.Verify(nft) != nil {
		t.Fatal("empty artifact")
	}
	if got := (driver.HopKey{RouteID: "r", HopIndex: 3}).String(); got != "r/3" {
		t.Fatal(got)
	}
}

func TestEngineHops(t *testing.T) {
	s := &forwardv1.NodeForwardState{Hops: []*forwardv1.NodeHop{
		hop("c", 0, nft), hop("a", 1, nft), hop("b", 0, gost), hop("a", 0, nft), hop("d", 0, nft), hop("d", 0, nft), hop("d", 0, nft),
	}}
	hops, errs := driver.EngineHops(s, nft)
	var keys []driver.HopKey
	for _, h := range hops {
		keys = append(keys, driver.KeyOf(h))
	}
	if want := []driver.HopKey{{RouteID: "a"}, {RouteID: "a", HopIndex: 1}, {RouteID: "c"}}; !slices.Equal(keys, want) {
		t.Fatalf("hops %v", keys)
	}
	if len(errs) != 1 || errs[0].Key != (driver.HopKey{RouteID: "d"}) || !errors.Is(errs[0], driver.ErrInvalidState) {
		t.Fatalf("errs %v", errs)
	}
}

func TestRenderError(t *testing.T) {
	if driver.NewRenderError(nil) != nil {
		t.Fatal("no hop errors must be a nil error")
	}
	err := driver.NewRenderError([]*driver.HopError{
		{Key: driver.HopKey{RouteID: "a"}, Engine: nft, Err: driver.ErrUnsupported},
		{Key: driver.HopKey{RouteID: "b", HopIndex: 1}, Engine: nft, Err: driver.ErrInvalidState},
	})
	if !errors.Is(err, driver.ErrUnsupported) || !errors.Is(err, driver.ErrInvalidState) || errors.Is(err, driver.ErrConflict) {
		t.Fatalf("errors.Is through RenderError: %v", err)
	}
	var he *driver.HopError
	if !errors.As(err, &he) || he.Key.RouteID != "a" {
		t.Fatal("errors.As *HopError")
	}
	var re *driver.RenderError
	if !errors.As(err, &re) {
		t.Fatal("errors.As *RenderError")
	}
	p := re.ToProto()
	if len(p) != 2 || p[1].GetRouteId() != "b" || p[1].GetHopIndex() != 1 || p[1].GetEngine() != nft || p[1].GetMessage() == "" {
		t.Fatalf("ToProto %v", p)
	}
}

func TestRegistry(t *testing.T) {
	r := driver.NewRegistry()
	host := fake.NewHost(nil)
	n := fake.New(host, fake.Options{})
	g := fake.New(fake.NewHost(nil), fake.Options{Engine: gost})
	if err := r.Register(g); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(n); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(fake.New(host, fake.Options{})); !errors.Is(err, driver.ErrDuplicateEngine) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := r.Register(nil); !errors.Is(err, driver.ErrInvalidArgument) {
		t.Fatalf("nil: %v", err)
	}
	if got := r.Engines(); !slices.Equal(got, []forwardv1.Engine{nft, gost}) {
		t.Fatalf("engines %v", got)
	}
	if d, ok := r.Get(nft); !ok || d != n {
		t.Fatal("Get")
	}
	if _, ok := r.Get(forwardv1.Engine_ENGINE_ANIXOPS); ok {
		t.Fatal("Get unregistered")
	}

	tls := hop("g", 0, gost)
	tls.Ingress.Security = forwardv1.LinkSecurity_LINK_SECURITY_TLS // the default fake caps have RAW only
	s := &forwardv1.NodeForwardState{NodeRef: "forward-1", Generation: 2, Hops: []*forwardv1.NodeHop{
		hop("a", 0, nft), hop("b", 0, gost), hop("x", 0, forwardv1.Engine_ENGINE_ANIXOPS), tls,
	}}
	arts, hopErrs, err := r.Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 || !slices.Equal(arts[nft].Hops, []driver.HopKey{{RouteID: "a"}}) || !slices.Equal(arts[gost].Hops, []driver.HopKey{{RouteID: "b"}}) {
		t.Fatalf("artifacts %v", arts)
	}
	if len(hopErrs) != 2 || hopErrs[0].Key.RouteID != "g" || hopErrs[1].Key.RouteID != "x" ||
		!errors.Is(hopErrs[0], driver.ErrUnsupported) || !errors.Is(hopErrs[1], driver.ErrUnsupported) {
		t.Fatalf("hop errors %v", hopErrs)
	}
	if _, _, err := r.Render(nil); !errors.Is(err, driver.ErrInvalidState) {
		t.Fatalf("nil state: %v", err)
	}
	// An engine with no hops renders an empty artifact, which removes.
	arts, _, _ = r.Render(&forwardv1.NodeForwardState{Generation: 3, Hops: []*forwardv1.NodeHop{hop("a", 0, nft)}})
	if !arts[gost].Empty() || arts[gost].Generation != 3 {
		t.Fatalf("gost artifact %+v", arts[gost])
	}
}

func TestObservationToReport(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_123).UTC()
	o := driver.Observation{
		Engine: nft, Applied: true, NodeRef: "forward-1", Generation: 9, StateHash: "h", Digest: "d",
		Counters:   []*forwardv1.Counters{{RouteId: "a", UpBytes: 5}},
		Health:     []*forwardv1.UpstreamHealth{{RouteId: "a", State: forwardv1.HealthState_HEALTH_STATE_HEALTHY}},
		ObservedAt: at,
	}
	r := o.ToReport()
	if r.GetNodeRef() != "forward-1" || r.GetGeneration() != 9 || r.GetStateHash() != "h" || !r.GetApplied() ||
		len(r.GetCounters()) != 1 || len(r.GetHealth()) != 1 || r.GetObservedAtUnixMs() != 1_700_000_000_123 {
		t.Fatalf("report %v", r)
	}
	if (driver.Observation{}).ToReport().GetObservedAtUnixMs() != 0 {
		t.Fatal("zero time must be 0")
	}
}
