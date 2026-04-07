# Configuration

This page explains the minimum files and values you need to configure `v2board_AnixOps`.

## Source Of Truth

| Location | Role | Read by |
|------|------|------|
| [`config/config.yaml`](../../config/config.yaml) | bootstrap app config for server, database, cache, JWT, app token, admin | backend startup |
| [`.env`](../../.env) | Compose/install env file and `FORWARD_RUNTIME_*` source | Docker Compose, installer scripts, process environment |
| `v2_system_config` | persisted runtime/system config | runtime services and `/admin/system` |

Important:
- local `go run` does not auto-load [`.env`](../../.env)
- the backend does not automatically map `JWT_SECRET`, `API_TOKEN`, `DB_*`, or `REDIS_*` into the config object
- current env bootstrap is specific to `FORWARD_RUNTIME_*`, which are persisted into `v2_system_config`
- if you use the install scripts, they render [`config/config.yaml`](../../config/config.yaml) for you

## Primary Files

- root env file: [`.env`](../../.env)
- env example: [`.env.example`](../../.env.example)
- app config: [`config/config.yaml`](../../config/config.yaml)
- app config example: [`config/config.yaml.example`](../../config/config.yaml.example)
- compose entry: [`docker-compose.yml`](../../docker-compose.yml)

## Minimum Bootstrap Config

`config/config.yaml`:

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
  secret: "replace-with-32-char-secret"
  expire: 86400

admin:
  email: "admin@example.com"
  password: "replace-with-admin-password"
```

At minimum, fill:

- `jwt.secret`
- `app.api_token`
- `admin.email`
- `admin.password`

## `.env` Usage

Use [`.env`](../../.env) for:

- Docker host port mapping and timezone
- installer-driven config generation
- `FORWARD_RUNTIME_*` runtime bootstrap

Do not assume [`.env`](../../.env) replaces [`config/config.yaml`](../../config/config.yaml) for local startup.

## Minimum Docker Env For Runtime Bootstrap

`.env`:

```env
TZ=Asia/Shanghai
GIN_MODE=release
PANEL_FRONTEND_PORT=3000
PANEL_API_PORT=8080
PANEL_GRPC_PORT=50051
FORWARD_RUNTIME_NODEX_MODE=true
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080
FORWARD_RUNTIME_NODEX_TOKEN=replace-with-shared-token
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=15
```

If you use [`docker-compose.prod.yml`](../../docker-compose.prod.yml) or [`install.sh`](../../install.sh), keep `DB_*`, `REDIS_*`, `JWT_SECRET`, and `API_TOKEN` in sync with the final values written into [`config/config.yaml`](../../config/config.yaml).

## Forward Runtime Modes

### NodeX Mode

Use when you want the private stateful runtime.

Required values:

```env
FORWARD_RUNTIME_NODEX_MODE=true
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080
FORWARD_RUNTIME_NODEX_TOKEN=replace-with-shared-token
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=15
```

Behavior:

- `v2board` stores config and exposes the admin UI
- `NodeX` owns doctor, runtime status, and actual private execution
- the admin UI now uses `/api/v2/admin/forward/runtime/status` and `/api/v2/admin/forward/runtime/doctor` as control-plane proxies

### iptables_ansible Mode

Use when you want stateless forwarding execution without NodeX ingress/egress semantics.

Minimum example:

```env
FORWARD_RUNTIME_NODEX_MODE=false
FORWARD_RUNTIME_BACKEND=iptables_ansible
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON={"inventory":"/app/config/deploy/ansible/inventory.ini","playbookApply":"/app/config/deploy/ansible/playbooks/forward_apply.yml","playbookRemove":"/app/config/deploy/ansible/playbooks/forward_remove.yml","workingDir":"/app/config/deploy/ansible","targetPattern":"{{node.host}}","timeoutSeconds":120,"environment":{"ANSIBLE_CONFIG":"/app/config/deploy/ansible/ansible.cfg"}}
FORWARD_RUNTIME_ANSIBLE_HOST=198.51.100.10
FORWARD_RUNTIME_ANSIBLE_PORT=22
FORWARD_RUNTIME_ANSIBLE_USER=root
FORWARD_RUNTIME_ANSIBLE_PASSWORD=replace-with-password
```

Behavior:

- no NodeX ingress node selection is required
- only the forward execution node, SSH path, and playbook material matter
- this mode should stay operationally separate from proxy nodes under `/admin/nodes`

## Runtime Keys Written Into System Config

Startup initialization syncs runtime env values into these keys:

- `forward.runtime_backend`
- `forward.runtime.nodex_mode`
- `forward.runtime.nodex.base_url`
- `forward.runtime.nodex.token`
- `forward.runtime.nodex.timeout_seconds`
- `forward.runtime.iptables_ansible.config`

## Runtime Verification

After config is in place:

1. open `/admin/system`
2. verify the NodeX mode or ansible settings
3. use the NodeX Operator Console to run status and doctor
4. create a forward node under `/admin/forward/nodes`
5. create a tunnel under `/admin/forward/tunnel`
6. create a forward under `/admin/forward`

## Related Docs

- [`quickstart.md`](quickstart.md)
- [`runtime.md`](runtime.md)
- [`../guide/nodex-internal-extension.md`](../guide/nodex-internal-extension.md)
- [`../guide/forward-tunnel-runtime-ops.md`](../guide/forward-tunnel-runtime-ops.md)
