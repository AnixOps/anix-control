package gost_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
)

var update = flag.Bool("update", false, "rewrite the gost goldens in contracts/forward/v1/gost from the cases in golden_test.go")

// goldenDir holds, per case, the input state (<case>.state.json), the
// rendered configuration (<case>.json) and, when the render rejected hops,
// their errors (<case>.errors.txt); and the unit file (anixops-gost.service).
const goldenDir = "../../../../contracts/forward/v1/gost"

// fixtureDir holds the planner goldens.
const fixtureDir = "../../../../contracts/forward/v1"

type goldenCase struct {
	name  string
	cfg   func(*gost.Config)
	state *forwardv1.NodeForwardState
}

const (
	gostE = forwardv1.Engine_ENGINE_GOST
	tcp   = forwardv1.L4Protocol_L4_PROTOCOL_TCP
	udp   = forwardv1.L4Protocol_L4_PROTOCOL_UDP
	both  = forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP

	rr       = forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN
	random   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM
	ipHash   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH
	leastC   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN
	failover = forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER

	secRAW  = forwardv1.LinkSecurity_LINK_SECURITY_RAW
	secTLS  = forwardv1.LinkSecurity_LINK_SECURITY_TLS
	secWSS  = forwardv1.LinkSecurity_LINK_SECURITY_WSS
	secQUIC = forwardv1.LinkSecurity_LINK_SECURITY_QUIC
	secGRPC = forwardv1.LinkSecurity_LINK_SECURITY_GRPC
)

// testConfig is the default configuration with a version, so the driver is
// available, and the default link certificate paths.
func testConfig() gost.Config {
	c := gost.DefaultConfig()
	c.Version = "gost (static test configuration)"
	return c
}

func newDriver(t testing.TB, mod func(*gost.Config)) *gost.Driver {
	t.Helper()
	c := testConfig()
	if mod != nil {
		mod(&c)
	}
	d, err := gost.New(c)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// builder makes hops on the conformance suite's default topology.
func builder(t testing.TB) conformance.Builder {
	t.Helper()
	caps, err := newDriver(t, nil).Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return conformance.Builder{Engine: gostE, Caps: caps, Top: conformance.DefaultTopology()}
}

func goldenCases(t testing.TB) []goldenCase {
	b := builder(t)
	v4, v6 := b.Top.UpstreamsV4, b.Top.UpstreamsV6
	state := func(hops ...*forwardv1.NodeHop) *forwardv1.NodeForwardState {
		return conformance.State(b.Top.NodeRef, 1, hops...)
	}
	entry := func(p forwardv1.L4Protocol, s forwardv1.BalanceStrategy, ups []*forwardv1.Upstream) *forwardv1.NodeHop {
		return b.Entry(conformance.RouteA, 0, p, s, ups)
	}
	ups := func(addrs []string, n int) []*forwardv1.Upstream { return b.Upstreams(addrs, n) }

	failoverTied := entry(tcp, failover, ups(v4, 3))
	failoverTied.Upstreams[1].Priority = 0
	failoverTied.Upstreams[2].Priority = 0

	equalWeights := entry(tcp, rr, ups(v4, 3))
	for _, u := range equalWeights.Upstreams {
		u.Weight = 5
	}

	listenAddr := entry(both, rr, append(ups(v4, 2), ups(v6, 2)...))
	listenAddr.Listen.Address = "2001:db8::11"

	relay := b.Relay(conformance.RouteB, 1, ups(v4, 2))
	relay.IngressSources = []string{"198.51.100.0/24", "198.51.100.7", "2001:db8:100::/64", "::ffff:203.0.113.9"}
	exit := b.Relay(conformance.RouteC, 2, append(ups(v4, 1), ups(v6, 1)...))
	exit.HopIndex, exit.Role = 2, forwardv1.HopRole_HOP_ROLE_EXIT
	exit.IngressSources = []string{"198.51.100.7"}

	limits := entry(both, rr, ups(v4, 2))
	limits.Limits = &forwardv1.Limits{BandwidthBps: 100_000_000, MaxConns: 2000, ExpiresAtUnixMs: 1798761600000}

	paused := b.Simple(conformance.RouteA, 0)
	paused.Paused = true
	paused.Limits = &forwardv1.Limits{MaxConns: 10}
	pausedRelay := b.Relay(conformance.RouteC, 2, ups(v4, 1))
	pausedRelay.Paused = true

	breaker := entry(tcp, rr, ups(v4, 2))
	breaker.CircuitBreaker = &forwardv1.CircuitBreaker{FailureThreshold: 5, OpenMs: 1500}

	// links: an exit terminating each encrypted link, and an entry
	// dialling each, with and without mux.
	var linkHops []*forwardv1.NodeHop
	for i, l := range []*forwardv1.LinkTransport{
		{Security: secTLS},
		{Security: secTLS, Mux: true},
		{Security: secWSS, Path: "/anixops/ws"},
		{Security: secWSS, Mux: true},
		{Security: secQUIC},
		{Security: secGRPC, Path: "/anixops.Tunnel/Relay"},
		{Security: secRAW, Mux: true},
	} {
		in := b.Relay("01JF2A00000000000000000L"+strconv.Itoa(i)+"0", 0, ups(v4, 1))
		in.HopIndex, in.Role = 1, forwardv1.HopRole_HOP_ROLE_EXIT
		in.Listen.Port = 31000 + uint32(i) // #nosec G115 -- small
		in.Listen.Protocol = both
		in.Ingress = proto.Clone(l).(*forwardv1.LinkTransport)
		in.Ingress.ServerName = "forward-11"
		in.IngressPeers = []string{"spiffe://anixops/example/agent/forward-21"}
		out := b.Entry("01JF2A00000000000000000L"+strconv.Itoa(i)+"1", 0, both, rr, ups(v4, 2))
		out.Listen.Port = 32000 + uint32(i) // #nosec G115 -- small
		for j, u := range out.Upstreams {
			u.Egress = proto.Clone(l).(*forwardv1.LinkTransport)
			u.NodeRef = "forward-4" + strconv.Itoa(j)
			u.Egress.ServerName = u.NodeRef
			u.PeerIdentity = "spiffe://anixops/example/agent/" + u.NodeRef
		}
		linkHops = append(linkHops, in, out)
	}

	// hop-errors: one good hop and one of each kind of rejected hop.
	anixops := b.Simple("01JF2A000000000000000000B2", 1)
	anixops.Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS}
	name := b.Simple("01JF2A000000000000000000C2", 2)
	name.Upstreams[0].Address = "target.example.com"
	injected := b.Simple("01JF2A000000000000000000D2", 3)
	injected.RouteId = `x"; "services": [], "y": "`
	loopback := b.Simple("01JF2A000000000000000000E1", 0)
	loopback.Listen.Port = 30005
	loopback.Upstreams = ups([]string{"127.0.0.1"}, 1)
	dupA := b.Simple("01JF2A000000000000000000F1", 0)
	dupA.Listen.Port = 30006
	dupB := b.Simple("01JF2A000000000000000000F2", 0)
	dupB.Listen.Port = 30006
	quota := b.Simple("01JF2A000000000000000000F3", 0)
	quota.Listen.Port = 30008
	quota.Limits = &forwardv1.Limits{QuotaBytes: 1 << 30}
	mixed := b.Simple("01JF2A000000000000000000F4", 0)
	mixed.Listen.Port = 30009
	mixed.Upstreams[1].Egress = &forwardv1.LinkTransport{Security: secTLS, ServerName: "forward-41"}
	mixed.Upstreams[1].NodeRef, mixed.Upstreams[1].PeerIdentity = "forward-41", "spiffe://anixops/example/agent/forward-41"
	openRelay := b.Relay("01JF2A000000000000000000F5", 0, ups(v4, 1))
	openRelay.Listen.Port = 30010
	openRelay.IngressSources = nil
	noPeers := b.Relay("01JF2A000000000000000000F6", 0, ups(v4, 1))
	noPeers.Listen.Port = 30011
	noPeers.Ingress = &forwardv1.LinkTransport{Security: secTLS}
	noIdentity := b.Simple("01JF2A000000000000000000F7", 0)
	noIdentity.Listen.Port = 30012
	for _, u := range noIdentity.Upstreams {
		u.Egress = &forwardv1.LinkTransport{Security: secTLS, ServerName: "forward-41"}
	}
	badPath := b.Simple("01JF2A000000000000000000F8", 0)
	badPath.Listen.Port = 30013
	badPath.Ingress = &forwardv1.LinkTransport{Security: secRAW, Path: "/ws"}

	// no-link-tls: without a link certificate the driver carries RAW only.
	tlsExit := b.Relay(conformance.RouteB, 1, ups(v4, 1))
	tlsExit.Ingress = &forwardv1.LinkTransport{Security: secTLS}
	tlsExit.IngressPeers = []string{"spiffe://anixops/example/agent/forward-21"}

	custom := entry(both, ipHash, append(ups(v4, 3), ups(v6, 2)...))
	custom.Limits = &forwardv1.Limits{BandwidthBps: 10_000_000}

	return append(plannerCases(t), []goldenCase{
		{name: "empty", state: state()},
		{name: "strategy-round-robin", state: state(entry(tcp, rr, ups(v4, 3)))},
		{name: "strategy-round-robin-equal-weights", state: state(equalWeights)},
		{name: "strategy-random", state: state(entry(tcp, random, ups(v4, 3)))},
		{name: "strategy-ip-hash", state: state(entry(tcp, ipHash, ups(v4, 3)))},
		{name: "strategy-least-conn", state: state(entry(tcp, leastC, ups(v4, 3)))},
		{name: "strategy-failover", state: state(entry(tcp, failover, ups(v4, 3)))},
		{name: "strategy-failover-shared-priority", state: state(failoverTied)},
		{name: "udp", state: state(entry(udp, rr, ups(v4, 2)))},
		{name: "ipv6", state: state(entry(tcp, rr, ups(v6, 2)))},
		{name: "dual-stack-tcp-udp", state: state(entry(both, rr, append(ups(v4, 2), ups(v6, 2)...)))},
		{name: "listen-address", state: state(listenAddr)},
		{name: "relay-and-exit-ingress-sources", state: state(relay, exit)},
		{name: "limits", state: state(limits)},
		{name: "circuit-breaker", state: state(breaker)},
		{name: "paused", state: state(paused, b.Simple(conformance.RouteB, 1), pausedRelay)},
		{name: "several-routes", state: state(b.Simple(conformance.RouteA, 0), b.Relay(conformance.RouteB, 1, ups(v4, 2)), b.Simple(conformance.RouteC, 2))},
		{name: "links", state: state(linkHops...)},
		{name: "hop-errors", state: state(b.Simple(conformance.RouteA, 0), anixops, name, injected, loopback, dupA, dupB, quota, mixed, openRelay, noPeers, noIdentity, badPath), cfg: func(c *gost.Config) {
			c.Strategies = []forwardv1.BalanceStrategy{rr, failover}
		}},
		{name: "no-link-tls", state: state(b.Simple(conformance.RouteA, 0), tlsExit), cfg: func(c *gost.Config) {
			c.LinkCert, c.LinkKey, c.LinkCA = "", "", ""
		}},
		{name: "custom-config", state: state(custom), cfg: func(c *gost.Config) {
			c.RuntimeDir = "/run/gost-custom"
			c.LinkCert, c.LinkKey, c.LinkCA = "/etc/anixops/link.pem", "/etc/anixops/link-key.pem", "/etc/anixops/ca.pem"
		}},
	}...)
}

// plannerCases are the planner's own output: every node state with a gost
// hop in the planner goldens (contracts/forward/v1/plan-*.json), named
// plan-<fixture>-<node>, and live planner runs of variants of the
// nftables-entry, gost-relay, gost-exit route (planner-<variant>-<node>),
// one per link security, with mux, UDP, a gost entry with limits and a
// paused route, all with IP targets.
func plannerCases(t testing.TB) []goldenCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(fixtureDir, "plan-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no planner fixtures: %v", err)
	}
	var cases []goldenCase
	add := func(prefix string, states []*forwardv1.NodeForwardState) {
		for _, s := range states {
			if strings.HasPrefix(prefix, "planner-") && s.GetNodeRef() == "forward-42" {
				continue // the second exit renders like the first
			}
			if slices.ContainsFunc(s.GetHops(), func(h *forwardv1.NodeHop) bool { return h.GetEngine() == gostE }) {
				cases = append(cases, goldenCase{name: prefix + "-" + s.GetNodeRef(), state: s})
			}
		}
	}
	var base forwardv1.PlanRouteRequest
	for _, f := range files {
		b, err := os.ReadFile(f) // #nosec G304 -- a planner golden
		if err != nil {
			t.Fatal(err)
		}
		var fx struct {
			Request  json.RawMessage   `json:"request"`
			Response json.RawMessage   `json:"response"`
			States   []json.RawMessage `json:"states"`
		}
		if err := json.Unmarshal(b, &fx); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "plan-"), ".json")
		var states []*forwardv1.NodeForwardState
		if len(fx.Response) > 0 {
			var resp forwardv1.PlanRouteResponse
			if err := protojson.Unmarshal(fx.Response, &resp); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			states = resp.GetStates()
		}
		for _, raw := range fx.States {
			var s forwardv1.NodeForwardState
			if err := protojson.Unmarshal(raw, &s); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			states = append(states, &s)
		}
		add("plan-"+name, states)
		if name == "nft-entry-gost-relay-exit-failover" {
			if err := protojson.Unmarshal(fx.Request, &base); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
		}
	}
	if base.GetRoute() == nil {
		t.Fatal("no plan-nft-entry-gost-relay-exit-failover.json")
	}

	variant := func(name string, mod func(*forwardv1.PlanRouteRequest)) {
		req := proto.Clone(&base).(*forwardv1.PlanRouteRequest)
		// IP targets: the Agent resolves names before Render.
		req.Route.Targets[1].Host = "198.51.100.8"
		mod(req)
		resp, err := planner.PlanRoute(req, nil, nil, planner.Options{Cluster: "example"})
		if err != nil || len(resp.GetViolations()) > 0 {
			t.Fatalf("planning variant %s: %v %v", name, err, resp.GetViolations())
		}
		add("planner-"+name, resp.GetStates())
	}
	exitLink := func(l *forwardv1.LinkTransport) func(*forwardv1.PlanRouteRequest) {
		return func(r *forwardv1.PlanRouteRequest) { r.Route.Hops[2].Ingress = l }
	}
	variant("relay-tls-mux", func(*forwardv1.PlanRouteRequest) {})
	variant("relay-tls", exitLink(&forwardv1.LinkTransport{Security: secTLS}))
	variant("relay-wss", exitLink(&forwardv1.LinkTransport{Security: secWSS, Path: "/anixops"}))
	variant("relay-quic", exitLink(&forwardv1.LinkTransport{Security: secQUIC}))
	variant("relay-grpc", exitLink(&forwardv1.LinkTransport{Security: secGRPC}))
	variant("relay-raw-mux", exitLink(&forwardv1.LinkTransport{Security: secRAW, Mux: true}))
	variant("relay-tls-udp", func(r *forwardv1.PlanRouteRequest) {
		r.Route.Listen.Protocol = both
		r.Route.Hops[2].Ingress = &forwardv1.LinkTransport{Security: secTLS}
	})
	variant("relay-round-robin-paused", func(r *forwardv1.PlanRouteRequest) {
		r.Route.Paused = true
		r.Route.Policy.NextHop, r.Route.Policy.Target = rr, rr
		r.Route.Targets[0].Weight = 3
	})
	variant("gost-entry-limits", func(r *forwardv1.PlanRouteRequest) {
		r.Route.Hops[0].Engine = gostE
		r.Route.Hops[1].Ingress = &forwardv1.LinkTransport{Security: secTLS}
		for _, n := range r.Nodes {
			if n.GetNodeRef() == "forward-21" {
				n.Engines = append(n.Engines, proto.Clone(r.Nodes[1].Engines[0]).(*forwardv1.EngineCapabilities))
			}
		}
	})
	return cases
}

// stateJSON is the canonical JSON of a state: protojson re-indented, since
// protojson's own spacing is deliberately unstable.
func stateJSON(t testing.TB, s *forwardv1.NodeForwardState) []byte {
	t.Helper()
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		t.Fatal(err)
	}
	out.WriteByte('\n')
	return out.Bytes()
}

func errorsText(err error) []byte {
	var re *driver.RenderError
	if !errors.As(err, &re) {
		return nil
	}
	var b strings.Builder
	for _, h := range re.Hops {
		b.WriteString(h.Error())
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// checkGolden compares want with the golden file (or rewrites it with
// -update; an empty want removes it).
func checkGolden(t *testing.T, file string, want []byte) {
	t.Helper()
	path := filepath.Join(goldenDir, file)
	if *update {
		if len(want) == 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			return
		}
		if err := os.MkdirAll(goldenDir, 0o755); err != nil { // #nosec G301 -- a checked-in directory
			t.Fatal(err)
		}
		if err := os.WriteFile(path, want, 0o644); err != nil { // #nosec G306 -- a checked-in golden
			t.Fatal(err)
		}
		return
	}
	got, rerr := os.ReadFile(path) // #nosec G304 -- a golden of this test
	if len(want) == 0 {
		if rerr == nil {
			t.Errorf("%s exists but the case renders none; run with -update", file)
		}
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs (run with -update to rewrite):\n%s", file, firstDiff(got, want))
	}
}

// TestGoldens renders every case and compares the configuration with
// contracts/forward/v1/gost/<case>.json, the input with <case>.state.json
// and the hop errors with <case>.errors.txt. Run with -update to rewrite
// them.
func TestGoldens(t *testing.T) {
	cases := goldenCases(t)
	names := map[string]bool{}
	for _, c := range cases {
		if names[c.name] {
			t.Fatalf("case %s twice", c.name)
		}
		names[c.name] = true
		t.Run(c.name, func(t *testing.T) {
			d := newDriver(t, c.cfg)
			a, err := d.Render(c.state)
			var re *driver.RenderError
			if err != nil && !errors.As(err, &re) {
				t.Fatalf("Render: %v", err)
			}
			if err := a.Verify(gostE); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(c.name, "plan") && err != nil {
				// The planner's states render, except for target names,
				// which the Agent resolves before Render.
				for _, h := range re.Hops {
					if !errors.Is(h, driver.ErrUnsupported) || !strings.Contains(h.Error(), "is a name") {
						t.Fatalf("the planner's state does not render: %v", h)
					}
				}
			}
			files := map[string][]byte{
				c.name + ".state.json": stateJSON(t, c.state),
				c.name + ".json":       a.Content,
				c.name + ".errors.txt": errorsText(err),
			}
			for _, f := range slices.Sorted(maps.Keys(files)) {
				if f == c.name+".state.json" && !*update {
					got, rerr := os.ReadFile(filepath.Join(goldenDir, f)) // #nosec G304 -- a golden of this test
					if rerr == nil {
						var s forwardv1.NodeForwardState
						if err := protojson.Unmarshal(got, &s); err != nil {
							t.Fatalf("%s: %v", f, err)
						}
						if !proto.Equal(&s, c.state) {
							t.Errorf("%s differs from the case; run with -update", f)
						}
						continue
					}
				}
				checkGolden(t, f, files[f])
			}
		})
	}
	t.Run("unit", func(t *testing.T) {
		u, err := gost.UnitFile(gost.DefaultUnit())
		if err != nil {
			t.Fatal(err)
		}
		checkGolden(t, gost.UnitName, u)
	})
	// Every file in the directory belongs to a case.
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		base := e.Name()
		if base == gost.UnitName {
			continue
		}
		for _, ext := range []string{".state.json", ".errors.txt", ".json"} {
			base = strings.TrimSuffix(base, ext)
		}
		if !names[base] {
			t.Errorf("%s/%s belongs to no case", goldenDir, e.Name())
		}
	}
}

func firstDiff(got, want []byte) string {
	g, w := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return "line " + strconv.Itoa(i+1) + ":\n  got  " + gl + "\n  want " + wl
		}
	}
	return "(no line differs)"
}
