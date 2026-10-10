# AnixOps Control Release Installation

This guide installs the panel from GitHub Release assets. The target host
downloads a single installer script, the matching binary archive, the frontend
archive, and `SHA256SUMS.txt`. It does not clone the repository and it does not
run `go build`, `npm`, or Docker image builds on the server.

> **Containers first.** Containers are the primary deployment
> ([`../DEPLOYMENT.md`](../DEPLOYMENT.md)): Docker Compose with an external
> PostgreSQL, or the Helm chart. This systemd installer installs the release
> tag you name with `--version` and never resolves "latest". Do not mix both
> models on one host.

The supported native-install targets are Linux `amd64` and Linux `arm64` hosts
with systemd. Upgrades are in [`../UPGRADE.md`](../UPGRADE.md).

## Before You Start

1. Use a fresh Debian 12/Ubuntu 22.04+ host where ports `8080`, `3000`, and,
   when needed, `50051` are available.
2. Put the panel behind Nginx or Caddy for public TLS. Do not expose the admin
   API directly to the Internet without a reverse proxy and firewall policy.
   gRPC stays on `127.0.0.1:50051` until it has a TLS certificate; with one
   the installer opens it for Agents ([Agent access](#agent-access)).
3. Decide the exact version to install. Pinning a tag makes the operation
   reproducible. The `v4.0.0` package-only release is installable only when
   its release assets include a verified `v4-release-evidence.tar.gz`;
   historical `v4.0.0-alpha.*` tags are previews, not substitutes for that
   evidence. Later releases carry no evidence bundle: verify them with
   `SHA256SUMS.txt` and the pinned official root (`docs/UPGRADE.md`).
4. Back up any existing panel before running `update` or `rollback`.

## Fresh Install

Download the installer from the exact release tag. This fetches one script, not
the repository checkout:

```bash
export VERSION=v4.2.0   # the release tag to install
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${VERSION}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh install \
  --version "${VERSION}" \
  --admin-email "admin@example.com" \
  --grpc-name grpc.example.com
rm -f /tmp/anix-control-install.sh
```

`--grpc-name` is the host name Agents dial for gRPC. When
`/etc/letsencrypt/live/<name>/` holds a certificate (or you pass
`--grpc-tls-cert` and `--grpc-tls-key`), the install is ready for Agents at
once; see [Agent access](#agent-access).

The installer will:

1. Create the `anixops` service user and `/opt/anixops/control` layout.
2. Download the GitHub Release binary and frontend packages.
3. Verify both packages against the release `SHA256SUMS.txt`.
4. For `v4.*`, download and checksum-verify the signed identity package trio,
   then stage it under `/opt/anixops/control/bootstrap/` with root ownership.
5. For `v4.*`, place Control host sockets and materialized package artifacts
   under the installation root with service-user-only permissions. The installer
   sets these internal execution paths on both fresh installs and updates so
   they remain within the systemd writable root. The bootstrap directory is set
   only when empty and
   `plugins.control_execution_enabled` is enabled so the identity package can
   serve login.
6. Download the configuration template that matches the selected tag.
7. Set `env: "production"` and generate the JWT secret, node API token, and
   first administrator password ([Environment](#environment-production)).
8. Generate the CA key-encryption key (`module_runtime.ca_kek`) in
   `config/secrets/module_ca_kek` (mode 0600, printed only as a fingerprint)
   and, given a publicly trusted certificate, enable gRPC with TLS
   ([Agent access](#agent-access)).
9. Install and enable `anix-control.service`.
10. Start the service and require `http://127.0.0.1:8080/health` to succeed;
   a fresh V4 install also verifies the identity package gateway and login.

The normal configuration template keeps package execution, Agent dispatch, and
topology execution disabled. A V4 installer is the explicit exception: it
enables Control package execution because login is owned by the verified
identity package, while leaving Agent dispatch and topology execution disabled.
Existing configuration files retain every other value; a non-empty
operator-selected bootstrap directory is not replaced.

The generated initial password is stored at
`/opt/anixops/control/.bootstrap-admin-password` with restricted permissions. Store it
in a password manager, sign in, change the password, then remove that file:

```bash
sudo rm -f /opt/anixops/control/.bootstrap-admin-password
```

To supply an initial password non-interactively, use `--admin-password`. Do
not put secrets in shell history on shared hosts.

### Environment: production

The template is also the local-development file, so it says
`env: "development"`. A host install is not a development install: the
installer writes `env: "production"` into the new `config.yaml` (as the
container defaults are), next to the generated `jwt.secret`.

- In production Control refuses to start when `jwt.secret` is empty or a
  template value, and it never alters existing tables on start: an empty
  database gets the full schema, an existing one only the tables it lacks
  (`docs/reference/startup-config.md`). In `development` it runs a full
  `AutoMigrate` on every start and accepts the template secret.
- Only a **fresh** install writes the file. `update`, `rollback` and
  `enable-agents` never change `env`. A host installed by an earlier release
  has `env: "development"`; check with
  `grep '^env:' /opt/anixops/control/config/config.yaml`. To move it to
  production, confirm `jwt.secret` is a random value of your own, set
  `env: "production"` and restart (`sudo systemctl restart anix-control`).

## Installed Layout And Operations

| Path | Purpose |
|---|---|
| `/opt/anixops/control/bin/anix-control` | GitHub Actions-built backend binary |
| `/opt/anixops/control/bin/v2board` | Compatibility symlink to the primary binary |
| `/opt/anixops/control/web/public` | GitHub Actions-built frontend files |
| `/opt/anixops/control/config/config.yaml` | Persistent control-plane configuration |
| `/opt/anixops/control/config/secrets/module_ca_kek` | CA key-encryption key (service user, 0600); never replaced; back it up with the database |
| `/opt/anixops/control/config/tls/control.crt`, `control.key` | gRPC certificate (0644) and key (0600) for Agents |
| `/opt/anixops/control/config/data/v2board.db` | Default SQLite database; filename retained for compatibility |
| `/opt/anixops/control/runtime/plugin-hosts/` | Service-user-private Control package host sockets and process state |
| `/opt/anixops/control/data/plugin-artifacts/` | Service-user-private materialized Control package artifacts |
| `/opt/anixops/control/backups/` | Binary/frontend rollback snapshots |
| `/opt/anixops/control/.release-version` | Installed release tag |

Useful commands:

```bash
sudo systemctl status anix-control
sudo journalctl -u anix-control -n 200 --no-pager
curl -fsS http://127.0.0.1:8080/health
sudo cat /opt/anixops/control/.release-version
```

Before attaching a node, set the real domain, proxy trust list, CORS origins,
TLS path or reverse proxy, registration policy, and node API key in
`/opt/anixops/control/config/config.yaml`. Restart only after validating the intended
change:

```bash
sudo systemctl restart anix-control
sudo systemctl status anix-control --no-pager
```

## Agent access

AnixOps Agents enroll (AgentEnrollment) and connect over gRPC with TLS, and
Control signs their certificates with its built-in CA. From v4.2
`agent_control.mtls` defaults to `required`, so this is the only way Agents
connect. Two things are needed, and the installer sets up both:

1. **The CA key-encryption key** (`module_runtime.ca_kek`). A fresh install
   writes 32 random bytes (base64) to
   `/opt/anixops/control/config/secrets/module_ca_kek`, owned by the service
   user with mode 0600, and the unit passes it as
   `ANIX_CONTROL_MODULE_RUNTIME_CA_KEK_FILE`. Only its fingerprint is printed
   (`sudo sha256sum /opt/anixops/control/config/secrets/module_ca_kek | cut -c1-16`).
   The installer never replaces it, and leaves it alone when `config.yaml`
   (or a systemd drop-in) sets the key already. Back it up with the database:
   the CA is sealed with it, and with another key Control cannot sign Agent
   certificates (`module CA … key cannot be unsealed: wrong key-encryption
   key`) and every Agent must enroll again. Changing it is a manual
   procedure ([`../UPGRADE.md`](../UPGRADE.md), "Agent Transports").
2. **A gRPC certificate Agents trust.** Agents verify Control's certificate
   against the node's system CA roots, for the host they dial
   (`agent_install.grpc_target`, else `agent_install.public_url`'s host).
   They have no option to trust a private CA, so **a self-signed certificate
   does not work** and the installer never creates one. Use a publicly
   trusted certificate:
   - Let's Encrypt over DNS-01:
     `DOMAIN=grpc.example.com CLOUDFLARE_API_TOKEN=<token> sudo -E bash config/deploy/grpc_tls/setup_certbot.sh`,
     then `--grpc-name grpc.example.com`. Its renewal hook copies renewed
     certificates into `config/tls/` and restarts Control.
   - The certificate your reverse proxy already has for the same name:
     `--grpc-tls-cert <fullchain.pem> --grpc-tls-key <privkey.pem>`.

   The installer checks the certificate the way an Agent will (chain to the
   system roots, the name, the key) and refuses one that fails, before it
   changes anything. It copies it to `config/tls/` (the service user cannot
   read `/etc/letsencrypt`), sets `grpc.enabled: true`, `grpc.host:
   "0.0.0.0"` (when it was loopback), `grpc.tls_cert_file`/`tls_key_file`,
   and with `--grpc-name` `agent_install.grpc_target: "<name>:<grpc.port>"`.
   Open that port from the nodes.

Without a certificate the install still finishes, gRPC stays on loopback
without TLS, and the installer prints these steps; Control's startup log
also says no Agent can connect.

For an existing install, or once you have the certificate:

```bash
sudo bash /tmp/anix-control-install.sh enable-agents --grpc-name grpc.example.com
# or: ... enable-agents --grpc-tls-cert /path/fullchain.pem --grpc-tls-key /path/privkey.pem
sudo /opt/anixops/control/bin/anix-control -config /opt/anixops/control/config/config.yaml \
  agents transports --check-required
```

`enable-agents` downloads nothing: it creates the CA key when none is
configured (keeping an existing one), installs the certificate, rewrites the
unit and restarts Control. Before running it on an install that ever had a
`module_runtime.ca_kek`, read [`../UPGRADE.md`](../UPGRADE.md) ("Adding the
CA key to an existing install"). `update` keeps both the key and the
certificate; pass `--grpc-tls-cert`/`--grpc-tls-key` to it to replace the
certificate.

Then install Agents from the node page ([`agent-onboarding.md`](agent-onboarding.md)).
A legacy Agent configured by hand (only while `agent_control.mtls` is
`preferred` or `optional`) uses `Transport: "http"` to preserve the legacy
data-plane fallback, and enables both `AgentControlEnabled` and
`PluginSupervisorEnabled` with the same official public key.

## Import The Signed Official Packages

The package center is intentionally operator-driven. For `v4.0.0`, verify
`v4-release-evidence.tar.gz` on a management workstation before importing any
package. The verifier and official root below must come from a pre-existing,
independently trusted Control source distribution and operator trust store.
Do not obtain either value from the checkout, configuration, or archive being
verified. This is not a production-host build step; do not unpack the archive
with a general-purpose tar command before the verifier has validated its
members and signature:

```bash
: "${ANIXOPS_TRUSTED_CONTROL_SOURCE:?set to a previously verified immutable Control source tree}"
: "${ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY:?set to the separately managed pinned official root}"
trusted_verifier="${ANIXOPS_TRUSTED_CONTROL_SOURCE}/config/scripts/verify_v4_evidence.py"
[[ -f "${trusted_verifier}" && ! -L "${trusted_verifier}" ]] || exit 1
[[ -f "${ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY}" && ! -L "${ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY}" ]] || exit 1
python3 "${trusted_verifier}" \
  --archive v4-release-evidence.tar.gz \
  --trusted-official-public-key "${ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY}"
```

Pin and independently confirm the official-root fingerprint when that trust
store is provisioned. A candidate tag may be used only after this verification;
it is never the source of the verifier or its trust anchor.

The evidence bundle must name all sixteen signed package artifacts, manifests,
signatures, public keys, SBOMs, route gates, WebSocket relay checks, SQLite and
PostgreSQL rehearsals, and signed canary/support approvals. In the admin UI
open **Control > Plugins > Import release**, paste or upload the manifest and
signature, choose the matching `.anxp` artifact, and submit each release. The
UI verifies the signed manifest and artifact digest before it becomes
installable; do not mix assets from different tags or evidence bundles.

From `v4.1.0-rc.3` on, the package files are not separate release assets: they
are in the signed `anix-control-packages-<version>.tar.gz`. Check its `.sig`
against the pinned official root and extract the `.anxp`, `.manifest.json` and
`.manifest.sig` you import with the commands in
[`../UPGRADE.md`](../UPGRADE.md#getting-a-package-from-the-release), for example:

```bash
tar -xzf "anix-control-packages-${VERSION#v}.tar.gz" --strip-components=1 \
  --wildcards "*/machine-telemetry-${VERSION#v}.*"
```

On a Control that runs packages signed by another root (the 4.2 root change),
login is down until `identity-platform` is moved to the new release, so this
page cannot be reached without a session token taken before the restart:
follow [`../UPGRADE.md`](../UPGRADE.md#the-official-signing-root-changes-v42).

After importing a release:

1. Open **Control > Plugins**, install every Control-target package while
   disabled, and configure it if its manifest exposes a schema.
2. For a package with an Agent target, open **Control > Assignments**, select a node, and create the service role.
   The version and configuration revision default from the enabled Agent
   installation. Keep the role disabled until the node is connected if this is
   a first canary.
3. Enable the role. Control queues `install -> update -> enable` and exposes the
   operation chain in **Control > Operations**. The Agent reports observed
   revision, health, and Unix-socket readiness; wait for `succeeded`/`healthy`
   before adding another node.
4. For a rollback, stop the role, select the previous signed release, and
   verify the operation and observed state before re-enabling it. Disabling a
   package preserves its database records; use a separate purge workflow for
   destructive removal.

After the installer has bootstrapped the official identity package, the
remaining V4 package cohort remains manual import by design. It does not
silently fetch untrusted third-party packages or put API keys in package URLs;
Agent downloads use `X-API-Key` and same-origin, digest-addressed paths. Follow
[v4-plugin-only-upgrade.md](v4-plugin-only-upgrade.md) for the release cohort
and [v4-plugin-only-rollback.md](v4-plugin-only-rollback.md) for fault
recovery.

## Upgrade And Rollback

The installer never overwrites an existing SQLite data directory. For `v4.*`,
it preserves existing configuration except an empty identity bootstrap path,
`plugins.control_execution_enabled`, and the internal plugin runtime paths that
must remain inside the systemd writable installation root. It saves
the prior configuration in the release backup before replacing binaries. A
failed health or identity gateway check restores that snapshot.

Upgrade to an explicit release:

```bash
export TARGET=v4.2.0   # the release tag to upgrade to
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${TARGET}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh update --version "${TARGET}"
rm -f /tmp/anix-control-install.sh
```

To roll back the application files, run the same version-pinned installer with
the previous known-good tag. Configuration and database rollback are separate
operator decisions; do not restore a database merely because an application
binary was rolled back. Going back to a 4.1 release from 4.2 also needs the
package installations moved back, because the official signing root changed
([`../UPGRADE.md`](../UPGRADE.md#rolling-back-after-the-import)).

```bash
export PREVIOUS=v4.1.0   # the previous known-good tag
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${PREVIOUS}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh rollback --version "${PREVIOUS}"
rm -f /tmp/anix-control-install.sh
```

For a database-engine migration, follow
[`../reference/sqlite-to-postgres-migration.md`](../reference/sqlite-to-postgres-migration.md).
For older panel products or a coordinated panel/node cutover, follow
[`legacy-migration.md`](legacy-migration.md).

## Security Notes

- Pin a tag for production. The `go_dev` branch is for integration, not a
  production version selector.
- Review the script before execution and keep release checksums with the change
  record.
- Keep the release's `SHA256SUMS.txt` and the packages archive's `.sig` with
  the deployment record. Only `v4.0.0` has a verified
  `v4-release-evidence.tar.gz`; later releases ship no evidence bundle. Do not
  install a partial package set or bypass an unhealthy package with a direct
  legacy route.
- Do not use the legacy `install.sh` source/Docker path unless
  `ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1` is intentionally set for a controlled
  recovery. It is not the stable release path.
- `config/secrets/module_ca_kek` is a secret: keep it out of tickets and
  shell history, and in the same backups as the database.
- Release artifacts are built and tested by GitHub Actions. Do not compile a
  replacement binary or frontend bundle on the panel host.

Existing `/opt/v2board` installations should use the in-place command and
compatibility notes in [`../BRAND_MIGRATION.md`](../BRAND_MIGRATION.md).
