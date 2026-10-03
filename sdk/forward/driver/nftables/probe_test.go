package nftables

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// fakeHost answers commands by a function, for the probe's outcomes.
type fakeHost func(name string, args []string, stdin string) (string, string, error)

func (f fakeHost) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, stderr, err := f(name, args, string(stdin))
	if err != nil {
		return []byte(out), &CommandError{Name: name, Args: args, Stderr: stderr, Err: err}
	}
	return []byte(out), nil
}

func TestProbeOutcomes(t *testing.T) {
	exit1 := errors.New("exit status 1")
	healthy := func(name string, args []string, stdin string) (string, string, error) {
		switch {
		case name == "nft" && args[0] == "--version":
			return "nftables v1.0.9 (Old Doc Yak #3)\n", "", nil
		case name == "nft" && args[0] == "-j":
			return `{"nftables":[{"metainfo":{"version":"1.0.9"}},{"chain":{"family":"ip","table":"filter","name":"FORWARD","handle":1,"type":"filter","hook":"forward","prio":0,"policy":"drop"}}]}`, "", nil
		}
		return "", "", nil
	}
	for _, c := range []struct {
		name  string
		host  fakeHost
		check func(t *testing.T, cfg Config, rep *ProbeReport)
	}{
		{"healthy", healthy, func(t *testing.T, cfg Config, rep *ProbeReport) {
			if cfg.Version != "nft 1.0.9" || !cfg.BandwidthLimit || len(rep.Missing) != 0 || len(rep.Warnings) != 1 {
				t.Fatalf("%+v %+v", cfg, rep)
			}
		}},
		{"nft missing", func(string, []string, string) (string, string, error) {
			return "", "", fmt.Errorf("start: %w", exec.ErrNotFound)
		}, func(t *testing.T, cfg Config, _ *ProbeReport) {
			if cfg.Version != "" || cfg.Unavailable != "nft is not installed" {
				t.Fatalf("%+v", cfg)
			}
		}},
		{"no CAP_NET_ADMIN", func(name string, args []string, stdin string) (string, string, error) {
			if args[0] == "-j" {
				return "", "Error: Operation not permitted\n", exit1
			}
			return healthy(name, args, stdin)
		}, func(t *testing.T, cfg Config, _ *ProbeReport) {
			if cfg.Version != "" || !strings.Contains(cfg.Unavailable, "CAP_NET_ADMIN") {
				t.Fatalf("%+v", cfg)
			}
		}},
		{"old kernel", func(name string, args []string, stdin string) (string, string, error) {
			if args[0] == "-c" && strings.Contains(stdin, "probe_state") {
				return "", "Error: Could not process rule: Not supported\n", exit1
			}
			return healthy(name, args, stdin)
		}, func(t *testing.T, cfg Config, _ *ProbeReport) {
			if cfg.Version != "" || !strings.Contains(cfg.Unavailable, "Not supported") {
				t.Fatalf("%+v", cfg)
			}
		}},
		{"some features missing", func(name string, args []string, stdin string) (string, string, error) {
			switch {
			case name == "tc":
				return "", "", fmt.Errorf("start: %w", exec.ErrNotFound)
			case args[0] == "-c" && (strings.Contains(stdin, "jhash") || strings.Contains(stdin, "ct count") || strings.Contains(stdin, "ip6 to")):
				return "", "Error: Could not process rule: No such file or directory\n", exit1
			}
			return healthy(name, args, stdin)
		}, func(t *testing.T, cfg Config, rep *ProbeReport) {
			want := []forwardv1.BalanceStrategy{
				forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN,
				forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM,
				forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN,
				forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
			}
			if cfg.Version == "" || cfg.IPv6 || cfg.MaxConns || cfg.BandwidthLimit || !cfg.Quota || fmt.Sprint(cfg.Strategies) != fmt.Sprint(want) || len(rep.Missing) != 4 {
				t.Fatalf("%+v %+v", cfg, rep)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg, rep, err := Probe(t.Context(), c.host, e2eConfig())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := New(cfg); err != nil {
				t.Fatalf("probed configuration refused: %v", err)
			}
			c.check(t, cfg, rep)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := Probe(ctx, fakeHost(healthy), e2eConfig()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled probe: %v", err)
	}
}

// TestUnavailableCapabilities: a probed-unavailable driver says why.
func TestUnavailableCapabilities(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Unavailable = "nft is not installed"
	d, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c, err := d.Capabilities(t.Context())
	if err != nil || c.GetAvailable() || c.GetUnavailableReason() != "nft is not installed" {
		t.Fatalf("%v %v", c, err)
	}
}

func TestParseFilterFormats(t *testing.T) {
	d, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		`{"kind":"fw","options":{"fw":{"mark":"0x10000","mask":"0xfff0001"},"classid":"af00:2"}}`,
		`{"kind":"fw","handle":"0x10000/0xfff0001","options":{"classid":"af00:2"}}`,
		`{"kind":"fw","options":{"handle":"0x10000/0xfff0001","flowid":"af00:2"}}`,
	} {
		var m map[string]any
		if err := decodeJSON([]byte(f), &m); err != nil {
			t.Fatal(err)
		}
		if mark, minor, ok := d.parseFilter(m); !ok || mark != 0x10000 || minor != 2 {
			t.Fatalf("%s: %#x %d %v", f, mark, minor, ok)
		}
	}
	var foreign map[string]any
	_ = decodeJSON([]byte(`{"kind":"fw","options":{"fw":{"mark":"0x1"},"classid":"1:2"}}`), &foreign)
	if _, _, ok := d.parseFilter(foreign); ok {
		t.Fatal("a filter to another qdisc's class read as the driver's")
	}
}

func TestStateDocument(t *testing.T) {
	doc := &hostDoc{V: 1, Node: `node "x" ünïcode`, Gen: 7, Digest: strings.Repeat("ab", 32),
		Hops: []*docHop{{Route: "R1", Ups: []docUp{{Addr: "192.0.2.1", Port: 1, Weight: 1}}}}}
	for range 6 {
		doc.Hops = append(doc.Hops, doc.Hops...)
	}
	chunks := doc.encode()
	m := map[uint64]string{}
	for i, c := range chunks {
		if len(c) > stateChunk || strings.ContainsAny(c, "\"\\ ") {
			t.Fatalf("chunk %d %q", i, c)
		}
		m[uint64(i)] = c // #nosec G115 -- small test index
	}
	back, err := decodeDoc(m)
	if err != nil || back.Node != doc.Node || len(back.Hops) != len(doc.Hops) {
		t.Fatalf("%v %+v", err, back)
	}
	delete(m, 1)
	if _, err := decodeDoc(m); err == nil {
		t.Fatal("a missing chunk decoded")
	}
}

func TestParseManifestRefuses(t *testing.T) {
	d, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	good := "# anixops-hop route=R1 hop=0 mark=1 balance=BALANCE_STRATEGY_FAILOVER listen=tcp/1 address=any bandwidth=0\n" +
		"# anixops-upstream route=R1 hop=0 address=192.0.2.1 port=1 weight=1 priority=0\n"
	hops := []driver.HopKey{{RouteID: "R1"}}
	for name, c := range map[string]struct {
		content string
		hops    []driver.HopKey
	}{
		"hops differ":       {good, nil},
		"no upstream":       {strings.SplitAfter(good, "\n")[0], hops},
		"unknown hop":       {strings.Replace(good, "upstream route=R1", "upstream route=R2", 1), hops},
		"bad strategy":      {strings.Replace(good, "FAILOVER", "BEST", 1), hops},
		"bad route":         {strings.ReplaceAll(good, "route=R1", "route=R-1"), hops},
		"bad port":          {strings.Replace(good, "port=1", "port=0", 1), hops},
		"bad listen":        {strings.Replace(good, "tcp/1", "sctp/1", 1), hops},
		"repeated field":    {strings.Replace(good, "mark=1", "mark=1 mark=2", 1), hops},
		"hop twice":         {strings.SplitAfter(good, "\n")[0] + good, hops},
		"bad upstream addr": {strings.Replace(good, "192.0.2.1", "example.com", 1), hops},
	} {
		a := driver.NewArtifact(d.Engine(), nil, c.hops, []byte(c.content))
		if _, err := parseManifest(a); !errors.Is(err, driver.ErrInvalidArtifact) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := parseManifest(driver.NewArtifact(d.Engine(), nil, hops, []byte(good))); err != nil {
		t.Fatal(err)
	}
}

func TestRateText(t *testing.T) {
	for bits, want := range map[uint64]string{
		800: "800bit", 100_000_000: "100Mbit", 1_600: "1600bit", 999_992: "999992bit",
		12_345_678_896: "12345Mbit", 8_000: "8Kbit", 1_000_000_000_000_000: "1000Tbit",
	} {
		if got := rateText(bits / 8); got != want {
			t.Errorf("%d: %s, want %s", bits, got, want)
		}
		if b, ok := parseRateText(want); !ok || rateText(b/8) != want {
			t.Errorf("round trip %s: %d %v", want, b, ok)
		}
	}
	if (tcRate{bytes: 12_345_000_000 / 8, exact: true}).equals(12_345_678_896 / 8) {
		t.Error("exact rates compared loosely")
	}
	if !(tcRate{bytes: 12_345_000_000 / 8, exact: false}).equals(12_345_678_896 / 8) {
		t.Error("text rates compared exactly")
	}
}
