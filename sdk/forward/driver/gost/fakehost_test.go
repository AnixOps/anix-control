package gost_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// fakeGost simulates gost and the host for the driver without privileges:
// it is the Supervisor (a "process" that loads the configuration file,
// serves its metrics path on the unix socket and "binds" its listeners)
// and the Runner (ss lists those listeners and planted foreign ones; gost
// -V prints the pinned version). A reload that cannot bind a listener
// closes every service, as gost's does.
type fakeGost struct {
	config, socket, dir string

	mu       sync.Mutex
	running  bool
	instance int
	path     string   // the metrics path being served
	bound    []string // ss lines of the services' sockets
	srv      *http.Server
	foreign  []string // ss lines of planted foreign sockets
	hidden   []string // foreign sockets ss does not show yet (taken after a check)
	failNext bool
	applies  int
}

func newFakeGost(t testing.TB) (*fakeGost, gost.Config) {
	t.Helper()
	root := shortDir(t)
	cfg := testConfig()
	cfg.Dir, cfg.RuntimeDir = filepath.Join(root, "d"), filepath.Join(root, "r")
	for _, d := range []string{cfg.Dir, cfg.RuntimeDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeGost{config: filepath.Join(cfg.Dir, gost.ConfigFile), socket: filepath.Join(cfg.RuntimeDir, gost.MetricsSocket), dir: cfg.Dir}
	t.Cleanup(func() { _ = f.Stop(context.Background()) })
	return f, cfg
}

func (f *fakeGost) Run(ctx context.Context, name string, args []string, _ []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case name == "ss" && slices.Equal(args, []string{"-V"}):
		return []byte("ss utility, iproute2-6.15.0\n"), nil
	case name == "ss":
		lines := slices.Clone(f.foreign)
		if f.running {
			lines = append(lines, f.bound...)
		}
		return []byte(strings.Join(lines, "\n") + "\n"), nil
	case name == "gost":
		return []byte("gost v" + gost.PinnedVersion + " (go1.25.4 linux/amd64)\n"), nil
	}
	return nil, fmt.Errorf("fakeGost: unexpected %s %v", name, args)
}

// load reads the configuration file as gost would: the metrics path and
// the sockets of its services.
func (f *fakeGost) load() (path string, bound []string, ok bool) {
	b, err := os.ReadFile(f.config)
	if err != nil || f.failNext {
		f.failNext = false
		return "", nil, false
	}
	var c struct {
		AnixOps struct {
			Hops []struct {
				Listeners []struct {
					Network, Address string
					Port             uint32
				}
			}
		}
		Metrics struct{ Path string }
	}
	if json.Unmarshal(b, &c) != nil || c.Metrics.Path == "" {
		return "", nil, false
	}
	for _, h := range c.AnixOps.Hops {
		for _, l := range h.Listeners {
			addr := "*"
			if l.Address != "" {
				addr = l.Address
				if strings.Contains(addr, ":") {
					addr = "[" + addr + "]"
				}
			}
			state := "LISTEN"
			if l.Network == "udp" {
				state = "UNCONN"
			}
			bound = append(bound, fmt.Sprintf("%s %s 0 4096 %s:%d *:*", l.Network, state, addr, l.Port))
		}
	}
	return c.Metrics.Path, bound, true
}

func (f *fakeGost) Check(ctx context.Context) error { return ctx.Err() }

func (f *fakeGost) Status(ctx context.Context) (gost.Status, error) {
	if err := ctx.Err(); err != nil {
		return gost.Status{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.running {
		return gost.Status{}, nil
	}
	return gost.Status{Running: true, Instance: fmt.Sprintf("fake-%d", f.instance)}, nil
}

func (f *fakeGost) taken(lines []string) bool {
	for _, l := range lines {
		local := strings.Fields(l)[4]
		port := local[strings.LastIndexByte(local, ':'):]
		for _, o := range append(slices.Clone(f.foreign), f.hidden...) {
			if strings.Fields(o)[0] == strings.Fields(l)[0] && strings.HasSuffix(strings.Fields(o)[4], port) {
				return true
			}
		}
	}
	return false
}

func (f *fakeGost) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applies++
	if f.running {
		return nil
	}
	path, bound, ok := f.load()
	if !ok || f.taken(bound) {
		return nil // gost exits before serving
	}
	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		return nil // a stale socket: gost exits
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path != f.path {
			http.NotFound(w, r)
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	f.running, f.srv, f.path, f.bound = true, srv, path, bound
	f.instance++
	return nil
}

func (f *fakeGost) Reload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applies++
	if !f.running {
		return errors.New("fakeGost: not running")
	}
	path, bound, ok := f.load()
	switch {
	case !ok: // a configuration gost cannot parse: nothing changes
	case f.taken(bound): // a listener gost cannot bind: every service closed
		f.bound = nil
	default:
		f.path, f.bound = path, bound
	}
	return nil
}

func (f *fakeGost) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running {
		_ = f.srv.Close()
		f.running, f.srv, f.path, f.bound = false, nil, "", nil
	}
	return nil
}

// fakeEnv is the conformance Env of a fakeGost.
type fakeEnv struct {
	f   *fakeGost
	cfg gost.Config
}

var (
	_ conformance.Env             = (*fakeEnv)(nil)
	_ conformance.Damager         = (*fakeEnv)(nil)
	_ conformance.ApplyFaulter    = (*fakeEnv)(nil)
	_ conformance.ConflictPlanter = (*fakeEnv)(nil)
	_ conformance.ImpostorPlanter = (*fakeEnv)(nil)
	_ conformance.ApplyCounter    = (*fakeEnv)(nil)
)

func newFakeEnv(t testing.TB) *fakeEnv {
	f, cfg := newFakeGost(t)
	return &fakeEnv{f: f, cfg: cfg}
}

func (e *fakeEnv) NewDriver(t testing.TB) driver.Driver {
	t.Helper()
	d, err := gost.New(e.cfg, gost.WithRunner(e.f), gost.WithSupervisor(e.f))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (e *fakeEnv) owned() bool {
	b, err := os.ReadFile(filepath.Join(e.cfg.Dir, gost.StateFile))
	return err == nil && strings.Contains(string(b), gost.OwnerMark)
}

func (e *fakeEnv) configDigest() string {
	b, err := os.ReadFile(e.f.config)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func (e *fakeEnv) Owned(testing.TB) []string {
	var out []string
	if d := e.configDigest(); d != "" && e.owned() {
		out = append(out, "config "+d)
	}
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	if e.f.running {
		out = append(out, "gost running")
		out = append(out, e.f.bound...)
	}
	return out
}

func (e *fakeEnv) PlantForeign(t testing.TB) { e.PlantConflict(t, 39999) }

func (e *fakeEnv) Foreign(testing.TB) []string {
	e.f.mu.Lock()
	out := slices.Clone(e.f.foreign)
	e.f.mu.Unlock()
	if d := e.configDigest(); d != "" && !e.owned() {
		out = append(out, "impostor config "+d)
	}
	return out
}

// Damage kills gost and leaves its socket behind, as a crash would.
func (e *fakeEnv) Damage(t testing.TB) {
	_ = e.f.Stop(context.Background())
	if err := os.WriteFile(e.f.socket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (e *fakeEnv) FailNextApply(testing.TB) {
	e.f.mu.Lock()
	e.f.failNext = true
	e.f.mu.Unlock()
}

func (e *fakeEnv) PlantConflict(_ testing.TB, port uint32) {
	e.f.mu.Lock()
	e.f.foreign = append(e.f.foreign, fmt.Sprintf("tcp LISTEN 0 4096 *:%d *:*", port))
	e.f.mu.Unlock()
}

func (e *fakeEnv) PlantImpostor(t testing.TB) {
	if err := os.WriteFile(e.f.config, []byte(`{"services": []}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (e *fakeEnv) Applies(testing.TB) int {
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	return e.f.applies
}

// TestConformanceFakeHost runs the conformance suite against the driver
// on a simulated gost and host, without privileges.
func TestConformanceFakeHost(t *testing.T) {
	var opts []conformance.Option
	for _, s := range f4bScenarios {
		opts = append(opts, conformance.Skip(s, "F4b: SetUpstreams changes gost's nodes through its web API"))
	}
	conformance.Run(t, func(t *testing.T) conformance.Env {
		e := newFakeEnv(t)
		e.cfg.ReadyTimeout = 300 * 1e6 // 300 ms: the fake serves at once or never
		return e
	}, opts...)
}
