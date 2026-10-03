package nftables_test

import (
	"slices"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// renderOnlyEnv is a host the driver never touches: F2b has Render only,
// so the conformance scenarios that apply are skipped until F2c.
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
// suite. The suite runs whole with Apply in F2c and on network namespaces
// in F2d.
func TestConformanceRender(t *testing.T) {
	var opts []conformance.Option
	for _, sc := range conformance.Scenarios() {
		if !slices.Contains(renderScenarios, sc.Name) {
			opts = append(opts, conformance.Skip(sc.Name, "needs Apply (F2c)"))
		}
	}
	conformance.Run(t, func(*testing.T) conformance.Env { return renderOnlyEnv{} }, opts...)
}
