# AnixOps Product Release Line And Delivery Status

Date: 2026-09-28

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
| Compatible Control Center lifecycle slice | Preview/Partial | `Anixops-control-center` connects with a separate Control administrator session via `/api/v2/login`, then reads the official `/api/v3` catalog and installation state, edits revisioned configuration, submits idempotent Control/Agent install/update/enable/disable/rollback actions, and renders operation chains. Its Workers session remains independent. Local frontend tests/build and a full-process signed Agent install/update/enable rehearsal pass; live staging evidence is still required. |
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

For detailed architecture and phase-specific evidence, see
[`plugin-platform-roadmap.md`](plugin-platform-roadmap.md) and
[`upgrade-program.md`](upgrade-program.md).
