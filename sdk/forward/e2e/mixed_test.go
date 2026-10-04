//go:build linux

package e2e

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// Mixed-engine chains (F4c): nftables and gost hops in one route, each
// node applying its own engine's part of the plan.

const (
	routeB = "01JF2D0000000000000000000B"
	exit4  = "forward-4"
)

// linkCerts writes a test link CA and, per node name, a link certificate
// as the gost driver needs it (forward-sdk.md section 6.2): the name as
// DNS name, server and client auth, signed by the CA. It answers the
// certificate, key and CA paths per node.
func linkCerts(t testing.TB, nodes ...string) map[string]*[3]string {
	t.Helper()
	dir, err := os.MkdirTemp("", "afe2e-pki")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	write := func(path, typ string, der []byte) {
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "forward e2e link CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(dir, "ca.crt")
	write(caPath, "CERTIFICATE", caDER)
	out := map[string]*[3]string{}
	for i, n := range nodes {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse("spiffe://anixops/e2e/agent/" + n)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(int64(i) + 2), Subject: pkix.Name{CommonName: n},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
			DNSNames: []string{n}, URIs: []*url.URL{u},
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		kder, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		crt, kf := filepath.Join(dir, n+".crt"), filepath.Join(dir, n+".key")
		write(crt, "CERTIFICATE", der)
		write(kf, "EC PRIVATE KEY", kder)
		out[n] = &[3]string{crt, kf, caPath}
	}
	return out
}

// udpPing sends one datagram of size bytes on a connected UDP socket (one
// session through every hop) and answers which target echoed it.
func udpPing(c *net.UDPConn, size int, timeout time.Duration) (exchange, error) {
	data := payload(size)
	if _, err := c.Write(data); err != nil {
		return exchange{}, err
	}
	if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return exchange{}, err
	}
	buf := make([]byte, 64<<10)
	n, err := c.Read(buf)
	if err != nil {
		return exchange{}, err
	}
	id, body, ok := bytes.Cut(buf[:n], []byte("\n"))
	if !ok || !bytes.Equal(body, data) {
		return exchange{}, fmt.Errorf("answer of %d bytes is not the echo", n)
	}
	return exchange{target: string(id), up: size, down: n, datagrams: 1}, nil
}

// pingEcho connects, reads the greeting, sends size bytes and reads them
// back on the open connection, then closes it: an exchange that needs no
// half-close, which gost carries over an encrypted or multiplexed link as
// a full close (a mux stream cannot half-close), cutting the answer short.
func pingEcho(t testing.TB, from *netns, to netip.AddrPort, size int) exchange {
	t.Helper()
	h, err := holdTCP(from, to, 10*time.Second)
	if err != nil {
		t.Fatalf("TCP connection to %s: %v", to, err)
	}
	defer func() { _ = h.Close() }()
	if err := h.ping(size, 10*time.Second); err != nil {
		t.Fatalf("TCP exchange with %s: %v", to, err)
	}
	return exchange{target: h.target, up: size, down: len(h.target) + 1 + size}
}

// instance answers the identity of a gost node's running gost.
func (n *node) instance(t testing.TB) string {
	t.Helper()
	st, err := n.gost.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running {
		t.Fatalf("%s: gost is not running", n.ref)
	}
	return st.Instance
}

// checkCarrier checks the counters of a gost hop whose ingress is an
// encrypted or multiplexed link: they count the carrier's bytes, so at
// least the payload, and no packets.
func checkCarrier(t testing.TB, what string, d traffic, up, down int) {
	t.Helper()
	if d.upBytes < uint64(up) || d.downBytes < uint64(down) || d.upPackets != 0 || d.downPackets != 0 { // #nosec G115 -- test sizes
		t.Errorf("%s: counted %v; want at least up %d and down %d bytes, no packets", what, d, up, down)
	}
}

// TestMixedChain: an nftables entry, a gost relay and two gost exit nodes
// (failover between them) before one target, over a RAW link and over a
// mutual-TLS mux link between relay and exits (certificates of a test
// CA). TCP and UDP reach the target; every hop counts the traffic (the
// nftables entry packets and headers, the gost relay the payload exactly,
// the gost exit the payload over RAW and the carrier over TLS); adding
// and removing another route on the gost relay leaves the route's
// established TCP connection, its UDP session, its counter epoch and the
// gost process alone; and when the primary exit's gost dies, the relay
// fails over to the other exit through SetUpstreams.
func TestMixedChain(t *testing.T) {
	for _, c := range []struct {
		name string
		link model.LinkTransport
	}{
		{"raw", model.LinkTransport{Security: model.LinkSecurityRaw}},
		{"tls-mux", model.LinkTransport{Security: model.LinkSecurityTLS, Mux: true}},
	} {
		t.Run(c.name, func(t *testing.T) { mixedChain(t, c.link) })
	}
}

func mixedChain(t *testing.T, link model.LinkTransport) {
	l := newLab(t)
	cli, ent, rel, x1, x2, t1 := l.netns("cli"), l.netns("ent"), l.netns("rel"), l.netns("x1"), l.netns("x2"), l.netns("t1")
	cl := l.connect(ent, cli, clientNet4, clientNet6)
	er := l.connectNext(ent, rel)
	r1 := l.connectNext(rel, x1)
	r2 := l.connectNext(rel, x2)
	x1t := l.connectNext(x1, t1)
	x2t := l.connectNext(x2, t1)
	// The target answers on its address on the first exit's link; the
	// second exit reaches it through its own link.
	tgt := x1t.b4
	x2.must(t, "ip", "route", "add", netip.PrefixFrom(tgt, 32).String(), "via", x2t.b4.String())
	l.serve(t1, "t1", tgt)
	// The relay's neighbour on the entry's side answers route B.
	l.serve(ent, "ent", er.a4)

	var certs map[string]*[3]string
	if link.Security != model.LinkSecurityRaw {
		certs = linkCerts(t, relay2, exit3, exit4)
	}
	entry := l.node(entry1, ent, nil, er.a4, cl.a4)
	relay := l.gostNodeWith(relay2, rel, certs[relay2], er.b4, r1.a4, r2.a4)
	exitA := l.gostNodeWith(exit3, x1, certs[exit3], r1.b4, x1t.a4)
	exitB := l.gostNodeWith(exit4, x2, certs[exit4], r2.b4, x2t.a4)

	gostHop := func(role model.HopRole, in model.LinkTransport, refs ...string) model.Hop {
		return model.Hop{Role: role, Engine: model.EngineGost, NodeRefs: refs, Ingress: in}
	}
	ra := route(routeA, model.L4ProtocolTCPUDP, []model.Hop{
		entryHop(entry1),
		gostHop(model.HopRoleRelay, model.LinkTransport{Security: model.LinkSecurityRaw}, relay2),
		gostHop(model.HopRoleExit, link, exit3, exit4),
	}, target(tgt))
	ra.Policy.NextHop = model.BalanceFailover
	rb := route(routeB, model.L4ProtocolTCPUDP, []model.Hop{gostHop(model.HopRoleEntry, model.LinkTransport{}, relay2)}, target(er.a4))

	d := l.deploy(t, ra)
	in := d.listen(t, entry1, routeA, 0, cl.a4)
	relayHop := d.hop(t, relay2, routeA, 1)
	if ups := relayHop.GetUpstreams(); len(ups) != 2 || ups[0].GetAddress() != r1.b4.String() || ups[1].GetAddress() != r2.b4.String() {
		t.Fatalf("relay upstreams %v, want the exits at %s and %s", ups, r1.b4, r2.b4)
	}
	checkExit, echo := checkPayload, mustEcho
	if link.Security != model.LinkSecurityRaw {
		checkExit, echo = checkCarrier, pingEcho
	}
	type reading struct{ entry, relay, exitA, exitB *forwardv1.Counters }
	read := func() reading {
		return reading{entry.counters(t, routeA, 0), relay.counters(t, routeA, 1), exitA.counters(t, routeA, 2), exitB.counters(t, routeA, 2)}
	}

	t.Run("tcp", func(t *testing.T) {
		before := read()
		ex := echo(t, cli, in, 128<<10)
		if ex.target != "t1" {
			t.Fatalf("answered by %q", ex.target)
		}
		after := read()
		checkTCP(t, "nftables entry", counted(t, before.entry, after.entry), ex.up, ex.down, false)
		checkPayload(t, "gost relay", counted(t, before.relay, after.relay), ex.up, ex.down)
		checkExit(t, "gost exit", counted(t, before.exitA, after.exitA), ex.up, ex.down)
		if got := counted(t, before.exitB, after.exitB); got != (traffic{}) {
			t.Errorf("the backup exit counted %v", got)
		}
	})

	t.Run("udp", func(t *testing.T) {
		before := read()
		ex, err := udpEcho(cli, in, []int{100, 1200, 333, 64, 900}, 3*time.Second)
		if err != nil || ex.target != "t1" {
			t.Fatalf("UDP exchange with %s: %v (answered by %q)", in, err, ex.target)
		}
		after := read()
		checkUDP(t, "nftables entry", counted(t, before.entry, after.entry), ex, false)
		checkPayload(t, "gost relay", counted(t, before.relay, after.relay), ex.up, ex.down)
		checkExit(t, "gost exit", counted(t, before.exitA, after.exitA), ex.up, ex.down)
	})

	// The regression F4c fixes: changing the gost relay's routes used to
	// reload gost, re-creating every service (UDP sessions and mux
	// carriers restarted, every counter epoch ended).
	t.Run("unrelated-route", func(t *testing.T) {
		held, err := holdTCP(cli, in, 3*time.Second)
		if err != nil || held.target != "t1" {
			t.Fatalf("held connection: %v (%v)", held, err)
		}
		defer func() { _ = held.Close() }()
		session, err := cli.dialUDP(in)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = session.Close() }()
		if ex, err := udpPing(session, 200, 3*time.Second); err != nil || ex.target != "t1" {
			t.Fatalf("UDP session: %v (%q)", err, ex.target)
		}
		relayGost, exitGost := relay.instance(t), exitA.instance(t)
		before := read()
		up, down := 0, 0
		flows := func(why string) {
			t.Helper()
			if err := held.ping(1000, 3*time.Second); err != nil {
				t.Fatalf("%s: the established TCP connection: %v", why, err)
			}
			ex, err := udpPing(session, 300, 3*time.Second)
			if err != nil || ex.target != "t1" {
				t.Fatalf("%s: the UDP session: %v (%q)", why, err, ex.target)
			}
			up, down = up+1000+ex.up, down+1000+ex.down
			after := read()
			checkPayload(t, why+": gost relay", counted(t, before.relay, after.relay), up, down)
			checkExit(t, why+": gost exit", counted(t, before.exitA, after.exitA), up, down)
			if relay.instance(t) != relayGost || exitA.instance(t) != exitGost {
				t.Fatalf("%s: gost was restarted", why)
			}
		}
		flows("before")

		d2 := l.deploy(t, ra, rb)
		if r := d2.results[relay2]; !r.Changed {
			t.Fatalf("adding route B did not change the relay: %+v", r)
		}
		bIn := d2.listen(t, relay2, routeB, 0, er.b4)
		if ex := mustEcho(t, ent, bIn, 4096); ex.target != "ent" {
			t.Fatalf("route B answered by %q", ex.target)
		}
		flows("route B added")

		d3 := l.deploy(t, ra)
		if r := d3.results[relay2]; !r.Changed {
			t.Fatalf("removing route B did not change the relay: %+v", r)
		}
		if _, err := tcpEcho(ent, netip.Addr{}, bIn, 64, time.Second); err == nil {
			t.Fatal("route B still forwards after its removal")
		}
		flows("route B removed")
	})

	t.Run("failover", func(t *testing.T) {
		if err := exitA.gost.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
		up := relay.healthy(relayHop)
		if len(up) != 1 || up[0].Address != r2.b4.String() {
			t.Fatalf("health check: healthy %s, want only the second exit (%s)", addrStrings(up), r2.b4)
		}
		// The health check's connection passed the second exit, which
		// forwarded it to the target: its greeting is counted there.
		time.Sleep(200 * time.Millisecond)
		before := read()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := relay.drv.SetUpstreams(ctx, routeA, 1, up); err != nil {
			t.Fatalf("SetUpstreams: %v", err)
		}
		ex := echo(t, cli, in, 64<<10)
		if ex.target != "t1" {
			t.Fatalf("answered by %q", ex.target)
		}
		uex, err := udpEcho(cli, in, []int{500, 700}, 3*time.Second)
		if err != nil || uex.target != "t1" {
			t.Fatalf("UDP after the failover: %v (%q)", err, uex.target)
		}
		relayAfter, exitBAfter := relay.counters(t, routeA, 1), exitB.counters(t, routeA, 2)
		checkPayload(t, "gost relay across the failover", counted(t, before.relay, relayAfter), ex.up+uex.up, ex.down+uex.down)
		checkExit(t, "the second exit", counted(t, before.exitB, exitBAfter), ex.up+uex.up, ex.down+uex.down)
	})
}

// TestMixedGostEntryNftablesExit: a gost entry hands over to an nftables
// exit over a RAW link: TCP and UDP reach the target, the gost entry
// counts the payload exactly and the nftables exit the packets.
func TestMixedGostEntryNftablesExit(t *testing.T) {
	l := newLab(t)
	cli, ent, ext, t1 := l.netns("cli"), l.netns("ent"), l.netns("ext"), l.netns("t1")
	cl := l.connect(ent, cli, clientNet4, clientNet6)
	ex := l.connectNext(ent, ext)
	xt := l.connectNext(ext, t1)
	l.serve(t1, "t1", xt.b4)
	entry := l.gostNode(entry1, ent, ex.a4, cl.a4)
	exit := l.node(exit3, ext, nil, ex.b4)
	gostEntry := entryHop(entry1)
	gostEntry.Engine = model.EngineGost
	r := route(routeA, model.L4ProtocolTCPUDP, []model.Hop{gostEntry, exitHop(exit3)}, target(xt.b4))
	d := l.deploy(t, r)
	in := d.listen(t, entry1, routeA, 0, cl.a4)
	if got := d.hop(t, exit3, routeA, 1).GetIngressSources(); len(got) != 2 || got[0] != ex.a4.String() {
		t.Fatalf("the nftables exit admits %v, want the gost entry's addresses", got)
	}

	e0, x0 := entry.counters(t, routeA, 0), exit.counters(t, routeA, 1)
	tcp := mustEcho(t, cli, in, 128<<10)
	if tcp.target != "t1" {
		t.Fatalf("answered by %q", tcp.target)
	}
	e1, x1 := entry.counters(t, routeA, 0), exit.counters(t, routeA, 1)
	checkPayload(t, "gost entry, tcp", counted(t, e0, e1), tcp.up, tcp.down)
	checkTCP(t, "nftables exit, tcp", counted(t, x0, x1), tcp.up, tcp.down, false)

	udp, err := udpEcho(cli, in, []int{100, 1200, 333}, 3*time.Second)
	if err != nil || udp.target != "t1" {
		t.Fatalf("UDP exchange with %s: %v (answered by %q)", in, err, udp.target)
	}
	checkPayload(t, "gost entry, udp", counted(t, e1, entry.counters(t, routeA, 0)), udp.up, udp.down)
	checkUDP(t, "nftables exit, udp", counted(t, x1, exit.counters(t, routeA, 1)), udp, false)
}
