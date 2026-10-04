# AnixOps relay protocol: secure transport between nodes

Status: DRAFT FOR REVIEW (H22). Scope set by the owner on 2026-10-04: this
document covers only the secure transport between AnixOps nodes. Section 8
is reserved for the owner. Nothing here is implemented; the contract slots
it builds on (`ENGINE_ANIXOPS`, `LINK_SECURITY_ANIXOPS`) exist in
`sdk/api/forward/v1` and are refused by `sdk/forward/validate` until
`Options.EnableAnixOps` is set (`docs/architecture/forward-sdk.md`
section 6.4).

> 中文摘要：AnixOps 中继协议只用于节点之间的转发链路（入口 → 中转 → 出口），由 Control 下发配置，
> 客户端和目标永远不直接使用它。本文只写安全传输部分，伪装相关内容留给 owner（第 8 节）。
> - **安全**：TLS 1.3 双向认证，用 H28 的转发链路证书；拨号方同时校验 DNS 名（`server_name`）和
>   SPIFFE URI（`peer_identity`），监听方校验对端 URI 属于 `ingress_peers`，这是 gost 做不到的逐身份绑定。
>   只用 Go 标准库密码学，不自创算法；不开 TLS 0-RTT 早期数据（防重放），会话恢复默认关闭。
> - **帧与多路复用**：自有的小型帧格式（OPEN/DATA/WINDOW/RESET/PING/GOAWAY/SETTINGS），
>   流级与载体级流控、10 秒心跳 30 秒超时、支持半关闭（FIN）、流数上限与背压；
>   目的地址只由本地状态决定，对端无法指定，天然不是开放代理。
> - **载体**：TLS over TCP（默认）、QUIC（原生流 + DATAGRAM 承载 UDP）、明文（仅可信专线，类似 nftables RAW）；
>   `AUTO` 优先 QUIC、失败回退 TLS，永不回退到明文。可选按路由向目标发送 PROXY protocol v2。
> - **吸取 gost 教训**：载体不得比监听器或进程活得更久；关闭监听器即关闭它接受的全部载体，
>   QUIC 关闭发 CONNECTION_CLOSE、重启后用无状态重置让对端立即重连；证书热更新不重启、不换计数纪元。
> - **集成**：`anixops` 引擎驱动（Render/Apply/Observe/SetUpstreams/Remove），推荐独立的
>   `anixops-relay.service`（仅 `CAP_NET_BIND_SERVICE`，不持有 Agent 密钥），经 unix 套接字热更新。
>   契约只做加法（载体枚举、PROXY v2、能力字段）。
> - **计划**：v4.2 实验原型默认关闭（Control 与 Agent 两侧开关 `forward.anixops_experimental`），
>   v4.3 正式版。文末 P1–P10 为待定问题，各附推荐答案。

## Contents

1. [Goals, non-goals and deployment](#1-goals-non-goals-and-deployment)
2. [Security model](#2-security-model)
3. [Authentication and cryptography](#3-authentication-and-cryptography)
4. [Framing and multiplexing](#4-framing-and-multiplexing)
5. [Carriers](#5-carriers)
6. [Integration](#6-integration)
7. [Performance, observability and operations](#7-performance-observability-and-operations)
8. [Reserved: camouflage (owner to author)](#8-reserved-camouflage-owner-to-author)
9. [Plan, risks and open questions](#9-plan-risks-and-open-questions)

## 1. Goals, non-goals and deployment

### 1.1 Goals

- **T1. Authenticated, private links.** Every byte between two AnixOps
  nodes on an encrypted carrier is confidential and integrity-protected, and
  each end knows exactly which node identity it talks to.
- **T2. Identity pinning.** A dialler accepts only the node the state names
  (`Upstream.peer_identity`) and a listener accepts only the nodes the state
  lists (`NodeHop.ingress_peers`). The gost driver can enforce neither per
  identity (`forward-sdk.md` section 6.2, "TLS and peer identities").
- **T3. Cheap connections.** A client connection becomes a stream on an
  established carrier: no handshake and no extra round trip before its first
  bytes leave the dialler.
- **T4. TCP and UDP.** UDP rides QUIC datagrams natively and falls back to
  UDP over a stream; TCP keeps half-close end to end.
- **T5. Owned lifecycle.** Carriers never outlive the listener that
  accepted them or the process; certificate renewal and structural changes
  of one hop never end another hop's connections (the gost lessons of
  `forward-sdk.md` section 6.2).
- **T6. Exact accounting.** Payload counters per hop and direction, exact
  byte quota, bandwidth and connection limits, following every rule of the
  driver interface (`forward-sdk.md` section 6.0).
- **T7. Control only.** Nodes take routes from `NodeForwardState` alone, as
  every other engine does. There is no node-local configuration and no CLI.

### 1.2 Non-goals

- A client-facing protocol. Clients connect to the entry with plain TCP or
  UDP (`Route.listen`); targets receive plain TCP or UDP from the exit.
- New cryptography. No custom cipher, handshake, key derivation or record
  layer: TLS 1.3 from Go's `crypto/tls` does all of it (section 3).
- End-to-end encryption across hops. Each link is protected separately; a
  relay sees the payload it relays, as with gost (section 2.3).
- Replacing nftables on trusted private lines where nothing but DNAT is
  needed. The plaintext carrier (section 5.3) is for trusted links that need
  multiplexing, per-hop accounting or UDP aggregation.
- Interoperability with gost or any third-party relay protocol.
- Camouflage: section 8 is reserved for the owner.

### 1.3 Deployment model

The protocol runs only between nodes of one AnixOps deployment, on the
links of a route that the planner wires (`forward-sdk.md` sections 4.2 and
5.3):

```
client --TCP/UDP--> entry --anixops--> relay --anixops--> exit --TCP/UDP--> target
                    (ENGINE_ANIXOPS)    (ENGINE_ANIXOPS)   (ENGINE_ANIXOPS)
```

- A hop whose next hop's `ingress.security` is `LINK_SECURITY_ANIXOPS`
  originates the protocol; that next hop terminates it. Both must run
  `ENGINE_ANIXOPS` (`validate.CanCarry`). Mixing engines across a link
  (an nftables entry handing raw traffic to an anixops relay) needs the
  anixops engine to also terminate `LINK_SECURITY_RAW`; that is P5.
- Control configures everything: which nodes listen, on which port, which
  identities may dial them, which identity each upstream must present.
  Credentials are the nodes' forward link certificates (H28); the state
  carries identities, never keys.
- Both editions (H23 recommendation, still open): the engine is core
  forwarding, like gost.

## 2. Security model

### 2.1 Assets and parties

| Asset | Protected by |
|---|---|
| Payload between nodes | TLS 1.3 record protection on encrypted carriers |
| Which node is on the other end | mutual certificate authentication plus identity pinning (section 3.2) |
| Where a relayed stream goes | the listener's own `NodeHop`: the peer cannot choose a destination (section 4.3) |
| Metering and limits | counted and enforced by the relay process on the hop that carries the limits |
| Link credentials | per-node link key generated by the Agent, readable only by the relay's user (H28) |

Parties: Control (the authority: it issues link certificates and decides
who may talk to whom), the Agent (holds the Control credential, writes the
relay's configuration and link files), the relay process (holds only the
link key), peer nodes, and everyone else on the network.

### 2.2 Adversaries and what holds

| Adversary | Can | Cannot |
|---|---|---|
| Passive on-path observer | see carrier endpoints, timing and sizes | read or link payload to streams on encrypted carriers |
| Active on-path attacker | drop, delay, reset carriers (availability) | inject, modify, reorder or replay records undetected; impersonate a node without its link key |
| Any internet host reaching a relay or exit port | open a TCP or QUIC connection, consume a handshake slot | complete a handshake without a certificate from the link CA whose identity the listener allows; reach any target |
| A node of the deployment that is not in `ingress_peers` | complete chain validation (its certificate is genuine) | pass the listener's identity check (section 3.2) |

### 2.3 Compromise scenarios

- **A compromised node** holds its own link key and the payload of every
  route it carries. It can impersonate only itself, reach only listeners
  that list its identity, and send traffic only to the upstreams its state
  names (section 4.3). It can lie about its counters; Control's ledger can
  compare an entry's counters with its exit's (section 7.3). Containment:
  the administrator disables or retires the node, `agentpki.RevokeNode`
  revokes its link certificates, and the next plan removes its identity
  from its peers' `ingress_peers` and upstreams. Unlike gost, the relay
  enforces that change at once: new handshakes from the identity fail, and
  carriers it already holds are closed when the state that drops it is
  applied (section 3.5). The revoked certificate stays cryptographically
  valid until it expires (at most 7 days), but no peer accepts the identity
  any more.
- **A stolen link key** without the node: the same as above for the
  certificate's remaining lifetime, from wherever the thief is, except that
  listeners also check `ingress_sources`, so the thief must come from the
  node's addresses. Replacing the node's credentials revokes it; the next
  renewal uses a new key anyway (keys are never reused).
- **A compromised link CA key**: an attacker can mint certificates for any
  identity. Response: rotate the link CA and drop the compromised CA from
  the trust bundle at once instead of after the normal overlap (P8), which
  forces every node to renew; until then pinning still limits the attacker
  to identities in some `ingress_peers`, and source admission to those
  nodes' addresses.
- **A compromised Control** controls forwarding entirely: it issues link
  certificates and writes the state. This protocol does not defend against
  it; Control's own controls apply (the link CA key sealed with
  `module_runtime.ca_kek`, audit records for every issued certificate,
  administrator authorization of every route change). Nodes accept state
  only over the authenticated Agent Control stream.
- **A compromised Agent** on a node equals a compromised node.
- **A compromised relay process** (a parser bug exploited by a peer) is
  confined by its unit: no `CAP_NET_ADMIN`, no Agent key, no write access
  outside its runtime directory (section 6.2).

### 2.4 Replay and downgrade

- Within a carrier, TLS 1.3 (and QUIC's packet protection) authenticates
  every record with a sequence number: a replayed, reordered or truncated
  record ends the carrier.
- Across carriers, nothing can be replayed because TLS 1.3 early data is
  never accepted (section 3.4). Stream opening happens inside an
  established, authenticated carrier.
- TLS 1.3 only (`MinVersion` and `MaxVersion` are TLS 1.3) and the ALPN
  protocol is required, so there is nothing to downgrade to. Protocol
  versions are negotiated by ALPN inside the authenticated handshake
  (section 6.7).
- The plaintext carrier (section 5.3) has none of these properties; it is
  allowed only where the link itself is trusted.

### 2.5 Denial of service

- A listener reads the peer address before the handshake and closes a
  connection whose source is not in `ingress_sources` without spending a
  signature (TCP accept, QUIC Initial).
- Bounded handshakes: at most 64 in flight per listener (configurable), a
  10 s handshake deadline, and QUIC address validation (Retry) when the
  handshake queue is over half full.
- Per-carrier limits on concurrent streams, frame sizes and buffered bytes
  (section 4.5); a peer that exceeds them gets `GOAWAY` and the carrier is
  closed.

## 3. Authentication and cryptography

### 3.1 Credentials

Each node uses its forward link certificate exactly as H28 defines it
(`sdk/api/agent/v1/PROTOCOL.md`, "Forward link certificates";
`internal/agentpki/link.go`):

- one DNS SAN and the CN: the node's identity name (`forward-41`), the
  default `server_name`;
- one URI SAN: its SPIFFE ID, `spiffe://anixops/<cluster>/agent/forward-41`,
  the value of `peer_identity` and `ingress_peers`;
- `serverAuth` and `clientAuth`, 7 days, renewed at two thirds with a new
  key;
- issued by the forward link CA, a separate ECDSA P-256 root
  name-constrained to `spiffe://anixops` URIs, which signs nothing else;
- the trust bundle (current, next and retired CAs) in `link-ca.crt`.

The relay reads the same three files the gost driver uses, from its own
directory (section 6.2). It never sees the Agent's certificate or key.

### 3.2 Handshake and identity pinning

TLS 1.3 with mutual authentication on every encrypted carrier, ALPN
`anixops/1` (`anixops/0` for the v4.2 prototype, section 9.1).

The dialler (previous hop) for an upstream with `node_ref` set:

1. presents its link certificate;
2. verifies the listener's chain to the link trust bundle with
   `ExtKeyUsageServerAuth`;
3. requires the DNS SAN to equal the link's `server_name` (empty means the
   upstream's `node_ref`);
4. requires exactly one URI SAN, equal to `Upstream.peer_identity`.

The listener (next hop):

1. requires a client certificate and verifies its chain to the link trust
   bundle with `ExtKeyUsageClientAuth`;
2. requires exactly one URI SAN and that it is in the listener's
   `NodeHop.ingress_peers`.

Chain and DNS-name verification are `crypto/tls`'s own: the dialler's
`tls.Config` has the link trust bundle as `RootCAs` and the expected name
as `ServerName`; the listener's has it as `ClientCAs` with
`RequireAndVerifyClientCert`. `VerifyConnection` adds only the identity
checks (dialler step 4, listener step 2); Go calls it on full and resumed
handshakes alike (unlike `VerifyPeerCertificate`). Verification is never
disabled. The trust pool and the expected identities change without
re-creating anything: the listener builds its configuration per handshake
with `GetConfigForClient` from the current bundle and `ingress_peers`, and
the dialler builds a `tls.Config` per upstream from the current bundle and
`peer_identity` for each new carrier. A failure ends the handshake with a
`bad_certificate` alert and is counted by reason (section 7.3).

`server_name` on an ANIXOPS link must be empty or the next node's identity
name: any other value is refused by validation (code
`server_name_unsupported`) until a later design gives it a meaning, since
the link certificate carries only that name.

### 3.3 Primitives

- Go's `crypto/tls`, `crypto/x509`, `crypto/ecdsa` and `crypto/rand` only,
  with the module's toolchain (Go 1.25, toolchain 1.26). QUIC uses the same
  `crypto/tls` through its QUIC API; the QUIC library adds no cryptography
  of its own.
- Cipher suites and key exchange: Go's TLS 1.3 defaults (AES-128-GCM,
  AES-256-GCM, ChaCha20-Poly1305; X25519 and the hybrid post-quantum
  X25519MLKEM768 that recent Go releases enable by default). The relay does
  not narrow them, so a toolchain upgrade brings Go's current defaults.
- No `crypto/tls` settings that weaken it: no `InsecureSkipVerify`, no TLS
  1.2, no renegotiation, no custom `Rand` or `Time` outside tests.
- Session tickets are encrypted by `crypto/tls`'s own rotating keys.

### 3.4 Session resumption and 0-RTT

- **TLS early data (0-RTT): never.** Early data is replayable by design, and
  a relay's first bytes (a stream open followed by client payload) are not
  idempotent. Neither the TCP nor the QUIC carrier accepts or sends it.
- **Stream-level zero round trip: always.** The latency 0-RTT would save is
  already saved by multiplexing: a new client connection is an `OPEN` frame
  on a warm carrier, followed immediately by the client's first bytes
  (section 4.3). This is inside an authenticated carrier, so it is
  replay-safe.
- **Session resumption (PSK): off by default** (P3). Carriers are few and
  long-lived, so the saved signature verification is negligible, while
  resumption keeps authentication state in tickets that outlive a state
  change. When enabled, the `VerifyConnection` checks run on resumption
  too, so a removed peer cannot resume; tickets are dropped when the trust
  bundle or the node's link certificate changes.

### 3.5 Key rotation and revocation

- **Renewal.** The Agent renews the link certificate (new key) and writes
  the files as for gost. The relay reloads them without re-creating any
  listener or carrier: `GetCertificate` and `GetClientCertificate` return
  the current certificate, and the trust pool is swapped atomically.
  Counter epochs do not change (unlike gost, `forward-sdk.md` section 6.2,
  "Certificate reload").
- **Carriers on old keys.** After a renewal the dialler opens new carriers
  for new streams and sends `GOAWAY` on the old ones, which close when their
  last stream ends. A carrier never lives longer than the link certificate
  lifetime (7 days); at that age it gets `GOAWAY` regardless.
- **Trust bundle change.** A carrier whose peer chain no longer verifies
  under the new bundle (its CA was dropped) is closed at once.
- **Peer removal.** Applying a state that removes an identity from a
  listener's `ingress_peers`, or changes an upstream's `peer_identity`,
  closes every carrier authenticated as the removed identity (`GOAWAY` with
  `peer_not_allowed`, then close). This is the revocation path; peers do not
  fetch CRLs (P9).

## 4. Framing and multiplexing

### 4.1 Carriers and streams

A **carrier** is one authenticated connection between two nodes for one
hop of one route: a TLS-over-TCP connection, a QUIC connection or a
plaintext TCP connection. A **stream** is one client connection (TCP) or
one client UDP association, inside a carrier. Only the dialler opens
streams: traffic always flows from the previous hop to the next, so the
listener never needs to.

On QUIC, streams are native QUIC bidirectional streams and UDP uses QUIC
datagrams (RFC 9221); the frames below that QUIC already provides (data,
flow control, reset, ping, close) are not used. On TCP carriers the relay
uses its own framing, specified here. Existing libraries were considered:
`hashicorp/yamux` is MPL-2.0, outside `forward-sdk.md` section 1's
MIT/Apache/BSD rule, and none examined offered half-close, in-band open
results and the bounds below together; a fuller survey is part of P6.

### 4.2 Frame format (TCP carriers)

Every frame has an 8-byte header, big-endian:

```
| length (16) | type (8) | flags (8) | stream id (32) | payload (length bytes) |
```

| Type | Name | Stream | Payload |
|---|---|---|---|
| 0x0 | `SETTINGS` | 0 | key/value pairs (u16 key, u32 value); first frame each side sends |
| 0x1 | `OPEN` | new | stream kind and open parameters (section 4.3) |
| 0x2 | `RESULT` | existing | u16 result code; sent once by the listener per `OPEN` |
| 0x3 | `DATA` | existing | payload bytes; flag `FIN` (0x1) ends the sender's direction |
| 0x4 | `WINDOW` | 0 or existing | u32 credit increment for the carrier (0) or the stream |
| 0x5 | `RESET` | existing | u16 reason; aborts both directions |
| 0x6 | `PING` | 0 | 8 opaque bytes; flag `ACK` (0x2) on the answer |
| 0x7 | `GOAWAY` | 0 | last accepted stream id, u16 reason: no new streams on this carrier |
| 0x8 | `DATAGRAM` | existing UDP | one UDP datagram (UDP over stream, section 4.7) |

Rules: unknown frame types on stream 0 are ignored, on a stream they reset
it (`unknown_frame`); a frame larger than the receiver's
`SETTINGS_MAX_FRAME` (default 16 KiB, at most 65535) ends the carrier;
stream ids are odd, opened by the dialler in increasing order and never
reused; at 2^31 the dialler moves to a new carrier.

### 4.3 Opening a stream

When a client connection arrives at the entry (or a stream arrives at a
relay), the hop picks an upstream (balancing and circuit breaker as in
`forward-sdk.md` section 7) and sends `OPEN` at once, without waiting for
the client's first bytes: protocols whose server speaks first (SSH, SMTP,
databases) then work, the lesson of gost's `nodelay` (F4c). `OPEN` carries:

- the stream kind: `TCP` or `UDP`;
- the route id and hop index the dialler believes it is feeding; the
  listener resets a stream whose pair is not its own (`route_mismatch`),
  which catches a stale or crossed state instead of delivering traffic to
  the wrong route;
- the original client address and port, as the entry saw them (for
  `IP_HASH` at later hops, PROXY protocol v2 at the exit, and diagnosis).

`OPEN` carries **no destination**. The listener sends the stream to its own
`NodeHop.upstreams`; a peer cannot make a relay or exit dial anything its
state does not name, so no listener is an open relay by construction.

The dialler sends the client's first bytes right after `OPEN` and keeps up
to the initial stream window of them until `RESULT` arrives. When `RESULT`
is a failure (`upstream_unreachable`, `admission_denied`, `paused`,
`quota_exceeded`, `limit_exceeded`, `route_mismatch`) and nothing has yet
come back from the stream, the dialler counts a failed dial against that
upstream (the circuit breaker's passive failure) and may retry the next
upstream with the buffered bytes; otherwise it resets the client
connection.

### 4.4 Flow control and backpressure

- Credit-based, per stream and per carrier, as in HTTP/2: the sender may
  have at most the granted credit in flight; the receiver grants more with
  `WINDOW` as it delivers bytes to the socket on its side.
- Initial windows: 256 KiB per stream, 1 MiB per carrier, grown up to 16 MiB
  and 64 MiB when the measured bandwidth-delay product needs it.
- Backpressure is end to end: a receiver stops granting credit while its
  write to the next socket (target, client or the next carrier's stream) is
  blocked, and a sender with no credit stops reading its source socket. A
  slow target therefore slows only its own stream; other streams on the
  carrier keep their credit.
- Head-of-line blocking at the TCP level is inherent to TCP carriers (one
  lost segment stalls every stream on the carrier). The dialler spreads
  streams over a small pool of carriers per upstream (section 4.5); QUIC
  avoids it.

### 4.5 Limits

| Limit | Default | Notes |
|---|---|---|
| Concurrent streams per carrier | 1024 | `SETTINGS_MAX_STREAMS`; excess `OPEN` is reset (`refused_stream`) and retried on another carrier |
| Carriers per upstream | 4 | opened on demand, one per 256 active streams, at least 1 kept warm while the hop has traffic |
| Buffered bytes per carrier | the carrier window | a peer that sends beyond its credit ends the carrier (`flow_control_error`) |
| Frame size | 16 KiB | `SETTINGS_MAX_FRAME`, at most 65535 |
| Open streams per node | 65536 | configurable resource cap across all hops; over it, new client connections are refused |
| Pending handshakes per listener | 64 | section 2.5 |

`max_conns` (the route's limit) is enforced on the hop that carries the
limits, the entry, by counting client connections there.

### 4.6 Liveness: carriers must not outlive their owner

The gost driver found that gost 3.2.6 ties accepted mux and QUIC carriers to
its process, not to the service: a removed listener left carriers that
answered keepalives while their new streams were never accepted, so new
connections hung, and a QUIC listener kept its port
(`forward-sdk.md` section 6.2, "Mux and QUIC carriers"). The relay makes
the opposite rules invariants, each with a test (section 6.6):

- **L1. A carrier belongs to its listener.** Closing or re-creating a
  listener sends `GOAWAY` (`listener_closed`) on every carrier it accepted
  and closes them after their streams end or a 5 s drain, whichever is
  first; a removed hop's streams end as nftables' do. The QUIC listener's
  UDP socket closes with its connections.
- **L2. Every stream is answered.** A stream that arrives for a hop the
  process does not run, or that the hop refuses, gets `RESULT` with the
  reason at once. No stream is ever left unaccepted.
- **L3. Process exit closes everything.** On a clean stop the relay sends
  `GOAWAY` on TCP carriers and `CONNECTION_CLOSE` on QUIC connections. On a
  crash the kernel closes TCP sockets (peers see a reset or FIN within one
  RTT); for QUIC, the relay keeps a stateless reset key in its state
  directory, so after a restart it answers packets of its previous
  connections with a stateless reset (RFC 9000 section 10.3) and peers
  redial within one round trip instead of waiting for the idle timeout.
- **L4. Keepalives.** Each side sends `PING` after 10 s without receiving a
  frame; a carrier with nothing received for 30 s is closed and redialled.
  TCP carriers also set `TCP_USER_TIMEOUT` to 30 s so a write into a dead
  path fails, and QUIC uses a 10 s keepalive period with a 30 s idle
  timeout. These match the gost driver's mux and QUIC settings and the
  health-check cadence (H21).
- **L5. Stuck carriers are retired.** A dialler that sees `OPEN` frames go
  unanswered by `RESULT` for 5 s on a carrier that otherwise answers `PING`
  retires it (`GOAWAY`) and opens a new one.

### 4.7 Half-close and resets

- A `DATA` frame with `FIN` (or a QUIC stream's FIN) means the sender has
  finished writing: the far side calls `CloseWrite` on its socket and keeps
  reading. The stream ends when both directions have finished. A client's
  half-close therefore reaches the target as a half-close over relayed
  links, which gost cannot do over TLS or mux (`forward-sdk.md` section 6.2).
- A client or target reset (RST) becomes `RESET` (`peer_reset`) and is
  passed on as an abortive close (`SO_LINGER` 0) on the far socket.

### 4.8 UDP

- A client UDP association (one client address and port on an entry
  listener) is one `UDP` stream; its id names the association. It ends 60 s
  after its last datagram, as gost's UDP sessions do.
- **QUIC carriers: native.** Each datagram is a QUIC DATAGRAM frame holding
  the association's stream id (a QUIC variable-length integer) and the
  payload; delivery is unreliable and unordered, as UDP's is. A datagram
  larger than the connection's current maximum DATAGRAM size is sent on the
  association's stream instead (as below) rather than dropped, and counted
  (`udp_oversize_fallback`).
- **TCP carriers or QUIC without datagram support: UDP over a stream.**
  `DATAGRAM` frames on the association's stream, one datagram per frame,
  boundaries preserved. Datagrams are then reliable and ordered, with TCP's
  head-of-line effect; latency-sensitive UDP should use QUIC (`AUTO` prefers
  it). A datagram never waits for credit beyond its stream window: when the
  window is exhausted it is dropped and counted, as a full socket buffer
  would.
- Datagram counters count packets as well as bytes, so the anixops engine
  reports packets for UDP (TCP streams report 0 packets, like gost).

### 4.9 PROXY protocol v2 toward targets (optional per route)

When a route asks for it, the exit (the last hop) writes a PROXY protocol
v2 header (binary format, `PROXY` command) with the original client address
from `OPEN` as the first bytes of every TCP connection to a target. Only
the entry sets the client address; relays pass it on unchanged, and only a
peer in `ingress_peers` can send `OPEN`. UDP targets get no header in the
first version (P7). Contract fields: section 6.5.

## 5. Carriers

### 5.1 TLS over TCP (`TLS_TCP`)

The default carrier: TLS 1.3 over TCP with the framing of section 4.2, ALPN
`anixops/1`. `TCP_NODELAY` is set (frames are already batched by the
writer), keepalives as in section 4.6.

### 5.2 QUIC (`QUIC`)

QUIC version 1 (RFC 9000) with TLS 1.3 (RFC 9001), the same certificates,
checks and ALPN, native streams and DATAGRAM frames (RFC 9221). Preferred
on lossy or long links because a lost packet stalls only its stream, and
for UDP traffic. The library must satisfy `forward-sdk.md` section 1's
licence rule; `quic-go` is already an indirect dependency of this module
and is the candidate (P2). The installer's forward-node sysctl drop-in
must raise `net.core.rmem_max` and `wmem_max` for QUIC's socket buffers.

### 5.3 Plaintext (`PLAIN`, trusted links only)

The framing of section 4.2 over plain TCP, without TLS. It mirrors how
nftables' `RAW` links are used: on IEPL/IPLC private lines and
same-provider intranets, where the link itself is trusted and the CPU cost
of encryption buys nothing. Its gains over nftables are multiplexing,
per-hop payload counters, health-checked failover and UDP aggregation.

- **Authenticity** comes only from the listener's `ingress_sources`
  admission (the previous hop's addresses), exactly as for `RAW` links. No
  confidentiality, integrity or replay protection beyond the link's own.
- **Allowed only** when both hops' nodes carry a trusted-link label
  (`link=iepl` or `link=iplc`, the labels `forward-sdk.md` section 4.3
  already uses) and the route is an administrator's; validation refuses it
  otherwise (code `plain_untrusted`, P4).
- Never selected by `AUTO` and never a fallback.

### 5.4 Selection and fallback

The link's carrier is a new `LinkTransport` field (section 6.5):

| Value | Dialler behaviour |
|---|---|
| `AUTO` (default) | QUIC first; if no QUIC handshake completes within 3 s, or three QUIC dials in a row fail, use `TLS_TCP`, and re-try QUIC every 5 minutes while on the fallback |
| `TLS_TCP` | TLS over TCP only |
| `QUIC` | QUIC only; failures count against the upstream like any dial failure |
| `PLAIN` | plaintext TCP only (section 5.3) |

The listener of an `AUTO` link listens on the hop's port for both: TCP for
`TLS_TCP` and UDP for QUIC. The planner already holds each port per node
for TCP and UDP together (`forward-sdk.md` section 5.2); Apply's pre-bind
(section 6.1) refuses a port another process holds in either protocol. Fallback is only ever between the
two authenticated carriers; nothing falls back to plaintext. A typical
reason for the fallback is a firewall that blocks UDP between the nodes;
the fallback count is a metric (section 7.3).

## 6. Integration

### 6.1 The `anixops` driver

`sdk/forward/driver/anixops` implements `driver.Driver` for
`ENGINE_ANIXOPS`, following every rule of `forward-sdk.md` section 6.0:

- **Capabilities** probes the relay binary (`anixops-relay -V`), its unit,
  the link certificate files and UDP socket buffer limits; it reports
  `LINK_SECURITY_ANIXOPS`, every strategy, IPv6, UDP, `bandwidth_limit`,
  `quota` (exact, section 6.3) and `max_conns`, plus the new carrier,
  protocol-version and PROXY v2 fields (section 6.5). Without link files
  only `PLAIN` is offered.
- **Render** is pure: a JSON configuration (as the gost driver writes one)
  with a manifest of the driver's ownership mark, the hops, and per hop its
  listener, carrier, `ingress_sources`, `ingress_peers`, upstreams with
  `peer_identity` and weights, strategy, breaker, limits, PROXY v2, pause.
  Goldens in `contracts/forward/v1/anixops`. `Content` excludes generation,
  `state_hash` and `node_ref`.
- **Apply** writes the configuration (temporary file and rename) and sends
  it to the relay over its control socket. The relay validates the whole
  configuration and pre-binds every new listener first; if any check or
  bind fails nothing changes (`ErrConflict` for a port another process
  holds). Then it changes hop by hop: hot fields (upstreams, weights,
  limits, quota, admission, peers, pause) in place, keeping listeners,
  carriers, streams and epochs; a hop whose listener changes gets a new
  listener and its old carriers are closed (L1). Other hops are untouched.
  The applied generation, `state_hash` and digest are recorded in the
  driver's `state.json`. Restarting the relay is needed only to change its
  binary.
- **Observe** reads per-hop counters, the rotation, passive upstream
  failures and retired counters from the control socket.
- **SetUpstreams** replaces a hop's rotation in place; established streams
  keep their upstream.
- **Remove** stops the relay and deletes the driver's files.
- **Credential reload** happens inside the relay (section 3.5); the driver
  only tells it to re-read the files, so no epoch ends.

### 6.2 Process model

**Recommendation: a separate unit, `anixops-relay.service`** (P1), built
like `anixops-gost.service`:

- its own user `anixops-relay` with `CAP_NET_BIND_SERVICE` only, and the
  same sandbox (`NoNewPrivileges`, `ProtectSystem=strict`,
  `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`,
  `SystemCallFilter=@system-service ~@privileged`, and the rest of
  `forward-sdk.md` section 6.2);
- the binary shipped in the Agent's package, one version per Agent release
  (as H20 does for gost), at `/usr/lib/anixops-agent/anixops-relay`;
- its files in `/var/lib/anixops-relay` (configuration, `state.json`, the
  QUIC stateless reset key, `tls/` with the link files, group-readable by
  `anixops-relay` only) and its control and metrics sockets in
  `/run/anixops-relay`, guarded by file permissions as gost's web API is;
- the protocol itself (framing, carriers, verification) as a library in the
  SDK (`sdk/forward/relay`), the binary's `main` in the Agent repository.

Why not inside the Agent:

- **Upgrades keep forwarding.** Restarting or upgrading the Agent must not
  end connections (the reason H20 put gost in its own unit).
- **Privilege separation.** The relay parses bytes from the network on
  public ports. H13 gives the Agent `CAP_NET_ADMIN` and H28 keeps the
  Agent's Control key away from the forwarding process; a parser bug in
  the Agent would expose both.
- **Crash isolation.** A relay crash restarts the relay (systemd
  `Restart=on-failure`), not the Agent's Control stream, nftables hops or
  reports.

The gost lessons shape the control path: per-hop changes over a unix
socket, so a structural change of one route never restarts the process, and
a restart (binary upgrade only) closes every carrier cleanly (L3). Handing
listening sockets to the new binary on upgrade, so that even upgrades keep
established connections, is a later improvement (P10).

### 6.3 Counters, epochs and limits

- **Counters** are payload bytes per hop and direction (`up` client to
  target, `down` back), counted where the hop reads from and writes to its
  client side: comparable with nftables' and with the end-to-end suite's
  payload checks. UDP datagrams count packets too. `total_conns` and
  `active_conns` count client connections and UDP associations on the entry
  and streams on later hops.
- **Epochs** follow the driver rules: the epoch of a hop is a hash of the
  relay instance (a random id made at start) and the hop listener's
  creation sequence. A relay restart or a re-created listener starts a new
  epoch; hot changes, `SetUpstreams`, quota changes, pauses and certificate
  renewals keep it. The relay keeps the final counters of a hop whose epoch
  ended in a retired list until an Observe has read them, so nothing is
  lost between two observations. Counters are not persisted across a
  restart; the new epoch is counted in full by Control's ledger
  (`forward-sdk.md` section 11).
- **Quota** is exact: the relay counts every byte it moves, so the entry
  stops the hop's traffic within one frame of `quota_bytes` in the current
  epoch, and admits again when the quota is raised. Established streams are
  ended when the quota is reached, as nftables' named quota drops them.
  `QuotaEnforcer` is not needed.
- **Bandwidth** is a token bucket per hop and direction; **max_conns**
  counts client connections on the entry.
- **Least connections** reads active streams per upstream from the relay
  (exact, unlike gost, where `ss` sees only carriers on a multiplexed link).

### 6.4 Health and failover

The Agent's health loop (`forward-sdk.md` section 7.3) is unchanged and
calls `SetUpstreams`. In addition, failed `OPEN`s (`RESULT` failures, dial
timeouts) are passive failures for the breaker, and the relay reports them
in `Observe` as `UpstreamHealth`. A carrier's `PING` round trip is a free
latency sample for `forward-sdk.md` section 7.5.

### 6.5 Contract additions (forward.v1, additions only)

`anixops.forward.v1` is binding: elements are only added. Proposed
(numbers tentative; the contract PR fixes them):

| Element | Proposal |
|---|---|
| `enum AnixOpsCarrier` | `ANIXOPS_CARRIER_UNSPECIFIED = 0` (means `AUTO`), `AUTO = 1`, `TLS_TCP = 2`, `QUIC = 3`, `PLAIN = 4` |
| `LinkTransport.carrier` | `AnixOpsCarrier carrier = 5`; only for `LINK_SECURITY_ANIXOPS`, refused otherwise |
| `enum ProxyProtocol` | `PROXY_PROTOCOL_UNSPECIFIED = 0` (off), `PROXY_PROTOCOL_OFF = 1`, `PROXY_PROTOCOL_V2 = 2` |
| `Policy.proxy_protocol` | `ProxyProtocol proxy_protocol = 7`; the route's choice |
| `NodeHop.proxy_protocol` | `ProxyProtocol proxy_protocol = 17`; set by the planner on the last hop only |
| `EngineCapabilities.carriers` | `repeated AnixOpsCarrier carriers = 12` |
| `EngineCapabilities.proxy_protocol` | `bool proxy_protocol = 13` |
| `EngineCapabilities.protocol_versions` | `repeated uint32 protocol_versions = 14`; wire versions the node speaks |
| Violation codes | `carrier_unsupported`, `plain_untrusted`, `server_name_unsupported`, `proxy_protocol_unsupported` |

`LinkTransport.mux` is implied for ANIXOPS links (always multiplexed);
validation accepts either value and the planner renders `true`. No key
material and no secrets are added to the state. The gost engine could
adopt `proxy_protocol` later; until then validation refuses it on gost
exits.

### 6.6 Testing

- **Conformance.** `conformance.Run` against the driver with a simulated
  relay (no privileges) and with the real relay in a network namespace
  (`ANIXOPS_RELAY_E2E=1` as root), no scenario skipped.
- **Transport tests** (netns, real relay, CI shard like the gost suite):
  identity pinning (a certificate from the link CA for an identity not in
  `ingress_peers` is refused; an upstream presenting another identity is
  refused), peer removal closing carriers, certificate renewal without a
  new epoch or dropped connection, half-close through a three-hop chain,
  a server-first protocol, L1 to L5 (restart an exit and see new
  connections through the relay pass within 1 s over TLS and QUIC),
  backpressure (a stalled stream does not stall its neighbour on the same
  carrier), UDP over datagrams and over streams with oversize datagrams,
  PROXY v2 at a target, `AUTO` fallback with UDP blocked, `PLAIN` refused
  without the trusted-link label.
- **Fuzzing** of the frame decoder, `OPEN` parameters and `SETTINGS`
  (Go fuzz tests, run in CI for a bounded time).
- **Forward Netns E2E** gains anixops chains and, once P5 is decided,
  mixed nftables/anixops chains.

### 6.7 Version negotiation

- **Wire version** by ALPN: the dialler offers the versions it speaks,
  highest first (`anixops/2`, `anixops/1`); the listener picks the highest
  it shares, or the handshake fails with `no_application_protocol`. The
  prototype's `anixops/0` never matches a production version.
- **Features within a version** by `SETTINGS` keys: unknown keys are
  ignored, so optional features are added without a new version.
- **Planning.** Nodes report `protocol_versions`; validation refuses a link
  whose two nodes share no version. Each Agent release speaks the current
  version and the previous one, so the upgrade order of
  `forward-sdk.md` section 10 (Agents node by node) never breaks a link.

## 7. Performance, observability and operations

### 7.1 Targets

Targets for v4.3, measured on a reference node (4 vCPU x86-64 with AES-NI,
1 Gbit/s link); the v4.2 prototype reports against them without having to
meet them.

| Measure | Target |
|---|---|
| Single-stream throughput, `TLS_TCP` | line rate (at least 900 Mbit/s on 1 Gbit/s) |
| Throughput per core, `TLS_TCP` | at least 1.5 Gbit/s of payload |
| Throughput per core, `QUIC` | at least 70 % of `TLS_TCP` |
| Added latency per hop, LAN, 50 % load | p50 under 0.5 ms, p99 under 2 ms |
| New stream on a warm carrier | no round trip before the first client byte is sent |
| New streams per second per core | at least 5000 |
| Memory per idle stream | under 32 KiB; 10 000 idle streams under 400 MiB in total |
| CPU per Gbit/s compared with gost 3.2.6 `mtls` | no worse |

### 7.2 Benchmark plan

- Topology: three network namespaces (entry, relay, exit) plus client and
  target, joined by veth pairs; `tc netem` profiles of 0, 30 and 150 ms RTT
  with 0, 0.5 and 2 % loss.
- Workloads: bulk TCP (iperf3), many short TCP connections and a
  request/response pattern (a Go load generator measuring time to first
  byte), UDP at fixed packet rates, and 10 000 idle streams.
- Compared engines on the same topology: nftables (`RAW`), gost `mtls`,
  `quic` and `tls`, and anixops `TLS_TCP`, `QUIC` and `PLAIN`.
- Measured: throughput, CPU per process (cgroup accounting), memory, p50
  and p99 time to first byte, packet loss for UDP.
- Run nightly and before each release, not on every PR; results are kept
  with the release notes, and a regression of more than 20 % against the
  previous release fails the nightly job.

### 7.3 Observability

- **Metrics** on the relay's metrics socket (Prometheus text), labelled only
  by bounded values: route id, hop index, upstream node reference, carrier
  type, reason codes. Never by client address (the unbounded-label problem
  the gost driver avoids). Series: carriers by type and state, handshakes
  and handshake failures by reason (`unknown_ca`, `identity_mismatch`,
  `peer_not_allowed`, `source_not_allowed`, `timeout`), streams opened,
  active and refused, `RESULT` failures and `RESET`s by code, bytes and
  packets per direction, flow-control stalls, keepalive timeouts, QUIC to
  TLS fallbacks, UDP datagrams dropped or sent over a stream, and the link
  certificate's remaining lifetime.
- **Reports.** The driver maps counters and health into
  `NodeForwardReport` as the other engines do; handshake failures by
  identity and the fallback state are summarised as hop errors when they
  persist, so the UI shows them.
- **Cross-checks.** Control can compare an entry's `up` bytes with its
  exit's for the same route and epoch window to spot a misbehaving or
  broken relay (section 2.3).
- **Logs.** Structured JSON to the journal: carrier lifecycle and handshake
  failures with the peer identity and address, configuration changes;
  client addresses only at debug level, which is off by default.

### 7.4 Operations

- Installed, upgraded and removed with the Agent: the installer writes the
  unit (golden `contracts/forward/v1/anixops/anixops-relay.service`), the
  uninstaller removes the unit and `/var/lib/anixops-relay`.
- Rollback: change a route's engines back to gost or nftables; the relay
  stops when no anixops hop is left on the node (an empty artifact).
- Runbook entries: `identity_mismatch` (a node's link certificate or the
  state is stale; check renewal), `peer_not_allowed` (the dialler is not in
  the route's previous hop), rising QUIC fallbacks (UDP blocked between the
  nodes), `quota_exceeded` results, certificate lifetime under one day
  (renewal is failing).

## 8. Reserved: camouflage (owner to author)

This section is intentionally left for the owner to specify.

## 9. Plan, risks and open questions

### 9.1 Plan

| Phase | Release | Content | Gate |
|---|---|---|---|
| A0 | v4.2 | this document | H22 |
| A1 | v4.2 | `sdk/forward/relay`: framing, TLS and plaintext carriers, verification, fuzzing | H22 |
| A2 | v4.2 | QUIC carrier and native UDP | P2 |
| A3 | v4.2 | `anixops` driver, the relay binary and unit (agent), goldens, conformance and netns tests | P1 |
| A4 | v4.2 | contract additions of section 6.5 | owner review |
| A5 | v4.2 | benchmarks of section 7.2 | |
| A6 | v4.3 | freeze wire version 1, production defaults, runbook | owner sign-off, section 8 |

**v4.2: experimental, off by default.** Two flags, both needed:

- Control: `forward.anixops_experimental` (default `false`) sets
  `validate.Options.EnableAnixOps`, so routes may use `ENGINE_ANIXOPS` and
  `LINK_SECURITY_ANIXOPS`;
- Agent: `forward.anixops_experimental` (default `false`) registers the
  driver, so the node advertises the engine; without it the planner refuses
  the node for anixops hops.

The prototype speaks ALPN `anixops/0`; its wire format may change without
notice and it never interoperates with `anixops/1`. The UI marks anixops
routes experimental.

**v4.3: production.** Wire version 1 frozen, the flags default to on (the
engine still needs a node that advertises it), and the gost engine stays
for links the anixops engine does not cover.

### 9.2 Risks

- **Own framing.** Parser and flow-control bugs; mitigated by a small frame
  set, fuzzing, hard limits and the conformance and netns suites.
- **QUIC cost.** QUIC in user space costs more CPU per byte than TLS over
  TCP and needs larger socket buffers; mitigated by `AUTO`'s TLS fallback,
  per-link carrier choice and the benchmarks.
- **TCP head-of-line blocking** on lossy links with `TLS_TCP`; mitigated
  by carrier pools and QUIC.
- **Go runtime** latency at high stream counts (GC, goroutine per stream);
  measured by the 10 000-stream benchmark.
- **A second encrypted engine to maintain** next to gost; accepted, since
  gost cannot pin identities, half-close or renew certificates without
  ending connections.
- **Contract lock-in.** Section 6.5's fields are permanent once merged;
  they are kept engine-neutral where possible.
- **Pre-authentication exhaustion** of handshake slots; mitigated by
  source admission before the handshake, bounded queues and QUIC Retry.

### 9.3 Open questions

| # | Question | Recommendation |
|---|---|---|
| P1 | Run the relay in the Agent or as its own unit | Its own unit, `anixops-relay.service`, user `anixops-relay`, `CAP_NET_BIND_SERVICE` only, controlled over a unix socket (section 6.2) |
| P2 | QUIC implementation | `quic-go`, after checking its licence against `forward-sdk.md` section 1 and pinning one version per Agent release; QUIC lands in A2, after the TLS carrier works |
| P3 | TLS session resumption | Off in v4.2 and by default in v4.3; revisit only if benchmarks show handshake cost matters. TLS early data never |
| P4 | When the plaintext carrier is allowed | Only on administrator routes whose two nodes both carry `link=iepl` or `link=iplc`; refused on user routes |
| P5 | Should the anixops engine also terminate and originate `LINK_SECURITY_RAW`, for nftables entries handing over to anixops relays and anixops exits dialling raw | Yes, in v4.3: it lets the kernel do the entry while anixops carries the long link; v4.2's prototype stays anixops-to-anixops |
| P6 | Own framing or an existing multiplexer | Own framing as specified in section 4.2 (small, half-close, in-band results, bounds), unless a survey in A1 finds an MIT, Apache or BSD library with all three; `yamux` is MPL-2.0 |
| P7 | PROXY protocol v2 for UDP targets | Not in the first version; TCP only, revisit on demand |
| P8 | Emergency link CA rotation (drop a compromised CA without the normal overlap) | Add `anix-control agent link-ca rotate --emergency`, which drops the old CA from the bundle at once and makes every node renew; links recover as nodes renew |
| P9 | Revocation checks (CRL or OCSP) on links | None: identity pinning plus state changes remove a peer at once (section 3.5), and certificates live 7 days |
| P10 | Keep established connections across relay binary upgrades | Not in v4.3; consider passing listening sockets to the new process later. Upgrades are rare and announced |
