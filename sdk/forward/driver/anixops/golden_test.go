package anixops_test

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
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
)

var update = flag.Bool("update", false, "rewrite the anixops goldens in contracts/forward/v1/anixops from the cases in golden_test.go")

// goldenDir holds, per case, the input state (<case>.state.json), the
// rendered configuration (<case>.json) and, when the render rejected hops,
// their errors (<case>.errors.txt); and the unit file (anixops-relay.service).
const goldenDir = "../../../../contracts/forward/v1/anixops"

// fixtureDir holds the planner goldens.
const fixtureDir = "../../../../contracts/forward/v1"

type goldenCase struct {
	name  string
	cfg   func(*anixops.Config)
	state *forwardv1.NodeForwardState
}

const (
	tcp  = forwardv1.L4Protocol_L4_PROTOCOL_TCP
	udp  = forwardv1.L4Protocol_L4_PROTOCOL_UDP
	both = forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP

	rr       = forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN
	random   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM
	ipHash   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH
	leastC   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN
	failover = forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER

	secRAW     = forwardv1.LinkSecurity_LINK_SECURITY_RAW
	secTLS     = forwardv1.LinkSecurity_LINK_SECURITY_TLS
	secAnixOps = forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS

	carAuto  = forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_AUTO
	carTLS   = forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP
	carQUIC  = forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC
	carPlain = forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN
)

// testConfig is the default configuration with a version, so the driver is
// available, and the default link file paths.
func testConfig() anixops.Config {
	c := anixops.DefaultConfig()
	c.Version = "anixops-relay (static test configuration)"
	return c
}

func newDriver(t testing.TB, mod func(*anixops.Config)) *anixops.Driver {
	t.Helper()
	c := testConfig()
	if mod != nil {
		mod(&c)
	}
	d, err := anixops.New(c)
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
	return conformance.Builder{Engine: forwardv1.Engine_ENGINE_ANIXOPS, Caps: caps, Top: conformance.DefaultTopology()}
}

const peer = "spiffe://anixops/example/agent/forward-21"

// anixopsLink is an AnixOps transport with a carrier.
func anixopsLink(c forwardv1.AnixOpsCarrier) *forwardv1.LinkTransport {
	return &forwardv1.LinkTransport{Security: secAnixOps, Mux: true, Carrier: c}
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
	limits.Limits = &forwardv1.Limits{BandwidthBps: 100_000_000, QuotaBytes: 1 << 40, MaxConns: 2000, ExpiresAtUnixMs: 1798761600000}

	paused := b.Simple(conformance.RouteA, 0)
	paused.Paused = true
	paused.Limits = &forwardv1.Limits{MaxConns: 10}
	pausedRelay := b.Relay(conformance.RouteC, 2, ups(v4, 1))
	pausedRelay.Paused = true

	breaker := entry(tcp, rr, ups(v4, 2))
	breaker.CircuitBreaker = &forwardv1.CircuitBreaker{FailureThreshold: 5, OpenMs: 1500}

	// links: an exit terminating each carrier, and an entry dialling each.
	var linkHops []*forwardv1.NodeHop
	for i, c := range []forwardv1.AnixOpsCarrier{forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_UNSPECIFIED, carAuto, carTLS, carQUIC, carPlain} {
		in := b.Relay("01JF2A00000000000000000L"+strconv.Itoa(i)+"0", 0, ups(v4, 1))
		in.HopIndex, in.Role = 1, forwardv1.HopRole_HOP_ROLE_EXIT
		in.Listen.Port = 31000 + uint32(i) // #nosec G115 -- small
		in.Listen.Protocol = both
		in.Ingress = anixopsLink(c)
		in.Ingress.ServerName = "forward-11"
		in.IngressPeers = []string{peer, "spiffe://anixops/example/agent/forward-22"}
		if c == carPlain {
			in.IngressPeers = nil
		}
		out := b.Entry("01JF2A00000000000000000L"+strconv.Itoa(i)+"1", 0, both, rr, ups(v4, 2))
		out.Listen.Port = 32000 + uint32(i) // #nosec G115 -- small
		for j, u := range out.Upstreams {
			u.Egress = anixopsLink(c)
			u.NodeRef = "forward-4" + strconv.Itoa(j)
			u.Egress.ServerName = u.NodeRef
			u.PeerIdentity = "spiffe://anixops/example/agent/" + u.NodeRef
		}
		linkHops = append(linkHops, in, out)
	}

	// hop-errors: one good hop and one of each kind of rejected hop.
	tlsLink := b.Simple("01JF2A000000000000000000B2", 1)
	tlsLink.Ingress = &forwardv1.LinkTransport{Security: secTLS}
	name := b.Simple("01JF2A000000000000000000C2", 2)
	name.Upstreams[0].Address = "target.example.com"
	injected := b.Simple("01JF2A000000000000000000D2", 3)
	injected.RouteId = `x"; "hops": [], "y": "`
	loopback := b.Simple("01JF2A000000000000000000E1", 0)
	loopback.Listen.Port = 30005
	loopback.Upstreams = ups([]string{"127.0.0.1"}, 1)
	dupA := b.Simple("01JF2A000000000000000000F1", 0)
	dupA.Listen.Port = 30006
	dupB := b.Simple("01JF2A000000000000000000F2", 0)
	dupB.Listen.Port = 30006
	mixed := b.Simple("01JF2A000000000000000000F4", 0)
	mixed.Listen.Port = 30009
	mixed.Upstreams[1].Egress = anixopsLink(carTLS)
	mixed.Upstreams[1].NodeRef, mixed.Upstreams[1].PeerIdentity = "forward-41", "spiffe://anixops/example/agent/forward-41"
	openRelay := b.Relay("01JF2A000000000000000000F5", 0, ups(v4, 1))
	openRelay.Listen.Port = 30010
	openRelay.IngressSources = nil
	noPeers := b.Relay("01JF2A000000000000000000F6", 0, ups(v4, 1))
	noPeers.Listen.Port = 30011
	noPeers.Ingress = anixopsLink(carAuto)
	noIdentity := b.Simple("01JF2A000000000000000000F7", 0)
	noIdentity.Listen.Port = 30012
	for _, u := range noIdentity.Upstreams {
		u.Egress = anixopsLink(carAuto)
		u.NodeRef = "forward-41"
	}
	badPath := b.Simple("01JF2A000000000000000000F8", 0)
	badPath.Listen.Port = 30013
	badPath.Ingress = &forwardv1.LinkTransport{Security: secRAW, Path: "/ws"}
	rawMux := b.Simple("01JF2A000000000000000000F9", 0)
	rawMux.Listen.Port = 30014
	rawMux.Ingress = &forwardv1.LinkTransport{Security: secRAW, Mux: true}
	proxy := b.Simple("01JF2A000000000000000000FA", 0)
	proxy.Listen.Port = 30015
	proxy.ProxyProtocol = forwardv1.ProxyProtocol_PROXY_PROTOCOL_V2
	carrierOnRaw := b.Simple("01JF2A000000000000000000FB", 0)
	carrierOnRaw.Listen.Port = 30016
	carrierOnRaw.Ingress = &forwardv1.LinkTransport{Security: secRAW, Carrier: carQUIC}

	// no-link-files: without the node's link certificate only PLAIN works.
	plainExit := b.Relay(conformance.RouteB, 1, ups(v4, 1))
	plainExit.Ingress = anixopsLink(carPlain)
	autoExit := b.Relay(conformance.RouteC, 2, ups(v4, 1))
	autoExit.Ingress = anixopsLink(carAuto)
	autoExit.IngressPeers = []string{peer}
	quicExit := b.Relay("01JF2A000000000000000000B3", 2, ups(v4, 1))
	quicExit.Ingress = anixopsLink(carQUIC)
	quicExit.IngressPeers = []string{peer}

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
		{name: "carriers", state: state(linkHops...)},
		{name: "hop-errors", state: state(b.Simple(conformance.RouteA, 0), tlsLink, name, injected, loopback, dupA, dupB, mixed, openRelay, noPeers, noIdentity, badPath, rawMux, proxy, carrierOnRaw), cfg: func(c *anixops.Config) {
			c.Strategies = []forwardv1.BalanceStrategy{rr, failover}
		}},
		{name: "no-link-files", state: state(b.Simple(conformance.RouteA, 0), plainExit, autoExit, quicExit), cfg: func(c *anixops.Config) {
			c.LinkCert, c.LinkKey, c.LinkCA = "", "", ""
			c.Carriers = []forwardv1.AnixOpsCarrier{carPlain}
		}},
		{name: "custom-config", state: state(custom), cfg: func(c *anixops.Config) {
			c.Dir, c.RuntimeDir = "/var/lib/relay-custom", "/run/relay-custom"
			c.LinkCert, c.LinkKey, c.LinkCA = "/etc/anixops/link.pem", "/etc/anixops/link-key.pem", "/etc/anixops/ca.pem"
		}},
	}...)
}

// plannerCases are live planner runs of the anixops fixture's route
// (plan-anixops-experimental.json) on nodes that report the driver's real
// capabilities: the three-hop route with its QUIC and AUTO carriers, a
// PLAIN variant on trusted-link nodes, a UDP variant and a paused one. The
// fixture's PROXY protocol v2 is left out: the relay does not write it yet,
// so the driver does not report it and validation refuses it.
func plannerCases(t testing.TB) []goldenCase {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, "plan-anixops-experimental.json")) // #nosec G304 -- a planner golden
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	var base forwardv1.PlanRouteRequest
	if err := protojson.Unmarshal(fx.Request, &base); err != nil {
		t.Fatal(err)
	}
	caps, err := newDriver(t, nil).Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	variant := func(name string, mod func(*forwardv1.PlanRouteRequest)) {
		req := proto.Clone(&base).(*forwardv1.PlanRouteRequest)
		req.Route.Policy.ProxyProtocol = forwardv1.ProxyProtocol_PROXY_PROTOCOL_UNSPECIFIED
		for _, n := range req.Nodes {
			n.Engines = []*forwardv1.EngineCapabilities{proto.Clone(caps).(*forwardv1.EngineCapabilities)}
		}
		mod(req)
		resp, err := planner.PlanRoute(req, nil, nil, planner.Options{Cluster: "example", EnableAnixOps: true})
		if err != nil || len(resp.GetViolations()) > 0 {
			t.Fatalf("planning variant %s: %v %v", name, err, resp.GetViolations())
		}
		for _, s := range resp.GetStates() {
			cases = append(cases, goldenCase{name: "planner-" + name + "-" + s.GetNodeRef(), state: s})
		}
	}
	variant("quic-and-auto", func(*forwardv1.PlanRouteRequest) {})
	variant("udp", func(r *forwardv1.PlanRouteRequest) { r.Route.Listen.Protocol = both })
	variant("plain-trusted", func(r *forwardv1.PlanRouteRequest) {
		r.Route.Owner = "admin"
		for i := 1; i < len(r.Route.Hops); i++ {
			r.Route.Hops[i].Ingress = &forwardv1.LinkTransport{Security: secAnixOps, Carrier: carPlain}
		}
		for _, n := range r.Nodes {
			n.Labels = map[string]string{"link": "iepl"}
		}
	})
	variant("paused-weighted", func(r *forwardv1.PlanRouteRequest) {
		r.Route.Paused = true
		r.Route.Policy.NextHop, r.Route.Policy.Target = rr, rr
		r.Route.Targets[0].Weight = 3
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
// contracts/forward/v1/anixops/<case>.json, the input with <case>.state.json
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
			if err := a.Verify(forwardv1.Engine_ENGINE_ANIXOPS); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(c.name, "planner-") && err != nil {
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
		u, err := anixops.UnitFile(anixops.DefaultUnit())
		if err != nil {
			t.Fatal(err)
		}
		checkGolden(t, anixops.UnitName, u)
	})
	// Every file in the directory belongs to a case.
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		base := e.Name()
		if base == anixops.UnitName {
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
