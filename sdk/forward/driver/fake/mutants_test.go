package fake_test

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/fake"
)

// The mutants prove the suite catches what it claims to: each breaks one
// rule of the driver contract, and the suite must fail the named scenario.

type mutant struct {
	*fake.Driver
	host *fake.Host
	kind string
	seq  int
}

func (m *mutant) Render(s *forwardv1.NodeForwardState) (driver.Artifact, error) {
	a, err := m.Driver.Render(s)
	switch m.kind {
	case "nondeterministic-render":
		m.seq++
		a = driver.NewArtifact(a.Engine, s, a.Hops, append(a.Content, byte(m.seq)))
	case "digest-includes-generation":
		a = driver.NewArtifact(a.Engine, s, a.Hops, append(a.Content, byte(s.GetGeneration())))
	case "accepts-unsupported":
		err = nil
	}
	return a, err
}

func (m *mutant) Apply(ctx context.Context, a driver.Artifact) (driver.ApplyResult, error) {
	switch m.kind {
	case "ignores-context":
		ctx = context.Background()
	case "forgets-generation":
		_ = m.Driver.Remove(ctx)
	case "recreates-every-hop":
		// A structural change re-creates every hop, as a reload of the
		// whole engine would.
		if o, err := m.Driver.Observe(ctx); err == nil && o.Applied && !slices.Equal(keysOf(o), a.Hops) {
			_ = m.Driver.Remove(ctx)
		}
	}
	r, err := m.Driver.Apply(ctx, a)
	if m.kind == "always-changed" && err == nil {
		r.Changed = true
	}
	return r, err
}

func (m *mutant) Observe(ctx context.Context) (driver.Observation, error) {
	o, err := m.Driver.Observe(ctx)
	if m.kind == "counters-go-down" {
		m.seq++
		for _, c := range o.Counters {
			c.UpBytes = 1_000_000 / uint64(m.seq) // #nosec G115 -- seq > 0
		}
	}
	return o, err
}

func (m *mutant) Remove(ctx context.Context) error {
	if m.kind == "remove-touches-foreign" {
		m.host.PlantForeign("table ip nat", "flushed")
	}
	return m.Driver.Remove(ctx)
}

func (m *mutant) SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error {
	if m.kind == "set-upstreams-ignored" {
		return nil
	}
	return m.Driver.SetUpstreams(ctx, routeID, hopIndex, active)
}

func keysOf(o driver.Observation) []driver.HopKey {
	out := make([]driver.HopKey, 0, len(o.Counters))
	for _, c := range o.Counters {
		out = append(out, driver.HopKey{RouteID: c.GetRouteId(), HopIndex: c.GetHopIndex()})
	}
	return out
}

type mutantEnv struct {
	env
	kind string
}

func (e *mutantEnv) NewDriver(testing.TB) driver.Driver {
	return &mutant{Driver: fake.New(e.host, e.opts), host: e.host, kind: e.kind}
}

// TestMutantHelper runs the suite on the mutant named by FAKE_MUTANT; it is
// a child process of TestMutants and skips otherwise.
func TestMutantHelper(t *testing.T) {
	kind := os.Getenv("FAKE_MUTANT")
	if kind == "" {
		t.Skip("run by TestMutants")
	}
	conformance.Run(t, func(*testing.T) conformance.Env {
		return &mutantEnv{env: env{host: fake.NewHost(nil)}, kind: kind}
	})
}

func TestMutants(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns test processes")
	}
	for kind, scenario := range map[string]string{
		"nondeterministic-render":    "render-determinism",
		"digest-includes-generation": "render-ignores-identity",
		"accepts-unsupported":        "render-unsupported-capability",
		"ignores-context":            "context-cancelled",
		"forgets-generation":         "apply-stale-generation",
		"always-changed":             "apply-idempotent",
		"counters-go-down":           "observe-monotonic",
		"remove-touches-foreign":     "remove",
		"set-upstreams-ignored":      "set-upstreams-failover",
		"recreates-every-hop":        "apply-leaves-unrelated-hops",
	} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(os.Args[0], "-test.run=^TestMutantHelper$", "-test.v", "-test.count=1") // #nosec G204 -- re-runs this test binary
			cmd.Env = append(os.Environ(), "FAKE_MUTANT="+kind)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("the suite passed mutant %s", kind)
			}
			if !strings.Contains(string(out), "--- FAIL: TestMutantHelper/"+scenario+" ") {
				t.Fatalf("mutant %s did not fail scenario %s:\n%s", kind, scenario, out)
			}
		})
	}
}
