//go:build linux

package e2e

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
	"github.com/AnixOps/anix-control/sdk/forward/driver/nftables"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// Addressing. Every link is a veth pair; the side that is created first
// gets the first host address of the link's prefixes, the other the second.
// The client's link is a /24 so the client can hold many source addresses
// (IP hash); the others are /28s of TEST-NET-3 and /64s of the
// documentation prefix. Targets live on documentation addresses, which the
// default target policy (PUBLIC_ONLY) allows, as v4.1 did.
var (
	clientNet4 = netip.MustParsePrefix("198.51.100.0/24")
	clientNet6 = netip.MustParsePrefix("2001:db8:100::/64")
)

// linkNet answers the prefixes of the k-th link that is not the client's.
func linkNet(k int) (netip.Prefix, netip.Prefix) {
	if k < 0 || k > 15 {
		panic("forward e2e: at most 16 links per lab")
	}
	v4 := netip.AddrFrom4([4]byte{203, 0, 113, byte(16 * k)})
	v6 := netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, byte(k + 1)})
	return netip.PrefixFrom(v4, 28), netip.PrefixFrom(v6, 64)
}

// host answers the i-th address of a prefix (1 is the first host).
func host(p netip.Prefix, i int) netip.Addr {
	a := p.Addr()
	for range i {
		a = a.Next()
	}
	return a
}

// Ports: nodes allocate from nodePorts, targets listen on targetPort.
var nodePorts = &forwardv1.PortRange{First: 20000, Last: 20999}

const targetPort = 7000

// lab is one test's network: its namespaces, nodes, echo servers and the
// control plane state that a re-plan carries over.
type lab struct {
	t       *testing.T
	seq     int64
	order   []*netns
	nodes   []*node
	servers []*server
	links   int

	// What Control keeps between plans: allocations and generations.
	alloc planner.Allocations
	gens  map[string]planner.Generation
}

var labSeq atomic.Int64

func newLab(t *testing.T) *lab {
	t.Helper()
	requireE2E(t)
	l := &lab{t: t, seq: labSeq.Add(1)}
	t.Cleanup(l.close)
	return l
}

// close dumps the namespaces' state when the test failed, stops the echo
// servers and deletes the namespaces. It runs even when the test fails.
func (l *lab) close() {
	if l.t.Failed() {
		l.dump()
	}
	for _, s := range l.servers {
		s.kill()
	}
	for i := len(l.order) - 1; i >= 0; i-- {
		if err := delNetns(l.order[i].name); err != nil {
			l.t.Errorf("cleanup: %v", err)
		}
	}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// dump writes the state of every namespace and the servers' logs to
// ANIXOPS_FORWARD_E2E_LOGDIR/<test>/, or to the test log.
func (l *lab) dump() {
	dir := os.Getenv(envLogDir)
	if dir != "" {
		dir = filepath.Join(dir, unsafeName.ReplaceAllString(l.t.Name(), "_"))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			l.t.Logf("dump: %v", err)
			dir = ""
		}
	}
	emit := func(name, content string) {
		if dir == "" {
			l.t.Logf("=== %s\n%s", name, content)
			return
		}
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(content), 0o600); err != nil {
			l.t.Logf("dump %s: %v", name, err)
		}
	}
	for _, n := range l.order {
		emit("ns-"+n.role, n.name+"\n"+n.state())
	}
	for _, s := range l.servers {
		emit("server-"+s.spec.ID, s.stderr.String())
	}
	if dir != "" {
		l.t.Logf("namespace state written to %s", dir)
	}
}

// netns creates a namespace for a role (cli, ent, rel, t1, ...).
func (l *lab) netns(role string) *netns {
	l.t.Helper()
	n, err := addNetns(nsName(l.seq, role), role)
	if err != nil {
		l.t.Fatal(err)
	}
	l.order = append(l.order, n)
	return n
}

// link is a veth pair between two namespaces, named after the peer's role
// on each side ("to-t1" in the entry, "to-ent" in t1).
type link struct {
	aIf, bIf       string
	a4, b4, a6, b6 netip.Addr
	p4             netip.Prefix
}

// connect joins a and b with a veth pair addressed from p4 and p6.
func (l *lab) connect(a, b *netns, p4, p6 netip.Prefix) *link {
	l.t.Helper()
	k := &link{
		aIf: "to-" + b.role, bIf: "to-" + a.role,
		a4: host(p4, 1), b4: host(p4, 2), a6: host(p6, 1), b6: host(p6, 2), p4: p4,
	}
	a.must(l.t, "ip", "link", "add", k.aIf, "type", "veth", "peer", "name", k.bIf, "netns", b.name)
	for _, side := range []struct {
		ns     *netns
		dev    string
		v4, v6 netip.Addr
	}{{a, k.aIf, k.a4, k.a6}, {b, k.bIf, k.b4, k.b6}} {
		side.ns.must(l.t, "ip", "addr", "add", netip.PrefixFrom(side.v4, p4.Bits()).String(), "dev", side.dev)
		side.ns.must(l.t, "ip", "-6", "addr", "add", netip.PrefixFrom(side.v6, p6.Bits()).String(), "dev", side.dev, "nodad")
		side.ns.must(l.t, "ip", "link", "set", side.dev, "up")
	}
	return k
}

// connectNext joins a and b on the next free link prefixes.
func (l *lab) connectNext(a, b *netns) *link {
	p4, p6 := linkNet(l.links)
	l.links++
	return l.connect(a, b, p4, p6)
}

// serve starts an echo server in ns answering as id on targetPort of
// every address given, TCP and UDP.
func (l *lab) serve(ns *netns, id string, addrs ...netip.Addr) *server {
	l.t.Helper()
	s := &server{ns: ns, spec: serveSpec{ID: id}}
	for _, a := range addrs {
		ap := netip.AddrPortFrom(a, targetPort).String()
		s.spec.TCP = append(s.spec.TCP, ap)
		s.spec.UDP = append(s.spec.UDP, ap)
	}
	l.servers = append(l.servers, s)
	s.start(l.t)
	return s
}

// node is a forwarding node: a namespace with forwarding on and a driver,
// probed there as the Agent probes its host: the nftables driver (node),
// or the gost driver with gost running in the namespace (gostNode).
type node struct {
	ref   string
	ns    *netns
	addrs []netip.Addr
	cfg   nftables.Config // the nftables driver's probed configuration
	drv   driver.Driver
	// mk makes a driver instance on the node.
	mk func(testing.TB) driver.Driver
	// gost supervises a gost node's gost; nil on an nftables node.
	gost *gost.ProcessSupervisor
}

// node makes ns a forwarding node. addrs are its NodeInfo addresses: the
// first is where previous hops dial it, and all of them are what the next
// hop admits. limitIfs are the interfaces tc limits bandwidth on.
func (l *lab) node(ref string, ns *netns, limitIfs []string, addrs ...netip.Addr) *node {
	l.t.Helper()
	ns.must(l.t, "sysctl", "-qw", "net.ipv4.ip_forward=1", "net.ipv6.conf.all.forwarding=1")
	base := nftables.DefaultConfig()
	base.LimitInterfaces = limitIfs
	cfg, rep, err := nftables.Probe(context.Background(), nsRunner{ns}, base)
	if err != nil {
		l.t.Fatal(err)
	}
	if cfg.Version == "" {
		l.t.Fatalf("%s: nftables driver unavailable: %s (missing %v)", ref, cfg.Unavailable, rep.Missing)
	}
	for _, m := range rep.Missing {
		l.t.Logf("%s: probe: missing %s", ref, m)
	}
	for _, w := range rep.Warnings {
		l.t.Logf("%s: probe: %s", ref, w)
	}
	n := &node{ref: ref, ns: ns, addrs: addrs, cfg: cfg}
	n.mk = func(t testing.TB) driver.Driver {
		t.Helper()
		d, err := nftables.New(n.cfg, nftables.WithRunner(nsRunner{n.ns}))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	n.drv = n.newDriver(l.t)
	l.nodes = append(l.nodes, n)
	return n
}

// newDriver answers a new driver instance on the node, as a restarted
// Agent makes one from the same probed configuration.
func (n *node) newDriver(t testing.TB) driver.Driver {
	t.Helper()
	return n.mk(t)
}

// inventory answers the nodes as Control knows them: addresses, port range
// and the capabilities their drivers report.
func (l *lab) inventory(t testing.TB) []*forwardv1.NodeInfo {
	t.Helper()
	var out []*forwardv1.NodeInfo
	for _, n := range l.nodes {
		caps, err := n.drv.Capabilities(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		info := &forwardv1.NodeInfo{NodeRef: n.ref, PortRange: nodePorts, Engines: []*forwardv1.EngineCapabilities{caps}}
		for _, a := range n.addrs {
			info.Addresses = append(info.Addresses, a.String())
		}
		out = append(out, info)
	}
	return out
}

// deployment is one plan as every node applied it.
type deployment struct {
	states  map[string]*forwardv1.NodeForwardState
	results map[string]driver.ApplyResult
}

// deploy does what Control and the Agents will do with routes: validate
// each against the inventory, plan them all (keeping the previous
// allocations), stamp generations, and on every node render its state and
// apply it with the node's driver. Any violation, rejected hop or failed
// apply fails the test.
func (l *lab) deploy(t testing.TB, routes ...model.Route) deployment {
	t.Helper()
	inv := l.inventory(t)
	vopts := validate.Options{Nodes: model.NodesFromProto(inv)}
	protos := make([]*forwardv1.Route, 0, len(routes))
	for i := range routes {
		if vs := validate.Route(&routes[i], vopts); len(vs) > 0 {
			t.Fatalf("route %s refused by validation: %v", routes[i].ID, vs)
		}
		protos = append(protos, routes[i].ToProto())
	}
	// The cluster names the Agent identities pinned on encrypted links.
	res, err := planner.Plan(protos, inv, l.alloc, planner.Options{Cluster: "e2e"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(res.Violations) > 0 {
		t.Fatalf("plan refused: %v", res.Violations)
	}
	for _, w := range res.Warnings {
		t.Logf("planner: %s", w)
	}
	l.alloc = res.Allocations
	l.gens = planner.Stamp(res.States, l.gens)
	dep := deployment{states: res.States, results: map[string]driver.ApplyResult{}}
	for _, n := range l.nodes {
		s := res.States[n.ref]
		a, err := n.drv.Render(s)
		if err != nil {
			t.Fatalf("%s: render generation %d: %v", n.ref, s.GetGeneration(), err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		r, err := n.drv.Apply(ctx, a)
		cancel()
		if err != nil {
			t.Fatalf("%s: apply generation %d: %v", n.ref, s.GetGeneration(), err)
		}
		t.Logf("%s: generation %d applied (changed %v, %d hops)", n.ref, r.Generation, r.Changed, len(a.Hops))
		dep.results[n.ref] = r
	}
	return dep
}

// hop answers the planned hop of a route on a node.
func (d deployment) hop(t testing.TB, ref, routeID string, index uint32) *forwardv1.NodeHop {
	t.Helper()
	for _, h := range d.states[ref].GetHops() {
		if h.GetRouteId() == routeID && h.GetHopIndex() == index {
			return h
		}
	}
	t.Fatalf("no hop %s/%d on %s", routeID, index, ref)
	return nil
}

// listen answers where a hop listens on an address of its node.
func (d deployment) listen(t testing.TB, ref, routeID string, index uint32, addr netip.Addr) netip.AddrPort {
	t.Helper()
	return netip.AddrPortFrom(addr, uint16(d.hop(t, ref, routeID, index).GetListen().GetPort())) // #nosec G115 -- a planned port is at most 65535
}

// observe answers a node's observation.
func (n *node) observe(t testing.TB) driver.Observation {
	t.Helper()
	o, err := n.drv.Observe(context.Background())
	if err != nil {
		t.Fatalf("%s: observe: %v", n.ref, err)
	}
	return o
}

// counters answers a hop's counters on the node.
func (n *node) counters(t testing.TB, routeID string, index uint32) *forwardv1.Counters {
	t.Helper()
	for _, c := range n.observe(t).Counters {
		if c.GetRouteId() == routeID && c.GetHopIndex() == index {
			return c
		}
	}
	t.Fatalf("%s: no counters for %s/%d", n.ref, routeID, index)
	return nil
}

// healthy is the Agent's health check of a hop's upstreams, run from the
// node (forward-sdk.md section 7.3): the upstreams that accept a TCP
// connection, as SetUpstreams takes them.
func (n *node) healthy(hop *forwardv1.NodeHop) []driver.Upstream {
	var out []driver.Upstream
	for _, u := range hop.GetUpstreams() {
		ap := netip.AddrPortFrom(netip.MustParseAddr(u.GetAddress()), uint16(u.GetPort())) // #nosec G115 -- a planned port is at most 65535
		c, err := n.ns.dialTCP(netip.Addr{}, ap, time.Second)
		if err != nil {
			continue
		}
		_ = c.Close()
		out = append(out, driver.Upstream{Address: u.GetAddress(), Port: u.GetPort()})
	}
	return out
}

// Routes.

func route(id string, protocol model.L4Protocol, hops []model.Hop, targets ...model.Target) model.Route {
	return model.Route{
		ID: id, Owner: model.OwnerAdmin, Name: "f2d e2e " + id,
		Listen: model.Listen{Protocol: protocol}, Hops: hops, Targets: targets,
	}
}

func entryHop(refs ...string) model.Hop {
	return model.Hop{Role: model.HopRoleEntry, Engine: model.EngineNFTables, NodeRefs: refs}
}

func exitHop(refs ...string) model.Hop {
	return model.Hop{
		Role: model.HopRoleExit, Engine: model.EngineNFTables, NodeRefs: refs,
		Ingress: model.LinkTransport{Security: model.LinkSecurityRaw},
	}
}

func relayHop(refs ...string) model.Hop {
	h := exitHop(refs...)
	h.Role = model.HopRoleRelay
	return h
}

func target(a netip.Addr) model.Target {
	return model.Target{Host: a.String(), Port: targetPort}
}

// Counters.

// traffic is what a hop counted between two observations.
type traffic struct {
	upBytes, downBytes, upPackets, downPackets uint64
}

func (d traffic) String() string {
	return fmt.Sprintf("up %d bytes in %d packets, down %d bytes in %d packets", d.upBytes, d.upPackets, d.downBytes, d.downPackets)
}

// counted answers the difference of two counter readings of one hop, which
// must be in the same epoch.
func counted(t testing.TB, before, after *forwardv1.Counters) traffic {
	t.Helper()
	if before.GetCounterEpoch() != after.GetCounterEpoch() {
		t.Fatalf("%s/%d: counter epoch changed from %s to %s", after.GetRouteId(), after.GetHopIndex(), before.GetCounterEpoch(), after.GetCounterEpoch())
	}
	d := traffic{
		upBytes: after.GetUpBytes() - before.GetUpBytes(), downBytes: after.GetDownBytes() - before.GetDownBytes(),
		upPackets: after.GetUpPackets() - before.GetUpPackets(), downPackets: after.GetDownPackets() - before.GetDownPackets(),
	}
	if after.GetUpBytes() < before.GetUpBytes() || after.GetDownBytes() < before.GetDownBytes() ||
		after.GetUpPackets() < before.GetUpPackets() || after.GetDownPackets() < before.GetDownPackets() {
		t.Fatalf("%s/%d: counters decreased within epoch %s: %v then %v", after.GetRouteId(), after.GetHopIndex(), after.GetCounterEpoch(), before, after)
	}
	return d
}

// Header sizes. The counters count IP packets, headers included, as the
// forward hook sees them: a TCP segment carries a 20-byte TCP header plus
// up to 40 bytes of options, behind a 20-byte IPv4 or 40-byte IPv6 header;
// veth hands over TSO segments whole, so a counted packet may carry many
// MSS of payload but still one set of headers. A UDP datagram adds exactly
// 8 bytes to its IP header.
func headers(v6 bool) (tcpMin, tcpMax, udp uint64) {
	ip := uint64(20)
	if v6 {
		ip = 40
	}
	return ip + 20, ip + 60, ip + 8
}

// checkTCP checks counted TCP traffic against the payload the client moved:
// the payload must fit in the counted bytes after the smallest headers
// (nothing was missed), and the counted bytes after the largest headers
// must not exceed the payload by more than retransmissions could (nothing
// was counted twice or in the wrong direction).
func checkTCP(t testing.TB, what string, d traffic, up, down int, v6 bool) {
	t.Helper()
	tcpMin, tcpMax, _ := headers(v6)
	for _, dir := range []struct {
		name           string
		bytes, packets uint64
		payload        int
	}{{"up", d.upBytes, d.upPackets, up}, {"down", d.downBytes, d.downPackets, down}} {
		p := uint64(dir.payload) // #nosec G115 -- a test payload size
		slack := p/10 + 16<<10
		switch {
		case dir.packets == 0:
			t.Errorf("%s: no %s packet counted (%v)", what, dir.name, d)
		case dir.bytes < dir.packets*tcpMin || dir.bytes-dir.packets*tcpMin < p:
			t.Errorf("%s: %s counted %d bytes in %d packets, less than the %d payload bytes plus headers (%v)", what, dir.name, dir.bytes, dir.packets, p, d)
		case dir.bytes > dir.packets*tcpMax && dir.bytes-dir.packets*tcpMax > p+slack:
			t.Errorf("%s: %s counted %d bytes in %d packets, more than the %d payload bytes plus headers and %d bytes of slack (%v)", what, dir.name, dir.bytes, dir.packets, p, slack, d)
		}
	}
}

// checkUDP checks counted UDP traffic exactly: one packet per datagram,
// payload plus IP and UDP headers.
func checkUDP(t testing.TB, what string, d traffic, ex exchange, v6 bool) {
	t.Helper()
	_, _, hdr := headers(v6)
	n := uint64(ex.datagrams) // #nosec G115 -- a test count
	wantUp := uint64(ex.up) + n*hdr
	wantDown := uint64(ex.down) + n*hdr
	if d.upPackets != n || d.downPackets != n || d.upBytes != wantUp || d.downBytes != wantDown {
		t.Errorf("%s: counted %v; want %d packets each way, up %d bytes, down %d bytes", what, d, n, wantUp, wantDown)
	}
}

// Helpers for expectations.

func mustEcho(t testing.TB, from *netns, to netip.AddrPort, size int) exchange {
	t.Helper()
	ex, err := tcpEcho(from, netip.Addr{}, to, size, 10*time.Second)
	if err != nil {
		t.Fatalf("TCP exchange with %s: %v", to, err)
	}
	return ex
}

// mustNotConnect checks that a new connection fails (dropped or refused).
func mustNotConnect(t testing.TB, from *netns, to netip.AddrPort, why string) error {
	t.Helper()
	ex, err := tcpEcho(from, netip.Addr{}, to, 64, 1500*time.Millisecond)
	if err == nil {
		t.Fatalf("%s: a connection to %s reached %s", why, to, ex.target)
	}
	return err
}

func addrStrings(as []driver.Upstream) string {
	var s []string
	for _, a := range as {
		s = append(s, fmt.Sprintf("%s:%d", a.Address, a.Port))
	}
	return strings.Join(s, ",")
}
