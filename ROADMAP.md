# Roadmap

Date: 2026-07-17

This roadmap is ordered by stability and release risk. Items move to done only after code, tests, and CI evidence exist.
The P0 WireGuard protocol support phase takes priority over ordinary P1/P2 cleanup once implementation starts.

## Primary 3.1 To 4.0 Plugin Transformation

Status: in progress

The version-gated microkernel migration is now the primary product roadmap.
See [`docs/architecture/plugin-platform-roadmap.md`](docs/architecture/plugin-platform-roadmap.md)
for the package model, WebUI extension contract, exit evidence, and rollback
requirements. The executable gate checklist and exact verification commands are
in [`docs/architecture/upgrade-program.md`](docs/architecture/upgrade-program.md).

- `3.1.0`: signed package lifecycle, durable Control/Agent operations, and the
  pluggable WebUI foundation.
- `3.2.0`: declarative topology execution and `nftables-forward` canary.
- `3.3.0`: `gost-mesh` plus `nat-egress`.
- `3.4.0`: WireGuard and protocol-runtime composition.
- `3.5.0`: business-domain plugin migration and `/api/v2` adapter ownership.
- `4.0.0`: remove the original coupled business/runtime mode and boot as a
  plugin-only platform.

The `v4.0.0-alpha.6` slice advances the 3.2 gate with a signed crash-safe
`nftables-forward` 1.2.0 package, its package WebUI, and a kernel-observed
promotion contract. The Agent semantically verifies live nftables JSON, emits
a stable ruleset SHA-256 plus per-rule counters through a private observation
file, and the Control kernel persists only bounded evidence from authenticated
heartbeats. Topology promotion and rollback bind that evidence to exact
version/config-hash/revision state and use a durable grace deadline. Topology
execution remains off by default. Live staging restore, legacy fallback
rehearsal, rollout records, a fresh 72-hour canary, and explicit stable-release
authorization remain required.

The older workstreams below remain relevant compatibility and quality work,
but they do not redefine these release gates. Existing coupled runtime work is
a fallback until the corresponding plugin has equivalent tested behavior.

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
- First panel UI slice is implemented for WireGuard visual protocol configuration, including the one-click WSS compatibility switch through GOST relay+WSS; WSS is not the default mode.
- Panel first slice is implemented: `wireguard` protocol template, `v2_wireguard_peer`, automatic IPv4 peer IP allocation, X25519 keypair custody, preshared key custody, default MTU/DNS/allowed IPs, native WireGuard `.conf` output, sing-box 1.13-compatible WireGuard endpoint output, and unit coverage.
- Add remaining panel API validation and safer operator workflows for CIDR, server keys, routes, limits, traffic accounting, entry node, exit node, and migration behavior.
- Verify Shadowrocket, Loon, v2rayN, and other common client import behavior beyond the native `.conf`/sing-box outputs now covered by tests.
- V2bX runtime slice is in progress: the panel now exposes runtime peer fields and relay TUN contract fields over UniProxy HTTP and gRPC, and V2bX can consume those fields for the WireGuard entry runtime, parse peer traffic deltas, report initial peer online state, plan entry/exit GOST TUN relay commands, apply entry policy routing, and apply exit NAT commands. Real dual-node relay evidence, WSS compatibility evidence, speed-limit behavior, and CI relay-path evidence remain required.
- Add integration tests and GitHub Actions verification before marking the protocol path complete.

Current slice: panel peer custody, subscription output, runtime peer downlink fields, relay TUN contract fields, first admin WireGuard visual protocol form, initial V2bX WireGuard entry runtime, traffic delta parsing, peer online-state reporting, GOST TUN command planning, entry policy routing, and exit NAT command application are implemented. Real GOST relay+QUIC/WSS dual-node evidence, API validation hardening, migration dry-run evidence, speed-limit behavior, and CI relay-path verification remain pending.

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
