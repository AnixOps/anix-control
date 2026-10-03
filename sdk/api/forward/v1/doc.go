// Package forwardv1 holds the forwarding contract (anixops.forward.v1):
// routes as chains of hops with a per-hop engine (nftables, gost, the
// AnixOps protocol), the per-node desired state the planner renders, the
// reports nodes send, and the ForwardControl and ForwardNode services. It is
// designed in docs/architecture/forward-sdk.md and approved (H11). It is
// binding since F3a, which serves it: it grows by additions only.
package forwardv1
