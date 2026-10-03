package nftables

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// Replay tests: a scenario runs once on a real kernel (in a network
// namespace, with ANIXOPS_NFT_RECORD=1 as root) and its commands, their
// input and their answers are recorded in testdata/replay. Without
// privileges the test replays the recording through a fake runner that
// answers each command as the kernel did and fails on any command, stdin
// or order that differs: the recording is the golden command sequence of
// the scenario. Re-record after a deliberate change to the commands.
const nftRecord = "ANIXOPS_NFT_RECORD"

type exchange struct {
	Cmd    string `json:"cmd"`
	Stdin  string `json:"stdin,omitempty"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
	Fail   bool   `json:"fail,omitempty"`
}

func cmdLine(name string, args []string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

// recorder runs commands through another runner and records them.
type recorder struct {
	next Runner
	mu   sync.Mutex
	log  []exchange
}

func (r *recorder) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	out, err := r.next.Run(ctx, name, args, stdin)
	x := exchange{Cmd: cmdLine(name, args), Stdin: string(stdin), Stdout: string(out)}
	if err != nil {
		x.Fail, x.Stderr = true, stderrOf(err)
	}
	r.mu.Lock()
	r.log = append(r.log, x)
	r.mu.Unlock()
	return out, err
}

// replayer answers commands from a recording, in order.
type replayer struct {
	t   testing.TB
	mu  sync.Mutex
	log []exchange
	pos int
}

func (r *replayer) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pos >= len(r.log) {
		r.t.Fatalf("command %d %q after the end of the recording", r.pos, cmdLine(name, args))
	}
	x := r.log[r.pos]
	r.pos++
	if got := cmdLine(name, args); got != x.Cmd || string(stdin) != x.Stdin {
		r.t.Fatalf("command %d:\n got  %s\n%s\n want %s\n%s", r.pos-1, got, stdin, x.Cmd, x.Stdin)
	}
	if x.Fail {
		return []byte(x.Stdout), &CommandError{Name: name, Args: args, Stderr: x.Stderr, Err: errors.New("exit status 1")}
	}
	return []byte(x.Stdout), nil
}

// withReplay runs scenario on a recording, or records it with
// ANIXOPS_NFT_RECORD=1. The scenario gets a runner and a function that
// makes drivers on it (with a fixed nonce and clock).
func withReplay(t *testing.T, name string, scenario func(t *testing.T, r Runner, plant func(script string))) {
	file := filepath.Join("testdata", "replay", name+".json")
	if os.Getenv(nftRecord) == "1" {
		t.Setenv(nftE2E, "1")
		ns := newNetns(t)
		rec := &recorder{next: &nsRunner{ns: ns}}
		scenario(t, rec, func(script string) { ns.nft(t, script) })
		if t.Failed() {
			return
		}
		b, err := json.MarshalIndent(rec.log, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(file) // #nosec G304 -- a recording of this package
	if err != nil {
		t.Fatalf("%v (record it with %s=1 as root)", err, nftRecord)
	}
	rp := &replayer{t: t}
	if err := json.Unmarshal(b, &rp.log); err != nil {
		t.Fatal(err)
	}
	scenario(t, rp, func(string) {}) // planted objects are in the recording
	if rp.pos != len(rp.log) {
		t.Fatalf("replay used %d of %d recorded commands; next %q", rp.pos, len(rp.log), rp.log[rp.pos].Cmd)
	}
}

// replayDriver probes and makes a driver with a fixed nonce and clock.
func replayDriver(t testing.TB, r Runner, opts ...Option) *Driver {
	t.Helper()
	cfg, rep, err := Probe(context.Background(), r, e2eConfig())
	if err != nil || cfg.Version == "" {
		t.Fatalf("probe: %v %+v %+v", err, cfg, rep)
	}
	d, err := New(cfg, append([]Option{WithRunner(r)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	d.nonce = func() string { return "0123456789abcdef" }
	d.now = func() time.Time { return time.Unix(1_790_000_000, 0) }
	return d
}

func replayApply(t testing.TB, d *Driver, s *forwardv1.NodeForwardState) (driver.ApplyResult, error) {
	t.Helper()
	a, err := d.Render(s)
	if err != nil {
		t.Fatal(err)
	}
	return d.Apply(context.Background(), a)
}

// TestReplayLifecycle: the life of a node's table: a first apply with a
// rate-limited hop, a no-op re-apply, a newer generation with the same
// content, failover, a changing apply that removes a hop, an empty
// artifact and Remove.
func TestReplayLifecycle(t *testing.T) {
	withReplay(t, "lifecycle", func(t *testing.T, r Runner, _ func(string)) {
		var retired []*forwardv1.Counters
		d := replayDriver(t, r, WithRetiredCounters(func(c []*forwardv1.Counters) { retired = append(retired, c...) }))
		b := e2eBuilder(t, d)
		node := b.Top.NodeRef
		a, bb := limited(b, conformance.RouteA, 0, 100_000_000), b.Simple(conformance.RouteB, 1)
		other := b.Simple(conformance.RouteC, 2)
		other.Engine = forwardv1.Engine_ENGINE_GOST
		other.Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS}

		step := func(what string, s *forwardv1.NodeForwardState, changed bool) {
			t.Helper()
			res, err := replayApply(t, d, s)
			if err != nil || res.Changed != changed || res.Generation != s.GetGeneration() {
				t.Fatalf("%s: %+v, %v (want changed %v)", what, res, err, changed)
			}
		}
		step("first apply", conformance.State(node, 1, a, bb), true)
		step("same artifact", conformance.State(node, 1, a, bb), false)
		step("newer generation, same hops", conformance.State(node, 2, a, bb, other), false)
		o, err := d.Observe(context.Background())
		if err != nil || !o.Applied || o.Generation != 2 || len(o.Counters) != 2 || o.Counters[0].GetCounterEpoch() != "0123456789abcdef" {
			t.Fatalf("observe: %+v, %v", o, err)
		}
		u := a.GetUpstreams()[2]
		if err := d.SetUpstreams(context.Background(), conformance.RouteA, 0, []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort(), Weight: 9}}); err != nil {
			t.Fatal(err)
		}
		o, err = d.Observe(context.Background())
		if err != nil || len(o.Rotation) != 2 || len(o.Rotation[0].Active) != 1 || o.Rotation[0].Active[0].Weight != 9 {
			t.Fatalf("observe after failover: %+v, %v", o.Rotation, err)
		}
		step("route B removed", conformance.State(node, 3, a), true)
		if len(retired) != 1 || retired[0].GetRouteId() != conformance.RouteB {
			t.Fatalf("retired %v", retired)
		}
		step("empty", conformance.State(node, 4), true)
		step("empty again", conformance.State(node, 5), false)
		if err := d.Remove(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

// TestReplayRefusals: the refusals that leave the host alone: a foreign
// table of the driver's name, a stale generation, a generation conflict,
// a foreign DNAT rule on a listener, and SetUpstreams on unknown hops.
func TestReplayRefusals(t *testing.T) {
	withReplay(t, "refusals", func(t *testing.T, r Runner, plant func(string)) {
		d := replayDriver(t, r)
		b := e2eBuilder(t, d)
		node := b.Top.NodeRef
		a := b.Simple(conformance.RouteA, 0)

		plant("table inet anixops_fwd {\n\tcomment \"not ours\"\n}\n")
		if _, err := replayApply(t, d, conformance.State(node, 1, a)); !errors.Is(err, driver.ErrNotOwned) {
			t.Fatalf("impostor: %v", err)
		}
		if err := d.Remove(context.Background()); err != nil {
			t.Fatalf("Remove with an impostor: %v", err)
		}
		if o, err := d.Observe(context.Background()); err != nil || o.Applied {
			t.Fatalf("observe with an impostor: %+v, %v", o, err)
		}
		plant("delete table inet anixops_fwd\n")

		if _, err := replayApply(t, d, conformance.State(node, 5, a)); err != nil {
			t.Fatal(err)
		}
		if _, err := replayApply(t, d, conformance.State(node, 4, a)); !errors.Is(err, driver.ErrStaleGeneration) {
			t.Fatalf("stale: %v", err)
		}
		if _, err := replayApply(t, d, conformance.State(node, 5, a, b.Simple(conformance.RouteB, 1))); !errors.Is(err, driver.ErrGenerationConflict) {
			t.Fatalf("conflict: %v", err)
		}
		plant(fmt.Sprintf("table ip foreign_conflict {\n\tchain pre {\n\t\ttype nat hook prerouting priority dstnat; policy accept;\n\t\ttcp dport { 1-10, %d } dnat to 192.0.2.99:80\n\t}\n}\n", b.Top.ListenPorts[1]))
		if _, err := replayApply(t, d, conformance.State(node, 6, a, b.Simple(conformance.RouteB, 1))); !errors.Is(err, driver.ErrConflict) {
			t.Fatalf("foreign DNAT: %v", err)
		}
		one := []driver.Upstream{{Address: a.GetUpstreams()[0].GetAddress(), Port: a.GetUpstreams()[0].GetPort()}}
		for _, c := range []struct {
			route string
			hop   uint32
			sel   []driver.Upstream
			want  error
		}{
			{conformance.RouteB, 0, one, driver.ErrNotFound},
			{conformance.RouteA, 0, []driver.Upstream{{Address: "192.0.2.99", Port: 443}}, driver.ErrNotFound},
			{conformance.RouteA, 0, []driver.Upstream{{Address: "not an address", Port: 443}}, driver.ErrNotFound},
			{conformance.RouteA, 0, append(slices.Clone(one), one...), driver.ErrInvalidArgument},
		} {
			if err := d.SetUpstreams(context.Background(), c.route, c.hop, c.sel); !errors.Is(err, c.want) {
				t.Fatalf("SetUpstreams %s/%d %v: %v, want %v", c.route, c.hop, c.sel, err, c.want)
			}
		}
		if err := d.Remove(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

// TestReplayRecordingsReadable keeps the recordings small enough to review.
func TestReplayRecordingsReadable(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "replay", "*.json"))
	if len(files) == 0 {
		t.Fatal("no recordings")
	}
	for _, f := range files {
		b, err := os.ReadFile(f) // #nosec G304 -- a recording of this package
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("/root/")) || bytes.Contains(b, []byte("/home/")) {
			t.Fatalf("%s holds a local path", f)
		}
	}
}
