# Plugin Kernel Contract

The `/api/v3` control-kernel surface is additive. Existing `/api/v2` routes,
legacy tables, UniProxy synchronization, and forwarding runtime workers remain
unchanged.

## Current Boundary

The kernel persists signed plugin releases, installation intent, scoped access
groups, verified immutable plugin artifacts, node assignments, topology
revisions, deployments, and idempotent operations. Control-target lifecycle
operations may be executed by the opt-in in-process Control executor; Agent
target operations are dispatched only when the Agent bridge flag is enabled.
The kernel still does not switch forwarding traffic. A deployment created
through `/api/v3/deployments` stays inert until an operator calls apply and the
feature-gated topology executor is running. `plugins.topology_execution_enabled`
defaults to false and startup refuses it unless `plugins.dispatch_enabled` is
also true.

## Trust Root

Set `plugins.official_public_key` to the Base64-encoded Ed25519 public key for
the AnixOps release signer. `POST /api/v3/plugin-releases` rejects requests
until this key is configured. The manifest signature covers the canonical
`PluginManifest` JSON representation; configuration schema JSON is normalized
before signing and verification.

Each admitted release stores the signing trust-root fingerprint and key ID;
later catalog, configuration, and v2 route reads re-verify a release with the
trust root bound to it. Only one root is active: at startup Control activates
the configured `plugins.official_public_key` and retires every other recorded
root (`service.EnsurePluginTrustRoot`), so releases signed by a retired root
stop verifying until they are re-signed and imported as a new version.
Changing the configured key is therefore a root rotation; follow
[`../guide/release-root-rotation.md`](../guide/release-root-rotation.md).

Only official `AnixOps` manifests are accepted. A plugin installation or
operation must reference a registered release. Disabling an installation marks
it disabled and preserves its database state; no purge endpoint is exposed.

## Artifact Repository

`POST /api/v3/plugin-releases/:id/artifact` stores the release artifact after
checking its SHA-256 against the signed manifest. Stored artifacts are
content-addressed by `plugins/<plugin>/<version>/<sha256>.artifact` metadata
and immutable for a release. Re-uploading the same bytes is idempotent; changing
the bytes for an existing release is rejected.

If the signed manifest declares `webui`, the uploaded artifact must be a
package archive containing the declared `webui/*.js` or `webui/*.mjs` bundle
path. The kernel extracts that one file from zip, tar, or tar.gz packages,
verifies the bundle SHA-256 from the signed manifest, and stores it as an
immutable same-origin asset. The extension catalog exposes only the verified
content-addressed URL:
`/api/v3/extensions/<plugin>/<version>/webui/<sha256>/<filename>`. Browser
runtime code fetches that same-origin asset, computes SHA-256 again, and only
then imports the module. The production runtime rejects a catalog entry without
that verified URL; a compiled local-module loader exists only as an explicit
test harness option and is never a package WebUI fallback.

Extension routes and menus carry the signed permission declared by the package.
The admin frontend filters menus and blocks extension route navigation unless
the current admin profile grants that permission. Existing admin profiles with
no explicit permission list remain legacy-compatible as super-admins during the
3.x transition.

### WebUI CSS Classes

A package WebUI bundle renders into the admin console and is styled by
Control's global stylesheet (`web/src/style.css`), not by CSS of its own.
These classes are a **stable contract** for plugin WebUIs:

| Class | What it styles |
|---|---|
| `.btn` | a button (pill, 36 px; 44 px minimum on touch screens) |
| `.btn-secondary` | the secondary button look, with `.btn` |
| `.table-container` | the scrolling, rounded frame around a table |
| `.data-table` | a table: cell padding, header row, separators, row hover |
| `.empty-state` | the muted, centred "nothing here" text (a table cell) |

Plugins ship and are signed separately from Control, so an installed plugin
keeps rendering these classes across Control upgrades: Control must not
remove or rename them, nor change what they mean (it may restyle them with
the design tokens). A change that has to break one is a WebUI contract
change (`anixops.webui/v2`), not a Control refactor.
`web/src/__tests__/pluginWebuiClasses.test.js` fails when one of them loses
its rule in `style.css`, and checks that the bundled packages use them.
Other class names a bundle uses (`page-header`, `stats-grid`, ...) are
the plugin's own: Control gives them no style and promises nothing about
them.

Backend package APIs are admitted only through the kernel gateway namespace
`/api/v3/plugins/<plugin-id>/...`. A signed manifest may register exact routes
or `/*` suffix wildcard prefixes in `control_routes`, and any plugin declaring
backend routes must also declare `<plugin-id>.api`. The gateway verifies the
official release, enabled version-bound Control installation, local artifact,
registered route, and `plugin_api` resource grants. If no explicit
`plugin_api` grant exists for that plugin, legacy admins remain compatible
during the 3.x transition; once such a grant exists, group membership is
required. With `plugins.control_execution_enabled=true`, the gateway dispatches
to the version-exact local Control executor after admission. The first
reference executor is the read-only `machine-telemetry` status route. With the
flag off, or when no executor is registered, the gateway remains fail-closed
and returns `501 plugin_route_not_implemented`; this preserves the legacy
production path.

An enabled installation requires both a signed release and a verified artifact.
This creates a reproducible package-manager fact before the dispatcher or Agent
Supervisor can act on desired state. The current repository stores artifact
bytes in the kernel database so SQLite and PostgreSQL migrations remain
self-contained; extracting those bytes to a filesystem/object-store backend can
be added behind the same metadata contract later.

## Manifest Contract

Control and Agent share the canonical `PluginManifest` byte contract. The
fixture at `contracts/plugin/v1/manifest-golden.json` is byte-identical in both
repositories and includes the first `machine-telemetry` Control + Agent + WebUI
package descriptor. The signed byte stream includes `webui` metadata, so Agent
must decode the same WebUI structures as Control instead of treating that field
as client-only metadata.

Global manifest validation now rejects unsupported API versions, unsafe
package IDs or versions, duplicate targets, malformed architecture tokens,
self/duplicate/overlapping dependencies and conflicts, unsafe entrypoints,
non-object configuration schemas, invalid frontend bundle digests, negative
migration versions, permissions outside the plugin namespace, and backend
control routes outside `/api/v3/plugins/<plugin-id>/...`. Agent still performs
the additional runtime architecture match before installing an artifact on a
specific node.

The shared negative fixture suite at
`contracts/plugin/v1/manifest-negative.json` is also byte-identical in Control
and Agent. It covers strict unknown-field rejection plus unsafe versions,
duplicate targets, architecture errors, dependency/conflict errors, unsafe
entrypoints, invalid frontend digests, negative migrations, non-object config
schemas, backend control-route violations, global permission namespace
violations, and WebUI namespace/bundle/permission violations. Both repositories
must reject every case before a manifest can be admitted or installed.

Control additionally checks the `capabilities` grammar (Control-only, so the
shared negative fixture is unchanged). Names are lowercase and unique.
Capabilities in the `kernel.` namespace grant kernel authority, so only these
forms are accepted:

| Capability | Grants |
|------------|--------|
| `kernel.observed-state` | Agent observed-state reports |
| `kernel.identity.v1` | the `KernelIdentity` contract: subscribers, the account projection, revocations; honoured only for official AnixOps packages, checked on every call |
| `kernel.order.complete.v1` | the `KernelOrder` contract (`order-service.md`): a paid payment record marks its order paid and completes it, after the kernel re-checks that it pays the order; honoured only for official AnixOps packages, checked on every call |
| `kernel.subscriber.entitlements.v1`, `.traffic.v1`, `.credentials.v1`, `.balance.v1`, `.directory.v1`, `.groups.v1`, `.summary.v1` | one method family each of the `KernelSubscriber` contract (`subscriber-service.md`): plan activation and entitlement edits, traffic ledger, credential resets, balances, directory and change feed, subscription group membership, the subscription summary the kernel caches (`kernel-caches.md`); honoured only for official AnixOps packages, checked on every call |
| `kernel.telemetry.dashboard.v1` | the `KernelTelemetry` contract (`kernel-caches.md`): the administrator dashboard's snapshot from the kernel's cache, with the online users as a count; honoured only for official AnixOps packages, checked on every call |
| `kernel.nodeops.forward.v1`, `.nodeconfig.v1`, `.diagnose.v1`, `.agents.v1`, `.credentials.v1` | the `KernelNodeOps` contract (`node-ops-service.md`) for one operation family: submit that family's node operations; any one of the five reads, watches and cancels the package's own operations; honoured only for official AnixOps packages, checked on every call against the host's generation. No operation kind executes yet: a submission answers `UNIMPLEMENTED` |
| `kernel.settings.<namespace>.read.v1`, `.write.v1`, `.secrets.v1` | the `KernelSettings` contract (`settings-service.md`) for one settings namespace (`mail`, `invite`, `nodex`, `forward-runtime`, `backup`): read, write (with the kernel's audit entries and in-memory refresh), and secret values in clear (requires `read`); unknown namespaces are refused; honoured only for official AnixOps packages, checked on every call |
| `kernel.storage.v1` | a per-package database role and schema (storage lease) |
| `kernel.storage.adopt:<table>` | read/write on an existing table, adopted in place; requires `kernel.storage.v1`; kernel and identity tables (`v2_user*`, `v2_system_config`, `v2_audit_log`, `v2_operation_log`, `v3_kernel_*`, `v4_kernel_*`, `identity_*`, `kapi_*`) the node and forward agent credential tables (`v2_node`, `v2_authorized_key`, `v2_forward_node`, `v2_forward_clean_agent`, `v2_forward_runtime_job`) and the node runtime secret tables (`v2_node_protocol`, `v2_wireguard_peer`) cannot be adopted |
| `kernel.view:kapi_<name>_v<N>` | read access to a kernel API view; requires `kernel.storage.v1` |

The packaged migration index (`migrations/index.json`, format
`anixops.migrations/v1`) names the package and the release version and lists
ordered steps `{id, path, sha256}`. `build_package.py` writes the release
version (source indexes use the `__ANIXOPS_PACKAGE_VERSION__` token) and the
SHA-256 of every step script. When the kernel materializes a Control artifact
it verifies the index digest, the package and version, every step id
(`[0-9a-z][0-9a-z_]*`) and path (under `migrations/`), and every step digest,
and exposes the verified steps on the artifact reference. v4.0.0 indexes carry
no step digests; the kernel computes them from the packaged scripts.

When a Control installation is enabled, updated, or rolled back, the kernel
compiles the same-target dependency closure into a durable lifecycle plan.
Dependencies execute first, the root operation remains the user-visible
operation, and failed, timed-out, cancelled, or superseded apply work stops
later expansion. Changed steps are compensated in reverse order by restoring
the previous installation snapshot or disabling newly introduced packages.
Disabling an installed package is rejected while another enabled package on
the same target depends on it. Target-level transaction locks serialize these
checks across concurrent Control workers. These checks and plans only create
package-manager intent; the dispatcher flags remain off by default.

## Package Configuration

The kernel owns one revisioned configuration document per plugin installation.
`GET` and `PUT /api/v3/plugin-installations/:id/config` expose that document to
official package WebUI code. Writes are canonicalized, SHA-256 hashed, checked
against the currently signed release's JSON Schema, and use an optional
optimistic `expected_revision`; configuration is never written directly by a
plugin process. A configuration document is package-level intent, not an
implicit command to every node. Node assignment expansion and per-node
`plugin.configure` operations remain an explicit dispatcher step.

The top-level key `routes` is reserved by the kernel for Control packages. It
maps v2 route ids to a route mode:

| Mode | Meaning |
|------|---------|
| `legacy` (default, also when absent) | the host passes the request through the bridge to the legacy handler |
| `shadow` | GET only: the legacy result is returned, the host runs its native implementation in the background and counts mismatches |
| `native` | the host answers with its native implementation |

A route's runtime mode is not its extraction mode, which
`config/package-extraction.json` records per route
([`package-extraction.md`](package-extraction.md#32-per-route-modes)):
`native-flagged` and `native` routes have a native handler; `bridged` routes
are relayed until a kernel contract lets the package serve them, and
`kernel-owned` routes are relayed by design. A relayed route stays `legacy`
whatever its runtime mode says.

On `PUT`, the kernel validates `routes` against the verified compatibility
routes of the installation's release: unknown routes, other modes, `shadow`
on non-GET routes, and non-legacy WebSocket routes are rejected. It then
removes the key before applying the package's own `config_schema`, so package
schemas need not declare it. A Control `plugin.configure` operation completes
immediately, because hosts pull their configuration.

Operators switch route modes with the route-mode administration rather than
editing `routes` by hand: the admin API under `/api/v4/kernel/route-modes`,
the CLI `anix-control routes` and the admin page
(`internal/service/package_route_mode_admin.go`). It writes through the same
configuration path, so every rule above holds, and adds its own: `native` only
for `native-flagged` routes, no switch of `kernel-owned` routes or of identity
group A (identity cutover only), super administrators only, a confirmation and
a reason for `native`. Each switch is audited and recorded per route in
`v4_kernel_route_mode_revision`.

Hosts read their configuration with the package bridge RPC
`GetPackageConfig`, which returns the revision, the configuration hash and the
non-legacy route modes. Unlike `Invoke` and `OpenWebSocket`, session-scoped
RPCs carry no per-request capability. The caller is authorized by the session
identity, i.e. the package id, version and lifecycle generation that the
kernel bound to the inherited socketpair when it started the host. Every call
is fenced against the installation: a disabled package, another desired
version, or another lifecycle generation gets `PermissionDenied`. A kernel
without session operations answers `Unimplemented`, and hosts then keep every
route in `legacy` mode.

`sdk/pluginhostsdk.Router` implements this for package hosts. It polls
`GetPackageConfig` every 5 s and keeps the last successful modes when a poll
fails. A route without a native handler stays `legacy` and is reported as
`mode_unsupported`. In `shadow` mode the legacy response is returned first and
the native handler runs in the background with a timeout and a concurrency
limit; status codes and bodies are compared after
`sdk/v2compat.NormalizeForCompare` drops the envelope `ts`. WebSocket routes
always relay through the bridge. The router reports its configuration status
and per-route counters in `HealthResponse.details_json`. The kernel's
supervisor calls `Health` on every running host every 30 s, keeps the last
details document, and `/metrics` exports it as `anixops_package_config_status`,
`anixops_package_route_mode` and the `anixops_package_native_*` and
`anixops_package_shadow_*` counters.

The details document also carries `shadow_samples` (added in 4.1.0; a
kernel that does not read it ignores it): the router's latest 32 shadow
mismatches of the last 10 minutes, built and sanitized by
`sdk/shadowsample` before they leave the host (route, method, sanitized
path, both status codes, request id, observed time and a structural diff of
at most 64 paths and 8 KB with masked values; never the request body). The
kernel's collector (`internal/shadowsamples`) reads them after each health
poll, keeps only samples of routes `config/package-extraction.json` assigns
to the reporting package, sanitizes them again, replaces the path with the
route's template, and stores each sample once in
`v4_kernel_shadow_mismatch_sample` (7 days, 100 per route). A host has no
other write path to that table. The generic Control host runs on the
router with no native routes; the native routes of the package hosts are
listed in `package-extraction.md` and `identity-service.md`.

## Package Storage

A package host never receives kernel credentials: not the kernel DSN, the
JWT signing key or the Control configuration. A package that declares
`kernel.storage.v1` may lease its own least-privilege storage with the
session-scoped bridge RPC `LeaseStorage`, which is authorized and fenced like
`GetPackageConfig`. The kernel reads the grants from the signed manifest of
the host's release, verified again on every lease.

On PostgreSQL (`internal/packagestore`):

- **Role.** Each package gets a login role `anix_pkg_<id>` (`-` becomes `_`)
  with `NOINHERIT`, no other attributes, and `CONNECTION LIMIT 4`. It is a
  member of the `NOLOGIN` group `anix_packages`, which holds no privileges and
  exists so `pg_hba.conf` can admit every package role as `+anix_packages`. A
  pre-existing role with elevated attributes is refused.
- **Schema.** The kernel creates and owns schema `pkg_<id>`. The package role
  may create tables there, and its `search_path` is its schema, then the
  kernel's.
- **Grants.** Each lease replaces every privilege the role holds in the
  kernel schema with exactly the declared ones:
  - `SELECT, INSERT, UPDATE, DELETE` on adopted tables, and `USAGE, SELECT` on
    their sequences;
  - `SELECT` on kernel API views.

  A dropped capability is revoked on the next lease, including for
  connections that are already open.
- **Password.** Each lease sets a new random password from a SCRAM-SHA-256
  verifier computed in the kernel, so neither the server nor a statement log
  sees the password. Connections of the previous lease stay open, but it
  cannot open new ones.
- **Connection string.** The package connection string reuses only the
  server settings of the kernel's (host, port, database, TLS verification,
  time zone).
- **Kernel role.** The kernel's database role needs `CREATEROLE`
  (`ALTER ROLE <kernel_role> CREATEROLE` as a superuser). Without it, leases
  fail with an error that says so, and routes in `legacy` mode are
  unaffected.
- **Ledger.** `v4_kernel_package_storage` records the role, the schema, the
  grants and the lease generation of each package.

On SQLite there are no roles. Packages share the kernel's database file and
name their own tables with the prefix `pkg_<id>_`. This isolates nothing and
is meant for development and tests.

**Kernel API views** are versioned read-only views, created at startup and
never changed once published. `kapi_user_directory_v1` exposes `id`, `email`,
`is_admin`, `is_staff`, `banned`, `plan_id`, `group_id`, `expired_at` and
`created_at` of `v2_user`, and no password hash, token or UUID.
`kapi_system_audit_log_v1` exposes the `v2_operation_log` rows of module
`system` (the audit trail of configuration and backup changes, which records
whether a secret is set, never its value). `kapi_plan_catalog_v1` exposes
what an order needs of a plan (`id`, `group_id`, `transfer_enable`,
`speed_limit`, `device_limit` and the seven period prices of `v2_plan`),
`kapi_plan_name_v1` a plan's `id` and `name` (which the order answers show),
and `kapi_plan_subscription_group_v1` the `plan_id` and `group_id` of
`v2_plan_subscription_group`. `kapi_order_billing_v1` exposes what a payment
needs of an order: `id`, `user_id`, `total_amount` and `status` of
`v2_order`. `kapi_user_referral_v1` exposes `id` and `invite_user_id` of
`v2_user`: who invited whom. `kapi_affiliate_settings_v1` exposes the `value`
of the one `v2_system_config` row keyed `invite.frontend.config` (the
affiliate's frontend settings, which the kernel does not treat as sensitive)
and no other row. `kapi_user_subscription_group_v1` exposes `user_id`,
`group_id` and `expire_at` of `v2_user_subscription_group`: the subscription
groups a subscriber holds and until when. `kapi_node_protocol_v1` exposes
`id` and `node_id` of `v2_node_protocol`, and `kapi_node_heartbeat_v1` `id`
and `last_check_at` of `v2_node`; neither shows an address, a key or any
settings. `kapi_node_status_v1` exposes `id`, `status`, `last_check_at`,
`total_upload` and `total_download` of `v2_node`, and none of a node's
credentials. `kapi_forward_node_v1` exposes every column of
`v2_forward_node` but `api_token`, which authenticates the node's agent.
`kapi_forward_runtime_settings_v1` exposes `key` and `value` of the three
`v2_system_config` rows that choose the forward runtime backend
(`forward.runtime.nodex_mode`, `forward.runtime_backend`,
`forward.runtime.ansible.backend`) and no other row. `kapi_traffic_log_v1`
exposes `user_id`, `u`, `d`, `rate` and `log_at` of `v2_server_log`: each
node traffic report's bytes per user, for traffic charts. A view that filters
rows is created `WITH
(security_barrier)` on PostgreSQL, so a package's own functions never see the
rows it hides. If a view cannot be created, or its source table does not
exist, startup continues and leases that grant it fail.

`sdk/packagestoresdk` is the host side:

- `Open` leases storage and connects with at most 4 connections.
- `Store.Table` names the package's own tables: `pkg_<id>.<name>` on
  PostgreSQL, `pkg_<id>_<name>` on SQLite.
- `RunEmbeddedMigrations` applies the steps of an embedded
  `migrations/index.json`.
  - Each step runs in its own transaction with its row in
    `schema_migrations`, and `__PKG_PREFIX__` in scripts expands to the same
    prefix.
  - `__PKG_NAME_PREFIX__` is for index and constraint names. PostgreSQL does
    not schema-qualify them (they live in their table's schema), so it
    expands to nothing there and to the table prefix on SQLite.
  - An applied step whose digest changed is an error.
  - The run reports `StepsDigest`: the SHA-256 over `id:sha256\n` lines of
    the index's steps, sorted by id.
- `IndexMigrator` plugs this into `pluginhostsdk.RouterConfig.IndexMigration`.

### Package Migrations

When a Control host starts (`plugin.install`, `enable`, `update` or
`rollback`) for a release that declares `kernel.storage.v1` and lists
migration steps, the lifecycle dispatcher runs the release's migration index
through the migration ledger before the operation succeeds:

1. **Begin.** `BeginPackageMigration` records one run per package lifecycle
   generation with the migration id `index.<first 32 hex of the index
   digest>`.
   - A run that upgrades from an earlier validated generation records the
     backup reference `operator-managed:<package>:<generation>:<unix time>`:
     operators take the database backup before an upgrade, as `UPGRADE.md`
     requires.
2. **Migrate.** `MigratePackageHost` sends that id to the host. The host
   applies its embedded index and reports `StepsDigest`.
3. **Check.** The kernel compares the reported digest with the digest of the
   verified index in the artifact, whose per-step SHA-256 values were checked
   when the artifact was materialized. This proves that the scripts embedded
   in the host binary are the signed ones.
4. **Validate.** `RecordPackageValidation` closes the run.

If any step fails, the host is stopped and the lifecycle operation fails. A
Control restart re-runs `plugin.enable` for the same generation; the recorded
run is confirmed and nothing is applied twice. A failed run is not retried
within its generation; a new lifecycle operation gets a new generation. The
ledger's route cohorts are not used: traffic moves with route modes. Releases
without `kernel.storage.v1` are unaffected. identity-platform keeps its
kernel-side migration until it moves to a storage lease.

## Scoped Authorization

`service_scope` owns an independent authorization namespace. The startup
catalog creates `subscription`, `proxy`, `forward`, and `monitoring`; the
`monitoring` scope is owned by `machine-telemetry` so machine health does not
consume subscription or forwarding authorization and quota state. Startup does
not migrate existing subscription groups. `access_group_user` and
`access_group_plan` are combined as an allow-union within one scope. Resource
grants and quota policies are never evaluated across scopes.

Administrators manage this model from `/admin/access-groups`, backed by the
Kernel `/api/v3/access-groups` surface. A selected group is read through
`GET /api/v3/access-groups/:id`, which returns only member ID/email and plan
ID/name summaries alongside its grants and quota policies; it never returns
user credentials or profile state. The WebUI calls the server-side
`/api/v3/access-groups/resolve` endpoint for the effective-access preview so
the browser does not duplicate or widen the allow-union calculation.

## Topologies

Topology revisions are immutable snapshots. Validation rejects cycles, missing
vertices, port conflicts on one node, invalid MTU, missing secure-protocol
secret references, inline secret material, missing physical nodes, and missing
enabled node/plugin assignments. Topology JSON may reference secrets by ID or
`*_ref`; it must not contain passwords, API keys, tokens, or private keys.
Observed-state write-back is revision-fenced and monotonic: an Agent result is
accepted only for the exact deployment revision and known node, stale desired
or observed revisions are ignored, and a deployment reaches a terminal state
only after every distinct assigned node has reported. The feature-gated
executor compiles deployment steps by DAG order, dispatches per-node
`plugin.configure`/`plugin.enable` or disable operations, fences later steps on
failure, and rolls back changed steps in reverse DAG order. A succeeded task is
not sufficient to promote a revision: the executor reads a bounded projection
of the terminal Agent operation result and requires the exact plugin ID,
version, configuration hash, desired revision, observed revision, enabled
state, and health (`healthy` for enabled vertices or `disabled` for removals).
Raw Agent JSON, raw errors, and unknown fields are never copied into topology
health records.

Packages that declare the signed `kernel.observed-state` capability add a
second gate. The Agent heartbeat must persist a fresh structured observation
after the terminal operation, with exact version/config hash/revisions,
`healthy` state, a valid live ruleset SHA-256, and, for `nftables-forward`, the
same rule-ID counter set as the desired topology config. The deployment stores
a durable 90-second observation deadline so a Control restart resumes waiting;
missing evidence only rolls back after that deadline, while mismatched or
unhealthy evidence fails closed immediately. Rollback restore/disable steps use
the same fixed terminal-state gate before a deployment becomes `rolled_back`.
The Supervisor verifies the local package health socket on every heartbeat;
when it cannot safely read a current private observation it sends an
Supervisor-owned `unhealthy` record rather than replaying old health evidence.
Control treats observations stale after two minutes of trusted receipt time.
Public deployment, observed-state, and operation endpoints expose only fixed
state and generic error summaries, never the configuration, runtime payload,
session identity, or raw plugin error.
This executor is off by default and must not be used as production traffic evidence until the
corresponding protocol package and network tests exist.

## Operations

Operations are persisted before dispatch and require an operation UUID,
idempotency key, plugin/version, revision, canonical JSON config, SHA-256
config hash, and future deadline. The dispatcher writes the active Agent
session ID only when it claims an operation. Node operations advance a durable
desired-revision cursor under a transaction. Retrying requires the same
operation identity and payload; the same idempotency key cannot be rebound.
Expired pending, dispatching, running, or cancellation-requested work is marked
`timed_out`. Control-target operations are claimed in installation order under
a lease; a lost lease cancels the executor and fences late results. Implicit
installation updates include a monotonic lifecycle generation in their
idempotency key, so a disable/enable cycle cannot replay an old terminal
operation.

The payload fixture at `contracts/agent/v1/operation-envelope-golden.json` is
byte-identical in Control and Agent. Control tests assert the dispatcher
produces exactly those `DesiredOperation.payload_json` bytes; Agent tests assert
the same bytes decode under the active session and re-encode without drift.

When `plugins.dispatch_enabled` is explicitly enabled, the Control dispatcher
claims pending lifecycle operations, obtains the active Agent stream session,
constructs the versioned envelope from canonical persisted config, and records
ACK plus observed state back to the same operation row. It only dispatches the
currently implemented lifecycle capabilities
`plugin.inspect/configure/enable/disable/update/rollback/health`; Control
dependency plans and topology deployment plans create those primitive
operations instead of bypassing the durable dispatcher. The flag is disabled by
default, so a 3.x upgrade cannot alter legacy traffic behavior.
