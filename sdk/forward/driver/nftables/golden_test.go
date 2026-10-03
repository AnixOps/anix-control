package nftables_test

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
	"github.com/AnixOps/anix-control/sdk/forward/driver/nftables"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
)

var update = flag.Bool("update", false, "rewrite the nft goldens in contracts/forward/v1/nft from the cases in golden_test.go")

// goldenDir holds, per case, the input state (<case>.state.json), the
// rendered script (<case>.nft) and, when the render rejected hops, their
// errors (<case>.errors.txt).
const goldenDir = "../../../../contracts/forward/v1/nft"

type goldenCase struct {
	name  string
	cfg   func(*nftables.Config)
	state *forwardv1.NodeForwardState
}

const (
	nftE = forwardv1.Engine_ENGINE_NFTABLES
	tcp  = forwardv1.L4Protocol_L4_PROTOCOL_TCP
	udp  = forwardv1.L4Protocol_L4_PROTOCOL_UDP
	both = forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP

	rr       = forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN
	random   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM
	ipHash   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH
	leastC   = forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN
	failover = forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER
)

// testConfig is the default configuration with a version, so the driver is
// available.
func testConfig() nftables.Config {
	c := nftables.DefaultConfig()
	c.Version = "nft (static test configuration)"
	return c
}

func newDriver(t testing.TB, mod func(*nftables.Config)) *nftables.Driver {
	t.Helper()
	c := testConfig()
	if mod != nil {
		mod(&c)
	}
	d, err := nftables.New(c)
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
	return conformance.Builder{Engine: nftE, Caps: caps, Top: conformance.DefaultTopology()}
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

	listenAddr := entry(both, rr, append(ups(v4, 2), ups(v6, 2)...))
	listenAddr.Listen.Address = "2001:db8::11"

	relay := b.Relay(conformance.RouteB, 1, ups(v4, 2))
	relay.IngressSources = []string{"198.51.100.0/24", "198.51.100.7", "2001:db8:100::/64", "::ffff:203.0.113.9"}
	exit := b.Relay(conformance.RouteC, 2, append(ups(v4, 1), ups(v6, 1)...))
	exit.HopIndex, exit.Role = 2, forwardv1.HopRole_HOP_ROLE_EXIT
	exit.IngressSources = []string{"198.51.100.7"}

	limits := entry(tcp, rr, ups(v4, 2))
	limits.Limits = &forwardv1.Limits{BandwidthBps: 100_000_000, QuotaBytes: 1 << 40, MaxConns: 2000}

	paused := b.Simple(conformance.RouteA, 0)
	paused.Paused = true
	paused.Limits = &forwardv1.Limits{QuotaBytes: 1 << 30, MaxConns: 10}

	// hop-errors: one good hop and one of each kind of rejected hop.
	tls := b.Simple(conformance.RouteB, 1)
	tls.Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS}
	name := b.Simple(conformance.RouteC, 2)
	name.Upstreams[0].Address = "target.example.com"
	injected := b.Simple("01JF2A000000000000000000D1", 3)
	injected.RouteId = `x"; flush ruleset; #`
	loopback := b.Simple("01JF2A000000000000000000E1", 0)
	loopback.Listen.Port = 30005
	loopback.Mark = 5
	loopback.Upstreams = ups([]string{"127.0.0.1"}, 1)
	dupA := b.Simple("01JF2A000000000000000000F1", 0)
	dupA.Listen.Port, dupA.Mark = 30006, 6
	dupB := b.Simple("01JF2A000000000000000000F2", 0)
	dupB.Listen.Port, dupB.Mark = 30007, 6
	least := b.Simple("01JF2A000000000000000000F3", 0)
	least.Listen.Port, least.Mark, least.Balance = 30008, 8, leastC

	custom := entry(both, ipHash, append(ups(v4, 3), ups(v6, 2)...))
	custom.Limits = &forwardv1.Limits{BandwidthBps: 10_000_000}

	return append(plannerCases(t), []goldenCase{
		{name: "empty", state: state()},
		{name: "strategy-round-robin", state: state(entry(tcp, rr, ups(v4, 3)))},
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
		{name: "paused", state: state(paused, b.Simple(conformance.RouteB, 1))},
		{name: "several-routes", state: state(b.Simple(conformance.RouteA, 0), b.Relay(conformance.RouteB, 1, ups(v4, 2)), b.Simple(conformance.RouteC, 2))},
		{name: "hop-errors", state: state(b.Simple(conformance.RouteA, 0), tls, name, injected, loopback, dupA, dupB, least), cfg: func(c *nftables.Config) {
			c.Strategies = []forwardv1.BalanceStrategy{rr, failover}
		}},
		{name: "custom-config", state: state(custom), cfg: func(c *nftables.Config) {
			c.MarkMask = 0x0000ff00
			c.DirectionBit = 0x00010000
			c.Slots = 16
			c.MSSClampInterfaces = []string{"wg0", "eth1"}
		}},
	}...)
}

// fixtureDir holds the planner goldens.
const fixtureDir = "../../../../contracts/forward/v1"

// plannerCases are the planner's own output: every node state with an
// nftables hop in the planner goldens (contracts/forward/v1/plan-*.json,
// which sdk/forward/planner reproduces byte for byte), named
// plan-<fixture>-<node>, and live planner runs of fixture requests with a
// change (planner-<fixture>-<change>-<node>).
func plannerCases(t testing.TB) []goldenCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(fixtureDir, "plan-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no planner fixtures: %v", err)
	}
	var cases []goldenCase
	add := func(prefix string, states []*forwardv1.NodeForwardState) {
		for _, s := range states {
			if slices.ContainsFunc(s.GetHops(), func(h *forwardv1.NodeHop) bool { return h.GetEngine() == nftE }) {
				cases = append(cases, goldenCase{name: prefix + "-" + s.GetNodeRef(), state: s})
			}
		}
	}
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

		// A paused route stays rendered with paused hops (forward-sdk.md
		// section 5.3): plan the single-hop IEPL route paused.
		if name == "single-hop-nftables-iepl" {
			var req forwardv1.PlanRouteRequest
			if err := protojson.Unmarshal(fx.Request, &req); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			req.Route.Paused = true
			resp, err := planner.PlanRoute(&req, nil, nil, planner.Options{})
			if err != nil || len(resp.GetViolations()) > 0 {
				t.Fatalf("planning %s paused: %v %v", f, err, resp.GetViolations())
			}
			add("planner-"+name+"-paused", resp.GetStates())
		}
	}
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

// TestGoldens renders every case and compares the script with
// contracts/forward/v1/nft/<case>.nft, the input with <case>.state.json and
// the hop errors with <case>.errors.txt. Run with -update to rewrite them.
func TestGoldens(t *testing.T) {
	cases := goldenCases(t)
	names := map[string]bool{}
	for _, c := range cases {
		names[c.name] = true
		t.Run(c.name, func(t *testing.T) {
			d := newDriver(t, c.cfg)
			a, err := d.Render(c.state)
			var re *driver.RenderError
			if err != nil && !errors.As(err, &re) {
				t.Fatalf("Render: %v", err)
			}
			if err := a.Verify(nftE); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(c.name, "plan") && err != nil {
				t.Fatalf("the planner's state does not render: %v", err)
			}
			files := map[string][]byte{
				c.name + ".state.json": stateJSON(t, c.state),
				c.name + ".nft":        a.Content,
				c.name + ".errors.txt": errorsText(err),
			}
			for _, f := range slices.Sorted(maps.Keys(files)) {
				path := filepath.Join(goldenDir, f)
				want := files[f]
				if *update {
					if len(want) == 0 {
						if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
							t.Fatal(err)
						}
						continue
					}
					if err := os.WriteFile(path, want, 0o644); err != nil { // #nosec G306 -- a checked-in golden
						t.Fatal(err)
					}
					continue
				}
				got, rerr := os.ReadFile(path) // #nosec G304 -- a golden of this test
				if len(want) == 0 {
					if rerr == nil {
						t.Errorf("%s exists but the case renders none; run with -update", f)
					}
					continue
				}
				if f == c.name+".state.json" && rerr == nil {
					var s forwardv1.NodeForwardState
					if err := protojson.Unmarshal(got, &s); err != nil {
						t.Fatalf("%s: %v", f, err)
					}
					if !proto.Equal(&s, c.state) {
						t.Errorf("%s differs from the case; run with -update", f)
					}
					continue
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s differs (run with -update to rewrite):\n%s", f, firstDiff(got, want))
				}
			}
		})
	}
	// Every file in the directory belongs to a case.
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		base := e.Name()
		for _, ext := range []string{".state.json", ".nft", ".errors.txt"} {
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
