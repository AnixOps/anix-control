# Startup And Config

This page gives the exact startup flow for `v2board_AnixOps` and clarifies how `config/config.yaml`, `.env`, and runtime system config interact.

## 1. Config Ownership

| Location | What it controls | When it is read |
|------|------|------|
| `config/config.yaml` | app bootstrap: server, frontend, database, cache, jwt, admin | always on backend startup |
| `.env` | Docker Compose and install/runtime seed vars | when Compose/scripts load it, or when you export values manually |
| `v2_system_config` | persisted runtime settings such as `forward.runtime.*` | read by services and `/admin/system` after startup |

Important:
- `go run .\cmd\server\main.go` does **not** auto-load `.env`
- Docker Compose does load `.env`
- current backend bootstrap still reads `jwt.secret`, `app.api_token`, database, cache, and admin values from `config/config.yaml`
- current environment import during backend startup is specific to `FORWARD_RUNTIME_*`
- `InitForwardRuntimeSystemConfigFromEnv` seeds `v2_system_config` from `FORWARD_RUNTIME_*` during backend startup

## 2. Minimal `config/config.yaml`

Copy the template first:

```powershell
Copy-Item config/config.yaml.example config/config.yaml
```

Minimal sqlite example:

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
  version: "2.0.0"
  api_token: "replace-with-node-api-token"
  traffic_log_enable: true
  subscribe_path: "s"

admin:
  email: "admin@example.com"
  password: "replace-me"
```

## 3. Docker Compose Startup

1. Prepare files:

```powershell
Copy-Item .env.example .env
Copy-Item config/config.yaml.example config/config.yaml
```

2. Fill `config/config.yaml` at minimum:
- `jwt.secret`
- `app.api_token`
- `admin.email`
- `admin.password`

3. If you want runtime bootstrap, fill `.env`:
- NodeX mode:
  - `FORWARD_RUNTIME_NODEX_MODE=true`
  - `FORWARD_RUNTIME_BACKEND=gost`
  - `FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080`
  - `FORWARD_RUNTIME_NODEX_TOKEN=...`
- or stateless mode:
  - `FORWARD_RUNTIME_NODEX_MODE=false`
  - `FORWARD_RUNTIME_BACKEND=iptables_ansible`

4. Start:

```powershell
docker compose up -d
docker compose logs -f v2board
```

5. Open:
- UI: `http://127.0.0.1:3000`
- API health: `http://127.0.0.1:8080/health`

## 4. Local Startup Without Docker

1. Prepare `config/config.yaml`
2. Fill `jwt.secret`, `app.api_token`, and `admin.password`
3. Export runtime env vars if you need forward runtime bootstrap
4. Start backend:

```powershell
go run .\cmd\server\main.go -config .\config\config.yaml
```

5. Start frontend:

```powershell
Set-Location .\web
npm install
npm run dev
```

## 5. Local Startup With NodeX Mode

Before `go run`, export:

```powershell
$env:FORWARD_RUNTIME_NODEX_MODE = 'true'
$env:FORWARD_RUNTIME_BACKEND = 'gost'
$env:FORWARD_RUNTIME_NODEX_BASE_URL = 'http://127.0.0.1:18080'
$env:FORWARD_RUNTIME_NODEX_TOKEN = 'replace-with-your-token'
$env:FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS = '15'
go run .\cmd\server\main.go -config .\config\config.yaml
```

What happens on startup:

1. `config/config.yaml` is loaded
2. database is initialized
3. `InitForwardRuntimeSystemConfigFromEnv` reads `FORWARD_RUNTIME_*`
4. values are written to `v2_system_config`
5. `/admin/system` shows the persisted values

That is why local runtime settings still "work" without Docker:
- not because `.env` is auto-read
- but because the process environment is read at startup and persisted into the database

## 6. Local Startup With `iptables_ansible`

Example shell exports:

```powershell
$env:FORWARD_RUNTIME_NODEX_MODE = 'false'
$env:FORWARD_RUNTIME_BACKEND = 'iptables_ansible'
$env:FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON = '{"inventory":"config/deploy/ansible/inventory.ini","playbookApply":"config/deploy/ansible/playbooks/forward_apply.yml","playbookRemove":"config/deploy/ansible/playbooks/forward_remove.yml","workingDir":"config/deploy/ansible","targetPattern":"{{node.host}}","timeoutSeconds":120,"environment":{"ANSIBLE_CONFIG":"config/deploy/ansible/ansible.cfg"}}'
go run .\cmd\server\main.go -config .\config\config.yaml
```

Required assets already live here:
- `config/deploy/ansible/ansible.cfg`
- `config/deploy/ansible/inventory.ini.example`
- `config/deploy/ansible/playbooks/forward_apply.yml`
- `config/deploy/ansible/playbooks/forward_remove.yml`

## 7. Where To Edit Later

After initial boot:

- use `/admin/system` to inspect or change `forward.runtime.*`
- use `/admin/system` -> `NodeX Operator Console` to query:
  - runtime status
  - doctor output
  - operator commands

## 8. Validation Checklist

After startup, verify:

1. `GET /health` returns `200`
2. `/admin/system` shows the expected runtime backend and NodeX values
3. `GET /api/v2/admin/forward/runtime/status` succeeds when NodeX mode is configured
4. `GET /api/v2/admin/forward/runtime/doctor` returns health + runtime summary
5. `/admin/forward`, `/admin/forward/tunnel`, and `/admin/forward/nodes` load normally
6. proxy nodes under `/admin/nodes` and forward nodes under `/admin/forward/nodes` stay clearly separated
