//go:build linux

package e2e

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// Route ids are ULIDs, as Control assigns them.
const (
	routeA = "01JF2D0000000000000000000A"
	entry1 = "forward-1"
	relay2 = "forward-2"
	exit3  = "forward-3"
)

// star is a client, one entry node and target hosts, each target on its
// own link from the entry with an echo server on both its addresses.
type star struct {
	*lab
	cli     *netns
	client  *link // entry (a) to client (b)
	entry   *node
	engine  model.Engine // the entry's
	targets []*targetHost
}

type targetHost struct {
	id     string
	ns     *netns
	a4, a6 netip.Addr
	srv    *server
}

// newStar builds a star around an nftables entry.
func newStar(t *testing.T, ids ...string) *star {
	t.Helper()
	return newStarOf(t, model.EngineNFTables, ids...)
}

// newStarOf builds a star around an entry of the engine.
func newStarOf(t *testing.T, engine model.Engine, ids ...string) *star {
	t.Helper()
	l := newLab(t)
	s := &star{lab: l, cli: l.netns("cli"), engine: engine}
	ent := l.netns("ent")
	s.client = l.connect(ent, s.cli, clientNet4, clientNet6)
	limit := []string{s.client.aIf}
	var addrs []netip.Addr
	for _, id := range ids {
		ns := l.netns(id)
		k := l.connectNext(ent, ns)
		limit = append(limit, k.aIf)
		addrs = append(addrs, k.a4, k.a6)
		th := &targetHost{id: id, ns: ns, a4: k.b4, a6: k.b6}
		th.srv = l.serve(ns, id, th.a4, th.a6)
		s.targets = append(s.targets, th)
	}
	addrs = append(addrs, s.client.a4, s.client.a6)
	if engine == model.EngineGost {
		s.entry = l.gostNode(entry1, ent, addrs...)
	} else {
		s.entry = l.node(entry1, ent, limit, addrs...)
	}
	return s
}

// singleHop is a one-hop route of the protocol from the entry to every
// target's IPv4 address.
func (s *star) singleHop(protocol model.L4Protocol) model.Route {
	var targets []model.Target
	for _, th := range s.targets {
		targets = append(targets, target(th.a4))
	}
	entry := entryHop(entry1)
	entry.Engine = s.engine
	return route(routeA, protocol, []model.Hop{entry}, targets...)
}

// in answers where the client reaches the route over IPv4.
func (s *star) in(t testing.TB, d deployment) netip.AddrPort {
	t.Helper()
	return d.listen(t, entry1, routeA, 0, s.client.a4)
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// TestSingleHop: one nftables entry forwards TCP and UDP, over IPv4 and
// IPv6, to a target; the client gets the target's answers and the entry
// hop's counters match the bytes moved, per direction.
func TestSingleHop(t *testing.T) {
	s := newStar(t, "t1")
	t1 := s.targets[0]
	r := route(routeA, model.L4ProtocolTCPUDP, []model.Hop{entryHop(entry1)}, target(t1.a4), target(t1.a6))
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
		v6, udp  bool
		listenOn netip.Addr
	}{
		{"tcp4", false, false, s.client.a4},
		{"tcp6", true, false, s.client.a6},
		{"udp4", false, true, s.client.a4},
		{"udp6", true, true, s.client.a6},
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
				checkUDP(t, c.name, counted(t, before, s.entry.counters(t, routeA, 0)), ex, c.v6)
				return
			}
			ex := mustEcho(t, s.cli, to, 256<<10)
			if ex.target != "t1" {
				t.Fatalf("answered by %q", ex.target)
			}
			checkTCP(t, c.name, counted(t, before, s.entry.counters(t, routeA, 0)), ex.up, ex.down, c.v6)
		})
	}
}

// TestTwoHopAdmission: an nftables entry hands over to an nftables exit
// on another node over a RAW link; both hops count the traffic, and the
// exit admits only the entry's addresses: the client itself, routed to
// the exit's port, is dropped.
func TestTwoHopAdmission(t *testing.T) {
	l := newLab(t)
	cli, ent, rel, t1 := l.netns("cli"), l.netns("ent"), l.netns("rel"), l.netns("t1")
	cl := l.connect(ent, cli, clientNet4, clientNet6)
	er := l.connectNext(ent, rel)
	rt := l.connectNext(rel, t1)
	l.serve(t1, "t1", rt.b4)
	// The relay's own echo server on a port no route uses: a control for
	// the admission check below.
	l.serve(rel, "rel", er.b4)
	// The entry's first address is the one it masquerades to the relay
	// with; the relay's first is where the entry dials it.
	entry := l.node(entry1, ent, nil, er.a4, cl.a4)
	relay := l.node(relay2, rel, nil, er.b4)

	r := route(routeA, model.L4ProtocolTCP, []model.Hop{entryHop(entry1), exitHop(relay2)}, target(rt.b4))
	d := l.deploy(t, r)
	exit := d.hop(t, relay2, routeA, 1)
	if got, want := exit.GetIngressSources(), []string{er.a4.String(), cl.a4.String()}; !slices.Equal(got, want) {
		t.Fatalf("exit admits %v, want the entry's addresses %v", got, want)
	}
	in := d.listen(t, entry1, routeA, 0, cl.a4)
	exitPort := d.listen(t, relay2, routeA, 1, er.b4)
	if ups := d.hop(t, entry1, routeA, 0).GetUpstreams(); len(ups) != 1 || ups[0].GetAddress() != er.b4.String() || ups[0].GetPort() != uint32(exitPort.Port()) {
		t.Fatalf("entry upstreams %v, want the exit at %s", ups, exitPort)
	}

	e0, r0 := entry.counters(t, routeA, 0), relay.counters(t, routeA, 1)
	ex := mustEcho(t, cli, in, 128<<10)
	if ex.target != "t1" {
		t.Fatalf("answered by %q", ex.target)
	}
	checkTCP(t, "entry hop 0", counted(t, e0, entry.counters(t, routeA, 0)), ex.up, ex.down, false)
	checkTCP(t, "exit hop 1", counted(t, r0, relay.counters(t, routeA, 1)), ex.up, ex.down, false)

	// Route the client to the relay through the entry, which forwards it
	// untouched (it is none of the entry's hops), and the relay back.
	cli.must(t, "ip", "route", "add", er.p4.String(), "via", cl.a4.String())
	rel.must(t, "ip", "route", "add", clientNet4.String(), "via", er.a4.String())
	if ex, err := tcpEcho(cli, netip.Addr{}, netip.AddrPortFrom(er.b4, targetPort), 64, 5*time.Second); err != nil || ex.target != "rel" {
		t.Fatalf("control: the client cannot reach the relay's own server (%v, %q), so the check below proves nothing", err, ex.target)
	}
	r1 := relay.counters(t, routeA, 1)
	if err := mustNotConnect(t, cli, exitPort, "an unadmitted source"); !isTimeout(err) {
		t.Errorf("an unadmitted source got %v; want its SYN dropped (a timeout)", err)
	}
	if got := counted(t, r1, relay.counters(t, routeA, 1)); got != (traffic{}) {
		t.Errorf("the exit counted the unadmitted source: %v", got)
	}
	if ex := mustEcho(t, cli, in, 1024); ex.target != "t1" {
		t.Fatalf("answered by %q", ex.target)
	}
}

// TestThreeHopChain: entry, relay and exit, all nftables over RAW links:
// each hop dials the next node's first address on the port planned there,
// admits only the previous node's addresses, and counts the same traffic.
func TestThreeHopChain(t *testing.T) {
	l := newLab(t)
	cli, ent, rel, ext, t1 := l.netns("cli"), l.netns("ent"), l.netns("rel"), l.netns("ext"), l.netns("t1")
	cl := l.connect(ent, cli, clientNet4, clientNet6)
	er := l.connectNext(ent, rel)
	re := l.connectNext(rel, ext)
	xt := l.connectNext(ext, t1)
	l.serve(t1, "t1", xt.b4)
	// Each node's first address is where the previous hop dials it; the
	// address it masquerades to the next hop with is among the others.
	nodes := []*node{
		l.node(entry1, ent, nil, er.a4, cl.a4),
		l.node(relay2, rel, nil, er.b4, re.a4),
		l.node(exit3, ext, nil, re.b4),
	}
	r := route(routeA, model.L4ProtocolTCP, []model.Hop{entryHop(entry1), relayHop(relay2), exitHop(exit3)}, target(xt.b4))
	d := l.deploy(t, r)
	for i, want := range [][]string{nil, {er.a4.String(), cl.a4.String()}, {er.b4.String(), re.a4.String()}} {
		if got := d.hop(t, nodes[i].ref, routeA, uint32(i)).GetIngressSources(); !slices.Equal(got, want) { // #nosec G115 -- a hop index
			t.Fatalf("hop %d admits %v, want %v", i, got, want)
		}
	}
	before := make([]*forwardv1.Counters, len(nodes))
	for i, n := range nodes {
		before[i] = n.counters(t, routeA, uint32(i)) // #nosec G115 -- a hop index
	}
	ex := mustEcho(t, cli, d.listen(t, entry1, routeA, 0, cl.a4), 128<<10)
	if ex.target != "t1" {
		t.Fatalf("answered by %q", ex.target)
	}
	for i, n := range nodes {
		checkTCP(t, fmt.Sprintf("hop %d on %s", i, n.ref), counted(t, before[i], n.counters(t, routeA, uint32(i))), ex.up, ex.down, false) // #nosec G115 -- a hop index
	}
}

// spread answers which target each of n new connections reached.
func spread(t testing.TB, from *netns, to netip.AddrPort, n int, src func(i int) netip.Addr) []string {
	t.Helper()
	out := make([]string, n)
	for i := range n {
		var local netip.Addr
		if src != nil {
			local = src(i)
		}
		ex, err := tcpEcho(from, local, to, 32, 5*time.Second)
		if err != nil {
			t.Fatalf("connection %d to %s: %v", i, to, err)
		}
		out[i] = ex.target
	}
	return out
}

func tally(hits []string) map[string]int {
	m := map[string]int{}
	for _, h := range hits {
		m[h]++
	}
	return m
}

// TestBalance: each balance strategy spreads new connections over three
// targets as rendered: round robin in turn (by weight), random and least
// connections (weighted random until the Agent re-weights it) in
// proportion, IP hash stable per client address.
func TestBalance(t *testing.T) {
	s := newStar(t, "t1", "t2", "t3")
	var sources []netip.Addr
	for i := 10; i < 34; i++ {
		a := host(clientNet4, i)
		s.cli.must(t, "ip", "addr", "add", netip.PrefixFrom(a, clientNet4.Bits()).String(), "dev", s.client.bIf)
		sources = append(sources, a)
	}
	deploy := func(t *testing.T, strategy model.BalanceStrategy, weights ...uint32) netip.AddrPort {
		t.Helper()
		r := s.singleHop(model.L4ProtocolTCP)
		r.Policy.Target = strategy
		for i := range weights {
			r.Targets[i].Weight = weights[i]
		}
		d := s.deploy(t, r)
		if h := d.hop(t, entry1, routeA, 0); h.GetBalance().String() != strategy.String() {
			t.Fatalf("entry balances with %s, want %s", h.GetBalance(), strategy)
		}
		return s.in(t, d)
	}

	t.Run("round_robin", func(t *testing.T) {
		// 30 connections right after the apply (which restarts numgen)
		// take the first 30 of 128 interleaved slots: 10 each, give or
		// take one.
		hits := spread(t, s.cli, deploy(t, model.BalanceRoundRobin), 30, nil)
		c := tally(hits)
		lo, hi := 30, 0
		for _, th := range s.targets {
			lo, hi = min(lo, c[th.id]), max(hi, c[th.id])
		}
		if len(c) != 3 || hi-lo > 1 {
			t.Fatalf("round robin over 3 targets: %v (%v)", c, hits)
		}
		t.Logf("round robin: %v", c)
	})
	t.Run("round_robin_weighted", func(t *testing.T) {
		hits := spread(t, s.cli, deploy(t, model.BalanceRoundRobin, 2, 1, 1), 40, nil)
		c := tally(hits)
		for id, want := range map[string]int{"t1": 20, "t2": 10, "t3": 10} {
			if d := c[id] - want; d < -2 || d > 2 {
				t.Fatalf("weighted round robin 2:1:1 over 40 connections: %v, want 20/10/10 within 2", c)
			}
		}
		t.Logf("weighted round robin: %v", c)
	})
	for _, strategy := range []model.BalanceStrategy{model.BalanceRandom, model.BalanceLeastConn} {
		t.Run(map[model.BalanceStrategy]string{model.BalanceRandom: "random", model.BalanceLeastConn: "least_conn"}[strategy], func(t *testing.T) {
			to := deploy(t, strategy)
			// 90 connections, 30 expected per target (standard deviation
			// 4.5): each must get at least 15. A sample that misses is
			// retried twice before the test fails.
			var c map[string]int
			for attempt := range 3 {
				c = tally(spread(t, s.cli, to, 90, nil))
				ok := len(c) == 3
				for _, n := range c {
					ok = ok && n >= 15
				}
				if ok {
					t.Logf("%s: %v (attempt %d)", strategy, c, attempt+1)
					return
				}
				t.Logf("%s: uneven sample %v (attempt %d)", strategy, c, attempt+1)
			}
			t.Fatalf("%s over 3 targets: %v, want every target at least 15 of 90", strategy, c)
		})
	}
	t.Run("ip_hash", func(t *testing.T) {
		to := deploy(t, model.BalanceIPHash)
		// Three connections from each of 24 client addresses: each address
		// sticks to one target, and the addresses spread over several.
		byTarget := map[string]int{}
		for _, src := range sources {
			hits := spread(t, s.cli, to, 3, func(int) netip.Addr { return src })
			if hits[0] != hits[1] || hits[1] != hits[2] {
				t.Fatalf("IP hash: %s reached %v", src, hits)
			}
			byTarget[hits[0]]++
		}
		if len(byTarget) < 2 {
			t.Fatalf("IP hash sent all %d client addresses to one target: %v", len(sources), byTarget)
		}
		t.Logf("IP hash: client addresses per target %v", byTarget)
	})
}

// TestFailover: failover sends everything to the primary target; when it
// dies, connections fail until the health loop (simulated: TCP checks from
// the entry node) takes it out of rotation with SetUpstreams, which moves
// traffic to the backup without a new generation or a counter reset; when
// it comes back, the loop restores the rendered upstreams and traffic
// returns to it.
func TestFailover(t *testing.T) {
	s := newStar(t, "t1", "t2")
	t1, t2 := s.targets[0], s.targets[1]
	r := s.singleHop(model.L4ProtocolTCP)
	r.Policy.Target = model.BalanceFailover
	r.Targets[1].Priority = 1
	d := s.deploy(t, r)
	hop := d.hop(t, entry1, routeA, 0)
	in := s.in(t, d)
	expect := func(t *testing.T, id string) {
		t.Helper()
		for i, hit := range spread(t, s.cli, in, 5, nil) {
			if hit != id {
				t.Fatalf("connection %d reached %s, want %s", i, hit, id)
			}
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

	t1.srv.kill()
	err := mustNotConnect(t, s.cli, in, "the primary is down and still in rotation")
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Logf("connection to a dead primary: %v", err)
	}
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
	after := s.entry.observe(t)
	if after.Generation != before.Generation || after.Digest != before.Digest || after.StateHash != before.StateHash {
		t.Fatalf("failover changed the applied state: generation %d -> %d, digest %s -> %s", before.Generation, after.Generation, before.Digest, after.Digest)
	}
	if got := rotation(t); len(got) != 1 || got[0].Address != t2.a4.String() {
		t.Fatalf("rotation after failover: %s", addrStrings(got))
	}
	counted(t, before.Counters[0], after.Counters[0]) // same epoch, not decreasing

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
}

// TestQuota: a route with a small byte quota (both directions) carries
// traffic until the quota is used up; then every new connection fails.
// Raising the quota re-declares it in place: what was used stays used and
// traffic flows again.
func TestQuota(t *testing.T) {
	s := newStar(t, "t1")
	const quota, size = 300_000, 32 << 10
	r := s.singleHop(model.L4ProtocolTCP)
	r.Limits.QuotaBytes = quota
	d := s.deploy(t, r)
	if got := d.hop(t, entry1, routeA, 0).GetLimits().GetQuotaBytes(); got != quota {
		t.Fatalf("entry hop quota %d, want %d", got, quota)
	}
	in := s.in(t, d)
	moved, done := 0, 0
	for ; done < 20; done++ {
		ex, err := tcpEcho(s.cli, netip.Addr{}, in, size, 2*time.Second)
		if err != nil {
			t.Logf("exchange %d stopped after %d payload bytes: %v", done, moved, err)
			break
		}
		moved += ex.up + ex.down
	}
	switch {
	case done == 20:
		t.Fatalf("the quota of %d bytes never stopped traffic: %d payload bytes moved", quota, moved)
	case moved > quota:
		t.Fatalf("%d payload bytes moved in complete exchanges, more than the %d-byte quota", moved, quota)
	case moved < quota/2:
		t.Fatalf("traffic stopped after %d payload bytes, far below the %d-byte quota", moved, quota)
	}
	for i := range 2 {
		_ = mustNotConnect(t, s.cli, in, fmt.Sprintf("new connection %d after the quota", i))
	}
	used := s.entry.counters(t, routeA, 0)
	if total := used.GetUpBytes() + used.GetDownBytes(); total > quota {
		t.Fatalf("counted %d bytes past a %d-byte quota", total, quota)
	}

	r.Limits.QuotaBytes = quota + 10<<20
	s.deploy(t, r)
	mustEcho(t, s.cli, in, size)
	if d := counted(t, used, s.entry.counters(t, routeA, 0)); d.upBytes < size || d.downBytes < size {
		t.Fatalf("after raising the quota: %v", d)
	}
}

// TestConnLimit: with max_conns N, N connections stay open and the N+1th
// is refused (ct count rejects it); closing one frees its slot.
func TestConnLimit(t *testing.T) {
	s := newStar(t, "t1")
	const limit = 3
	r := s.singleHop(model.L4ProtocolTCP)
	r.Limits.MaxConns = limit
	d := s.deploy(t, r)
	in := s.in(t, d)
	var held []*heldConn
	t.Cleanup(func() {
		for _, h := range held {
			_ = h.Close()
		}
	})
	for i := range limit {
		h, err := holdTCP(s.cli, in, 5*time.Second)
		if err != nil {
			t.Fatalf("connection %d of %d: %v", i+1, limit, err)
		}
		held = append(held, h)
	}
	_, err := tcpEcho(s.cli, netip.Addr{}, in, 64, 3*time.Second)
	switch {
	case err == nil:
		t.Fatalf("connection %d was accepted with max_conns %d", limit+1, limit)
	case !errors.Is(err, syscall.ECONNREFUSED):
		t.Errorf("connection %d: %v; want it refused (reject)", limit+1, err)
	}
	for i, h := range held {
		if err := h.ping(1024, 5*time.Second); err != nil {
			t.Fatalf("held connection %d stopped working: %v", i+1, err)
		}
	}
	_ = held[0].Close()
	held = held[1:]
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err := tcpEcho(s.cli, netip.Addr{}, in, 64, 3*time.Second)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("closing a connection did not free a slot: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// htbAvailable checks that the kernel has HTB, on a dummy interface of
// the namespace.
func htbAvailable(t testing.TB, ns *netns) error {
	t.Helper()
	ns.must(t, "ip", "link", "add", "htbcheck", "type", "dummy")
	defer ns.must(t, "ip", "link", "del", "htbcheck")
	_, errOut, err := ns.run(context.Background(), "tc", []string{"qdisc", "add", "dev", "htbcheck", "root", "handle", "1:", "htb"}, nil)
	if err != nil {
		return fmt.Errorf("%w: %s", err, errOut)
	}
	return nil
}

var sentBytes = regexp.MustCompile(`Sent (\d+) bytes`)

// classSent answers the bytes a tc class sent, from the text listing of
// every class on the device ("class htb af00:2 root ..." followed by
// " Sent 123 bytes ..."), which every iproute2 prints.
func classSent(t testing.TB, ns *netns, dev, classID string) uint64 {
	t.Helper()
	out := ns.must(t, "tc", "-s", "class", "show", "dev", dev)
	current := ""
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) >= 3 && f[0] == "class" {
			current = f[2]
			continue
		}
		if m := sentBytes.FindStringSubmatch(line); m != nil && current == classID {
			n, _ := strconv.ParseUint(m[1], 10, 64)
			return n
		}
	}
	t.Fatalf("no tc class %s on %s:\n%s", classID, dev, out)
	return 0
}

// throughput answers the steady rate, in bits per second, at which the
// echo of size bytes comes back: from the first quarter of the bytes to the
// last, so neither the start of the transfer nor the queues filling and
// draining at its ends count. The echo is shaped in both directions, so a
// direction limited below the rate shows.
func throughput(t testing.TB, from *netns, to netip.AddrPort, size int) float64 {
	t.Helper()
	c, err := from.dialTCP(netip.Addr{}, to, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.SetDeadline(time.Now().Add(60 * time.Second)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("no greeting: %v", err)
	}
	werr := make(chan error, 1)
	go func() {
		_, err := c.Write(payload(size))
		if err == nil {
			err = c.CloseWrite()
		}
		werr <- err
	}()
	buf := make([]byte, 64<<10)
	got, mark := 0, size/4
	var from25 time.Time
	for got < size {
		n, err := r.Read(buf)
		got += n
		if from25.IsZero() && got >= mark {
			from25, mark = time.Now(), got
		}
		if err != nil {
			t.Fatalf("echo stopped after %d of %d bytes: %v", got, size, err)
		}
	}
	elapsed := time.Since(from25)
	if err := <-werr; err != nil {
		t.Fatal(err)
	}
	return float64(size-mark) * 8 / elapsed.Seconds()
}

// TestBandwidth: bandwidth_bps shapes each direction of the route with tc
// HTB on the entry: an echo through it runs at the limit (within a
// generous tolerance), much slower than the same path unlimited, and both
// directions pass through the hop's classes.
func TestBandwidth(t *testing.T) {
	s := newStar(t, "t1")
	if !s.entry.cfg.BandwidthLimit {
		t.Skip("the probe found no bandwidth limit support")
	}
	if err := htbAvailable(t, s.entry.ns); err != nil {
		t.Skipf("tc HTB unavailable: %v", err)
	}
	const limit = 8_000_000 // bits per second, each direction
	r := s.singleHop(model.L4ProtocolTCP)
	d := s.deploy(t, r)
	in := s.in(t, d)
	base := throughput(t, s.cli, in, 8<<20)
	t.Logf("unlimited: %.1f Mbit/s", base/1e6)
	if base < 4*limit {
		t.Fatalf("the unlimited path runs at %.1f Mbit/s, too slow to tell a %d bit/s limit", base/1e6, limit)
	}

	r.Limits.BandwidthBPS = limit
	d = s.deploy(t, r)
	const size = 2_500_000 // about 2.5 s at the limit
	got := throughput(t, s.cli, in, size)
	ratio := got / limit
	t.Logf("limited to %d bit/s: %.2f Mbit/s (%.2f of the limit)", limit, got/1e6, ratio)
	if ratio < 0.5 || ratio > 1.5 {
		t.Fatalf("throughput %.2f Mbit/s is outside 0.5-1.5 times the %d bit/s limit", got/1e6, limit)
	}
	// Up leaves the entry towards the target in class 2*mark, down
	// towards the client in class 2*mark+1.
	mark := d.hop(t, entry1, routeA, 0).GetMark()
	upClass := "af00:" + strconv.FormatUint(uint64(2*mark), 16)
	downClass := "af00:" + strconv.FormatUint(uint64(2*mark+1), 16)
	if sent := classSent(t, s.entry.ns, "to-t1", upClass); sent < size {
		t.Errorf("up class %s on to-t1 sent %d bytes, less than the %d-byte payload", upClass, sent, size)
	}
	if sent := classSent(t, s.entry.ns, "to-cli", downClass); sent < size {
		t.Errorf("down class %s on to-cli sent %d bytes, less than the %d-byte payload", downClass, sent, size)
	}
}

// TestPause: pausing a route keeps its hops applied but drops its
// traffic, new and established, uncounted; the counters keep their epoch
// and values, and unpausing resumes counting from them.
func TestPause(t *testing.T) {
	s := newStar(t, "t1")
	r := s.singleHop(model.L4ProtocolTCP)
	d := s.deploy(t, r)
	in := s.in(t, d)
	held, err := holdTCP(s.cli, in, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	mustEcho(t, s.cli, in, 32<<10)
	running := s.entry.counters(t, routeA, 0)

	r.Paused = true
	paused := s.deploy(t, r)
	if g0, g1 := d.results[entry1].Generation, paused.results[entry1].Generation; g1 <= g0 || !paused.results[entry1].Changed {
		t.Fatalf("pausing: generation %d -> %d, changed %v", g0, g1, paused.results[entry1].Changed)
	}
	if !paused.hop(t, entry1, routeA, 0).GetPaused() {
		t.Fatal("the planned hop is not paused")
	}
	if err := mustNotConnect(t, s.cli, in, "the route is paused"); !isTimeout(err) {
		t.Errorf("new connection to a paused route: %v; want it dropped (a timeout)", err)
	}
	if err := held.ping(64, 1500*time.Millisecond); err == nil {
		t.Error("an established connection still works while the route is paused")
	}
	if got := counted(t, running, s.entry.counters(t, routeA, 0)); got != (traffic{}) {
		t.Errorf("counted while paused: %v", got)
	}

	r.Paused = false
	s.deploy(t, r)
	ex := mustEcho(t, s.cli, in, 32<<10)
	if got := counted(t, running, s.entry.counters(t, routeA, 0)); got.upBytes < uint64(ex.up) || got.downBytes < uint64(ex.down) {
		t.Fatalf("after unpausing: %v for %d bytes up, %d down", got, ex.up, ex.down)
	}
}

// TestAgentRestart: a new driver instance (a restarted Agent) observes
// what the old one applied: identity, counters with their epochs and the
// rotation; re-applying the state, directly or through a re-plan, changes
// nothing and keeps established connections.
func TestAgentRestart(t *testing.T) {
	s := newStar(t, "t1")
	r := s.singleHop(model.L4ProtocolTCP)
	d := s.deploy(t, r)
	in := s.in(t, d)
	held, err := holdTCP(s.cli, in, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	mustEcho(t, s.cli, in, 64<<10)
	before := s.entry.observe(t)

	s.entry.drv = s.entry.newDriver(t)
	after := s.entry.observe(t)
	if !after.Applied || after.NodeRef != before.NodeRef || after.Generation != before.Generation ||
		after.StateHash != before.StateHash || after.Digest != before.Digest {
		t.Fatalf("restarted driver observes %+v, the old one %+v", after, before)
	}
	if len(after.Counters) != 1 || counted(t, before.Counters[0], after.Counters[0]) != (traffic{}) {
		t.Fatalf("counters after restart %v, before %v", after.Counters, before.Counters)
	}
	if len(after.Rotation) != 1 || !slices.Equal(after.Rotation[0].Active, before.Rotation[0].Active) {
		t.Fatalf("rotation after restart %v, before %v", after.Rotation, before.Rotation)
	}

	a, err := s.entry.drv.Render(d.states[entry1])
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.entry.drv.Apply(context.Background(), a)
	if err != nil || res.Changed || res.Generation != before.Generation {
		t.Fatalf("re-applying the running state after a restart: %+v, %v; want no change", res, err)
	}
	if again := s.deploy(t, r); again.results[entry1].Changed || again.results[entry1].Generation != before.Generation {
		t.Fatalf("re-planning unchanged routes: %+v; want the same generation, unchanged", again.results[entry1])
	}
	if err := held.ping(4096, 5*time.Second); err != nil {
		t.Fatalf("an established connection did not survive the restart: %v", err)
	}
	ex := mustEcho(t, s.cli, in, 64<<10)
	if got := counted(t, before.Counters[0], s.entry.counters(t, routeA, 0)); got.upBytes < uint64(ex.up) {
		t.Fatalf("after the restart: %v", got)
	}
}

// TestRemove: Remove deletes the driver's table and tc objects on the
// node, and nothing else: another table and a qdisc on another interface
// stay as they were. It is idempotent.
func TestRemove(t *testing.T) {
	s := newStar(t, "t1")
	ent := s.entry.ns
	ent.must(t, "ip", "link", "add", "frn0", "type", "dummy")
	ent.must(t, "ip", "link", "set", "frn0", "up")
	ent.must(t, "tc", "qdisc", "add", "dev", "frn0", "root", "handle", "1:", "htb")
	if _, errOut, err := ent.run(context.Background(), "nft", []string{"-f", "-"}, []byte(`table inet foreign_t {
	counter seen {
	}
	chain forward {
		type filter hook forward priority 10; policy accept;
		ct mark 0x5 counter name "seen" accept
	}
}
`)); err != nil {
		t.Fatalf("plant the foreign table: %v: %s", err, errOut)
	}
	foreign := func() string {
		return ent.must(t, "nft", "list", "table", "inet", "foreign_t") + ent.must(t, "tc", "qdisc", "show", "dev", "frn0")
	}
	planted := foreign()

	r := s.singleHop(model.L4ProtocolTCP)
	r.Limits = model.Limits{BandwidthBPS: 50_000_000, QuotaBytes: 1 << 30, MaxConns: 100}
	d := s.deploy(t, r)
	in := s.in(t, d)
	mustEcho(t, s.cli, in, 16<<10)
	owned := func() (table bool, qdiscs []string) {
		table = regexp.MustCompile(`(?m)^table inet anixops_fwd`).MatchString(ent.must(t, "nft", "list", "tables"))
		for _, dev := range []string{"to-cli", "to-t1"} {
			if regexp.MustCompile(`qdisc htb af00: `).MatchString(ent.must(t, "tc", "qdisc", "show", "dev", dev)) {
				qdiscs = append(qdiscs, dev)
			}
		}
		return table, qdiscs
	}
	if table, qdiscs := owned(); !table || len(qdiscs) != 2 {
		t.Fatalf("before Remove: table %v, driver qdiscs on %v; want the table and both interfaces", table, qdiscs)
	}

	for i := range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := s.entry.drv.Remove(ctx)
		cancel()
		if err != nil {
			t.Fatalf("Remove %d: %v", i+1, err)
		}
		if table, qdiscs := owned(); table || len(qdiscs) != 0 {
			t.Fatalf("after Remove %d: table %v, driver qdiscs on %v", i+1, table, qdiscs)
		}
	}
	if got := foreign(); got != planted {
		t.Fatalf("Remove changed foreign objects:\nbefore:\n%s\nafter:\n%s", planted, got)
	}
	if o := s.entry.observe(t); o.Applied {
		t.Fatalf("observed after Remove: %+v", o)
	}
	_ = mustNotConnect(t, s.cli, in, "the driver's objects are removed")
}

// TestStaleNamespaceCleanup: the cleanup of leftovers removes this suite's
// namespaces whose run died, with the processes inside, and keeps those
// of a live run and every other namespace.
func TestStaleNamespaceCleanup(t *testing.T) {
	requireE2E(t)
	// Above PID_MAX_LIMIT (4194304), so no process has it.
	dead := fmt.Sprintf("%s%d-0-stale", nsPrefix, 4194304+12345)
	live := nsName(labSeq.Add(1), "live")
	other := fmt.Sprintf("f2d-other-%d", labSeq.Add(1))
	for _, name := range []string{dead, live, other} {
		if _, err := ipCmd("netns", "add", name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if slices.Contains(listNetns(), name) {
				_ = delNetns(name)
			}
		})
	}
	sleeper := exec.Command("ip", "netns", "exec", dead, "sleep", "600") // #nosec G204 -- a generated namespace name
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- sleeper.Wait() }()

	removeStaleNamespaces(t.Logf)
	names := listNetns()
	if slices.Contains(names, dead) {
		t.Errorf("the dead run's namespace %s is still there", dead)
	}
	for _, name := range []string{live, other} {
		if !slices.Contains(names, name) {
			t.Errorf("namespace %s was removed", name)
		}
	}
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		_ = sleeper.Process.Kill()
		t.Error("the process inside the dead run's namespace was not killed")
	}
}
