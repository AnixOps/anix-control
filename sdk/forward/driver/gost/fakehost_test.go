package gost_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// fakeGost simulates gost and the host for the driver without privileges:
// it is the Supervisor (a "process" that loads the configuration file,
// serves its metrics path and its web API on the unix sockets and "binds"
// its listeners) and the Runner (ss lists those listeners, planted
// foreign ones and the connections of conns; gost -V prints the pinned
// version). A reload that cannot bind a listener closes every service, as
// gost's does. The API keeps per service the statistics Traffic adds and
// a creation time from a counter, re-created by every start and reload,
// and lets the driver replace hops, admissions and limiters; it refuses a
// hop whose failTimeout is not a number, as gost's binding would zero it.
type fakeGost struct {
	config, socket, apiSocket, dir string

	mu       sync.Mutex
	running  bool
	instance int
	path     string   // the metrics path being served
	bound    []string // ss lines of the services' sockets
	srv, api *http.Server
	foreign  []string // ss lines of planted foreign sockets
	hidden   []string // foreign sockets ss does not show yet (taken after a check)
	conns    []string // ss lines of established sockets
	failNext bool
	failAPI  int // the next failAPI hot changes fail
	applies  int
	puts     int
	created  int64
	live     *fakeLive
}

// fakeLive is the running configuration of a fakeGost.
type fakeLive struct {
	services []*fakeService
	objects  map[string]map[string]json.RawMessage // kind -> name -> object
	order    map[string][]string                   // kind -> names in file order
}

type fakeService struct {
	Name    string
	Created int64
	Stats   struct{ TotalConns, CurrentConns, InputBytes, OutputBytes uint64 }
}

var hotKinds = []string{"hops", "admissions", "limiters", "climiters"}

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
	f := &fakeGost{
		config: filepath.Join(cfg.Dir, gost.ConfigFile), dir: cfg.Dir,
		socket: filepath.Join(cfg.RuntimeDir, gost.MetricsSocket), apiSocket: filepath.Join(cfg.RuntimeDir, gost.APISocket),
	}
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
	case name == "ss" && slices.Equal(args, []string{"-H", "-t", "-u", "-n"}):
		return []byte(strings.Join(f.conns, "\n") + "\n"), nil
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

// load reads the configuration file as gost would: the metrics path, the
// sockets of its services and its objects.
func (f *fakeGost) load() (path string, bound []string, live *fakeLive, ok bool) {
	b, err := os.ReadFile(f.config)
	if err != nil || f.failNext {
		f.failNext = false
		return "", nil, nil, false
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
		Services []struct{ Name string }
		Metrics  struct{ Path string }
	}
	var objs map[string]json.RawMessage
	if json.Unmarshal(b, &c) != nil || c.Metrics.Path == "" || json.Unmarshal(b, &objs) != nil {
		return "", nil, nil, false
	}
	live = &fakeLive{objects: map[string]map[string]json.RawMessage{}, order: map[string][]string{}}
	for _, kind := range hotKinds {
		var list []json.RawMessage
		_ = json.Unmarshal(objs[kind], &list)
		live.objects[kind] = map[string]json.RawMessage{}
		for _, o := range list {
			var n struct{ Name string }
			_ = json.Unmarshal(o, &n)
			live.objects[kind][n.Name] = o
			live.order[kind] = append(live.order[kind], n.Name)
		}
	}
	for _, s := range c.Services {
		f.created++
		live.services = append(live.services, &fakeService{Name: s.Name, Created: f.created})
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
	return c.Metrics.Path, bound, live, true
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
	path, bound, live, ok := f.load()
	if !ok || f.taken(bound) {
		return nil // gost exits before serving
	}
	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		return nil // a stale socket: gost exits
	}
	aln, err := net.Listen("unix", f.apiSocket)
	if err != nil {
		_ = ln.Close()
		return nil
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path != f.path {
			http.NotFound(w, r)
		}
	})}
	api := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(f.serveAPI)}
	go func() { _ = srv.Serve(ln) }()
	go func() { _ = api.Serve(aln) }()
	f.running, f.srv, f.api, f.path, f.bound, f.live = true, srv, api, path, bound, live
	f.instance++
	return nil
}

// serveAPI answers GET /config and PUT /config/<hot kind>/<name>.
func (f *fakeGost) serveAPI(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.live == nil {
		http.Error(w, `{"msg":"not running"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/config" {
		out := map[string]any{}
		var svcs []map[string]any
		for _, s := range f.live.services {
			svcs = append(svcs, map[string]any{"name": s.Name, "status": map[string]any{
				"createTime": s.Created, "state": "ready",
				"stats": map[string]uint64{"totalConns": s.Stats.TotalConns, "currentConns": s.Stats.CurrentConns, "inputBytes": s.Stats.InputBytes, "outputBytes": s.Stats.OutputBytes},
			}})
		}
		out["services"] = svcs
		for _, kind := range hotKinds {
			var list []json.RawMessage
			for _, n := range f.live.order[kind] {
				list = append(list, f.live.objects[kind][n])
			}
			out[kind] = list
		}
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	kind, name, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/config/"), "/")
	if r.Method != http.MethodPut || !ok || f.live.objects[kind] == nil {
		http.Error(w, `{"msg":"unsupported"}`, http.StatusNotFound)
		return
	}
	if _, ok := f.live.objects[kind][name]; !ok {
		http.Error(w, `{"code":40004,"msg":"`+kind+` `+name+` not found"}`, http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	var obj map[string]any
	if err != nil || json.Unmarshal(body, &obj) != nil || obj["name"] != name {
		http.Error(w, `{"code":40001,"msg":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if kind == "hops" {
		sel, _ := obj["selector"].(map[string]any)
		if _, isNumber := sel["failTimeout"].(float64); !isNumber {
			http.Error(w, `{"code":40001,"msg":"failTimeout is not a duration in nanoseconds"}`, http.StatusBadRequest)
			return
		}
	}
	if f.failAPI > 0 {
		f.failAPI--
		http.Error(w, `{"code":50000,"msg":"injected failure"}`, http.StatusInternalServerError)
		return
	}
	f.puts++
	f.live.objects[kind][name] = body
	_, _ = io.WriteString(w, `{"msg":"OK"}`)
}

// object answers a running object of the fake, decoded.
func (f *fakeGost) object(kind, name string, v any) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.live == nil || f.live.objects[kind][name] == nil {
		return false
	}
	return json.Unmarshal(f.live.objects[kind][name], v) == nil
}

// traffic adds to the statistics of the services named prefix-*.
func (f *fakeGost) traffic(prefix string, up, down uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.live == nil {
		return false
	}
	found := false
	for _, s := range f.live.services {
		if strings.HasPrefix(s.Name, prefix+"-") {
			s.Stats.TotalConns++
			s.Stats.InputBytes += up
			s.Stats.OutputBytes += down
			found = true
		}
	}
	return found
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
	path, bound, live, ok := f.load()
	switch {
	case !ok: // a configuration gost cannot parse: nothing changes
	case f.taken(bound): // a listener gost cannot bind: every service closed
		f.bound = nil
		f.live.services = nil
	default:
		f.path, f.bound, f.live = path, bound, live
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
		_ = f.api.Close()
		f.running, f.srv, f.api, f.path, f.bound, f.live = false, nil, nil, "", nil, nil
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
	_ conformance.TrafficSource   = (*fakeEnv)(nil)
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

// Damage kills gost and leaves its sockets behind, as a crash would.
func (e *fakeEnv) Damage(t testing.TB) {
	_ = e.f.Stop(context.Background())
	for _, s := range []string{e.f.socket, e.f.apiSocket} {
		if err := os.WriteFile(s, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// Traffic adds bytes and a connection to the hop's services.
func (e *fakeEnv) Traffic(t testing.TB, hop driver.HopKey) {
	t.Helper()
	if !e.f.traffic(fmt.Sprintf("r%s-h%d", hop.RouteID, hop.HopIndex), 1000, 3000) {
		t.Fatalf("Traffic: gost runs no service of %s", hop)
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

// TestConformanceFakeHost runs the whole conformance suite against the
// driver on a simulated gost and host, without privileges.
func TestConformanceFakeHost(t *testing.T) {
	conformance.Run(t, func(t *testing.T) conformance.Env {
		e := newFakeEnv(t)
		e.cfg.ReadyTimeout = 300 * time.Millisecond // the fake serves at once or never
		return e
	})
}
