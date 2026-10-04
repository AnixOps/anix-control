package gost_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// TestApplyRestartsForMuxService: an apply that would re-create a running
// service with a mux listener restarts gost instead of changing it
// through the web API (the carriers the listener accepted would outlive
// it): every hop starts a new epoch and hands its counters over, and the
// state records the load. An apply that changes other services, or only
// adds services, beside the mux listener goes through the web API.
func TestApplyRestartsForMuxService(t *testing.T) {
	var r retiredLog
	d, f, cfg := credsDriver(t, &r)
	muxHop, rawHop, dialHop := credsHops(t, true)
	km, kr := driver.KeyOf(muxHop), driver.KeyOf(rawHop)
	apply(t, d, 1, muxHop, rawHop)
	f.traffic("r"+km.RouteID+"-h1", 11, 12)
	f.traffic("r"+kr.RouteID+"-h0", 21, 22)
	o0 := observe(t, d)
	_, loads0 := recordedSeq(t, cfg)
	inst, structs := f.instance, f.structs

	// A limit adds a connection limiter to the mux hop's service.
	muxHop.Limits = &forwardv1.Limits{MaxConns: 100}
	apply(t, d, 2, muxHop, rawHop)
	if f.instance != inst+1 || f.structs != structs {
		t.Fatalf("re-creating a mux service: %d starts, %d service changes; want a restart", f.instance-inst, f.structs-structs)
	}
	if _, loads := recordedSeq(t, cfg); loads != loads0+1 {
		t.Fatalf("state loads %d, want %d", loads, loads0+1)
	}
	retired := r.take()
	if len(retired) != 2 || retired[0].GetUpBytes() != 11 || retired[1].GetUpBytes() != 21 {
		t.Fatalf("retired %v, want both hops' counters", retired)
	}
	o1 := observe(t, d)
	for _, k := range []driver.HopKey{km, kr} {
		if c := countersOf(t, o1, k); c.GetCounterEpoch() == countersOf(t, o0, k).GetCounterEpoch() {
			t.Fatalf("hop %s kept its epoch across the restart", k)
		}
	}

	// Re-creating the RAW hop's service and adding a hop leave the mux
	// listener alone: the web API does it.
	inst, structs = f.instance, f.structs
	rawHop.Limits = &forwardv1.Limits{MaxConns: 100}
	apply(t, d, 3, muxHop, rawHop, dialHop)
	if f.instance != inst || f.structs != structs+3 {
		t.Fatalf("changes beside a mux listener: %d starts, %d service changes; want 0 and 3", f.instance-inst, f.structs-structs)
	}
	if c := countersOf(t, observe(t, d), km); c.GetCounterEpoch() != countersOf(t, o1, km).GetCounterEpoch() {
		t.Fatal("the mux hop's epoch ended although its service ran on")
	}

	// Removing the mux hop deletes its service: a restart.
	apply(t, d, 4, rawHop, dialHop)
	if f.instance != inst+1 {
		t.Fatal("deleting a mux service did not restart gost")
	}
}

// TestApplyFallbackRestartsMuxNode: without the web API a changing apply
// restarts a gost that runs a mux listener instead of reloading it (a
// reload re-creates every service); TestApplyFallsBackToReload has a node
// without one reload.
func TestApplyFallbackRestartsMuxNode(t *testing.T) {
	d, f, _ := credsDriver(t, &retiredLog{})
	muxHop, rawHop, _ := credsHops(t, true)
	apply(t, d, 1, muxHop, rawHop)
	inst, applies := f.instance, f.applies
	f.with(func() { f.apiDown = true })
	rawHop.Limits = &forwardv1.Limits{MaxConns: 100}
	apply(t, d, 2, muxHop, rawHop)
	f.with(func() { f.apiDown = false })
	if f.instance != inst+1 || f.applies != applies+1 {
		t.Fatalf("fallback on a mux node: %d starts of %d starts or reloads; want one restart", f.instance-inst, f.applies-applies)
	}
}

// muxRecovery bounds how long after an exit changed its mux listener new
// connections through the relay may fail: the mux keepalive timeout (the
// longest a silent carrier lives) and slack. A restarted exit closes the
// relay's carrier at once, so they pass within about a second.
const muxRecovery = 30*time.Second + 10*time.Second

// TestNetnsMuxRestart: a real gost relay dials a real gost exit over TLS
// with mux; the exit's mux service changes structurally (a connection
// limit), through Apply with the web API ("api") and through the fallback
// without it ("fallback"). Either way the exit restarts (deleting the
// service through the web API, or a reload, would close its listening
// socket only, and the relay would keep opening streams on its carrier
// that nothing accepts, so new connections would hang), and new
// connections through the relay pass again within muxRecovery.
func TestNetnsMuxRestart(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		name := "api"
		if fallback {
			name = "fallback"
		}
		t.Run(name, func(t *testing.T) { testNetnsMuxRestart(t, fallback) })
	}
}

func testNetnsMuxRestart(t *testing.T, fallback bool) {
	ns := newNetns(t)
	for _, a := range []string{"10.234.0.1", "10.234.0.2", "10.234.0.10"} {
		ns.addAddress(t, a)
	}
	pki := linkPKI(t, shortDir(t), "forward-32", "forward-42")
	relay, exit := newNode(t, ns, "forward-32", pki), newNode(t, ns, "forward-42", pki)
	startEcho(t, ns, "10.234.0.10:7000")

	const route = "01JF4D000000000000000000A1"
	link := &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS, Mux: true, ServerName: "forward-42"}
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
	ed, rd := exit.driver(t), relay.driver(t)
	if _, err := ed.Apply(t.Context(), render(t, ed, conformance.State("forward-42", 1, exitHop))); err != nil {
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
	// Two connections: both streams of the relay's one carrier.
	for _, what := range []string{"one", "two"} {
		if got, err := try(what); err != nil || got != "echo:"+what {
			t.Fatalf("through relay and exit: %q %v", got, err)
		}
	}

	if fallback {
		// gost's web API no longer answers: Apply cannot use it.
		if err := os.Remove(filepath.Join(exit.cfg.RuntimeDir, gost.APISocket)); err != nil {
			t.Fatal(err)
		}
	}
	pid, relayPID := exit.sup.pid(t), relay.sup.pid(t)
	exitHop.Limits = &forwardv1.Limits{MaxConns: 100}
	if _, err := ed.Apply(t.Context(), render(t, ed, conformance.State("forward-42", 2, exitHop))); err != nil {
		t.Fatal(err)
	}
	if exit.sup.pid(t) == pid {
		t.Fatal("the exit's gost was not restarted")
	}
	if relay.sup.pid(t) != relayPID {
		t.Fatal("the relay's gost restarted")
	}
	start := time.Now()
	for {
		got, err := try("after")
		if err == nil && got == "echo:after" {
			break
		}
		if time.Since(start) > muxRecovery {
			t.Fatalf("no connection through relay and exit %v after the exit's change: %q %v", muxRecovery, got, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("new connections through the relay passed %v after the exit's change", time.Since(start).Round(time.Millisecond))
	if got, err := try("again"); err != nil || got != "echo:again" {
		t.Fatalf("a second connection after the change: %q %v", got, err)
	}
}
