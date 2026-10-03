package nftables_test

import (
	"slices"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// renderOnlyEnv is a host the driver never touches, for the scenarios that
// only render: they run everywhere, without privileges. The whole suite
// runs on a real kernel in TestNetnsConformance (ANIXOPS_NFT_E2E=1, root).
type renderOnlyEnv struct{}

func (renderOnlyEnv) NewDriver(t testing.TB) driver.Driver { return newDriver(t, nil) }
func (renderOnlyEnv) Owned(testing.TB) []string            { return nil }
func (renderOnlyEnv) PlantForeign(testing.TB)              {}
func (renderOnlyEnv) Foreign(testing.TB) []string {
	return []string{"render-only environment: nothing is applied"}
}

// renderScenarios are the scenarios that never call Apply. The other
// render scenarios (render-empty-state, render-unsupported-capability,
// render-invalid-state) apply what they render; render_test.go covers their
// Render half.
var renderScenarios = []string{
	"capabilities",
	"render-determinism",
	"render-order-independent",
	"render-ignores-identity",
	"render-ignores-other-engines",
	"render-does-not-mutate",
	"render-unknown-enum",
}

// TestConformanceRender runs the render scenarios of the conformance
// suite without a host.
func TestConformanceRender(t *testing.T) {
	var opts []conformance.Option
	for _, sc := range conformance.Scenarios() {
		if !slices.Contains(renderScenarios, sc.Name) {
			opts = append(opts, conformance.Skip(sc.Name, "needs a host: TestNetnsConformance runs it"))
		}
	}
	conformance.Run(t, func(*testing.T) conformance.Env { return renderOnlyEnv{} }, opts...)
}
