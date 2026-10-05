package anixops_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/anixopstest"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// scripted is a Runner that answers from a table of "name arg arg" commands.
type scripted struct {
	mu    sync.Mutex
	out   map[string]string
	errs  map[string]error
	calls []string
}

func (s *scripted) Run(ctx context.Context, name string, args []string, _ []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, key)
	if err, ok := s.errs[key]; ok {
		return nil, err
	}
	return []byte(s.out[key]), nil
}

func TestProbe(t *testing.T) {
	dir := t.TempDir()
	link := func(names ...string) (c, k, ca string) {
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return filepath.Join(dir, "link.crt"), filepath.Join(dir, "link.key"), filepath.Join(dir, "ca.crt")
	}
	sys := t.TempDir()
	for _, n := range []string{"rmem_max", "wmem_max"} {
		if err := os.WriteFile(filepath.Join(sys, n), []byte("212992\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defer anixops.SetSysctlDir(sys)()

	base := anixops.DefaultConfig()
	base.LinkCert, base.LinkKey, base.LinkCA = link("link.crt", "link.key", "ca.crt")
	version := "anixops-relay 4.2.0-rc.2 wire=0"
	runner := &scripted{out: map[string]string{"anixops-relay -V": version + "\n"}}
	unit := &scripted{out: map[string]string{"systemctl show --property=LoadState,Description anixops-relay.service": "LoadState=loaded\nDescription=AnixOps forward relay (" + anixops.OwnerMark + ")\n"}}
	sup := anixops.SystemdSupervisor{Runner: unit}

	t.Run("a good host", func(t *testing.T) {
		cfg, rep, err := anixops.Probe(t.Context(), runner, sup, base)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Version != "anixops-relay 4.2.0-rc.2" || cfg.Unavailable != "" || !slices.Equal(cfg.ProtocolVersions, []uint32{0}) {
			t.Fatalf("probed %+v", cfg)
		}
		if len(cfg.Carriers) != 3 {
			t.Fatalf("carriers %v", cfg.Carriers)
		}
		if len(rep.Missing) != 0 || len(rep.Warnings) != 2 {
			t.Fatalf("report %+v: want the two socket buffer warnings", rep)
		}
		d, err := anixops.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		caps, _ := d.Capabilities(t.Context())
		if !caps.GetAvailable() || caps.GetProxyProtocol() || !slices.Equal(caps.GetProtocolVersions(), []uint32{0}) ||
			!slices.Equal(caps.GetLinkSecurities(), []forwardv1.LinkSecurity{secRAW, secAnixOps}) ||
			!slices.Equal(caps.GetCarriers(), []forwardv1.AnixOpsCarrier{carTLS, carQUIC, carPlain}) {
			t.Fatalf("capabilities %v", caps)
		}
	})

	t.Run("wire versions come from the binary", func(t *testing.T) {
		r := &scripted{out: map[string]string{"anixops-relay -V": "anixops-relay dev wire=1,0\n"}}
		cfg, _, err := anixops.Probe(t.Context(), r, sup, base)
		if err != nil || !slices.Equal(cfg.ProtocolVersions, []uint32{1, 0}) {
			t.Fatalf("%+v %v", cfg.ProtocolVersions, err)
		}
	})

	t.Run("no link files leaves PLAIN", func(t *testing.T) {
		b := base
		b.LinkCert = filepath.Join(dir, "missing.crt")
		cfg, rep, err := anixops.Probe(t.Context(), runner, sup, b)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(cfg.Carriers, []forwardv1.AnixOpsCarrier{carPlain}) || cfg.LinkCert != "" || len(rep.Missing) != 1 || len(rep.Warnings) != 0 {
			t.Fatalf("%+v %+v", cfg.Carriers, rep)
		}
	})

	for name, c := range map[string]struct {
		runner *scripted
		sup    anixops.Supervisor
		want   string
	}{
		"binary missing": {&scripted{errs: map[string]error{"anixops-relay -V": exec_notfound()}}, sup, "not installed"},
		"binary broken":  {&scripted{errs: map[string]error{"anixops-relay -V": errors.New("segfault")}}, sup, "segfault"},
		"no version":     {&scripted{out: map[string]string{"anixops-relay -V": "hello\n"}}, sup, "printed no version"},
		"no unit":        {runner, anixops.SystemdSupervisor{Runner: &scripted{out: map[string]string{"systemctl show --property=LoadState,Description anixops-relay.service": "LoadState=not-found\n"}}}, "not installed"},
		"foreign unit":   {runner, anixops.SystemdSupervisor{Runner: &scripted{out: map[string]string{"systemctl show --property=LoadState,Description anixops-relay.service": "LoadState=loaded\nDescription=someone else's\n"}}}, "does not carry"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, _, err := anixops.Probe(t.Context(), c.runner, c.sup, base)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Version != "" || !strings.Contains(cfg.Unavailable, c.want) {
				t.Fatalf("unavailable %q (version %q), want %q", cfg.Unavailable, cfg.Version, c.want)
			}
			d, _ := anixops.New(cfg)
			if caps, _ := d.Capabilities(t.Context()); caps.GetAvailable() || caps.GetUnavailableReason() == "" {
				t.Fatalf("capabilities of an unavailable host: %v", caps)
			}
		})
	}

	t.Run("a done context is the only error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, _, err := anixops.Probe(ctx, runner, sup, base); !errors.Is(err, context.Canceled) {
			t.Fatalf("%v", err)
		}
	})
}

func exec_notfound() error {
	_, err := exec.LookPath("anixops-relay-that-does-not-exist")
	return fmt.Errorf("run: %w", err)
}

func TestSystemdSupervisor(t *testing.T) {
	r := &scripted{out: map[string]string{
		"systemctl show --property=ActiveState anixops-relay.service": "ActiveState=active\n",
	}}
	s := anixops.SystemdSupervisor{Runner: r}
	st, err := s.Status(t.Context())
	if err != nil || !st.Running {
		t.Fatalf("%+v %v", st, err)
	}
	r.out["systemctl show --property=ActiveState anixops-relay.service"] = "ActiveState=activating\n"
	if st, _ := s.Status(t.Context()); st.Running || !st.Starting {
		t.Fatalf("%+v", st)
	}
	r.out["systemctl show --property=ActiveState anixops-relay.service"] = "ActiveState=inactive\n"
	if st, _ := s.Status(t.Context()); st.Running || st.Starting {
		t.Fatalf("%+v", st)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.calls, "systemctl start anixops-relay.service") || !slices.Contains(r.calls, "systemctl stop anixops-relay.service") {
		t.Fatalf("calls %v", r.calls)
	}
	r.errs = map[string]error{"systemctl start anixops-relay.service": errors.New("denied")}
	if err := s.Start(t.Context()); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("%v", err)
	}
	// The runner refuses anything else.
	if _, err := (anixops.ExecRunner{}).Run(t.Context(), "rm", []string{"-rf", "/"}, nil); err == nil {
		t.Fatal("ExecRunner ran a command it should refuse")
	}
}

func TestUnitFile(t *testing.T) {
	good := anixops.DefaultUnit()
	if _, err := anixops.UnitFile(good); err != nil {
		t.Fatal(err)
	}
	for name, mod := range map[string]func(*anixops.Unit){
		"binary with a space":      func(u *anixops.Unit) { u.Binary = "/usr/lib/a b/relay" },
		"relative dir":             func(u *anixops.Unit) { u.Dir = "var/lib/relay" },
		"runtime dir outside /run": func(u *anixops.Unit) { u.RuntimeDir = "/tmp/relay" },
		"nested runtime dir":       func(u *anixops.Unit) { u.RuntimeDir = "/run/a/b" },
		"user with a space":        func(u *anixops.Unit) { u.User = "any ops" },
		"group with a newline":     func(u *anixops.Unit) { u.Group = "g\nExecStart=/bin/sh" },
	} {
		t.Run(name, func(t *testing.T) {
			u := good
			mod(&u)
			if _, err := anixops.UnitFile(u); !errors.Is(err, anixops.ErrInvalidConfig) {
				t.Fatalf("%v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestNewRefusesBadConfig(t *testing.T) {
	for name, mod := range map[string]func(*anixops.Config){
		"relative dir":         func(c *anixops.Config) { c.Dir = "relative" },
		"partial link files":   func(c *anixops.Config) { c.LinkKey = "" },
		"AUTO as a carrier":    func(c *anixops.Config) { c.Carriers = append(c.Carriers, carAuto) },
		"encrypted w/o files":  func(c *anixops.Config) { c.LinkCert, c.LinkKey, c.LinkCA = "", "", "" },
		"unknown strategy":     func(c *anixops.Config) { c.Strategies = append(c.Strategies, 99) },
		"zero ready timeout":   func(c *anixops.Config) { c.ReadyTimeout = 0 },
		"unprintable version":  func(c *anixops.Config) { c.Version = "x\n" },
		"socket path too long": func(c *anixops.Config) { c.RuntimeDir = "/run/" + strings.Repeat("a", 120) },
	} {
		t.Run(name, func(t *testing.T) {
			c := testConfig()
			mod(&c)
			if _, err := anixops.New(c); !errors.Is(err, anixops.ErrInvalidConfig) {
				t.Fatalf("%v, want ErrInvalidConfig", err)
			}
		})
	}
}

// hostEnv is a host with echo targets, and the conformance topology on free
// ports.
type hostEnv struct {
	t    *testing.T
	host *anixopstest.Host
	b    conformance.Builder
}

func newHostEnv(t *testing.T) *hostEnv {
	host := anixopstest.NewHost(t)
	tcp, udp := echo(t)
	host.SetTarget("tcp", tcp)
	host.SetTarget("udp", udp)
	top := conformance.DefaultTopology()
	top.ListenPorts = []uint32{freePort(t), freePort(t), freePort(t), freePort(t)}
	top.IngressSources = []string{"127.0.0.1"}
	caps, err := host.Driver().Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return &hostEnv{t: t, host: host, b: conformance.Builder{Engine: forwardv1.Engine_ENGINE_ANIXOPS, Caps: caps, Top: top}}
}

func (e *hostEnv) state(gen uint64, hops ...*forwardv1.NodeHop) *forwardv1.NodeForwardState {
	return conformance.State(e.b.Top.NodeRef, gen, hops...)
}

func (e *hostEnv) apply(d driver.Driver, s *forwardv1.NodeForwardState) driver.Artifact {
	e.t.Helper()
	a, err := d.Render(s)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := d.Apply(e.t.Context(), a); err != nil {
		e.t.Fatal(err)
	}
	return a
}

func echoOnce(t *testing.T, port uint32, msg string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(port)), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != msg {
		t.Fatalf("echo %q, %v", buf, err)
	}
}

func TestFirstApplyThatConflictsLeavesNothing(t *testing.T) {
	e := newHostEnv(t)
	port := e.b.Top.ListenPorts[0]
	l, err := net.Listen("tcp", ":"+strconv.Itoa(int(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	d := e.host.Driver()
	a, err := d.Render(e.state(1, e.b.Simple(conformance.RouteA, 0)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Apply(t.Context(), a); !errors.Is(err, driver.ErrConflict) {
		t.Fatalf("Apply onto a held port: %v, want ErrConflict", err)
	}
	if e.host.Relay() != nil {
		t.Fatal("a first apply that failed left the relay running")
	}
	entries, _ := os.ReadDir(e.host.Dir)
	if len(entries) != 0 {
		t.Fatalf("a first apply that failed left files: %v", entries)
	}
	if o, err := d.Observe(t.Context()); err != nil || o.Applied {
		t.Fatalf("observed %+v, %v", o, err)
	}
	// With the port free the same artifact applies.
	_ = l.Close()
	if _, err := d.Apply(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	echoOnce(t, port, "hello")
}

func TestRelayCrashIsRepairedByTheNextApply(t *testing.T) {
	e := newHostEnv(t)
	d := e.host.Driver()
	hop := e.b.Simple(conformance.RouteA, 0)
	a := e.apply(d, e.state(1, hop))
	port := hop.GetListen().GetPort()
	echoOnce(t, port, "before the crash")
	before, err := d.Observe(t.Context())
	if err != nil || before.Counters[0].CounterEpoch == "stopped" {
		t.Fatalf("%+v %v", before, err)
	}

	e.host.Kill() // the supervisor restarts it in production; here nothing does
	o, err := d.Observe(t.Context())
	if err != nil || !o.Applied || o.Counters[0].CounterEpoch != "stopped" || o.Counters[0].UpBytes != 0 {
		t.Fatalf("a dead relay observes as %+v, %v", o, err)
	}
	if len(o.Rotation) != 1 || len(o.Rotation[0].Active) != len(hop.GetUpstreams()) {
		t.Fatalf("the rotation of a dead relay: %+v", o.Rotation)
	}
	res, err := d.Apply(t.Context(), a)
	if err != nil || !res.Changed {
		t.Fatalf("re-applying onto a dead relay: %+v, %v", res, err)
	}
	echoOnce(t, port, "after the repair")
	after, err := d.Observe(t.Context())
	if err != nil || after.Counters[0].CounterEpoch == "stopped" || after.Counters[0].CounterEpoch == before.Counters[0].CounterEpoch {
		t.Fatalf("the epoch must be new after a restart: %v then %v", before.Counters[0].CounterEpoch, after.Counters[0].CounterEpoch)
	}
}

func TestRetiredCountersReachTheHook(t *testing.T) {
	e := newHostEnv(t)
	var got []*forwardv1.Counters
	d := e.host.Driver(anixops.WithRetiredCounters(func(c []*forwardv1.Counters) { got = append(got, c...) }))
	hop := e.b.Simple(conformance.RouteA, 0)
	e.apply(d, e.state(1, hop))
	echoOnce(t, hop.GetListen().GetPort(), "some payload")
	time.Sleep(50 * time.Millisecond)
	first, err := d.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Route A moves to another port: its listener is re-created, a new epoch.
	moved := e.b.Simple(conformance.RouteA, 0)
	moved.Listen.Port = e.b.Top.ListenPorts[3]
	e.apply(d, e.state(2, moved))
	second, err := d.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if first.Counters[0].CounterEpoch == second.Counters[0].CounterEpoch {
		t.Fatal("a moved listener kept its epoch")
	}
	if len(got) != 1 || got[0].GetCounterEpoch() != first.Counters[0].CounterEpoch || got[0].GetUpBytes() == 0 || got[0].GetNodeRef() != first.NodeRef {
		t.Fatalf("retired counters %v, want the first epoch's final counters", got)
	}
	if _, err := d.Observe(t.Context()); err != nil || len(got) != 1 {
		t.Fatalf("the retired list drains once: %v %v", got, err)
	}
}

func TestSetUpstreamsNamesAnUpstreamTheWayRenderDid(t *testing.T) {
	e := newHostEnv(t)
	d := e.host.Driver()
	hop := e.b.Entry(conformance.RouteA, 0, forwardv1.L4Protocol_L4_PROTOCOL_TCP, forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER, e.b.Upstreams(e.b.Top.UpstreamsV6, 2))
	hop.Upstreams[0].Address = "2001:DB8::20" // upper case, as an operator typed it
	e.apply(d, e.state(1, hop))
	// The rendered form is the canonical one; both spellings find it.
	for _, spelling := range []string{"2001:db8::20", "2001:DB8::20"} {
		if err := d.SetUpstreams(t.Context(), conformance.RouteA, 0, []driver.Upstream{{Address: spelling, Port: e.b.Top.UpstreamPort}}); err != nil {
			t.Fatalf("%s: %v", spelling, err)
		}
	}
	o, err := d.Observe(t.Context())
	if err != nil || len(o.Rotation[0].Active) != 1 || o.Rotation[0].Active[0].Address != "2001:db8::20" {
		t.Fatalf("%+v %v", o.Rotation, err)
	}
}

func TestRemoveLeavesTheUnitAndForeignFiles(t *testing.T) {
	e := newHostEnv(t)
	d := e.host.Driver()
	e.apply(d, e.state(1, e.b.Simple(conformance.RouteA, 0)))
	foreign := filepath.Join(e.host.Dir, "tls", "link.crt")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("the node's certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("Remove deleted the link files: %v", err)
	}
	for _, name := range []string{anixops.ConfigFile, anixops.StateFile} {
		if _, err := os.Stat(filepath.Join(e.host.Dir, name)); err == nil {
			t.Fatalf("Remove left %s", name)
		}
	}
	if e.host.Relay() != nil {
		t.Fatal("Remove left the relay running")
	}
}

func TestReloadCredentials(t *testing.T) {
	e := newHostEnv(t)
	d := e.host.Driver()
	// Not running: nothing to reload; the next start reads the files.
	if err := d.ReloadCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.apply(d, e.state(1, e.b.Simple(conformance.RouteA, 0)))
	// Running without link files: the relay says so.
	if err := d.ReloadCredentials(t.Context()); err == nil {
		t.Fatal("a reload on a relay without link credentials succeeded")
	}
}
