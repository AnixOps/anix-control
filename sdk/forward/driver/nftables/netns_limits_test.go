package nftables

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// Real-kernel tests of what the conformance suite cannot see: the tc
// classes of bandwidth limits, quota usage across re-declaration, the tc
// rollback of a failed apply, foreign qdiscs, the probe's warnings and the
// retired counters of removed hops.

func e2eBuilder(t testing.TB, d *Driver) conformance.Builder {
	t.Helper()
	caps, err := d.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return conformance.Builder{Engine: d.Engine(), Caps: caps, Top: conformance.DefaultTopology()}
}

func limited(b conformance.Builder, route string, port int, bps uint64) *forwardv1.NodeHop {
	h := b.Simple(route, port)
	h.Limits = &forwardv1.Limits{BandwidthBps: bps, QuotaBytes: 1 << 30, MaxConns: 100}
	return h
}

func mustApply(t testing.TB, d *Driver, s *forwardv1.NodeForwardState) driver.ApplyResult {
	t.Helper()
	a, err := d.Render(s)
	if err != nil {
		t.Fatal(err)
	}
	r, err := d.Apply(context.Background(), a)
	if err != nil {
		t.Fatalf("Apply generation %d: %v", s.GetGeneration(), err)
	}
	return r
}

func tcLines(t testing.TB, d *Driver) []string {
	t.Helper()
	l, err := d.tcListing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// TestNetnsBandwidthClasses: rate-limited hops get an HTB class and a fw
// filter per direction on the limit interface; a changed rate changes the
// class in place; a removed hop loses its classes; no limited hop left
// removes the qdisc. The foreign qdisc on the other interface stays.
func TestNetnsBandwidthClasses(t *testing.T) {
	forTCOutputs(t, testBandwidthClasses)
}

// forTCOutputs runs a test with tc's JSON listings and with the text
// listings older iproute2 prints.
func forTCOutputs(t *testing.T, f func(t *testing.T, text bool)) {
	for _, text := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "text"}[text], func(t *testing.T) { f(t, text) })
	}
}

func testBandwidthClasses(t *testing.T, text bool) {
	e := newNsEnv(t)
	e.run.textTC = text
	e.PlantForeign(t)
	foreign := e.Foreign(t)
	d := e.driver(t)
	b := e2eBuilder(t, d)
	node := b.Top.NodeRef

	mustApply(t, d, conformance.State(node, 1, limited(b, conformance.RouteA, 0, 100_000_000), limited(b, conformance.RouteB, 1, 8_000_000)))
	want := []string{
		"tc lim0 qdisc htb af00:",
		"tc lim0 class af00:2 rate 100Mbit",
		"tc lim0 class af00:3 rate 100Mbit",
		"tc lim0 class af00:4 rate 8Mbit",
		"tc lim0 class af00:5 rate 8Mbit",
		"tc lim0 filter 0x10000 -> af00:2",
		"tc lim0 filter 0x10001 -> af00:3",
		"tc lim0 filter 0x20000 -> af00:4",
		"tc lim0 filter 0x20001 -> af00:5",
	}
	if got := tcLines(t, d); !slices.Equal(got, want) {
		t.Fatalf("tc after the first apply:\n got  %q\n want %q", got, want)
	}

	// Route B's rate changes, route A goes away.
	mustApply(t, d, conformance.State(node, 2, limited(b, conformance.RouteB, 1, 16_000_000)))
	want = []string{
		"tc lim0 qdisc htb af00:",
		"tc lim0 class af00:4 rate 16Mbit",
		"tc lim0 class af00:5 rate 16Mbit",
		"tc lim0 filter 0x20000 -> af00:4",
		"tc lim0 filter 0x20001 -> af00:5",
	}
	if got := tcLines(t, d); !slices.Equal(got, want) {
		t.Fatalf("tc after the second apply:\n got  %q\n want %q", got, want)
	}

	// No limit left: the qdisc goes, the table stays.
	mustApply(t, d, conformance.State(node, 3, b.Simple(conformance.RouteB, 1)))
	if got := tcLines(t, d); len(got) != 0 {
		t.Fatalf("tc after removing every limit: %q", got)
	}
	if got := e.Foreign(t); !slices.Equal(got, foreign) {
		t.Fatalf("foreign objects changed:\n got  %q\n want %q", got, foreign)
	}

	// Damaged tc state is repaired by applying the same artifact.
	mustApply(t, d, conformance.State(node, 4, limited(b, conformance.RouteB, 1, 16_000_000)))
	e.ns.must(t, "tc", "filter", "del", "dev", limitIface, "parent", "af00:", "protocol", "all", "prio", "10", "handle", "0x20001/0xfff0001", "fw")
	a, _ := d.Render(conformance.State(node, 4, limited(b, conformance.RouteB, 1, 16_000_000)))
	if r, err := d.Apply(context.Background(), a); err != nil || !r.Changed {
		t.Fatalf("apply over damaged tc: changed %v, %v", r.Changed, err)
	}
	if got := tcLines(t, d); !slices.Equal(got, want) {
		t.Fatalf("tc after repair:\n got  %q\n want %q", got, want)
	}
	if err := d.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := tcLines(t, d); len(got) != 0 {
		t.Fatalf("tc after Remove: %q", got)
	}
}

// TestNetnsBandwidthRollback: when the kernel refuses the transaction,
// the tc classes added for it are removed and changed rates restored.
func TestNetnsBandwidthRollback(t *testing.T) {
	forTCOutputs(t, testBandwidthRollback)
}

func testBandwidthRollback(t *testing.T, text bool) {
	e := newNsEnv(t)
	e.run.textTC = text
	d := e.driver(t)
	b := e2eBuilder(t, d)
	node := b.Top.NodeRef
	mustApply(t, d, conformance.State(node, 1, limited(b, conformance.RouteA, 0, 100_000_000)))
	before, tcBefore := e.Owned(t), tcLines(t, d)
	e.FailNextApply(t)
	a, _ := d.Render(conformance.State(node, 2, limited(b, conformance.RouteA, 0, 50_000_000), limited(b, conformance.RouteB, 1, 8_000_000)))
	if _, err := d.Apply(context.Background(), a); err == nil {
		t.Fatal("Apply succeeded despite the injected failure")
	}
	if got := tcLines(t, d); !slices.Equal(got, tcBefore) {
		t.Fatalf("tc after a failed apply:\n got  %q\n want %q", got, tcBefore)
	}
	if got := e.Owned(t); !slices.Equal(got, before) {
		t.Fatalf("table after a failed apply differs")
	}
	if _, err := d.Apply(context.Background(), a); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

// TestNetnsForeignRootQdisc: a foreign root qdisc on the limit interface
// refuses rate-limited hops with ErrConflict and is left alone; hops
// without a limit still apply.
func TestNetnsForeignRootQdisc(t *testing.T) {
	e := newNsEnv(t)
	e.ns.must(t, "tc", "qdisc", "add", "dev", limitIface, "root", "handle", "1:", "fq_codel")
	d := e.driver(t)
	b := e2eBuilder(t, d)
	a, _ := d.Render(conformance.State(b.Top.NodeRef, 1, limited(b, conformance.RouteA, 0, 1_000_000)))
	if _, err := d.Apply(context.Background(), a); !errors.Is(err, driver.ErrConflict) {
		t.Fatalf("Apply over a foreign root qdisc: %v, want ErrConflict", err)
	}
	if out := string(e.ns.must(t, "tc", "qdisc", "show", "dev", limitIface)); !strings.Contains(out, "fq_codel 1: root") {
		t.Fatalf("foreign qdisc changed: %s", out)
	}
	mustApply(t, d, conformance.State(b.Top.NodeRef, 1, b.Simple(conformance.RouteA, 0)))
	if err := d.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	if out := string(e.ns.must(t, "tc", "qdisc", "show", "dev", limitIface)); !strings.Contains(out, "fq_codel 1: root") {
		t.Fatalf("Remove touched the foreign qdisc: %s", out)
	}
}

// TestNetnsQuotaKeepsUsage: re-declaring a quota (a re-apply, a changed
// limit) keeps what it used, and a counter keeps its epoch comment. This
// is nft and kernel behaviour the driver relies on; CI runs it on the
// runner's nft.
func TestNetnsQuotaKeepsUsage(t *testing.T) {
	e := newNsEnv(t)
	d := e.driver(t)
	b := e2eBuilder(t, d)
	hop := limited(b, conformance.RouteA, 0, 0)
	base := hopName(driver.KeyOf(hop))
	// An owned table whose quota has used 50 bytes and whose counter has
	// counted, as if traffic had flowed.
	e.ns.nft(t, fmt.Sprintf(`table inet anixops_fwd {
	comment %q
	quota %[2]s_quota {
		over 1073741824 bytes used 50 bytes
	}
	counter %[2]s_up {
		comment "anixops epoch feedc0de"
		packets 3 bytes 180
	}
}
`, OwnerComment, base))
	mustApply(t, d, conformance.State(b.Top.NodeRef, 1, hop))
	hop.Limits.QuotaBytes = 2 << 30
	mustApply(t, d, conformance.State(b.Top.NodeRef, 2, hop))
	h, err := d.readHost(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	q, ok := h.table.object("quota", base+"_quota")
	if !ok || q.num("used") != 50 || q.num("bytes") != 2<<30 {
		t.Fatalf("quota after re-declaration: %v", q.fields)
	}
	o, err := d.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c := o.Counters[0]
	if c.GetUpPackets() != 3 || c.GetUpBytes() != 180 || !strings.HasPrefix(c.GetCounterEpoch(), "feedc0de.") {
		t.Fatalf("counters after re-declaration: %v", c)
	}
}

// TestNetnsRetiredCounters: an apply that removes a hop hands its last
// counters to the WithRetiredCounters hook.
func TestNetnsRetiredCounters(t *testing.T) {
	e := newNsEnv(t)
	var retired []*forwardv1.Counters
	d, err := New(e.cfg, WithRunner(e.run), WithRetiredCounters(func(c []*forwardv1.Counters) { retired = append(retired, c...) }))
	if err != nil {
		t.Fatal(err)
	}
	b := e2eBuilder(t, d)
	a, bb := b.Simple(conformance.RouteA, 0), b.Simple(conformance.RouteB, 1)
	mustApply(t, d, conformance.State(b.Top.NodeRef, 1, a, bb))
	e.Traffic(t, driver.KeyOf(bb))
	mustApply(t, d, conformance.State(b.Top.NodeRef, 2, a))
	if len(retired) != 1 || retired[0].GetRouteId() != conformance.RouteB || retired[0].GetUpPackets() == 0 {
		t.Fatalf("retired counters %v", retired)
	}
	h, err := d.readHost(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range h.table.items {
		if strings.Contains(it.str("name"), conformance.RouteB) {
			t.Fatalf("object of the removed hop left: %s %s", it.kind, it.str("name"))
		}
	}
}

// TestNetnsProbe: the probe finds every feature in a namespace and warns
// about a foreign forward chain that drops by default, without changing
// it.
func TestNetnsProbe(t *testing.T) {
	ns := newNetns(t)
	ns.nft(t, "table ip filter {\n\tchain FORWARD {\n\t\ttype filter hook forward priority filter; policy drop;\n\t}\n}\n")
	before := ns.must(t, "nft", "list", "ruleset")
	cfg, rep, err := Probe(context.Background(), &nsRunner{ns: ns}, e2eConfig())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version == "" || len(rep.Missing) != 0 || !cfg.BandwidthLimit || !cfg.IPv6 || !cfg.Quota || !cfg.MaxConns || len(cfg.Strategies) != 5 {
		t.Fatalf("probe: %+v, report %+v", cfg, rep)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "table ip filter chain FORWARD drops") {
		t.Fatalf("warnings %q", rep.Warnings)
	}
	if after := ns.must(t, "nft", "list", "ruleset"); string(after) != string(before) {
		t.Fatalf("probe changed the ruleset:\n%s", after)
	}
	t.Logf("probed %s", cfg.Version)
}
