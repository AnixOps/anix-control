package gost_test

import (
	"encoding/json"
	"errors"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

func observe(t testing.TB, d *gost.Driver) driver.Observation {
	t.Helper()
	o, err := d.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func countersOf(t testing.TB, o driver.Observation, k driver.HopKey) *forwardv1.Counters {
	t.Helper()
	for _, c := range o.Counters {
		if c.GetRouteId() == k.RouteID && c.GetHopIndex() == k.HopIndex {
			return c
		}
	}
	t.Fatalf("no counters for %s", k)
	return nil
}

func apply(t testing.TB, d *gost.Driver, gen uint64, hops ...*forwardv1.NodeHop) driver.ApplyResult {
	t.Helper()
	r, err := d.Apply(t.Context(), render(t, d, conformance.State("forward-11", gen, hops...)))
	if err != nil {
		t.Fatalf("Apply generation %d: %v", gen, err)
	}
	return r
}

// TestObserveCounters: the counters are the hop's services' statistics;
// a hot apply keeps their epoch and values, an apply that creates or
// re-creates a hop's services through the web API starts a new epoch for
// that hop only, and a stopped gost reports 0 in the epoch "stopped".
func TestObserveCounters(t *testing.T) {
	d, f, _ := fakeDriver(t)
	b := builder(t)
	a, bb := b.Simple(conformance.RouteA, 0), b.Simple(conformance.RouteB, 1)
	ka, kb := driver.KeyOf(a), driver.KeyOf(bb)
	apply(t, d, 1, a, bb)
	f.traffic("r"+ka.RouteID+"-h0", 100, 250)
	f.traffic("r"+ka.RouteID+"-h0", 1, 2)
	c := countersOf(t, observe(t, d), ka)
	if c.GetUpBytes() != 101 || c.GetDownBytes() != 252 || c.GetTotalConns() != 2 || c.GetUpPackets() != 0 {
		t.Fatalf("counters %v", c)
	}
	epoch := c.GetCounterEpoch()
	if other := countersOf(t, observe(t, d), kb); other.GetUpBytes() != 0 || other.GetCounterEpoch() == epoch {
		t.Fatalf("route B counters %v share route A's", other)
	}

	// Upstreams, weights, a pause and limits are hot: no reload, the
	// epoch and the values stay.
	applies := f.applies
	a2 := proto.Clone(a).(*forwardv1.NodeHop)
	a2.Upstreams = a2.Upstreams[:2]
	a2.Upstreams[1].Weight = 5
	b2 := proto.Clone(bb).(*forwardv1.NodeHop)
	b2.Paused = true
	if r := apply(t, d, 2, a2, b2); !r.Changed {
		t.Fatal("changing apply reported no change")
	}
	if f.applies != applies {
		t.Fatal("a hot change reloaded gost")
	}
	if c2 := countersOf(t, observe(t, d), ka); c2.GetCounterEpoch() != epoch || c2.GetUpBytes() != 101 {
		t.Fatalf("hot apply: counters %v, epoch was %s", c2, epoch)
	}
	var adm struct {
		Whitelist bool
		Matchers  []string
	}
	if !f.object("admissions", "r"+kb.RouteID+"-h0", &adm) || !adm.Whitelist || len(adm.Matchers) != 0 {
		t.Fatalf("paused hop's admission %+v", adm)
	}

	// A new hop changes the structure: its services are created through
	// the web API, without a reload, and every other hop keeps its epoch
	// and values. Twice in a row: gost then serves a metrics path the
	// configuration file no longer names.
	kc := driver.KeyOf(b.Simple(conformance.RouteC, 2))
	apply(t, d, 3, a2, b2, b.Simple(conformance.RouteC, 2))
	o3 := observe(t, d)
	if c3 := countersOf(t, o3, ka); c3.GetCounterEpoch() != epoch || c3.GetUpBytes() != 101 {
		t.Fatalf("after route C was added: counters %v, epoch was %s", c3, epoch)
	}
	epochB := countersOf(t, o3, kb).GetCounterEpoch()
	f.traffic("r"+kc.RouteID+"-h0", 5, 5)
	moved := proto.Clone(b2).(*forwardv1.NodeHop)
	moved.Listen.Port = 30004
	apply(t, d, 4, a2, moved, b.Simple(conformance.RouteC, 2))
	if f.applies != applies {
		t.Fatal("a structural change reloaded gost")
	}
	o4 := observe(t, d)
	if c4 := countersOf(t, o4, ka); c4.GetCounterEpoch() != epoch || c4.GetUpBytes() != 101 {
		t.Fatalf("after route B moved: counters %v, epoch was %s", c4, epoch)
	}
	if c4 := countersOf(t, o4, kc); c4.GetUpBytes() != 5 {
		t.Fatalf("route C after route B moved: %v", c4)
	}
	if countersOf(t, o4, kb).GetCounterEpoch() == epochB {
		t.Fatal("route B's re-created service kept its counter epoch")
	}

	if err := f.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	o := observe(t, d)
	if c4 := countersOf(t, o, ka); c4.GetCounterEpoch() != "stopped" || c4.GetUpBytes() != 0 {
		t.Fatalf("stopped gost: counters %v", c4)
	}
	if len(o.Rotation) != 3 || len(o.Rotation[0].Active) != 2 {
		t.Fatalf("rotation %+v", o.Rotation)
	}
}

// TestSetUpstreamsReplacesTheHop: SetUpstreams replaces the running hop
// through the API, with the nodes Render would give the selection and the
// selection in the hop's metadata; selecting every upstream with its
// rendered weight runs exactly the rendered hop again.
func TestSetUpstreamsReplacesTheHop(t *testing.T) {
	for _, s := range []forwardv1.BalanceStrategy{failover, rr, random, ipHash, leastC} {
		t.Run(s.String(), func(t *testing.T) {
			d, f, _ := fakeDriver(t)
			b := builder(t)
			hop := b.Entry(conformance.RouteA, 0, tcp, s, b.Upstreams(b.Top.UpstreamsV4, 3))
			k := driver.KeyOf(hop)
			apply(t, d, 1, hop)
			name := "r" + k.RouteID + "-h0"
			var rendered, got map[string]any
			if !f.object("hops", name, &rendered) {
				t.Fatal("no running hop")
			}
			applies, puts := f.applies, f.puts
			u := hop.GetUpstreams()
			sel := []driver.Upstream{{Address: u[2].GetAddress(), Port: u[2].GetPort(), Weight: 4}, {Address: u[0].GetAddress(), Port: u[0].GetPort()}}
			if err := d.SetUpstreams(t.Context(), k.RouteID, 0, sel); err != nil {
				t.Fatal(err)
			}
			if f.applies != applies || f.puts != puts+1 {
				t.Fatalf("SetUpstreams: %d reloads, %d API changes", f.applies-applies, f.puts-puts)
			}
			f.object("hops", name, &got)
			nodes := got["nodes"].([]any)
			addrs := map[string]bool{}
			for _, n := range nodes {
				addrs[n.(map[string]any)["addr"].(string)] = true
			}
			if len(addrs) != 2 || !addrs[u[0].GetAddress()+":443"] || !addrs[u[2].GetAddress()+":443"] {
				t.Fatalf("nodes %v", nodes)
			}
			if s == random || s == leastC {
				for _, n := range nodes {
					m := n.(map[string]any)
					want := "1"
					if m["addr"] == u[2].GetAddress()+":443" {
						want = "4"
					}
					if w := m["metadata"].(map[string]any)["weight"]; w != want {
						t.Fatalf("node %v weight %v, want %s", m["addr"], w, want)
					}
				}
			}
			rot := observe(t, d).Rotation[0].Active
			if len(rot) != 2 || rot[0].Address != u[0].GetAddress() || rot[0].Weight != 1 || rot[1].Weight != 4 {
				t.Fatalf("rotation %+v", rot)
			}
			all := make([]driver.Upstream, 0, 3)
			for _, x := range u {
				all = append(all, driver.Upstream{Address: x.GetAddress(), Port: x.GetPort()})
			}
			if err := d.SetUpstreams(t.Context(), k.RouteID, 0, all); err != nil {
				t.Fatal(err)
			}
			got = nil
			f.object("hops", name, &got)
			// The API takes the selector's failTimeout in nanoseconds.
			rendered["selector"].(map[string]any)["failTimeout"] = 3e10
			rb, _ := json.Marshal(rendered)
			gb, _ := json.Marshal(got)
			if string(rb) != string(gb) {
				t.Fatalf("restoring every upstream runs\n%s\nnot the rendered hop\n%s", gb, rb)
			}
		})
	}
}

// TestApplyHotFailureRecovers: an API change that fails is rolled back
// through the API: gost keeps running, the previous artifact runs.
func TestApplyHotFailureRecovers(t *testing.T) {
	d, f, _ := fakeDriver(t)
	b := builder(t)
	a := b.Simple(conformance.RouteA, 0)
	a1 := render(t, d, conformance.State("forward-11", 1, a))
	if _, err := d.Apply(t.Context(), a1); err != nil {
		t.Fatal(err)
	}
	inst := f.instance
	a2h := proto.Clone(a).(*forwardv1.NodeHop)
	a2h.Upstreams = a2h.Upstreams[:1]
	a2h.Limits = &forwardv1.Limits{MaxConns: 10}
	f.with(func() { f.failAPI = 2 }) // the climiter is created, the hop refused
	if _, err := d.Apply(t.Context(), render(t, d, conformance.State("forward-11", 2, a2h))); err == nil {
		t.Fatal("Apply succeeded although the API refused a change")
	}
	if f.instance != inst {
		t.Fatal("gost was restarted")
	}
	if r, err := d.Apply(t.Context(), a1); err != nil || r.Changed {
		t.Fatalf("the host does not run the previous artifact: %+v %v", r, err)
	}
}

// TestEnforceQuotas: a hop whose counters reached its quota admits nobody
// until the quota is raised; the soft quota can be turned off.
func TestEnforceQuotas(t *testing.T) {
	d, f, _ := fakeDriver(t)
	b := builder(t)
	a, other := b.Simple(conformance.RouteA, 0), b.Simple(conformance.RouteB, 1)
	a.Limits = &forwardv1.Limits{QuotaBytes: 10_000}
	k := driver.KeyOf(a)
	name := "r" + k.RouteID + "-h0"
	apply(t, d, 1, a, other)
	held, err := d.EnforceQuotas(t.Context())
	if err != nil || len(held) != 0 {
		t.Fatalf("EnforceQuotas under the quota: %v %v", held, err)
	}
	f.traffic(name, 4000, 6000)
	f.traffic("r"+conformance.RouteB+"-h0", 1<<20, 1<<20)
	epoch := countersOf(t, observe(t, d), k).GetCounterEpoch()
	held, err = d.EnforceQuotas(t.Context())
	if err != nil || !slices.Equal(held, []driver.HopKey{k}) {
		t.Fatalf("EnforceQuotas at the quota: %v %v", held, err)
	}
	var adm struct {
		Whitelist bool
		Matchers  []string
	}
	if f.object("admissions", name, &adm); !adm.Whitelist || len(adm.Matchers) != 0 {
		t.Fatalf("admission of a hop at its quota: %+v", adm)
	}
	if c := countersOf(t, observe(t, d), k); c.GetCounterEpoch() != epoch {
		t.Fatal("EnforceQuotas ended the counter epoch")
	}
	if held, _ := d.EnforceQuotas(t.Context()); len(held) != 1 {
		t.Fatalf("a held hop is not reported again: %v", held)
	}

	a2 := proto.Clone(a).(*forwardv1.NodeHop)
	a2.Limits.QuotaBytes = 50_000
	apply(t, d, 2, a2, other)
	if f.object("admissions", name, &adm); !adm.Whitelist {
		t.Fatal("raising the quota alone opened the hop before EnforceQuotas")
	}
	held, err = d.EnforceQuotas(t.Context())
	if err != nil || len(held) != 0 {
		t.Fatalf("EnforceQuotas after the quota was raised: %v %v", held, err)
	}
	if f.object("admissions", name, &adm); adm.Whitelist {
		t.Fatalf("a hop under its raised quota still admits nobody: %+v", adm)
	}

	off := newDriver(t, func(c *gost.Config) { c.SoftQuota = false })
	if _, err := off.Render(conformance.State("forward-11", 1, a)); !errors.Is(err, driver.ErrUnsupported) {
		t.Fatalf("a quota without the soft quota: %v", err)
	}
	if caps, _ := off.Capabilities(t.Context()); caps.GetQuota() {
		t.Fatal("quota capability without the soft quota")
	}
}

// TestActiveConns: established sockets to the applied hops' upstreams,
// per address and port.
func TestActiveConns(t *testing.T) {
	d, f, _ := fakeDriver(t)
	b := builder(t)
	apply(t, d, 1, b.Simple(conformance.RouteA, 0))
	f.mu.Lock()
	f.conns = []string{
		"tcp ESTAB 0 0 10.0.0.1:40000 192.0.2.20:443",
		"tcp ESTAB 0 0 10.0.0.1:40001 192.0.2.20:443",
		"tcp ESTAB 0 0 [::ffff:10.0.0.1]:40002 [::ffff:192.0.2.21]:443",
		"udp ESTAB 0 0 10.0.0.1:40003 192.0.2.21:443",
		"tcp TIME-WAIT 0 0 10.0.0.1:40004 192.0.2.22:443",
		"tcp ESTAB 0 0 10.0.0.1:40005 192.0.2.99:443",
		"tcp ESTAB 0 0 10.0.0.1:40006 192.0.2.22:80",
	}
	f.mu.Unlock()
	got, err := d.ActiveConns(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[netip.AddrPort]uint64{
		netip.MustParseAddrPort("192.0.2.20:443"): 2,
		netip.MustParseAddrPort("192.0.2.21:443"): 2,
	}
	if !maps.Equal(got, want) {
		t.Fatalf("ActiveConns %v, want %v", got, want)
	}
}

// TestRetiredCounters: an apply that deletes or re-creates a hop's
// services hands that hop's last counters to the hook; an apply that
// adds a hop or changes hot objects only retires nothing; a reload
// retires every hop.
func TestRetiredCounters(t *testing.T) {
	f, cfg := newFakeGost(t)
	cfg.ReadyTimeout = 300 * time.Millisecond
	var retired []*forwardv1.Counters
	d, err := gost.New(cfg, gost.WithRunner(f), gost.WithSupervisor(f), gost.WithRetiredCounters(func(c []*forwardv1.Counters) { retired = append(retired, c...) }))
	if err != nil {
		t.Fatal(err)
	}
	b := builder(t)
	a, bb := b.Simple(conformance.RouteA, 0), b.Simple(conformance.RouteB, 1)
	ka, kb := driver.KeyOf(a), driver.KeyOf(bb)
	apply(t, d, 1, a)
	f.traffic("r"+ka.RouteID+"-h0", 70, 90)
	epoch := countersOf(t, observe(t, d), ka).GetCounterEpoch()
	a2 := proto.Clone(a).(*forwardv1.NodeHop)
	a2.Upstreams = a2.Upstreams[:1]
	apply(t, d, 2, a2)
	apply(t, d, 3, a2, bb)
	if len(retired) != 0 {
		t.Fatalf("a hot apply or an added route retired %v", retired)
	}
	f.traffic("r"+kb.RouteID+"-h0", 7, 9)
	epochB := countersOf(t, observe(t, d), kb).GetCounterEpoch()
	apply(t, d, 4, a2)
	if len(retired) != 1 || retired[0].GetRouteId() != kb.RouteID || retired[0].GetUpBytes() != 7 || retired[0].GetCounterEpoch() != epochB {
		t.Fatalf("retired %v, want route B's 7/9 in epoch %s", retired, epochB)
	}
	retired = nil
	a3 := proto.Clone(a2).(*forwardv1.NodeHop)
	a3.Listen.Protocol = forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP // a udp service is added
	apply(t, d, 5, a3)
	if len(retired) != 1 || retired[0].GetUpBytes() != 70 || retired[0].GetDownBytes() != 90 || retired[0].GetCounterEpoch() != epoch {
		t.Fatalf("retired %v, want route A's 70/90 in epoch %s", retired, epoch)
	}
	if c := countersOf(t, observe(t, d), ka); c.GetCounterEpoch() == epoch {
		t.Fatal("route A kept its epoch although a service was added")
	}
	// A configuration file that is not the recorded one makes Apply
	// reload: every hop's epoch ends.
	retired = nil
	cf := filepath.Join(cfg.Dir, gost.ConfigFile)
	if b, err := os.ReadFile(cf); err != nil || os.WriteFile(cf, append(b, '\n'), 0o600) != nil {
		t.Fatal(err)
	}
	apply(t, d, 6, a3, bb)
	if len(retired) != 1 || retired[0].GetRouteId() != ka.RouteID {
		t.Fatalf("a reload retired %v, want route A", retired)
	}
}
