# V4 Plugin-Only Stable Design

**Status:** Approved for implementation planning

**Goal:** Release AnixOps Control `v4.0.0` as a plugin-only platform while
preserving existing client, node, and `/api/v2` compatibility through
plugin-backed adapters and supporting a zero-downtime, reversible in-place
upgrade.

## Product Boundary

AnixOps Control v4 is a platform kernel, not the owner of subscription,
commercial, forwarding, support, or runtime business behavior. The kernel may
own identity, authentication, session validation, authorization, signed-package
verification, secrets, plugin lifecycle orchestration, routing, audit, generic
data migration orchestration, and operator health reporting.

Every domain operation outside those platform concerns executes in a signed
Control package or signed Agent package. The name and response envelope of an
existing `/api/v2` endpoint remain stable, but the endpoint enters a
compatibility adapter and then a domain package. It must not call a legacy
domain handler, service, model query, or background worker directly.

## Package Ownership

The v4 release contains these signed packages and no coupled substitutes:

| Package | Owns |
| --- | --- |
| `subscription` | Subscription groups, templates, rendering, public subscription output, and subscription usage contract. |
| `proxy-node` | Proxy-node registration, node protocol configuration, node configuration/user delivery, traffic and online reporting. |
| `plan` | Plan catalog, plan eligibility, user plan assignment, quota definitions, and entitlement reads. |
| `order` | Orders, renewals, promotions and coupon application, commercial state transitions, and entitlement requests. |
| `payment` | Payment gateway configuration, payment initiation, callbacks, reconciliation, and payment records. |
| `forward` | Forward rules, tunnels, relay assignments, limits, forwarding observability, and the public forwarding control-plane contract. |
| `ticket` | Tickets, messages, status transitions, and operator/user support views. |
| `notification` | Notices, Telegram and other outbound notifications, delivery history, and retry policy. |
| `knowledge` | Knowledge-base articles, categories, visibility, and user/admin article APIs. |
| `machine-telemetry` | Existing Control/Agent machine telemetry package. |
| `nftables-forward` | Existing Control/Agent declarative forwarding runtime package. |
| `gost-mesh` | Existing Control/Agent tunnel-mesh runtime package. |
| `nat-egress` | Existing Control/Agent NAT egress runtime package. |
| `wireguard` | WireGuard peer lifecycle, configuration, and Agent runtime package. |
| `protocol-runtime` | Protocol composition, runtime adapter selection, and Agent runtime package. |

Identity, authentication, users, access groups, package policy, package
permissions, generic audit, package registry, and generic operation state stay
in the kernel. A domain package accesses identity through a versioned platform
identity contract; it does not own credentials or JWT signing keys.

## Control Plugin Host

Control-target packages run as separately supervised local processes. A package
artifact contains its signed Control entrypoint, WebUI bundle, manifest,
configuration schema, migrations, and optional Agent entrypoints. The kernel
starts one host per installed Control package and communicates over a
permission-restricted local Unix socket using a versioned RPC contract.

The kernel verifies the package signature, manifest, artifact digest, installed
version, declared route, declared permission, and lifecycle generation before
dispatch. The request envelope contains the authenticated principal, stable
request identity, idempotency key, trace identifier, route identifier, deadline,
and package routing generation. The host returns only a typed status, response
headers, response body, operation references, and structured failure code.

Control packages are not network listeners. They receive only kernel-dispatched
requests and leased secrets that match declared capabilities. A missing package,
failed signature, incompatible API version, expired lifecycle lease, failed
health check, or unauthorized request fails closed. The kernel never silently
routes that request to legacy domain code.

## Compatibility Routing

The `/api/v2` router keeps public paths, methods, authentication requirements,
request fields, response envelopes, and documented error codes stable. Its
compatibility adapter resolves a declarative mapping from legacy route to a
package route and passes the request envelope to the package host.

The adapter is limited to protocol translation, actor context, idempotency,
correlation, response-envelope translation, and rollout generation selection.
It may not query domain tables, make domain decisions, mutate domain state, or
start a domain worker. New `/api/v4/plugins/<package>/...` paths expose the same
package contracts without legacy envelope translation.

The static release gate rejects a v4 build if a required business route in
`internal/router` references a legacy domain handler or if a required domain
worker starts from the kernel. Compatibility adapters are the only permitted
legacy-path handlers after cutover.

## Data Migration And Rollback

Each package owns an additive, versioned migration set and tables prefixed with
its package namespace so the design works with both SQLite and PostgreSQL. The
kernel owns generic migration-run, checksum, rollout-generation, backup
reference, and validation-result tables only. It does not own a domain mapping
or domain projection rule.

Every domain follows this fixed transition:

1. Install the signed package with execution disabled and run preflight checks.
2. Add package tables and a package-owned compatibility projection without
   removing legacy tables or columns.
3. Backfill in durable checkpoints and compare row counts, aggregate values,
   domain hashes, and sampled API results.
4. Route a cohort through the package. During the mirror interval, package
   writes maintain the old-format projection in the same database transaction
   for money, entitlement, quota, and subscription mutations; asynchronous
   delivery uses a durable package outbox.
5. Advance the routing generation only after validation and request drain.
   Retain the prior package version and compatibility projection during the
   canary interval.
6. Remove legacy execution only after the full canary, restore rehearsal, and
   explicit destructive-migration approval. Old data remains in the approved
   backup set until the rollback window closes.

Migration runs are idempotent and record the before/after schema version,
migration digest, checkpoint, validation digest, package version, and rollback
target. A failed migration resumes from its checkpoint or restores the previous
routing generation and package version. No destructive migration is permitted
before a verified backup and a tested reverse migration.

## Rollout Model

Package rollout is ordered by dependency:

1. Plugin host, package routing, package migration, compatibility adapter, and
   generic observability.
2. Read-mostly domains: knowledge and notification.
3. Support and catalog domains: ticket and plan.
4. Commercial domains: order and payment, including coupon behavior owned by
   the order package.
5. Subscription and proxy-node domains, including public subscription and
   node-facing contracts.
6. Forward domain and its runtime package integrations.
7. WireGuard and protocol-runtime packages.
8. Final removal of coupled domain execution after all package canaries pass.

Each package rolls out at 1, 5, 25, and 100 percent of the configured cohort.
The kernel records the exact route generation for each request. A package host
crash, lease loss, migration validation mismatch, incompatible response,
duplicate non-idempotent mutation, health failure, or configured error/latency
threshold breach halts expansion and rolls the affected cohort back to the
previous verified generation.

## Required Verification

The v4 release cannot be tagged until all of the following have current,
machine-readable evidence:

- Deterministic build, signature, SBOM, manifest, checksum, and install tests
  for all fifteen packages.
- Clean SQLite and PostgreSQL bootstrap that installs, enables, disables,
  uninstalls, and reinstalls every package.
- Final v3 database upgrade rehearsal with all package migrations, legacy-path
  contract comparisons, subscription output comparisons, commercial-state
  checks, node configuration/user delivery checks, and forwarding behavior
  checks.
- Contract tests proving every supported `/api/v2` business endpoint is served
  by a package compatibility adapter and remains response-compatible.
- Fault-injection tests for host crashes, deadlines, duplicate requests,
  interrupted migrations, Agent disconnects, failed package upgrades, and
  rollback failures.
- Backup/restore and full reverse-upgrade rehearsals on SQLite and PostgreSQL.
- Real cross-repository Control/Agent verification for every Agent-target
  package, including network-runtime acceptance where applicable.
- Production canary evidence spanning at least 72 hours through the 1/5/25/100
  rollout stages, with no unresolved data-validation, health, or rollback
  incident.
- Explicit operator approval after the canary and before the formal tag.

## Release Artifacts And Historical Hygiene

The formal `v4.0.0` tag is created only after version surfaces, package scope,
release manifest, release notes, Docker image, binaries, frontend assets,
signed packages, checksums, SBOM, upgrade guide, rollback guide, and artifact
verification agree on the same commit and package versions.

The existing read-only GitHub pull-request ref for PR #4 still exposes an old
history object outside normal branch and tag control. GitHub Support must purge
that ref and cached content before v4 is described as fully history-scrubbed.
This external request is separate from code implementation and remains a
release-hygiene prerequisite.

## Non-Negotiable Invariants

- No production business request reaches a coupled legacy handler or worker
  after its package cutover.
- `/api/v2` compatibility remains stable for supported clients and nodes.
- Every mutation is idempotent, audited, generation-fenced, and rollbackable.
- Every package operation is signed, authorized, observable, and fail-closed.
- Database changes are additive until validated rollback evidence exists.
- A zero-downtime upgrade never requires an operator to hand-edit database rows,
  package files, or route state.

