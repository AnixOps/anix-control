# AnixOps Control Release Installation

This guide installs the panel from GitHub Release assets. The target host
downloads a single installer script, the matching binary archive, the frontend
archive, and `SHA256SUMS.txt`. It does not clone the repository and it does not
run `go build`, `npm`, or Docker image builds on the server.

The supported native-install targets are Linux `amd64` and Linux `arm64` hosts
with systemd. For Docker or PostgreSQL deployments, use the existing
[`../DEPLOYMENT.md`](../DEPLOYMENT.md) and [`../UPGRADE.md`](../UPGRADE.md)
runbooks instead of mixing deployment models on the same host.

## Before You Start

1. Use a fresh Debian 12/Ubuntu 22.04+ host where ports `8080`, `3000`, and,
   when needed, `50051` are available.
2. Put the panel behind Nginx or Caddy for public TLS. Do not expose the admin
   API directly to the Internet without a reverse proxy and firewall policy.
   The fresh development template binds gRPC to `127.0.0.1:50051`; remote
   Agents require an explicit TLS/proxy setup and a deliberate bind-address change.
3. Decide the exact version to install. Pinning a tag makes the operation
   reproducible. A `v4.0.0` package-only release is installable only when its
   release assets include a verified `v4-release-evidence.tar.gz`; historical
   `v4.0.0-alpha.*` tags are previews, not substitutes for that evidence.
4. Back up any existing panel before running `update` or `rollback`.

## Fresh Install

Download the installer from the exact release tag. This fetches one script, not
the repository checkout:

```bash
export VERSION=v4.0.0
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${VERSION}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh install \
  --version "${VERSION}" \
  --admin-email "admin@example.com"
rm -f /tmp/anix-control-install.sh
```

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
7. Generate the JWT secret, node API token, and first administrator password.
8. Install and enable `anix-control.service`.
9. Start the service and require `http://127.0.0.1:8080/health` to succeed;
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

## Installed Layout And Operations

| Path | Purpose |
|---|---|
| `/opt/anixops/control/bin/anix-control` | GitHub Actions-built backend binary |
| `/opt/anixops/control/bin/v2board` | Compatibility symlink to the primary binary |
| `/opt/anixops/control/web/public` | GitHub Actions-built frontend files |
| `/opt/anixops/control/config/config.yaml` | Persistent control-plane configuration |
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

For a remote Agent, terminate TLS either in Control or in an HTTP/2-capable
gRPC proxy before changing the listener bind. Configure the Agent with
`Transport: "http"` to preserve the legacy data-plane fallback, and explicitly
enable both `AgentControlEnabled` and `PluginSupervisorEnabled` with the same
official public key.

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
export TARGET=v4.0.0
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${TARGET}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh update --version "${TARGET}"
rm -f /tmp/anix-control-install.sh
```

To roll back the application files, run the same version-pinned installer with
the previous known-good tag. Configuration and database rollback are separate
operator decisions; do not restore a database merely because an application
binary was rolled back.

```bash
export PREVIOUS=v2.5.0
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
- For v4, retain the verified `v4-release-evidence.tar.gz` with the deployment
  record. Do not install a partial sixteen-package set or bypass an unhealthy
  package with a direct legacy route.
- Do not use the legacy `install.sh` source/Docker path unless
  `ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1` is intentionally set for a controlled
  recovery. It is not the stable release path.
- Release artifacts are built and tested by GitHub Actions. Do not compile a
  replacement binary or frontend bundle on the panel host.

Existing `/opt/v2board` installations should use the in-place command and
compatibility notes in [`../BRAND_MIGRATION.md`](../BRAND_MIGRATION.md).
