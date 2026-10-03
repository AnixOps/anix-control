package gost_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

func fakeDriver(t testing.TB) (*gost.Driver, *fakeGost, gost.Config) {
	t.Helper()
	f, cfg := newFakeGost(t)
	cfg.ReadyTimeout = 300 * time.Millisecond
	d, err := gost.New(cfg, gost.WithRunner(f), gost.WithSupervisor(f))
	if err != nil {
		t.Fatal(err)
	}
	return d, f, cfg
}

// TestApplyRecoversFromHalfFailedReload: a listener taken between the
// driver's check and gost's reload makes gost close every service; Apply
// fails and restarts gost on the previous configuration.
func TestApplyRecoversFromHalfFailedReload(t *testing.T) {
	d, f, _ := fakeDriver(t)
	b := builder(t)
	a1 := render(t, d, conformance.State("forward-11", 1, b.Simple(conformance.RouteA, 0)))
	if _, err := d.Apply(t.Context(), a1); err != nil {
		t.Fatal(err)
	}
	inst := f.instance
	a2 := render(t, d, conformance.State("forward-11", 2, b.Simple(conformance.RouteA, 0), b.Simple(conformance.RouteB, 1)))
	f.hidden = []string{"tcp LISTEN 0 4096 *:30002 *:*"}
	if _, err := d.Apply(t.Context(), a2); err == nil {
		t.Fatal("Apply succeeded although gost closed its services")
	}
	o, err := d.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if o.Generation != 1 || o.Digest != a1.Digest {
		t.Fatalf("after the failed apply the host runs %d %s", o.Generation, o.Digest)
	}
	if f.instance == inst {
		t.Fatal("gost was not restarted on the previous configuration")
	}
	if r, err := d.Apply(t.Context(), a1); err != nil || r.Changed {
		t.Fatalf("the host does not run the previous artifact: %+v %v", r, err)
	}
	// Once ss shows the socket, the check refuses before touching gost.
	f.foreign, f.hidden = f.hidden, nil
	inst = f.instance
	if _, err := d.Apply(t.Context(), a2); !errors.Is(err, driver.ErrConflict) {
		t.Fatalf("Apply onto a foreign socket: %v", err)
	}
	if f.instance != inst || !f.running {
		t.Fatal("a refused apply touched gost")
	}
}

// TestApplyFirstFailureLeavesNothing: when gost refuses the first
// configuration, Apply stops it and deletes both files.
func TestApplyFirstFailureLeavesNothing(t *testing.T) {
	d, f, cfg := fakeDriver(t)
	f.failNext = true
	a := render(t, d, conformance.State("forward-11", 1, builder(t).Simple(conformance.RouteA, 0)))
	if _, err := d.Apply(t.Context(), a); err == nil {
		t.Fatal("Apply succeeded although gost refused the configuration")
	}
	for _, name := range []string{gost.ConfigFile, gost.StateFile} {
		if _, err := os.Stat(filepath.Join(cfg.Dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left behind: %v", name, err)
		}
	}
	if f.running {
		t.Error("gost still runs")
	}
	if _, err := d.Apply(t.Context(), a); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

// TestApplyCancelledInFlight: a context that ends while Apply waits for gost
// leaves the host on the previous artifact.
func TestApplyCancelledInFlight(t *testing.T) {
	d, f, _ := fakeDriver(t)
	b := builder(t)
	a1 := render(t, d, conformance.State("forward-11", 1, b.Simple(conformance.RouteA, 0)))
	if _, err := d.Apply(t.Context(), a1); err != nil {
		t.Fatal(err)
	}
	f.failNext = true // gost never serves the next one, so Apply waits
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	a2 := render(t, d, conformance.State("forward-11", 2, b.Simple(conformance.RouteB, 1)))
	if _, err := d.Apply(ctx, a2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Apply: %v, want the context's error", err)
	}
	if r, err := d.Apply(t.Context(), a1); err != nil || r.Changed {
		t.Fatalf("the host does not run the previous artifact: %+v %v", r, err)
	}
}

// TestApplyRefusesForeignUnit: a unit of the driver's name without its
// mark is ErrNotOwned for Apply and left alone by Remove.
func TestApplyRefusesForeignUnit(t *testing.T) {
	_, f, cfg := fakeDriver(t)
	r := &systemctlRunner{show: "LoadState=loaded\nDescription=someone's gost\n"}
	d, err := gost.New(cfg, gost.WithRunner(f), gost.WithSupervisor(gost.SystemdSupervisor{Runner: r}))
	if err != nil {
		t.Fatal(err)
	}
	a := render(t, d, conformance.State("forward-11", 1, builder(t).Simple(conformance.RouteA, 0)))
	if _, err := d.Apply(t.Context(), a); !errors.Is(err, driver.ErrNotOwned) {
		t.Fatalf("Apply with a foreign unit: %v", err)
	}
	if err := d.Remove(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(r.calls, ";"), "stop") {
		t.Fatalf("Remove stopped a foreign unit: %v", r.calls)
	}
}
