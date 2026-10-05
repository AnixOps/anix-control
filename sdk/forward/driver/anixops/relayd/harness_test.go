package relayd_test

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayd"
	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

const testWait = 10 * time.Second

// freePort answers a port that is free for TCP and UDP on loopback.
func freePort(t testing.TB) uint32 {
	t.Helper()
	for range 50 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		pc, err := net.ListenPacket("udp", "127.0.0.1:"+strconv.Itoa(port))
		_ = l.Close()
		if err != nil {
			continue
		}
		_ = pc.Close()
		return uint32(port) // #nosec G115 -- a port
	}
	t.Fatal("no free port")
	return 0
}

// pki is the link CA of a test and the files of each node's credentials.
type pki struct {
	t   testing.TB
	ca  *relaytest.PKI
	dir string
}

func newPKI(t testing.TB) *pki {
	t.Helper()
	ca, err := relaytest.New("test")
	if err != nil {
		t.Fatal(err)
	}
	return &pki{t: t, ca: ca, dir: t.TempDir()}
}

// files writes the link files of node and answers them.
func (p *pki) files(node string) *relayctl.LinkFiles {
	p.t.Helper()
	cert, key, err := p.ca.Issue(relaytest.Cert{Node: node})
	if err != nil {
		p.t.Fatal(err)
	}
	f := &relayctl.LinkFiles{
		Cert: filepath.Join(p.dir, node+".crt"),
		Key:  filepath.Join(p.dir, node+".key"),
		CA:   filepath.Join(p.dir, "ca.crt"),
	}
	for path, data := range map[string][]byte{f.Cert: cert, f.Key: key, f.CA: p.ca.CAPEM()} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			p.t.Fatal(err)
		}
	}
	return f
}

func identity(node string) string { return relaytest.Identity("test", node) }

func relaytestCert(node string) relaytest.Cert { return relaytest.Cert{Node: node} }

// node is one relay with the link files of an identity name.
type node struct {
	t     testing.TB
	name  string
	relay *relayd.Relay
	link  *relayctl.LinkFiles
	cfg   relayctl.Config
}

func newNode(t testing.TB, p *pki, name string, opts relayd.Options) *node {
	t.Helper()
	n := &node{t: t, name: name, relay: relayd.New(opts), link: p.files(name)}
	t.Cleanup(n.relay.Close)
	n.cfg = relayctl.Config{Format: relayctl.Format, Owner: "test", Note: "test", Link: n.link}
	return n
}

// apply sends the node's configuration; the hops are sorted as the driver
// renders them.
func (n *node) apply(hops ...relayctl.Hop) (bool, error) {
	n.t.Helper()
	n.cfg.Hops = slices.Clone(hops)
	slices.SortFunc(n.cfg.Hops, func(a, b relayctl.Hop) int {
		if a.Route != b.Route {
			if a.Route < b.Route {
				return -1
			}
			return 1
		}
		return int(a.Hop) - int(b.Hop)
	})
	b, err := relayctl.Encode(&n.cfg)
	if err != nil {
		n.t.Fatal(err)
	}
	changed, _, err := n.relay.Apply(b)
	return changed, err
}

func (n *node) mustApply(hops ...relayctl.Hop) {
	n.t.Helper()
	if _, err := n.apply(hops...); err != nil {
		n.t.Fatalf("apply on %s: %v", n.name, err)
	}
}

// hopObs answers the observation of one hop.
func (n *node) hopObs(route string, hop uint32) relayctl.HopObs {
	n.t.Helper()
	for _, h := range n.relay.Observe(false).Hops {
		if h.Route == route && h.Hop == hop {
			return h
		}
	}
	n.t.Fatalf("%s has no hop %s/%d", n.name, route, hop)
	return relayctl.HopObs{}
}

const route = "01JF2A000000000000000000A1"

func rawHop(port uint32, tcp, udp bool, ups ...relayctl.Upstream) relayctl.Hop {
	return relayctl.Hop{
		Route: route, Hop: 0, Role: "entry",
		Listen:    relayctl.Listen{Address: "127.0.0.1", Port: port, TCP: tcp, UDP: udp},
		Ingress:   relayctl.Ingress{Security: relayctl.SecurityRaw},
		Upstreams: ups,
		Balance:   relayctl.BalanceRoundRobin,
		Breaker:   relayctl.Breaker{MaxFails: 3, OpenMs: 30000},
	}
}

// target is a raw upstream.
func target(addr string, port uint32) relayctl.Upstream {
	return relayctl.Upstream{Address: addr, Port: port, Weight: 1, Egress: relayctl.Egress{Security: relayctl.SecurityRaw}}
}

// next is an AnixOps upstream to the node serving the next hop.
func next(port uint32, carrier, nodeRef string) relayctl.Upstream {
	return relayctl.Upstream{
		Address: "127.0.0.1", Port: port, Weight: 1, NodeRef: nodeRef, PeerIdentity: identity(nodeRef),
		Egress: relayctl.Egress{Security: relayctl.SecurityAnixOps, Carrier: carrier, ServerName: nodeRef},
	}
}

// ingress is the hop that terminates an AnixOps link.
func ingress(hop uint32, port uint32, carrier, role string, peers []string, ups ...relayctl.Upstream) relayctl.Hop {
	h := relayctl.Hop{
		Route: route, Hop: hop, Role: role,
		Listen:    relayctl.Listen{Address: "127.0.0.1", Port: port},
		Ingress:   relayctl.Ingress{Security: relayctl.SecurityAnixOps, Carrier: carrier},
		Sources:   []string{"127.0.0.1"},
		Peers:     peers,
		Upstreams: ups,
		Balance:   relayctl.BalanceRoundRobin,
		Breaker:   relayctl.Breaker{MaxFails: 3, OpenMs: 30000},
	}
	switch carrier {
	case relayctl.CarrierAuto:
		h.Listen.TCP, h.Listen.UDP = true, true
	case relayctl.CarrierQUIC:
		h.Listen.UDP = true
	default:
		h.Listen.TCP = true
	}
	if carrier == relayctl.CarrierPlain {
		h.Peers = nil
	}
	return h
}

// echoTCP serves a TCP echo server and answers its address; it half-closes
// like a real server: after the client's EOF it finishes echoing and closes.
func echoTCP(t testing.TB) (string, uint32) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	a := l.Addr().(*net.TCPAddr)
	return a.IP.String(), uint32(a.Port) // #nosec G115 -- a port
}

// echoUDP serves a UDP echo server.
func echoUDP(t testing.TB) (string, uint32) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, a, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], a)
		}
	}()
	a := pc.LocalAddr().(*net.UDPAddr)
	return a.IP.String(), uint32(a.Port) // #nosec G115 -- a port
}

func dialTCP(t testing.TB, port uint32) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(port)), testWait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// roundTrip sends data and reads it back.
func roundTrip(t testing.TB, c net.Conn, data []byte) {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(testWait))
	errc := make(chan error, 1)
	go func() {
		_, err := c.Write(data)
		errc <- err
	}()
	got := make([]byte, len(data))
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatalf("read the echo: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatal("echo differs from what was sent")
	}
}

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i>>8)
	}
	return b
}

func waitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// chain is entry (raw) -> relay -> exit (raw to a target), as three nodes on
// loopback over one carrier.
type chain struct {
	entry, mid, exit *node
	entryPort        uint32
	target           string
	targetPort       uint32
}

func newChain(t testing.TB, carrier string, opts relayd.Options) *chain {
	t.Helper()
	p := newPKI(t)
	c := &chain{
		entry: newNode(t, p, "forward-1", opts),
		mid:   newNode(t, p, "forward-2", opts),
		exit:  newNode(t, p, "forward-3", opts),
	}
	c.target, c.targetPort = echoTCP(t)
	midPort, exitPort := freePort(t), freePort(t)
	c.entryPort = freePort(t)
	c.exit.mustApply(ingress(2, exitPort, carrier, "exit", []string{identity("forward-2")}, target(c.target, c.targetPort)))
	c.mid.mustApply(ingress(1, midPort, carrier, "relay", []string{identity("forward-1")}, next(exitPort, carrier, "forward-3")))
	c.entry.mustApply(rawHop(c.entryPort, true, true, next(midPort, carrier, "forward-2")))
	return c
}

func listenForeign(t testing.TB, port uint32) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(int(port)))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func dialRaw(port uint32) (net.Conn, error) {
	return net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(port)), testWait)
}

// echoCheck sends data, half-closes, and expects it back whole.
func echoCheck(c net.Conn, data []byte) error {
	_ = c.SetDeadline(time.Now().Add(testWait))
	errc := make(chan error, 1)
	go func() {
		if _, err := c.Write(data); err != nil {
			errc <- err
			return
		}
		errc <- c.(*net.TCPConn).CloseWrite()
	}()
	got, err := io.ReadAll(c)
	if err != nil {
		return err
	}
	if err := <-errc; err != nil {
		return err
	}
	if string(got) != string(data) {
		return fmt.Errorf("echo of %d bytes came back as %d different bytes", len(data), len(got))
	}
	return nil
}
