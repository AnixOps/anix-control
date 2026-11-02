// Package validate holds the forwarding route rules that Control runs
// before it stores a route and the planner before it plans one, so both
// refuse the same routes for the same reasons. The rules are those of the
// contract's comments (sdk/api/forward/v1) and the network model.
//
// Route and Proto answer Violations: every failure, each with the field
// path into the route in the contract's field names
// ("hops[1].ingress.security"), a stable Code and an English message.
// Violations.ToProto converts them for the contract's Violation (field,
// message, code). Validation is pure: it asks no DNS and
// reads no clock but Options.Now.
//
// Every route runs on the network model: the engines with a driver of
// their own (nftables, gost, the anixops relay) were removed, so a hop
// runs ENGINE_NETWORK (else CodeEngineNotEnabled) and every link is
// LINK_SECURITY_NETWORK (else CodeLinkUnsupported). The rules:
//
//   - id is a ULID or empty; owner is "admin" or "user:<id>"; name is text
//     of at most MaxNameRunes; the encoded route is at most MaxRouteBytes.
//   - listen: address an IP literal or empty, an explicit port 1..65535,
//     protocol set, entry_hostname a DNS name and required with several
//     entry nodes.
//   - hops: 1..MaxHops; hop 0 is ENTRY, the last of several is EXIT, the
//     others RELAY; 1..MaxNodesPerHop distinct Agent identity names per
//     hop, one per hop after the entry (several entry nodes are entry
//     HA); no hop port, dial_address, mux, server_name or path (the
//     network model's transport decides them); network_egress_id on the
//     exit only, network_transport_id and the transport mode, carriers,
//     multipath and FEC per the network model.
//   - targets: 1..MaxTargets, host an IP literal or DNS name (no port, no
//     numeric IPv4 forms), port 1..65535, weight at most MaxWeight, no
//     duplicate host and port; the target policy (CheckTargetAddress), with
//     ALLOW_PRIVATE only on an administrator's route and never loopback.
//   - policy: known strategies, no direct mode, health and breaker values
//     within bounds, health timeout shorter than the interval; PROXY
//     protocol needs TCP.
//   - limits: expires_at not negative, and in the future on create.
//   - labels: at most MaxLabels, keys and values within their syntax and
//     length.
//   - ports: the listen port is not in Options.ReservedPorts of an entry
//     node, nor outside its range in Options.Nodes.
//
// The codes the removed rules answered (inventory capabilities, port
// allocation, carriers) stay in violation.go: they are part of the
// contract.
//
// The caps and bounds are proposals in caps.go; the defaults are in
// sdk/forward/model.
package validate
