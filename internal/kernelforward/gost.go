package kernelforward

import (
	"fmt"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// onGost reports whether a hop of route still runs on the removed gost.
func onGost(route *forwardv1.Route) bool {
	for _, hop := range route.GetHops() {
		if hop.GetEngine() == forwardv1.Engine_ENGINE_GOST {
			return true
		}
	}
	return false
}

// gostRemoved refuses the ENGINE_GOST hops of route. gost was removed and
// the network model carries what it did, so Control neither stores a new
// gost hop nor plans a stored one (EnforcedGostRemoved).
func gostRemoved(route *forwardv1.Route) validate.Violations {
	var out validate.Violations
	for i, hop := range route.GetHops() {
		if hop.GetEngine() == forwardv1.Engine_ENGINE_GOST {
			out = append(out, validate.Violation{
				Field:   fmt.Sprintf("hops[%d].engine", i),
				Code:    validate.CodeEngineNotEnabled,
				Message: "ENGINE_GOST was removed: move the route to the network model (ENGINE_NETWORK)",
			})
		}
	}
	return out
}
