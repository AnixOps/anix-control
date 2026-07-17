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
- Backend: Go module `github.com/AnixOps/anix-control/v4`; the toolchain is
  pinned to Go `1.26.5` in CI.
- Frontend: Vue 3 + Vite under [`web/`](web), Node.js `22` in CI.
- Default database: SQLite, with PostgreSQL migration/dry-run tooling.
- Current preview: `v4.0.0-alpha.4` (the signed `machine-telemetry` 1.1.0
  Control/Agent/WebUI reference package and actor-scoped plugin management are
  usable in this alpha).
- Agent-first status: the `anix.agent.v1` bidirectional gRPC control stream is
  an opt-in foundation in this alpha, not yet the only production task path.

Do not infer production completeness from a route or UI existing. Check
[`docs/features.md`](docs/features.md), [`TODO.md`](TODO.md), and
[`docs/audit/test-gap.md`](docs/audit/test-gap.md) before marking a feature
complete.

The v4 alpha keeps REST/UniProxy, the legacy panel-node gRPC services, and the
existing WebSocket paths available while the new Agent control stream is
validated. Production task sources have not all moved to the new stream, so
operators should keep a tested fallback configured. This is an alpha canary,
not a stable all-purpose Agent rollout: stable publication requires a passing
72-hour canary and explicit operator authorization.

## Documentation

- Documentation landing page: [`docs/README.md`](docs/README.md)
- Feature status register: [`docs/features.md`](docs/features.md)
- Roadmap: [`ROADMAP.md`](ROADMAP.md)
- Concrete backlog: [`TODO.md`](TODO.md)
- Changelog: [`CHANGELOG.md`](CHANGELOG.md)
- Brand and artifact migration: [`docs/BRAND_MIGRATION.md`](docs/BRAND_MIGRATION.md)
- Deployment guide: [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)
- Upgrade runbook: [`docs/UPGRADE.md`](docs/UPGRADE.md)
- Native release install: [`docs/guide/release-installation.md`](docs/guide/release-installation.md)
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

```text
cmd/                 Go command entrypoints
config/              Config examples, deployment scripts, migration tooling
docs/                Product, audit, deployment, and reference docs
internal/            Backend handlers, services, models, middleware, runtime code
public/              Legacy/static frontend output location if present
web/                 Vue 3 frontend source
.github/workflows/   CI, integration, and release workflows
```

Generated local outputs should stay ignored and out of the source tree. Use:

```bash
bash config/deploy/clean_local_build_artifacts.sh --dry-run
bash config/deploy/clean_local_build_artifacts.sh --dry-run --include-deploy-backups
```

## Release Install

Production installation downloads checked GitHub Release assets and does not
clone the repository or build on the target host. Pin the production tag:

```bash
export VERSION=v4.0.0-alpha.4
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${VERSION}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh install --version "${VERSION}" --admin-email "admin@example.com"
rm -f /tmp/anix-control-install.sh
```

See the [release installation guide](docs/guide/release-installation.md) for
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

On first use this creates the ignored `config/config.dev.yaml` from
`config/config.dev.yaml.example`. It listens on `127.0.0.1:19080` for the API,
`127.0.0.1:19000` for the frontend, and `127.0.0.1:50052` for gRPC, and uses a
separate development SQLite database. An already running system
`anix-control` service and its production-style ports are not changed. Set
`RUN_CONFIG=/path/to/config.yaml` to run with another explicit configuration.

```bash
go test ./...
go test -race ./...
bash api/grpc/gen.sh
git diff --exit-code -- api/grpc/v2boardpb api/grpc/agent/v1
bash config/deploy/clean_local_build_artifacts.sh --dry-run
```

Frontend checks are run in CI from a clean checkout:

```bash
cd web
npm ci
npm audit --audit-level=moderate
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
`v4.0.0`, `v4.0.0-alpha.4`, `v4.0.0-beta.1`, and `v4.0.0-rc.1`. Tag builds
produce:

- multi-platform `anix-control-*` backend artifacts
- `anix-control-frontend.*` archives
- checksum file
- SPDX SBOM
- migration dry-run evidence
- Docker metadata
- machine-readable `RELEASE_MANIFEST.json`
- operator deployment runbook
- deterministic `RELEASE_NOTES.md` plus generated GitHub release notes

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
moved to `github.com/AnixOps/anix-control/v4`.

Do not change these response shapes without checking
[`docs/features.md`](docs/features.md) and adding compatibility tests.
