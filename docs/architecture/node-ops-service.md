# Node Operations Service (KernelNodeOps), Node Credential Split and Agent A2

Status: DESIGN (2026-10-01), for the owner's review. Nothing here is
implemented. The draft contract is `sdk/api/kernelnodeops/v1`
(`anixops.kernelnodeops.v1`), unreleased (section 3.10). This is phase 3 of
the 2026-10 plan, done together with Agent line A2.

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
- **Until NO-1, the names are reserved, not accepted.** The grammar does not
  list them yet, so a manifest that declares them fails validation
  (`unknown kernel capability`). That keeps the draft unreleased (section
  3.10).

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

### 3.10 Draft status and versioning

**Choice: package `anixops.kernelnodeops.v1`, unreleased.** It is not
`v1alpha1`.

Why:

- **An alpha name buys nothing here.** `internal/tests/protocompat` freezes
  every listed element: golden lines may be added, never removed.
  - A `v1alpha1` package in the golden would be frozen too.
  - Its lines would stay forever after a `v1` replaced it.
  - Every package would have to change its imports, and the kernel would
    serve two packages during the switch.
- **The other contracts are `v1`** and grow additively. KernelNodeOps
  should look the same once it is served.
- **"Unreleased" is enforced by mechanism, not by name:**
  - no kernel serves the service;
  - the `kernel.nodeops.*` capabilities are not in the grammar, so no
    manifest that declares them installs;
  - the proto file and the Go package doc say DRAFT, UNRELEASED;
  - the contract is in the CI generated-code check, so the checked-in code
    cannot drift.

**Golden policy while it is a draft.** The draft is registered in
`contracts/proto/descriptors.golden`, so any change shows up in review.

- Until NO-1 merges, a design change may edit this draft's own golden lines
  by hand, in the PR that changes the proto. The PR lists the removed lines.
- From NO-1 on, normal rules apply: additions only.
- An `sdk/v*` tag cut before NO-1 carries the draft. The draft's doc comment
  says not to build against it, and no kernel accepts the capabilities.

Rejected alternative: `v1alpha1` now, `v1` at NO-1. If the owner prefers it
(D1), NO-1 adds `anixops.kernelnodeops.v1`, the alpha package is never
served, and the golden keeps both.

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
- **Operation tables** (section 3.4): `v4_kernel_node_operation` and
  `v4_kernel_node_operation_event`.
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
| **P2 dual-read** | both | new tables. A missing or mismatching row falls back to the old column and counts `anix_node_secret_fallback_total{table}` | yes | `anix-control node-secrets verify`: the digests over every secret of the old and new forms match, and the phase moves to `dual_read` |
| **P3 finalized** | new tables only; old columns get tombstones (section 4.4) | new tables only | **no** (restore from backup) | `anix-control node-secrets finalize --table <t>`, after a zero fallback count. It rewrites every old secret to its tombstone or redacted form in batches, verifies, and records `finalized_at` |
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

**Rollback:**

- **P1 or P2 to P0:** set the phase back (`anix-control node-secrets
  phase legacy`). The old columns still hold every value, so an older
  binary works.
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
| `v2_node.secret` | `""` | |
| `v2_authorized_key.key` (unique) | `!moved:<key id>` | |
| `v2_authorized_key.key_hash` | `""` | |
| `v2_forward_node.api_token` | `""` | "has a token" now comes from `kapi_node_credential_status_v1` |
| `v2_forward_clean_agent.token` (unique, not null) | `!moved:<agent id>` | |
| `v2_wireguard_peer.private_key`, `.preshared_key` | `""` | |
| JSON secret columns | the redacted document: `RedactNodeSecretsJSON` of the original, with `********` at every secret position | the builder takes secret positions from `v4_kernel_protocol_secret` only (section 3.8) |

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
- `v2_wireguard_peer`: the subscription renderer creates peers.

**New views.** Each is created by `EnsureKernelAPIViews`, and only once its
source table is finalized, where the view depends on it:

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
| **WebSocket** (`node/sync.go`, `/api/v2/agent/ws`, fallback `/api/v2/node/ws`) | Control → agent: `config_update`, `user_update`, `user_ban`, `rule_update`, `cert_update`, `ping`, `force_reload`; agent → Control: `heartbeat`, `ack`, `pong` | `X-API-Key`, `X-Node-ID` | on for REST nodes; 60 s polling fallback |
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
- **Listener.** Mode `agent_control.mtls`:
  - `optional` (the first release): a client certificate is verified when
    given; without one, the legacy metadata authenticates;
  - `preferred`: legacy still works, but answers deprecation signals;
  - `required` (v5): certificates only.

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
  - **Rebuilds.** `SyncNode` and every kernel write that changes what a
    node runs rebuild it: the protocols, the raw configuration, the
    node's secrets.
  - **Pushes.**
    - When the hash changed, the kernel pushes a `ConfigSnapshot` to a
      connected agent.
    - On `Hello`, it pushes when the agent's `config_revision` is older.
  - **The agent's answer.** The agent answers `ConfigStatus`; a failure
    there becomes the node's runtime health.
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

| Agent \ Control | Before A2 (4.1) | 4.x with A2 (`mtls: optional`, then `preferred`) | 5.0 (`mtls: required`) |
|---|---|---|---|
| **Today's agents** (v1.1.0 SDK) | REST, WebSocket, v2board gRPC; stream for operations only | unchanged: every legacy path is served. Deprecation headers and metrics, and the node inventory shows "legacy" | only what F3 keeps for third-party nodes (UniProxy, v2board gRPC) still answers them; they must be upgraded before 5.0 |
| **A2 agents** | `Enroll` is unimplemented, so they keep the API key. No `server_capabilities`, so data stays on the legacy paths | they enroll with their API key, then everything moves to the mTLS stream; the legacy paths are a fallback while the stream is down | mTLS stream only |

Stages:

- **T1, the 4.x release with A2-1 to A2-5.**
  - Additions only: the legacy routes, the WebSocket, UniProxy and the
    v2board services stay.
  - The release notes announce the removal at 5.0. That is the "one major
    version" ahead, aligned with the v2 API policy (only v4 after v5) and
    decision D8.
- **T2, the next 4.x.**
  - `mtls: preferred` becomes the default.
  - `GET /api/v4/kernel/agents/transports` lists each node's transport,
    agent version and identity.
  - The legacy agent endpoints answer `Deprecation` and `Sunset` headers
    and count `anix_agent_legacy_requests_total{path}`.
- **T3, 5.0.**
  - The transports only AnixOps agents use are removed:
    - the agent and node WebSocket;
    - `/api/v2/agent/*` and `/api/v2/node/*`;
    - the clean agent endpoints, whose agents become anix-agent with the
      forward plugins (A5).
  - API key authentication on the stream is removed too.
  - This removes the 7 agent-channel routes that section 6 marks
    `kernel-owned`, and the 7 agent routes that are kernel-owned already
    (protocol-runtime's 6 and proxy-node's WebSocket).
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
    batch ids and the on-disk spool.
  - The legacy reporters stop while the stream is healthy and resume for
    new data after the grace period.
- **Defaults.**
  - The stream is on by default when the server advertises A2.
  - `ValidateForProduction` (on `origin/dev_new`) requires a certificate
    and refuses `AgentControlAllowInsecure`.
  - The legacy transports remain as a fallback until 5.0.
- **Forward nodes.**
  - An agent on a forward node enrolls as `forward-<id>` with its token.
  - Plugin rule counters feed `TrafficReport` for forwards. This joins A5.

## 6. Route mapping

The 83 routes waiting on KernelNodeOps and the 7 open-decision routes:

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

### 6.1 forward (59)

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
| `GET /admin/forward/runtime/status`, `/runtime/doctor`, `/local/status`, `/local/doctor` | none | **kernel-owned**: they describe the kernel's own executors (Control's disk and Ansible paths, its NodeX client, its job queue). A native answer would only relay a kernel summary. They go once the runtime moves to agents (A5). **Q4** |

**Clean agents, administrator side (3): native.**

| Route | Planned calls | Mode |
|---|---|---|
| `GET /admin/forward/agents` | none; `kapi_forward_clean_agent_v1` | native |
| `POST /admin/forward/agents` | `IssueCleanAgent{name, forward_node_id}` (the token answered as a handle) | native |
| `POST /admin/forward/agents/:id/revoke` | `RevokeCredential{clean_agent}` | native |

**Flow accounting (3): kernel-owned (Q4).** This covers
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

**Agent channel (4 of the 7): kernel-owned (Q3).**

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
| `GET /admin/nodes/:id/credentials` | none | **kernel-owned**: the answer is the stored key and secret, and no contract call reveals a stored secret (section 3.7). **Q4** |
| `GET /admin/nodes/:id/raw-config` | none; the stored, redacted `raw_config` | native |
| `PUT /admin/nodes/:id/raw-config` | `ValidateNodeConfig{RAW_CONFIG}`; `PutSecretDocument{NODE_RAW_CONFIG}` (handles from the request); store `redacted_json`; `SyncNode` | native |
| `POST /admin/nodes/validate-config` | `ValidateNodeConfig` | native |
| `GET /admin/auth-keys` | none; `kapi_registration_key_v1` (masked, as since #92) | native |
| `POST /admin/auth-keys` | `IssueRegistrationKey` (answered as a handle) | native |
| `DELETE /admin/auth-keys/:id` | `RevokeRegistrationKey` | native |
| `POST /internal/auth-keys` | `IssueRegistrationKey`, under the app-token principal | native |
| `GET /admin/loadbalancers/:id/stats` | none; `kapi_forward_node_v1` (exists) and the adopted `v2_load_balancer`. No node operation is needed; it was bridged together with the check | native |
| `POST /admin/loadbalancers/:id/check` | `CheckEndpoints{forward nodes of the group, record_status}`: the kernel writes the status columns of forward's table (wait T) | native |
| `POST /node/register`, `/node/heartbeat`, `/node/runtime-health` | none | **kernel-owned (Q3)**: registration mints node credentials, and the others are authenticated by them. With A2 they become enrollment and stream reports, and 5.0 removes them |

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
| NO-1 | Contract engine: the capability grammar, `internal/kernelnodeops` (Submit, Get, List, Watch, Cancel, GetCapabilities), `v4_kernel_node_operation` and its events, served on the bridge and the module listener, `bridgecontract` tests, `GET /api/v4/kernel/node-operations`. **The contract becomes binding.** | NO-0 | control | L |
| NO-2 | Split P1: the new secret tables, `internal/nodesecrets` as the one writer, dual-write in every writer, `node-secrets backfill` and `verify`, the split state table | NO-0 | control | L |
| NO-3 | Split P2: every reader through `nodesecrets` with fallback and metrics; validate on build, report-only | NO-2 | control | L |
| NO-4 | Sealed secret handles: gateway substitution and expansion, `config/node-secret-fields.json`, shadow-mode handling, fail-closed tests | NO-1 | control | M |
| NO-5 | Credential and secret operations: `IssueCredential`, `RevokeCredential`, registration keys, `IssueCleanAgent`, `PutSecretDocument`, `RetireNode`, `RetireProtocol`, `ValidateNodeConfig`; the legacy handlers move onto the same functions | NO-1, NO-3, NO-4 | control | L |
| NO-6 | Node configuration and agents: `SyncNode` with `v4_kernel_node_desired_config`, `AgentControlOperation`, `RunAgentDiagnostic`, the agent session RPCs; the durable dispatcher takes node kinds | NO-1 | control | L |
| NO-7 | Forward operations: `ApplyForward`, `ApplyTunnel`, `SyncForwardBackend`, `ApplyLegacyRule` over the existing executors; job payloads without tokens and the payload scrub; endpoint pinning | NO-1, NO-3 | control | L |
| NO-8 | Diagnosis: `CheckEndpoints`, `CollectNodeStats`, `DiagnoseForward`, `DiagnoseTunnel` (Control vantage; node vantage after A2-5) | NO-1 | control | M |
| NO-9 | Split P3: `node-secrets finalize` and `unsplit`, conditional adoption, the new views, the static gate on moved columns | NO-3, NO-5, NO-7 | control | M |
| M3-1 | protocol-runtime: 11 routes native, `protocolruntimecompat` with fake agents | NO-4, NO-5, NO-6, NO-9 | control | M |
| M3-2 | proxy-node: 14 routes native | NO-4, NO-5, NO-6, NO-8, NO-9 | control | L |
| M3-3 | subscription: 2 routes native | NO-9 | control | S |
| M3-4 | forward: nodes, Ansible machines, clean agents, runtime jobs (20 routes) | NO-4, NO-5, NO-7, NO-8, NO-9 | control | L |
| M3-5 | forward: changes, tunnels, permissions, legacy rules (28 routes) | NO-7, NO-8 | control | L |
| A2-1 | Agent PKI: `v4_kernel_agent_enrollment` and `_certificate`, the `AgentEnrollment` service, optional client certificates on the agent listener, the SAN as node identity (including forward nodes on the stream), enrollment admin API and CLI | none | control | L |
| A2-2 | Stream data-plane contract: the additive `agent.proto` payloads, the descriptor test as a superset, the golden | none | control | S |
| A2-3 | Configuration push: snapshots from the desired configuration, `Hello` reconcile, `ConfigStatus` | A2-2, NO-6 | control | M |
| A2-4 | User deltas from the subscriber change log, cursor, paged resync | A2-2 | control | M |
| A2-5 | Reports: traffic, online, logs and status with batch ids, `ReportAck`, `diag.*` for the node vantage | A2-2 | control | M |
| A2-6 | Transition: transport inventory, deprecation headers and metrics, the `agent_control.mtls` modes | A2-1 to A2-5 | control | S |
| A2-7 | Cross-repo E2E and chaos suite with the real agent (section 9) | AG-2 to AG-5 | control | M |
| AG-1 | anix-agent on the Control SDK with the A2 messages (after an `sdk/v*` tag or pseudo-version) | A2-1, A2-2 | agent | S |
| AG-2 | Identity: enroll, store, renew, mTLS dial, no API key once enrolled | AG-1 | agent | M |
| AG-3 | Configuration from the stream | AG-1, A2-3 | agent | M |
| AG-4 | Users from the stream | AG-1, A2-4 | agent | M |
| AG-5 | Reports on the stream, with the spool | AG-1, A2-5 | agent | M |
| AG-6 | Stream on by default; production validation requires mTLS | AG-2 to AG-5 | agent | S |
| AG-7 | Forward-node agents on the stream; plugin counters to traffic (joins A5) | AG-6 | agent | M |

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
| R7 | **Churn in the draft contract** | Review before NO-1; the draft golden policy (section 3.10) |
| R8 | **Size and half-way states**: 90 routes, about 29 PRs | Per-route modes; legacy stays the default; every PR keeps both sides working; parity on SQLite and PostgreSQL |
| R9 | **Field agents not upgraded by 5.0** | The transport inventory, deprecation signals a major ahead, and automatic enrollment with the existing key |
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
- **Sealed handles.**
  - Every listed route and field is substituted.
  - The placeholder and empty values are kept.
  - A handle from another request, route or target is refused.
  - Expansion happens only in the bound answer.
  - The fail-closed fallback to legacy.
  - Shadow comparison with handles masked.
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
| D1 | Draft naming: `anixops.kernelnodeops.v1` unreleased, or `v1alpha1` now with `v1` at NO-1 | `v1` unreleased (section 3.10), with the draft golden policy |
| D2 | Sealed secret handles at the gateway (no package ever sees a secret, about 1 extra PR), or accept per-request transit of secrets an administrator types or is shown, as the bridged relay does today | handles. They are what makes "never sees a token or private key" true |
| D3 (Q3) | The 7 node and agent channel routes: `kernel-owned` until 5.0 removes them, or native behind node authentication in the kernel | `kernel-owned`; moving them is wasted work before their removal |
| D4 (Q4) | The other 8 kernel-owned routes: the node credentials display, the forward runtime status and doctor (4), flow accounting (3) | `kernel-owned` as in section 6 |
| D5 | Secrets at rest: in clear in the protected tables (parity), or sealed under a new `node_secrets.kek` | clear now; sealing in a later PR |
| D6 | Finalize (P3) on production: when, and who runs it | not in this phase; on production at least one release after P2, with your approval, after a staging rehearsal |
| D7 | Validate on build: enforce (rows failing validation are left out of node configurations), or stay report-only | enforce one release after report-only shows zero exclusions on a staging copy |
| D8 | Legacy agent transports removed in 5.0 (announced in the 4.x release with A2) or in 6.0 | 5.0, aligned with the v2 API policy |
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
