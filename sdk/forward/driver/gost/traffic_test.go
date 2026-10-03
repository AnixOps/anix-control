package gost_test

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
	"github.com/AnixOps/anix-control/sdk/forward/leastconn"
)

// envEcho turns the test binary into a TCP and UDP echo server on the
// address it names, which the netns tests start inside their namespaces;
// envEchoTag replaces its answers' prefix "echo" (to tell targets apart).
const (
	envEcho    = "ANIXOPS_GOST_ECHO"
	envEchoTag = "ANIXOPS_GOST_ECHO_TAG"
)

func TestMain(m *testing.M) {
	if addr := os.Getenv(envEcho); addr != "" {
		os.Exit(echo(addr))
	}
	os.Exit(m.Run())
}

// echo answers every TCP stream and UDP datagram with "echo:" (or the
// envEchoTag and ":") and what it got, until its standard input closes.
func echo(addr string) int {
	tag := "echo:"
	if t := os.Getenv(envEchoTag); t != "" {
		tag = t + ":"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if _, err := io.WriteString(c, tag+line); err != nil {
						return
					}
				}
			}()
		}
	}()
	go func() {
		buf := make([]byte, 64<<10)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(append([]byte(tag), buf[:n]...), from)
		}
	}()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

// do runs f on an OS thread that joined the namespace, so the sockets f
// creates belong to it. The thread is never unlocked: it ends with its
// goroutine.
func (n *netns) do(f func() error) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		fd, err := unix.Open(filepath.Join("/run/netns", n.name), unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			errc <- err
			return
		}
		defer func() { _ = unix.Close(fd) }()
		if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
			errc <- err
			return
		}
		errc <- f()
	}()
	return <-errc
}

func (n *netns) dial(network, addr string) (net.Conn, error) {
	var c net.Conn
	err := n.do(func() error {
		var err error
		c, err = net.DialTimeout(network, addr, 3*time.Second)
		return err
	})
	return c, err
}

// exchange writes a line and reads the echo.
func exchange(c net.Conn, msg string) (string, error) {
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(c, msg+"\n"); err != nil {
		return "", err
	}
	buf := make([]byte, 256)
	n, err := c.Read(buf)
	return strings.TrimSpace(string(buf[:n])), err
}

// TestNetnsTraffic moves TCP and UDP through a gost relay (RAW in, mutual
// TLS out) and a gost exit (TLS in, RAW to the target) in one namespace,
// checks that a hot change to the relay (its sources) keeps established
// connections, the UDP session and the counter epoch, that an apply that
// changes the relay's structure reloads gost without dropping an
// established TCP connection and starts a new counter epoch, that a
// paused hop refuses new connections, that the bandwidth and connection
// limits hold, and that the exit refuses a TLS client without a link
// certificate of its CA.
func TestNetnsTraffic(t *testing.T) {
	ns := newNetns(t)
	for _, a := range []string{"10.231.0.1", "10.231.0.2", "10.231.0.10"} {
		ns.addAddress(t, a)
	}
	pki := linkPKI(t, shortDir(t), "forward-31", "forward-41")
	relay, exit := newNode(t, ns, "forward-31", pki), newNode(t, ns, "forward-41", pki)

	startEcho(t, ns, "10.231.0.10:7000")

	const route = "01JF4A000000000000000000A1"
	both := forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP
	link := &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS, Mux: true, ServerName: "forward-41"}
	exitHop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 2, Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: gostE,
		Listen:  &forwardv1.Listen{Address: "10.231.0.2", Port: 20000, Protocol: both},
		Ingress: link,
		Upstreams: []*forwardv1.Upstream{{
			Address: "10.231.0.10", Port: 7000, Weight: 1,
			Egress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		}},
		Balance:        failover,
		TargetPolicy:   forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		IngressSources: []string{"10.231.0.0/24"},
		IngressPeers:   []string{"spiffe://anixops/example/agent/forward-31"},
		Mark:           1,
	}
	relayHop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 1, Role: forwardv1.HopRole_HOP_ROLE_RELAY, Engine: gostE,
		Listen:  &forwardv1.Listen{Address: "10.231.0.1", Port: 30001, Protocol: both},
		Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		Upstreams: []*forwardv1.Upstream{{
			Address: "10.231.0.2", Port: 20000, Weight: 1, Egress: link,
			NodeRef: "forward-41", PeerIdentity: "spiffe://anixops/example/agent/forward-41",
		}},
		Balance:        failover,
		TargetPolicy:   forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		IngressSources: []string{"10.231.0.0/24"},
		Mark:           1,
	}
	ed, rd := exit.driver(t), relay.driver(t)
	if _, err := ed.Apply(t.Context(), render(t, ed, conformance.State("forward-41", 1, exitHop))); err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Apply(t.Context(), render(t, rd, conformance.State("forward-31", 1, relayHop))); err != nil {
		t.Fatal(err)
	}

	tcpConn, err := ns.dial("tcp", "10.231.0.1:30001")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tcpConn.Close() }()
	if got, err := exchange(tcpConn, "one"); err != nil || got != "echo:one" {
		t.Fatalf("TCP through relay and exit: %q %v", got, err)
	}
	udpConn, err := ns.dial("udp", "10.231.0.1:30001")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = udpConn.Close() }()
	if got, err := exchange(udpConn, "dgram"); err != nil || got != "echo:dgram" {
		t.Fatalf("UDP through relay and exit: %q %v", got, err)
	}

	// A hot change (the relay's sources) goes through gost's API: the
	// established connection, the UDP session and the counter epoch stay,
	// and the counters grew with the traffic.
	before, err := rd.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if c := before.Counters[0]; c.GetUpBytes() < 10 || c.GetDownBytes() < 20 || c.GetTotalConns() != 1 || c.GetActiveConns() != 1 {
		t.Fatalf("relay counters after one TCP and one UDP exchange: %v", c)
	}
	sourced := proto.Clone(relayHop).(*forwardv1.NodeHop)
	sourced.IngressSources = []string{"10.231.0.0/24", "192.0.2.0/24"}
	if r, err := rd.Apply(t.Context(), render(t, rd, conformance.State("forward-31", 2, sourced))); err != nil || !r.Changed {
		t.Fatalf("hot apply: %+v %v", r, err)
	}
	if got, err := exchange(tcpConn, "hot"); err != nil || got != "echo:hot" {
		t.Fatalf("established TCP connection after a hot change: %q %v", got, err)
	}
	if got, err := exchange(udpConn, "hot"); err != nil || got != "echo:hot" {
		t.Fatalf("UDP session after a hot change: %q %v", got, err)
	}
	hot, err := rd.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if b, h := before.Counters[0], hot.Counters[0]; h.GetCounterEpoch() != b.GetCounterEpoch() || h.GetUpBytes() <= b.GetUpBytes() || h.GetDownBytes() <= b.GetDownBytes() {
		t.Fatalf("a hot change: counters %v -> %v", b, h)
	}

	// A structural change reloads gost: the established TCP connection
	// survives; every service is re-created, so the counters start a new
	// epoch.
	relay2 := conformance.State("forward-31", 3, sourced, &forwardv1.NodeHop{
		RouteId: "01JF4A000000000000000000B1", HopIndex: 0, Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: gostE,
		Listen:       &forwardv1.Listen{Address: "10.231.0.1", Port: 30002, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Upstreams:    []*forwardv1.Upstream{{Address: "10.231.0.10", Port: 7000}},
		TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
	})
	if r, err := rd.Apply(t.Context(), render(t, rd, relay2)); err != nil || !r.Changed {
		t.Fatalf("changing apply: %+v %v", r, err)
	}
	if got, err := exchange(tcpConn, "two"); err != nil || got != "echo:two" {
		t.Fatalf("established TCP connection after the reload: %q %v", got, err)
	}
	after, err := rd.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if b, a := hot.Counters[0].GetCounterEpoch(), after.Counters[0].GetCounterEpoch(); a == b {
		t.Fatalf("the reload kept the counter epoch %s although it re-created the services", a)
	}
	direct, err := ns.dial("tcp", "10.231.0.1:30002")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = direct.Close() }()
	if got, err := exchange(direct, "three"); err != nil || got != "echo:three" {
		t.Fatalf("the added hop: %q %v", got, err)
	}

	// A paused hop keeps its listener and refuses new connections.
	paused := proto.Clone(relayHop).(*forwardv1.NodeHop)
	paused.Paused = true
	if _, err := rd.Apply(t.Context(), render(t, rd, conformance.State("forward-31", 4, paused))); err != nil {
		t.Fatal(err)
	}
	if c, err := ns.dial("tcp", "10.231.0.1:30001"); err == nil {
		got, err := exchange(c, "four")
		_ = c.Close()
		if err == nil {
			t.Fatalf("a paused hop forwarded a new connection: %q", got)
		}
	}

	// Limits: 800 kbit/s (100 KB/s each way) and two connections.
	limited := &forwardv1.NodeHop{
		RouteId: "01JF4A000000000000000000C1", HopIndex: 0, Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: gostE,
		Listen:       &forwardv1.Listen{Address: "10.231.0.1", Port: 30003, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Upstreams:    []*forwardv1.Upstream{{Address: "10.231.0.10", Port: 7000}},
		TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		Limits:       &forwardv1.Limits{BandwidthBps: 800_000, MaxConns: 2},
	}
	if _, err := rd.Apply(t.Context(), render(t, rd, conformance.State("forward-31", 5, limited))); err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	for i := range 3 {
		c, err := ns.dial("tcp", "10.231.0.1:30003")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		conns = append(conns, c)
		if i < 2 {
			if got, err := exchange(c, "conn"); err != nil || got != "echo:conn" {
				t.Fatalf("connection %d under the limit: %q %v", i, got, err)
			}
		}
	}
	if got, err := exchange(conns[2], "conn"); err == nil {
		t.Fatalf("a third connection passed the limit of 2: %q", got)
	}
	_ = conns[1].Close()
	_ = conns[2].Close()
	big := strings.Repeat("x", 300_000)
	_ = conns[0].SetDeadline(time.Now().Add(30 * time.Second))
	start := time.Now()
	if _, err := io.WriteString(conns[0], big+"\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conns[0], make([]byte, len("echo:")+len(big)+1)); err != nil {
		t.Fatal(err)
	}
	// 300 KB up and 300 KB down at 100 KB/s, less the limiter's burst.
	if d := time.Since(start); d < 3*time.Second || d > 20*time.Second {
		t.Fatalf("600 KB through a 100 KB/s limit took %v", d)
	}

	// Mutual TLS: the exit refuses a client without a certificate.
	pool := x509.NewCertPool()
	ca, _ := os.ReadFile(pki["forward-41"][2]) // #nosec G304 -- the test's CA
	pool.AppendCertsFromPEM(ca)
	raw, err := ns.dial("tcp", "10.231.0.2:20000")
	if err != nil {
		t.Fatal(err)
	}
	c := tls.Client(raw, &tls.Config{ServerName: "forward-41", RootCAs: pool, MinVersion: tls.VersionTLS12})
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	err = c.Handshake()
	if err == nil {
		_, err = c.Read(make([]byte, 1))
	}
	if err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("the exit accepted a TLS client without a certificate: %v", err)
	}
}

// startEcho runs the test binary as an echo server on addr inside the
// namespace until the test ends; tag (when not empty) prefixes its
// answers instead of "echo".
func startEcho(t testing.TB, ns *netns, addr string, tag ...string) {
	t.Helper()
	srv := exec.Command(ns.ip, "netns", "exec", ns.name, os.Args[0], "-test.run=^$") // #nosec G204 -- the test binary as echo server
	srv.Env = append(os.Environ(), envEcho+"="+addr)
	if len(tag) > 0 {
		srv.Env = append(srv.Env, envEchoTag+"="+tag[0])
	}
	stdin, _ := srv.StdinPipe()
	out, _ := srv.StdoutPipe()
	srv.Stderr = os.Stderr
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = srv.Wait() })
	if line, _ := bufio.NewReader(out).ReadString('\n'); line != "ready\n" {
		t.Fatalf("echo server on %s: %q", addr, line)
	}
}

func render(t testing.TB, d *gost.Driver, s *forwardv1.NodeForwardState) driver.Artifact {
	t.Helper()
	a, err := d.Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return a
}

// TestNetnsHotChanges drives a real gost entry with two targets through the
// changes that go through gost's web API, with an established TCP
// connection and a UDP session open: SetUpstreams fails over and back
// (new connections follow it, the established ones stay on their target,
// the counter epoch stays), ActiveConns sees gost's sockets per target, the
// soft quota closes the hop once its counters reach it and opens it when
// the quota is raised, and a pause refuses new connections while an
// established one runs on (gost cannot close it; forward-sdk.md section
// 6.2).
func TestNetnsHotChanges(t *testing.T) {
	ns := newNetns(t)
	for _, a := range []string{"10.232.0.1", "10.232.0.10", "10.232.0.11"} {
		ns.addAddress(t, a)
	}
	startEcho(t, ns, "10.232.0.10:7000", "t1")
	startEcho(t, ns, "10.232.0.11:7000", "t2")
	n := newNode(t, ns, "forward-11", linkPKI(t, shortDir(t), "forward-11"))
	d := n.driver(t)

	const route = "01JF4B000000000000000000A1"
	hop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 0, Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: gostE,
		Listen: &forwardv1.Listen{Address: "10.232.0.1", Port: 30001, Protocol: both},
		Upstreams: []*forwardv1.Upstream{
			{Address: "10.232.0.10", Port: 7000, Priority: 0},
			{Address: "10.232.0.11", Port: 7000, Priority: 1},
		},
		Balance:      failover,
		TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		Mark:         1,
	}
	gen := uint64(0)
	applyHop := func(h *forwardv1.NodeHop) {
		t.Helper()
		gen++
		if _, err := d.Apply(t.Context(), render(t, d, conformance.State("forward-11", gen, h))); err != nil {
			t.Fatal(err)
		}
	}
	dial := func(network string) (net.Conn, error) { return ns.dial(network, "10.232.0.1:30001") }
	reaches := func(want string) {
		t.Helper()
		c, err := dial("tcp")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		if got, err := exchange(c, "new"); err != nil || got != want+":new" {
			t.Fatalf("a new connection answered %q %v, want %s", got, err, want)
		}
	}
	refused := func(why string) {
		t.Helper()
		c, err := dial("tcp")
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		if got, err := exchange(c, "new"); err == nil {
			t.Fatalf("%s: a new connection was forwarded: %q", why, got)
		}
	}
	counters := func() *forwardv1.Counters {
		t.Helper()
		o, err := d.Observe(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return o.Counters[0]
	}

	applyHop(hop)
	held, err := dial("tcp")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	session, err := dial("udp")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	for _, c := range []net.Conn{held, session} {
		if got, err := exchange(c, "first"); err != nil || got != "t1:first" {
			t.Fatalf("first exchange: %q %v", got, err)
		}
	}
	c0 := counters()
	epoch := c0.GetCounterEpoch()

	// Failover to t2 and back, as the Agent's health loop does.
	pid := n.sup.pid(t)
	if err := d.SetUpstreams(t.Context(), route, 0, []driver.Upstream{{Address: "10.232.0.11", Port: 7000}}); err != nil {
		t.Fatal(err)
	}
	reaches("t2")
	for _, c := range []net.Conn{held, session} {
		if got, err := exchange(c, "kept"); err != nil || got != "t1:kept" {
			t.Fatalf("an established flow after the failover: %q %v", got, err)
		}
	}
	conns, err := d.ActiveConns(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if conns[netip.MustParseAddrPort("10.232.0.10:7000")] < 1 {
		t.Fatalf("ActiveConns %v: no socket to t1, which the held connection uses", conns)
	}
	if err := d.SetUpstreams(t.Context(), route, 0, []driver.Upstream{{Address: "10.232.0.10", Port: 7000}, {Address: "10.232.0.11", Port: 7000}}); err != nil {
		t.Fatal(err)
	}
	reaches("t1")
	if n.sup.pid(t) != pid {
		t.Fatal("SetUpstreams restarted gost")
	}
	c1 := counters()
	// The two new connections may still be closing on gost's side.
	if c1.GetCounterEpoch() != epoch || c1.GetUpBytes() <= c0.GetUpBytes() || c1.GetTotalConns() != c0.GetTotalConns()+2 || c1.GetActiveConns() < 1 {
		t.Fatalf("counters across the failover: %v -> %v", c0, c1)
	}

	// The soft quota: the hop's bytes passed it, EnforceQuotas closes it.
	quota := proto.Clone(hop).(*forwardv1.NodeHop)
	quota.Limits = &forwardv1.Limits{QuotaBytes: c1.GetUpBytes() + c1.GetDownBytes()}
	applyHop(quota)
	if keys, err := d.EnforceQuotas(t.Context()); err != nil || len(keys) != 1 {
		t.Fatalf("EnforceQuotas: %v %v", keys, err)
	}
	refused("over the quota")
	quota.Limits.QuotaBytes *= 100
	applyHop(quota)
	if keys, err := d.EnforceQuotas(t.Context()); err != nil || len(keys) != 0 {
		t.Fatalf("EnforceQuotas after the quota was raised: %v %v", keys, err)
	}
	reaches("t1")

	// A pause refuses new connections; gost cannot close the established
	// one, which runs on.
	paused := proto.Clone(quota).(*forwardv1.NodeHop)
	paused.Paused = true
	applyHop(paused)
	refused("paused")
	if got, err := exchange(held, "paused"); err != nil || got != "t1:paused" {
		t.Fatalf("the established connection of a paused hop: %q %v", got, err)
	}
	if c := counters(); c.GetCounterEpoch() != epoch || n.sup.pid(t) != pid {
		t.Fatalf("hot changes ended the epoch or restarted gost: %v", c)
	}
}

// TestNetnsLeastConn re-weights a real gost LEAST_CONN hop from the
// connections ActiveConns sees (L1): three connections held on t1 make t2
// take most new connections, without a reload.
func TestNetnsLeastConn(t *testing.T) {
	ns := newNetns(t)
	for _, a := range []string{"10.233.0.1", "10.233.0.10", "10.233.0.11"} {
		ns.addAddress(t, a)
	}
	startEcho(t, ns, "10.233.0.10:7000", "t1")
	startEcho(t, ns, "10.233.0.11:7000", "t2")
	n := newNode(t, ns, "forward-11", linkPKI(t, shortDir(t), "forward-11"))
	d := n.driver(t)
	const route = "01JF4C000000000000000000A1"
	t1 := driver.Upstream{Address: "10.233.0.10", Port: 7000, Weight: 1}
	t2 := driver.Upstream{Address: "10.233.0.11", Port: 7000, Weight: 1}
	hop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 0, Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: gostE,
		Listen:       &forwardv1.Listen{Address: "10.233.0.1", Port: 30001, Protocol: tcp},
		Upstreams:    []*forwardv1.Upstream{{Address: t1.Address, Port: 7000, Weight: 1}, {Address: t2.Address, Port: 7000, Weight: 1}},
		Balance:      leastC,
		TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		Mark:         1,
	}
	if _, err := d.Apply(t.Context(), render(t, d, conformance.State("forward-11", 1, hop))); err != nil {
		t.Fatal(err)
	}
	hit := func() string {
		t.Helper()
		c, err := ns.dial("tcp", "10.233.0.1:30001")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		got, err := exchange(c, "x")
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSuffix(got, ":x")
	}
	// Three connections held on t1.
	if err := d.SetUpstreams(t.Context(), route, 0, []driver.Upstream{t1}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		c, err := ns.dial("tcp", "10.233.0.1:30001")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		if got, err := exchange(c, "held"); err != nil || got != "t1:held" {
			t.Fatalf("held connection: %q %v", got, err)
		}
	}
	if err := d.SetUpstreams(t.Context(), route, 0, []driver.Upstream{t1, t2}); err != nil {
		t.Fatal(err)
	}
	if conns, err := d.ActiveConns(t.Context()); err != nil || conns[netip.MustParseAddrPort("10.233.0.10:7000")] != 3 {
		t.Fatalf("ActiveConns %v %v, want 3 on t1", conns, err)
	}

	rotation := func() []driver.Upstream {
		t.Helper()
		o, err := d.Observe(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return o.Rotation[0].Active
	}
	pid := n.sup.pid(t)
	rw := leastconn.Reweighter{Source: d, Setter: d}
	lh := leastconn.Hop{RouteID: route, Upstreams: []driver.Upstream{t1, t2}, Rotation: rotation()}
	if err := rw.Tick(t.Context(), []leastconn.Hop{lh}); err != nil {
		t.Fatal(err)
	}
	got := rotation()
	if len(got) != 2 || got[0].Weight != 25 || got[1].Weight != 100 {
		t.Fatalf("rotation after re-weighting %+v, want t1 25 and t2 100", got)
	}
	// 1:4 expects 20 of 100 on t1; even weights would put 50 there. More
	// than 33 is 3.4 standard deviations off the first and 3.3 below the
	// second.
	count := map[string]int{}
	for range 100 {
		count[hit()]++
	}
	if count["t1"] > 33 {
		t.Fatalf("100 new connections after re-weighting: %v", count)
	}
	if n.sup.pid(t) != pid {
		t.Fatal("re-weighting restarted gost")
	}
}
