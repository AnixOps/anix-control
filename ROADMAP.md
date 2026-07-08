# Roadmap

Date: 2026-07-08

This roadmap is ordered by stability and release risk. Items move to done only after code, tests, and CI evidence exist.

## Phase 1: Audit And Baseline

Status: in progress

- Maintain repository, security, concurrency, performance, and test-gap audits.
- Keep manual intervention requirements explicit.
- Keep `docs/features.md` current as the feature status source of truth.
- Keep `CHANGELOG.md` updated for every fix.
- Keep full Go test, vet, race, and vulnerability checks green.
- Separate confirmed product risks from test-only/tooling risks.

## Phase 2: CI/CD Hardening

Status: in progress

- Keep blocking gates for `go mod tidy`, `gofmt`, `go vet`, full Go tests, race tests, and `govulncheck`.
- Reduce `gosec` findings until production runtime findings can become blocking.
- Reduce `golangci-lint` findings until lint can become blocking.
- Add migration dry-run gates for all schema changes.
- Add SBOM generation to release.
- Replace placeholder production deploy steps with documented operator-controlled deployment.

## Phase 3: Core Stability And Security

Status: in progress

- Fix unchecked errors in production services.
- Improve `context.Context` propagation through network, database, worker, and handler paths.
- Remove predictable or weak random sources from runtime code.
- Harden token generation, bootstrap credentials, subscription compatibility, and log redaction.
- Audit WebSocket auth, lifecycle, deadlines, and cleanup.
- Make test databases isolated enough for reliable parallel package execution.

## Phase 4: Data Integrity And Traffic Accounting

Status: in progress

- Keep negative traffic and invalid rate inputs out of write paths.
- Sanitize historical dirty traffic rows during reads where safe.
- Verify hourly traffic, ranking, dashboard, and PostgreSQL stats behavior.
- Document traffic log indexes, retention, and migration behavior.
- Add benchmarks for high-volume stats queries.

## Phase 5: Forwarding Module Stabilization

Status: in progress

- Keep forwarding features minimal, auditable, and role-scoped.
- Harden runtime job idempotency, partial unique guards, and repair flows.
- Cover pause/resume/start/stop state transitions with service, handler, and race tests.
- Add forwarding design, API, security, and compatibility docs.
- Review long-running worker cancellation and drain behavior.

## Phase 6: API And UI Consistency

Status: planned

- Normalize API error responses without breaking existing clients.
- Keep legacy subscription compatibility while documenting the preferred route.
- Improve admin traffic, node, user, and forwarding views with tested UI behavior.
- Keep frontend test/build/audit jobs green.
- Avoid copying third-party AGPL code, assets, UI text, or implementation details.

## Phase 7: Release Maturity

Status: planned

- Keep strict `vX.Y.Z` release tags.
- Produce multi-platform binaries and checksums.
- Add SBOM and migration dry-run artifacts.
- Publish Docker metadata and digest.
- Produce release notes from verified changes.
- Maintain rollback notes for schema/runtime changes.
