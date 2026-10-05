package relayctl_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
)

const goldens = "../../../../../contracts/forward/v1/anixops"

func goodHop() relayctl.Hop {
	return relayctl.Hop{
		Route: "01JF2A000000000000000000A1", Role: "entry",
		Listen:    relayctl.Listen{Port: 30001, TCP: true},
		Ingress:   relayctl.Ingress{Security: relayctl.SecurityRaw},
		Upstreams: []relayctl.Upstream{{Address: "192.0.2.20", Port: 443, Weight: 1, Egress: relayctl.Egress{Security: relayctl.SecurityRaw}}},
		Balance:   relayctl.BalanceRoundRobin,
		Breaker:   relayctl.Breaker{MaxFails: 3, OpenMs: 30000},
	}
}

func doc(hops ...relayctl.Hop) *relayctl.Config {
	return &relayctl.Config{Format: relayctl.Format, Owner: "test", Note: "test", Hops: hops}
}

func TestEncodeParseRoundTrip(t *testing.T) {
	c := doc(goodHop())
	b, err := relayctl.Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := relayctl.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := relayctl.Encode(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(b2) {
		t.Fatal("a parsed document encodes to other bytes")
	}
	if relayctl.Digest(b) != relayctl.Digest(b2) || len(relayctl.Digest(b)) != 64 {
		t.Fatal("digest")
	}
}

func TestCheckRefusesWhatTheRelayCannotRun(t *testing.T) {
	mod := func(f func(*relayctl.Hop)) *relayctl.Config {
		h := goodHop()
		f(&h)
		return doc(h)
	}
	second := goodHop()
	second.Hop = 1
	for name, c := range map[string]*relayctl.Config{
		"wrong format":       {Format: "other", Hops: nil},
		"link without paths": {Format: relayctl.Format, Link: &relayctl.LinkFiles{Cert: "/c"}},
		"unsorted hops":      doc(second, goodHop()),
		"repeated hop":       doc(goodHop(), goodHop()),
		"route with a slash": mod(func(h *relayctl.Hop) { h.Route = "a/b" }),
		"empty route":        mod(func(h *relayctl.Hop) { h.Route = "" }),
		"unknown role":       mod(func(h *relayctl.Hop) { h.Role = "boss" }),
		"port zero":          mod(func(h *relayctl.Hop) { h.Listen.Port = 0 }),
		"port too high":      mod(func(h *relayctl.Hop) { h.Listen.Port = 70000 }),
		"no socket":          mod(func(h *relayctl.Hop) { h.Listen.TCP = false }),
		"raw with a carrier": mod(func(h *relayctl.Hop) { h.Ingress.Carrier = relayctl.CarrierQUIC }),
		"unknown ingress":    mod(func(h *relayctl.Hop) { h.Ingress.Security = "tls" }),
		"unknown carrier": mod(func(h *relayctl.Hop) {
			h.Ingress = relayctl.Ingress{Security: relayctl.SecurityAnixOps, Carrier: "sctp"}
		}),
		"no upstream":     mod(func(h *relayctl.Hop) { h.Upstreams = nil }),
		"unknown balance": mod(func(h *relayctl.Hop) { h.Balance = "fastest" }),
		"no breaker":      mod(func(h *relayctl.Hop) { h.Breaker = relayctl.Breaker{} }),
		"zero weight":     mod(func(h *relayctl.Hop) { h.Upstreams[0].Weight = 0 }),
		"upstream port":   mod(func(h *relayctl.Hop) { h.Upstreams[0].Port = 0 }),
		"upstream twice":  mod(func(h *relayctl.Hop) { h.Upstreams = append(h.Upstreams, h.Upstreams[0]) }),
		"unknown egress":  mod(func(h *relayctl.Hop) { h.Upstreams[0].Egress.Security = "quic" }),
		"anixops egress bare": mod(func(h *relayctl.Hop) {
			h.Upstreams[0].Egress = relayctl.Egress{Security: relayctl.SecurityAnixOps, Carrier: relayctl.CarrierAuto}
		}),
	} {
		if err := c.Check(); !errors.Is(err, relayctl.ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if err := doc(goodHop()).Check(); err != nil {
		t.Fatal(err)
	}
}

func TestParseIsStrict(t *testing.T) {
	good, _ := relayctl.Encode(doc(goodHop()))
	for name, b := range map[string][]byte{
		"empty":            nil,
		"not json":         []byte("hops"),
		"unknown field":    []byte(strings.Replace(string(good), `"note"`, `"unexpected": 1, "note"`, 1)),
		"trailing data":    append(append([]byte{}, good...), []byte(`{}`)...),
		"wrong type":       []byte(`{"format": 5}`),
		"unknown nested":   []byte(strings.Replace(string(good), `"max_fails"`, `"max_fail"`, 1)),
		"another document": []byte(`{"format":"anixops.relay/v1","hops":[{}]}`),
	} {
		if _, err := relayctl.Parse(b); !errors.Is(err, relayctl.ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
}

func TestListenerKeySeparatesTheStructuralFromTheHot(t *testing.T) {
	a := goodHop()
	same := a
	same.Balance, same.Paused = relayctl.BalanceFailover, true
	same.Upstreams = nil
	same.Limits.QuotaBytes = 5
	same.Sources, same.Peers = []string{"192.0.2.0/24"}, []string{"spiffe://anixops/x/agent/forward-1"}
	if a.ListenerKey() != same.ListenerKey() {
		t.Fatal("a hot field changed the listener key")
	}
	for name, mod := range map[string]func(*relayctl.Hop){
		"port":    func(h *relayctl.Hop) { h.Listen.Port++ },
		"address": func(h *relayctl.Hop) { h.Listen.Address = "192.0.2.1" },
		"udp":     func(h *relayctl.Hop) { h.Listen.UDP = true },
		"tcp":     func(h *relayctl.Hop) { h.Listen.TCP = false },
		"ingress": func(h *relayctl.Hop) {
			h.Ingress = relayctl.Ingress{Security: relayctl.SecurityAnixOps, Carrier: relayctl.CarrierQUIC}
		},
		"carrier": func(h *relayctl.Hop) {
			h.Ingress = relayctl.Ingress{Security: relayctl.SecurityAnixOps, Carrier: relayctl.CarrierAuto}
		},
	} {
		b := a
		mod(&b)
		if a.ListenerKey() == b.ListenerKey() {
			t.Errorf("%s did not change the listener key", name)
		}
	}
}

// Every golden the driver rendered parses, and encodes back to its bytes.
func TestGoldensParse(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(goldens, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no goldens: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".state.json") {
			continue
		}
		b, err := os.ReadFile(f) // #nosec G304 -- a golden
		if err != nil {
			t.Fatal(err)
		}
		c, err := relayctl.Parse(b)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
			continue
		}
		out, err := relayctl.Encode(c)
		if err != nil || string(out) != string(b) {
			t.Errorf("%s does not encode back to its bytes", filepath.Base(f))
		}
	}
}

// FuzzParse: whatever Parse takes encodes to bytes it takes again, to the same
// document; whatever it refuses is ErrInvalid; it never panics.
func FuzzParse(f *testing.F) {
	good, _ := relayctl.Encode(doc(goodHop()))
	f.Add(good)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"format":"anixops.relay/v1","owner":"o","note":"n","hops":[]}`))
	if files, _ := filepath.Glob(filepath.Join(goldens, "*.json")); len(files) > 0 {
		for _, name := range files[:min(len(files), 6)] {
			if b, err := os.ReadFile(name); err == nil { // #nosec G304 -- a golden
				f.Add(b)
			}
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := relayctl.Parse(data)
		if err != nil {
			if !errors.Is(err, relayctl.ErrInvalid) {
				t.Fatalf("a refusal that is not ErrInvalid: %v", err)
			}
			return
		}
		out, err := relayctl.Encode(c)
		if err != nil {
			t.Fatal(err)
		}
		c2, err := relayctl.Parse(out)
		if err != nil {
			t.Fatalf("what Parse took does not parse after Encode: %v", err)
		}
		out2, _ := relayctl.Encode(c2)
		if string(out) != string(out2) {
			t.Fatal("Encode is not stable")
		}
	})
}
