package anixops_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

// The privileged suite of anixops-protocol.md section 6.6: the real relay
// binary in network namespaces, three nodes on a bridge and the client and
// the target on the host, driven through the driver with the planner's own
// states. It needs root (CAP_NET_ADMIN and CAP_SYS_ADMIN for the namespaces),
// iproute2 and the relay binary (ANIXOPS_RELAY_BIN, or a Go toolchain to build
// it), and runs only with
// ANIXOPS_RELAY_E2E=1, which also makes a missing prerequisite fail instead of
// skip. The nft binary adds the UDP-blocked scenario; without it that one is
// skipped.
const relayE2E = "ANIXOPS_RELAY_E2E"

func requireLab(t *testing.T) {
	t.Helper()
	if os.Getenv(relayE2E) != "1" {
		t.Skipf("set %s=1 (as root) to run the anixops relay in network namespaces", relayE2E)
	}
	if os.Geteuid() != 0 {
		t.Fatalf("%s=1 needs root", relayE2E)
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Fatalf("%s=1 needs iproute2: %v", relayE2E, err)
	}
}

// relayBinary answers the relay to run: ANIXOPS_RELAY_BIN (CI builds it before
// it runs the test as root, which may not have a Go toolchain), or one built
// from testdata/anixops-relay.
func relayBinary(t *testing.T, dir string) string {
	t.Helper()
	if p := os.Getenv("ANIXOPS_RELAY_BIN"); p != "" {
		return p
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("%s=1 needs ANIXOPS_RELAY_BIN or a Go toolchain: %v", relayE2E, err)
	}
	bin := filepath.Join(dir, "anixops-relay")
	build := exec.Command("go", "build", "-o", bin, "./testdata/anixops-relay") // #nosec G204 -- fixed
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the relay: %v\n%s", err, out)
	}
	return bin
}

// sh runs a command and fails the test with its output when it fails.
func sh(t testing.TB, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput() // #nosec G204 -- fixed tools with arguments the test built
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// testLog sends a relay's output to the test log.
type testLog struct {
	t   *testing.T
	ref string
}

func (w testLog) Write(p []byte) (int, error) {
	for line := range strings.Lines(string(p)) {
		w.t.Logf("[%s] %s", w.ref, strings.TrimSpace(line))
	}
	return len(p), nil
}

// labNode is one namespace with its relay, driver and link files.
type labNode struct {
	ref, ns, addr string
	dir           string
	sup           *anixops.ProcessSupervisor
	drv           *anixops.Driver
}

// lab is the topology: a bridge in a namespace of its own, the host (the client
// and the target, 10.<n>.0.1) and three node namespaces on it.
type lab struct {
	t        *testing.T
	ca       *relaytest.PKI
	nodes    [3]*labNode // entry, relay, exit
	targetIP string
	tcp, udp string // the target's echo servers
	gen      uint64
	caps     *forwardv1.EngineCapabilities
}

func newLab(t *testing.T) *lab {
	requireLab(t)
	root, err := os.MkdirTemp("", "ar")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	bin := relayBinary(t, root)

	pid := os.Getpid()
	prefix := "ar" + strconv.Itoa(pid%10000)
	net24 := 100 + pid%100
	subnet := fmt.Sprintf("10.%d.0", net24)
	hub := prefix + "hub"
	l := &lab{t: t, targetIP: subnet + ".1"}
	t.Cleanup(func() {
		for _, n := range l.nodes {
			if n != nil {
				n.sup.Kill()
				_ = exec.Command("ip", "netns", "del", n.ns).Run() // #nosec G204 -- fixed
			}
		}
		_ = exec.Command("ip", "netns", "del", hub).Run()        // #nosec G204 -- fixed
		_ = exec.Command("ip", "link", "del", prefix+"rt").Run() // #nosec G204 -- fixed
	})
	// The bridge lives in a namespace of its own: in the host namespace the
	// host's firewall (Docker's FORWARD policy) would filter the frames it
	// bridges between the nodes.
	sh(t, "ip", "netns", "add", hub)
	sh(t, "ip", "-n", hub, "link", "add", "name", "hubbr", "type", "bridge")
	sh(t, "ip", "-n", hub, "link", "set", "hubbr", "up")
	sh(t, "ip", "link", "add", prefix+"rt", "type", "veth", "peer", "name", "rt", "netns", hub)
	sh(t, "ip", "-n", hub, "link", "set", "rt", "master", "hubbr")
	sh(t, "ip", "-n", hub, "link", "set", "rt", "up")
	sh(t, "ip", "addr", "add", l.targetIP+"/24", "dev", prefix+"rt")
	sh(t, "ip", "link", "set", prefix+"rt", "up")

	if l.ca, err = relaytest.New("test"); err != nil {
		t.Fatal(err)
	}
	for i, role := range []string{"en", "re", "ex"} {
		n := &labNode{ref: "forward-" + strconv.Itoa(11+i), ns: prefix + role, addr: fmt.Sprintf("%s.%d", subnet, 11+i)}
		n.dir = filepath.Join(root, n.ref)
		l.nodes[i] = n
		sh(t, "ip", "netns", "add", n.ns)
		host, peer := role, "eth0"
		sh(t, "ip", "link", "add", host, "netns", hub, "type", "veth", "peer", "name", peer, "netns", n.ns)
		sh(t, "ip", "-n", hub, "link", "set", host, "master", "hubbr")
		sh(t, "ip", "-n", hub, "link", "set", host, "up")
		sh(t, "ip", "-n", n.ns, "addr", "add", n.addr+"/24", "dev", peer)
		sh(t, "ip", "-n", n.ns, "link", "set", peer, "up")
		sh(t, "ip", "-n", n.ns, "link", "set", "lo", "up")

		tls := filepath.Join(n.dir, "lib", "tls")
		if err := os.MkdirAll(tls, 0o750); err != nil {
			t.Fatal(err)
		}
		l.writeLinkFiles(n, n.ref)
		cfg := anixops.DefaultConfig()
		cfg.Version = "anixops-relay e2e"
		cfg.Dir, cfg.RuntimeDir = filepath.Join(n.dir, "lib"), filepath.Join(n.dir, "run")
		cfg.LinkCert, cfg.LinkKey, cfg.LinkCA = filepath.Join(tls, "link.crt"), filepath.Join(tls, "link.key"), filepath.Join(tls, "link-ca.crt")
		if err := os.MkdirAll(cfg.RuntimeDir, 0o750); err != nil {
			t.Fatal(err)
		}
		n.sup = &anixops.ProcessSupervisor{
			Command: []string{"ip", "netns", "exec", n.ns, bin},
			Config:  filepath.Join(cfg.Dir, anixops.ConfigFile),
			Socket:  filepath.Join(cfg.RuntimeDir, anixops.ControlSocket),
			Log:     testLog{t, n.ref},
		}
		if n.drv, err = anixops.New(cfg, anixops.WithSupervisor(n.sup)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = n.drv.Remove(context.Background()) })
	}
	l.caps, err = l.nodes[0].drv.Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	l.startTargets()
	return l
}

// writeLinkFiles gives node n the link certificate of identity ref.
func (l *lab) writeLinkFiles(n *labNode, ref string) {
	l.t.Helper()
	cert, key, err := l.ca.Issue(relaytest.Cert{Node: ref})
	if err != nil {
		l.t.Fatal(err)
	}
	tls := filepath.Join(n.dir, "lib", "tls")
	for name, data := range map[string][]byte{"link.crt": cert, "link.key": key, "link-ca.crt": l.ca.CAPEM()} {
		if err := os.WriteFile(filepath.Join(tls, name), data, 0o600); err != nil {
			l.t.Fatal(err)
		}
	}
}

// startTargets runs the echo servers the route's targets are, on the host
// side of the bridge.
func (l *lab) startTargets() {
	t := l.t
	ln, err := net.Listen("tcp", l.targetIP+":0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	l.tcp = ln.Addr().String()
	pc, err := net.ListenPacket("udp", l.targetIP+":"+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
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
	l.udp = pc.LocalAddr().String()
}

func (l *lab) targetPort() uint32 {
	_, p, _ := net.SplitHostPort(l.tcp)
	n, _ := strconv.Atoi(p)
	return uint32(n) // #nosec G115 -- a port
}

// route plans the three-hop route with the given carrier on every link and
// answers each node's state.
func (l *lab) route(carrier forwardv1.AnixOpsCarrier, trusted bool) [3]*forwardv1.NodeForwardState {
	t := l.t
	// Private targets (the host side of the bridge) are an administrator's.
	owner := "admin"
	nodes := make([]*forwardv1.NodeInfo, 3)
	for i, n := range l.nodes {
		info := &forwardv1.NodeInfo{
			NodeRef: n.ref, Addresses: []string{n.addr},
			PortRange: &forwardv1.PortRange{First: 41000, Last: 41999},
			Engines:   []*forwardv1.EngineCapabilities{l.caps},
		}
		if trusted {
			info.Labels = map[string]string{"link": "iepl"}
		}
		nodes[i] = info
	}
	link := &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS, Carrier: carrier}
	req := &forwardv1.PlanRouteRequest{
		Route: &forwardv1.Route{
			Id: "01JF1D000000000000000000E1", Owner: owner, Name: "e2e",
			Listen: &forwardv1.Listen{Port: 41001, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP},
			Hops: []*forwardv1.Hop{
				{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_ANIXOPS, NodeRefs: []string{l.nodes[0].ref}},
				{Role: forwardv1.HopRole_HOP_ROLE_RELAY, Engine: forwardv1.Engine_ENGINE_ANIXOPS, NodeRefs: []string{l.nodes[1].ref}, Ingress: link},
				{Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_ANIXOPS, NodeRefs: []string{l.nodes[2].ref}, Ingress: link},
			},
			Targets: []*forwardv1.Target{{Host: l.targetIP, Port: l.targetPort()}},
			Policy:  &forwardv1.Policy{TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE},
		},
		Nodes: nodes,
	}
	resp, err := planner.PlanRoute(req, nil, nil, planner.Options{Cluster: "test", EnableAnixOps: true})
	if err != nil || len(resp.GetViolations()) > 0 {
		t.Fatalf("planning: %v %v", err, resp.GetViolations())
	}
	l.gen++
	var out [3]*forwardv1.NodeForwardState
	for _, s := range resp.GetStates() {
		for i, n := range l.nodes {
			if s.GetNodeRef() == n.ref {
				out[i] = conformance.State(n.ref, l.gen, s.GetHops()...)
			}
		}
	}
	for i, s := range out {
		if s == nil {
			t.Fatalf("the plan has no state for node %d", i)
		}
	}
	return out
}

func (l *lab) apply(states [3]*forwardv1.NodeForwardState) {
	t := l.t
	t.Helper()
	// Exit first, entry last: the entry takes clients once it is applied.
	for _, i := range []int{2, 1, 0} {
		n := l.nodes[i]
		a, err := n.drv.Render(states[i])
		if err != nil {
			t.Fatalf("render %s: %v", n.ref, err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		_, err = n.drv.Apply(ctx, a)
		cancel()
		if err != nil {
			t.Fatalf("apply %s: %v", n.ref, err)
		}
	}
}

func (l *lab) entryPort(states [3]*forwardv1.NodeForwardState) string {
	for _, h := range states[0].GetHops() {
		if h.GetHopIndex() == 0 {
			return net.JoinHostPort(l.nodes[0].addr, strconv.Itoa(int(h.GetListen().GetPort())))
		}
	}
	l.t.Fatal("no entry hop")
	return ""
}

// echoThrough sends size bytes through the entry, half-closes and expects
// them back.
func echoThrough(addr string, size int) error {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i*13 + i>>9)
	}
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
	if !bytes.Equal(got, data) {
		return fmt.Errorf("%d bytes came back as %d different bytes", size, len(got))
	}
	return nil
}

func (l *lab) mustEcho(addr string, size int) {
	l.t.Helper()
	if err := echoThrough(addr, size); err != nil {
		l.t.Fatalf("echo through %s: %v", addr, err)
	}
}

func (l *lab) udpEcho(addr string) error {
	c, err := net.Dial("udp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = c.Write([]byte("udp through the chain"))
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 100)
		if n, err := c.Read(buf); err == nil {
			if string(buf[:n]) != "udp through the chain" {
				return fmt.Errorf("udp echo %q", buf[:n])
			}
			return nil
		}
	}
	return fmt.Errorf("no udp echo from %s", addr)
}

func (l *lab) observe(i int) driver.Observation {
	l.t.Helper()
	o, err := l.nodes[i].drv.Observe(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	return o
}

// fresh stops every relay and removes what the drivers own, so a scenario
// starts on new processes and carriers.
func (l *lab) fresh() {
	for _, n := range l.nodes {
		if err := n.drv.Remove(l.t.Context()); err != nil {
			l.t.Fatal(err)
		}
	}
}

func TestNetnsRelayChain(t *testing.T) {
	l := newLab(t)
	for _, c := range []struct {
		name    string
		carrier forwardv1.AnixOpsCarrier
		admin   bool
	}{
		{"AUTO", forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_AUTO, false},
		{"TLS_TCP", forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP, false},
		{"QUIC", forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC, false},
		{"PLAIN", forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			l.t = t
			l.fresh()
			states := l.route(c.carrier, c.admin)
			l.apply(states)
			addr := l.entryPort(states)

			// Data, half-close and counters through three hops.
			const size = 1 << 20
			l.mustEcho(addr, size)
			for range 4 {
				l.mustEcho(addr, 10_000)
			}
			if err := l.udpEcho(addr); err != nil {
				t.Fatal(err)
			}
			want := uint64(size + 4*10_000)
			deadline := time.Now().Add(10 * time.Second)
			for {
				done := true
				for i := range l.nodes {
					o := l.observe(i)
					c := o.Counters[0]
					// The UDP association stays for its 60 s idle time.
					if c.GetUpBytes() < want || c.GetDownBytes() < want || c.GetActiveConns() > 1 {
						done = false
					}
				}
				if done {
					break
				}
				if time.Now().After(deadline) {
					for i := range l.nodes {
						t.Logf("node %d: %v", i, l.observe(i).Counters)
					}
					t.Fatal("the counters never reached the payload on every hop")
				}
				time.Sleep(50 * time.Millisecond)
			}
			// Several concurrent connections over the shared carriers.
			errs := make(chan error, 16)
			for range 16 {
				go func() { errs <- echoThrough(addr, 50_000) }()
			}
			for range 16 {
				if err := <-errs; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestNetnsIdentityPinningAndPeerRemoval(t *testing.T) {
	l := newLab(t)
	carrier := forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP

	t.Run("an upstream presenting another identity is refused", func(t *testing.T) {
		l.t = t
		l.fresh()
		states := l.route(carrier, false)
		// The relay holds a certificate of another node of the same CA.
		l.writeLinkFiles(l.nodes[1], "forward-19")
		l.apply(states)
		addr := l.entryPort(states)
		if err := echoThrough(addr, 1000); err == nil {
			t.Fatal("the entry used a relay that is not the identity it pinned")
		}
		l.writeLinkFiles(l.nodes[1], l.nodes[1].ref) // put it back for the next scenarios
	})

	t.Run("a peer removed from ingress_peers loses its carriers", func(t *testing.T) {
		l.t = t
		l.fresh()
		states := l.route(carrier, false)
		l.apply(states)
		addr := l.entryPort(states)
		l.mustEcho(addr, 1000)

		// A held connection through the chain.
		held, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.Close() }()
		_ = held.SetDeadline(time.Now().Add(15 * time.Second))
		if _, err := held.Write([]byte("hi")); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 2)
		if _, err := io.ReadFull(held, buf); err != nil {
			t.Fatal(err)
		}

		// The exit's state now names another peer: the relay's carrier gets
		// GOAWAY peer_not_allowed and the held connection ends.
		changed := l.route(carrier, false)
		for _, h := range changed[2].GetHops() {
			h.IngressPeers = []string{relaytest.Identity("test", "forward-19")}
		}
		changed[2] = conformance.State(l.nodes[2].ref, l.gen+1, changed[2].GetHops()...)
		a, err := l.nodes[2].drv.Render(changed[2])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.nodes[2].drv.Apply(t.Context(), a); err != nil {
			t.Fatal(err)
		}
		_, _ = held.Write([]byte("x"))
		if _, err := held.Read(buf[:1]); err == nil {
			t.Fatal("a connection survived the removal of its carrier's peer")
		}
		if err := echoThrough(addr, 1000); err == nil {
			t.Fatal("the exit served an identity that was removed from ingress_peers")
		}
	})
}

func TestNetnsExitRestart(t *testing.T) {
	l := newLab(t)
	for _, c := range []struct {
		name    string
		carrier forwardv1.AnixOpsCarrier
	}{
		{"TLS_TCP", forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP},
		{"QUIC", forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC},
	} {
		t.Run(c.name, func(t *testing.T) {
			l.t = t
			l.fresh()
			states := l.route(c.carrier, false)
			l.apply(states)
			addr := l.entryPort(states)
			l.mustEcho(addr, 1000)

			// The exit's relay crashes and its supervisor starts it again;
			// the entry and relay get a new carrier and new connections pass
			// within seconds (L1 to L5).
			l.nodes[2].sup.Kill()
			if err := l.nodes[2].sup.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				if err := echoThrough(addr, 1000); err == nil {
					return
				} else if time.Now().After(deadline) {
					t.Fatalf("no connection passes %v after the exit restarted: %v", 10*time.Second, err)
				}
				time.Sleep(100 * time.Millisecond)
			}
		})
	}
}

// With UDP blocked between the nodes AUTO still connects: its QUIC probe
// fails and TLS_TCP takes over (section 5.4).
func TestNetnsAutoFallsBackWhenUDPIsBlocked(t *testing.T) {
	l := newLab(t)
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("nft is needed to block UDP")
	}
	for _, n := range l.nodes {
		sh(t, "ip", "netns", "exec", n.ns, "nft", "add", "table", "inet", "blockudp")
		sh(t, "ip", "netns", "exec", n.ns, "nft", "add", "chain", "inet", "blockudp", "in", "{ type filter hook input priority 0 ; }")
		sh(t, "ip", "netns", "exec", n.ns, "nft", "add", "rule", "inet", "blockudp", "in", "udp", "dport", ">=", "41000", "drop")
	}
	states := l.route(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_AUTO, false)
	l.apply(states)
	addr := l.entryPort(states)
	start := time.Now()
	l.mustEcho(addr, 100_000)
	if d := time.Since(start); d > 12*time.Second {
		t.Fatalf("the first connection took %v: AUTO's QUIC probe is 3 s per hop", d)
	}
	l.mustEcho(addr, 100_000)
}
