# Upgrade Runbook

This runbook covers operator upgrades for AnixOps Control releases. Release
artifacts must come from GitHub Actions. Do not build binaries, frontend assets,
Docker metadata, checksums, SBOMs, or release manifests on the production host.

Use this together with:

- [`DEPLOYMENT.md`](DEPLOYMENT.md)
- [`manual-intervention.md`](manual-intervention.md)
- [`features.md`](features.md)
- [`reference/sqlite-to-postgres-migration.md`](reference/sqlite-to-postgres-migration.md)

## Scope

Use this runbook when moving a running panel from one GitHub Release tag to
another. It applies to systemd binary deployments and Docker-based deployments.

Do not use it for:

- first-time installation
- SQLite to PostgreSQL migration without the dedicated migration runbook
- emergency data repair
- payment credential changes
- domain, TLS, or reverse-proxy ownership changes

Those operations require their own operator approval and rollback plan.

**Weak shared secrets are reported at startup.** A `jwt.secret`,
`app.api_token` or `grpc.api_token` that is set but shorter than 32 bytes,
made of a few repeated characters, or still a template value logs one
`WARNING` naming the setting and how to rotate it (never the value). Nothing
refuses to start and short secrets keep working; replace them with
`openssl rand -hex 32` at a restart you choose (changing `jwt.secret` signs
everyone out).

**Upgrading to 4.1.0: 15 packages now default to native routes.** After the
upgrade, 151 rehearsed v2 routes are answered by their packages' native
handlers unless a mode is stored for them. `package_routes.default_mode:
legacy` keeps the 4.0 behaviour exactly; `anix-control routes rollback
--package <id>` rolls one package back. Read
["Upgrading To 4.1.0: Packages Now Default To Native Routes"](#upgrading-to-410-packages-now-default-to-native-routes)
before you upgrade.

**Upgrading to v4.2: `agent_control.mtls` defaults to `required`.** Legacy
API-key Agents are refused on the AnixOps Agent channels. Run
`anix-control agents transports --check-required` (exit status 0) before you
upgrade, and read
["Agent Transports: v4.2 Requires Enrolled Agents"](#agent-transports-v42-requires-enrolled-agents).
Fresh installs now generate the CA key and enable gRPC TLS; existing ones add
them first (Compose: `secrets/module_ca_kek` must exist before `up`).

**Upgrading to v4.2: the flux forwarding API and pages are removed.** The
v2 routes that change forwards, rules, tunnels, nodes, Ansible machines and
clean agents answer 404; use `/api/v4/forward/*`. Old forwarding data is not
migrated (F5c archives it). Read
["Flux Forwarding API Removed (v4.2)"](#flux-forwarding-api-removed-v42).

**Upgrading to v4.2: the official signing root changes, and every package
except `identity-platform` stays down until you import its 4.2 build.** The
first start with the new root retires the old one, so every installed
package fails closed and the `/api/v2` business routes answer `404
package_route_not_found`. `identity-platform` (login) recovers by itself on
that same start, because the image's bootstrap package is signed with the new
root ([Identity-Platform Recovers By Itself](#identity-platform-recovers-by-itself));
**4.2.0-rc.1 does not**: there nobody can log in until an administrator moves
it, so **open an administrator session before you restart and keep its
token** (`data.token` of `POST /api/v2/login`, valid for 24 hours). The
commercial packages (`order`, `payment`, `affiliate`) have no build signed
with the new root ([the warning](#commercial-packages-have-no-new-root-build)).
Read ["The Official Signing Root Changes (v4.2)"](#the-official-signing-root-changes-v42).

**Upgrading to 4.2.0-rc.3: the identity security fixes are in a package, and
nothing moves it for you.** The login, registration, reset and second-factor
fixes of rc.3 live in the `identity-platform` package, not in the Control
binary: starting rc.3 registers the new package release but leaves a healthy
installation on the release it runs. Until you move it
(`PUT /api/v3/plugin-installations`) the old behaviour stays. The gRPC refusal
of the kernel's HS256 tokens after identity's cutover is finalized is in the
binary and applies at once. Read
["Upgrading From 4.2.0-rc.2 To 4.2.0-rc.3"](#upgrading-from-420-rc2-to-420-rc3).

## Fixed Legacy Native Layout

For the specific legacy layout discovered on the old native host
(`/usr/local/v2board/v2board`, `/etc/v2board/config.yaml`, and
`/var/lib/v2board/frontend`), the repository includes
[`scripts/upgrade_legacy_panel_v2.5.0.sh`](../scripts/upgrade_legacy_panel_v2.5.0.sh).
It is an opt-in, root-only helper for a PostgreSQL database named `v2board`; it
backs up the database and runtime files, verifies release checksums, performs
an `/health` check, and writes a rollback script. Do not use it for Docker,
SQLite, foreign V2Board/XBoard schemas, or installations with different paths.
Run the general artifact and migration checks in this document first, and set
`VERSION` explicitly when upgrading to a different release tag.

## Pre-Upgrade Checklist

Before touching production:

- Confirm the target GitHub Release tag and current production version.
- Read the release notes and `CHANGELOG.md` entries for the target version.
- Check [`docs/features.md`](features.md) for partial/planned features and
  compatibility surfaces.
- Confirm the maintenance window and rollback owner.
- Back up the database and active config.
- If the release mentions schema or data changes, record the migration dry-run
  evidence attached to the GitHub Release.
- Verify the deployment type: systemd binary, Docker Compose, or another
  operator-managed wrapper.
- **Upgrading past 4.1.0-rc.2 with payments, orders, coupons or the invite
  commission in use: set `app.edition: commercial` first**
  ([Community Edition By Default](#community-edition-by-default-set-commercial-before-upgrading)).
- **Upgrading past 4.1.0-rc.2 behind a reverse proxy that is not on the
  same host: list it in `server.trusted_proxies` first**
  ([Reverse Proxies](#reverse-proxies-must-be-in-servertrusted_proxies-security)).
- **Upgrading past 4.1.0-rc.3: move the `identity-platform` installation to
  the new release after the upgrade**
  ([rc.3 → rc.4 Checklist](#rc3--rc4-checklist)).
- **Upgrading past 4.1.0-rc.4 with forwards on the `nftables_ansible`
  backend: relays need Linux 5.2+ and nft 0.9.1+, and traffic numbers jump
  to their real values** ([rc.4 → rc.5 Checklist](#rc4--rc5-checklist)).
- **Upgrading to v4.2: every enabled node must run an enrolled Agent, since
  `agent_control.mtls` now defaults to `required`; `anix-control agents
  transports --check-required` must exit 0 (or keep
  `agent_control.mtls: preferred` until it does)**
  ([v4.2 Requires Enrolled Agents](#agent-transports-v42-requires-enrolled-agents)).
- **Upgrading to v4.2: the flux forwarding API (`/api/v2/forward/*` and the
  administrator's forward, rule, node, Ansible machine and clean agent
  routes) is removed; scripts and clients must move to `/api/v4/forward/*`**
  ([Flux Forwarding API Removed](#flux-forwarding-api-removed-v42)).
- **Upgrading to v4.2: the official signing root changes. Set
  `plugins.official_public_key` to the new root (or remove the old value),
  take an administrator session token before the restart (the way in on
  4.2.0-rc.1, the fallback otherwise), and plan the window until the 4.2
  packages are imported. `identity-platform` recovers by itself; the other
  packages need your import. The commercial packages (`order`, `payment`,
  `affiliate`) are not in the release and have no build signed with the new
  root: an installation that runs them loses them**
  ([The Official Signing Root Changes](#the-official-signing-root-changes-v42)).

Evidence to keep:

- current service version or commit
- target tag
- downloaded artifact names
- `SHA256SUMS.txt` verification output
- `RELEASE_MANIFEST.json` summary
- database/config backup paths and checksums
- `/health` output before and after upgrade
- rollback decision

## Artifact Verification

Download artifacts from the GitHub Release for the target tag.
[`RELEASING.md`](RELEASING.md) lists what every release carries. At minimum,
download:

- the matching `anix-control-<os>-<arch>.tar.gz` backend artifact (or the
  matching Windows `.exe.zip`)
- `anix-control-frontend.tar.gz` (releases up to `v4.1.0-rc.2` also attached
  `anix-control-frontend.zip` with the same files)
- `SHA256SUMS.txt`
- `RELEASE_MANIFEST.json`
- `RELEASE_NOTES.md`

Every official package is built at the release version and shipped inside one
signed archive, `anix-control-packages-<version>.tar.gz`, with its detached
signature `anix-control-packages-<version>.tar.gz.sig`. The archive holds, for
each package ID, `<plugin-id>-<version>.anxp`, `.manifest.json`,
`.manifest.sig` and `.sbom.spdx.json`, plus one `official-public-key.pem`
(the official root in PEM form). `RELEASE_MANIFEST.json` lists each package
under `packages` with its version and the SHA-256 of its `.anxp` and manifest.
The identity bootstrap package (`identity-platform-<version>.anxp`,
`.manifest.json`, `.manifest.sig`) is also attached on its own, because the
release installer and the image use it. Releases up to `v4.1.0-rc.2` attached
every package file, and a `<plugin-id>-<version>.public-key.pem` per package,
as separate assets.

Do not import a mixed-version package set. The `v4.0.0` release also attaches
`v4-release-evidence.tar.gz`; the v4 plugin-only upgrade guide covers it.

Verify checksums before replacing any production file (`--ignore-missing`
skips the assets you did not download):

```bash
sha256sum --ignore-missing -c SHA256SUMS.txt
```

### Getting A Package From The Release

Download the archive, its signature, `official-public-key.raw` and
`SHA256SUMS.txt` from the same release. Compare `official-public-key.raw` with
the root pinned in your environment (`plugins.official_public_key`) first, then
check the archive signature against that root and extract the packages you
need:

```bash
VERSION=4.1.0-rc.3   # the release version, without the leading v
ARCHIVE="anix-control-packages-${VERSION}.tar.gz"
sha256sum --ignore-missing -c SHA256SUMS.txt
# The raw Base64 root as a PEM public key: Ed25519 SPKI prefix + the 32 key bytes.
{ printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'; base64 -d official-public-key.raw; } \
  | openssl pkey -pubin -inform DER -out official-root.pem
base64 -d "${ARCHIVE}.sig" > packages.sig.bin
openssl pkeyutl -verify -pubin -inkey official-root.pem -rawin -in "${ARCHIVE}" -sigfile packages.sig.bin
# "Signature Verified Successfully". Extract one package (or drop the
# wildcard to extract all of them into anix-control-packages-<version>/):
tar -xzf "${ARCHIVE}" --strip-components=1 --wildcards "*/machine-telemetry-${VERSION}.*"
```

Each extracted manifest can still be checked on its own, exactly as before:

```bash
PKG="machine-telemetry-${VERSION}"
base64 -d "${PKG}.manifest.sig" > manifest.sig.bin
openssl pkeyutl -verify -pubin -inkey official-root.pem -rawin -in "${PKG}.manifest.json" -sigfile manifest.sig.bin
grep -o '"artifact_sha256":"[0-9a-f]*"' "${PKG}.manifest.json"
sha256sum "${PKG}.anxp"   # must equal artifact_sha256
```

`openssl pkeyutl -rawin` loads the whole archive into memory (a few hundred
MB). A Control source checkout can run the same checks, including every
package in the archive, with
`python3 packages/shared/build_package.py --all --version "${VERSION}" --verify-release-archive "${ARCHIVE}" --official-public-key official-public-key.raw`.

Open `RELEASE_MANIFEST.json` and confirm:

- `build_source` is `github-actions`
- `manual_deployment_required` is `true`
- `commit` and `tag` match the intended release
- every artifact you will deploy appears with the expected size and SHA-256

If the manifest or checksum verification fails, stop the upgrade.

Control verifies every package manifest against its configured official root
(`plugins.official_public_key`) when it imports a package, and refuses any
package signed by another key. Before importing, compare the release's
`official-public-key.raw` with the root pinned in your environment. If they
differ, stop before enabling any plugin flag.

## Plugin-Only Bootstrap And Execution

`v4.0.0` is the formal plugin-only Control release. A new database must import
the signed `identity-platform` package before it can serve authenticated
Control routes. The release installer downloads the three identity assets,
verifies each against the release `SHA256SUMS.txt`, stores them under
`/opt/anixops/control/bootstrap/identity-platform-<version>` as root-owned,
group-readable files, sets the bootstrap directory and enables only Control
package execution. It also creates service-user-private `runtime/plugin-hosts`
and `data/plugin-artifacts` directories under the selected installation root,
then verifies `/health`, the package gateway, and the initial administrator
login before reporting a fresh installation as successful.

For a manual deployment, verify the formal evidence bundle first, then place
exactly `identity-platform-4.0.0.anxp`,
`identity-platform-4.0.0.manifest.json`, and
`identity-platform-4.0.0.manifest.sig` in a root-owned directory with mode
`0750` and files mode `0640`, readable by the Control service group. Configure
that absolute path before the first Control start:

```yaml
plugins:
  identity_bootstrap_package_dir: "/var/lib/anixops/bootstrap"
  control_execution_enabled: true
  control_host_runtime_dir: "/opt/anixops/control/runtime/plugin-hosts"
  control_host_artifact_dir: "/opt/anixops/control/data/plugin-artifacts"
  dispatch_enabled: true
  topology_execution_enabled: false
```

The bootstrap import accepts only one V2 Control-target identity package signed
by the configured official root; it is idempotent and never replaces an
operator-selected non-empty bootstrap path. The installer preserves every
other existing configuration value, except that plugin runtime paths are set
under the installation root so they remain writable inside the systemd service
sandbox. Enablement order still matters:
`topology_execution_enabled=true` is refused unless `dispatch_enabled=true`.
Do not enable traffic or topology changes until the package artifact,
signature, migration evidence, node rollback plan, and canary cohort are
recorded. V4 retains route semantics through capability-scoped,
kernel-owned compatibility bridge operations, but an uninstalled, disabled,
unhealthy, unsigned, or incompatible package still returns a package-unavailable
response instead of falling back to direct HTTP handling.

`gost-mesh` v1 is canary-only. Its signed package embeds the exact GOST v3.2.6
runtime and supports QUIC and WSS, not TUIC. Both transports require mutual
TLS. An entry must verify the exit CA/server name and present a client
certificate; an exit must present its server certificate and trust only the
configured client CA. Enabled health probes require a source address covered by
the entry source-policy CIDRs so the probe follows the same route as business
traffic.

Before applying a `gost-mesh` configuration, verify the participating Linux
host has forwarding enabled and strict reverse-path filtering disabled:

```bash
sysctl net.ipv4.ip_forward
sysctl -a 2>/dev/null | grep '^net.ipv4.conf.*.rp_filter ='
```

The required values are `net.ipv4.ip_forward=1` and
`net.ipv4.conf.*.rp_filter=0` for all participating scopes/interfaces. The
plugin validates these prerequisites and fails closed; it does not change
host-wide sysctls. Current TLS configuration refers to private files already
materialized on the Agent. Control Secret ID to private-file materialization,
renewal, deletion, and audit are not complete, so a stable or production
`gost-mesh` rollout is prohibited even when package signature checks pass.

## Upgrading From A v3.1 Or v4.0 Alpha Build

Production installations that were deployed from a `v3.1.0-alpha` or
`v4.0.0-alpha.1`-`alpha.7` tag need extra checks when they move to `v4.0.x`.
This path was rehearsed on 2026-09-29 against a copy of a production
PostgreSQL database. The results are in
[`architecture/release-line-status.md`](architecture/release-line-status.md#production-baseline-and-upgrade-rehearsal).

### Identify The Running Build

`app.version` in the production config can be stale; check the schema instead:

```bash
psql -At -d v2board -c "select count(*) from information_schema.tables
  where table_schema='public' and table_name like 'v4_kernel_package_%'"
```

`0` means the database was last started by an alpha build: no `v4.0.0`
package kernel tables exist yet.

### Before The Upgrade

- `v4.0.0-alpha.7` can crash with `fatal error: concurrent map writes` when
  several forward latency probes fail at once. Until the upgrade, reduce the
  risk by setting `forward_runtime.latency.concurrency: 1` in the running
  config; the fix (PR #14) is only in the new build.
- Take the pre-upgrade database backup (`pg_dump -Fc`) and keep the old binary,
  frontend and config for the [Rollback](#rollback) section.

### What Changes At Startup

With `env: production`, the server never alters existing tables: it creates
only missing tables and then runs its `Ensure*` schema helpers (`anix-control
migrate` runs the same step and exits). From an alpha.6/alpha.7 schema they add exactly six
tables and their indexes: `v4_kernel_package_backup_reference`,
`v4_kernel_package_migration_run`, `v4_kernel_package_rollout_lock`,
`v4_kernel_package_route_generation`, `v4_kernel_package_storage`, and
`v4_kernel_package_validation_result`, plus the read-only `kapi_*` views:
`kapi_user_directory_v1`, `kapi_subscriber_entitlement_v1` and
`kapi_user_referral_v1` over `v2_user`, `kapi_system_audit_log_v1` over
`v2_operation_log`, `kapi_plan_catalog_v1` and `kapi_plan_name_v1` over
`v2_plan`,
`kapi_plan_subscription_group_v1` over `v2_plan_subscription_group`,
`kapi_order_billing_v1` over `v2_order`, `kapi_affiliate_settings_v1` over
one row of `v2_system_config`, `kapi_user_subscription_group_v1` over
`v2_user_subscription_group`, `kapi_node_protocol_v1` over `v2_node_protocol`,
`kapi_node_heartbeat_v1` and `kapi_node_status_v1` over `v2_node`,
`kapi_forward_node_v1` over `v2_forward_node`,
`kapi_forward_runtime_settings_v1` over three rows of `v2_system_config` and
`kapi_traffic_log_v1` over `v2_server_log`. No existing table, column or index
changes.

Package storage leases additionally need `CREATEROLE` on the Control
database role and a `pg_hba.conf` entry that admits the `+anix_packages`
group (see `docs/architecture/plugin-kernel-contract.md`, "Package
Storage"). Neither is needed while every route runs in `legacy` mode.

### Package Install Window

`v4.0.x` serves every `/api/v2` business route through a signed package. Until
a route's package is installed and healthy, that route returns
`package_unavailable`. The kernel keeps serving `/s/:token`,
`/api/v1/client/subscribe`, `/api/v1/server/UniProxy/*` and `/flow/upload`
directly, so proxy clients and nodes are not affected by this window.

1. Verify the release evidence bundle and download the sixteen `v4.0.0`
   packages: `.anxp`, `.manifest.json` and `.manifest.sig` for each. From
   `v4.1.0-rc.3` on, take them from the signed packages archive instead
   ([Getting A Package From The Release](#getting-a-package-from-the-release)).
2. Containers: the release image already contains the identity package and
   enables package execution by default, so start `docker-compose.prod.yml`
   (see [Docker Compose Upgrade](#docker-compose-upgrade)) and continue with
   step 3. An `:edge` or `sha-<commit>` image (Dockerfile `source` target)
   does not bundle the package: mount the three verified
   `identity-platform-<version>` files read-only and point the server at them,
   for example with a `compose.override.yml`:

   ```yaml
   x-identity: &identity
     environment:
       ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR: /app/bootstrap/identity-platform
     volumes:
       - ./bootstrap/identity-platform:/app/bootstrap/identity-platform:ro
   services:
     migrate: *identity
     control: *identity
   ```

   Compose merges an override's `environment` and `volumes` into the base
   file, so the secrets and the artifact volume stay in place. The directory
   and files must not be group- or world-writable.

   Native installs: configure `identity_bootstrap_package_dir` and
   `control_execution_enabled: true` as described in
   [Plugin-Only Bootstrap And Execution](#plugin-only-bootstrap-and-execution),
   then start the new binary. `identity-platform` is imported on first start,
   and admin login works once it is healthy.
3. For each of the other fifteen packages, as an administrator:
   - `POST /api/v3/plugin-releases` with `{"manifest": "<manifest JSON text>", "signature": "<.manifest.sig contents>"}`;
   - `POST /api/v3/plugin-releases/<id>/artifact` with `{"artifact_base64": "<base64 of the .anxp>"}`;
   - `PUT /api/v3/plugin-installations` with `{"plugin_id": "<id>", "target": "control", "desired_version": "4.0.0", "enabled": true}`;
   - poll `GET /api/v3/plugin-installations` until the package is healthy.
4. Confirm that all sixteen installations are healthy, then run the admin,
   user and node smoke checks from [Post-Upgrade Record](#post-upgrade-record).

### Rehearse On A Copy First

Rehearse on a restored copy of the backup, never on the live database.
Several background workers contact real infrastructure and have no config
switch:

- the forward runtime job executor (ansible over SSH, NodeX);
- the forward agent bridge worker;
- the forward gost and ansible stats workers;
- the forward flow reset worker (NodeX calls on reset);
- the forward latency prober (TCP dials to forward targets).

Request handlers can also call Telegram, SMTP and payment providers. Run the
rehearsal servers and the test client in a network namespace that has only
loopback (`unshare -n`), and reach PostgreSQL over its Unix socket. Invalidate
the Telegram, mail, NodeX and payment credentials in the copy as a second
safeguard.

## Systemd Binary Upgrade

The exact service name and paths are operator-owned. The example below uses the
current installer defaults:

- service: `anix-control.service`
- binary path: `/opt/anixops/control/bin/anix-control`
- frontend path: `/opt/anixops/control/web/public`
- config path: `/opt/anixops/control/config/config.yaml`

Prepare backups:

```bash
sudo install -d -m 0750 /opt/anixops/control/backups
sudo cp -a /opt/anixops/control/bin/anix-control /opt/anixops/control/backups/anix-control.$(date +%Y%m%d%H%M%S)
sudo cp -a /opt/anixops/control/web/public /opt/anixops/control/backups/public.$(date +%Y%m%d%H%M%S)
sudo cp -a /opt/anixops/control/config/config.yaml /opt/anixops/control/backups/config.$(date +%Y%m%d%H%M%S).yaml
```

Stop, replace, and start:

```bash
tar -xzf ./anix-control-linux-amd64.tar.gz

sudo systemctl stop anix-control.service

sudo install -m 0755 ./anix-control-linux-amd64 /opt/anixops/control/bin/anix-control
sudo rm -rf /opt/anixops/control/web/public.new
sudo mkdir -p /opt/anixops/control/web/public.new
sudo tar -xzf ./anix-control-frontend.tar.gz -C /opt/anixops/control/web/public.new
sudo rm -rf /opt/anixops/control/web/public
sudo mv /opt/anixops/control/web/public.new /opt/anixops/control/web/public

sudo systemctl start anix-control.service
sudo systemctl status anix-control.service --no-pager
```

Verify:

```bash
curl -fsS http://127.0.0.1:8080/health
sudo journalctl -u anix-control.service -n 120 --no-pager
```

Then check:

- login
- admin dashboard
- users, plans, nodes, orders, and payments
- subscription download route
- traffic hourly/ranking pages
- forwarding pages if enabled
- node heartbeat and UniProxy config pull

## Docker Compose Upgrade

Container deployments pin the image by digest. Never build images on the
production host.

1. Read the target release's `docker-image.txt` and verify the signature:

   ```bash
   cosign verify ghcr.io/anixops/anix-control@sha256:<digest> \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     --certificate-identity-regexp '^https://github.com/AnixOps/anix-control/'
   ```

2. Back up the database (`pg_dump -Fc`) and keep the current `.env`, so the
   previous digest is on record.
3. Replace `docker-compose.prod.yml` with the target tag's copy, set
   `ANIX_CONTROL_IMAGE=ghcr.io/anixops/anix-control@sha256:<digest>` in `.env`, then:

   ```bash
   docker compose -f docker-compose.prod.yml pull
   docker compose -f docker-compose.prod.yml up -d      # migrate runs first
   docker compose -f docker-compose.prod.yml logs migrate
   docker compose -f docker-compose.prod.yml ps         # control healthy
   curl -fsS http://127.0.0.1:8080/readyz
   ```

   Two things stop the move to v4.2 before `control` starts (both rehearsed
   from 4.1.0 to 4.2.0-rc.1):

   - **`secrets/module_ca_kek` is missing.** The v4.2 Compose file mounts
     it, so `up -d` fails at once with `invalid mount config for type
     "bind": bind source path does not exist: .../secrets/module_ca_kek`,
     before it touches the running containers (4.1 keeps serving). Run
     `sudo bash init-secrets.sh` first
     (["Fresh Installs Are Ready; Adding The CA Key And gRPC TLS"](#fresh-installs-are-ready-adding-the-ca-key-and-grpc-tls)).
   - **`control.env` still names the old root.** The v4.2 image carries the
     identity package signed with the new root, so with
     `ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY` set to the 4.1 value `migrate`
     exits 1 (`Failed to prepare database: bootstrap identity platform
     package: verify identity bootstrap package: plugin signature
     verification failed`) and `control` is never created: Control is down
     from the `up -d` that recreated it until you correct the value and run
     `up -d` again. Set the new root, or delete the line to take the 4.2
     default.

   A start that succeeds begins the signing-root window: no business routes
   until the 4.2 packages are imported, and no login on 4.2.0-rc.1 (later
   builds recover `identity-platform` by themselves). Have an administrator
   session token ready and follow
   ["The Official Signing Root Changes (v4.2)"](#the-official-signing-root-changes-v42).

Rollback: restore the previous `.env`, `docker-compose.prod.yml` and
`control.env` (the root is part of the configuration) and run `up -d` again;
the root change adds a step once the 4.2 packages are imported
(["Rolling Back After The Import"](#rolling-back-after-the-import)). Restore
the database backup only when the approved rollback plan says so.

## Database And Migration Notes

For ordinary patch upgrades, keep the existing database driver and config.

The PostgreSQL connection string quotes every value. Before PR #13, an empty
`database.password` shifted the next setting into the password, and the
server connected to the default `postgres` database instead of the configured
one. An empty password (for example with peer or trust authentication over a
Unix socket) now works as written.

If a release requires schema/data changes:

- review release notes before the maintenance window
- take and verify a database backup first
- record pre/post row counts for affected tables
- keep rollback instructions for each changed schema helper
- do not combine SQLite-to-PostgreSQL migration with an unrelated feature
  upgrade unless the maintenance plan explicitly approves it

For SQLite-to-PostgreSQL migration, use
[`reference/sqlite-to-postgres-migration.md`](reference/sqlite-to-postgres-migration.md).

### Nodes Without An API Key

Agents authenticate with their node's API key, or with a forward node's API
token. Earlier builds accepted an empty token for a node whose key was empty;
such a node now refuses every agent until it has a key. Before upgrading,
check for one (read-only):

```sql
SELECT id, name FROM v2_node WHERE COALESCE(api_key, '') = '' AND COALESCE(api_key_hash, '') = '';
SELECT id, name FROM v2_forward_node WHERE COALESCE(api_token, '') = '';
```

Give a forward node a token from its edit form and update its agent's
configuration. No route sets the key of an existing proxy node (`v2_node`),
so replace such a node: register its agent again with an authorization key,
or create the node again in the panel (both issue a new key), then delete
the keyless node.

### Forward Node Tokens

Earlier builds showed a user the API tokens of the relay and exit nodes of
their forward rules (`GET /api/v2/user/forward/rules`), and any user could
create such a rule. A forward node's token authenticates its agent. If users
had rules, give the forward nodes new tokens from their edit form
(`PUT /api/v2/admin/forward/nodes/:id` with `api_token`) and update their
agents' configuration. The nodes that served user rules (read-only):

```sql
SELECT DISTINCT n.id, n.name FROM v2_forward_node n
JOIN v2_forward_rule r ON n.id IN (r.relay_node_id, r.exit_node_id)
WHERE r.user_id IS NOT NULL;
```

`GET /api/v2/forward/agent/rules` now answers only a forward node that sends
its id (`node_id` or `X-Node-ID`) and its token (`X-API-Key`, `api_key` or
`token`); it answered anyone before.

### Agent HTTP Routes Need The Node's Credentials

`POST /api/v2/agent/heartbeat`, `GET /api/v2/agent/tasks`,
`POST /api/v2/agent/result` and `POST /api/v2/agent/monitor` had no
authentication; they now take the credentials the agent WebSocket takes:
the node id in `X-Node-ID` (or the `node_id` query) and the node's API key,
or a forward node's API token, in `X-API-Key` (or the `api_key` or `token`
query). A request without them is answered `401`, a body naming another
node `403`, and a result for another node's task `404`. `anix-agent` uses
the WebSocket and is not affected; a custom agent or script that polls
these routes must send the credentials.

### Audit Request Bodies Written Before The Redaction Fix

Earlier builds stored the raw body of every administrator write request in
`v2_audit_log.request_body`, and logged the start of it. Those bodies can
hold user passwords, payment gateway keys, SMTP and S3 credentials, and bot
tokens. New rows are redacted; the upgrade does not rewrite old rows.

After taking a backup, an operator who wants the old bodies gone can clear
them. Adjust the cut-off to the upgrade time:

```sql
UPDATE v2_audit_log SET request_body = '' WHERE created_at < '2026-10-01';
```

Rotate any credential that was set through the administrator API while the
old build ran, and apply the same care to log files kept from that time.

## Upgrading From 4.1.0-rc.1 To 4.1.0-rc.2

These sections cover what changes from 4.1.0-rc.1 to 4.1.0-rc.2
(`CHANGELOG.md`, "4.1.0-rc.2"). They come in the order an operator needs
them: what needs action, then API answers that change, then what runs by
itself, then optional features.

### rc.1 → rc.2 Checklist

Before the upgrade:

1. **Node gRPC callers.** Give every script or probe that calls port 50051
   without a node key a node's API key, or set `grpc.api_token`
   (`ANIX_CONTROL_GRPC_API_TOKEN`) and send that.
2. **Disabled nodes.** Note which nodes should be disabled. A disabled
   V2bX or XrayR node now gets 403 from UniProxy.
3. **Forward.** List users' legacy rules and users' forwards with a
   non-public target, and check each clean agent's node. Scripts that issue
   clean agent tokens must send `nodeId`.
4. **API clients.** Update clients that read tokens from the
   administrator's user list, `user` or `plan` rows from the order answers,
   or secrets from administrator answers (now `********`). A user's
   `POST /api/v2/user/invite/generate` stops at `code_count` unused codes.
5. **Payments.** Run the query in "Paid Payments Left With A Pending Order
   Are Completed" and cancel any order you want kept pending.

After the upgrade:

6. Disable again any node an agent had re-enabled.
7. Run `anix-control node-secrets status`, set the API port of each node
   under `forward_nodes_without_api_port`, then run
   `anix-control node-secrets backfill` and `anix-control node-secrets verify`.
8. Read the log for `code=Unauthenticated` or `code=PermissionDenied` gRPC
   lines, `removed the node tokens from N stored payloads` and
   `Order payment reconciler:`.
9. Upgrade the forward package before plan, and affiliate before
   identity-platform.

Optional, after a staging rehearsal: move the node credential readers to
`dual_read` (Phase P2), let agents enroll for client certificates, and
watch KernelNodeOps at `GET /api/v4/kernel/node-operations`.

### The Node gRPC Listener Needs A Node Key

With `grpc.api_token` empty (the default), the node gRPC listener
(`grpc.*`, port 50051) let through any caller that sent an `authorization`
header, whatever its value, and answered it for any node: node
configurations with their protocol keys, user UUIDs, traffic, status and
log reports. It now authenticates only:

- a node's own API key, in `x-api-key` with the node's id in `x-node-id`, as
  V2bX and anix-agent send them. The call acts for that node only: a request
  whose `node_id` names another node is answered `PermissionDenied`, and a
  stream that sends one ends;
- `grpc.api_token`, when it is set, as `authorization: Bearer <token>`. It is
  the administrator's token and acts for any node;
- without credentials, `HealthService` and `NodeService/Register`, as
  before.

A caller without a node key, such as a script or probe that sent a made-up
token, is now answered `Unauthenticated`. Give it a node's API key, or set
`grpc.api_token` (`ANIX_CONTROL_GRPC_API_TOKEN`) to a generated secret and
send that. After the upgrade, refused calls show in the log as
`grpc unary request` or `grpc stream request` lines with
`code=Unauthenticated` or `code=PermissionDenied`.

A disabled node's key is refused too: see the next section.

### Disabled Nodes Stay Disabled

An administrator's disable now holds on every node transport. Before the
upgrade, note which nodes should be disabled; after it, check them.

- A disabled node's API key is refused on the listener (`PermissionDenied`,
  `node is disabled`), as the HTTP node API and the Agent control stream
  already refused it. A status stream that is open when the node is
  disabled ends at its next report.
- Heartbeats no longer re-enable a disabled node. Every heartbeat (gRPC,
  UniProxy over HTTP, the agent WebSocket and its HTTP routes) records
  `last_check_at` and sets a pending or offline node online, as before, but
  leaves a disabled node disabled, and the administrator's node list shows
  it disabled. A node an agent had silently re-enabled stays enabled after
  the upgrade: check the nodes that should be disabled and disable them
  again.
- UniProxy over HTTP refuses a disabled node's polling with 403
  (`{"error":"node disabled"}`) before the heartbeat, as the HTTP node API
  and the gRPC listener do. A disabled V2bX or XrayR node therefore gets no
  configuration and no users until an administrator enables it again.

### Forward Node Tokens Are Pinned To Their Endpoint

A forward node's API token is now presented only at the endpoint it was
bound to, `host:api_port` as recorded in `v4_kernel_node_credential.endpoint`
by the kernel's own forward node writers (decision D12,
`docs/architecture/node-ops-service.md` section 3.8). A NodeX request for
the gost backend or a legacy rule whose node's address is not the pinned
one fails with `ENDPOINT_UNCONFIRMED`
(`the forward node's address is not the one its token is pinned to`), and
nothing is sent.

- **What moves the pin.** Saving the node through the administrator's
  forward node or Ansible machine routes (`PUT /api/v2/admin/forward/nodes/:id`)
  re-pins it to the row as saved. A write that bypasses Control (direct
  SQL) does not: re-save the node in the administrator UI to confirm the
  address.
- **Until `node-secrets backfill` has run**, a node without a credential
  row has no pin yet, its token is presented as before, and it is counted in
  `anixops_node_secrets_pin_total{reason="unpinned"}`. An unconfirmed
  address is counted with `reason="unconfirmed"` and logged once per node,
  without the address or the value.

**Forward nodes without an API port: before the first backfill.** Once
`backfill` has run, a forward node's token is pinned to its endpoint,
`host:api_port` (decided by the owner, 2026-10-01). A forward node that has
a token but no `api_port` has no endpoint, so its token is presented
nowhere: gost backend changes and legacy rules on it fail with
`ENDPOINT_UNCONFIRMED` until an administrator sets the API port, which pins
it. Before the backfill nothing changes: such a node has no credential row
and is used as before. List those nodes first and set their ports, then
backfill ("Phase P1", below):

```bash
anix-control node-secrets status     # "forward_nodes_without_api_port": count, and each node's id and name
```

### Legacy Forward Rules Are Administrator-Only

Users can no longer create or change legacy forward rules.
`POST /api/v2/user/forward/rules`, the only user write on them, answers a
user with the panel error "only administrators can create or change legacy
forward rules; forward through your tunnels instead" and stores nothing; an
administrator's call still creates a rule. It let any user create a rule on
any relay and exit node, to any target. No user entitlement covers a legacy
rule: tunnel permissions grant tunnels, while a rule names its nodes, is not
counted in a permission's forward or traffic quota, carries the limits its
creator chose, and keeps running when a permission ends. Users forward
through the tunnels they are granted (`POST /api/v2/forward/create`); the web
panel never called the rule route. `GET /api/v2/user/forward/rules` still
lists a user's rules, read-only.

Rules that users created before the upgrade are kept and keep running. List
them (read-only):

```sql
SELECT id, user_id, name, relay_node_id, exit_node_id, listen_port, target_host, target_port, enabled
FROM v2_forward_rule WHERE user_id IS NOT NULL ORDER BY id;
```

Only an administrator can now disable, change or delete one
(`POST /api/v2/admin/forward/rules/:id/toggle`,
`PUT /api/v2/admin/forward/rules/:id` or
`DELETE /api/v2/admin/forward/rules/:id`), which also updates the nodes.
Rotate the tokens of the nodes they used, as "Forward Node Tokens" above
says.

### Users' Forward Targets Must Be Public

`POST /api/v2/forward/create` and `POST /api/v2/forward/update` now refuse a
user's target that is, or resolves to, a loopback, private (RFC 1918, ULA),
link-local, unspecified, multicast, carrier-grade NAT or other
special-purpose address, as well as `localhost` and its aliases, numeric
IPv4 forms such as `127.1`, and names that do not resolve on Control. An
administrator's forwards are not checked.

- Names are resolved from Control. A name that only the nodes' resolvers
  know (split-horizon DNS) is refused for users; an administrator can create
  such a forward.
- The check runs when a forward is written. DNS can answer differently
  later, when the node connects, so it is not complete protection against
  DNS rebinding.
- Existing forwards are not changed and keep running. A user can still
  pause, resume and delete one, but an update must replace a refused target.

To review existing user forwards with a non-public target, start from this
coarse filter (read-only). It also lists some public addresses (for example
`110.x` matches `10.`) and misses names that resolve to private addresses,
so check each row:

```sql
SELECT f.id, f.user_id, f.name, f.remote_addr, f.status
FROM v2_forward f JOIN v2_user u ON u.id = f.user_id
WHERE u.is_admin = 0 AND (
  f.remote_addr LIKE '%127.%' OR f.remote_addr LIKE '%localhost%'
  OR f.remote_addr LIKE '%10.%' OR f.remote_addr LIKE '%192.168.%'
  OR f.remote_addr LIKE '%172.%' OR f.remote_addr LIKE '%169.254.%'
  OR f.remote_addr LIKE '%100.%' OR f.remote_addr LIKE '%0.0.0.0%'
  OR f.remote_addr LIKE '%[f%' OR f.remote_addr LIKE '%[::%')
ORDER BY f.id;
```

Pause or delete an unwanted one as an administrator
(`POST /api/v2/admin/forward/pause` or `/api/v2/admin/forward/delete`), which
also removes it from the node.

### Clean Agents Are Bound To Their Node

A clean agent's token is now bound to one forward node: the node it was
issued for or, for a token issued without a node, the node of its first
registration that names one. `POST /api/v2/forward-agent/register` with
another `nodeId` is answered `403` (`agent is bound to another node`); a
registration without `nodeId` keeps the binding. Earlier builds moved the
agent to whatever node a registration named, so any token could take another
node's jobs.

- Each agent's current `node_id` becomes its binding at the upgrade. Before
  or right after upgrading, check that every live agent is on the node it
  serves (read-only):

  ```sql
  SELECT a.id, a.name, a.node_id, n.name AS node_name, a.hostname, a.public_ip, a.status, a.last_seen
  FROM v2_forward_clean_agent a LEFT JOIN v2_forward_node n ON n.id = a.node_id
  WHERE a.status <> -1 ORDER BY a.node_id, a.id;
  ```

  Revoke an agent whose node, host or address is wrong
  (`POST /api/v2/admin/forward/agents/:id/revoke`), issue a new token with
  the right `nodeId`, and rotate the API token of any node whose jobs it may
  have claimed.
- An agent whose configuration names a different node than its token now
  fails to register. Fix its `node_id` setting, or issue a token for the
  node it should serve. Moving an agent to another node always takes a new
  token.
- Issuing a token now requires its node. `POST /api/v2/admin/forward/agents`
  without a `nodeId`, with `0` or with an id that is not a forward node is
  refused with a panel error (`nodeId is required: a clean agent token is
  issued for one forward node` or `forward node not found`); scripts that
  issue tokens must send the forward node's id. Tokens issued earlier
  without a node keep working: each is bound by its first registration that
  names a node, so whoever holds it first chooses the node. Revoke unused
  unbound tokens and issue new ones for their nodes. To list them
  (read-only):

  ```sql
  SELECT id, name, status, last_seen FROM v2_forward_clean_agent
  WHERE (node_id IS NULL OR node_id = 0) AND status <> -1 ORDER BY id;
  ```
- The binding uses the existing `node_id` column: no migration.

### Paid Callbacks Complete Pending Orders Only

A paid payment callback marks its order paid and completes it only while the
order is pending. A payment that arrives for an order that was cancelled,
completed, or paid by another payment is recorded (the payment reads paid)
and the order is left as it is. Before, such an order was marked paid and
completed again. The reason is logged and kept in
`v4_kernel_subscriber_request` under `payment:<trade_no>`.

- **Review.** Paid payments whose order did not complete need a refund or an
  administrator's "mark paid":

  ```sql
  SELECT r.trade_no, r.order_id, o.status
  FROM v2_payment_record r JOIN v2_order o ON o.id = r.order_id
  WHERE r.status = 1 AND o.status NOT IN (1, 3);
  ```

- **Native callbacks.** The payment module can serve the four callback
  routes itself (`docs/architecture/order-service.md`). The PayPal webhook
  then verifies deliveries from the payment host, which needs outbound HTTPS
  to `api-m.paypal.com` (or `api-m.sandbox.paypal.com`). The callback URLs
  stay the same.

### Paid Payments Left With A Pending Order Are Completed

Control now completes the order of a paid payment whose callback did not
(`docs/architecture/order-service.md`). At start and every five minutes,
every Control process applies the paid payment records whose order is
still pending, paid more than two minutes ago and within the last 90 days,
that have no outcome under `payment:<trade_no>` yet. The checks and the
result are the callback's: the order is paid and completed and its plan
granted once, or the payment is refused and the order left as it is.

- **First start.** Records already in that state are applied at the first
  run after the upgrade, including records from before it. To see them
  beforehand:

  ```sql
  SELECT r.trade_no, r.order_id, r.paid_at
  FROM v2_payment_record r JOIN v2_order o ON o.id = r.order_id
  WHERE r.status = 1 AND o.status = 0
    AND r.paid_at >= CURRENT_TIMESTAMP - INTERVAL '90 days'
    AND NOT EXISTS (SELECT 1 FROM v4_kernel_subscriber_request q
                    WHERE q.request_id = 'payment:' || r.trade_no);
  ```

  On SQLite, write the age condition as
  `r.paid_at >= datetime('now', '-90 days')`. Cancel an order you want
  kept pending (`POST /api/v2/admin/orders/:id/cancel`) before the
  upgrade.
- **Refusals.** A payment that does not pay its order (another user's
  order, an amount below the total) is logged as `payment <trade_no> does
  not pay order <id>: <reason>` and recorded under `payment:<trade_no>`;
  the order is unchanged, and the payment is not tried again. Handle it as
  "Paid Callbacks Complete Pending Orders Only" describes.
- **Logs.** Each run that finds a payment logs a summary starting with
  `Order payment reconciler:`, and a line per completed order and per
  failure. Payments paid more than 90 days ago are left as they are.

### The Administrator's User List No Longer Shows Subscription Tokens

`GET /api/v2/admin/users` answered every listed user's whole `v2_user` row,
the subscription token and proxy UUID included, with the plan's whole row.
Each user in `data.list` now carries only the account, the subscription
summary and its plan's `id` and `name`; `total` and the query parameters
are unchanged.

- **Kept:** `id`, `email`, `balance`, `commission_balance`, `device_limit`,
  `speed_limit`, `flowResetTime`, `transfer_enable`, `u`, `d`, `plan_id`,
  `group_id`, `expired_at`, `banned`, `is_admin`, `is_staff` and
  `created_at`.
- **Slimmed:** `plan` is `{id, name}`, as in the order answers. A user
  without a plan, or whose plan no longer exists, has no `plan`, as before.
  The rest of the plan row is gone: `group_id`, `transfer_enable`,
  `speed_limit`, `device_limit`, `content`, `show`, `sort`, `renew`,
  `reset_price`, `reset_traffic_method`, `capacity_limit`, the seven prices,
  `created_at` and `updated_at`.
- **Removed:** `token`, `uuid`, `invite_user_id`, `telegram_id`, `discount`,
  `commission_type`, `commission_rate`, `remark_content`, `last_login_at`
  and `updated_at`. Password hashes were never serialized.
- **Where to read them.** `GET /api/v2/admin/users/:id` still answers one
  user's whole row, token, UUID, remark and plan included. A whole plan is
  in `GET /api/v2/admin/plans/:id`.
- **Order.** Users created in the same second now keep a stable order
  (`created_at DESC, id DESC`), so pages no longer repeat or skip them.

The bundled administrator page reads the token from the user detail when it
copies a subscription link and fills the edit form from the user detail; it
still names plans with `plan.name`. Control Center's
v2board plugin passes the answer through unchanged. A script that read
tokens from the list must read `GET /api/v2/admin/users/:id` per user.
`GET /api/v2/admin/users/stats` is unchanged.

The identity module can serve both routes natively (`native-flagged`) once
identity is authoritative (see the next section). It reads the subscription
summary from the view `kapi_subscriber_entitlement_v1` and the plan names
from `kapi_plan_name_v1`, so its signed release declares
`kernel.view:kapi_subscriber_entitlement_v1` and
`kernel.view:kapi_plan_name_v1`: install that release before switching the
routes to `native`.

### Order Lists And Details No Longer Embed The Buyer And Plan Rows

The four order list and detail routes embedded the buyer's whole `v2_user`
row (subscription token and proxy UUID included) and the whole `v2_plan`
row. They now carry the order, its plan's `id` and `name`, and, for an
administrator, the buyer's `id` and `email`. The bundled frontend reads only
those; an API client that read anything else of `user` or `plan` must change.

| Route | `user` | `plan` |
|-------|--------|--------|
| `GET /api/v2/admin/orders` (each `list` item), `GET /api/v2/admin/orders/:id` | `{id, email}` | `{id, name}` |
| `GET /api/v2/user/order` (each `list` item), `GET /api/v2/user/order/:id` | removed | `{id, name}` |

The order's own fields are unchanged. A plan or buyer that no longer exists
is left out, as before. The fields that disappear:

- From `user`: `invite_user_id`, `telegram_id`, `balance`, `discount`,
  `commission_type`, `commission_rate`, `commission_balance`, `token`, `uuid`,
  `device_limit`, `speed_limit`, `flowResetTime`, `transfer_enable`, `u`, `d`,
  `plan_id`, `group_id`, `expired_at`, `banned`, `remark_content`,
  `is_admin`, `is_staff`, `last_login_at`, `created_at` and `updated_at`. The
  user routes drop `user` altogether, `id` and `email` included. Password
  hashes were never serialized.
- From `plan`: `group_id`, `transfer_enable`, `speed_limit`, `device_limit`,
  `content`, `show`, `sort`, `renew`, `reset_price`, `reset_traffic_method`,
  `capacity_limit`, the seven prices (`month_price`, `quarter_price`,
  `half_year_price`, `year_price`, `two_year_price`, `three_year_price`,
  `onetime_price`), `created_at` and `updated_at`.

Read them where they belong: a user's own account from
`GET /api/v2/user/profile` and `GET /api/v2/user/subscription`, a buyer from
`GET /api/v2/admin/users/:id`, and a plan from `GET /api/v2/user/plan` or
`GET /api/v2/admin/plans/:id`.

Two more changes come with it. The administrator's `email` filter now
works: it always failed with an ambiguous `created_at`. The user's
`page_size` is clamped as the administrator's (1 to 100, default 20 for 0 or
less); a negative size used to list every order and 0 none.

The `order` package can serve these routes natively (`native-flagged`). It
reads the plan name from the new view `kapi_plan_name_v1` and the buyer's
e-mail from `kapi_user_directory_v1`, so its signed release declares
`kernel.view:kapi_plan_name_v1`: install that release before switching the
routes to `native`.

### Node Secrets Read As `********` In Administrator Answers

The administrator API no longer answers node secrets in clear: protocol
private keys, passwords and tokens, the secrets in a node's raw
configuration, node registration keys and forward node API tokens read
`********`. Nodes and agents still receive the real values, so running
nodes need nothing.

- **Editing.** Saving a protocol, a raw configuration or a forward node with
  `********` keeps the stored secret, and a new value replaces it. The panel
  editors work as before. A script that reads a protocol or raw
  configuration and writes it back keeps working too; a script that read a
  secret from these answers must keep its own copy instead.
- **Registration keys.** `GET /api/v2/admin/auth-keys` no longer shows
  existing keys, and they keep registering nodes. Copy a key when you
  generate it: Nodes, Auth Key, Generate Key, or
  `POST /api/v2/admin/auth-keys`. A key set with `NODE_DEFAULT_AUTH_KEY` is
  the one in that variable.
- **Forward node tokens.** A forward node's API token is answered once, by
  `POST /api/v2/admin/forward/nodes`; the forward node page and the setup
  wizard show a generated token after creating the node. Record it where
  you configure the relay's gost API. The connection test of an existing
  node needs the token typed in; the kernel dials with it, and the package
  host sees only a sealed handle. If a token is lost, set a new one from the
  node's edit form and update the relay and its agent.
- **Proxy node credentials.** `GET /api/v2/admin/nodes/:id/credentials`
  still answers a node's API key and secret, for the deployment helper and
  Ansible. Each read is now recorded in `v2_audit_log` with action `reveal`.

### The SMTP Password Reads As `********`

The administrator API no longer answers the SMTP password in clear:
`GET /api/v2/admin/notification/email/config` answers `password` as
`********` when one is stored, and the system configuration routes show the
`notification.email.config` value with `"password":"********"`. The test
e-mail still uses the stored password, so mail delivery needs nothing.

- **Panel.** Notifications, Email starts the password field empty with
  "Password stored; leave blank to keep it". Saving with the field empty
  keeps the stored password; typing a new one replaces it. Editing
  `notification.email.config` under System, Configuration shows the
  placeholder in the value; saving it as shown keeps the password.
- **Scripts.** A script that reads the e-mail configuration (either route)
  and writes it back keeps the password: the placeholder is kept. A script
  that read the password from these answers must keep its own copy.

### System Configuration Secrets Read As `********`

`GET /api/v2/admin/system/configs/:key` now masks a sensitive value as the
list (`GET /api/v2/admin/system/configs`) already did: a key whose name
contains `token`, `secret`, `password`, `passwd`, `private_key`, `api_key`,
`access_key` or `client_secret` (in any case, with or without the
underscore) reads `********` in `value` and `display_value` when a value is
stored, and `""` when none is; `sensitive` and `has_value` say which. The
kernel and the packages granted a namespace's secrets still read the real
values, so nothing that uses the NodeX token, the SMTP password or another
secret changes.

- **Editing.** Saving `********` back keeps the stored value, and a new
  value replaces it, as before. The NodeX page shows a stored token as
  `********` and keeps it when saved; its commands show
  `<FORWARD_API_TOKEN>` instead of the token. Saving `********` for a
  secret that is not stored is refused (`value is required`).
- **Scripts.** A script that read a secret from this route must keep its
  own copy. A script that reads a value and writes it back keeps working.

### Forward Runtime Job Payloads No Longer Hold Node Tokens

Until this release a forward change on a job backend (local Ansible, clean
agent) and every gost change recorded the ingress node's API token inside
the `v2_forward_runtime_job` payload (`panelForward.ingressNode.apiToken`),
so the job table, the administrator's job list
(`GET /api/v2/admin/forward/runtime/jobs`) and a clean agent's claim all
carried it (`docs/architecture/node-ops-service.md`, sections 3.7 and
3.11).

- **New rows carry no token.** The kernel resolves the token when it sends
  a request to NodeX, and only for the node's pinned endpoint ("Forward
  Node Tokens Are Pinned To Their Endpoint", above). Ansible and clean
  agent jobs never needed it: the agent authenticates with its own token.
- **Old rows are scrubbed at start.** The Control process that holds the
  singleton worker lease rewrites, before its job executors serve a row,
  every stored payload that still names a token (`"apiToken":"..."` and
  every other secret key), in batches of 200, and logs the count
  (`removed the node tokens from N stored payloads`). The pass is
  idempotent: a scrubbed row no longer matches, so later starts read
  nothing. It needs no operator action and no command.
- **During the transition** every reader scrubs what it serves: a clean
  agent that claims a row written before the upgrade gets the payload
  without the token, and so does the job list. Nothing an agent or an
  administrator reads holds a token.
- **Rollback.** An older binary writes tokens into new rows again; the
  next start of this release scrubs them.

### Node Credentials Are Also Kept In The Split Tables (Phase P1)

This release starts the node credential split
(`docs/architecture/node-ops-service.md`, section 4) at phase P1,
dual-write. At start Control creates three new tables, and alters no
existing one:

- `v4_kernel_node_credential`: node API keys and shared secrets,
  registration keys, forward node tokens (with the `host:api_port` they are
  pinned to) and clean agent tokens;
- `v4_kernel_protocol_secret`: the secrets inside node protocol settings
  and raw configurations (by JSON pointer), and WireGuard peer keys;
- `v4_kernel_node_secret_split`: each table's phase (`dual_write`) and the
  outcome of the commands below.

Every kernel write of a credential or secret (node creation, update and
deletion, registration, registration keys, forward nodes, clean agents,
protocols, WireGuard peers and their rotation) now also writes these
tables, in the same transaction. Readers keep using the legacy columns,
which keep every value, until you move a table to `dual_read` (next
section). An older binary therefore works as before after a rollback;
writes it makes do not reach the new tables, so run `backfill` again after
upgrading again.

The values are stored in clear, like the legacy columns, and the tables are
protected: no package can adopt them. Database backups and dumps hold them,
as they hold the legacy columns.

Set the API port of forward nodes without one before the first `backfill`
("Forward Node Tokens Are Pinned To Their Endpoint", above).

After the upgrade, copy the existing rows and compare both forms. Each
command prints JSON with counts and digests and never a secret; run it with
the server's configuration (`ANIX_CONTROL_CONFIG` or `-config`):

```bash
anix-control node-secrets backfill   # idempotent; resumes an interrupted pass
anix-control node-secrets verify     # exit 3 when the forms differ
anix-control node-secrets status     # phase, backfill progress, last verify, forward nodes without an API port
```

- `backfill` works table by table in batches by id (`-batch 500`), one
  transaction per batch, and records its progress, so it can run while
  Control serves. `-table v2_node,v2_node_protocol` limits it; `-restart`
  starts a new pass instead of resuming. Running it again changes nothing.
- `verify` reads both forms in one snapshot and names up to `-samples 20`
  differing secrets per table by subject and kind or JSON pointer
  (`missing`, `extra`, `different`). A difference after a completed backfill
  means a write bypassed Control's writers; `backfill` repairs it.
- The phase stays `dual_write` until you change it (next section), which
  requires a matching `verify` first.

### Moving The Node Credential Readers To The Split Tables (Phase P2)

Phase P2, dual-read, moves a table's readers to the new tables. It is an
operator command per table, never automatic, and the way back is another
phase change. Rehearse it on a staging copy first.

**What moves.** Every kernel reader of a moved column reads through the
split, in its table's phase:

- node API key checks: UniProxy, the node API, package downloads, the gRPC
  listener, the Agent Control stream, and the agent WebSocket and HTTP
  routes;
- the request signature's shared secret and node registration keys;
- forward node tokens (agent checks, the gost API, NodeX payloads) and
  clean agent tokens;
- protocol secrets in node configurations (UniProxy, gRPC) and in
  subscriptions, raw configurations, WireGuard peer keys, and the
  administrators' credentials route.

In `dual_write` they read the legacy columns, exactly as before. In
`dual_read` they read the new tables. Where a new row is missing or differs,
they fall back to the legacy column, so no node is locked out.

**The procedure:**

```bash
anix-control node-secrets backfill                  # copy what a bypassing write left behind
anix-control node-secrets verify                    # must exit 0
anix-control node-secrets phase -by <you> all dual_read
anix-control node-secrets status                    # every table in dual_read
```

- `phase` takes one table, a comma-separated list, or `all`. It moves a
  table to `dual_read` only if the table's latest `verify` matched: no
  mismatch, not followed by a failing verify, and at most an hour old.
  - Otherwise it prints each table's reason, changes nothing and exits 2.
    With `all`, one refusal refuses every table.
  - Run `verify` again and repeat.
- Running Control processes follow a change within 5 seconds; no restart is
  needed.
- Each change writes an audit entry: `v2_operation_log`, module
  `node_secrets`, action `phase_dual_read` or `phase_dual_write`. It names
  the table, the phases, the verification it relied on, and who made it
  (`-by`, default `$USER`).

**Watch the fallbacks.** `/metrics` counts every read that used a legacy
column in `anixops_node_secrets_fallback_total{table,kind,reason}`, with
reason `missing`, `mismatch` or `error`. It should stay at zero.

- The log has one line per subject, `node secret read fell back to the
  legacy column`. It names the table, kind, row id and JSON pointer, never a
  value.
- A fallback means a write bypassed Control's writers. `backfill` repairs
  it, and `verify` confirms the repair.
- Finalizing (P3) will require a zero fallback count.

**Rollback** is a phase change and needs no verification:

```bash
anix-control node-secrets phase -by <you> all dual_write
```

The writers dual-write in both phases, and `phase` changes only the split's
state table. The legacy columns therefore still hold every value, and an
older binary authenticates every node without any phase change.

**Validate on build (report-only).** Before Control builds a node's
configuration from a protocol or a raw configuration (UniProxy, gRPC), it
checks the secrets:

- no secret is the mask `********` or a tombstone;
- a WireGuard entry's server key pair is valid;
- a Reality private key is 32 bytes;
- a Shadowsocks 2022 server key has its cipher's length.

A failing row is counted in `anixops_node_secrets_invalid_total{table,type,
reason}` and logged once with the node, protocol and field, never the
value. It is still sent to the node: nothing is excluded in this release. To
scan every row:

```bash
anix-control node-secrets validate                  # exit 3 when a row fails
```

A later release leaves failing rows out of node configurations, once a
staging copy shows none. Fix what the scan reports before then.

**Tombstones.** In every phase, no reader accepts `!moved:<id>` or
`********` as a node key, registration key, forward node token or clean
agent token. Finalize (P3) writes these values into the legacy columns. A
request signed with a placeholder secret is refused.

### Agent Client Certificates Are Optional

Control can issue mTLS client certificates to AnixOps Agents
(`docs/architecture/module-runtime.md`, "Agent PKI"). Nothing is required of
operators: the new key `agent_control.mtls` defaults to `optional`, and
every agent keeps authenticating with its node API key as before.

- **What is new.** The tables `v4_kernel_agent_enrollment` and
  `v4_kernel_agent_certificate` (created at startup), the gRPC service
  `anix.agent.v1.AgentEnrollment` on the agent listener,
  `POST /api/v4/kernel/agents/enrollment-tokens` and
  `anix-control agent token create -node proxy-12`.
- **To let agents enroll.** Set `module_runtime.ca_kek`
  (`ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`, 32 random bytes as base64 or hex;
  keep it secret and backed up) and TLS on the gRPC listener
  (`grpc.tls_cert_file`, `grpc.tls_key_file`). The module runtime need not be
  enabled: the kernel creates the built-in CA at startup and does not open
  the module listener. Agents that support enrollment (anix-agent with AG-2)
  then enroll with their API key. Without the CA, or with
  `module_runtime.pki: external`, `Enroll` answers `FailedPrecondition` and
  the listener behaves exactly as before.
- **An existing `ca_kek` with the module runtime off.** From this release a
  `module_runtime.ca_kek` creates the built-in CA even when
  `module_runtime.enabled` is false, and a malformed key stops startup.
  Remove the key if you do not want the CA.
- **Later modes.** `preferred` keeps legacy credentials and answers a
  legacy control stream with the header `x-anix-auth-deprecated`; `required`
  (planned for 5.0) accepts only certificates on the Agent services. Both
  need the built-in CA and gRPC TLS, or Control refuses to start. Do not
  switch before every agent has enrolled.
- **Revocation.** Disabling or deleting a node, or replacing a forward
  node's token, revokes the node's agent certificates. A re-enabled node's
  agent enrolls again with its credential.

### KernelNodeOps And Configuration On The Agent Control Stream

Packages now request node operations from the kernel (KernelNodeOps,
`docs/architecture/node-ops-service.md`), and agents that ask for it get
their configuration, users and report acknowledgements on the Agent Control
stream. Nothing is required of operators; existing agents keep the legacy
transports.

- **New tables, created at startup.** `v4_kernel_node_operation`,
  `v4_kernel_node_operation_target` and `v4_kernel_node_operation_event`
  (the operation ledger), `v4_kernel_node_desired_config` (each node's
  desired configuration with its revision and hash, written by the
  administrator's node sync, section 5.5), `v4_kernel_node_config_status`
  (what each node's agent last reported about its configuration on the
  Agent Control stream, `config.v1`) and `v4_kernel_agent_report_batch`
  (report batches applied, pruned after 7 days). All are protected from
  package adoption; no existing table changes.
- **The ledger.** Administrators list operations read-only at
  `GET /api/v4/kernel/node-operations`. Ended operations are kept 90 days
  and events 7 days.
- **Configuration push.** Agents that list `config.v1` receive their
  configuration as snapshots on the stream; the first rebuild after the
  upgrade moves every stored revision by one, since the document gained the
  UniProxy answers.
- **Package order.** The speed-limit routes moved from plan to forward and
  the user's invite routes from identity-platform to affiliate: upgrade the
  forward package before plan, and affiliate before identity-platform.
- **Metrics.** `/metrics` adds the `anixops_agent_config_*`,
  `anixops_agent_user_*` and `anixops_agent_users_*` series and
  `anixops_v2_gateway_sealed_secrets_total` (`CHANGELOG.md`, 4.1.0-rc.2).

## Upgrading From 4.1.0-rc.2 To 4.1.0-rc.3

These sections cover what changes from 4.1.0-rc.2 to 4.1.0-rc.3
(`CHANGELOG.md`, "4.1.0-rc.3"). **Two changes need action before you
upgrade:** the community edition is now the default, so an install that
sells plans must opt in to `commercial` first, and forwarding headers count
only from `server.trusted_proxies`, so every reverse proxy that is not on
the same host must be listed. Upgrading changes no phase of the node
credential split; finalizing it (Phase P3) is an operator procedure for
later, under the owner's approval.

### rc.2 → rc.3 Checklist

Before the upgrade:

1. **Decide the edition.** An install that uses payments, orders, coupons,
   plan purchase or the invite commission sets `app.edition: commercial`
   (`ANIX_CONTROL_APP_EDITION=commercial`) before starting the new binary
   or image, and plans how it will get the commercial packages: the
   release no longer ships `affiliate`, `order` and `payment`, so keep the
   installed versions or build them with
   `packages/shared/build_package.py --all --edition commercial` until a
   commercial channel exists. An install going community drains pending
   payments first
   ([Community Edition By Default](#community-edition-by-default-set-commercial-before-upgrading)).
2. **List your reverse proxies** (the peer address Control sees from each)
   and set `server.trusted_proxies`
   (`ANIX_CONTROL_SERVER_TRUSTED_PROXIES`), keeping `127.0.0.1/32,::1/128`;
   replace an old private-range list with the real addresses, and set the
   public addresses (`forward_runtime.clean_agent.public_url`,
   `app.subscribe_domains`)
   ([Reverse Proxies](#reverse-proxies-must-be-in-servertrusted_proxies-security)).
3. Download and verify the release as in
   [Artifact Verification](#artifact-verification): packages now come in
   one signed archive ([Fewer Release Assets](#fewer-release-assets)).

After the upgrade:

4. `curl -s http://127.0.0.1:<port>/api/v4/public/config` shows the
   `"edition"` you chose.
5. Subscription links and the clean agent install script
   (`/api/v2/forward-agent/install.sh`, `PANEL_URL`) show the public host
   over `https://`, not `http://` or an internal address; logs and audit
   entries show client IPs, not the proxy's.
6. Payment callbacks still arrive and complete orders (commercial), or
   answer `404` `package_route_not_found` (community).

Optional, only after a staging rehearsal and with the owner's written
approval: finalize the node credential split
([Phase P3](#finalizing-the-node-credential-split-phase-p3)).

### Community Edition By Default: Set `commercial` Before Upgrading

> **Action required for paid installs.** From 4.1.0-rc.3 Control runs as the
> **community** edition unless the configuration says otherwise. If this
> install uses payments, orders, coupons, plan purchase or the invite
> commission, set the edition to `commercial` **before** you start the new
> binary or image, or those features disappear for administrators, users
> and payment providers alike.

Set one of:

```yaml
# config.yaml
app:
  edition: "commercial"
```

```bash
# Docker / Kubernetes / systemd environment
ANIX_CONTROL_APP_EDITION=commercial
```

An unknown value stops the server at start-up (`invalid app.edition`).
Check what the running server uses with
`curl -s http://127.0.0.1:<port>/api/v4/public/config` (`"edition"`).

What the community edition does (`docs/features.md`, "Product editions"):

- Every `/api/v2` route of the `order`, `payment` and `affiliate`
  packages, and `GET /api/v2/user/plan`, answers `404`
  `{"error":{"code":"package_route_not_found","message":"package route is not declared"}}`,
  like a path that does not exist. This includes the payment callbacks and
  webhooks (`/api/v2/payment/callback/:type`, `/payment/stripe/webhook`,
  `/payment/paypal/webhook`, `/payment/x402/callback`): **a provider that
  calls back for an order paid just before the switch gets 404**, so drain
  pending payments or stay on `commercial`.
- Nothing is deleted: orders, payment records, coupons, commissions,
  balances and plan prices stay in the database and come back with
  `commercial`. Plans remain as free subscription templates: administrators
  edit and assign them, and their subscription groups keep working; the
  admin page hides prices but sends a stored price back unchanged.
- The web app hides the commercial menus and pages (and redirects their
  URLs to the dashboard), the user balance field and the revenue cards.
- Registration keeps invite codes when `auth.registration.require_invite`
  is on, but users cannot generate new codes in the community edition (the
  generator belongs to the `affiliate` package).
- The community release no longer ships the `affiliate`, `order` and
  `payment` packages. Installed ones keep running (their routes are hidden);
  a commercial install keeps the versions it has, or builds them from
  source with `build_package.py --all --edition commercial`, until a
  commercial channel publishes them.

x402 changes in both editions: `GET /api/v2/payment/methods` no longer
reports x402 as enabled when no payment method is configured, and
`POST /api/v2/payment/x402/create` answers `gateway is disabled` unless an
enabled x402 payment configuration exists. Enable it explicitly if you rely
on it.

To switch back later, set `commercial` and restart; nothing else is needed.

### Reverse Proxies Must Be In `server.trusted_proxies` (Security)

Control now reads `X-Forwarded-Proto`, `X-Forwarded-Host`,
`X-Forwarded-For` and `X-Real-IP` only when the TCP peer is listed in
`server.trusted_proxies` (`ANIX_CONTROL_SERVER_TRUSTED_PROXIES`). From any
other peer they are ignored: the scheme comes from the connection (TLS or
not), the host from the `Host` header, the client IP from the peer address
(`CHANGELOG.md`, 4.1.0-rc.3, Security).

Before, any client could set the scheme and host of the links Control hands
out (clean agent install script, subscription links, Telegram webhook, the
`request_scheme`/`request_host` package hosts receive), and the built-in
default trusted every private address (`10.0.0.0/8`, `172.16.0.0/12`,
`192.168.0.0/16`) for the client IP.

**Defaults now:**

| Where | `trusted_proxies` |
|---|---|
| unset, container defaults, `config/config*.yaml*` templates | `127.0.0.1/32,::1/128` (loopback only) |
| `config/deploy/compose/control.env.example` | `127.0.0.1/32,::1/128,172.16.0.0/12` (Docker bridge gateway for a proxy on the host) |
| Helm `values.yaml` | `127.0.0.1/32,::1/128` plus the private ranges: narrow them to the ingress controller's pod CIDR |
| explicit `[]` or an empty variable | no proxy |

**What to do before upgrading:**

1. Find the address your reverse proxy connects to Control from (the peer
   Control sees): `127.0.0.1` for Nginx/Caddy on the same host; the Docker
   bridge gateway for a proxy on the host in front of the Compose
   deployment; the proxy container's network for Traefik; the ingress
   controller pod CIDR on Kubernetes.
2. If it is not loopback, set it, keeping loopback (a list replaces the
   default, and Control's own UI port proxies `/api` over loopback):

   ```bash
   ANIX_CONTROL_SERVER_TRUSTED_PROXIES=127.0.0.1/32,::1/128,10.0.5.20
   ```

   or in `config.yaml`:

   ```yaml
   server:
     trusted_proxies: ["127.0.0.1/32", "::1/128", "10.0.5.20"]
   ```

3. If you set the old private-range list yourself (it was in
   `config/config.prod.yaml`), replace it with the proxy's real addresses:
   every listed address can choose the client IP and the links' host.
4. Configure the public addresses so links do not depend on requests at all:
   `forward_runtime.clean_agent.public_url`, the subscription domains
   (`app.subscribe_domains`), and pass `url` when setting the Telegram
   webhook.
5. Make the proxy set (overwrite) `X-Forwarded-Proto` and
   `X-Forwarded-Host`; Control uses the last value. Examples for Nginx,
   Caddy, Traefik and Kubernetes ingress: `docs/DEPLOYMENT.md`, section 6.1
   "Reverse proxies".

An invalid entry (not an IP or CIDR) stops Control at startup with
`server.trusted_proxies: invalid trusted proxy ...`.

**Symptoms of a missing proxy after the upgrade:** install scripts,
subscription links or the Telegram webhook show `http://` or an internal
address, HSTS is not sent, and logs, audit entries and rate limits see the
proxy's address instead of the client's. Add the proxy and restart.

**Check** from a host that is not a proxy (the output must not name
`evil.example`):

```bash
curl -s -H 'X-Forwarded-Host: evil.example' -H 'X-Forwarded-Proto: https' \
  http://<control>:8080/api/v2/forward-agent/install.sh | grep PANEL_URL
```

**Rollback:** the setting is read by older releases too (for the client IP
only), so it can stay when you go back.

### Fewer Release Assets

The release page has 18 assets instead of 104. The packages are in one
signed archive, `anix-control-packages-<version>.tar.gz` (signature:
`.tar.gz.sig`); take a package from it as in
[Getting A Package From The Release](#getting-a-package-from-the-release).
The per-package `.public-key.pem` files and `anix-control-frontend.zip` are
gone: use `official-public-key.raw` (or the archive's `official-public-key.pem`)
and `anix-control-frontend.tar.gz`. `scripts/install.sh`, the image and the
identity bootstrap package (still a separate asset) are unaffected.

### Finalizing The Node Credential Split (Phase P3)

Phase P3, finalize, removes the node credentials and protocol secrets from
the legacy columns: from then on they live only in the split tables. Nothing
runs it automatically, and upgrading to this release changes nothing: every
table stays in the phase it is in.

**Production needs the owner's approval, and a staging rehearsal first
(decision D6).** Run finalize on production only:

- at least one release after the tables moved to `dual_read`;
- after a rehearsal on a staging copy of production: finalize, the node
  fleet authenticating, `unsplit`, finalize again;
- with the owner's written approval for that installation.

**Upgrade order.** After finalize, never start a Control binary older than
this release against the database:

- binaries before the release that shipped P2 (NO-3) compare the legacy
  columns directly, and accept a presented tombstone (`!moved:<id>`) as a
  node key or clean agent token: they would **fail open**;
- binaries from P2 up to the one before this release read the new tables
  correctly, but their writers would write secrets back into the legacy
  columns and treat a tombstone as a value.

Upgrade every Control process (server, workers, CLI) to this release first,
then finalize. To go back to an older binary, `unsplit` first.

**What finalize writes** (docs/architecture/node-ops-service.md, section
4.4). Values only; no table is altered:

| Legacy column | After finalize |
|---|---|
| `v2_node.api_key`, `.secret`, `v2_authorized_key.key`, `v2_forward_node.api_token`, `v2_forward_clean_agent.token`, `v2_wireguard_peer.private_key`, `.preshared_key` | `!moved:<row id>`: unique per row, never a credential |
| `v2_node.api_key_hash`, `v2_authorized_key.key_hash` | empty |
| `v2_node.raw_config`, the settings columns of `v2_node_protocol` | the masked document administrators already see, with `********` at every secret position |

An empty value stays empty. The new tables keep every secret, and keep the
original bytes of each JSON column so `unsplit` restores them exactly.

**The procedure,** per table or for `all`:

```bash
anix-control node-secrets status                    # every table to finalize in dual_read
anix-control node-secrets verify                    # must exit 0, within the hour before finalize
# /metrics: anixops_node_secrets_fallback_total must not have grown since dual_read
anix-control node-secrets finalize -confirm -by <you> all
anix-control node-secrets status                    # phase finalized, finalized_at set
anix-control node-secrets verify                    # finalized tables compare by presence; must exit 0
```

- `finalize` refuses, changes nothing and exits 2 without `-confirm`, for a
  table not in `dual_read`, or without a matching `verify` at most an hour
  old. With `all`, one refusal refuses every table.
- It first sets the phase to `finalized` (readers read the new tables only;
  writers write tombstones), then rewrites the rows in batches of `-batch`
  (500) by id, each in its own transaction, and records `finalized_at` once a
  pass finds nothing left.
- A tombstone that would collide with another row's value on a unique
  column (a row already holding `!moved:<that id>`) stops the batch and names
  both row ids. Give that row its own value and run `finalize` again.
- An interrupted finalize (phase `finalized`, `finalized_at` empty) is
  resumed by running the same command again; it needs no new `verify`.
- Each step is audited in `v2_operation_log` (module `node_secrets`,
  actions `finalize_started` and `finalized`). No command prints a secret.

**After finalize:**

- Every reader reads the new tables only and never falls back. A literal
  secret written into a finalized JSON column past Control's writers is
  ignored, and its position is sent as `********` (reported by `validate`).
- The views of the credential-free remainder appear
  (`kapi_node_public_v1`, `kapi_node_protocol_public_v1`,
  `kapi_node_credential_status_v1`, `kapi_registration_key_v1`,
  `kapi_forward_clean_agent_v1`, `kapi_wireguard_peer_v1`). Packages that
  declare them, or `kernel.storage.adopt:` for `v2_node`,
  `v2_node_protocol` or `v2_forward_node`, get them at their next storage
  lease. Before finalize the lease leaves them out.
- A package host leases its storage when it first opens it, so restart
  Control (which starts the package hosts again) after finalize. Until
  then the routes on these views and tables answer from the legacy handler,
  whatever their mode: protocol-runtime's node protocol routes (M3-1),
  proxy-node's node list, detail, deletion and raw configuration and its
  registration key list (M3-2), and subscription's group protocols and
  protocol pool (M3-3).

**Rollback: `unsplit`.**

```bash
anix-control node-secrets unsplit -confirm -by <you> all   # exit 3 if verify then differs
anix-control node-secrets status                           # back in dual_read
```

- It returns each table to `dual_read`, drops the views that show a column
  that held secrets, writes every secret back from the new tables (the JSON
  columns byte for byte as they were), and runs `verify`.
- It refuses a table a package's storage lease adopted: that package would
  read the secrets written back. Remove the grant first (uninstall the
  package, or lease a release without it), then add `-adopted-ok`. It does
  not undo what the package wrote.
- An interrupted `unsplit` is resumed by running it again.
- From `dual_read`, `phase all dual_write` goes back further, as in P2.

## Upgrading From 4.1.0-rc.3 To 4.1.0-rc.4

These sections cover what changes from 4.1.0-rc.3 to 4.1.0-rc.4
(`CHANGELOG.md`, "4.1.0-rc.4"): the UI redesign preview and invite codes for
administrators in every edition. There is no schema migration and no
configuration key changes. **One action is needed:** move `identity-platform`
to this release's version, since it serves the new invite-code routes.

### rc.3 → rc.4 Checklist

Before the upgrade:

1. Download and verify the release as in
   [Artifact Verification](#artifact-verification), including the three
   `identity-platform-<version>` assets (or take them from the packages
   archive).
2. If a reverse proxy or CDN in front of Control caches `index.html`,
   plan to purge it after the upgrade.
   The new frontend's scripts and styles have new hashed names; a cached
   `index.html` keeps pointing at the old ones, which are gone.

After the upgrade:

3. **Move `identity-platform` to this release.** Start-up registers the
   bundled `identity-platform` release (the image and `scripts/install.sh`
   ship it) but never moves an existing installation to it (the one
   exception is an installation whose release is bound to a retired signing
   root, [Identity-Platform Recovers By Itself](#identity-platform-recovers-by-itself)).
   As an administrator, `PUT /api/v3/plugin-installations` with
   `{"plugin_id": "identity-platform", "target": "control", "desired_version": "4.1.0-rc.4", "enabled": true}`
   and poll `GET /api/v3/plugin-installations` until it is healthy. A manual
   install without the bootstrap directory first registers the release with
   `POST /api/v3/plugin-releases` and `POST /api/v3/plugin-releases/<id>/artifact`
   ([Package Install Window](#package-install-window), step 3). Until then
   the admin 邀请码 page answers `404` `package_route_not_found`.
4. Purge the proxy or CDN cache from step 2, and tell administrators to
   reload once (a hard reload if a tab still shows the old sidebar).
5. Check sign-in, the admin dashboard and one user page in light and dark,
   then open 用户 → 邀请码 and generate one code.
6. Tell administrators and users where things moved
   ([The New Web UI](#the-new-web-ui)).

### The New Web UI

The web app is redesigned (AnixOps Design v1.0.2). Page bodies, the API and
page routes are unchanged; what moved:

- **Language, theme and version.** Language and theme (light, dark or the
  new 跟随系统) are in the account menu: the avatar at the bottom of the admin
  sidebar, or in the top bar on phones. The version and build line is under
  账户菜单 → 关于. A language or theme choice saved in the browser is kept.
- **Admin sidebar.** Grouped 概览, 用户, 网络, 扩展, 系统 (and 商业 in the
  commercial edition); plugin menus appear under 扩展 or 系统. Invite codes
  are under 用户 → 邀请码. `⌘K` / `Ctrl+K` opens a command palette that jumps
  to any page and finds users by email.
- **Forward suite.** Its sub-navigation is a strip at the top of every
  `/admin/forward*` page, no longer in the sidebar.
- **Users.** Phones get a bottom tab bar; the new 账户 page holds profile,
  two-factor authentication, language and appearance.
- **Undo instead of confirm.** Banning or unbanning a user, enabling or
  disabling a payment gateway, deleting the Telegram webhook, removing a group
  from a plan and removing a member or plan from an access group now happen
  at once, with 撤销 in the toast for 5 s. Scripts or runbooks that expected a
  browser confirmation before these actions need updating.
- **Typed confirmations.** Deleting a node, a NodeX forward node or an Ansible
  machine asks for its name; sending a Telegram broadcast asks first.
- A non-administrator opening an admin page sees 无权限 instead of being
  sent to the user dashboard; unknown paths show a 404 page.

### Administrators Manage Invite Codes In Every Edition

In 4.1.0-rc.3 the community edition could not create invite codes, because
their generator belongs to the commercial `affiliate` package, so
`require_invite` registration only worked with codes that already existed.
Administrators can now generate, list and revoke invite codes in every
edition, on the admin "邀请码 / Invite codes" page (用户 → 邀请码,
`/admin/invite-codes`; `GET`/`POST /api/v2/admin/invite/codes`,
`DELETE /api/v2/admin/invite/codes/:id`). Users still cannot generate codes
in the community edition.

- These routes are declared by `identity-platform`: install the
  `identity-platform` package of this release (checklist step 3), or an older
  installed one answers them `404` `package_route_not_found`.
- Codes generated there belong to no user and attribute no referral; they do
  not count toward any user's code limit.
- Commission, withdrawals, invite statistics and the invite configuration stay
  with the commercial `affiliate` package.

## Upgrading From 4.1.0-rc.4 To 4.1.0-rc.5

These sections cover what changes from 4.1.0-rc.4 to 4.1.0-rc.5
(`CHANGELOG.md`, "4.1.0-rc.5"): the rest of the UI redesign, users resetting
their own subscription link, and the nftables forward path counting traffic
correctly. There is no schema migration and no configuration key changes.
**Two things need attention:** move `identity-platform` to this release's
version, since it serves the new reset route, and, with forwards on the
`nftables_ansible` backend, expect their traffic numbers to jump to the real
values.

### rc.4 → rc.5 Checklist

Before the upgrade:

1. Download and verify the release as in
   [Artifact Verification](#artifact-verification), including the three
   `identity-platform-<version>` assets (or take them from the packages
   archive).
2. **Forwards on `nftables_ansible` only.** Check every relay with
   `uname -r` and `nft --version`: Linux 5.2+ and nft 0.9.1+ are needed
   ([nftables Forwards](#nftables-forwards-move-to-the-inet-v2b_forward-table)).
   Review quotas sized against the old, too-low numbers. The image ships the
   playbooks; a host that runs Control with its own copy of
   `config/deploy/ansible/playbooks/` replaces it with this release's (it
   gains `files/v2b_forward_nft.sh`).
3. If a reverse proxy or CDN in front of Control caches `index.html`,
   plan to purge it after the upgrade. The frontend is split into new
   chunks and per-language message files with new hashed names; a cached
   `index.html` keeps pointing at the old ones, which are gone.

After the upgrade:

4. **Move `identity-platform` to this release**, as in the
   [rc.3 → rc.4 Checklist](#rc3--rc4-checklist), step 3, with
   `{"plugin_id": "identity-platform", "target": "control", "desired_version": "4.1.0-rc.5", "enabled": true}`
   in `PUT /api/v3/plugin-installations`, and poll
   `GET /api/v3/plugin-installations` until it is healthy. Start-up never
   moves an existing installation. Until then the subscription reset answers
   `404` `package_route_not_found`.
5. Purge the proxy or CDN cache from step 3, and tell administrators to
   reload once (a hard reload if a tab still shows an old page).
6. **Forwards on `nftables_ansible` only.** Nothing changes on the relays at
   upgrade time; each forward moves to `inet v2b_forward` on its next apply.
   To move them now, pause and resume each forward, or save it unchanged,
   then check a relay with `nft list table inet v2b_forward` (and that
   `nft list table ip v2b_forward` is gone once its last forward moved).
   Expect the traffic of moved forwards to rise to the real values.
7. Check sign-in, the admin dashboard, a node page, the forward list,
   系统设置 and the user 概览 on a phone, in light and dark.
8. With a test account, reset the subscription link on 订阅 → 重置链接…: the
   new link imports, the old `/s/<token>` link no longer does.
9. Tell administrators and users where things moved
   ([Pages That Moved](#pages-that-moved)).

### Pages That Moved

Page bodies moved onto the AnixOps Design templates; the API is unchanged
apart from the new reset route. Old paths redirect, so bookmarks keep
working.

| Page | Now | Old paths that redirect |
| --- | --- | --- |
| 系统设置 | `/admin/system/:section` (通用, 转发运行时, 备份, 负载均衡, 审计日志, 关于; `/admin/system` shows 通用) | none |
| 安全 | `/admin/security/:section` (MFA policy, access groups) | `/admin/mfa`, `/admin/access-groups` |
| 通知 | `/admin/notifications/:channel` (e-mail, Telegram, templates, send log) | `/admin/telegram` |
| 流量与监控 | `/admin/monitor/:section` (实时节点, 用户流量, 节点延迟, 转发), `?range=` | `/admin/traffic-hourly`, `/admin/forward/observability` |
| Node page | `/admin/nodes/:id` (`?section=`) | none (new) |
| NodeX forward node, Ansible machine | `/admin/forward/nodes/:id`, `/admin/forward/ansible-machines/:id` (`?tab=`) | none (new) |
| Subscription group | `/admin/subscriptions/:id/:section` | none (new) |

- The sidebar's 系统 group is 系统设置, 安全, 通知; `⌘K` / `Ctrl+K` lists
  every section by name (访问组, 审计日志, ...).
- Admin lists keep search, filters and page in the URL, so a copied link
  opens the same view.
- Users reset their own link on 订阅 instead of filing a ticket; "备用码"
  are now "恢复码" (recovery codes).

### Users Reset Their Own Subscription Link

Users can reset their own subscription link on the 订阅 page
(`POST /api/v2/user/subscription/reset`). It rotates what the
administrator's reset rotates: a new subscription token, the proxy UUID
kept, so nodes and connected clients are unaffected and only the old
`/s/<token>` link stops working. It asks for the current password, or a
TOTP or recovery code when the account has two-step verification, and
allows three attempts per user and hour. The reset-request ticket the
subscription page filed before is gone.

- The route is declared by `identity-platform`: move that installation to
  this release ([rc.4 → rc.5 Checklist](#rc4--rc5-checklist), step 4), or an
  older installed one answers it `404` `package_route_not_found`.
- It belongs to identity group A, because it checks credentials. Before the
  identity cutover nothing changes: the cutover and rollback switch it with
  the rest of group A.
- **Identity already authoritative** (cutover done before this upgrade):
  nothing to do. The route modes the cutover stored do not name the new
  route, and the kernel resolves a group A route the stored modes leave out
  as `native` while identity is authoritative (`identity` or `finalized`)
  and as `legacy` before (`service.ResolvePackageRouteModes`). So the
  identity host serves the reset natively, with identity's credentials,
  as soon as `identity-platform` is moved, and configuration writes that
  do not name the route are not refused. A route stored explicitly as
  `legacy` still counts as legacy: group A must stay native together.
- The limit is counted in memory by the legacy handler, so with several
  replicas before the cutover each replica allows three attempts an hour
  (as for login, [container deployment](architecture/container-deployment.md));
  after the cutover identity counts them in its shared table.

### nftables Forwards Move To The `inet v2b_forward` Table

Only for forwards on the `nftables_ansible` backend. The playbooks now keep
every forward in an `inet v2b_forward` table (IPv4 and IPv6) instead of the
IPv4-only `ip v2b_forward` table, and count traffic in both directions.

- Nothing changes on the relays at upgrade time. The old rules keep working
  until the forward is next applied (created, saved, resumed, or its tunnel
  changed). That apply removes the forward from `ip v2b_forward` in the same
  nft transaction that adds it to `inet v2b_forward`, so no connection is
  translated twice; the old table is deleted with the last forward that used
  it. Pause and delete clean both tables. To migrate a forward now, pause and
  resume it, or save it unchanged.
- The relay needs Linux 5.2+ and nft 0.9.1+ (NAT chains in an `inet` table).
  On an older relay the apply fails, the old rules stay in place, and the
  forward is marked as an error.
- Traffic numbers jump to correct values after the forward migrates. Before,
  the counter sat on the NAT chain, which sees only the first packet of each
  connection, download was always 0, and with the default Ansible output
  settings the stats playbook's numbers did not reach the panel at all. Now
  upload counts every packet from client to target and download every packet
  back. Usage, quotas and quota-triggered pauses for affected users and
  tunnel grants will rise accordingly; check quotas that were sized against
  the old, too-low numbers. Totals already recorded are not changed. Until a
  forward migrates, its old counter is read (upload only).
- IPv6 targets (`[2001:db8::1]:443`) now work. `net.ipv6.conf.all.forwarding`
  is turned on only on relays with an IPv6 target; on a relay that takes its
  IPv6 default route from router advertisements, set `accept_ra=2` on that
  interface first.
- A specific tunnel listen address is now honoured: `0.0.0.0` accepts IPv4
  clients only, a concrete address only traffic to that address. Empty,
  `::` and `[::]` accept both families as before.
- `fifo` (主备) and `hash` still use only the first target, and speed limits
  are still not enforced on this path
  ([layout and limits](guide/forward-tunnel-runtime-ops.md#nftables_ansible-rule-layout)).

## Upgrading To 4.1.0: Packages Now Default To Native Routes

**Behaviour changes on upgrade.** From 4.1.0 the 151 v2 routes of the 15
packages that passed the staging rehearsal run their packages' native
handlers by default instead of the kernel's legacy handlers (decision H8).
**One setting turns this off and gives zero behaviour change:**
`package_routes.default_mode: legacy` (or
`ANIX_CONTROL_PACKAGE_ROUTES_DEFAULT_MODE=legacy`). **One command rolls a
package back:** `anix-control routes rollback --package <id>`.

The packages, by rehearsal batch (route counts in brackets; the exact list is
`config/package-route-defaults.json`):

| Batch | Packages |
|---|---|
| 1 | `knowledge` (6), `ticket` (8) |
| 2 | `notification` (23), `platform` (5), `machine-telemetry` (3), `protocol-runtime` (3) |
| 3 | `plan` (7), `order` (13), `payment` (20), `affiliate` (10) |
| 4 | `subscription` (21), `forward` (21), `proxy-node` (7), `gost-mesh` (3), `wireguard` (1) |

**What an installation sees depends on the packages it has.** The 151 routes
include the 43 of the commercial packages `affiliate` (10), `order` (13) and
`payment` (20), which the release no longer ships
([Community Edition By Default](#community-edition-by-default-set-commercial-before-upgrading)).
With the 15 packages of the published 4.1.0 archive, 108 routes of 12
packages are native by default (`GET /api/v4/kernel/route-modes`, `source`
`default`); the startup log still states the policy's 151. From v4.2 the
forward package leaves the set: 130 routes of 14 packages by policy, 87
routes of 11 packages with the packages of the published archive.

Each batch passed the R5 rehearsal on the local staging stack (every read
route at least 200 shadow comparisons with 0 mismatches over at least 2
hours; every write route reconciled against the database with 0 differences;
the SQLite and PostgreSQL parity suites green) and was signed off by the
owner (H7).

What changes, exactly:

- A listed route runs natively only when **all** of these hold: the
  installation stores no mode for it (no `routes` entry); the installed
  package release is at least `4.1.0-rc.5`, the rehearsed release (signed
  4.0.0 packages stay installable and stay `legacy`); and
  `package_routes.default_mode` is `rehearsed`, the default.
- A stored mode always wins. A route you switched with `routes set` (to
  any mode, `legacy` included) or rolled back keeps that mode.
- Nothing else defaults to native: not identity-platform (group A still
  moves only with the identity cutover), not kernel-owned or WebSocket
  routes, and not routes that become native-flagged later; a route joins
  the default set only by an explicit change in a release.
- Package hosts learn the resolved modes at their next configuration poll
  (about 5 seconds after the kernel starts); no package needs a restart.

**Before upgrading**, decide:

- Keep the 4.0 behaviour for now: set the kill switch before the new
  binary starts, then move packages one by one with `routes set` when you
  are ready (or remove the switch later).

  ```yaml
  package_routes:
    default_mode: legacy   # rehearsed (default) | legacy
  ```

- Take the defaults: nothing to do. The startup log states the policy:
  `package route defaults: policy rehearsed (package_routes.default_mode):
  151 routes of 15 packages default to native ...` (from v4.2, 130 routes
  of 14 packages: the forward package left the set, F5d).

**See what runs where** and why. `SOURCE` (`source` in the API) is `stored`
(set explicitly), `default` (native by default), `kill-switch` (default off
by `package_routes.default_mode`), `package-too-old` (the installed release
is older than the rehearsed one), `identity-authority` (identity group A)
or `unset` (legacy, no default):

```bash
anix-control routes list --package order
anix-control routes list --json | jq '[.[] | .routes[] | select(.source == "default")] | length'
```

The admin page 插件中心 → 路由模式 shows a defaulted route as
“原生（默认）” / “Native (default)” and says when a package's defaults are
off and why. `GET /api/v4/kernel/route-modes` returns `source` per route and
a `defaults` object per package (`policy`, `routes`, `min_version`,
`source`, `note`).

**Roll back** one package (no confirmation needed; the reason is optional):

```bash
anix-control routes rollback --package payment --reason "mismatch in callbacks"
# or one route
anix-control routes set --package payment --route <route-id> --mode legacy
```

Rollback and `set --mode legacy` work on the effective mode: a route that
runs natively by default gets an explicit stored `legacy`, recorded in the
revision history and the audit log like any switch, so it stays legacy
whatever the default policy becomes. A rollback while the kill switch is on
changes nothing (every unstored route is already legacy); if you want a
package pinned to legacy for when you lift the switch, lift it first and
then roll the package back, or set its routes to `legacy` explicitly.

**All packages at once**: set `package_routes.default_mode: legacy` and
restart Control. Routes with a stored `native` or `shadow` mode keep it;
`anix-control routes list` shows them with `SOURCE stored`.

## Moving Logins To The Identity Module

From 4.1 the identity module can own accounts, passwords, MFA and token
signing (`docs/architecture/identity-service.md`). Nothing moves until an
administrator runs the cutover. Until finalize, a rollback is one API call.

### Before You Start

- **Package storage.** identity-platform keeps its tables in its own package
  storage.
  - On PostgreSQL, the kernel database role needs `CREATEROLE`: it creates the
    package role and schema.
  - A remote identity module needs PostgreSQL.
- **Identity key.** Set the identity key-encryption key: `identity.kek` in
  `config.yaml` for a local host, `ANIX_IDENTITY_KEK_FILE` for a module
  container. It is 32 random bytes, base64 or hex.
  - Every identity replica needs the same key.
  - Back it up with the database. Without it, identity cannot unseal its
    signing keys or users' TOTP secrets.
  - Without a key, identity publishes no token keys and the cutover refuses
    to start.
- **Settings.** The `auth.*` settings (registration policy, attempt limits,
  MFA configuration) are copied into identity the first time a native route
  needs them. After that, change them through the admin API. `config.yaml`
  no longer applies to them.
- **Backup.** Take a database backup. Finalize removes the legacy password
  hashes and cannot be undone.

### Cutover

All calls are administrator calls. `GET /api/v4/kernel/identity` reports the
state, the import progress, the latest cutover or rollback, and the last
events.

1. **Full import.** `POST /api/v4/kernel/identity/import` with `{}`. Wait
   until `import.completed_at` is set. The state is now `importing`, and the
   legacy handlers still serve logins.
2. **Cutover.** `POST /api/v4/kernel/identity/cutover`. It runs in the
   background:
   - it imports recent changes;
   - it pauses login, registration, MFA and admin account writes: clients
     get 503 with `Retry-After` for a few seconds;
   - it imports the last changes and switches those routes to the identity
     module.

   If the identity host does not take over within 30 seconds, nothing
   changes and `authority_change.error` says why.
3. **Check.** A new login returns a token signed by identity (`alg`
   `EdDSA`).
   - Clients need no change.
   - Tokens issued before the cutover keep working until they expire or
     until finalize.
   - Control publishes the keys at `/api/v4/identity/jwks.json`.
   - `POST /api/v4/identity/logout` ends one session.

### Rollback Window

Until finalize, `POST /api/v4/kernel/identity/rollback` returns logins to the
legacy handlers. Identity mirrors password hashes and TOTP secrets back, so
accounts created or changed after the cutover keep working. Backup codes
generated after the cutover are not mirrored: those users use TOTP or
regenerate codes. A later cutover imports what changed meanwhile.

### Finalize

Run it after at least a day on identity, once no rollback is expected:
`POST /api/v4/kernel/identity/finalize` (`{"force": true}` skips the day).

- Legacy password hashes become unusable and `v2_user_mfa` is emptied.
- Control stops accepting the tokens it signed itself (HS256).
- There is no rollback afterwards. Restoring the backup also restores the old
  state.
- Control no longer creates a default administrator. Create administrators
  through the admin API.

## Agent Transports: v4.2 Requires Enrolled Agents

**From v4.2, `agent_control.mtls` defaults to `required`** (owner decision
H5): a kernel that leaves it unset accepts only enrolled (mTLS) Agents on
the AnixOps Agent channels. `preferred`, `optional` and `off` stay
selectable.

- **What `required` refuses.** API key authentication on the Agent Control
  stream and the API key bootstrap of `AgentEnrollment.Enroll` (one-time
  enrollment credentials still work); `/api/v2/agent/*`; `/api/v2/node/*`
  (register, heartbeat, runtime-health, ws); `/api/v2/forward/agent/rules`;
  the clean agent endpoints `/api/v2/forward-agent/register|heartbeat|report`.
  They answer HTTP 403 `{"code": "agent_mtls_required"}`, or gRPC
  `Unauthenticated` with the trailer `x-anix-error-code: agent_mtls_required`.
  An enrolled Agent that still sent heartbeats or maintenance events over
  REST or the WebSocket loses them: it must also negotiate the data plane
  (see "Enrolled is not enough" below).
- **What it leaves open.** UniProxy (`/api/v1|v2/server/UniProxy/*`), the
  v2board gRPC services (XrayR, V2bX and other third-party node software),
  the plugin release download `/api/v3/agent/plugin-releases/...`,
  `install.sh`, and the admin APIs.
- **Who is affected.** Only kernels that leave `agent_control.mtls` unset.
  A config file that sets `mtls: "preferred"` (the 4.1 `config.yaml.example`
  did) keeps `preferred` after the upgrade: delete the line, or set
  `required`, once the check below passes. The 4.2 templates leave it empty,
  except `config.dev.yaml.example`, which sets `preferred` for local
  API-key agents.

### Order Of Operations

1. **On 4.1, upgrade the Agent of every node that already runs one, and let
   it enroll** (the checklist in "Preparing For v4.2" below; forward nodes:
   "The New Agent First, Then Control"). Control needs `grpc.enabled`,
   `grpc.tls_cert_file` / `grpc.tls_key_file` and `module_runtime.ca_kek` for
   that. A 4.1 Control cannot serve the one-command installer (the script
   reads `/install/agent.env`, which only a 4.2 Control answers), so a node
   with no Agent to update moves after step 3, from its node page, with
   `agent_control.mtls: preferred` kept until it has
   ([Forward Nodes](#forward-nodes-the-new-agent-first-then-control),
   [Keeping `preferred` For A While](#keeping-preferred-for-a-while)).
2. **Run the gate** against the database Control uses:

   ```bash
   anix-control agents transports --check-required          # table and reasons
   anix-control agents transports --check-required --json   # for scripts
   echo $?   # 0: required refuses no enabled node; 3: it would; 2: an error
   ```

   It lists every enabled node `required` would refuse: `legacy` (its
   newest AnixOps Agent channel is a legacy one, however long ago) and
   `never_enrolled` (never seen and no valid agent certificate). Disabled
   nodes and third-party nodes (UniProxy or v2board gRPC only) do not
   count. Upgrade and enroll the listed Agents, or disable nodes you are
   retiring, until it exits 0. `GET /api/v4/kernel/agents/transports`
   answers the same in `summary.ready_for_required`,
   `summary.required_reasons` and `summary.required_blockers`.
   The flag ships with v4.2: on a 4.1 release without it, require that
   `anix-control agents transports --legacy-only` prints "refuses none" and
   that no enabled node shows `unseen` without a certificate in
   `anix-control agents transports`. Do not run the v4.2 binary against the
   4.1 database just for the check: admin commands migrate the schema
   first. The nodes you move after the upgrade (step 1) are listed too, which
   is why `preferred` stays until they have moved.
3. **Upgrade Control to v4.2.** Its startup log states the mode, and under
   `required`:
   - `WARNING: Agent transports: agent_control.mtls=required refuses N enabled
     node(s): ...` when the inventory still holds blockers: the counts (legacy
     nodes, those seen within the last 7 days, which are cut off now, and
     nodes that never enrolled), up to ten node names, and the command
     above. Control **starts anyway**: you may be retiring those nodes on
     purpose.
   - `WARNING: Agent transports: agent_control.mtls=required (the default
     since v4.2): ... agent enrollment: unavailable ...` when the gRPC
     listener, its TLS or the built-in CA is missing: no Agent can connect
     at all. The default still starts (an install without Agents, or with
     third-party node software only, needs nothing); an explicit
     `mtls: "required"` refuses to start without them, as in 4.1. Fresh
     installs now generate `module_runtime.ca_kek` and, given a publicly
     trusted certificate, enable gRPC TLS; an existing install adds them
     as described in
     ["Fresh Installs Are Ready; Adding The CA Key And gRPC TLS"](#fresh-installs-are-ready-adding-the-ca-key-and-grpc-tls).
4. **Watch** `anixops_agent_legacy_refused_total{path}` on `/metrics`.
   Refused requests are not recorded in the transport inventory, so a
   refused node keeps its last legacy sighting and drops out of the 7-day
   window after a week; it stays in `--check-required` until it enrolls or
   is disabled.

### Fresh Installs Are Ready; Adding The CA Key And gRPC TLS

Every shipped install path now prepares what enrolled Agents need on a fresh
install (owner decision, 2026-10-04):

| Path | CA key (`module_runtime.ca_kek`) | gRPC TLS |
|------|----------------------------------|----------|
| `scripts/install.sh` (systemd) | `config/secrets/module_ca_kek`, 0600, passed as `ANIX_CONTROL_MODULE_RUNTIME_CA_KEK_FILE` | `--grpc-tls-cert`/`--grpc-tls-key`, or `/etc/letsencrypt/live/<--grpc-name>/` |
| Compose (`init-secrets.sh`) | `secrets/module_ca_kek`, 0400 uid 10001 | the same flags; `tls/` mounted at `/run/anix-control/tls` |
| Helm chart 0.3.0 | Secret `<fullname>-ca-kek` (generated once, `lookup`, immutable, kept on uninstall), or `caKek.existingSecret` | `grpc.enabled` + `grpc.tls.secretName` (cert-manager) |

The key is never printed (only `sha256sum <file> | cut -c1-16`) and never
replaced. **Agents verify Control's gRPC certificate against the node's
system CA roots and cannot be given a private CA**, so the installers do not
generate a self-signed certificate: they check the certificate the way an
Agent does and refuse one that fails. Without a certificate they finish,
print the next steps, and Control logs that no Agent can connect.

**Existing installs, before v4.2.** First check whether the database already
holds a CA (a key was set once, even if it is not configured now):

```sql
SELECT cluster, state, key_id FROM v4_kernel_service_ca;
```

If it returns rows, find the key that sealed them and configure that one;
a new key cannot unseal them (`module CA … key cannot be unsealed: wrong
key-encryption key`, when Control first signs). With no rows, a new key is
safe:

- **systemd.** `sudo bash install.sh enable-agents --grpc-name grpc.example.com`
  (or `--grpc-tls-cert`/`--grpc-tls-key`). It creates the key unless
  `config.yaml` or a drop-in sets one, installs the certificate, rewrites the
  unit and restarts Control; nothing is downloaded. `update` keeps both and
  reports a missing key.
- **Compose.** The v4.2 `docker-compose.prod.yml` reads
  `secrets/module_ca_kek`; `docker compose up` fails while the file is
  missing. Run `sudo bash init-secrets.sh [--grpc-name grpc.example.com]`
  before `up`. If `control.env` sets `ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`
  (or `_FILE`), the script stops: move the value into
  `secrets/module_ca_kek` (`printf '%s' "$key" | install -m 0400 -o 10001 -g
  10001 /dev/stdin secrets/module_ca_kek`) and delete the line; Control
  refuses to start with both. With the modules overlay, the file is the one
  you already have.
- **Helm.** `helm upgrade` creates `<fullname>-ca-kek` when no key is
  configured. A key in `secrets.files` (`ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`)
  keeps precedence and no Secret is created. A key passed any other way
  (`config.ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`, extra environment) now
  collides with the chart's `_FILE` variable and Control refuses to start:
  move it into a Secret and set `caKek.existingSecret`, or
  `caKek.generate: false`. GitOps renders (Argo CD, Flux) have no `lookup`:
  use `caKek.existingSecret`. For TLS, see the chart README.

Then `anix-control agents transports --check-required`.

**Replacing the key** is a manual procedure: there is no command that
re-seals the CA, so a new key means new CAs and every Agent and module
enrolling again. Do it only when the key is lost or exposed, and rehearse it
on a copy of the database first (this is a recovery outline, not a tested
runbook): stop Control,
back up the database, delete the sealed roots (`DELETE FROM
v4_kernel_service_ca; DELETE FROM v4_kernel_forward_link_ca;`), install the
new key, start Control (it creates new CAs), then issue new enrollment
tokens for every Agent and redo the module bootstrap (trust bundle and
enrollment credential, `docs/architecture/module-runtime.md`).

### Keeping `preferred` For A While

If nodes cannot move before the upgrade, keep serving them:

```yaml
agent_control:
  mtls: "preferred"
```

or `ANIX_CONTROL_AGENT_CONTROL_MTLS=preferred`, and restart. Legacy Agents
are served again with the deprecation signals; enrolled Agents are not
affected. This is also the rollback if the upgrade cut off nodes by
surprise: no data changes with the mode. Plan to return to the default: the
legacy agent routes themselves are removed in a later release.

## v4.2: The Systemd Services Panel

The node page's 服务 section (the read-only systemd services table, owner
decision H24) is new in v4.2. Nothing is collected after the upgrade until
an administrator turns it on, node by node.

- **It needs the matching Agent.** Control 4.2, the machine-telemetry
  package from 4.1 (its release declares `telemetry.systemd.read`), and the
  Agent released with Control 4.2. That Agent collects the report in the
  plugin and sends it on the Agent Control stream (`package-reports.v1`).
  An older Agent refuses the `systemd_services` setting, and an Agent that
  is not enrolled has no stream to send it on. Upgrade and enroll the
  Agents first ([v4.2 Requires Enrolled Agents](#agent-transports-v42-requires-enrolled-agents)),
  then upgrade the machine-telemetry package and its Agent assignment.
- **Turn it on per node.** On the node page, or in the Agent installation's
  configuration of machine-telemetry:

  ```json
  {"systemd_services": {"nodes": {"12": {"enabled": true, "include": [], "exclude": ["*-debug.service"]}}}}
  ```

  The first report arrives with the Agent's next package report: within 5
  minutes, or at once when the Agent reconnects. A node without systemd or
  cgroup v2 reports `supported: false` with the reason.
- **What is collected and kept.** Per `.service` unit: the name, its
  states, CPU averaged over 10 minutes and its peak, memory and its peak.
  No descriptions, command lines, environments, paths or logs. Only the
  latest report per node is kept, and it shows as stale after 25 minutes.
  The privacy rules in full are in
  [`architecture/package-reports.md`](architecture/package-reports.md#privacy-the-systemd-services-report).
- **Turning it off** (`"enabled": false`, or removing the node's entry)
  stops collection on the node at the next configuration. The panel then
  answers "not enabled"; the last stored report stays in the database
  until a newer one replaces it.
- **Checking it.** The node's session in
  `GET /api/v4/kernel/agents/transports` lists `package-reports.v1` among
  its negotiated capabilities. Control counts reports in
  `anixops_agent_package_reports_total{result}` and refusals in
  `anixops_agent_package_reports_refused_total{reason}`; `not_assigned`,
  `version_mismatch` and `missing_capability` point to the package release
  or its assignment.

## Agent Transports: Preparing For v4.2

From 4.1.0, `agent_control.mtls` defaults to `preferred`; from v4.2 it
defaults to `required` (owner decision H5). **Before upgrading to v4.2,
every node must run an AnixOps Agent that has enrolled (mTLS); legacy
API-key agents will be refused** on the AnixOps Agent channels.

- **What 4.1.0 changes.** Nothing stops working. A kernel that left
  `agent_control.mtls` unset moves from `optional` to `preferred`: legacy
  agents are still served, and are now told so. The legacy HTTP and
  WebSocket agent paths answer `Deprecation: true` and a `Link` to this
  section (and `Sunset` when you set `agent_control.legacy_sunset`), and the
  control stream answers `x-anix-auth-deprecated`. Unlike the release candidates,
  `preferred` no longer needs gRPC TLS or the built-in CA to start; without
  them agents just cannot enroll yet. Set `optional` to stay silent, or
  `off` to stop requesting client certificates altogether.
- **What v4.2's `required` refuses.** API key authentication on the Agent
  Control stream and the API key bootstrap of `Enroll`;
  `/api/v2/agent/*`; `/api/v2/node/*` (register, heartbeat,
  runtime-health, ws); `/api/v2/forward/agent/rules`; the clean agent
  endpoints `/api/v2/forward-agent/register|heartbeat|report`. They answer
  HTTP 403 with `"code": "agent_mtls_required"`, or gRPC `Unauthenticated`
  with the trailer `x-anix-error-code: agent_mtls_required`.
- **What it does not touch.** UniProxy (`/api/v1|v2/server/UniProxy/*`) and
  the v2board gRPC services, which XrayR, V2bX and other third-party node
  software use, keep their node API keys in every mode. Nodes running such
  software need nothing.
- **The new table.** `v4_kernel_agent_transport`, created at startup,
  records which transport each node was last seen on (at most one write a
  minute per node and transport).
- **Enrolled is not enough: the Agent must also negotiate the data plane.**
  An enrolled Agent that still sends its heartbeat, runtime health or
  maintenance events over REST or the WebSocket loses them under
  `required`. Every legacy path has a stream equivalent (Agent contract,
  `PROTOCOL.md`, "Data plane"); the Agent release for v4.2 uses them. Check
  each node's live session in `GET /api/v4/kernel/agents/transports` (the
  `session` of a node: `authentication`, the certificate, and
  `negotiated_capabilities`) or `GET /admin/nodes/:id/agent-control`: a proxy
  node is ready when it shows `mtls` and negotiates `config.v1`, `users.v1`,
  `reports.v1`, `alive.v1` (device limits across nodes) and, with the plugin
  supervisor on, `maintenance.v1` and `artifacts.v1` (plugin downloads).
- **Plugin downloads by certificate.** An enrolled Agent no longer holds
  its node API key, so it downloads assigned plugin releases from the
  `AgentArtifacts` gRPC service on the agent listener, by its client
  certificate (`artifacts.v1`). The HTTP download
  `/api/v3/agent/plugin-releases/...` stays, with the node API key, in
  every mode, including `required`: it serves Agents that have not enrolled
  yet. Nothing to configure.
- **Device limits across nodes.** With `alive.v1` the Agent receives every
  user's online device count from the stream instead of pulling UniProxy
  `alivelist`. The counts and `alivelist`'s now come from the same reader,
  which also fixes `alivelist` answering an empty list with the built-in
  memory cache: device limits that counted only one node's connections now
  count every node's, for Agents on either path.
- **Diagnostic tasks reach stream Agents.** `POST /admin/agent/tasks` and
  `/admin/agent/execute` (NodeX Agents → 诊断) send the task on the Agent
  Control stream when the node has no agent WebSocket and its Agent
  advertises `agent.diagnostic`; before, they answered "node offline". The
  answer adds `"channel": "agent_control"`; the result reaches the task as
  before.
- **Maintenance events land in the node log.** With `maintenance.v1`, the
  plugin supervisor's incidents and recoveries (until now sent only on the
  WebSocket, which Control did not answer) are stored once each as node log
  entries of source `maintenance`; `/metrics` counts them in
  `anixops_agent_maintenance_events_total{result}`.
- **Why an Agent was refused.** Refusals of a certificate carry a code the
  Agent logs: `agent_cert_revoked` (also a disabled or deleted node),
  `agent_cert_expired`, `agent_cert_invalid`, `agent_cert_wrong_cluster`,
  `agent_cert_wrong_node` (a configuration error on the node), and
  `agent_enrollment_rejected` for an unusable enrollment credential. The
  first four make the Agent enroll again. Acknowledgements of reports,
  maintenance events and configuration carry codes too (`report_*`,
  `maintenance_*`, `config_*`); a failed configuration's code is kept with
  the node's configuration status and leads the failed `node.sync`'s
  message (`config_apply_failed: ...`).

### The Checklist

```bash
# Which mode runs, and every node with its last transport, agent version,
# certificate and last sighting.
anix-control agents transports

# The nodes v4.2 would refuse. This must print "refuses none" before you
# upgrade (or before you set agent_control.mtls: required yourself).
anix-control agents transports --legacy-only

# From v4.2: the upgrade gate, legacy and never-enrolled enabled nodes;
# exit status 3 while there is one.
anix-control agents transports --check-required

# The same for scripts.
anix-control agents transports --legacy-only --json
```

The admin page NodeX Agents → Agent 连接方式 (`/admin/agent/transports`) and
`GET /api/v4/kernel/agents/transports?legacy_only=true` show the same.
A node's status follows its newest AnixOps Agent channel:

| Status | Meaning | Action before v4.2 |
|---|---|---|
| `mtls` | its agent uses the mTLS stream | none |
| `legacy` | its agent still uses the API key stream, the legacy HTTP paths, the WebSocket or a clean agent | upgrade the agent and let it enroll |
| `third-party` | seen on UniProxy or v2board gRPC only | none (not affected); if it is in fact an old anix-agent, upgrade it too |
| `unseen` | no sighting since 4.1.0 | check the node; it may be offline |

To move a `legacy` node:

1. Give the kernel what enrollment needs: `module_runtime.ca_kek`
   (`ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`), `grpc.enabled: true` and
   `grpc.tls_cert_file`/`grpc.tls_key_file`. The startup log line
   `Agent transports: ... agent enrollment: available` confirms it.
2. Upgrade anix-agent to 4.2.0-rc.1 or later, the first release with
   enrollment (on a root install `sudo anix-agent update <tag>`, see
   `docs/INSTALL.md` in the anix-agent repository). It enrolls with its
   existing node key on its next start; a new node can use a one-time
   credential (`anix-control agent token create -node proxy-12`).
3. Watch the node turn `mtls` in `anix-control agents transports` (within
   a minute), and `anixops_agent_legacy_requests_total{path}` on `/metrics`
   stop growing.
4. Optionally rehearse v4.2 early: set `agent_control.mtls: required` on
   4.1.0 and watch `anixops_agent_legacy_refused_total{path}`. Roll back by
   setting `preferred` and restarting.

To announce a date to legacy agents, set
`agent_control.legacy_sunset: "YYYY-MM-DD"`
(`ANIX_CONTROL_AGENT_CONTROL_LEGACY_SUNSET`); it is sent as the `Sunset`
header and the `x-anix-auth-sunset` stream metadata. It is unset by default
because the v4.2 release date is not fixed.

### Forward Nodes: The New Agent First, Then Control

The steps above are written for proxy nodes. Forward nodes have one more
constraint: v4.2 replaces the flux-compatible forwarding, retires the clean
agent and NodeX, and (once you confirm it) drops the old forwarding tables. A
clean agent node still on its old agent when Control moves to v4.2 is refused
(`agent_mtls_required`), and Control can then no longer clean it. Upgrade in
this order. The one-command installer is served by a 4.2 Control only, so
with Control still on 4.1 the nodes that have no anix-agent to update wait
until Control is upgraded (step 3) and move afterwards ("Step 1 needs the
installer of a 4.2 Control", after the list):

1. **Update the anix-agent first, on every forward node that already runs
   one**, while Control is still on 4.1: install 4.2.0-rc.1 or later (on a
   root install `sudo anix-agent update <tag>`) and let it enroll ("To move
   a `legacy` node" above). A clean agent, NodeX or Ansible node has no
   anix-agent to update, and a 4.1 Control cannot serve it the one-command
   installer: it moves after Control is upgraded, as described below the
   list. Removing the legacy forward runtime locally, without Control, is
   the one-command installer's work: the `inet v2b_forward` and
   `ip v2b_forward` tables, the `ip anixops_forward` table, and the gost
   services the flux runtime or the clean agent created. It touches no other
   table or service. The Agent's own installer replaces the Agent only, so a
   node that still has one of them takes the one-command installer as well
   once Control is on 4.2.
2. **Check** that `anix-control agents transports --legacy-only` prints
   "refuses none" (with the v4.2 command line,
   `anix-control agents transports --check-required` exits 0), except for
   the nodes that wait for the upgrade: a clean agent node shows as
   `legacy`, a NodeX or Ansible node as `unseen` without a certificate.
   Setting `agent_control.mtls: preferred` before the upgrade (below) keeps
   the clean agents among them served.
3. **Then upgrade Control to v4.2.** Its upgrade checks that every forward
   node is clean before it drops the old tables, and asks you to confirm that
   step on its own: it cannot be undone ("Forwarding: Archive, Clean The
   Nodes, Drop The Old Tables" below). Control cleans NodeX hosts through
   NodeX's HTTP API and Ansible hosts over SSH; neither depends on
   `agent_control.mtls`.

That Agent release is anix-agent 4.2.0-rc.1 or later: the first with
enrollment, the credential-only configuration the install script writes, and
`upgrade.v1`. The candidates `v4.2.0-rc.1` to `v4.2.0-rc.4` are published, and
the Agent's 4.2.0 release comes with Control 4.2.0, since the two share a
version number (H25). The same order is in "Order Of Operations" above
([design](architecture/forward-sdk.md#10-upgrade-from-v41)).

**Step 1 needs the installer of a 4.2 Control.** The one-command installer
below is served by Control 4.2 (`/install.sh`, with `/install/agent.env`). A
4.1 Control has neither (its `/install.sh` is the clean agent's, under
`/api/v2/forward-agent/`), so the script cannot read the Agent release from it
and stops before it changes anything. On 4.1, step 1 therefore holds only for
the nodes you can update with the Agent's own installer. For the others, which
have no anix-agent to update (clean agent, NodeX and Ansible nodes), keep them
served while Control moves first:

1. Set `agent_control.mtls: preferred` (`ANIX_CONTROL_AGENT_CONTROL_MTLS`)
   before the upgrade, so the clean agents' register, heartbeat and report
   stay served after it
   ([Keeping `preferred` For A While](#keeping-preferred-for-a-while)).
2. Upgrade Control to v4.2.
3. Run each of those nodes' install command from its page (below). It removes
   the legacy runtime on the node and enrolls the Agent.
4. When `anix-control agents transports --check-required` exits 0, remove
   the setting and restart. "Forwarding: Archive, Clean The Nodes, Drop The
   Old Tables" follows as written; until step 3, `forward legacy check`
   reports these nodes unreachable.

#### Installing Or Switching A Node With One Command

A 4.2 Control serves the installer for step 1: on the node's page,
**复制安装命令** issues a single-use enrollment token for the node (super
administrators; 1 hour by default, at most 7 days) and prints the command to
paste on the node as root:

```sh
curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- \
  --control https://panel.example.com --node forward-41 --token anixagt_...
```

It installs the Agent as the unprivileged `anixops-agent` service, removes
the legacy forward runtime of that machine (the tables above and the clean
agent's `v2forward-agent` service) and lists what it removed, then waits for
the Agent to enroll. On a node that already runs the Agent the same command
upgrades it in place and keeps its identity. Before the first command:

- Control needs the built-in agent CA and TLS on its gRPC listener (the
  checklist above), and an https address nodes reach:
  `agent_install.public_url` unless the request's origin is already right.
- The node needs systemd, root, `curl`, `sha256sum` and `unzip`.
- It needs anix-agent 4.2.0-rc.1 or later, which accepts the credential-only
  configuration the script writes; earlier Agents refuse it (`ApiKey is
  required`, and the script stops at "did not enroll"). That release is the
  one step 1 names.
- The installer removes only objects it can name: the three tables and the
  clean agent. gost services the flux runtime created through gost's API on
  NodeX hosts live in a gost the operator installed; Control's upgrade
  cleans them through NodeX's API (step 3), and the installer leaves that
  gost alone.

- The installer checks the node first (preflight) and changes nothing when
  a check fails: systemd 240 or later; on forward nodes Linux 5.10 and
  nftables 0.9.7 or later; free disk space; the clock within 5 minutes of
  Control's; and Control's https address and gRPC target with verified TLS.
  It warns about polkit older than 0.106 (Ubuntu 22.04: gost hops cannot
  run there), firewalld, ufw or a Docker `FORWARD DROP` policy, and IPv6
  SLAAC interfaces (`--accept-ra`). Fix a failure, or add `--skip-preflight`.
- Nodes without internet access install from an offline bundle:
  `anix-control agent offline-bundle -arch amd64 -o agent-offline-amd64.tar.gz`
  (needs the Agent release with `SHA256SUMS` and the `.sig` files in
  `agent_install.artifact_dir`), copied to the node and installed with
  `--offline <file>`. Enrolling still needs the node to reach Control's gRPC
  target.
- `install.sh uninstall` removes the Agent and keeps its identity;
  `uninstall --purge` also removes its state, users and the forwarding
  objects the drivers created. Revoke the node's credentials (or delete
  the node) in Control afterwards.

The [onboarding guide](guide/agent-onboarding.md) has the details: mirrors,
`--reset`, preflight, offline bundles, uninstalling, verifying the signed
script and troubleshooting.

### Forward Link Certificates: A Second CA Under The Same Key

Encrypted links between forward nodes (gost's TLS, WSS, QUIC and gRPC
links) use per-node link certificates from a dedicated forward link CA
(owner decision H28; `sdk/api/agent/v1/PROTOCOL.md`, "Forward link
certificates").

- **What is new.** The tables `v4_kernel_forward_link_ca` (the link CAs,
  each key sealed with AES-256-GCM) and
  `v4_kernel_forward_link_certificate` (every issued link certificate),
  created at startup; the RPCs `AgentEnrollment.IssueLinkCertificate` and
  `GetLinkTrustBundle` on the agent listener; and
  `anix-control agent link-ca list|bundle|rotate`.
- **Key material.** Nothing to configure. Wherever the built-in CA runs
  (`module_runtime.ca_kek` with `module_runtime.pki: builtin`), the first
  start creates the link CA, an ECDSA P-256 root separate from the module
  CA, and seals its key with the same `module_runtime.ca_kek`. The key
  therefore now protects two CAs: keep it secret and backed up as before,
  and restore it with the database. Changing it leaves both CAs unusable
  (`wrong key-encryption key` on the first issuance). With an external PKI
  the kernel holds no CA key and issues no link certificates
  (`link_cert_unavailable`).
- **Who gets one.** Only an enrolled Agent (client certificate, never the
  node API key or token) of an enabled node whose Agent negotiated
  `forward.v1`. Older Agents never ask; nothing changes for them.
- **Rotation.** `anix-control agent link-ca rotate` creates the next link
  CA. It is in the link trust bundle at once and signs from 7 days later
  (the kernel promotes it within the hour after that); the retired CA
  stays trusted for 7 more days. Rotate well before the CA's 5-year
  validity ends.
- **Revocation.** Disabling, deleting or retiring a node, or replacing its
  credentials, revokes its link certificates with its agent certificates.
  gost does not check revocation, so a revoked link certificate verifies at
  its peers until it expires (at most 7 days); remove the node from its
  routes to drop its addresses from its peers' admission at once.

## Forward v4 API (v4.2)

v4.2 adds the forwarding API `/api/v4/forward/*` and the command line
`anix-control forward routes|nodes|stats`. The reference is
[`forwarding/v4-api.md`](forwarding/v4-api.md); the design is
[`architecture/forward-sdk.md`](architecture/forward-sdk.md) (F5a).

- **Install the forward package of the same release.**
  - The API is the forward package's control route on the kernel's
    `ForwardControl`. The package's manifest gains the `kernel.forward.v1`
    capability and the control route `/api/v4/plugins/forward/*`.
  - Until the package of this release is installed and healthy,
    `/api/v4/forward/*` answers `404 plugin_route_not_found`.
- **Permissions.**
  - Administrators may read and write as on every package control route.
    While no access group grants anything for the forward package, every
    administrator may. Once a group with a `forward.api` grant exists, only
    its members may.
  - Every `DELETE` (a route, a forward node, an Ansible machine) needs a
    super administrator.
  - Writes are audited as module `forward`.
- **The flux v2 routes are gone** (F5d, below). The 19 v2 node management
  routes (forward nodes, Ansible machines, observability) map to the v4 API:
  check is the node view, and sync-stats is the traffic ledger. Forward
  nodes stay rows of `v2_forward_node`.
- **Credentials.** No v4 answer shows a forward node's API token. A node
  added through the v4 API enrolls its Agent with an install token.
- **Node writes and routes.** A node that a v4 route uses cannot be disabled
  or deleted until the route moves off it (`409`, code `node_in_use`).
- **Editions.** The community edition serves the whole API (H23). The
  prefixes `/api/v4/forward/self/`, `/plans/` and `/multipliers/` are
  reserved for the commercial edition's v4.3 features and do not exist in
  the community edition.
- **Rollback.** The API adds no table of its own: the routes, inventory and
  ledger are the kernel's F3a tables (`v4_kernel_forward_*`), which the first
  v4.2 start creates. Rolling back to a release without it only removes the
  API and the commands.

## Forwarding: Archive, Clean The Nodes, Drop The Old Tables (v4.2)

v4.2 does not migrate the v4.1 flux forwarding (forwards, tunnels, user
tunnel grants, speed limits, legacy rules, runtime jobs): you reconfigure
forwarding as v4 routes. The upgrade archives the old data, checks that no
forward node still runs the old runtime, and, only when you confirm it on
the command line, drops the old tables
([design](architecture/forward-sdk.md#10-upgrade-from-v41), F5c).

> **The drop is IRREVERSIBLE.** After it, rolling back to 4.1 needs the
> database backup you took before it, and forwarding must be reconfigured
> either way. There is no button for it in the web UI on purpose: it runs
> only from the command line, with Control stopped.

Do this after the order above (the new Agent on every forward node, then
Control v4.2). Step by step:

1. **The archive.** The first v4.2 start (or the first `anix-control
   migrate`) writes one to the data directory
   (`config/data/forward-legacy/`, or next to the SQLite database) and logs
   its path and SHA-256. Write another, anywhere, at any time:

   ```bash
   anix-control forward legacy archive -o /root/forward-legacy/
   ```

   It is one JSON file (mode 0600) with every row of the old forwarding
   tables and, for reference, the forward nodes and clean agents; node
   tokens and other secrets are left out. It never overwrites a file: a
   directory (a path ending in `/` is created when missing) gets a
   timestamped name. A super administrator can also download the newest
   one: `GET /api/v4/forward/legacy/archive` (audited).

   **On the Compose deployment the startup archive is not written.** The
   container's root file system is read-only, so the first start logs
   `WARNING: the v4.1 forwarding data was not archived: create the archive
   directory: mkdir config/data: read-only file system` and goes on. Write
   the archive yourself with `-o` (without it the command tries
   `config/data/forward-legacy` and fails the same way), into a place that
   outlives the container, since `forward legacy drop` reads the recorded
   file again from its own container:

   ```bash
   docker compose -f docker-compose.prod.yml exec control \
     /app/anix-control forward legacy archive -o /var/lib/anixops/forward-legacy/
   ```

   `/var/lib/anixops` is the writable `plugin-artifacts` volume (`/tmp` is
   a RAM disk). The volume is meant to be disposable, so also copy the file
   out now with the download above, or bind-mount a host directory in a
   Compose override and pass that to `-o`.
2. **Check the nodes.**

   ```bash
   anix-control forward legacy check        # or --node <name|forward-id>
   anix-control forward legacy status
   ```

   Each forward node is `clean`, `dirty` or `unreachable`:
   - a node running the enrolled Agent is clean: its installer removed the
     old tables and the clean agent. An Agent installed another way: run
     the node's install command again (it keeps the node's identity);
   - Control deletes the old gost services of NodeX hosts through NodeX's
     API (`forward.runtime.nodex.base_url` and `token` must still be set),
     and runs `config/deploy/ansible/playbooks/forward_legacy_cleanup.yml`
     on Ansible hosts with the Ansible settings forwarding used;
   - a node still on the clean agent or another legacy channel is
     unreachable: install the new Agent and check again;
   - a node with no enrolled Agent, no clean agent, not an Ansible machine
     and referenced by no old forward or rule is reported clean ("no legacy
     forward runtime was placed on this node") without being contacted:
     check such hosts yourself if they ever ran forwarding by hand.

   A `dirty` node still has old rules: fix the cause in the detail and
   check again. A node you cannot reach any more (returned, broken) can be
   abandoned by name, with a reason; its old rules, if any, stay on it:

   ```bash
   anix-control forward legacy abandon jp-exit-2 --reason "returned to the provider"
   ```
3. **Back up the database.** On PostgreSQL `pg_dump`; on SQLite a backup of
   type database or full in Control's backup settings (or a copy of the
   file). It must be at most 24 hours old.
4. **Stop every Control process** (all replicas) and wait 30 seconds (the
   singleton lease expires).
5. **Drop.** The phrase must be exact:

   ```bash
   anix-control forward legacy drop --confirm "DROP v4.1 FORWARDING TABLES" \
     --backup-taken /root/backup/anix-control-20261004.dump
   ```

   It refuses, listing every reason, unless the newest archive is readable,
   unchanged and current (archive again if the old data changed), every
   forward node is clean or abandoned, no installed package release still
   adopts one of the old tables (the forward package of this release no
   longer does; an older one would lose its storage, so upgrade the package
   first), no Control holds the lease, and the
   backup exists (`--backup-taken` may be left out on SQLite when Control's
   own backup of the last 24 hours exists). It then drops, in one
   transaction: `v2_forward_port_binding`, `v2_forward_traffic_cursor`,
   `v2_forward_agent_bridge_task`, `v2_forward_runtime_job`,
   `v2_forward_user_tunnel`, `v2_speed_limit`, `v2_forward`,
   `v2_forward_tunnel`, `v2_forward_rule`, `v2_forward_route`,
   `v2_forward_log` and `v2_forward_stats`. A table already gone is
   reported. `v2_forward_node` (the node inventory) and
   `v2_forward_clean_agent` are kept.
6. **Start Control.** It no longer creates the dropped tables or runs the
   old forwarding workers. `forward legacy status` shows when the drop ran.

**Rollback.** Before the drop, rolling back to 4.1 is the usual binary
rollback: the old tables are untouched (the upgrade adds only
`v4_forward_legacy_archive`, `v4_forward_legacy_node` and
`v4_forward_legacy_drop`). After it, 4.1 needs the database restored from
the backup of step 3; anything changed since then is lost.

## Flux Forwarding API Removed (v4.2)

**v4.2 drops flux compatibility (F5d, owner decision).** The flux v2
forwarding API and the flux-clone pages are removed; `/api/v4/forward/*`
([`forwarding/v4-api.md`](forwarding/v4-api.md)) and the 转发 area replace
them. Old forwarding data is not migrated: the legacy cleanup (F5c)
archives the old tables and then drops them
(["Forwarding: Archive, Clean The Nodes, Drop The Old Tables"](#forwarding-archive-clean-the-nodes-drop-the-old-tables-v42)).

- **Removed (53 routes, they answer 404):**
  - forwards: `POST /api/v2/forward/{create,update,delete,force-delete,pause,resume,diagnose}`
    and the same seven under `/api/v2/admin/forward/`;
  - legacy rules: `/api/v2/admin/forward/rules` and `/rules/:id` (list,
    create, get, update, delete, toggle) and `POST /api/v2/user/forward/rules`;
  - `POST /api/v2/admin/forward/sync-backend`, `GET /api/v2/admin/forward/runtime/jobs`;
  - tunnels and permissions: `POST /api/v2/admin/tunnel/{diagnose,update}`,
    `POST /api/v2/tunnel/user/{remove,update}` and
    `/api/v2/admin/tunnel/user/{remove,update}`, `POST /api/v2/speed-limit/update`;
  - forward nodes and Ansible machines: `/api/v2/admin/forward/nodes*` and
    `/api/v2/admin/forward/ansible-machines*` (8 each); use
    `/api/v4/forward/nodes` and `/api/v4/forward/ansible-machines`;
  - observability: `GET /api/v2/admin/forward/observability/{targets,trend,topology}`;
    use `/api/v4/forward/observability/*` (the trend is now hourly traffic);
  - clean agents: `GET`/`POST /api/v2/admin/forward/agents`,
    `POST /api/v2/admin/forward/agents/:id/revoke` and
    `GET /api/v2/forward-agent/install.sh`; nodes enrol the new Agent
    instead (`/install.sh`, install tokens).
- **Still served, without pages, until the legacy runtime goes (F5c):** the
  forward and tunnel lists and order, tunnel creation and deletion,
  permission assignment and list, speed limit create/list/delete/tunnels,
  `POST /api/v2/user/reset`, multi-ingress and statistics, the runtime,
  local and NodeX status and doctor, the clean agent register, heartbeat
  and report, and the internal traffic upload/report/snapshot.
- **UI.** `/admin/forward` opens the forwarding overview; every removed page
  (转发（旧版）, 转发节点（旧版）, the setup wizard, tunnels, speed limits,
  Ansible machines, the local and NodeX runtimes) opens it too. Without the
  forward package's v4 API the forwarding area leads to 插件中心. 流量与监控
  no longer has 节点延迟 or 转发, 用户 no longer has the tunnel grant
  dialog, and 系统设置 → 转发运行时 no longer lists runtime jobs.
- **Existing forwards** keep running on their old runtime until you clean
  it up (F5c, [`architecture/forward-sdk.md`](architecture/forward-sdk.md)
  section 10); nothing can change them from the UI or the removed routes.
  Recreate them as v4 routes.
- **Install the forward package of this release before the legacy cleanup
  (F5c).** It adopts no table and reads no kernel view any more (its
  manifest declares only `kernel.forward.v1`); the remaining forward v2
  routes, `POST /api/v2/user/reset` (用户 → 重置流量) among them, are served
  by the kernel whatever route mode is stored for them, and the forward
  package left `config/package-route-defaults.json`. A 4.1 forward package
  adopts the flux tables, and a storage lease fails as a whole when an
  adopted table is missing, so after F5c's drop it would not start and
  `/api/v4/forward/*` would go down with it.
- **Rollback.** No table changes in F5d: rolling back to 4.1 brings the
  routes and pages back over the same data, as long as F5c has not dropped
  the tables.

## Staged Agent Upgrades (v4.2)

v4.2 lets Control push Agent releases to the nodes in canary batches
(owner decision H19; design: [`architecture/forward-sdk.md`](architecture/forward-sdk.md),
section 9, "Upgrades (O4)"; protocol: `sdk/api/agent/v1/PROTOCOL.md`,
"Agent upgrades").

- **New tables.** `v4_kernel_agent_upgrade_campaign` and
  `v4_kernel_agent_upgrade_node` are created at startup (protected kernel
  tables; no existing table changes). Rolling back to a release without
  them leaves them unused.
- **Agents must be re-installed once.** Control can upgrade only an Agent
  that negotiates `upgrade.v1`, which needs the privileged updater units
  (`anixops-agent-updater.path`, `anixops-agent-updater.service`) that this
  release's `install.sh` writes. Agents installed before it are skipped by a
  campaign (`upgrade_unsupported`) and are upgraded by re-running the
  install command (it keeps the identity, see the
  [onboarding guide](guide/agent-onboarding.md#running-it-again)).
- **Put the release in `agent_install.artifact_dir`.** A campaign starts
  only when `<artifact_dir>/<tag>/` has both architectures' zips with
  their `.sig`, `SHA256SUMS` and `SHA256SUMS.sig`, all verifying with
  `plugins.official_public_key`, and `agent_install.public_url` is the
  https address nodes download from.
- **Run it.** The Control version and the Agent version are the same
  (H25), and a Control hands out only its own release: a campaign refuses a
  `target_version` newer than Control, and `/install/agent.env` names `v` +
  Control's version unless `agent_install.agent_version` is set. On a 4.2
  Control, upgrade Control first and then the Agents; an Agent one release
  behind keeps working meanwhile (the Agent contract only grows).

  ```bash
  anix-control agent upgrade start -reason "v4.2.0"   # or POST /api/v4/kernel/agents/upgrades
  anix-control agent upgrade status                    # batches and nodes
  anix-control agent upgrade pause|resume -id <campaign>
  anix-control agent upgrade abort -id <campaign> [-rollback]
  ```

  Batches take 5%, 25% and 100% of the nodes, 30 minutes each at least, so a
  campaign lasts at least 90 minutes. A batch in which more than 5% of the
  offered nodes fail (refused, failed to apply, or no reconnect with the new
  version within 10 minutes) is rolled back and the campaign stops; fix the
  cause shown per node and start a new campaign. Forwarding keeps running
  during an Agent upgrade.

## The Official Signing Root Changes (v4.2)

The previous official release key could not be recovered, so **4.2.0 is
signed with a new Ed25519 key**. The new public root is
`jW26nr2tbthASoeq6RmIpx8Ah+uhPNIv9V1ewRVb1VE=`; the SHA-256 of the raw key is
`83fe4c1bed0bb2ed1b9f31ba873b799ead676835b6086a8c2bc2d272a96ae5de`. Take the
key from this document or the release notes and keep your own copy: it is
the value to pin, and a release asset cannot vouch for itself. Releases up to
4.1 stay signed by the old key and verify only against it.

This is a root rotation, so read
[`guide/release-root-rotation.md`](guide/release-root-rotation.md) ("How
Control Treats The Root") too. What it means for an upgrade:

- **Control.** A Control has exactly one active root. The first start with
  the new root records it and retires the old one; every package release
  admitted under the old root then fails closed until the 4.2 packages,
  signed with the new root, are imported and enabled. **`identity-platform`
  is one of them, so login would be down too**, and it is the one package
  Control repairs itself: the image's bootstrap package is signed with the new
  root, and the bootstrap moves the dead installation to it on that start
  ([Identity-Platform Recovers By Itself](#identity-platform-recovers-by-itself);
  4.2.0-rc.1 lacks this, see "The Window"). **Every other package needs your
  import**, signed with the new root and at a new version. Plan a maintenance
  window from the restart until the import finishes.
- **Configuration.** The 4.2 templates carry the new root in
  `plugins.official_public_key` (`ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY`).
  A configuration that copied the 4.1 value must be changed: the 4.2 image
  then refuses to start (`migrate` exits 1 with `bootstrap identity platform
  package: verify identity bootstrap package: plugin signature verification
  failed`; [Docker Compose Upgrade](#docker-compose-upgrade)). One that leaves
  the key out takes the new default. If
  `plugins.identity_bootstrap_package_dir` is set, replace the bootstrap
  `identity-platform` package there with the 4.2 build before restarting.
- **Commercial packages.** See the warning
  [below](#commercial-packages-have-no-new-root-build): `affiliate`, `order`
  and `payment` are not in the release and have no build signed with the new
  root.
- **Agents.** Verify the `agent-install.sh` release asset of 4.2 (the
  `/install.sh` of a 4.2 Control is the same file) against the new root; the
  commands are in the script's header. It runs only against a 4.2 Control: it
  reads that Control's `/install/agent.env`, which a 4.1 Control does not have
  (`/install.sh` answers 404 there). From 4.1 the Agents therefore move in
  the order of ["Order Of Operations"](#order-of-operations): the nodes that
  run an anix-agent first, with its own installer, and the others from their
  node pages once Control is on 4.2. An Agent built before 4.2 cannot verify
  the new root; a node that
  enables the plugin supervisor sets `PluginOfficialPublicKey` itself and
  must change it to the new root before it takes packages signed with it.
  Verified for v4.2.0-rc.1: the script and the Agent zip verify against the
  new root and fail against the old one, `SHA256SUMS` verifies against the
  new root, and the Agent enrolls with a credential-only configuration.
- **Pinned roots.** If you pin the root outside Control
  (`ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY` in
  [`guide/release-installation.md`](guide/release-installation.md)), change it
  to the new value.
- **Rollback.** Redeploy the previous release and configuration. Startup
  re-activates the old root's record and retires the new one, so packages
  signed with the old root verify again and those signed with the new root
  stop verifying. Once the 4.2 packages are installed, the installations
  must also be pointed back at 4.1.0
  ([Rolling Back After The Import](#rolling-back-after-the-import)).

### Commercial Packages Have No New-Root Build

> **Warning for operators of the commercial edition.** `order`, `payment`
> and `affiliate` are **not shipped** in this repository's release (the
> 4.2.0-rc.1 archive holds 15 packages, none of them) and there is **no
> build of them signed with the new root**. An installation that runs them
> has releases bound to the old root; they stop verifying at the first start
> with the new root and nothing replaces them, so orders, payments, plan
> purchase and the invite commission stay down after the upgrade, and the
> automatic recovery below does not cover them. They must be rebuilt and
> signed with the new official key: the published release builds the
> community package set only, and signing needs the release key, which only
> the release owner holds. The release owner builds them with the manual
> **Commercial Packages** workflow (`.github/workflows/commercial-packages.yml`):
> give it a release tag and it builds `order`, `payment` and `affiliate` at
> that tag's version, signs them with the official key, verifies them against
> the configured root, and publishes them as a workflow artifact (and, with
> `attach`, as release assets next to a `SHA256SUMS-commercial.txt`). Import
> them the same way as the other packages, at a version newer than the old
> one, **before** the first start of a Control that runs them; until you have
> them, do not upgrade that Control. The staging rehearsal ran the community
> package set and did not cover this.

### Identity-Platform Recovers By Itself

After a root change the installation of `identity-platform` points at a
release bound to the retired root, so it cannot start and nobody can log in
(login is served by it). From the build that carries this change
(4.2.0-rc.1 and earlier do not), the bootstrap import that runs at every
start repairs exactly that installation when **all** of these hold:

- `plugins.identity_bootstrap_package_dir` holds an `identity-platform`
  package, as in the 4.2 image and in `scripts/install.sh` installs, and it
  verifies under the **active** root (the configured
  `plugins.official_public_key`) with the same checks as a first bootstrap
  (signature, manifest, v2 Control package without dependencies, artifact
  hash). A package that does not verify still stops the start with
  `bootstrap identity platform package: verify identity bootstrap package:
  plugin signature verification failed`, and changes nothing;
- the installation exists, is **enabled**, and the release it desires is
  bound to a root that is not the active one (the "official plugin trust
  root is required" condition). On the active root a bootstrap never
  upgrades a healthy installation, as in rc.4;
- the bootstrap release is **newer** (strictly, by full `X.Y.Z[-pre]`
  version) than the installed one. Releases are immutable per version, so a
  package re-signed under the same version is refused with `existing identity
  release uses a different trust root`: build the bootstrap package at a new
  version;
- the move passes the validation of an administrator's update
  (`PUT /api/v3/plugin-installations`: dependency and conflict rules, stored
  artifact).

It then does what that update does and nothing more: the installation's
desired version becomes the bootstrap release, its previous version the one
it left, its state `pending`, and its lifecycle generation goes up by one
(the migration ledger runs the new release as a new generation). The lifecycle
worker that starts with Control brings it up on the same boot, host start and
package migrations included, and login works as soon as it is healthy. The
move is one database transaction with its audit entry (`kernel` module,
action `bootstrap_installation_update`, actor `system/bootstrap`, both
versions and both root fingerprints) and one log line:

```text
WARNING: identity-platform installation 1 (target control) moved from release 4.1.0 (trust root a3cec15e..., retired) to the bootstrap release 4.2.0 (trust root 83fe4c1b..., active), lifecycle generation 3 -> 4. ...
```

A second start finds the installation on the active root and changes
nothing. **What it never does:** touch any other package (every other
installation stays on its old-root release until you import it), move an
installation an administrator disabled, move to an older or not provably
newer release (the start goes on and logs `WARNING ... the installation is not
moved`, login stays down, and the fallback below applies), or move anything
when the validation fails (same warning with the reason). Its audit entry and
the warning are what to look for if login did not come back.

Verified by service-level tests on SQLite and PostgreSQL (retired root, same
root, a third key, other packages, idempotence, older and unusable versions,
a disabled installation, the validation, the audit, the migration ledger) and
a gateway test (login `404` before, `200` after the worker ran). It has not
been rehearsed on a staging image yet; the rehearsal of 4.2.0-rc.1 is the one
that found the lockout.

**Fallback (4.2.0-rc.1 and earlier, or when the recovery did not apply):**
move the installation by hand with a session token issued before the restart
([Upgrade Procedure](#upgrade-procedure), step 3).

### The Window

From the first start with the new root until `identity-platform` runs again
(rehearsed on Compose and PostgreSQL, 4.1.0 to 4.2.0-rc.1, all 15 packages of
the community archive installed, **without** the automatic recovery above: on
a build with it `identity-platform` runs again as soon as the lifecycle worker
has started it on that boot, and only the rows about the other packages
remain):

| What | Answer |
|---|---|
| `/readyz`, `/health` | 200 |
| `/s/<token>`, `/api/v1/client/subscribe` | 200, unchanged: the kernel serves them |
| `POST /api/v2/login` | `404 package_route_not_found` ("package route is not declared"): there is no login |
| every other `/api/v2` business route, with a session token issued before the restart | `404 package_route_not_found`, not `503 package_unavailable`: the installations are `failed` (`plugin host unavailable: verified artifact reference is unavailable`) and declare no routes |
| `/api/v4/forward/*` | `404 plugin_route_not_found` until the 4.2 forward package runs |
| `/api/v3/*` and `/api/v4/kernel/*`, with that token | work: the kernel verifies the token with `jwt.secret` |
| `anix-control routes list` | `error: verify plugin release: official plugin trust root is required` per package, no routes |
| Agent channels | `agent_control.mtls: required` answers as ever (403 `agent_mtls_required`) |

**4.2.0-rc.1 has no login in the window.** A token issued before the restart
keeps working for `jwt.expire` seconds (24 hours by default). Nothing else
reaches the admin API: Control has no command-line way to import a release or
to sign in. The 4.2.0-rc.1 bootstrap registers its `identity-platform`
release on start but leaves an existing installation on the old version (as
[rc.3 → rc.4 Checklist](#rc3--rc4-checklist) step 3 already says), so login
comes back only when an administrator moves the installation. Later builds
move it themselves
([Identity-Platform Recovers By Itself](#identity-platform-recovers-by-itself)).
The session token path was rehearsed before the identity cutover ("Moving
Logins To The Identity Module"), when the kernel signs and verifies the tokens
itself (HS256). After the cutover, identity signs them (EdDSA) and publishes
the keys that verify them, and it is down in the window (`Identity token keys
not refreshed: plugin host unavailable`): that path was not rehearsed and may
not work, which leaves the rollback or the database backup.

### Upgrade Procedure

1. **Before the restart**, as a super administrator, take a session token
   and keep it where the person doing the upgrade can reach it, never in a
   ticket. It is the way in on 4.2.0-rc.1 and the fallback on later builds
   (when `identity-platform` did not recover by itself):

   ```bash
   curl -fsS -X POST "$PANEL/api/v2/login" -H 'Content-Type: application/json' \
     -d '{"email":"admin@example.com","password":"..."}' | jq -r .data.token
   ```

2. Back up the database, then upgrade
   ([Docker Compose Upgrade](#docker-compose-upgrade): `init-secrets.sh`, the
   new root in `control.env`). Control starts; the window begins.
3. **`identity-platform` first.** A build with
   [the automatic recovery](#identity-platform-recovers-by-itself) moves it
   on the start of step 2: check that login works (`POST /api/v2/login`)
   within a minute and, if not, read the `WARNING: identity-platform
   installation ...` line of the log. On 4.2.0-rc.1, or when the recovery
   did not apply, move it by hand with the token. Login works again a few
   seconds later (3 s in the rehearsal):

   ```bash
   TOKEN=...            # from step 1
   VERSION=4.2.0        # the release you install, as in the image's bootstrap package
   curl -fsS -X PUT "$PANEL/api/v3/plugin-installations" \
     -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d "{\"plugin_id\":\"identity-platform\",\"target\":\"control\",\"desired_version\":\"${VERSION}\",\"enabled\":true}"
   ```

4. **Import the other packages** from the verified archive, each one
   (nothing but `identity-platform` is moved for you; the commercial
   packages have [no new-root build](#commercial-packages-have-no-new-root-build))
   ([Getting A Package From The Release](#getting-a-package-from-the-release)):
   per package `POST /api/v3/plugin-releases` (`manifest`, `signature`),
   `POST /api/v3/plugin-releases/<id>/artifact` (`artifact_base64`) and
   `PUT /api/v3/plugin-installations` as above with `"desired_version"` set
   to the release version, exactly the calls of
   [Package Install Window](#package-install-window), step 3. Fourteen
   packages took 30 seconds. Control > Plugins > Import release does the
   same once you can sign in.
5. Check `GET /api/v3/plugin-installations`: every package at the 4.2
   version, `healthy`, then the smoke checks of
   [Post-Upgrade Record](#post-upgrade-record). Rehearsed with the calls
   scripted: 96 seconds from the old Control's shutdown to the last
   installation healthy (login back after 65, the import itself 34); the
   operator's pause between the restart and the first call was 53 of them.

Without a token (never taken, or expired) on a build without the automatic
recovery, the way out is the rollback below or restoring the database backup.

### Rolling Back After The Import

- **Before the import** (still in the window): redeploy the previous image
  and configuration. Rehearsed: 33 seconds from `docker compose down` to
  every route answering; the old root is active again, the installations
  still point at the 4.1.0 releases and are healthy.
- **After the import** the installations point at the 4.2 releases, which
  the old root does not verify. The automatic recovery does not run in this
  direction: 4.1.0 has no such change, and a bootstrap package never moves an
  installation to an older release in any case. After the redeploy login and every route
  answer `503 package_unavailable` until `forward` and `identity-platform` are
  moved back, the other routes `404 package_route_not_found` until their own
  package is. Moving them needs a session token from before the upgrade (24
  hours). Order matters:
  1. `forward` first. The 4.2 forward manifest declares
     `kernel.forward.v1`, which 4.1 does not know, so any other
     `PUT /api/v3/plugin-installations` is refused with `409 release_invalid`
     (`enabled plugin forward is invalid: ... unknown kernel capability
     "kernel.forward.v1"`) until `forward` is back on 4.1.0;
  2. then `identity-platform` (login returns);
  3. then the other installations, `"desired_version": "4.1.0"`.

  Rehearsed: 2 minutes 11 seconds with the calls made by hand.

  `config/scripts/rollback_installations.py` makes the same calls in that
  order, with the same token (`ANIX_CONTROL_TOKEN`; the redeployed 4.1.0 has
  no command of its own for this). It lists the installations, prints the
  plan, and changes nothing until `--apply`; it keeps each installation's
  `target` and `enabled`, stops at the first refusal (the `409` above means
  `forward` did not move), is safe to run again, and waits until every moved
  installation is on the version and healthy:

  ```bash
  export ANIX_CONTROL_TOKEN=...   # the session token from before the upgrade
  python3 config/scripts/rollback_installations.py --panel "$PANEL" --version 4.1.0          # dry run
  python3 config/scripts/rollback_installations.py --panel "$PANEL" --version 4.1.0 --apply
  ```
- **No token:** restore the pre-upgrade database backup (rehearsed:
  `pg_restore` of the Compose backup into a fresh database, 30 seconds, then
  4.1.0 started on it, login and all 15 packages healthy after 38 seconds)
  and lose what changed since.
- The 4.2 start adds 23 tables, 2 views and 1 column to the database (the
  `v4_forward_legacy_*`, `v4_kernel_agent_upgrade_*`, `v4_kernel_forward_*`
  and `v4_kernel_package_report_state` tables, plus `v4_kernel_admin_api_token`,
  `v4_kernel_alert` and `v4_kernel_user_activity`; the views
  `kapi_package_report_v1` and `kapi_plugin_configuration_v1`; and the column
  `reported_error_code` of `v4_kernel_node_config_status`, `NOT NULL DEFAULT
  ''`) and drops no table; the row counts of the existing tables were
  unchanged. 4.1.0 ignores the new ones, so a rollback needs no schema step.

## Upgrading From 4.2.0-rc.2 To 4.2.0-rc.3

rc.3 changes no table and no signing root: nothing under any `migrations`
directory differs from rc.2, and the root is the same
`jW26nr2tbthASoeq6RmIpx8Ah+uhPNIv9V1ewRVb1VE=`. The Agent and Control share a
version number and a Control hands out only its own Agent release, so from
rc.2 you upgrade Control first and the Agents after it
([Staged Agent Upgrades](#staged-agent-upgrades-v42)); rc.2 Agents keep
working meanwhile. Coming from 4.1.0, do everything in
["The Official Signing Root Changes (v4.2)"](#the-official-signing-root-changes-v42)
with the rc.3 builds in place of rc.2; the notes below are on top of it.

### rc.3 Checklist (4.2)

1. Back up the database and the configuration, as always.
2. Deploy the rc.3 Control. `identity-platform` keeps running its rc.2 release:
   the bootstrap moves an installation only when its release can no longer be
   verified ([Identity-Platform Recovers By Itself](#identity-platform-recovers-by-itself)),
   and under the same root it can.
3. Upgrade the Agents to rc.3 with a campaign (`anix-control agent upgrade
   start`, the rc.3 release in `agent_install.artifact_dir`), or re-run the
   install command of a node: both install Control's own Agent release, so
   they need the rc.3 Control from step 2
   ([Staged Agent Upgrades (v4.2)](#staged-agent-upgrades-v42)).
4. **Move `identity-platform` to rc.3** with an administrator session token
   (the call of step 3 in [Upgrade Procedure](#upgrade-procedure), with
   `VERSION=4.2.0-rc.3`), then check `GET /api/v3/plugin-installations` for
   `desired_version` `4.2.0-rc.3` and `healthy`. The package restarts, so
   login pauses for a few seconds (3 s in the rc.2 rehearsal; rc.3 was not
   rehearsed again). A deployment that runs `identity-platform` as a module
   container ([`config/deploy/compose/modules.md`](../config/deploy/compose/modules.md))
   also points `ANIX_MODULE_IDENTITY_IMAGE` at the module image of the new
   version.
5. The other packages did not change since rc.2 (only `identity-platform` has
   different files); an installation on its rc.2 release keeps working. Import
   the rc.3 builds from the verified archive when you want the installations on
   the release version
   ([Getting A Package From The Release](#getting-a-package-from-the-release)).
6. If a reverse proxy or CDN caches `index.html`, purge it and reload open tabs:
   the web app's assets changed (Vue 3.5.43, the API-token page).
7. Check the new behaviour below with the rc.3 smoke checks.

### What Changes For Operators

- **Identity (needs step 4).** Login, registration and the subscription-link
  reset count an attempt before the guarded check, so parallel guesses at one
  account no longer all get tried; the second factor and the password re-check
  of the MFA disable endpoint are limited per account, whichever address they
  come from, with the admin MFA settings `max_attempts` and `lockout_duration`
  (defaults 5 and 15 minutes; they were stored but never applied before, so an
  installation that set them sees them take effect now); an unknown e-mail
  costs the same bcrypt work as a wrong password; the unauthenticated login and
  registration answers no longer carry infrastructure error text (they read
  `服务暂时不可用，请稍后重试`; refusals meant for the caller are unchanged). The
  limits use the existing throttle table, so there is no migration; counts are
  per key and the first attempt of a key now counts exactly once.
- **`POST /user/mfa/totp/setup` is refused while MFA is enabled**
  (`MFA already enabled; disable it first`; disabling asks for the password). This
  is also in the Control binary, so it takes effect on restart, before and after
  the cutover. A setup that was never enabled can still be replaced. The web app
  only offers it while the second factor is off; a script that re-runs the setup
  on an enabled account must disable it first.
- **gRPC refuses the kernel's HS256 tokens once identity's cutover is
  finalized.** Finalizing already refused them on the HTTP APIs and the admin
  monitor WebSocket; gRPC still read `jwt.secret`. A gRPC caller that presented a
  session token minted before the cutover now gets `invalid or expired JWT
  token`; EdDSA tokens issued by identity and the configured `grpc.api_token`
  (a separate credential) keep working, and Agent services use their own
  mTLS/node credentials. The same gap existed in 4.1.0. Check
  `legacy_tokens_refused` in `GET /api/v4/kernel/identity`. Before finalize
  nothing changes; after it there is no switch back, so a caller that must keep
  working needs an identity token or the `grpc.api_token`.
- **Credential revocation covers a certificate being issued at that moment.**
  An Agent certificate, enrollment or forward link certificate (up to 7 days),
  and a module certificate (24 hours by default), requested while the node or the
  enrollment was being revoked can no longer be recorded after the revocation on
  PostgreSQL. No action needed.
- **A stale sign-in has its own code** when an administrator creates an API
  token with the identity module holding the credentials: `403
  step_up_sign_in_stale` (message unchanged) instead of `step_up_required`.
  Clients that match `step_up_required` for that case must match both. The rc.2
  web app does not know the new code and shows a generic error; this release's
  web app handles both. A sign-in at most ten minutes old is still required.
- **API token names are counted in characters** (limit 100) on the server, as the
  form did, and the token list carries `owner_email`.
- **Plugin operations stuck behind a one-off operation** (the rc.2 known issue,
  `revision N is not newer than M`) are renumbered above the node's cursor and
  sent when their node is connected and before their `deadline_at`; operations
  stuck on rc.2 therefore recover by themselves. Revision numbers of pending
  operations can change after creation. No schema change.
- **An Agent's configuration status is kept** when its control session ends right
  after the Agent reports it, and a Hello that reports the desired revision gets
  the snapshot again if the kernel never recorded it as applied (the Agent only
  reports it again). Expect one repeated snapshot per such Agent after the
  upgrade.
- **Weak shared secrets** log a startup `WARNING` (see the top of this runbook).
  The development `docker-compose.yml` secret is shorter than 32 bytes and
  warns; the installers' generated secrets do not.
- **Source builds need Go 1.26** (the root module, the `sdk` and `identity`
  modules and the Control Center module). Release images use
  `golang:1.26.9-alpine` from v4.2.0-rc.4.
  The Agent (`anix-agent`) builds with Go 1.26.9 and `x/net` v0.60.0 from
  v4.2.0-rc.4. Four reachable advisories in its hysteria and quic-go dependencies
  are not fixed in that release; the Agent's CHANGELOG lists them.

### For The Release Owner

- **Commercial packages.** After the rc.3 tag exists, run the manual
  `Commercial Packages` workflow with `tag=v4.2.0-rc.3` and `attach=true`; it
  builds `order`, `payment` and `affiliate` at that version, signs them with the
  official key, verifies them against the configured root and adds them with
  `SHA256SUMS-commercial.txt` to the release (a second `attach` run on the same
  tag fails, as the upload does not overwrite). rc.2 builds exist too: they were
  attached to the v4.2.0-rc.2 release the same way. A commercial installation
  imports them as in [Upgrade Procedure](#upgrade-procedure) before the first
  start of a Control that runs them.
- **npm audit waivers expire:** braces on 2026-11-02 and sprintf-js on
  2026-11-05 (`web/audit-allowlist.json`); the frontend audit fails after that
  unless they are renewed or fixed.
- **Nightly security scan** (`Nightly Security`) runs `govulncheck`, the `gosec`
  gates, the frontend audit and the relay fuzz targets every night
  ([`RELEASING.md`](RELEASING.md)).

### Rolling Back From rc.3

Redeploy the previous image or binary as in [Rollback](#rollback). The identity
installation moved in step 4 can stay on rc.3 or go back with the same
`PUT /api/v3/plugin-installations` call; the rc.3 package and the rc.2 package
run on the same tables. Going back to 4.1.0 is
[Rolling Back After The Import](#rolling-back-after-the-import) and
`config/scripts/rollback_installations.py`. The rc.3 package adds no column to the throttle table, so rows written by
either package are read by the other.

### Known Limits

- Creating an administrator API token after the identity cutover is finalized
  still needs a sign-in at most ten minutes old (no identity-module step-up call
  yet).
- The identity throttle table is shared by the login, registration and MFA
  limiters, and each attempt deletes idle rows of the other limiters older than
  its own window; with the defaults a registration counter that stayed idle for
  about 20 minutes can be forgotten earlier than its one-hour window.
- A TOTP code can be used twice inside its roughly 90-second window, and
  regenerating backup codes asks only for the session; neither changed in rc.3.

## Switching Route Modes

Each v2 route of a Control package runs in one of three modes: `legacy` (the
kernel's legacy handler answers; the default for a route without a stored
mode, except the rehearsed default set, see "Upgrading To 4.1.0: Packages
Now Default To Native Routes"), `shadow` (GET routes only:
legacy answers and the package's native implementation runs alongside and
counts mismatches) or `native` (the package answers). The modes are stored in
the package configuration; package hosts apply a change at their next
configuration poll, about 5 seconds later. Switch them with the CLI, the
admin API or the admin page 插件中心 → 路由模式; editing the `routes` key of
the configuration by hand still works but records no route-mode revision.

Only a super administrator may switch modes: an administrator (`is_admin`)
who is not staff and not banned. Every switch writes an audit log entry
(module `kernel`, action `route_mode_set` or `route_mode_rollback`) and the
revision history (`v4_kernel_route_mode_revision`). The CLI records the
actor `system/cli`.

```bash
# What runs where, and which modes each route may take.
anix-control routes list --package knowledge

# Shadow first: GET routes only.
anix-control routes set --package knowledge --mode shadow \
  --reason "batch 1 shadow"

# Watch anixops_package_shadow_mismatches_total on /metrics, then go
# native. Native needs a reason and --yes.
anix-control routes set --package knowledge --mode native \
  --reason "batch 1: 48 h of shadow, no mismatch" --yes

# Or single routes (repeat --route).
anix-control routes set --package knowledge --route knowledge.article.list \
  --mode native --reason "hot path first" --yes

# What changed, by whom and why.
anix-control routes history --package knowledge
```

Add `--json` for machine-readable output. Without `--route`, `set` switches
every route of the package that may take the mode and lists the others as
skipped: `shadow` needs a GET route, `native` a route that
`config/package-extraction.json` marks `native-flagged`; `bridged` routes
stay `legacy`, `kernel-owned` and WebSocket routes do not switch, and
identity group A moves only with the identity cutover and rollback
("Moving Logins To The Identity Module").

### Reading Shadow Mismatches

Before switching a route to `native`, check what its shadow runs disagree
on. The admin page 插件中心 → 路由模式 shows each route's mismatch rate
(mismatches / shadow runs since the package host started) and when the last
mismatch happened; 样本 opens the stored samples of the route with a
structural diff of the two answers. The same data is in the admin API:

```bash
# Rates: host.mismatch_rate, host.last_mismatch_at and mismatch_samples
# (stored, last_observed_at) of each route.
curl -H "Authorization: Bearer $TOKEN" \
  "https://panel.example.com/api/v4/kernel/route-modes?package_id=knowledge"

# Samples, newest first (limit defaults to 50, at most 100).
curl -H "Authorization: Bearer $TOKEN" \
  "https://panel.example.com/api/v4/kernel/route-modes/mismatches?package_id=knowledge&route_id=knowledge.article.list&limit=20"
```

Any administrator may read samples. Each one has the route, method, path
template (query values masked), the legacy and native status codes, the
request id (to find the request in the logs), and a diff of the JSON paths
that differ, with both sides masked and at most 64 paths or 8 KB. Request
bodies are never stored. `anixops_package_shadow_mismatches_total` on
`/metrics` still counts every mismatch; samples show what differed.

How samples reach the kernel: the package host keeps its latest 32 samples
(for 10 minutes) in its Health details document, and the kernel reads them
at its health poll, every 30 seconds. Hosts built with the 4.0.0 SDK report
no samples; their counters still work. The host masks every value before it
leaves the process, and the kernel masks it again before storing it.

Privacy: these masking rules apply on both sides.

- Fully masked (`***`): every value under a field whose name contains
  token, secret, password, pwd, key, uuid, sign, cookie, authorization or
  auth, hash, salt, credential, private, session, nonce, otp or a
  subscription URL name (case-insensitive), whatever its type; and anywhere,
  whatever the field name, strings that look like a JWT, a UUID, a password
  hash, a PEM private key or a long hex or base64 secret, and `Bearer` or
  `Basic` credentials.
- URLs keep only their scheme and host (`https://sub.example.com/***`);
  share links and URLs with user information keep only the scheme
  (`vless://***`).
- E-mail addresses keep the first character and the domain
  (`a***@example.com`).
- IPv4 addresses keep the first two octets (`10.2.*.*`), IPv6 addresses
  the first two hextets (`2001:db8:*:*:*:*:*:*`).

Retention: samples are kept in `v4_kernel_shadow_mismatch_sample` for 7
days and at most 100 per route (the oldest go first); the hourly retention
job deletes the rest. The kernel stores samples only for routes that
`config/package-extraction.json` assigns to the reporting package.

### Route Mode Rollback

One command returns a whole package to `legacy`, in one configuration
revision, with no confirmation (the reason is optional):

```bash
anix-control routes rollback --package knowledge --reason "mismatch in orders"
```

The admin API equivalent is `POST /api/v4/kernel/route-modes/rollback` with
`{"package_id":"knowledge","reason":"..."}`. Identity group A is left as it
is; roll it back with the identity rollback.

The rollback works on the effective modes: routes that run natively only by
default (`source` `default`) get an explicit stored `legacy`, with a revision
row each, so the rollback holds whatever `package_routes.default_mode`
becomes. The same holds for `routes set --mode legacy`.

## Rollback

Rollback should restore the exact previous artifact and config set.

For systemd binary deployments:

```bash
sudo systemctl stop anix-control.service
sudo cp -a /opt/anixops/control/backups/anix-control.YYYYMMDDHHMMSS /opt/anixops/control/bin/anix-control
sudo rm -rf /opt/anixops/control/web/public
sudo cp -a /opt/anixops/control/backups/public.YYYYMMDDHHMMSS /opt/anixops/control/web/public
sudo cp -a /opt/anixops/control/backups/config.YYYYMMDDHHMMSS.yaml /opt/anixops/control/config/config.yaml
sudo systemctl start anix-control.service
curl -fsS http://127.0.0.1:8080/health
```

For Docker Compose deployments:

```bash
docker compose -f docker-compose.prod.yml down
# restore the previously approved image tag, env file, and mounted config
docker compose -f docker-compose.prod.yml up -d
curl -fsS http://127.0.0.1:8080/health
```

If the upgrade changed database state, restore the database backup only when the
approved rollback plan says so. After `anix-control forward legacy drop`
(v4.2), a rollback to 4.1 always needs the database backup taken before the
drop: the old forwarding tables are gone. Keep the failed-upgrade logs and the final
database driver/config in the maintenance record.

## Post-Upgrade Record

After a successful upgrade, record:

- target tag and commit
- artifact checksum verification output
- service restart time
- `/health` output
- core admin/user/node smoke results
- database migration evidence, if any
- rollback artifacts retained and retention period

Do not delete backups until the rollback window has expired.

## Forward Entry HA Through DNS (v4.2)

v4.2 can keep a forwarding route's `entry_hostname` on its healthy entry
nodes through a DNS provider (L2; design:
[`architecture/forward-sdk.md`](architecture/forward-sdk.md) section 7.4;
setup: [`guide/forward-entry-ha.md`](guide/forward-entry-ha.md)).

- **New tables.** `v4_kernel_forward_dns_provider`,
  `v4_kernel_forward_dns_binding` and `v4_kernel_forward_dns_node` are
  created at startup (protected kernel tables; no existing table changes).
  Nothing happens until a route is bound. Rolling back to a release without
  them leaves them unused, and the DNS records stay as last published.
- **`module_runtime.ca_kek` seals the provider credentials.** Without it a
  provider cannot be added. Keep the key with the database backup: a
  restored database with another key cannot open the credentials (the
  bindings then report `error` until the provider is updated with new
  credentials).
- **Routes that relied on hand-made DNS** keep working unbound. Binding one
  in DDNS mode makes Control the owner of the name's A/AAAA records in that
  zone: it replaces records it did not write.
