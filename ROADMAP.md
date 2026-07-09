# Roadmap

Date: 2026-07-08

This roadmap is ordered by stability and release risk. Items move to done only after code, tests, and CI evidence exist.
The P0 WireGuard protocol support phase takes priority over ordinary P1/P2 cleanup once implementation starts.

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

## Phase 6: P0 WireGuard Protocol Support

Status: in progress

Priority: P0, above ordinary P1/P2 finishing work.

- Deliver WireGuard user access through the selected dual-node relay path: WireGuard access -> domestic entry termination -> GOST relay+QUIC -> overseas exit NAT.
- Keep GOST relay+QUIC as the default domestic-entry to overseas-exit tunnel backend.
- Provide a panel UI one-click switch for WSS compatibility mode through GOST relay+WSS; WSS is not the default mode.
- Panel first slice is implemented: `wireguard` protocol template, `v2_wireguard_peer`, automatic IPv4 peer IP allocation, X25519 keypair custody, preshared key custody, default MTU/DNS/allowed IPs, native WireGuard `.conf` output, sing-box WireGuard outbound output, and unit coverage.
- Add remaining panel API contracts, validation, and admin UI for CIDR, server keys, tunnel type, routes, limits, traffic accounting, entry node, exit node, and WSS compatibility switching.
- Verify Shadowrocket, Loon, v2rayN, and other common client import behavior beyond the native `.conf`/sing-box outputs now covered by tests.
- V2bX runtime slice is in progress: the panel now exposes runtime peer fields over UniProxy HTTP and gRPC, and V2bX can consume those fields for the WireGuard entry runtime, parse peer traffic deltas, and report initial peer online state in v2.3.3. Full dual-node relay, WSS mode, exit NAT, speed-limit behavior, and CI relay-path evidence remain required.
- Add integration tests and GitHub Actions verification before marking the protocol path complete.

Current slice: panel peer custody, subscription output, runtime peer downlink fields, initial V2bX WireGuard entry runtime, traffic delta parsing, and peer online-state reporting are implemented. Admin UI, full GOST relay+QUIC/WSS dual-node orchestration, exit NAT evidence, migration dry-run evidence, speed-limit behavior, and CI relay-path verification remain pending.

## Phase 7: API And UI Consistency

Status: planned

- Normalize API error responses without breaking existing clients.
- Keep legacy subscription compatibility while documenting the preferred route.
- Improve admin traffic, node, user, and forwarding views with tested UI behavior.
- Keep frontend test/build/audit jobs green.
- Avoid copying third-party AGPL code, assets, UI text, or implementation details.

## Phase 8: Release Maturity

Status: planned

- Keep strict `vX.Y.Z` release tags.
- Produce multi-platform binaries and checksums.
- Add SBOM and migration dry-run artifacts.
- Publish Docker metadata and digest.
- Produce release notes from verified changes.
- Maintain rollback notes for schema/runtime changes.
