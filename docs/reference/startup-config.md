# Startup And Config

This page describes the exact startup flow after the unified runtime config update.

## 1. Ownership Model

| Location | Purpose |
|------|------|
| [`config/config.yaml`](../../config/config.yaml) | canonical startup config for app settings and `forward_runtime` |
| [`.env`](../../.env) | optional Docker or installer env file |
| `v2_system_config` | persisted merged runtime snapshot used by runtime services |

Important:

- backend startup always begins from [`config/config.yaml`](../../config/config.yaml)
- local `go run` does not auto-load [`.env`](../../.env)
- runtime services read the forward runtime values that were written into `v2_system_config`

## 2. Minimal Local Config

Copy the template first:

```powershell
Copy-Item config/config.yaml.example config/config.yaml
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
  version: "4.0.0-alpha.1"
  api_token: "replace-with-node-api-token"
  traffic_log_enable: true
  subscribe_path: "s"

plugins:
  official_public_key: "IaqXgif/OGydNv/mQHoyFmqOvzeplICaMZndrhqMG0M="
  control_execution_enabled: true
  control_poll_interval: "5s"
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

This is the `4.0.0-alpha.1` signed-package profile. The pinned value is the raw
32-byte AnixOps Ed25519 release public key encoded as Base64. It is public trust
material, not a private signing key. The loopback gRPC bind makes local
Control/Agent acceptance reproducible while preventing an unauthenticated
network listener from appearing during installation.

An Agent keeps `Transport: "http"` for the existing configuration, user, and
traffic data plane, then independently opts into package operations with
`AgentControlEnabled` and `PluginSupervisorEnabled`. Do not enable a remote
Agent by changing `grpc.host` alone: configure server TLS or an HTTP/2 gRPC
proxy and firewall first.

## 3. Local Binary Startup

Backend:

```powershell
go run .\cmd\server\main.go -config .\config\config.yaml
```

Frontend:

```powershell
Set-Location .\web
npm install
npm run dev
```

Default local URLs:

- UI: `http://127.0.0.1:3000`
- API: `http://127.0.0.1:8080`
- health: `http://127.0.0.1:8080/health`

## 4. Docker Startup

Prepare files:

```powershell
Copy-Item .env.example .env
Copy-Item config/config.yaml.example config/config.yaml
```

Then:

1. fill `config/config.yaml`
2. optionally customize `.env` for deployment-specific values that are unrelated to runtime selection
3. start containers

```powershell
docker compose up -d
docker compose logs -f anix-control
```

## 5. What Happens On Startup

The runtime flow is now:

1. resolve `config/config.yaml`
2. load app config and `forward_runtime`
3. normalize sqlite path, frontend path, and ansible runtime paths
4. initialize database
5. run `InitForwardRuntimeSystemConfig`
6. persist the parsed runtime snapshot into `v2_system_config`

When the alpha plugin profile is enabled, startup also exposes the signed
package APIs, starts the durable Control lifecycle worker, and dispatches ready
operations to authenticated Agent control streams. Topology execution remains
off until a separate canary decision.

That is why local startup and Docker startup now share the same primary config structure.

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
