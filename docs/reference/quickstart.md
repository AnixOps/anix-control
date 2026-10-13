# Quickstart

This page is the shortest path to getting `anix-control` running with the unified config model.

## What The Backend Actually Reads

Always prepare these inputs first:

- `config/config.yaml` (local, gitignored; copy from [`config/config.yaml.example`](../../config/config.yaml.example), the `domain` line)
  - the operator keys ([configuration](configuration.md#operator-keys)); the deprecated keys still read until later milestones (the database, the first administrator, `forward_runtime`) go here too
- `data_dir/root.key` (`/var/lib/anix-control` by default): the root key, which Control creates at its first start
- `.env` (local, gitignored; copy from [`.env.example`](../../.env.example))
  - optional Docker Compose and installer env file
  - use it for deployment-specific ports or metadata; runtime selection comes from `config/config.yaml.forward_runtime`

Important:

- local `go run` does not auto-load `.env`
- `config/config.yaml` holds the operator keys and no secret; `anix-control config check` checks it
- runtime selection is defined in `forward_runtime` (deprecated, read until M5)
- merged runtime values are persisted into `v2_system_config`
- no reusable administrator email or password is shipped with Control; set unique values in `admin.email` and `admin.password` (or the installer's corresponding flags) before first start
- the official packages are built into the release binary and installed at every start; a binary built from a checkout carries them only after `make build` (or `make build-packages`), signed with a development key that `make run` configures (`plugins.official_public_key`); a plain `go build` carries none and imports `identity-platform` from `plugins.identity_bootstrap_package_dir`

## Docker Quickstart

Development stack (builds this checkout, runs a throwaway PostgreSQL, no config
file needed):

```bash
ANIX_CONTROL_ADMIN_PASSWORD='choose-a-password' docker compose up -d --build
docker compose ps                  # control becomes healthy
```

Open:

- panel API: `http://127.0.0.1:8080` (`/readyz` for readiness)
- admin or user frontend: the same `http://127.0.0.1:8080` (with `domain`
  set, `https://<domain>` on 443 instead)

Settings are `ANIX_CONTROL_*` environment variables in `docker-compose.yml`
(`docker compose run --rm control -print-env` lists the operator keys; the
deprecated keys' variables are in [configuration](configuration.md#deprecated-keys)).
The root key is generated in the `control-data` volume. For example, NodeX
mode (deprecated keys, read until M5):

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

2. Set `domain` (or remove it to run behind a proxy on `127.0.0.1:8080`),
   and, for an empty database, `admin.email` and `admin.password`. No JWT
   secret or API token: the root key derives the JWT secret.

3. Start backend:

```bash
GOWORK=off go mod download
GOWORK=off go run ./cmd/server -debug -config config/config.yaml
```

For an isolated development instance with its own ports, SQLite database
and root key (`.dev/`), use `make run` instead (see the root
[`README.md`](../../README.md)).

4. Start frontend:

```bash
cd web
npm ci
npm run dev
```

## Runtime Overrides

Adjust runtime behavior by editing `forward_runtime` in the configuration file and restarting the backend. The `.env` file remains reserved for ports, secret tokens, and ancillary deployment flags rather than runtime selection.

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

- AnixOps Control web, API and install paths: `8080` (and `3000`, the same
  handler) without `domain`; `443` and `80` with `domain` (`tls.mode` acme)
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
