# AnixOps Product Release Line And Delivery Status

Date: 2026-09-29

This document records the staged product-version plan and the current delivery
boundary. The active `4.0.x` RC plan and its verification status are in
[`ROADMAP-4.0.x-RC.md`](../ROADMAP-4.0.x-RC.md) and
[`RC-EVIDENCE-4.0.x.md`](../RC-EVIDENCE-4.0.x.md).

## Version Decision

The `v4.0.0-alpha.1` through `v4.0.0-alpha.7` tags were early package-platform
experiments before the staged product programme was frozen. They are immutable
historical preview evidence, not approval for production data-plane traffic.
The later `v4.0.0` tag was published on 2026-07-20 with a signed sixteen-package
baseline. Its evidence binds that tag's commit, not the current `4.0.x` RC
worktree or any new production traffic approval.

The earlier planned product line was:

```text
v3.1.0 -> v3.2.0 -> v3.3.0 -> v3.4.0 -> v3.5.0 -> v4.0.0
```

The published tags did not follow every intermediate stage in that sequence;
`v3.1.0-alpha.2` was a planned candidate, not the next current release. The Go
module path remains `github.com/AnixOps/anix-control/v4`; import-major version
and product version are separate contracts.

Historical preview tags must never be moved. Release automation treats them
as audit evidence only. The current worktree requires a new commit-bound CI
run, staging records, and operator approval before an additional RC tag is
published.

## What Exists Now

The current worktree retains the following foundation from the staged plan.
"Implemented" means source and focused test coverage exist; it does not
silently imply a production rollout of new worktree changes.

| Area | Status | Evidence and boundary |
|------|--------|-----------------------|
| Signed package kernel | Implemented | Canonical manifest validation, official trust root, immutable artifact storage, installation state, revisioned configuration, durable operations, audit, and rollback are exposed through `/api/v3`. |
| Official package WebUI | Implemented and locally E2E-verified | Control serves a content-addressed same-origin bundle only after signature, artifact, installation, and permission checks. The browser verifies its SHA-256 before import. Production runtime rejects a local compiled-module fallback. |
| Reference package | Implemented | `machine-telemetry` is the only formal 3.1 package: Control, Agent, and WebUI targets are declared together. |
| Scoped authorization | Implemented and administrable | `service_scope`, access groups, group users/plans, resource grants, quota policies, and server-side effective-access resolution exist. `/admin/access-groups` manages the model without returning member credentials. |
| Agent Supervisor canary configuration | Implemented, default off | The node deployment wizard can emit Supervisor fields only after explicit opt-in, a trusted Control channel, and an official public key. It does not alter the legacy data plane. |
| Real signed-WebUI browser gate | Implemented | An isolated Playwright test builds a real Control binary/frontend, registers an ephemeral signed package, checks catalog/asset/menu/route behavior, disables it through a durable operation, and verifies revocation. No browser route interception is used. |
| Release package scope | Implemented | `config/scripts/release-stage-contract.json` limits 3.1 assets to `machine-telemetry`; later forwarding packages cannot be represented as 3.1 release assets. |
| Compatible Control Center lifecycle slice | Preview/Partial | The Control Center (`control-center/`, imported from the archived `Anixops-control-center` repository) connects with a separate Control administrator session via `/api/v2/login`, then reads the official `/api/v3` catalog and installation state, edits revisioned configuration, submits idempotent Control/Agent install/update/enable/disable/rollback actions, and renders operation chains. Its Workers session remains independent. Local frontend tests/build and a full-process signed Agent install/update/enable rehearsal pass; live staging evidence is still required. |
| Default deployment safety | Implemented | The normal and production templates keep Control execution, Agent dispatch, and topology execution disabled. The development template is explicitly separate. |
| Legacy compatibility | Retained intentionally | `/api/v2`, subscription behavior, UniProxy synchronization, and the existing forwarding path remain active until their owning packages reach parity and migration evidence exists. |

## Current Verification

The foundation is verified with focused code, browser, and release-contract
checks. The expected commands are:

```bash
GOWORK=off go test ./internal/handler ./internal/router ./internal/config -count=1
cd web
npm test -- --run src/__tests__/AccessGroups.test.js src/__tests__/Nodes.test.js \
  src/__tests__/extensionRuntime.test.js src/__tests__/kernelApi.test.js
npm run build
npx playwright test --config playwright.live-control.config.js
cd ..
python3 config/scripts/check_release_stage.py --self-test
bash config/scripts/check_release_workflow.sh --self-test
bash config/scripts/check_release_workflow.sh
```

The current `4.0.x` release candidate must additionally pass the
repository-wide CI gates, signed-package workflow, cross-repository Agent
process gate, live staging, and release artifact verification before a new tag
is created.

## Production Baseline And Upgrade Rehearsal

Recorded 2026-09-29 from the production PostgreSQL backup taken that day
(`pg_dump` custom format, PostgreSQL 15.18).

- **Running version.** The production config still reports `app.version`
  `2.0.1`, but that value is stale. The schema matches the tables that
  `v4.0.0-alpha.6`/`v4.0.0-alpha.7` create (97 tables, no `v4_kernel_package_*`
  tables), and the operator confirmed a `v3.1`/`v4.0` alpha tag. The rehearsal
  therefore used the published `v4.0.0-alpha.7` binary as the old side.
- **Data size.** About 150 MB of PostgreSQL data: 18 users, 14 nodes (15
  node/protocol pairs), 23 WireGuard peers; no orders, payments, or forwarding
  rules. The largest node serves 16 users. Response sizes: `/s/:token` median
  3.8 KB, max 5.3 KB; UniProxy `config` max 1.4 KB; UniProxy `user` max 9.3 KB,
  far below the 1 MiB default package payload limit.
- **Schema delta to current `go_dev`.** Starting in `env: production`, which
  skips `AutoMigrate` and runs only the `Ensure*` schema helpers, adds exactly
  five tables: `v4_kernel_package_backup_reference`,
  `v4_kernel_package_migration_run`, `v4_kernel_package_rollout_lock`,
  `v4_kernel_package_route_generation`, and
  `v4_kernel_package_validation_result`. No existing table, column, or index
  changes. (M3 later adds `v4_kernel_package_storage` and the
  `kapi_user_directory_v1` view; see `docs/UPGRADE.md`.)
- **Crash exposure.** `v4.0.0-alpha.7` crashes with
  `fatal error: concurrent map writes` in the forward background error logger
  when several forward latency probes fail at the same time (latency
  concurrency > 1). The rehearsal hit it while outbound traffic was
  blocked.
  Fixed on `go_dev` (PR #14). Until production is upgraded, keep
  `forward_runtime.latency.concurrency: 1`.

The rehearsal ran `v4.0.0-alpha.7` (old) and `go_dev` `b905a4a8`
(new) side by side, on two copies of that backup. The new side installed the
sixteen signed `v4.0.0` packages. Both servers and the test client ran in one
network namespace with only `lo`. Credentials in the copies were replaced with
invalid values. Procedure: [`../UPGRADE.md`](../UPGRADE.md#upgrading-from-a-v31-or-v40-alpha-build).

| Check | Result |
|-------|--------|
| Subscriptions (`/s/:token`, `/api/v1/client/subscribe`, every user and `?type=`) | 360/360 byte-identical |
| UniProxy v1 (`config`, `user`, `alivelist` for every node and protocol) | 45/45 byte-identical |
| `/api/v2` GET routes from the catalog, admin and user identities | 82/82 status equal; 77/82 bodies equal. The 5 differences are three `cached_at` timestamps and two local-ansible paths that differ per instance. |
| Latency p50, old/new (ms) | subscribe 0.52/0.52; UniProxy user 4.13/4.18; `/api/v2/user/info` 0.24/0.23; `/api/v2/admin/users` 0.25/0.24 |
| Fault injection | A killed `identity-platform` host restarted after 1 s; the next `/api/v2/admin/users` returned 200 |
| Network isolation | No non-loopback sockets; the only blocked egress was the forward latency prober |

Defects found by the rehearsal, all fixed on `go_dev` before this record:

- the alpha.7 `concurrent map writes` crash above (PR #14);
- `/api/v2` route resolution chose the first matching route instead of the
  most specific one (PR #15);
- error bodies on `data`/`panel` routes were wrapped or rejected as 502
  (PR #17);
- every `/api/v2` request re-verified every signed package: 190-255 ms
  uncached, 0.22 ms with the verified-route cache (PR #18);
- the bridge replaced the request `Host` with `package-bridge`, so the
  forward-agent `install.sh` pointed at `http://package-bridge` (PR #19).

This is a local rehearsal on a copy of production data. It is not a staging
canary and does not authorize the production upgrade.

## Explicitly Not Complete

The following statements are intentionally false today:

- The current `4.0.x` worktree changes are not released or operator approved.
- The published `v4.0.0` evidence does not certify later uncommitted changes.
- No plugin package is authorized to take over proxy or forwarding traffic in
  3.1.
- `nftables-forward`, `gost-mesh`, `nat-egress`, WireGuard, and
  `protocol-runtime` are not part of the 3.1 package-release scope.
- A successful unit or browser test is not a 72-hour canary. Stable release
  still needs the canary record and explicit operator authorization.
- The legacy business domains have not yet been moved out of the kernel.

## Historical Stage Targets

| Stage | Product objective | Exit boundary |
|-------|-------------------|---------------|
| 3.1 | Signed package lifecycle and Modular Web UI | Release only `machine-telemetry`; prove package installation, Control/Agent lifecycle, real browser load/revocation, rollback, and legacy regression compatibility. |
| 3.2 | Declarative topology and dedicated forwarding | Add `nftables-forward` only after real TCP/UDP, IPv4/IPv6, rollback, staged rollout, and legacy-fallback evidence. |
| 3.3 | Tunnel mesh and NAT egress | Add `gost-mesh` and `nat-egress` only after mutual-TLS, health, cleanup, accounting, secret materialization, and multi-node rollback evidence. |
| 3.4 | WireGuard and protocol composition | Package WireGuard and protocol adapters; prove composed topology upgrades, client import, accounting, and rollback. |
| 3.5 | Business-domain migration | Move subscription, proxy, plan/order/payment, forwarding, ticket, notification, and content ownership behind packages while `/api/v2` acts as an adapter. |
| 4.0 | Plugin-only cutover | Remove coupled business/runtime paths only after clean bootstrap, final-3.5 upgrade, package reinstall, backup/restore, and full rollback evidence prove parity. |

## Stop Rules

Work on a prerelease stops only when its declared scope, tests, signed assets,
release metadata, and rollback documentation agree. Production promotion
requires the applicable canary and explicit operator authorization. A new
candidate cannot borrow completion from a historical tag or later worktree.

For the kernel/package contract, see
[`plugin-kernel-contract.md`](plugin-kernel-contract.md). The current release
work is tracked in [`../ROADMAP-4.0.x-RC.md`](../ROADMAP-4.0.x-RC.md) and its
evidence in [`../RC-EVIDENCE-4.0.x.md`](../RC-EVIDENCE-4.0.x.md). Earlier
3.1-to-4.0 planning documents were retired; their history remains in Git and
`CHANGELOG.md`.
