# Test And CI Gap Register

## 2026-07-08 CI Baseline

New CI coverage added:

- `go mod tidy` cleanliness check.
- `gofmt` check for tracked Go files.
- `go vet ./...`.
- Full `go test ./... -count=1 -p=1`.
- Full `go test -race ./... -count=1 -p=1`.
- Benchmark smoke test for `internal/service`, `internal/handler`, and `internal/router`.
- Blocking `govulncheck ./...`.
- Blocking production `gosec` gate for `cmd/server`, generated gRPC package with generated files excluded, and non-test `internal/...` packages.
- Blocking generated-file-excluded full-repository `gosec` gate with JSON report artifact upload.
- Blocking `golangci-lint` gate with report artifact upload.
- `gosec` report artifact upload for the blocking full-repository generated-file-excluded scan.
- Docker build smoke test.
- Release SBOM generation for source dependencies.

Local verification status:

- `govulncheck ./...` passed after dependency upgrades, with zero reachable vulnerabilities.
- `gosec -exclude-generated ./...` currently reports 0 findings.
- Raw `gosec ./...` still reports 4 generated protobuf `G103` findings, which are excluded from CI gosec reports and production gates.
- `golangci-lint run --timeout=8m` currently reports zero findings.
- `golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0` currently reports zero findings.
- `gofmt` has been applied to tracked Go files so the new CI gofmt gate has a clean baseline.
- Service benchmark smoke includes realistic hourly traffic and user-ranking queries over 250 active users, 750 zero-traffic users, and 168 hours of traffic logs.
- Service benchmark smoke includes forwarding runtime job listing filters and clean-agent heartbeat claiming through `BenchmarkForwardRuntimeJobListing` and `BenchmarkForwardRuntimeJobCleanAgentClaiming`.
- Service tests now use a per-process temporary SQLite database directory instead of a fixed `/tmp` path.
- gRPC bidirectional stream tests cover cancellation for status, user change, config change, traffic, and online streams.
- Background worker tests cover shared delayed-cycle cancellation, Gost stats worker idle-loop cancellation, and runtime executor cancellation draining a claimed job into a terminal failed state.
- WebSocket tests cover shared Origin allowlist matching, user subscription/admin monitor/agent upgrader Origin behavior, and agent read-deadline enforcement under the race detector.
- Frontend CI now generates and uploads a bundle size report after the Vite build, covering total JS/CSS output and heavy admin/runtime chunks.
- Frontend local verification passed for `npm test`, `npm audit --audit-level=moderate`, `npm run build -- --outDir ./public-check`, and bundle reporting against that clean output directory.
- Ansible Machines handler tests now assert panel `code/msg/ts/data` envelopes for list/create/sync success paths and invalid status/body/not-found error paths.
- Forward Node management handler tests now assert panel `code/msg/ts/data` envelopes for invalid ID, missing body, not-found, scope rejection, toggle, and sync-stats paths, with frontend API/Forward tests covering caller compatibility.
- Forward Rule management handler tests now assert panel `code/msg/ts/data` envelopes for list/create/get/update/delete/toggle success paths and missing body, invalid ID, and not-found error paths, with frontend API/Forward tests covering caller compatibility.
- Forward stats, user-rule, and connection-test handler tests now assert panel `code/msg/ts/data` envelopes, including invalid JSON/service-error paths and `data.success=false` connection diagnostics, with frontend API/Forward tests covering caller compatibility.
- Forward observability handler tests now assert panel `code/msg/ts/data` envelopes for targets, trend, topology, and multi-ingress responses, with frontend admin API mapping tests covering all observability calls.
- Forward internal traffic report/snapshot handler tests now assert panel `code/msg/ts/data` envelopes for success, binding-error, and service-error responses; the Flux-compatible upload endpoint remains covered as a plain `ok` compatibility path.
- Admin traffic hourly/user-ranking handler tests now assert panel `code/msg/ts/data` envelopes, and TrafficHourly frontend tests cover both legacy and enveloped payloads.
- Admin dashboard handler tests now assert panel `code/msg/ts/data` envelopes, and Dashboard frontend tests cover both legacy and enveloped payloads.
- Admin user stats handler tests now assert panel `code/msg/ts/data` envelopes, and Users frontend tests cover both legacy and enveloped payloads.
- Admin order stats handler tests now assert panel `code/msg/ts/data` envelopes, and Orders frontend tests cover both legacy and enveloped payloads.
- Admin node stats handler tests now assert panel `code/msg/ts/data` envelopes, and Nodes frontend tests cover both legacy and enveloped payloads.
- Admin system info handler tests now assert panel `code/msg/ts/data` envelopes, and AdminLayout frontend tests cover both legacy and enveloped payloads.
- Admin invite stats handler tests now assert panel `code/msg/ts/data` envelopes, and Invite frontend tests cover both legacy and enveloped payloads.
- Admin invite config handler tests now assert panel `code/msg/ts/data` envelopes, and Invite frontend tests cover both legacy and enveloped payloads.
- Admin payment stats handler tests now assert panel `code/msg/ts/data` envelopes, and Payment frontend tests cover both legacy and enveloped payloads.
- Admin payment gateway list handler tests now assert panel `code/msg/ts/data` envelopes, and Payment frontend tests cover both legacy and enveloped payloads.
- Admin payment gateway CRUD/toggle and payment-record list handler tests now assert panel `code/msg/ts/data` envelopes, and Payment frontend tests cover legacy and enveloped record-list payloads.
- User payment channels/create/status/records handler tests now assert panel `code/msg/ts/data` envelopes while payment callbacks remain covered as plain-text compatibility responses.
- Admin subscription stats handler tests now assert panel `code/msg/ts/data` envelopes, and Subscriptions frontend tests cover both legacy and enveloped payloads.
- Admin system backup stats handler tests now assert panel `code/msg/ts/data` envelopes, and System backup frontend tests cover both legacy and enveloped payloads.
- Admin load balancer stats handler tests now assert panel `code/msg/ts/data` envelopes, and admin API mapping tests cover the stats route.
- Admin system backup config handler tests now assert panel `code/msg/ts/data` envelopes, sensitive-field tests preserve S3 credential masking, and System backup frontend tests cover both legacy and enveloped payloads.
- Admin system backup list/create/delete/restore handler tests now assert panel `code/msg/ts/data` envelopes, audit tests cover backup record mutations, and System backup frontend tests cover legacy and enveloped list payloads.
- Admin subscription settings handler tests now assert panel `code/msg/ts/data` envelopes, with System and Users frontend tests plus admin API mapping coverage for legacy and enveloped payloads.
- User registration and order-save handler tests now assert panel `code/msg/ts/data` envelopes while checking the preserved token and order payload fields under `data`.
- User login handler tests now assert panel `code/msg/ts/data` envelopes, and Login frontend tests cover the enveloped token payload.
- User order list/detail handler tests now assert panel `code/msg/ts/data` envelopes, and user Orders frontend tests cover legacy and enveloped payloads.
- User subscription info handler tests now assert panel `code/msg/ts/data` envelopes, and user Subscribe frontend tests cover legacy and enveloped payloads.
- User profile handler tests now assert panel `code/msg/ts/data` envelopes, and user store tests cover the enveloped profile payload.
- User dashboard handler tests now assert panel `code/msg/ts/data` envelopes, with user API mapping coverage for the route.
- User plan list handler tests now assert panel `code/msg/ts/data` envelopes, and user Plans frontend tests cover legacy and enveloped payloads.
- User coupon-check handler tests now assert panel `code/msg/ts/data` envelopes, and user Plans frontend tests cover legacy and enveloped coupon payloads.
- User knowledge list/detail handler tests now assert panel `code/msg/ts/data` envelopes, and user Knowledge frontend tests cover legacy and enveloped article-list payloads.
- User ticket list/create/detail/reply/close handler tests now assert panel `code/msg/ts/data` envelopes, and user Tickets frontend tests cover legacy and enveloped ticket payloads.
- Public payment methods/status handler tests now assert panel `code/msg/ts/data` envelopes and key payment payload fields.
- Admin ticket list/reply/close handler tests now assert panel `code/msg/ts/data` envelopes and ticket status side effects, and admin Tickets frontend tests cover legacy and enveloped payloads.
- Admin coupon list/create/delete handler tests now assert panel `code/msg/ts/data` envelopes and delete side effects, and admin Coupons frontend tests cover legacy and enveloped payloads.
- Admin knowledge list/create/update/delete handler tests now assert panel `code/msg/ts/data` envelopes, partial-update field preservation, and delete side effects; admin Knowledge frontend tests cover legacy and enveloped payloads plus restored locale text.
- Admin system audit-log handler tests now assert panel `code/msg/ts/data` envelopes and sensitive-content redaction, and System audit frontend tests cover legacy and enveloped payloads.
- Admin system config CRUD handler tests now assert panel `code/msg/ts/data` envelopes, sensitive-field tests preserve secret masking/audit redaction, and System runtime/config frontend tests cover enveloped payloads.
- Admin load balancer CRUD and health-check handler tests now assert panel `code/msg/ts/data` envelopes, and System load balancer frontend tests cover legacy and enveloped payloads.
- Production runtime `gosec` scanning is clean and now blocking in CI for `cmd/server`, generated gRPC package with generated files excluded, and non-test `internal/...` packages.
- There are no remaining full-repository `G104` findings.

Known gaps:

- `golangci-lint` is now a blocking CI gate; keep the zero-finding baseline clean.
- `.golangci.yml` now excludes `web/node_modules`, so frontend dependency source is no longer treated as first-party Go code.
- Full-repository generated-file-excluded `gosec` is now a blocking CI gate. Raw generated protobuf `G103` findings remain excluded from gosec gates.
- Full race testing is now configured in CI, but long-running behavior should be watched on GitHub Actions before making it a release blocker for every branch protection profile.
- Frontend local `npm run build` may fail on this host while `web/public` or `web/public/assets` is owned by root. CI runs from a clean checkout and is not affected.
- Forwarding design/API/security/compatibility docs now exist under `docs/forwarding/`, but the remaining forwarding test gaps are broader permission and quota handler tests, admin audit tests, and end-to-end runtime smoke evidence.
