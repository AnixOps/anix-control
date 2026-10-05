// Package anixops is the forward driver for ENGINE_ANIXOPS
// (docs/architecture/anixops-protocol.md section 6.1, phase A3): it runs the
// anixops hops of a node's state in the relay process of
// sdk/forward/driver/anixops/relayd, the `anixops-relay` unit the Agent's
// package ships, under its own systemd unit anixops-relay.service (decision
// P1), so restarting or upgrading the Agent keeps forwarding. The transport
// (framing, TLS and QUIC carriers, identity pinning, carrier selection) is
// sdk/forward/relay; the driver owns what the library leaves to it: the
// carrier pool per upstream, the table of UDP associations, the strategies,
// the counters in epochs and the quota.
//
// The prototype is experimental and off by default: both Control and the
// Agent need forward.anixops_experimental, and the wire format (ALPN
// anixops/0) may change without notice until A6 freezes it.
//
// # Packages
//
//	anixops          this package: Render, Apply, Observe, SetUpstreams, Remove,
//	                 Probe, the Supervisor and the unit file; it links neither
//	                 the transport library nor QUIC
//	anixops/relayctl the relay's configuration document and the control API's
//	                 messages and client, shared by the driver and the relay
//	anixops/relayd   the relay process: hop runtime, control API server, Main
//	anixops/anixopstest  the relay in the test process, for tests
//
// # Configuration
//
// Render writes the relay's configuration as JSON (relayctl.Config;
// encoding/json writes it deterministically from typed values and does all
// quoting): the driver's ownership mark, where the link files are, and per
// hop (sorted by route and index; route ids are 1 to 64 letters and digits,
// rejected otherwise, never escaped) its listener, ingress, admitted sources
// and peers, upstreams in the state's order with weight, priority, egress,
// next node and the identity it must present, strategy, breaker, limits and
// pause. Golden examples are in contracts/forward/v1/anixops.
//
//   - A RAW ingress listens for TCP and UDP clients: the entry's, and any
//     hop's on a trusted network (the same listener code serves a RAW hop in
//     the middle of a route; whether Control may plan one is validation's
//     decision, section 9.3 P5, not the driver's). A RAW egress dials targets
//     directly.
//   - An AnixOps ingress listens for the carrier of the link: TCP for TLS_TCP
//     and PLAIN, UDP for QUIC, both at one port for AUTO. The traffic its
//     streams carry (TCP or UDP) is not the listener's business. It needs
//     ingress_sources and, for an encrypted carrier, ingress_peers. An AnixOps
//     egress dials the next node with a Selector per upstream (AUTO's QUIC
//     probe and TLS_TCP fallback), the server name the planner set and the
//     peer identity it pinned.
//   - A hop's upstreams are all targets or all next nodes; a mix is
//     ErrUnsupported. Upstream addresses must be IP literals (the Agent
//     resolves names before Render), and targets pass the target policy.
//   - Capabilities report RAW and ANIXOPS links, every strategy, IPv6, UDP,
//     bandwidth, the exact quota and max connections, the carriers the host
//     serves (PLAIN always; TLS_TCP and QUIC with the link files), the wire
//     versions the relay binary speaks, and proxy_protocol false: PROXY v2
//     toward targets is not implemented yet, so validation refuses it and a
//     hop that asks for it is ErrUnsupported.
//
// # Process and files
//
// The driver owns Config.Dir: relay.json, the configuration the relay runs
// (the unit loads it at start), state.json, its own record (OwnerMark, node,
// generation, state_hash, digest), and stateless-reset.key, the 32 bytes a
// QUIC listener answers the packets of connections it forgot with (the relay
// cannot write its directory, so the driver does). A configuration without the
// driver's state file, or a state file without OwnerMark, is foreign: Apply
// refuses it (ErrNotOwned) and Remove leaves it. The link files in Dir/tls are
// the Agent's. The relay's control socket is Config.RuntimeDir/control.sock.
// A Supervisor runs the relay: SystemdSupervisor manages UnitName with
// systemctl (the unit must carry OwnerMark in its Description; UnitFile
// renders it), ProcessSupervisor runs it as a child process (tests, network
// namespaces). The Runner runs anixops-relay -V (Probe) and systemctl, nothing
// else.
//
// # Apply
//
// Apply checks ownership and the generation, then compares the host with the
// artifact: the recorded digest, the configuration file's bytes and the relay
// running them (its status names the digest and every hop listens). All equal
// is a no-op that at most records a newer generation. Otherwise it makes sure
// the relay runs (the unit needs a configuration file to start, so a first
// apply writes an empty document), PUTs the artifact to the control socket,
// and then writes the file and the state.
//
// The relay does the changing, and does it the way the driver rules need
// (relayd's documentation has the details): it validates the whole document,
// binds every new listener before it changes anything (a port another process
// holds is ErrConflict and nothing changes), keeps the listener, carriers,
// established connections and counter epoch of every hop whose listener did
// not change, takes the hot fields (upstreams, weights, limits, quota, sources,
// peers, pause) in place, and puts every rendered upstream back in rotation.
// A hop whose listener changes, and a new hop, start a new epoch; a removed
// hop's listener closes with GOAWAY to its carriers (L1). A failed first apply
// stops the relay and removes what it wrote; a failed later one leaves the
// previous configuration running. An empty artifact stops the relay and
// deletes the files.
//
// # Observe, failover and credentials
//
// Observe answers the recorded identity, one Counters per applied hop from the
// relay (payload bytes up and down, UDP datagrams as packets, connections in
// flight and since the epoch began, the epoch), the rotation and the relay's
// own view of its upstreams: consecutive failures and an open breaker are
// reported as UpstreamHealth, so a persistent failure shows without the
// Agent's checks. While the relay does not run every hop reports 0 in the epoch
// "stopped". The final counters of an epoch that ended (a hop whose listener
// changed or that was removed) go to the WithRetiredCounters hook once.
//
// SetUpstreams puts the named upstreams of one hop in rotation, with weights,
// through the control socket: no apply, no new generation, no epoch ends and
// established connections keep their upstream. The rotation lives in the
// running relay, so it survives an Agent restart and a no-op Apply. The relay
// balances LEAST_CONN itself from the streams in flight per upstream (exact,
// unlike gost's carriers), so the Agent's re-weighting loop has no source for
// this engine and leaves such hops at their rendered weights.
//
// ReloadCredentials makes the relay read the link files again after a renewal:
// no connection ends and no epoch changes; a carrier whose peer is no longer
// trusted under the new bundle ends with GOAWAY credentials_changed.
//
// # Not here yet
//
// PROXY protocol v2, the Prometheus socket of section 7.3, the 7-day carrier
// age, and sending a connection's first bytes behind the OPEN (a new connection
// waits one round trip per hop for the next hop's answer, which keeps every
// refusal's code and lets a refusing next hop be failed over; A5 decides).
package anixops
