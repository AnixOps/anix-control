# AnixOps Control

`anix-control` is the public control-plane repository in the AnixOps stack.

It owns:
- public web UI and panel API
- user, plan, order, ticket, knowledge, and subscription data
- public proxy-node inventory and UniProxy-compatible APIs
- Flux-compatible `/admin/forward*` control pages

It does not own the private execution plane.

Boundary:
- `anix-control`: public control plane, persistence, and admin UI
- `anix-agent`: proxy-node and forwarding runtime
- `NodeX` and legacy clean-agent paths: compatibility runtimes being consolidated into AnixOps Agent

## Start Here

- [`docs/README.md`](docs/README.md): top-level docs entrypoint
- [`docs/intro/README.md`](docs/intro/README.md): repository role and deployment boundary
- [`docs/reference/README.md`](docs/reference/README.md): startup, config, repository layout, and runtime references
- [`docs/guide/README.md`](docs/guide/README.md): Flux-clone and implementation deep dives

## Exact Startup Truth

Before you try to start the panel, keep these rules straight:

- the backend always loads [`config/config.yaml`](config/config.yaml) through `-config`
- local `go run` does not auto-load [`.env`](.env)
- `config/config.yaml.forward_runtime` is the canonical runtime entry
- `InitForwardRuntimeSystemConfig` normalizes the YAML contents and writes them into `v2_system_config`
- current app bootstrap still expects `jwt.secret`, `app.api_token`, `admin.*`, and database/cache values in [`config/config.yaml`](config/config.yaml)

If you use [`install.sh`](install.sh) or [`panel_install.sh`](panel_install.sh), those scripts generate [`config/config.yaml`](config/config.yaml) for you. If you skip the installer, fill it manually.

## Fixed Entry Points

- Docker: [`docs/reference/quickstart.md`](docs/reference/quickstart.md)
- Local dev: [`docs/reference/startup-config.md`](docs/reference/startup-config.md)
- Runtime config migration: [`docs/reference/forward-runtime-migration.md`](docs/reference/forward-runtime-migration.md)
- NodeX mode and runtime semantics: [`docs/reference/runtime.md`](docs/reference/runtime.md)
- Verified relay proof and manual smoke: [`docs/guide/forward-tunnel-smoke-test.md`](docs/guide/forward-tunnel-smoke-test.md)
- Relay onboarding and acceptance: [`docs/guide/forward-relay-onboarding.md`](docs/guide/forward-relay-onboarding.md)
- Config examples: [`config/examples/README.md`](config/examples/README.md)

Current verified deployment truth:

- the currently proven dual-runtime path is `binary + SQLite + systemd`
- the verified single-UI runtime split is:
  - AnixOps Control UI on `3000`
  - AnixOps Control API on `8080`
  - NodeX control-plane on `18081`
  - relay gost API on `18080`
- Docker deployment docs remain important, but the full real-machine proof recorded today is not the Docker path yet

## Runtime Modes

- `NodeX mode`
  - `forward_runtime.backend=gost` via `config/config.yaml.forward_runtime`
  - requires `forward_runtime.nodex.base_url` and `forward_runtime.nodex.token`
  - stateful private runtime handoff to NodeX
- `iptables_ansible mode`
  - `forward_runtime.backend=iptables_ansible` via `config/config.yaml.forward_runtime`
  - stateless ansible/iptables execution on forward nodes
  - does not use proxy-node ingress semantics

Keep the resource split explicit:
- `/admin/nodes` manages proxy nodes
- `/admin/forward/nodes` manages forward execution nodes

## Repository Index

- [`AGENTS.md`](AGENTS.md): contributor and agent guardrails
- [`docs/README.md`](docs/README.md): documentation landing page
- [`docs/reference/repository-layout.md`](docs/reference/repository-layout.md): root ownership and root hygiene rules
- [`docs/reference/configuration.md`](docs/reference/configuration.md): config source-of-truth and key mapping
- [`config/examples/README.md`](config/examples/README.md): sample YAML inputs and helper commands
- [`config/deploy/ansible/README.md`](config/deploy/ansible/README.md): ansible runtime assets

## Root Hygiene

The root should stay NodeX-like:
- short README
- source directories
- deploy/config entrypoints
- no ad-hoc sample YAML or scratch logs

Generated artifacts belong under:
- `logs/`
- `test-reports/`
- `config/examples/`
- ignored local-only paths such as `.codex_*.log` and `tmp_*.log`
