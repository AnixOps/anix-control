# AnixOps Major Upgrade Program

Date: 2026-07-17

This document is the execution plan for the 3.x to 4.0 migration. A version is
promotable only when its exit evidence exists in CI or in a recorded staging
rehearsal. A route, table, mock page, or unit test by itself is not release
evidence.

## Current Decision

The immediate target is **3.1.0 package-platform foundation**, not 4.0. The
legacy `/api/v2` APIs, subscription behavior, UniProxy synchronization, and
forwarding workers stay on the production path while the package manager is
introduced behind feature flags.

Current status: **the `v3.1.0-alpha.2` foundation candidate is under
verification; it is canary-only and blocked for production data-plane
release**. Historical `v4.0.0-alpha.*` tags are not a substitute for the
formal 3.1 through 4.0 gates. See
[`release-line-status.md`](release-line-status.md) for the version decision and
current implementation boundary.

Already present in the current branch:

- signed Control/Agent manifest and operation-envelope contracts;
- official trust-root and immutable artifact/WebUI verification;
- scoped access-group and plugin-installation models;
- durable Control lifecycle operations with ACK, observed state, leases,
  cancellation, restart replay, rollback, and lifecycle-generation idempotency;
- Agent Supervisor package extraction, process health, cancellation, restart
  journal replay, and the `machine-telemetry` reference process;
- import-free, same-origin, digest-verified WebUI extension loading.
- a repeatable SQLite-to-PostgreSQL restore rehearsal with schema and row
  evidence, wired into CI;
- an opt-in cross-repository process gate using a real Agent binary, Control
  database, `KernelOperationBridge`, and terminal observed state;
- a real Chromium Playwright gate for WebUI route isolation and fail-closed
  bundle loading;
- deterministic package build, public-key-only signature verification, and
  tamper-rejection evidence in CI.
- dependency-aware Control lifecycle plans with dependency-first execution,
  cancellation/deadline reconciliation, restart recovery, and reverse rollback;
- feature-gated topology deployment fan-out with durable per-node steps,
  failure fencing, reverse rollback, and observed-state write-back;
- production release-tag package signing/upload workflow that requires the
  `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY` GitHub secret and publishes signature,
  public-key, package, manifest, and checksum evidence for `machine-telemetry`,
  `nftables-forward`, `gost-mesh`, and `nat-egress`.
- deterministic `nftables-forward` package source with WebUI smoke coverage,
  real Agent runtime packaging, public-key verification, tamper rejection, and
  CI release-contract evidence.
- privileged `nftables-forward` namespace acceptance in the pinned Agent repo
  proving TCP DNAT, UDP DNAT, plugin-created table deletion on rollback, and
  pre-existing nftables table snapshot restoration.
- the real `nat-egress` Agent runtime with nftables masquerade, fwmark policy
  routing, interface-bound marked health probes, crash-safe ownership
  journaling, and privileged namespace traffic/rollback acceptance. Its
  production release signing, artifact verification, and immutable Agent
  revision pin are wired and locally gate-verified.
- the real `gost-mesh` Agent runtime with aggregate `tunnels[]`, signed GOST
  v3.2.6 auxiliary-runtime packaging, QUIC/WSS mutual TLS, entry source-policy
  routing, source-bound health probes, bounded restart, ownership journaling,
  and privileged namespace TCP/UDP, TLS-negative, health, and cleanup evidence.
  TUIC is intentionally excluded from v1 because GOST v3.2.6 does not implement
  it.
- Supervisor `plugin.runtime-state` and `plugin.cleanup` contracts with durable
  `cleanup_pending` recovery across crashes and Agent restarts.

The following remain release blockers: staging rehearsal that starts the
restored Control service and checks login/subscription/catalog behavior,
canary rollout records, legacy fallback rehearsal, and a manual approval to
enable topology execution outside isolated test nodes. For `gost-mesh`, Control
Secret ID to Agent private-file materialization, renewal, deletion, and audit
are also blocking. Current TLS path fields refer to files already present on
the Agent and are acceptable only for an isolated canary.

## Release Invariants

These invariants apply to every phase:

1. `plugins.control_execution_enabled` and `plugins.dispatch_enabled` remain
   false by default until the phase canary is explicitly approved.
   `plugins.topology_execution_enabled` also remains false by default and is
   refused at startup unless Agent dispatch is enabled.
2. No phase changes production data-plane traffic before its package has a
   tested legacy fallback and an operator rollback command.
3. Every operation has one durable identity, one idempotency key, a revision,
   a deadline, a canonical config hash, and a terminal result.
4. Disabled packages retain data. Deletion is a separate audited purge action.
5. A failed health check, lease loss, reconciliation error, or rollback error
   stops rollout expansion.
6. A stateful plugin update may restart the old version only after the target
   version's signed cleanup succeeds. Cleanup failure leaves the plugin
   disabled with `cleanup_pending` and blocks rollout until recovery succeeds.

## Phase 0: Baseline And Freeze

### Goal

Make the existing 2.x/3.0 behavior reproducible before enabling any package
runtime.

### Deliverables

- Pin Go, Node, npm, protobuf, PostgreSQL, and SQLite tool versions.
- Record the current database schema checksum, row counts, `/health` response,
  subscription bytes, and representative UniProxy responses.
- Keep a release artifact manifest, SHA-256 list, SBOM, migration output, and
  rollback artifact for every candidate.

### Exit tests

```bash
cd /home/dev/anixops/v2board_AnixOps
GOCACHE=/tmp/anix-control-go-cache GOTOOLCHAIN=local GOWORK=off go test ./... -count=1 -p=1
GOCACHE=/tmp/anix-control-go-cache GOTOOLCHAIN=local GOWORK=off go vet ./...
cd web && npm test -- --run
cd .. && git diff --check

cd /home/dev/anixops/V2bX_AnixOps
GOEXPERIMENT=jsonv2 GOWORK=off go test ./... -count=1
git diff --check
```

### Stop condition

Any legacy regression or missing backup/rollback evidence blocks the next
phase. Do not enable Control or Agent plugin execution as a workaround.

## Phase 1: 3.1.0 Lifecycle Hardening

### Goal

Make package intent and lifecycle operations safe across duplicate requests,
multiple Control workers, Control restart, Agent restart, cancellation, and
lease loss.

### Deliverables

- target-level transaction locking for dependency/conflict validation;
- one ordered operation stream per plugin installation;
- monotonic lifecycle-generation keys for implicit Upsert operations;
- lease renewal fencing that cancels the executor when ownership is lost;
- bounded automatic rollback that remains under lease supervision;
- explicit install versus enable semantics and cancellation propagation;
- Agent forced-stop success handling and unexpected-exit observation.

### Exit tests

- two workers cannot execute the same installation concurrently;
- Control restart does not enqueue a new enable ahead of an unexpired old lease;
- a lease-loss test cancels the running executor and prevents late success;
- disable then enable creates a new operation rather than replaying an old one;
- cancelled, timed-out, superseded, succeeded, and failed states never regress;
- Agent process crash, forced kill, configure failure, and journal replay are
  tested with real temporary processes.

This phase is a prerequisite for any mutating official Control plugin.

## Phase 2: 3.1.0 Promotion Evidence

### Goal

Prove that the package platform works across repositories and deployment
boundaries, not only in isolated unit tests.

### Deliverables

1. **PostgreSQL restore rehearsal (evidence complete)**: the script and CI job
   import a fixture, run `pg_dump`, restore into a clean database, and compare
   schema/row/hash invariants. A staging run must still start Control and check
   `/health`, login, subscription, and plugin catalog.
2. **Real cross-repository E2E (evidence complete, scope bounded)**: the
   opt-in gate builds a real Agent fixture, connects it to the real Control
   gRPC server and durable bridge, and asserts terminal state plus envelope
   identity. The Agent repository separately covers Supervisor plus the
   signed `machine-telemetry` process chain.
3. **Browser E2E (evidence complete, fixture plus isolated live Control)**:
   Playwright covers same-origin digest loading, route collision isolation,
   disabled entries, tampered bundles, and kernel-page recovery. The CI
   live-Control gate additionally boots a real temporary Control/frontend,
   registers and enables an ephemeral signed `machine-telemetry` package, and
   verifies catalog/asset/menu/route creation plus worker-completed disable
   revocation. A production-like staging canary is still required.
4. **Dependency execution (implementation complete, feature gated)**:
   recursive loading, stable dependency-first order, cycle/missing/conflict
   detection, trust-root re-verification, dependency-aware Control execution,
   cancellation, restart reconciliation, and reverse rollback are implemented
   and covered by focused tests.
5. **Topology deployment (implementation complete, feature gated)**:
   monotonic revision-fenced observed-state write-back, bridge mapping,
   deployment planning, DAG-ordered per-node operation fan-out, failure
   fencing, and reverse rollback are implemented. The executor does not run
   unless `plugins.dispatch_enabled` and `plugins.topology_execution_enabled`
   are both explicitly enabled.
6. **Release pipeline (production signing path complete)**: deterministic
   package builds, unsigned verification, Ed25519 signing, public-key-only
   verification, tamper rejection, release-tag signing with
   `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY`, package checksum evidence, and
   signed package upload are wired into CI.

### 3.1 release gate

The completed evidence items and all Phase 1 tests must pass in CI. Staging
service rehearsal and canary records must still be closed before
`machine-telemetry` can leave an isolated development canary; 3.1 is still not
production-complete.

## Phase 3: 3.1 Machine-Telemetry Canary

Enable `control_execution_enabled` only for one non-production Control instance
and one non-business Agent node. Observe at least one full restart cycle and
record operation latency, lease-renewal failures, process restarts, health
failures, duplicate operation count, and audit events. The package must be
disabled automatically on any invariant violation. This phase does not carry
proxy or forwarding traffic.

## Phase 4: 3.2 Declarative Topology And Dedicated Forwarding

Implement immutable topology revisions, DAG validation, capability/port/MTU/
route/secret checks, deployment fan-out, canary groups, atomic nftables
snapshots, and rollback. The Control fan-out and rollback executor is now
present behind `plugins.topology_execution_enabled`; the deterministic
`nftables-forward` package release contract and real Agent runtime are present
in CI/release packaging. The pinned Agent namespace acceptance script now
proves namespace TCP/UDP traffic and nftables snapshot rollback. The remaining
3.2 work is staging restore smoke, rollout records, legacy fallback rehearsal,
and operator approval. Agent processes must not proxy the bulk traffic.

## Phase 5: 3.3 Tunnel Mesh And NAT

Deliver signed `gost-mesh` and `nat-egress` packages for WSS and QUIC. TUIC is
deferred to an independent runtime or `protocol-runtime`; it must not be
advertised by `gost-mesh` v1 because the pinned GOST v3.2.6 executable does not
implement it.

Both packages now have real Agent runtimes, deterministic package builds,
public-key verification, tamper rejection, crash-safe ownership journals, and
signed cleanup. `gost-mesh` manages aggregate `tunnels[]`, entry source-policy
routing, source-bound health probes, and bounded GOST child-process restart.
QUIC and WSS require mutual TLS. Namespace acceptance proves TCP/UDP traffic,
the expected UDP/TCP transport, wrong-SNI and untrusted-client rejection,
health, and cleanup. `nat-egress` proves policy-routed marked forwarding,
wrong-mark isolation, masquerade, and rollback. Release package gates require
real Agent binaries and verify both the GOST v3.2.6 archive checksum and the
extracted binary checksum.

Before any apply, operators must set `net.ipv4.ip_forward=1` and disable strict
reverse-path filtering for the participating namespaces/interfaces
(`net.ipv4.conf.*.rp_filter=0`). The plugin validates and fails closed; it does
not modify host-wide sysctls. Current TLS fields are Agent-local file paths.
Control Secret ID materialization, renewal, deletion, and audit remain a hard
stable-release gate. Both plugins also require composed exit-failure rollback,
MTU/loss/reconnect evidence, accounting isolation, multi-node rollback, and a
sustained canary before production rollout.

## Phase 6: 3.4 WireGuard And Protocol Composition

Move WireGuard peer custody, key rotation, policy routing, limits, and protocol
adapters into packages. Prove WireGuard-to-GOST/NAT and WireGuard-to-protocol-
adapter/NAT topologies, client imports, traffic accounting, interrupted
upgrade, and composed-topology rollback. Keep the coupled implementation as a
fallback until parity evidence is complete.

## Phase 7: 3.5 Business Ownership Migration

Migrate subscriptions, proxy nodes, plans/orders/payments, forwarding, tickets,
notifications, and content one package at a time. Use shadow reads, dual-write
or change capture, row/amount reconciliation, and an independently reversible
ownership switch. `/api/v2` becomes an adapter to package services; it remains
available throughout the observation window.

## Phase 8: 4.0 Plugin-Only Cutover

Only after the 3.5 observation window, remove coupled handlers/workers,
global-node-type semantics, and obsolete ownership tables. Verify clean-database
bootstrap, upgrade from final 3.5, backup/restore, install/reinstall of every
default package, no kernel imports of plugin domains, no request reaching legacy
handlers, and a full rollback rehearsal. The kernel must boot without business
packages and install the default profile through its own package manager.

## Operator Upgrade Procedure

For every version:

1. Freeze the current release tag, config, database backup, schema checksum,
   and legacy smoke results.
2. Download only CI artifacts; verify `SHA256SUMS`, release manifest, SBOM,
   migration output, and package signatures.
3. Run the migration/restore rehearsal for the target release before the
   maintenance window.
4. Deploy to staging, then one canary Control/Agent pair with plugin flags off.
5. Enable only the phase-specific flag; wait for health and reconciliation
   evidence; expand by rollout group.
6. On any failed health, stale revision, duplicate operation, accounting
   mismatch, or rollback error, disable the flag, restore the previous binary
   and config, and follow `docs/UPGRADE.md`.
7. Retain backups, logs, hashes, and canary observations until the rollback
   window expires.

The next engineering milestone is therefore **Phase 1 completion plus the six
Phase 2 promotion deliverables**. No 3.2 data-plane work should be declared
production-ready before that gate is closed.
