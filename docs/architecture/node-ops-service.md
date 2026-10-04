# Node Operations Service (KernelNodeOps), Node Credential Split and Agent A2

Status: DESIGN (2026-10-01), decided (section 10). NO-1 is implemented:
the kernel serves the contract `sdk/api/kernelnodeops/v1`
(`anixops.kernelnodeops.v1`), which is binding (section 3.10), with its
ledger, but executes no operation kind yet (section 3.11). NO-4 is
implemented: the gateway seals node secrets into handles, and the kernel
verifies request bindings (section 3.7). NO-7 is implemented: the kernel
executes the forward family and the gost API connection test, runtime job
payloads carry no token, and forward node tokens are pinned to their
endpoints (sections 3.8, 3.11 and 6.1). M3-4 and M3-5 are cancelled
(2026-10-04): v4.2's forwarding redesign deletes or rewrites their routes
(section 7). This is phase 3 of the 2026-10 plan, done together with Agent
line A2.

> 中文摘要：剩余桥接路由里，有 83 条在等“内核代办节点操作”和“节点凭据外置”，
> 另有 7 条在等节点/Agent 通道的决定。本文给出三件事的设计：
> - **KernelNodeOps 契约**：模块提交类型化、幂等的节点操作（应用转发、同步节点、
>   诊断、签发凭据），由内核持有凭据和 Agent 连接、选择通道执行，并返回回执和结果。
>   模块拿不到任何令牌或私钥：管理员请求和应答里的密钥由网关换成一次性句柄。
> - **凭据外置**：把凭据和密钥从 `v2_node`、`v2_forward_node`、`v2_node_protocol`、
>   `v2_wireguard_peer` 等表移到新的受保护表，走双写、回填、双读、定稿四步。
>   不 ALTER 旧表，旧列定稿后写入墓碑值。之后 proxy-node、protocol-runtime 和
>   forward 可以接管去掉凭据后的表。
> - **Agent A2**：配置、用户变更、流量、日志统一走 mTLS 的 Agent Control 流。
>   证书由模块 PKI 签发，SAN 为 `spiffe://anixops/<cluster>/agent/<node>`。
>   REST、WebSocket 和旧 v2board gRPC 保留一个大版本。
>
> 90 条路由的规划：75 条原生，15 条标为内核所有。
>
> 进度：NO-1 已完成。内核已提供 KernelNodeOps 契约和操作台账，契约自此定稿，只增不改。
> 但还没有任何操作类型可执行：提交这类操作会得到 `UNIMPLEMENTED`，且不留任何记录（第 3.11 节）。
> NO-4 已完成：对 `config/node-secret-fields.json` 列出的路由，网关把请求里的节点密钥换成一次性句柄，
> 应答里只还原为本请求生成的句柄；无法替换时由内核的旧处理器直接应答，模块看不到这个请求（第 3.7 节）。
> NO-7 已完成：内核可执行转发族四种操作（应用转发、隧道变更、后端同步、旧版规则）和 gost API 连通性测试；
> 转发运行时任务的载荷不再含节点令牌，旧载荷在启动时清洗；转发节点令牌钉在其管理端点上，
> 地址未经管理员确认时不出示令牌（第 3.8、3.11、6.1 节）。

## Contents

1. [Why](#1-why)
2. [Goals and non-goals](#2-goals-and-non-goals)
3. [Contract: KernelNodeOps](#3-contract-kernelnodeops)
4. [Node credential split](#4-node-credential-split)
5. [Agent A2: unified control plane](#5-agent-a2-unified-control-plane)
6. [Route mapping](#6-route-mapping)
7. [Rollout](#7-rollout)
8. [Risks](#8-risks)
9. [Test strategy](#9-test-strategy)
10. [Decisions for the owner](#10-decisions-for-the-owner)
11. [Not in scope](#11-not-in-scope)

## 1. Why

`package-extraction.md` section 3.2 leaves 112 routes `bridged`.

- 83 of them wait on "KernelNodeOps and the node credential split".
- 7 more wait on a decision about the node and agent channels.

The routes' hosts say what each one needs (the `bridgedRoutes` comments in
`packages/{forward,proxy-node,protocol-runtime,subscription}/control/service.go`).
There are five needs:

| Need | Examples | Where it lives today |
|---|---|---|
| **Node pushes** | apply a forward on its node, push a node's configuration, re-apply a tunnel's forwards, push a legacy rule | the forward runtime executors (`internal/service/forward_runtime_executors.go`): NodeX over HTTP, a local Ansible job, a clean agent job (`v2_forward_runtime_job`, bridged to the AnixOps Agent's task poll by `forward_agent_bridge_*`); `AgentControlManager` (`internal/grpc/agent_control_server.go`: `node.reload`, `users.reload`, `agent.ping`); the agent WebSocket (`internal/handler/agent.go`: diagnostic tasks); the legacy pull (UniProxy, the v2board gRPC node service) |
| **Diagnostics dialing out** | forward node and load balancer checks, forward and tunnel diagnoses, gost statistics sync | the kernel dials from Control's network, with the node's token where the node's API needs it |
| **Node caches** | the node cache (`nodeCacheKey`), the gost manager's address and token per node (`internal/gost/manager.go`), live agent connections and monitor snapshots | kernel memory |
| **Credentials and secrets** | node API keys and shared secrets, forward node tokens, clean agent tokens, registration keys, Reality and WireGuard private keys, WireGuard peer keys | columns of the protected tables `v2_node`, `v2_authorized_key`, `v2_forward_node`, `v2_forward_clean_agent`, `v2_forward_runtime_job` (payloads), `v2_node_protocol`, `v2_wireguard_peer` (`internal/service/plugin_capabilities.go`) |
| **Live connections** | the agent list, Agent Control status, task creation that waits for an acknowledgement | `AgentControlManager` and `AgentHandler` in kernel memory |

**What #92 changed.** Administrator answers no longer show node secrets:

- protocol settings and raw configurations read `********` at every secret
  key (`RedactNodeSecretsJSON`, `IsNodeSecretKey`);
- a forward node's token and a registration key are shown once, in the
  answer that creates them;
- saving the placeholder keeps the stored value (`KeepNodeSecretsJSON`).

So reading is no longer the blocker. What blocks is the rest:

- **writes:** a package that adopts a table can write its credential
  columns;
- **pushes:** the channels are in the kernel;
- **standing access:** whoever adopts a table can read every secret in it.

## 2. Goals and non-goals

Goals:

- **G1.** A package asks for typed, idempotent node operations and learns
  their outcome. Receipts, polling and a watch stream.
- **G2.** No package holds or sees a node credential or private key: not at
  rest, not in a contract answer, not in an administrator's request or
  answer (section 3.7).
- **G3.** After the credential split, the credential-free node tables can
  be adopted in place, like every other domain (`package-extraction.md`
  section 3.4).
- **G4.** One agent control plane: the mTLS Agent Control stream carries
  configuration, user changes, traffic, logs and operations (A2).
- **G5.** Every step can be rolled back until the operator finalizes the
  split. Everything is additive:
  - new tables, RPCs, fields and capabilities only;
  - no `ALTER` of an existing table;
  - v4.0.0 hosts keep working.

Non-goals:

- moving the forward runtime executors onto agent plugins (line A5);
- deciding where UniProxy and the subscription renderer live (phase 4,
  "F3");
- several kernel replicas (`container-deployment.md`);
- changing a v2 response shape.

## 3. Contract: KernelNodeOps

### 3.1 Shape

One gRPC service, `KernelNodeOps`, served by the kernel:

| RPC | Kind | Purpose |
|---|---|---|
| `SubmitOperation` | write | record and start one typed operation; idempotent by `request_id` |
| `GetOperation` | read | one operation, by `operation_id` or `request_id` |
| `ListOperations` | read | the caller's operations, filtered, paged |
| `WatchOperations` | server stream | every change of the caller's operations after a cursor |
| `CancelOperation` | write | stop an operation that has not ended |
| `ValidateNodeConfig` | read | the kernel's raw configuration and protocol validators |
| `ListAgentSessions`, `GetAgentSession`, `GetAgentMonitor` | read | the live agent sessions and their last reports, which only the kernel holds |
| `GetCapabilities` | read | the families the caller holds, the kinds the kernel serves, the split phase per table |

**Operations name resources, not payloads.**

- `ApplyForward{forward_id, action}` makes the forward's node match the
  forward's rows as they are when the kernel executes. The package never
  sends what to run.
- So a repeat, a late retry or two operations on one forward all converge
  on the latest state. A pending operation can be replaced by a newer one on
  the same resource (`SUPERSEDED`).
- **Deletions are the exception.** The kernel captures what to remove when
  the operation is submitted. The package deletes its rows only after the
  operation ends.

Synchronous v2 routes stay synchronous: `SubmitOperation` takes a wait mode.

| Wait mode | The call answers when |
|---|---|
| `NONE` | the operation is recorded |
| `ACCEPTED` | the channel took it: the agent acknowledged it, the job was queued, or the configuration a pulling node reads was rebuilt |
| `TERMINAL` | the operation ended, or `wait_timeout_ms` (60 s at most) passed |

The legacy routes wait in the same way: a forward change on gost answers
the NodeX result, and an agent task answers the acknowledgement. The native
routes pick the matching mode, so their answers are the same.

### 3.2 Families and capabilities

Each family needs its own signed capability, so a package holds only what
it uses. The capabilities are checked by `validateManifestCapabilities` and,
on every call, by `PackageHostOperations.AuthorizeCapability`, against the
calling host's current generation and verified signed release. Only
official AnixOps packages get them, as for KernelSubscriber and
KernelSettings.

| Capability | Operations and RPCs | Holders |
|---|---|---|
| `kernel.nodeops.forward.v1` | `ApplyForward`, `ApplyTunnel`, `SyncForwardBackend`, `ApplyLegacyRule` | forward |
| `kernel.nodeops.nodeconfig.v1` | `SyncNode`, `RetireNode`, `RetireProtocol`, `PutSecretDocument`; `ValidateNodeConfig` | proxy-node, protocol-runtime, forward (`SyncNode` and `RetireNode` on forward nodes) |
| `kernel.nodeops.diagnose.v1` | `CheckEndpoints`, `CollectNodeStats`, `DiagnoseForward`, `DiagnoseTunnel`, `RunAgentDiagnostic` | forward, proxy-node (load balancer check), protocol-runtime (agent tasks) |
| `kernel.nodeops.agents.v1` | `AgentControlOperation`; `ListAgentSessions`, `GetAgentSession`, `GetAgentMonitor` | protocol-runtime |
| `kernel.nodeops.credentials.v1` | `IssueCredential`, `RevokeCredential`, `IssueRegistrationKey`, `RevokeRegistrationKey`, `IssueCleanAgent` | proxy-node, forward |

- `GetOperation`, `ListOperations`, `WatchOperations` and `CancelOperation`
  need any one of the five, and answer only operations the caller owns
  (section 3.4). `GetCapabilities` needs none.
- **The kernel checks the target's kind too.** A forward node may not be
  the target of `RunAgentDiagnostic`. A proxy node is refused by
  `ApplyForward`. So a family cannot reach another domain's nodes.
- **Since NO-1 the grammar accepts the five names**
  (`service.NodeOpsCapabilities`). Any other `kernel.nodeops.*` name fails
  validation (`unknown kernel capability`).

**What a holder can do.** If a package is compromised, its capabilities
bound the damage.

| Family | A compromised holder can | It cannot |
|---|---|---|
| forward | re-apply, pause, resume or remove forwards (their rows are its own anyway) | read a token, point a token at another address (section 3.8) |
| nodeconfig | make the kernel rebuild and push a node's configuration from the rows; store secrets an administrator sent in the request it is serving; retire a node's kernel state | read a secret; push a configuration the kernel's validators refuse; plant a secret of its choosing (only handles sealed from the bound administrator request resolve) |
| diagnose | make the kernel dial nodes, and public targets for users | dial a private target for a user (the kernel applies the actor's rules from the request binding) |
| agents | ping or reload agents; read sessions and monitoring | send any other agent operation (the kind list is fixed) |
| credentials | issue or revoke node credentials; the generated value goes to the administrator's answer as a handle | see a credential's value, or issue one outside an administrator request (the handle needs the binding) |

### 3.3 Operation kinds

Each kind has a stable name. It is recorded with the operation and answered
in `Operation.kind`.

| Kind | Message | Family | Channels | What it does today, in the kernel |
|---|---|---|---|---|
| `forward.apply` | `ApplyForward` | forward | NodeX, local Ansible, clean agent job | `PanelForwardRuntimeService` apply, through the backend in force |
| `forward.tunnel` | `ApplyTunnel` | forward | fan-out of `forward.apply` | a tunnel update re-applies its forwards |
| `forward.sync_backend` | `SyncForwardBackend` | forward | fan-out of `forward.apply` | the administrator's backend sync |
| `forward.legacy_rule` | `ApplyLegacyRule` | forward | NodeX | `ForwardRuntimeProvider` (legacy rules) |
| `node.sync` | `SyncNode` | nodeconfig | Agent Control (`node.reload`; after A2, a configuration snapshot), legacy pull | `NodeHandler.SyncProtocol`, `NodeService.SyncProtocolToNode`, cache deletion in `UpdateNode` |
| `node.retire` | `RetireNode` | nodeconfig | kernel | the node deletion transaction (protocols, WireGuard peers, subscription group links) |
| `protocol.retire` | `RetireProtocol` | nodeconfig | kernel | the protocol deletion transaction (WireGuard peers, subscription group links) |
| `secrets.put` | `PutSecretDocument` | nodeconfig | kernel | the secret-keeping part of protocol and raw configuration writes (`keepNodeProtocolSecretUpdates`, `KeepNodeRawConfig`) |
| `diagnose.endpoints` | `CheckEndpoints` | diagnose | Control dial; agent after A2 | `ForwardNodeService.HealthCheck`, `LoadBalancerService` health check |
| `diagnose.node_stats` | `CollectNodeStats` | diagnose | the node's gost metrics endpoint, local Ansible | `ForwardHandler.SyncNodeStats`, the Ansible statistics |
| `diagnose.forward` | `DiagnoseForward` | diagnose | Control dial; agent after A2 | `PanelForwardService.DiagnoseForward` (public targets only for users) |
| `diagnose.tunnel` | `DiagnoseTunnel` | diagnose | Control dial; agent after A2 | `PanelForwardService.DiagnoseTunnel` |
| `agent.diagnostic` | `RunAgentDiagnostic` | diagnose | agent WebSocket, Agent Control after A2 | `AgentHandler.CreateTask`, `ExecuteCommand` (whitelist, `v2_agent_diagnostic_task`, acknowledgement) |
| `agent.operation` | `AgentControlOperation` | agents | Agent Control | `NodeHandler.DispatchAgentControlOperation` (`agent.ping`, `node.reload`, `users.reload`) |
| `credential.issue` | `IssueCredential` | credentials | kernel | node creation, forward node token handling, rotation |
| `credential.revoke` | `RevokeCredential` | credentials | kernel | clean agent revocation |
| `regkey.issue`, `regkey.revoke` | `IssueRegistrationKey`, `RevokeRegistrationKey` | credentials | kernel | `GenerateAuthKey`, `InternalGenerateAuthKey`, `DeleteAuthKey` |
| `cleanagent.issue` | `IssueCleanAgent` | credentials | kernel | `ForwardCleanAgentService.CreateToken` |

The kernel keeps its executors. KernelNodeOps is a typed, authorized and
durable front door to code that already runs. The legacy handlers move onto
the same functions, as KernelOrder's legacy callbacks did. Native and
legacy then write identical rows.

### 3.4 Request ids and the ledger

- **Request ids.** Every `SubmitOperation` carries a `request_id` of at most
  128 bytes. It is unique per intent, and its prefix names the domain. Routes
  derive it from the request's `Idempotency-Key`, else from its request id,
  as the other contracts do.
  - Examples: `forward.apply:<forward>:<action>:<digest>`,
    `forward.tunnel:<tunnel>:<digest>`,
    `node.sync:proxy-<id>:<digest>`,
    `credential.issue:forward-<id>:token:<digest>`,
    `regkey.issue:<digest>`,
    `agent.diag:<node>:<digest>`.
  - **Retirements carry no digest:** `node.retire:proxy-<id>`,
    `protocol.retire:<id>`. Ids are never reused, so every retry of a
    deletion converges.
  - **The legacy handlers derive the same ids.** So a retry applies once,
    whichever side serves it.
- **Ledger.** The operation table is the ledger. The new table
  `v4_kernel_node_operation` has `request_id` unique. Each row holds:
  - the digest of the canonical operation;
  - the owner: the package of the route family (forward, proxy-node or
    protocol-runtime), also when the kernel's legacy handler submitted it;
  - who submitted it: `<package>@<generation>`, or `kernel:<route id>`;
  - family, kind, targets, state, channel, attempt and node revision;
  - the parent and the fan-out counters;
  - links to `v3_kernel_operation` (the durable Agent dispatch) and
    `v2_forward_runtime_job`;
  - deadline, times, result and error.
- **Repeats.**
  - The same id with the same digest answers the first operation, with
    `applied: false`.
  - The same id with another digest, or another owner, is
    `FAILED_PRECONDITION`.
  - A target that does not exist is `NOT_FOUND` and records nothing.
- **Canonical form.** The canonical operation never holds a secret. Sealed
  handles are resolved in the submitting call and replaced by the stored
  secrets' ids before the digest is computed.
- **Retention.** Terminal operations are kept 90 days, the dedupe window
  of the other ledgers, and pruned hourly. Results are capped at 64 KiB.
- **Events.** `v4_kernel_node_operation_event` is an append-only log:
  cursor, operation, owner, state, time. Every state change appends one row
  in the same transaction. Rows are kept 7 days.

### 3.5 Status: polling and the watch stream

States are `PENDING`, `DISPATCHING`, `RUNNING`, then one of the terminal
states: `SUCCEEDED`, `FAILED`, `CANCELLED`, `TIMED_OUT` or `SUPERSEDED`.

- **Terminal is monotonic.** The transitions, and the rule that a terminal
  state never changes, are those of `KernelOperationBridge.recordObserved`.
- **A late result does not reverse a decision.** A result that arrives after
  a deadline is recorded as evidence and does not change the state. A later
  `SYNC` operation converges the node.

Polling and watching:

- **Poll.** `GetOperation` by id or request id, or `ListOperations` filtered
  by family, state or target. After a restart, a package lists its
  non-terminal operations.
- **Watch.** `WatchOperations(after_cursor)` streams `CHANGED` events with
  the operation as it is. A cursor older than the log answers `RESYNC` with
  a fresh cursor, as `WatchSubscriberChanges` does. The kernel polls the
  event log every second and re-authorizes the stream every 30 seconds.
- **Fan-out.** `ApplyTunnel` and `SyncForwardBackend` create one child
  `forward.apply` per forward, with `parent_operation_id`. The parent
  counts its children (`FanOut`) and ends when they all have: `SUCCEEDED`
  when none failed, else `FAILED` with the counts.

### 3.6 Generation fencing

Four generations guard an operation.

1. **The caller's package generation.** Every call is authorized against
   the host's current lifecycle generation (`AuthorizeCapability`).
   - A fenced generation's calls are `PERMISSION_DENIED`, so an old
     instance cannot start operations after an upgrade.
   - Operations it already submitted run to completion. They belong to the
     owner package, not to the instance, and the new generation sees them.
2. **The node's desired revision.** On the Agent Control stream, every
   dispatch takes the node's next revision (`AgentControlManager`), and the
   agent reports work older than its observed revision as `SUPERSEDED`. That
   is the agent's ordering guarantee. The kernel records the revision in
   `Operation.node_revision`.
3. **The resource.** Operations are level-triggered (section 3.1), so the
   last execution wins.
   - The kernel runs at most one operation per resource at a time, per
     forward and per node configuration.
   - It supersedes pending duplicates.
   - A deletion's captured state is fenced against a newer creation of the
     same forward id: ids are not reused.
4. **The agent session.** Acknowledgements and observations from a replaced
   session are refused (`resolveAck`, `recordObserved`).
   - After a reconnect, the kernel replays retained operations in revision
     order, and the agent's completed-operation cache answers terminal
     states without running them again.
   - After a kernel restart, `KernelOperationBridge` recovers durable
     operations, and NodeOps operations dispatched on the stream go through
     the same bridge.

### 3.7 Results without secrets

A package learns everything it needs and never a secret.

1. **Operations carry ids, never secrets.** The kernel loads credentials
   when it executes. Forward runtime job payloads stop embedding node
   tokens: the executor resolves them when it runs the job (NO-7), and a
   scrub rewrites stored payloads.
2. **Results are typed and scrubbed.**
   - Every result message carries what the legacy answer shows: runtime
     status and message, the acknowledgement, the diagnosis outcomes,
     counters.
   - Free text from a node, NodeX or Ansible is scrubbed: every credential
     the operation used, and anything `IsNodeSecretKey` marks, is replaced by
     `********` before it is stored or answered. NodeX error texts can echo
     a token.
3. **Status, not values.** New views show whether a credential exists, its
   version and when it was rotated (section 4.6). They never show a value
   or hash.
4. **Sealed handles at the gateway.** Some v2 routes carry secrets in the
   administrator's request or answer:
   - a protocol's Reality private key, a raw configuration's WireGuard key,
     a forward node token the administrator types;
   - the API key and secret shown once when a node is created.

   For these routes the kernel's gateway does the substitution, in both
   directions, so the package host never reads the value. Each route and
   field is listed in a kernel-owned table, `config/node-secret-fields.json`,
   which packages cannot change.
   - **Inbound.** Before dispatching to a host, the gateway replaces each
     secret value in the listed fields with a handle,
     `anix-sealed:v1:<random>`. Within JSON documents, the values at
     `IsNodeSecretKey` keys are replaced. The placeholder `********` and
     empty values are left alone.
   - **Use.** The package passes handles to `PutSecretDocument` and
     `IssueCredential`, with the request binding (the `bridge_capability` of
     its dispatch).
     - The kernel resolves a handle only within the request it was sealed
       for, and only for the bound route's targets.
     - A forward node's token sealed from `PUT /admin/forward/nodes/7`
       stores only for forward node 7.
   - **Outbound.** A generated secret is answered as a `SecretHandle`. The
     package puts the handle where the v2 answer shows the secret, and the
     gateway expands handles minted for this request in this answer only.
     The bytes the administrator receives are what the legacy handler
     answers.
   - **Lifetime.** Handles live in kernel memory until the request's
     deadline.
   - **Failure.** If a listed route cannot be substituted (a body that is
     not JSON where JSON is expected), the gateway serves it legacy. It
     fails closed.
   - **Shadow mode.** The legacy side gets the original body, and the
     comparison masks handles.

   **What NO-4 built** (`internal/sealedsecrets`, the gateway in
   `internal/compat/v2`, the binding in `internal/packagebridge` and
   `internal/kernelnodeops`):

   - **The field list.** `config/node-secret-fields.json` names, per route
     id, a target and the fields of each direction. The kernel binary embeds
     it (`config.NodeSecretFields`). The route gate
     (`check_plugin_only_routes.py`) checks every route id against
     `package-extraction.json`, and each path parameter target against the
     route's path.

     | Route | Request fields | Answer fields | Target |
     |---|---|---|---|
     | `POST /admin/nodes` | `/raw_config` (document) | `/data/api_key`, `/data/secret` | new proxy node |
     | `PUT /admin/nodes/:id` | `/raw_config` (document) | | proxy node `:id` |
     | `PUT /admin/nodes/:id/raw-config` | `/raw_config` (document) | | proxy node `:id` |
     | `POST /admin/nodes/validate-config` | `/raw_config` (document) | | none: never resolves |
     | `POST /admin/nodes/:id/protocols` | the five protocol columns (documents) | | new protocol |
     | `PUT /admin/nodes/:id/protocols/:protocol_id` | the five protocol columns (documents) | | protocol `:protocol_id` |
     | `POST /admin/forward/nodes` | `/api_token` (value) | `/data/api_token` | new forward node |
     | `PUT /admin/forward/nodes/:id` | `/api_token` (value) | | forward node `:id` |
     | `POST /admin/auth-keys`, `POST /internal/auth-keys` | | `/data/key` | new registration key |
     | `POST /admin/forward/agents` | | `/data/token` | new clean agent |
     | `POST /admin/forward/test-connection` | `/api_token` (value) | | `dial`: no resource (NO-7) |

   - **Handles.**
     - **Format.** A handle is `anix-sealed:v1:` followed by 43 unpadded
       base64url characters, 32 random bytes (`v2compat.IsSealedHandle`).
     - **Binding.** Each is bound to its request (a random key the bridge
       capability carries, the package generation, the request id and the
       route), the route's target and its field.
     - **Use.** A handle resolves once, all of a resolution or none.
     - **Lifetime.** It lives in kernel memory only (`sealedsecrets.Store`)
       and dies when the gateway finishes the request, or at its deadline.
   - **Substitution.**
     - **Which values.** In a listed route's body the gateway seals:
       - each listed value field;
       - in each listed document, inline or as a JSON string, every value
         under a key `IsNodeSecretKey` marks;
       - anywhere else in the body, every value under such a key.

       A non-empty array under a secret key is sealed whole, as the masked
       answers hide it.
     - **Spelling.** Member names match a listed field whatever their case,
       underscores or hyphens, as the legacy handlers bind them.
     - **What passes.** The placeholder, empty values and null pass.
     - **The rest of the body.** Only the sealed values are rewritten; every
       other byte stays as sent.
     - **Legacy reads the original.** The host reads the sealed body. The
       bridge capability keeps the original for the legacy handler
       (`DispatchInput.BridgeBody`).
   - **Field names.** A handle's field is the listed pointer as the list
     spells it, followed in a document by the secret's own pointer, for
     example `/reality_settings/private_key`.
   - **Targets.** A target is the resource named by a path parameter. A
     route that creates its resource binds its target at the first
     resolution: every later one in the request must name the same resource.
   - **Bindings are verified.** `SubmitOperation` verifies a request
     binding on the session the call arrived on, local or module listener
     (`packagebridge.BoundRequest`).
     - The capability must be live: minted for the calling generation, not
       consumed, revoked or expired.
     - Anything else is `PERMISSION_DENIED` before `Prepare`, and nothing is
       recorded.
     - A `Preparer` gets the verified binding as `Submission.Request`, with
       the request id, the route and the actor. It resolves handles with
       `Submission.Unseal`.
   - **Generated secrets.** An executor mints a handle with
     `Run.Reveal(name, value)`, for a name the route's answer lists; the
     value is also scrubbed from the operation's texts.
     - The ledger stores the result with each `SecretHandle`'s field and
       expiry, never its handle.
     - The handles are answered once, to the submitting call bound to the
       same request.
   - **The ledger never holds a handle.** A request id, reason or operation
     holding one is refused (`INVALID_ARGUMENT`, or `INTERNAL` when a
     `Preparer` left one). The canonical form keeps only the bare prefix
     where a `SecretRef` was (section 3.4).
   - **Expansion.**
     - The gateway expands a handle only at a listed answer field, under its
       name, minted for this request, once.
     - Any other handle of a request in flight anywhere in the answer or its
       headers is refused with 502 `sealed_secret_refused`, and the request
       is not served again.
     - A string with a handle's shape that is no live handle, outside a
       listed field, is data and passes.
   - **Fail closed.**
     - **Legacy fallback.** A request whose secrets cannot be sealed is
       served by the kernel's legacy handler without the package host
       (`pluginhost.Supervisor.DispatchLegacy`). That covers a body that is
       not JSON, a value field that is not a string, a target parameter that
       is not an id, a sealed body over the request limit, or an unavailable
       field list.
     - **Refusal.** A route without a legacy handler is refused with 503
       `sealed_secret_unavailable`.
     - **Metric.** Both are counted in
       `anixops_v2_gateway_sealed_secrets_total{package,route,stage,result,reason}`,
       with fixed labels and no handle.
   - **Shadow mode.**
     - The SDK router gives a shadow run no binding (`NativeRequest.Binding`
       is nil). Its request's capability is consumed by the legacy answer,
       and its handles die with the request, so they never resolve.
     - The shadow comparison (`v2compat.EqualForCompare`) masks handles on
       both sides: a handle matches any string the other answer shows there.
     - `packagecompat` compares the same way.
   - **Deviations from the design above.**
     - **Every mode is sealed.** A listed route is sealed in every mode,
       legacy included, not only in native and shadow. The host polls its
       mode, so the kernel cannot tell when it leaves legacy, and the legacy
       handler reads the original anyway.
     - **The fallback skips the host.** It runs in the kernel, so the
       package never sees an unsealed request.
     - **Handles are never echoed.** A request's own handle is not expanded
       in its answer: an executor reveals what an answer shows.
     - **A dial target.** `POST /admin/forward/test-connection` (gost-mesh,
       native-flagged) is listed since NO-7 with target kind `dial`: the
       typed token is sealed, resolved for the bound request only, and
       presented once by the kernel to the address the request names
       (`diagnose.forward_backend`, section 6.1). A `dial` target names no
       resource: a resolution names the kind and no id, and the store
       binds nothing. The package's connection test submits the handle and
       waits for the result; it never dials.
5. **No call reveals a stored secret.** No KernelNodeOps call answers an
   existing credential's value, even as a handle. The one route whose answer
   is a stored secret, `GET /admin/nodes/:id/credentials`, is
   `kernel-owned` (section 6).

Without handles, packages would still have no standing access: a package
would see only the secrets an administrator types or is shown, in the
request it serves. That is what a bridged relay sees today. Section 10, D2,
offers that cheaper variant.

### 3.8 Channels, endpoints and validation

The kernel chooses the channel. The package never names one, except that
`AgentControlOperation` is always the stream.

| Channel | Used for | Code today |
|---|---|---|
| `KERNEL` | credential and secret operations, retirements | `internal/service` |
| `AGENT_CONTROL` | `node.sync` and `agent.operation`; after A2, configuration, users, diagnostics | `AgentControlManager`, `KernelOperationBridge` (`v3_kernel_operation`) |
| `AGENT_WEBSOCKET` | `agent.diagnostic` until A2 | `AgentHandler.dispatchWithAckRetry` |
| `LEGACY_PULL` | `node.sync` for nodes without a stream: nothing is pushed, and the node's next periodic UniProxy or v2board pull reads the rebuilt configuration | `NodeService.SyncProtocolToNode` (a no-op that logs today) |
| `NODEX` | `forward.apply` (gost backend), `forward.legacy_rule` | `nodeXForwardRuntimeClient` |
| `LOCAL_ANSIBLE` | `forward.apply` (nftables and iptables backends), Ansible statistics | the forward runtime job executor |
| `CLEAN_AGENT_JOB` | `forward.apply` (clean agent backend) | `v2_forward_runtime_job`, `ForwardAgentBridgeService` |
| `CONTROL_DIAL` | `diagnose.*` with the Control vantage | `net.Dial` in the kernel |

**The durable dispatcher.** Stream operations go through
`KernelOperationBridge`. Two things change in NO-6:

- the dispatchable kinds grow beyond `plugin.*`;
- the dispatcher runs whenever KernelNodeOps is enabled, not only with
  `plugins.dispatch_enabled`.

The bridge's `recoverOnce` assumes one replica, which Control is today.

**An address is a credential.** A package that can write a forward node's
`host` or `api_port` could point the kernel's next NodeX or gost call, which
carries the node's token, at an address it controls. That is the "a write
can be a read" rule of `settings-service.md`. So a forward node's token is
pinned to the endpoint it was issued for:

- `v4_kernel_node_credential.endpoint` holds that endpoint;
- when the row's address no longer matches, the kernel does not present the
  token there, and the operation fails with `ENDPOINT_UNCONFIRMED`;
- the pin moves only in a `SyncNode` bound to an administrator's
  `PUT /admin/forward/nodes/:id` (or the Ansible machine equivalent) for
  that node, and only to the host and port in that request's body. The
  kernel retained that body, so the package cannot choose the address.

**The rule as NO-7 built it** (`nodesecrets.ForwardNodeTokenAt`,
`service.ForwardNodeAPIToken`):

- **What is compared.** The token is presented only when the row's current
  `host:api_port` is the credential row's `endpoint`, and both are set. The
  host is compared as written, the port as a number. A changed host, a
  changed port, a removed port and a port added after the pin are each
  another endpoint: `ENDPOINT_UNCONFIRMED`, not retryable, with a message
  that names no address and no value. Nothing is sent.
- **Where it applies.** Every call that carries the token to an address:
  NodeX requests for the gost backend and the legacy rules (both nodes of a
  rule must be confirmed), the NodeX translation of a clean agent job, and
  the gost manager's clients. Job backends carry no node token (section
  3.11), so they need no pin. The legacy routes and the kernel's executors
  run the same functions, so a legacy forward change on an unconfirmed node
  fails the same way.
- **A node without an API port** has no endpoint: its pin is empty and its
  token is presented nowhere (decided by the owner, 2026-10-01). It cannot
  be used by the gost backend or a legacy rule until an administrator sets
  its API port, which pins it. `node-secrets status` lists such nodes by id
  and name (`forward_nodes_without_api_port`). This closes the gap NO-2
  left open: an attacker could otherwise clear the port, move the host
  while "unpinned", and set the port again.
- **What moves the pin.** The kernel's own forward node writers
  (`ForwardNodeService.Create`, `Update` and the Ansible machine routes),
  which re-derive the endpoint from the row they just wrote
  (`nodesecrets.Sync`). Today every write of `v2_forward_node` goes through
  them, driven by the administrator's `PUT /admin/forward/nodes/:id`; had
  the table been adopted (M3-4, cancelled: section 7), only the `SyncNode`
  bound to that request would move it (NO-5/NO-6). A write that bypasses
  them, a package's or a direct SQL one, leaves the pin where it was.
- **Before the backfill** a node has no credential row and so no pin: its
  token is presented as before the split, and the read is counted
  (`anixops_node_secrets_pin_total{reason="unpinned"}`). Run
  `node-secrets backfill` after the upgrade to pin every node. An
  unconfirmed address is counted with `reason="unconfirmed"` and logged
  once per node. The rule applies in every split phase: the pin is the
  kernel's record, not a secret.

The rule lapses once the forward runtime runs on agent plugins (A5): agents
dial Control, and Control no longer dials nodes with tokens.

**Validate on build.** Today the kernel validates a protocol or raw
configuration only on its own writes. After adoption, packages write those
rows. So the configuration builder (UniProxy, the gRPC node service, the
A2 snapshot) validates every row before it uses it:

- a row that fails is left out of the node's configuration and reported
  (`NodeSyncResult.excluded_protocols`, a metric, a log line);
- secret positions take their values only from the kernel's secret table
  (section 4). A literal secret value in an adopted row counts as absent.
  So a package cannot plant a key it knows.

This starts in report-only mode in NO-3. Enforcement is decision D7.

### 3.9 Serving

The kernel serves the contract like KernelSettings and KernelOrder:

- on local package bridge sessions;
- on the mTLS module listener, where a call resolves to the bound
  instance's generation.

A host without a contract connection keeps its node routes legacy.

**Quotas.** Each package may have at most 256 non-terminal operations
(`RESOURCE_EXHAUSTED` beyond), and each node at most 32.

**Audit.** Every submission and every state change is visible to
administrators:

- through `v4_kernel_node_operation`;
- through a kernel admin route, `GET /api/v4/kernel/node-operations` (NO-1);
- credential operations also record the audit entries their legacy
  handlers record.

The admin route is read-only, behind the `/api/v4` administrator
authentication. It answers every package's operations, newest first:

- **Filters:** `package_id`, `family`, `kind`, `state`, `request_id`,
  `operation_id`, `parent_operation_id`, and `target_kind` with
  `target_id`.
- **Paging:** `before` (a cursor) and `limit` (at most 500).
- **Each row:** who submitted it (`<package>@<generation>`), the
  canonical operation, targets, resource, channel, attempt, fan-out
  counts, the scrubbed result and error, any late evidence, the cancel
  request, and the times.

### 3.10 Versioning: binding since NO-1

**Choice (D1): package `anixops.kernelnodeops.v1`, binding since NO-1.**
It was never `v1alpha1`.

Why:

- **An alpha name buys nothing here.** `internal/tests/protocompat` freezes
  every listed element: golden lines may be added, never removed.
  - A `v1alpha1` package in the golden would be frozen too.
  - Its lines would stay forever after a `v1` replaced it.
  - Every package would have to change its imports, and the kernel would
    serve two packages during the switch.
- **The other contracts are `v1`** and grow additively. KernelNodeOps looks
  the same now that it is served.

**Until NO-1 the draft was unreleased by mechanism, not by name:** no kernel
served it, its capabilities were not in the grammar, and the proto and Go
package doc said DRAFT, UNRELEASED. NO-1 ended all three:

- the kernel serves it on local bridge sessions and the module listener
  (section 3.9);
- the grammar accepts the five `kernel.nodeops.*` capabilities (section
  3.2);
- the proto and the Go package doc say the contract is binding.

**Golden policy.** The contract is in `contracts/proto/descriptors.golden`
and in the CI generated-code check, like every other contract.

- **From NO-1 on, normal rules apply:** additions only. A field, message,
  RPC or enum value is never removed, renumbered or retyped.
- **Before NO-1** a design change could edit the draft's own golden lines
  in the PR that changed the proto. That exception has ended. NO-1 changed
  no element, only comments.
- An `sdk/v*` tag cut before NO-1 carries the draft's comments. Its
  elements are the binding ones.

**Kinds without an executor.** A kernel serves the contract before it
executes every kind (NO-5 to NO-8 add the executors). Section 3.11 says
how such a kind behaves. `GetCapabilities.kinds` is how a package tells
what its kernel executes.

### 3.11 What NO-1 implements

**The engine** (`internal/kernelnodeops`):

- the ledger and its event log;
- the state machine;
- the dispatcher, with an executor registry (`kernelnodeops.Registry`,
  `DefaultExecutors`);
- the watch stream, quotas, fan-out counting and cancellation;
- the admin listing.

One engine runs per kernel database (`kernelnodeops.EngineFor`). The local
bridge and the module listener share it. Its dispatcher runs in the
process that holds the singleton-worker lease (`cmd/server`, R6).

**Unexecuted kinds fail in a defined way.** No kind has an executor in
NO-1.

- **At submission:** `SubmitOperation` for a kind without an executor
  answers `UNIMPLEMENTED`, after authorization, validation and the
  request-id replay check. Nothing is recorded and nothing is applied.
  - A retry after the kernel gained the executor applies; it is not
    answered with a stale refusal.
  - A repeat of a request id recorded earlier still answers its receipt.
  - An operation kind the kernel does not know at all (a newer contract's
    oneof case) is `UNIMPLEMENTED` too.
- **At dispatch:** an operation recorded while its kind had an executor,
  but dispatched by a kernel without one (a rollback), ends `FAILED` at
  once, with `INTERNAL`, retryable, "operation kind ... has no executor in
  this kernel". It never waits for an executor that is not there.

**Executors** implement `Execute(ctx, *Run) Outcome`:

- `Run.Accept` moves the operation to RUNNING and records the channel,
  node revision and links.
- `Run.UseSecret` names the credentials whose values are scrubbed from the
  outcome.
- `Outcome` is one of `Succeeded`, `Failed`, `Cancelled` or `FanOut`.
- An optional `Preparer.Prepare` runs in the submitting call: it uses the
  verified request binding (`Submission.Request`), resolves handles
  (`Submission.Unseal`) and captures deletions (NO-4, NO-5, NO-7).
- `Run.Request` is the verified binding while its request is live, and
  `Run.Reveal` mints the handle of a generated secret for its answer
  (section 3.7).
- The context ends at the operation's deadline, or when it is cancelled.

**Details the sections above leave open, as NO-1 settles them:**

- **Targets.** `v4_kernel_node_operation_target` holds one row per target
  node, so per-node quotas and `ListOperations(target)` are queries.
  Section 4.2 listed two operation tables; this is the third.
- **Writes are serialized.** Every ledger write runs under one lock: a
  PostgreSQL advisory lock, or SQLite's write lock taken first. So event
  cursors commit in order, a watcher never steps over an event still being
  written, and the quota counts are exact.
- **Deadlines.** An operation's deadline is 10 minutes after submission,
  for every kind (`Engine.Timeout`). Fan-out children inherit their
  parent's. An operation still pending at its deadline ends `TIMED_OUT`
  without running. An executor that fails after its deadline is recorded
  `TIMED_OUT`.
- **Resources.** Each kind names the resource it changes: `forward:<id>`,
  `tunnel:<id>`, `forward-backend`, `legacy-rule:<id>`,
  `nodeconfig:<kind>-<id>`, `protocol:<id>`,
  `secret:<scope>:<owner>:<column>`, `credential:<kind>-<id>` or
  `regkey:<id>`. Diagnoses and agent operations change none.
  - One operation runs per resource at a time.
  - Only `forward.apply`, `forward.tunnel`, `forward.sync_backend`,
    `forward.legacy_rule` and `node.sync` are level-triggered: a newer one
    supersedes a pending one of the same kind on the same resource.
  - Exceptions: only a deletion supersedes a pending deletion, and only a
    forced sync supersedes a pending forced sync.
- **Target kinds.** A node kind an operation does not take is
  `PERMISSION_DENIED`. A malformed reference is `INVALID_ARGUMENT`.
  - `node.sync` and `node.retire` take proxy and forward nodes.
  - `diagnose.endpoints` and `diagnose.node_stats` take forward nodes.
  - `agent.*` operations take proxy nodes.
  - Credentials must be ones the subject's node kind holds.
- **Fan-out.**
  - Children are recorded with request ids `fanout:<parent>:<n>`; package
    request ids may not start with `fanout:`.
  - Children must be of the parent's family, and may not fan out again.
  - Children are not counted against the quotas: their parent was.
  - A superseded child counts as succeeded, since the operation that
    replaced it carries its work.
  - A child whose target is gone is recorded `FAILED` (`TARGET_GONE`) and
    counted as failed.
  - Cancelling the parent cancels its children. The parent ends with its
    last child.
- **Cancellation.**
  - A pending operation ends `CANCELLED` at once.
  - A started one has its executor's context cancelled; `CancelOperation`
    waits up to 2 s and answers the operation as it is.
  - An outcome the executor reports despite the cancellation stands.
  - A cancellation recorded by another process reaches the executor
    through the ledger, on the dispatcher's next tick.
- **Restarts.** At start, the dispatcher settles what a stopped process
  left started:
  - cancelled operations end `CANCELLED`;
  - fan-outs go on with their children;
  - level-triggered operations are dispatched again;
  - anything else ends `FAILED` (`INTERNAL`, retryable), since whether it
    applied is unknown.
- **Unknown fields.** An operation that carries a field the kernel does not
  know is `INVALID_ARGUMENT`. The kernel does not ignore an option it
  cannot honour.
- **Results.** A stored result is capped at 64 KiB. A larger one is
  dropped; the operation keeps its state and gets an `INTERNAL` error
  saying so.
- **Request bindings.** A binding is passed to a `Preparer` and never
  stored. Since NO-4 the kernel verifies it on the session the call arrived
  on: a binding that names no live request of the calling package is
  `PERMISSION_DENIED` before anything is prepared or recorded (section
  3.7). The binding is kept in memory for the operation's executor until
  its request ends, and is lost on a restart.
- **Split phases.** `GetCapabilities.tables` answers `LEGACY` for the seven
  tables of section 4.1 until the split state is plugged in. Since NO-9 it
  is (`SplitPhasesFrom`): each split table's phase, `FINALIZED` only once
  finalize completed, and `LEGACY` for `v2_forward_runtime_job`.

**What NO-5 built** (`internal/kernelnodeops` `kinds_credentials.go`,
`kinds_retire.go`, `kinds_secrets.go`, `validate.go`; the shared functions
in `internal/service/node_credential_ops.go`; `nodesecrets.Retire`):

- **Kinds.** The credentials family (`credential.issue`,
  `credential.revoke`, `regkey.issue`, `regkey.revoke`, `cleanagent.issue`),
  the nodeconfig family's `node.retire`, `protocol.retire` and `secrets.put`,
  and the `ValidateNodeConfig` RPC. `GetCapabilities` lists the eight
  kinds. Every executor runs on the `KERNEL` channel, in one transaction of
  the kernel database.
- **One implementation.** Each executor runs the function of
  `internal/service` its legacy route runs, and the legacy routes were
  moved onto those functions first: node creation and deletion, the raw
  configuration and protocol writes and deletions, registration keys, clean
  agent creation and revocation, and the forward node deletion. Every
  `nodesecrets.Sync` and every agent certificate revocation (#107, A2-1)
  stays in the transaction it was in. The legacy answers are pinned byte
  for byte (`TestNodeCredentialAnswers`, written before the move) and
  unchanged.
- **Secrets in.** A typed secret (a forward node token, a protocol or raw
  configuration secret) arrives as a sealed handle. The `Preparer` resolves
  it with `Submission.Unseal` for the bound request, the operation's target
  and the field the gateway sealed it from (`/api_token`, or the column's
  pointer followed by the secret's, `/reality_settings/private_key`), all
  or none, and keeps the values in kernel memory for the executor
  (`Submission.Retain`, `Run.Prepared`), with the binding: they die with
  the request and with a restart. An operation dispatched after that stores
  nothing and ends `FAILED` (`SECRET_HANDLE_INVALID`). The operation keeps
  the bare prefix where each handle was. A handle outside a secret position
  is `INVALID_ARGUMENT`.
- **Secrets out.** A generated secret (a node's API key and secret, a
  forward node token, a registration key, a clean agent token, an
  `anixagt_` enrollment credential) is minted with `Run.Reveal` inside the
  transaction that stores it: when the bound answer cannot show it (no
  binding, the request ended, the route lists no field of that name) the
  transaction rolls back and nothing is issued. The names are the answer
  fields the field list spells: `api_key`, `secret`, `api_token`, `key`,
  `token`, and `credential` for enrollment. A typed secret is never
  echoed. Issuing a credential outside an administrator's request (no
  binding) is `PERMISSION_DENIED` before anything is recorded.
- **IssueCredential.** A proxy node's key and secret, both or one, and a
  forward node's token, generated or typed. An existing credential is
  replaced only with `replace`: otherwise `FAILED_PRECONDITION` in the
  submitting call. Replacing revokes the node's agent certificates and
  enrollments (`credentials_replaced`), as the legacy update does. A forward
  node's token keeps its endpoint pin through `nodesecrets.Sync` (NO-7
  moves the pin). `AGENT_ENROLLMENT` mints a one-time credential through
  `agentpki.CreateEnrollmentToken` (section 5.3), bound to the subject, 24
  hours, hashed at rest, with the legacy audit entry; the actor in the
  entry is `kernelnodeops:<package>`. No listed route answers a
  `credential` field yet, so the kind issues nothing until one does. A clean
  agent's token is issued with `IssueCleanAgent` only.
- **RevokeCredential.** A clean agent (the legacy revocation: status,
  `revoked_at`, the split row), or a node's agent enrollments and
  certificates (`AGENT_ENROLLMENT`, `credentials_revoked`). A proxy node's
  key or secret and a forward node's token are not revoked in place: no
  kernel route does, and the unique `api_key` index leaves no empty state
  to revoke into. They are rotated with `replace` or retired with the node.
- **Registration keys and clean agents** are issued and revoked as
  `POST /admin/auth-keys`, `DELETE /admin/auth-keys/:id`, `POST
  /admin/forward/agents` and `POST /admin/forward/agents/:id/revoke` do; a
  key gone since the submission is nothing to do.
- **Cascades (D11).** `RetireNode` on a proxy node deletes its protocols
  with their users' WireGuard peers, their subscription group links and
  their secrets, the node's credentials and raw configuration secrets in
  the split tables (`nodesecrets.Retire`), and revokes its agent
  certificates (`node_deleted`); on a forward node, the token's split row
  and the certificates. `RetireProtocol` deletes the peers, links and
  secrets. The node or protocol row is the package's: the legacy deletion
  removes it in the same transaction, a package after the operation
  succeeded. A retirement of what is already gone succeeds with nothing
  counted; a failure anywhere rolls the whole cascade back, retryable.
- **PutSecretDocument.** The document, with the resolved values at its
  handles and the stored values at its placeholders
  (`KeepNodeSecretsJSON`), is written to the legacy column and the split
  table, as the protocol and raw configuration routes write it; the result
  is the redacted document (`RedactNodeSecretsJSON`, the bytes the masked
  answers show) with the counts of secrets stored, kept and cleared. A
  document with only placeholders needs no binding. In a finalized table
  (P3) the writer's `Sync` then rewrites the legacy column to the redacted
  document (NO-9).
- **ValidateNodeConfig.** Runs `ValidateRawNodeConfig` (the route's
  decoding, WireGuard check and `server_port` warning, answered as an issue
  at `/server_port`) or `ValidateNodeProtocol` on a protocol row's document.
  A handle or the placeholder at a secret position counts as present: it is
  replaced by a stand-in before the validators run, a generated WireGuard
  key pair for `server_private_key` and its public key. The answer's
  `message` is the route's.
- **Tests.** Each executor on SQLite and PostgreSQL: success, refusals
  (unknown target, a node kind or credential kind the operation does not
  take, a handle of another request, target or field, no binding), the
  idempotent repeat by request id, the legacy column and split row in
  agreement in `dual_write` and `dual_read`, the walk showing no secret or
  handle in any stored result; the cascade rolling back as one; an engine
  round trip over the bridge (`bridgecontract`); the typed and generated
  secrets through the real gateway, bridge and executors
  (`internal/tests/sealedhandles`).
**What NO-6 implements: node configuration and agents.** The kernel
executes `node.sync` (nodeconfig), `agent.operation` (agents) and
`agent.diagnostic` (diagnose), and serves the agent session RPCs.
`GetCapabilities.kinds` lists the three.

- **The dispatcher takes node kinds.** `internal/agentstreams.Streams` is
  the kernel's view of the Agent Control streams by node kind;
  `internal/grpc.AgentStreams` implements it over the proxy and forward
  managers, since their ids overlap. The executors dispatch through it and
  follow the agent's observed states until the terminal one (the
  acknowledgement moves the operation to `RUNNING` with the stream
  revision in `node_revision`; `SUCCEEDED`, `FAILED` and `SUPERSEDED` end
  it; a cancellation sends `operation.cancel`; the deadline ends it
  `TIMED_OUT` and a late report is evidence only). The NodeOps ledger is
  the durable record of these dispatches: `v3_kernel_operation` stays the
  plugin operations' (its rows and `v3_kernel_node_operation_revision` key
  by proxy node id only, and the kernel never alters a table), so
  `kernel_operation_id` stays empty for them. After a kernel restart the
  engine's recovery applies: `node.sync` runs again, the others end
  `FAILED` (retryable).
- **`node.sync`.** `kernelnodeops.SyncNode` rebuilds the node's desired
  configuration (section 5.5), stores it, drops the kernel's node cache,
  and, when the node's agent holds a stream, pushes `node.reload` through
  the existing control-stream operation path once the hash changed or the
  sync is forced. A node on the legacy transports ends `SUCCEEDED` on
  `LEGACY_PULL`: its next periodic pull reads the stored configuration.
  `NodeSyncResult` carries the channel, `config_hash`, `config_revision`
  (field 8, added), `changed`, and the push's operation id, revision and
  acknowledgement. Since A2-3 an agent that negotiated `config.v1` is
  pushed a `ConfigSnapshot` instead of `node.reload`, and the operation
  ends on its `ConfigStatus` (section 5.5).
- **`agent.operation`.** `kernelnodeops.DispatchAgentOperation`: the three
  kinds, the acknowledgement wait bounded by `timeout_seconds` (10 s by
  default, 60 s at most), then the terminal state. A node without a
  stream is `NODE_OFFLINE`; a kind the agent does not advertise is
  `CAPABILITY_MISSING`; a refused acknowledgement is `AGENT_REJECTED` with
  the agent's reason; a stream that closes before the acknowledgement is
  `NODE_OFFLINE` (retryable).
- **`agent.diagnostic`.** `kernelnodeops.RunAgentDiagnostic` validates the
  action against the whitelist (parameters normalized), writes the task
  row, then sends the task: as an `agent.diagnostic` operation (payload
  `{"task": ...}`, the WebSocket's `task.assign` payload) to an agent that
  advertises the capability of that name, else on the node's WebSocket
  with the legacy task message as the fallback (`AGENT_WEBSOCKET`). The
  stream path completes the task row from the agent's report
  (`state_json`: `success`, `output`, `error`, `duration_ms`).
- **Agent session RPCs.** `ListAgentSessions` (both transports, both
  node kinds, ordered by node kind, id and transport), `GetAgentSession`
  and `GetAgentMonitor` for a proxy node, to holders of
  `kernel.nodeops.agents.v1`. A session shows its node, transport,
  version, instance, capabilities, times, revisions and the identity it
  authenticated by (the SPIFFE ID, or `api-key`); never a key, token or
  certificate. What an agent reported (system information, observed
  states, monitor snapshots) is scrubbed like a result.
- **The legacy routes** `POST /admin/nodes/:id/sync`,
  `POST /admin/nodes/:id/agent-control/operations`,
  `POST /admin/agent/tasks` and `POST /admin/agent/execute` call the same
  functions and answer the same bytes (`internal/handler`'s parity tests
  pin them); the sync route writes the desired configuration row too.
- **Tests.** `internal/tests/fakeagent` is the scripted Agent Control
  client of section 9 against the real listener (API key and
  certificate), and `internal/tests/nodeopsagent` runs the executors and
  the RPCs on SQLite and PostgreSQL: the stream and legacy paths, the
  agent's refusals and failures, a disconnect before and after the
  acknowledgement, a replaced session, cancellation, the deadline, and a
  walk of every answer for the fixture's credentials.
**Forward operations, as NO-7 built them** (`kernelnodeops.Forward`). The
kernel executes `ApplyForward`, `ApplyTunnel`, `SyncForwardBackend` and
`ApplyLegacyRule`, and `GetCapabilities` lists them. No route switches to
native, and none will: M3-4 and M3-5 are cancelled (section 7).

- **One implementation.** Each kind runs the legacy routes' code
  (`PanelForwardService.ApplyForwardRuntime`,
  `ForwardRuleService.ApplyRuntime`) over the executors the kernel has: the
  NodeX client for the gost backend and the legacy rules, the local Ansible
  job executor and the clean agent job queue. The legacy routes call the
  same functions, so their answers are unchanged byte for byte
  (`TestForwardOperationsAnswers`, written before the move).
- **What `forward.apply` does.** It loads the forward and its tunnel when
  it runs (level-triggered: a repeat converges on the rows as they are) and
  applies the action through the backend in force for that forward:
  `CREATE`, `UPDATE`, `PAUSE`, `RESUME` and `SYNC` as named, `DELETE` and
  `FORCE_DELETE` as the runtime's delete. The forward's runtime columns are
  written as every legacy change writes them; the status column stays the
  package's, as it is the caller's in every legacy route.
  - On the gost backend NodeX is called in the operation (`NODEX`,
    accepted when the call starts), and the result is NodeX's answer.
  - On a job backend the operation is accepted when the job is queued
    (`LOCAL_ANSIBLE` or `CLEAN_AGENT_JOB`, with the job id in the receipt)
    and follows the job to its end, reading it every 250 ms: the job's
    result or error is the operation's. A job still queued when the
    operation is cancelled or times out stays queued; the executor runs it,
    and the next operation on the forward converges.
  - The deletion is not captured at submission: the package deletes its
    rows only after the operation ends (section 3.1), so the rows are
    there when the kernel executes.
  - `FORCE_DELETE` succeeds whatever the node answered, with the failure in
    its result, so the package may delete the forward. A forward gone
    since the submission is `TARGET_GONE`; a refusing node is
    `BACKEND_FAILED`, retryable; an unconfirmed endpoint is
    `ENDPOINT_UNCONFIRMED` (section 3.8).
- **Fan-outs.** `forward.tunnel` creates one `forward.apply UPDATE` per
  active forward of the tunnel, as the legacy tunnel update applies them
  (the contract's comment said `SYNC`; `UPDATE` keeps the job rows the
  legacy route writes). `forward.sync_backend` takes the target backend from
  the new `SyncForwardBackend.backend` field (the backend in force when
  empty), records the target on every active forward not on it, and
  creates one `forward.apply SYNC` per forward. The legacy backend sync
  records the target only after a forward applied; the kernel records it
  first, so a failed child leaves the forward on the target backend with
  its runtime status failed, where the administrator sees it. A fan-out
  without children ends at once.
- **Legacy rules.** `forward.legacy_rule` pushes the row as it is through
  NodeX, with both nodes' tokens resolved at send time: `CREATE`, `UPDATE`
  and `DELETE` as named, `PAUSE`, `RESUME` and `SYNC` as the runtime's
  sync. The result says the rule was applied on both nodes, or carries the
  refusal.
- **Job payloads without tokens.** A `v2_forward_runtime_job` payload no
  longer holds the ingress node's token (`apiToken` is omitted). The gost
  executor resolves the token when it sends the request to NodeX, the
  bridge worker when it asks NodeX to translate a clean agent job, each at
  the node's pinned endpoint only; the stored task parameters NodeX
  answers are scrubbed too. A clean agent's claim and the administrator's
  job list serve a payload written before NO-7 scrubbed, and a start-up
  pass in the singleton worker process rewrites those rows
  (`docs/UPGRADE.md`), so both shapes are served during the transition.
- **Results.** `ForwardApplyResult` and `LegacyRuleResult`, scrubbed: the
  nodes' tokens and every value at a secret key are masked, in results,
  errors and the forward's runtime message.

## 4. Node credential split

### 4.1 What moves

| Table | Column(s) | What it is | New home |
|---|---|---|---|
| `v2_node` | `api_key`, `api_key_hash` | the node's API key, and its SHA-256 used for lookups | `v4_kernel_node_credential` (kind `node_api_key`) |
| `v2_node` | `secret` | the node's shared signing secret | `v4_kernel_node_credential` (`node_shared_secret`) |
| `v2_node` | secrets inside `raw_config` | WireGuard and other private keys of the raw configuration | `v4_kernel_protocol_secret` (scope `node_raw_config`) |
| `v2_authorized_key` | `key`, `key_hash` | registration keys, which mint node credentials | `v4_kernel_node_credential` (`registration_key`) |
| `v2_forward_node` | `api_token` | the forward node's token: its agent, and the relay's gost API | `v4_kernel_node_credential` (`forward_node_token`, with the pinned `endpoint`) |
| `v2_forward_clean_agent` | `token` | the clean agent's token | `v4_kernel_node_credential` (`clean_agent_token`) |
| `v2_forward_runtime_job` | tokens inside `payload` | node tokens copied into clean agent job payloads | none: resolved when the job runs; old payloads scrubbed |
| `v2_node_protocol` | secrets inside `settings`, `tls_settings`, `transport_settings`, `reality_settings`, `custom_config` | Reality and TLS private keys, WireGuard server keys, Shadowsocks keys, passwords, and so on (`IsNodeSecretKey`) | `v4_kernel_protocol_secret` (scope `node_protocol`) |
| `v2_wireguard_peer` | `private_key`, `preshared_key` | every user's WireGuard keys | `v4_kernel_protocol_secret` (scope `wireguard_peer`) |

### 4.2 New tables

All new tables carry the `v4_kernel_` prefix, so they are protected by
`protectedTablePrefixes` with no new rule.

- **`v4_kernel_node_credential`**
  - Columns:
    - `id`;
    - `subject_kind` (`proxy`, `forward`, `clean_agent`,
      `registration_key`, `agent_enrollment`);
    - `subject_id`, `kind`, `version`;
    - `key_hash`: SHA-256 hex, for authentication lookups;
    - `value`: the secret, needed where the kernel presents it (NodeX, the
      gost API, job payloads, the credentials route);
    - `sealed`, `kek_id` (section 4.5);
    - `endpoint`: the pinned `host:port` for forward node tokens;
    - `status` (`active`, `retired`, `revoked`), `expires_at`;
    - `source` (`backfill`, `dual_write`, `issued`);
    - `created_at`, `rotated_at`, `revoked_at`.
  - Unique on (`subject_kind`, `subject_id`, `kind`, `version`).
  - Indexed on (`kind`, `key_hash`).
- **`v4_kernel_protocol_secret`**
  - Columns:
    - `id`;
    - `scope` (`node_protocol`, `node_raw_config`, `wireguard_peer`);
    - `owner_id` (the protocol, node or peer id);
    - `column_name`;
    - `json_pointer`: RFC 6901; empty for a whole value or a peer key;
    - `value`, `sealed`, `kek_id`, `version`, `updated_at`.
  - Unique on (`scope`, `owner_id`, `column_name`, `json_pointer`).
- **`v4_kernel_node_secret_split`**
  - One row per table being split: `table_name` (key), `phase`,
    `backfilled_rows`, `digest`, `verified_at`, `finalized_at`,
    `finalized_by`.
  - This is the state machine of section 4.3. `GetCapabilities` answers it.
- **Operation tables** (section 3.4): `v4_kernel_node_operation`,
  `v4_kernel_node_operation_event` and, since NO-1,
  `v4_kernel_node_operation_target` (section 3.11). They exist since NO-1.
- **Desired configuration** (A2, section 5.5):
  `v4_kernel_node_desired_config`, one row per node: `node_kind`,
  `node_id`, `revision`, `config_hash`, `excluded`, `updated_at`.

### 4.3 Migration path

Existing tables are never altered. The old columns stay, and finally hold
tombstones. Each table moves through four phases, and the kernel records
the phase in `v4_kernel_node_secret_split`. P1 starts with the release that
ships NO-2. Every later phase is an operator command, never a release.

| Phase | Writes | Reads | Old binary rollback | Enter with |
|---|---|---|---|---|
| **P0 legacy** (today) | old columns | old columns | n/a | (default) |
| **P1 dual-write** | old columns and new tables, in one transaction, through one writer (`internal/nodesecrets`) | old columns | yes | the release that ships NO-2: dual-write is always on, and a backfill copies the existing rows (`anix-control node-secrets backfill`, idempotent, in batches by id, checkpointed) |
| **P2 dual-read** | both | new tables. A missing or mismatching row falls back to the old column and counts `anixops_node_secrets_fallback_total{table,kind,reason}` | yes | `anix-control node-secrets verify`: the digests over every secret of the old and new forms match; then `anix-control node-secrets phase <table\|all> dual_read` |
| **P3 finalized** | new tables only; old columns get tombstones (section 4.4) | new tables only | **no**: `unsplit` first (or restore from backup) | `anix-control node-secrets finalize -confirm <table\|all>`, after a zero fallback count. It rewrites every old secret to its tombstone or redacted form in batches, verifies, and records `finalized_at` |
| **P4 adopted** | packages write the credential-free columns | | | a package release that declares `kernel.storage.adopt:<table>` (section 4.6) |

**Every kernel writer goes through the single writer from P1 on:**

- node creation, registration and update;
- forward node creation and update;
- clean agent creation, registration and revocation;
- registration key creation and use;
- protocol creation, update and deletion;
- raw configuration updates;
- WireGuard peer creation and rotation (`wireguard_rotation.go`, the
  subscription renderer).

The writer derives the new rows from what it writes to the old columns.
So P1 cannot drift, and the parity tests compare both forms.

**Every reader moves in P2:**

- node authentication: the gRPC interceptor (`authenticateNode`), the node
  middleware (`NodeAuth`, `NodeAPIKeyAuth`, `NodeAPIKeyHeaderAuth`: UniProxy
  and agent package downloads), the agent WebSocket, forward node token
  checks, clean agent authentication, registration;
- the node configuration builder and the subscription renderer;
- the NodeX payloads and the gost manager.

Each reads through `internal/nodesecrets`, which applies the phase. A static
gate (NO-9) refuses any other kernel read of a moved column.

**Built in NO-3.** Every reader above reads through `internal/nodesecrets`
(`read.go`), which applies its table's phase:

- **The readers.**
  - Node API keys: `NodeAuth` (UniProxy), `NodeAPIKeyAuth` and
    `NodeAPIKeyHeaderAuth` (the node API, package downloads), the gRPC
    interceptor and the Agent Control stream (`NodeService.GetNodeByAPIKey`),
    and the agent WebSocket and HTTP routes (`verifyForwardNodeToken`).
  - The shared secret of `SignatureAuth`; registration keys in
    `RegisterNode`; forward node tokens in the agent checks, the gost
    manager and the NodeX payloads (which also fill clean agent job
    payloads); clean agent tokens in `Register`, `Heartbeat` and `Report`.
  - Protocol secrets in `BuildNodeProtocolConfig` (UniProxy and the gRPC
    configuration) and in the subscription renderer; the raw configuration
    in UniProxy; WireGuard peer keys in subscriptions and in the node's user
    list; the administrators' credentials route.
  - The A2-1 enrollment bootstrap (`agentpki.Enroll` with a node API key or
    a forward token), and the forward node update's token-change check that
    revokes agent certificates. The listener's legacy stream path shares
    `GetNodeByAPIKey`.
- **The rule.** Phases are read from `v4_kernel_node_secret_split` and
  cached for 5 seconds per process.
  - In `dual_write` a reader reads the legacy column exactly as before and
    never the new tables.
  - In `dual_read` a credential check accepts a match in the new table, and
    otherwise the legacy column. A value is the new table's where it equals
    the legacy one or the legacy column no longer holds it (empty, a
    tombstone, the placeholder); otherwise the legacy value is used.
  - Each use of the legacy column is counted in
    `anixops_node_secrets_fallback_total{table,kind,reason}`, with reason
    `missing`, `mismatch` or `error`, and logged once per subject (row id and
    JSON pointer), never with a value. The name follows the `anixops_`
    prefix of Control's other metrics.
  - A JSON column is rewritten only where its legacy document holds the
    placeholder, so an answer stays byte for byte as before while both forms
    agree.
- **The gate.** `anix-control node-secrets phase [-by <name>] <table|all>
  dual_read|dual_write`:
  - `dual_read` needs the table's latest verification to have matched: no
    mismatch, not followed by a failing one, at most an hour old
    (`VerifyMaxAge`) and not dated in the future;
  - `dual_write`, the way back, is always allowed;
  - `finalized` and `legacy` are neither asked for nor left;
  - `all` moves every table or none;
  - each change writes an audit entry (`v2_operation_log`, module
    `node_secrets`) in the same transaction.
- **Tombstones.** No reader accepts `!moved:<id>` or `********` as a
  credential in any phase, presented or stored, including the plain-key
  fallback for rows without a hash. A request signed with a placeholder
  secret is refused.
- **Validate on build, report-only (D7).** Before building a node's
  configuration from a protocol or a raw configuration, the builder checks
  its secrets:
  - no secret position holds the placeholder or a tombstone;
  - a WireGuard entry's server key pair is valid, a Reality private key is
    32 bytes, a Shadowsocks 2022 server key has its cipher's length.

  A failing row is counted in `anixops_node_secrets_invalid_total{table,
  type,reason}` and logged once naming the node, protocol and field. It is
  still built. `anix-control node-secrets validate` scans every row and
  exits 3 on a finding.
- **Not moved.**
  - The forward node inventory filter (`api_token = ''`) is a presence check,
    not a read of a value. NO-9 moved it (below).
  - Enrollment has no registration-key method; registration keys are read
    only by `RegisterNode`.

**Built in NO-9: finalize and unsplit** (`internal/nodesecrets`
`finalize.go`, `tombstone.go`; `anix-control node-secrets finalize` and
`unsplit`; the procedure is in `docs/UPGRADE.md`). Nothing runs them by
themselves; production finalize waits for the owner (D6).

- **finalize** needs `-confirm`, phase `dual_read` and the dual_read gate's
  verification (the latest verify matched, within `VerifyMaxAge`); one
  refused table refuses all, and a refusal changes nothing.
  - It sets the phase to `finalized` with `finalized_at` empty, in one
    audited transaction (`finalize_started`). From then on the readers read
    the new tables only and every writer's `Sync` writes tombstones.
  - It rewrites the legacy rows in batches by id. Each batch is one
    transaction that locks its rows, brings the new tables in line with them
    (as `backfill`), then writes the tombstones of section 4.4. Already
    finalized values are skipped, so it is idempotent.
  - A pass that rewrites nothing records `finalized_at` (`finalized`).
    Three passes at most; a writer of an older binary still writing secrets
    makes it stop and say so.
  - An interrupted finalize (`finalized`, no `finalized_at`) resumes when
    run again, without a new verification.
- **Unique indexes.** Every tombstone is `!moved:<row id>`, unique per row.
  A row that already holds `!moved:<another row's id>` (an import, a
  package's insert) would collide: the batch stops with
  `ErrTombstoneCollision`, naming both row ids, and rolls back.
- **The writers of a finalized table** are unchanged: `Sync` reads the phase
  in its transaction, uncached, derives the new rows from what the writer
  wrote, then rewrites those legacy rows to their finalized form. A
  tombstone or the placeholder written back (a row loaded and saved) keeps
  the secret; an empty value removes it.
- **The readers** of a finalized table never fall back. A literal secret in
  a finalized JSON column (written past the writers) is ignored: the
  position takes the new table's value, or the placeholder.
- **verify** of a finalized table compares by presence: every secret a
  tombstone or placeholder stands for has its new row, and a secret still in
  a legacy column is a difference.
- **unsplit** needs `-confirm` and phase `finalized` (or `dual_read`, to
  resume). It refuses a table a package's storage lease adopted unless
  `-adopted-ok`. It moves the phase to `dual_read` and drops the views that
  exist only after finalize in one transaction, writes the secrets back in
  batches, and verifies. JSON columns get their original bytes back:
  finalize keeps each original document (and the empty key hash of an old
  row) in `v4_kernel_protocol_secret`, scope `legacy_original`, which no
  reader reads and which goes with its row. The round trip is byte for byte.
- **An older binary after finalize.** Readers of this and the P2 releases
  refuse the tombstones on the legacy path, so a binary reading the legacy
  columns fails closed (tested). Binaries before P2 compared the legacy
  columns directly and would accept a presented tombstone, and P2 writers
  would write secrets back: `docs/UPGRADE.md` requires every process on
  this release before finalize, and `unsplit` before a downgrade.
- **The moved reader.** The forward node inventory and the status command
  ask whether a node has a token through
  `nodesecrets.ForwardNodeTokenPresent`: a live credential row (active, with
  a value) in `dual_read` and `finalized`, the usable legacy value before.
  Other reads that skipped the split moved too: the default registration key
  from the environment (`RegistrationKeyExists`), the diagnosis scrubber's
  tokens, and the forward node update's token-change check (a tombstone
  saved back is no change).
- **The static gate** (`config/scripts/check_moved_columns.sh`, in CI and
  the gate list) type-checks `./internal/...` and `./cmd/...` and refuses a
  read of a moved credential field (`model.Node.APIKey`, `.APIKeyHash`,
  `.Secret`, `model.AuthorizedKey.Key`, `.KeyHash`,
  `model.ForwardNode.APIToken`, `model.ForwardCleanAgent.Token`,
  `model.WireGuardPeer.PrivateKey`, `.PresharedKey`), or a moved column
  named in a gorm call's SQL, outside `internal/nodesecrets`. Writes, tests,
  `internal/model`, `cmd/migrate` and `cmd/sqlite2postgres` are allowed;
  every other exception is listed with its reason, and a stale one fails.
  The JSON settings columns are not gated: their non-secret parts are read
  everywhere, and their secret positions come from `ResolveProtocols`.
- **Conditional adoption.** A manifest may declare
  `kernel.storage.adopt:` for `v2_node`, `v2_node_protocol` and
  `v2_forward_node`; `service.EffectiveStorageGrants` honours it at the
  lease only once `nodesecrets.AdoptionAllowed` (finalized, with
  `finalized_at`), and leaves the views of section 4.6 out until they
  exist. `GetCapabilities.tables` answers each table's phase
  (`SplitPhasesFrom`), `FINALIZED` only once `finalized_at` is recorded.

**Rollback:**

- **P1 or P2 to P0:** set the phase back (`anix-control node-secrets
  phase all dual_write`). The writers dual-write in every phase since P1,
  so `dual_write` is the way back: the readers read only the old columns.
  The old columns still hold every value, so an older binary works.
- **After P3:** only `anix-control node-secrets unsplit` (or a backup)
  writes values back. `unsplit` is the documented escape hatch; it does not
  undo adoption.
- **Production is not touched by this phase** (owner decision,
  2026-10-01). Finalize is rehearsed on local staging. Running it on
  production comes later, at least one release after P2, and needs the
  owner's approval (D6).

**Conditional adoption.** A table's `kernel.storage.adopt:<table>` grant
is honoured only once the table is finalized. Before that:

- the lease leaves the table out (`LeaseStorageResponse.adopted_tables`);
- the host's native routes for it report `mode_unsupported` and stay
  legacy.

So one package release works on every installation, whatever its phase.
The protected-table rule becomes "protected unless finalized" for exactly
`v2_node`, `v2_node_protocol` and `v2_forward_node`. The others stay
protected (section 4.6).

### 4.4 Tombstones and unique indexes

Writing an empty string where a unique index applies would collide on the
second row. Tombstones are unique and non-empty, and can never authenticate.

| Column | Tombstone | Why it cannot authenticate |
|---|---|---|
| `v2_node.api_key` (unique) | `!moved:<node id>` | the kernel looks up `v4_kernel_node_credential.key_hash`; `!` never occurs in generated keys |
| `v2_node.api_key_hash` | `""` | no SHA-256 is empty; nothing reads it after P3 |
| `v2_node.secret` | `!moved:<node id>` | a tombstone signs nothing (`Unusable`) |
| `v2_authorized_key.key` (unique) | `!moved:<key id>` | |
| `v2_authorized_key.key_hash` | `""` | |
| `v2_forward_node.api_token` | `!moved:<node id>` | "has a token" comes from the credential row (`ForwardNodeTokenPresent`, `kapi_node_credential_status_v1`) |
| `v2_forward_clean_agent.token` (unique, not null) | `!moved:<agent id>` | |
| `v2_wireguard_peer.private_key`, `.preshared_key` | `!moved:<peer id>` | the renderer reads peer keys from `v4_kernel_protocol_secret` |
| JSON secret columns | the redacted document: `RedactNodeSecretsJSON` of the original, with `********` at every secret position | the builder takes secret positions from `v4_kernel_protocol_secret` only (section 3.8) |

NO-9 changed three rows of this table from `""` to `!moved:<id>`
(`v2_node.secret`, `v2_forward_node.api_token`, the peer keys). An empty
value is a writer's "no secret" (an Ansible machine's cleared token), so a
writer of a finalized table could not tell a tombstone from a removal. An
empty shared secret would sign every request for a binary on the legacy
path, and P2 writers delete the credential behind an empty column. A
non-empty tombstone keeps all three cases apart, and every reader since P2
refuses it. Decided by the owner, 2026-10-01: `!moved:<id>` for every moved
secret column.

Two more rules:

- **Rows packages insert.** A package that adopts `v2_node` inserts rows
  with `api_key = "!moved:" + <uuid>`, because the unique index needs a
  value. The node's credential comes from `IssueCredential`.
- **Byte parity.** The redacted document is exactly what `RedactNodeProtocol`
  and `RedactNode` answer administrators today, so native answers built from
  stored rows equal the legacy answers byte for byte:
  - a document without secrets is unchanged;
  - a value that is not JSON becomes `********` whole, which the raw
    configuration GET refuses on both sides, as now.

### 4.5 Secrets at rest

- **Default (parity):** values are stored in clear in the protected tables,
  as in the old columns today, with the same table protection.
- **Option:** values sealed with AES-256-GCM under a new
  `node_secrets.kek`, with `kek_id` recorded, like `identity.kek` and the
  module CA KEK.
  - Lookups use `key_hash`. Values are opened only where the kernel
    presents them.
  - The move makes this a column-local change: re-sealing rewrites only
    the new tables.
  - Recommended as a later step (D5). It needs operator key management, and
    the split does not depend on it.

### 4.6 What each package adopts afterwards

| Package | Adopts once finalized | Through the contract | Reads through views |
|---|---|---|---|
| proxy-node | `v2_node` (credential columns tombstoned, `raw_config` redacted), `v2_node_group` (not protected; can be adopted now) | credentials (API key and secret, registration keys), `PutSecretDocument` (raw configuration), `SyncNode`, `RetireNode`, `ValidateNodeConfig`, `CheckEndpoints` (load balancer check) | `kapi_node_protocol_public_v1`, `kapi_registration_key_v1`, `kapi_node_credential_status_v1`, `kapi_forward_node_v1` (exists) |
| protocol-runtime | `v2_node_protocol` (secret positions redacted) | `PutSecretDocument`, `RetireProtocol`, `SyncNode`, `ValidateNodeConfig`, the agents family, `RunAgentDiagnostic` | `kapi_node_public_v1`, `kapi_wireguard_peer_v1` |
| forward | `v2_forward_node` (token tombstoned; the endpoint pinned in the kernel), `v2_forward_port_binding` (not protected; written in the forward change flow) | the forward family, `CheckEndpoints`, `CollectNodeStats`, `DiagnoseForward`, `DiagnoseTunnel`, credentials (forward node tokens, clean agents), `SyncNode` and `RetireNode` on forward nodes | `kapi_node_credential_status_v1`, `kapi_forward_clean_agent_v1`, `kapi_forward_runtime_job_v1`, `kapi_node_public_v1` (also unblocks the 3 observability routes) |
| subscription | nothing new | none | `kapi_node_protocol_public_v1`, `kapi_node_public_v1` |

These tables stay kernel-owned and protected. The kernel writes them, and
packages read them through views or change them through operations:

- `v2_authorized_key`;
- `v2_forward_clean_agent`: the kernel-owned registration, heartbeat and
  report routes write it;
- `v2_forward_runtime_job`: the kernel's executors claim and complete
  jobs;
- `v2_forward_agent_bridge_task`;
- `v2_wireguard_peer`: the subscription renderer creates peers. It stays
  protected and non-adoptable after finalize too (decided by the owner,
  2026-10-01); packages read peers through `kapi_wireguard_peer_v1`.

**New views.** Each is created by `EnsureKernelAPIViews`, and only once its
source table is finalized, where the view depends on it (NO-9: every view
below but `kapi_forward_runtime_job_v1`, which waits for a record that the
payload scrub completed; `kapi_node_credential_status_v1` once any credential
table is finalized; unsplit drops the others again):

| View | Source | Columns |
|---|---|---|
| `kapi_node_public_v1` | `v2_node` | every column but `api_key`, `api_key_hash`, `secret`; `raw_config` only once finalized |
| `kapi_node_protocol_public_v1` | `v2_node_protocol` | every column; created only once finalized |
| `kapi_node_credential_status_v1` | `v4_kernel_node_credential` | `subject_kind`, `subject_id`, `kind`, `version`, `status`, `rotated_at`, `has_value`; no value or hash |
| `kapi_registration_key_v1` | `v2_authorized_key` + credential status | `id`, `name`, `used`, `expire_at`, `created_at`, `updated_at`, `has_key` |
| `kapi_forward_clean_agent_v1` | `v2_forward_clean_agent` | every column but `token` |
| `kapi_forward_runtime_job_v1` | `v2_forward_runtime_job` | every column; created only once its payloads are scrubbed |
| `kapi_wireguard_peer_v1` | `v2_wireguard_peer` | `id`, `node_protocol_id`, `user_id`, `peer_ip`, `public_key`, times |

## 5. Agent A2: unified control plane

### 5.1 Today

The agent survey was taken from anix-agent `dev_new` at c459383.
`origin/dev_new` is 10 commits ahead, with production config validation,
the maintenance WebSocket outbox and plugin secret materials. Branch
`feat/control-sdk` moves the agent onto `anix-control/sdk`.

| Path | What flows | Authentication | Default |
|---|---|---|---|
| **REST** (`api/panel`) | UniProxy `config` (protocols, keys), `user` (UUIDs, WireGuard peer keys), `push` (traffic), `alive`/`alivelist` (online IPs); `node/register`; `node/runtime-health` | `X-API-Key` plus `node_id`; HMAC signing only with `AutoRegister` and `EnableSign` | yes (`Transport: http`) |
| **WebSocket** (`node/sync.go`, `/api/v2/agent/ws`, fallback `/api/v2/node/ws`) | Control → agent: `config_update`, `user_update`, `user_ban`, `rule_update`, `cert_update`, `ping`, `force_reload`; agent → Control: `heartbeat`, `ack`, `pong`, and `maintenance_events` (the plugin supervisor's durable outbox, answered by `maintenance_ack`; gRPC agents open a WebSocket for it alone). Control itself sends only `task.assign` (diagnostics), `ack` and `auth`, and never answered `maintenance_events` | `X-API-Key`, `X-Node-ID` | on for REST nodes; 60 s polling fallback |
| **Legacy v2board gRPC** (`api/grpc`) | `NodeService.Register/GetConfig/ReportStatus`, `UserService.GetUsers`, `TrafficService.ReportTraffic/ReportOnline`, `NodeLogService.ReportLogs` (logs and runtime health) | metadata `x-node-id`, `x-api-key` | when `Transport: grpc` |
| **Agent Control stream** (`api/agent`, `node/agent_control.go`) | operations only: `agent.ping`, `node.reload` and `users.reload` (both re-pull over the legacy transport), `operation.cancel`, `plugin.*`; heartbeats with plugin observations | metadata `x-node-id`, `x-api-key`; TLS without client certificates | opt-in (`AgentControlEnabled`) |

Gaps A2 closes:

- configuration, users, traffic and logs never use the stream;
- there are no client certificates;
- stored credentials are "encrypted" with a key derived from
  `/etc/machine-id`;
- signing is off by default;
- a forward node cannot open a stream: the stream authenticates `v2_node`
  only.

### 5.2 Target

One mTLS stream per node carries everything:

| Direction | Payload | Replaces |
|---|---|---|
| Control → agent | `ConfigSnapshot`: the node's configuration, revisioned and hashed | UniProxy `config`, `GetConfig`, WebSocket `config_update`, the `node.reload` re-pull |
| Control → agent | `UserDelta`: users added, changed or removed, from the subscriber change log, after the agent's cursor | UniProxy `user`, `GetUsers`, WebSocket `user_update` and `user_ban` |
| Control → agent | `DesiredOperation` (as today): plugin lifecycle, ping, diagnostics (`diag.*`, A2-5) | WebSocket `task.assign` |
| agent → Control | `TrafficReport`: per-user bytes and online IPs, with a batch id | UniProxy `push` and `alive`, `ReportTraffic`, `ReportOnline` |
| agent → Control | `LogBatch` | `NodeLogService.ReportLogs` |
| agent → Control | `NodeStatus`: system and runtime health | `ReportStatus`, `node/runtime-health`, `node/heartbeat` |
| agent → Control | `ConfigStatus`: applied or failed, per configuration revision | (none; runtime health today) |
| agent → Control | `MaintenanceEvents`, answered by `MaintenanceAck` per event (`maintenance.v1`) | WebSocket `maintenance_events` and `maintenance_ack` |
| both | `Hello`, `HelloAck`, `Heartbeat`, `OperationAck`, `ObservedState` (as today) | |

### 5.3 Identity and enrollment

- **SAN.** `spiffe://anixops/<cluster>/agent/<node>`, where `<node>` is
  `proxy-<id>` for a `v2_node` row or `forward-<id>` for a
  `v2_forward_node` row.
  - Node ids of the two tables overlap, so the kind is part of the name.
  - The module CA's name constraint (`spiffe://anixops/...`) already allows
    it.
  - The stream takes the node from the certificate. An envelope's `node_id`
    must match it, as it must match `x-node-id` today.
- **Issuer.** The module PKI's CA (`internal/modulepki`), with the same
  rotation (`current` and `next`).
- **Enrollment service.** A new service, `AgentEnrollment`
  (`Enroll`, `Renew`, `GetTrustBundle`), in a new file of package
  `anix.agent.v1`. A new file leaves `agent.proto`'s descriptor as it is.
  - It is served on the agent listener (`internal/grpc/server.go`, port
    50051), which faces the nodes. The module listener stays private.
  - The kernel ignores the CSR's subject and issues the SAN of the
    authenticated node.
- **Bootstrap, in three ways:**
  1. **Agents in the field** authenticate `Enroll` with the credentials
     they already have (`x-node-id` plus `x-api-key`, or the forward
     token). This is the plan's "re-register once on upgrade" (decision 4
     of 2026-09-30), done automatically.
  2. **New nodes** use a one-time agent enrollment credential bound to the
     node (`anixagt_...`, at most 7 days, stored hashed).
     - It comes from `POST /api/v4/kernel/agents/enrollment-tokens` or
       `anix-control agent token create -node proxy-12`.
     - Packages can mint one through `IssueCredential{kind:
       AGENT_ENROLLMENT}`, as a handle.
  3. **Registration keys** (`/api/v2/node/register`) keep minting node
     credentials during the transition. The new agent then enrolls with
     them, as in way 1.
- **Lifetime.** 7 days, renewed at two thirds through `Renew` with the
  current certificate. Modules use 24 hours; nodes can be offline longer,
  and during the transition an expired agent falls back to its legacy key
  (D9).
- **Revocation.** Every certificate is recorded in
  `v4_kernel_agent_certificate` (serial, node kind and id, enrollment,
  issuer, `not_after`, `revoked_at`). Enrollments are in
  `v4_kernel_agent_enrollment`.
  - Revoking or replacing a node's credentials, disabling the node, or
    deleting it (`RetireNode`) revokes its certificates.
  - The listener refuses revoked serials, with a cache of at most 30 s, as
    the module listener does.
- **Forward link certificates (H28).** A node whose Agent negotiated
  `forward.v1` also gets a link certificate for its forward engines
  (`AgentEnrollment.IssueLinkCertificate`, `GetLinkTrustBundle`), from a
  forward link CA that is a separate root (`v4_kernel_forward_link_ca`,
  sealed with the same `module_runtime.ca_kek`), for a key of its own:
  the node's identity name as its DNS name, its SPIFFE ID as its URI,
  serverAuth and clientAuth, 7 days. Only an agent certificate
  authenticates the call. Issued certificates are recorded in
  `v4_kernel_forward_link_certificate` and revoked with the node's agent
  credentials (every path above, `RetireNode` included). The link key and
  certificate live in gost's directory, never in the Agent's PKI
  directory (`docs/architecture/forward-sdk.md` section 6.2;
  `PROTOCOL.md`, "Forward link certificates").
- **Refusal codes.** Every refusal of a certificate or a bootstrap names
  its reason in the `x-anix-error-code` trailer, as `agent_mtls_required`
  does: `agent_cert_revoked` (also a node disabled or deleted, which revokes
  its certificates), `agent_cert_expired`, `agent_cert_invalid`,
  `agent_cert_wrong_cluster`, `agent_cert_wrong_node` and, for `Enroll`,
  `agent_enrollment_rejected` (`sdk/agentcontrol`, `ErrorCode*`;
  `PROTOCOL.md`, "Error codes"). The stream sets it at connection and when
  the heartbeat's recheck ends an open session; `Renew`,
  `GetTrustBundle` and the v2board services set it too. A transient
  `Unavailable` carries none. An Agent tells "enroll again" from "retry"
  by the code alone.
- **Listener.** Mode `agent_control.mtls` (section 5.6 has the full
  table; owner decision H5 of 2026-10-02 sets the defaults):
  - `off`: no client certificate is requested or accepted, and
    `AgentEnrollment` is unavailable; a rollback switch;
  - `optional` (the 4.1 release candidates): a client certificate is
    verified when given; without one, the legacy metadata authenticates;
  - `preferred` (the default from 4.1.0): legacy still works, but answers
    deprecation signals;
  - `required` (the default from 4.2): certificates only on the AnixOps
    Agent channels.

  The server keeps its public TLS certificate (`TLSCertFile`), and agents
  verify it as today. Client certificates are verified against the module
  CA bundle.
- **On the agent:**
  - the key and certificate live in `/var/lib/anix-agent/pki`, mode
    0600, owned by root, or come from systemd credentials;
  - the derived-key "encryption" is not used for them;
  - `x-api-key` is no longer sent once a certificate is in hand.

### 5.4 Stream additions

Everything is additive to `anix.agent.v1`, in A2-2. This is a sketch; the
proto is not part of this draft.

```proto
// agent.proto (additive)
message Hello {
  // ...fields 1-5 unchanged...
  uint64 config_revision = 6;  // last configuration revision applied
  uint64 users_cursor = 7;     // last user change cursor applied
}
message HelloAck {
  // ...fields 1-4 unchanged...
  repeated Capability server_capabilities = 5;  // config.v1, users.v1, reports.v1, diag.v1
}
message ControlToAgent {
  oneof payload {
    // ...10-12 unchanged...
    ConfigSnapshot config = 13;
    UserDelta users = 14;
    ReportAck report_ack = 15;
  }
}
message AgentToControl {
  oneof payload {
    // ...10-13 unchanged...
    ConfigStatus config_status = 14;
    TrafficReport traffic = 15;
    LogBatch logs = 16;
    NodeStatus status = 17;
  }
}
message ConfigSnapshot { uint64 config_revision = 1; string config_hash = 2; string format = 3; bytes config_json = 4; }
message ConfigStatus   { uint64 config_revision = 1; string config_hash = 2; bool applied = 3; string error = 4; }
message NodeUser       { uint64 user_id = 1; string uuid = 2; int64 speed_limit_mbps = 3; int32 device_limit = 4; bytes extra_json = 5; }
message UserDelta      { uint64 cursor = 1; bool full = 2; bool last_page = 3; repeated NodeUser upserts = 4; repeated uint64 removed_user_ids = 5; }
message TrafficReport  { string batch_id = 1; repeated UserTraffic users = 2; repeated OnlineUser online = 3; int64 window_end_unix_ms = 4; }
message LogBatch       { string batch_id = 1; repeated LogEntry entries = 2; }
message ReportAck      { string batch_id = 1; bool applied = 2; string error = 3; }
```

**Negotiation.** Each side sends the new payloads only when the other
advertised them.

- An agent sends reports only when `HelloAck.server_capabilities` lists
  `reports.v1`. A kernel without A2 sends none, and the stream rejects an
  unknown payload ("control message payload is required").
- Control sends snapshots and deltas only to agents whose `Hello` lists
  `config.v1` or `users.v1`. Old agents ignore the new `HelloAck` field.
- `agent_descriptor_test.go` changes from "equal to v1.1.0 except
  `go_package`" to "a superset of v1.1.0".

### 5.5 Delivery semantics

- **Configuration.** `v4_kernel_node_desired_config` holds each node's
  desired revision and hash.
  - **The table (NO-6).** One row per node kind and id: `revision`
    (starts at 1, grows by one when the hash changes, never otherwise),
    `config_hash` (the SHA-256 of the canonical document: sorted keys,
    no whitespace, so the same configuration always has the same hash),
    `format` (`anixops.nodeconfig/v1`; `anixops.nodeconfig/v2`, with the
    node's forwarding state, for a node whose Agent negotiated `forward.v1`,
    `forward-sdk.md` section 8.1), `config_json` (the document, with
    the node's protocol secrets: the table is protected and no call
    answers it), `excluded_protocols` and `built_at`. A proxy node's
    document is its node row, raw configuration and enabled protocols
    through the kernel's builder (`service.BuildNodeProtocolConfig`, the
    source UniProxy and the gRPC node service share); a forward node's is
    its node row, legacy rules and tunnels. Two writers of one node are
    serialized by the row's revision. `kernelnodeops.BuildDesiredConfig`,
    `StoreDesiredConfig` and `LoadDesiredConfig` are the API A2-3 reads.
  - **Rebuilds.** `SyncNode` rebuilds it (NO-6). For an agent on the
    stream with `config.v1`, A2-3 also rebuilds it at the agent's `Hello`
    and about once a minute while the session lasts
    (`kernelnodeops.RefreshDesiredConfig`: built, and stored only when the
    hash moved). The kernel writes that change what a node runs (the
    protocols, the raw configuration, the node's secrets) thus reach the
    agent within a minute, as they reach the legacy pull; they do not
    rebuild it themselves.
  - **What the document carries (A2-3).** A proxy node's document also
    holds `legacy_pull`: the UniProxy answer for no node type
    (`default`) and for each type the node serves (`types`), from
    `service.BuildUniProxyNodeConfig`, which the UniProxy handler calls
    too. Each protocol's `config` is the v2board `GetConfig` source, and
    `raw_config` is read through the node credential split as UniProxy
    reads it. So the snapshot carries what the legacy pulls give the node,
    and no other secret (`internal/tests/nodeopsagent` compares the two
    byte for byte and walks every secret-named key). The forward document
    is unchanged; its `legacy_rules` are what
    `GET /api/v2/forward/agent/rules` serves, with the node's role.
  - **Pushes (A2-3, `config.v1`).** The kernel sends the stored row as a
    `ConfigSnapshot` (`kernelnodeops.ConfigSnapshotOf`) only to an agent
    whose `Hello` listed `config.v1`, and lists `config.v1` in
    `HelloAck.server_capabilities` for proxy and forward nodes:
    - on `Hello`, when the agent's `config_revision` is not the desired
      revision (none, older, or newer from another database), after the
      rebuild; the same revision is sent nothing;
    - on `node.sync` (the executor and the legacy sync route), when the
      configuration changed, the sync is forced, or the agent has not
      applied the stored revision. Such an agent is not sent
      `node.reload`; agents without `config.v1` still are;
    - when the per-session rebuild moves the revision.

    A session is never sent a revision older than one it was sent
    (`AgentControlConnection.sendConfig`).
  - **The agent's answer.** The agent answers `ConfigStatus`. The kernel
    records the last one per node in `v4_kernel_node_config_status` (new,
    protected; `kernelnodeops.RecordConfigStatus`) with its verdict:
    `applied` or `failed` when it names the desired revision and hash,
    `stale` for an older revision, `mismatch` otherwise. Only a verified
    applied status moves the row's `applied_revision` and `applied_hash`,
    the node's applied revision. A failure is logged and recorded there; it
    does not write the node's runtime-health columns yet.
  - **`node.sync` ends on the answer.** On a `config.v1` agent the
    operation runs at the configuration revision (`node_revision`) and
    ends on the `ConfigStatus` that names the pushed revision and hash,
    from any session of the node: `SUCCEEDED` when applied, `FAILED`
    (`BACKEND_FAILED`, retryable) with the agent's error otherwise. A stale
    or mismatched status is recorded and does not end it; a status verified
    as applied at a newer revision does (the node runs a newer desired
    configuration). The result's `ack` is the status (accepted is applied,
    with the answering session and revision). An agent that reconnects
    meanwhile is sent the snapshot again by the `Hello` rule and answers on
    its new session. Without an answer the deadline ends it `TIMED_OUT`.
  - **Metrics.** `anixops_agent_config_snapshots_sent_total{trigger}`
    (`hello`, `sync`, `refresh`),
    `anixops_agent_config_statuses_total{result}` (the verdicts, and
    `unrecorded` when the database failed), and
    `anixops_agent_config_lagging_nodes` (nodes whose applied revision is
    behind the desired one).
  - **Level-triggered.** A snapshot is the whole configuration, so a lost or
    repeated snapshot is harmless. Deltas are a later optimization (the
    "incremental config" TODO).
- **Users.** The kernel follows `v4_kernel_subscriber_change`, the
  KernelSubscriber change log, and pushes the changes for subscribers
  active on the node's group (`subscriber.Active`).
  - A reconnect resumes from `Hello.users_cursor`.
  - A cursor older than the log (7 days) gets a paged full list
    (`full: true` ... `last_page: true`).
- **Traffic.** Batch ids are `node:<kind>-<id>:<boot id>:<sequence>`, and
  the kernel records them with `RecordTrafficTx`'s batch idempotency.
  - A batch the kernel applied before is acknowledged again with
    `applied: false`.
  - **The spool.** The agent keeps unacknowledged batches in an on-disk
    spool and resends them on the stream only.
  - **One path per batch.** A batch never moves to REST. REST has no batch
    ids, so a move could count it twice. Data produced while the stream is
    down for longer than a grace period (5 minutes) goes to the legacy
    path as new batches.
- **Online IPs.** They ride in `TrafficReport.online` and feed the same
  alive set that UniProxy feeds.
- **Logs.** `LogBatch` goes into `v2_node_log` (as `NodeLogService`), with
  the same batch idempotency.
- **Operations.** As today: at-least-once, with replay by operation id and
  revision, and handlers made idempotent. KernelNodeOps operations are
  level-triggered, so the agent's in-memory completed-operation cache (256
  entries) is enough. Plugin operations are also journaled on disk.

### 5.6 Transition for agents in the field

Owner decision H5 (2026-10-02) supersedes the earlier plan, which kept
legacy agents until 5.0: 4.1.0 makes `preferred` the default, and 4.2 makes
`required` the default. Before upgrading to 4.2, every node must run an
Agent that has enrolled (mTLS); legacy API-key agents are refused.

| Agent \ Control | 4.1.0-rc (`optional`) | 4.1.0 (`preferred`, default) | 4.2 (`required`, default) |
|---|---|---|---|
| **Today's agents** (v1.1.0 SDK) | REST, WebSocket, v2board gRPC; stream for operations only | unchanged: every legacy path is served, with deprecation signals and counters; the transport inventory shows the node as `legacy` | the AnixOps Agent channels refuse them (`agent_mtls_required`); only UniProxy and v2board gRPC, which third-party node software shares, still answer them. They must be upgraded and enrolled before 4.2 |
| **A2 agents** | they enroll with their API key, then everything moves to the mTLS stream | the same; the legacy paths are a fallback while the stream is down | mTLS stream only |

**The modes** (A2-6). `agent_control.mtls`
(`ANIX_CONTROL_AGENT_CONTROL_MTLS`):

| Mode | Client certificate | Legacy credential on the AnixOps Agent channels | Signals |
|---|---|---|---|
| `off` | neither requested nor accepted; `Enroll` answers `FailedPrecondition` | served | none |
| `optional` | verified when presented | served | none |
| `preferred` (4.1.0 default) | verified when presented | served | `Deprecation`, `Sunset`, `Link` headers; `x-anix-auth-deprecated` on the stream |
| `required` (4.2 default) | required | refused: HTTP 403 `{"code":"agent_mtls_required"}`, gRPC `Unauthenticated` with the trailer `x-anix-error-code: agent_mtls_required`; `Enroll` takes only one-time enrollment credentials | the refusal carries the deprecation headers |

- Only `required` needs, at startup, the gRPC listener with TLS and the
  built-in CA: an enrolled agent could not connect otherwise. `preferred`
  and `optional` start without them; agents then cannot enroll yet, which
  the startup log says (`Agent transports: agent_control.mtls=...`).
- **Scope of `required`.** The AnixOps Agent channels only:
  - API key authentication on `AgentControlService.ControlStream`, and the
    API key bootstrap of `AgentEnrollment.Enroll`;
  - `/api/v2/agent/*` (register, heartbeat, tasks, result, monitor, ws);
  - `/api/v2/node/*` (register, heartbeat, runtime-health, ws);
  - `/api/v2/forward/agent/rules` (the forward agent's rule pull);
  - the clean agent endpoints `/api/v2/forward-agent/register`,
    `heartbeat` and `report` (`install.sh` is an operator download and
    stays).

  Not affected, in any mode: UniProxy (`/api/v1|v2/server/UniProxy/*`) and
  the v2board gRPC services (`NodeService`, `UserService`,
  `TrafficService`, `NodeLogService`), which XrayR, V2bX and other
  third-party node software use (the F3 decision); the admin APIs. These
  are read from the router (`internal/router`, `legacyAgentHTTP` and
  `legacyAgentWebSocket`) and the gRPC interceptors
  (`internal/grpc/interceptor.go`).
- **Signals.** On a legacy HTTP or WebSocket agent path, `preferred`
  answers `Deprecation: true`, `Link: <docs/UPGRADE.md#agent-transports-preparing-for-v42>;
  rel="deprecation"` and, when `agent_control.legacy_sunset` is set, `Sunset`
  (RFC 8594). The stream's header and trailer carry
  `x-anix-auth-deprecated`, `x-anix-auth-deprecation-link` and
  `x-anix-auth-sunset`. No Sunset is sent by default: the 4.2 release date
  is not fixed, and a Sunset date that passes while the server still answers
  would teach clients to ignore it. Operators who plan their 4.2 upgrade set
  the date.
- **Metrics.** `anixops_agent_legacy_requests_total{path}` counts the
  requests a legacy AnixOps Agent channel served, in every mode, so the
  legacy traffic can be watched draining; `path` is the route template or
  the gRPC method. `anixops_agent_legacy_refused_total{path}` counts
  `required`'s refusals, and `anixops_agent_mtls_mode{mode}` is 1 for the
  mode in force. The names follow the repository's `anixops_` prefix
  (this section's draft said `anix_agent_legacy_requests_total`).
- **Transport inventory.** `GET /api/v4/kernel/agents/transports` (admin;
  `legacy_only=true` filters) and `anix-control agents transports [--json]
  [--legacy-only]` list every proxy and forward node with the transport it
  was last seen on (`mtls-stream`, `apikey-stream`, `http-legacy`,
  `websocket`, `clean-agent`, `uniproxy`, `v2board-grpc`), its agent
  version (from the stream's `Hello`, else the node row or the clean
  agent), its newest valid certificate (serial, expiry) and when it was last
  seen. A node's status follows its newest AnixOps Agent channel: `mtls`,
  `legacy`, `third-party` (UniProxy or v2board gRPC only) or `unseen`.
  - Sightings live in memory and in the new table
    `v4_kernel_agent_transport` (one row per node kind, id and transport;
    protected). A row is written at most once a minute per node and
    transport, and at once when the version or identity changes; the API
    overlays this process's newer sightings, the CLI reads the table.
  - Clean agents are read from `v2_forward_clean_agent`, not recorded.
  - The admin page NodeX Agents → Agent 连接方式
    (`/admin/agent/transports`) shows it with a warning on legacy nodes.

- **Stream equivalents of the refused paths.** Under `required` an agent
  that negotiates the data plane needs none of the paths above
  (`PROTOCOL.md`, "Data plane", lists each one's equivalent):
  - `/api/v2/node/heartbeat` and `/api/v2/node/runtime-health` are
    `NodeStatus` (`reports.v1`), which writes the same columns, the node's
    online status, and a sighting of `mtls-stream` in the inventory;
  - the WebSocket's agent → Control traffic is `Heartbeat` and, for the
    maintenance outbox, `MaintenanceEvents` (`maintenance.v1`, new: each
    event stored once per node and event id as a node log row of source
    `maintenance`, answered per event); its Control → agent `task.assign` is
    the `agent.diagnostic` operation;
  - `/api/v2/node/register` is `Enroll` with a one-time credential;
  - UniProxy `alivelist` (each user's online device count across nodes,
    for device limits) is `AliveList` (`alive.v1`, new): the same counts,
    from the same online sets, after `HelloAck` and whenever they change
    (checked once a minute);
  - the plugin release download (`/api/v3/agent/plugin-releases/...`, by
    node API key, which an enrolled agent no longer holds) is the
    `AgentArtifacts` service (`artifacts.proto`, `artifacts.v1`, new),
    authenticated by the client certificate: the same assignment checks,
    content addresses and verified bytes, the artifact in 1 MiB chunks.
    The HTTP download is not refused under `required` (it is not a legacy
    agent channel) and stays for agents that have not enrolled;
  - the administrator routes `POST /admin/agent/tasks` and
    `/admin/agent/execute` send the task as the `agent.diagnostic`
    operation on the stream when the node has no WebSocket and its agent
    advertises the operation, as the KernelNodeOps executor does.

  `internal/grpc`'s `TestAgentStreamUnderRequiredNeedsNoLegacyPath` walks
  it: an agent enrolled with a one-time credential under `required`
  negotiates `config.v1`, `users.v1`, `reports.v1`, `maintenance.v1`,
  `alive.v1` and `artifacts.v1`; configuration, users, heartbeats, status
  and runtime health, traffic, logs, maintenance events, the alive list
  (counting the online IP the traffic report carried), a plugin release
  download by certificate and a diagnostic task all flow; no legacy
  counter moves and the inventory lists the node on `mtls-stream` only.
- **Codes in acknowledgements.** `ReportAck` and `MaintenanceEventResult`
  carry an `error_code` on every refusal, and the agent sets one on a
  `ConfigStatus` that was not applied (kept as the node's
  `reported_error_code`, and leading a failed `node.sync`'s message).
  Transient answers carry a retry hint: `maintenance_unavailable` with
  `retry_after_ms` on every maintenance result Control could not store,
  and `report_unavailable` with `retry_after_ms` on a report batch, only
  to an agent that negotiated the `transient_ack` attribute of
  `reports.v1` (any other drops a batch on any `ReportAck`, so it still
  gets none). Stream refusals of an unnegotiated payload carry
  `agent_capability_not_negotiated`. `PROTOCOL.md`, "Error codes", lists
  them.
- **The offer rule.** `HelloAck.server_capabilities` lists a data-plane
  capability only when the agent's `Hello` lists it (it listed `users.v1`
  to every proxy node before), so it is the session's negotiated set.
- **Sessions.** `AgentControlSnapshot` (`GET /admin/nodes/:id/agent-control`)
  and the inventory's `session` (`GET /api/v4/kernel/agents/transports`)
  show how a live stream session authenticated (`mtls` or `api-key`), its
  certificate's serial, expiry and SAN, and its negotiated capabilities.
- **Prerequisites for the v4.2 default.** `required` can become the
  default only when the Agent release that operators install runs
  everything on the stream: anix-agent AG-3 (configuration, `config.v1`),
  AG-4 (users, `users.v1`), AG-5 (reports and status, `reports.v1`), and
  the maintenance outbox on `maintenance.v1`, on top of AG-2 (enrollment,
  which reads the refusal codes), and a Control with these stream
  equivalents. The Control-side gaps AG-2 to AG-5 reported are closed by
  additions to the Agent contract (owner approval of 2026-10-04: new
  fields and messages only):
  - done: the legacy admin routes `POST /admin/agent/tasks` and
    `/admin/agent/execute` reach stream agents through `agent.diagnostic`;
  - done: UniProxy `alivelist` has its stream equivalent, `alive.v1`;
  - done: plugin artifacts and manifests download from `AgentArtifacts`
    by the client certificate (`artifacts.v1`);
  - done: `ReportAck`, `MaintenanceEventResult` and `ConfigStatus` carry
    machine-readable codes, with retry hints on transient answers.

  Still open: `diag.v1` is reserved: no `diag.*` operations or schemas
  exist yet (deferred, not a `required` blocker). The Agent side of the
  three additions is anix-agent work (section 5.7).
- **Agent health on the session.** The Agent's own heartbeat metrics
  (`agent_control_*`, `agent_identity_*`, `agent_dataplane_*`: the stream,
  the certificate, spool depth and drops, apply failures; at most 64,
  finite values) are kept with the live session, the latest heartbeat's
  set, and shown as `agent_metrics` in `AgentControlSnapshot` and in the
  inventory's `session`. Plugin telemetry (`plugin.*`) is persisted as
  before; other names are dropped.

Stages:

- **T1, the 4.1 release candidates with A2-1 to A2-5.** Additions only, in
  `optional`.
- **T2, 4.1.0 (A2-6).** `preferred` is the default; the inventory, the
  signals and the counters above. The release notes and `docs/UPGRADE.md`
  ("Agent transports: preparing for v4.2") announce `required` for 4.2.
- **T3, 4.2.** `required` becomes the default: the AnixOps Agent channels
  accept enrolled agents only. An operator may still set `preferred` for a
  while; the removal of the legacy agent routes themselves (the agent and
  node WebSocket, `/api/v2/agent/*`, `/api/v2/node/*`, the clean agent
  endpoints, API key authentication on the stream; the 7 agent-channel
  routes section 6 marks `kernel-owned` and the 7 agent routes that are
  kernel-owned already) follows in a later release.
  - anix-agent no longer uses UniProxy or the v2board `NodeService`,
    `UserService`, `TrafficService` and `NodeLogService`. Third-party node
    software (V2bX, XrayR) may still. Whether the kernel keeps serving them
    is the F3 decision of phase 4, not this one.

### 5.7 What changes in anix-agent

PRs AG-1 to AG-7 (section 7):

- **SDK.** Depend on `github.com/AnixOps/anix-control/sdk` at a version
  with A2 (the `feat/control-sdk` branch is the start). The frozen
  `anix-agent/sdk` stays for old builds.
- **Identity.**
  - A PKI client enrolls with the existing `credential.json` key, or with
    an `EnrollCredentialFile`.
  - It stores the key and certificate with mode 0600 under
    `AgentIdentity.CertDir` and renews at two thirds of the lifetime.
  - It dials with mTLS and drops `x-api-key` once enrolled.
  - New settings: `AgentIdentity{Cluster, CertDir, EnrollCredentialFile}`.
  - Encrypted credential files are not used for the new key. The A1 item
    "remove fake encryption" lands alongside.
- **Configuration from the stream.**
  - `ConfigSnapshot` is applied through the same core adapters the legacy
    pull feeds, and answered with `ConfigStatus`.
  - `node.reload` stops re-pulling over the legacy transport once
    snapshots are active.
  - Certificate hot reload is the A2 TODO of the earlier plan.
- **Users from the stream.** `UserDelta` with a persisted cursor and full
  resync. `user_ban` urgency is kept by pushing changes as soon as they
  are written.
- **Reports on the stream.**
  - Traffic, online IPs, logs and node status move to the stream, with
    batch ids and the on-disk spool. Node status replaces the REST
    heartbeat and runtime-health reports.
  - The maintenance outbox moves to `maintenance.v1`; the maintenance-only
    WebSocket of gRPC agents is no longer needed.
  - Device limits read the alive list from `alive.v1` (`AliveList`
    replaces the `alivelist` pull) instead of counting only the node's
    own connections.
  - Plugin installs download from `AgentArtifacts` once enrolled
    (`artifacts.v1`), with the same verification, and keep the HTTP
    download with the node API key until then.
  - Acknowledgements are decided on their `error_code`; the agent sets
    `ConfigStatus.error_code`, lists `transient_ack: "v1"` on `reports.v1`
    and honors `retry_after_ms`.
  - The legacy reporters stop while the stream is healthy and resume for
    new data after the grace period.
- **Defaults.**
  - The stream is on by default when the server advertises A2.
  - `ValidateForProduction` (on `origin/dev_new`) requires a certificate
    and refuses `AgentControlAllowInsecure`.
  - The legacy transports remain as a fallback until 4.2 makes
    `agent_control.mtls: required` the default (H5, section 5.6).
- **Forward nodes.**
  - An agent on a forward node enrolls as `forward-<id>` with its token.
  - Plugin rule counters feed `TrafficReport` for forwards. This joins A5.

## 6. Route mapping

The 83 routes waiting on KernelNodeOps and the 7 node and agent channel routes
(decided D3):

| Package | Routes | Native | Kernel-owned |
|---|---|---|---|
| forward | 55 + 4 | 48 | 11 (4 runtime status and doctor, 3 flow accounting, 4 agent channel) |
| proxy-node | 15 + 3 | 14 | 4 (credentials display, 3 node channel) |
| protocol-runtime | 11 | 11 | 0 |
| subscription | 2 | 2 | 0 |
| **all** | **83 + 7 = 90** | **75** | **15** |

"Native" means `native-flagged` with a parity test, once the route's
contract calls, views and adopted tables exist (section 7). Routes on an
adopted table wait for that table to be finalized on an installation (section
4.3). Open questions are marked **Q** and listed in section 10.

Call names below are `SubmitOperation` kinds unless they are RPCs. "wait T"
means `wait: TERMINAL` and "wait A" means `wait: ACCEPTED`.

**The connection test, as NO-7 built it.** `POST /admin/forward/test-connection`
(gost-mesh, `native-flagged`) runs through the kernel: the gateway seals the
typed token (section 3.7), the package's native handler submits
`TestForwardBackend{host, api_port, token}` (`diagnose.forward_backend`,
family diagnose, held by gost-mesh through `kernel.nodeops.diagnose.v1`)
with its request binding and waits for the result, and the kernel dials
the gost API with the kernel's own gost client, as the legacy handler does,
so the answer is byte for byte the legacy one (`gostmeshcompat`). The
operation's `Preparer` resolves the handle in the submitting call and holds
the value in kernel memory until the executor dials, or the request's
deadline; the ledger holds neither token nor handle, and the token is
scrubbed from the result. A refusing or unreachable API is the result, not
a failed operation. The SSRF rule of the diagnoses (#84, #90) applies: a
request that is not an administrator's dials public addresses only; the
route is administrator-only, so the kernel dials what the administrator
named, loopback included, as before. A shadow run has no binding and so
submits nothing; a host without KernelNodeOps keeps the route legacy; a
value the gateway did not seal (the placeholder) is never sent, and the
router answers from the legacy handler.

**Diagnoses, as NO-8 built them.** The kernel executes `CheckEndpoints`,
`CollectNodeStats`, `DiagnoseForward` and `DiagnoseTunnel`
(`kernelnodeops.Diagnosis`), and `GetCapabilities` lists them.

- **One implementation.** The executors and the legacy routes run the same
  functions from Control (`internal/service/forward_diagnosis.go`), so a
  result is what the route computes. The routes' answers are pinned byte for
  byte (`TestForwardDiagnosisAnswers`, written before the move).
- **Probes.** An operation's probes run in its executor, at most 8 at a
  time (`service.DiagnosisProbes`), under the operation's deadline and
  cancellation. They are not fan-out children: one operation answers one
  report, as the routes need. A stopped operation keeps the probes that
  completed, and a stopped endpoint check records nothing.
- **Outcomes.** Failed probes are the result, not a failed operation. A
  node, forward or tunnel deleted since the submission is `TARGET_GONE`; a
  gost node without an API or metrics port is `VALIDATION_FAILED`.
- **Statistics.** The kernel chooses the source: an Ansible machine's
  counters (`LOCAL_ANSIBLE`), else the node's gost metrics (`CONTROL_DIAL`).
  Nothing is recorded, as in the legacy sync.
- **Endpoint checks.** `record_status` writes what the forward node check
  writes. The legacy load balancer check differs. After each check it saves
  the node's row as it was loaded before the check, which overwrites the
  check's last check, latency and uptime. It marks the node online unless
  the check returned an error, and an unreachable node is not an error.
  M3-2 decides whether its native route reproduces that.
- **Private targets.** Until NO-4 verifies request bindings, the kernel
  cannot tell an administrator's `DiagnoseForward` from a user's. It checks
  every forward's targets as a user's (public addresses only, the #84
  guard). An administrator's diagnosis of a private target therefore stays
  on the legacy route until then.
- **Vantage (D10).** Every check and diagnosis carries a `VantageReport`.
  The node vantage is selected when `CONTROL` was not requested and the
  agent of every node concerned advertises `diag.v1` (`AgentDirectory`).
  The kernel does not run diagnoses on agents before A2-5, so it dials from
  Control and says so in the report. No forward node holds an Agent Control
  session before A2-1, so the kernel's directory is empty for now.

### 6.1 forward (59)

**Superseded (2026-10-04).** The native modes below are not built: M3-4
and M3-5 are cancelled (section 7). v4.2's forwarding redesign deletes the
flux routes in F5d, rewrites the node management routes as
`/api/v4/forward/*` in F5a, and retires the clean agent routes with the
switch to the new Agent (`forward-sdk.md` section 10). The tables are kept
as the record of the plan.

**Forward nodes (8): native.** These are rows of `v2_forward_node`. Forward
adopts the table once it is finalized; the token is pinned in the kernel.

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/forward/nodes` | none; adopted rows, with the token mask from `kapi_node_credential_status_v1` | native |
| `POST /admin/forward/nodes` | insert the row; `IssueCredential{forward, FORWARD_NODE_TOKEN, value: handle or generated}` (answered as a handle); `SyncNode` | native |
| `GET /admin/forward/nodes/:id` | none | native |
| `PUT /admin/forward/nodes/:id` | update the row; `IssueCredential{replace}` when a new token is sent; `SyncNode` bound to the request (re-pins the endpoint, section 3.8) | native |
| `DELETE /admin/forward/nodes/:id` | `RetireNode{forward}`, then delete the row | native |
| `POST /admin/forward/nodes/:id/check` | `CheckEndpoints{record_status}` (wait T) | native |
| `POST /admin/forward/nodes/:id/toggle` | update the row; `SyncNode` (drops the gost manager's copy) | native |
| `POST /admin/forward/nodes/:id/sync-stats` | `CollectNodeStats` (wait T) | native |

**Ansible machines (8): native.** These are also rows of `v2_forward_node`.
The same calls apply to `GET`/`POST /admin/forward/ansible-machines` and to
`GET`/`PUT`/`DELETE /:id`, `/:id/check`, `/:id/toggle` and
`/:id/sync-stats`. The statistics come from the local Ansible collector
(`CollectNodeStats`, channel `LOCAL_ANSIBLE`).

**Panel forward changes (16): native.** The package keeps the forward rows,
port bindings (adopting `v2_forward_port_binding`) and quota checks.

| Route | Planned calls | Mode |
|---|---|---|
| `POST /forward/create`, `/admin/forward/create` | write the rows; `ApplyForward{CREATE}` (wait T on gost, wait A on job backends) | native |
| `POST /forward/update`, `/admin/forward/update` | `ApplyForward{UPDATE}` | native |
| `POST /forward/delete`, `/admin/forward/delete` | `ApplyForward{DELETE}` (wait T on gost, wait A on job backends: the kernel has captured the forward), then delete the rows | native |
| `POST /forward/force-delete`, `/admin/forward/force-delete` | `ApplyForward{FORCE_DELETE}`, then delete the rows | native |
| `POST /forward/pause`, `/admin/forward/pause` | `ApplyForward{PAUSE}` | native |
| `POST /forward/resume`, `/admin/forward/resume` | `ApplyForward{RESUME}` | native |
| `POST /forward/diagnose`, `/admin/forward/diagnose` | `DiagnoseForward` with the request binding, so a user's forward dials public targets only (wait T). **Q10**: vantage | native |
| `POST /admin/tunnel/diagnose` | `DiagnoseTunnel` (wait T) | native |
| `POST /admin/forward/sync-backend` | `SyncForwardBackend` (fan-out; wait T bounded, then the summary) | native |

**Tunnels and permissions (5): native.**

| Route | Planned calls | Mode |
|---|---|---|
| `POST /admin/tunnel/update` | update the tunnel and its bindings; `ApplyTunnel{reasons}` when the protocol, listen addresses or interface change | native |
| `POST /tunnel/user/remove`, `/admin/tunnel/user/remove` | `ApplyForward{DELETE}` for each of the permission's forwards (waits as for a forward deletion), then delete the rows | native |
| `POST /tunnel/user/update`, `/admin/tunnel/user/update` | update the permission; `ApplyForward{PAUSE}` when it lapses, `{UPDATE}` when the speed limit changes | native |

**Legacy rules (7 of 8): native.** The package reads its adopted
`v2_forward_rule` and the forward nodes, with tokens masked as since #92.

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/forward/rules`, `GET /admin/forward/rules/:id` | none | native |
| `POST /admin/forward/rules` | write the row; `ApplyLegacyRule{CREATE}` (wait T) | native |
| `PUT /admin/forward/rules/:id` | `ApplyLegacyRule{UPDATE}` | native |
| `DELETE /admin/forward/rules/:id` | `ApplyLegacyRule{DELETE}`, then delete the row | native |
| `POST /admin/forward/rules/:id/toggle` | `ApplyLegacyRule{PAUSE or RESUME}` | native |
| `POST /user/forward/rules` | administrators: as `POST /admin/forward/rules`; users: the refusal of #90 | native |

**Runtime (5).**

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/forward/runtime/jobs` | none; `kapi_forward_runtime_job_v1`, once the payloads are scrubbed | native |
| `GET /admin/forward/runtime/status`, `/runtime/doctor`, `/local/status`, `/local/doctor` | none | **kernel-owned**: they describe the kernel's own executors (Control's disk and Ansible paths, its NodeX client, its job queue). A native answer would only relay a kernel summary. They go once the runtime moves to agents (A5). **decided D4** |

**Clean agents, administrator side (3): native.**

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/forward/agents` | none; `kapi_forward_clean_agent_v1` | native |
| `POST /admin/forward/agents` | `IssueCleanAgent{name, forward_node_id}` (the token answered as a handle) | native |
| `POST /admin/forward/agents/:id/revoke` | `RevokeCredential{clean_agent}` | native |

**Flow accounting (3): kernel-owned (decided D4).** This covers
`POST /internal/forward/traffic/upload`, `/report` and `/snapshot`.

- In one kernel transaction, under a per-forward lock, they change:
  - the forward's counters;
  - the subscriber's traffic (`RecordTrafficTx`);
  - the permission's traffic.
- Exhaustion pauses forwards on their nodes.
- **Why not two steps.** The callers send no batch id. Split into
  idempotent steps, as KernelOrder does, a retry after a partial failure
  would count the forward's counters twice. The single transaction does
  not.
- **Later.** With A2, forward traffic arrives on the stream with batch ids.
  A later contract ("forward traffic events") can then move the accounting.

**Agent channel (4 of the 7): kernel-owned (decided D3).**

| Route | Why |
|---|---|
| `POST /forward-agent/register`, `/heartbeat`, `/report` | clean agent authentication, job claims and results: the legacy channel A2 and A5 replace |
| `GET /forward/agent/rules` | authenticated by a forward node's token; A2 pushes the rules instead |

### 6.2 proxy-node (18)

Proxy-node adopts `v2_node` once it is finalized.

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/nodes` | none; adopted rows with protocols from `kapi_node_protocol_public_v1` (stored redacted, the bytes of today's masked answer) | native |
| `POST /admin/nodes` | insert the row (`api_key` tombstone); `IssueCredential{proxy, kind unspecified}`: the API key and secret answered as handles; `SyncNode` | native |
| `GET /admin/nodes/:id` | none | native |
| `PUT /admin/nodes/:id` | update the row; `SyncNode{reasons: node}` (cache, desired configuration) | native |
| `DELETE /admin/nodes/:id` | `RetireNode{proxy}`: the kernel deletes the credentials and certificates, and the protocols with their secrets, WireGuard peers and subscription group links, as the legacy transaction does. Then the package deletes the row. **Q11** | native |
| `GET /admin/nodes/:id/credentials` | none | **kernel-owned**: the answer is the stored key and secret, and no contract call reveals a stored secret (section 3.7). **decided D4** |
| `GET /admin/nodes/:id/raw-config` | none; the stored, redacted `raw_config` | native |
| `PUT /admin/nodes/:id/raw-config` | `ValidateNodeConfig{RAW_CONFIG}`; `PutSecretDocument{NODE_RAW_CONFIG}` (handles from the request); store `redacted_json`; `SyncNode` | native |
| `POST /admin/nodes/validate-config` | `ValidateNodeConfig` | native |
| `GET /admin/auth-keys` | none; `kapi_registration_key_v1` (masked, as since #92) | native |
| `POST /admin/auth-keys` | `IssueRegistrationKey` (answered as a handle) | native |
| `DELETE /admin/auth-keys/:id` | `RevokeRegistrationKey` | native |
| `POST /internal/auth-keys` | `IssueRegistrationKey`, under the app-token principal | native |
| `GET /admin/loadbalancers/:id/stats` | none; `kapi_forward_node_v1` (exists) and the adopted `v2_load_balancer`. No node operation is needed; it was bridged together with the check | native |
| `POST /admin/loadbalancers/:id/check` | `CheckEndpoints{forward nodes of the group, record_status}`: the kernel writes the status columns of forward's table (wait T) | native |
| `POST /node/register`, `/node/heartbeat`, `/node/runtime-health` | none | **kernel-owned (decided D3)**: registration mints node credentials, and the others are authenticated by them. With A2 they become enrollment and stream reports, and 5.0 removes them |

### 6.3 protocol-runtime (11): all native

Protocol-runtime adopts `v2_node_protocol` once it is finalized.

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/nodes/:id/protocols` | none; adopted rows (redacted) | native |
| `POST /admin/nodes/:id/protocols` | `ValidateNodeConfig{PROTOCOL}`; insert the row; `PutSecretDocument` per secret column (handles); store each `redacted_json`; `SyncNode` | native |
| `PUT /admin/nodes/:id/protocols/:protocol_id` | the same, with the placeholder keeping stored secrets | native |
| `DELETE /admin/nodes/:id/protocols/:protocol_id` | `RetireProtocol` (secrets, WireGuard peers, and the subscription group links, which are the subscription package's: **Q11**); delete the row; `SyncNode` | native |
| `POST /admin/nodes/:id/sync` | `SyncNode{force}` (wait A); the answer's `transport`, `operation_id`, `revision` and `ack` come from `NodeSyncResult` | native |
| `GET /admin/nodes/:id/agent-control` | `GetAgentSession` | native |
| `POST /admin/nodes/:id/agent-control/operations` | `AgentControlOperation` (wait A) | native |
| `GET /admin/agent/list` | `ListAgentSessions{WEBSOCKET}` | native |
| `GET /admin/agent/monitor` | `GetAgentMonitor` | native |
| `POST /admin/agent/tasks` | `RunAgentDiagnostic` (wait A): the kernel validates the whitelist, writes the task row, dispatches and waits for the acknowledgement, with the legacy fallback | native |
| `POST /admin/agent/execute` | `RunAgentDiagnostic` | native |

### 6.4 subscription (2): native

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/subscription/groups/:id/protocols` | none; `kapi_node_protocol_public_v1` and `kapi_node_public_v1` (redacted, as masked since #92) | native |
| `GET /admin/subscription/protocols/available` | none; the same views | native |

Not counted here, but unblocked on the way: `kapi_node_public_v1` is the
"proxy node view with parent and load" that the 3 forward observability
routes wait on.

## 7. Rollout

The PRs are listed with their dependencies, in the style of the earlier
plans. Each Control PR keeps every route working in legacy mode. Native
handlers ship `native-flagged`, and operators choose the runtime mode.

| ID | PR | Depends on | Repo | Size |
|---|---|---|---|---|
| NO-0 | This design and the draft contract | none | control | S |
| NO-10 | Mark the 15 kernel-owned routes, with reasons, after D3 and D4 | NO-0 and the decisions | control | S |
| NO-1 | Contract engine: the capability grammar, `internal/kernelnodeops` (Submit, Get, List, Watch, Cancel, GetCapabilities), `v4_kernel_node_operation` and its events, served on the bridge and the module listener, `bridgecontract` tests, `GET /api/v4/kernel/node-operations`. **The contract becomes binding.** Done: section 3.11 | NO-0 | control | L |
| NO-2 | Split P1: the new secret tables, `internal/nodesecrets` as the one writer, dual-write in every writer, `node-secrets backfill` and `verify`, the split state table | NO-0 | control | L |
| NO-3 | Split P2: every reader through `nodesecrets` with fallback and metrics; validate on build, report-only | NO-2 | control | L |
| NO-4 | Sealed secret handles: gateway substitution and expansion, `config/node-secret-fields.json`, shadow-mode handling, fail-closed tests. Done: section 3.7 | NO-1 | control | M |
| NO-5 | Credential and secret operations: `IssueCredential`, `RevokeCredential`, registration keys, `IssueCleanAgent`, `PutSecretDocument`, `RetireNode`, `RetireProtocol`, `ValidateNodeConfig`; the legacy handlers move onto the same functions. Done: section 3.11 | NO-1, NO-3, NO-4 | control | L |
| NO-6 | Node configuration and agents: `SyncNode` with `v4_kernel_node_desired_config`, `AgentControlOperation`, `RunAgentDiagnostic`, the agent session RPCs; the durable dispatcher takes node kinds | NO-1 | control | L |
| NO-7 | Forward operations: `ApplyForward`, `ApplyTunnel`, `SyncForwardBackend`, `ApplyLegacyRule` over the existing executors; job payloads without tokens and the payload scrub; endpoint pinning | NO-1, NO-3 | control | L |
| NO-8 | Diagnosis: `CheckEndpoints`, `CollectNodeStats`, `DiagnoseForward`, `DiagnoseTunnel` (Control vantage; node vantage after A2-5) | NO-1 | control | M |
| NO-9 | Split P3: `node-secrets finalize` and `unsplit`, conditional adoption, the new views, the static gate on moved columns. Done: section 4.3 | NO-3, NO-5, NO-7 | control | M |
| M3-1 | protocol-runtime: 11 routes native, `protocolruntimecompat` with fake agents | NO-4, NO-5, NO-6, NO-9 | control | M |
| M3-2 | proxy-node: 14 routes native | NO-4, NO-5, NO-6, NO-8, NO-9 | control | L |
| M3-3 | subscription: 2 routes native | NO-9 | control | S |
| M3-4 | ~~forward: nodes, Ansible machines, clean agents, runtime jobs (20 routes)~~ **Cancelled** (2026-10-04, superseded by forward F5a/F5d) | NO-4, NO-5, NO-7, NO-8, NO-9 | control | L |
| M3-5 | ~~forward: changes, tunnels, permissions, legacy rules (28 routes)~~ **Cancelled** (2026-10-04, superseded by forward F5d) | NO-7, NO-8 | control | L |
| A2-1 | Agent PKI: `v4_kernel_agent_enrollment` and `_certificate`, the `AgentEnrollment` service, optional client certificates on the agent listener, the SAN as node identity (including forward nodes on the stream), enrollment admin API and CLI | none | control | L |
| A2-2 | Stream data-plane contract: the additive `agent.proto` payloads, the descriptor test as a superset, the golden | none | control | S |
| A2-3 | Configuration push: snapshots from the desired configuration, `Hello` reconcile, `ConfigStatus`. Done: section 5.5 | A2-2, NO-6 | control | M |
| A2-4 | User deltas from the subscriber change log, cursor, paged resync | A2-2 | control | M |
| A2-5 | Reports: traffic, online, logs and status with batch ids, `ReportAck`, `diag.*` for the node vantage | A2-2 | control | M |
| A2-6 | Transition: transport inventory, deprecation headers and metrics, the `agent_control.mtls` modes | A2-1 to A2-5 | control | S |
| A2-6b | The `required` prerequisites on Control (section 5.6): stream equivalents of the refused paths (`maintenance.v1`; `NodeStatus` for the heartbeat and runtime-health reports), certificate refusal codes, the capability offer as an intersection, session identity in the snapshot and the inventory. Done | A2-6 | control | M |
| A2-7 | Cross-repo E2E and chaos suite with the real agent (section 9) | AG-2 to AG-5 | control | M |
| AG-1 | anix-agent on the Control SDK with the A2 messages (after an `sdk/v*` tag or pseudo-version) | A2-1, A2-2 | agent | S |
| AG-2 | Identity: enroll, store, renew, mTLS dial, no API key once enrolled | AG-1 | agent | M |
| AG-3 | Configuration from the stream | AG-1, A2-3 | agent | M |
| AG-4 | Users from the stream | AG-1, A2-4 | agent | M |
| AG-5 | Reports on the stream, with the spool; node status in place of the REST heartbeat and runtime-health; the maintenance outbox on `maintenance.v1` | AG-1, A2-5, A2-6b | agent | M |
| AG-6 | Stream on by default; production validation requires mTLS | AG-2 to AG-5 | agent | S |
| AG-7 | Forward-node agents on the stream; plugin counters to traffic (joins A5) | AG-6 | agent | M |

**M3-4 and M3-5 are cancelled** (owner decision, 2026-10-04). v4.2 drops
flux compatibility, so a native port of these routes would be thrown away
in the same release: their flux routes go with F5d, the forward node and
Ansible machine routes are rewritten as `/api/v4/forward/*` in F5a, and the
clean agent routes retire with the switch to the new Agent (`forward-sdk.md`
section 10).

**Agent versions (H25, 2026-10-04).** anix-agent follows Control's version
numbers: the two are released together (for example `v4.2.0-rc.N` for
both), and Control's CI pins the same Agent commit. AG-1 and AG-2 start now;
each Agent tag is asked first.

**The critical path:** NO-1 and NO-2 → NO-3 → NO-5 and NO-7 → NO-9 → M3-*.
NO-4, NO-6 and NO-8 run beside it, and the A2 and AG line is independent of
it up to A2-3.

**Merges, if wanted.** NO-2 and NO-3 can merge, and so can NO-6 and NO-8.

**Estimate.**

- 22 Control PRs after this one, and 7 anix-agent PRs: about 29, or about
  26 merged.
- The plan's estimate was "15–20 including the Agent". The difference is
  the credential split (NO-2, NO-3, NO-9) and sealed handles (NO-4), which
  the plan counted as one item.
- At the pace of phase 2 that is 3–4 weeks.
- Finalize on production is not part of it (D6).

## 8. Risks

| # | Risk | Mitigation |
|---|---|---|
| R1 | **The split locks nodes out**: a reader looks in the new table and misses | Dual-read falls back with a metric. `verify` digests the whole table before P2 and before finalize. Rollback before P3 is a phase change. `unsplit` exists. The staging rehearsal includes a node fleet of fake agents across every phase |
| R2 | **Traffic counted twice or lost** while agents straddle transports | A batch stays on one path; batch ids are applied once; the spool is on disk and bounded, with a metric when it drops; E2E cases for stream loss during a report |
| R3 | **New surfaces leak secrets**: results, logs, handles, views, errors | Echo scrubbing; tests that walk every `IsNodeSecretKey` key through every result type; handles single-request and in memory only; views created only after finalize; fail-closed substitution; gosec and the redaction gate |
| R4 | **An address is a credential**: a forward node's address is written to steal its token | Endpoint pinning (`ENDPOINT_UNCONFIRMED`); the pin moves only to the address in the bound administrator request |
| R5 | **Validate on build drops rows production relies on** | Report-only first (NO-3), with a scan of a staging copy; enforcement is a decision (D7) |
| R6 | **Kernel memory and restarts**: sessions, handles and acknowledgement waiters live in one process | Durable operations through `KernelOperationBridge`; waits are bounded and answer the current state; handles expire with their request; HA stays out of scope |
| R7 | **Churn in the draft contract** | Reviewed before NO-1; binding and additive since NO-1 (section 3.10) |
| R8 | **Size and half-way states**: 90 routes, about 29 PRs | Per-route modes; legacy stays the default; every PR keeps both sides working; parity on SQLite and PostgreSQL |
| R9 | **Field agents not upgraded by 4.2** (H5) | The transport inventory and `anix-control agents transports --legacy-only`, deprecation signals from 4.1.0, automatic enrollment with the existing key, and `preferred` kept available as an override |
| R10 | **Cross-package cascades** (a node deletes its protocols; a protocol deletes subscription links) | The kernel performs the cascade in its transaction, as today (Q11); the steps are idempotent by request id |

## 9. Test strategy

- **Contract unit tests** (`internal/kernelnodeops`):
  - the ledger: a repeat answers the first receipt, and a reused id with
    another operation is refused;
  - the state machine and monotonic terminal states;
  - the watch cursor and `RESYNC`;
  - authorization per family and target kind; fenced generations;
  - quotas, fan-out counting and cancellation.
- **Bridge contract tests** (`internal/tests/bridgecontract`): the SDK
  client against the real server, over the local bridge and the module
  listener.
- **Parity with fake agents.** Each `internal/tests/<pkg>compat` runs the
  real KernelNodeOps server in process, as `paymentcompat` runs
  KernelOrder, against fakes:
  - `internal/tests/fakeagent`: a scripted Agent Control client (Hello,
    acknowledgements, observed states, refusals, replays, a replaced
    session) and a WebSocket agent;
  - the NodeX fake that gost-mesh's tests already have, a gost API fake, a
    clean agent claiming jobs, and an Ansible runner stub.
  - **What is compared, on SQLite and PostgreSQL:**
    - the answers, byte for byte;
    - the v2 rows;
    - `v2_forward_runtime_job`, `v3_kernel_operation` and the NodeOps
      ledger;
    - the secret tables;
    - what each fake received: the operations and payloads dispatched are
      part of the compared state, as PayPal's API calls are in
      `paymentcompat`.
- **Credential split tests.**
  - Backfill and verify digests.
  - Dual-read fallback at zero.
  - Tombstones against the unique indexes.
  - Finalize, then the builder ignoring literal secrets in adopted rows.
  - An older binary authenticating every node after P1 and P2.
  - `unsplit`.
  - A redaction round trip: the stored redacted document equals today's
    masked answer.
- **Sealed handles** (NO-4).
  - **Where.** `internal/sealedsecrets`, the gateway tests in
    `internal/compat/v2`, and `internal/tests/sealedhandles` end to end:
    through a real bridge session, the SDK router and the KernelNodeOps
    server.
  - **What.**
    - Every listed route and field is substituted.
    - The placeholder and empty values are kept.
    - A handle from another request, route, target or field is refused.
    - Expansion happens only in the bound answer.
    - The fail-closed fallback to legacy.
    - Shadow comparison with handles masked.
    - A walk proves that no value under an `IsNodeSecretKey` key reaches a
      package on any listed route.
- **Cross-repo E2E** (`ANIXOPS_CROSS_REPO_E2E=1`, the existing harness in
  `internal/grpc/*cross_repo_e2e_test.go`). A pinned anix-agent build:
  - enrolls with its API key and reconnects with mTLS;
  - applies a configuration snapshot and reports `ConfigStatus`;
  - follows user deltas across a reconnect and a resync;
  - reports traffic that is counted once.

  A v1.1.0-SDK agent keeps working on every legacy path, and on the stream
  for operations only.
- **Chaos**, in the fake-agent suite and in A2-7 with the real agent:
  - an agent disconnects mid-operation: before the acknowledgement, after
    it but before the terminal state, and during replay;
  - Control restarts mid-operation (the bridge's recovery);
  - acknowledgements and observations from a replaced session;
  - a partition longer than the deadline (`TIMED_OUT`, a late success
    recorded as evidence only, convergence by the next `SYNC`);
  - concurrent operations on one forward (supersession);
  - an agent restart that loses its completed-operation cache (idempotent
    replays);
  - the certificate expiring during a partition (fallback to the legacy
    key in T1 and T2, then re-enrollment);
  - the database failing inside the observed-state callback (the durable
    worker reconciles);
  - clock skew on deadlines.
- **Staging** (local kind and Compose, the only environment for now): all
  modules, a fleet of 1,000 fake agents, 10,000 operations, the split
  phases P0 → P3 with a rollback at each step, and the route modes switched
  shadow → native → legacy.
- **Gates.**
  - The route gate keeps checking every mode against the hosts, and every
    new `kernel-owned` row carries its reason.
  - The new static gate refuses kernel reads of moved columns outside
    `internal/nodesecrets`.
  - The proto golden; the CI generated-code check.

## 10. Decisions for the owner

Decided 2026-10-01: the owner accepted every recommendation below (D1–D12).
Implementation follows section 7 in that order.

| # | Decision | Decided (the recommendation) |
|---|---|---|
| D1 | Draft naming: `anixops.kernelnodeops.v1` unreleased, or `v1alpha1` now with `v1` at NO-1 | `v1` unreleased, with the draft golden policy; binding since NO-1 (section 3.10) |
| D2 | Sealed secret handles at the gateway (no package ever sees a secret, about 1 extra PR), or accept per-request transit of secrets an administrator types or is shown, as the bridged relay does today | handles. They are what makes "never sees a token or private key" true |
| D3 (Q3) | The 7 node and agent channel routes: `kernel-owned` until 5.0 removes them, or native behind node authentication in the kernel | `kernel-owned`; moving them is wasted work before their removal |
| D4 (Q4) | The other 8 kernel-owned routes: the node credentials display, the forward runtime status and doctor (4), flow accounting (3) | `kernel-owned` as in section 6 |
| D5 | Secrets at rest: in clear in the protected tables (parity), or sealed under a new `node_secrets.kek` | clear now; sealing in a later PR |
| D6 | Finalize (P3) on production: when, and who runs it | not in this phase; on production at least one release after P2, with your approval, after a staging rehearsal |
| D7 | Validate on build: enforce (rows failing validation are left out of node configurations), or stay report-only | enforce one release after report-only shows zero exclusions on a staging copy |
| D8 | Legacy agent transports removed in 5.0 (announced in the 4.x release with A2) or in 6.0 | 5.0, aligned with the v2 API policy. Superseded by H5 (2026-10-02): refused by default from 4.2 (`agent_control.mtls: required`), section 5.6 |
| D9 | Agent certificates: a 7-day lifetime (modules: 24 h), and node names `proxy-<id>` and `forward-<id>` in the SAN | as proposed |
| D10 (Q10) | Diagnosis vantage: Control by default (parity), the node's agent when it advertises `diag.v1`; never from package hosts | as proposed |
| D11 (Q11) | Cascades: the kernel deletes a node's protocols and a protocol's subscription group links in `RetireNode` and `RetireProtocol` (parity), or packages react to events | the kernel, for parity; events later |
| D12 | Endpoint pinning for forward node tokens (an address change is confirmed only by the administrator's own request) | as proposed; it lapses with A5 |

## 11. Not in scope

- Moving UniProxy, the v2board node services and the subscription renderer
  out of the kernel (phase 4, "F3").
- The forward runtime on agent plugins and the end of NodeX and Ansible
  dialing (A5).
- Kernel replicas: stream ownership, shared handles, the dispatcher's
  recovery per replica.
- The 22 other `bridged` routes:
  - invite codes and the Telegram webhooks;
  - speed limits;
  - the dashboard and subscription summary caches;
  - UniProxy and the subscription preview;
  - the subscription link settings;
  - the forward observability routes (`kapi_node_public_v1` unblocks
    them);
  - the clean agent install script.
