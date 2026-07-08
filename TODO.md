# TODO

Date: 2026-07-08

This list is intentionally concrete. Do not mark an item done without code, tests, and verification evidence where applicable.

## P0: Audit Deliverables

- [x] Create `docs/audit/security-risk.md`.
- [x] Create `docs/audit/test-gap.md`.
- [x] Create `docs/audit/repository-audit.md`.
- [x] Create `docs/audit/concurrency-risk.md`.
- [x] Create `docs/audit/performance-risk.md`.
- [x] Create `ROADMAP.md`.
- [x] Create `TODO.md`.
- [x] Create `docs/manual-intervention.md`.
- [ ] Keep all audit files current as fixes land.

## P0: CI Baseline

- [x] Add `go mod tidy` diff check.
- [x] Add gofmt check.
- [x] Add `go vet ./...`.
- [x] Add full `go test ./... -count=1 -p=1`.
- [x] Add `go test -race ./... -count=1 -p=1`.
- [x] Add benchmark smoke job.
- [x] Add blocking `govulncheck`.
- [x] Add generated-file-excluded `gosec` report artifact.
- [x] Add non-blocking `golangci-lint` report.
- [x] Add `.golangci.yml` to exclude frontend dependency trees from Go linting.
- [x] Add Docker build smoke.
- [x] Add migration dry-run gate for release workflow.
- [x] Make production `gosec` findings blocking after triage.
- [x] Make generated-file-excluded full-repository `gosec` findings blocking after triage.
- [x] Make `golangci-lint` blocking after baseline cleanup.
- [x] Add SBOM generation to release workflow.

## P1: Security And Error Handling

- [x] Replace fixed bootstrap admin password when `admin.password` is empty.
- [x] Replace load balancer `math/rand` selection.
- [x] Fix node cache invalidation key conversion.
- [x] Fix remaining service-package production `G104` unchecked error findings.
- [x] Fix remaining production `G104` unchecked error findings outside `internal/service` after full-repository triage.
- [x] Review `G402`, `G204`, `G304`, `G703`, and `G401` findings by runtime risk.
- [x] Review WebSocket auth, origin, deadlines, ping/pong, and cleanup.
- [x] Make token generation return and handle entropy errors in forwarding paths.
- [x] Remove SMTP `InsecureSkipVerify` from notification delivery.
- [x] Return explicit errors for corrupt MFA backup-code JSON.
- [x] Handle stats cache write/delete errors explicitly.
- [x] Return explicit errors for node online cache updates and malformed legacy server config JSON.
- [x] Harden local backup restore path validation, archive size limits, and backup directory permissions.
- [x] Restrict local forward runtime command execution to reviewed `ansible-playbook` executables.
- [x] Triage remaining service-package gosec findings and reduce `gosec ./internal/service` to zero issues.
- [x] Clear handler/websocket gosec findings for WebSocket writes, metrics integer formatting, and UniProxy ETag hashing.
- [x] Clear gRPC package gosec findings for protobuf integer narrowing and unchecked heartbeat updates.
- [x] Clear `cmd/verify` gosec findings for local E2E verification.
- [x] Clear `cmd/subtest` gosec findings for subscription test tooling.
- [x] Clear `cmd/configgen` gosec findings for generated integration client configs.
- [x] Clear `cmd/report` gosec findings for local report generation.
- [x] Clear integration mock server gosec findings.
- [x] Clear integration echo server gosec findings.
- [x] Clear integration local environment gosec findings.
- [x] Clear shared testutil database gosec findings.
- [x] Clear integration runner gosec findings.
- [x] Clear integration binary manager gosec findings.
- [x] Clear integration clients gosec findings.
- [x] Clear `cmd/migrate` dump parser gosec findings.
- [x] Clear all non-generated full-repository gosec findings.
- [x] Suppress generated protobuf `G103` findings in full-repository gosec reports.
- [x] Clear `cmd/sqlite2postgres` golangci-lint findings.
- [x] Clear `cmd/integration-test` golangci-lint findings.
- [x] Clear `cmd/report` golangci-lint findings.
- [x] Clear `cmd/verify` golangci-lint findings.
- [x] Clear integration echo server golangci-lint findings.
- [x] Clear integration local environment golangci-lint findings.
- [x] Clear integration mock server golangci-lint findings.
- [x] Clear integration binary manager golangci-lint findings.
- [x] Clear integration runner golangci-lint findings.
- [x] Clear WebSocket origin utils golangci-lint findings.
- [x] Clear database package golangci-lint findings.
- [x] Clear integration setup script golangci-lint findings.
- [x] Clear middleware package golangci-lint findings.
- [x] Clear integration clients golangci-lint findings.
- [x] Clear `cmd/migrate` and `cmd/server` database close golangci-lint findings.
- [x] Clear router, smoke, and root integration test golangci-lint findings.
- [x] Clear websocket package golangci-lint findings.
- [x] Clear GOST package golangci-lint findings.
- [x] Clear cache package golangci-lint findings.
- [x] Clear gRPC package golangci-lint findings.
- [x] Clear E2E test package golangci-lint findings.
- [x] Clear integration E2E test package golangci-lint findings.
- [x] Clear handler package golangci-lint findings.
- [x] Clear service small-file golangci-lint findings outside the consolidated `service_test.go` suite.
- [x] Clear remaining `internal/service/service_test.go` errcheck findings.
- [x] Harden EPay callback signature and amount verification.
- [x] Ensure plugin payment callback signature tests cover every registered gateway.
- [x] Ensure implemented payment callback/webhook handlers cover valid and rejected signatures for EPay, Stripe, PayPal, and X402.
- [ ] Add callback implementations and tests before enabling Alipay, WeChat, or USDT payment callbacks.

## P1: Concurrency And Lifecycle

- [x] Fix integration echo server race around shutdown/server capture.
- [x] Isolate service test SQLite databases.
- [x] Audit memory cache init/close lifecycle.
- [x] Make forward node health checks context-aware.
- [x] Add cancellation/drain tests for background workers.
- [x] Add gRPC stream cancellation cleanup tests.

## P1: Traffic And Stats

- [x] Fix negative traffic write rejection in core service paths.
- [x] Normalize invalid traffic rate handling in stats paths.
- [x] Add PostgreSQL stats regression coverage.
- [x] Document traffic log indexes and retention policy.
- [x] Add realistic stats/ranking benchmarks.
- [x] Add upper-bound request limit tests for ranking APIs.

## P2: Forwarding

- [x] Make panel forward pause/resume idempotent while runtime jobs are active.
- [x] Add partial unique guard for pending/running runtime jobs.
- [x] Add repair step for historical duplicate runtime jobs.
- [x] Add `docs/forwarding/design.md`.
- [x] Add `docs/forwarding/api.md`.
- [x] Add `docs/forwarding/security.md`.
- [x] Add `docs/forwarding/compatibility.md`.
- [x] Add more executor cancellation and retry tests.
- [x] Benchmark runtime job claiming/listing.

## P2: Compatibility And Operations

- [x] Add legacy subscription route `/api/v1/client/subscribe?token=`.
- [x] Add deployment script syntax and self-test CI checks.
- [x] Document production deploy command and environment prerequisites.
- [x] Document SQLite-to-PostgreSQL migration dry run and rollback.
- [x] Replace placeholder release deploy job with explicit manual/operator flow.

## P3: API And UI Consistency

- [ ] Normalize API error envelopes module by module.
- [ ] Add handler tests before changing response shapes.
- [x] Normalize Ansible Machines response envelopes with handler and frontend compatibility tests.
- [x] Normalize Forward Node management response envelopes with handler and frontend compatibility tests.
- [x] Normalize Forward Rule management response envelopes with handler and frontend compatibility tests.
- [x] Normalize Forward stats, user-rule, and connection-test response envelopes with handler and frontend compatibility tests.
- [x] Cover Forward observability response envelopes with handler and frontend API compatibility tests.
- [x] Cover Forward internal traffic report/snapshot response envelopes with handler tests.
- [x] Normalize admin traffic hourly/user-ranking response envelopes with handler and TrafficHourly frontend compatibility tests.
- [x] Normalize admin dashboard response envelope with handler and Dashboard frontend compatibility tests.
- [x] Normalize admin user stats response envelope with handler and Users frontend compatibility tests.
- [x] Normalize admin order stats response envelope with handler and Orders frontend compatibility tests.
- [x] Normalize admin node stats response envelope with handler and Nodes frontend compatibility tests.
- [x] Normalize admin system info response envelope with handler and AdminLayout frontend compatibility tests.
- [x] Normalize admin invite stats response envelope with handler and Invite frontend compatibility tests.
- [x] Normalize admin payment stats response envelope with handler and Payment frontend compatibility tests.
- [x] Normalize admin subscription stats response envelope with handler and Subscriptions frontend compatibility tests.
- [x] Normalize admin system backup stats response envelope with handler and System frontend compatibility tests.
- [x] Normalize admin load balancer stats response envelope with handler and admin API mapping tests.
- [x] Normalize admin system backup config response envelope with handler, sensitive-field, and System frontend compatibility tests.
- [x] Normalize admin system backup list/create/delete/restore success response envelopes with handler, audit, and System frontend compatibility tests.
- [x] Normalize admin invite config response envelope with handler and Invite frontend compatibility tests.
- [x] Normalize user invite info/code/commission/withdrawal and admin invite config update/withdrawal response envelopes with handler, Invite frontend, and admin API compatibility tests.
- [x] Normalize admin subscription settings response envelope with handler, System, Users, and admin API compatibility tests.
- [x] Normalize admin payment gateway list response envelope with handler and Payment frontend compatibility tests.
- [x] Normalize admin payment gateway CRUD/toggle and payment-record list response envelopes with handler and Payment frontend compatibility tests.
- [x] Normalize user payment channels/create/status/records success response envelopes with handler tests while preserving callback compatibility.
- [x] Normalize legacy X402 and fiat payment create/check success and user-error response envelopes with handler tests while preserving callback/webhook compatibility.
- [x] Normalize user/admin MFA success response envelopes with handler and Admin MFA frontend compatibility tests.
- [x] Normalize user/admin notification success response envelopes with handler and Admin Notifications frontend compatibility tests.
- [x] Normalize admin/user Telegram panel API success response envelopes with handler and Admin Telegram frontend compatibility tests while preserving webhook compatibility.
- [x] Normalize admin subscription management response envelopes with handler, Subscriptions frontend, and admin API compatibility tests while preserving public subscription download compatibility.
- [x] Normalize admin node management CRUD/protocol/log/raw-config/auth-key response envelopes with handler, Nodes frontend, and admin API compatibility tests while preserving node registration/heartbeat compatibility.
- [x] Normalize admin Agent list/task-result/task-history/monitor/task-create/execute response envelopes with handler, Agent frontend, WebSocket ack, and admin API compatibility tests while preserving runtime Agent protocol responses.
- [x] Normalize admin user management CRUD/ban/unban/reset response envelopes with handler, Users frontend, and admin API compatibility tests while preserving `/user/reset` compatibility.
- [x] Normalize admin plan management list/detail/create/update/delete/assign response envelopes with handler, Plans frontend, and admin API compatibility tests.
- [x] Normalize admin order management list/detail/status/paid/cancel response envelopes with handler, Orders frontend, and admin API compatibility tests.
- [x] Normalize user register success and order-save success/user-error response envelopes with handler coverage.
- [x] Normalize user login success response envelope with handler and Login frontend compatibility tests.
- [x] Normalize user order list/detail success and user-error response envelopes with handler and Orders frontend compatibility tests.
- [x] Normalize user subscription info success and user-error response envelope with handler and Subscribe frontend compatibility tests.
- [x] Normalize user profile success and user-error response envelope with handler and user store compatibility tests.
- [x] Normalize user dashboard success and user-error response envelope with handler and user API mapping tests.
- [x] Normalize user plan list response envelope with handler and Plans frontend compatibility tests.
- [x] Normalize user coupon-check success and business-error response envelopes with handler and Plans frontend compatibility tests.
- [x] Normalize user knowledge list/detail success and user-error response envelopes with handler and Knowledge frontend compatibility tests.
- [x] Normalize user ticket list/create/detail/reply/close success and user-error response envelopes with handler and Tickets frontend compatibility tests.
- [x] Normalize public payment methods/status response envelopes with handler tests, including missing payment records.
- [x] Normalize admin ticket list/reply/close success and user-error response envelopes with handler and Tickets frontend compatibility tests.
- [x] Normalize admin coupon list/create/delete success and user-error response envelopes with handler and Coupons frontend compatibility tests.
- [x] Normalize admin knowledge list/create/update/delete success and user-error response envelopes with handler and Knowledge frontend compatibility tests.
- [x] Normalize admin system audit-log response envelope with handler and System frontend compatibility tests.
- [x] Normalize admin system config CRUD success response envelopes with handler, sensitive-field, and System frontend compatibility tests.
- [x] Normalize admin load balancer CRUD and health-check success response envelopes with handler and System frontend compatibility tests.
- [ ] Keep frontend build/test/audit green for admin and user workflows.
- [x] Track frontend bundle size for heavy admin pages.
