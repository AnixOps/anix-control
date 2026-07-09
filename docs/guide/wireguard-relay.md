# WireGuard Dual-Node Relay Plan

Date: 2026-07-10

Status: Planned/P0. This document is planning only. It does not mean the panel,
subscription formatter, V2bX runtime, tests, or CI verification already exist.

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
   - Add WireGuard protocol fields for CIDR, MTU, DNS, tunnel type, entry node,
     exit node, and routing policy.
   - Add user peer allocation, keypair storage, preshared key storage, and
     validation.
   - Add admin UI for WireGuard protocol configuration and one-click WSS
     compatibility mode switching.

3. Subscription output.
   - Add WireGuard formatters for Shadowrocket, Loon, v2rayN, and other common
     clients where format behavior is verified.
   - Add compatibility tests for generated profiles.

4. V2bX runtime.
   - Apply domestic entry WireGuard termination, peer state, route policy, limits,
     online tracking, and peer traffic accounting.
   - Apply entry-to-exit GOST relay+QUIC by default and GOST relay+WSS when
     compatibility mode is enabled.
   - Apply overseas exit NAT and runtime health reporting.

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
