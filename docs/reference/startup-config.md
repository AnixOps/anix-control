# Startup And Config

This page describes the exact startup flow after the unified runtime config update.

## 1. Ownership Model

| Location | Purpose |
|------|------|
| `config/config.yaml` (template: [`config/config.yaml.example`](../../config/config.yaml.example)) | canonical startup config for app settings and `forward_runtime` |
| `ANIX_CONTROL_*` environment variables | per-key overrides and secrets ([configuration.md](configuration.md#environment-variables)) |
| `.env` (template: [`.env.example`](../../.env.example)) | optional Docker or installer env file |
| `v2_system_config` | persisted merged runtime snapshot used by runtime services |

Important:

- backend startup reads `-config`, else `ANIX_CONTROL_CONFIG`, else
  `config/config.yaml` when it exists; an explicitly named file must exist
- with no config file, startup uses the built-in container defaults; `ANIX_CONTROL_*`
  variables override either source
- local `go run` does not auto-load `.env`
- runtime services read the forward runtime values that were written into `v2_system_config`

## 2. Minimal Local Config

Copy the template first:

```bash
cp config/config.yaml.example config/config.yaml
```

Minimum sqlite example:

```yaml
env: "development"

server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

frontend:
  enable: true
  port: 3000
  path: "web/public"

database:
  driver: "sqlite"
  database: "config/data/v2board.db"

cache:
  driver: "memory"

jwt:
  secret: "replace-with-at-least-32-characters"
  expire: 86400

app:
  name: "AnixOps Control"
  version: "4.0.0"
  api_token: "replace-with-node-api-token"
  traffic_log_enable: true
  subscribe_path: "s"

plugins:
  official_public_key: "lvbhRmhzVbSAbrw3vm0k7vYqpEu4/dF/ZqVbp2gS7uM="
  # Required for a new plugin-only database before package execution is enabled.
  identity_bootstrap_package_dir: "/var/lib/anixops/bootstrap"
  control_execution_enabled: true
  control_poll_interval: "5s"
  # Use paths writable by the service user. The V4 installer creates these
  # under its selected installation root with mode 0700.
  control_host_runtime_dir: "/opt/anixops/control/runtime/plugin-hosts"
  control_host_artifact_dir: "/opt/anixops/control/data/plugin-artifacts"
  dispatch_enabled: true
  dispatch_poll_interval: "5s"
  topology_execution_enabled: false
  topology_poll_interval: "5s"

grpc:
  enabled: true
  host: "127.0.0.1"
  port: 50051
  api_token: ""
  tls_cert_file: ""
  tls_key_file: ""

admin:
  email: "admin@example.com"
  # Leave empty to generate and print a one-time random bootstrap password.
  password: ""

forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

This is the `4.0.0` signed-package profile. The pinned value is the raw 32-byte
AnixOps Ed25519 release public key encoded as Base64. It is public trust
material, not a private signing key. The V4 release installer fetches and
checksum-verifies the identity trio automatically, then stages it in a
root-owned `0750` directory with group-readable `0640` files. For a manual
deployment, place exactly one verified `identity-platform-<version>.anxp`,
`identity-platform-<version>.manifest.json`, and
`identity-platform-<version>.manifest.sig` trio in the configured absolute
directory with the same ownership and mode requirements before starting
Control. The bootstrap import is root-pinned and idempotent. When package
execution is enabled manually, create the configured Control host runtime and
artifact directories as non-symlink `0700` directories owned by the Control
service user. The loopback gRPC bind makes local Control/Agent acceptance
reproducible while preventing an unauthenticated network listener from
appearing during installation.

An Agent keeps `Transport: "http"` for the existing configuration, user, and
traffic data plane, then independently opts into package operations with
`AgentControlEnabled` and `PluginSupervisorEnabled`. Do not enable a remote
Agent by changing `grpc.host` alone: configure server TLS or an HTTP/2 gRPC
proxy and firewall first.

## 3. Local Binary Startup

Backend:

```bash
GOWORK=off go run ./cmd/server -config config/config.yaml
```

Frontend:

```bash
cd web
npm ci
npm run dev
```

Default local URLs:

- UI: `http://127.0.0.1:3000`
- API: `http://127.0.0.1:8080`
- health: `http://127.0.0.1:8080/health`

## 4. Docker Startup

Development (builds this checkout and runs a throwaway PostgreSQL):

```bash
docker compose up -d --build
docker compose logs migrate   # generated admin password on the first run
```

Production uses `docker-compose.prod.yml` with a released image digest, an
external PostgreSQL, `control.env` and `secrets/`; see
[`../DEPLOYMENT.md`](../DEPLOYMENT.md). The containers need no config file: the
built-in defaults plus `ANIX_CONTROL_*` variables configure them. To keep a
YAML file instead, mount it and set `ANIX_CONTROL_CONFIG`.

## 5. What Happens On Startup

The runtime flow is now:

1. resolve the config file (or use the built-in defaults) and apply `ANIX_CONTROL_*` variables
2. load app config and `forward_runtime`
3. normalize sqlite path, frontend path, and ansible runtime paths
4. initialize database
5. prepare the database (below), including `InitForwardRuntimeSystemConfig`
6. persist the parsed runtime snapshot into `v2_system_config`

Database preparation runs under a PostgreSQL advisory lock, so several Control
processes starting together (a rolling update, or `migrate` next to a server)
run it one at a time:

- `env: development` / `test`: full `AutoMigrate` of the application models.
- other environments: an empty database gets the full schema; an existing one
  only gets tables it does not have. Existing tables, columns and indexes are
  never altered.
- then the `Ensure*` schema helpers, the plugin trust root, the identity
  package bootstrap import, and the default admin, subscription group, plan
  and authorized key seeds.

`anix-control migrate [flags]` runs only this preparation and exits (status 0
on success). Use it as a one-shot Compose service or Kubernetes Job before the
server starts; the server repeats it idempotently.

When the alpha plugin profile is enabled, startup also exposes the signed
package APIs, starts the durable Control lifecycle worker, and dispatches ready
operations to authenticated Agent control streams. Topology execution remains
off until a separate canary decision.

That is why local startup and Docker startup now share the same primary config structure.

Invalid `plugins.*_poll_interval` values, or dispatch/topology flags without
their prerequisites, stop startup before any listener opens. Once the gRPC
listener is up, a later failure (for example the API port already in use) runs
the normal shutdown below and exits with status 1.

On SIGINT or SIGTERM, Control shuts down in this order: `/readyz` and
`/health` return 503 (see below), HTTP servers drain for up to 30s, background
workers are cancelled and awaited for up to 15s, the gRPC server stops (up to
10s), Control plugin hosts stop (up to 15s), then the cache and database close.
A clean shutdown exits with status 0. A second signal forces an immediate exit
with status 1.

### Probes And Shutdown

Both the API server (`server.port`) and the UI server (`frontend.port`) answer:

| Path | Meaning |
|------|------|
| `/livez` | the process answers HTTP; use as the liveness probe |
| `/readyz` | startup finished, shutdown has not begun, and the database answers a ping (2 s bound); use as the readiness probe and Docker `HEALTHCHECK` |
| `/health` | legacy check: `{"status":"ok"}`, or `503 {"status":"draining"}` during shutdown |

On `SIGTERM` the server marks itself draining (`/readyz` and `/health` fail,
`/livez` keeps succeeding), keeps serving for `server.shutdown_drain_delay`
(default `0`; `5s` in the built-in container defaults) so load balancers stop
routing to it, then closes the listeners and drains in-flight requests (30 s),
stops background workers (15 s), gRPC (10 s) and plugin hosts (15 s). Allow at
least 90 s of termination grace (`stop_grace_period`,
`terminationGracePeriodSeconds`).

## 6. Runtime Tuning

If you need to change runtime behavior, edit `config/config.yaml.forward_runtime` before startup. That now includes:

- backend selection under `forward_runtime.backend`
- NodeX or local ansible runtime details under `forward_runtime.nodex`, `forward_runtime.nftables_ansible`, and the legacy-compatible `forward_runtime.iptables_ansible`
- local worker tuning under `forward_runtime.jobs` and `forward_runtime.gost_stats`

Any downstream services will read whichever values were persisted into `v2_system_config` when the backend initialized.

## 7. Validation Checklist

After startup, verify:

1. `GET /health` returns `200`
2. `/admin/system` shows the expected merged runtime config
3. `/admin/forward`
4. `/admin/forward/tunnel`
5. `/admin/forward/ansible-machines` for stateless execution hosts
6. `/admin/forward/nodes` for NodeX relay/exit topology
7. `GET /api/v2/admin/forward/runtime/status` works in NodeX mode
8. `GET /api/v2/admin/forward/runtime/doctor` works when NodeX is reachable
9. `/admin/control` loads the official package catalog and installation state
10. an Agent using the same official public key reports an active control stream

For a production-template rehearsal, verify the opposite before adding secrets:
`control_execution_enabled`, `dispatch_enabled`, `topology_execution_enabled`,
and `grpc.enabled` must all remain `false`. The production template contains no
default node API token, gRPC fallback token, or NodeX shared token.

## 8. Related Docs

- [`configuration.md`](configuration.md)
- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
