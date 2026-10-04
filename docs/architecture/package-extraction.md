# Package Extraction Design

Date: 2026-09-29 (status refreshed 2026-10-01)

This is the design of record for moving business domains out of the Control
kernel and into signed packages. It replaces the retired plans that lived under
`docs/superpowers/` (see [Sources](#sources)); what is still binding from them
is copied here. Feature status stays in [`../features.md`](../features.md) and
open work in [`../../TODO.md`](../../TODO.md).

> 中文摘要：v4.0.0 的「插件化」只到路由层；本文定义「一个领域真正住在插件里」
> 的验收标准、目标机制（存储租约 + 按路由模式 + 类型化内核操作，均已实现）、
> 保留下来的旧计划约束，以及 M0–M4 里程碑。243 条 v2 路由中，155 条
> `native-flagged`，36 条 `bridged`（待契约），52 条 `kernel-owned`（按设计留在内核）。
> v4.2（F5d）删除了 forward 包的 53 条 `bridged` flux 路由；forward 包不再接管任何表，
> 其余 32 条 v2 路由全部交给内核处理，直到 F5c 删表。

Markers used below: **CURRENT** = true in the tree today; **PLANNED** = accepted
design, not implemented yet; **HISTORICAL** = preserved from a retired plan for
context, not a commitment.

## 1. Status And Reality (CURRENT)

v4.0.0 (published 2026-07-20) is plugin-only at the routing level only.

- All 243 `/api/v2` routes in `config/v2-package-route-catalog.json` enter the
  package gateway (v4.2 removed the forward package's 53 `bridged` flux
  routes, F5d; [`forward-sdk.md`](forward-sdk.md) section 10). 214 are registered with `registeredPackageRoute`
  (`internal/router/router.go`), which registers the legacy gin handler into
  `packagebridge.DefaultRouteRegistry()` and returns the gateway; the 26
  `identity-platform` routes use the bare `v2PackageGateway.Serve` (their
  legacy handlers are in the identity bridge); the 3 WebSocket routes use
  `registeredPackageWebSocketRoute`.
- `config/package-extraction.json` records each route's extraction mode
  (`bridged`, `kernel-owned`, `native-flagged` or `native`; section 3.2) and
  where its legacy handler lives
  (`router`, `identity-bridge` or `none`). 155 routes are `native-flagged`:
  identity-platform (23: group A's 16, the profile, dashboard, user detail,
  user list and user statistics, and the traffic and subscription resets),
  affiliate (10), gost-mesh (3), knowledge (6),
  machine-telemetry (3), notification (23), order (13), payment (20), plan
  (7), platform (5), protocol-runtime (3), proxy-node (7), subscription (23),
  ticket (8) and wireguard (1). 52 routes are `kernel-owned`: they stay in the
  kernel by design, and each row says why. The other 36 are `bridged` until a
  kernel contract lets their package serve them (section 3.2 lists what
  unblocks them). None is `native` yet. The identity routes are
  `identity-bridge`. `check_plugin_only_routes.py` enforces the map against
  the router, the identity bridge and the package hosts.
- Request path: gin middleware -> `compatv2` gateway -> route resolution
  (`internal/compat/v2/registry.go`, `verifiedRouteSource`) -> package host
  process over Unix gRPC -> package bridge (FD 4) -> **the legacy in-kernel
  handler/service**.
- 15 of 16 packages run the generic forwarding host
  `packages/shared/controlhost/service.go` (`build_package.py` falls back to it
  when `packages/<id>/control` is absent). Only `identity-platform` has its own
  host (`packages/identity-platform/control/service.go`), and it too forwards
  every call to the kernel allowlist in
  `internal/identitybridge/identity_bridge.go`.
- 11 packages (forward, knowledge, notification, order, payment, plan,
  protocol-runtime, proxy-node, subscription, ticket, wireguard) are 4-file
  placeholders (`manifest.template.json`, `compat/`, `migrations/`, `webui/`).
- **After v4.0.0.** 23 of the 45 identity-platform routes moved to other
  packages:
  - system configuration, audit and backup (12) to the new `platform`
    package;
  - commissions, withdrawals and invite statistics and configuration (8),
    then the user's invite codes and their generation (2), to the new
    `affiliate` package;
  - `/user/reset` to `forward`.

  They are now `registeredPackageRoute` routes. identity-platform keeps 22,
  whose legacy handlers are in the identity bridge, and has since gained the
  administrator's invite codes (3, bridged; see Identity leftovers) and the
  user's own subscription reset (1, group A). `internal/compat/v2/moved_routes.go`
  lists the moves: the kernel accepts the old route ids from old
  identity-platform releases and, while both packages declare a route,
  prefers the new owner. There are now 18 packages. All but nat-egress and
  nftables-forward, which have no v2 routes, have their own host.
- The 4.0 stage exit condition "production requests no longer reach coupled
  legacy handlers" was **not** met. Handlers, services and workers stay in
  the kernel; packages adopt business tables (`v2_*`) in place
  (section 3.4).
- Production (per the operator, 2026-09) still runs a pre-v4 Control on
  PostgreSQL. Moving it to v4 is a database migration plus host move (M2).
- Outside the package gate, kernel handlers serve `/api/v1/server/UniProxy/*`,
  `/{subscribe_path}/:token`, `/api/v1/client/subscribe`, and `/flow/upload`.

Route modes per package (2026-10-04, after F5d):

| Package | `native-flagged` | `bridged` | `kernel-owned` |
|---|---|---|---|
| affiliate | 10 | 0 | 0 |
| forward | 0 | 0 | 32 |
| gost-mesh | 3 | 0 | 0 |
| identity-platform | 23 | 3 | 0 |
| knowledge | 6 | 0 | 0 |
| machine-telemetry | 3 | 0 | 2 |
| notification | 23 | 1 | 0 |
| order | 13 | 0 | 0 |
| payment | 20 | 0 | 0 |
| plan | 7 | 0 | 0 |
| platform | 5 | 0 | 7 |
| protocol-runtime | 3 | 11 | 6 |
| proxy-node | 7 | 19 | 5 |
| subscription | 23 | 2 | 0 |
| ticket | 8 | 0 | 0 |
| wireguard | 1 | 0 | 0 |
| **all** | **155** | **36** | **52** |

Reusable pieces that already exist:

| Piece | Where |
|-------|-------|
| Signed package build, compiles `packages/<id>/control` when present | `packages/shared/build_package.py` |
| Versioned, schema-validated package config | `GET/PUT /api/v3/plugin-installations/:id/config` |
| Rollout/migration ledger models | `internal/model/plugin_rollout.go`, `internal/service/plugin_rollout.go` |
| Legacy/native comparison harness (`RunRead`, `RunWrite`) | `internal/tests/packagecompat` |
| Lifecycle plan with reverse compensation | `internal/plugincontrol/lifecycle_plan.go` |
| Compiling a real host inside a test | `internal/pluginhost/identity_platform_host_test.go` |
| E2E fixture that installs a real signed package | `internal/tests/e2e/identity_package_fixture_test.go` |

## 2. Definition Of Done For An Extracted Domain

A domain counts as extracted only when all four hold:

1. Its code lives in `packages/<id>/control/{main.go, app/...}` and imports
   nothing from `internal/` (enforced by a boundary gate).
2. Every route of the domain is handled natively by the host: `router.go`
   registers the bare `v2PackageGateway.Serve` and no legacy handler is
   registered for it.
3. The package owns its tables, and its migrations run through the kernel
   migration runner.
4. The kernel has no handler, service, model, or worker left for the domain.

Identity is no longer exempt (decision of 2026-09-30). Login, credentials,
MFA and invite codes move into the identity module, which runs as a network
module and is the first domain extracted; see
[`identity-service.md`](identity-service.md) and
[`module-runtime.md`](module-runtime.md).

## 3. Target Mechanism (M3 infrastructure CURRENT; no domain extracted yet)

The host process and bridge stay. Three additions make real extraction
possible, because today a host has no data channel: it never receives database
credentials, and its one-shot capability can only call back into its own
legacy route.

### 3.1 Storage lease

- `internal/packagestore.EnsurePackageRole` (idempotent) creates, per package,
  a PostgreSQL role `anix_pkg_<id>` and schema `pkg_<id>`.
- Grants come only from signed capabilities in the package manifest:
  `kernel.storage.v1` (own schema) and `kernel.storage.adopt:<table>` (for
  example `kernel.storage.adopt:v2_knowledge`). Column-level grants are allowed
  for shared tables (for example `v2_order(status, paid_at)`).
- New bridge RPC `LeaseStorage` returns connection parameters for that role
  only; the host opens it through `sdk/packagestoresdk.Open` and uses GORM.
  Each lease is capped at 2–4 connections and requested only by packages that
  have at least one route in `native` mode.
- `sdk/packagestoresdk.RunEmbeddedMigrations`: embedded SQL, a per-package
  state table, a digest per step. The kernel records runs in the existing
  `v4_kernel_package_*` ledger.
- New kernel model `v4_kernel_package_storage` records role, schema, granted
  tables, and lease generation.

Status (2026-09-30): implemented. `internal/packagestore`, `LeaseStorage`,
`sdk/packagestoresdk`, `v4_kernel_package_storage`, `kapi_user_directory_v1`
and the CI job `package-storage-postgres` are in place. Column-level grants
are not implemented yet. Package migrations of storage packages run
through the ledger when their host starts (`plugin-kernel-contract.md`,
"Package Migrations").

### 3.2 Per-route modes

A route has two modes: what the code supports (the extraction mode, per
route) and what an installation runs (the runtime mode).

**Runtime mode** (CURRENT): `legacy` | `shadow` | `native`, in the reserved
`routes` key of the package's versioned configuration, which the host polls
every 5 s through the bridge RPC `GetPackageConfig`
([`plugin-kernel-contract.md`](plugin-kernel-contract.md#package-configuration)).

- `legacy` (the default for a route without a stored mode): the host relays
  the request through the bridge to the legacy handler.
- Rehearsed default (4.1.0, decision H8): the 151 routes of
  `config/package-route-defaults.json` (130 of 14 packages from v4.2: the
  forward package's 21 left with F5d), the 15 packages that passed the
  staging rehearsal (R5) and were signed off (H7), run `native` when no mode
  is stored for them, the installed release is at least the package's
  `min_version` (`4.1.0-rc.5`) and `package_routes.default_mode` is
  `rehearsed` (the default; `legacy` is the kill switch). The kernel
  resolves this (`ResolveEffectivePackageRouteModes`) and hands the host
  the resolved map. The set is explicit: a route that becomes
  `native-flagged` later does not default to native until it is added to
  the file and to the gate test's record
  (`internal/service/testdata/rehearsed-routes.json`).
- `shadow` (GET only): the legacy result is returned; the host runs its
  native implementation in the background, compares the normalized output
  and counts mismatches.
- `native`: the host answers; the legacy handler is not called.
- Rollback sets the route back to `legacy`, effective within 5 s and audited
  through the configuration revision history. It acts on the effective
  mode: a route native by default gets an explicit stored `legacy`. Operators switch and roll
  back with `anix-control routes`, the admin API
  `/api/v4/kernel/route-modes` or the admin page; each switch is audited and
  recorded per route in `v4_kernel_route_mode_revision`, and `routes
  rollback` returns a whole package to `legacy` in one revision.
- `sdk/pluginhostsdk.Router` implements the modes, and `sdk/v2compat` keeps
  the output byte-identical. Route modes and shadow counters are exported
  on `/metrics`; sanitized samples of shadow mismatches are stored for 7
  days and listed by `GET /api/v4/kernel/route-modes/mismatches` and the
  admin page. A route without a native handler stays `legacy`
  (`mode_unsupported`), and the kernel refuses a non-legacy mode for a
  WebSocket route.

**Extraction mode** (CURRENT): `config/package-extraction.json`, enforced
by `check_plugin_only_routes.py` (counts in section 1).

| Mode | Meaning | Routes |
|---|---|---|
| `native-flagged` | the package host has a native handler, proved by a parity test; the legacy handler stays, so every runtime mode works | 155 |
| `native` | the legacy handler is deleted and the host answers alone | 0 |
| `bridged` | the host only relays, until a kernel contract lets the package serve the route | 36 |
| `kernel-owned` | the host only relays, by design: the route stays in the kernel, and the row's `reason` says why | 52 |

A `bridged` or `kernel-owned` route is registered and relayed alike: it is
in its host's `bridgedRoutes`, and no other source of its package names it,
so it has no native handler. The `kernel-owned` routes:

- platform: the generic system configuration routes (no package gets a
  settings grant over every namespace,
  [`settings-service.md`](settings-service.md#routes)) and backup creation,
  deletion and restore (archives on the kernel's disk);
- machine-telemetry: the system information (the kernel binary's build
  metadata) and the monitoring WebSocket;
- protocol-runtime: the agent channel (registration, heartbeat, task poll,
  result, monitor and the WebSocket): node credentials checked in the
  kernel, connections and reports in its memory;
- forward, decided in [`node-ops-service.md`](node-ops-service.md#6-route-mapping)
  (D3 and D4): the runtime status and doctor (the kernel's own executors,
  until the runtime moves to agents, A5), flow accounting (one kernel
  transaction; the callers send no batch id) and the agent channel (clean
  agent registration, heartbeat and report, and the agents' rule list);
- proxy-node: the node credentials display (no contract call reveals a
  stored secret, D4), node registration, heartbeat and runtime health (D3)
  and the node agent WebSocket.

5.0 removes the agent and node channel routes of forward, protocol-runtime
and proxy-node (D8).

What unblocks the `bridged` routes:

| Unblocked by | Routes | Count |
|---|---|---|
| KernelNodeOps and the node credential split (section 3.3) | forward: nodes, Ansible machines, clean agent tokens, every change applied on a node (a speed limit update included), runtime jobs; proxy-node: node administration, raw configuration, authorization keys, load balancer checks; protocol-runtime: node protocols, sync, Agent Control and agent operations | 74 |
| Open decision: whether UniProxy and the subscription renderer stay in the kernel | UniProxy (5) and the subscription preview | 6 |
| A proxy node view with parent and load | forward observability targets, trend and topology | 3 |
| A read of Control's process configuration `forward_runtime.clean_agent.public_url` (a KernelSettings-style namespace, or the host's environment) | the clean agent install script: its panel URL is that setting when set, else the request's scheme and host | 1 |
| A contract read of a member's subscription link | the public Telegram webhook (`/sub`) | 1 |
| A KernelSettings namespace for the subscription link (with `app.subscribe_path`) | the subscription link settings | 1 |
| A contract (or a storage adoption by identity-platform) for `v2_invite_code`, which registration consumes inside Control | identity-platform: the administrator's invite codes (list, generate, revoke) | 3 |
| Kernel caches (done, no invalidation events needed): the kernel keeps both caches and answers them through `KernelTelemetry.GetDashboard` and `KernelSubscriber.GetSubscriptionSummary`, so both modes answer the same entry and `cached_at` ([`kernel-caches.md`](kernel-caches.md)) | none left: the dashboard (the online set crosses as a count) and a user's subscription summary are `native-flagged` | 0 |

**Request address** (CURRENT). Hosts receive the original request's scheme
and host, so a native route can build absolute URLs as its legacy handler
does: the kernel sends `request_scheme` (`https` when the connection was TLS,
else `http`) and `request_host` (the `Host` header, only when it is a plain
`host[:port]`) as fields of `DispatchRequest` and `WebSocketOpen`, which the
SDK exposes as `RequestMetadata.Scheme` and `Host`. They are the values the
kernel's bridge already passed to the legacy handlers. They are new protobuf
fields, not keys of the request metadata JSON, which hosts built with the
v4.0.0 SDK decode with `DisallowUnknownFields`: those hosts skip the fields,
so no metadata version is needed. A host can tell a kernel that predates
them by the empty scheme; a native handler that needs them then returns
`pluginhostsdk.ErrNativeUnavailable`, and the router answers from the legacy
handler (in shadow mode the comparison is skipped). Since 4.1.0-rc.3 the
kernel resolves scheme and host with `internal/requestorigin`:
`X-Forwarded-Proto`/`X-Forwarded-Host` are folded in only when the peer is
a trusted proxy (`server.trusted_proxies`), and no forwarding header is
passed to package hosts or bridged handlers.

Status (2026-10-01): every installation runs every route `legacy` unless an
operator sets another mode; no route is `native` yet.

### 3.3 Typed kernel operations

Packages change other domains' data, and read protected data, only through
typed, idempotent kernel contracts. The kernel serves them to official
packages whose signed manifest declares the capability and authorizes every
call. Their writes are idempotent, so a retry applies once whichever side,
legacy handler or package, serves it.

| Contract | Capabilities | Holders |
|---|---|---|
| KernelIdentity ([`identity-service.md`](identity-service.md)) | `kernel.identity.v1` | identity-platform |
| KernelSubscriber ([`subscriber-service.md`](subscriber-service.md)): entitlements, traffic, credentials, balance, directory, subscription groups, the subscription summary the kernel caches | `kernel.subscriber.<family>.v1` | plan and order (entitlements), identity-platform and forward (resets), affiliate (balance), subscription (groups, summary) |
| KernelSettings ([`settings-service.md`](settings-service.md)): settings per namespace, secrets masked without the namespace's `secrets` capability | `kernel.settings.<namespace>.<read\|write\|secrets>.v1` | notification (`mail`), affiliate (`invite`), gost-mesh (`nodex`), platform (`backup`) |
| KernelOrder ([`order-service.md`](order-service.md)): a paid payment record completes its order | `kernel.order.complete.v1` | payment |
| KernelTelemetry ([`kernel-caches.md`](kernel-caches.md)): the administrator dashboard's snapshot from the kernel's cache, the online users as a count | `kernel.telemetry.dashboard.v1` | machine-telemetry |
| KernelNodeOps ([`node-ops-service.md`](node-ops-service.md)): typed, idempotent node operations with a ledger, polling and a watch stream; no operation kind executes yet | `kernel.nodeops.<forward\|nodeconfig\|diagnose\|agents\|credentials>.v1` | none yet (planned: forward, proxy-node, protocol-runtime) |

The planned `kernel.entitlement.apply.v1` became
`KernelSubscriber.ApplyEntitlement`.

- **Protected tables.** No manifest may adopt a table that holds credentials
  or kernel state (`service.protectedTables`): `v2_system_config`,
  `v2_audit_log`, `v2_operation_log`; the node credentials `v2_node` and
  `v2_authorized_key`; the node secrets `v2_node_protocol` and
  `v2_wireguard_peer`; the forward credentials `v2_forward_node`,
  `v2_forward_clean_agent` and `v2_forward_runtime_job`; the node
  credential split's `v4_kernel_node_credential`,
  `v4_kernel_protocol_secret` and `v4_kernel_node_secret_split`, named as
  well as covered by their prefix; and every `v2_user*`, `v3_kernel_*`,
  `v4_kernel_*`, `identity_*`, `kapi_*` and `pg_*` table.
- **Node credential split, phase P1 (dual-write).** The node credentials
  and protocol secrets of `v2_node`, `v2_authorized_key`,
  `v2_forward_node`, `v2_forward_clean_agent`, `v2_node_protocol` and
  `v2_wireguard_peer` are also kept in `v4_kernel_node_credential` and
  `v4_kernel_protocol_secret`, in clear like the legacy columns.
  `internal/nodesecrets` is their one writer: every kernel writer of a
  moved column calls it in the transaction of its legacy write.
  `anix-control node-secrets backfill` copies older rows and `verify`
  compares the digests of both forms; `v4_kernel_node_secret_split` holds
  each table's phase (`dual_write`) and the outcome.
- **Node credential split, phase P2 (dual-read).** Every kernel reader of
  a moved column reads through `internal/nodesecrets`, in its table's
  phase. `anix-control node-secrets phase <table|all> dual_read` moves a
  table's readers to the new tables after a recent matching `verify`; a
  missing or differing row falls back to the legacy column and is counted.
  `phase ... dual_write` moves them back. The legacy columns keep every
  value in both phases, so the tables stay protected and no package route
  changes. The credential-free tables become adoptable only once finalized
  (the KernelNodeOps design, section 4).
- **Kernel views.** Packages read other domains through 16 read-only
  `kapi_*` views (listed at the end of section 3.4), granted by
  `kernel.view:<view>`; none shows a credential.
- **KernelNodeOps (AVAILABLE, no executors yet).** Most `bridged` routes act
  on nodes. A module asks the kernel for a typed operation on a node (apply
  a forward, sync its protocols, run a diagnosis); the kernel holds the
  credentials, carries the operation out and answers the outcome,
  idempotently. The design, with the node credential split, Agent A2 and
  the plan for the 83 + 7 routes, is
  [`node-ops-service.md`](node-ops-service.md).
  - **Served since NO-1.** The kernel serves `sdk/api/kernelnodeops/v1`
    (`internal/kernelnodeops`) on local bridge sessions and the module
    listener. Manifests may declare its five capabilities. The contract is
    binding and grows by additions only.
  - **The engine.** The ledger is `v4_kernel_node_operation`, with its
    targets and event log, all protected. It has request-id idempotency,
    states that end once, a watch stream, quotas, fan-out counting and
    cancellation. Administrators list it at
    `GET /api/v4/kernel/node-operations`.
  - **No operation kind executes yet.** `SubmitOperation` answers
    `UNIMPLEMENTED` for every kind and records nothing, and
    `GetCapabilities` lists no kind. So every node route stays bridged
    until its kinds' executors land (NO-5 to NO-8).
  - **Sealed secret handles (NO-4).** A package never reads a node secret,
    even one an administrator types or is shown once.
    - **The field list.** `config/node-secret-fields.json` lists the
      routes and fields that carry one, per direction. It is kernel-owned:
      the kernel binary embeds it (`config.NodeSecretFields`), and the route
      gate checks every route id against `package-extraction.json`.
    - **Requests.** The v2 gateway (`internal/sealedsecrets`) replaces each
      secret of a listed route's body with an opaque handle,
      `anix-sealed:v1:<43 base64url characters>`, before the package host
      reads it. The handle is bound to the request, its route, target and
      field, and is single-use. The bridge capability keeps the original
      body, so the legacy handler, in any mode, reads the request as sent.
    - **Use.** A KernelNodeOps call carries the request's binding, which the
      kernel now verifies against the live dispatch. Its executor resolves
      the handles only for that request.
    - **Answers.** A secret the kernel generates is answered to the
      package as a handle, and the gateway expands it only in the answer to
      the request it was minted for.
    - **Fail closed.** A request that cannot be sealed is served by the
      kernel's legacy handler without the package host, and is counted in
      `anixops_v2_gateway_sealed_secrets_total`. An answer that would show
      a handle is refused. Handles live in kernel memory only and die with
      their request.

    The details are in `node-ops-service.md` section 3.7.
  - **What comes next.** Node credentials move to a protected table of
    their own (NO-2), so a package can adopt the rest of `v2_node`.

### 3.4 In-place adoption and kernel views

- Existing tables are adopted in place by grant: no copy, no dual write. The
  `v4_<pkg>_*` shapes in section 6 are long-term targets, not the first step.
- Consistency evidence is the shadow mismatch counter plus the migration
  validation digest (row count + content hash).
- **Knowledge pilot (in place).** It is the pattern for self-contained
  domains.
  - `packages/knowledge` declares `kernel.storage.v1` and
    `kernel.storage.adopt:v2_knowledge`.
  - Its host (`packages/knowledge/control`) serves all 6 routes natively on
    the adopted table (`packages/knowledge/native`), and
    `internal/tests/knowledgecompat` proves byte parity on SQLite and
    PostgreSQL.
  - Legacy handlers and native routes share the table, so a route's mode
    (the installation's `routes` configuration, optionally `shadow` for GETs
    first) can switch either way at any time. There is no import and no
    finalize.
  - Deleting the legacy handlers (mode `native`) follows once the operator
    has run natively for a release.
- **Ticket (in place).** The same pattern on `v2_ticket` and
  `v2_ticket_message`, proved by `internal/tests/ticketcompat`. The admin
  list's legacy preload of the user is not needed: only `user_id` is
  returned.
- **Notification (in place).** 23 of 24 routes on the adopted
  `v2_notification_template`, `v2_notification_log`, `v2_telegram_bot` and
  `v2_telegram_user` tables and the kernel's KernelSettings, proved by
  `internal/tests/notificationcompat`; binding e-mails come from
  `kapi_user_directory_v1`.
  - The e-mail configuration (GET and PUT) and the test send read and write
    `notification.email.config` through KernelSettings, namespace `mail`
    ([`settings-service.md`](settings-service.md)). Its value holds the SMTP
    password: the package reads it in clear (`kernel.settings.mail.secrets.v1`)
    because the test e-mail is sent from the package host; the GET masks it
    as the kernel's handler does, and an update that keeps it sends the
    placeholder. The parity test runs a test SMTP server and compares the
    mail each side delivers.
  - Setting the webhook without a `url` points it at
    `<scheme>://<host>/api/v2/telegram/webhook` of the administrator's
    request: the request scheme and host the kernel sends (forwarding
    headers count only from a trusted proxy, `internal/requestorigin`)
    (section 3.2, "Request address"). The parity test answers the Bot API
    calls of both sides and compares them.
  - One stays bridged: the public webhook (`/sub` needs the subscription
    token).
- **Platform (in place).** 5 of 12 routes: the backup list and statistics
  on the adopted `v2_backup_record` table, the system audit log through
  `kapi_system_audit_log_v1`, and the backup configuration (GET and PUT)
  through KernelSettings (namespace `backup`, read and write, no secrets),
  proved by `internal/tests/platformcompat`.
  - The package no longer adopts `v2_backup_config`: it never holds the S3
    keys, which KernelSettings answers masked, as the kernel's handler
    shows them. The parity test gives these routes no storage.
  - The update is written by the kernel: it saves the row, records the
    audit entry its handler records and makes the backup service reload
    the copy it keeps in memory, which backup creation reads. The parity
    test compares the rows, the audit entries and that copy.
  - Seven are `kernel-owned`:
    - the system configuration routes: they reach every key of the
      protected `v2_system_config`, and a grant over every key holds every
      secret ([`settings-service.md`](settings-service.md#routes));
    - creating, deleting and restoring backups: archives of the database
      and files on the kernel's disk.
- **Plan (in place).** The first module that changes shared subscriber
  state. All 7 routes run on the adopted `v2_plan` and `v2_event` tables,
  proved by `internal/tests/plancompat`.
  - `v2_event` is the plan domain's event log: only the plan routes write
    it, and nothing reads it.
  - An administrator's assignment calls `KernelSubscriber.ApplyEntitlement`
    (`kernel.subscriber.entitlements.v1`) over the bridge connection instead
    of writing `v2_user`.
  - Its request id is `plan.assign:<plan>:<user>:<digest>`, a digest of the
    request's `Idempotency-Key` (else its request id) and the expiry. The
    legacy handler derives the same id, so a retry applies once whichever
    side serves it. The parity test runs the real KernelSubscriber server
    and compares `v2_user`, the request ledger and the change log.
  - The five `/speed-limit/*` routes moved to forward: they are Flux
    forward limits, whose rows name forward tunnels (see Forward).
  - The subscription module checks a plan through `kapi_plan_catalog_v1`;
    the kernel's subscription renderer, which stays in the kernel, still
    reads `v2_plan` directly.
- **Order (in place).** All 13 routes run on the adopted `v2_order` and
  `v2_coupon` tables, proved by `internal/tests/ordercompat`: the coupon
  routes, order statistics, status changes, cancellation, "mark paid", the
  user's order creation, and the administrator's and user's order lists and
  details.
  - Other domains are read through views: `kapi_plan_catalog_v1` (a plan's
    prices, group, transfer and limits), `kapi_plan_subscription_group_v1`
    (the subscription groups a plan grants), `kapi_plan_name_v1` (a plan's
    name, for the order answers) and `kapi_user_directory_v1` (the buyer's
    current plan, which makes an order new, a renewal or an upgrade, and the
    buyer's e-mail in an administrator's order answers).
  - "Mark paid" completes the order through
    `KernelSubscriber.ApplyEntitlement` (`kernel.subscriber.entitlements.v1`)
    with `request_id = "order:<order id>"`, the id the kernel's completion and
    the payment callbacks use, so a plan is granted once whichever path
    completes the order. The kernel grants and completes in one transaction
    under the order's row lock. The module grants first, then marks the order
    completed; if that last write fails, the order stays paid with its plan
    granted, and a retry completes it without granting again. The parity test
    runs the real KernelSubscriber server and compares the orders,
    subscribers, subscription groups, request ledger and change log.
  - Completion pays no affiliate commission and sends no notification, in
    the kernel as in the module: `InviteService.AddCommission` and
    `NotifyOrderPaid` have no caller. Paying commission on completion needs
    an affiliate contract first.
  - The kernel keeps writing `v2_order`: it marks orders paid and completes
    them for the payment callbacks, legacy or through `KernelOrder`
    (`order-service.md`), and the dashboard and invite statistics read it.
  - The order list and detail answers carry the order, its plan's `id` and
    `name` and, for an administrator, its buyer's `id` and `email`. They
    embedded the buyer's whole `v2_user` row, subscription token and proxy
    UUID included, which no kernel view may expose, and the whole `v2_plan`
    row; the routes stayed bridged until the kernel's answers were slimmed to
    what the frontend reads (`docs/UPGRADE.md` lists the dropped fields).
    A user's list and detail are the caller's own orders: the owner is part
    of the query, and another user's order is "not found", as an unknown
    one. The parity cases include other users' orders, orders whose plan or
    buyer was deleted, and every list filter and paging edge.
- **Payment (in place).** All 20 routes run on the adopted
  `v2_payment_gateway`, `v2_payment_record` and `v2_payment` tables, proved
  by `internal/tests/paymentcompat`: gateway administration, payment records
  and statistics, the user's channels, payments and their status, the method
  list, the x402 and fiat payment creation (stubs that call no provider, on
  both sides), and the four provider callbacks.
  - Payment owns the gateways and so holds their secrets (merchant keys,
    webhook secrets) through the adopted `v2_payment_gateway`. Its
    administrator answers show them as `********`, as the kernel's do.
  - An order is read only through `kapi_order_billing_v1` (its buyer, total
    and status): a payment is created for the caller's own pending order and
    its exact total.
  - The four callback routes (`/payment/callback/:type`, the x402 callback,
    the Stripe and PayPal webhooks) verify the provider's signature with the
    gateway's secrets, as the kernel's do; the PayPal webhook calls PayPal's
    API for it. A paid callback takes two steps (`order-service.md`):
    1. the module marks the record paid and adds it to the gateway's
       statistics, in one transaction on its tables;
    2. the kernel applies the paid record to its order through
       `KernelOrder.CompleteOrderPayment` (`kernel.order.complete.v1`): it
       re-checks that the record pays the order (the record's user, a pending
       order, the amount covering its total), marks it paid and completes it
       with `order:<id>`, in one transaction, once per trade number.

    A repeat of a callback whose record is already paid runs step 2 again, so
    a failure between the steps converges and no order is paid without a
    paid record. The kernel's legacy callbacks call the same function in
    their single transaction. If step 2 fails, the module's callback fails
    (502) so the provider delivers again; the kernel's answers its own
    error. The parity test runs the real KernelOrder server: 78 callback
    cases on SQLite and PostgreSQL each.
- **Affiliate (in place).** All 10 routes run on the adopted
  `v2_commission_record`, `v2_commission_withdraw`, `v2_invite_config` and
  `v2_invite_code` tables, proved by `internal/tests/affiliatecompat`: the
  user's invite codes, code generation, commissions, withdrawals and
  withdrawal request, and the administrator's withdrawal list and
  decisions, invite statistics and configuration.
  - Other domains are read through views: `kapi_user_referral_v1` (who
    invited whom) for the statistics, `kapi_order_billing_v1` (an order's
    buyer and status) for the paying users a user invited,
    `kapi_subscriber_entitlement_v1` for the caller's commission balance,
    and `kapi_affiliate_settings_v1` for the frontend settings (code prefix
    and length, withdrawal fee and methods).
  - **Invite codes.** The user's invite codes and their generation moved
    from identity-platform (`affiliate.user.invite.get`,
    `affiliate.user.invite.generate.post`). `v2_invite_code` is not a
    protected table: a code admits a registration and attributes the
    referral, and holds no credential of an existing account. Control's
    registration (the kernel's, and the identity module's through
    `CreateSubscriber`) still consumes a code inside the transaction that
    creates the user.
    - A user holds at most `code_count` (default 5) unused codes. The
      kernel used to count them under a row lock on the user's `v2_user`
      row, which no package may read or write. Both sides now count and
      create under one PostgreSQL advisory lock keyed by the user,
      `pg_advisory_xact_lock(class, user)` with the class `0x696e7663`
      ("invc") and the user id masked to 31 bits
      (`service.InviteCodeLockKeys`, `native.InviteCodeLockKeys`).
      Advisory locks need no grant, so the package role takes the lock the
      kernel takes; a test proves a leased role waits for it. On SQLite one
      writer runs at a time, and a generation that read before another's
      write is retried, on both sides.
    - The parity test also runs the kernel's and the package's generations
      concurrently on one database: they never pass the limit. With the
      kernel's old row lock they did.
  - `kapi_affiliate_settings_v1` shows one row of the protected
    `v2_system_config`: the value of `invite.frontend.config`, a key the
    kernel does not treat as sensitive and shows its administrators in
    clear. On PostgreSQL it is a `security_barrier` view, so a function in
    the package's own query cannot see the rows its filter hides; a test
    shows a secret leaking without the barrier.
  - The commission balance is the kernel's (`v2_user.commission_balance`).
    A withdrawal debits it and a rejection refunds it through
    `KernelSubscriber.AdjustBalance` (`kernel.subscriber.balance.v1`, kind
    `COMMISSION`; no contract change), with the ledger ids the kernel's
    legacy handlers use: `affiliate.withdraw:<id>` and
    `affiliate.withdraw.refund:<id>`.
  - The kernel writes a withdrawal and its debit, or a rejection and its
    refund, in one transaction; the module cannot. A withdrawal is first a
    reservation (status `-1`) that no decision accepts, and becomes pending
    once debited; a refused debit removes it, and an unknown outcome leaves
    it for an administrator (`?status=-1`) to reconcile with the ledger. A
    rejection claims the pending withdrawal, then refunds it; a refused
    refund puts it back to pending, and an unknown outcome leaves it
    rejected with a failed answer. No path pays out or refunds an amount
    that was not debited.
  - Updating the configuration writes the adopted `v2_invite_config`, then
    `invite.frontend.config` through KernelSettings (namespace `invite`),
    which makes the kernel's invite services reload the configuration they
    keep in memory. Neither path records an audit
    entry.
- **Identity leftovers (in place).** All 7 of identity-platform's routes
  outside group A run natively, proved by `internal/tests/identitycompat`
  (byte parity and the same Control state, on SQLite and PostgreSQL).
  identity-platform adopts no kernel table.
  - **Account reads:** the user's profile and dashboard and the
    administrator's user detail, user list and user statistics.
    - The account (email, administrator, staff and ban flags) comes from
      identity's own store. Identity's store is current only while identity
      is authoritative, so the kernel lets these five routes leave legacy
      mode (shadow or native) only then
      (`service.IdentityAccountReadRoutes`). The rollback returns them to
      legacy with group A. The cutover leaves them to the operator: they are
      not part of group A, and group A's rule is unchanged.
    - The subscriber fields come from `KernelIdentity.GetSubscriber`
      (`kernel.identity.v1`), for one user per call, never from a view:
      plan, traffic and expiry, and the subscription token and proxy uuid
      that the profile and the administrator's detail show.
      `GetSubscriber` now includes the subscriber's plan, as the v2 detail
      does.
    - The plugin permissions come from `ResolveActorAccess`.
    - The dashboard is computed on every request; the kernel cached it for
      30 seconds. The kernel's dashboard no longer shows the subscription
      link settings, which a summary read used to write into its cached
      entry ([`kernel-caches.md`](kernel-caches.md)); identity's never did.
  - **User directory:** the administrator's user list and statistics
    (`native.UserDirectory`, [identity-service.md](identity-service.md#user-directory)).
    - One query on identity-platform's own storage joins its `account`
      table with `kapi_user_directory_v1` and the newly declared
      `kapi_subscriber_entitlement_v1` and `kapi_plan_name_v1`. It filters
      by e-mail, plan and status ("active" is not banned in identity and
      not expired in Control), orders by `created_at DESC, id DESC`, pages
      and counts.
    - The total and the page come from one read-only, repeatable-read
      transaction, so they agree.
    - The list no longer shows any user's subscription token or proxy UUID,
      in legacy mode either: each user is the account, the subscription
      summary and its plan as `{id, name}`. The token is read for one user
      from the user detail (`docs/UPGRADE.md` lists the removed fields).
  - **Resets:** the administrator's traffic reset and subscription reset.
    - They change only the subscriber, through
      `KernelSubscriber.ResetTraffic` (`kernel.subscriber.traffic.v1`) and
      `ResetCredentials` (`kernel.subscriber.credentials.v1`), so they switch
      at any time.
    - Their request id is `identity.reset_traffic:<user>:<digest>` or
      `identity.reset_subscribe:<user>:<digest>`, a digest of the request's
      `Idempotency-Key` (else its request id). The legacy handlers now derive
      the same id and record it in the subscriber request ledger, so a retry
      applies once whichever side serves it. A retried subscription reset
      answers the token the first one issued.
    - `ResetCredentials` and `AdjustEntitlement` now refuse a subscriber
      that does not exist (`NotFound`) instead of recording an empty change.
  - **Stay bridged:** the user's invite codes and their generation. They
    are affiliate data, not identity's. Control keeps the codes
    (`v2_invite_code`, which registration consumes inside Control), and the
    answer adds the commission balance and invite statistics that join the
    order and affiliate packages' tables. They belong with the affiliate
    package.
  - **Moved to affiliate:** the user's invite codes and their generation,
    affiliate data rather than identity's (see Affiliate).
  - **Administrator's invite codes (bridged, every edition).**
    `GET`/`POST /api/v2/admin/invite/codes` and
    `DELETE /api/v2/admin/invite/codes/:id`
    (`identity.admin.invite.codes.*`) are registration control, so they are
    identity-platform's and the community edition serves them (owner
    decision 2026-10-01); the affiliate package, which the community
    edition hides and does not ship, keeps the user's codes, commissions,
    withdrawals and statistics. They are new routes, not moves, so no
    parity test applies: the host relays them (`bridgedRoutes`) to
    `handler.InviteCodeAdminHandler` until identity-platform can read
    `v2_invite_code`. An administrator's codes belong to no user and do not
    count toward a user's `code_count`; the user's generation, its limit
    and advisory lock are unchanged.
  - **User's own subscription reset (group A, every edition).**
    `POST /api/v2/user/subscription/reset`
    (`identity.user.subscription.reset.post`, owner decision 2026-10-02).
    It is identity-platform's, beside the administrator's reset, and in
    group A rather than among the resets that switch at any time: before it
    resets, it checks the user's password or second factor, which identity
    owns once authoritative (and the kernel no longer holds after finalize).
    The reset itself is the administrator's: `KernelSubscriber.ResetCredentials`
    with `subscription_token` only, under
    `identity.user_reset_subscribe:<user>:<digest>`. Both sides allow three
    attempts that check a credential per user and hour (the kernel in
    memory, identity in its `throttle` table), and the kernel's audit
    middleware records it as `user` / `reset_subscribe` in every mode.
    Proved by `internal/tests/identitycompat/subscription_reset_test.go`.
- **Subscription (in place).** 23 of 25 routes run on the adopted
  `v2_subscription_group`, `v2_subscription_template`,
  `v2_plan_subscription_group` and `v2_subscription_group_node_protocols`
  tables, proved by `internal/tests/subscriptioncompat`: groups (list, read,
  create, update, delete) and templates (list, read, create, update,
  delete), a group's node protocol links, a plan's groups, a user's groups
  (list, grant, take away), the statistics, the two static lists, the
  user's subscription summary, and a group's protocols and the protocol
  pool once the node credential split is finalized (below). The
  subscription link endpoints (`/s/:token`,
  `/api/v1/client/subscribe`) are kernel routes outside the package gate and
  stay in the kernel with the renderer.
  - Other domains are read through views: `kapi_plan_catalog_v1` (whether a
    plan exists), `kapi_subscriber_entitlement_v1` (whether a user exists,
    and their traffic), and the new `kapi_user_subscription_group_v1` (the
    groups a subscriber holds, until when), `kapi_node_protocol_v1` (a
    protocol's node) and `kapi_node_heartbeat_v1` (a node's last report);
    after the split's finalize also `kapi_node_protocol_public_v1` and
    `kapi_node_public_v1` (below). No view shows a token, UUID, e-mail
    address or key, and protocol settings only redacted.
  - The kernel's renderer, order completion and
    `kapi_plan_subscription_group_v1` read the adopted tables; the module's
    writes leave the rows the legacy handlers leave. The kernel caches
    nothing about them and no write pushes to nodes. Linking protocols
    touches the group's `updated_at`, as the kernel's association replace
    does.
  - The bound and answered types mirror the kernel model in
    `packages/subscription/native/model`, a package named `model` like the
    kernel's, because JSON binding errors print a field's type with its
    package; a test compares the two.
  - Group and template writes no longer save the associations nested in a
    body (a security fix made in the kernel first): a group's `protocols`
    upserted proxy-node rows and nodes, which the module may not write.
  - **Memberships go through the kernel.** `v2_user_subscription_group` is
    subscriber state: only the kernel writes it, and no package may adopt a
    `v2_user*` table. Granting a user a group, taking it away and deleting
    a group call `KernelSubscriber` (`kernel.subscriber.groups.v1`,
    [`subscriber-service.md`](subscriber-service.md)):
    - a grant is `GrantSubscriptionGroup`, a removal
      `RevokeSubscriptionGroup`;
    - deleting a group first takes it from every member
      (`RemoveSubscriptionGroupMembers`), then deletes the group's
      templates, plan links and node protocol links with the group, in one
      transaction on the adopted tables. A retry after a failure in between
      finds no members and completes.

    The request ids are `subscription.grant:<user>:<group>:<digest>`,
    `subscription.revoke:<user>:<group>:<digest>` and
    `subscription.delete_group:<group>:<digest>`. The digest covers the
    request's `Idempotency-Key` (else its request id) and, for a grant, the
    granted fields. The kernel's legacy handlers derive the same ids and
    call the same engine functions, so both sides write the same
    memberships, request ledger and change log. Without a bridge connection
    these routes stay legacy.
  - **The user's subscription summary comes from the kernel's cache.** The
    kernel caches each user's summary for 30 seconds, an entry its legacy
    user dashboard reads too, and adds the subscription link settings on
    every request. The module reads the entry through
    `KernelSubscriber.GetSubscriptionSummary` (`kernel.subscriber.summary.v1`,
    [`kernel-caches.md`](kernel-caches.md)), the function the legacy
    handler calls. Both modes therefore answer the same entry with the same
    `cached_at`, and `refresh=true` rebuilds it. The answer carries no
    token or UUID. Without a bridge connection the route stays legacy.
  - **A group's protocols and the protocol pool read the split's public
    views (M3-3).** They answer whole node protocols with their nodes, so
    they read `kapi_node_protocol_public_v1` and `kapi_node_public_v1`
    ([`node-ops-service.md`](node-ops-service.md) section 4.6): every
    column but the node's API key, key hash and shared secret, the JSON
    columns in the redacted form a finalized table keeps. The kernel
    creates both views, and grants them to the package's lease, only once
    `v2_node_protocol` and `v2_node` are finalized. Until the lease grants
    both, the routes answer from the legacy handler in every mode, also on
    SQLite, where the package shares the database file that holds the
    tables. The lease is taken when the host first opens its storage: a
    host started before the finalize serves them natively after a restart.
    The answer masks every secret position again
    (`v2compat.RedactNodeSecrets`, the kernel's rule, which
    `internal/nodesecrets` calls), so a value written past the kernel's
    writers is masked as the legacy answer masks it.
  - Two routes stay bridged:
    - the preview: the kernel's renderer, with the user's token and UUID,
      the nodes, and the WireGuard peers it creates (whether the renderer
      stays in the kernel is an open decision);
    - the subscription link settings: `app.subscribe_path` is process
      configuration and `app.subscribe_domains` lives in the protected
      `v2_system_config`. A KernelSettings namespace can carry only
      `v2_system_config` and `v2_backup_config` keys, so this needs a new
      kind of namespace with the process configuration.
- **Proxy node (in place).** 7 of proxy-node's 31 routes run natively,
  proved by `internal/tests/proxynodecompat`: the load balancer list,
  detail, creation, update and deletion on the adopted `v2_load_balancer`
  (only these routes use it), a node's runtime logs on the adopted
  `v2_node_log` (the kernel's agent control writes it), and the node
  statistics.
  - **Node credentials stay in the kernel.** proxy-node owns the node
    domain, but `v2_node` also holds each node's API key, key hash and
    shared secret, which the kernel's node authentication checks: the node
    API, UniProxy, the agent WebSocket and gRPC control stream, and agent
    package downloads. A package that could read or write those columns
    could act as any node, read every subscriber's proxy UUID from the
    UniProxy user list, or let in a node of its choosing. Owning the domain
    is not owning that boundary. Until column-level grants (section 3.1)
    can withhold the credential columns, or node authentication moves into
    the module behind a contract, `v2_node` and `v2_authorized_key` (the
    registration keys, stored in clear, that mint node credentials) are
    protected kernel tables (`service.protectedTables`).
  - The statistics, and whether a node exists for its logs, come from
    `kapi_node_status_v1`: each node's id, status, last check and traffic
    counters.
  - `v2_node_protocol` (Reality private keys, protocol settings, custom
    configurations) is not proxy-node's: the protocol routes belong to
    protocol-runtime, whose extraction decides whether it may hold those
    keys, as payment holds its gateways' secrets.
  - **Stay bridged** (19) or **kernel-owned** (5: the credentials,
    registration, heartbeat, runtime health and the agent WebSocket), with
    the reason in the host's route map:
    - node list, detail, creation, update, deletion, credentials
      (`kernel-owned`, D4) and raw configuration: they read or write the
      node credentials, the list and detail embed each node's protocols
      with their Reality private keys, the raw configuration carries
      WireGuard private keys, an update
      clears the kernel's in-memory node cache, and a deletion removes the
      node's protocols and WireGuard peers in one transaction;
    - configuration validation: no table, but the kernel's WireGuard
      protocol validator, which the protocol routes share;
    - the authorization keys: the list answers each key in clear, and the
      kernel's HTTP and gRPC registration read them;
    - registration, heartbeat and runtime health (`kernel-owned`, D3):
      authenticated or minted node credentials, and writes to `v2_node`;
    - the agent WebSocket (`kernel-owned`), a live connection the kernel
      holds and pushes to, and UniProxy: node-authenticated, with every
      eligible subscriber's UUID in the user list, subscriber traffic in a
      push and the online list in the kernel's in-memory cache;
    - the load balancer statistics and health check: they read the forward
      package's `v2_forward_node`, and the check probes each forward node
      and writes its status.
- **Forward (withdrawn in v4.2, F5d).** Until v4.1, 21 of 85 routes ran
  natively on the adopted `v2_forward`, `v2_forward_tunnel`,
  `v2_forward_user_tunnel`, `v2_speed_limit`, `v2_forward_rule` and
  `v2_forward_latency_bucket` tables, proved by
  `internal/tests/forwardcompat`. F5d withdrew them: the forward package
  adopts no table and reads no view (its manifest declares only
  `kernel.forward.v1`, for the v4 API), so the legacy cleanup (F5c) can drop
  the flux tables without breaking the package's storage lease; the 21
  routes are `kernel-owned` (the kernel serves them until F5c), they left
  `config/package-route-defaults.json`, and `forwardcompat` is deleted. They
  were:
  - the forward lists and their display order (user and administrator);
  - the tunnel list, creation and the deletion of an unused tunnel, and the
    tunnels a forward may use;
  - granting and listing tunnel permissions (the list raises a
    permission's traffic to its forwards' totals, as the kernel does);
  - a forward's ingress latencies, the node statistics and the user's
    legacy rules;
  - the administrator's `POST /api/v2/user/reset`: type 1 resets a
    subscriber's traffic through `KernelSubscriber.ResetTraffic`
    (`kernel.subscriber.traffic.v1`), with the request id
    `forward.reset_traffic:<user>:<digest>` that the legacy handler now
    derives too; type 2 a permission's traffic;
  - the speed limits, moved from plan (`forward.speed_limit.*`): creation,
    the list, the deletion of a limit no permission names, and the tunnels
    a limit may name (every active one, for any caller, as the kernel
    answers). The update, which re-applied the forwards of every permission
    that names the limit on their nodes, was removed in v4.2 (F5d).

  None of them changes what a node runs: a tunnel, a permission or a speed
  limit alone runs nothing, the display order is not part of a forward's
  runtime payload, and the kernel's forward runtime re-reads these rows on
  every use.
  - Forward nodes are read through `kapi_forward_node_v1` (every column but
    `api_token`), the runtime backend through
    `kapi_forward_runtime_settings_v1` (three keys of the protected
    `v2_system_config`, a security barrier), users through
    `kapi_user_directory_v1` and their speed limit through
    `kapi_subscriber_entitlement_v1`.
  - `v2_forward_node`, `v2_forward_clean_agent` and
    `v2_forward_runtime_job` are protected kernel tables no manifest may
    adopt. A forward node's API token authenticates its agent (WebSocket,
    gRPC and REST), a clean agent's token authenticates it, and clean agent
    jobs carry the node's API token in their payloads: a package holding
    them could act as any forward node or agent.
  - **Removed in v4.2 (F5d):** the 53 `bridged` forward routes — the 30
    flux routes (every forward change, pause, resume, deletion and
    diagnosis, the backend sync, the tunnel update and diagnosis,
    permission removal and updates, the speed limit update, the legacy
    rules other than the user's list, and the runtime job list), the 19
    node management routes F5a rewrote as `/api/v4/forward/*` (forward
    nodes, Ansible machines, the observability targets, trend and
    topology) and the 4 clean agent routes (list, create, revoke and the
    install script). [`forward-sdk.md`](forward-sdk.md) section 10 lists
    them; [`../forwarding/v4-api.md`](../forwarding/v4-api.md) is the
    replacement.
  - **Kernel-owned (11: D3 and D4 in
    [`node-ops-service.md`](node-ops-service.md#6-route-mapping)):**
    runtime status and diagnosis, local and NodeX (the protected NodeX and
    Ansible settings, NodeX calls, Control's disk); the agents' rule list
    (node token authentication); clean agent registration, heartbeat and
    report (agent tokens, job claims); and flow upload, report and snapshot:
    the forward's counters, the subscriber's traffic and the permission's
    traffic change in one kernel transaction under a per-forward lock in
    Control's memory, and exhaustion pauses forwards on their nodes. They
    go with the legacy runtime (F5c, 5.0).
- **Protocol runtime (in place).** 3 of protocol-runtime's 20 routes run
  natively, proved by `internal/tests/protocolruntimecompat`: the protocol
  templates (static data) and the administrator's diagnostic task history
  and detail on the adopted `v2_agent_diagnostic_task`.
  - The kernel keeps writing that table: creating a task, the agents' polls
    and results. Since a package may now write it too, the agents' HTTP poll
    checks every pending task against the diagnostic whitelist again before
    an agent gets it (`AgentDiagnosticTaskService.PullPendingTasks`); a task
    off the whitelist fails.
  - **Reality and WireGuard keys stay in the kernel.** `v2_node_protocol`
    and `v2_wireguard_peer` are protected kernel tables
    (`service.protectedTables`), and the protocol routes stay bridged.
    - `v2_node_protocol` holds each protocol's Reality private key,
      WireGuard server private key and custom configuration, and
      `v2_wireguard_peer` every user's WireGuard private and preshared keys.
      Whoever holds them can impersonate a node to its clients.
    - The kernel builds every node's configuration (UniProxy, the gRPC node
      service) and every subscription from `v2_node_protocol`, and validates
      a protocol only when its own routes write it. A package that could
      write the table could push unvalidated configuration (WireGuard relay
      files, interfaces, routing tables) to every node.
    - Column-level grants (section 3.1) do not exist, so the secrets cannot
      be withheld from an adopting package. Payment holds its gateways'
      secrets because the payment domain uses them; the protocol keys are
      used by the kernel's node configuration and subscription rendering,
      not by these routes.
    - A bridged answer still passes through the package host, so the host
      sees the keys of the protocols an administrator opens or saves. The
      protection removes standing access to every key and every write, not
      that relay.
  - **Stay bridged** (11) or **kernel-owned** (the 6 agent routes), with
    the reason in the host's route map:
    - the node protocol list, creation, update and deletion: the protected
      `v2_node_protocol`; the list answers the keys in clear (the
      administrator's editor round-trips them), and a deletion also deletes
      the protocol's WireGuard peers and subscription group links;
    - node synchronization, Agent Control status and operations: the gRPC
      control streams the kernel's Agent Control manager holds, and the
      protected `v2_node`;
    - the administrator's agent list, monitoring data, task creation and
      command execution: the agents' live WebSocket connections and reports
      in the kernel's memory; creating a task sends it over the connection
      and waits for the acknowledgement;
    - the agent routes (registration, heartbeat, task poll, result, monitor,
      WebSocket; `kernel-owned`): node credentials checked in the kernel,
      node status in `v2_node` and `v2_forward_node`, connections in the
      kernel's memory, and the forward package's bridge tasks and runtime
      jobs.
- **Machine telemetry (in place).** 3 of machine-telemetry's 5 routes run
  natively, proved by `internal/tests/machinetelemetrycompat`: the
  administrator's hourly traffic series, the user traffic ranking and the
  dashboard.
  - The package adopts no table. It reads the traffic log through the new
    `kapi_traffic_log_v1` (the user, bytes, rate and time of each node
    traffic report in `v2_server_log`, no node or credential) and e-mail
    addresses through `kapi_user_directory_v1`. The kernel writes the log
    when a node reports traffic, in the transaction that counts the
    subscriber's traffic; a package that could write it would change the
    charts, so it only reads it.
  - The hourly buckets are local hours, as the kernel's; the kernel passes
    its time zone to the host.
  - **The dashboard comes from the kernel's cache.** The snapshot counts
    users (the protected `v2_user`), orders, revenue and the legacy server
    tables, and takes the online users from the alive set in the kernel's
    memory. The kernel caches it for 60 seconds. The module reads it
    through `KernelTelemetry.GetDashboard` (`kernel.telemetry.dashboard.v1`,
    [`kernel-caches.md`](kernel-caches.md)), the function the legacy
    handler calls, so both modes answer the same snapshot and `cached_at`,
    and `refresh=true` rebuilds it. The online set leaves the kernel only
    as a count. Without a bridge connection the route stays legacy.
  - **Kernel-owned** (the other two), with the reason in the host's route
    map:
    - the system information: the kernel binary's own build metadata;
    - the monitoring WebSocket: the kernel's node list from the protected
      `v2_node`; WebSocket routes always relay to the kernel.
- **Gost mesh (in place).** All 3 of gost-mesh's routes run natively,
  proved by `internal/tests/gostmeshcompat`.
  - The administrator's gost API connection test reads no table. Since
    NO-7 the kernel runs it: the gateway seals the typed token, the host
    submits `diagnose.forward_backend` with its request binding, and the
    kernel calls the gost API at the host and port in the request with the
    kernel's own client, so the answer carries the same errors
    (`docs/architecture/node-ops-service.md`, section 6.1).
  - The NodeX runtime status and diagnosis read the NodeX address, shared
    token and timeout through KernelSettings (namespace `nodex`; the token
    in clear with `kernel.settings.nodex.secrets.v1`) and call NodeX from
    the package host. The parity test runs test NodeX servers that answer,
    refuse the token, fail or answer what does not decode.
- **WireGuard (in place).** wireguard's one route runs natively, proved by
  `internal/tests/wireguardcompat`: the administrator's server keypair for
  the protocol form. It reads and stores nothing; the administrator saves
  the private key into a protocol through protocol-runtime's bridged routes
  on the protected `v2_node_protocol`. The answer carries a private key
  either way: a bridged answer passes through the package host too, so
  generating it in the package gives the package nothing it did not see.
  The parity test masks the random keys and checks on both sides that each
  answer is a fresh X25519 pair.
- The kernel publishes read-only views `kapi_*`, created at startup by
  `EnsureKernelAPIViews`: `kapi_user_directory_v1`,
  `kapi_system_audit_log_v1` (the `v2_operation_log` rows of module
  `system`), `kapi_subscriber_entitlement_v1`, `kapi_plan_catalog_v1`,
  `kapi_plan_name_v1`, `kapi_plan_subscription_group_v1`,
  `kapi_order_billing_v1`, `kapi_user_referral_v1`, `kapi_traffic_log_v1`,
  `kapi_affiliate_settings_v1`, `kapi_user_subscription_group_v1`,
  `kapi_node_protocol_v1`, `kapi_node_heartbeat_v1`, `kapi_node_status_v1`,
  `kapi_forward_node_v1` and `kapi_forward_runtime_settings_v1`. Packages
  read other domains only through `kapi_*` views or typed operations. A view
  whose source table does not exist is left out. A view that filters rows is
  a PostgreSQL `security_barrier` view.

### 3.5 Isolation switch and SQLite

- `plugins.storage_isolation: role | shared`. `role` is the PostgreSQL default;
  `shared` is the escape hatch (lease uses the kernel pool with SDK-enforced
  table allowlists) for PG grant problems (sequence privileges, PG15+ `public`
  schema defaults).
- SQLite has no roles or schemas. Proposed: on SQLite the lease is always
  `shared`, tables use a `pkg_<id>_` prefix instead of a schema, and
  `role` is rejected at startup. Production extraction targets PostgreSQL.

### 3.6 Rejected alternatives

- SQL over IPC: about 3x the code and no smaller privilege.
- In-process business modules: would discard the shipped signing, lifecycle,
  and rollback model.

## 4. Security Invariants

**Amended invariant (CURRENT since M3):** a package host never receives
**kernel** credentials — the kernel DSN, JWT signing key, or full Control
configuration. It may lease its own per-package least-privilege role; see
`plugin-kernel-contract.md`, "Package Storage".

**Session-scoped RPCs (CURRENT since M3):** besides the per-request
`Invoke`/`OpenWebSocket`, a host may call named session operations
(`GetPackageConfig`, `LeaseStorage`) without a capability. They are
authorized by the session identity (package id, version, lifecycle
generation bound to the inherited socketpair) and fenced against the current
installation on every call; see `plugin-kernel-contract.md`, "Package
Configuration".

Preserved bridge invariants (from `2026-07-19-v2-package-bridge.md`, still
CURRENT; the per-request capability rule applies to `Invoke` and
`OpenWebSocket`):

- A host receives only a private inherited socket endpoint; never a DSN,
  signing key, full configuration, or arbitrary SQL capability.
- The parent endpoint is bound to one package id, version, lifecycle
  generation, and host lease. A replacement generation gets a new bridge.
- Every call carries a cryptographically random, short-lived, single-request
  capability minted by the kernel after v2 route resolution; the host cannot
  choose the principal, route, package, or deadline.
- The bridge exposes explicit named operations only and rejects unknown
  operations, wrong route, stale generation, reused capability, and expired
  deadline. No generic query, filesystem, config, process, or token-signing
  operation.
- The bridge returns application data only; HTTP status, headers, and framing
  stay with the host RPC and compatibility gateway.
- The bridge is a **migration adapter**, not a hidden database escape hatch.
  "Package-owned projections and checkpointed import migrations can replace
  each operation independently" once the data contract exists.

Identity waves (bridge plan): login/registration first, then profile/session,
MFA/invitations, administration, and platform configuration/audit/backup. Each
wave keeps its gin middleware and needs an enabled-host assertion, a
disabled-host 503 assertion, and a rollback assertion before the next wave.

Bridge non-goals: no direct database credentials or arbitrary SQL interface
(amended only by the storage lease above); no legacy handler as a router
fallback after a route is cut over; no response-envelope guessing — every
signed route declaration names its transport and envelope.

## 5. Preserved Constraints From Retired Plans (HISTORICAL, still guiding)

From `2026-07-18-v4-plugin-only-stable.md`, Global Constraints (condensed; the
package list is now sixteen with `identity-platform`):

- Product v4.0.0; module paths stay `github.com/AnixOps/anix-control/v4` and
  `github.com/AnixOps/anix-agent/v4`.
- A package is the only owner of its domain behaviour, migrations,
  compatibility projection, background work, and WebUI. Kernel code may not
  import a package domain service or query a package-owned table.
- Supported `/api/v2` path, method, auth semantics, request fields, envelope,
  and documented error codes stay stable, entering a package adapter with no
  fallback to coupled legacy code.
- Packages are separately supervised local processes, never network
  listeners; they receive only authenticated, authorized, deadline-bound
  requests over a permission-restricted Unix socket.
- Every artifact, manifest, WebUI bundle, migration digest, and entrypoint is
  Ed25519-signed and verified against the official root before activation.
- Every mutation carries a stable request identity and idempotency key, is
  audited, and is fenced by the package route generation.
- Database work stays additive until verified backup, validation, reverse
  migration, and the 72-hour canary pass. No hand edits of rows, package
  files, or route state during an upgrade.
- Rollout 1/5/25/100 % of a cohort; host failure, lease loss, validation
  mismatch, incompatible response, duplicate non-idempotent mutation, health
  failure, or error/latency breach halts and returns to the previous verified
  generation.
- SQLite and PostgreSQL both pass clean bootstrap, upgrade, backup/restore,
  and reverse-upgrade rehearsals.
- Agent changes ship as a tagged `github.com/AnixOps/anix-agent/sdk`; no
  submodules either way. (Superseded: the Agent contract now lives in
  `github.com/AnixOps/anix-control/sdk`, and `anix-agent/sdk` is frozen at
  v1.1.0.)

From `2026-07-19-v2-full-package-cutover.md`, Global Constraints (condensed):

- Every supported `/api/v2` business method/path has exactly one package
  owner, signed declaration, route id, envelope, middleware group, and
  transport (`config/v2-package-route-catalog.json`).
- `router.go` keeps its middleware chain; final registrations call the
  gateway, never a legacy handler/service/model/worker.
- Verified installed artifacts are the only runtime source of route
  declarations; the source catalog is a build-time gate, never a fallback.
- Disabled, unsigned, stale, unhealthy, or incompatible packages fail closed.
- WebSocket routes relay over versioned bidirectional Unix gRPC after normal
  HTTP middleware.
- Each ownership wave is additive and generation-fenced. A missing
  `ANIX_TEST_POSTGRES_DSN` is an evidence gap, never a pass.

Cutover waves (Tasks 6–9, condensed):

| Wave | Packages | Must hold |
|------|----------|-----------|
| A: content/support/catalog | knowledge, notification, ticket, plan | Notification outbox exactly-once; generation-fenced ticket state |
| B: commercial/subscription | order, payment, subscription | Callback receipt persisted before any transition; order owns coupons; byte-stable subscription output |
| C: node/runtime/topology | proxy-node, protocol-runtime, wireguard, forward, machine-telemetry, nftables-forward, gost-mesh, nat-egress | Includes the 3 WebSocket paths; legacy handlers removed only after package tests and migrations pass |
| Final enforcement | all | `check_plugin_only_routes.py` rejects a catalogued direct handler, missing declaration, owner mismatch, missing host, or legacy WebSocket binding; kernel handlers allowed only where the catalog says `transport: "kernel"` |

Reality: the final gate passes today because `registeredPackageRoute` returns
the gateway; the legacy handler is still reached through the bridge registry.
The planned `config/package-extraction.json` (M3, section 10) closes that gap.

## 6. Per-Domain Target Data Contracts (non-binding)

Route counts are from `config/v2-package-route-catalog.json`; interfaces and
tables are from Tasks 7–12 of the retired stable plan. Table names are target
shapes; the first extraction step adopts the existing `v2_*` tables in place.

| Domain (task) | Routes | Planned interfaces | Planned tables | Notes |
|---------------|-------:|--------------------|----------------|-------|
| knowledge (T7) | 6 | `knowledge.article.list/read`, `knowledge.admin.write` | `v4_knowledge_article`, `v4_knowledge_category` | Pilot; adopts `v2_knowledge` |
| notification (T7) | 24 | `notification.notice.list`, `notification.delivery.enqueue/retry` | `v4_notification_notice`, `_delivery`, `_outbox` | Retry idempotent, one outbox row; SMTP/Telegram send moves to host |
| ticket (T8) | 8 | `ticket.list`, `ticket.message.create`, `ticket.status.transition` | `v4_ticket_ticket`, `v4_ticket_message` | Generation-fenced transitions; adopts `v2_ticket`, `v2_ticket_message` |
| plan (T8) | 7 | `plan.catalog.list`, `plan.assignment.read`, `plan.entitlement.read` | `v4_plan_catalog`, `_assignment`, `_quota` | Publishes versioned `EntitlementReader`; the 5 `/speed-limit/*` routes, Flux forward limits, moved to forward |
| order (T9) | 13 | `order.create`, `order.apply_coupon`, `order.transition`, `order.request_entitlement` | `v4_order_order`, `_promotion`, `_coupon_redemption` | Owns coupons; entitlement requests are durable messages |
| payment (T9) | 20 | `payment.initiate`, `payment.callback`, `payment.reconcile` | `v4_payment_record`, `_callback`, `_outbox` | Receipt stored before transition; exactly-once per gateway event; one-time secret lease |
| subscription (T10) | 25 | `subscription.render`, `subscription.usage.read` | `v4_subscription_group`, `_template`, `_usage` | Byte-identical output incl. content type and cache headers |
| proxy-node (T10) | 31 | `proxy-node.register`, `.config.deliver`, `.user.deliver`, `.traffic.report` | `v4_proxy_node`, `_config`, `_user`, `_usage` | Delivery becomes a versioned Agent operation; kernel gRPC transport-only |
| forward (T11) | 32 | `forward.rule.create/update`, `forward.tunnel.assign`, `forward.observation.read`, `forward.agent.apply` | `v4_forward_rule`, `_tunnel`, `_assignment`, `_observation`, `_outbox` | Assignment + Agent op atomic, rolled back on Agent reject; 6 kernel workers move to the package; superseded by v4.2: the 53 bridged flux routes were removed and the other 32 are kernel-owned (F5d) |
| wireguard (T12) | 1 | peer lifecycle, generated config | package migration (names not fixed) | Needs anix-agent revival |
| protocol-runtime (T12) | 20 | protocol composition, runtime-adapter selection, Agent task/monitor | package migration (names not fixed) | Needs anix-agent revival |
| machine-telemetry, nftables-forward, gost-mesh, nat-egress (T12) | 5 / 0 / 3 / 0 | keep runtime semantics; add v2 entrypoint, generation, `RuntimeStatus` reports | — | Agent-target runtime packages |
| identity-platform | 26 | group A switches with the identity cutover | — | 23 native-flagged (section 3.4); the user's two invite routes moved to affiliate; the administrator's 3 invite code routes are bridged; the user's own subscription reset is group A |
| platform | 12 | system configuration, audit, backup | — | Moved from identity-platform after v4.0.0; bridged |
| affiliate | 10 | invite codes, commissions, withdrawals, invite statistics and configuration | — | Moved from identity-platform after v4.0.0; all native-flagged (section 3.4) |

## 7. Rollout Record Semantics (T5)

CURRENT: models exist in `internal/model/plugin_rollout.go` (registered in
`KernelModels()` in `internal/model/kernel.go`), with services
`BeginPackageMigration`, `MigratePackageHost`, `AdvancePackageCohort`, and
`RollbackPackageGeneration` in `internal/service/plugin_rollout.go`, tested in
`internal/tests/integration/plugin_rollout_test.go`. **Nothing in the running
server calls them.**

Semantics to keep when wiring:

- Tables `v4_kernel_package_migration_run`, `_validation_result`,
  `_route_generation`, `_backup_reference` (plus `_rollout_lock`).
- Stored per run: migration checksum, package version, before/after schema
  version, opaque checkpoint, validation digest, backup reference, previous
  generation, rollback reason.
- The package supplies the opaque checkpoint and validation digest; the kernel
  never computes a domain mapping or queries a package table.
- Cohorts only 1 → 5 → 25 → 100, one step at a time from a verified
  generation; each step creates a new immutable generation and retries are
  idempotent. Rollback restores the previous verified generation.
- Checkpoints are persisted after each host reply; route activation is refused
  until the host health lease matches the requested generation.
- For extraction, per-route modes (3.2) are the traffic switch; the ledger
  remains the package-version rollout record.

## 8. Worker And Static Gate Design (T14)

- CURRENT `config/scripts/check_plugin_only_workers.py` scans the non-test
  Go files of `cmd/server` for `service.New*Worker/Executor/Prober/...`
  constructors. The kernel's topology executor is allowed. Seven legacy
  domain workers, all started from `cmd/server/singleton_workers.go`, are
  listed: `NewPanelForwardRuntimeJobExecutor`,
  `NewForwardAgentBridgeWorker`, `NewForwardFlowResetWorker`,
  `NewNodeMonthlyResetWorker`, `NewForwardGostStatsWorker`,
  `NewForwardAnsibleStatsWorker` and `NewForwardLatencyProber`. A new one
  fails the gate. A listed one that is no longer started must be removed, so
  the list only shrinks.
- CURRENT `config/scripts/check_package_boundaries.sh` fails when
  `packages/...` or `pkg/...` code depends on `internal/`, even
  transitively. A test may import `internal/` only through a reasoned
  allowlist entry; today only the `sdk/packagebridgesdk` contract tests do.
- CURRENT `config/package-extraction.json` and `check_plugin_only_routes.py`:
  - `bridged` and `native-flagged` routes keep their legacy handler, in the
    router or the identity bridge.
  - A `native` route must bind the bare gateway and have no legacy handler
    left.
  - `native-flagged` and `native` routes need a package host in
    `packages/<id>/control` that names the route.
- All three gates run in the required Go Quality Gates job.
- Release gates must fail on: missing required package, direct legacy route
  registration, legacy domain worker startup, unsigned artifact, missing
  migration evidence, or unverified package version.
- Kernel end state keeps only token verification and authorization, package
  policy, the module PKI, audit, lifecycle, migration orchestration, and
  health. Token issuing, credentials and MFA belong to the identity module.
- Delete a domain's in-process routes/workers only after it is `native` and
  its reverse migration passed. Historical types may stay only to read old
  data during the rollback window; no live path may call them.

## 9. Platform Gaps To Fix First (CURRENT)

| Gap | Where |
|-----|-------|
| Package execution off in the production template | `plugins.control_execution_enabled: false` in `config/config.prod.yaml` |

Fixed in M0/M1 (2026-09-29): the route gates now run on every PR (PR #11), and
the kernel workers share a cancellable root context with ordered shutdown
(PR #13).

Fixed in M3 (2026-09-30): package migrations run through the ledger when a
storage package's host starts (`internal/service/package_host_migration.go`,
`startResolvedHost`).

## 10. Roadmap

Status (2026-09-29): step 0 (cleanup PRs, `go_dev` ruleset, this document),
M0 and M1 are done. M0/M1 landed as PRs #11-#18. The M2 rehearsal on a
restored production dump found five defects, all fixed by PRs #14, #15, #17,
#18 and #19; the results are in
[`release-line-status.md`](release-line-status.md#production-baseline-and-upgrade-rehearsal).
The M2 production cutover is next and needs a CI-built release first.

Status (2026-09-30): the M3 extraction infrastructure landed as PRs #32-#37:
- migration index digests and the capability grammar;
- `GetPackageConfig` and route modes;
- `pluginhostsdk.Router` with shadow comparison and `/metrics`;
- per-package PostgreSQL roles and `LeaseStorage`;
- ledger-run migrations on host start;
- the extraction map, boundary and worker gates, and the
  `internal/tests/packagecompat` harness.

Not yet repeated for M3: the Docker rehearsal of the v4.0.0 signed packages
against the new kernel.

Next (decision of 2026-09-30): the network module runtime and the identity
module (N1–N2 in [`module-runtime.md`](module-runtime.md) and
[`identity-service.md`](identity-service.md)). The knowledge pilot (M4) moves
after identity.

| Milestone | Weeks | Scope | Done when |
|-----------|-------|-------|-----------|
| M0 baseline + gates to PR | 1 | Production inventory into `release-line-status.md` (version, PG size, node count, largest node user count, `/s/:token` and UniProxy `config`/`user` size and p50/p99); run `check_plugin_only_routes.py` and `check_v2_package_route_catalog.py` on PRs; proto-generation drift check for `sdk/api/pluginhost`, `sdk/api/packagebridge` | Inventory recorded; gates required on PRs |
| M1 harden v4 | 1–3 | Route-resolution cache keyed by installation id, desired/observed version, `LifecycleGeneration`, enabled state, `ArtifactSHA256`, trust-root fingerprint (success-only, fail-closed); `recover` (`package_panic`); watchdog with backoff; stderr to kernel log with `[pkg:<id>]`; pass `TZ` + `time/tzdata`; configurable body caps; per-package/route metrics; benchmarks in `go-benchmark-smoke`; cancellable worker contexts | Route resolution ≥10x faster; host panic/kill auto-recovers; >1 MiB UniProxy response served |
| M2 production to v4 | 3–4 | Rehearse on a restored prod PG dump per [`../UPGRADE.md`](../UPGRADE.md) and [`../guide/v4-plugin-only-upgrade.md`](../guide/v4-plugin-only-upgrade.md); enable `control_execution_enabled`; install the 16 signed packages (all bridged); smoke admin/user UI, byte-compare `/s/:token`, UniProxy `config`/`user`/`push`/`alive`, payment callback (test mode), latency; cut over in a window; keep old host/DB N days | Production stable on v4, all routes bridged, error/latency baseline recorded; 72 h with error rate ≤ old version |
| M3 extraction infrastructure | 4–6 | `internal/packagestore`, `v4_kernel_package_storage`, `kapi_user_directory_v1`, `LeaseStorage`, `sdk/packagestoresdk`, CI job `package-storage-postgres`; wire the migration runner in `startResolvedHost` (compensated by the lifecycle plan); auto migration-index versions in `build_package.py`; `GetPackageConfig` + `sdk/pluginhostsdk/router.go`; `sdk/v2compat`; `internal/tests/packagecompat` (`RunRead`, `RunWrite`); `config/package-extraction.json` (`bridged`/`native-flagged`/`native` per route) with `check_plugin_only_routes.py` allowing direct handling only for `native`; `check_package_boundaries.sh`; `check_plugin_only_workers.py` | Listed packages' tests pass; PG grant tests pass |
| M4 pilot + first domains | 6–8 | knowledge, then ticket, then plan (after `kernel.entitlement.apply.v1` and `kapi_plan_catalog_v1`) | 72 h of zero shadow mismatches in production; one production rollback to `legacy` rehearsed; gate shows `native`; boundary check clean; legacy code deleted |

Pilot: **knowledge** (6 routes, ~240 lines in `internal/handler/knowledge.go`
and `admin_knowledge.go`). Capabilities `kernel.storage.v1` +
`kernel.storage.adopt:v2_knowledge`; migration `001` validates only (tables,
columns, row count, digest). Sequence: all `legacy` -> GET routes `shadow` for
3 days with zero mismatches -> all `native` for 7 days -> delete the legacy
handler and model, unwrap `router.go`, mark `native`. Byte-compat quirks to
reproduce exactly:

- User list returns `data: null` when empty (nil slice); admin list returns `[]`.
- Create coerces `show: 0` to `1` (a hidden article cannot be created) and
  defaults `category` to `公告`.
- Timestamps are mixed: list/detail `created_at`/`updated_at` are Unix
  seconds; the create response embeds the model (RFC3339 times); envelope
  `ts` is Unix milliseconds.
- Error messages are Chinese strings (`获取知识库失败`, `文章不存在`, ...).

Next quarter, in dependency order:

1. notification (24 routes): SMTP/Telegram sending moves into the host.
2. order + payment together (33 routes): keep one transaction, column-level
   grant on `v2_order(status, paid_at)`, replay payment-callback fixtures.
   A successful callback marks the payment and order paid and completes the
   order (owner decision, 2026-09-30, `subscriber-service.md`). Order (13 of
   13 routes) and payment (20 of 20) are in place (section 3.4); the
   callbacks complete orders through `KernelOrder` (`order-service.md`).
3. subscription + proxy-node (56 routes plus parser and gRPC): map
   `/s/:token` and UniProxy v1 through a kernel-side `Gateway.ServeRoute`.
4. forward (79 routes, ~12k lines of forward handler/service code, 6 workers).
5. wireguard / protocol-runtime: requires reviving anix-agent first.

Identity goes first, ahead of the knowledge pilot, as a network module; see
[`identity-service.md`](identity-service.md).

## 11. Risks And Rollback

| Risk | Mitigation |
|------|------------|
| Output not byte-identical (`ts`, `null` vs `[]`, time zone, validation text) | `sdk/v2compat`, pass `TZ`, copy structs verbatim, per-route compare tests, `shadow` before `native` |
| Host crash takes a domain down | `recover` + watchdog (same-generation restart with 1–30 s backoff, failed after 5 restarts in 5 min; `internal/pluginhost/supervision.go`); switch the mode back to `legacy` at any time before legacy code is deleted |
| PG grant mistakes (sequence privileges, PG15+ `public` defaults) | PG CI job, idempotent creation, `plugins.storage_isolation: shared` escape hatch |
| Connection count grows | 2–4 connections per lease; only `native` packages lease |
| Money paths | Last in order, single transaction, no silent behaviour change |
| Production cutover (M2) fails | Switch DNS/ApiHost back to the retained old instance; reconcile writes made in the window by hand |

Through M4 every step is reversible: tables are adopted in place, never moved
or copied.

## Sources

Retired plans, readable with `git show 57c62541:<path>`:

- `docs/superpowers/plans/2026-07-18-v4-plugin-only-stable.md` (Global
  Constraints; T5 rollout records; T7–T12 domains; T14 gates)
- `docs/superpowers/plans/2026-07-19-v2-package-bridge.md` (security
  invariants, adapter role, identity waves, non-goals)
- `docs/superpowers/plans/2026-07-19-v2-full-package-cutover.md` (Global
  Constraints; Tasks 6–9 waves and final enforcement)
- `docs/superpowers/plans/2026-07-20-v4-release-root-rotation.md` (now
  [`../guide/release-root-rotation.md`](../guide/release-root-rotation.md))

Related: [`release-line-status.md`](release-line-status.md), [`plugin-kernel-contract.md`](plugin-kernel-contract.md).
