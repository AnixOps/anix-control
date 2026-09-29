# AnixOps Control

`anix-control` is the public control-plane repository in the AnixOps stack.

It owns:
- public web UI and panel API (`/api/v2`, routed through the signed-package gateway)
- the v4 plugin kernel: signed package catalog, releases, installations,
  operations, and deployments under `/api/v3`, plus the package host processes
  that serve `packages/*`
- user, plan, order, ticket, knowledge, and subscription data
- public proxy-node inventory and UniProxy-compatible APIs
- Flux-compatible `/admin/forward*` control pages

It does not own the private execution plane.

Boundary:
- `anix-control`: public control plane, plugin kernel, persistence, and admin UI
- `control-center/` (in this repository): Control Center clients (CLI/TUI, Vue web, Flutter) and their Cloudflare Workers API; a separate Go module that talks to Control over its public `/api/v2` and `/api/v3` APIs
- `anix-agent` (separate repository `AnixOps/anix-agent`): proxy-node, forwarding, and package runtime on nodes; Control depends only on its `github.com/AnixOps/anix-agent/sdk` module and talks to it over gRPC
- `NodeX` and legacy clean-agent paths: compatibility runtimes being consolidated into AnixOps Agent

## Start Here

- [`docs/README.md`](README.md): top-level docs entrypoint
- [`docs/intro/README.md`](intro/README.md): repository role, v4 architecture, and deployment boundary
- [`docs/reference/README.md`](reference/README.md): startup, config, repository layout, and runtime references
- [`docs/guide/README.md`](guide/README.md): Flux-clone and implementation deep dives

## Exact Startup Truth

Before you try to start the panel, keep these rules straight:

- the backend always loads `config/config.yaml` through `-config` (template: [`config/config.yaml.example`](../config/config.yaml.example))
- local `go run` does not auto-load `.env` (template: [`.env.example`](../.env.example))
- `config/config.yaml.forward_runtime` is the canonical runtime entry
- `InitForwardRuntimeSystemConfig` normalizes the YAML contents and writes them into `v2_system_config`
- current app bootstrap still expects `jwt.secret`, `app.api_token`, `admin.*`, and database/cache values in `config/config.yaml`

If you use [`scripts/install.sh`](../scripts/install.sh) (release install), [`install.sh`](../install.sh), or [`panel_install.sh`](../panel_install.sh), those scripts generate `config/config.yaml` for you. If you skip the installer, copy the template and fill it manually.

## Fixed Entry Points

- Release install: [`docs/guide/release-installation.md`](guide/release-installation.md)
- Docker: [`docs/reference/quickstart.md`](reference/quickstart.md)
- Local dev: [`docs/reference/startup-config.md`](reference/startup-config.md)
- Runtime config migration: [`docs/reference/forward-runtime-migration.md`](reference/forward-runtime-migration.md)
- NodeX mode and runtime semantics: [`docs/reference/runtime.md`](reference/runtime.md)
- Verified relay proof and manual smoke: [`docs/guide/forward-tunnel-smoke-test.md`](guide/forward-tunnel-smoke-test.md)
- Relay onboarding and acceptance: [`docs/guide/forward-relay-onboarding.md`](guide/forward-relay-onboarding.md)

Current verified deployment truth:

- the currently proven dual-runtime path is `binary + SQLite + systemd`
- the verified single-UI runtime split is:
  - AnixOps Control UI on `3000`
  - AnixOps Control API on `8080`
  - NodeX control-plane on `18081`
  - relay gost API on `18080`
- Docker deployment docs remain important, but the full real-machine proof recorded today is not the Docker path yet

## Runtime Modes

`config/config.yaml.forward_runtime.backend` selects the forward execution
plane; startup persists it as the system config key `forward.runtime_backend`.

- `NodeX mode`
  - `forward_runtime.backend=gost`
  - requires `forward_runtime.nodex.base_url` and `forward_runtime.nodex.token`
  - stateful private runtime handoff to NodeX
- local Ansible mode (recommended: `nftables_ansible`)
  - `forward_runtime.backend=nftables_ansible`, configured under `forward_runtime.nftables_ansible`
  - stateless ansible/nftables execution on forward nodes
  - does not use proxy-node ingress semantics
  - `iptables_ansible` is a legacy value; the runtime normalizes it to `nftables_ansible`
- `clean_agent`
  - clean-room pull agent mode; see [`forward-clean-room/spec.md`](forward-clean-room/spec.md)

Keep the resource split explicit:
- `/admin/nodes` manages proxy nodes
- `/admin/forward/nodes` manages forward execution nodes

## Flux-panel

The `/admin/forward*` pages clone the upstream
[`flux-panel`](https://github.com/bqlpfy/flux-panel) forward/tunnel/user-tunnel
surface. Clone status and remaining gaps live in
[`guide/flux-panel-clone.md`](guide/flux-panel-clone.md); the endpoint and DTO
contract lives in [`guide/flux-forward-contract.md`](guide/flux-forward-contract.md).

## Repository Index

- [`AGENTS.md`](../AGENTS.md): contributor and agent guardrails
- [`docs/README.md`](README.md): documentation landing page
- [`docs/reference/repository-layout.md`](reference/repository-layout.md): root ownership and root hygiene rules
- [`docs/reference/configuration.md`](reference/configuration.md): config source-of-truth and key mapping
- [`config/deploy/ansible/README.md`](../config/deploy/ansible/README.md): ansible runtime assets

## Root Hygiene

The root should stay NodeX-like:
- short README
- source directories (including the self-contained `control-center/` app)
- deploy/config entrypoints
- no ad-hoc sample YAML or scratch logs

Generated artifacts belong under:
- `logs/`
- `test-reports/`
- ignored local-only paths such as `.codex_*.log` and `tmp_*.log`
