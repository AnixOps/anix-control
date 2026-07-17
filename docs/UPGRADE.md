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

Download artifacts from the GitHub Release for the target tag. At minimum,
download:

- the matching `anix-control-<os>-<arch>.tar.gz` backend artifact (or the
  matching Windows `.exe.zip`)
- `anix-control-frontend.tar.gz` or `anix-control-frontend.zip`
- `SHA256SUMS.txt`
- `RELEASE_MANIFEST.json`
- `OPERATOR_DEPLOYMENT.md`
- `RELEASE_NOTES.md`
- `verify-machine-telemetry-signature.py`
- `migration-dry-run.txt`
- `anix-control-source.sbom.spdx.json`

Plugin-platform releases may also attach signed official package artifacts. The
asset set is defined by the target product stage, not by every package source
present in the repository. Formal 3.1 releases attach only
`machine-telemetry`; `nftables-forward`, `gost-mesh`, and `nat-egress` begin
at their later stages. Historical `v4.0.0-alpha.*` assets retain their original
package sets only as frozen preview evidence. For each attached package ID,
keep these files together:

- `<plugin-id>-<plugin-version>.tar`
- `anixops-<plugin-id>-<plugin-version>.manifest.json`
- `anixops-<plugin-id>-<plugin-version>.sig`
- `anixops-<plugin-id>-<plugin-version>.public-key.pem`
- `anixops-<plugin-id>-<plugin-version>.public-key.raw`
- `anixops-<plugin-id>-<plugin-version>.SHA256SUMS.txt`

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
sha256sum -c anixops-machine-telemetry-1.1.0.SHA256SUMS.txt
```

Then verify the manifest signature with the public key published by the same
GitHub Release, or with the pinned AnixOps trust root already approved in your
environment:

```bash
python3 verify-machine-telemetry-signature.py \
  --manifest anixops-machine-telemetry-1.1.0.manifest.json \
  --artifact machine-telemetry-1.1.0.tar \
  --signature anixops-machine-telemetry-1.1.0.sig \
  --public-key anixops-machine-telemetry-1.1.0.public-key.pem
```

If the package hash, manifest signature, or trust-root fingerprint does not
match the release record, stop before enabling any plugin flag.

## Plugin Platform Flags

`v4.0.0-alpha.7` is a historical signed-package/WebUI canary artifact, not a
formal 4.0 release. This section applies only when operating that exact frozen
artifact. The formal product line resumes at `v3.1.0-alpha.2` and must pass the
3.1 through 3.5 gates before a plugin-only 4.0 cutover. See
[`architecture/release-line-status.md`](architecture/release-line-status.md).

The historical artifact adds the signed `nftables-forward` 1.2.0 runtime
observation contract: live ruleset SHA-256 and per-rule counters are persisted
from Agent heartbeats and are required by topology promotion for that package.
Start the 72-hour observation window again after installing `alpha.7`. A fresh
alpha configuration enables the Control package executor and Agent dispatch,
while topology execution remains disabled. An upgrade preserves the existing
configuration, so an existing installation is not silently switched to the new
path. Enable the alpha flags only after importing the official release assets,
recording checksums/signatures, and selecting a canary node:

```yaml
plugins:
  control_execution_enabled: true
  dispatch_enabled: true
  topology_execution_enabled: false
```

Enablement order matters. `topology_execution_enabled=true` is refused unless
`dispatch_enabled=true`. Package artifacts, signatures, database migration
evidence, canary rollback commands, and a 72-hour observation record are still
required before any stable rollout. Enabling topology execution does not by
itself approve business traffic migration; dedicated forwarding still requires
the real `nftables-forward` Agent runtime, network-namespace TCP/UDP evidence,
and a recorded rollout plan. Stable 4.0 publication additionally requires
explicit operator authorization.

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
