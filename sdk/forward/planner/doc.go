// Package planner turns forwarding routes into the desired state of every
// node (docs/architecture/forward-sdk.md section 5). It is pure and
// deterministic: it reads nothing but its arguments (no clock, no DNS, no
// randomness, no map order), and the same arguments give the same bytes.
//
// Entry points:
//
//   - Plan renders every route into one NodeForwardState per node of the
//     inventory, with the ports and marks of every hop (Allocations).
//   - PlanRoute is the single-route plan of ForwardControl.PlanRoute and of
//     the golden fixtures in contracts/forward/v1: only that route's hops,
//     only on the nodes it uses.
//   - Stamp sets each state's state_hash and generation from the previous
//     generations; StateHash is the hash alone.
//
// Validation first. Every route runs through sdk/forward/validate with the
// node inventory, and the planner adds the rules that need every route on
// a node (port_in_use, port_exhausted, no_port_range, mark_exhausted,
// no_address; the codes are listed with validate's). Any violation refuses
// the whole plan: nothing is planned, so Control keeps every node on its
// previous state. A misuse of the planner (Options.Cluster missing for an
// encrypted link) is an error, never a violation.
//
// Ports. The entry listens on listen.port, other hops on Hop.port; 0 asks
// the planner. The entry's nodes share one port, the lowest free on all of
// them; other hops decide per node. Allocation runs in three passes in
// route id, hop and node order: previous allocations that are still legal
// stick (in the range, not reserved, not taken, and for an explicit port
// still the one asked for); then explicit ports are claimed, and one
// already held on the node is refused with port_in_use; then the remaining
// hops get the lowest free port of the node's range that is not in
// Options.ReservedPorts or Options.Taken. A port is held per node across
// TCP and UDP. A sticky port that had to move is reported in Warnings.
// A route that is not planned any more holds nothing: Control keeps its
// allocations in Options.Taken for the grace period.
//
// Marks. Every hop on a node gets a connection mark in 1..MaxMark, unique
// on the node and sticky like ports. The driver shifts it into its mark
// mask.
//
// Wiring. Each hop's upstreams are the next hop's nodes in node_refs order
// (which is their failover priority), dialled at dial_address or the
// node's first address on the port allocated there, with the next hop's
// ingress as egress; an encrypted link presents the node's identity name
// as server name unless one is set, and pins the node's Agent identity
// (spiffe://anixops/<cluster>/agent/<node>). The last hop's upstreams are
// the targets in route order with their weights (0 is 1) and priorities.
// Non-entry hops admit only the previous hop's node addresses
// (ingress_sources) and, on an encrypted link, identities (ingress_peers).
// balance is policy.next_hop towards nodes and policy.target towards
// targets; health and circuit breaker carry their defaults (model) on
// every hop; limits are set on the entry hop only. DIRECT_MODE_PREFERRED
// puts the targets first among the entry's upstreams, with the next hop's
// nodes after the targets' highest priority, balanced with policy.target.
// A paused route keeps its hops, ports and marks, rendered with paused set.
// An nftables hop that accepts clients of an address family without an
// upstream of that family is reported in Warnings (one DNAT map per
// family).
//
// Generations. state_hash is the lowercase hex SHA-256 of the
// deterministic protobuf encoding of a NodeForwardState holding only the
// node's hops, which the planner sorts by route id and hop index. Stamp
// keeps a node's generation while its hash is unchanged and bumps it by
// one when it changes, so a change to one route bumps only the nodes whose
// state it changes, and re-planning identical input bumps nothing.
package planner
