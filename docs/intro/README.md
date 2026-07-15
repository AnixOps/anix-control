# AnixOps Control Intro

Use this section first when you need to understand what `anix-control` owns and how it should be started.

## Repository Role

`anix-control` is the public-facing control plane in the AnixOps stack:

- browser/admin UI
- panel API
- user, plan, order, knowledge, ticket, and subscription data
- public proxy-node inventory and UniProxy-compatible APIs
- Flux-compatible `/admin/forward*` control pages

It is not the private execution plane.

Execution ownership:
- `anix-control`: persistent control-plane state, API, and admin UI
- `anix-agent`: proxy protocols, forwarding execution, diagnostics, and traffic reporting
- `NodeX` and the clean forward agent: compatibility runtimes being absorbed into AnixOps Agent

## Startup Paths

There are two normal ways to start `anix-control`:

1. Docker Compose
2. local backend + local frontend

Use [`../reference/startup-config.md`](../reference/startup-config.md) for the exact commands and config snippets.

## Which Config Lives Where

This is the main source of confusion and should stay explicit:

- `config/config.yaml`
  - required for app bootstrap
  - controls server, frontend, database, cache, jwt, admin
- `.env`
  - deployment-time variables
  - used by Docker Compose and install scripts
- `v2_system_config`
  - runtime-persisted values shown in `/admin/system`
  - seeded from `config/config.yaml.forward_runtime` on startup by `InitForwardRuntimeSystemConfig`

Important:
- local `go run` does not automatically read `.env`
- if you start locally without Docker, edit `config/config.yaml` directly before startup

## Runtime Modes

Forward runtime has two distinct modes:

- NodeX mode
  - `forward.runtime_backend = gost`
  - requires `forward.runtime.nodex.base_url` and `forward.runtime.nodex.token`
  - stateful runtime handoff to NodeX
- local Ansible mode (recommended: `nftables_ansible`)
  - `forward.runtime_backend = nftables_ansible`
  - stateless ansible-driven runtime on the panel host
  - uses the local executor plus execution-node SSH/inventory/playbook material
  - `iptables_ansible` remains available only as legacy compatibility for older relay playbooks/firewall environments

Do not mix proxy-node language and forward-node language.

- `Node` under `/admin/nodes` is the public proxy-node concept.
- `ForwardNode` under `/admin/forward/nodes` is the forward execution concept.

## Read Next

- [`../reference/startup-config.md`](../reference/startup-config.md)
- [`../reference/repository-layout.md`](../reference/repository-layout.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/nodex-internal-extension.md`](../guide/nodex-internal-extension.md)
