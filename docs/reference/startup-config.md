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
  name: "V2Board"
  version: "2.0.1"
  api_token: "replace-with-node-api-token"
  traffic_log_enable: true
  subscribe_path: "s"

admin:
  email: "admin@example.com"
  password: "replace-me"

forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

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
docker compose logs -f v2board
```

## 5. What Happens On Startup

The runtime flow is now:

1. resolve `config/config.yaml`
2. load app config and `forward_runtime`
3. normalize sqlite path, frontend path, and ansible runtime paths
4. initialize database
5. run `InitForwardRuntimeSystemConfig`
6. persist the parsed runtime snapshot into `v2_system_config`

That is why local startup and Docker startup now share the same primary config structure.

## 6. Runtime Tuning

If you need to change runtime behavior, edit `config/config.yaml.forward_runtime` before startup. That now includes:

- backend selection under `forward_runtime.backend`
- NodeX or ansible runtime details under `forward_runtime.nodex` and `forward_runtime.iptables_ansible`
- local worker tuning under `forward_runtime.jobs` and `forward_runtime.gost_stats`

Any downstream services will read whichever values were persisted into `v2_system_config` when the backend initialized.

## 7. Validation Checklist

After startup, verify:

1. `GET /health` returns `200`
2. `/admin/system` shows the expected merged runtime config
3. `/admin/forward`
4. `/admin/forward/tunnel`
5. `/admin/forward/nodes`
6. `GET /api/v2/admin/forward/runtime/status` works in NodeX mode
7. `GET /api/v2/admin/forward/runtime/doctor` works when NodeX is reachable

## 8. Related Docs

- [`configuration.md`](configuration.md)
- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
