package gost_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// systemctlRunner answers systemctl show with show and records every
// command; gost and ss answer as the pinned release and an empty listing.
type systemctlRunner struct {
	mu    sync.Mutex
	show  string
	fail  map[string]error // by verb
	calls []string
	gostV string
	noSS  bool
}

func (r *systemctlRunner) Run(ctx context.Context, name string, args []string, _ []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	switch name {
	case "systemctl":
		if err := r.fail[args[0]]; err != nil {
			return nil, err
		}
		if args[0] == "show" {
			return []byte(r.show), nil
		}
		return nil, nil
	case "gost":
		if r.gostV == "" {
			return nil, &gost.CommandError{Name: name, Args: args, Err: exec.ErrNotFound}
		}
		return []byte(r.gostV + "\n"), nil
	case "ss":
		if r.noSS {
			return nil, &gost.CommandError{Name: name, Args: args, Stderr: "ss: not found", Err: exec.ErrNotFound}
		}
		return nil, nil
	}
	return nil, errors.New("unexpected command")
}

func TestSystemdSupervisor(t *testing.T) {
	ctx := t.Context()
	r := &systemctlRunner{show: "LoadState=loaded\nDescription=AnixOps forward gost (" + gost.OwnerMark + ")\nActiveState=active\nInvocationID=abc123\n"}
	s := gost.SystemdSupervisor{Runner: r}
	if err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	st, err := s.Status(ctx)
	if err != nil || !st.Running || st.Instance != "abc123" {
		t.Fatalf("Status %+v %v", st, err)
	}
	for _, f := range []func(context.Context) error{s.Start, s.Reload, s.Stop} {
		if err := f(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"systemctl show --property=LoadState,Description anixops-gost.service",
		"systemctl show --property=ActiveState,InvocationID anixops-gost.service",
		"systemctl start anixops-gost.service",
		"systemctl reload anixops-gost.service",
		"systemctl stop anixops-gost.service",
	}
	if !slices.Equal(r.calls, want) {
		t.Fatalf("commands\n got  %q\n want %q", r.calls, want)
	}

	r.show = "LoadState=not-found\nActiveState=inactive\n"
	if err := s.Check(ctx); !errors.Is(err, gost.ErrNoUnit) {
		t.Fatalf("Check of a missing unit: %v", err)
	}
	if st, _ := s.Status(ctx); st.Running {
		t.Fatal("inactive unit runs")
	}
	r.show = "LoadState=loaded\nDescription=gost\n"
	if err := s.Check(ctx); !errors.Is(err, driver.ErrNotOwned) {
		t.Fatalf("Check of a foreign unit: %v", err)
	}
	r.fail = map[string]error{"reload": errors.New("Job failed")}
	if err := s.Reload(ctx); err == nil {
		t.Fatal("failed reload")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := (gost.SystemdSupervisor{Runner: gost.ExecRunner{}}).Start(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start with a done context: %v", err)
	}
}

func TestProbe(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	base := gost.DefaultConfig()
	base.LinkCert, base.LinkKey, base.LinkCA = filepath.Join(dir, "c"), filepath.Join(dir, "k"), filepath.Join(dir, "ca")
	for _, p := range []string{base.LinkCert, base.LinkKey, base.LinkCA} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ok := "LoadState=loaded\nDescription=AnixOps forward gost (" + gost.OwnerMark + ")\n"
	for _, c := range []struct {
		name       string
		r          *systemctlRunner
		link       bool
		version    string
		unavail    string
		warn, miss string
	}{
		{name: "pinned", r: &systemctlRunner{gostV: "gost v3.2.6 (go1.25.4 linux/amd64)", show: ok}, link: true, version: "gost 3.2.6"},
		{name: "other-3.x", r: &systemctlRunner{gostV: "gost v3.3.0 (go1.26 linux/amd64)", show: ok}, link: true, version: "gost 3.3.0", warn: "not the pinned release"},
		{name: "missing", r: &systemctlRunner{show: ok}, unavail: "gost is not installed"},
		{name: "v2", r: &systemctlRunner{gostV: "gost 2.11.5 (go1.19)", show: ok}, unavail: "printed no version"},
		{name: "old-3", r: &systemctlRunner{gostV: "gost v3.0.0 (go1.21)", show: ok}, unavail: "older than 3.2"},
		{name: "no-ss", r: &systemctlRunner{gostV: "gost v3.2.6", show: ok, noSS: true}, unavail: "ss (iproute2)"},
		{name: "no-unit", r: &systemctlRunner{gostV: "gost v3.2.6", show: "LoadState=not-found\n"}, unavail: "not installed"},
		{name: "foreign-unit", r: &systemctlRunner{gostV: "gost v3.2.6", show: "LoadState=loaded\nDescription=gost\n"}, unavail: "Description"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg, rep, err := gost.Probe(ctx, c.r, gost.SystemdSupervisor{Runner: c.r}, base)
			if err != nil {
				t.Fatal(err)
			}
			if c.unavail != "" {
				if cfg.Version != "" || !strings.Contains(cfg.Unavailable, c.unavail) {
					t.Fatalf("version %q unavailable %q, want %q", cfg.Version, cfg.Unavailable, c.unavail)
				}
				d, _ := gost.New(cfg)
				caps, _ := d.Capabilities(ctx)
				if caps.GetAvailable() || caps.GetUnavailableReason() == "" {
					t.Fatalf("capabilities %v", caps)
				}
				return
			}
			if cfg.Version != c.version || (cfg.LinkCert != "") != c.link {
				t.Fatalf("version %q link %q", cfg.Version, cfg.LinkCert)
			}
			if c.warn != "" && !strings.Contains(strings.Join(rep.Warnings, ";"), c.warn) {
				t.Fatalf("warnings %v", rep.Warnings)
			}
		})
	}
	t.Run("link-files-missing", func(t *testing.T) {
		r := &systemctlRunner{gostV: "gost v3.2.6", show: ok}
		cfg, rep, err := gost.Probe(ctx, r, nil, gost.DefaultConfig())
		if err != nil || cfg.Version == "" || cfg.LinkCert != "" || len(rep.Missing) != 1 {
			t.Fatalf("%+v %+v %v", cfg, rep, err)
		}
		d, err := gost.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		caps, _ := d.Capabilities(ctx)
		if !slices.Equal(caps.GetLinkSecurities(), []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW}) || caps.GetQuota() {
			t.Fatalf("capabilities %v", caps)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		c, cancel := context.WithCancel(ctx)
		cancel()
		if _, _, err := gost.Probe(c, &systemctlRunner{}, nil, base); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

func TestNewRefusesConfig(t *testing.T) {
	for name, mod := range map[string]func(*gost.Config){
		"relative dir":         func(c *gost.Config) { c.Dir = "var/lib/gost" },
		"unclean dir":          func(c *gost.Config) { c.Dir = "/var/lib/../gost" },
		"space in path":        func(c *gost.Config) { c.LinkCert = "/a b" },
		"link paths partial":   func(c *gost.Config) { c.LinkKey = "" },
		"socket path too long": func(c *gost.Config) { c.RuntimeDir = "/run/" + strings.Repeat("x", 100) },
		"no ready timeout":     func(c *gost.Config) { c.ReadyTimeout = 0 },
		"unknown strategy":     func(c *gost.Config) { c.Strategies = []forwardv1.BalanceStrategy{42} },
		"version not text":     func(c *gost.Config) { c.Version = "gost\n3" },
	} {
		c := gost.DefaultConfig()
		mod(&c)
		if _, err := gost.New(c); !errors.Is(err, gost.ErrInvalidConfig) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestPinnedVersionMatchesCI keeps PinnedVersion and the release ci.yml
// downloads (and checks by SHA-256) for the real-gost tests the same.
func TestPinnedVersionMatchesCI(t *testing.T) {
	b, err := os.ReadFile("../../../../.github/workflows/ci.yml")
	if err != nil {
		t.Skipf("ci.yml not found: %v", err)
	}
	m := regexp.MustCompile(`(?m)^\s*GOST_VERSION:\s*'([^']+)'`).FindSubmatch(b)
	if m == nil || string(m[1]) != gost.PinnedVersion {
		t.Fatalf("ci.yml GOST_VERSION %q, PinnedVersion %q", m, gost.PinnedVersion)
	}
}

func TestUnitFile(t *testing.T) {
	u, err := gost.UnitFile(gost.DefaultUnit())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Description=AnixOps forward gost (" + gost.OwnerMark + ")",
		"ExecStart=" + gost.DefaultBinary + " -C " + gost.DefaultDir + "/" + gost.ConfigFile,
		"ExecReload=/bin/kill -HUP $MAINPID",
		"User=" + gost.DefaultUser,
		"RuntimeDirectory=anixops-gost",
		"AmbientCapabilities=CAP_NET_BIND_SERVICE\n",
		"CapabilityBoundingSet=CAP_NET_BIND_SERVICE\n",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
	} {
		if !bytes.Contains(u, []byte(want)) {
			t.Errorf("unit lacks %q", want)
		}
	}
	if bytes.Contains(u, []byte("CAP_NET_ADMIN")) {
		t.Error("gost gets CAP_NET_ADMIN")
	}
	for name, mod := range map[string]func(*gost.Unit){
		"runtime dir outside /run": func(u *gost.Unit) { u.RuntimeDir = "/var/run2/x" },
		"nested runtime dir":       func(u *gost.Unit) { u.RuntimeDir = "/run/a/b" },
		"bad user":                 func(u *gost.Unit) { u.User = "root; rm" },
		"bad binary":               func(u *gost.Unit) { u.Binary = "gost" },
	} {
		u := gost.DefaultUnit()
		mod(&u)
		if _, err := gost.UnitFile(u); !errors.Is(err, gost.ErrInvalidConfig) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// systemd-analyze verify checks the syntax and every directive when it
	// is available. It needs the binary to exist; nothing is installed.
	sa, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze not available")
	}
	dir := t.TempDir()
	v := gost.DefaultUnit()
	v.Binary, _ = exec.LookPath("true")
	v.Dir = dir
	u, err = gost.UnitFile(v)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, gost.UnitName)
	if err := os.WriteFile(path, u, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(sa, "verify", "--man=no", path).CombinedOutput(); err != nil { // #nosec G204 -- systemd-analyze on a temporary file
		t.Fatalf("systemd-analyze verify: %v\n%s", err, out)
	}
}
