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

Each admitted release stores the signing trust-root fingerprint and key ID.
The kernel records all active official trust roots it has seen so normal key
rotation does not invalidate already admitted releases; later catalog and
configuration reads re-verify a release with the trust root bound to that
release. Removing or retiring a compromised trust root is a separate operator
decision, not an implicit side effect of changing `config.yaml`.

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
