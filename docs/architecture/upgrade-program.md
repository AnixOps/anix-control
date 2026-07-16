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

Current status: **3.1 preview, blocked for general release**.

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

The following remain release blockers: executing a resolved dependency graph
with dependency-aware rollback, completing topology deployment fan-out and
observed-state integration with real topology executors, a production
secret-backed package signing/upload job (the current CI gate signs only an
ephemeral test key), and a staging rehearsal that starts the restored Control
service and checks login/subscription/catalog behavior.

## Release Invariants

These invariants apply to every phase:

1. `plugins.control_execution_enabled` and `plugins.dispatch_enabled` remain
   false by default until the phase canary is explicitly approved.
2. No phase changes production data-plane traffic before its package has a
   tested legacy fallback and an operator rollback command.
3. Every operation has one durable identity, one idempotency key, a revision,
   a deadline, a canonical config hash, and a terminal result.
4. Disabled packages retain data. Deletion is a separate audited purge action.
5. A failed health check, lease loss, reconciliation error, or rollback error
   stops rollout expansion.

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
3. **Browser E2E (evidence complete, catalog fixture)**: Playwright covers
   same-origin digest loading, route collision isolation, disabled entries,
   tampered bundles, and kernel-page recovery. A later staging test must use a
   running Control catalog instead of the HTTP fixture.
4. **Dependency resolver (preflight complete)**: recursive loading, stable
   dependency-first order, cycle/missing/conflict detection, and trust-root
   re-verification are implemented. Dependency-aware execution and graph
   rollback remain to be delivered.
5. **Topology observed state (write contract complete)**: monotonic,
   revision-fenced node/deployment write-back and bridge mapping are tested.
   Real topology executors, fan-out, and rollback integration remain pending.
6. **Release pipeline (contract evidence complete)**: deterministic package
   builds, unsigned verification, ephemeral Ed25519 signing, public-key-only
   verification, and tamper rejection run in CI. Production secret-backed
   signing, upload, and release-manifest package binding remain pending.

### 3.1 release gate

The completed evidence items and all Phase 1 tests must pass in CI. The
remaining execution, topology, production-signing, and staging items must be
closed before `machine-telemetry` can leave an isolated development canary;
3.1 is still not production-complete.

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
snapshots, and rollback. Deliver `nftables-forward` for domestic dedicated-line
TCP and UDP only. Verify it in network namespaces with a 1/5/25/100 percent
rollout and a legacy fallback. Agent processes must not proxy the bulk traffic.

## Phase 5: 3.3 Tunnel Mesh And NAT

Deliver signed `gost-mesh` and `nat-egress` packages for WSS, TUIC, and QUIC.
Test certificates, MTU, IPv4/IPv6, UDP loss, reconnect, route-loop prevention,
accounting isolation, exit failure, and multi-node rollback before canary.

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
