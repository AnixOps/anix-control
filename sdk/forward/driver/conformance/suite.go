package conformance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Scenario is one named behaviour every driver must show.
type Scenario struct {
	Name string
	Doc  string
	Run  func(t *testing.T, h *H)
}

// Option configures Run.
type Option func(*config)

type config struct {
	top     Topology
	skip    map[string]string
	timeout time.Duration
}

// WithTopology sets the addresses and ports of the generated states.
func WithTopology(top Topology) Option { return func(c *config) { c.top = top } }

// Skip skips a scenario by name, with a reason (a known gap of a driver
// under construction). Use sparingly: a released driver passes everything.
func Skip(name, reason string) Option {
	return func(c *config) { c.skip[name] = reason }
}

// WithTimeout bounds every driver call (default 30 s).
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// Run runs every scenario as a subtest, each on a fresh Env from factory.
// Before a scenario it plants foreign objects; after it, it checks they are
// untouched. Every observation in every scenario is checked for being well
// formed and for counter monotonicity within an epoch.
func Run(t *testing.T, factory Factory, opts ...Option) {
	t.Helper()
	cfg := config{top: DefaultTopology(), skip: map[string]string{}, timeout: 30 * time.Second}
	for _, o := range opts {
		o(&cfg)
	}
	if err := cfg.top.check(); err != nil {
		t.Fatal(err)
	}
	for _, sc := range Scenarios() {
		t.Run(sc.Name, func(t *testing.T) {
			if reason, ok := cfg.skip[sc.Name]; ok {
				t.Skip(reason)
			}
			h := newH(t, factory(t), cfg)
			if sc.Name != "capabilities" {
				h.RequireAvailable()
			}
			sc.Run(t, h)
		})
	}
}

// H is a scenario's handle on the driver under test and its Env.
type H struct {
	T    *testing.T
	Env  Env
	D    driver.Driver
	Caps *forwardv1.EngineCapabilities
	B    Builder

	cfg config
	*shared
}

// shared is the state of one scenario that its subtests' handles share.
type shared struct {
	mu       sync.Mutex
	counters map[driver.HopKey]*forwardv1.Counters
	foreign  []string
}

// Sub answers a handle for a subtest of the scenario: the same Env, driver
// and counter history, failing t.
func (h *H) Sub(t *testing.T) *H {
	c := *h
	c.T = t
	return &c
}

func newH(t *testing.T, env Env, cfg config) *H {
	t.Helper()
	h := &H{T: t, Env: env, cfg: cfg, shared: &shared{counters: map[driver.HopKey]*forwardv1.Counters{}}}
	h.D = env.NewDriver(t)
	if h.D == nil {
		t.Fatal("Env.NewDriver answered nil")
	}
	ctx, cancel := h.Ctx()
	defer cancel()
	caps, err := h.D.Capabilities(ctx)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	h.Caps = caps
	h.B = Builder{Engine: h.D.Engine(), Caps: caps, Top: cfg.top}
	env.PlantForeign(t)
	h.foreign = env.Foreign(t)
	if len(h.foreign) == 0 {
		t.Fatal("Env.PlantForeign planted nothing Foreign lists")
	}
	t.Cleanup(func() {
		h.mu.Lock()
		want := h.foreign
		h.mu.Unlock()
		if got := env.Foreign(t); !slices.Equal(got, want) {
			t.Errorf("foreign objects changed:\n got  %q\n want %q", got, want)
		}
	})
	return h
}

// Ctx answers a context bounded by the suite's timeout.
func (h *H) Ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), h.cfg.timeout)
}

// RequireAvailable skips the scenario when the driver is unavailable on
// this host.
func (h *H) RequireAvailable() {
	h.T.Helper()
	if !h.Caps.GetAvailable() {
		h.T.Skipf("driver unavailable: %s", h.Caps.GetUnavailableReason())
	}
}

// Restart replaces the driver with a new instance on the same host.
func (h *H) Restart() {
	h.T.Helper()
	h.D = h.Env.NewDriver(h.T)
}

// State builds a node state for the scenario's node.
func (h *H) State(generation uint64, hops ...*forwardv1.NodeHop) *forwardv1.NodeForwardState {
	return State(h.cfg.top.NodeRef, generation, hops...)
}

// Render renders a state that must render without error.
func (h *H) Render(state *forwardv1.NodeForwardState) driver.Artifact {
	h.T.Helper()
	a, err := h.D.Render(state)
	if err != nil {
		h.T.Fatalf("Render: %v", err)
	}
	if a.Engine != h.D.Engine() {
		h.T.Fatalf("Render: artifact engine %v, driver %v", a.Engine, h.D.Engine())
	}
	if err := a.Verify(h.D.Engine()); err != nil {
		h.T.Fatalf("Render: artifact does not verify: %v", err)
	}
	if a.NodeRef != state.GetNodeRef() || a.Generation != state.GetGeneration() || a.StateHash != state.GetStateHash() {
		h.T.Fatalf("Render: artifact identity %s/%d/%s, state %s/%d/%s", a.NodeRef, a.Generation, a.StateHash, state.GetNodeRef(), state.GetGeneration(), state.GetStateHash())
	}
	return a
}

// Apply applies an artifact that must apply.
func (h *H) Apply(a driver.Artifact) driver.ApplyResult {
	h.T.Helper()
	ctx, cancel := h.Ctx()
	defer cancel()
	r, err := h.D.Apply(ctx, a)
	if err != nil {
		h.T.Fatalf("Apply generation %d: %v", a.Generation, err)
	}
	if r.Generation != a.Generation || r.Digest != a.Digest || r.StateHash != a.StateHash {
		h.T.Fatalf("Apply result %+v does not name the artifact (generation %d digest %s)", r, a.Generation, a.Digest)
	}
	return r
}

// RenderApply renders and applies a state.
func (h *H) RenderApply(state *forwardv1.NodeForwardState) (driver.Artifact, driver.ApplyResult) {
	h.T.Helper()
	a := h.Render(state)
	return a, h.Apply(a)
}

// Observe observes and checks the observation (see CheckObservation).
func (h *H) Observe() driver.Observation {
	h.T.Helper()
	ctx, cancel := h.Ctx()
	defer cancel()
	o, err := h.D.Observe(ctx)
	if err != nil {
		h.T.Fatalf("Observe: %v", err)
	}
	if err := h.CheckObservation(o); err != nil {
		h.T.Fatal(err)
	}
	return o
}

// CheckObservation checks that o is well formed and that no counter went
// down within its epoch since any earlier observation in this scenario,
// across driver restarts. It is safe for concurrent use.
func (h *H) CheckObservation(o driver.Observation) error {
	if o.Engine != h.D.Engine() {
		return fmt.Errorf("observation engine %v, driver %v", o.Engine, h.D.Engine())
	}
	if !o.Applied {
		if o.Generation != 0 || o.Digest != "" || len(o.Counters) > 0 || len(o.Rotation) > 0 || len(o.Health) > 0 {
			return fmt.Errorf("observation not applied but carries state: %+v", o)
		}
		return nil
	}
	if o.Digest == "" {
		return fmt.Errorf("applied observation without digest")
	}
	keys := map[driver.HopKey]bool{}
	for _, c := range o.Counters {
		k := driver.HopKey{RouteID: c.GetRouteId(), HopIndex: c.GetHopIndex()}
		if keys[k] {
			return fmt.Errorf("counters for %s twice", k)
		}
		keys[k] = true
		if c.GetNodeRef() != o.NodeRef {
			return fmt.Errorf("counters %s node_ref %q, observation %q", k, c.GetNodeRef(), o.NodeRef)
		}
		if c.GetCounterEpoch() == "" {
			return fmt.Errorf("counters %s without counter_epoch", k)
		}
	}
	rot := map[driver.HopKey]bool{}
	for _, r := range o.Rotation {
		k := driver.HopKey{RouteID: r.RouteID, HopIndex: r.HopIndex}
		if !keys[k] || rot[k] {
			return fmt.Errorf("rotation for %s without counters, or twice", k)
		}
		rot[k] = true
		if len(r.Active) == 0 {
			return fmt.Errorf("rotation for %s is empty", k)
		}
	}
	if len(rot) != len(keys) {
		return fmt.Errorf("rotation for %d hops, counters for %d", len(rot), len(keys))
	}
	for _, hl := range o.Health {
		k := driver.HopKey{RouteID: hl.GetRouteId(), HopIndex: hl.GetHopIndex()}
		if !keys[k] {
			return fmt.Errorf("health for unknown hop %s", k)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range o.Counters {
		k := driver.HopKey{RouteID: c.GetRouteId(), HopIndex: c.GetHopIndex()}
		if prev, ok := h.counters[k]; ok && prev.GetCounterEpoch() == c.GetCounterEpoch() {
			if err := monotonic(prev, c); err != nil {
				return fmt.Errorf("counters %s epoch %s: %w", k, c.GetCounterEpoch(), err)
			}
		}
		h.counters[k] = proto.Clone(c).(*forwardv1.Counters)
	}
	return nil
}

func monotonic(prev, cur *forwardv1.Counters) error {
	type f struct {
		name     string
		old, new uint64
	}
	for _, x := range []f{
		{"up_bytes", prev.GetUpBytes(), cur.GetUpBytes()},
		{"down_bytes", prev.GetDownBytes(), cur.GetDownBytes()},
		{"up_packets", prev.GetUpPackets(), cur.GetUpPackets()},
		{"down_packets", prev.GetDownPackets(), cur.GetDownPackets()},
		{"total_conns", prev.GetTotalConns(), cur.GetTotalConns()},
	} {
		if x.new < x.old {
			return fmt.Errorf("%s went down from %d to %d", x.name, x.old, x.new)
		}
	}
	return nil
}

// Owned answers the Env's owned objects.
func (h *H) Owned() []string {
	h.T.Helper()
	return h.Env.Owned(h.T)
}

// Refreeze re-reads the foreign baseline after the scenario planted more
// foreign objects on purpose.
func (h *H) Refreeze() {
	h.T.Helper()
	f := h.Env.Foreign(h.T)
	h.mu.Lock()
	h.foreign = f
	h.mu.Unlock()
}

// Traffic makes traffic flow through a hop when the Env can; it answers
// false when it cannot.
func (h *H) Traffic(k driver.HopKey) bool {
	h.T.Helper()
	ts, ok := h.Env.(TrafficSource)
	if ok {
		ts.Traffic(h.T, k)
	}
	return ok
}

// Applies answers the Env's apply count, or -1 without an ApplyCounter.
func (h *H) Applies() int {
	h.T.Helper()
	if ac, ok := h.Env.(ApplyCounter); ok {
		return ac.Applies(h.T)
	}
	return -1
}

// WantErr fails unless err wraps want.
func (h *H) WantErr(what string, err, want error) {
	h.T.Helper()
	if !errors.Is(err, want) {
		h.T.Fatalf("%s: error %v, want %v", what, err, want)
	}
}

// epochs answers each hop's counter epoch.
func epochs(o driver.Observation) map[driver.HopKey]string {
	m := map[driver.HopKey]string{}
	for _, c := range o.Counters {
		m[driver.HopKey{RouteID: c.GetRouteId(), HopIndex: c.GetHopIndex()}] = c.GetCounterEpoch()
	}
	return m
}

// rotation answers a hop's rotation as sorted "addr:port*weight" strings.
func rotation(o driver.Observation, k driver.HopKey) []string {
	for _, r := range o.Rotation {
		if r.RouteID == k.RouteID && r.HopIndex == k.HopIndex {
			return fmtUpstreams(r.Active)
		}
	}
	return nil
}

func fmtUpstreams(us []driver.Upstream) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = fmt.Sprintf("%s:%d*%d", u.Address, u.Port, u.Weight)
	}
	slices.Sort(out)
	return out
}

// renderedRotation answers what a freshly applied hop has in rotation.
func renderedRotation(hop *forwardv1.NodeHop) []string {
	us := make([]driver.Upstream, 0, len(hop.GetUpstreams()))
	for _, u := range hop.GetUpstreams() {
		us = append(us, driver.Upstream{Address: u.GetAddress(), Port: u.GetPort(), Weight: max(u.GetWeight(), 1)})
	}
	return fmtUpstreams(us)
}

func keyOf(h *forwardv1.NodeHop) driver.HopKey { return driver.KeyOf(h) }

func keysOf(o driver.Observation) []driver.HopKey {
	out := make([]driver.HopKey, 0, len(o.Counters))
	for _, c := range o.Counters {
		out = append(out, driver.HopKey{RouteID: c.GetRouteId(), HopIndex: c.GetHopIndex()})
	}
	slices.SortFunc(out, driver.HopKey.Compare)
	return out
}

func diff(a, b []string) string {
	return fmt.Sprintf("\n before %s\n after  %s", strings.Join(a, "\n        "), strings.Join(b, "\n        "))
}
