# Package Extraction Design

Date: 2026-09-29

This is the design of record for moving business domains out of the Control
kernel and into signed packages. It replaces the retired plans that lived under
`docs/superpowers/` (see [Sources](#sources)); what is still binding from them
is copied here. Feature status stays in [`../features.md`](../features.md) and
open work in [`../../TODO.md`](../../TODO.md).

> 中文摘要：v4.0.0 的「插件化」只到路由层；本文定义「一个领域真正住在插件里」
> 的验收标准、目标机制（存储租约 + 按路由模式 + 类型化内核操作，均为**计划中**）、
> 保留下来的旧计划约束，以及 M0–M4 里程碑。

Markers used below: **CURRENT** = true in the tree today; **PLANNED** = accepted
design, not implemented yet; **HISTORICAL** = preserved from a retired plan for
context, not a commitment.

## 1. Status And Reality (CURRENT)

v4.0.0 (published 2026-07-20) is plugin-only at the routing level only.

- All 292 `/api/v2` routes in `config/v2-package-route-catalog.json` enter the
  package gateway. 244 are registered with `registeredPackageRoute`
  (`internal/router/router.go`), which registers the legacy gin handler into
  `packagebridge.DefaultRouteRegistry()` and returns the gateway; the 45
  `identity-platform` routes use the bare `v2PackageGateway.Serve`; the 3
  WebSocket routes use `registeredPackageWebSocketRoute`.
- `config/package-extraction.json` records each route's extraction mode
  (`bridged`, `native-flagged` or `native`) and where its legacy handler lives
  (`router`, `identity-bridge` or `none`). 140 routes are `native-flagged`:
  identity-platform (20: group A's 15, the profile, dashboard and user detail,
  and the traffic and subscription resets), affiliate (7), forward (17),
  knowledge (6), notification (19), order (9), payment (16), plan (7),
  platform (4), protocol-runtime (3), proxy-node (7), subscription (17) and
  ticket (8). The rest are `bridged`. The identity routes are
  `identity-bridge`. `check_plugin_only_routes.py` enforces the map against
  the router and the identity bridge.
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
- **After v4.0.0.** 21 of the 45 identity-platform routes moved to other
  packages, still bridged:
  - system configuration, audit and backup (12) to the new `platform`
    package;
  - commissions, withdrawals and invite statistics and configuration (8) to
    the new `affiliate` package;
  - `/user/reset` to `forward`.

  They are now `registeredPackageRoute` routes. identity-platform keeps 24,
  all still served through the identity bridge. `internal/compat/v2/moved_routes.go`
  lists the moves: the kernel accepts the old route ids from old
  identity-platform releases and, while both packages declare a route,
  prefers the new owner. There are now 18 packages. Ten have their own host
  (affiliate, identity-platform, knowledge, notification, order, payment,
  plan, platform, proxy-node and ticket); the other eight run the generic
  host.
- The 4.0 stage exit condition "production requests no longer reach coupled
  legacy handlers" was **not** met. Business tables (`v2_*`), handlers,
  services, and workers are still kernel-owned.
- Production (per the operator, 2026-09) still runs a pre-v4 Control on
  PostgreSQL. Moving it to v4 is a database migration plus host move (M2).
- Outside the package gate, kernel handlers serve `/api/v1/server/UniProxy/*`,
  `/{subscribe_path}/:token`, `/api/v1/client/subscribe`, and `/flow/upload`.

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

- Each route has a mode `legacy` | `shadow` | `native`, stored in the package's
  existing versioned config document and polled by the host every 5 s through a
  new bridge RPC `GetPackageConfig`.
- `legacy`: pass through the bridge to the legacy handler (today's behaviour).
- `shadow` (GET only): return the legacy result, run the native implementation
  in the background, compare normalized output, count mismatches in
  `HealthResponse.details_json`.
- `native`: the host answers; the legacy handler is not called.
- Rollback = set the route back to `legacy` (effective within 5 s, audited
  through the config revision history).
- Router, mode logic, legacy pass-through, and shadow comparison move into
  `sdk/pluginhostsdk/router.go` (from `packages/shared/controlhost`).
- `sdk/v2compat` exposes `PanelSuccess`/`PanelError`/`NormalizeForCompare`;
  legacy handlers delegate to it so output stays byte-identical.

Status (2026-09-30): implemented. `GetPackageConfig` and the reserved
`routes` configuration key, `sdk/pluginhostsdk.Router` and `sdk/v2compat` are
in place, and route modes and shadow counters are exported on `/metrics`
(see [`plugin-kernel-contract.md`](plugin-kernel-contract.md#package-configuration)).
No package has a native route yet.

### 3.3 Typed kernel operations

Cross-domain writes go through typed, idempotent kernel operations instead of
foreign-table writes. First one: `kernel.entitlement.apply.v1`, covering plan
assignment and order fulfilment, executed exactly once via an idempotency
table and proven equivalent to `PlanService.AssignToUser` and steps 4–5 of
`OrderService.Complete`.

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
- **Notification (in place).** 19 of 24 routes on the adopted
  `v2_notification_template`, `v2_notification_log`, `v2_telegram_bot` and
  `v2_telegram_user` tables, proved by
  `internal/tests/notificationcompat`; binding e-mails come from
  `kapi_user_directory_v1`. Five stay bridged: the e-mail configuration and
  test send (settings in `v2_system_config`), setting the webhook (needs the
  request host) and the public webhook (`/sub` needs the subscription token).
- **Platform (in place).** 4 of 12 routes: the backup configuration, list
  and statistics on the adopted `v2_backup_config` and `v2_backup_record`
  tables, and the system audit log through `kapi_system_audit_log_v1`, proved
  by `internal/tests/platformcompat`. Eight stay bridged:
  - the system configuration routes: `v2_system_config` is a protected
    kernel table, its values include secrets, and its writes record audit
    entries in the protected `v2_operation_log`;
  - updating the backup configuration: it records an audit entry and
    refreshes the copy the kernel's backup service keeps in memory, which
    backup creation reads;
  - creating, deleting and restoring backups: archives of the database and
    files on the kernel's disk.
- **Plan (in place).** The first module that changes shared subscriber
  state. 7 of 12 routes run on the adopted `v2_plan` and `v2_event` tables,
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
  - The five `/speed-limit/*` routes stay bridged. They are Flux forward
    limits: their rows name forward tunnels, they read
    `v2_forward_tunnel` and `v2_forward_user_tunnel`, and an update
    re-pushes forwards to nodes. They join forward later.
  - The subscription module checks a plan through `kapi_plan_catalog_v1`;
    the kernel's subscription renderer, which stays in the kernel, still
    reads `v2_plan` directly.
- **Order (in place).** 9 of 13 routes run on the adopted `v2_order` and
  `v2_coupon` tables, proved by `internal/tests/ordercompat`: the coupon
  routes, order statistics, status changes, cancellation, "mark paid" and
  the user's order creation.
  - Other domains are read through views: `kapi_plan_catalog_v1` (a plan's
    prices, group, transfer and limits), `kapi_plan_subscription_group_v1`
    (the subscription groups a plan grants) and `kapi_user_directory_v1`
    (the buyer's current plan, which makes an order new, a renewal or an
    upgrade).
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
  - The kernel keeps writing `v2_order`: the payment callbacks mark orders
    paid and complete them, and the dashboard and invite statistics read it.
  - The four order list and detail routes stay bridged. Their answers embed
    the buyer's whole `v2_user` row, subscription token and proxy UUID
    included, which no kernel view may expose.
- **Payment (in place).** 16 of 20 routes run on the adopted
  `v2_payment_gateway`, `v2_payment_record` and `v2_payment` tables, proved
  by `internal/tests/paymentcompat`: gateway administration, payment records
  and statistics, the user's channels, payments and their status, the method
  list, and the x402 and fiat payment creation (stubs that call no provider,
  on both sides).
  - Payment owns the gateways and so holds their secrets (merchant keys,
    webhook secrets) through the adopted `v2_payment_gateway`. Its
    administrator answers show them as `********`, as the kernel's do.
  - An order is read only through `kapi_order_billing_v1` (its buyer, total
    and status): a payment is created for the caller's own pending order and
    its exact total.
  - The four callback routes (`/payment/callback/:type`, the x402 callback,
    the Stripe and PayPal webhooks) stay bridged. A paid callback updates the
    payment record and the gateway statistics, marks the order paid and
    completes it, in one kernel transaction. The order is the order module's
    table, and there is no contract for those writes yet. The provider
    signature checks stay with them, and the PayPal webhook calls PayPal's
    API to verify each delivery.
- **Affiliate (in place).** 7 of 8 routes run on the adopted
  `v2_commission_record`, `v2_commission_withdraw` and `v2_invite_config`
  tables, proved by `internal/tests/affiliatecompat`: the user's commissions,
  withdrawals and withdrawal request, and the administrator's withdrawal
  list and decisions, invite statistics and configuration.
  - Other domains are read through views: `kapi_user_referral_v1` (who
    invited whom) for the statistics, `kapi_subscriber_entitlement_v1` for
    the caller's commission balance, and `kapi_affiliate_settings_v1` for
    the frontend settings (code prefix and length, withdrawal fee and
    methods).
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
  - Updating the configuration stays bridged. It writes
    `invite.frontend.config` into `v2_system_config`, which no package may
    adopt and no contract writes.
- **Identity leftovers (in place).** 5 of identity-platform's 9 routes
  outside group A run natively, proved by `internal/tests/identitycompat`
  (byte parity and the same Control state, on SQLite and PostgreSQL).
  identity-platform adopts no kernel table.
  - **Account reads:** the user's profile and dashboard and the
    administrator's user detail.
    - The account (email, administrator, staff and ban flags) comes from
      identity's own store. Identity's store is current only while identity
      is authoritative, so the kernel lets these three routes leave legacy
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
      30 seconds.
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
  - **Stay bridged:**
    - the administrator's user list: it filters, orders and pages one
      query over identity's ban flag and email and the subscriber's expiry
      and plan, and shows every listed user's subscription token and proxy
      uuid, which no contract lists;
    - the user statistics: "active" means not banned (identity) and not
      expired (subscriber), one predicate over both stores;
    - the user's invite codes and their generation: affiliate data, not
      identity's. Control keeps the codes (`v2_invite_code`, which
      registration consumes inside Control), and the answer adds the
      commission balance and invite statistics that join the order and
      affiliate packages' tables. They belong with the affiliate package.
- **Subscription (in place).** 17 of 25 routes run on the adopted
  `v2_subscription_group`, `v2_subscription_template`,
  `v2_plan_subscription_group` and `v2_subscription_group_node_protocols`
  tables, proved by `internal/tests/subscriptioncompat`: groups and templates
  (list, read, create, update; templates also delete), a group's node
  protocol links, a plan's groups, a user's groups, the statistics and the
  two static lists. The subscription link endpoints (`/s/:token`,
  `/api/v1/client/subscribe`) are kernel routes outside the package gate and
  stay in the kernel with the renderer.
  - Other domains are read through views: `kapi_plan_catalog_v1` (whether a
    plan exists), `kapi_subscriber_entitlement_v1` (whether a user exists,
    and their traffic), and the new `kapi_user_subscription_group_v1` (the
    groups a subscriber holds, until when), `kapi_node_protocol_v1` (a
    protocol's node) and `kapi_node_heartbeat_v1` (a node's last report). No
    view shows a token, UUID, e-mail address, key or protocol settings.
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
  - Eight routes stay bridged:
    - deleting a group, and granting or taking away a user's group: they
      write `v2_user_subscription_group`, subscriber state only the kernel
      writes (no package may adopt a `v2_user*` table), and
      `KernelSubscriber` has no call that edits one membership. An extension
      is proposed in [`subscriber-service.md`](subscriber-service.md);
    - a group's protocols and the protocol pool: they answer whole
      `v2_node_protocol` and `v2_node` rows, Reality private keys and custom
      configuration included, which no view may carry;
    - the preview: the kernel's renderer, with the user's token and UUID,
      the nodes, and the WireGuard peers it creates;
    - the subscription link settings: `app.subscribe_path` is process
      configuration and `app.subscribe_domains` lives in the protected
      `v2_system_config`;
    - the user's subscription summary: served from the kernel's 30-second
      cache, with its `cached_at`, which a native answer cannot share.
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
  - **Stay bridged** (24, with the reason in the host's route map):
    - node list, detail, creation, update, deletion, credentials and raw
      configuration: they read or write the node credentials, the list and
      detail embed each node's protocols with their Reality private keys,
      the raw configuration carries WireGuard private keys, an update
      clears the kernel's in-memory node cache, and a deletion removes the
      node's protocols and WireGuard peers in one transaction;
    - configuration validation: no table, but the kernel's WireGuard
      protocol validator, which the protocol routes share;
    - the authorization keys: the list answers each key in clear, and the
      kernel's HTTP and gRPC registration read them;
    - registration, heartbeat and runtime health: authenticated or minted
      node credentials, and writes to `v2_node`;
    - the agent WebSocket, a live connection the kernel holds and pushes
      to, and UniProxy: node-authenticated, with every eligible
      subscriber's UUID in the user list, subscriber traffic in a push and
      the online list in the kernel's in-memory cache;
    - the load balancer statistics and health check: they read the forward
      package's `v2_forward_node`, and the check probes each forward node
      and writes its status.
- **Forward (in place).** 17 of 80 routes run on the adopted `v2_forward`,
  `v2_forward_tunnel`, `v2_forward_user_tunnel`, `v2_speed_limit`,
  `v2_forward_rule` and `v2_forward_latency_bucket` tables, proved by
  `internal/tests/forwardcompat`:
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
    derives too; type 2 a permission's traffic.

  None of them changes what a node runs: a tunnel or a permission alone runs
  nothing, the display order is not part of a forward's runtime payload, and
  the kernel's forward runtime re-reads these rows on every use.
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
  - **Stay bridged (63):**
    - forward nodes and Ansible machines (`v2_forward_node`; the answers
      show tokens to administrators, the kernel's gost manager caches each
      node's address and token, the checks and statistics sync reach the
      node over the network);
    - every forward change, pause, resume, deletion and diagnosis, the
      backend sync, the tunnel update and diagnosis, and permission removal
      and updates: they apply forwards on their nodes (NodeX, a local
      Ansible job or a clean agent job) or dial from Control;
    - the legacy rules other than the user's list (pushed to NodeX with the
      nodes' tokens; the administrator's answers embed the nodes' tokens)
      and the agents' rule list (node token authentication);
    - runtime status, diagnosis and jobs (the protected NodeX and Ansible
      settings, NodeX calls, Control's disk, job payloads with tokens);
    - the observability targets, trend and topology, which read the proxy
      nodes of `v2_node`;
    - clean agents and their registration, heartbeat, report and install
      script (agent tokens, job claims, the request's host);
    - flow upload, report and snapshot: the forward's counters, the
      subscriber's traffic and the permission's traffic change in one kernel
      transaction under a per-forward lock in Control's memory, and
      exhaustion pauses forwards on their nodes.
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
  - **Stay bridged** (17, with the reason in the host's route map):
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
      WebSocket): node credentials checked in the kernel, node status in
      `v2_node` and `v2_forward_node`, connections in the kernel's memory,
      and the forward package's bridge tasks and runtime jobs.
- The kernel publishes read-only views `kapi_*`, created at startup by
  `EnsureKernelAPIViews` (first `kapi_user_directory_v1`, then
  `kapi_system_audit_log_v1`, the `v2_operation_log` rows of module
  `system`; later `kapi_plan_catalog_v1`). Packages read other domains only
  through `kapi_*` views or typed operations.
  `EnsureKernelAPIViews` (`kapi_user_directory_v1`,
  `kapi_subscriber_entitlement_v1`, `kapi_plan_catalog_v1`,
  `kapi_plan_subscription_group_v1`, `kapi_order_billing_v1`,
  `kapi_user_referral_v1`, `kapi_affiliate_settings_v1`,
  `kapi_user_subscription_group_v1`, `kapi_node_protocol_v1`,
  `kapi_node_heartbeat_v1`). Packages
  `kapi_node_status_v1`). Packages
  `kapi_forward_node_v1`, `kapi_forward_runtime_settings_v1`). Packages
  read other domains only
  through `kapi_*` views or typed operations. A view whose source table does
  not exist is left out. A view that filters rows is a PostgreSQL
  `security_barrier` view.

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
  submodules either way.

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
| plan (T8) | 12 | `plan.catalog.list`, `plan.assignment.read`, `plan.entitlement.read` | `v4_plan_catalog`, `_assignment`, `_quota` | Publishes versioned `EntitlementReader`; 5 `/speed-limit/*` routes are Flux forward limits, stay bridged, later join forward |
| order (T9) | 13 | `order.create`, `order.apply_coupon`, `order.transition`, `order.request_entitlement` | `v4_order_order`, `_promotion`, `_coupon_redemption` | Owns coupons; entitlement requests are durable messages |
| payment (T9) | 20 | `payment.initiate`, `payment.callback`, `payment.reconcile` | `v4_payment_record`, `_callback`, `_outbox` | Receipt stored before transition; exactly-once per gateway event; one-time secret lease |
| subscription (T10) | 25 | `subscription.render`, `subscription.usage.read` | `v4_subscription_group`, `_template`, `_usage` | Byte-identical output incl. content type and cache headers |
| proxy-node (T10) | 31 | `proxy-node.register`, `.config.deliver`, `.user.deliver`, `.traffic.report` | `v4_proxy_node`, `_config`, `_user`, `_usage` | Delivery becomes a versioned Agent operation; kernel gRPC transport-only |
| forward (T11) | 80 | `forward.rule.create/update`, `forward.tunnel.assign`, `forward.observation.read`, `forward.agent.apply` | `v4_forward_rule`, `_tunnel`, `_assignment`, `_observation`, `_outbox` | Assignment + Agent op atomic, rolled back on Agent reject; 6 kernel workers move to the package; 17 native-flagged (section 3.4), the rest bridged |
| wireguard (T12) | 1 | peer lifecycle, generated config | package migration (names not fixed) | Needs anix-agent revival |
| protocol-runtime (T12) | 20 | protocol composition, runtime-adapter selection, Agent task/monitor | package migration (names not fixed) | Needs anix-agent revival |
| machine-telemetry, nftables-forward, gost-mesh, nat-egress (T12) | 5 / 0 / 3 / 0 | keep runtime semantics; add v2 entrypoint, generation, `RuntimeStatus` reports | — | Agent-target runtime packages |
| identity-platform | 24 | stays bridged until the identity cutover | — | Only migrations move to the lease |
| platform | 12 | system configuration, audit, backup | — | Moved from identity-platform after v4.0.0; bridged |
| affiliate | 8 | commissions, withdrawals, invite statistics and configuration | — | Moved from identity-platform after v4.0.0; 7 native-flagged (section 3.4), the configuration update bridged |

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
   order (owner decision, 2026-09-30, `subscriber-service.md`). Order (9 of
   13 routes) and payment (16 of 20) are in place (section 3.4); the
   callbacks, which write the payment and the order in one transaction, are
   not.
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
