// Package planner checks forwarding routes and keeps the generations of
// every node's desired state (docs/architecture/forward-sdk.md section 5).
// It is pure and deterministic: it reads nothing but its arguments.
//
// Every route runs on the network model (ENGINE_NETWORK), which Control
// plans outside NodeForwardState: the engines that rendered hops on a node
// (nftables, gost, the anixops relay) were removed, and with them the
// planner's port and mark allocation and hop wiring. What is left:
//
//   - Plan validates every route through sdk/forward/validate with the
//     node inventory and answers one empty NodeForwardState per inventory
//     node, which Control fills with vantage probes. Any violation refuses
//     the whole plan, so Control keeps every node on its previous state.
//   - PlanRoute is the single-route check of ForwardControl.PlanRoute.
//   - Stamp sets each state's state_hash and generation from the previous
//     generations; StateHash and StateHashOf are the hash alone.
//
// Generations. state_hash is the lowercase hex SHA-256 of the
// deterministic protobuf encoding of a NodeForwardState holding only the
// node's hops and vantage probes. Stamp keeps a node's generation while
// its hash is unchanged and bumps it by one when it changes, so
// re-planning identical input bumps nothing.
package planner
