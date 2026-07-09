# WireGuard Dual-Node Relay Plan

Date: 2026-07-10

Status: Partial/P0. The first panel peer-custody/subscription-output slice,
first admin visual protocol form, and initial V2bX entry/exit runtime slices
exist. Hardened API validation, real dual-node routing evidence, speed limits,
end-to-end tests, migration evidence, and CI relay verification are still
incomplete.

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
exit node uses `relay.role=exit`, `relay.tun_port`, `relay.entry_tun_address`,
`relay.exit_tun_address`, and optional `relay.outbound_iface` to listen for the
entry tunnel and apply NAT. The default relay mode remains `relay+quic`; setting
`tunnel_type=wss` or `relay.wss_compat=true` selects `relay+wss` only as the
compatibility path.

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
- Report runtime health and tunnel status back through the normal V2bX panel
  communication path.

First-version traffic accounting uses the domestic entry WireGuard peer as the
source of truth. Exit-side counters can be added later as reconciliation or
fraud-detection evidence, but they are not the first billing source.

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
     routing table/priority, and exit NAT hints.
   - Still pending: hardened admin API validation, safer server key management,
     entry/exit node selection workflow, route policy, and migration behavior.

3. Subscription output.
   - Done for the first slice: native WireGuard `.conf` output and sing-box
     WireGuard outbound output with unit coverage.
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
   - Still pending: real domestic-entry and overseas-exit integration evidence,
     speed-limit enforcement, runtime health reporting, and GitHub Actions
     relay-path verification.

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
