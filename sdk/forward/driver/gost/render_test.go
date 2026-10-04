package gost_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// walkKeys calls f with every object key of a JSON document and the key
// of the object that holds it ("" at the top).
func walkKeys(v any, parent string, f func(parent, key string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			f(parent, k)
			walkKeys(e, k, f)
		}
	case []any:
		for _, e := range x {
			walkKeys(e, parent, f)
		}
	}
}

// muxKeys are the dotted keys gost reads a mux carrier's keepalives from.
// Inside the metadata of a listener or dialer (in the services' or hops'
// arrays) gost's file loader keeps them whole: viper does not descend
// into arrays (gost -O json shows them as loaded).
var muxKeys = map[string]bool{"mux.keepaliveInterval": true, "mux.keepaliveTimeout": true}

// TestRenderIsPlainJSON: every rendered configuration is JSON whose keys
// hold no dot (gost's loader would split such a key into nested objects)
// but the mux keepalives in a metadata object, and that names no file but
// the configured link certificate and socket.
func TestRenderIsPlainJSON(t *testing.T) {
	for _, c := range goldenCases(t) {
		d := newDriver(t, c.cfg)
		a, _ := d.Render(c.state)
		var v any
		if err := json.Unmarshal(a.Content, &v); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		walkKeys(v, "", func(parent, k string) {
			if strings.Contains(k, ".") && (parent != "metadata" || !muxKeys[k]) {
				t.Errorf("%s: key %q in %q", c.name, k, parent)
			}
		})
		if bytes.Contains(a.Content, []byte("preUp")) || bytes.Contains(a.Content, []byte("postUp")) {
			t.Errorf("%s: renders gost commands", c.name)
		}
	}
}

// TestRenderMetricsPathNamesTheConfiguration: the metrics path changes with
// the configuration's structure (a listener), not with the hot objects
// (upstreams, weights, pause, sources, limits' values) or the state's
// identity.
func TestRenderMetricsPathNamesTheConfiguration(t *testing.T) {
	d := newDriver(t, nil)
	b := builder(t)
	path := func(hops ...*forwardv1.NodeHop) string {
		a := render(t, d, conformance.State("forward-11", 1, hops...))
		var c struct{ Metrics struct{ Path string } }
		if err := json.Unmarshal(a.Content, &c); err != nil {
			t.Fatal(err)
		}
		return c.Metrics.Path
	}
	h := b.Simple(conformance.RouteA, 0)
	h.Limits = &forwardv1.Limits{BandwidthBps: 8_000_000, MaxConns: 10}
	p1 := path(h)
	hot := proto.Clone(h).(*forwardv1.NodeHop)
	hot.Upstreams[2].Weight = 9
	hot.Upstreams = hot.Upstreams[1:]
	hot.Balance = forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM
	hot.Paused = true
	hot.Limits = &forwardv1.Limits{BandwidthBps: 1_000_000, MaxConns: 3, QuotaBytes: 1 << 30}
	if p2 := path(hot); p2 != p1 || !strings.HasPrefix(p1, "/anixops-") {
		t.Fatalf("a change of hot objects moved the metrics path %s -> %s", p1, p2)
	}
	structural := proto.Clone(h).(*forwardv1.NodeHop)
	structural.Listen.Port++
	if p3 := path(structural); p3 == p1 {
		t.Fatalf("a listener change kept the metrics path %s", p1)
	}
	s := conformance.State("forward-99", 7, h)
	if a := render(t, d, s); !bytes.Contains(a.Content, []byte(p1)) {
		t.Fatal("the state's identity changed the metrics path")
	}
}

// TestRenderServerNameFallback: an encrypted upstream without a server
// name is verified for its node's identity name.
func TestRenderServerNameFallback(t *testing.T) {
	d := newDriver(t, nil)
	h := builder(t).Simple(conformance.RouteA, 0)
	for i, u := range h.Upstreams {
		u.Egress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_QUIC}
		u.NodeRef = "forward-5" + string(rune('0'+i))
		u.PeerIdentity = "spiffe://anixops/example/agent/" + u.NodeRef
	}
	a := render(t, d, conformance.State("forward-11", 1, h))
	for _, n := range []string{"forward-50", "forward-51", "forward-52"} {
		if !bytes.Contains(a.Content, []byte(`"serverName": "`+n+`"`)) {
			t.Fatalf("no server name %s in\n%s", n, a.Content)
		}
	}
}

// TestGostParsesGoldens loads every golden configuration with the gost
// binary's own parser (`gost -C <file> -O json`, which reads and prints the
// configuration and starts nothing), when a gost binary is available
// (ANIXOPS_GOST_BIN or PATH). TestNetnsGoldens also runs them.
func TestGostParsesGoldens(t *testing.T) {
	bin := gostBinary()
	if bin == "" {
		t.Skip("no gost binary (set ANIXOPS_GOST_BIN)")
	}
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, ".state.json") {
			continue
		}
		n++
		t.Run(filepath.Base(f), func(t *testing.T) {
			t.Parallel()
			out, err := exec.Command(bin, "-C", f, "-O", "json").CombinedOutput() // #nosec G204 -- the test's gost on a golden
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			var parsed, ours struct {
				Services []struct{ Name string }
				Chains   []struct{ Name string }
			}
			if err := json.Unmarshal(out, &parsed); err != nil {
				t.Fatalf("gost printed %v", err)
			}
			b, err := os.ReadFile(f) // #nosec G304 -- a golden of this test
			if err != nil || json.Unmarshal(b, &ours) != nil {
				t.Fatalf("%s: %v", f, err)
			}
			if len(parsed.Services) != len(ours.Services) || len(parsed.Chains) != len(ours.Chains) {
				t.Errorf("gost read %d services and %d chains, the file has %d and %d", len(parsed.Services), len(parsed.Chains), len(ours.Services), len(ours.Chains))
			}
		})
	}
	if n == 0 {
		t.Fatal("no golden configuration")
	}
}
