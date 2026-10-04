# Forward SDK: routes, hops, engines and drivers

Status: DESIGN APPROVED (gates H11–H14 decided by the owner on 2026-10-02).
F1b and F1c are implemented: the domain model (`sdk/forward/model`), the
shared validation (`sdk/forward/validate`) and the planner
(`sdk/forward/planner`). F2a is implemented: the driver interface
(`sdk/forward/driver`), an in-memory fake driver
(`sdk/forward/driver/fake`) and the driver conformance suite
(`sdk/forward/driver/conformance`). F2b is implemented: the nftables
driver's Render (`sdk/forward/driver/nftables`) with golden scripts in
`contracts/forward/v1/nft`. F3a is implemented: Control serves the
contract (`internal/kernelforward`, section 8): routes, planning and
generations, the node state over the Agent Control stream, the reports and
the traffic ledger, and `ForwardControl` for official packages. F4a is
implemented: the gost driver's Render and process management
(`sdk/forward/driver/gost`) with golden configurations in
`contracts/forward/v1/gost` (section 6.2). F5a is implemented: the
forward package serves `/api/v4/forward/*` on `ForwardControl`
(`packages/forward/v4api`, reference in
[`../forwarding/v4-api.md`](../forwarding/v4-api.md)), and
`anix-control forward` administers routes, nodes and statistics. The
contract is
`sdk/api/forward/v1` (`anixops.forward.v1`), binding since F3a: additions
only (section 15); the planner goldens are in `contracts/forward/v1`. This
is the v4.2 forwarding redesign. It replaces the flux-panel clone
(`/api/v2/forward/*`) in v4.2.

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
- the AnixOps relay protocol itself (design in `anixops-protocol.md`,
  prototype in v4.2, production in v4.3; only the `ANIXOPS` engine slot
  exists here);
- user self-service forwarding, forwarding plans and resellers (v4.3 and
  later; the model leaves room for them, section 11);
- a web terminal, plugin store or mobile app.

## 3. Layers

Everything lives in the existing `sdk/` module
(`github.com/AnixOps/anix-control/sdk`), versioned with `sdk/vX.Y.Z` tags
(H12). It moves to its own repository only once a second product uses it.

| Layer | Path | Contents |
|---|---|---|
| Contract | `sdk/api/forward/v1` | `forward.proto`: the model, `NodeForwardState`, `NodeForwardReport`, the services `ForwardControl` and `ForwardNode` (F1a; binding since F3a) |
| Wire | `sdk/forward/wire` | how forwarding rides the Agent Control stream: the `forward.v1` Hello attribute, the `anixops.nodeconfig/v2` member, the forward report and their checks (F3a, implemented) |
| Model and validation | `sdk/forward/model`, `sdk/forward/validate` | Go domain types with lossless conversion to and from the contract and the defaults (`model/defaults.go`); one set of validation rules used by Control, the planner and the Agent (F1b, implemented) |
| Planner | `sdk/forward/planner` | routes and node inventory in, per-node states, port and mark allocations and generations out; pure functions (F1c, implemented) |
| Drivers | `sdk/forward/driver`, `.../driver/fake`, `.../driver/conformance`, `.../driver/nftables`, `.../driver/gost`, `.../driver/ansible` | the driver interface, registry, fake driver and conformance suite (F2a, implemented), the nftables driver (Render F2b, Apply, Observe, failover and tc F2c, implemented), the gost driver (Render and process management F4a; Observe, hot updates over gost's web API, failover and the soft quota F4b; per-service structural changes over the web API F4c; implemented), least-connections re-weighting (`sdk/forward/leastconn`, L1, implemented) and the Ansible fallback |
| Client | `sdk/api/forward/v1` | the generated `ForwardControlClient`, which the forward package's v4 API uses directly (F5a); a higher-level `sdk/forward/forwardctl` waits for a second caller |

Consumers:

- **Agent** (anix-agent): embeds the drivers. Receives `NodeForwardState`
  in `config.v1` and reports `NodeForwardReport` as a `PackageReport`
  (section 8), both through `sdk/forward/wire`.
- **Control**: the kernel's forwarding state (`internal/kernelforward`,
  F3a) runs the planner and serves `ForwardControl` to official packages
  (section 8). The forward package builds on it a native API under
  `/api/v4/forward/*` (F5a, implemented: `packages/forward/v4api`). The
  operator CLI `anix-control forward ...` runs on the kernel's state
  directly (F5a). A new UI follows (F5b).
- **Other AnixOps products**: import `sdk/forward` and call
  `ForwardControl`.

Drivers run only inside the Agent and inside test harnesses (the
conformance suite and the netns end-to-end suite, `sdk/forward/e2e`).
There is no `cmd/anixops-forward`.

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
| `ANIXOPS` | `ANIXOPS` (v4.3) | mux with stream-level zero round trip, TLS, QUIC and trusted-link plain carriers, per-identity pinning with the H28 link certificates (`anixops-protocol.md`) | v4.3 |

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
  not taken by another route's listener on the node (this one needs every
  route, so the planner checks it, section 5.2);
- targets: syntax, and the target policy (section 14);
- limits: non-negative; `expires_at` in the future on create.
- size caps (hops, nodes per hop, targets, labels, the encoded route) and
  the health and breaker bounds are proposals in
  `sdk/forward/validate/caps.go`; the defaults (section 7.3, H21) are in
  `sdk/forward/model/defaults.go`.

## 5. Planner

Implemented in F1c: `sdk/forward/planner` (the package documentation is
the reference; this section is the summary).

### 5.1 Contract

```go
// Plan renders every route into one state per node of the inventory. It
// reads nothing but its arguments and returns the same output for the
// same input.
func Plan(routes []*forwardv1.Route, nodes []*forwardv1.NodeInfo, previous Allocations, opts Options) (Result, error)

// PlanRoute is ForwardControl.PlanRoute: one route, only its hops, only on
// the nodes it uses, generation 0 and no state_hash.
func PlanRoute(req *forwardv1.PlanRouteRequest, inventory []*forwardv1.NodeInfo, previous Allocations, opts Options) (*forwardv1.PlanRouteResponse, error)

// Stamp sets state_hash and generation from the previous generations.
func Stamp(states map[string]*forwardv1.NodeForwardState, previous map[string]Generation) map[string]Generation

type Result struct {
    States      map[string]*forwardv1.NodeForwardState // by node_ref, generation 0, no state_hash
    Allocations Allocations                            // (route, hop, node) -> port and mark
    Violations  []RouteViolation                       // validate.Violation with the route's id
    Warnings    []string
}

type Options struct {
    Cluster       string              // names the Agent identities pinned on encrypted links
    ReservedPorts map[string][]uint32 // per node: never allocated, refused by validation
    Taken         Allocations         // held outside this plan: grace period, other routes
    EnableAnixOps, OnCreate bool; Now time.Time // passed to validation
}
```

- **Validation first.** Every route runs through `sdk/forward/validate`
  with the inventory. The planner adds the rules that need every route on a
  node, with codes in the same list (`validate/violation.go`):
  `port_in_use`, `port_exhausted`, `no_port_range`, `mark_exhausted`,
  `no_address`; `Plan` also needs a distinct, non-empty id per route
  (`required`, `duplicate` on `id`). **Any violation refuses the whole
  plan**: no states, no allocations, so Control keeps every node on its
  previous generation and a node never gets half a change. The error
  return is only for a misuse of the planner (`Options.Cluster` missing
  when a route has an encrypted link).
- Control keeps the allocations and the generations in its own tables and
  passes them back on the next plan. The contract's `PortAllocation`
  carries both the port and the mark (`mark = 5`, added in F1c under the
  draft golden policy); `planner.AllocationsFromProto` turns the stored
  list back into `Allocations`, so sticky ports and marks round-trip.
- `PlanRoute` is what the goldens in `contracts/forward/v1` record (section
  13). Its route id may be empty (a preview before the route is stored);
  `previous` holds the route's own allocations and `Options.Taken` every
  other route's.

### 5.2 Port allocation

- The entry listens on `Route.listen.port`; 0 asks the planner for one. The
  entry's nodes share **one** port, the lowest free on all of them, so
  DNS-based entry HA works.
- Every other hop listens on `Hop.port`, or on a port the planner allocates
  per node when it is 0. Nodes of one hop may get different ports; the
  previous hop dials each on its own.
- Allocation runs in three passes, each in route id, hop index and node
  order, so it is deterministic:
  1. **Allocations stick.** A `(route, hop, node)` keeps its previous port
     while that is still legal: inside the node's range, not reserved, not
     taken, and for an explicit port still the one the route asks for.
     Editing a route's targets never moves its relay port.
  2. Explicit ports are claimed. A port already held on the node, by
     another route, another hop of the same route or a route in its grace
     period, is refused (`port_in_use`), so the route that holds a port
     keeps it.
  3. The remaining hops get the lowest port of the node's range that is
     free, not in `Options.ReservedPorts` and not in `Options.Taken`; none
     is `port_exhausted`, no range is `no_port_range`.
- A sticky port that had to move (the range shrank, the port became
  reserved) is reported in `Warnings`. A port is held per node for TCP and
  UDP together.
- A route that is no longer planned holds nothing. A deleted route's ports
  are released after a grace period (proposed 10 minutes) so a late packet
  never reaches a new route: the planner is clock-free, so Control passes
  them in `Options.Taken` until the grace period ends. `Taken` entries of
  a route being planned are ignored, so Control may pass every stored
  allocation.
- **Marks.** Every hop on a node gets a connection mark in 1..4095
  (`planner.MaxMark`), unique on the node and sticky like ports. The
  driver shifts it into its mark mask (section 6.1); `mark_exhausted` when a
  node already runs 4095 hops.
- The planner refuses a port that another listener of ours holds on the
  node. The Agent's preflight and the `LISTEN` probe catch ports held by
  others (section 7.6).

### 5.3 Chain wiring and where limits land

For each hop *i* and each of its nodes the planner emits a `NodeHop`:

- `listen`: `Route.listen` with the entry's port on the entry; the
  allocated port (bound to `dial_address` when one is set) with the route's
  protocol otherwise;
- `ingress`: the link it terminates (`RAW` on the entry); an encrypted link
  without a `server_name` presents the node's identity name
  (`forward-41`);
- `upstreams`: hop *i+1*'s nodes in `node_refs` order, which is also their
  failover `priority` (0, 1, ...), each with its address (`dial_address` or
  the node's first address), allocated port, weight 1, the link to dial
  with (`hops[i+1].ingress`, server name filled in as above) and, for
  encrypted links, the next node's pinned Agent identity
  (`spiffe://anixops/<cluster>/agent/<node>`, `Options.Cluster`); on the
  last hop, the targets in route order with their weight (0 is 1) and
  priority, dialled `RAW`;
- `balance`: `Policy.next_hop` towards nodes, `Policy.target` towards
  targets;
- `health`, `circuit_breaker` and `target_policy` with defaults filled in
  (section 7.3), on every hop;
- `ingress_sources` and `ingress_peers`: every address of the previous
  hop's nodes (in order, once each) and, on an encrypted link, their
  identities, so a relay or exit port is not an open proxy; empty on the
  entry;
- `mark`: the hop's connection mark on the node (section 5.2);
- `paused`: a paused route stays in the states with `paused` set on its
  hops, so its ports, marks, counters and quota survive the pause; the
  drivers drop its traffic (`Route.paused` and `NodeHop.paused` say so in
  the contract).

States list their hops sorted by route id and hop index; `PlanRoute`
answers states sorted by node. An nftables hop that accepts clients of an
address family (its listen address, or the node's addresses) without an
upstream of that family gets a warning, since the driver keeps one DNAT map
per family (section 6.1).

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

Decided by default with F3a (the owner may revisit): global quota across
several entries is Control-authoritative with a local remainder, as above.
F3a implements the Control side: when the entry hop's metered bytes (both
directions, every entry node, every counter epoch; section 11) reach
`quota_bytes`, or `expires_at_unix_ms` has passed (checked every minute),
Control plans the route as paused (`KernelForwardRoute.enforced` is
`quota` or `expired`) without changing the stored route, so its hops,
ports and counters survive; raising the quota or the expiry lifts it at
the next plan. Rendering each entry's local remainder is **deferred**:
until then every entry carries the full `quota_bytes`, so while Control is
down several entries can together pass the quota (each stops at it).
Re-rendering per node on every report would move generations continuously
and needs a throttle of its own.

### 5.4 Generations

- `state_hash` is the lowercase hex SHA-256 of the deterministic protobuf
  encoding of a `NodeForwardState` holding only the node's (sorted) hops.
  Control and the Agent compare hashes, never contents.
- `Stamp` keeps a node's `generation` while its hash is unchanged and
  bumps it by one when the hash changes (1 for a new node, whose empty
  state is a state too). A change to one route bumps only the nodes whose
  state it changes, and re-planning identical input bumps nothing.
  `Plan` answers a state for every node of the inventory, so a node that
  loses its last hop gets a new, empty generation.
- The Agent applies only a newer generation, persists the last applied
  state locally, and reports the generation and hash it runs. Control shows
  a node as converged when they match.

### 5.5 Direct mode

`Policy.direct` is the private-line "direct from entry" policy. `PREFERRED`
renders the targets as the first upstreams of the entry, with their own
priorities, followed by the next hop's nodes with priorities after the
targets' highest, all balanced with `Policy.target`; so the entry dials the
targets while they are healthy and falls back to the chain otherwise.
`FORCED` renders only the targets, and validation refuses it on a route
with later hops (`unused_hops`).

## 6. Drivers

### 6.0 Interface

Implemented in F2a (`sdk/forward/driver`; the package documentation is
normative and `sdk/forward/driver/conformance` checks every rule below).

```go
type Driver interface {
    // Engine is the engine the driver serves; Registry keys drivers by it.
    Engine() forwardv1.Engine
    // Capabilities probes the host: engine version, IPv6, UDP, strategies,
    // link securities, limits support. Unavailable drivers say why.
    Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error)
    // Render turns the engine's hops of one node state into the engine's
    // artifact (nft script, gost config). Pure; golden-tested.
    Render(state *forwardv1.NodeForwardState) (Artifact, error)
    // Apply makes the host run exactly the artifact, atomically, touching
    // only objects the driver owns.
    Apply(ctx context.Context, artifact Artifact) (ApplyResult, error)
    // Observe reads counters, the engine's view of upstream health and
    // the upstreams in rotation.
    Observe(ctx context.Context) (Observation, error)
    // SetUpstreams changes which upstreams of a hop are in rotation
    // (failover) and their weights (least-conn) without a full apply.
    SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []Upstream) error
    // Remove deletes everything the driver owns.
    Remove(ctx context.Context) error
}

```

The shared types (fields abridged): `Artifact` (engine, node, generation,
`state_hash`, the hops it runs, opaque `Content` and its SHA-256 `Digest`),
`ApplyResult` (`Changed`, and the generation, hash and digest now run),
`Observation` (applied identity, `Counters` and `UpstreamHealth` from the
contract, the `Rotation` per hop, the time) and `Upstream` (address, port
and a weight, 0 keeping the rendered one).

The Agent splits a node's state by engine (`Registry.Render`), renders and
applies each part, and runs one health loop that calls `SetUpstreams`. The
rules every driver keeps:

- **Ownership** (H13). A driver touches only objects it owns and marks
  (the nftables driver: `inet anixops_fwd`, its tc handles, its sysctl
  drop-in). An object with its name but not its mark is foreign: Apply
  refuses it with `ErrNotOwned`, Remove leaves it. An artifact that would
  collide with a foreign object (a listen port another table or process
  holds) is `ErrConflict`; neither side changes.
- **Render** is pure and deterministic and reads only its engine's hops, in
  any order. `Content` and `Digest` (SHA-256 of `Content`) exclude the
  generation, `state_hash` and `node_ref`, so a generation bump caused by
  another engine leaves this engine's artifact unchanged. A hop the driver
  cannot run is left out and reported as a `HopError` (`ErrUnsupported`,
  `ErrInvalidState`) inside a `RenderError` returned *with* the artifact of
  the other hops: the Agent applies that and reports the hop errors, so a
  hop that fails is reported while the node's other hops keep running. A
  state without the engine's hops renders an empty artifact, and applying
  it removes everything the driver owns.
- **Apply** is atomic (all of the artifact or the previous state), leaves
  the hops it does not change alone (their objects, connections and
  counter epochs; conformance scenario `apply-leaves-unrelated-hops`, F4c)
  and compares the host with the artifact, not with its memory: applying what
  the host runs is a no-op (`Changed` false; a newer generation is only
  recorded), applying onto partial or damaged owned state repairs it. The
  applied generation, `state_hash` and digest are recorded on the host, so
  a restarted Agent's new driver instance observes them. An older
  generation is `ErrStaleGeneration` (the contract's `FAILED_PRECONDITION`),
  the same generation with another digest `ErrGenerationConflict`.
- **Counters.** Cumulative fields never decrease within a `counter_epoch`.
  The epoch belongs to one hop's counter objects and ends only when they
  are re-created from zero (hop removed and re-added, objects lost, reboot,
  engine restart, Remove then Apply); a generation change or a rewrite of
  the hop keeps epoch and values.
- **SetUpstreams** selects a non-empty subset of a hop's rendered upstreams
  (`ErrInvalidArgument` for an empty or duplicate selection, `ErrNotFound`
  for an unknown hop or upstream) and rewrites map elements or gost nodes
  only: digest, generation and counter epochs stay. The rotation lives on
  the host (it survives a restart and a no-op apply); an Apply that changes
  the host puts every rendered upstream back and the health loop
  re-asserts its selection. Keeping the last upstream when all are down
  (section 7.3) is the health loop's decision, not the driver's.
- **Paused hops** stay applied: their objects and counters remain (so the
  epoch and history survive a pause), they appear in Observe, and the
  driver refuses or drops their traffic.
- **Soft quota** (F4b). A driver whose engine has no byte quota may
  report the `quota` capability by implementing `QuotaEnforcer`: the Agent
  calls `EnforceQuotas` after every Observe and Apply, and a hop whose
  counters in its current epoch reached `quota_bytes` refuses new
  connections as a paused hop does until the quota is raised above them.
  The gost driver does; nftables' quota is exact and in the kernel.
- **Remove** deletes every owned object and nothing else; it is idempotent.
- **Context and concurrency.** A call with a done context returns
  `ctx.Err()` and changes nothing. Every method is safe for concurrent use;
  Apply, SetUpstreams and Remove are serialised and Observe never sees a
  half-applied state.

### 6.1 nftables

Render is implemented in F2b, Apply, Observe, `SetUpstreams`, Remove, the
host probe and the tc limits in F2c (`sdk/forward/driver/nftables`; the
package documentation is normative). The driver runs `nft` and `tc`
through an injectable `Runner`. `nftables.Probe` checks the host once
(`nft --version`, `CAP_NET_ADMIN`, and every kernel feature with `nft -c`
of a snippet inside the driver's own table name, never committed) and fills
the `nftables.Config` that `New` takes, so Render stays a function of its
configuration; it also warns, without changing anything, about another
table's forward chain with a drop policy (Docker's `ip filter FORWARD`).
Minimum versions: nft 0.9.7 and Linux 5.10 (table, counter and set element
comments); tested with nft 1.0.9 (iproute2 6.1, whose `tc -j class show`
prints text, which the driver also reads) on the CI runner and nft 1.1.3 on
Linux 6.12.

**Ownership.** One table, `inet anixops_fwd`, marked by its comment
`anixops-forward-driver v1`. The driver creates, rewrites and deletes only
this table, its own tc handles and its own sysctl drop-in. It never runs
`flush ruleset` and never edits another table.

**Shape.** Render produces one `nft -f` script, one transaction in three
steps: a `table inet anixops_fwd` block declaring the table with its
comment and every object with no elements and no rules; `flush table` and a
`flush set`/`flush map` for each admission set and balancing map (`flush
table` keeps elements, and re-adding a changed interval fails); a second
block adding the elements and rules. Counters, quotas and the `ct count`
sets are never flushed, so they keep their values. A manifest of comment
lines after the header (`# anixops-hop`, `# anixops-upstream`) carries what
Apply needs and the objects do not: every rendered upstream with weight and
priority (failover's backups are in no map), the listener and the
bandwidth. The script itself deletes nothing; Apply adds the deletions and
the recorded state to the same transaction (below). The rules and elements for
fixture `plan-single-hop-nftables-iepl.json` (golden
`contracts/forward/v1/nft/plan-single-hop-nftables-iepl-forward-11.nft`; declarations and flushes
omitted):

```
table inet anixops_fwd {
	map r_01JF1A000000000000000000A1_h0_lb4 {
		type mark : ipv4_addr . inet_service
		flags interval
		elements = {
			0-127 : 10.88.0.20 . 443,
		}
	}
	chain r_01JF1A000000000000000000A1_h0_dnat {
		ct mark set ct mark and 0xf000ffff or 0x00010000
		meta nfproto ipv4 meta l4proto { tcp, udp } dnat ip to numgen inc mod 128 map @r_01JF1A000000000000000000A1_h0_lb4
		drop
	}
	chain r_01JF1A000000000000000000A1_h0_acct {
		ct state new add @r_01JF1A000000000000000000A1_h0_conns { ct mark ct count over 2000 } reject
		quota name "r_01JF1A000000000000000000A1_h0_quota" drop
		ct direction original counter name "r_01JF1A000000000000000000A1_h0_up" meta mark set meta mark and 0xf000fffe or 0x00010000
		ct direction reply counter name "r_01JF1A000000000000000000A1_h0_down" meta mark set meta mark and 0xf000fffe or 0x00010001
	}
	chain prerouting {
		type nat hook prerouting priority dstnat; policy accept;
		meta l4proto { tcp, udp } th dport 30001 fib daddr type local goto r_01JF1A000000000000000000A1_h0_dnat
	}
	chain forward {
		type filter hook forward priority filter; policy accept;
		ct mark and 0x0fff0000 vmap {
			0x00010000 : jump r_01JF1A000000000000000000A1_h0_acct,
		}
	}
	chain postrouting {
		type nat hook postrouting priority srcnat; policy accept;
		ct mark and 0x0fff0000 {
			0x00010000,
		} masquerade
	}
}
```

- **Names.** Every object of a hop is `r_<route id>_h<hop index>_<suffix>`:
  counters `_up` and `_down`, quota `_quota`, `ct count` set `_conns`,
  admission sets `_src4`/`_src6`, balancing maps `_lb4`/`_lb6`, chains
  `_dnat` and `_acct`. A route id that is not 1 to 64 ASCII letters and
  digits is rejected, never escaped; only checked literals (these names,
  numbers, addresses parsed by `net/netip`) reach the script. Labels, host
  names, node references and the state's identity never do.
- **inet family**, IPv4 and IPv6. A listener DNATs a v4 client to v4
  upstreams and a v6 client to v6 upstreams (one map per family); a family
  without upstreams is dropped in the `_dnat` chain so it never reaches
  local input on the listen port. One protocol is matched with `tcp dport` or
  `udp dport` (nft accepts `meta l4proto tcp th dport` but cannot read it
  back), both with `meta l4proto { tcp, udp } th dport`. A listen address
  pins the listener to that address, otherwise `fib daddr type local`.
- **Admission.** Relay and exit hops must carry `ingress_sources` (an
  open relay is rejected); their `_dnat` chain drops every other source.
- **Counters per direction.** Named counters in the filter `forward` chain,
  reached through a verdict map on the connection mark and split by
  `ct direction original|reply`. This fixes v4.1's undercounting (counters
  in the nat hook see only a connection's first packet). Named counters
  survive rule rewrites.
- **Load balancing maps.** Each map has a fixed number of slots
  (`Config.Slots`, default 128, enough for 64 targets plus 16 next-hop
  nodes) keyed by `numgen inc` (round robin, failover), `numgen random`
  (random, least-conn) or `jhash ip saddr`/`jhash ip6 saddr` (IP hash),
  modulo the slots. The maps are `type mark : ipv4_addr . inet_service`
  (the `typeof numgen ... : ip daddr . th dport` form cannot be read back).
  Slots go to upstreams by weight, at least one each: interleaved in smooth
  weighted round-robin order for `numgen inc`, one run per upstream
  otherwise. Failover maps only the upstreams of the best priority. Since
  the modulus never changes, failover and health only rewrite map elements
  (`SetUpstreams`), never rules. Least-conn is weighted random until the
  Agent re-weights it (L1).
- **Named quota** for `quota_bytes`, over both directions. Re-declaring a
  quota updates its limit in place and keeps its usage.
- **`ct count`** in a dynamic set keyed by the hop's mark for `max_conns`;
  excess new connections are rejected.
- **Bandwidth: tc HTB + ct mark.** For a hop with `bandwidth_bps` the
  `_acct` chain copies the hop's mark to the packet mark, with
  `Config.DirectionBit` (default `0x1`) on reply packets, keeping the other
  packet mark bits. On each egress interface the driver owns one HTB qdisc
  handle (`Config.TCHandle`, default `af00:`) and, per rate-limited hop,
  one class per direction at `bandwidth_bps` (minor `2*mark` up,
  `2*mark+1` down), selected by a `fw` filter on the mark under
  `MarkMask|DirectionBit`. Both directions leave the node as egress, so no
  ifb is needed. The interfaces are `Config.LimitInterfaces`; without one
  the probe turns `bandwidth_limit` off. A foreign root qdisc (another
  non-zero handle) on one of them makes an artifact with limited hops
  `ErrConflict`; the kernel's default root qdisc is replaced and comes back
  when the driver deletes its own.
- **Marks.** The driver owns a mark mask (`0x0fff0000` by default: 4095
  hops per node; configurable per node to avoid other mark users such as
  Docker, WireGuard and policy routing, H13). `NodeHop.mark` is an index
  from the planner, 1 up to the mask's width, shifted into the mask; the
  other connection mark bits are kept. Hops that share a mark or an
  overlapping listener are all rejected, whatever their order.
- **Paused hops** keep every object; their `_dnat` chain drops new
  connections and their `_acct` chain drops established ones, uncounted.
- **Flowtable** (optional, off by default, not rendered yet): offloaded
  flows skip the forward chain, so it is allowed only on routes without
  bandwidth or quota limits, and counters then come from conntrack
  accounting.
- **MSS clamping** on encapsulating egress interfaces
  (`Config.MSSClampInterfaces`): SYNs of the driver's connections leaving
  them get `tcp option maxseg size set rt mtu`. `ip_forward` and IPv6
  forwarding via a sysctl drop-in written at install.
- **DNS targets.** Render takes IP literals only and rejects a name
  (`ErrUnsupported`). The Agent resolves target names (honouring TTL, at
  most every 60 s), re-checks the target policy on every answer, and
  rewrites the maps; Render re-checks literal targets against the target
  policy too.
- **Atomic apply** (F2c). Apply reads `nft -j list table inet
  anixops_fwd`; a table of that name without the ownership comment is
  `ErrNotOwned` before anything runs (the script's `flush table` would
  empty it). It compares the host with the artifact: the recorded digest,
  the table's fingerprint against the seal recorded after the last change,
  and the tc objects. A match is a no-op that at most records a newer
  generation. Otherwise it refuses foreign DNAT or redirect rules on the
  artifact's listen ports (`ErrConflict`, read from `nft -j list ruleset`),
  checks the transaction with `nft -c -f`, adds the tc qdisc and classes it
  needs, and runs one `nft -f` transaction: counter declarations carrying a
  new epoch nonce, the rendered script, `delete` of every object the script
  does not declare (removed hops; their last counters go to the
  `WithRetiredCounters` hook), and the state document. If nft refuses it,
  the tc additions are undone, so the host keeps its previous state; after
  it, the seal is recorded and tc classes no longer needed are deleted.
- **State on the host** (F2c). Nothing lives in the driver's memory, so a
  restarted Agent observes and guards what it applied. Set `anixops_state`
  holds the state document (node_ref, generation, `state_hash`, digest, and
  per hop the strategy, rendered upstreams and current rotation) as
  base64url JSON in 120-character element comments (nft allows 128),
  flushed and re-added in the same transaction as the rules. Set
  `anixops_seal` holds the SHA-256 of the normalized listing (handles,
  counter values, quota usage, dynamic and state elements left out), so a
  damaged table is detected and repaired by the next Apply. The counter
  epoch is the nonce of the hop's counter comments: nft keeps the comment
  of an existing counter, so the epoch ends exactly when a counter is
  re-created. `active_conns` and `total_conns` are not counted (0).
- **Failover** (F2c). `SetUpstreams` rewrites the hop's map elements from
  the selected upstreams with Render's slot layout (failover keeps the best
  priority among them), records the rotation in the state document in the
  same transaction and re-seals; selecting every upstream with weight 0
  restores the rendered elements.
- **Persistence.** nftables rules do not survive a reboot. The Agent
  re-applies its last applied state at start, before it connects to
  Control. Counters then start a new `counter_epoch`.
- **Goldens.** `contracts/forward/v1/nft` holds, per case, the input
  (`<case>.state.json`), the script (`<case>.nft`) and the rejected hops
  (`<case>.errors.txt`). The `plan-<fixture>-<node>` cases are every node
  state with an nftables hop in the planner goldens, and
  `planner-single-hop-nftables-iepl-paused-forward-11` is a live planner run
  of a paused route; they must render without hop errors. `go test ./forward/driver/nftables -update` rewrites them. The
  tests run `nft -c -f` on every golden and conformance state where nft and
  `CAP_NET_ADMIN` are available (in a fresh network namespace as root); CI
  installs nftables and runs them under sudo. `FuzzRender` checks that any
  state renders without panic into the script grammar and that Apply can
  read its manifest.
- **Real-kernel tests** (F2c). With `ANIXOPS_NFT_E2E=1` as root, the
  `TestNetns` tests run the whole conformance suite against the real driver
  (every optional `Env` interface, traffic from a client namespace over a
  veth pair) and the tc, quota-usage, rollback, foreign-qdisc and probe
  tests, each in a throwaway namespace (`ip netns add`); the host's own
  ruleset and qdiscs are never touched. CI runs them under sudo in Backend
  Tests shard 1 (H14). Unprivileged replay tests
  (`testdata/replay`, recorded with `ANIXOPS_NFT_RECORD=1`) check the exact
  command sequence of an apply lifecycle and of the refusals. The
  multi-namespace suite (F2d, section 13) runs the driver behind the
  planner, node by node, with real traffic through several hops.

### 6.2 gost

Render, Capabilities, the host probe and the process management of Apply
and Remove are implemented in F4a (`sdk/forward/driver/gost`; the package
documentation is normative); F4b (implemented) adds gost's web API: Apply
changes upstreams, sources, pauses, limits and quotas without a reload,
`SetUpstreams` fails over in place, Observe reads every service's
statistics, and the driver keeps a soft quota. F4c (implemented) makes
structural changes through the web API too, service by service: adding,
changing or removing a route re-creates no other route's services. gost v3 (MIT; the licence ships in the release archive
and was checked) replaces the NodeX dependency. The Agent manages it;
Control never talks to gost or NodeX. The SDK does not import gost: the
driver writes gost's configuration and runs the unmodified binary.

- **Binary (H20, decided).** One gost v3 release per Agent release, shipped
  in the Agent's package with its checksum and upgraded only with the
  Agent: `gost.PinnedVersion` 3.2.6, the release ci.yml's `GOST_VERSION`
  downloads and checks by SHA-256 (a test keeps the two equal).
  `gost.Probe` runs `gost -V` and `ss -V` and checks the unit: gost or ss
  missing, a gost that is not v3 or is older than 3.2, or a missing or
  foreign unit make the driver unavailable with the reason; another 3.x
  release is a warning. Without the link certificate files the driver
  carries RAW links only.
- **Process (H20).** `anixops-gost.service` is a unit of its own, so
  restarting or upgrading the Agent keeps forwarding. The installer writes
  it from `gost.UnitFile` (golden
  `contracts/forward/v1/gost/anixops-gost.service`), enables it and lets the
  Agent start, reload and stop it (a polkit rule); the driver
  (`SystemdSupervisor`, systemctl through its runner) does only that, and
  refuses a unit of that name whose Description lacks its mark
  (`ErrNotOwned`). gost runs as its own user `anixops-gost` with
  `CAP_NET_BIND_SERVICE` only: it forwards sockets and needs no
  `CAP_NET_ADMIN`, which is the Agent's (H13). Sandbox: `NoNewPrivileges`,
  `ProtectSystem=strict` (everything read only but its
  `RuntimeDirectory`), `ProtectHome`, `PrivateTmp`, `PrivateDevices`, the
  kernel, cgroup, clock and hostname protections, `ProtectProc=invisible`,
  `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`, `RestrictNamespaces`,
  `MemoryDenyWriteExecute`, `SystemCallFilter=@system-service
  ~@privileged`, and `ConditionPathExists` on the configuration, so it
  never starts without one. `ExecReload` sends SIGHUP. Tests and
  containers use `ProcessSupervisor`, gost as a child process.
- **Files.** `Config.Dir` (`/var/lib/anixops-gost`, owned by the Agent,
  group `anixops-gost`, mode 0750) holds `gost.json`, the configuration
  gost runs, `state.json`, the driver's record (its ownership mark, node,
  generation, `state_hash`, digest and the applied hops; written before the
  first configuration and after every apply that succeeded), and `tls/`,
  the link certificate (the installer gives the gost user read access;
  `Probe` only sees that the files exist). gost's metrics socket is in its
  `RuntimeDirectory` (`/run/anixops-gost`), with its web API's socket
  (`api.sock`, F4b); the Agent is in the unit's group. A configuration without the driver's state file, or a state file
  without its mark, is foreign: Apply refuses it, Remove leaves it.
- **Configuration: JSON.** gost loads JSON and YAML alike; `encoding/json`
  writes typed values deterministically, does all the quoting (no name or
  path can inject a key) and keeps the SDK free of a YAML dependency. A
  top-level `anixops` member, which gost ignores, is the manifest Apply
  reads. Names come from the route id only (1 to 64 letters and digits,
  rejected otherwise). Per `NodeHop`:
  - services `r<route>-h<hop>-<listener>`: a `RAW` ingress listens with
    `tcp` and `udp` listeners (both for `TCP_UDP`) whose handlers forward
    the client's bytes; every other ingress is one carrier listener
    (`tls`, `mtls` for TLS with mux, `wss`, `mwss`, `quic`, `grpc`, `mtcp`
    for RAW with mux) whose `relay` handler carries TCP and UDP. QUIC and
    gRPC multiplex natively. The relay handler and connector run with
    `nodelay` (F4c): the relay request and its answer cross the link when
    the connection is dialled, not with the client's first bytes, so
    protocols whose server speaks first (SSH, SMTP, databases) work over
    relayed links (the mixed-engine suite found them hanging). A client's
    half-close (FIN with the reverse direction still open) arrives as a
    full close over relayed links, since gost cannot half-close a TLS or
    mux stream; RAW links keep it. UDP listeners keep a client's session for
    60 s after its last datagram. Both ends of a mux or QUIC link render
    keepalives: one every 10 s, the carrier closed after 30 s without data
    (`mux.keepaliveInterval`/`mux.keepaliveTimeout`; QUIC `keepAlive`,
    `ttl`, `maxIdleTimeout`), gost's defaults written out, so a carrier
    whose peer vanished without a close is dropped and dialled again.
    Every service keeps gost's statistics (`enableStats`, F4b);
  - upstreams: the nodes of a top-level gost hop `r<route>-h<hop>` (F4b),
    which the service names, so it can be replaced alone. `RAW` upstreams
    are dialled directly: the service's forwarder names the hop. Relayed
    upstreams (an encrypted or multiplexed next hop) carry gost's relay
    connector and the link's dialer; the service dials through chain
    `r<route>-h<hop>`, which names the hop, and the forwarder holds one
    placeholder node, since the next node's relay handler forwards to its
    own upstreams whatever address it is sent. A hop mixing both kinds
    (direct mode `PREFERRED` with an encrypted next hop) is
    `ErrUnsupported`;
  - the hop's selector from `balance` (section 7.1): `round`, `rand`, `hash` on
    the client address, `fifo` over the upstreams sorted by priority for
    failover (ties in route order), `rand` for least connections.
    `rand` reads the weights; `round` and `hash` ignore weights, so a hop
    with unequal weights lists each upstream once per entry in smooth
    weighted round-robin order (at most about 128 entries). The circuit
    breaker is the selector's fail filter: `maxFails` is
    `failure_threshold`, `failTimeout` is `open_ms` (3 and 30 s by
    default, H21), and an upstream comes back after one trial succeeds
    (counted per gost node: an upstream listed w times is skipped once each
    of its entries failed);
  - an admission on every hop (F4b): a whitelist of `ingress_sources`
    (required on relay and exit hops, so no relay is open), an empty
    blacklist on an entry without sources;
  - a traffic limiter for `bandwidth_bps` (the hop's services together, in
    each direction) and a connection limiter for `max_conns`;
  - a paused hop keeps its services and admits nobody: new connections are
    refused, established ones run until they close (nftables drops them;
    gost can close a listener but not the connections it accepted, so no
    API call closes them, F4b);
  - gost's web API on a unix socket in the runtime directory, without
    authentication (F4b, below);
  - metrics on another unix socket, under a path named by a hash of the
    configuration's structure (services, chains, log, API), and warnings
    only in gost's log.

  Services, chains, the log and the API are the configuration's
  structure: a reload makes gost re-create every service. Hops, admissions
  and limiters are hot objects (F4b): gost resolves them by name at every
  connection, so the driver replaces them through the web API and no
  service, listener, established connection, UDP session, mux carrier or
  counter is touched. Services and chains are created and deleted through
  the web API too (F4c), one by one; only the log, the API and the
  metrics address need a reload.
- **Apply.** It compares the host with the artifact: the recorded digest,
  the configuration file's bytes and gost running it (it serves the
  configuration's metrics path, or the one recorded in `state.json` as
  loaded by the last start or reload, holds every listener, and runs
  exactly the configuration's services and hops). All equal is a no-op
  that at most records a newer generation. Otherwise it reads the
  listening sockets with `ss` (no privilege needed) and refuses a listener
  whose port a socket holds that the running, applied configuration does
  not declare (`ErrConflict`, before anything changes).

  When gost runs the recorded configuration, its web API answers and the
  artifact keeps the globals (log, API, metrics address), Apply goes
  through the web API (F4c): it writes the configuration (a temporary file
  and a rename, so a restart loads exactly the new state), then, object by
  object, deletes the services that were removed or changed, creates or
  replaces (`POST`, `PUT`) the admissions, limiters and chains that were
  added or changed and every hop (which also restores every upstream
  `SetUpstreams` took out of rotation), creates the added and changed
  services, and deletes the chains, hops, admissions and limiters no
  longer rendered. A changed service is deleted and created again, never
  replaced with `PUT` (gost's `PUT` closes the old service before it
  builds the new one and keeps it registered, closed, when that fails).
  Every service that did not change keeps its listener, established
  connections, UDP sessions, mux carriers and statistics: adding,
  changing or removing a route re-creates nothing of the node's other
  routes (tested end to end with a held TCP connection and UDP session).
  Mux and QUIC listeners are the exception (see "Mux and QUIC carriers"
  below): an apply that would delete or re-create a running `mtcp`,
  `mtls`, `mwss` or `quic` service restarts gost instead; adding one goes
  through the web API. gost answers a refusal before it changes anything; when it refuses one
  call (a port taken meanwhile, a certificate it cannot read), Apply puts
  back, through the web API, exactly the objects it had changed as the
  previous configuration has them, writes the previous file back and
  fails; only when that fails too does it restart gost on the previous
  configuration (or stop it after a first apply).

  Otherwise (gost stopped, its web API silent, a configuration file that
  is not the recorded one, other globals, or a mux or QUIC service to
  delete or re-create) Apply writes the configuration and reloads gost
  with SIGHUP (or starts it, or restarts it when gost already serves that
  structure, since a reload would not move the metrics path, or when the
  configuration gost loaded has a mux or QUIC listener or cannot be read)
  and waits until gost serves the new metrics path with every listener
  bound. A reload keeps the process, so established TCP connections
  survive it (tested); it re-creates every service of the node, so UDP
  sessions may restart and every hop's counters start a new epoch (a
  restart ends every connection as well) (the `WithRetiredCounters` hook gets every
  hop's counters read just before, when the web API answers). gost's
  reload is not atomic (a listener it cannot bind closes the old services
  first), so when the new configuration is not served within
  `Config.ReadyTimeout` Apply writes the previous one back and restarts
  gost on it, or stops gost after a first apply, and fails. An empty
  artifact stops gost and deletes the files.

  What an established connection of a deleted or re-created service moves
  afterwards goes to the closed service's statistics, which nothing reads
  any more: like a reload, but limited to the hops Apply changed.
- **Mux and QUIC carriers.** A mux (`mtcp`, `mtls`, `mwss`) or QUIC
  listener accepts long-lived carriers on which the previous hop's dialer
  opens a stream per connection. gost 3.2.6 ties them to its process, not
  to the service (measured): deleting the service through the web API, or
  a SIGHUP reload re-creating it, closes the listening socket only; the
  accepted carriers stay up and keep answering keepalives, so the peer
  keeps opening streams on them that nothing accepts and every new
  connection through it hangs (over 40 s, with no end, in
  `TestNetnsMuxRestart` before the rule); a QUIC listener keeps its UDP
  port bound while its connections live, so the new service cannot bind.
  The driver therefore never deletes, re-creates or reloads such a service
  on a running gost: Apply (per-service sync and the reload fallback) and
  `ReloadCredentials` restart gost, whose exit closes every carrier
  (`Loads` counted, every hop's epoch ends, `WithRetiredCounters` gets
  every hop's counters when the web API answers), and peers dial new ones:
  over mux new connections pass within about a second (the peer sees the
  TCP close), over QUIC at the peer's idle timeout (30 s; gost sends no
  stateless reset). The price is that such a structural change ends every
  established connection of the node; hot changes, changes to other
  services and added services do not restart it, and nodes without a mux
  or QUIC listener keep the SIGHUP fallback, which keeps established TCP
  connections.
- **TLS and peer identities.** Links between nodes are mutual TLS with the
  node's link certificate (`Config.LinkCert`, `LinkKey`, `LinkCA`). The
  listener requires a client certificate `LinkCA` signed; the dialler
  verifies the next node's certificate for the link's server name (default:
  the node's identity name, `forward-41`) under `LinkCA` and presents its
  own. gost cannot match a SPIFFE URI, so `Upstream.peer_identity` is pinned
  through that server name and `ingress_peers` is required on encrypted
  ingress but not matched per identity; the admission of `ingress_sources`
  narrows the listener to the previous hop's addresses. The link
  certificates therefore need the node's identity name as a DNS name, both
  serverAuth and clientAuth, and a CA that signs forward nodes' link
  certificates only; today's Agent certificates (client auth, URI name,
  the CA that also signs module and kernel certificates) do not qualify,
  and gost must not hold the Agent's Control key. Decided (H28): a
  dedicated forward link CA issues them, requested and renewed alongside
  the Agent certificate. No key material travels in the state.
- **Link certificates (H28, Control side implemented).** Control's forward
  link CA (`internal/agentpki/link.go`) is a self-signed ECDSA P-256 root,
  separate from the CA of modules, the kernel and Agents, name-constrained
  to `spiffe://anixops` URIs, its key sealed with `module_runtime.ca_kek`
  under additional data of its own (`v4_kernel_forward_link_ca`). It exists
  wherever the built-in CA does (created at startup; an external PKI has no
  key in the kernel and issues none). `AgentEnrollment.IssueLinkCertificate`
  issues a node a link certificate for a key the Agent generates for it
  alone (never the Agent key, which Control refuses): CN and the only DNS
  name the node's identity name (`forward-41`, the default `server_name`),
  the only URI its SPIFFE ID (the state's `peer_identity` and
  `ingress_peers`), serverAuth and clientAuth, 7 days, renewed at two
  thirds. Only an Agent certificate authenticates the call (never a node
  credential), only for an enabled node whose Agent negotiated forward.v1,
  and the CSR may name nothing but the node's DNS name and SPIFFE ID.
  Every certificate is recorded in `v4_kernel_forward_link_certificate`
  and revoked with the node's Agent credentials (`agentpki.RevokeNode`:
  disable, credential replacement, deletion, `RetireNode`). The answer and
  `GetLinkTrustBundle` carry the link trust bundle. Rotation
  (`anix-control agent link-ca rotate`) mirrors the module CA's with the
  link lifetime as overlap: the next CA joins the bundle at once and signs
  only 7 days later, by when every node renewing at two thirds (4.7 days)
  has fetched it; a retired CA stays in the bundle for 7 days more. The
  planner needs nothing new: `server_name` defaults to the node's ref and
  `peer_identity` is its SPIFFE ID, both names the link certificate
  carries. An operator-chosen `server_name` (a CDN name on WSS) is not in
  the exit's link certificate, so the dialler's verification fails; it
  stays unsupported. The Agent's half (F3b: the key, the files under
  `/var/lib/anixops-gost/tls`, the reload) is specified in
  `sdk/api/agent/v1/PROTOCOL.md`, "Forward link certificates".
- **Certificate reload.** gost 3.2.6 has no certificate hot-reload: it
  reads a certificate only when it creates the service (listener) or hop
  (dialer) that uses it, and a renewal leaves the configuration file, which
  names the files, unchanged. After every renewal the Agent calls
  `(*gost.Driver).ReloadCredentials`, serialised with Apply: it checks
  that the files load, then re-creates through the web API every service
  with an encrypted listener (TLS, WSS, gRPC) and replaces every hop with
  encrypted dialers (keeping a `SetUpstreams` selection). The re-created
  services' hops start a new `counter_epoch`, recorded in `state.json` as
  Apply's per-service changes are, and `WithRetiredCounters` gets their
  last counters; every other hop keeps its epoch, and established
  connections run on with the old certificate. A mux (mtls, mwss) or QUIC
  listener cannot be re-created in place (measured: a deleted mux listener
  leaves the carriers it accepted running, so a peer's new streams on them
  are never accepted; a QUIC listener keeps its UDP port while its
  connections live), so a node with one restarts gost instead, as when the
  web API does not answer: every connection ends, every hop starts a new
  epoch and hands its counters over, and a peer's QUIC carrier recovers
  at its idle timeout (30 s). A supervisor reload outside the driver would
  leave the epoch unmoved. Apply follows the same rule ("Mux and QUIC
  carriers").
- **Web API (F4b).** gost's web API listens on `api.sock` in the runtime
  directory, without authentication: the socket's permissions are its only
  key. Under the unit, gost creates it with `UMask=0007` in its
  `RuntimeDirectory` of mode 0750 (owner `anixops-gost`), so the gost user
  and its group, which the Agent is in, reach it and nobody else; no key
  or password exists to leak. The driver uses `GET /config` (the running
  configuration, every service with its creation time and statistics),
  `PUT /config/{hops,admissions,limiters,climiters}/<name>` (one hot
  object) and, for structural changes (F4c), `POST /config/<kind>` and
  `DELETE /config/<kind>/<name>` for services, chains and the hot objects. The API decodes bodies with `encoding/json`, so a duration is
  an integer of nanoseconds there.
- **Counters (F4b).** Observe sums, per hop, its services' statistics from
  `GET /config`: bounded, one entry per service. gost's Prometheus metrics
  label every series with the client address, an unbounded set on a public
  entry, so the driver does not read them. `up_bytes` is what the services
  read from their clients and `down_bytes` what they wrote back (payload,
  or the carrier's bytes on an encrypted or multiplexed ingress; the
  end-to-end suite finds a gost entry's counters equal to the payload
  moved); `total_conns` and `active_conns` are the accepted connections
  (UDP sessions and the streams inside a mux carrier are not); gost counts
  no packets, so packets are 0. The `counter_epoch` names the statistics
  objects: a hash of the gost instance (the unit's InvocationID), the
  starts and reloads Apply and `ReloadCredentials` made (recorded in
  `state.json`), a per-hop sequence number Apply and `ReloadCredentials`
  record in `state.json` whenever they delete or create one of the hop's
  services through the web API (gost's creation
  times have a resolution of one second, too coarse to tell two
  re-creations apart), and the creation time of each of the hop's
  services. A start, a reload, or creating or deleting one of the hop's
  services ends it, for that hop only; hot changes, structural changes of
  other hops, `SetUpstreams` and the soft quota keep it. The
  `WithRetiredCounters` hook gets the last counters of every hop whose
  epoch an Apply or a `ReloadCredentials` ended, read just before its
  first change. While gost does not run every hop reports 0 in the epoch `stopped`.
  gost's fail marking is not exposed, so `Health` stays empty and the
  Agent's checks are the source of truth.
- **Failover (F4b).** `SetUpstreams` replaces the running hop through the
  API with the selected upstreams (the nodes Render would give them, with
  their weights) and records the selection in the hop's metadata, where
  Observe reads it: the rotation lives in the running gost, so it survives
  an Agent restart and a no-op Apply, and a start, a reload or a changing
  Apply restores every rendered upstream (the health loop re-asserts its
  selection, also after gost restarted). No file, generation, digest or
  counter epoch changes, and established flows keep their upstream.
- **Quota.** gost has no byte quota. With `Config.SoftQuota` (default on,
  F4b) the driver reports `quota` true, renders `quota_bytes` into its
  manifest and keeps it softly: `EnforceQuotas` (the optional
  `driver.QuotaEnforcer`, which the Agent calls after every Observe and
  Apply) closes the admission of a hop whose up and down bytes in the
  current counter epoch reached the quota, as a pause does, and opens it
  again once the quota was raised above them. It holds within one
  observation interval; established connections run until they close; a
  new counter epoch starts it again, as nftables' named quota restarts
  when its object is re-created. Control's ledger over every epoch and
  entry stays the authority (section 5.3). Entries that need an exact
  quota should be `NFTABLES`. Health checks are the Agent's loop
  (section 7.3); target names are rejected like nftables' (the Agent
  resolves them).
- **Least connections (L1).** `ActiveConns` answers the host's established
  TCP connections and connected UDP sockets per upstream address and port
  (`ss`, no privilege), the gost source of `sdk/forward/leastconn`
  (section 7.1): exact for RAW upstreams, carriers on an encrypted or
  multiplexed link, and shared by every hop with the same upstream.
- **Goldens.** `contracts/forward/v1/gost` holds, per case, the input
  (`<case>.state.json`), the configuration (`<case>.json`) and the rejected
  hops (`<case>.errors.txt`): `plan-<fixture>-<node>` for every planner
  golden state with a gost hop (the exits of
  `plan-nft-entry-gost-relay-exit-failover` keep the error for their target
  name), `planner-<variant>-<node>` from live planner runs of that route
  with IP targets (TLS with and without mux, WSS, QUIC, gRPC, RAW with
  mux, TCP and UDP over TLS, a paused weighted round robin, a gost entry
  with limits), synthetic cases, and the unit file. `go -C sdk test
  ./forward/driver/gost -run Goldens -update` rewrites them.
- **Tests.** Without privileges: the render scenarios of the conformance
  suite (with and without a link certificate), the whole suite against a
  simulated gost and host (which serves the web API and counts simulated
  traffic), and unit tests. With a gost binary (`ANIXOPS_GOST_BIN` or
  `PATH`), gost parses every golden. With `ANIXOPS_GOST_E2E=1` as root,
  gost runs in a throwaway network namespace per test: the whole
  conformance suite with no scenario skipped and traffic through the hops
  (`TrafficSource`), every golden applied and served with certificates of
  a test CA, TCP and UDP through a mutual-TLS mux relay to an exit (a hot
  change and adding another route through the web API keep established
  flows, the UDP session and the counter epoch, without a reload; the
  reload fallback keeps an established TCP connection and starts a new
  epoch; a paused hop refuses new connections; the exit refuses a
  client without a certificate), failover through the API with a held TCP
  connection and UDP session, the soft quota and a pause
  (`TestNetnsHotChanges`), least-connections re-weighting
  (`TestNetnsLeastConn`), and link certificates renewed in place over TLS,
  WSS, gRPC, QUIC and TLS-with-mux links (`TestNetnsReloadCredentials`:
  the new certificate served, the epochs and retired counters, traffic
  resuming, and over TLS an established connection kept), and a relay
  dialling an exit over TLS with mux whose mux service changes
  structurally, through the web API path and the fallback
  (`TestNetnsMuxRestart`: the exit restarts, the relay does not, and new
  connections through the relay pass again well within the 30 s keepalive
  timeout plus slack, measured about 0.1 s). CI runs them under sudo in Backend Tests shard 1
  with the pinned release, after the nftables driver's real-kernel tests:
  every change runs them, as for nftables. The netns conformance Env fails
  an apply through the web API by refusing its first service creation
  (`SetAPIFault`, test-only), so `apply-failure-keeps-previous` runs the
  API rollback. The Forward Netns E2E job runs a gost entry too (exact
  payload counters over TCP and UDP, IPv4 and IPv6, and failover) and the
  mixed-engine chains (F4c, section 13).

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
the engine. The secure-transport design is a separate document,
[`anixops-protocol.md`](anixops-protocol.md) (H22): TLS 1.3 mutual
authentication with the H28 link certificates and per-identity pinning of
`peer_identity` and `ingress_peers`, multiplexing with stream-level zero
round trip (no TLS early data), TLS-over-TCP, QUIC and trusted-link
plaintext carriers, native UDP with UDP-over-stream fallback, half-close,
PROXY v2 toward targets, and an `anixops-relay` unit of its own. By the
owner's scoping decision (2026-10-04) camouflage is not part of it: its
section 8 is reserved for the owner.

### 6.5 Capability matrix (v4.2 target)

| Feature | nftables | gost | Ansible fallback |
|---|---|---|---|
| TCP / UDP | yes / yes | yes / yes | yes / yes |
| IPv6 | yes | yes | yes |
| Encrypted links | no | TLS, WSS, QUIC, gRPC, mux | no |
| Round robin, random, IP hash, failover | yes | yes | yes, slow failover |
| Least connections | approximate (re-weighting; the Agent supplies conntrack counts) | approximate (re-weighting from socket counts) | no |
| Bandwidth limit | tc HTB | traffic limiter | tc HTB |
| Byte quota | exact (named quota) | soft (`EnforceQuotas` closes the hop's admission within an observation interval) | exact |
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
| `LEAST_CONN` | fewest live connections | the Agent re-weights the random map every 10 s from conntrack counts per upstream (a Source the Agent supplies) | the Agent re-weights the gost hop every 10 s from the established sockets per upstream (`ActiveConns`) |
| `FAILOVER` | the healthy upstream with the lowest priority | one-element map | `fifo` |

Neither engine has a native least-connections scheduler; the approximation
is documented in the UI. L1 (implemented, `sdk/forward/leastconn`): every
`DefaultLeastConnReweight` (10 s, H21) a `Reweighter` reads the live
connections per upstream address and port from a `Source` once and, for
every `LEAST_CONN` hop the health loop hands it (only the upstreams the
loop keeps in rotation, with their rendered weights), sets through
`SetUpstreams` each weight to the rendered weight divided by the
upstream's connections plus one, scaled so the largest is 100 and none is
below 1; an idle hop keeps its rendered weights, and a weighting equal to
the driver's reported rotation is not set again. Sources: the gost
driver's `ActiveConns` (`ss`: exact for RAW upstreams, carriers on an
encrypted or multiplexed link, shared by every hop with the same
upstream); for nftables the forwarded connections are conntrack entries,
which the standard-library-only SDK cannot read, so the Agent (F3b)
supplies a `Source` from conntrack over netlink (the original direction's
destination after DNAT), and until it does, `LEAST_CONN` on nftables
stays weighted random by the rendered weights. The fake driver answers
`Host.SetUpstreamConns`.

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
pointed at the healthy entries. This is the one failover that needs
Control, and it changes DNS only, so entries keep serving clients that
still resolve them.

L2 (implemented): a route is bound to a name in a DNS provider's zone
(`v4_kernel_forward_dns_binding`), in one of two modes:

- **DDNS:** Control writes the A/AAAA records of the route's
  `entry_hostname` itself, which must be the binding's `record_name`.
- **CNAME:** Control writes the A/AAAA records of a name it manages
  (`record_name`, for example `r1.ha.example.net`), and the operator points
  `entry_hostname` at it with a CNAME. The route's DNS status answers the
  name as `cname_target`.

**Providers** (H21; `internal/forwardddns`, standard library only, signing
written by hand):

| Kind | API | Credentials |
|---|---|---|
| Cloudflare | API v4, Bearer token | `api_token` |
| Alibaba Cloud DNS | Alidns 2015-01-09, signature V3 (`ACS3-HMAC-SHA256`) | `access_key_id`, `access_key_secret` |
| DNSPod | Tencent Cloud API 3.0, `dnspod` 2021-03-23 (`TC3-HMAC-SHA256`) | `secret_id`, `secret_key` |
| Huawei Cloud DNS | DNS API v2, AK/SK (`SDK-HMAC-SHA256`) | `access_key`, `secret_key` |
| Webhook | HTTPS POST of JSON, `X-AnixOps-Signature: sha256=HMAC(secret, timestamp "." body)` | `secret` |

- A provider sets a whole record set (one name and type). It adds the new
  values before it deletes the old ones, so the name never resolves to
  nothing while it changes. An `endpoint` setting overrides the API host
  (a regional endpoint); a webhook takes `url`.
- The signatures are pinned to the vendors' published worked examples, and
  each provider's calls run against a test server that checks the
  signature on the wire. No test reaches a real provider.
- **Credentials** (`v4_kernel_forward_dns_provider`) are sealed with
  AES-256-GCM under `module_runtime.ca_kek`, with additional data naming
  the provider row, so neither a CA key nor another provider's credentials
  open them. Without `ca_kek` a provider is refused
  (`secret_store_unavailable`). No answer, log line, metric label or audit
  entry carries a credential; on update a credential left out, or sent as
  `********`, keeps its stored value.

**Controller** (`kernelforward.EntryHA`, the singleton worker, every 10 s):

- An entry node is **healthy** when:
  - it is in the inventory with a public address of a published family
    (its settings' addresses, else its host when that is an IP address;
    private, loopback and link-local addresses are never published);
  - its latest report arrived within 150 s;
  - its Agent is online: a session in this process, or an Agent Control
    stream seen within 180 s (`v4_kernel_agent_transport`) by any process;
  - its report has no hop error on the route's entry hop;
  - not every upstream of the entry hop is unhealthy or open.
- A node whose report is behind its desired generation, or not applied, is
  *converging*: neither healthy nor unhealthy, so a replan does not pull
  it out.
- **Hysteresis:** a node leaves rotation after 3 unhealthy evaluations in
  a row and rejoins after 3 healthy ones (about 30 s each way). Before a
  binding's first publication a healthy node joins at once.
- **Never empty:** when no node is in rotation for a record type, Control
  keeps the published values, the binding becomes `degraded`, a warning is
  logged and `forward.dns_degraded` audited (`forward.dns_recovered` when
  it ends).
- **Provider calls:**
  - only when the desired values or the TTL differ from what was
    published, and at least 30 s after the binding's last publication;
  - again every hour, to undo drift made by hand;
  - per provider at most a burst of 10, refilled one every 6 s;
  - after a failure, retried with back-off from 30 s doubling to 15
    minutes.
- Every publication is audited as `system/forward-dns`
  (`forward.dns_publish`, with the values before and after).
- Metrics: `anixops_forward_dns_bindings{state}` and
  `anixops_forward_dns_updates_total{provider,result}`.
- The state (published values, streaks, back-off) is in the binding and
  node tables (`v4_kernel_forward_dns_node`), so a new lease holder carries
  on.
- A paused binding, or a paused or enforced route, changes nothing.
  Deleting a binding with `purge` deletes the records Control published.

The API is in [`../forwarding/v4-api.md`](../forwarding/v4-api.md) ("Entry
HA through DNS"). How to create provider credentials with the least
permissions is in [`../guide/forward-entry-ha.md`](../guide/forward-entry-ha.md).
Control does not dial the entries itself for entry HA: a route's entries
are judged by their own reports and sessions.

### 7.5 Latency probes

Every active check records the connect round trip (`UpstreamHealth.rtt_us`).
Control composes per-hop and end-to-end latency for each route and shows
it in the UI, with history.

### 7.6 End-to-end diagnosis

**Status: implemented in F3c** (`internal/kernelforward/diagnose.go`,
`internal/kernelnodeops/forward_checks.go`). `DiagnoseRoute` follows
fluxlite's method of proving each step rather than inferring it. It reads
and probes; it never takes the plan lock or changes a node. Served as
`POST /api/v4/forward/routes/{id}/diagnose`
([`../forwarding/v4-api.md`](../forwarding/v4-api.md)) and
`anix-control forward routes diagnose <id>`.

The stages, in order:

1. **Control's records** (vantage `CONTROL`):
   - a route-wide `CONFIG` step when the route is paused (`route_paused`)
     or Control pauses it (`route_enforced`);
   - one `CONFIG` step per node of each hop. It fails when the node's
     desired state does not hold the hop (`not_planned`), the node never
     reported (`never_reported`), its report runs an older generation
     (`not_applied`), the report names an error for the hop (`hop_error`),
     or the report is not applied (`apply_failed`);
   - one `HEALTH` step per upstream of the node's hop, from the latest
     report: `healthy`, or failed as `unhealthy` or `circuit_open` (with
     the failures in a row and the time the breaker reopens). Without data
     the step is inconclusive (`no_health`).
2. **Node vantage** (vantage `NODE`), for each node whose Agent holds an
   Agent Control session advertising `agent.diagnostic` and `diag.v1`. The
   forward checks of the `agent.diagnostic` operation run on each of its
   hops (`sdk/api/agent/v1/PROTOCOL.md`, "Forward diagnostic checks"):

   | Check | Step kind | Proves |
   |---|---|---|
   | `forward.listen` | `LISTEN` | the hop's own listener holds its port: the nftables rule in `inet anixops_fwd`, or the gost service bound to it |
   | `forward.port_conflict` | `PORT_CONFLICT` | no foreign process listens on the port and no other table's dnat, redirect or tproxy rule (nat table included) claims it |
   | `forward.connect` | `TCP_CONNECT`, `DELIVERY` on the last hop | a TCP connect from the node to each upstream: the next hop's nodes, or the targets |
   | `forward.udp_probe` | `UDP_EXCHANGE`, `DELIVERY` on the last hop | for UDP routes, a short datagram to each upstream and any reply; no reply is `INCONCLUSIVE` (`no_reply`), never a failure |

   - The Agent probes only what its applied state holds for the hop, so a
     check names a hop, never an address.
   - On the last hop, a literal target the route's policy refuses is not
     probed (`SKIPPED`, `target_not_allowed`), and the check is narrowed to
     the others. A private target is probed only on an administrator's
     route with `TARGET_POLICY_ALLOW_PRIVATE`. The Agent repeats the check
     on every address a target name resolves to.
   - An Agent runs its operations one at a time, so a node's checks go out
     in sequence and the nodes in parallel (at most 8 at once).
   - A node that cannot probe gets its node steps `SKIPPED`:
     `node_offline` when its Agent is not connected,
     `node_vantage_unavailable` when it does not advertise the checks or
     the process holds no Agent sessions (the command line).
     `DiagnoseRouteResponse.nodes` says, per node, whether it was connected
     and could probe.
3. **Control vantage** (vantage `CONTROL`), for what no node probed:
   - a TCP connect to the entry listeners, at the route's listen address or
     the entry node's primary address (`LISTEN`);
   - a TCP connect to the targets when a last hop's node cannot probe
     (`TCP_CONNECT`, no node).

   Control dials public addresses only, whatever the route's policy
   (`not_public` otherwise): it is not on the exit's network. It never
   dials relay or exit listeners, which admit only the previous hop
   (`ingress_sources`), and does not probe UDP (`control_udp_not_probed`).

Each step is a `DiagnoseStep`: node, hop, `ProbeKind`, vantage, target
(`host:port`), protocol, and a `ProbeResult` with its `ProbeStatus`
(`OK`, `FAILED`, `INCONCLUSIVE`, `SKIPPED`), stable `code`, message, round
trip and time. `ok` is true when no step failed.

**Bounds.**

- Budget: 15 s by default (`timeout_ms`, 1 to 25 s, never past the
  caller's deadline less a second). A check that the Agent does not answer
  in its timeout plus 2 s is `INCONCLUSIVE` (`agent_timeout`); one the
  budget leaves no time for is `SKIPPED` (`deadline`).
- Cache: a diagnosis of the same route that finished within 10 s, or that
  is running, is answered again with `cached` true. This also rate-limits
  each route.
- Concurrency: at most 4 routes are diagnosed at once per Control process;
  one more is `RESOURCE_EXHAUSTED` (`429 rate_limited`).

**Not yet.** Proving delivery from the entry's counters with a nonce flow
(the last hop's counters showing the flow arrived and left) is not built.
The last hop's connect or UDP exchange to the targets stands in for it.

## 8. Control and Agent transport

Implemented in F3a: `internal/kernelforward` (the kernel's forwarding
state and `ForwardControl`), `internal/grpc/agent_control_forward.go` (the
stream) and `sdk/forward/wire` (the wire rules both sides share);
`sdk/api/agent/v1/PROTOCOL.md`, "Forwarding", is the Agent's reference.

### 8.0 The kernel's forwarding state

Nine new kernel tables, protected (no package can adopt them), written only
by `internal/kernelforward` (L2 adds three for entry HA, section 7.4:
`v4_kernel_forward_dns_provider`, `v4_kernel_forward_dns_binding` and
`v4_kernel_forward_dns_node`):

| Table | Holds |
|---|---|
| `v4_kernel_forward_route` | each route as protojson, its owner and revision, and `enforced` (Control's own pause, section 5.3) |
| `v4_kernel_forward_allocation` | the port and mark of each (route, hop, node), sticky across plans; a deleted route's rows get `released_at` and stay taken for 10 minutes |
| `v4_kernel_forward_node` | the inventory: per node the persisted `negotiated` flag, the Agent's `NodeCapabilities` and their hash, and the node's settings (port range, reserved ports, addresses, labels) |
| `v4_kernel_forward_node_state` | each node's stamped `NodeForwardState`, `generation` and `state_hash` |
| `v4_kernel_forward_node_report` | each node's latest `NodeForwardReport` without its counters |
| `v4_kernel_forward_counter` | the traffic ledger's cursor: per route, hop, node and counter epoch the largest values reported |
| `v4_kernel_forward_traffic` | the traffic ledger: the raw growth per route, hop, node and UTC hour, by direction |
| `v4_kernel_forward_request` | applied `ForwardControl` writes by request id, kept 7 days |
| `v4_kernel_forward_plan` | the lock row every plan takes, and the last plan's outcome |

- **Inventory.** A proxy or forward node is in it once its Agent
  negotiated `forward.v1` or it has forwarding settings
  (`Service.SetNodeSettings`, the internal API F5a exposes), while its node
  row exists and is enabled; a disabled node drops out, so a route on it
  becomes `unknown_node`. Its engines are those of its last `forward.v1`
  Hello (none before one). Its addresses are its settings', else the node
  row's host when that is an IP address (for a proxy node with a DNS host,
  its reported server IP): the planner needs literals for
  `ingress_sources`. Its port range defaults to 30000-39999; SSH (22) and
  the node's own service ports (a forward node's port, API and metrics
  ports; a proxy node's port and protocol ports) are always reserved.
- **Plans.** Every route write and every inventory change runs
  `planner.Plan` over every stored route with the inventory, the active
  allocations as previous and the held ones as `Options.Taken`, under the
  lock row (one plan at a time across Control processes), then
  `planner.Stamp` from the stored generations (or a node's reported one
  when its report is ahead, 8.2 "Generation recovery"). Nodes whose generation moved
  are written and their sessions pushed (8.1); a node that left the
  inventory with hops gets an empty state, so it never keeps a deleted
  route. Any violation refuses the whole plan: a route write is refused
  (8.6); an inventory change (a Hello with other capabilities, new
  settings) leaves every node on its generation, logs, and records the
  violations (`Service.PlanStatus`, gauge `anixops_forward_plan_refused`).
- **Internal Go API** (`kernelforward.Service`): route writes and reads,
  `PlanRoute`, `RouteStats`, `RouteHealth`, `Traffic` (the hourly ledger),
  `State`, `Nodes`, `SetNodeSettings`, F5a's `ListNodes`, `GetNode`,
  `SetNodeSettingsAnswer`, `CreateForwardNode`, `UpdateForwardNode`,
  `DeleteForwardNode`, `TrafficBuckets` and `Enforcement`, `RecordHello`, `RecordReport`,
  `Replan`, `PlanStatus`, `Maintain` (the singleton worker's minute tick:
  expiry, request-id retention), `ResetNode` (the operator's generation
  bump, `anix-control forward reset-node`; never served on
  `ForwardControl`), and the package functions
  `NodeConfigMember`, `NodeConvergence`, `OnStateChange` and
  `WritePrometheus`.

### 8.1 Desired state

- A new capability, `forward.v1`, in `Hello.capabilities` and
  `HelloAck.server_capabilities`. The Agent lists it with its
  `NodeCapabilities` as protojson in the attribute `node_capabilities` (at
  most 16 KiB; `sdk/forward/wire`), so Control knows what the node can do
  before it builds the node's first snapshot. Control serves it for proxy
  and forward nodes when the Hello lists it with a valid attribute and the
  session gets `config.v1`; a malformed attribute only withholds it.
- The node's forwarding state rides in `config.v1`: `ConfigSnapshot` with a
  new format `anixops.nodeconfig/v2`, which is `anixops.nodeconfig/v1` plus
  a `forward` member holding the `NodeForwardState` (protojson, proto field
  names, decoded and re-encoded with sorted keys so the document hash
  depends only on the state). The format follows the node's persisted flag
  (whether its last Hello negotiated `forward.v1`), never the live session,
  so every builder (the Hello reconcile, the minute refresh, `node.sync`)
  builds the same document and the revision does not flap; an older Agent
  keeps v1 and gets no forwarding. Every new generation bumps
  `config_revision`, and the plan that made it pushes the snapshot to the
  node's session at once (trigger `forward` of
  `anixops_agent_config_snapshots_sent_total`). `generation` 0 means
  Control has no state for the node yet: the Agent keeps what it runs.
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
  `forward.v1`, not from package releases: the stream routes the kind
  before the release-capability check of other kinds. Forward nodes are
  offered `package-reports.v1` with `forward.v1`. Refusals are counted
  with the package reports' (`anixops_agent_package_reports_refused_total`,
  new reason `unnegotiated`); the stream stays open. The report must name
  the stream's node and stay within the bounds of `wire.CheckReport`; the
  256 KiB payload cap holds about 800 hops of counters, and chunking a
  larger node's counters is deferred.
- Control stores the report without its counters as the node's latest
  (dropped when one observed later is stored) and meters the counters
  (section 11). Gauges: `anixops_forward_nodes`,
  `anixops_forward_lagging_nodes` (reported generation behind the desired
  one, or never reported), `anixops_forward_unreported_nodes`,
  `anixops_forward_generation_lag_max`, `anixops_forward_hop_errors`.
- **Generation recovery** (`internal/kernelforward/generation.go`). The
  Agent applies only a generation newer than the one it holds, and refuses
  the one it holds with another `state_hash`; its report carries the
  generation and `state_hash` it holds. After Control's database is reset
  or restored from a backup, `v4_kernel_forward_node_state` restarts below
  that, and every state Control sends would be ignored (the drivers answer
  `ErrStaleGeneration`). Control recovers from the reports alone, with no
  Agent change:
  - A node's report is *ahead* of its stored state (`g`, `h`) when its
    generation `G` is higher than `g`, or `G == g > 0` with another
    non-empty `state_hash`.
  - When `RecordReport` stores a report that is ahead, it moves the node's
    stored generation, under the plan lock, to `G` when the report's
    `state_hash` equals `h` (the node already runs those hops), else to
    `G + 1`, keeping the hops (the stored `state_json` is re-encoded with
    the new generation), and pushes the node's snapshot at once.
  - Every plan stamps from the larger of the stored and the reported
    generation, so a node whose report is ahead and that has no stored
    state yet (its plans were refused since the reset) gets a generation
    above the report with its first accepted plan. A report never creates
    a state: a refused plan still leaves every node on what it runs.
  - `anix-control forward reset-node <node_ref>` (`Service.ResetNode`,
    admin-only: the command line on Control's database, never
    `ForwardControl`) moves a node with a stored state to
    `max(g, G) + 1`, keeping its hops, and writes the audit log as
    `system/cli`; the running Control sends it at its next configuration
    refresh, within a minute.
  - Each recovery is logged (warning `forward generation recovered`) and
    counted by `anixops_forward_generation_recoveries_total{reason}`
    (`report`, `plan`, `operator`; per Control process).
  - Until the node's first report after the reset (at most 60 s) its Agent
    ignores the lower generation and keeps running the state it holds.

### 8.3 Probes

Implemented in F3c. The node probes of `DiagnoseRoute` (7.6) ride the
existing `agent.diagnostic` operation on the Agent Control stream, with the
forward actions `forward.listen`, `forward.port_conflict`, `forward.connect`
and `forward.udp_probe`. There is no `forward.probe` operation kind.

- Their parameters mirror `ProbeRequest`: `route_id`, `hop_index` and
  `timeout_ms`, with `generation`, `upstream` and `target_policy`.
- The Agent's `ObservedState` carries the generic diagnostic answer with
  the check's structured `result`.
- Control sends them only to a session that advertises `agent.diagnostic`
  and `diag.v1`, from `kernelnodeops.ForwardChecks` directly on the stream.
  They are probes, not administrator tasks: neither
  `v2_agent_diagnostic_task` nor the KernelNodeOps ledger records them. The
  diagnosis endpoint is audited.
- The administrator diagnostic routes and the KernelNodeOps
  `agent.diagnostic` kind refuse the forward actions
  (`service.ValidateAgentDiagnosticTask`).

`ForwardNode.Probe` remains the in-process shape of one step.
`sdk/api/agent/v1/PROTOCOL.md`, "Forward diagnostic checks", is the Agent's
reference.

### 8.4 Connection

The lesson from flux issue #25 (a provider banned a node over frequent
panel connections):

- one long-lived Agent Control stream per node; no per-route or polling
  connections;
- a low-frequency heartbeat (60 s for forward nodes, against today's
  default of 20 s) with jitter. Decided by default with F3a (the owner may
  revisit): `HelloAck.heartbeat_interval_seconds` is 60 for a forward
  node's session that negotiated `forward.v1`, the existing per-session
  field, so older Agents and proxy nodes keep 20 s. A certificate revoked
  while the stream is open ends it at the next heartbeat, so within a
  minute on these sessions;
- reconnects with exponential backoff and jitter, from 1 s to 5 minutes;
- several Control endpoints (IPv6, alternate domains), from enrollment and
  updated in the configuration.

### 8.5 When Control is down

The node keeps forwarding on its last applied state: nftables rules stay in
the kernel and gost keeps running, health checks and failover keep running,
counters keep counting, local quota and expiry still apply. On reconnect
the Agent sends its `config_revision`; Control sends a newer snapshot if
there is one, and the next report brings the counters up to date.

### 8.6 ForwardControl for packages

The kernel serves `ForwardControl` to official packages that declare the
kernel capability `kernel.forward.v1`, on local bridge sessions and the
module listener, authorized on every call against the host's generation
(as `KernelNodeOps`). The forward package (F5a) builds `/api/v4/forward/*`
on it ([`../forwarding/v4-api.md`](../forwarding/v4-api.md)).

- Writes take a `request_id` (1 to 128 bytes): a retry answers the recorded
  response once; the same id with another request is
  `FAILED_PRECONDITION`; a refused write is not recorded.
  `CreateRoute` assigns the id (a ULID), revision 1 and the times;
  `UpdateRoute` needs `expected_revision` (`ABORTED` when stale).
- Refusals: `INVALID_ARGUMENT` for a malformed route, `FAILED_PRECONDITION`
  when the nodes cannot host it (unknown or disabled node, missing engine or
  capability, port taken, reserved or out of range, ports or marks
  exhausted, no address) or another stored route no longer plans. The
  status's details carry the method's response (`PlanRouteResponse` for
  `DeleteRoute`) whose violations name their route
  (`Violation.route_id`, empty for the new route of a create).
- `PlanRoute` previews without storing; `GetRouteStats` answers the
  ledger's totals per hop and node; `GetRouteHealth` the upstream health of
  the route's nodes' latest reports.
- **Added in F5a** (additions only):
  - `GetRoute` and `ListRoutes` answer `enforced`, the reason Control
    itself pauses a route (`quota`, `expired`).
  - `ListNodes` and `GetNode` answer the inventory as `NodeSummary`: every
    forward node, plus the proxy nodes in the inventory. Each carries the
    node's stored `NodeSettings`, the planner's `NodeInfo`, the reserved
    ports, the capabilities, the desired generation and hop count, the
    latest report's generation, `applied` and hop errors, and a forward
    node's `ForwardNodeRecord`. `GetNode` adds the desired state and the
    latest report.
  - `SetNodeSettings` is `Service.SetNodeSettings`. A replan the settings
    make impossible stores them anyway and answers the violations.
  - `CreateForwardNode`, `UpdateForwardNode` and `DeleteForwardNode` write
    the forward node registry (`v2_forward_node`, the Agent identity
    `forward-<id>`).
    - They go through `service.ForwardNodeService`, so the credential split
      and Agent certificate revocation follow, and they use the plan lock
      and the request ledger.
    - A node a stored route uses cannot be disabled or deleted:
      `FAILED_PRECONDITION`, with violations of the new code `node_in_use`
      naming the routes.
    - Control assigns a new Agent node the legacy credential the v2 paths
      read until F5d. No answer carries it, and an update never changes it.
    - `NodeTransport` tells Agent nodes from Ansible machines (section 6.3).
  - `GetTraffic` answers the ledger's hourly buckets by route and node, over
    at most 31 days and at most 20000 buckets.
- **Added in F3c** (additions only): `DiagnoseRoute` answers (7.6). It
  added the `ProbeKind` values `CONFIG`, `HEALTH` and `PORT_CONFLICT`, the
  enums `ProbeStatus` and `DiagnoseVantage`, `ProbeResult.status` and
  `code`, `DiagnoseStep.vantage`, `target` and `protocol`,
  `DiagnoseRouteResponse.route_id`, `started_at_unix_ms`,
  `finished_at_unix_ms`, `cached` and `nodes`, and the message
  `DiagnoseNode`. Too many diagnoses at once is `RESOURCE_EXHAUSTED`.
- **Added in L2** (additions only): entry HA through DNS (7.4).
  `ListDnsProviders`, `GetDnsProvider`, `CreateDnsProvider`,
  `UpdateDnsProvider`, `DeleteDnsProvider`, `ListDnsBindings`,
  `CreateDnsBinding`, `UpdateDnsBinding`, `DeleteDnsBinding` and
  `GetRouteDns`, with the messages `DnsProvider`, `DnsBinding`,
  `RouteDnsStatus`, `DnsRecordStatus`, `DnsEntryNode` and the enums
  `DnsProviderKind`, `DnsBindingMode`, `DnsRecordType`.
  - Credentials travel only in the create and update requests
    (`credentials`, write-only); `DnsProvider` has no field for them.
  - Refusals carry the method's response with violations (codes
    `secret_store_unavailable`, `provider_in_use`, `binding_exists`,
    `name_taken`, `unknown_route`, `unknown_provider`,
    `entry_hostname_required`, `hostname_mismatch`, `immutable`,
    `invalid_format`, `required`, `unknown_field`).
  - A purge the provider refuses is `UNAVAILABLE`.

## 9. Node onboarding

The target is nyanpass's experience: copy one command, paste it on the
machine, and the node appears. This is part of F3 and reuses the agent
enrollment (`internal/agentpki`).

**Status: O1, O2 and O3 implemented** (the command, the token API, the
signed script; preflight and offline bundles; uninstall; operator guide
`docs/guide/agent-onboarding.md`). **O4 implemented on the Control side**
(staged upgrades; the Agent's `upgrade.v1` is a follow-up in anix-agent,
see "Upgrades (O4)" below). As built:

- `POST /api/v4/kernel/agents/install-tokens` (super administrators,
  `service.IsSuperAdmin`): `{node: "proxy-<id>"|"forward-<id>",
  ttl_seconds}` → `{enrollment, credential, node, expires_at,
  agent_version, commands: [{mirror, command, available, note}], script:
  {url, signature_url, signed}}`. The token is an AgentPKI one-time
  credential (`CreateEnrollmentToken`: SHA-256 only, in
  `v4_kernel_agent_enrollment`, audited as `agent_enrollment_token_issue`),
  1 hour by default, 60 s to 7 days (H18).
- **The command carries `--control` and `--node` besides H18's flags.**
  The script is static so that one release signature covers every copy: it
  is embedded in Control (`internal/agentinstall/install.sh`), served
  byte for byte at `GET /install.sh` and published as the release asset
  `agent-install.sh`. A static script cannot know its Control or node, so
  the command names them:
  `curl -fsSL https://<control>/install.sh | sudo bash -s -- --control https://<control> --node forward-41 --token <t> [--mirror cn|github]`.
- **Signature.** The release job signs the script with the official
  package root (`plugins.official_public_key`, Ed25519 over the exact
  bytes, base64: the packages archive's `.sig` format) and publishes
  `agent-install.sh.sig`; the release image ships it and Control serves it
  at `/install.sh.sig` only when it verifies the embedded script.
- **Release metadata.** `GET /install/agent.env` (public, rate limited)
  tells the script the Agent release (`v` + Control's version, H25), the
  gRPC target, the mirror bases and, when Control holds the release
  (`agent_install.artifact_dir`, served at `/install/agent/<tag>/<asset>`),
  its SHA-256. The checksum comes from Control, else GitHub, never from the
  mirror; a `.sig` next to an Agent asset is verified with the official key.
- **Node groups are not implemented.** Tokens bind to an existing node:
  forward nodes have no group model and creating the node on first use needs
  a change to `agentpki.Enroll`. `--group` is accepted by the parser and
  refused with that reason.
- **Preflight (O2).** After reading `/install/agent.env` (or the bundle)
  and before changing anything the script checks: systemd ≥ 240 (fail;
  < 247 warns about the gost sandbox), the kernel ≥ 5.10 and nft ≥ 0.9.7 on
  forward nodes (fail), `tc` and `nf_conntrack` (warn), polkit ≥ 0.106
  (warn: 0.105 reads only `.pkla`, which cannot be limited to one unit, so
  no `.pkla` is written and gost hops cannot run there), firewalld, ufw's
  forward policy and the iptables `FORWARD DROP` policy (warn), SLAAC
  interfaces whose `accept_ra` is not 2 (warn; `--accept-ra` writes
  `net.ipv6.conf.<if>.accept_ra = 2` to the drop-in), listeners in
  `--port-range` (warn; skipped without it: the Agent listens on no port),
  200 MiB free (fail), Control's https address and the gRPC target with TLS
  verified against the system CAs (fail), and the clock against Control's
  `Date` header (warn > 30 s, fail > 5 min). Every failure prints a fix;
  the script stops after all checks, and `--skip-preflight` overrides it.
- **Offline (O2).** `--offline <bundle>` installs from a tar.gz of flat
  files: `agent.env`, the Agent zip of one architecture and its `.sig`,
  `SHA256SUMS` and `SHA256SUMS.sig`, `install.sh` (and its `.sig`).
  `anix-control agent offline-bundle -arch amd64|arm64 -o <file>` writes it
  from `agent_install.artifact_dir`, after checking the signatures. The
  script accepts no other entry and requires both signatures by the
  embedded official key and the zip's digest in `SHA256SUMS`; the bundle's
  `agent.env` is unsigned and never supplies a digest. Enrolling still
  needs the gRPC target.
- **Uninstall (O3).** `install.sh uninstall [--purge]` (the Agent's own
  `uninstall` predates this layout): stops and removes `anix-agent.service`,
  then `anixops-gost.service`, the polkit rule and the binaries, and keeps
  the identity, configuration, state, users and the forwarding objects.
  `--purge` also deletes `inet anixops_fwd` only when it carries the
  ownership comment, root HTB qdiscs with the handle `af00:` (configured
  `LimitInterfaces` and every interface), the state and configuration
  directories, the sysctl drop-in and the users. It cannot reach Control:
  the administrator revokes the node's credentials (`agentpki.RevokeNode`).
- The script follows the steps below except OpenRC (refused). On a forward node
  (or a proxy node with `--forward`) it writes the sysctl drop-in
  `/etc/sysctl.d/90-anixops-forward.conf` (`net.ipv4.ip_forward = 1`,
  `net.ipv6.conf.all.forwarding = 1`, the file the Agent's nftables
  driver check names, from `anix-agent forward sysctl-dropin` when the
  Agent has it) and applies it with `sysctl -e -p`; one that cannot apply
  now is noted and applies at the next boot. The Agent unit has
  `RuntimeDirectory=anixops-agent` (0750) for the plugins' sockets. A host
  switching from anix-agent's root install (a unit without
  `User=anixops-agent`, or `/var/lib/anix-agent`,
  `/var/lib/anixops/plugins`) runs `anix-agent migrate-paths --chown
  anixops-agent` before the new unit starts. It runs the Agent as `anixops-agent` with `SupplementaryGroups=anixops-gost`,
  ambient `CAP_NET_ADMIN CAP_NET_BIND_SERVICE` and the sandbox of section
  14, installs `anixops-gost.service` from the contract and a polkit rule for
  it, writes the token to `/var/lib/anixops-agent/enroll.credential` (0600,
  owned by the Agent, which removes it), removes the legacy runtime of
  section 10 (the three tables and the clean agent's `v2forward-agent`
  unit and files) and reports it, then waits for `anix-agent identity
  --json` to show a valid certificate. Re-running it upgrades in place and
  keeps the identity; `--reset` enrolls again.

- **The command.** The node list and the node's "部署" section have a "复制安装命令"
  button:
  `curl -fsSL https://<control>/install.sh | bash -s -- --token <one-time token> [--group <node group>] [--mirror control|cn|github]`
  (as built: plus `--control` and `--node`, see the status above).
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
  an Agent upgrade (kernel rules, separate gost unit). As built (O4), see
  "Upgrades (O4)" below.
- **One Agent per node.** nyanpass starts extra instances for load
  sharing; we do not need that: nftables is in the kernel and gost scales
  across cores.

### Upgrades (O4)

Owner decision H19 (2026-10-04): batches of 5%, 25% and 100%, at least
30 minutes each; a batch rolls back automatically when more than 5% of its
Agents fail to reconnect within 10 minutes or fail to apply; the artifacts
are signed with the official Ed25519 key and the node verifies them; an
Agent never upgrades on its own. H25: the Agent release is Control's.

- **Campaigns** (`internal/agentupgrade`). Two new protected tables:
  `v4_kernel_agent_upgrade_campaign` (target version, the verified
  artifacts per architecture, the batches as cumulative percent and
  minimum duration, the H19 threshold and reconnect timeout, the
  exclusions, status, current batch and its start, rollback start, reason,
  error code, actor; `active_slot`, unique and NULL once terminal, keeps one
  campaign active on SQLite and PostgreSQL) and
  `v4_kernel_agent_upgrade_node` (campaign, node kind and id, batch, canary
  order key, state, the version it ran, the operation and session, the
  rollback operation, error code and message, offered, handed-off,
  reconnected, finished and rollback times).
- **Nodes.** Enabled proxy and forward nodes whose Agent was seen on the
  Agent Control stream (`v4_kernel_agent_transport`, `mtls-stream` or
  `apikey-stream`), minus excluded nodes and node tags, ordered by
  SHA-256 of `anixops-agent-upgrade:<node>`, so the canaries are the same
  in every campaign. Batch *i* takes the nodes up to its cumulative share,
  rounded up (the canary batch has at least one node; an empty batch passes
  at once). Custom batches must start at 5% or less, grow, end at 100% and
  last 30 minutes or more.
- **States.** A campaign is `running`, `paused` (nothing is offered and no
  batch starts, but offered nodes are still judged and a failing batch
  still rolls back; the paused time does not count towards the batch),
  `rolling_back`, then `succeeded`, `rolled_back` or `aborted`. A node is
  `pending`, `offered` (the operation was acknowledged), `upgrading`
  (progress or hand-off reported), then `succeeded`, `failed`,
  `rolled_back` or `skipped` (no `upgrade.v1`, offline for the whole
  batch, or connected but never acknowledging during it: not counted).

  ```text
  running ──(batch settled ∧ ≥ min duration)──▶ next batch … ──▶ succeeded
     │ ▲ pause/resume                     failed/offered > 5% (any time)
     ▼ │                                              │
  paused ───────────────(failed > 5%)───────────────▶ rolling_back ──▶ rolled_back
  running/paused ──abort──▶ aborted        abort --rollback ──▶ rolling_back
  ```
- **The worker** runs in the singleton-worker process, which holds the
  Agent streams, every 5 seconds. It offers `agent.upgrade` to the current
  batch's pending nodes (16 at a time), judges in-flight ones (a session
  opened after the offer with the target version in `Hello`, and no
  `failed` `ConfigStatus` after the reconnect; a reconnect with the old
  version after the hand-off; 10 minutes), applies the threshold on every
  pass, and advances a settled batch once it has lasted its minimum
  duration. On rollback it sends `agent.upgrade` with action `rollback` to
  the batch's nodes that run the target and ends the campaign when they
  reconnect with another version, or after twice the reconnect timeout
  (`rollback_unconfirmed`). Automatic transitions are audited as `system`.
- **Contract.** `upgrade.v1` (offered by the intersection rule to proxy and
  forward nodes) and the `agent.upgrade` operation, payload
  `anixops.agent-upgrade/v1` (`sdk/agentcontrol`): `PROTOCOL.md`, "Agent
  upgrades", has the schema and exactly what the Agent and its updater do.
  No protobuf change. Packages cannot send it.
- **Artifacts.** The release must be in `agent_install.artifact_dir/<tag>/`:
  each architecture's zip with its `.sig`, `SHA256SUMS` and
  `SHA256SUMS.sig`, both signatures valid with `plugins.official_public_key`
  and every zip's digest listed (`agentinstall.UpgradeArtifacts`). The
  operation names the control mirror's URL
  (`<public_url>/install/agent/<tag>/<asset>`), the SHA-256, size and
  signature; the Agent verifies them with the key it embeds.
- **Privileged updater (decision).** The Agent runs as `anixops-agent`
  under `ProtectSystem=strict` and cannot replace `/usr/lib/anixops-agent`.
  The installer writes `anixops-agent-updater.path` (`PathExists=` the
  request file under `/var/lib/anixops-agent/upgrade/`, which the Agent may
  write) and the root oneshot `anixops-agent-updater.service`, whose
  `ExecStart` is the installed, root-owned `anix-agent upgrade apply`: it
  verifies again, keeps `anix-agent.prev` for a rollback, swaps atomically,
  restarts the Agent and reinstates the previous binary when the new one
  does not stay up. A path unit rather than polkit, because polkit 0.105
  hosts cannot scope a rule to one unit (section 9, preflight) and some
  hosts have no polkit; a path unit needs only systemd. The trust anchor is
  the installed binary, not the request: a compromised Agent can only ask
  for a signed official release, or a rollback to the kept one. Agents in
  the field have neither the units nor `upgrade.v1`: the first move onto
  this path is an installer re-run.
- **API and tools.** `POST /api/v4/kernel/agents/upgrades` (super
  administrators; `target_version` defaults to Control's Agent release,
  `batches`, `exclude {nodes, tags}`, `reason`), `GET` (list) and
  `GET /:id` (with every node), `POST /:id/pause`, `/resume`, `/abort`
  (`{"rollback": true}` rolls the current batch back first); audited in
  `v2_operation_log` as module `agent_upgrade`.
  `anix-control agent upgrade start|status|pause|resume|abort`. The Agent
  transports page shows the latest campaign with its batches.

## 10. Upgrade from v4.1

Decided: old forwarding data is not migrated.

**Order (owner decision, 2026-10-04).** v4.2 makes
`agent_control.mtls: required` the default (H5), which refuses the clean
agent's register, heartbeat and report endpoints. Control v4.2 could then no
longer reach a clean agent node to clean it, so the old runtime is removed
before Control is upgraded:

1. **Release the new Agent first** (AG-2 and the forward component, F3b).
   On install it removes the old forward runtime locally, without relying on
   Control: the `inet v2b_forward` and `ip v2b_forward` tables (the Ansible
   path), the `ip anixops_forward` table (the canary Agent plugin), and the
   gost services the flux runtime and the clean agent created. Like the
   uninstaller (section 9), it touches no other table or service. (O1, as
   built: the three tables and the clean agent's `v2forward-agent` unit,
   `/etc/v2board-forward-agent` and `/usr/local/bin/v2forward-agent`. gost
   services the flux runtime created through gost's API on NodeX hosts live
   in an operator-installed gost, which the installer does not touch; step 2
   of the upgrade below cleans them through NodeX's API. Further unit names
   are added only once confirmed.)
2. **Switch every forward node to the new Agent**, NodeX nodes included
   (H20). `anix-control agents transports --legacy-only` must list no node
   (`--check-required`, from v4.2, must exit 0: it also catches enabled
   nodes that never enrolled).
3. **Upgrade Control to v4.2**, with `agent_control.mtls` defaulting to
   `required` (H5; done: an empty `agent_control.mtls` is `required`).

**The upgrade itself (F5c)** then runs three steps in this order. The last
one is **IRREVERSIBLE** and is gate **H15**, confirmed on its own.

1. **Archive.** Export `v2_forward`, `v2_forward_tunnel`,
   `v2_forward_user_tunnel`, `v2_speed_limit`, `v2_forward_rule` and
   `v2_forward_runtime_job` (and any other forward route tables found by
   the F5 audit) to one JSON file, readable only by administrators (file
   mode 0600 under the data directory, and a super-administrator download).
   Node tokens and other secrets are left out. Forward nodes
   (`v2_forward_node`) are not dropped: they are node inventory and the
   Agent identity (`forward-<id>`).
2. **Check the nodes are clean.** Agent nodes cleaned themselves in step 1
   of the order above; F5c only verifies them, from the Agent's report.
   Control still cleans the hosts the Agent channels do not reach, which
   `required` does not affect: NodeX hosts through NodeX's own HTTP API
   (its delete calls), and hosts on the Ansible fallback (section 6.3)
   with a cleanup playbook over SSH. Each node's result is recorded and
   re-checked by listing.
3. **Drop the tables.** Only when every node reports clean, or an
   administrator marks the unreachable ones as abandoned (listed by name),
   and only after the H15 confirmation. A rollback to v4.1 after this step
   needs a database backup; UPGRADE.md says so prominently and says that
   forwarding must be reconfigured.

The flux v2 routes, `forwardcompat`, the route catalog entries and the
flux guardrails in AGENTS.md go in the same release (F5, H17).

**The 53 bridged forward routes.** `config/package-extraction.json` has 53
`bridged` routes in the `forward` package (its `native-flagged` and
`kernel-owned` routes are not counted here). None of them is moved to a
native handler: M3-4 and M3-5 (`node-ops-service.md` section 7) are
cancelled. They split three ways:

- **30 flux routes, deleted by F5d:**
  - administrator forwards: `POST /admin/forward/create`, `update`,
    `delete`, `force-delete`, `pause`, `resume` and `diagnose` (7);
  - legacy rules: `GET`/`POST /admin/forward/rules`, and `GET`, `PUT`,
    `DELETE` and `POST .../toggle` on `/admin/forward/rules/:id` (6);
  - `POST /admin/forward/sync-backend` (1) and
    `GET /admin/forward/runtime/jobs` (1);
  - tunnels: `POST /admin/tunnel/diagnose`, `update`, `user/remove` and
    `user/update` (4);
  - user forwards: `POST /forward/create`, `update`, `delete`,
    `force-delete`, `pause`, `resume` and `diagnose` (7);
  - `POST /speed-limit/update` (1), `POST /tunnel/user/remove` and
    `update` (2), and `POST /user/forward/rules` (1).
- **19 node management routes, rewritten in F5a** as `/api/v4/forward/*`:
  forward nodes (`/admin/forward/nodes`, 8), Ansible machines
  (`/admin/forward/ansible-machines`, 8) and observability
  (`/admin/forward/observability/targets`, `topology` and `trend`, 3). The
  v2 routes go with F5d. As built (F5a,
  [`../forwarding/v4-api.md`](../forwarding/v4-api.md)):
  - list, create, get, update, delete and toggle map one to one onto
    `/nodes` and `/ansible-machines`;
  - `check` is the node view (`GET /nodes/{ref}`): the Agent's report
    replaces Control dialling the node;
  - `sync-stats` is the traffic ledger (`GET /stats?node_ref=`): the
    nodes push their counters;
  - the trend is hourly traffic, since the kernel keeps no latency history.
- **4 clean agent routes, retired with the switch to the new Agent:**
  `GET`/`POST /admin/forward/agents`,
  `POST /admin/forward/agents/:id/revoke` and
  `GET /forward-agent/install.sh`.

## 11. Metering

- **The SDK reports raw counters**: bytes and packets per node, route, hop
  and direction, plus connections. It applies no multiplier.
- **Control keeps a traffic ledger** of the deltas (section 8.2), per
  route, hop, node and direction, and enforces `quota_bytes` on raw bytes.
  Implemented in F3a: `v4_kernel_forward_counter` keeps, per route, hop,
  node and counter epoch, the largest cumulative values reported; a report
  adds each field's growth over them to `v4_kernel_forward_traffic` (the
  hour of its observation, up and down bytes and packets, new
  connections). Epoch rules: within an epoch only growth counts, and a
  field that went down adds nothing and keeps the stored value; a new
  epoch (reset, re-created hop, restart) counts its values in full; a
  report observed before the node's stored one is dropped whole. A route's
  metered traffic is the sum of its epoch rows (`GetRouteStats`), its
  entry's sum drives the quota (section 5.3), and no multiplier is applied
  anywhere in the kernel.
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
features go into which edition was decided by the owner on 2026-10-04
(H23), as proposed:

| Feature | Community | Commercial |
|---|---|---|
| Routes, hops, nftables and gost engines, limits, counters | yes | yes |
| Load balancing, two-level failover, circuit breaker, latency, diagnosis | yes | yes |
| AnixOps relay protocol engine (v4.3; experimental in v4.2) | yes | yes |
| Entry HA via DDNS | yes | yes |
| One-command onboarding, staged upgrades | yes | yes |
| User self-service forwarding page (v4.3) | no | yes |
| Forwarding plans, auto-renewal, billing multipliers (v4.3) | no | yes |
| Resellers and panel federation (v4.4) | no | yes |

As built (F5a):

- Every `/api/v4/forward/*` endpoint is in both editions.
- `config/editions.json` `commercial_api_prefixes` reserves
  `/api/v4/forward/self/`, `/plans/` and `/multipliers/` for v4.3. The
  community edition answers them as routes that do not exist, in the kernel,
  before the package (`edition.Policy.HidesPath`).
- Each endpoint of the package carries an edition (`v4api.Endpoints`), and a
  commercial one must sit under a reserved prefix.
- User self-service needs a user-facing route group in v4.3, since `/api/v4`
  is administrator-only.

## 13. Testing

- **Golden fixtures** in `contracts/forward/v1`: a `PlanRouteRequest` and the
  `PlanRouteResponse` the planner must answer, plus negative cases with the
  expected violation. This PR adds three drafts;
  `internal/tests/protocompat` keeps them parseable as the draft contract
  and internally consistent (ports, wiring, ingress sources, where limits
  land). F1c's planner tests produce them byte for byte (canonical
  protojson, two-space indented; `go -C sdk test ./forward/planner -run
  Golden -update` rewrites them) and run the negative cases; F1c added
  goldens for UDP with IPv6 targets, several entry nodes behind an entry
  hostname, a sticky re-plan, port exhaustion and a multi-route plan with
  generations. F2/F4 add the rendered nft and gost artifacts: the nft
  goldens are in `contracts/forward/v1/nft` (F2b, section 6.1), the gost
  configurations in `contracts/forward/v1/gost` (F4a, section 6.2).
- **Planner unit and property tests**: allocation stickiness, collisions,
  exhaustion, wiring, generations; properties: re-planning is idempotent,
  ports and marks never collide on a node, every port is in range and not
  reserved, removing a route frees its ports, and a generation bumps
  exactly when the node's state bytes change.
- **Driver conformance suite** (F2a, implemented:
  `sdk/forward/driver/conformance`). `conformance.Run(t, factory)` runs one
  scenario list against every driver, each scenario on a fresh `Env` (the
  host: an in-memory one for the fake, a namespace for the nftables driver,
  F2c). The required
  `Env` methods make driver instances (a second one is an Agent restart),
  list owned objects, and plant and list foreign ones; optional interfaces
  (`Damager`, `TrafficSource`, `ApplyFaulter`, `ConflictPlanter`,
  `ImpostorPlanter`, `ApplyCounter`) unlock the scenarios that need them,
  which skip otherwise. Data-driven state cases, each run when the driver's
  capabilities cover it, on a topology the harness may override: single
  target, each strategy (failover included), TCP+UDP, UDP, IPv6, dual stack,
  limits, several routes, a paused hop. Scenarios: render determinism,
  order independence, identity excluded from the digest, other engines
  ignored, empty state, input not mutated, unsupported capability and
  unknown enums (`ErrUnsupported`, per hop), invalid state; apply of every
  case, idempotent re-apply, newer generation with the same content, stale
  generation, generation conflict, invalid artifact, restart, repair of
  partial state, failed apply keeps the previous state, conflict with a
  foreign object, impostor not owned, empty artifact removes; unrelated
  hops untouched by adding, moving and removing other hops (F4c); counters
  kept across re-apply, re-created hops, monotonic observation; failover and
  recovery through `SetUpstreams`, weights, its errors, reset by a changing
  apply, rotation surviving a restart; removal leaving nothing behind;
  cancelled and expired contexts; concurrent calls. In every scenario the
  foreign objects must be unchanged at the end and every observation is
  checked for counter monotonicity within an epoch. The fake driver
  (`sdk/forward/driver/fake`) passes it in unit tests under several
  capability sets, and mutant drivers prove each rule is enforced.
- **netns end-to-end** (F2d, implemented: `sdk/forward/e2e`). Each test
  builds its own network namespaces (`ip netns add`, named
  `afe2e-<pid>-<lab>-<role>`) joined by veth pairs: a client, an entry
  node, relay and exit nodes and target hosts, addressed in IPv4 (TEST-NET)
  and IPv6 (`2001:db8::/32`), with forwarding switched on inside the node
  namespaces only. It drives them as Control and the Agents will: routes
  from `sdk/forward/model`, `validate.Route` against an inventory of the
  namespace nodes with the capabilities their drivers probed in the
  namespace (`nftables.Probe`), `planner.Plan` with the previous
  allocations and `planner.Stamp`, then `Render` and `Apply` by each
  node's nftables driver through a runner that runs nft and tc in the
  node's namespace. The targets are echo servers (the test binary
  re-executed with `ip netns exec`); the client dials from its namespace
  (`setns` on a locked thread). Every scenario moves real traffic and reads
  the replies:
  - one hop, TCP and UDP over IPv4 and IPv6; the entry hop's counters match
    the bytes moved in each direction: UDP exactly (payload plus 28 or 48
    header bytes per datagram), TCP within the header range of the counted
    packets (TSO segments, so not wire packets), plus 10% and 16 KiB for
    retransmissions;
  - two hops (entry, exit) and three (entry, relay, exit) over RAW links:
    every hop counts the same traffic and admits only the previous node's
    addresses; the client routed straight to the exit's port times out,
    while it reaches a server of the exit's own;
  - balancing over three targets: round robin 10/10/10 (within 1) of 30
    connections after an apply and 2:1:1 weights within 2 of 20/10/10,
    random and least connections at least 15 of 90 per target (30
    expected; a sample that misses is retried twice), IP hash stable for
    each of 24 client addresses and spread over more than one target;
  - failover: the primary target killed, new connections fail until a
    simulated health loop (TCP checks from the entry node) calls
    `SetUpstreams` with the backup; traffic moves without a new generation,
    digest or counter epoch, and returns to the primary when it is back and
    every upstream is restored;
  - quota (300 000 bytes): exchanges stop before the quota, new connections
    fail and the counters stay under it; raising it keeps the usage and
    traffic resumes;
  - connection limit 3: the fourth connection is refused while three stay
    open, and closing one frees its slot;
  - bandwidth 8 Mbit/s with tc HTB: the steady echo rate is within 0.5 to
    1.5 times the limit (the same path unlimited is many times faster), and
    both directions pass their hop's HTB classes; skipped when tc HTB is
    unavailable;
  - pause: new and established traffic is dropped uncounted, counters keep
    their epoch and values, and counting resumes on unpause;
  - Agent restart: a new driver instance observes the same generation,
    hash, digest, counters and rotation; re-applying, directly or through
    a re-plan, changes nothing and keeps an established connection;
  - Remove: the table and the tc qdiscs go, another table and a qdisc on
    another interface stay, and a second Remove succeeds;
  - leftovers: namespaces of the suite whose test process is gone are
    removed with the processes inside them (the suite does this before it
    starts); a live run's namespaces and every other namespace stay.

  A failed test writes each namespace's ruleset, qdiscs and classes,
  addresses, routes and sockets and the echo servers' logs to
  `ANIXOPS_FORWARD_E2E_LOGDIR` (or the test log). The suite needs root
  (`CAP_NET_ADMIN`, and `CAP_SYS_ADMIN` for the namespaces) and runs only
  with `ANIXOPS_FORWARD_E2E=1`; otherwise it skips, and with the variable
  set a missing tool fails. It is gated by that variable, not a build tag,
  so every lane compiles, vets and lints it. To run it locally:

  ```sh
  go -C sdk test -c -o /tmp/forward-e2e.test ./forward/e2e
  sudo ANIXOPS_FORWARD_E2E=1 ANIXOPS_GOST_BIN=/path/to/gost /tmp/forward-e2e.test -test.v
  # or through go test; sudo's secure_path drops the Go toolchain, so pass
  # PATH (this leaves root-owned entries in your Go build cache):
  sudo -E env "PATH=$PATH" ANIXOPS_FORWARD_E2E=1 go -C sdk test ./forward/e2e -count=1 -v
  ```

  CI runs it in the "Forward Netns E2E" job: on the GitHub-hosted Ubuntu
  runner, a VM with passwordless sudo, so the test binary runs under sudo
  and no privileged container is needed (H14). It runs on pull requests in
  the `forward` change class (which covers `sdk/forward`,
  `sdk/api/forward` and `contracts/forward`), nightly and on manual runs,
  with a 20-minute timeout, and is not a required check until it has been
  green for two weeks (H14). Backend Tests shard 1 keeps running the
  nftables driver's real-kernel conformance tests (section 6.1) on every
  change, and the gost driver's conformance suite against the pinned gost
  (section 6.2). F4b adds a gost entry to this suite (`TestGost*`: the
  gost driver with the pinned gost in the entry's namespace, which the job
  downloads and checks; exact payload counters over TCP and UDP, IPv4 and
  IPv6, and failover through gost's web API with a connection held across
  it). F4c adds mixed-engine chains (`TestMixed*`): an nftables entry, a
  gost relay and two gost exit nodes before one target, over a RAW link
  and over a mutual-TLS mux link between relay and exits (link
  certificates of a test CA, planned with a cluster name): TCP and UDP
  reach the target and every hop counts them (the nftables entry packets
  and headers, the gost relay the payload exactly, the gost exit the
  payload over RAW and at least the payload over the carrier); adding and
  removing another route on the gost relay leaves the route's held TCP
  connection, its UDP session, its counter epoch and the gost processes
  alone; the primary exit's gost dies and the relay fails over to the
  other exit through `SetUpstreams`. A gost entry before an nftables exit
  carries TCP and UDP with exact counters on both.
- **The v4 API** (F5a, implemented): handler tests in
  `packages/forward/v4api` and `packages/forward/control`, the kernel
  gateway's in `internal/handler`, and `internal/tests/forwardv4`, which
  runs the API against the real `ForwardControl` over gRPC on SQLite and
  PostgreSQL.
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
  gost runs as another user with `CAP_NET_BIND_SERVICE` only, in the
  sandbox of its own unit (section 6.2). systemd sandboxing of the Agent:
  `NoNewPrivileges`, `ProtectSystem=strict`,
  `ReadWritePaths=/var/lib/anixops-agent /var/lib/anixops-gost` (the second
  is the gost driver's directory), `ProtectHome`, `PrivateTmp`,
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
- **No secrets in state.** Node-to-node TLS uses the nodes' own link
  certificates and keys (H28); the state names identities, not keys. The
  link CA is a separate root that signs only link certificates, so a link
  certificate never authenticates to Control and an Agent certificate never
  authenticates a link; gost holds only the link key, which the Agent
  generates for it, never the Agent's Control key. gost checks no
  revocation: a revoked link certificate (its node disabled, deleted,
  retired or its credentials replaced) verifies at peers until it expires,
  at most 7 days, while the peers' `ingress_sources` admission still
  admits only the routes' previous hops; removing the node from its routes
  removes its addresses there. gost's
  web API (F4b) and metrics listen on unix sockets in its runtime
  directory, guarded by file permissions (the unit's `UMask=0007` in a
  `RuntimeDirectory` of mode 0750: the gost user and its group, which only
  the Agent is in), so there is no API key at all. The API can change
  every gost object; nothing but the Agent may join that group.
- **Every change goes through Control**, authorized and audited there.

## 15. Rollout

Sizes: S (under 300 changed lines), M (300 to 1000), L (over 1000).
Agent-repository PRs are marked (agent).

| Phase | PR | Content | Size | Gate |
|---|---|---|---|---|
| F1 | F1a | this design, draft contract, draft fixtures | M | H11 |
| | F1b | `sdk/forward/model` and `validate` (implemented) | M | |
| | F1c | `sdk/forward/planner`: allocation, wiring, generations, golden runner (implemented) | L | |
| F2 | F2a | driver interface, fake driver, conformance suite (implemented) | M | |
| | F2b | nftables Render and nft goldens (implemented) | L | H13 |
| | F2c | nftables Apply, Observe, `SetUpstreams`, tc HTB, host probe, real-kernel conformance (implemented) | L | H13 |
| | F2d | netns end-to-end suite and CI job (implemented) | M | H14 |
| F3 | F3a | Control: `forward.v1`, `nodeconfig/v2`, the `forward` report and traffic ledger (implemented) | L | H25 |
| | F3a-L | Control: forward link CA and per-node link certificates (`IssueLinkCertificate`, `GetLinkTrustBundle`, implemented) | M | H28 |
| | F3b | (agent) forward component: drivers, persisted state, apply at boot, health loop, reports, link certificates | L | H25, H28 |
| | F3c | probes and diagnosis plumbing: `DiagnoseRoute`, the `agent.diagnostic` forward checks, `POST /routes/{id}/diagnose`, `forward routes diagnose` (implemented; the Agent's checks are an Agent PR) | M | |
| | O1 | `install.sh`, group tokens, mirrors | M | H18 |
| | O2 | preflight and offline package | M | H18 |
| | O3 | uninstall | S | |
| | O4 | staged upgrades with canary and rollback (Control side implemented; (agent) `upgrade.v1` and the updater follow) | L | H19 |
| F4 | F4a | gost driver: Render, process management (implemented) | L | H20 |
| | F4b | gost Observe, hot updates, failover (implemented) | M | H20 |
| | F4c | gost per-service structural changes, mixed-engine end-to-end (implemented) | M | |
| | L1 | least-connections re-weighting (implemented) | S | H21 |
| | L2 | entry HA via DDNS and CNAME (implemented) | L | H21 |
| F5 | F5a | Control forward package: `ForwardControl`, `/api/v4/forward/*`, `anix-control forward` (implemented) | L | H23 |
| | F5b | new forwarding UI: `/admin/forward/{overview,routes,inventory}` in the core app, shown with the forward package's v4 API; `can_delete` on the list answers, previews left out of the audit log (implemented; [`docs/design/forward-ui`](../design/forward-ui/README.md), [`docs/guide/forwarding.md`](../guide/forwarding.md)) | L | H16 |
| | F5c | upgrade: archive, check the nodes are clean (Control cleans NodeX and Ansible hosts), drop tables | M | H15 |
| | F5d | remove flux routes, `forwardcompat`, catalog entries; rewrite AGENTS.md rules; archive the flux docs | M | H17 |
| F6 | A0 | AnixOps relay transport design (`anixops-protocol.md`; secure transport only, approved 2026-10-04; camouflage reserved for the owner) | M | H22 |
| | A1–A5 | v4.2 experimental prototype, off by default (`forward.anixops_experimental` on Control and Agent): relay library, QUIC, driver and `anixops-relay` unit (agent), contract additions, benchmarks (`anixops-protocol.md` section 9) | L | H22 |
| | A6 | v4.3 production: wire version 1 frozen | M | H22, owner sign-off |

F1a–F1c and F2 do not depend on the Agent line. F3 needs AG-1. F5c runs
last, after every forward node runs the new Agent (section 10), and only
after its own confirmation.

**Golden policy.** As with KernelNodeOps before NO-1
(`node-ops-service.md` section 3.10), `anixops.forward.v1` is in
`contracts/proto/descriptors.golden` and the CI generated-code check. Until
F3a, which serves it, a design change could edit this package's own golden
lines in the PR that changed the proto. **Since F3a, additions only:** a
field, message, RPC or enum value is never removed, renumbered or retyped.
`internal/tests/protocompat` rejects an element that disappears, and
`config/scripts/check_proto_golden.py` (Documentation Sync Check job)
rejects a golden file that lost or rewrote a line of the base revision;
its `DRAFT_PACKAGES` is empty. F3a added only `Violation.route_id`; F5a
added the node, registry and traffic RPCs and messages of section 8.6 and
the `enforced` answers, and changed nothing else; F3c added the diagnosis
fields, enum values and `DiagnoseNode` of section 8.6; L2 added the DNS
provider, binding and route DNS RPCs and messages of section 8.6.

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
| H22 | AnixOps protocol design review (threat model, cryptography, REALITY-like fallback) | Transport design written: [`anixops-protocol.md`](anixops-protocol.md), with its questions P1–P10 (decided 2026-10-04); camouflage reserved for the owner (its section 8). Prototype off by default and marked experimental in v4.2 |
| H23 | Community vs commercial boundary for forwarding | Section 12: core forwarding, LB, failover and onboarding in both; self-service, plans, multipliers and resellers commercial |
| H28 | Forward link certificates for encrypted gost links (F3b, AgentPKI): gost verifies a certificate chain and the dialled server name, not SPIFFE URIs, and must not hold the Agent's Control key | AgentPKI issues each forward node a separate link certificate: DNS name = the node's identity name (`forward-41`, the planner's default `server_name`), URI = its SPIFFE identity, serverAuth and clientAuth, from a link CA (a separate root) that signs nothing else, with the same lifetime and rotation as the Agent certificate. The Agent writes it, its own key and the link CA bundle to `/var/lib/anixops-gost/tls` and reloads gost on rotation. An operator-chosen `server_name` (a CDN name on WSS) then needs that name in the exit's link certificate, or stays unsupported. Per-identity matching of `ingress_peers` would need a gost plugin; source admission plus the link CA is the v4.2 boundary |

Decided by the owner (2026-10-02; H20 and H21 2026-10-03):

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
- **H21** (defaults part, 2026-10-03): health checks every 5 s with a 2 s
  timeout, the breaker opens after 3 failures in a row for 30 s,
  least-connections re-weights every 10 s
  (`sdk/forward/model/defaults.go`). The DDNS providers settled with L2:
  Cloudflare, Alibaba Cloud DNS, DNSPod, Huawei Cloud DNS and a generic
  webhook, implemented in L2 (section 7.4); entry HA judges the entry nodes
  by their reports with the H21 health defaults and the same hysteresis
  of 3.
- **H20** (2026-10-03): one pinned gost v3 release per Agent release
  (3.2.6, MIT), shipped with the Agent and run as `anixops-gost.service`, a
  unit the Agent owns and manages, so Agent upgrades keep forwarding; NodeX
  is removed in v4.2. gost runs as its own user with `CAP_NET_BIND_SERVICE`
  only and the sandbox of section 6.2 (H13 gives `CAP_NET_ADMIN` to the
  Agent, not to gost).

Decided by the owner (2026-10-04):

- **Upgrade order:** the new Agent first, cleaning the old forward runtime
  on install; then every forward node switches to it; then Control v4.2,
  keeping the `required` default (section 10). M3-4 and M3-5 are cancelled.
- **H25:** anix-agent uses Control's version numbers and is released with
  it (for example `v4.2.0-rc.N` for both), and Control's CI pins the same
  Agent commit. Each Agent tag is still asked first.
- **H28** (forward link certificates): as recommended. A dedicated forward
  link CA, separate from the CA of modules, the kernel and Agents, issues
  each forward node a link certificate (DNS name = the node's identity
  name, its SPIFFE identity as URI, serverAuth and clientAuth), requested
  and renewed alongside the Agent certificate. gost holds only the link
  certificate and its key, never the Agent's Control key. Control's side
  is implemented (section 6.2, "Link certificates": a separate self-signed
  root rather than an intermediate, so the module CA's trust bundle never
  admits link certificates); the Agent's is F3b.
- **H22:** the AnixOps relay protocol document covers the secure
  transport between nodes only, and its transport design is approved
  (`anixops-protocol.md`, with P1–P10 decided as recorded in its section
  9.3: a separate `anixops-relay.service`; plaintext only on
  administrator routes whose nodes are both labelled `link=iepl` or
  `link=iplc`; nftables `RAW` handover to anixops in v4.3, the v4.2
  prototype anixops-to-anixops only; the rest as recommended). Camouflage
  (section 8 of that document) is still reserved for the owner.
- **H23:** as recommended (section 12). Core forwarding, load balancing,
  failover and onboarding in both editions; user self-service, forwarding
  plans, billing multipliers and resellers commercial; the AnixOps relay
  protocol in both editions. F5a builds the community side and reserves the
  commercial API prefixes (section 12).

H19 is still open; it is asked before the work it gates.

Decided by the owner (H18): as recommended. Flags `--token`,
`--group`, `--mirror control|cn|github`, `--offline <file>`; tokens
single-use, bound to a node or a node group, 1 hour by default and at most
7 days; `install.sh` served by Control at `/install.sh` and mirrored to
GitHub releases, signed; re-running the command is safe (extra instances
are not needed). O1 adds `--control` and `--node` (section 9: the signed
script is static) and binds tokens to nodes only for now.

Smaller questions raised by this design:

- The 60 s heartbeat for forward nodes (section 8.4): decided by default
  with F3a, the owner may revisit.
- Global quota across several entries, Control-authoritative with a local
  remainder (section 5.3): decided by default with F3a, the owner may
  revisit; the local remainder's rendering is deferred.
- Forward nodes (`v2_forward_node`) kept through the v4.2 upgrade as node
  inventory (section 10; H15)? Still open; F3a's inventory uses them.

## 17. Not in scope

- The AnixOps relay protocol's implementation (H22, v4.3; its transport
  design is `anixops-protocol.md`).
- User self-service forwarding, plans, auto-renewal and multipliers (v4.3),
  resellers and federation (v4.4).
- API keys with scopes and IP allow-lists, and MCP access to
  `ForwardControl` (later, on Control's API layer).
- Migrating v4.1 forwarding data (decided: archive only).
