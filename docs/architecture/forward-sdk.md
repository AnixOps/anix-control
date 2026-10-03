# Forward SDK: routes, hops, engines and drivers

Status: DESIGN APPROVED (gates H11–H14 decided by the owner on 2026-10-02).
F1b is implemented: the domain model (`sdk/forward/model`) and the shared
validation (`sdk/forward/validate`). Nothing serves forwarding with them yet
and no behaviour changes. The draft contract is
`sdk/api/forward/v1` (`anixops.forward.v1`, DRAFT, UNRELEASED); the draft
planner goldens are in `contracts/forward/v1`. This is phase F1 of the v4.2
forwarding redesign. It replaces the flux-panel clone (`/api/v2/forward/*`)
in v4.2.

> 中文摘要：v4.2 把转发做成 AnixOps SDK 的一等能力，不再兼容 flux，也不提供独立 CLI，
> 所有操作都经过 Control。
> - **模型**：一条转发路由（Route）是一串跳（Hop）：入口 → 可选中转 → 出口 → 目标。
>   每一跳各选引擎：`NFTABLES`（内核 DNAT，不加密，用于 IEPL/IPLC 等可信链路）、
>   `GOST`（gost v3，TLS/WSS/QUIC/gRPC/多路复用，用于公网）、`ANIXOPS`（自研协议，v4.3）。
> - **规划器**：纯函数，把路由渲染成每个节点的期望状态 `NodeForwardState`，
>   负责分配端口、串联各跳、把限速/配额/连接数/到期放在入口跳，并按节点递增代数（generation）。
> - **驱动**：统一接口 `Capabilities / Render / Apply / Observe / Remove`。
>   nftables 驱动只管自己的 `inet anixops_fwd` 表，用 `ct direction` 分上下行计数、
>   numgen/jhash 映射做负载均衡、命名 quota、`ct count`、tc HTB + ct mark 限速，`nft -f` 原子应用；
>   gost 驱动由 Agent 托管 gost v3，取代 NodeX；Ansible 只做无 Agent 主机的兜底，下发同一份 nft 产物。
> - **负载均衡与故障转移**：轮询/随机/IP 哈希/最少连接/主备，出口级和目标级两层，
>   熔断（建议连续 3 次失败跳过 30 秒），入口用 DDNS/CNAME 做高可用，逐跳延迟探测。
> - **传输**：期望状态随 `config.v1` 下发，报告走 `PackageReport`；一条长连接、低频心跳，
>   Control 宕机时节点照常转发。
> - **对接与升级**：nyanpass 式一条命令安装；v4.2 升级时旧 `v2_forward*` 数据归档为仅管理员可读的
>   JSON，清理节点上的旧规则后删表（**不可逆**，H15）。
> - 文末列出待你拍板的问题（H11–H14、H18–H23），每条附推荐答案。

## Contents

1. [Why](#1-why)
2. [Goals and non-goals](#2-goals-and-non-goals)
3. [Layers](#3-layers)
4. [Model](#4-model)
5. [Planner](#5-planner)
6. [Drivers](#6-drivers)
7. [Load balancing and failover](#7-load-balancing-and-failover)
8. [Control and Agent transport](#8-control-and-agent-transport)
9. [Node onboarding](#9-node-onboarding)
10. [Upgrade from v4.1](#10-upgrade-from-v41)
11. [Metering](#11-metering)
12. [Editions](#12-editions)
13. [Testing](#13-testing)
14. [Security](#14-security)
15. [Rollout](#15-rollout)
16. [Open questions for owner review](#16-open-questions-for-owner-review)
17. [Not in scope](#17-not-in-scope)

## 1. Why

**Where v4.1 stands.** Forwarding is a 1:1 clone of flux-panel. AGENTS.md
("Flux-panel Clone Guardrails"), `docs/guide/flux-panel-clone.md`,
`docs/guide/flux-forward-contract.md`, the `forwardcompat` byte-parity tests
and the route gates freeze its API, DTOs and page layout. Its runtime has
three paths, none of them complete:

| Path | What it does | What is missing |
|---|---|---|
| nftables via Ansible | Control SSHes in and runs a playbook that renders `inet v2b_forward` (`internal/service/forward_nftables_plan.go`, `config/deploy/ansible/playbooks/files/v2b_forward_nft.sh`). 4.1.0-rc.5 (F0) moved it to `inet`, added IPv6 and `ct direction` up/down counters | no failover (`fifo` and `hash` use the first target), speed limits never reach the node, no health checks, Control must reach every node over SSH |
| gost via NodeX | Control calls NodeX's HTTP API | NodeX is outside this repository; a tunnel forward does not send the exit node, so the chain depends on NodeX's own state |
| Agent nftables-forward plugin | canary, `ip anixops_forward` table with snapshot rollback and per-rule counters | not tied to `v2_forward`; `config.v1` carries no forwarding |

flux-panel, the upstream, paused development in 2026-01. The market survey
(`forward-market-survey-2026-10`, summarized in the owner's plan) found
that what operators value is load balancing and failover, per-route latency
probes, billing multipliers, forwarding that survives the panel going down,
and easy node onboarding. nyanpass sets the bar on all four.

**Owner decisions (2026-10-02)** this design follows:

- Fully drop flux compatibility in v4.2. The v2 forward routes, tables and
  byte-parity tests are removed. Old data is **not migrated**: it is
  archived to an administrator-only JSON file, nodes are cleaned, and the
  tables are dropped. Operators reconfigure.
- SDK first: forwarding is a first-class capability of the existing `sdk/`
  module. Control, the Agent and other AnixOps products use the same
  contracts and libraries. Compatibility with other panels is not a goal.
- Direction "B": a kernel-first, lean multi-hop design with the engine
  chosen per hop, plus selected commercial features (two-level failover
  with a circuit breaker, metering per hop and direction, end-to-end
  diagnosis, a private-line "direct from entry" policy).
- No standalone CLI. Everything goes through Control; a node never accepts
  routes from anywhere else.
- nftables is applied natively by the Agent. Ansible is only a fallback for
  hosts without an Agent. gost is managed by the Agent; NodeX goes.
- The AnixOps relay protocol ships in v4.3. v4.2 delivers nftables and gost.
- Benchmark nyanpass on onboarding, own protocol (v4.3), load balancing and
  failover, and the commercial loop (commercial edition).

**Licensing.** The design borrows ideas only. nft-forward has no licence and
kids' licence chain is doubtful; FLVX, ForwardX and frp-panel are GPL or
AGPL. None of their code is copied, adapted or translated. Code may be
reused only from MIT, Apache-2.0 or BSD projects (gost, realm, RelayPanel,
go-panel, NodePass, arloor), with attribution. gost v3 (MIT) is run as an
unmodified, pinned binary.

## 2. Goals and non-goals

Goals:

- **G1. One model.** A route is a chain of hops; each hop picks its engine.
  The same contract serves the UI, the operator CLI, the Agent and other
  products.
- **G2. Deterministic planning.** A pure planner turns routes into one
  desired state per node, tested by golden fixtures.
- **G3. Native, owned execution.** The Agent applies the state through
  drivers that own their kernel objects and processes and touch nothing
  else.
- **G4. Correct limits and metering.** Bandwidth, quota, connection limit
  and expiry are enforced on the node; traffic is counted per hop and per
  direction.
- **G5. Failover without Control.** Health checks, circuit breakers and
  failover run on the nodes and keep working while Control is down.
- **G6. nyanpass-grade onboarding.** One command adds a node; Control
  upgrades it; one command removes everything we installed.

Non-goals:

- compatibility with flux-panel, its API, DTOs or data (dropped in v4.2);
- a standalone CLI or node-local configuration that bypasses Control;
- the AnixOps relay protocol itself (design and prototype in v4.2,
  production in v4.3; only the `ANIXOPS` engine slot exists here);
- user self-service forwarding, forwarding plans and resellers (v4.3 and
  later; the model leaves room for them, section 11);
- a web terminal, plugin store or mobile app.

## 3. Layers

Everything lives in the existing `sdk/` module
(`github.com/AnixOps/anix-control/sdk`), versioned with `sdk/vX.Y.Z` tags
(H12). It moves to its own repository only once a second product uses it.

| Layer | Path | Contents |
|---|---|---|
| Contract | `sdk/api/forward/v1` | `forward.proto`: the model, `NodeForwardState`, `NodeForwardReport`, the services `ForwardControl` and `ForwardNode` (this PR, draft) |
| Model and validation | `sdk/forward/model`, `sdk/forward/validate` | Go domain types with lossless conversion to and from the contract and the defaults (`model/defaults.go`); one set of validation rules used by Control, the planner and the Agent (F1b, implemented) |
| Planner | `sdk/forward/planner` | routes and node inventory in, per-node states and port allocations out; pure functions (F1c) |
| Drivers | `sdk/forward/driver`, `.../driver/nftables`, `.../driver/gost`, `.../driver/ansible` | the driver interface and its implementations (F2, F4) |
| Client | `sdk/forward/forwardctl` | a Go client for `ForwardControl` (F5) |

Consumers:

- **Agent** (anix-agent): embeds the drivers. Receives `NodeForwardState`
  in `config.v1` and reports `NodeForwardReport` as a `PackageReport`
  (section 8).
- **Control**: a new forward package built on the planner and serving
  `ForwardControl`, with a native API under `/api/v4/forward/*`, the
  operator CLI `anix-control forward ...` and a new UI (F5).
- **Other AnixOps products**: import `sdk/forward` and call
  `ForwardControl`.

Drivers run only inside the Agent and inside test harnesses (the
conformance suite and the netns end-to-end tests). There is no
`cmd/anixops-forward`.

## 4. Model

The contract is `sdk/api/forward/v1/forward.proto`. Its comments are
normative; this section explains the shape.

### 4.1 Route

```
Route { id, owner, name, listen, hops[], targets[], policy, limits, labels, paused, revision }
Listen { address, port, protocol TCP|UDP|TCP_UDP, entry_hostname }
Hop { role ENTRY|RELAY|EXIT, engine NFTABLES|GOST|ANIXOPS, node_refs[], ingress, port, dial_address }
Target { host, port, weight, priority }
Policy { next_hop, target, health, circuit_breaker, direct, target_policy }
Limits { bandwidth_bps, quota_bytes, max_conns, expires_at_unix_ms }
Counters { route_id, hop_index, node_ref, up/down bytes and packets, active_conns, total_conns, counter_epoch }
```

- **Ids.** `Route.id` is a ULID assigned by Control. Node references are
  the Agent identity names of node-ops-service.md D9 (`forward-<id>`,
  `proxy-<id>`). `owner` is `admin` or `user:<id>`.
- **Hops** are ordered. The first is `ENTRY`; with more than one hop the
  last is `EXIT` and the others are `RELAY`. A one-hop route's entry dials
  the targets itself. `hop_index` is the index in `Route.hops`, 0 for the
  entry.
- **Several nodes per hop.** On `ENTRY` they are entry HA: each runs the same
  listener and DNS points clients at the healthy ones (section 7.4). On
  `RELAY` or `EXIT` they are the previous hop's upstreams, balanced by
  `Policy.next_hop`: this is the exit-level failover.
- **Targets** are addresses or DNS names, dialled by the last hop. Names are
  resolved and re-resolved on that node (section 6.1).
- **Direction.** `up` is client to target, `down` is target to client, on
  every hop.

### 4.2 Engines and links

A hop's engine runs on the hop's nodes and carries traffic to the next hop.
The link between hop *i* and hop *i+1* is `hops[i+1].ingress` (a
`LinkTransport`: security, mux, server name, path). Hop *i* must be able to
originate it and hop *i+1* to terminate it.

| Engine | Originates and terminates | Strengths | Use |
|---|---|---|---|
| `NFTABLES` | `RAW` only | kernel DNAT, near-zero CPU, survives Agent restarts | IEPL/IPLC private lines, same-provider intranets, plain public forwarding |
| `GOST` | `RAW`, `TLS`, `WSS`, `QUIC`, `GRPC`, with optional mux | encryption, obfuscation, multiplexing | cross-border public internet |
| `ANIXOPS` | `ANIXOPS` (v4.3) | mux with 0-RTT, TLS/REALITY-like, QUIC and plain carriers, per node-pair keys | v4.3 |

So `entry NFTABLES → relay GOST(ingress RAW) → exit GOST(ingress TLS)` is
valid: the entry DNATs raw traffic to the relay, which wraps it in TLS to
the exit. `entry NFTABLES → exit GOST(ingress TLS)` is not: nftables cannot
originate TLS (fixture `plan-negative.json`).

### 4.3 Node roles

A node has no fixed role. Roles belong to hops, so one node can be the
entry of one route and the exit of another. The node's inventory
(`NodeInfo`) says what it can do: its addresses, the port range the planner
may allocate from, its engines with their capabilities
(`EngineCapabilities`), and labels such as `link=iepl` that mark trusted
links. The Agent reports capabilities at enrollment and in every hello.

### 4.4 Validation

`sdk/forward/validate` holds the rules (implemented in F1b; the package
documentation lists every rule and its code). Control runs them before
storing a route, the planner runs them again, and the Agent re-checks what
it can see (target addresses after resolution with
`validate.CheckTargetAddress`, section 14). Each failure is a
`Violation{field, message, code}` with a path into the route
(`hops[1].ingress.security`), an English message and a stable code
(`link_unsupported`) that UIs and API clients act on without parsing the
message (`Violation.code`, added in F1b under the draft golden policy).
Rules that need the node inventory run only when one is given. The
main rules:

- hop roles in order; every node exists, is enabled, and advertises the
  hop's engine with every feature the route needs (strategy, link
  security, UDP, IPv6, limits);
- links: the dialling engine originates and the listening engine
  terminates `hops[i+1].ingress`;
- `dial_address` only with exactly one node on that hop;
- ports in 1..65535, inside the node's range, not reserved (section 14),
  not taken by another route's listener on the node;
- targets: syntax, and the target policy (section 14);
- limits: non-negative; `expires_at` in the future on create.
- size caps (hops, nodes per hop, targets, labels, the encoded route) and
  the health and breaker bounds are proposals in
  `sdk/forward/validate/caps.go`; the defaults (section 7.3, H21) are in
  `sdk/forward/model/defaults.go`.

## 5. Planner

### 5.1 Contract

```go
// Plan renders every route that touches the given nodes into one state per
// node. It reads nothing but its arguments and returns the same output for
// the same input.
func Plan(routes []*forwardv1.Route, nodes []*forwardv1.NodeInfo, previous Allocations) (Result, error)

type Result struct {
    States      map[string]*forwardv1.NodeForwardState // by node_ref, generation 0, no state_hash
    Allocations Allocations                            // (route, hop, node) -> port and mark
    Violations  []*forwardv1.Violation
    Warnings    []string
}
```

Control keeps the allocations and the generation counters in its own
tables, and sets `generation` and `state_hash` after merging. `PlanRoute`
exposes a single-route run for previews and the goldens in
`contracts/forward/v1`.

### 5.2 Port allocation

- The entry listens on `Route.listen.port`; 0 asks the planner for one from
  the entry nodes' ranges (the same port on every entry node, so DNS-based
  entry HA works).
- Every other hop listens on `Hop.port`, or on a port the planner allocates
  from each node's range when it is 0. Nodes of one hop may get different
  ports; the previous hop dials each on its own.
- **Allocations stick.** A `(route, hop, node)` keeps its port across
  replans, so editing a route's targets never moves its relay port. A
  deleted route's ports are released after a grace period (proposed 10
  minutes), so a late packet never reaches a new route.
- The planner refuses a port that another listener of ours holds on the
  node. The Agent's preflight and the `LISTEN` probe catch ports held by
  others (section 7.6).

### 5.3 Chain wiring and where limits land

For each hop *i* and each of its nodes the planner emits a `NodeHop`:

- `listen`: `Route.listen` on the entry; the allocated port (bound to
  `dial_address` when one is set) otherwise;
- `ingress`: the link it terminates (`RAW` on the entry);
- `upstreams`: hop *i+1*'s nodes, each with its address (`dial_address` or
  the node's primary address), allocated port, the link to dial with and,
  for encrypted links, the next node's pinned Agent identity; on the last
  hop, the targets;
- `balance`: `Policy.next_hop` towards nodes, `Policy.target` towards
  targets;
- `health` and `circuit_breaker` with defaults filled in (section 7.3);
- `ingress_sources` and `ingress_peers`: the previous hop's node addresses
  and identities, so a relay or exit port is not an open proxy;
- `mark`: a per-node connection mark for the route (section 6.1).

**Limits land on the entry hop** and only there. The entry is where clients
connect, so it is the one place that sees all of a route's traffic exactly
once and can refuse a connection before it costs relay or exit bandwidth.
With several entry nodes:

- `bandwidth_bps` and `max_conns` apply per entry node (documented; an
  exact global cap would need coordination between nodes);
- `quota_bytes` is global. Control is authoritative: it sums the entries'
  counters and pauses the route when the quota is used up. Each entry also
  carries a node-local named quota of the remaining bytes at render time,
  so the route stops even while Control is down; Control re-renders the
  remainder as counters arrive;
- `expires_at_unix_ms` is enforced by Control (pause) and by the Agent
  (a local timer), for the same reason.

### 5.4 Generations

- A node's `generation` increases whenever any hop on the node changes. A
  change to one route bumps only the nodes it touches.
- `state_hash` is the SHA-256 of the canonical (deterministic protobuf)
  encoding of the state's hops. Control and the Agent compare hashes, never
  contents.
- The Agent applies only a newer generation, persists the last applied
  state locally, and reports the generation and hash it runs. Control shows
  a node as converged when they match.

### 5.5 Direct mode

`Policy.direct` is the private-line "direct from entry" policy. `PREFERRED`
renders the targets as extra upstreams of the entry with a better priority
than the next hop, so the entry dials them while they are healthy and falls
back to the chain otherwise; `FORCED` renders only the targets and the
planner refuses a route whose later hops would then be unused.

## 6. Drivers

### 6.0 Interface

```go
type Driver interface {
    // Capabilities probes the host: engine version, IPv6, UDP, strategies,
    // link securities, limits support. Unavailable drivers say why.
    Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error)
    // Render turns the engine's hops of one node state into the engine's
    // artifact (nft script, gost config). Pure; golden-tested.
    Render(state *forwardv1.NodeForwardState) (Artifact, error)
    // Apply makes the host run exactly the artifact, atomically, touching
    // only objects the driver owns.
    Apply(ctx context.Context, artifact Artifact) error
    // Observe reads counters and upstream health.
    Observe(ctx context.Context) (Observation, error)
    // SetUpstreams changes which upstreams of a hop are in rotation
    // (failover) without a full apply.
    SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []Upstream) error
    // Remove deletes everything the driver owns.
    Remove(ctx context.Context) error
}
```

The Agent splits a node's state by engine, renders and applies each part,
and runs one health loop that calls `SetUpstreams`. A hop that fails to
apply is reported as a `HopError`; the node's other hops keep running.

### 6.1 nftables

**Ownership.** One table, `inet anixops_fwd`. The driver creates, rewrites
and deletes only this table, its own tc handles and its own sysctl drop-in.
It never runs `flush ruleset` and never edits another table.

**Shape** (illustrative; exact syntax is settled in F2 with the netns
tests). For fixture `plan-single-hop-nftables-iepl.json`:

```
table inet anixops_fwd {
  counter r1_h0_up {}
  counter r1_h0_down {}
  quota r1_q { over 1099511627776 bytes used 0 bytes }
  set r1_conns { typeof ct mark; flags dynamic; }
  map r1_up4 { typeof numgen inc mod 1 : ip daddr . th dport; elements = { 0 : 10.88.0.20 . 443 } }

  chain prerouting {
    type nat hook prerouting priority dstnat; policy accept;
    meta l4proto { tcp, udp } th dport 30001 fib daddr type local goto r1_h0_dnat
  }
  chain r1_h0_dnat {
    ct mark set 0x00010000
    meta nfproto ipv4 dnat ip to numgen inc mod 1 map @r1_up4
  }
  chain forward {
    type filter hook forward priority filter; policy accept;
    ct mark and 0x0fff0000 == 0x00010000 jump r1_h0_acct
  }
  chain r1_h0_acct {
    ct state new add @r1_conns { ct mark ct count over 2000 } reject
    quota name "r1_q" drop
    ct direction original counter name "r1_h0_up" meta mark set ct mark
    ct direction reply counter name "r1_h0_down" meta mark set ct mark or 0x1
  }
  chain postrouting {
    type nat hook postrouting priority srcnat; policy accept;
    ct mark and 0x0fff0000 == 0x00010000 masquerade
  }
}
```

- **inet family**, IPv4 and IPv6. A listener DNATs a v4 client to v4
  upstreams and a v6 client to v6 upstreams (one map per family); the
  planner warns when a family has no upstream.
- **Counters per direction.** Named counters in the filter `forward` chain,
  selected by `ct direction original|reply` on the route's connection mark.
  This fixes v4.1's undercounting (counters in the nat hook see only a
  connection's first packet). Named counters survive rule rewrites.
- **Load balancing maps.** `numgen inc` (round robin), `numgen random`
  (random), `jhash ip saddr` (IP hash), weighted intervals for weights and
  approximate least-conn, a one-element map for failover. Failover and
  health only rewrite map elements (`SetUpstreams`), never rules.
- **Named quota** for `quota_bytes`. When a route's quota changes, the quota
  object is re-created with `used` set to the bytes already counted, so a
  limit change never resets usage.
- **`ct count`** in a dynamic set keyed by the route's mark for
  `max_conns`.
- **Bandwidth: tc HTB + ct mark.** The filter chain copies the connection
  mark to the packet mark, with the low bit for direction. On each egress
  interface the driver owns one HTB qdisc handle and one class per route and
  direction at `bandwidth_bps`, selected by a `fw` filter on the mark. Both
  directions leave the node as egress, so no ifb is needed.
- **Marks.** The driver owns a mark mask (proposed `0x0fff0000`: 4095 hops
  per node); the planner allocates `NodeHop.mark` within it. The mask is
  configurable per node to avoid other mark users (Docker, WireGuard,
  policy routing).
- **Flowtable** (optional, off by default): offloaded flows skip the forward
  chain, so it is allowed only on routes without bandwidth or quota limits,
  and counters then come from conntrack accounting.
- **MSS clamping** on encapsulating egress interfaces; `ip_forward` and IPv6
  forwarding via a sysctl drop-in written at install.
- **DNS targets.** The Agent resolves target names (honouring TTL, at most
  every 60 s), re-checks the target policy on every answer, and rewrites
  the maps.
- **Atomic apply.** Render produces one script. Apply diffs the desired
  objects against `nft -j list table inet anixops_fwd`, then builds one
  transaction: ensure the table, flush the chains it keeps, delete chains,
  maps and sets that are gone, re-add rules and map elements, add new
  counters and quotas, and delete counters of removed hops after their
  final values are reported. `nft -c -f` checks it, `nft -f` applies it. A
  failed apply leaves the previous ruleset in place.
- **Persistence.** nftables rules do not survive a reboot. The Agent
  re-applies its last applied state at start, before it connects to
  Control. Counters then start a new `counter_epoch`.

### 6.2 gost

gost v3 (MIT) replaces the NodeX dependency. The Agent manages it; Control
never talks to gost or NodeX.

- **Binary.** A pinned gost v3 release, shipped in the Agent's package with
  its checksum, upgraded only with the Agent (H20).
- **Process.** A separate systemd unit, `anixops-gost.service`, owned by the
  Agent: restarting or upgrading the Agent does not drop forwarded
  connections. It runs as its own user with `CAP_NET_BIND_SERVICE` only.
- **Configuration.** Render produces the full gost YAML. One gost service
  per `NodeHop`, named `r<route>-h<hop>`:
  - listener and handler from `ingress` (`tcp`/`udp` with a forwarder for
    `RAW`; `relay` over `tls`, `wss`, `quic` or `grpc`, `mtls`/`mwss` for
    mux);
  - a chain whose hop holds one gost node per upstream, dialing with
    `egress`;
  - the selector from `balance` (`round`, `rand`, `hash`, `fifo` for
    failover) with `maxFails` and `failTimeout` from the circuit breaker;
  - a traffic limiter for `bandwidth_bps` and a connection limiter for
    `max_conns`;
  - metrics on a loopback listener for per-service input and output bytes.
- **Hot updates** go through gost's web API, bound to loopback with a
  random key the Agent generates. A change to one hop touches one service;
  the full file is written for restarts.
- **TLS.** Links between nodes are mutual TLS with the nodes' AgentPKI
  certificates. The dialler pins `Upstream.peer_identity`; the listener
  accepts only `ingress_peers`. No key material travels in the state.
- **Quota.** gost has no byte quota. The Agent enforces `quota_bytes` on a
  gost entry from the observed counters and stops the service when it is
  used up (granularity: the observe interval, proposed 10 s). Entries that
  need an exact quota should be `NFTABLES`.

### 6.3 Ansible fallback

For hosts without an Agent, Control renders the same nftables artifact with
the same driver and ships it over SSH with a small apply script (`nft -c
-f`, then `nft -f`), replacing `v2b_forward_nft.sh`. Statistics come from a
playbook reading `nft -j list counters table inet anixops_fwd`. Limits:

- nftables only, no gost;
- health checks and failover run from Control's vantage and re-render, so
  failover takes minutes, not seconds;
- the UI shows these hosts as degraded.

### 6.4 anixops (v4.3)

The `ENGINE_ANIXOPS` slot and `LINK_SECURITY_ANIXOPS` exist so the contract
does not change in v4.3. The planner refuses them until a node advertises
the engine. The protocol's design (multiplexing with 0-RTT, TLS/REALITY-like,
QUIC and plain carriers, native UDP with UDP-over-TCP fallback, per
node-pair keys from AgentPKI, PROXY v2, padding schemes pushed by Control,
fallback to a real site on authentication failure) is a separate document
(H22).

### 6.5 Capability matrix (v4.2 target)

| Feature | nftables | gost | Ansible fallback |
|---|---|---|---|
| TCP / UDP | yes / yes | yes / yes | yes / yes |
| IPv6 | yes | yes | yes |
| Encrypted links | no | TLS, WSS, QUIC, gRPC, mux | no |
| Round robin, random, IP hash, failover | yes | yes | yes, slow failover |
| Least connections | approximate (re-weighting) | approximate (re-weighting) | no |
| Bandwidth limit | tc HTB | traffic limiter | tc HTB |
| Byte quota | exact (named quota) | soft (Agent stops service) | exact |
| Connection limit | `ct count` | connection limiter | `ct count` |
| Counters per direction | yes | yes | yes, polled |
| Failover without Control | yes | yes | no |

## 7. Load balancing and failover

### 7.1 Strategies

| Strategy | Meaning | nftables | gost |
|---|---|---|---|
| `ROUND_ROBIN` | in turn, by weight | `numgen inc` over weighted intervals | `round` |
| `RANDOM` | random, by weight | `numgen random` | `rand` |
| `IP_HASH` | a client address sticks to one upstream while it is healthy | `jhash ip saddr` | `hash` on the client address |
| `LEAST_CONN` | fewest live connections | the Agent re-weights the random map every 10 s from conntrack counts per upstream | the Agent re-weights the gost nodes from metrics |
| `FAILOVER` | the healthy upstream with the lowest priority | one-element map | `fifo` |

Neither engine has a native least-connections scheduler; the approximation
is documented in the UI.

### 7.2 Two levels

- **Exit level.** A `RELAY` or `EXIT` hop with several nodes: the previous
  hop balances over them with `Policy.next_hop`. Fixture
  `plan-nft-entry-gost-relay-exit-failover.json` fails over from
  `forward-41` to `forward-42`.
- **Target level.** The last hop balances over the targets with
  `Policy.target`.

Each node decides for its own upstreams, from its own checks, so failover
works while Control is down.

### 7.3 Health checks and the circuit breaker

- **Active checks.** Each node dials each of its upstreams (TCP connect;
  for UDP routes, a UDP exchange where the upstream is one of our nodes)
  every `health.interval_ms` (default 5000) with `timeout_ms` (default
  2000).
- **Passive failures.** A failed dial of a real connection counts too.
- **Circuit breaker** (proposed defaults from RelayPanel's practice, H21):
  `failure_threshold` 3 failures in a row open the breaker; the upstream is
  skipped for `open_ms` 30000; then one trial decides (half-open). An
  upstream comes back after one success.
- When every upstream is open, the hop keeps the last one in rotation
  rather than refusing all traffic, and reports it.

### 7.4 Entry HA

A route with several entry nodes has an `entry_hostname`. Control keeps it
pointed at the healthy entries:

- DDNS: Control updates A/AAAA records through a DNS provider (proposed:
  Cloudflare, Alibaba Cloud DNS, DNSPod, Huawei Cloud DNS, and a generic
  webhook; H21). Provider credentials stay in Control's secret store.
- CNAME: the operator points their own name at a Control-managed name.

Entry health comes from the entries' reports and Control's own checks.
This is the one failover that needs Control, and it changes DNS only, so
entries keep serving clients that still resolve them.

### 7.5 Latency probes

Every active check records the connect round trip (`UpstreamHealth.rtt_us`).
Control composes per-hop and end-to-end latency for each route and shows
it in the UI, with history.

### 7.6 End-to-end diagnosis

`DiagnoseRoute` runs probes hop by hop, following fluxlite's method of
proving each step rather than inferring it:

1. `LISTEN` on every hop: the port is ours and no other rule or process
   holds it, including nat-table rules of other tables;
2. `TCP_CONNECT` from every hop to its upstreams;
3. `UDP_EXCHANGE` for UDP routes, a real datagram through the route;
4. `DELIVERY`: the entry sends a nonce flow, and the last hop's counters
   prove it arrived and left towards the target.

Probes travel as Agent desired operations (section 8.3).

## 8. Control and Agent transport

### 8.1 Desired state

- A new capability, `forward.v1`, in `Hello.capabilities` and
  `HelloAck.server_capabilities`.
- The node's forwarding state rides in `config.v1`: `ConfigSnapshot` with a
  new format `anixops.nodeconfig/v2`, which is `anixops.nodeconfig/v1` plus
  a `forward` member holding the `NodeForwardState` (protojson). Control
  sends v2 only to Agents that negotiated `forward.v1`; an older Agent
  keeps v1 and gets no forwarding. Every new generation bumps
  `config_revision`.
- `ConfigStatus` answers the snapshot as today. Per-hop errors go in the
  report.

### 8.2 Reports

The Agent sends `NodeForwardReport` as a `PackageReport` (the generic
report added for the systemd services panel, `systemd-services-v4.2`):
`plugin_id` `forward`, `kind` `forward.report`, `version` `v1`,
`payload_json` the report.

- Every 60 s, and after an apply or a health change (at most one every
  10 s).
- `PackageReport` is "latest value wins, no ack, no spool". That is safe
  because counters are cumulative within a `counter_epoch`: Control keeps
  the last value per (node, route, hop, epoch) and adds the difference; a
  new epoch adds its full value. A lost report loses nothing.
- Control accepts the `forward` report from Agents that negotiated
  `forward.v1`, not from package releases. The kernel's report handler
  must allow this alongside the systemd panel's release-capability check.

### 8.3 Probes

`ForwardNode.Probe` is a `DesiredOperation` of kind `forward.probe` with a
`ProbeRequest` as payload; the `ObservedState` carries the `ProbeResult`.

### 8.4 Connection

The lesson from flux issue #25 (a provider banned a node over frequent
panel connections):

- one long-lived Agent Control stream per node; no per-route or polling
  connections;
- a low-frequency heartbeat (proposed 60 s for forward nodes, against
  today's default of 20 s) with jitter;
- reconnects with exponential backoff and jitter, from 1 s to 5 minutes;
- several Control endpoints (IPv6, alternate domains), from enrollment and
  updated in the configuration.

### 8.5 When Control is down

The node keeps forwarding on its last applied state: nftables rules stay in
the kernel and gost keeps running, health checks and failover keep running,
counters keep counting, local quota and expiry still apply. On reconnect
the Agent sends its `config_revision`; Control sends a newer snapshot if
there is one, and the next report brings the counters up to date.

## 9. Node onboarding

The target is nyanpass's experience: copy one command, paste it on the
machine, and the node appears. This is part of F3 and reuses the agent
enrollment (`internal/agentpki`).

- **The command.** The node list and the node's "部署" section have a "复制安装命令"
  button:
  `curl -fsSL https://<control>/install.sh | bash -s -- --token <one-time token> [--group <node group>] [--mirror control|cn|github]`.
  The token is an AgentPKI one-time enrollment credential: single use,
  stored as a hash, bound to a node or (new) to a node group, where first
  use creates the node. Proposed default lifetime 1 hour (today's default
  is 24 hours, maximum 7 days; H18).
- **Mirrors.** The Control domain, a mainland mirror or GitHub releases,
  like nyanpass's two routes.
- **The script** detects the architecture (amd64, amd64-v3, arm64) and the
  init system (systemd, OpenRC), installs the Agent and its pinned gost,
  writes the sysctl drop-in, enrolls (the Agent creates its key and gets
  its certificate), and starts the service. The node then reports its
  capabilities: nftables version, kernel, cgroup version, IPv6.
- **Preflight** before installing anything: kernel and nftables
  availability, conflicting tables or services, ports in use, clock skew,
  reachability of Control. Each failure prints a readable reason and a fix
  command.
- **Offline.** `--offline <package>` installs from a downloaded, signed
  package; the script verifies the signature against Control's release key.
- **Uninstall.** `anixops-agent uninstall [--purge]` removes the
  `inet anixops_fwd` table, our tc qdiscs, the `anixops-gost` unit and the
  sysctl drop-in, then the Agent; `--purge` also removes its keys, state
  and logs. It never touches other tables or services.
- **Upgrades** are pushed by Control in batches with a canary (proposed
  5% → 25% → 100%), with signed artifacts; a node never pulls on its own.
  A batch stops and rolls back automatically when upgraded Agents do not
  reconnect or fail to apply their state (H19). Forwarding continues during
  an Agent upgrade (kernel rules, separate gost unit).
- **One Agent per node.** nyanpass starts extra instances for load
  sharing; we do not need that: nftables is in the kernel and gost scales
  across cores.

## 10. Upgrade from v4.1

Decided: old forwarding data is not migrated. v4.2's upgrade runs three
steps in this order. The last one is **IRREVERSIBLE** and is gate **H15**,
confirmed on its own.

1. **Archive.** Export `v2_forward`, `v2_forward_tunnel`,
   `v2_forward_user_tunnel`, `v2_speed_limit`, `v2_forward_rule` and
   `v2_forward_runtime_job` (and any other forward route tables found by
   the F5 audit) to one JSON file, readable only by administrators (file
   mode 0600 under the data directory, and a super-administrator download).
   Node tokens and other secrets are left out. Forward nodes
   (`v2_forward_node`) are not dropped: they are node inventory and the
   Agent identity (`forward-<id>`).
2. **Clean the nodes.** Before any table is dropped, Control removes the old
   runtime from every node: the `inet v2b_forward` and `ip v2b_forward`
   tables (Ansible path), the `ip anixops_forward` table (the canary Agent
   plugin), and the gost services NodeX or the flux runtime created. Agent
   nodes get a `forward.legacy_cleanup` operation, Ansible hosts a cleanup
   playbook, NodeX hosts NodeX's delete calls. Each node's result is
   recorded and re-checked by listing.
3. **Drop the tables.** Only when every node reports clean, or an
   administrator marks the unreachable ones as abandoned (listed by name),
   and only after the H15 confirmation. A rollback to v4.1 after this step
   needs a database backup; UPGRADE.md says so prominently and says that
   forwarding must be reconfigured.

The flux v2 routes, `forwardcompat`, the route catalog entries and the
flux guardrails in AGENTS.md go in the same release (F5, H17).

## 11. Metering

- **The SDK reports raw counters**: bytes and packets per node, route, hop
  and direction, plus connections. It applies no multiplier.
- **Control keeps a traffic ledger** of the deltas (section 8.2), per
  route, hop, node and direction, and enforces `quota_bytes` on raw bytes.
- **Billing applies multipliers**: billable bytes =
  Σ over billed hops (up × hop multiplier × up multiplier + down × hop
  multiplier × down multiplier). A one-way plan is a direction multiplier
  of 0. By default only the entry hop is billed; relay and exit counters are
  for operations. Plan quotas in billed bytes are enforced by Control,
  which pauses the route.
- Multipliers, forwarding plans and auto-renewal are v4.3 and commercial
  (section 12).

## 12. Editions

`config/editions.json` lists commercial packages and routes. Which forwarding
features go into which edition is open (H23). The proposal:

| Feature | Community | Commercial |
|---|---|---|
| Routes, hops, nftables and gost engines, limits, counters | yes | yes |
| Load balancing, two-level failover, circuit breaker, latency, diagnosis | yes | yes |
| Entry HA via DDNS | yes | yes |
| One-command onboarding, staged upgrades | yes | yes |
| User self-service forwarding page (v4.3) | no | yes |
| Forwarding plans, auto-renewal, billing multipliers (v4.3) | no | yes |
| Resellers and panel federation (v4.4) | no | yes |

## 13. Testing

- **Golden fixtures** in `contracts/forward/v1`: a `PlanRouteRequest` and the
  `PlanRouteResponse` the planner must answer, plus negative cases with the
  expected violation. This PR adds three drafts;
  `internal/tests/protocompat` keeps them parseable as the draft contract
  and internally consistent (ports, wiring, ingress sources, where limits
  land). F1c's planner tests compare its output with them byte for byte,
  and F2/F4 add the rendered nft and gost artifacts per fixture.
- **Planner unit tests**: allocation stickiness, generations, every
  validation rule.
- **Driver conformance suite** (F2a): one scenario list run against every
  driver: single target, each strategy, failover and recovery, limits,
  IPv4 and IPv6, TCP and UDP, idempotent re-apply, counters kept across
  re-apply, removal leaving nothing behind. A fake driver runs it in unit
  tests.
- **netns end-to-end** (F2d): client, entry, relay and target namespaces
  joined by veth pairs, running the real nftables and gost drivers; checks
  traffic, counters, quota, connection limit, bandwidth (with tolerance) and
  failover by killing a target. It needs root and `CAP_NET_ADMIN` (H14).
  GitHub-hosted Ubuntu runners are VMs with passwordless sudo, so the job
  can run `sudo -E go test -tags netns` directly; no privileged container is
  needed.
- **Cross-repository E2E** with anix-agent (the A2-7 suite): config.v1 with
  `forward.v1`, reports, probes.
- **Chaos**: stop Control and check forwarding, failover and counters
  continue; restart the Agent and check no connection drops.

## 14. Security

- **Ownership.** Drivers touch only their own objects: the `inet anixops_fwd`
  table, their tc handles, the `anixops-gost` unit and its files. Uninstall
  and the v4.2 cleanup remove exactly those plus the named legacy tables.
- **Privileges** (H13). Proposed: the installer runs as root once (packages,
  sysctl drop-in, units); the Agent runs as a dedicated user with ambient
  `CAP_NET_ADMIN` (nftables and tc over netlink) and `CAP_NET_BIND_SERVICE`;
  gost runs as another user with `CAP_NET_BIND_SERVICE` only. systemd
  sandboxing: `NoNewPrivileges`, `ProtectSystem=strict`,
  `ReadWritePaths=/var/lib/anixops-agent`, `ProtectHome`, `PrivateTmp`,
  `RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK AF_UNIX`. Agent
  self-upgrade then needs a small root-owned updater unit that installs only
  signed artifacts.
- **Target policy** carried over from v4.1 (`isPublicProbeAddress`):
  `PUBLIC_ONLY` by default refuses loopback, private, link-local, multicast
  and other special-purpose addresses, also when a name resolves to one.
  `ALLOW_PRIVATE` is for administrator routes to an exit's own network,
  never loopback, never on a user's route. Control checks at save time; the
  Agent re-checks every DNS answer (DNS rebinding).
- **No open relays.** Relay and exit listeners admit only
  `ingress_sources` (nftables rules or gost admission), and encrypted
  ingress only `ingress_peers`.
- **Reserved ports.** The planner never allocates, and validation refuses,
  the node's SSH port, the Agent's ports and a per-node reserved list.
- **No secrets in state.** Node-to-node TLS uses the nodes' own AgentPKI
  keys; the state names identities, not keys. The gost API key never leaves
  the node.
- **Every change goes through Control**, authorized and audited there.

## 15. Rollout

Sizes: S (under 300 changed lines), M (300 to 1000), L (over 1000).
Agent-repository PRs are marked (agent).

| Phase | PR | Content | Size | Gate |
|---|---|---|---|---|
| F1 | F1a | this design, draft contract, draft fixtures | M | H11 |
| | F1b | `sdk/forward/model` and `validate` (implemented) | M | |
| | F1c | `sdk/forward/planner`: allocation, wiring, generations, golden runner | L | |
| F2 | F2a | driver interface, fake driver, conformance suite | M | |
| | F2b | nftables Render and nft goldens | L | H13 |
| | F2c | nftables Apply, Observe, `SetUpstreams`, tc HTB | L | H13 |
| | F2d | netns end-to-end CI job | M | H14 |
| F3 | F3a | Control: `forward.v1`, `nodeconfig/v2`, the `forward` report and traffic ledger | M | H25 |
| | F3b | (agent) forward component: drivers, persisted state, apply at boot, health loop, reports | L | H25 |
| | F3c | probes and diagnosis plumbing | M | |
| | O1 | `install.sh`, group tokens, mirrors | M | H18 |
| | O2 | preflight and offline package | M | H18 |
| | O3 | uninstall | S | |
| | O4 | staged upgrades with canary and rollback | L | H19 |
| F4 | F4a | gost driver: Render, process management | L | H20 |
| | F4b | gost Observe, hot updates, failover | M | H20 |
| | F4c | mixed-engine end-to-end | M | |
| | L1 | least-connections re-weighting | S | H21 |
| | L2 | entry HA via DDNS and CNAME | M | H21 |
| F5 | F5a | Control forward package: `ForwardControl`, `/api/v4/forward/*`, `anix-control forward` | L | H23 |
| | F5b | new forwarding UI | L | H16 |
| | F5c | upgrade: archive, node cleanup, drop tables | M | H15 |
| | F5d | remove flux routes, `forwardcompat`, catalog entries; rewrite AGENTS.md rules; archive the flux docs | M | H17 |

F1a–F1c and F2 do not depend on the Agent line. F3 needs AG-1. F5c runs
last and only after its own confirmation.

**Draft golden policy.** As with KernelNodeOps before NO-1
(`node-ops-service.md` section 3.10), `anixops.forward.v1` is in
`contracts/proto/descriptors.golden` and the CI generated-code check from
this PR, so its generated code cannot drift. Until the first change that
serves it (F3a), a design change may edit this package's own golden lines
in the PR that changes the proto. From then on, additions only.

## 16. Open questions for owner review

Each question has a recommendation; the owner's answer is recorded here
when decided.

| # | Question | Recommendation |
|---|---|---|
| H11 | Approve the model (Route/Hop/Policy/Limits/Counters, per-hop engine, link rule), the planner semantics (limits on the entry, sticky allocations, generations) and the draft contract | Approve as drafted; contract freezes when F3a serves it |
| H12 | SDK versioning: `sdk/v0.x` first (breaking changes allowed) or `sdk/v1.0.0` at once | `sdk/v0.x` through v4.2's release candidates; `sdk/v1.0.0` with v4.2.0. Wire compatibility is enforced by `protocompat` either way |
| H13 | nftables driver privileges: own table `inet anixops_fwd` only; `CAP_NET_ADMIN` or root; systemd sandbox | Dedicated user with ambient `CAP_NET_ADMIN` + `CAP_NET_BIND_SERVICE` and the sandbox of section 14; root only for the installer and a signed-artifact updater unit. Mark mask `0x0fff0000`, configurable |
| H14 | May CI run privileged tests for the netns end-to-end suite | Yes, on GitHub-hosted runners with sudo (no privileged container); in the `forward` change class and nightly; required after two green weeks |
| H18 | Install command: parameters, mirrors, token lifetime, where `install.sh` lives | `--token`, `--group`, `--mirror control/cn/github`, `--offline`; tokens single-use, 1 hour by default (max 7 days); `install.sh` served by Control at `/install.sh` and mirrored to GitHub releases, signed |
| H19 | Agent auto-upgrade: batch sizes, rollback conditions, signature checks | 5% → 25% → 100% with at least 30 minutes per batch; roll back a batch when over 5% of its Agents do not reconnect within 10 minutes or fail to apply; artifacts signed with the release key (cosign) and verified by the Agent and the updater |
| H20 | gost version pinning and process management; impact of dropping NodeX | Pin one gost v3 release per Agent release, shipped with the Agent; run it as `anixops-gost.service` owned by the Agent; announce NodeX's removal in v4.2's release notes and UPGRADE (NodeX nodes need the Agent installed) |
| H21 | LB and failover defaults: circuit breaker, check interval, DDNS providers | Breaker 3 failures → skip 30 s; checks every 5 s with a 2 s timeout; least-conn re-weighting every 10 s; DDNS: Cloudflare, Alibaba Cloud DNS, DNSPod, Huawei Cloud DNS, generic webhook |
| H22 | AnixOps protocol design review (threat model, cryptography, REALITY-like fallback) | Separate design document before any prototype; prototype off by default and marked experimental in v4.2 |
| H23 | Community vs commercial boundary for forwarding | Section 12: core forwarding, LB, failover and onboarding in both; self-service, plans, multipliers and resellers commercial |

Decided by the owner (2026-10-02):

- **H11:** approved as drafted. The contract freezes when F3a serves it.
- **H12:** `sdk/v0.x` through v4.2's release candidates, then `sdk/v1.0.0`
  with v4.2.0.
- **H13:** dedicated user with ambient `CAP_NET_ADMIN` (+
  `CAP_NET_BIND_SERVICE`) and the systemd sandbox of section 14. Root is
  only for the installer and the signed-artifact updater unit. The driver
  only touches `inet anixops_fwd`.
- **H14:** the netns suite runs on GitHub-hosted runners with sudo, on
  forward changes and nightly. It becomes a required check after two green
  weeks.

H18–H23 are still open; each is asked before the work it gates.

Smaller questions raised by this design, to settle with the gates above:

- Is the 60 s heartbeat for forward nodes acceptable (section 8.4; H11)?
- Global quota across several entries: Control-authoritative with a local
  remainder (section 5.3; H11)?
- Forward nodes (`v2_forward_node`) kept through the v4.2 upgrade as node
  inventory (section 10; H15)?

## 17. Not in scope

- The AnixOps relay protocol's design and implementation (H22, v4.3).
- User self-service forwarding, plans, auto-renewal and multipliers (v4.3),
  resellers and federation (v4.4).
- API keys with scopes and IP allow-lists, and MCP access to
  `ForwardControl` (later, on Control's API layer).
- Migrating v4.1 forwarding data (decided: archive only).
