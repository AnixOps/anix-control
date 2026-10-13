# Startup And Config

This page describes the exact startup flow after the unified runtime config update.

## 1. Ownership Model

| Location | Purpose |
|------|------|
| the configuration file (`-config`, else `ANIX_CONTROL_CONFIG`, else `config/config.yaml` when it exists; template: [`config/config.yaml.example`](../../config/config.yaml.example)) | the operator keys an installation changes, usually `domain` alone ([configuration.md](configuration.md#operator-keys)) |
| `ANIX_CONTROL_*` environment variables | per-key overrides ([configuration.md](configuration.md#environment-variables)) |
| `data_dir/root.key` (`/var/lib/anix-control/root.key`) | the root key: generated or adopted at the first start ([configuration.md](configuration.md#the-root-key)) |
| `.env` (template: [`.env.example`](../../.env.example)) | optional Docker or installer env file |
| `v2_system_config` | persisted merged runtime snapshot used by runtime services |

Important:

- the built-in values come first, then the file key by key, then the
  variables; an explicitly named file must exist, and without one Control
  runs on its built-in values
- the file is read strictly: a deprecated key warns (still read or
  ignored), any other unknown key stops the start naming the closest key;
  `anix-control config check` shows the result without starting
- local `go run` does not auto-load `.env`
- runtime services read the forward runtime values that were written into `v2_system_config`

## 2. Minimal Configuration

```yaml
domain: panel.example.com
```

That is the whole file of an ordinary installation: Control obtains its
certificate for the domain, serves https on 443 and http on 80, and runs
the node gRPC listener on 50051. Behind a reverse proxy or CDN that
terminates TLS:

```yaml
domain: panel.example.com
tls:
  mode: external
  cert_file: /etc/anix-control/tls/fullchain.pem   # the nodes' gRPC certificate
  key_file: /etc/anix-control/tls/privkey.pem
listen: 127.0.0.1:8080
```

An earlier (4.x) file still loads: its keys are in the
[deprecated list](configuration.md#deprecated-keys), each warned about once.
`anix-control config rewrite --in <old> --out <new>` turns it into the
operator keys ([UPGRADE](../UPGRADE.md#configuration-rewrite-on-upgrade)).
While PostgreSQL is still supported (until M2 P4), a file for a SQLite
panel is:

```yaml
domain: panel.example.com
database:
  driver: sqlite
  database: /opt/anixops/control/config/data/v2board.db
```

No secret and no administrator go into the file: the root key in `data_dir`
derives the JWT secret and seals the CA, ACME and join token keys, and the
first administrator claims the panel with a one-time link
([quickstart](quickstart.md#the-first-administrator)). `admin.email` and
`admin.password` are ignored with a warning.

## 3. Local Binary Startup

The development instance:

```bash
make run
make claim-link    # on an empty database: the link that creates the first administrator
```

It keeps its SQLite database and root key in `.dev/` (ignored), serves the
web app and the API on `http://127.0.0.1:19080` (`tls.mode` external, no
domain) and gRPC on `127.0.0.1:50052`, runs with `-debug` (gin debug mode,
debug logging), and verifies the built-in packages against the development
key. `RUN_CONFIG=<file>` loads a configuration file as well.

Without make:

```bash
GOWORK=off go run ./cmd/server -debug -config config/config.yaml
```

Frontend development server:

```bash
cd web
npm ci
npm run dev
```

Without `domain`, `tls.mode` is external: one plain handler for the web app,
the API and the install paths on `listen` (`127.0.0.1:8080` by default; the
deprecated `server.port` and `frontend.port` still add their listeners when
set). Health: `http://127.0.0.1:8080/health`.

With `domain` set, Control serves https on `:443` and http on `:80` with a
certificate it obtains itself, or a plain `listen` address behind a CDN or
proxy (`tls.mode: external`); see
[`configuration.md`](configuration.md#panel-address-listeners-and-certificates).

## 4. Docker Startup

Development (builds this checkout and runs a throwaway PostgreSQL):

```bash
docker compose up -d --build
docker compose exec control /app/anix-control admin claim-link   # the first administrator's link
```

Production uses `docker-compose.prod.yml` with a released image digest, an
external PostgreSQL, `control.env` and `secrets/`; see
[`../DEPLOYMENT.md`](../DEPLOYMENT.md). The containers need no config file: the
built-in values plus `ANIX_CONTROL_*` variables configure them (the image
sets `ANIX_CONTROL_LISTEN=0.0.0.0:8080`). The root key lives in the
`control-data` volume at `/var/lib/anix-control`, adopted from
`secrets/module_ca_kek`. To keep a YAML file instead, mount it and set
`ANIX_CONTROL_CONFIG`.

## 5. What Happens On Startup

The runtime flow is now:

1. load the built-in values, overlay the config file key by key and the
   `ANIX_CONTROL_*` variables, and log one warning per deprecated key
2. resolve the root key in `data_dir` (`root.key`; the server generates it,
   or adopts `module_runtime.ca_kek` into it) and derive the JWT secret and
   the identity KEK from it unless they were adopted
3. normalize sqlite path, frontend path, and ansible runtime paths
4. take a shared lock on `data_dir/control.lock`, held until the process
   exits (the server and `migrate`): `anix-control backup restore` needs it
   exclusively, so a restore never runs under a Control, and a start while a
   restore runs stops with "a backup restore is running"
   ([backups.md](backups.md#restore))
5. initialize database
6. prepare the database (below): the migrations (before a pending one on a
   database that is not empty, a `pre-migration` backup; when it fails,
   nothing is migrated and the start stops), then the check that the root
   key opens the stored CA keys (a key that cannot stops the start), then
   the seeds and `InitForwardRuntimeSystemConfig`
7. persist the parsed runtime snapshot into `v2_system_config`
8. load the panel certificates (`internal/panelcert`): the ones stored in
   `v5_acme_cache`, the certificate files, and the self-signed fallback, so
   that every listener serves the right certificate from its first handshake
9. bind the node gRPC listener, then every HTTP listener of `tls.mode`; a
   port that is taken stops the start
10. start the background workers, among them the certificate loop, which
    sends the first ACME request at once and never blocks a handshake, and
    the backup worker (in the process that holds the singleton lease): it
    verifies the newest unverified bundle (this start's pre-migration
    backup), applies the retention and backs up daily
    ([backups.md](backups.md))

Before step 7, a database without an administrator logs one line,
``No administrator yet. Run `sudo anix-control admin claim-link` to get the
claim link.``: the start creates no administrator and generates no link
or password ([quickstart](quickstart.md#the-first-administrator)).

Database preparation runs under a PostgreSQL advisory lock, so several Control
processes starting together (a rolling update, or `migrate` next to a server)
run it one at a time:

- the schema comes from the ordered migrations in every environment (`env`
  no longer changes it; [database-migrations.md](database-migrations.md)): an
  empty database gets every migration, a database an earlier Control prepared
  is adopted once (missing tables, columns and indexes are added, nothing is
  altered or dropped), and pending migrations are applied, each in its own
  transaction. Control refuses to start on a database a newer Control
  migrated, on a changed migration, and on a non-empty database that is not
  a Control database, and says what to do.
- then the seeds that run at every start: the default service scopes, the
  official plugin catalog and target locks, the node secret split state, the
  module and forward link CAs, the kernel API views, the plugin trust root,
  the built-in package bootstrap (below), and the default subscription
  group, plan and authorized key; while the flux tables exist, the forward
  runtime settings and port bindings. No administrator is created.

The built-in package bootstrap verifies every official package embedded in
the binary against `plugins.official_public_key` and installs, upgrades and
enables it, in dependency order; a restart with the same binary changes
nothing. A package that fails is logged (`WARNING: built-in package ...`) and
raised as a kernel alert, and the start goes on
([`../architecture/plugin-kernel-contract.md`](../architecture/plugin-kernel-contract.md#built-in-packages)).
A binary built without them (`go build` from a checkout; `make build` embeds
a development-key set) imports `identity-platform` from
`plugins.identity_bootstrap_package_dir` instead. When the package host
supervisor starts, Control logs its own executable and SHA-256, starts every
enabled package's host as a child process of the same binary
(`anix-control package-host <id>`) and runs the migrations of a package whose
version changed.

`anix-control migrate [flags]` runs only this preparation and exits (status 0
on success). Use it as a one-shot Compose service or Kubernetes Job before the
server starts; the server repeats it idempotently.

Startup always starts the durable Control lifecycle worker and the package
hosts. Agent plugin dispatch and topology execution stay off until M4: no
setting turns them on, and the release builds no Agent plugin for them. A
configuration that still sets one of the removed keys
(`plugins.control_execution_enabled`, `dispatch_enabled`,
`dispatch_poll_interval`, `topology_execution_enabled`,
`topology_poll_interval`, `package_routes.default_mode`) starts; each key is
logged once as ignored ([`../UPGRADE.md`](../UPGRADE.md#plugin-center-reduced-to-package-status)).

That is why local startup and Docker startup now share the same primary config structure.

An invalid `plugins.control_poll_interval` stops startup before any listener
opens. Once the gRPC listener is up, a later failure (for example port 443 or
the `listen` port already in use) runs the normal shutdown below and exits
with status 1. A certificate that cannot be obtained never stops the start:
Control serves the self-signed fallback and says why
(`anix-control tls status`).

On SIGINT or SIGTERM, Control shuts down in this order: `/readyz` and
`/health` return 503 (see below), HTTP servers drain for up to 30s, background
workers are cancelled and awaited for up to 15s, the gRPC server stops (up to
10s), Control plugin hosts stop (up to 15s), then the cache and database close.
A clean shutdown exits with status 0. A second signal forces an immediate exit
with status 1.

### Probes And Shutdown

Every HTTP listener of the panel answers them (they are one handler):

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

If you need to change runtime behavior, edit `forward_runtime` in the configuration file before startup (deprecated keys, read until M5). That now includes:

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
9. `/admin/system/packages` (系统设置 → 内置包) lists the built-in packages, running
10. an Agent using the same official public key reports an active control stream

For a production-template rehearsal, verify the opposite before adding secrets:
`grpc.enabled` must remain `false`. The production template contains no
default node API token, gRPC fallback token, or NodeX shared token.

## 8. Related Docs

- [`configuration.md`](configuration.md)
- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
