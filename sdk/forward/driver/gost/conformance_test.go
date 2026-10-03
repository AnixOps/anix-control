package gost_test

import (
	"slices"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// renderOnlyEnv is a host the driver never touches, for the scenarios that
// only render: they run everywhere, without privileges. The whole suite
// runs against real gost in TestNetnsConformance (ANIXOPS_GOST_E2E=1,
// root) and against a simulated host in TestConformanceFakeHost.
type renderOnlyEnv struct{ mod func(*gost.Config) }

func (e renderOnlyEnv) NewDriver(t testing.TB) driver.Driver { return newDriver(t, e.mod) }
func (renderOnlyEnv) Owned(testing.TB) []string              { return nil }
func (renderOnlyEnv) PlantForeign(testing.TB)                {}
func (renderOnlyEnv) Foreign(testing.TB) []string {
	return []string{"render-only environment: nothing is applied"}
}

// renderScenarios are the scenarios that never call Apply.
var renderScenarios = []string{
	"capabilities",
	"render-determinism",
	"render-order-independent",
	"render-ignores-identity",
	"render-ignores-other-engines",
	"render-does-not-mutate",
	"render-unknown-enum",
}

// TestConformanceRender runs the render scenarios of the conformance suite
// without a host, with and without a link certificate (without one, TLS,
// WSS, QUIC and gRPC are unsupported capabilities).
func TestConformanceRender(t *testing.T) {
	var opts []conformance.Option
	for _, sc := range conformance.Scenarios() {
		if !slices.Contains(renderScenarios, sc.Name) {
			opts = append(opts, conformance.Skip(sc.Name, "needs a host: TestNetnsConformance runs it"))
		}
	}
	for name, mod := range map[string]func(*gost.Config){
		"link-tls": nil,
		"raw-only": func(c *gost.Config) { c.LinkCert, c.LinkKey, c.LinkCA = "", "", "" },
	} {
		t.Run(name, func(t *testing.T) {
			conformance.Run(t, func(*testing.T) conformance.Env { return renderOnlyEnv{mod: mod} }, opts...)
		})
	}
}
