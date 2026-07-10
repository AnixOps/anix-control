# WireGuard Dual-Node Relay Plan

Date: 2026-07-10

Status: Partial/P0. The panel peer-custody/subscription-output slice, admin
visual/keypair workflow, hardened protocol validation, production peer schema
hook, V2bX entry/exit runtime slices, and release-gated GitHub Actions QUIC/WSS
network-namespace route acceptance jobs exist. `v2.5.0-rc.6` passed both
acceptance paths. Real cross-region routing and real-client import evidence are
still incomplete.

The first runtime release is IPv4-only for the managed relay path. IPv6 peer
CIDRs, relay TUN CIDRs, and IPv6 default routes are rejected until the runtime
has matching `ip -6`, forwarding, and NAT handling.

## Decision

Use option B:

```text
WireGuard access -> domestic entry termination -> GOST relay+QUIC -> overseas exit NAT
```

The user connects to the domestic entry node with WireGuard. The domestic entry
node terminates WireGuard and owns peer state, address allocation, route policy,
speed limits, online state, and traffic accounting. The domestic entry then
relays traffic to an overseas exit node through GOST relay+QUIC by default. The
overseas exit node performs NAT and forwards traffic to the public internet.

Both entry and exit nodes are V2bX-managed nodes. The panel sends the desired
WireGuard, relay tunnel, and exit NAT configuration to V2bX, and V2bX applies the
runtime state on each node.

## Compatibility Mode

The default entry-to-exit tunnel is GOST relay+QUIC.

The panel UI must provide a one-click switch for WSS compatibility mode. When
enabled, the entry-to-exit tunnel changes to GOST relay+WSS for networks where
QUIC/UDP is blocked or heavily degraded.

WSS is a compatibility mode, not the default mode.

Do not use WireGuard-over-WSS as the primary design. WireGuard is UDP-oriented,
while WSS runs over TCP/TLS/WebSocket. Encapsulating WireGuard over WSS can
introduce TCP-over-TCP style head-of-line blocking, retransmission amplification,
latency spikes, and lower throughput. GOST relay+WSS stays available only as a
fallback transport between managed nodes.

## Tunnel Types

The first implementation should expose only these tunnel type enum values:

| Value | Meaning |
|-------|---------|
| `quic` | Default GOST relay+QUIC tunnel between domestic entry and overseas exit. |
| `wss` | GOST relay+WSS compatibility tunnel selected by the admin one-click switch. |

The runtime contract now carries explicit GOST TUN relay fields. The entry node
uses `relay.role=entry`, `relay.server`, `relay.server_port`,
`relay.tun_port`, and `relay.entry_tun_address` to dial the overseas exit. The
exit node uses `relay.role=exit`, `relay.server_port`, `relay.tun_port`,
`relay.entry_tun_address`, `relay.exit_tun_address`, and optional
`relay.outbound_iface` to listen for the entry tunnel and apply NAT. The default relay mode remains `relay+quic`; setting
`tunnel_type=wss` or `relay.wss_compat=true` selects `relay+wss` only as the
compatibility path.

## WSS Certificate Contract

WSS uses TLS between the managed entry and exit. The entry configuration must
set `relay.wss_secure=true`, a matching `relay.wss_server_name`, and optionally
`relay.wss_ca_file` for a private CA. The exit configuration must use the same
`relay.wss_path` and set `relay.wss_cert_file` plus `relay.wss_key_file`.
The panel validates these role-specific fields before saving; the exit-only
certificate paths are never placed in an entry configuration.

Reserve extension slots for future transports, but do not expose them in the
first implementation:

- `h3`
- `grpc`
- `h2`
- `kcp`
- `phts`

## Panel Ownership

The administrator manually enters the CIDR for each WireGuard protocol instance.
The panel automatically allocates user peer IP addresses from that CIDR.

Default values:

| Setting | Default | Override |
|---------|---------|----------|
| MTU | `1280` | Admin can override per WireGuard protocol. |
| DNS | `1.1.1.1`, `8.8.8.8` | Admin can override per WireGuard protocol. |

The panel owns and stores each user's WireGuard keypair and preshared key. This
is required so the panel can regenerate subscriptions, rotate credentials, and
reconcile V2bX runtime state. Key custody must be treated as sensitive secret
storage and covered by masking, audit, backup, and export rules before runtime
implementation is marked complete.

## Runtime Ownership

Domestic entry node responsibilities:

- Terminate user WireGuard sessions.
- Apply peer public keys, preshared keys, allowed IPs, routes, MTU, DNS, and CIDR
  allocation received from the panel.
- Enforce per-user route policy and speed limits for the WireGuard peer.
- Track online state by WireGuard peer.
- Count upload/download traffic by WireGuard peer.
- Relay accepted traffic to the selected overseas exit through GOST relay+QUIC by
  default, or GOST relay+WSS when compatibility mode is enabled.

Overseas exit node responsibilities:

- Receive the managed relay tunnel from the domestic entry.
- Apply exit-side routing and NAT.
- Never receive the domestic entry's WireGuard private key or user peer
  credentials or any runtime user list; the exit runtime only needs relay
  CIDR/TUN/NAT configuration.
- Report runtime health and tunnel status back through the normal V2bX panel
  communication path.

Runtime health is reported through `POST /api/v2/node/runtime-health` for REST
nodes or the existing gRPC node-log channel for gRPC nodes. The payload is
`{"healthy":true|false,"error":"..."}`. A GOST process exit marks the node
unhealthy, records the error and checked timestamp in the panel, and is retried
by V2bX using `GostRestartDelaySeconds`. The admin node list exposes the last
reported runtime state separately from ordinary node reachability.

First-version traffic accounting uses the domestic entry WireGuard peer as the
source of truth. Exit-side counters can be added later as reconciliation or
fraud-detection evidence, but they are not the first billing source.

Both managed Linux nodes require `iproute2`, `iptables`, and GOST; the entry
also requires WireGuard kernel support and `wireguard-tools`. V2bX enables IPv4
forwarding and manages only the relay-specific forwarding rules: WireGuard to
TUN and established return traffic on the entry, then TUN to egress and
established return traffic on the exit. `relay.exit_nat=false` omits only
MASQUERADE and requires upstream routing for the WireGuard CIDR.

## Subscription Output

The subscription layer must output WireGuard configuration for common clients
that support importing or rendering WireGuard profiles, including:

- Shadowrocket
- Loon
- v2rayN

The subscription output should include the user peer private key, preshared key,
assigned peer IP, DNS, MTU, endpoint, and allowed IPs according to each client's
accepted format. Client-specific formatter behavior must be tested before the
feature is marked implemented.

When the selected subscription contains only WireGuard profiles, V2Ray-link
user agents including Shadowrocket, Loon, and v2rayN are automatically served
the native `.conf` profile instead of an empty V2Ray link list. Mixed-protocol
subscriptions retain their requested format; operators should place WireGuard
in a dedicated subscription group or request `type=wireguard` for clients that
need the native profile.

The user subscription page exposes the same `WireGuard (.conf)` URL and saves
its preview with the `.conf` extension for direct native-client import.

## Implementation Phases

1. Documentation planning.
   - Record the chosen option B architecture and status gaps.
   - Keep `TODO.md`, `docs/features.md`, `ROADMAP.md`, `CHANGELOG.md`, and
     relevant audit/test documents synchronized.

2. Panel model and API.
   - Done for the first slice: `wireguard` protocol template, `v2_wireguard_peer`,
     automatic IPv4 peer allocation, user keypair storage, preshared key storage,
     default MTU/DNS/allowed IPs, node-config relay defaults, and UniProxy/gRPC
     runtime user fields for peer IP, public key, and preshared key.
   - Done for the first admin UI slice: the node protocol visual form can produce
     WireGuard CIDR, server key material, MTU, DNS, entry/exit GOST relay role,
     QUIC/WSS tunnel selection, one-click WSS compatibility mode, TUN addresses,
     routing table/priority, exit NAT hints, and role-specific WSS SNI/CA or
     certificate/private-key paths.
   - Done: API validation, keypair generation, relay default normalization
     (`relay.backend` defaults to `gost`),
     entry/exit key separation, exit peer/subscription exclusion, peer
     allocation migration, secure WSS certificate contract, and production
     peer-schema initialization.
   - Still pending: a guided entry/exit node selection workflow and real
     operator migration/rollback evidence.

3. Subscription output.
   - Done for the first slice: native WireGuard `.conf` output, automatic
     `.conf` fallback for link-only user agents with WireGuard-only groups, and
     sing-box 1.13 WireGuard endpoint output with unit coverage.
   - Still pending: verified Shadowrocket, Loon, and v2rayN client-specific
     import behavior.

4. V2bX runtime.
   - Initial V2bX entry runtime support can consume panel WireGuard config and
     peer fields, apply the Linux WireGuard interface with `ip`/`wg`, and report
     peer traffic deltas from `wg show <iface> transfer`.
   - V2bX v2.3.3 adds initial peer online-state reporting from recent
     `wg show <iface> dump` handshakes through the existing panel `/alive` path.
   - Current V2bX runtime slice adds GOST TUN relay command planning, entry
     WireGuard-CIDR policy routing, `relay+quic`/`relay+wss` selection, and exit
     iptables NAT command application.
   - Done: per-peer/node `tc` shaping, dynamic-limit convergence, startup
     retry for GOST-created TUN devices, relay/limit contract verification, and
     RC/tag-gated GitHub Actions QUIC/WSS network-namespace route acceptance jobs.
   - Done: runtime health reporting and GOST process supervision through the
     normal REST/gRPC node communication paths.
   - Still pending: real domestic-entry and overseas-exit integration evidence,
     production restart-recovery evidence, and real WSS compatibility evidence.

5. Integration testing.
   - Cover panel API validation, CIDR exhaustion, duplicate peer allocation,
     key rotation, subscription rendering, entry peer counters, online state,
     rate limits, QUIC tunnel behavior, WSS compatibility behavior, and exit NAT.

6. GitHub Actions verification.
   - Add CI jobs or fixtures that can prove the WireGuard relay path without
     requiring local release builds.
   - Release artifacts must continue to be built only by GitHub Actions.

## Non-Goals For The First Version

- Do not expose `h3`, `grpc`, `h2`, `kcp`, or `phts` tunnel types in the UI/API.
- Do not make WireGuard-over-WSS the primary transport.
- Do not mark exit-side traffic reconciliation as billing source-of-truth.
- Do not claim production readiness without V2bX runtime evidence and GitHub
  Actions verification.
