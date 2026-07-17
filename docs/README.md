# AnixOps Control Docs

This is the main documentation landing page for `AnixOps/anix-control`.

Use this tree like NodeX:

- [`intro/README.md`](intro/README.md)
  - start here if you need repository role, boundary, and mode semantics
- [`reference/README.md`](reference/README.md)
  - use this for exact startup steps, config ownership, runtime mode references, and repository layout
- [`guide/README.md`](guide/README.md)
  - use this for deep implementation details, Flux-clone workstreams, and smoke-test guides

## Fast Paths

- Feature status register: [`features.md`](features.md)
- Plugin platform roadmap: [`architecture/plugin-platform-roadmap.md`](architecture/plugin-platform-roadmap.md)
- Major upgrade execution program: [`architecture/upgrade-program.md`](architecture/upgrade-program.md)
- Product version line and delivery status: [`architecture/release-line-status.md`](architecture/release-line-status.md)
- Plugin kernel contract: [`architecture/plugin-kernel-contract.md`](architecture/plugin-kernel-contract.md)
- Brand and compatibility migration: [`BRAND_MIGRATION.md`](BRAND_MIGRATION.md)
- Upgrade runbook: [`UPGRADE.md`](UPGRADE.md)
- Native release install: [`guide/release-installation.md`](guide/release-installation.md)
- Legacy panel migration: [`guide/legacy-migration.md`](guide/legacy-migration.md)
- Docker quickstart: [`reference/quickstart.md`](reference/quickstart.md)
- Exact startup flow: [`reference/startup-config.md`](reference/startup-config.md)
- Config source-of-truth: [`reference/configuration.md`](reference/configuration.md)
- Runtime config migration: [`reference/forward-runtime-migration.md`](reference/forward-runtime-migration.md)
- Runtime mode entrypoint: [`reference/runtime.md`](reference/runtime.md)
- Relay onboarding: [`guide/forward-relay-onboarding.md`](guide/forward-relay-onboarding.md)
- P0 WireGuard relay plan: [`guide/wireguard-relay.md`](guide/wireguard-relay.md)
- Config examples: [`../config/examples/README.md`](../config/examples/README.md)

## Rule Of Thumb

- if you are asking "what does this repo own": read `intro`
- if you are asking "what do I edit to boot this": read `reference`
- if you are asking "how was this feature cloned or implemented": read `guide`
- if you are asking "what is implemented or still planned": read `features.md`
