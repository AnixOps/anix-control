# Quickstart

This page is the shortest path to getting `anix-control` running with the unified config model.

## What The Backend Actually Reads

Always prepare these inputs first:

- `config/config.yaml` (local, gitignored; copy from [`config/config.yaml.example`](../../config/config.yaml.example))
  - canonical bootstrap config for server, database, cache, JWT, admin, and `forward_runtime`
- `.env` (local, gitignored; copy from [`.env.example`](../../.env.example))
  - optional Docker Compose and installer env file
  - use it for deployment-specific ports or metadata; runtime selection comes from `config/config.yaml.forward_runtime`

Important:

- local `go run` does not auto-load `.env`
- `config/config.yaml` is the primary source of truth
- runtime selection is defined in `config/config.yaml.forward_runtime`
- merged runtime values are persisted into `v2_system_config`
- no reusable administrator email or password is shipped with Control; set unique values in `admin.email` and `admin.password` (or the installer's corresponding flags) before first start

## Docker Quickstart

Development stack (builds this checkout, runs a throwaway PostgreSQL, no config
file needed):

```bash
docker compose up -d --build
docker compose logs migrate        # prints the generated admin password once
docker compose ps                  # control becomes healthy
```

Open:

- panel API: `http://127.0.0.1:8080` (`/readyz` for readiness)
- admin or user frontend: `http://127.0.0.1:3000`

Settings are `ANIX_CONTROL_*` environment variables in `docker-compose.yml`
(`docker compose run --rm control -print-env` lists all of them). For example,
NodeX mode:

```yaml
    environment:
      ANIX_CONTROL_FORWARD_RUNTIME_BACKEND: gost
      ANIX_CONTROL_FORWARD_RUNTIME_NODEX_BASE_URL: http://nodex-control-plane:18081
      ANIX_CONTROL_FORWARD_RUNTIME_NODEX_TOKEN: replace-with-shared-token
```

Map-valued keys such as `forward_runtime.nftables_ansible.environment` can only
be set in a YAML file: mount one and set `ANIX_CONTROL_CONFIG`. Production
deployments use `docker-compose.prod.yml` with a released image; see
[`../DEPLOYMENT.md`](../DEPLOYMENT.md).

## Local Development

1. Prepare `config/config.yaml`:

```bash
cp config/config.yaml.example config/config.yaml
```

2. Fill `jwt.secret`, `app.api_token`, `admin.password`, and `forward_runtime`.

3. Start backend:

```bash
GOWORK=off go mod download
GOWORK=off go run ./cmd/server -config config/config.yaml
```

For an isolated development instance with its own ports and SQLite database, use `make run` instead (see the root [`README.md`](../../README.md)).

4. Start frontend:

```bash
cd web
npm ci
npm run dev
```

## Runtime Overrides

Adjust runtime behavior by editing `config/config.yaml.forward_runtime` and restarting the backend. The `.env` file remains reserved for ports, secret tokens, and ancillary deployment flags rather than runtime selection.

## Minimum NodeX Mode Setup

Use this in `config/config.yaml`:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

Current verified single-UI port split:

- AnixOps Control API: `8080`
- AnixOps Control UI: `3000`
- NodeX control-plane: `18081`
- relay gost API: `18080`

## Minimum Local Ansible Mode Setup

Use this in `config/config.yaml`:

```yaml
forward_runtime:
  backend: "nftables_ansible"
  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
```

`nftables_ansible` is the recommended backend. `iptables_ansible` remains available only for legacy relay hosts that still need the old playbooks and firewall driver.

If you prefer password auth instead of SSH keys, keep the runtime config in `config/config.yaml.forward_runtime` and keep inventory entries or extra vars updated with the relay credentials. Do not rely on `FORWARD_RUNTIME_*` for configuring runtime access.

## Current Proven Deployment Path

The current end-to-end proof in this repository is:

- `binary + SQLite + systemd`
- real `nftables_ansible` or legacy `iptables_ansible` relay rules, depending on the target host firewall stack
- real `NodeX/gost` relay services

Docker remains supported for startup and future deployment work, but the recorded dual-runtime proof today is not the Docker path.

## What To Read Next

- [`configuration.md`](configuration.md)
- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`startup-config.md`](startup-config.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-runtime-ops.md`](../guide/forward-tunnel-runtime-ops.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
