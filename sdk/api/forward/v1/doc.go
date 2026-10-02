// Package forwardv1 holds the DRAFT, UNRELEASED forwarding contract
// (anixops.forward.v1): routes as chains of hops with a per-hop engine
// (nftables, gost, the AnixOps protocol), the per-node desired state the
// planner renders, the reports nodes send, and the ForwardControl and
// ForwardNode services. It is designed in docs/architecture/forward-sdk.md
// and awaits the owner's review (H11). Nothing serves it yet, and it may
// change until the first change that does.
package forwardv1
