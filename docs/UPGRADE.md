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
- `anix-control-frontend.tar.gz` or `anix-control-frontend.zip`
- `SHA256SUMS.txt`
- `RELEASE_MANIFEST.json`
- `RELEASE_NOTES.md`

Every official package is attached at the release version. For each package
ID, keep these files from the same release together:

- `<plugin-id>-<version>.anxp`
- `<plugin-id>-<version>.manifest.json`
- `<plugin-id>-<version>.manifest.sig`
- `<plugin-id>-<version>.public-key.pem`
- `<plugin-id>-<version>.sbom.spdx.json`

Do not import a mixed-version package set. The `v4.0.0` release also attaches
`v4-release-evidence.tar.gz`; the v4 plugin-only upgrade guide covers it.

Verify checksums before replacing any production file:

```bash
sha256sum -c SHA256SUMS.txt
```

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
   packages: `.anxp`, `.manifest.json` and `.manifest.sig` for each.
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

Rollback: set the previous digest in `.env` and run `up -d` again. Restore the
database backup only when the approved rollback plan says so.

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
Rotate the tokens of the nodes they used, as the previous section says.

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

Disabled nodes:

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
  node needs the token typed in. If a token is lost, set a new one from the
  node's edit form and update the relay and its agent.
- **Proxy node credentials.** `GET /api/v2/admin/nodes/:id/credentials`
  still answers a node's API key and secret, for the deployment helper and
  Ansible. Each read is now recorded in `v2_audit_log` with action `reveal`.

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
  described above.
- **Logs.** Each run that finds a payment logs a summary starting with
  `Order payment reconciler:`, and a line per completed order and per
  failure. Payments paid more than 90 days ago are left as they are.

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

**Forward nodes without an API port.** Once `backfill` has run, a forward
node's token is pinned to its endpoint, `host:api_port` (decided by the
owner, 2026-10-01; see "Forward Node Tokens Are Pinned To Their Endpoint"
below). A forward node that has a token but no `api_port` has no endpoint,
so its token is presented nowhere: gost backend changes and legacy rules on
it fail with `ENDPOINT_UNCONFIRMED` until an administrator sets the API
port, which pins it. Before the backfill nothing changes: such a node has
no credential row and is used as before. List those nodes first and set
their ports, then backfill:

```bash
anix-control node-secrets status     # "forward_nodes_without_api_port": count, and each node's id and name
```

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

### Forward Runtime Job Payloads No Longer Hold Node Tokens

Until this release a forward change on a job backend (local Ansible, clean
agent) and every gost change recorded the ingress node's API token inside
the `v2_forward_runtime_job` payload (`panelForward.ingressNode.apiToken`),
so the job table, the administrator's job list
(`GET /api/v2/admin/forward/runtime/jobs`) and a clean agent's claim all
carried it (`docs/architecture/node-ops-service.md`, sections 3.7 and
3.11).

- **New rows carry no token.** The kernel resolves the token when it sends
  a request to NodeX, and only for the node's pinned endpoint (next
  section). Ansible and clean agent jobs never needed it: the agent
  authenticates with its own token.
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
- **A node without an API port** has no endpoint and its token is presented
  nowhere (decided by the owner, 2026-10-01): set the API port before using
  the node with the gost backend or a legacy rule.
  `anix-control node-secrets status` lists such nodes under
  `forward_nodes_without_api_port`, by id and name, with their count.
- **Run `node-secrets backfill`** after the upgrade if you have not: a node
  without a credential row has no pin yet, is presented as before, and is
  counted in `anixops_node_secrets_pin_total{reason="unpinned"}`. An
  unconfirmed address is counted with `reason="unconfirmed"` and logged
  once per node, without the address or the value.

### Agent Client Certificates Are Optional

Control can issue mTLS client certificates to AnixOps Agents
(`docs/architecture/module-runtime.md`, "Agent PKI"). Nothing is required of
operators: the new key `agent_control.mtls` defaults to `optional`, and
every agent keeps authenticating with its node API key as before.

- **What is new.** The tables `v4_kernel_agent_enrollment` and
  `v4_kernel_agent_certificate` (created at startup), the gRPC service
  `anix.agent.v1.AgentEnrollment` on the agent listener,
  `POST /api/v4/kernel/agents/enrollment-tokens` and
  `anix-control agent token create -node proxy-12`. KernelNodeOps also
  creates `v4_kernel_node_desired_config` at startup: each node's desired
  configuration with its revision and hash, written by the administrator's
  node sync (`docs/architecture/node-ops-service.md`, section 5.5), and
  `v4_kernel_node_config_status`: what each node's agent last reported
  about its configuration on the Agent Control stream (`config.v1`). Agents
  that list `config.v1` receive their configuration as snapshots on the
  stream; the first rebuild after the upgrade moves every stored revision
  by one, since the document gained the UniProxy answers. Nothing is
  required of operators.
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
  (planned for 5.0)
  accepts only certificates on the Agent services. Both need the built-in
  CA and gRPC TLS, or Control refuses to start. Do not switch before every
  agent has enrolled.
- **Revocation.** Disabling or deleting a node, or replacing a forward
  node's token, revokes the node's agent certificates. A re-enabled node's
  agent enrolls again with its credential.

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
approved rollback plan says so. Keep the failed-upgrade logs and the final
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
