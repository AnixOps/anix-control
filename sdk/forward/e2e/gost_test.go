//go:build linux

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// The gost engine in the suite (F4b: one gost entry, failover and
// counters; F4c: mixed-engine chains, mixed_test.go). gost runs inside the node's
// namespace as a child process (gost.ProcessSupervisor), the binary from
// ANIXOPS_GOST_BIN or PATH: with the suite enabled a missing gost fails.

// gostBinary answers the gost to run.
func gostBinary(t testing.TB) string {
	t.Helper()
	if p := os.Getenv("ANIXOPS_GOST_BIN"); p != "" {
		return p
	}
	p, err := exec.LookPath("gost")
	if err != nil {
		t.Fatalf("%s=1 needs gost for the gost tests: set ANIXOPS_GOST_BIN or put gost on PATH", envE2E)
	}
	return p
}

// gostRunner runs the gost driver's commands (ss, and gost for Probe)
// inside a node's namespace.
type gostRunner struct {
	ns  *netns
	bin string
}

var _ gost.Runner = gostRunner{}

func (r gostRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	bin := name
	switch name {
	case "ss":
	case "gost":
		bin = r.bin
	default:
		return nil, fmt.Errorf("forward e2e gost runner: refusing to run %q", name)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, errOut, err := r.ns.run(ctx, bin, args, stdin)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return out, &gost.CommandError{Name: name, Args: args, Stderr: string(errOut), Err: err}
	}
	return out, nil
}

// logBuffer collects gost's output for a failed test's log.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// gostNode makes ns a forwarding node that runs the gost driver, RAW links
// only (no link certificate): gost runs in the namespace, its files in a
// directory of the test.
func (l *lab) gostNode(ref string, ns *netns, addrs ...netip.Addr) *node {
	l.t.Helper()
	return l.gostNodeWith(ref, ns, nil, addrs...)
}

// gostNodeWith makes ns a gost node with the link certificate, key and CA
// of certs (linkCerts), so it carries encrypted links too; nil carries RAW
// links only.
func (l *lab) gostNodeWith(ref string, ns *netns, certs *[3]string, addrs ...netip.Addr) *node {
	l.t.Helper()
	bin := gostBinary(l.t)
	root, err := os.MkdirTemp("", "afe2e")
	if err != nil {
		l.t.Fatal(err)
	}
	l.t.Cleanup(func() { _ = os.RemoveAll(root) })
	base := gost.DefaultConfig()
	base.Dir, base.RuntimeDir = filepath.Join(root, "d"), filepath.Join(root, "r")
	base.LinkCert, base.LinkKey, base.LinkCA = "", "", ""
	if certs != nil {
		base.LinkCert, base.LinkKey, base.LinkCA = certs[0], certs[1], certs[2]
	}
	base.ReadyTimeout = 10 * time.Second
	for _, d := range []string{base.Dir, base.RuntimeDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			l.t.Fatal(err)
		}
	}
	log := &logBuffer{}
	sup := &gost.ProcessSupervisor{
		Command: []string{"ip", "netns", "exec", ns.name, bin},
		Config:  filepath.Join(base.Dir, gost.ConfigFile),
		Log:     log,
	}
	run := gostRunner{ns: ns, bin: bin}
	cfg, rep, err := gost.Probe(context.Background(), run, sup, base)
	if err != nil {
		l.t.Fatal(err)
	}
	if cfg.Version == "" {
		l.t.Fatalf("%s: gost driver unavailable: %s (missing %v)", ref, cfg.Unavailable, rep.Missing)
	}
	for _, w := range rep.Warnings {
		l.t.Logf("%s: probe: %s", ref, w)
	}
	l.t.Cleanup(func() {
		_ = sup.Stop(context.Background())
		if l.t.Failed() {
			l.t.Logf("gost on %s:\n%s", ref, log.String())
		}
	})
	n := &node{ref: ref, ns: ns, addrs: addrs, gost: sup}
	n.mk = func(t testing.TB) driver.Driver {
		t.Helper()
		d, err := gost.New(cfg, gost.WithRunner(run), gost.WithSupervisor(sup))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	n.drv = n.newDriver(l.t)
	l.nodes = append(l.nodes, n)
	return n
}

// checkPayload checks a gost hop's counted traffic: gost counts the bytes
// its services read from and wrote to their clients, so the counters equal
// the payload the client moved, and no packets.
func checkPayload(t testing.TB, what string, d traffic, up, down int) {
	t.Helper()
	if d.upBytes != uint64(up) || d.downBytes != uint64(down) || d.upPackets != 0 || d.downPackets != 0 { // #nosec G115 -- test sizes
		t.Errorf("%s: counted %v; want up %d and down %d payload bytes, no packets", what, d, up, down)
	}
}

// TestGostSingleHop: a gost entry forwards TCP and UDP, over IPv4 and IPv6,
// to a target; the entry hop's counters are the payload the client moved,
// per direction, exactly.
func TestGostSingleHop(t *testing.T) {
	s := newStarOf(t, model.EngineGost, "t1")
	t1 := s.targets[0]
	r := route(routeA, model.L4ProtocolTCPUDP, []model.Hop{{Role: model.HopRoleEntry, Engine: model.EngineGost, NodeRefs: []string{entry1}}}, target(t1.a4), target(t1.a6))
	d := s.deploy(t, r)
	if res := d.results[entry1]; !res.Changed || res.Generation != 1 {
		t.Fatalf("first apply: %+v", res)
	}
	sizes := make([]int, 20)
	for i := range sizes {
		sizes[i] = 100 + 61*i
	}
	for _, c := range []struct {
		name     string
		udp      bool
		listenOn netip.Addr
	}{
		{"tcp4", false, s.client.a4},
		{"tcp6", false, s.client.a6},
		{"udp4", true, s.client.a4},
		{"udp6", true, s.client.a6},
	} {
		t.Run(c.name, func(t *testing.T) {
			to := d.listen(t, entry1, routeA, 0, c.listenOn)
			before := s.entry.counters(t, routeA, 0)
			if c.udp {
				ex, err := udpEcho(s.cli, to, sizes, 3*time.Second)
				if err != nil {
					t.Fatalf("UDP exchange with %s: %v", to, err)
				}
				if ex.target != "t1" {
					t.Fatalf("answered by %q", ex.target)
				}
				checkPayload(t, c.name, counted(t, before, s.entry.counters(t, routeA, 0)), ex.up, ex.down)
				return
			}
			ex := mustEcho(t, s.cli, to, 256<<10)
			if ex.target != "t1" {
				t.Fatalf("answered by %q", ex.target)
			}
			after := s.entry.counters(t, routeA, 0)
			checkPayload(t, c.name, counted(t, before, after), ex.up, ex.down)
			if after.GetTotalConns() != before.GetTotalConns()+1 {
				t.Errorf("%s: total_conns %d -> %d", c.name, before.GetTotalConns(), after.GetTotalConns())
			}
		})
	}
}

// TestGostFailover: the primary target of a gost FAILOVER entry dies; the
// simulated health loop takes it out with SetUpstreams through gost's web
// API (no apply: generation, digest and counter epoch stay, gost keeps
// running), new connections reach the backup while a connection held on
// it lives on, and the primary takes over again once it is back and every
// upstream is restored. The counters grow by exactly the payload moved
// across all of it.
func TestGostFailover(t *testing.T) {
	s := newStarOf(t, model.EngineGost, "t1", "t2")
	t1, t2 := s.targets[0], s.targets[1]
	r := s.singleHop(model.L4ProtocolTCP)
	r.Policy.Target = model.BalanceFailover
	r.Targets[1].Priority = 1
	d := s.deploy(t, r)
	hop := d.hop(t, entry1, routeA, 0)
	in := s.in(t, d)
	moved := 0
	expect := func(t *testing.T, id string) {
		t.Helper()
		for i := range 5 {
			ex := mustEcho(t, s.cli, in, 4096)
			if ex.target != id {
				t.Fatalf("connection %d reached %s, want %s", i, ex.target, id)
			}
			moved += ex.up + ex.down
		}
	}
	rotation := func(t *testing.T) []driver.Upstream {
		t.Helper()
		for _, hr := range s.entry.observe(t).Rotation {
			if hr.RouteID == routeA && hr.HopIndex == 0 {
				return hr.Active
			}
		}
		t.Fatal("no rotation for the hop")
		return nil
	}
	expect(t, "t1")
	before := s.entry.observe(t)
	c0 := s.entry.counters(t, routeA, 0)
	moved = 0

	t1.srv.kill()
	up := s.entry.healthy(hop)
	if len(up) != 1 || up[0].Address != t2.a4.String() {
		t.Fatalf("health check: healthy %s, want only t2 (%s)", addrStrings(up), t2.a4)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.entry.drv.SetUpstreams(ctx, routeA, 0, up); err != nil {
		t.Fatalf("SetUpstreams(t2): %v", err)
	}
	expect(t, "t2")
	held, err := holdTCP(s.cli, in, 3*time.Second)
	if err != nil || held.target != "t2" {
		t.Fatalf("held connection: %v (%v)", held, err)
	}
	defer func() { _ = held.Close() }()
	moved += len("t2\n")
	after := s.entry.observe(t)
	if after.Generation != before.Generation || after.Digest != before.Digest || after.StateHash != before.StateHash {
		t.Fatalf("failover changed the applied state: generation %d -> %d, digest %s -> %s", before.Generation, after.Generation, before.Digest, after.Digest)
	}
	if got := rotation(t); len(got) != 1 || got[0].Address != t2.a4.String() {
		t.Fatalf("rotation after failover: %s", addrStrings(got))
	}

	t1.srv.start(t)
	up = s.entry.healthy(hop)
	if len(up) != 2 {
		t.Fatalf("health check after recovery: healthy %s, want both", addrStrings(up))
	}
	if err := s.entry.drv.SetUpstreams(ctx, routeA, 0, up); err != nil {
		t.Fatalf("SetUpstreams(t1, t2): %v", err)
	}
	expect(t, "t1")
	if got := rotation(t); len(got) != 2 {
		t.Fatalf("rotation after recovery: %s", addrStrings(got))
	}
	if err := held.ping(1000, 3*time.Second); err != nil {
		t.Fatalf("the connection held on t2 across the recovery: %v", err)
	}
	moved += 2000
	d2 := counted(t, c0, s.entry.counters(t, routeA, 0))        // same epoch, not decreasing
	if got := d2.upBytes + d2.downBytes; got != uint64(moved) { // #nosec G115 -- test sizes
		t.Fatalf("counted %v across the failover, want %d payload bytes in all", d2, moved)
	}
}
