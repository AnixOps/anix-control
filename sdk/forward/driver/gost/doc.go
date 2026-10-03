// Package gost is the forward driver for ENGINE_GOST
// (docs/architecture/forward-sdk.md section 6.2): it runs the gost hops of
// a node's state in one gost v3 process, the pinned release the Agent
// ships (PinnedVersion, owner decision H20), under its own systemd unit
// anixops-gost.service, so restarting or upgrading the Agent keeps
// forwarding. gost is MIT-licensed and runs unmodified; the SDK does not
// import it.
//
// F4a implements Render, Capabilities, Probe and the process management
// of Apply and Remove; Observe reports the applied identity and rotation
// with counters at 0, and SetUpstreams checks its arguments and answers
// ErrUnsupported. F4b reads the counters from gost's metrics and changes
// upstreams through gost's web API.
//
// # Configuration
//
// Render writes gost's configuration as JSON (gost loads JSON and YAML
// alike; encoding/json writes it deterministically from typed values, does
// all quoting, and keeps the SDK free of a YAML dependency). Golden
// examples are in contracts/forward/v1/gost. Per hop, named from the
// route id (1 to 64 letters and digits, rejected otherwise, never
// escaped):
//
//   - services r<route>-h<hop>-<listener>: a RAW ingress listens with tcp
//     and udp listeners (both for TCP_UDP) whose tcp and udp handlers
//     forward the client's bytes; every other ingress is one carrier
//     listener (tls, mtls for TLS with mux, wss, mwss, quic, grpc; mtcp for
//     RAW with mux) whose relay handler carries TCP and UDP. An encrypted
//     listener is mutual TLS: the node's link certificate (Config.LinkCert,
//     LinkKey) and the CA its peers' client certificates must chain to
//     (LinkCA). UDP listeners keep a client's session for 60 s after its
//     last datagram;
//   - upstreams: RAW upstreams (targets, a RAW next hop) are the service's
//     forwarder nodes, dialled directly. Relayed upstreams (an encrypted or
//     multiplexed next hop) are nodes of chain r<route>-h<hop>, each with
//     gost's relay connector and the link's dialer, which verifies the
//     next node's certificate for its server name (Upstream.egress
//     server_name, else its node reference) against LinkCA and presents
//     the link certificate; the forwarder then holds one placeholder node,
//     because the next node's relay handler forwards to its own upstreams
//     whatever address it is sent. A hop that mixes both kinds is
//     ErrUnsupported;
//   - balancing: the selector of the forwarder or the chain hop.
//     ROUND_ROBIN is round, RANDOM rand, IP_HASH hash (of the client
//     address), FAILOVER fifo over the upstreams sorted by priority (ties
//     in state order), LEAST_CONN rand until the Agent re-weights it.
//     rand reads weights from node metadata; round and hash ignore
//     weights, so a hop with unequal weights lists an upstream once per
//     entry in smooth weighted round-robin order (spread, at most about
//     MaxEntries entries). The circuit breaker is the selector's fail
//     filter: maxFails failure_threshold (default 3), failTimeout open_ms
//     (default 30 s, H21), counted per gost node, so an upstream listed w
//     times is skipped only once each of its entries failed. Health checks
//     are the Agent's;
//   - admission r<route>-h<hop>: a whitelist of ingress_sources (required
//     on relay and exit hops, so a relay is never open). A paused hop keeps
//     its services and admits nobody: new connections are refused, while
//     established ones run until they close;
//   - limiter r<route>-h<hop>: bandwidth_bps as a traffic limit for the
//     whole hop in each direction, in bytes per second; climiter
//     r<route>-h<hop>: max_conns. gost has no byte quota: a hop with
//     quota_bytes is ErrUnsupported (an exact quota needs an NFTABLES
//     entry);
//   - metrics on a unix socket in Config.RuntimeDir, under a path named by
//     a hash of the rest of the configuration: it answers only once gost
//     loaded this configuration, which is how Apply knows a reload took.
//
// Targets must be IP literals (the Agent resolves names before Render,
// and re-checks the target policy on every answer); Render re-checks
// literal targets against the target policy. A top-level "anixops" member,
// which gost ignores, is the manifest Apply reads: each hop's services,
// listening sockets, strategy and rendered upstreams.
//
// # Link certificates and peer identities
//
// gost verifies a peer's certificate chain and, when dialling, the server
// name; it cannot match a SPIFFE URI. So the dialler pins the next node by
// its server name under LinkCA, and a listener admits any client
// certificate LinkCA signed, from the ingress_sources addresses only.
// ingress_peers is checked to be present on encrypted ingress, not matched
// per identity. The link certificates must carry the node's identity name
// as a DNS name with both serverAuth and clientAuth, from a CA that signs
// forward nodes' link certificates only; the Agent's Control certificate
// (client auth only, URI name only) does not qualify (forward-sdk.md
// section 16, H28).
//
// # Process and files
//
// The driver owns Config.Dir: gost.json, the configuration gost runs, and
// state.json, its own record (OwnerMark, node, generation, state_hash,
// digest and the applied hops). The state file is written before the
// first configuration and after every apply that succeeded. A
// configuration without the driver's state file, or a state file without
// OwnerMark, is foreign: Apply refuses it (ErrNotOwned) and Remove leaves
// it. A Supervisor runs gost: SystemdSupervisor manages UnitName with
// systemctl (the unit must carry OwnerMark in its Description; UnitFile
// renders it); ProcessSupervisor runs gost as a child process, which the
// tests do inside a network namespace. The Runner runs gost (Probe), ss
// (the listening sockets) and systemctl, nothing else. Probe checks that
// the link certificate files exist as the Agent sees them; that the gost
// user can read them (group anixops-gost) is the installer's to set up.
// Without the unit (ErrNoUnit) nothing can run gost, so Remove then
// deletes the driver's files only.
//
// # Apply
//
// Apply checks ownership and the generation, then compares the host with
// the artifact: the recorded digest, the configuration file's bytes, gost
// serving the configuration's metrics path and every manifest listener
// bound. All equal is a no-op that at most records a newer generation.
// Otherwise it reads the listening sockets with ss and refuses a listener
// whose port a socket holds that the running, applied configuration does
// not declare (ErrConflict, before anything changes), writes the
// configuration (temporary file and rename), reloads gost (SIGHUP) or
// starts it, and waits until it serves the new metrics path with every
// listener bound. A reload keeps the gost process, so established TCP
// connections and the counter epoch survive it. When gost does not serve
// the configuration in Config.ReadyTimeout (it refused it, or exited),
// Apply writes the previous configuration back and restarts gost on it (a
// reload that failed halfway may have closed services), or stops gost
// after a first apply, and fails. Applying an empty artifact stops gost
// and deletes both files.
//
// # Observe
//
// Observe answers the recorded identity, one Counters per applied hop and
// the rendered upstreams as the rotation. The counter epoch is the gost
// instance (the unit's InvocationID): a reload keeps it, a restart ends
// it. Counter values are 0 until F4b reads gost's metrics.
package gost
