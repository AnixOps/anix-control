// Package validate holds the forwarding route rules that Control (before it
// stores a route), the planner (before it plans one) and the Agent (for what
// it can see) all run, so every side refuses the same routes for the same
// reasons. The rules are those of docs/architecture/forward-sdk.md section
// 4.4 and the contract's comments (sdk/api/forward/v1).
//
// Route and Proto answer Violations: every failure, each with the field
// path into the route in the contract's field names
// ("hops[1].ingress.security"), a stable Code and an English message.
// Violations.ToProto converts them for the contract's Violation (field,
// message, code). Validation is pure: it asks no DNS and
// reads no clock but Options.Now.
//
// Rules that need only the route:
//
//   - id is a ULID or empty; owner is "admin" or "user:<id>"; name is text
//     of at most MaxNameRunes; the encoded route is at most MaxRouteBytes.
//   - listen: address an IP literal or empty, port 0..65535 (0 allocates),
//     protocol set, entry_hostname a DNS name and required with several
//     entry nodes.
//   - hops: 1..MaxHops; hop 0 is ENTRY, the last of several is EXIT, the
//     others RELAY; engine set, known, and not ENGINE_ANIXOPS until it is
//     enabled (v4.3); 1..MaxNodesPerHop distinct Agent identity names per
//     hop (one node may serve several hops); on non-entry hops, port
//     0..65535, dial_address with exactly one node, and an ingress whose
//     security is set and whose server_name and path fit it. Every engine
//     may serve every role.
//   - links: hop i's engine originates and hop i+1's engine terminates
//     hops[i+1].ingress (NFTABLES: RAW; GOST: RAW, TLS, WSS, QUIC, GRPC;
//     ANIXOPS: ANIXOPS); mux needs gost or anixops on both ends.
//   - targets: 1..MaxTargets, host an IP literal or DNS name (no port, no
//     numeric IPv4 forms), port 1..65535, weight at most MaxWeight, no
//     duplicate host and port; the target policy (CheckTargetAddress), with
//     ALLOW_PRIVATE only on an administrator's route and never loopback.
//   - policy: known strategies and direct mode, DIRECT_MODE_FORCED only on a
//     one-hop route, health and breaker values within bounds, health timeout
//     shorter than the interval.
//   - limits: expires_at not negative, and in the future on create. The
//     other limits are unsigned, so never negative, and 0 means none.
//   - labels: at most MaxLabels, keys and values within their syntax and
//     length.
//   - ports: not in Options.ReservedPorts of a node that would listen on
//     them.
//
// Rules that need the inventory (Options.Nodes), skipped without one:
// every node exists; it advertises the hop's engine as available; the
// dialling and listening nodes support the link's security; the balancing
// hops offer the policy's strategies; every hop forwards UDP for a UDP
// listener; IPv6 for IPv6 targets and listen address; the entry nodes
// support the limits that are set; explicit ports are inside each node's
// range.
//
// The caps and bounds are proposals in caps.go; the defaults are in
// sdk/forward/model.
package validate
