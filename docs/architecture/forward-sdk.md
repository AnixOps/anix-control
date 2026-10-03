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
the traffic ledger, and `ForwardControl` for official packages. The
contract is `sdk/api/forward/v1` (`anixops.forward.v1`), binding since F3a:
additions only (section 15); the planner goldens are in
`contracts/forward/v1`. This is the v4.2 forwarding redesign. It replaces
the flux-panel clone (`/api/v2/forward/*`) in v4.2.

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
| Contract | `sdk/api/forward/v1` | `forward.proto`: the model, `NodeForwardState`, `NodeForwardReport`, the services `ForwardControl` and `ForwardNode` (F1a; binding since F3a) |
| Wire | `sdk/forward/wire` | how forwarding rides the Agent Control stream: the `forward.v1` Hello attribute, the `anixops.nodeconfig/v2` member, the forward report and their checks (F3a, implemented) |
| Model and validation | `sdk/forward/model`, `sdk/forward/validate` | Go domain types with lossless conversion to and from the contract and the defaults (`model/defaults.go`); one set of validation rules used by Control, the planner and the Agent (F1b, implemented) |
| Planner | `sdk/forward/planner` | routes and node inventory in, per-node states, port and mark allocations and generations out; pure functions (F1c, implemented) |
| Drivers | `sdk/forward/driver`, `.../driver/fake`, `.../driver/conformance`, `.../driver/nftables`, `.../driver/gost`, `.../driver/ansible` | the driver interface, registry, fake driver and conformance suite (F2a, implemented), the nftables driver (Render F2b, Apply, Observe, failover and tc F2c, implemented) and the other engines (F4) |
| Client | `sdk/forward/forwardctl` | a Go client for `ForwardControl` (F5) |

Consumers:

- **Agent** (anix-agent): embeds the drivers. Receives `NodeForwardState`
  in `config.v1` and reports `NodeForwardReport` as a `PackageReport`
  (section 8), both through `sdk/forward/wire`.
- **Control**: the kernel's forwarding state (`internal/kernelforward`,
  F3a) runs the planner and serves `ForwardControl` to official packages
  (section 8); a new forward package builds on it a native API under
  `/api/v4/forward/*`, the operator CLI `anix-control forward ...` and a
  new UI (F5).
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
- **Apply** is atomic (all of the artifact or the previous state) and
  compares the host with the artifact, not with its memory: applying what
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

Implemented in F3a: `internal/kernelforward` (the kernel's forwarding
state and `ForwardControl`), `internal/grpc/agent_control_forward.go` (the
stream) and `sdk/forward/wire` (the wire rules both sides share);
`sdk/api/agent/v1/PROTOCOL.md`, "Forwarding", is the Agent's reference.

### 8.0 The kernel's forwarding state

Nine new kernel tables, protected (no package can adopt them), written only
by `internal/kernelforward`:

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
  `planner.Stamp` from the stored generations. Nodes whose generation moved
  are written and their sessions pushed (8.1); a node that left the
  inventory with hops gets an empty state, so it never keeps a deleted
  route. Any violation refuses the whole plan: a route write is refused
  (8.6); an inventory change (a Hello with other capabilities, new
  settings) leaves every node on its generation, logs, and records the
  violations (`Service.PlanStatus`, gauge `anixops_forward_plan_refused`).
- **Internal Go API** (`kernelforward.Service`): route writes and reads,
  `PlanRoute`, `RouteStats`, `RouteHealth`, `Traffic` (the hourly ledger),
  `State`, `Nodes`, `SetNodeSettings`, `RecordHello`, `RecordReport`,
  `Replan`, `PlanStatus`, `Maintain` (the singleton worker's minute tick:
  expiry, request-id retention), and the package functions
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

### 8.3 Probes

`ForwardNode.Probe` is a `DesiredOperation` of kind `forward.probe` with a
`ProbeRequest` as payload; the `ObservedState` carries the `ProbeResult`.
F3c implements it; until then `ForwardControl.DiagnoseRoute` answers
`UNIMPLEMENTED`.

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
and the CLI on it.

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
   uninstaller (section 9), it touches no other table or service.
2. **Switch every forward node to the new Agent**, NodeX nodes included
   (H20). `anix-control agents transports --legacy-only` must list no node.
3. **Upgrade Control to v4.2**, with `agent_control.mtls` defaulting to
   `required` (H5).

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
  v2 routes go with F5d.
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
  land). F1c's planner tests produce them byte for byte (canonical
  protojson, two-space indented; `go -C sdk test ./forward/planner -run
  Golden -update` rewrites them) and run the negative cases; F1c added
  goldens for UDP with IPv6 targets, several entry nodes behind an entry
  hostname, a sticky re-plan, port exhaustion and a multi-route plan with
  generations. F2/F4 add the rendered nft and gost artifacts: the nft
  goldens are in `contracts/forward/v1/nft` (F2b, section 6.1).
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
  foreign object, impostor not owned, empty artifact removes; counters kept
  across re-apply, re-created hops, monotonic observation; failover and
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
  sudo ANIXOPS_FORWARD_E2E=1 /tmp/forward-e2e.test -test.v
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
  change. The gost driver joins the suite with F4c (mixed engines).
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
| | F1c | `sdk/forward/planner`: allocation, wiring, generations, golden runner (implemented) | L | |
| F2 | F2a | driver interface, fake driver, conformance suite (implemented) | M | |
| | F2b | nftables Render and nft goldens (implemented) | L | H13 |
| | F2c | nftables Apply, Observe, `SetUpstreams`, tc HTB, host probe, real-kernel conformance (implemented) | L | H13 |
| | F2d | netns end-to-end suite and CI job (implemented) | M | H14 |
| F3 | F3a | Control: `forward.v1`, `nodeconfig/v2`, the `forward` report and traffic ledger (implemented) | L | H25 |
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
| | F5c | upgrade: archive, check the nodes are clean (Control cleans NodeX and Ansible hosts), drop tables | M | H15 |
| | F5d | remove flux routes, `forwardcompat`, catalog entries; rewrite AGENTS.md rules; archive the flux docs | M | H17 |

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
its `DRAFT_PACKAGES` is empty. F3a added only `Violation.route_id`.

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

Decided by the owner (2026-10-02; H21 2026-10-03):

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
  (`sdk/forward/model/defaults.go`). The DDNS providers settle with L2.

Decided by the owner (2026-10-04):

- **Upgrade order:** the new Agent first, cleaning the old forward runtime
  on install; then every forward node switches to it; then Control v4.2,
  keeping the `required` default (section 10). M3-4 and M3-5 are cancelled.
- **H25:** anix-agent uses Control's version numbers and is released with
  it (for example `v4.2.0-rc.N` for both), and Control's CI pins the same
  Agent commit. Each Agent tag is still asked first.

H18–H20, H22 and H23 are still open; each is asked before the work it
gates.

Smaller questions raised by this design:

- The 60 s heartbeat for forward nodes (section 8.4): decided by default
  with F3a, the owner may revisit.
- Global quota across several entries, Control-authoritative with a local
  remainder (section 5.3): decided by default with F3a, the owner may
  revisit; the local remainder's rendering is deferred.
- Forward nodes (`v2_forward_node`) kept through the v4.2 upgrade as node
  inventory (section 10; H15)? Still open; F3a's inventory uses them.

## 17. Not in scope

- The AnixOps relay protocol's design and implementation (H22, v4.3).
- User self-service forwarding, plans, auto-renewal and multipliers (v4.3),
  resellers and federation (v4.4).
- API keys with scopes and IP allow-lists, and MCP access to
  `ForwardControl` (later, on Control's API layer).
- Migrating v4.1 forwarding data (decided: archive only).
