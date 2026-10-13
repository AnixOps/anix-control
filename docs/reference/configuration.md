# Configuration

This page defines the unified configuration scheme for `anix-control`.

## Source Of Truth

| Location | Role | Notes |
|------|------|------|
 | `config/config.yaml` (template: [`config/config.yaml.example`](../../config/config.yaml.example)) | canonical app and forward runtime config | read on startup when it exists, or when named by `-config` / `ANIX_CONTROL_CONFIG` |
 | built-in defaults ([`internal/config/defaults.yaml`](../../internal/config/defaults.yaml)) | production-shaped container defaults | used instead of a file when no config file is present |
 | `ANIX_CONTROL_*` environment variables | per-key overrides and secrets | applied on top of the file or the built-in defaults; see [Environment Variables](#environment-variables) |
 | `v2_system_config` | persisted runtime snapshot | written on startup from the loaded config and consumed by runtime services and `/admin/system` |

Important:

- local `go run` does not auto-load `.env` (template: [`.env.example`](../../.env.example))
- `jwt.secret`, `app.api_token`, `admin.*`, database, cache, and frontend settings come from the config file or its `ANIX_CONTROL_*` override
- the runtime selection resides in `forward_runtime`; startup writes exactly those values into `v2_system_config`

## Environment Variables

Every scalar and list key can be set from the environment. The variable name
is `ANIX_CONTROL_` plus the upper-cased YAML path joined with underscores:

| YAML key | Variable |
|------|------|
| `database.password` | `ANIX_CONTROL_DATABASE_PASSWORD` |
| `jwt.secret` | `ANIX_CONTROL_JWT_SECRET` |
| `plugins.control_poll_interval` | `ANIX_CONTROL_PLUGINS_CONTROL_POLL_INTERVAL` |
| `server.trusted_proxies` | `ANIX_CONTROL_SERVER_TRUSTED_PROXIES` (comma-separated) |

Rules:

- Precedence: config file (or built-in defaults) first, then the environment.
- `NAME_FILE=/path` reads the value from a file, for Docker and Kubernetes
  secrets. Setting both `NAME` and `NAME_FILE` is an error.
- Booleans accept `true`/`false`/`1`/`0`; lists are comma-separated.
- Map-valued keys (`forward_runtime.nftables_ansible.environment` and
  `extra_vars`) can only be set in the YAML file.
- Unknown `ANIX_CONTROL_*` names are logged at startup, so typos are visible.
- `anix-control -print-env` prints every variable with its type and built-in
  default; secret defaults are never printed.
- `server.trusted_proxies` (IPs or CIDRs) are the reverse proxies whose
  `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-For` and
  `X-Real-IP` Control honours (`internal/requestorigin`); from any other peer
  they are ignored. Unset means `127.0.0.1/32,::1/128`; an empty list trusts
  none; a list replaces the default. See `docs/DEPLOYMENT.md` section 6.1.

Without a config file, Control starts from the built-in defaults: `env:
production`, no `domain` (so `tls.mode` external), the one plain listener on
`127.0.0.1:8080` and `127.0.0.1:3000` (the container image sets
`ANIX_CONTROL_SERVER_HOST=0.0.0.0`), PostgreSQL
`anix_control@127.0.0.1:5432`, Control package execution enabled with private
temporary host directories, gRPC disabled. In production `jwt.secret` is
required and must not be a template value, or the server refuses to start.

## Panel Address, Listeners And Certificates

One HTTP handler serves everything a browser, a client app and a node
fetch: the web app (with its SPA fallback), the API, the subscription path,
`/install.sh` and `/install/*`, the health probes and `/metrics`. A path under
`/api/`, `/install/`, `/.well-known/` or the subscription path that matches
no route answers 404, never the web app's HTML. Where that handler listens,
and who terminates TLS, follows `tls.mode`:

```yaml
domain: panel.example.com    # the panel's name: https://<domain>
tls:
  mode: ""                   # acme | files | external; empty: acme with domain, external without
  cert_file: ""              # files: the web certificate; external: the gRPC certificate
  key_file: ""
listen: ""                   # external only: the plain http address; empty: server.host:server.port
grpc:
  port: 50051                # node gRPC port
  public_name: ""            # the name nodes dial when domain is behind a CDN; empty: domain
```

| Key | Default | Meaning |
|------|------|------|
| `domain` (`ANIX_CONTROL_DOMAIN`) | empty | The panel's DNS name (no scheme, port or address). The install script is `https://<domain>/install.sh`, the ACME certificate's name in `acme` mode and the gRPC name unless `grpc.public_name` is set. With it the gRPC listener is always on. The older `tls.domain` is not read (it was only ever logged). |
| `tls.mode` (`ANIX_CONTROL_TLS_MODE`) | empty | `acme`: Control obtains and renews a publicly trusted certificate for `domain` and serves https on `:443` and http on `:80` (the ACME HTTP-01 answers; everything else is redirected to https). `files`: https on `:443` with `tls.cert_file`/`tls.key_file`, reloaded when they change; `:80` redirects. `external`: a CDN or a reverse proxy terminates TLS and sends plain http to `listen`. Empty: `acme` with `domain`, `external` without. |
| `tls.cert_file`, `tls.key_file` | empty | PEM certificate chain and key. `files` mode: the web certificate (and the gRPC one for the same name). `external` mode: the gRPC listener's certificate, instead of one obtained over ACME. Both or neither. |
| `listen` (`ANIX_CONTROL_LISTEN`) | empty | `external` mode's plain http address (`host:port`; an address, not a name). Loopback is the safe choice: only the proxy on this machine reaches it. Empty: `server.host:server.port` (`127.0.0.1:8080` built in), and with `frontend.enable` also `server.host:frontend.port`, one handler on both (the legacy two-port layout; there is no proxy between them any more). An empty `server.host` is loopback. |
| `grpc.port` (`ANIX_CONTROL_GRPC_PORT`) | `50051` | Node gRPC port. `/install/agent.env` hands nodes `<gRPC name>:<grpc.port>`. |
| `grpc.public_name` (`ANIX_CONTROL_GRPC_PUBLIC_NAME`) | empty (`domain`) | The DNS-only name nodes dial for gRPC when `domain` is proxied by a CDN (the CDN mode below). Control obtains a publicly trusted certificate for it over ACME. |

The node gRPC listener's certificate, in this order: `grpc.tls_cert_file`
and `grpc.tls_key_file` when set (reloaded when they change; kept for
existing installs); the panel's own certificate when the gRPC name is
`domain` in `acme` or `files` mode; `tls.cert_file` in `external` mode; else
a certificate of its own for the gRPC name, over ACME. Without any name and
any file (no `domain`, no `grpc.public_name`) the gRPC listener is plain, as
in development and tests, and no Agent can connect. Every TLS listener takes
its certificate per handshake: a renewed or reloaded certificate reaches the
next connection without a restart, and Agent streams move to it within 30
minutes (`MaxConnectionAge`). The client certificate rule of the gRPC
listener does not change: it asks Agents for their certificate and verifies
it in the Agent services.

**Ports.** In `acme` and `files` mode Control owns `:443` and `:80` (both
IPv4 and IPv6); a port taken by another program stops the start. Under
systemd, a non-root Control needs `AmbientCapabilities=CAP_NET_BIND_SERVICE`
for them. In `external` mode Control binds `listen`, and when it obtains a
gRPC certificate over ACME it also binds `:80` (HTTP-01) and `:443`
(TLS-ALPN-01) when they are free; a challenge port that is taken is shown
in `anix-control tls status`, and the HTTP-01 request must then reach
`listen` through the proxy, which answers `/.well-known/acme-challenge/`
for the gRPC name too.

**ACME.** `golang.org/x/crypto/acme/autocert` against the public CA
(Let's Encrypt): TLS-ALPN-01 on 443 first, HTTP-01 on 80 second. The first
request goes out at start, in the background: until it succeeds, and
whenever no usable certificate is there, every listener serves a
self-signed fallback (ECDSA P-256, the name and this machine's addresses,
30 days) and handshakes never wait for the CA. The certificate is renewed a
third of its lifetime before its end (at most 30 days). Failed requests are
retried after 1, 5 and 15 minutes, then hourly three times, then every 6
hours, and Control never makes more than three requests per name within an
hour, manual retries included. The account key, the certificates and their
keys live in the database table `v5_acme_cache` (the schema migration
`0003_acme_cache` creates it), sealed with `module_runtime.ca_kek` (which
ACME therefore needs: without it the server refuses to start in `acme`
mode), so a restore or a move to another machine does not request them
again. No HSTS header is sent over Control's own TLS:
while the fallback is served a browser that remembered HSTS could not open
the panel at all. No e-mail address is registered: the panel's own expiry
alerts replace the CA's reminders.

**Status and failure.** `GET /api/v5/panel/status` (administrators), the
console's banner, `anix-control tls status` and `anix-control doctor` show
each certificate: `pending`, `valid` or `failed`, what is served (`acme`,
`file`, `self_signed`), whether it verifies against the system roots, its
expiry and renewal, the last error and a fix. While the gRPC certificate is
not valid and publicly trusted, or the panel's own certificate is not valid,
the install readiness check (`grpc_certificate`, `panel_certificate`)
withholds the join command: Agents verify the gRPC certificate against their
system CAs and fail closed. A panel certificate that is valid but not
publicly trusted (a CDN's origin certificate in `files` mode) only warns,
since nodes then download the script through the CDN; the banner shows it
only for a certificate the nodes dial. The
alerts `panel_certificate_expiring` and `panel_certificate_renewal_failing`
notify the administrators ([`kernel-alerts.md`](kernel-alerts.md)).
`anix-control tls retry` (or **重新申请** in the banner, `POST
/api/v5/panel/certificates/retry`) tries the failing ones again now.

**The CDN mode.** When the panel's domain must stay behind a CDN (a
proxying DNS record), the web side goes through the CDN and the nodes come
straight to this machine:

```yaml
domain: panel.example.com          # proxied by the CDN
tls:
  mode: external
listen: 127.0.0.1:8080             # the local proxy the CDN talks to; 0.0.0.0:80 when the CDN talks to Control itself
grpc:
  public_name: grpc.example.com    # a DNS-only A/AAAA record pointing at this machine
```

- The CDN (or the proxy behind it) terminates TLS for `panel.example.com`
  and forwards the whole host, `/install.sh` and `/install/*` included, to
  `listen`. The install script reads the node's address from
  `/install/ip` through the CDN, and Control takes it from
  `X-Forwarded-For` only from a peer in `server.trusted_proxies`: list the
  local proxy and the CDN's published address ranges there, or the join
  records the CDN's address for the node.
- Nodes fetch `https://panel.example.com/install.sh` through the CDN and
  get `grpc_target grpc.example.com:50051` from `/install/agent.env`; they
  dial that name directly and verify its certificate.
- Control obtains the certificate for `grpc.example.com` over ACME: HTTP-01
  on port 80 works for a DNS-only name (bound by Control when free, else
  answered on `listen` through the local proxy), TLS-ALPN-01 on 443 when 443
  is free. Open `grpc.port` and port 80 (or 443) to the internet.
- With your own certificate for the gRPC name instead, set
  `tls.cert_file`/`tls.key_file` (external mode) or `grpc.tls_cert_file`.
- A web certificate of your own on Control (a CDN origin certificate, for
  example) is `tls.mode: files` with `grpc.public_name` as above.

`agent_install.public_url` and `agent_install.grpc_target` still override
the derived `https://<domain>` and `<gRPC name>:<grpc.port>`.

`tls.enable` is not read: with `tls.cert_file` and no `tls.mode` Control
refuses to start (that combination served https on the web port, which is
now `tls.mode: files` on 443), otherwise a log line says it is ignored.
`tls.domain` is not read either (a log line points at `domain`), so an
existing configuration behind a reverse proxy does not move to `acme` by
itself. `frontend.enable`, `frontend.port` and `server.host`/`server.port`
remain as the legacy external layout described above.

### Testing ACME

Two environment variables, not configuration keys, point Control at
another ACME directory, for development, CI and a validation with the
public CA's staging directory before the real one:

| Variable | Meaning |
|------|------|
| `ANIX_CONTROL_ACME_DIRECTORY_URL` | the ACME directory, for example `https://acme-staging-v02.api.letsencrypt.org/directory`; empty: the public CA's production directory. Each directory keeps its own account and certificates in `v5_acme_cache`, so switching back requests a production certificate instead of serving the staging one. The status shows a non-default directory. |
| `ANIX_CONTROL_ACME_CA_FILE` | extra PEM roots the ACME client trusts for the directory's own https (a test CA) |

A staging certificate is not publicly trusted: the readiness checks keep
the join command withheld until the production directory issued one.

CI runs `TestPebbleIssuesOverBothChallenges` (`internal/panelcert`)
against Pebble, the ACME test server, in the E2E Tests job: Pebble
`v2.10.1` and `pebble-challtestsrv` downloaded by tag and checked by
SHA-256, every name resolving to 127.0.0.1, with `PEBBLE_VA_NOSLEEP=1`,
`PEBBLE_AUTHZREUSE=0` and `PEBBLE_WFE_NONCEREJECT=0`. To run it locally,
start them as that job does (`.github/workflows/ci.yml`) and set
`ANIX_CONTROL_TEST_PEBBLE_DIRECTORY`, `ANIX_CONTROL_TEST_PEBBLE_CA_FILE` and
`ANIX_CONTROL_TEST_PEBBLE_ROOTS_URL`; without them the test skips. The unit
tests use an in-process ACME CA (`internal/panelcert/panelcerttest`) that
checks the JWS envelope and validates both challenges against the
listeners under test, but has no DNS, rate limits or CAA.

Container-related settings:

| Key | Default | Notes |
|------|------|------|
| `server.shutdown_drain_delay` | empty (`5s` in built-in defaults) | time to keep serving after `/readyz` starts failing on shutdown; at most `5m` |
| `log.format` | `text` | `json` writes one JSON object per line to stdout (standard log lines, access logs, GORM); `text` keeps the historical output |
| `log.level` | `info` | minimum level for structured records in `json` format; standard log lines are always written |

PostgreSQL connection settings:

| Key | Default | Notes |
|------|------|------|
| `database.sslmode` | `disable` | libpq `sslmode`: `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full` |
| `database.timezone` | `Asia/Shanghai` | session `TimeZone`; must be a valid IANA name |
| `database.dsn` | empty | full connection string used verbatim instead of the fields above |

SQLite connection settings (`database.driver: sqlite`, a database file). They
are built in, not keys: `database.max_open_conns`, `max_idle_conns` and
`conn_max_lifetime` apply to PostgreSQL only.

| Setting | Value | Why |
|------|------|------|
| connections | one writer, a pool of 4 readers (`query_only`) | SQLite has one write transaction at a time; writers queue in Control instead of waiting inside SQLite, and in WAL mode reads neither block nor wait for the writer. Every write statement and every transaction runs on the writer, plain queries on the readers |
| `journal_mode` | `WAL` | readers and the writer do not block each other |
| `synchronous` | `NORMAL` | in WAL mode a commit does not wait for an fsync, the log is synced at checkpoints; a power loss may drop the last commits and never damages the database |
| `busy_timeout` | 10 seconds | how long a statement waits for a lock another *process* holds: the package hosts write the same file |
| transactions | `BEGIN IMMEDIATE` | a read-then-write transaction takes the write lock at BEGIN and cannot fail half way with `database is locked` |
| `auto_vacuum` | `INCREMENTAL` | only for a database file created by this version; the retention worker returns freed pages with `PRAGMA incremental_vacuum` |
| `foreign_keys` | off | as before: the schema and its data were never run under enforcement |

An in-memory database (`:memory:`, tests) keeps one handle. The package hosts
open the same file with their own 4 connections and the same `busy_timeout`,
`synchronous` and WAL settings (`sdk/packagestoresdk`).

## Experimental Forwarding Engine

| Key | Default | Notes |
|------|------|------|
| `forward.anixops_experimental` (`ANIX_CONTROL_FORWARD_ANIXOPS_EXPERIMENTAL`) | `false` | Lets routes use the experimental AnixOps engine and link (`ENGINE_ANIXOPS`, `LINK_SECURITY_ANIXOPS`), the secure relay between forward nodes (`docs/architecture/anixops-protocol.md`). Off, validation and the planner refuse them (`engine_not_enabled`). The Agents need their own switch, `Forward.AnixOps.Enable` in the Agent's JSON configuration (`"Forward": {"AnixOps": {"Enable": true}}`), to advertise the engine, or the planner refuses their nodes. v4.2 ships it as a prototype whose wire format (ALPN `anixops/0`) may change without notice; v4.3 freezes it. |

## Built-in Packages

Every official package is built into the binary and installed, upgraded and
enabled at every start
([`../architecture/plugin-kernel-contract.md`](../architecture/plugin-kernel-contract.md#built-in-packages)).
Two keys bear on it:

| Key | Default | Notes |
|------|------|------|
| `plugins.official_public_key` (`ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY`) | the AnixOps release root | Every built-in package must verify against it; a package that does not is left out (`builtin_package_invalid`) and Control starts without it. A development build (`make build`) is signed with a development key: `make run` sets the variable to it (`make dev-package-key` prints it). |
| `plugins.identity_bootstrap_package_dir` | empty | Ignored by a binary with built-in packages (every release). A development build without them still imports `identity-platform` from this directory at start. |

`app.edition` is gone: there are no community and commercial editions.
Orders, payments and affiliate are installed like every other package, and
a configuration that still sets the key loads (the key is ignored). The
public `GET /api/v4/public/config` keeps its `edition` (`commercial`) and
`hidden_packages` (empty) fields for the web app.

The package hosts always run, and Agent plugin dispatch and topology
execution are off until M4; none of them has a setting.

## Removed Keys

These keys are no longer read. A configuration file or an `ANIX_CONTROL_*`
variable that still sets one loads, and the start logs one line per key
(`Ignoring the removed configuration key <key> (set in <source>): <reason>`);
`internal/config/deprecated.go` lists them. Delete them when convenient
([`../UPGRADE.md`](../UPGRADE.md#plugin-center-reduced-to-package-status)).

| Key | Why |
|------|------|
| `plugins.control_execution_enabled` | The built-in packages always run |
| `plugins.dispatch_enabled`, `plugins.dispatch_poll_interval` | Agent plugin dispatch is off until M4 brings the Agent plugins |
| `plugins.topology_execution_enabled`, `plugins.topology_poll_interval` | Topology deployment execution is off until M4 |
| `package_routes.default_mode` | Route modes come only from the rehearsed list (below) |

## Unified Forward Runtime Layout

Put the runtime selection in `config/config.yaml`:

```yaml
forward_runtime:
  backend: "gost"

  jobs:
    poll_interval: "5s"
    idle_poll_interval: "30s"
    error_log_interval: "1m"
    batch_size: 10
    timeout_seconds: 120

  gost_stats:
    poll_interval: "30s"
    idle_poll_interval: "2m"
    error_log_interval: "5m"

  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15

  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    become: false
    extra_vars: {}
    command: ""
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120

  iptables_ansible:
    # Legacy compatibility only
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove.yml"
    become: false
    extra_vars: {}
    command: ""
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
```

Startup behavior:

1. backend loads `config/config.yaml`
2. `forward_runtime` is parsed
3. merged values from `forward_runtime` are persisted into `v2_system_config`
4. `/admin/system` and forward runtime services consume the persisted values

Optional worker tuning now also stays inside `forward_runtime`:

- `forward_runtime.jobs.*` controls the local ansible runtime job executor
- `forward_runtime.gost_stats.*` controls the gost traffic polling worker
- both use the same YAML file instead of separate env overrides

## `nodex_mode` Compatibility Switch

`forward_runtime.nodex_mode` remains supported for compatibility:

- `true` forces backend to `gost`
- `false` forces backend to the local ansible path, defaulting to `nftables_ansible`
- if omitted, `forward_runtime.backend` is used directly

If both are present, `nodex_mode` wins.

## NodeX Mode

Use this for the private stateful runtime:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

Required values:

- `forward_runtime.nodex.base_url`
- `forward_runtime.nodex.token`

## Local Ansible Mode (`nftables_ansible` Recommended)

Use this for the local stateless executor:

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

Operational rules:

- no NodeX control-plane is required
- no proxy ingress node is required
- the executor host must have `ansible-playbook`
- SSH credentials still come from inventory files referenced by the selected ansible block under `config/config.yaml.forward_runtime`
- `iptables_ansible` remains available only for legacy relay environments that still need the old firewall driver and playbooks

## Path Resolution

The selected local ansible block (`forward_runtime.nftables_ansible` by default, `forward_runtime.iptables_ansible` for legacy hosts) has these path fields normalized on startup:

- `inventory`
- `apply_playbook`
- `remove_playbook`
- `working_dir`
- `environment.ANSIBLE_CONFIG`

Relative paths are resolved from the config file and project/runtime location so local binary, Windows, Linux, and Docker stay aligned.

## Runtime Keys Written Into System Config

Startup persists the merged runtime snapshot into these keys:

- `forward.runtime_backend`
- `forward.runtime.nodex_mode`
- `forward.runtime.ansible.backend`
- `forward.runtime.nodex.base_url`
- `forward.runtime.nodex.token`
- `forward.runtime.nodex.timeout_seconds`
- `forward.runtime.ansible.config`
- `forward.runtime.ansible.inventory`
- `forward.runtime.ansible.apply_playbook`
- `forward.runtime.ansible.remove_playbook`
- `forward.runtime.ansible.become`
- `forward.runtime.ansible.extra_vars_json`

Legacy compatibility readers still understand:

- `forward.runtime.iptables_ansible.config`
- `forward.ansible.*`

## Control Package Payload Limits

`/api/v2` compatibility routes are served by signed control packages. Two
`plugins` keys bound the bodies that cross the package boundary:

```yaml
plugins:
  control_host_max_request_bytes: 1048576   # 0 or unset = 1 MiB
  control_host_max_response_bytes: 1048576  # 0 or unset = 1 MiB
```

| Key | Enforced by | Error when exceeded |
|------|------|------|
| `control_host_max_request_bytes` | the v2 gateway, before dispatch | `413` `plugin_request_too_large` |
| `control_host_max_response_bytes` | the package bridge (legacy handler body), the package-host SDK (package response body), the kernel host client | `502` `plugin_response_too_large` |

- Both values are integers in bytes, from `0` (default 1 MiB) to `67108864`
  (64 MiB). Startup fails on a negative or larger value.
- The response limit applies to the body only. The SDK additionally allows a
  fixed 64 KiB for status, headers, and identifiers, so a body the bridge
  accepts is never rejected by the host for its envelope.
- The kernel passes the response limit to each package host as
  `ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES`; hosts built on `sdk/pluginhostsdk`
  read it with `MaxResponseBytesFromEnvironment`.
- gRPC receive sizes grow with the limits, so values above 4 MiB work end to
  end. Package hosts should build their gRPC server with
  `pluginhostsdk.HostServerOptions()`.
- WebSocket frames are not affected by these keys.

The gateway exports per-package traffic on `/metrics`:
`anixops_v2_gateway_requests_total{package,route,code_class}`,
`anixops_v2_gateway_errors_total{package,route,code}` (gateway error codes such
as `package_unavailable`, `package_route_not_found`, `plugin_host_unavailable`,
`plugin_host_incompatible`, `plugin_request_too_large`,
`plugin_response_too_large`, `sealed_secret_unavailable`,
`sealed_secret_refused`), and the
`anixops_v2_gateway_request_duration_seconds{package}` histogram. `route` is
the package route id from the signed declaration, never the request path;
requests that match no declared route are counted under
`package="unresolved",route="unresolved"`.

The routes of `config/node-secret-fields.json` have their node secrets sealed
before a package host reads them (`docs/architecture/node-ops-service.md`
section 3.7).

- **The metric.** `anixops_v2_gateway_sealed_secrets_total`, with the labels
  `package`, `route`, `stage`, `result` and `reason`, counts the gateway's
  work:
  - `stage="request"`: requests sealed (`result="sealed"`), served by the
    kernel's legacy handler without the host because they could not be
    sealed (`legacy_fallback`), or refused (`refused`);
  - `stage="answer"`: handles expanded (`expanded`) or refused (`refused`).
- **Reasons.** `reason` is one of `not_json`, `value_not_string`,
  `target_invalid`, `too_large`, `fields_unavailable`, `answer_handle`,
  `store` or `none`.
- **Errors.** `sealed_secret_unavailable` (503) is a request that could not
  be sealed on a route with no legacy handler. `sealed_secret_refused` (502)
  is an answer that would show a handle.

While package execution is enabled, `/metrics` also reports each supervised
package host: `anixops_plugin_host_starts_total`,
`anixops_plugin_host_unexpected_exits_total`,
`anixops_plugin_host_restarts_total` and `anixops_plugin_host_failures_total`
(all `{package}`), and `anixops_plugin_host_state{package,state}`, which is `1`
for the current state (`running`, `restarting`, `failed`, `exited`,
`stopped`) and `0` for the others.

## Agent Client Certificates

`agent_control.mtls` (`ANIX_CONTROL_AGENT_CONTROL_MTLS`) sets how AnixOps
Agents authenticate on their channels: the gRPC listener (`grpc.*`) and the
legacy agent HTTP and WebSocket paths.

```yaml
agent_control:
  mtls: ""              # off | optional | preferred | required; empty: required (v4.2)
  legacy_sunset: ""     # optional YYYY-MM-DD, announced to legacy agents
```

| Mode | Agents without a client certificate |
|------|------|
| `off` | authenticate with their node API key; certificates are neither requested nor accepted and `Enroll` is unavailable (a rollback switch) |
| `optional` | authenticate with their node API key, silently |
| `preferred` (the 4.1 default) | still authenticate, and get deprecation signals: `Deprecation: true`, `Link` to the upgrade guide and, with `legacy_sunset`, `Sunset` on the legacy HTTP paths; `x-anix-auth-deprecated` (with `x-anix-auth-deprecation-link`, `x-anix-auth-sunset`) on the control stream |
| `required` (default from 4.2, also when empty) | are refused on the AnixOps Agent channels: HTTP 403 with `"code": "agent_mtls_required"`, gRPC `Unauthenticated` with the trailer `x-anix-error-code: agent_mtls_required`; `Enroll` takes only one-time enrollment credentials |

- **Scope.** The AnixOps Agent channels are the control stream's API key
  authentication, `/api/v2/agent/*`, `/api/v2/node/*`,
  `/api/v2/forward/agent/rules` and the clean agent endpoints
  (`/api/v2/forward-agent/register|heartbeat|report`). UniProxy
  (`/api/v1|v2/server/UniProxy/*`) and the v2board gRPC services, which
  third-party node software (XrayR, V2bX) uses, are never signalled or
  refused.
- Agents get certificates from `anix.agent.v1.AgentEnrollment`, signed by the
  built-in CA. It needs only `module_runtime.ca_kek`
  (`ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`, 32 bytes, base64 or hex) with
  `module_runtime.pki: builtin`; `module_runtime.enabled` and the module
  listener are not needed. Without the CA, or with `pki: external`, `off`,
  `optional` and `preferred` start, but agents cannot enroll (the startup log
  line `Agent transports: ...` says so).
- `required` needs the gRPC listener with TLS (`domain`, or `grpc.enabled`
  with `grpc.tls_cert_file` and `grpc.tls_key_file`) and the built-in CA for
  agents to enroll and connect.
  Set explicitly, startup fails without them. Left empty (the v4.2
  default), Control starts and logs a `WARNING: Agent transports: ...` line
  that no Agent can connect until they are set; legacy Agents are refused
  either way.
- Under `required`, startup also warns (`WARNING: Agent transports:
  agent_control.mtls=required refuses N enabled node(s) ...`) when the
  transport inventory still has enabled nodes on a legacy channel (counting
  those seen within the last 7 days) or nodes that never enrolled. It never
  stops the start.
- `legacy_sunset` (`ANIX_CONTROL_AGENT_CONTROL_LEGACY_SUNSET`) is empty by
  default, so no `Sunset` header is sent until an operator fixes a date.
- Before switching to `required` or upgrading to v4.2, run `anix-control
  agents transports --check-required` (exit status 3 while an enabled node
  would be refused: `legacy` or `never_enrolled`; 0 when none;
  `summary.ready_for_required` in `GET /api/v4/kernel/agents/transports`),
  or read the node list's connection and certificate columns. Every node it
  lists must first run
  an enrolled agent, or be disabled. To keep serving legacy Agents for a
  while, set `preferred`
  ([UPGRADE](../UPGRADE.md#agent-transports-v42-requires-enrolled-agents)). `/metrics` has
  `anixops_agent_legacy_requests_total{path}`,
  `anixops_agent_legacy_refused_total{path}` and
  `anixops_agent_mtls_mode{mode}`.
- Enrollment credentials: `anix-control agent token create -node proxy-12`
  or `POST /api/v4/kernel/agents/enrollment-tokens`. Details in
  [`../architecture/module-runtime.md`](../architecture/module-runtime.md#agent-pki)
  and [`../architecture/node-ops-service.md`](../architecture/node-ops-service.md)
  section 5.6.

## Agent Install (One-Command Onboarding)

`agent_install` configures the install commands Control renders, a node
group's (`GET /api/v4/kernel/node-groups/:id/command`) and an existing
node's (`POST /api/v4/kernel/agents/install-tokens`), both for super
administrators, and the public routes the script uses: `GET /install.sh`,
`/install.sh.sig`, `/install/agent.env`, `/install/agent/<tag>/<asset>` and
`/install/ip` (the caller's address as Control sees it, with
`server.trusted_proxies` applied) ([guide](../guide/agent-onboarding.md)).
Node group join tokens are sealed with `module_runtime.ca_kek`: without it
no node group can be created.

```yaml
agent_install:
  public_url: ""      # https address nodes reach; empty: https://<domain>, else the request's origin
  grpc_target: ""     # host:port nodes dial; empty: grpc.public_name (or domain, else public_url's host) and grpc.port
  agent_version: ""   # Agent release tag; empty: the Agent release this Control was released with
  artifact_dir: ""    # <dir>/<tag>/<asset> (+ .dgst/.sig): Control serves the Agent
  cn_mirror_url: ""   # mainland mirror of the GitHub release downloads
  signature_file: ""  # release signature of the install script (set by the image)
```

- Nodes must reach Control over https: without `public_url` and `domain`
  the request's origin is used, which behind a reverse proxy is only right
  when the proxy is in `server.trusted_proxies`; an http origin refuses to
  issue tokens (`agent_install_unconfigured`).
- Tokens are AgentPKI one-time enrollment credentials (`anixagt_`, stored as
  SHA-256 in `v4_kernel_agent_enrollment`), bound to a node, valid 1 hour by
  default and at most 7 days.
- `/install.sh.sig` is served only when `signature_file` verifies the
  embedded script with `plugins.official_public_key`; the release image sets
  `ANIX_CONTROL_AGENT_INSTALL_SIGNATURE_FILE`.
- The subscription path (`app.subscribe_path`) may not be `install`,
  `install.sh` or start with `install/`.
- `anix-control agent offline-bundle -arch amd64|arm64 -o <file>
  [-control https://<control>]` writes an offline install bundle from
  `artifact_dir/<tag>/` (the zip and its `.sig`, `SHA256SUMS` and
  `SHA256SUMS.sig`, checked with `plugins.official_public_key`) for
  `install.sh --offline`.

## Package Route Modes

No key sets them. The route modes of the v2 Control packages come only from
the rehearsed list, `config/package-route-defaults.json`, which ships in the
binary: its 130 routes of 14 packages run natively once the installed package
release is at least its `min_version` (`4.1.0-rc.5`), identity group A
follows the identity authority, and every other route runs its legacy
handler. Nothing stores or switches a route's mode any more (the route-mode
administration and `anix-control routes` were removed). The startup log line
`package route modes: ...` states the count.

## Alerts

`alerts.*` (`ANIX_CONTROL_ALERTS_*`) configures the alert monitor: Agent,
forward link and module certificates that were not renewed in time, CAs near
their end, the panel's own certificates (ending soon, or failing to renew),
and phased processes (the node credential split, the identity cutover) left
in a non-final phase. Every key has a default, so the block can
be left out.

```yaml
alerts:
  enabled: true
  check_interval: "15m"
  leaf_expiry_days: 14
  ca_expiry_days: 60
  renotify_interval: "24h"
  phase_stuck_after: "72h"   # "0" turns the phase alerts off
```

A leaf certificate alerts inside the smaller of `leaf_expiry_days` and a
sixth of its own lifetime (healthy holders renew at two thirds, so a fixed
14 days would fire for every fresh 7-day certificate). Details, the alert
kinds and the API are in [`kernel-alerts.md`](kernel-alerts.md).

## Related Docs

- [`kernel-alerts.md`](kernel-alerts.md)

- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`startup-config.md`](startup-config.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
