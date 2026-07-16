# Upgrade Runbook

This runbook covers operator upgrades for `v2board_AnixOps` releases. Release
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

Download artifacts from the GitHub Release for the target tag. At minimum,
download:

- the matching `v2board-*` backend artifact for the host OS/architecture
- `v2board-frontend.tar.gz` or `v2board-frontend.zip`
- `SHA256SUMS.txt`
- `RELEASE_MANIFEST.json`
- `OPERATOR_DEPLOYMENT.md`
- `RELEASE_NOTES.md`
- `migration-dry-run.txt`
- `v2board-source.sbom.spdx.json`

Plugin-platform releases may also attach signed official package artifacts.
For the `machine-telemetry` reference package, keep these files together:

- `machine-telemetry-*.tar`
- `machine-telemetry-manifest.json`
- `machine-telemetry-manifest.sig`
- `machine-telemetry-public.pem`
- `machine-telemetry-public.raw`
- `machine-telemetry-SHA256SUMS.txt`

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

For signed plugin packages, first verify package checksums:

```bash
sha256sum -c machine-telemetry-SHA256SUMS.txt
```

Then verify the manifest signature with the public key published by the same
GitHub Release, or with the pinned AnixOps trust root already approved in your
environment:

```bash
python3 packages/machine-telemetry/verify_signature.py \
  --manifest machine-telemetry-manifest.json \
  --artifact machine-telemetry-1.0.0.tar \
  --signature machine-telemetry-manifest.sig \
  --public-key machine-telemetry-public.pem
```

If the package hash, manifest signature, or trust-root fingerprint does not
match the release record, stop before enabling any plugin flag.

## Plugin Platform Flags

The package platform remains opt-in during the 3.x migration. These defaults
must stay false in production until the release notes explicitly approve the
phase:

```yaml
plugins:
  control_execution_enabled: false
  dispatch_enabled: false
  topology_execution_enabled: false
```

Enablement order matters. `topology_execution_enabled=true` is refused unless
`dispatch_enabled=true`, and neither flag should be enabled before package
artifacts, signatures, database migration evidence, and canary rollback
commands are recorded. Enabling topology execution does not by itself approve
business traffic migration; dedicated forwarding still requires the real
`nftables-forward` Agent runtime, network-namespace TCP/UDP evidence, and a
recorded rollout plan.

## Systemd Binary Upgrade

The exact service name and paths are operator-owned. The example below assumes:

- service: `v2board.service`
- binary path: `/opt/v2board/v2board`
- frontend path: `/opt/v2board/public`
- config path: `/opt/v2board/config/config.yaml`

Prepare backups:

```bash
sudo install -d -m 0750 /opt/v2board/backups
sudo cp -a /opt/v2board/v2board /opt/v2board/backups/v2board.$(date +%Y%m%d%H%M%S)
sudo cp -a /opt/v2board/public /opt/v2board/backups/public.$(date +%Y%m%d%H%M%S)
sudo cp -a /opt/v2board/config/config.yaml /opt/v2board/backups/config.$(date +%Y%m%d%H%M%S).yaml
```

Stop, replace, and start:

```bash
tar -xzf ./v2board-linux-amd64.tar.gz

sudo systemctl stop v2board.service

sudo install -m 0755 ./v2board-linux-amd64 /opt/v2board/v2board
sudo rm -rf /opt/v2board/public.new
sudo mkdir -p /opt/v2board/public.new
sudo tar -xzf ./v2board-frontend.tar.gz -C /opt/v2board/public.new
sudo rm -rf /opt/v2board/public
sudo mv /opt/v2board/public.new /opt/v2board/public

sudo systemctl start v2board.service
sudo systemctl status v2board.service --no-pager
```

Verify:

```bash
curl -fsS http://127.0.0.1:8080/health
sudo journalctl -u v2board.service -n 120 --no-pager
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

Docker deployments must still use GitHub Release evidence. Do not build release
images or frontend assets from the production checkout.

Before upgrade:

```bash
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs --tail=120
```

Verify the release's Docker metadata and digest in `docker-image.txt` if the
release includes one. Pull the approved image or deploy the approved binary and
frontend artifacts into the image strategy used by the operator. Then restart:

```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps
```

Verify:

```bash
curl -fsS http://127.0.0.1:8080/health
docker compose -f docker-compose.prod.yml logs --tail=120
```

If the deployment still requires a local Docker build, it is not a release
deployment under the current policy. Record it as a manual exception before
proceeding.

## Database And Migration Notes

For ordinary patch upgrades, keep the existing database driver and config.

If a release requires schema/data changes:

- review release notes before the maintenance window
- keep `migration-dry-run.txt` from the GitHub Release
- record pre/post row counts for affected tables
- keep rollback instructions for each changed schema helper
- do not combine SQLite-to-PostgreSQL migration with an unrelated feature
  upgrade unless the maintenance plan explicitly approves it

For SQLite-to-PostgreSQL migration, use
[`reference/sqlite-to-postgres-migration.md`](reference/sqlite-to-postgres-migration.md).

## Rollback

Rollback should restore the exact previous artifact and config set.

For systemd binary deployments:

```bash
sudo systemctl stop v2board.service
sudo cp -a /opt/v2board/backups/v2board.YYYYMMDDHHMMSS /opt/v2board/v2board
sudo rm -rf /opt/v2board/public
sudo cp -a /opt/v2board/backups/public.YYYYMMDDHHMMSS /opt/v2board/public
sudo cp -a /opt/v2board/backups/config.YYYYMMDDHHMMSS.yaml /opt/v2board/config/config.yaml
sudo systemctl start v2board.service
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
