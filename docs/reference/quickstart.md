# Quickstart

This page is the shortest path to getting `v2board_AnixOps` running.

## What The Backend Actually Reads

Always prepare these inputs first:

- [`config/config.yaml`](../../config/config.yaml)
  - authoritative bootstrap config for server, database, cache, JWT, app token, and admin account
- [`.env`](../../.env)
  - Compose and installer env file
  - also used for `FORWARD_RUNTIME_*` when you want runtime bootstrap

Important:
- local `go run` does not auto-load `.env`
- only `FORWARD_RUNTIME_*` are imported from the process environment into `v2_system_config`
- if you skip the installer, you must fill [`config/config.yaml`](../../config/config.yaml) yourself

## Docker Quickstart

1. Prepare files:

```powershell
Copy-Item .env.example .env
Copy-Item config\config.yaml.example config\config.yaml
```

2. Edit `config/config.yaml` at minimum:

- `jwt.secret`
- `app.api_token`
- `admin.email`
- `admin.password`
- database/cache values if you are not using the default sqlite + memory path

3. If you want NodeX mode, also set in `.env`:

- `FORWARD_RUNTIME_NODEX_MODE=true`
- `FORWARD_RUNTIME_BACKEND=gost`
- `FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080`
- `FORWARD_RUNTIME_NODEX_TOKEN=replace-with-shared-token`
- `FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=15`

4. Start:

```powershell
docker compose up -d
docker compose logs -f v2board
```

5. Open:

- panel API: `http://127.0.0.1:8080`
- admin/user frontend: `http://127.0.0.1:3000`

## Local Development

1. Prepare `config/config.yaml`:

```powershell
Copy-Item config\config.yaml.example config\config.yaml
```

2. Fill `jwt.secret`, `app.api_token`, and `admin.password`.

3. If you need runtime bootstrap, export `FORWARD_RUNTIME_*` in the shell first.

4. Start backend:

```powershell
go mod download
go run .\cmd\server\main.go -config .\config\config.yaml
```

5. Start frontend:

```powershell
Set-Location web
npm install
npm run dev
```

## Minimum NodeX Mode Setup

`v2board` only acts as the public control plane. For actual private forward runtime execution, set:

```env
FORWARD_RUNTIME_NODEX_MODE=true
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080
FORWARD_RUNTIME_NODEX_TOKEN=replace-with-shared-token
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=15
```

After container or app startup, these values are synced into system config keys:

- `forward.runtime_backend`
- `forward.runtime.nodex_mode`
- `forward.runtime.nodex.base_url`
- `forward.runtime.nodex.token`
- `forward.runtime.nodex.timeout_seconds`

In the admin UI, check:

- `/admin/system` -> NodeX mode enabled
- `/admin/system` -> NodeX Operator Console
- `/admin/forward`
- `/admin/forward/tunnel`
- `/admin/forward/nodes`

## Minimum iptables_ansible Mode Setup

```env
FORWARD_RUNTIME_NODEX_MODE=false
FORWARD_RUNTIME_BACKEND=iptables_ansible
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON={"inventory":"/app/config/deploy/ansible/inventory.ini","playbookApply":"/app/config/deploy/ansible/playbooks/forward_apply.yml","playbookRemove":"/app/config/deploy/ansible/playbooks/forward_remove.yml","workingDir":"/app/config/deploy/ansible","targetPattern":"{{node.host}}","timeoutSeconds":120,"environment":{"ANSIBLE_CONFIG":"/app/config/deploy/ansible/ansible.cfg"}}
FORWARD_RUNTIME_ANSIBLE_HOST=198.51.100.10
FORWARD_RUNTIME_ANSIBLE_USER=root
FORWARD_RUNTIME_ANSIBLE_PASSWORD=replace-with-password
```

Use this path when you do not want NodeX stateful ingress/egress orchestration and only need stateless forwarding execution on relay hosts.

## What To Read Next

- [`configuration.md`](configuration.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-tunnel-runtime-ops.md`](../guide/forward-tunnel-runtime-ops.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
