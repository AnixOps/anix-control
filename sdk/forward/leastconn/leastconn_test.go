package leastconn_test

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/fake"
	"github.com/AnixOps/anix-control/sdk/forward/leastconn"
)

func ap(s string) netip.AddrPort { return netip.MustParseAddrPort(s) }

func weightsOf(us []driver.Upstream) map[string]uint32 {
	m := map[string]uint32{}
	for _, u := range us {
		m[u.Address] = u.Weight
	}
	return m
}

func TestWeights(t *testing.T) {
	ups := []driver.Upstream{
		{Address: "192.0.2.20", Port: 443, Weight: 1},
		{Address: "192.0.2.21", Port: 443, Weight: 2},
		{Address: "192.0.2.22", Port: 443},
	}
	for _, c := range []struct {
		name  string
		conns map[netip.AddrPort]uint64
		want  map[string]uint32
	}{
		{"idle keeps the rendered weights", nil, map[string]uint32{"192.0.2.20": 1, "192.0.2.21": 2, "192.0.2.22": 1}},
		{"connections elsewhere only", map[netip.AddrPort]uint64{ap("192.0.2.99:443"): 7, ap("192.0.2.20:80"): 3}, map[string]uint32{"192.0.2.20": 1, "192.0.2.21": 2, "192.0.2.22": 1}},
		// 1/(9+1)=0.1, 2/(1+1)=1, 1/(0+1)=1: the two least loaded share the top.
		{"inverse of the load, by weight", map[netip.AddrPort]uint64{ap("192.0.2.20:443"): 9, ap("192.0.2.21:443"): 1}, map[string]uint32{"192.0.2.20": 10, "192.0.2.21": 100, "192.0.2.22": 100}},
		// 1/1001 of the top rounds to 0 and is kept at 1.
		{"never below 1", map[netip.AddrPort]uint64{ap("192.0.2.20:443"): 1000}, map[string]uint32{"192.0.2.20": 1, "192.0.2.21": 100, "192.0.2.22": 50}},
		// 1/4, 2/1, 1/1 of 2: 12.5 rounds to 13.
		{"IPv4-mapped addresses count", map[netip.AddrPort]uint64{netip.AddrPortFrom(netip.MustParseAddr("::ffff:192.0.2.20"), 443): 3}, map[string]uint32{"192.0.2.20": 13, "192.0.2.21": 100, "192.0.2.22": 50}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := leastconn.Weights(ups, c.conns)
			if w := weightsOf(got); len(w) != 3 || w["192.0.2.20"] != c.want["192.0.2.20"] || w["192.0.2.21"] != c.want["192.0.2.21"] || w["192.0.2.22"] != c.want["192.0.2.22"] {
				t.Fatalf("weights %v, want %v", w, c.want)
			}
			if ups[2].Weight != 0 {
				t.Fatal("Weights changed its argument")
			}
		})
	}
}

// countingSetter counts the SetUpstreams calls it passes on.
type countingSetter struct {
	leastconn.Setter
	mu    sync.Mutex
	calls int
}

func (c *countingSetter) SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.Setter.SetUpstreams(ctx, routeID, hopIndex, active)
}

func (c *countingSetter) n() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// lab is a fake driver running one LEAST_CONN hop with three upstreams.
type lab struct {
	host *fake.Host
	d    *fake.Driver
	spec *forwardv1.NodeHop
	set  *countingSetter
	rw   leastconn.Reweighter
}

func newLab(t *testing.T) *lab {
	t.Helper()
	host := fake.NewHost(nil)
	d := fake.New(host, fake.Options{})
	b := conformance.Builder{Engine: d.Engine(), Caps: fake.NFTablesCapabilities(), Top: conformance.DefaultTopology()}
	hop := b.Entry(conformance.RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_TCP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN, b.Upstreams(b.Top.UpstreamsV4, 3))
	a, err := d.Render(conformance.State("forward-11", 1, hop))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Apply(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	set := &countingSetter{Setter: d}
	return &lab{host: host, d: d, spec: hop, set: set, rw: leastconn.Reweighter{Source: d, Setter: set}}
}

// hop answers the hop as the health loop hands it over: the upstreams it
// keeps (all, or those named), with rendered weights, and the rotation the
// driver reports.
func (l *lab) hop(t *testing.T, keep ...string) leastconn.Hop {
	t.Helper()
	h := leastconn.Hop{RouteID: l.spec.GetRouteId(), HopIndex: l.spec.GetHopIndex()}
	for _, u := range l.spec.GetUpstreams() {
		if len(keep) == 0 || slices.Contains(keep, u.GetAddress()) {
			h.Upstreams = append(h.Upstreams, driver.Upstream{Address: u.GetAddress(), Port: u.GetPort(), Weight: u.GetWeight()})
		}
	}
	h.Rotation = l.rotation(t)
	return h
}

func (l *lab) rotation(t *testing.T) []driver.Upstream {
	t.Helper()
	o, err := l.d.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return o.Rotation[0].Active
}

// TestReweighterWithFakeDriver: the re-weighting follows the connections,
// is not set again while it holds, leaves an idle hop on its rendered
// weights, and weights only the upstreams the health loop keeps.
func TestReweighterWithFakeDriver(t *testing.T) {
	l := newLab(t)
	ctx := t.Context()

	if err := l.rw.Tick(ctx, []leastconn.Hop{l.hop(t)}); err != nil || l.set.n() != 0 {
		t.Fatalf("an idle hop was re-weighted: %v (%d calls)", err, l.set.n())
	}

	l.host.SetUpstreamConns(map[netip.AddrPort]uint64{ap("192.0.2.20:443"): 9, ap("192.0.2.21:443"): 1, ap("192.0.2.22:443"): 2})
	if err := l.rw.Tick(ctx, []leastconn.Hop{l.hop(t)}); err != nil {
		t.Fatal(err)
	}
	// Rendered weights 1, 2, 3: 1/10, 2/2, 3/3.
	if w := weightsOf(l.rotation(t)); w["192.0.2.20"] != 10 || w["192.0.2.21"] != 100 || w["192.0.2.22"] != 100 || l.set.n() != 1 {
		t.Fatalf("weights %v after %d calls", w, l.set.n())
	}
	if err := l.rw.Tick(ctx, []leastconn.Hop{l.hop(t)}); err != nil || l.set.n() != 1 {
		t.Fatalf("an unchanged weighting was set again: %v (%d calls)", err, l.set.n())
	}

	// The health loop took 192.0.2.21 out: it stays out.
	l.host.SetUpstreamConns(map[netip.AddrPort]uint64{ap("192.0.2.20:443"): 1})
	if err := l.rw.Tick(ctx, []leastconn.Hop{l.hop(t, "192.0.2.20", "192.0.2.22")}); err != nil {
		t.Fatal(err)
	}
	if w := weightsOf(l.rotation(t)); len(w) != 2 || w["192.0.2.20"] != 17 || w["192.0.2.22"] != 100 {
		t.Fatalf("weights %v", w)
	}

	// Load gone: back to the rendered weights.
	l.host.SetUpstreamConns(nil)
	if err := l.rw.Tick(ctx, []leastconn.Hop{l.hop(t)}); err != nil {
		t.Fatal(err)
	}
	if w := weightsOf(l.rotation(t)); w["192.0.2.20"] != 1 || w["192.0.2.21"] != 2 || w["192.0.2.22"] != 3 {
		t.Fatalf("idle weights %v", w)
	}
}

// TestReweighterErrors: a failing hop does not stop the others, a source
// that fails sets nothing, and a done context changes nothing.
func TestReweighterErrors(t *testing.T) {
	l := newLab(t)
	ctx := t.Context()
	l.host.SetUpstreamConns(map[netip.AddrPort]uint64{ap("192.0.2.20:443"): 4})
	unknown := leastconn.Hop{RouteID: conformance.RouteB, Upstreams: []driver.Upstream{{Address: "192.0.2.20", Port: 443}}}
	err := l.rw.Tick(ctx, []leastconn.Hop{unknown, l.hop(t)})
	if !errors.Is(err, driver.ErrNotFound) {
		t.Fatalf("Tick with an unknown hop: %v", err)
	}
	if w := weightsOf(l.rotation(t)); w["192.0.2.20"] == 1 {
		t.Fatal("the known hop was not re-weighted")
	}

	boom := errors.New("boom")
	failing := leastconn.Reweighter{Source: sourceFunc(func(context.Context) (map[netip.AddrPort]uint64, error) { return nil, boom }), Setter: l.set}
	calls := l.set.n()
	if err := failing.Tick(ctx, []leastconn.Hop{l.hop(t)}); !errors.Is(err, boom) || l.set.n() != calls {
		t.Fatalf("Tick with a failing source: %v", err)
	}
	done, cancel := context.WithCancel(ctx)
	cancel()
	if err := l.rw.Tick(done, []leastconn.Hop{l.hop(t)}); !errors.Is(err, context.Canceled) || l.set.n() != calls {
		t.Fatalf("Tick with a done context: %v", err)
	}
}

type sourceFunc func(context.Context) (map[netip.AddrPort]uint64, error)

func (f sourceFunc) ActiveConns(ctx context.Context) (map[netip.AddrPort]uint64, error) {
	return f(ctx)
}

// TestReweighterRun: Run ticks every interval until its context ends and
// reports errors without stopping.
func TestReweighterRun(t *testing.T) {
	l := newLab(t)
	l.host.SetUpstreamConns(map[netip.AddrPort]uint64{ap("192.0.2.22:443"): 5})
	var mu sync.Mutex
	var errs []error
	rw := l.rw
	rw.OnError = func(err error) { mu.Lock(); errs = append(errs, err); mu.Unlock() }
	ctx, cancel := context.WithCancel(t.Context())
	ticks := make(chan struct{}, 16)
	go func() {
		for range 3 {
			<-ticks
		}
		cancel()
	}()
	err := rw.Run(ctx, 10*time.Millisecond, func(context.Context) []leastconn.Hop {
		ticks <- struct{}{}
		return []leastconn.Hop{l.hop(t), {RouteID: conformance.RouteC, Upstreams: []driver.Upstream{{Address: "192.0.2.20", Port: 443}}}}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run answered %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(errs) < 2 || l.set.n() < 1 {
		t.Fatalf("Run: %d errors reported, %d weightings set", len(errs), l.set.n())
	}
	// Rendered weights 1, 2, 3: 1/1, 2/1, 3/6.
	if w := weightsOf(l.rotation(t)); w["192.0.2.20"] != 50 || w["192.0.2.21"] != 100 || w["192.0.2.22"] != 25 {
		t.Fatalf("weights %v", w)
	}
}
