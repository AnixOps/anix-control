# AnixOps Product Release Line And Delivery Status

Date: 2026-07-18

This document is the authoritative explanation of the product-version sequence
and the current delivery boundary. It prevents an experimental tag from being
mistaken for approval of a later product stage.

## Version Decision

The existing `v4.0.0-alpha.1` through `v4.0.0-alpha.7` tags were produced as
early package-platform experiments before the staged product programme was
frozen. They are immutable historical preview evidence. They do **not** mean
that AnixOps 4.0 has been released, that the coupled architecture has been
removed, or that production data-plane traffic is approved for plugin control.

The formal product line is deliberately ordered as:

```text
v3.1.0 -> v3.2.0 -> v3.3.0 -> v3.4.0 -> v3.5.0 -> v4.0.0
```

The next candidate is `v3.1.0-alpha.2`. At the time this document was written
it is a release candidate under verification, not a published tag. The Go
module path remains `github.com/AnixOps/anix-control/v4`; import-major version
and product version are separate contracts.

Historical preview tags must never be moved. Release automation treats a
historical preview as audit evidence only, and only a declared product-stage
tag may produce new signed package, Docker, or GitHub release assets.

## What Exists Now

The current worktree contains the following 3.1 foundation work. "Implemented"
means source and focused test coverage exist; it does not silently imply a
production rollout.

| Area | Status | Evidence and boundary |
|------|--------|-----------------------|
| Signed package kernel | Implemented | Canonical manifest validation, official trust root, immutable artifact storage, installation state, revisioned configuration, durable operations, audit, and rollback are exposed through `/api/v3`. |
| Official package WebUI | Implemented and locally E2E-verified | Control serves a content-addressed same-origin bundle only after signature, artifact, installation, and permission checks. The browser verifies its SHA-256 before import. Production runtime rejects a local compiled-module fallback. |
| Reference package | Implemented | `machine-telemetry` is the only formal 3.1 package: Control, Agent, and WebUI targets are declared together. |
| Scoped authorization | Implemented and administrable | `service_scope`, access groups, group users/plans, resource grants, quota policies, and server-side effective-access resolution exist. `/admin/access-groups` manages the model without returning member credentials. |
| Agent Supervisor canary configuration | Implemented, default off | The node deployment wizard can emit Supervisor fields only after explicit opt-in, a trusted Control channel, and an official public key. It does not alter the legacy data plane. |
| Real signed-WebUI browser gate | Implemented | An isolated Playwright test builds a real Control binary/frontend, registers an ephemeral signed package, checks catalog/asset/menu/route behavior, disables it through a durable operation, and verifies revocation. No browser route interception is used. |
| Release package scope | Implemented | `config/scripts/release-stage-contract.json` limits 3.1 assets to `machine-telemetry`; later forwarding packages cannot be represented as 3.1 release assets. |
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

The release candidate must additionally pass the repository-wide CI gates,
signed-package workflow, cross-repository Agent process gate, and release
artifact verification before a tag is created.

## Explicitly Not Complete

The following statements are intentionally false today:

- AnixOps 4.0 is not released and the kernel is not plugin-only.
- `v3.1.0-alpha.2` is not a stable release and is not production approved.
- No plugin package is authorized to take over proxy or forwarding traffic in
  3.1.
- `nftables-forward`, `gost-mesh`, `nat-egress`, WireGuard, and
  `protocol-runtime` are not part of the 3.1 package-release scope.
- A successful unit or browser test is not a 72-hour canary. Stable release
  still needs the canary record and explicit operator authorization.
- The legacy business domains have not yet been moved out of the kernel.

## Formal Stage Targets

| Stage | Product objective | Exit boundary |
|-------|-------------------|---------------|
| 3.1 | Signed package lifecycle and Modular Web UI | Release only `machine-telemetry`; prove package installation, Control/Agent lifecycle, real browser load/revocation, rollback, and legacy regression compatibility. |
| 3.2 | Declarative topology and dedicated forwarding | Add `nftables-forward` only after real TCP/UDP, IPv4/IPv6, rollback, staged rollout, and legacy-fallback evidence. |
| 3.3 | Tunnel mesh and NAT egress | Add `gost-mesh` and `nat-egress` only after mutual-TLS, health, cleanup, accounting, secret materialization, and multi-node rollback evidence. |
| 3.4 | WireGuard and protocol composition | Package WireGuard and protocol adapters; prove composed topology upgrades, client import, accounting, and rollback. |
| 3.5 | Business-domain migration | Move subscription, proxy, plan/order/payment, forwarding, ticket, notification, and content ownership behind packages while `/api/v2` acts as an adapter. |
| 4.0 | Plugin-only cutover | Remove coupled business/runtime paths only after clean bootstrap, final-3.5 upgrade, package reinstall, backup/restore, and full rollback evidence prove parity. |

## Stop Rules

Work on a prerelease stops only when its declared stage scope, tests, signed
assets, release metadata, and rollback documentation agree. A stable release
stops only after a 72-hour canary passes and the operator explicitly authorizes
promotion. A stage cannot borrow completion from a historical preview or a
later-stage package.

For detailed architecture and phase-specific evidence, see
[`plugin-platform-roadmap.md`](plugin-platform-roadmap.md) and
[`upgrade-program.md`](upgrade-program.md).
