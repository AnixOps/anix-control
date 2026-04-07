# V2Board Intro

Use this section first when you need to understand what `v2board_AnixOps` owns and how it should be started.

## Repository Role

`v2board_AnixOps` is the public-facing control plane in the three-repository stack:

- browser/admin UI
- panel API
- user, plan, order, knowledge, ticket, and subscription data
- public proxy-node inventory and UniProxy-compatible APIs
- Flux-compatible `/admin/forward*` control pages

It is not the private execution plane.

Execution ownership:
- `NodeX`: internal-only runtime execution, doctor/version tooling, operator workflows
- `v2board_AnixOps`: persistent control-plane state and admin UI
- `V2bX_AnixOps`: proxy-node runtime and traffic reporting

## Startup Paths

There are two normal ways to start `v2board`:

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
  - used by Docker Compose, install scripts, and env-seeded runtime config
  - especially important for `FORWARD_RUNTIME_*`
- `v2_system_config`
  - runtime-persisted values shown in `/admin/system`
  - seeded from env on startup by `InitForwardRuntimeSystemConfigFromEnv`

Important:
- local `go run` does not automatically read `.env`
- if you start locally without Docker, export the env vars in the shell first

## Runtime Modes

Forward runtime has two distinct modes:

- NodeX mode
  - `forward.runtime_backend = gost`
  - requires `forward.runtime.nodex.base_url` and `forward.runtime.nodex.token`
  - stateful runtime handoff to NodeX
- `iptables_ansible` mode
  - `forward.runtime_backend = iptables_ansible`
  - stateless ansible-driven runtime
  - uses execution-node SSH/inventory/playbook material

Do not mix proxy-node language and forward-node language.

- `Node` under `/admin/nodes` is the public proxy-node concept.
- `ForwardNode` under `/admin/forward/nodes` is the forward execution concept.

## Read Next

- [`../reference/startup-config.md`](../reference/startup-config.md)
- [`../reference/repository-layout.md`](../reference/repository-layout.md)
- [`../guide/nodex-internal-extension.md`](../guide/nodex-internal-extension.md)
