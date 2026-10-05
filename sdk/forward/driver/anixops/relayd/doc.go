// Package relayd is the relay process of the anixops forward engine
// (docs/architecture/anixops-protocol.md section 6): the hop runtime that the
// `anixops-relay` unit runs and the driver (sdk/forward/driver/anixops)
// controls over a unix socket. It joins the transport library
// (sdk/forward/relay: framing, TLS and QUIC carriers, identity pinning,
// carrier selection) with what a node's hop needs around it: client listeners,
// the choice of an upstream, proxying with half-close, the table of UDP
// associations, counters in epochs, the quota and the limits.
//
// It is a library, so the same code runs in the unit (the Agent's
// `cmd/anixops-relay` is a few lines around Main), in tests inside a process
// on loopback, and in network namespaces.
//
// # Hops
//
// A hop is the share of one route that runs on this node (relayctl.Hop). Its
// listener terminates either RAW client traffic (TCP and UDP, the entry's and
// the trusted-network kind) or the AnixOps link (a carrier listener: TLS over
// TCP, QUIC, both on one port for AUTO, or plaintext on a trusted link). Its
// upstreams are targets (RAW egress, dialled directly) or the next hop's nodes
// (AnixOps egress: a Selector per upstream and a small pool of carriers on
// which every connection is one stream).
//
//   - Admission. A carrier listener checks the source address before the
//     handshake and the dialler's identity after it (ingress_sources and
//     ingress_peers); a RAW listener checks the source address of every
//     connection and datagram. A stream whose route id and hop index are not
//     the hop's own is answered route_mismatch, a paused hop answers paused,
//     a hop whose quota is used up quota_exceeded, one at max_conns
//     limit_exceeded.
//   - Upstreams. The strategies of the contract pick among the upstreams in
//     rotation; a dial that fails counts against the upstream's breaker
//     (failure_threshold in a row opens it for open_ms) and the next candidate
//     is tried, so a dead upstream costs one failed dial and not a failed
//     connection. A relay or exit hop answers the previous hop only after it
//     knows its own upstream: a stream the exit could not connect is refused
//     with upstream_unreachable, and an entry or relay whose next hop refuses
//     moves on to another upstream. A connection's first bytes wait for that
//     answer (one round trip per hop and new connection); sending them behind
//     the OPEN is an optimisation the benchmarks (A5) decide.
//   - Proxying. A TCP connection is two copies with half-close (CloseWrite
//     after EOF), payload counted where the hop reads from its client side:
//     up is client to target, down the way back. UDP is an association per
//     client address (entry and RAW hops) or per stream (AnixOps hops), idle
//     for 60 s at most, counted in datagrams and bytes.
//   - Limits. max_conns bounds connections and associations in flight; the
//     bandwidth is a token bucket per direction shared by the hop; the quota
//     is exact: the hop counts every payload byte it moves in the epoch, ends
//     its connections once up plus down reached quota_bytes (within one copy
//     buffer, 32 KiB) and admits again when the quota is raised above them.
//     Pausing admits nobody new and leaves what runs.
//
// # Applying a configuration
//
// Apply (PUT /v1/config) validates the whole document, binds every new
// listener first, and only then changes anything: a port another process
// holds is a conflict and nothing changes. A hop whose listener key did not
// change keeps its listener, carriers, connections and counter epoch and
// takes the rest in place (hot fields); its rotation goes back to every
// rendered upstream. A hop whose listener changes, and a new hop, get a new
// listener and a new epoch; a hop that goes away closes its listener (L1: the
// carriers it accepted get GOAWAY and drain) and its final counters wait in
// the retired list until an Observe drains them.
//
// # Epochs
//
// The epoch of a hop is a hash of the relay instance (a random id made at
// start) and the creation sequence number of the hop's listener, so a restart
// or a re-created listener starts a new one and nothing else does. Counters
// are not persisted; Control's ledger takes a new epoch in full.
//
// # Not here yet
//
// PROXY protocol v2 toward targets, the Prometheus socket (section 7.3), the
// 7-day carrier age, and sending a connection's first bytes behind the OPEN
// are deferred (the driver reports proxy_protocol false, so validation
// refuses PROXY on anixops exits).
package relayd
