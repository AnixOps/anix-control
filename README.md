# AnixOps Control

`anix-control` is the control-plane service for AnixOps. It combines a Go
backend, Vue 3 admin/user frontend, node communication APIs, subscription
generation, payment/order management, traffic statistics, and the in-progress
minimal forwarding module.

The project is being hardened toward a stable, testable, GitHub Actions-only
release process. Treat [`docs/features.md`](docs/features.md) as the source of
truth for implemented, partial, planned, compatibility, and deferred features.

## Current Status

- Main branch: `go_dev`.
- Backend: Go module `github.com/AnixOps/anix-control/v5`; the toolchain is
  pinned to Go `1.26.9` in CI.
- Frontend: Vue 3 + Vite under [`web/`](web), Node.js `22` in CI.
- Default database: SQLite, with PostgreSQL migration/dry-run tooling.
- Current release: `v5.0.0-rc.1`, a pre-release (the first 5.0 release
  candidate; [upgrading from 4.2.x](docs/UPGRADE.md#upgrading-from-42x-to-500-rc1)).
  The 18 official packages under `packages/` are built into the Control
  binary, which installs, upgrades and enables them at every start; there is
  no packages archive and nothing to import. Binaries are published for Linux
  amd64 and arm64 only. Releases after `v4.0.0` carry no evidence bundle:
  verify them with `SHA256SUMS.txt` and its signature `SHA256SUMS.txt.sig`,
  cosign and the pinned official root
  ([`docs/UPGRADE.md`](docs/UPGRADE.md#artifact-verification)).
- Product delivery line: `v4.0.0-alpha.*` remains immutable historical preview
  evidence. It is not an alternative trust root or release channel.
- Agent-first status: nodes run `anix-agent` on the `anix.agent.v1`
  bidirectional gRPC control stream; Agent plugin dispatch and topology
  execution are off until the Agent release carries the Agent plugins (M4).
  Legacy data-plane protocols remain available for controlled compatibility
  and rollback.

Do not infer production completeness from a route or UI existing. Check
[`docs/features.md`](docs/features.md), [`TODO.md`](TODO.md), and
[`docs/audit/test-gap.md`](docs/audit/test-gap.md) before marking a feature
complete.

Control keeps REST/UniProxy, the legacy panel-node gRPC services, and existing
WebSocket paths available as compatibility surfaces. The official packages,
`identity-platform` included, are built into the release binary: at every
start Control verifies each one against the pinned official root, installs or
upgrades it to the binary's version, enables it and runs its package host
(`anix-control package-host <id>`). Nothing is staged from release assets or
imported, by the container image or the installer. Operators should retain
the documented rollback plan throughout rollout. Package hosts may invoke
narrowly scoped, kernel-owned compatibility bridge operations to preserve
established route semantics; there is no direct HTTP fallback when a package
is unavailable.

## Documentation

- Documentation landing page: [`docs/README.md`](docs/README.md)
- Feature status register: [`docs/features.md`](docs/features.md)
- Roadmap: [`ROADMAP.md`](ROADMAP.md)
- Concrete backlog: [`TODO.md`](TODO.md)
- Changelog: [`CHANGELOG.md`](CHANGELOG.md)
- Brand and artifact migration: [`docs/BRAND_MIGRATION.md`](docs/BRAND_MIGRATION.md)
- Deployment guide (containers first): [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)
- Container design and multi-replica status: [`docs/architecture/container-deployment.md`](docs/architecture/container-deployment.md)
- Upgrade runbook: [`docs/UPGRADE.md`](docs/UPGRADE.md)
- Native systemd install (the alternative to containers): [`docs/guide/release-installation.md`](docs/guide/release-installation.md)
- Legacy migration plan: [`docs/guide/legacy-migration.md`](docs/guide/legacy-migration.md)
- Manual intervention requirements:
  [`docs/manual-intervention.md`](docs/manual-intervention.md)
- Audit registers:
  [`docs/audit/repository-audit.md`](docs/audit/repository-audit.md),
  [`docs/audit/security-risk.md`](docs/audit/security-risk.md),
  [`docs/audit/concurrency-risk.md`](docs/audit/concurrency-risk.md),
  [`docs/audit/performance-risk.md`](docs/audit/performance-risk.md),
  [`docs/audit/test-gap.md`](docs/audit/test-gap.md)

## Repository Layout

See [`docs/reference/repository-layout.md`](docs/reference/repository-layout.md) (contributor and agent rules: [`AGENTS.md`](AGENTS.md)).

Generated local outputs should stay ignored and out of the source tree. Use:

```bash
bash config/deploy/clean_local_build_artifacts.sh --dry-run
bash config/deploy/clean_local_build_artifacts.sh --dry-run --include-deploy-backups
```

## Install

Containers are the primary deployment. Each release publishes a signed,
multi-architecture image `ghcr.io/anixops/anix-control` (digest in the release
`docker-image.txt`) that needs no config file: run it with
`docker-compose.prod.yml` and an external PostgreSQL, or with the Helm chart in
`config/deploy/helm/anix-control`. Steps: [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

The native systemd installer downloads checked GitHub Release assets and does
not clone the repository or build on the target host. It installs the release
tag you name with `--version` and never resolves a moving "latest". Pin the
production tag:

```bash
export VERSION=v5.0.0-rc.1   # the release tag to install
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${VERSION}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh install --version "${VERSION}"
rm -f /tmp/anix-control-install.sh
```

No administrator password is set anywhere: the installer ends by printing a
one-time link, `https://<domain>/claim#anixclaim_…`, that creates the first
administrator. It works once, for 24 hours; `anix-control admin claim-link`
prints a new one while the panel has no administrator (the release
installation guide has the exact command).

See the [release installation guide](docs/guide/release-installation.md) for the systemd path's
reverse proxy, update, rollback, and security steps. Existing panel or foreign
panel migrations must follow the [legacy migration plan](docs/guide/legacy-migration.md).

The `--include-deploy-backups` mode is only for ignored local deploy archive
leftovers in the checkout, such as stale frontend tarballs and internal zip
archives. It keeps database backups, config, certificates, and
`web/node_modules`.

## Development Checks

Prefer the same checks GitHub Actions runs. Focused local checks are fine while
developing; release builds are not.

Start an isolated local development instance with:

```bash
make run
```

It keeps its SQLite database and root key in the ignored `.dev/`, serves
the web app and the API on `127.0.0.1:19080` and gRPC on `127.0.0.1:50052`,
and runs with `-debug`. On an empty database `make claim-link` prints the
link that creates the first administrator. An already running
system `anix-control` service and its production-style ports are not
changed. Set `RUN_CONFIG=/path/to/config.yaml` to load a configuration file
as well.

```bash
go test ./...
go test -race ./...
bash api/grpc/gen.sh
git diff --exit-code -- api/grpc/v2boardpb
bash config/deploy/clean_local_build_artifacts.sh --dry-run
```

Frontend checks are run in CI from a clean checkout:

```bash
cd web
npm ci
npm audit --omit=dev --audit-level=moderate
npm test
npm run build
```

After local frontend verification, remove generated outputs with:

```bash
bash config/deploy/clean_local_build_artifacts.sh
bash config/deploy/clean_local_build_artifacts.sh --include-deploy-backups
```

## Release Policy

All release artifacts must be built by GitHub Actions. Do not build release
binaries, frontend archives, Docker metadata, checksums, SBOMs, or release
manifests on a production host.

Release jobs accept stable, alpha, beta, and release-candidate tags such as
`v4.0.0`, `v4.0.0-alpha.7`, `v4.0.0-beta.1`, and `v4.0.0-rc.1`; a suffixed
tag is published as a pre-release. Tag builds publish the signed image
`ghcr.io/anixops/anix-control` and these release assets
([`docs/RELEASING.md`](docs/RELEASING.md#what-the-tag-pipeline-does)):

- `anix-control-linux-amd64.tar.gz` and `anix-control-linux-arm64.tar.gz`
  (the official packages built in)
- `anix-control-frontend.tar.gz`
- `SHA256SUMS.txt` and its signature `SHA256SUMS.txt.sig`, made with the
  official package key, whose public half is `official-public-key.raw`
- the source SBOM `anix-control-source.sbom.spdx.json`
- `docker-image.txt` (the image digest)
- the machine-readable `RELEASE_MANIFEST.json`
- `RELEASE_NOTES.md`, also the release body: the build metadata, the tag's
  CHANGELOG section and the image digest
- the Agent installer `agent-install.sh` and its signature `agent-install.sh.sig`

Local deploy scripts that perform source-tree builds are guarded and are not a
release path. See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) and
[`docs/UPGRADE.md`](docs/UPGRADE.md) for operator deployment, upgrade, and
rollback expectations.

## Security Notes

- Keep secrets, production databases, TLS private keys, payment credentials, and
  domain/server changes out of commits.
- Payment callbacks are provider-specific compatibility surfaces; do not
  normalize or enable them without provider verification tests.
- Alipay, WeChat, and USDT payment callbacks remain planned and are blocked
  from enable/use until implementation and tests exist.
- Forwarding features must remain auditable, role-scoped, and covered by quota,
  permission, runtime, and cancellation tests.

## Compatibility Surfaces

Some routes intentionally keep legacy or external protocol behavior:

- `/api/v1/client/subscribe?token=`
- `/{subscribe_path}/:token`
- `/api/v1/server/UniProxy/*`
- `/api/v2/server/UniProxy/*`
- `/api/v2/node/register`
- `/api/v2/node/heartbeat`
- Telegram webhooks
- payment callbacks/webhooks
- forwarding agent and internal traffic upload endpoints

The `v2_*` database tables, `v2board` protobuf wire namespace, and historical
SQLite path are intentionally retained during the migration. The Go module has
moved to `github.com/AnixOps/anix-control/v5`.

Do not change these response shapes without checking
[`docs/features.md`](docs/features.md) and adding compatibility tests.
