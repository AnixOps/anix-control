# Quickstart

This page is the shortest path to getting `v2board_AnixOps` running with the unified config model.

## What The Backend Actually Reads

Always prepare these inputs first:

- [`config/config.yaml`](../../config/config.yaml)
  - canonical bootstrap config for server, database, cache, JWT, admin, and `forward_runtime`
- [`.env`](../../.env)
  - optional Docker Compose and installer env file
  - use it for deployment-specific ports or metadata; runtime selection comes from `config/config.yaml.forward_runtime`

Important:

- local `go run` does not auto-load `.env`
- `config/config.yaml` is the primary source of truth
- runtime selection is defined in `config/config.yaml.forward_runtime`
- merged runtime values are persisted into `v2_system_config`

## Docker Quickstart

1. Prepare files:

```powershell
Copy-Item .env.example .env
Copy-Item config\config.yaml.example config\config.yaml
```

2. Edit [`config/config.yaml`](../../config/config.yaml) at minimum:

- `jwt.secret`
- `app.api_token`
- `admin.email`
- `admin.password`
- `forward_runtime.backend`

3. If you need NodeX mode, set it in `config/config.yaml`:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://nodex-control-plane:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

4. If you need the local stateless Ansible path, set it in `config/config.yaml`:

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

5. Start:

```powershell
docker compose up -d
docker compose logs -f v2board
```

6. Open:

- panel API: `http://127.0.0.1:8080`
- admin or user frontend: `http://127.0.0.1:3000`

## Local Development

1. Prepare `config/config.yaml`:

```powershell
Copy-Item config\config.yaml.example config\config.yaml
```

2. Fill `jwt.secret`, `app.api_token`, `admin.password`, and `forward_runtime`.

3. Start backend:

```powershell
go mod download
go run .\cmd\server\main.go -config .\config\config.yaml
```

4. Start frontend:

```powershell
Set-Location web
npm install
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

- `v2board` API: `8080`
- `v2board` UI: `3000`
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
