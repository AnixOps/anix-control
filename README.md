# v2board_AnixOps

`v2board_AnixOps` is the panel-side service for AnixOps. It combines a Go
backend, Vue 3 admin/user frontend, node communication APIs, subscription
generation, payment/order management, traffic statistics, and the in-progress
minimal forwarding module.

The project is being hardened toward a stable, testable, GitHub Actions-only
release process. Treat [`docs/features.md`](docs/features.md) as the source of
truth for implemented, partial, planned, compatibility, and deferred features.

## Current Status

- Main branch: `go_dev`.
- Backend: Go module `github.com/anixops/v2board`, toolchain pinned to Go
  `1.26.5` in CI.
- Frontend: Vue 3 + Vite under [`web/`](web), Node.js `22` in CI.
- Default database: SQLite, with PostgreSQL migration/dry-run tooling.
- Current version line: frontend package `2.3.1`; release maturity work is
  still in progress.

Do not infer production completeness from a route or UI existing. Check
[`docs/features.md`](docs/features.md), [`TODO.md`](TODO.md), and
[`docs/audit/test-gap.md`](docs/audit/test-gap.md) before marking a feature
complete.

## Documentation

- Documentation landing page: [`docs/README.md`](docs/README.md)
- Feature status register: [`docs/features.md`](docs/features.md)
- Roadmap: [`ROADMAP.md`](ROADMAP.md)
- Concrete backlog: [`TODO.md`](TODO.md)
- Changelog: [`CHANGELOG.md`](CHANGELOG.md)
- Deployment guide: [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)
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
```

## Development Checks

Prefer the same checks GitHub Actions runs. Focused local checks are fine while
developing; release builds are not.

```bash
go test ./...
go test -race ./...
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
```

## Release Policy

All release artifacts must be built by GitHub Actions. Do not build release
binaries, frontend archives, Docker metadata, checksums, SBOMs, or release
manifests on a production host.

Release jobs are guarded by strict tag gating and CI policy checks. Tag builds
produce:

- multi-platform backend artifacts
- frontend archives
- checksum file
- SPDX SBOM
- migration dry-run evidence
- Docker metadata
- machine-readable `RELEASE_MANIFEST.json`
- operator deployment runbook
- generated GitHub release notes

Local deploy scripts that perform source-tree builds are guarded and are not a
release path. See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) for operator
deployment and rollback expectations.

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

Do not change these response shapes without checking
[`docs/features.md`](docs/features.md) and adding compatibility tests.
