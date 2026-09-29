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
- Control Center merge record (`control-center/`): [`CONTROL-CENTER-MERGE-PLAN.md`](CONTROL-CENTER-MERGE-PLAN.md)
- Open backlog: [`../TODO.md`](../TODO.md)
- Change history: [`../CHANGELOG.md`](../CHANGELOG.md)

## Install, Upgrade, And Operate

- Native release install: [`guide/release-installation.md`](guide/release-installation.md)
- Deployment guide: [`DEPLOYMENT.md`](DEPLOYMENT.md)
- Upgrade runbook: [`UPGRADE.md`](UPGRADE.md)
- V4 plugin-only upgrade: [`guide/v4-plugin-only-upgrade.md`](guide/v4-plugin-only-upgrade.md)
- V4 plugin-only rollback: [`guide/v4-plugin-only-rollback.md`](guide/v4-plugin-only-rollback.md)
- In-place Control migration: [`guide/control-migration.md`](guide/control-migration.md)
- Legacy panel migration: [`guide/legacy-migration.md`](guide/legacy-migration.md)
- SQLite to PostgreSQL migration: [`reference/sqlite-to-postgres-migration.md`](reference/sqlite-to-postgres-migration.md)
- Manual intervention requirements: [`manual-intervention.md`](manual-intervention.md)
- Brand and compatibility migration: [`BRAND_MIGRATION.md`](BRAND_MIGRATION.md)

## Startup And Configuration

- Docker quickstart: [`reference/quickstart.md`](reference/quickstart.md)
- Exact startup flow: [`reference/startup-config.md`](reference/startup-config.md)
- Config source-of-truth: [`reference/configuration.md`](reference/configuration.md)
- Repository layout: [`reference/repository-layout.md`](reference/repository-layout.md)
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
