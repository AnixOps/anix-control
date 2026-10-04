// Package gost is the forward driver for ENGINE_GOST
// (docs/architecture/forward-sdk.md section 6.2): it runs the gost hops of
// a node's state in one gost v3 process, the pinned release the Agent
// ships (PinnedVersion, owner decision H20), under its own systemd unit
// anixops-gost.service, so restarting or upgrading the Agent keeps
// forwarding. gost is MIT-licensed and runs unmodified; the SDK does not
// import it.
//
// F4a implements Render, Capabilities, Probe and the process management
// of Apply and Remove. F4b adds gost's web API: Apply changes upstreams,
// sources, pauses and limits without a reload, SetUpstreams fails over and
// re-weights in place, Observe reads every service's statistics, and the
// driver keeps a soft quota (EnforceQuotas) and answers each upstream's
// established connections for least-connections re-weighting
// (ActiveConns, sdk/forward/leastconn). F4c makes structural changes
// through the web API too, service by service, so adding, changing or
// removing a route re-creates no other route's services; a reload is the
// fallback.
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
//     RAW with mux) whose relay handler carries TCP and UDP, with nodelay
//     (the relay request and its answer cross the link when the
//     connection is dialled, so protocols whose server speaks first work;
//     a client's half-close arrives as a full close over such a link,
//     since gost cannot half-close a TLS or mux stream). An encrypted
//     listener is mutual TLS: the node's link certificate (Config.LinkCert,
//     LinkKey) and the CA its peers' client certificates must chain to
//     (LinkCA). UDP listeners keep a client's session for 60 s after its
//     last datagram. Every service keeps statistics (metadata enableStats):
//     its connections and the bytes it read from and wrote to its clients;
//   - hop r<route>-h<hop>: a top-level gost hop holding the upstreams.
//     RAW upstreams (targets, a RAW next hop) are dialled directly: the
//     service's forwarder names the hop. Relayed upstreams (an encrypted
//     or multiplexed next hop) are nodes with gost's relay connector
//     (nodelay) and
//     the link's dialer, which verifies the next node's certificate for its
//     server name (Upstream.egress server_name, else its node reference)
//     against LinkCA and presents the link certificate; the service's
//     handler then dials through chain r<route>-h<hop>, which names the
//     hop, and the forwarder holds one placeholder node, because the next
//     node's relay handler forwards to its own upstreams whatever address
//     it is sent. A hop that mixes both kinds is ErrUnsupported;
//   - balancing: the hop's selector. ROUND_ROBIN is round, RANDOM rand,
//     IP_HASH hash (of the client address), FAILOVER fifo over the
//     upstreams sorted by priority (ties in state order), LEAST_CONN rand
//     until the Agent re-weights it (sdk/forward/leastconn). rand reads
//     weights from node metadata; round and hash ignore weights, so a hop
//     with unequal weights lists an upstream once per entry in smooth
//     weighted round-robin order (spread, at most about MaxEntries
//     entries). The circuit breaker is the selector's fail filter:
//     maxFails failure_threshold (default 3), failTimeout open_ms (default
//     30 s, H21), counted per gost node, so an upstream listed w times is
//     skipped only once each of its entries failed. Health checks are the
//     Agent's;
//   - admission r<route>-h<hop>, on every hop: a whitelist of
//     ingress_sources (required on relay and exit hops, so a relay is
//     never open), an empty blacklist (everybody) on an entry without
//     sources. A paused hop keeps its services and its admission admits
//     nobody: new connections are refused, while established ones run
//     until they close (gost can close a listener, not the connections it
//     accepted);
//   - limiter r<route>-h<hop>: bandwidth_bps as a traffic limit for the
//     whole hop in each direction, in bytes per second; climiter
//     r<route>-h<hop>: max_conns. quota_bytes, which gost cannot enforce
//     itself, is the soft quota of EnforceQuotas (Config.SoftQuota;
//     without it a hop with quota_bytes is ErrUnsupported and an exact
//     quota needs an NFTABLES entry);
//   - the web API on a unix socket in Config.RuntimeDir (APISocket),
//     without authentication: the socket's permissions are its only key
//     (the unit's UMask 0007 in its RuntimeDirectory of mode 0750: the
//     gost user and its group, which the Agent is in);
//   - metrics on another unix socket there, under a path named by a hash
//     of the configuration's structure (services, chains, log, API): it
//     answers only once gost loaded this structure, which is how Apply
//     knows a start or a reload took. No API call moves it, so after
//     Apply changed services through the web API gost serves the path of
//     the structure it loaded last, which the state file records.
//
// Services, chains, the log and the API are the configuration's
// structure: a reload re-creates every service. Hops, admissions and
// limiters are hot objects: gost resolves them by name at every
// connection, so the driver replaces them through the web API and no
// service, listener, established connection, UDP session, mux carrier or
// counter is touched. Services and chains are created and deleted
// through the web API one by one (F4c); only the log, the API and the
// metrics address need a reload.
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
// (client auth only, URI name only) does not qualify, and gost never holds
// its key. Owner decision H28 (forward-sdk.md section 16): a dedicated
// forward link CA issues them, requested and renewed alongside the Agent
// certificate; it is built after F4a (Control, and the Agent in F3b).
//
// # Process and files
//
// The driver owns Config.Dir: gost.json, the configuration gost runs, and
// state.json, its own record (OwnerMark, node, generation, state_hash,
// digest, the applied hops, the starts and reloads it made gost do, the
// metrics path gost loaded last, and per hop the sequence number of the
// last apply that deleted or created one of its services).
// The state file is written before the
// first configuration and after every apply that succeeded. A
// configuration without the driver's state file, or a state file without
// OwnerMark, is foreign: Apply refuses it (ErrNotOwned) and Remove leaves
// it. A Supervisor runs gost: SystemdSupervisor manages UnitName with
// systemctl (the unit must carry OwnerMark in its Description; UnitFile
// renders it); ProcessSupervisor runs gost as a child process, which the
// tests do inside a network namespace. The Runner runs gost (Probe), ss
// (the listening sockets and, for ActiveConns, the established ones) and
// systemctl, nothing else; the web API and the metrics are unix sockets
// the driver dials itself. Probe checks that
// the link certificate files exist as the Agent sees them; that the gost
// user can read them (group anixops-gost) is the installer's to set up.
// Without the unit (ErrNoUnit) nothing can run gost, so Remove then
// deletes the driver's files only.
//
// # Apply
//
// Apply checks ownership and the generation, then compares the host with
// the artifact: the recorded digest, the configuration file's bytes, and
// gost running it (it serves the configuration's metrics path or the
// recorded one it loaded last, holds every manifest listener, and runs
// exactly the configuration's services and hops). All equal is a no-op
// that at most records a newer generation. Otherwise it reads the
// listening sockets with ss and refuses a listener whose port a socket
// holds that the running, applied configuration does not declare
// (ErrConflict, before anything changes). Then:
//
//   - when gost runs the recorded configuration, its web API answers and
//     the artifact keeps the globals (log, API, metrics address), Apply
//     writes the configuration (temporary file and rename, so a restart
//     loads it) and changes gost through the web API object by object
//     (sync.go): it deletes the services that were removed or changed,
//     creates or replaces the admissions, limiters and chains that were
//     added or changed and every hop (which also puts back every upstream
//     a SetUpstreams took out of rotation), creates the added and changed
//     services, and deletes the objects no longer rendered. A changed
//     service is deleted and created again (gost's PUT closes the old
//     service first and leaves it registered, closed, when the new one
//     fails). Every other service keeps its listener, established
//     connections, UDP sessions, mux carriers and statistics. The hops
//     whose services were deleted or created start a new counter epoch,
//     and the WithRetiredCounters hook gets their counters read just
//     before the first change;
//   - otherwise (gost stopped, its web API silent, a configuration file
//     that is not the recorded one, other globals) it writes the
//     configuration and reloads gost (SIGHUP), or starts it, or restarts
//     it when gost serves this structure already (a reload would not move
//     the metrics path, so whether it took could not be seen), and waits
//     until gost serves the new metrics path with every listener bound. A
//     reload keeps the gost process, so established TCP connections
//     survive it (tested), but it re-creates every service of the node:
//     UDP sessions and mux carriers may restart and every hop's counters
//     start a new epoch: the WithRetiredCounters hook gets every hop's
//     counters read just before, when the web API answers. gost's reload
//     is not atomic (a listener it cannot bind closes the old services
//     first).
//
// What connections of a deleted or re-created service move afterwards
// goes to the closed service's statistics, which nothing reads: not
// counted. When gost refuses a web API change (it checks a request before
// changing anything), Apply puts back, through the web API, the objects
// it had changed as the previous configuration has them, and the previous
// file; when that fails too, or a reload is not served within
// Config.ReadyTimeout, Apply writes the previous configuration back and
// restarts gost on it, or stops gost after a first apply. Either way it
// fails. Applying an empty artifact stops gost and deletes both files.
//
// # Observe
//
// Observe answers the recorded identity, one Counters per applied hop and
// the rotation. While gost runs it reads gost's running configuration
// through the web API (GET /config, bounded: one entry per service, never
// per client as gost's Prometheus metrics would be): a hop's counters are
// the sums of its services' statistics. up_bytes is what they read from
// their clients and down_bytes what they wrote back (payload; the
// carrier's bytes on an encrypted or multiplexed ingress), total_conns and
// active_conns the connections they accepted (UDP sessions and the
// streams inside a mux carrier are not connections); gost counts no
// packets, so packets are 0. The counter epoch names the statistics
// objects: a hash of the gost instance (the unit's InvocationID), the
// starts and reloads Apply made (recorded in the state file), the
// sequence number of the last apply that deleted or created one of the
// hop's services through the web API (the state file), and the creation
// time of each of the hop's services. A start, a reload, or deleting or
// creating one of the hop's services ends it, for that hop only; Apply's
// hot changes, structural changes of other hops, SetUpstreams and
// EnforceQuotas keep it. gost's creation times have a resolution of a
// second, which is why the driver numbers its own re-creations; nothing
// but the driver re-creates services. While
// gost does not run, every hop reports 0 in the epoch "stopped". gost's
// fail marking is not exposed, so Health is empty: the Agent's checks are
// the source of truth.
//
// # Failover, re-weighting and the soft quota
//
// SetUpstreams replaces the running hop through the web API with the
// selected upstreams (the nodes Render would give them, with their
// weights) and records the selection in the hop's metadata, where Observe
// reads it: the rotation lives in the running gost, so it survives an
// Agent restart and a no-op Apply, and a start, a reload or a changing
// Apply puts every rendered upstream back. The configuration file, the
// state file, the digest and the counter epochs stay. It needs gost
// running.
//
// ActiveConns answers the established TCP connections and connected UDP
// sockets per upstream address and port (ss, no privilege), the source of
// least-connections re-weighting: gost counts connections per service, not
// per upstream. It counts a mux or encrypted link's carriers, not its
// streams, and any process's sockets to the same address.
//
// EnforceQuotas (driver.QuotaEnforcer) closes the admission of a hop whose
// up and down bytes in the current counter epoch reached quota_bytes, as
// a pause does, and opens it again when the quota was raised above them.
// The Agent calls it after every Observe and Apply, so the quota holds
// within an observation interval; established connections run until they
// close; a new counter epoch starts the quota again, as nftables' named
// quota does when its object is re-created. Control's ledger stays the
// authority (it plans the route paused).
package gost
