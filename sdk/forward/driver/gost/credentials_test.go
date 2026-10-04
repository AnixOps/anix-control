package gost_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// retiredLog collects what the WithRetiredCounters hook gets.
type retiredLog struct {
	mu sync.Mutex
	c  []*forwardv1.Counters
}

func (r *retiredLog) add(c []*forwardv1.Counters) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.c = append(r.c, c...)
}

func (r *retiredLog) take() []*forwardv1.Counters {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.c
	r.c = nil
	return out
}

// credsDriver answers a driver on a fakeGost whose link certificate files
// exist, with the hook collecting into r.
func credsDriver(t testing.TB, r *retiredLog) (*gost.Driver, *fakeGost, gost.Config) {
	t.Helper()
	f, cfg := newFakeGost(t)
	cfg.ReadyTimeout = 300 * time.Millisecond
	files := linkPKI(t, shortDir(t), "forward-11")["forward-11"]
	cfg.LinkCert, cfg.LinkKey, cfg.LinkCA = files[0], files[1], files[2]
	d, err := gost.New(cfg, gost.WithRunner(f), gost.WithSupervisor(f), gost.WithRetiredCounters(r.add))
	if err != nil {
		t.Fatal(err)
	}
	return d, f, cfg
}

// credsHops answers three hops of the default topology: tlsHop listens
// with mutual TLS (with mux when mux), rawHop is RAW all through, and
// dialHop listens RAW and dials its upstreams over TLS.
func credsHops(t testing.TB, mux bool) (tlsHop, rawHop, dialHop *forwardv1.NodeHop) {
	t.Helper()
	b := builder(t)
	tlsHop = b.Relay(conformance.RouteA, 0, b.Upstreams(b.Top.UpstreamsV4, 2))
	tlsHop.Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS, Mux: mux}
	tlsHop.IngressPeers = []string{"spiffe://anixops/example/agent/forward-10"}
	rawHop = b.Simple(conformance.RouteB, 1)
	dialHop = b.Simple(conformance.RouteC, 2)
	for i, u := range dialHop.Upstreams {
		u.Egress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS}
		u.NodeRef = "forward-2" + string(rune('0'+i))
		u.PeerIdentity = "spiffe://anixops/example/agent/" + u.NodeRef
	}
	return tlsHop, rawHop, dialHop
}

// recordedSeq answers the state file's seq.
func recordedSeq(t testing.TB, cfg gost.Config) (seq, loads uint64) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cfg.Dir, "state.json")) // #nosec G304 -- the test's own file
	if err != nil {
		t.Fatal(err)
	}
	var st struct{ Seq, Loads uint64 }
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return st.Seq, st.Loads
}

// TestReloadCredentials: ReloadCredentials re-creates the TLS hop's
// service through the web API without a reload or restart; that hop
// alone starts a new epoch (recorded in the state file) and its counters
// reach the hook; the RAW hop and the hop that only dials over TLS keep
// their epochs, and the latter its SetUpstreams selection. Files that do
// not load change nothing; a silent web API makes it restart gost.
func TestReloadCredentials(t *testing.T) {
	var r retiredLog
	d, f, cfg := credsDriver(t, &r)
	tlsHop, rawHop, dialHop := credsHops(t, false)
	kt, kr, kd := driver.KeyOf(tlsHop), driver.KeyOf(rawHop), driver.KeyOf(dialHop)

	// Nothing applied: nothing to do.
	if err := d.ReloadCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	apply(t, d, 1, tlsHop, rawHop, dialHop)
	f.traffic("r"+kt.RouteID+"-h1", 11, 12)
	f.traffic("r"+kr.RouteID+"-h0", 21, 22)
	f.traffic("r"+kd.RouteID+"-h0", 31, 32)
	sel := dialHop.Upstreams[1]
	if err := d.SetUpstreams(t.Context(), kd.RouteID, kd.HopIndex, []driver.Upstream{{Address: sel.Address, Port: sel.Port, Weight: 7}}); err != nil {
		t.Fatal(err)
	}
	o0 := observe(t, d)
	seq0, loads0 := recordedSeq(t, cfg)
	inst, applies, structs := f.instance, f.applies, f.structs

	// Observe runs alongside (the race detector checks the locking).
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = d.Observe(t.Context())
			}
		}
	})
	err := d.ReloadCredentials(t.Context())
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if f.instance != inst || f.applies != applies {
		t.Fatal("ReloadCredentials restarted or reloaded gost")
	}
	if f.structs != structs+2 {
		t.Fatalf("ReloadCredentials made %d service changes, want 2 (delete and create the TLS service)", f.structs-structs)
	}
	if seq, loads := recordedSeq(t, cfg); seq != seq0+1 || loads != loads0 {
		t.Fatalf("state seq %d loads %d, want %d %d", seq, loads, seq0+1, loads0)
	}
	retired := r.take()
	if c0 := countersOf(t, o0, kt); len(retired) != 1 || retired[0].GetRouteId() != kt.RouteID || retired[0].GetUpBytes() != 11 ||
		retired[0].GetDownBytes() != 12 || retired[0].GetCounterEpoch() != c0.GetCounterEpoch() {
		t.Fatalf("retired %v, want the TLS hop's 11/12 in epoch %s", retired, c0.GetCounterEpoch())
	}
	o1 := observe(t, d)
	if c := countersOf(t, o1, kt); c.GetCounterEpoch() == countersOf(t, o0, kt).GetCounterEpoch() || c.GetUpBytes() != 0 {
		t.Fatalf("the TLS hop after the reload: %v", c)
	}
	for _, k := range []driver.HopKey{kr, kd} {
		if a, b := countersOf(t, o0, k), countersOf(t, o1, k); a.GetCounterEpoch() != b.GetCounterEpoch() || a.GetUpBytes() != b.GetUpBytes() {
			t.Fatalf("hop %s across the reload: %v -> %v", k, a, b)
		}
	}
	for _, rot := range o1.Rotation {
		if rot.RouteID == kd.RouteID && (len(rot.Active) != 1 || rot.Active[0].Weight != 7 || rot.Active[0].Address != sel.Address) {
			t.Fatalf("the dialling hop's rotation after the reload: %+v", rot.Active)
		}
	}
	// Traffic resumes in the new epoch.
	f.traffic("r"+kt.RouteID+"-h1", 5, 6)
	if c := countersOf(t, observe(t, d), kt); c.GetUpBytes() != 5 || c.GetCounterEpoch() != countersOf(t, o1, kt).GetCounterEpoch() {
		t.Fatalf("the TLS hop's new epoch: %v", c)
	}

	// A key that does not match the certificate: refused, nothing changes.
	good, err := os.ReadFile(cfg.LinkKey)
	if err != nil {
		t.Fatal(err)
	}
	other := linkPKI(t, shortDir(t), "forward-11")["forward-11"]
	bad, err := os.ReadFile(other[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.LinkKey, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	structs = f.structs
	if err := d.ReloadCredentials(t.Context()); err == nil {
		t.Fatal("ReloadCredentials took a key that does not match the certificate")
	}
	if f.structs != structs || f.instance != inst || len(r.take()) != 0 {
		t.Fatal("a refused reload changed gost")
	}
	if err := os.WriteFile(cfg.LinkKey, good, 0o600); err != nil {
		t.Fatal(err)
	}

	// A configuration file that is not the recorded one is refused.
	cf := filepath.Join(cfg.Dir, gost.ConfigFile)
	content, err := os.ReadFile(cf) // #nosec G304 -- the test's own file
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cf, append(content, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.ReloadCredentials(t.Context()); !errors.Is(err, driver.ErrConflict) {
		t.Fatalf("ReloadCredentials over a changed file: %v, want ErrConflict", err)
	}
	if err := os.WriteFile(cf, content, 0o600); err != nil {
		t.Fatal(err)
	}

	// A silent web API: gost restarts on the recorded file and every
	// hop starts a new epoch (no counters to hand over: none answer).
	o2 := observe(t, d)
	f.with(func() { f.apiDown = true })
	if err := d.ReloadCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.with(func() { f.apiDown = false })
	if f.instance != inst+1 {
		t.Fatal("ReloadCredentials did not restart gost when its web API was silent")
	}
	if _, loads := recordedSeq(t, cfg); loads != loads0+1 {
		t.Fatalf("state loads %d after the restart, want %d", loads, loads0+1)
	}
	o3 := observe(t, d)
	for _, k := range []driver.HopKey{kt, kr, kd} {
		if countersOf(t, o2, k).GetCounterEpoch() == countersOf(t, o3, k).GetCounterEpoch() {
			t.Fatalf("hop %s kept its epoch across a restart", k)
		}
	}
	_ = r.take()

	// gost stopped: a start reads the files, nothing to do.
	if err := f.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	applies = f.applies
	if err := d.ReloadCredentials(t.Context()); err != nil || f.applies != applies {
		t.Fatalf("ReloadCredentials with gost stopped: %v, %d starts", err, f.applies-applies)
	}
}

// TestReloadCredentialsMux: a TLS listener with mux makes ReloadCredentials
// restart gost (a re-created mux listener would strand its peers'
// carriers): every hop starts a new epoch and hands its counters over.
func TestReloadCredentialsMux(t *testing.T) {
	var r retiredLog
	d, f, cfg := credsDriver(t, &r)
	tlsHop, rawHop, _ := credsHops(t, true)
	kt, kr := driver.KeyOf(tlsHop), driver.KeyOf(rawHop)
	apply(t, d, 1, tlsHop, rawHop)
	f.traffic("r"+kt.RouteID+"-h1", 11, 12)
	f.traffic("r"+kr.RouteID+"-h0", 21, 22)
	o0 := observe(t, d)
	_, loads0 := recordedSeq(t, cfg)
	inst := f.instance
	if err := d.ReloadCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.instance != inst+1 {
		t.Fatal("ReloadCredentials did not restart gost for a mux listener")
	}
	if _, loads := recordedSeq(t, cfg); loads != loads0+1 {
		t.Fatalf("state loads %d, want %d", loads, loads0+1)
	}
	retired := r.take()
	if len(retired) != 2 || retired[0].GetUpBytes() != 11 || retired[1].GetUpBytes() != 21 {
		t.Fatalf("retired %v, want both hops' counters", retired)
	}
	o1 := observe(t, d)
	for _, k := range []driver.HopKey{kt, kr} {
		if c := countersOf(t, o1, k); c.GetCounterEpoch() == countersOf(t, o0, k).GetCounterEpoch() || c.GetUpBytes() != 0 {
			t.Fatalf("hop %s after the restart: %v", k, c)
		}
	}
}

// TestReloadCredentialsWithoutLinkRoutes: without a route that uses the
// link certificate there is nothing to reload, whatever the files hold.
func TestReloadCredentialsWithoutLinkRoutes(t *testing.T) {
	var r retiredLog
	d, f, cfg := credsDriver(t, &r)
	_, rawHop, _ := credsHops(t, false)
	apply(t, d, 1, rawHop)
	if err := os.WriteFile(cfg.LinkKey, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	inst, structs := f.instance, f.structs
	if err := d.ReloadCredentials(t.Context()); err != nil || f.instance != inst || f.structs != structs {
		t.Fatalf("ReloadCredentials without link routes: %v", err)
	}
}

// TestNetnsReloadCredentials renews the link certificates of a real gost
// relay (RAW in, mutual TLS out) and exit (TLS in, and a RAW route of its
// own) in place and calls ReloadCredentials on each.
//
// Over TLS and QUIC links the exit re-creates its TLS service through the
// web API, without a reload or restart: it serves the new certificate,
// its TLS hop starts a new counter epoch and its last counters reach the
// hook, its RAW route keeps its epoch and connection, the established
// TCP connection through relay and exit survives (TLS), and new
// connections pass. Over a TLS link with mux the exit restarts instead
// (a re-created mux listener would strand the relay's carrier, so new
// connections through it would hang): every hop starts a new epoch, every
// hop's counters reach the hook, established connections end, and the
// relay dials a new carrier. Either way the relay, whose dialer alone
// uses the certificate, keeps its epoch, connection and rotation.
func TestNetnsReloadCredentials(t *testing.T) {
	for _, tc := range []struct {
		name    string
		link    *forwardv1.LinkTransport
		restart bool
	}{
		{"tls", &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS, ServerName: "forward-42"}, false},
		{"wss", &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_WSS, ServerName: "forward-42"}, false},
		{"grpc", &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_GRPC, ServerName: "forward-42"}, false},
		{"quic", &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_QUIC, ServerName: "forward-42"}, true},
		{"tls-mux", &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS, Mux: true, ServerName: "forward-42"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) { testNetnsReloadCredentials(t, tc.link, tc.restart) })
	}
}

func testNetnsReloadCredentials(t *testing.T, link *forwardv1.LinkTransport, restart bool) {
	ns := newNetns(t)
	for _, a := range []string{"10.234.0.1", "10.234.0.2", "10.234.0.10"} {
		ns.addAddress(t, a)
	}
	pki := linkPKI(t, shortDir(t), "forward-32", "forward-42")
	relay, exit := newNode(t, ns, "forward-32", pki), newNode(t, ns, "forward-42", pki)
	startEcho(t, ns, "10.234.0.10:7000")

	const route, rawRoute = "01JF4D000000000000000000A1", "01JF4D000000000000000000B1"
	exitHop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 2, Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: gostE,
		Listen:  &forwardv1.Listen{Address: "10.234.0.2", Port: 20000, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Ingress: link,
		Upstreams: []*forwardv1.Upstream{{
			Address: "10.234.0.10", Port: 7000, Weight: 1,
			Egress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		}},
		Balance:        failover,
		TargetPolicy:   forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		IngressSources: []string{"10.234.0.0/24"},
		IngressPeers:   []string{"spiffe://anixops/example/agent/forward-32"},
		Mark:           1,
	}
	rawHop := &forwardv1.NodeHop{
		RouteId: rawRoute, HopIndex: 0, Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: gostE,
		Listen:       &forwardv1.Listen{Address: "10.234.0.2", Port: 30005, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Upstreams:    []*forwardv1.Upstream{{Address: "10.234.0.10", Port: 7000}},
		TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
	}
	relayHop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 1, Role: forwardv1.HopRole_HOP_ROLE_RELAY, Engine: gostE,
		Listen:  &forwardv1.Listen{Address: "10.234.0.1", Port: 30001, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		Upstreams: []*forwardv1.Upstream{{
			Address: "10.234.0.2", Port: 20000, Weight: 1, Egress: link,
			NodeRef: "forward-42", PeerIdentity: "spiffe://anixops/example/agent/forward-42",
		}},
		Balance:        failover,
		TargetPolicy:   forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		IngressSources: []string{"10.234.0.0/24"},
		Mark:           1,
	}
	var exitRetired, relayRetired retiredLog
	ed, err := gost.New(exit.cfg, gost.WithRunner(exit.run), gost.WithSupervisor(exit.sup), gost.WithRetiredCounters(exitRetired.add))
	if err != nil {
		t.Fatal(err)
	}
	rd, err := gost.New(relay.cfg, gost.WithRunner(relay.run), gost.WithSupervisor(relay.sup), gost.WithRetiredCounters(relayRetired.add))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ed.Apply(t.Context(), render(t, ed, conformance.State("forward-42", 1, exitHop, rawHop))); err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Apply(t.Context(), render(t, rd, conformance.State("forward-32", 1, relayHop))); err != nil {
		t.Fatal(err)
	}
	try := func(what string) (string, error) {
		c, err := ns.dial("tcp", "10.234.0.1:30001")
		if err != nil {
			return "", err
		}
		defer func() { _ = c.Close() }()
		return exchange(c, what)
	}
	through := func(what string) {
		t.Helper()
		if got, err := try(what); err != nil || got != "echo:"+what {
			t.Fatalf("%s: a new connection through relay and exit answered %q %v", what, got, err)
		}
	}
	held, err := ns.dial("tcp", "10.234.0.1:30001")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	if got, err := exchange(held, "one"); err != nil || got != "echo:one" {
		t.Fatalf("TCP through relay and exit: %q %v", got, err)
	}
	direct, err := ns.dial("tcp", "10.234.0.2:30005")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = direct.Close() }()
	if got, err := exchange(direct, "raw"); err != nil || got != "echo:raw" {
		t.Fatalf("the exit's RAW route: %q %v", got, err)
	}
	// The relay runs a selection (SetUpstreams) that must survive.
	if err := rd.SetUpstreams(t.Context(), route, 1, []driver.Upstream{{Address: "10.234.0.2", Port: 20000, Weight: 5}}); err != nil {
		t.Fatal(err)
	}
	through("selected")

	ke, kraw, kr := driver.KeyOf(exitHop), driver.KeyOf(rawHop), driver.KeyOf(relayHop)
	before := observe(t, ed)
	e0, raw0 := countersOf(t, before, ke), countersOf(t, before, kraw)
	r0 := countersOf(t, observe(t, rd), kr)
	// gost counts a QUIC carrier's streams as no connections.
	quic := link.GetSecurity() == forwardv1.LinkSecurity_LINK_SECURITY_QUIC
	if e0.GetUpBytes() == 0 || !quic && e0.GetTotalConns() < 2 || raw0.GetTotalConns() != 1 {
		t.Fatalf("counters before the renewal: exit %v, raw %v", e0, raw0)
	}

	// served answers the serial of the certificate the exit presents.
	served := func() int64 {
		t.Helper()
		pair, err := tls.LoadX509KeyPair(pki["forward-32"][0], pki["forward-32"][1])
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		ca, _ := os.ReadFile(pki["forward-42"][2]) // #nosec G304 -- the test's CA
		pool.AppendCertsFromPEM(ca)
		raw, err := ns.dial("tcp", "10.234.0.2:20000")
		if err != nil {
			t.Fatal(err)
		}
		c := tls.Client(raw, &tls.Config{ServerName: "forward-42", RootCAs: pool, Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
		defer func() { _ = c.Close() }()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		if err := c.Handshake(); err != nil {
			t.Fatal(err)
		}
		return c.ConnectionState().PeerCertificates[0].SerialNumber.Int64()
	}
	// (The test has no QUIC client: over QUIC, traffic shows the renewal.)
	var oldSerial int64
	if !quic {
		oldSerial = served()
	}

	// Renew the exit's certificate in place and reload it.
	pid, loads := exit.sup.pid(t), exit.sup.applies
	issueLink(t, pki["forward-42"], "forward-42", 100)
	if err := ed.ReloadCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	if restarted := exit.sup.pid(t) != pid || exit.sup.applies != loads; restarted != restart {
		t.Fatalf("ReloadCredentials restarted gost: %v, want %v", restarted, restart)
	}
	if restart && exit.sup.applies != loads+1 {
		t.Fatalf("ReloadCredentials started gost %d times", exit.sup.applies-loads)
	}
	if !quic {
		if s := served(); s != 100 {
			t.Fatalf("the exit presents serial %d after the renewal (was %d), want 100", s, oldSerial)
		}
	}
	retired := exitRetired.take()
	want := map[driver.HopKey]*forwardv1.Counters{ke: e0}
	if restart {
		want[kraw] = raw0
	}
	if len(retired) != len(want) {
		t.Fatalf("retired %v, want the counters of %d hops", retired, len(want))
	}
	for _, r := range retired {
		w := want[driver.HopKey{RouteID: r.GetRouteId(), HopIndex: r.GetHopIndex()}]
		if w == nil || r.GetCounterEpoch() != w.GetCounterEpoch() || r.GetUpBytes() < w.GetUpBytes() || r.GetTotalConns() < w.GetTotalConns() {
			t.Fatalf("retired %v, want %v", r, w)
		}
	}
	after := observe(t, ed)
	if e1 := countersOf(t, after, ke); e1.GetCounterEpoch() == e0.GetCounterEpoch() {
		t.Fatalf("the exit hop kept its epoch %s across the renewal", e1.GetCounterEpoch())
	}
	raw1 := countersOf(t, after, kraw)
	if restart {
		if raw1.GetCounterEpoch() == raw0.GetCounterEpoch() {
			t.Fatal("the exit's RAW route kept its epoch across a restart")
		}
		// The restart ended the established connections, the relay's
		// carrier among them: a connection that raced the relay noticing
		// may fail, then the relay dials a new carrier. A QUIC carrier
		// gets no close from the stopped gost (no stateless reset): the
		// relay drops it at its idle timeout (quic-go's 30 s default).
		start := time.Now()
		deadline := start.Add(5 * time.Second)
		if quic {
			deadline = start.Add(45 * time.Second)
		}
		for {
			got, err := try("after-exit")
			if err == nil && got == "echo:after-exit" {
				t.Logf("the relay passed new connections %v after the exit's restart", time.Since(start).Round(time.Second))
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("no connection through relay and exit after the exit's restart: %q %v", got, err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	} else {
		if raw1.GetCounterEpoch() != raw0.GetCounterEpoch() || raw1.GetTotalConns() != raw0.GetTotalConns() {
			t.Fatalf("the exit's RAW route: %v -> %v", raw0, raw1)
		}
		if got, err := exchange(direct, "raw2"); err != nil || got != "echo:raw2" {
			t.Fatalf("the RAW route's connection after the renewal: %q %v", got, err)
		}
		if link.GetSecurity() == forwardv1.LinkSecurity_LINK_SECURITY_TLS {
			if got, err := exchange(held, "two"); err != nil || got != "echo:two" {
				t.Fatalf("established TCP connection after the exit's renewal: %q %v", got, err)
			}
		}
		through("after-exit")
	}
	if e2 := countersOf(t, observe(t, ed), ke); !quic && e2.GetTotalConns() < 1 || e2.GetUpBytes() == 0 {
		t.Fatalf("no traffic counted in the exit hop's new epoch: %v", e2)
	}
	if restart || link.GetSecurity() != forwardv1.LinkSecurity_LINK_SECURITY_TLS {
		// The held connection is gone (or, over QUIC, its link may be):
		// hold a new one for the relay's renewal.
		_ = held.Close()
		if held, err = ns.dial("tcp", "10.234.0.1:30001"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.Close() }()
		if got, err := exchange(held, "two"); err != nil || got != "echo:two" {
			t.Fatalf("a new held connection: %q %v", got, err)
		}
		r0 = countersOf(t, observe(t, rd), kr)
	}

	// Renew the relay's: only its hop (the dialer) changes, so its epoch,
	// its counters and its selection stay.
	pid, loads = relay.sup.pid(t), relay.sup.applies
	issueLink(t, pki["forward-32"], "forward-32", 101)
	if err := rd.ReloadCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	if relay.sup.pid(t) != pid || relay.sup.applies != loads {
		t.Fatal("ReloadCredentials reloaded or restarted the relay's gost")
	}
	if r := relayRetired.take(); len(r) != 0 {
		t.Fatalf("the relay retired %v although no service was re-created", r)
	}
	o := observe(t, rd)
	if r1 := countersOf(t, o, kr); r1.GetCounterEpoch() != r0.GetCounterEpoch() || r1.GetTotalConns() < r0.GetTotalConns() {
		t.Fatalf("the relay's counters across its renewal: %v -> %v", r0, r1)
	}
	if rot := o.Rotation; len(rot) != 1 || len(rot[0].Active) != 1 || rot[0].Active[0].Weight != 5 {
		t.Fatalf("the relay's rotation after its renewal: %+v", rot)
	}
	through("after-relay")
	if got, err := exchange(held, "three"); err != nil || got != "echo:three" {
		t.Fatalf("established TCP connection after the relay's renewal: %q %v", got, err)
	}
}
