# AnixOps Control Docs

This is the main documentation landing page for `AnixOps/anix-control`.

Use this tree like NodeX:

- [`intro/README.md`](intro/README.md)
  - start here if you need repository role, v4 architecture, boundary, and mode semantics
- [`reference/README.md`](reference/README.md)
  - use this for exact startup steps, config ownership, runtime mode references, and repository layout
- [`guide/README.md`](guide/README.md)
  - use this for deep implementation details, Flux-clone contracts, runtime operations, and runbooks

## Status And Planning

- Feature status register: [`features.md`](features.md)
- Current 4.0.x RC execution roadmap: [`ROADMAP-4.0.x-RC.md`](ROADMAP-4.0.x-RC.md)
- Current RC verification snapshot: [`RC-EVIDENCE-4.0.x.md`](RC-EVIDENCE-4.0.x.md)
- Product version history and staged-plan status: [`architecture/release-line-status.md`](architecture/release-line-status.md)
- Plugin kernel contract: [`architecture/plugin-kernel-contract.md`](architecture/plugin-kernel-contract.md)
- Package extraction design and current development direction: [`architecture/package-extraction.md`](architecture/package-extraction.md)
- Order completion contract for the payment callbacks: [`architecture/order-service.md`](architecture/order-service.md)
- Node operations contract, node credential split and Agent A2 (design, draft): [`architecture/node-ops-service.md`](architecture/node-ops-service.md)
- Forward SDK: routes, hops, engines and drivers for v4.2 (design, draft for review): [`architecture/forward-sdk.md`](architecture/forward-sdk.md)
- Package reports on the Agent Control stream, and the privacy of the systemd services report: [`architecture/package-reports.md`](architecture/package-reports.md)
- Network module runtime (mTLS, module PKI, remote runtime): [`architecture/module-runtime.md`](architecture/module-runtime.md)
- Identity service design (login and credentials as a module): [`architecture/identity-service.md`](architecture/identity-service.md)
- Kernel contracts for modules: subscriber state [`architecture/subscriber-service.md`](architecture/subscriber-service.md), system settings [`architecture/settings-service.md`](architecture/settings-service.md), the kernel's cached answers [`architecture/kernel-caches.md`](architecture/kernel-caches.md)
- Control Center merge record (`control-center/`): [`CONTROL-CENTER-MERGE-PLAN.md`](CONTROL-CENTER-MERGE-PLAN.md)
- Open backlog: [`../TODO.md`](../TODO.md)
- Change history: [`../CHANGELOG.md`](../CHANGELOG.md)

## Install, Upgrade, And Operate

- Deployment guide (Docker Compose, Kubernetes): [`DEPLOYMENT.md`](DEPLOYMENT.md)
- Container design and multi-replica status: [`architecture/container-deployment.md`](architecture/container-deployment.md)
- Native systemd install (frozen): [`guide/release-installation.md`](guide/release-installation.md)
- Upgrade runbook: [`UPGRADE.md`](UPGRADE.md)
- V4 plugin-only upgrade: [`guide/v4-plugin-only-upgrade.md`](guide/v4-plugin-only-upgrade.md)
- V4 plugin-only rollback: [`guide/v4-plugin-only-rollback.md`](guide/v4-plugin-only-rollback.md)
- Official package signing root rotation: [`guide/release-root-rotation.md`](guide/release-root-rotation.md)
- In-place Control migration: [`guide/control-migration.md`](guide/control-migration.md)
- Legacy panel migration: [`guide/legacy-migration.md`](guide/legacy-migration.md)
- Route cutover staging rehearsal (legacy → shadow → native, batch sign-off): [`guide/staging-rehearsal.md`](guide/staging-rehearsal.md)
- SQLite to PostgreSQL migration: [`reference/sqlite-to-postgres-migration.md`](reference/sqlite-to-postgres-migration.md)
- Manual intervention requirements: [`manual-intervention.md`](manual-intervention.md)
- Brand and compatibility migration: [`BRAND_MIGRATION.md`](BRAND_MIGRATION.md)

## Startup And Configuration

- Docker quickstart: [`reference/quickstart.md`](reference/quickstart.md)
- Environment variables (`ANIX_CONTROL_*`): [`reference/environment-variables.md`](reference/environment-variables.md)
- Exact startup flow: [`reference/startup-config.md`](reference/startup-config.md)
- Config source-of-truth: [`reference/configuration.md`](reference/configuration.md)
- Repository layout: [`reference/repository-layout.md`](reference/repository-layout.md)
- Frontend design system (AnixOps Design tokens, brand assets, sync and lint): [`reference/frontend-design.md`](reference/frontend-design.md)
- Control boundary and entry points: [`control-boundary.md`](control-boundary.md)

## Forwarding Runtime

- Runtime mode entrypoint: [`reference/runtime.md`](reference/runtime.md)
- Runtime config migration: [`reference/forward-runtime-migration.md`](reference/forward-runtime-migration.md)
- Relay onboarding: [`guide/forward-relay-onboarding.md`](guide/forward-relay-onboarding.md)
- Forwarding module design, API, security, and compatibility:
  [`forwarding/design.md`](forwarding/design.md),
  [`forwarding/api.md`](forwarding/api.md),
  [`forwarding/security.md`](forwarding/security.md),
  [`forwarding/compatibility.md`](forwarding/compatibility.md)
- Clean-room forward agent: [`forward-clean-room/spec.md`](forward-clean-room/spec.md),
  [`forward-clean-room/provenance.md`](forward-clean-room/provenance.md)

## WireGuard

- P0 WireGuard relay plan: [`guide/wireguard-relay.md`](guide/wireguard-relay.md)
- Peer schema: [`reference/wireguard-peer-schema.md`](reference/wireguard-peer-schema.md)
- Entry network policy: [`guide/wireguard-network-policy.md`](guide/wireguard-network-policy.md)
- Peer key rotation: [`guide/wireguard-key-rotation.md`](guide/wireguard-key-rotation.md)

## Audit Registers

- [`audit/repository-audit.md`](audit/repository-audit.md)
- [`audit/security-risk.md`](audit/security-risk.md)
- [`audit/concurrency-risk.md`](audit/concurrency-risk.md)
- [`audit/performance-risk.md`](audit/performance-risk.md)
- [`audit/test-gap.md`](audit/test-gap.md)

## Rule Of Thumb

- if you are asking "what does this repo own": read `intro`
- if you are asking "what do I edit to boot this": read `reference`
- if you are asking "how was this feature cloned or implemented": read `guide`
- if you are asking "what is implemented or still planned": read `features.md`
