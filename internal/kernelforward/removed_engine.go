package kernelforward

import forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"

// onRemovedEngine reports whether a hop of a stored route still runs on a
// removed engine (gost, nftables, the anixops relay): every route runs on
// the network model now. Validation refuses such a hop on a write
// (sdk/forward/validate); a stored one is left out of the plan
// (EnforcedGostRemoved).
func onRemovedEngine(route *forwardv1.Route) bool {
	for _, hop := range route.GetHops() {
		if hop.GetEngine() != forwardv1.Engine_ENGINE_NETWORK {
			return true
		}
	}
	return false
}
