package relayd_test

import (
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayd"
)

// countingEcho is an echo server that counts the connections it accepted.
func countingEcho(t testing.TB) (port uint32, accepted *atomic.Int64) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	accepted = new(atomic.Int64)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return uint32(l.Addr().(*net.TCPAddr).Port), accepted // #nosec G115 -- a port
}

// closedPort answers a port nothing listens on.
func closedPort(t testing.TB) uint32 { return freePort(t) }

// refused reports whether the connection to port is closed on us at once
// (the hop admitted nobody) rather than answered.
func refused(t testing.TB, port uint32) bool {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(port)), testWait)
	if err != nil {
		return true
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = c.Write([]byte("ping"))
	buf := make([]byte, 4)
	_, err = io.ReadFull(c, buf)
	return err != nil
}

func TestStrategies(t *testing.T) {
	t.Run("failover passes a dead upstream and opens its breaker", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		live, _ := countingEcho(t)
		dead := closedPort(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target("127.0.0.1", dead), target("127.0.0.1", live))
		h.Balance = relayctl.BalanceFailover
		h.Upstreams[0].Priority, h.Upstreams[1].Priority = 0, 10
		h.Breaker = relayctl.Breaker{MaxFails: 2, OpenMs: 60000}
		n.mustApply(h)
		for range 4 {
			c := dialTCP(t, entry)
			roundTrip(t, c, []byte("hello"))
			_ = c.Close()
		}
		o := n.hopObs(route, 0)
		var dh *relayctl.UpstreamState
		for i := range o.Health {
			if o.Health[i].Port == dead {
				dh = &o.Health[i]
			}
		}
		if dh == nil || dh.ConsecutiveFailures < 2 || dh.CircuitOpenUntilMs == 0 {
			t.Fatalf("the dead upstream's health %+v, want an open breaker after two failures", o.Health)
		}
	})

	t.Run("round robin spreads by weight", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		a, na := countingEcho(t)
		b, nb := countingEcho(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target("127.0.0.1", a), target("127.0.0.1", b))
		h.Upstreams[0].Weight = 3
		n.mustApply(h)
		for range 8 {
			c := dialTCP(t, entry)
			roundTrip(t, c, []byte("x"))
			_ = c.Close()
		}
		if na.Load() != 6 || nb.Load() != 2 {
			t.Fatalf("connections %d and %d, want 6 and 2 for weights 3 and 1", na.Load(), nb.Load())
		}
	})

	t.Run("ip hash keeps a client on one upstream", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		a, na := countingEcho(t)
		b, nb := countingEcho(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target("127.0.0.1", a), target("127.0.0.1", b))
		h.Balance = relayctl.BalanceIPHash
		n.mustApply(h)
		for range 6 {
			c := dialTCP(t, entry)
			roundTrip(t, c, []byte("x"))
			_ = c.Close()
		}
		if (na.Load() != 6 || nb.Load() != 0) && (na.Load() != 0 || nb.Load() != 6) {
			t.Fatalf("one client went to both upstreams: %d and %d", na.Load(), nb.Load())
		}
	})

	t.Run("least conn picks the idle upstream", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		a, na := countingEcho(t)
		b, nb := countingEcho(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target("127.0.0.1", a), target("127.0.0.1", b))
		h.Balance = relayctl.BalanceLeastConn
		n.mustApply(h)
		var held []net.Conn
		for range 4 {
			c := dialTCP(t, entry)
			roundTrip(t, c, []byte("x"))
			held = append(held, c)
		}
		if na.Load() != 2 || nb.Load() != 2 {
			t.Fatalf("four held connections went %d and %d, want 2 and 2", na.Load(), nb.Load())
		}
		_ = held
	})

	t.Run("random reaches every upstream", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		a, na := countingEcho(t)
		b, nb := countingEcho(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target("127.0.0.1", a), target("127.0.0.1", b))
		h.Balance = relayctl.BalanceRandom
		n.mustApply(h)
		for range 60 {
			c := dialTCP(t, entry)
			roundTrip(t, c, []byte("x"))
			_ = c.Close()
		}
		if na.Load() == 0 || nb.Load() == 0 {
			t.Fatalf("random used %d and %d connections", na.Load(), nb.Load())
		}
	})
}

func TestAdmission(t *testing.T) {
	t.Run("a dialler whose identity is not in ingress_peers is refused", func(t *testing.T) {
		p := newPKI(t)
		entry, exit := newNode(t, p, "forward-1", relayd.Options{}), newNode(t, p, "forward-3", relayd.Options{})
		host, port := echoTCP(t)
		exitPort, entryPort := freePort(t), freePort(t)
		// The exit allows forward-9 only; forward-1 dials.
		exit.mustApply(ingress(1, exitPort, relayctl.CarrierTLSTCP, "exit", []string{identity("forward-9")}, target(host, port)))
		entry.mustApply(rawHop(entryPort, true, false, next(exitPort, relayctl.CarrierTLSTCP, "forward-3")))
		if !refused(t, entryPort) {
			t.Fatal("the exit served a dialler that is not in its ingress_peers")
		}
		o := entry.hopObs(route, 0)
		if len(o.Health) == 0 || o.Health[0].ConsecutiveFailures == 0 {
			t.Fatalf("the entry did not count the refusal against the upstream: %+v", o.Health)
		}
	})

	t.Run("an upstream that presents another identity is refused", func(t *testing.T) {
		p := newPKI(t)
		entry, exit := newNode(t, p, "forward-1", relayd.Options{}), newNode(t, p, "forward-3", relayd.Options{})
		host, port := echoTCP(t)
		exitPort, entryPort := freePort(t), freePort(t)
		exit.mustApply(ingress(1, exitPort, relayctl.CarrierTLSTCP, "exit", []string{identity("forward-1")}, target(host, port)))
		up := next(exitPort, relayctl.CarrierTLSTCP, "forward-3")
		up.PeerIdentity = identity("forward-4") // the entry expects another node
		entry.mustApply(rawHop(entryPort, true, false, up))
		if !refused(t, entryPort) {
			t.Fatal("the entry used an upstream that is not the identity it pinned")
		}
	})

	t.Run("a source outside ingress_sources is refused before the handshake", func(t *testing.T) {
		p := newPKI(t)
		entry, exit := newNode(t, p, "forward-1", relayd.Options{}), newNode(t, p, "forward-3", relayd.Options{})
		host, port := echoTCP(t)
		exitPort, entryPort := freePort(t), freePort(t)
		x := ingress(1, exitPort, relayctl.CarrierTLSTCP, "exit", []string{identity("forward-1")}, target(host, port))
		x.Sources = []string{"192.0.2.77"}
		exit.mustApply(x)
		entry.mustApply(rawHop(entryPort, true, false, next(exitPort, relayctl.CarrierTLSTCP, "forward-3")))
		if !refused(t, entryPort) {
			t.Fatal("the exit admitted a source outside ingress_sources")
		}
	})

	t.Run("a stream for another route or hop is answered route_mismatch", func(t *testing.T) {
		p := newPKI(t)
		entry, exit := newNode(t, p, "forward-1", relayd.Options{}), newNode(t, p, "forward-3", relayd.Options{})
		host, port := echoTCP(t)
		exitPort, entryPort := freePort(t), freePort(t)
		// The exit is hop 5; the entry's next hop is 1.
		exit.mustApply(ingress(5, exitPort, relayctl.CarrierTLSTCP, "exit", []string{identity("forward-1")}, target(host, port)))
		entry.mustApply(rawHop(entryPort, true, false, next(exitPort, relayctl.CarrierTLSTCP, "forward-3")))
		if !refused(t, entryPort) {
			t.Fatal("the exit served a stream for a hop it does not run")
		}
	})

	t.Run("a raw listener admits only its sources", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		host, port := echoTCP(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target(host, port))
		h.Role = "relay"
		h.Sources = []string{"192.0.2.1"}
		n.mustApply(h)
		if !refused(t, entry) {
			t.Fatal("a raw hop admitted a source outside its sources")
		}
		h.Sources = []string{"127.0.0.0/8"}
		n.mustApply(h)
		c := dialTCP(t, entry)
		roundTrip(t, c, []byte("now admitted"))
	})
}

func TestPauseMaxConnsAndQuota(t *testing.T) {
	t.Run("pause admits nobody new and keeps what runs, epoch and counters", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		host, port := echoTCP(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target(host, port))
		n.mustApply(h)
		running := dialTCP(t, entry)
		roundTrip(t, running, []byte("before"))
		epoch := n.hopObs(route, 0).Epoch
		h.Paused = true
		n.mustApply(h)
		if !refused(t, entry) {
			t.Fatal("a paused hop admitted a connection")
		}
		roundTrip(t, running, []byte("still running"))
		h.Paused = false
		n.mustApply(h)
		roundTrip(t, dialTCP(t, entry), []byte("admitted again"))
		if got := n.hopObs(route, 0).Epoch; got != epoch {
			t.Fatalf("pause and resume changed the epoch %s -> %s", epoch, got)
		}
	})

	t.Run("max_conns refuses the connection over the limit", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		host, port := echoTCP(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target(host, port))
		h.Limits.MaxConns = 2
		n.mustApply(h)
		a, b := dialTCP(t, entry), dialTCP(t, entry)
		roundTrip(t, a, []byte("1"))
		roundTrip(t, b, []byte("2"))
		if !refused(t, entry) {
			t.Fatal("a third connection was admitted at max_conns 2")
		}
		_ = a.Close()
		waitFor(t, "a slot", func() bool { return n.hopObs(route, 0).Active < 2 })
		roundTrip(t, dialTCP(t, entry), []byte("3"))
	})

	t.Run("quota ends the traffic and refuses more until it is raised", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		host, port := echoTCP(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target(host, port))
		h.Limits.QuotaBytes = 100_000
		n.mustApply(h)
		c := dialTCP(t, entry)
		_ = c.SetDeadline(time.Now().Add(testWait))
		go func() { _, _ = c.Write(pattern(2_000_000)) }()
		_, _ = io.Copy(io.Discard, c) // the hop ends the connection at the quota
		waitFor(t, "the connection to end", func() bool { return n.hopObs(route, 0).Active == 0 })
		o := n.hopObs(route, 0)
		if total := o.UpBytes + o.DownBytes; total < 100_000 || total > 100_000+4*32*1024 {
			t.Fatalf("moved %d bytes, want the quota 100000 plus at most a few copy buffers", total)
		}
		if !refused(t, entry) {
			t.Fatal("a hop at its quota admitted a connection")
		}
		epoch := o.Epoch
		h.Limits.QuotaBytes = 10_000_000
		n.mustApply(h)
		roundTrip(t, dialTCP(t, entry), []byte("quota raised"))
		if got := n.hopObs(route, 0).Epoch; got != epoch {
			t.Fatalf("raising the quota changed the epoch")
		}
	})

	t.Run("bandwidth paces both directions", func(t *testing.T) {
		n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
		host, port := echoTCP(t)
		entry := freePort(t)
		h := rawHop(entry, true, false, target(host, port))
		h.Limits.BandwidthBps = 4_000_000 // 500 kB/s each way
		n.mustApply(h)
		c := dialTCP(t, entry)
		start := time.Now()
		roundTrip(t, c, pattern(600_000))
		if d := time.Since(start); d < 700*time.Millisecond {
			t.Fatalf("600 kB each way at 500 kB/s took %v", d)
		}
	})
}

func TestApplyKeepsWhatDidNotChange(t *testing.T) {
	n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
	a, na := countingEcho(t)
	b, nb := countingEcho(t)
	entry := freePort(t)
	h := rawHop(entry, true, false, target("127.0.0.1", a))
	n.mustApply(h)
	held := dialTCP(t, entry)
	roundTrip(t, held, []byte("held"))
	epoch := n.hopObs(route, 0).Epoch

	// A hot change: the upstreams. The listener, the connection and the epoch stay.
	h.Upstreams = []relayctl.Upstream{target("127.0.0.1", b)}
	if changed, err := n.apply(h); err != nil || !changed {
		t.Fatalf("hot apply: changed %v, err %v", changed, err)
	}
	roundTrip(t, held, []byte("held, still on the old upstream"))
	roundTrip(t, dialTCP(t, entry), []byte("new connection"))
	if na.Load() != 1 || nb.Load() != 1 {
		t.Fatalf("connections a=%d b=%d, want the held one on a and the new one on b", na.Load(), nb.Load())
	}
	if got := n.hopObs(route, 0).Epoch; got != epoch {
		t.Fatalf("a hot apply changed the epoch")
	}
	// The same document again changes nothing.
	if changed, err := n.apply(h); err != nil || changed {
		t.Fatalf("re-apply: changed %v, err %v", changed, err)
	}

	// A structural change: another port. New listener, new epoch, the old
	// listener's final counters retired, the old port closed.
	old := entry
	h.Listen.Port = freePort(t)
	n.mustApply(h)
	if got := n.hopObs(route, 0).Epoch; got == epoch {
		t.Fatal("a new listener kept the epoch")
	}
	if _, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(old)), time.Second); err == nil {
		t.Fatal("the old port still listens")
	}
	retired := n.relay.Observe(true).Retired
	if len(retired) != 1 || retired[0].Epoch != epoch || retired[0].UpBytes == 0 {
		t.Fatalf("retired counters %+v", retired)
	}
	if again := n.relay.Observe(true).Retired; len(again) != 0 {
		t.Fatalf("retired counters are drained once, got %+v", again)
	}
}

func TestApplyConflictChangesNothing(t *testing.T) {
	n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
	host, port := echoTCP(t)
	entry := freePort(t)
	h := rawHop(entry, true, false, target(host, port))
	n.mustApply(h)
	epoch := n.hopObs(route, 0).Epoch
	applies := n.relay.Status().Applies

	foreign := freePort(t)
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(int(foreign)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	second := h
	second.Route = "01JF2A000000000000000000B1"
	second.Listen.Port = foreign
	_, err = n.apply(h, second)
	if !errors.Is(err, relayd.ErrConflict) {
		t.Fatalf("apply onto a foreign listener: %v, want ErrConflict", err)
	}
	if st := n.relay.Status(); st.Applies != applies || len(st.Hops) != 1 {
		t.Fatalf("a refused apply changed the relay: %+v", st)
	}
	if got := n.hopObs(route, 0).Epoch; got != epoch {
		t.Fatal("a refused apply ended the epoch")
	}
	roundTrip(t, dialTCP(t, entry), []byte("still serving"))
}

func TestSetRotation(t *testing.T) {
	n := newNode(t, newPKI(t), "forward-1", relayd.Options{})
	a, na := countingEcho(t)
	b, nb := countingEcho(t)
	entry := freePort(t)
	h := rawHop(entry, true, false, target("127.0.0.1", a), target("127.0.0.1", b))
	n.mustApply(h)
	err := n.relay.SetRotation(relayctl.Rotation{Route: route, Hop: 0, Active: []relayctl.RotationEntry{{Address: "127.0.0.1", Port: b}}})
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		c := dialTCP(t, entry)
		roundTrip(t, c, []byte("x"))
		_ = c.Close()
	}
	if na.Load() != 0 || nb.Load() != 4 {
		t.Fatalf("with only b in rotation: a=%d b=%d", na.Load(), nb.Load())
	}
	// A changing apply puts every rendered upstream back.
	h.Limits.MaxConns = 50
	n.mustApply(h)
	if o := n.hopObs(route, 0); len(o.Rotation) != 2 {
		t.Fatalf("rotation after an apply: %+v", o.Rotation)
	}
	for _, bad := range []relayctl.Rotation{
		{Route: route, Hop: 9, Active: []relayctl.RotationEntry{{Address: "127.0.0.1", Port: a}}},
		{Route: route, Hop: 0, Active: []relayctl.RotationEntry{{Address: "127.0.0.1", Port: 1}}},
		{Route: route, Hop: 0},
		{Route: route, Hop: 0, Active: []relayctl.RotationEntry{{Address: "127.0.0.1", Port: a}, {Address: "127.0.0.1", Port: a}}},
	} {
		if err := n.relay.SetRotation(bad); err == nil {
			t.Fatalf("rotation %+v was accepted", bad)
		}
	}
}

func TestUDPAssociationsIdleOut(t *testing.T) {
	n := newNode(t, newPKI(t), "forward-1", relayd.Options{UDPIdle: 300 * time.Millisecond})
	host, port := echoUDP(t)
	entry := freePort(t)
	n.mustApply(rawHop(entry, false, true, target(host, port)))
	uc, err := net.Dial("udp", "127.0.0.1:"+strconv.Itoa(int(entry)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = uc.Close() }()
	_, _ = uc.Write([]byte("hello"))
	_ = uc.SetReadDeadline(time.Now().Add(testWait))
	buf := make([]byte, 16)
	if _, err := uc.Read(buf); err != nil {
		t.Fatal(err)
	}
	if n.hopObs(route, 0).Active != 1 {
		t.Fatal("the association is not counted")
	}
	waitFor(t, "the association to idle out", func() bool { return n.hopObs(route, 0).Active == 0 })
}

func TestPeerRemovalAndCredentialReload(t *testing.T) {
	t.Run("removing a peer from ingress_peers ends its carriers", func(t *testing.T) {
		c := newChain(t, relayctl.CarrierTLSTCP, relayd.Options{})
		held := dialTCP(t, c.entryPort)
		roundTrip(t, held, []byte("through the chain"))
		// The exit no longer allows the relay: its carrier gets GOAWAY
		// peer_not_allowed and the held connection ends.
		exit := c.exit.cfg.Hops
		exit[0].Peers = []string{identity("forward-9")}
		c.exit.mustApply(exit...)
		_ = held.SetReadDeadline(time.Now().Add(testWait))
		buf := make([]byte, 1)
		_, _ = held.Write([]byte("x"))
		if _, err := held.Read(buf); err == nil {
			t.Fatal("a connection survived the removal of the relay from the exit's ingress_peers")
		}
		if !refused(t, c.entryPort) {
			t.Fatal("the exit served the identity that was removed")
		}
	})

	t.Run("a reload of renewed credentials keeps every connection", func(t *testing.T) {
		p := newPKI(t)
		entry, exit := newNode(t, p, "forward-1", relayd.Options{}), newNode(t, p, "forward-3", relayd.Options{})
		host, port := echoTCP(t)
		exitPort, entryPort := freePort(t), freePort(t)
		exit.mustApply(ingress(1, exitPort, relayctl.CarrierTLSTCP, "exit", []string{identity("forward-1")}, target(host, port)))
		entry.mustApply(rawHop(entryPort, true, false, next(exitPort, relayctl.CarrierTLSTCP, "forward-3")))
		held := dialTCP(t, entryPort)
		roundTrip(t, held, []byte("before the renewal"))
		epoch := entry.hopObs(route, 0).Epoch

		p.files("forward-1") // a new certificate for the same node, from the same CA
		if err := entry.relay.ReloadCredentials(); err != nil {
			t.Fatal(err)
		}
		roundTrip(t, held, []byte("after the renewal"))
		roundTrip(t, dialTCP(t, entryPort), []byte("a new connection"))
		if got := entry.hopObs(route, 0).Epoch; got != epoch {
			t.Fatal("a credential reload ended the epoch")
		}
	})

	t.Run("a reload that drops the peer's CA ends the carriers that depended on it", func(t *testing.T) {
		p := newPKI(t)
		entry, exit := newNode(t, p, "forward-1", relayd.Options{}), newNode(t, p, "forward-3", relayd.Options{})
		host, port := echoTCP(t)
		exitPort, entryPort := freePort(t), freePort(t)
		exit.mustApply(ingress(1, exitPort, relayctl.CarrierTLSTCP, "exit", []string{identity("forward-1")}, target(host, port)))
		entry.mustApply(rawHop(entryPort, true, false, next(exitPort, relayctl.CarrierTLSTCP, "forward-3")))
		held := dialTCP(t, entryPort)
		roundTrip(t, held, []byte("before"))

		// The entry now trusts another CA only: the exit's certificate no
		// longer verifies there.
		other := newPKI(t)
		files := entry.link
		if err := os.WriteFile(files.CA, other.ca.CAPEM(), 0o600); err != nil {
			t.Fatal(err)
		}
		certPEM, keyPEM, err := other.ca.Issue(relaytestCert("forward-1"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(files.Cert, certPEM, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(files.Key, keyPEM, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := entry.relay.ReloadCredentials(); err != nil {
			t.Fatal(err)
		}
		_ = held.SetReadDeadline(time.Now().Add(testWait))
		_, _ = held.Write([]byte("x"))
		if _, err := held.Read(make([]byte, 1)); err == nil {
			t.Fatal("a connection survived the loss of its peer's CA")
		}
		if !refused(t, entryPort) {
			t.Fatal("the entry dialled a node its trust bundle no longer vouches for")
		}
	})
}
