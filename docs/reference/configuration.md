# Configuration

This page defines the configuration of `anix-control`: ten operator keys (nine,
and `grpc.edge_proxies` for a reverse proxy on another machine), the root key
in the data directory, and the keys earlier releases read.

## Source Of Truth

| Location | Role | Notes |
|------|------|------|
| `/etc/anix-control/config.yaml` (or `-config <file>`, `ANIX_CONTROL_CONFIG`, `config/config.yaml` next to the binary; template: [`config/config.yaml.example`](../../config/config.yaml.example)) | the operator keys an installation changes, usually `domain` alone | optional: without a file Control runs on its built-in values |
| built-in values ([`internal/config/defaults.yaml`](../../internal/config/defaults.yaml)) | every default | the file and the variables are overlaid on them key by key |
| `ANIX_CONTROL_*` environment variables | per-key overrides | applied last; see [Environment Variables](#environment-variables) |
| `data_dir/root.key` | the root key | not configuration: see [The Root Key](#the-root-key) |
| `v2_system_config` | persisted runtime snapshot | written on startup from the loaded config and consumed by runtime services and `/admin/system` |

The file holds no secret. The root key seals what Control stores and
derives the JWT secret and the identity signing key KEK.

## Operator Keys

An ordinary installation's whole file:

```yaml
domain: panel.example.com
```

| Key | Default | Meaning |
|------|------|------|
| `domain` | none; required in `tls.mode` `acme` | The panel's name: the web, API, subscription and install address (`https://<domain>`), the name of the certificate Control obtains, and the name nodes dial for gRPC. |
| `tls.mode` | `acme` with `domain`, else `external` | `acme`: Control obtains its certificate and serves https on 443 and http on 80. `external`: a reverse proxy or CDN terminates TLS and sends plain http to `listen`. `files`: https on 443 with `tls.cert_file`. |
| `tls.cert_file`, `tls.key_file` | empty | `external`: the node gRPC listener's certificate (publicly trusted, for the name nodes dial), reloaded when it changes. `files`: the web certificate. |
| `listen` | `127.0.0.1:8080` | `external` only: the plain http address the reverse proxy sends the whole panel to. |
| `grpc.port` | `50051` | The node gRPC port; nodes dial `<grpc.public_name or domain>:<grpc.port>`. |
| `grpc.public_name` | `domain` | Only when `domain` sits behind a CDN: a DNS-only name that reaches this machine, for the nodes' mutual TLS. Not `grpc.host`, which was a listen address. |
| `grpc.edge_proxies` | empty | Non-empty: a reverse proxy on another machine (its addresses, IPs or CIDRs) terminates the nodes' TLS with the public certificate, and sends their gRPC here in plain HTTP/2 over the private network. Control then holds no gRPC certificate. Requires `tls.mode` `external` and no certificate files. The recommended layout: [two hosts](../guide/deploy-two-hosts.md). |
| `subscribe_path` | `s` | The subscription path (`/<subscribe_path>/:token`). Change it only to keep links already handed out working. |
| `data_dir` | `/var/lib/anix-control` | The data directory: `root.key`, the backups (`backups/`, [backups.md](backups.md)), their scratch space (`tmp/`) and the lock a running Control holds (`control.lock`); with a later milestone the database too. Absolute. |

[Panel Address, Listeners And Certificates](#panel-address-listeners-and-certificates)
describes the modes in detail.

### Loading Rules

1. The built-in values come first.
2. The configuration file overrides them key by key: a key the file does not
   set keeps its built-in value (the file no longer replaces the defaults as
   a whole). A key set to nothing (`key:`) keeps it too.
3. The `ANIX_CONTROL_*` variables override the file.

The YAML is read strictly. An operator key is applied. A key of the
[deprecated list](#deprecated-keys) is reported once, with where it is set
and why, and is then either still read (its feature still exists and
ignoring it would change what a running deployment does) or ignored (the
built-in value stands). Any other key stops the start, naming the file, the
line and the closest known key:

```text
config.yaml:3: unknown configuration key tls.mod; did you mean tls.mode? (docs/reference/configuration.md lists the keys)
```

While PostgreSQL is supported (until M2, P4), the `database` section keeps
its 4.x meaning: with a configuration file only the file's `database` keys
count (an empty `driver` is SQLite at `config/data/v2board.db` next to the
file); without a file the container defaults apply (PostgreSQL on
`127.0.0.1`). `app.subscribe_path` is read as `subscribe_path`; setting both
to different values stops the start.

### Environment Variables

Every operator key has a variable: `ANIX_CONTROL_` plus the upper-cased path
with `_` for `.` (`grpc.public_name` is `ANIX_CONTROL_GRPC_PUBLIC_NAME`);
[`environment-variables.md`](environment-variables.md) lists them and
`anix-control -print-env` prints them.

- `NAME_FILE=/path` reads the value from a file (Docker and Kubernetes
  secrets). Setting both `NAME` and `NAME_FILE` is an error.
- Booleans accept `true`/`false`/`1`/`0`; lists are comma-separated.
- The variables of deprecated keys keep working the way their keys do: still
  read with the warning, or warned about and ignored (an ignored `NAME_FILE`
  is never opened). Map-valued keys (`forward_runtime.nftables_ansible.environment`
  and `extra_vars`) have no variable.
- An unknown `ANIX_CONTROL_*` variable is reported with the closest known
  one but does not stop the start: the environment is shared with other
  tools.

## The Root Key

Control keeps one secret: 32 random bytes in `root.key` in the data
directory (mode 0600, the directory 0700, owned by the user Control runs
as). It is never logged, never part of a status answer, and backups must
carry it with the database (P10).

| Value | Where it comes from |
|------|------|
| the root key | `root.key` when it exists. Otherwise a configured `module_runtime.ca_kek` is adopted: the root key takes its value, so the CA keys, ACME keys, DNS credentials and join tokens it sealed stay readable, and the server writes it to `root.key`. Otherwise the server generates a new key at its first start. |
| the module and forward link CA keys, DNS provider credentials | sealed with the root key (AES-256-GCM), as with `module_runtime.ca_kek` before |
| node group join tokens | sealed with HKDF-SHA256(root key, `anixops node-group join token v1`), unchanged |
| ACME account and certificate keys (`v5_acme_cache`) | sealed with HKDF-SHA256(root key, `anixops acme cache v1`), unchanged |
| the JWT secret (`jwt.secret`) | a value the configuration sets (explicitly, not a template value) is adopted: recorded in `root.key` at the next start and kept until a rotation, also after the configuration drops it. Otherwise standard base64 of HKDF-SHA256(root key, `anixops jwt v1`). |
| the identity signing key KEK (`identity.kek`) | adopted the same way; otherwise standard base64 of HKDF-SHA256(root key, `anixops identity kek v1`) |

HKDF-SHA256 here is RFC 5869 without a salt, 32 bytes;
`internal/rootkey` pins the outputs in tests so that a derivation cannot
drift.

**The file.** Text, versioned by its first field:

```text
# AnixOps Control root key. Keep it secret; back it up with the database.
anixops-root-key: v1
key: <the 32 bytes, standard base64>
adopted-jwt-secret: <the adopted jwt.secret, standard base64 of its bytes>
adopted-identity-kek: <the adopted identity.kek, standard base64 of its 32 bytes>
```

The two `adopted-` fields are optional; `#` lines and empty lines are
ignored, and any other field, a repeated field or another version stops the
start (a damaged file is restored from the backup, never regenerated).

**Who writes it.** Only the server: at its first start, or when it adopts a
legacy key, and when the configuration sets a `jwt.secret` or `identity.kek`
that `root.key` does not record yet. Commands (`doctor`, `tls`, `agent`,
`migrate`, ...) only read it, so one run as root never creates a file the
service cannot read. A data directory the server cannot write stops the
first start with what to do (`install -d -m 0700 -o <user> <dir>`, or set
`data_dir`); with a configured `module_runtime.ca_kek` Control keeps using
that key and warns instead. A `root.key` other users can read is restricted
to 0600 (or the start stops). A configured `module_runtime.ca_kek` that is
not the key in `root.key` stops the start: only one of them sealed the
database.

**Every start checks it.** After the schema migrations and before anything
uses the key, Control opens the sealed key of every current and next module
CA and forward link CA with it; a database without a CA must open its
sealed ACME entry and a join token instead. A key that cannot open them
stops the start:

```text
the root key /var/lib/anix-control/root.key does not open the module CA key stored in this database (...). The database was sealed with another root key: restore the root.key that belongs to this database (from the backup taken with it) into /var/lib/anix-control/root.key, or start with the database that belongs to this key.
```

`anix-control doctor` reports the same check (`root_key`) instead of
stopping.

## Checking And Rewriting A Configuration

```bash
anix-control config check [--file /etc/anix-control/config.yaml]
anix-control config rewrite --in <old> --out <new> [--data-dir <dir>] [--force]
```

`config check` loads the file and the `ANIX_CONTROL_*` variables the way the
server does, prints every deprecated key with what happens to it, the
resulting layout and what the next start does with the root key, and exits
1 when the server would refuse to start. It opens no database.

`config rewrite` turns an earlier file into the operator keys, the way an
upgrade does (`docs/UPGRADE.md`, "Configuration rewrite on upgrade"):

- `domain` from `domain`, else the host of `agent_install.public_url`, else
  `tls.domain`;
- `grpc.tls_cert_file` and `grpc.tls_key_file` become `tls.cert_file` and
  `tls.key_file` with `tls.mode: external` when the old file did not use
  ACME (a 4.x file: no `domain`, no `tls.mode`); a 4.x file that served
  https itself (`tls.enable` with `tls.cert_file`) becomes `tls.mode: files`;
- the host of `agent_install.grpc_target` becomes `grpc.public_name` when it
  is not `domain`, so nodes keep dialling the same name;
- `grpc.port`, `subscribe_path` (or `app.subscribe_path`), `listen` and
  `data_dir` only when they are not the defaults;
- the `database` section is kept while PostgreSQL is supported, a relative
  SQLite path made absolute, so the panel keeps its database; so are a
  closed or restricted registration (`auth.registration.*`) and a cluster
  name other than `default` (`module_runtime.cluster`), while they are read;
- every other key is dropped and printed with the reason.

With `--out` equal to `--in` the old file is kept as
`<name>.bak-<UTC time>` (its mode and owner kept). An existing other
`--out` is refused. `module_runtime.ca_kek` and `identity.kek` may be dropped
only when `<data_dir>/root.key` holds them (start the new Control once with
the old file: it adopts them); otherwise nothing is written, unless
`--force`. Dropping `jwt.secret` that `root.key` does not hold signs every
user out once.

## Deprecated Keys

Every key of the 4.2 configuration that is not an operator key, and the
sections older templates carried. "still read" keeps working, with the
warning, until the milestone that removes the key's feature; "ignored" is
warned about and has no effect (the built-in value, today's default,
stands). The list is `internal/config/deprecated_keys.yaml`.

<!-- BEGIN GENERATED: deprecated configuration keys -->
| Key | Variable | Status | Removed in | Why |
|-----|----------|--------|------------|-----|
| `admin.email` | `ANIX_CONTROL_ADMIN_EMAIL` | ignored | M2 (P9) | the first administrator claims the panel with a one-time link (anix-control admin claim-link); existing administrators are not affected |
| `admin.password` | `ANIX_CONTROL_ADMIN_PASSWORD` | ignored | M2 (P9) | no administrator password is configured any more; the first administrator claims the panel with a one-time link (anix-control admin claim-link) and existing administrators keep theirs, so remove it |
| `agent_control.legacy_sunset` | `ANIX_CONTROL_AGENT_CONTROL_LEGACY_SUNSET` | ignored | M5 | the legacy Agent transports send no Sunset date; Agents enroll (agent_control.mtls is fixed to required) |
| `agent_control.mtls` | `ANIX_CONTROL_AGENT_CONTROL_MTLS` | ignored | M5 | fixed to required, the 4.2 default; the other modes are removed with the legacy Agent transports |
| `agent_install.agent_version` | `ANIX_CONTROL_AGENT_INSTALL_AGENT_VERSION` | still read | M4 (S9) | pins the Agent release nodes install until Control carries its Agent release manifest |
| `agent_install.artifact_dir` | `ANIX_CONTROL_AGENT_INSTALL_ARTIFACT_DIR` | still read | M4 (N30) | serves Agent releases from this directory until Control downloads and verifies them itself |
| `agent_install.cn_mirror_url` | `ANIX_CONTROL_AGENT_INSTALL_CN_MIRROR_URL` | still read | M4 (N30) | the mainland download mirror of Agent releases, until Control serves the releases itself |
| `agent_install.grpc_target` | `ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET` | still read | M2 (P11) | overrides the gRPC address nodes dial, which is derived from grpc.public_name (or domain) and grpc.port; use `grpc.public_name` |
| `agent_install.public_url` | `ANIX_CONTROL_AGENT_INSTALL_PUBLIC_URL` | still read | M2 (P11) | overrides the panel address nodes use, which is https://<domain>; config rewrite turns its host into domain; use `domain` |
| `agent_install.signature_file` | `ANIX_CONTROL_AGENT_INSTALL_SIGNATURE_FILE` | still read | M2 (P11) | the release signature of the Agent install script, until the signature is built into the binary |
| `alerts.ca_expiry_days` | `ANIX_CONTROL_ALERTS_CA_EXPIRY_DAYS` | ignored | M2 (P8) | a built-in value (60 days) |
| `alerts.check_interval` | `ANIX_CONTROL_ALERTS_CHECK_INTERVAL` | ignored | M2 (P8) | a built-in value (15m) |
| `alerts.enabled` | `ANIX_CONTROL_ALERTS_ENABLED` | ignored | M2 (P8) | the alert monitor always runs |
| `alerts.leaf_expiry_days` | `ANIX_CONTROL_ALERTS_LEAF_EXPIRY_DAYS` | ignored | M2 (P8) | a built-in value (14 days) |
| `alerts.phase_stuck_after` | `ANIX_CONTROL_ALERTS_PHASE_STUCK_AFTER` | ignored | M2 (P8) | a built-in value (72h) |
| `alerts.renotify_interval` | `ANIX_CONTROL_ALERTS_RENOTIFY_INTERVAL` | ignored | M2 (P8) | a built-in value (24h) |
| `app.api_token` | `ANIX_CONTROL_APP_API_TOKEN` | still read | M5 (N9) | the legacy shared token of /flow/upload and the internal v2 routes, removed with them; administrators use admin API tokens |
| `app.edition` | `ANIX_CONTROL_APP_EDITION` | ignored | M2 (P6) | there are no editions; orders, payments and affiliate are built in |
| `app.name` | `ANIX_CONTROL_APP_NAME` | ignored | M2 (P8) | nothing reads it |
| `app.subscribe_path` | `ANIX_CONTROL_APP_SUBSCRIBE_PATH` | still read | M5 | renamed; config rewrite moves it to subscribe_path; use `subscribe_path` |
| `app.traffic_log_enable` | `ANIX_CONTROL_APP_TRAFFIC_LOG_ENABLE` | ignored | M2 (P8) | nothing reads it |
| `app.version` | `ANIX_CONTROL_APP_VERSION` | ignored | M2 (P8) | nothing reads it; the version is the binary's |
| `auth.login_rate_limit.enabled` | `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_ENABLED` | ignored | M2 (P8) | login rate limiting is always on |
| `auth.login_rate_limit.lockout_seconds` | `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_LOCKOUT_SECONDS` | ignored | M2 (P8) | a built-in value (600) |
| `auth.login_rate_limit.max_attempts` | `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_MAX_ATTEMPTS` | ignored | M2 (P8) | a built-in value (6) |
| `auth.login_rate_limit.window_seconds` | `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_WINDOW_SECONDS` | ignored | M2 (P8) | a built-in value (300) |
| `auth.register_rate_limit.enabled` | `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_ENABLED` | ignored | M2 (P8) | registration rate limiting is always on |
| `auth.register_rate_limit.lockout_seconds` | `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_LOCKOUT_SECONDS` | ignored | M2 (P8) | a built-in value (3600) |
| `auth.register_rate_limit.max_attempts` | `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_MAX_ATTEMPTS` | ignored | M2 (P8) | a built-in value (5) |
| `auth.register_rate_limit.window_seconds` | `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_WINDOW_SECONDS` | ignored | M2 (P8) | a built-in value (3600) |
| `auth.registration.allowed_email_domains` | `ANIX_CONTROL_AUTH_REGISTRATION_ALLOWED_EMAIL_DOMAINS` | still read | M3 (N35) | the registration policy becomes an administrator setting; until then ignoring it would open registration to every address |
| `auth.registration.blocked_email_domains` | `ANIX_CONTROL_AUTH_REGISTRATION_BLOCKED_EMAIL_DOMAINS` | still read | M3 (N35) | the registration policy becomes an administrator setting; until then ignoring it would open registration to every address |
| `auth.registration.enabled` | `ANIX_CONTROL_AUTH_REGISTRATION_ENABLED` | still read | M3 (N35) | the registration policy becomes an administrator setting; until then ignoring it would reopen closed registration |
| `auth.registration.require_invite` | `ANIX_CONTROL_AUTH_REGISTRATION_REQUIRE_INVITE` | still read | M3 (N35) | the registration policy becomes an administrator setting; until then ignoring it would drop the invite requirement |
| `cache.driver` | `ANIX_CONTROL_CACHE_DRIVER` | ignored | M2 (P8) | nothing reads it; the cache is in memory |
| `cache.redis_db` | `ANIX_CONTROL_CACHE_REDIS_DB` | ignored | M2 (P8) | nothing reads it; the cache is in memory |
| `cache.redis_host` | `ANIX_CONTROL_CACHE_REDIS_HOST` | ignored | M2 (P8) | nothing reads it; the cache is in memory |
| `cache.redis_password` | `ANIX_CONTROL_CACHE_REDIS_PASSWORD` | ignored | M2 (P8) | nothing reads it; the cache is in memory |
| `cache.redis_port` | `ANIX_CONTROL_CACHE_REDIS_PORT` | ignored | M2 (P8) | nothing reads it; the cache is in memory |
| `database.conn_max_lifetime` | `ANIX_CONTROL_DATABASE_CONN_MAX_LIFETIME` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.database` | `ANIX_CONTROL_DATABASE_DATABASE` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.driver` | `ANIX_CONTROL_DATABASE_DRIVER` | still read | M2 (P4) | selects SQLite or PostgreSQL until PostgreSQL support ends; ignoring it would switch databases |
| `database.dsn` | `ANIX_CONTROL_DATABASE_DSN` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.host` | `ANIX_CONTROL_DATABASE_HOST` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.log_level` | `ANIX_CONTROL_DATABASE_LOG_LEVEL` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.max_idle_conns` | `ANIX_CONTROL_DATABASE_MAX_IDLE_CONNS` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.max_open_conns` | `ANIX_CONTROL_DATABASE_MAX_OPEN_CONNS` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.password` | `ANIX_CONTROL_DATABASE_PASSWORD` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.port` | `ANIX_CONTROL_DATABASE_PORT` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.sslmode` | `ANIX_CONTROL_DATABASE_SSLMODE` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.timezone` | `ANIX_CONTROL_DATABASE_TIMEZONE` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `database.username` | `ANIX_CONTROL_DATABASE_USERNAME` | still read | M2 (P4) | the database is SQLite in the data directory after PostgreSQL support ends |
| `env` | `ANIX_CONTROL_ENV` | ignored | M2 (P8) | Control always runs as production (its only effect was requiring jwt.secret, which is now derived); start a development instance with -debug |
| `forward.anixops_experimental` | `ANIX_CONTROL_FORWARD_ANIXOPS_EXPERIMENTAL` | ignored | M2 (P8) | the experimental AnixOps engine stays off (a built-in value) |
| `forward_runtime.backend` | `ANIX_CONTROL_FORWARD_RUNTIME_BACKEND` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.clean_agent.action_timeout_seconds` | `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_ACTION_TIMEOUT_SECONDS` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.clean_agent.heartbeat_interval_seconds` | `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_HEARTBEAT_INTERVAL_SECONDS` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.clean_agent.legacy_bridge_enabled` | `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_LEGACY_BRIDGE_ENABLED` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.clean_agent.public_url` | `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_PUBLIC_URL` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.clean_agent.token_expire_seconds` | `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_TOKEN_EXPIRE_SECONDS` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.gost_stats.error_log_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_GOST_STATS_ERROR_LOG_INTERVAL` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.gost_stats.idle_poll_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_GOST_STATS_IDLE_POLL_INTERVAL` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.gost_stats.poll_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_GOST_STATS_POLL_INTERVAL` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.iptables_ansible.*` | none | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.jobs.batch_size` | `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_BATCH_SIZE` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.jobs.error_log_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_ERROR_LOG_INTERVAL` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.jobs.idle_poll_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_IDLE_POLL_INTERVAL` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.jobs.poll_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_POLL_INTERVAL` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.jobs.timeout_seconds` | `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_TIMEOUT_SECONDS` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.latency.concurrency` | `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_CONCURRENCY` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.latency.dial_timeout` | `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_DIAL_TIMEOUT` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.latency.dials` | `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_DIALS` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.latency.error_log_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_ERROR_LOG_INTERVAL` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.latency.idle_poll_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_IDLE_POLL_INTERVAL` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.latency.poll_interval` | `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_POLL_INTERVAL` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.latency.retention_days` | `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_RETENTION_DAYS` | ignored | M2 (P8) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.apply_playbook` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_APPLY_PLAYBOOK` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.become` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_BECOME` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.command` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_COMMAND` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.environment` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_ENVIRONMENT` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.extra_vars` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_EXTRA_VARS` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.inventory` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_INVENTORY` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.remove_playbook` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_REMOVE_PLAYBOOK` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.target_pattern` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_TARGET_PATTERN` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.timeout_seconds` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_TIMEOUT_SECONDS` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nftables_ansible.working_dir` | `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_WORKING_DIR` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nodex.base_url` | `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_BASE_URL` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nodex.timeout_seconds` | `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nodex.token` | `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_TOKEN` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `forward_runtime.nodex_mode` | `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_MODE` | ignored | M5 (N5) | the flux forward runtime (gost, NodeX, Ansible and the clean agent) was removed; the key does nothing, so remove it |
| `frontend.enable` | `ANIX_CONTROL_FRONTEND_ENABLE` | still read | M2 (P11) | serves the web app from frontend.path until the web app is built into the binary |
| `frontend.path` | `ANIX_CONTROL_FRONTEND_PATH` | still read | M2 (P11) | the web app's directory until the web app is built into the binary |
| `frontend.port` | `ANIX_CONTROL_FRONTEND_PORT` | still read | M2 (P11) | a second plain listener of tls.mode external while listen is unset; config rewrite does not carry it, point the reverse proxy at listen; use `listen` |
| `grpc.api_token` | `ANIX_CONTROL_GRPC_API_TOKEN` | still read | M5 (N9) | the legacy administrator token of the v2board gRPC services, removed with them |
| `grpc.enabled` | `ANIX_CONTROL_GRPC_ENABLED` | still read | M2 (P11) | turns the gRPC listener on without domain; with domain it always runs |
| `grpc.host` | `ANIX_CONTROL_GRPC_HOST` | still read | M2 (P11) | the gRPC listener's address, until it listens on every address |
| `grpc.tls_cert_file` | `ANIX_CONTROL_GRPC_TLS_CERT_FILE` | still read | M2 (P11) | the gRPC listener's own certificate; config rewrite turns it into tls.cert_file with tls.mode external; use `tls.cert_file` |
| `grpc.tls_key_file` | `ANIX_CONTROL_GRPC_TLS_KEY_FILE` | still read | M2 (P11) | the gRPC listener's own certificate key; config rewrite turns it into tls.key_file with tls.mode external; use `tls.key_file` |
| `identity.kek` | `ANIX_CONTROL_IDENTITY_KEK` | still read | M5 | derived from the root key; a set value is adopted into root.key at the next start and stays in use until a rotation, so the file need not keep it |
| `jwt.expire` | `ANIX_CONTROL_JWT_EXPIRE` | ignored | M2 (P8) | a built-in value (86400 seconds) |
| `jwt.secret` | `ANIX_CONTROL_JWT_SECRET` | still read | M5 | derived from the root key; a set value is adopted into root.key at the next start and stays in use until a rotation, so the file need not keep it |
| `log.file_path` | `ANIX_CONTROL_LOG_FILE_PATH` | ignored | M2 (P8) | nothing reads it; Control logs to standard output |
| `log.format` | `ANIX_CONTROL_LOG_FORMAT` | still read | M2 (P11) | JSON lines for container log collectors, until the container files are removed |
| `log.level` | `ANIX_CONTROL_LOG_LEVEL` | still read | M2 (P11) | the JSON log level, until the container files are removed; -debug turns on debug logging |
| `log.max_age` | `ANIX_CONTROL_LOG_MAX_AGE` | ignored | M2 (P8) | nothing reads it |
| `log.max_backups` | `ANIX_CONTROL_LOG_MAX_BACKUPS` | ignored | M2 (P8) | nothing reads it |
| `log.max_size` | `ANIX_CONTROL_LOG_MAX_SIZE` | ignored | M2 (P8) | nothing reads it |
| `log.output` | `ANIX_CONTROL_LOG_OUTPUT` | ignored | M2 (P8) | nothing reads it; Control logs to standard output |
| `module_runtime.bind_timeout` | `ANIX_CONTROL_MODULE_RUNTIME_BIND_TIMEOUT` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.ca_kek` | `ANIX_CONTROL_MODULE_RUNTIME_CA_KEK` | still read | M5 | the root key; adopted into root.key in data_dir at the next start, after which it must match that file and can be removed |
| `module_runtime.cert_file` | `ANIX_CONTROL_MODULE_RUNTIME_CERT_FILE` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.cert_lifetime` | `ANIX_CONTROL_MODULE_RUNTIME_CERT_LIFETIME` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.cluster` | `ANIX_CONTROL_MODULE_RUNTIME_CLUSTER` | still read | M2 (P4) | names the cluster in every module and Agent identity; ignoring a set value would change the identities of enrolled Agents |
| `module_runtime.database_host` | `ANIX_CONTROL_MODULE_RUNTIME_DATABASE_HOST` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.enabled` | `ANIX_CONTROL_MODULE_RUNTIME_ENABLED` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.key_file` | `ANIX_CONTROL_MODULE_RUNTIME_KEY_FILE` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.listen` | `ANIX_CONTROL_MODULE_RUNTIME_LISTEN` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.pki` | `ANIX_CONTROL_MODULE_RUNTIME_PKI` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `module_runtime.trust_bundle_file` | `ANIX_CONTROL_MODULE_RUNTIME_TRUST_BUNDLE_FILE` | still read | M2 (P4) | the remote module runtime, removed with PostgreSQL support |
| `package_routes.default_mode` | `ANIX_CONTROL_PACKAGE_ROUTES_DEFAULT_MODE` | ignored | M2 (P7) | route modes come only from the rehearsed list (config/package-route-defaults.json) |
| `plugins.control_execution_enabled` | `ANIX_CONTROL_PLUGINS_CONTROL_EXECUTION_ENABLED` | ignored | M2 (P7) | the built-in packages always run; Control starts their hosts at every start |
| `plugins.control_host_artifact_dir` | `ANIX_CONTROL_PLUGINS_CONTROL_HOST_ARTIFACT_DIR` | still read | M2 (P11) | where package hosts' files are kept, until the systemd unit gives Control its directories |
| `plugins.control_host_max_request_bytes` | `ANIX_CONTROL_PLUGINS_CONTROL_HOST_MAX_REQUEST_BYTES` | ignored | M2 (P8) | a built-in value (1 MiB) |
| `plugins.control_host_max_response_bytes` | `ANIX_CONTROL_PLUGINS_CONTROL_HOST_MAX_RESPONSE_BYTES` | ignored | M2 (P8) | a built-in value (1 MiB) |
| `plugins.control_host_request_timeout` | `ANIX_CONTROL_PLUGINS_CONTROL_HOST_REQUEST_TIMEOUT` | ignored | M2 (P8) | a built-in value (30s) |
| `plugins.control_host_runtime_dir` | `ANIX_CONTROL_PLUGINS_CONTROL_HOST_RUNTIME_DIR` | still read | M2 (P11) | where package hosts' sockets live, until the systemd unit gives Control its runtime directory |
| `plugins.control_host_startup_timeout` | `ANIX_CONTROL_PLUGINS_CONTROL_HOST_STARTUP_TIMEOUT` | ignored | M2 (P8) | a built-in value (5s) |
| `plugins.control_host_websocket_session_timeout` | `ANIX_CONTROL_PLUGINS_CONTROL_HOST_WEBSOCKET_SESSION_TIMEOUT` | ignored | M2 (P8) | a built-in value (24h) |
| `plugins.control_poll_interval` | `ANIX_CONTROL_PLUGINS_CONTROL_POLL_INTERVAL` | ignored | M2 (P8) | a built-in value (5s) |
| `plugins.dispatch_enabled` | `ANIX_CONTROL_PLUGINS_DISPATCH_ENABLED` | ignored | M2 (P7) | Agent plugin dispatch is off until the Agent release manifest brings the Agent plugins (M4) |
| `plugins.dispatch_poll_interval` | `ANIX_CONTROL_PLUGINS_DISPATCH_POLL_INTERVAL` | ignored | M2 (P7) | Agent plugin dispatch is off until the Agent release manifest brings the Agent plugins (M4) |
| `plugins.identity_bootstrap_package_dir` | `ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR` | still read | M2 (P11) | imports identity-platform into a development binary that embeds no packages; a release binary ignores it |
| `plugins.official_public_key` | `ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY` | still read | M2 (P11) | the key the built-in packages verify against (development builds sign with their own), until the package index carries it |
| `plugins.topology_execution_enabled` | `ANIX_CONTROL_PLUGINS_TOPOLOGY_EXECUTION_ENABLED` | ignored | M2 (P7) | topology deployment execution is off until the Agent release manifest brings the Agent plugins (M4) |
| `plugins.topology_poll_interval` | `ANIX_CONTROL_PLUGINS_TOPOLOGY_POLL_INTERVAL` | ignored | M2 (P7) | topology deployment execution is off until the Agent release manifest brings the Agent plugins (M4) |
| `server.cors.allow_credentials` | `ANIX_CONTROL_SERVER_CORS_ALLOW_CREDENTIALS` | ignored | M2 (P8) | a built-in value (false) |
| `server.cors.allowed_headers` | `ANIX_CONTROL_SERVER_CORS_ALLOWED_HEADERS` | ignored | M2 (P8) | a built-in value (Content-Type, Authorization, X-API-Key) |
| `server.cors.allowed_methods` | `ANIX_CONTROL_SERVER_CORS_ALLOWED_METHODS` | ignored | M2 (P8) | a built-in value (GET, POST, PUT, DELETE, OPTIONS, PATCH) |
| `server.cors.allowed_origins` | `ANIX_CONTROL_SERVER_CORS_ALLOWED_ORIGINS` | ignored | M2 (P8) | a built-in value (any origin, without credentials) |
| `server.cors.max_age` | `ANIX_CONTROL_SERVER_CORS_MAX_AGE` | ignored | M2 (P8) | a built-in value (86400) |
| `server.host` | `ANIX_CONTROL_SERVER_HOST` | still read | M2 (P11) | the plain listener of tls.mode external while listen is unset; config rewrite does not carry it, set listen; use `listen` |
| `server.mode` | `ANIX_CONTROL_SERVER_MODE` | ignored | M2 (P8) | release mode is built in; start a development instance with -debug |
| `server.port` | `ANIX_CONTROL_SERVER_PORT` | still read | M2 (P11) | the plain listener of tls.mode external while listen is unset; config rewrite does not carry it, set listen; use `listen` |
| `server.read_timeout` | `ANIX_CONTROL_SERVER_READ_TIMEOUT` | ignored | M2 (P8) | a built-in value (30 seconds) |
| `server.shutdown_drain_delay` | `ANIX_CONTROL_SERVER_SHUTDOWN_DRAIN_DELAY` | ignored | M2 (P8) | a built-in value (5s) |
| `server.trusted_proxies` | `ANIX_CONTROL_SERVER_TRUSTED_PROXIES` | still read | M2 (P11) | the reverse proxies of a container deployment; afterwards tls.mode acme trusts none and tls.mode external trusts loopback |
| `server.write_timeout` | `ANIX_CONTROL_SERVER_WRITE_TIMEOUT` | ignored | M2 (P8) | a built-in value (30 seconds) |
| `tls.domain` | `ANIX_CONTROL_TLS_DOMAIN` | ignored | M2 (P8) | it was only logged; config rewrite turns it into domain; use `domain` |
| `tls.enable` | `ANIX_CONTROL_TLS_ENABLE` | still read | M2 (P11) | not used (TLS follows tls.mode); read only to refuse the 4.x combination of tls.enable, tls.cert_file and no tls.mode, which served https on the web port |
<!-- END GENERATED -->

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
listen: ""                   # external only: the plain http address; empty: 127.0.0.1:8080
grpc:
  port: 50051                # node gRPC port
  public_name: ""            # the name nodes dial when domain is behind a CDN; empty: domain
```

| Key | Default | Meaning |
|------|------|------|
| `domain` (`ANIX_CONTROL_DOMAIN`) | empty | The panel's DNS name (no scheme, port or address). The install script is `https://<domain>/install.sh`, the ACME certificate's name in `acme` mode and the gRPC name unless `grpc.public_name` is set. With it the gRPC listener is always on. The older `tls.domain` is ignored (it was only ever logged); `config rewrite` turns it into `domain`. |
| `tls.mode` (`ANIX_CONTROL_TLS_MODE`) | empty | `acme`: Control obtains and renews a publicly trusted certificate for `domain` and serves https on `:443` and http on `:80` (the ACME HTTP-01 answers; everything else is redirected to https). `files`: https on `:443` with `tls.cert_file`/`tls.key_file`, reloaded when they change; `:80` redirects. `external`: a CDN or a reverse proxy terminates TLS and sends plain http to `listen`. Empty: `acme` with `domain`, `external` without. |
| `tls.cert_file`, `tls.key_file` | empty | PEM certificate chain and key. `files` mode: the web certificate (and the gRPC one for the same name). `external` mode: the gRPC listener's certificate, instead of one obtained over ACME. Both or neither. |
| `listen` (`ANIX_CONTROL_LISTEN`) | empty | `external` mode's plain http address (`host:port`; an address, not a name). Loopback is the safe choice: only the proxy on this machine reaches it. Empty: `127.0.0.1:8080`. While they are still read (until M2, P11), the deprecated `server.host`, `server.port` and `frontend.port` place it instead when they are set: `server.host:server.port` (an empty host is loopback, port 0 is 8080), plus `server.host:frontend.port` with one handler on both. |
| `grpc.port` (`ANIX_CONTROL_GRPC_PORT`) | `50051` | Node gRPC port. `/install/agent.env` hands nodes `<gRPC name>:<grpc.port>`. |
| `grpc.public_name` (`ANIX_CONTROL_GRPC_PUBLIC_NAME`) | empty (`domain`) | The DNS-only name nodes dial for gRPC when `domain` is proxied by a CDN (the CDN mode below). Control obtains a publicly trusted certificate for it over ACME. |
| `grpc.edge_proxies` (`ANIX_CONTROL_GRPC_EDGE_PROXIES`) | empty | Addresses (IPs or CIDRs) of a reverse proxy that terminates the nodes' TLS on another machine. Non-empty: the gRPC listener is plain HTTP/2 and admits these addresses only (any other connection is closed before the gRPC layer), reads the node's certificate from the `x-anix-client-cert` header and its address from `x-forwarded-for`, and only from these addresses. `tls.mode` must be `external`; `grpc.tls_cert_file` and `tls.cert_file` must be unset. Nodes dial the proxy: set `agent_install.grpc_target` to its name and port 443. The design: [`design/edge-grpc`](../design/edge-grpc/README.md). |

With `grpc.edge_proxies` the listener holds no certificate and this order does
not apply: the proxy's certificate is the one nodes verify.

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
`0003_acme_cache` creates it), sealed with the [root key](#the-root-key),
so a restore or a move to another machine (with `root.key`) does not
request them again. No HSTS header is sent over Control's own TLS:
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

The deprecated `agent_install.public_url` and `agent_install.grpc_target`
still override the derived `https://<domain>` and `<gRPC name>:<grpc.port>`
until M2 (P11).

`tls.enable` has no effect except one: with `tls.cert_file` and no
`tls.mode` Control refuses to start (that combination served https on the
web port, which is now `tls.mode: files` on 443). `tls.domain` is ignored, so
an existing configuration behind a reverse proxy does not move to `acme` by
itself. `frontend.enable`, `frontend.port` and `server.host`/`server.port`
remain the legacy external layout described above until M2 (P11).

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

Container-related settings (deprecated: still read until the container
files go in M2, P11):

| Key | Default | Notes |
|------|------|------|
| `log.format` | `text` | `json` writes one JSON object per line to stdout (standard log lines, access logs, GORM); `text` keeps the historical output |
| `log.level` | `info` | minimum level for structured records in `json` format; standard log lines are always written; `-debug` sets `debug` |

On shutdown Control keeps serving for 5 seconds after `/readyz` starts
failing (built in; `server.shutdown_drain_delay` is ignored). The `-debug`
flag runs a development instance (gin debug mode, debug logging); `env` and
`server.mode` are ignored, Control always runs as production.

PostgreSQL connection settings (deprecated: still read until PostgreSQL
support ends in M2, P4):

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
| `forward.anixops_experimental` (ignored: the built-in value is `false`) | `false` | Lets routes use the experimental AnixOps engine and link (`ENGINE_ANIXOPS`, `LINK_SECURITY_ANIXOPS`), the secure relay between forward nodes (`docs/architecture/anixops-protocol.md`). Off, validation and the planner refuse them (`engine_not_enabled`). The Agents need their own switch, `Forward.AnixOps.Enable` in the Agent's JSON configuration (`"Forward": {"AnixOps": {"Enable": true}}`), to advertise the engine, or the planner refuses their nodes. v4.2 ships it as a prototype whose wire format (ALPN `anixops/0`) may change without notice; v4.3 freezes it. |

## Built-in Packages

Every official package is built into the binary and installed, upgraded and
enabled at every start
([`../architecture/plugin-kernel-contract.md`](../architecture/plugin-kernel-contract.md#built-in-packages)).
Two deprecated keys, still read until M2 (P11), bear on it:

| Key | Default | Notes |
|------|------|------|
| `plugins.official_public_key` (`ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY`) | the AnixOps release root | Every built-in package must verify against it; a package that does not is left out (`builtin_package_invalid`) and Control starts without it. A development build (`make build`) is signed with a development key: `make run` sets the variable to it (`make dev-package-key` prints it). |
| `plugins.identity_bootstrap_package_dir` | empty | Ignored by a binary with built-in packages (every release). A development build without them still imports `identity-platform` from this directory at start. |

`app.edition` is gone: there are no community and commercial editions.
Orders, payments and affiliate are installed like every other package, and
a configuration that still sets the key loads (the key is ignored with a
warning). The
public `GET /api/v4/public/config` keeps its `edition` (`commercial`) and
`hidden_packages` (empty) fields for the web app.

The package hosts always run, and Agent plugin dispatch and topology
execution are off until M4; none of them has a setting (their 4.2 keys are
in the [deprecated list](#deprecated-keys), ignored).

## Unified Forward Runtime Layout

The `forward_runtime` keys belong to the flux forward runtime, which M5
removes (N5): they are deprecated and still read until then, with a
warning; `forward_runtime.latency.*` is built in (ignored), and
`forward_runtime.iptables_ansible` was never a key (ignored; the iptables
backend runs with `nftables_ansible`). Put the runtime selection in the
configuration file:

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
- the `iptables_ansible` backend runs with the `nftables_ansible` block (a `forward_runtime.iptables_ansible` section is ignored)

## Path Resolution

The local ansible block (`forward_runtime.nftables_ansible`) has these path fields normalized on startup:

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
built-in limits (1 MiB each) bound the bodies that cross the package
boundary; their 4.2 keys `plugins.control_host_max_request_bytes` and
`control_host_max_response_bytes` are ignored, like the other
`plugins.control_host_*` timeouts and `plugins.control_poll_interval`:

| Limit | Enforced by | Error when exceeded |
|------|------|------|
| request body, 1 MiB | the v2 gateway, before dispatch | `413` `plugin_request_too_large` |
| response body, 1 MiB | the package bridge (legacy handler body), the package-host SDK (package response body), the kernel host client | `502` `plugin_response_too_large` |

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

AnixOps Agents authenticate on the gRPC listener with enrolled client
certificates only. The legacy agent channels (the API-key stream, the
`/api/v2/agent/*` and `/api/v2/node/*` REST and WebSocket paths and the
`off`, `optional` and `preferred` modes) are removed; the 4.2 keys
`agent_control.mtls` and `agent_control.legacy_sunset` are ignored and can be
deleted from the configuration.

- A call without a valid client certificate is refused: gRPC
  `Unauthenticated` with the trailer `x-anix-error-code: agent_mtls_required`;
  `Enroll` takes only one-time enrollment credentials. UniProxy
  (`/api/v1|v2/server/UniProxy/*`) and the v2board gRPC services, which
  third-party node software (XrayR, V2bX) uses, are not affected.
- Agents get certificates from `anix.agent.v1.AgentEnrollment`, signed by the
  built-in CA, whose key the [root key](#the-root-key) seals; every server
  has it (with the deprecated `module_runtime.pki: external` the kernel holds
  no CA key and Agents cannot enroll).
- The gRPC listener needs TLS (`domain`, or the deprecated `grpc.enabled`
  with `grpc.tls_cert_file` and `grpc.tls_key_file`) for agents to enroll and
  connect. Without it Control starts and logs a `WARNING: Agent transports:
  ...` line that no Agent can connect.
- Startup also warns (`WARNING: Agent transports: N enabled node(s) never
  enrolled ...`) when the transport inventory has enabled nodes that never
  enrolled. It never stops the start.
- `anix-control agents transports --check-required` lists them (exit status
  3 while an enabled node has never enrolled; `summary.ready_for_required` in
  `GET /api/v4/kernel/agents/transports`), as do the node list's connection
  and certificate columns. Every node it lists must first run an enrolled
  agent, or be disabled
  ([UPGRADE](../UPGRADE.md#agent-transports-v42-requires-enrolled-agents)).
  `/metrics` has `anixops_agent_legacy_refused_total{path}`.
- Enrollment credentials: `anix-control agent token create -node proxy-12`
  or `POST /api/v4/kernel/agents/enrollment-tokens`. Details in
  [`../architecture/module-runtime.md`](../architecture/module-runtime.md#agent-pki)
  and [`../architecture/node-ops-service.md`](../architecture/node-ops-service.md)
  section 5.6.

## Agent Install (One-Command Onboarding)

`agent_install` (deprecated: `public_url`, `grpc_target` and
`signature_file` are still read until M2, P11, `agent_version`,
`artifact_dir` and `cn_mirror_url` until M4) configures the install
commands Control renders, a node
group's (`GET /api/v4/kernel/node-groups/:id/command`) and an existing
node's (`POST /api/v4/kernel/agents/install-tokens`), both for super
administrators, and the public routes the script uses: `GET /install.sh`,
`/install.sh.sig`, `/install/agent.env`, `/install/agent/<tag>/<asset>` and
`/install/ip` (the caller's address as Control sees it, with
`server.trusted_proxies` applied) ([guide](../guide/agent-onboarding.md)).
Node group join tokens are sealed with a key derived from the
[root key](#the-root-key).

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
- The subscription path (`subscribe_path`) may not be `install`,
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

The alert monitor always runs: Agent, forward link and module certificates
that were not renewed in time, CAs near their end, the panel's own
certificates (ending soon, or failing to renew), and phased processes (the
node credential split, the identity cutover) left in a non-final phase. Its
settings are built in (the 4.2 `alerts.*` keys are ignored): every 15
minutes, CAs 60 days before their end, a notification at most every 24
hours, phases stuck after 72 hours.

A leaf certificate alerts inside the smaller of 14 days and a
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
