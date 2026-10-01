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

## v4 Architecture At A Glance

`anix-control` v4 is a plugin kernel with a compatibility bridge. The
important parts, in request order:

- **Kernel** (`cmd/server`, `internal/`): gin HTTP server, GORM persistence
  (SQLite default, PostgreSQL supported), JWT/admin middleware, background
  workers, and the gRPC listener for nodes.
- **`/api/v2` package gateway**: every `/api/v2` business route is registered
  through `registeredPackageRoute` in `internal/router/router.go`. A request
  goes to the package gateway, which forwards it to the host process of the
  signed package that owns the route (see
  `config/v2-package-route-catalog.json`). There is no request-time fallback
  when that package is missing, disabled, or unhealthy.
- **Bridge back into the kernel**: package hosts call narrowly scoped,
  kernel-owned bridge operations (`internal/packagebridge`,
  `internal/identitybridge`) that still execute the established in-kernel gin
  handlers and services. The legacy business domains have not moved out of
  the kernel yet (see
  [`../architecture/release-line-status.md`](../architecture/release-line-status.md)).
- **Routes outside the package gate**: `/api/v1/server/UniProxy/*`,
  `/{subscribe_path}/:token` (default `/s/:token`), `/api/v1/client/subscribe`,
  and `/flow/upload` are still served directly by kernel handlers.
- **`/api/v3` kernel API**: plugin catalog, releases and artifacts,
  installations and revisioned configuration, lifecycle operations,
  node assignments, topologies, deployments, access groups, resource grants,
  and quota policies. `/api/v4/plugins/:plugin_id/*` exposes the package route
  gateway directly to administrators.
- **Packages** (`packages/*`): sixteen official signed packages plus
  `packages/shared`. Package host processes use `sdk/pluginhostsdk` to serve
  the host protocol (`sdk/api/pluginhost/v1`) and `sdk/packagebridgesdk` to call
  the bridge protocol (`sdk/api/packagebridge/v1`).
- **Nodes**: the node runtime is `anix-agent` (separate repository
  `AnixOps/anix-agent`). Control imports no anix-agent module; it owns the
  Agent contract in its SDK module (`sdk/api/agent/v1`) and talks to agents
  over gRPC: the `anix.agent.v1` control stream for signed package lifecycle
  work, plus the legacy `v2board` panel-node services and UniProxy HTTP for
  compatibility.
- **Control Center** (`control-center/`): a separate Go module, web, Flutter,
  and Cloudflare Workers app that manages plugins through `/api/v2/login` and
  `/api/v3`.

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

Forward runtime has these distinct modes:

- NodeX mode
  - `forward.runtime_backend = gost`
  - requires `forward.runtime.nodex.base_url` and `forward.runtime.nodex.token`
  - stateful runtime handoff to NodeX
- local Ansible mode (recommended: `nftables_ansible`)
  - `forward.runtime_backend = nftables_ansible`
  - stateless ansible-driven runtime on the panel host
  - uses the local executor plus execution-node SSH/inventory/playbook material
  - `iptables_ansible` is a legacy value; the runtime normalizes it to `nftables_ansible`
- clean-room agent mode
  - `forward.runtime_backend = clean_agent`
  - pull-based clean-room forward agent; see [`../forward-clean-room/spec.md`](../forward-clean-room/spec.md)

Do not mix proxy-node language and forward-node language.

- `Node` under `/admin/nodes` is the public proxy-node concept.
- `ForwardNode` under `/admin/forward/nodes` is the forward execution concept.

## Read Next

- [`../reference/startup-config.md`](../reference/startup-config.md)
- [`../reference/repository-layout.md`](../reference/repository-layout.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/nodex-internal-extension.md`](../guide/nodex-internal-extension.md)
