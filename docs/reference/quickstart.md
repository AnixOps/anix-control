# Quickstart

This page is the shortest path to getting `anix-control` running with the unified config model.

## What The Backend Actually Reads

Always prepare these inputs first:

- `config/config.yaml` (local, gitignored; copy from [`config/config.yaml.example`](../../config/config.yaml.example), the `domain` line)
  - the operator keys ([configuration](configuration.md#operator-keys)); the deprecated keys still read until later milestones (the database, `forward_runtime`) go here too
- `data_dir/root.key` (`/var/lib/anix-control` by default): the root key, which Control creates at its first start
- `data_dir/backups/`: the backups Control takes of a SQLite panel every day and before every migration ([backups.md](backups.md)); `anix-control backup create` takes one now
- `.env` (local, gitignored; copy from [`.env.example`](../../.env.example))
  - optional Docker Compose and installer env file
  - use it for deployment-specific ports or metadata; runtime selection comes from `config/config.yaml.forward_runtime`

Important:

- local `go run` does not auto-load `.env`
- `config/config.yaml` holds the operator keys and no secret; `anix-control config check` checks it
- runtime selection is defined in `forward_runtime` (deprecated, read until M5)
- merged runtime values are persisted into `v2_system_config`
- no administrator, email or password is configured anywhere: on an empty database the first administrator claims the panel with a one-time link, `https://<domain>/claim#anixclaim_…`, that `anix-control admin claim-link` prints (it works once, for 24 hours; [first administrator](#the-first-administrator)). `admin.email` and `admin.password` are ignored with a warning
- the official packages are built into the release binary and installed at every start; a binary built from a checkout carries them only after `make build` (or `make build-packages`), signed with a development key that `make run` configures (`plugins.official_public_key`); a plain `go build` carries none and imports `identity-platform` from `plugins.identity_bootstrap_package_dir`

## Docker Quickstart

Development stack (builds this checkout, runs a throwaway PostgreSQL, no config
file needed):

```bash
docker compose up -d --build
docker compose ps                  # control becomes healthy
docker compose exec control /app/anix-control admin claim-link
                                   # open the link it prints: the first administrator
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

2. Set `domain` (or remove it to run behind a proxy on `127.0.0.1:8080`).
   No JWT secret, API token or administrator password: the root key derives
   the JWT secret, and the first administrator claims the panel (step 5).

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

5. On an empty database, print the claim link and open it (with `make run`,
   `make claim-link` does the same):

```bash
GOWORK=off go run ./cmd/server admin claim-link   # reads ANIX_CONTROL_CONFIG or config/config.yaml
```

## The First Administrator

A fresh panel has no administrator and no password anywhere. The start logs
one line, ``No administrator yet. Run `sudo anix-control admin claim-link` to
get the claim link.``, and nothing else; the sign-in page says the same.

- `anix-control admin claim-link [-ttl 24h]` prints
  `https://<domain>/claim#anixclaim_…` on its terminal only (never a log),
  plus `https://<this machine's public address>/claim#…` while the panel's
  certificate is not publicly trusted yet. Without `domain` it prints the
  path and the local listener instead.
- The link works once, for at most 24 hours; running the command again
  replaces it. The token stays in the URL fragment, which the browser never
  sends: the claim page posts it with the email and password you choose
  (`POST /api/v5/claim`), creates the administrator and signs you in.
- The password needs at least 12 characters (counted as characters, not
  bytes) and at most 72 bytes; a shorter one is refused with `weak_password`
  (422), and the page says so before it asks the server.
- Once the panel has an administrator the command refuses (exit status 1),
  the claim answers 404, and nothing changes for that administrator.

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
