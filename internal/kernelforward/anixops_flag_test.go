package kernelforward

import (
	"errors"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/stretchr/testify/require"
)

// forward.anixops_experimental decides whether a route may use the AnixOps
// engine and link: the validation and the planner both refuse them while it
// is off.
func TestAnixOpsRoutesNeedTheExperimentalFlag(t *testing.T) {
	route := twoHop(0)
	route.Hops[1].Engine = forwardv1.Engine_ENGINE_ANIXOPS
	route.Hops[1].Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS}

	codes := func(enabled bool) []validate.Code {
		f := newFixture(t, openSQLite(t))
		f.service.AnixOps = func() bool { return enabled }
		_, err := f.service.CreateRoute(f.ctx, "anixops", route)
		var refused *RefusedError
		require.True(t, errors.As(err, &refused), "the route must be refused either way (the nodes do not advertise the engine): %v", err)
		var out []validate.Code
		for _, v := range refused.Violations {
			out = append(out, v.Code)
		}
		return out
	}
	require.Contains(t, codes(false), validate.CodeEngineNotEnabled)
	require.NotContains(t, codes(true), validate.CodeEngineNotEnabled)
}

func TestAnixOpsFlagDefaultsOff(t *testing.T) {
	require.False(t, (&Service{}).anixOpsEnabled())
	require.True(t, (&Service{AnixOps: func() bool { return true }}).anixOpsEnabled())
}
