# Repository Audit Baseline

Date: 2026-07-08

This is the current audit baseline for `github.com/anixops/v2board`. It is a working document: items marked as gaps remain open until backed by code, tests, CI evidence, or operational runbooks.

## Current Shape

- Go module: `github.com/anixops/v2board`
- Go version in `go.mod`: `1.25.0`
- CI Go version: `1.25`
- Current package count from `go list ./...`: 39
- Current Go test file count under `cmd`, `internal`, and `config`: 79
- Primary backend entry point: `cmd/server/main.go`
- Supporting commands: `cmd/configgen`, `cmd/integration-test`, `cmd/migrate`, `cmd/report`, `cmd/sqlite2postgres`, `cmd/subtest`, `cmd/verify`
- Frontend: Vue/Vite app under `web/`
- Generated API docs: `docs/swagger.json`, `docs/swagger.yaml`, and `docs/docs.go`

## Backend Architecture

The backend is organized around these package boundaries:

- `internal/router`: Gin route registration.
- `internal/handler`: HTTP handlers, request binding, auth-facing API behavior, WebSocket endpoints, and admin/user/node API surfaces.
- `internal/service`: business logic for users, orders, subscriptions, nodes, forwarding, stats, notifications, MFA, Telegram, and runtime jobs.
- `internal/model`: GORM models and persistence schema definitions.
- `internal/database`: global GORM initialization and database helpers.
- `internal/cache`: in-memory cache implementation with TTL, set support, and LRU eviction. Redis is represented in config/API shape but the current local implementation baseline is memory cache.
- `internal/grpc`: gRPC server, interceptors, connection tracking, and node-facing services.
- `internal/gost`: GOST client/manager integration.
- `internal/parser`: subscription format parsing and rendering.
- `internal/payment`: payment registry and gateway implementations.
- `internal/websocket`: websocket subscription and messaging helpers.

The current server process initializes config, database, migrations, cache, HTTP routing, gRPC support, forward runtime bootstrap, and graceful shutdown from `cmd/server/main.go`.

## Configuration And Runtime Paths

Configuration is loaded from `config/config.yaml` by default and can be overridden with `-config`. Runtime path resolution now accounts for binaries launched from the repository root, a build directory, or a deployed binary directory.

Important config surfaces:

- HTTP server host/port/mode.
- TLS and trusted proxy settings.
- Frontend static path.
- SQLite or PostgreSQL database settings.
- Cache settings.
- JWT secret/expiry.
- Admin bootstrap credentials.
- Forward runtime settings, including Ansible/nftables paths.
- gRPC settings.

Risk note: production safety still depends on operator-provided secrets, TLS/domain configuration, payment gateway credentials, database backups, and deployment environment controls. These are tracked in `docs/manual-intervention.md`.

## Database Layer

The project uses GORM with SQLite and PostgreSQL support.

Current observations:

- SQLite is the default database path if no database config is provided.
- PostgreSQL connection pool settings are configurable.
- Migrations are mostly automatic through GORM `AutoMigrate`.
- Several targeted schema repair/guard helpers now exist for stats and forward runtime jobs.
- Test coverage includes SQLite service tests and a PostgreSQL stats regression workflow.
- A SQLite-to-PostgreSQL operator runbook now documents backup, dry run, import, verification evidence, and rollback.
- CI now runs a SQLite-to-PostgreSQL dry-run gate against a temporary SQLite fixture and PostgreSQL service.

Open gaps:

- Schema-helper-specific rollback evidence is not yet complete for every future schema change.
- Some service tests share fixed SQLite temp paths, so package tests must run serially (`-p=1`) until test isolation is improved.
- Large traffic/stat tables need explicit index and retention review.

## HTTP, gRPC, WebSocket, And Node APIs

Current API surfaces:

- Public health and subscription endpoints.
- User API under `/api/v2`.
- Admin API under `/api/v2/admin`.
- Node/UniProxy API for config, users, traffic push, online state, registration, and heartbeat.
- Compatibility route for legacy subscription clients: `/api/v1/client/subscribe?token=`.
- gRPC node/user/traffic/health services.
- WebSocket endpoints for monitoring/subscriptions.

Forwarding documentation now has a dedicated engineering baseline under
`docs/forwarding/`:

- `design.md`: panel/runtime/relay ownership, data model, backend semantics, and verification standards.
- `api.md`: Flux-shaped routes, admin/runtime/internal routes, response envelopes, and request DTOs.
- `security.md`: trust boundaries, auth controls, runtime command execution, quota, token, and audit risks.
- `compatibility.md`: Flux compatibility boundaries, local extension surfaces, and remaining clone gaps.

Open gaps:

- API error response structure is not fully uniform across all handlers.
  Ansible Machines admin routes, Forward Node management routes, and Forward
  Rule management routes, plus Forward stats, user-rule, and connection-test
  routes, now use the panel `code/msg/ts/data` envelope with handler tests for
  success and error cases. Forward observability targets/trend/topology/
  multi-ingress routes also have response-envelope coverage and frontend API
  mapping tests. Forward internal traffic report/snapshot JSON routes have
  response-envelope tests, while the legacy Flux-compatible `/flow/upload`
  endpoint intentionally remains a plain `ok` response. Admin traffic hourly
  and user-ranking routes now have response-envelope handler tests plus
  TrafficHourly frontend compatibility coverage for legacy and enveloped
  payloads. The admin dashboard route now also returns the panel envelope and
  has Dashboard frontend compatibility coverage. Admin user stats now has the
  same response-envelope handler coverage and Users frontend compatibility
  coverage, admin order stats has matching Orders frontend compatibility
  coverage, admin node stats has matching Nodes frontend compatibility
  coverage, and admin system info has matching AdminLayout version-display
  compatibility coverage. Admin invite stats and invite config now have
  matching Invite frontend compatibility coverage, user invite
  info/code/commission/withdrawal responses and admin invite config-update and
  withdrawal responses now have handler coverage plus Invite frontend/API
  compatibility coverage, admin payment stats,
  gateway CRUD/toggle, and payment-record list responses have matching Payment
  frontend compatibility coverage, user payment channel/create/status/record and
  legacy X402/fiat create/check success and user-error responses have handler
  envelope coverage with callback/webhook compatibility retained,
  user/admin MFA success responses have handler coverage and Admin MFA frontend
  compatibility coverage, user/admin notification success responses have handler
  coverage and Admin Notifications frontend compatibility coverage, admin/user
  Telegram panel API success responses have handler coverage and Admin Telegram
  frontend compatibility coverage while the public Telegram webhook remains a
  `status=ok` compatibility path, admin subscription management group,
  template, protocol, format, and preview responses have handler coverage plus
  Subscriptions frontend/API compatibility coverage while public subscription
  downloads remain plain subscription content, admin node management CRUD,
  credentials, logs, protocol, raw-config, and auth-key success responses have
  handler coverage plus Nodes frontend/API compatibility coverage while node
  registration and heartbeat remain node-client compatibility responses, admin
  Agent list/task-result/task-history/monitor/task-create/execute responses
  have handler coverage, including WebSocket ack dispatch success paths, plus
  Agent frontend/API compatibility coverage while runtime Agent protocol
  responses remain compatibility responses, admin
  user management CRUD, ban/unban, traffic-reset, and subscribe-reset success
  responses have handler coverage plus Users frontend/API compatibility
  coverage while `/user/reset` remains a Flux compatibility response, admin
  plan management list/detail/create/update/delete/assign success responses have
  handler coverage plus Plans frontend/API compatibility coverage, admin
  order management list/detail/status/paid/cancel success responses have handler
  coverage plus Orders frontend/API compatibility coverage, admin
  subscription stats has matching Subscriptions frontend compatibility coverage, admin
  subscription settings has matching System and Users compatibility coverage,
  admin system backup stats, backup config, and backup record operations have
  matching System backup-view compatibility coverage with sensitive-field
  masking retained, and admin load balancer stats has matching handler and
  frontend API mapping coverage. Admin
  ticket list/reply/close success and user-error responses now have handler coverage plus
  Tickets admin frontend compatibility coverage, and admin coupon
  list/create/delete success and user-error responses now have handler coverage
  plus Coupons admin frontend compatibility coverage. Admin knowledge list/create/update/delete
  success and user-error responses now have handler coverage plus Knowledge
  admin frontend compatibility coverage and restored locale strings. Admin system audit-log
  responses now have handler coverage plus System audit frontend compatibility
  coverage, and admin system config CRUD success responses now have handler
  coverage plus System runtime/config compatibility coverage while retaining
  sensitive-value masking. Admin load balancer CRUD and health-check success
  responses now have handler coverage plus System load balancer frontend
  compatibility coverage. User
  registration success and user-error paths, login success and user-error paths, profile success and user-error paths, dashboard success and user-error paths, subscription info success and user-error paths, plan list,
  coupon-check success and business-error paths, knowledge list/detail success and user-error paths, ticket list/create/detail/reply/close success and user-error paths,
  order-save success/user-error paths, order list/detail success and user-error paths, public payment methods, and public
  payment-status success and missing-record responses now use the same envelope
  while retaining their token, profile, subscription, and order payloads under
  `data`;
  remaining older modules still need the same treatment.
- Context propagation and cancellation handling are uneven in older service paths.
- WebSocket auth and lifecycle behavior needs a dedicated audit pass.
- Handler/service boundaries are improved in newer forward modules but still mixed in older modules.
- Forwarding docs are now present, but the module still needs broader permission,
  quota, audit-log, race, and runtime smoke coverage before the forwarding phase
  can be called complete.

## CI/CD Baseline

Current workflow files:

- `.github/workflows/ci.yml`
- `.github/workflows/integration-test.yml`

Current CI coverage includes:

- `go mod tidy` diff check.
- `gofmt` check.
- `go vet ./...`.
- Full `go test ./... -count=1 -p=1`.
- `go test -race ./... -count=1 -p=1`.
- Benchmark smoke for service/handler/router packages.
- Blocking `govulncheck ./...`.
- Blocking production `gosec` gate for `cmd/server` and non-test runtime packages.
- Blocking generated-file-excluded `gosec` gate with report artifact upload.
- Blocking `golangci-lint` gate with report artifact upload.
- Docker build smoke.
- Backend build.
- Frontend test/build/audit jobs.
- Frontend bundle size report artifact for Vite assets and heavy admin/runtime chunks.
- PostgreSQL stats regression job.
- Forward runtime, gRPC, and command-focused jobs.
- Strict release tag gate for `vX.Y.Z`.
- Multi-platform release binaries for linux, darwin, and windows on amd64/arm64.
- Release checksums.
- Release assets include `OPERATOR_DEPLOYMENT.md` with the manual deployment, verification, and rollback flow.
- Release assets include `v2board-source.sbom.spdx.json` generated by `anchore/sbom-action`.

Open CI/CD gaps:

- Full-repository generated-file-excluded `gosec` has a clean baseline and is now blocking in CI.
- `golangci-lint` is now blocking in CI; current local baseline is zero visible and zero uncapped findings.
- `.golangci.yml` excludes `web/node_modules` so frontend dependency source is not scanned as first-party Go code.
- Migration dry-run is not yet a complete release gate for all schema changes.

## Test Baseline

Current coverage types present in the repository:

- Unit tests for cache, config, database, models, parser, services, handlers, router, payment, utils, websocket.
- Service tests with table-style scenarios in several modules.
- Handler and router tests for auth, admin, forward, subscription, node, and payment behavior.
- Integration and smoke tests under `internal/tests`.
- Race detector CI.
- PostgreSQL stats regression test.
- Frontend unit tests and build checks.

Open gaps:

- Some tests depend on shared fixed SQLite paths and must be isolated.
- Fuzz tests are not yet established for subscription parsing, URL/token handling, and traffic payload parsing.
- Benchmark smoke now includes a realistic SQLite stats/ranking dataset; critical high-volume paths still need meaningful thresholds later.
- End-to-end production-like node integration still depends on environment secrets and cannot be assumed from unit tests alone.

## Immediate Engineering Priorities

1. Keep full Go test, vet, race, and vulnerability checks green.
2. Keep generated-file-excluded full-repository `gosec` and full-repository `golangci-lint` blocking with clean baselines.
3. Finish context/error handling audits in service paths that touch network, database, and runtime jobs.
4. Improve test database isolation so service tests can safely run in parallel package jobs.
5. Add schema-helper-specific rollback evidence as schema changes land.
6. Keep forwarding functionality small, auditable, and covered by service/handler/race tests.
