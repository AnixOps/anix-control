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
   reproducible; the first branded preview is `v3.0.0-alpha.1`.
4. Back up any existing panel before running `update` or `rollback`.

## Fresh Install

Download the installer from the exact release tag. This fetches one script, not
the repository checkout:

```bash
export VERSION=v3.1.0-alpha.1
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
4. Download the configuration template that matches the selected tag.
5. Generate the JWT secret, node API token, and first administrator password.
6. Install and enable `anix-control.service`.
7. Start the service and require `http://127.0.0.1:8080/health` to succeed.

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

## Upgrade And Rollback

The installer never overwrites an existing `config/config.yaml` or SQLite data
directory. Before replacing binaries, it copies the current binary and frontend
files to `/opt/anixops/control/backups/`. A failed health check automatically restores
that snapshot.

Upgrade to an explicit release:

```bash
export TARGET=v3.1.0-alpha.1
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
- Do not use the legacy `install.sh` source/Docker path unless
  `ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1` is intentionally set for a controlled
  recovery. It is not the stable release path.
- Release artifacts are built and tested by GitHub Actions. Do not compile a
  replacement binary or frontend bundle on the panel host.

Existing `/opt/v2board` installations should use the in-place command and
compatibility notes in [`../BRAND_MIGRATION.md`](../BRAND_MIGRATION.md).
