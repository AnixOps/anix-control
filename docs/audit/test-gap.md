# Test And CI Gap Register

## 2026-07-08 CI Baseline

New CI coverage added:

- Go setup is pinned to `1.26.5` in CI workflows so `govulncheck` runs against the fixed stdlib baseline used for Actions release artifacts.
- GitHub Actions dependencies are refreshed to current major versions so CI and release jobs do not depend on deprecated Node.js 20 action runtimes.
- Integration workflow unit tests now generate `coverage.out` before artifact upload.
- Local build artifact cleanup has a script self-test in CI covering removal of known Go, frontend build, frontend coverage, bundle-report, and release staging outputs while preserving config, database, and `web/node_modules` content.
- Release-build policy checks now fail deployment shell scripts that add local Go/frontend build commands without the GitHub Actions-only policy and `ALLOW_LOCAL_BUILD` guard.
- Documentation sync checks now fail implementation, frontend, deployment, workflow, or config diffs that omit maintained status documentation, with a script self-test covering source-only failure, source-plus-changelog success, and docs-only success.
- Release workflow policy checks now fail workflow changes that remove strict release tag gating, release quality/security/race/test prerequisites, required OS/arch release binary targets, frontend archives, Docker metadata, checksums, SBOM, operator deployment/upgrade runbooks, deterministic `RELEASE_NOTES.md`, or generated GitHub release notes; script self-tests cover a complete fixture plus missing-platform, missing-checksum, missing-SBOM, missing-upgrade-runbook, and missing-release-notes failures.
- Migration dry-run output is now uploaded as `migration-dry-run-report`, downloaded into tag release assets as `migration-dry-run.txt`, and covered by `SHA256SUMS.txt`; the release workflow policy guard now fails if that migration evidence is removed.
- Tag releases now include `RELEASE_MANIFEST.json` with tag, commit, GitHub Actions run metadata, a `build_source=github-actions` marker, manual deployment flag, artifact sizes, and artifact SHA-256 hashes; the release workflow policy guard self-test now fails if that manifest is removed.
- Release manifest generation now runs through `config/scripts/generate_release_manifest.py`, and CI runs its self-test to verify deterministic artifact ordering, SHA-256 hashing, manifest/checksum exclusion, run metadata, and newline-terminated JSON output.
- Release notes generation now runs through `config/scripts/generate_release_notes.py`, and CI runs its self-test to verify `CHANGELOG.md` section extraction, deterministic metadata, local-build warning text, and empty/missing section failures.
- Release artifact verification now runs through `config/scripts/verify_release_artifacts.py`; CI self-tests cover valid artifacts, tampered files, missing checksum entries, and missing required assets, and tag release jobs verify artifacts before publishing.
- Local build artifact cleanup now has an opt-in deploy-backup archive mode that removes stale ignored frontend/internal zip or tarball leftovers while preserving database backups, config, certificates, and `web/node_modules`; CI self-tests cover both default and opt-in behavior.
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
- Focused gRPC NodeLog dynamic-message, handler/client, persistence, context, and graceful-shutdown tests keep the gRPC package coverage gate above 80%.

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
- Admin traffic E2E tests now assert the enveloped hourly and user-ranking payload shape under `data.list`.
- Admin dashboard handler tests now assert panel `code/msg/ts/data` envelopes for success and database-error paths, service tests propagate stats query failures, and Dashboard frontend tests cover both legacy and enveloped payloads.
- Admin user stats handler tests now assert panel `code/msg/ts/data` envelopes for success and database-error paths, service tests propagate stats query failures, and Users frontend tests cover both legacy and enveloped payloads.
- Admin order stats handler tests now assert panel `code/msg/ts/data` envelopes for success and database-error paths, service tests propagate stats query failures, and Orders frontend tests cover both legacy and enveloped payloads.
- Async notification service coverage now waits for final send status before asserting copied user IDs, reducing CI timing sensitivity.
- Admin node stats handler tests now assert panel `code/msg/ts/data` envelopes, and Nodes frontend tests cover both legacy and enveloped payloads.
- Admin system info handler tests now assert panel `code/msg/ts/data` envelopes, and AdminLayout frontend tests cover both legacy and enveloped payloads.
- Admin invite stats handler tests now assert panel `code/msg/ts/data` envelopes, and Invite frontend tests cover both legacy and enveloped payloads.
- Admin invite config handler tests now assert panel `code/msg/ts/data` envelopes, and Invite frontend tests cover both legacy and enveloped payloads.
- User invite info/code/commission/withdrawal handler tests and admin invite config update/withdrawal handler tests now assert panel `code/msg/ts/data` envelopes; Invite frontend tests cover legacy, enveloped, and nested withdrawal/config/stats payloads with admin API mapping coverage for invite config and withdrawals.
- Admin payment stats handler tests now assert panel `code/msg/ts/data` envelopes, and Payment frontend tests cover both legacy and enveloped payloads.
- Admin payment gateway list handler tests now assert panel `code/msg/ts/data` envelopes, and Payment frontend tests cover both legacy and enveloped payloads.
- Admin payment gateway CRUD/toggle and payment-record list handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, including invalid IDs, invalid toggle values, missing/not-found gateways, and invalid stats dates. Payment frontend tests cover legacy/enveloped list payloads and `code=-1` mutation errors.
- User payment channels/create/status/records handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, including invalid, disabled, and unsupported historical gateway payment creation, while payment callbacks remain covered as plain-text compatibility responses.
- Payment gateway service tests now reject enabling Alipay, WeChat, and USDT gateways until live callback or confirmation implementations exist, and verify historical enabled rows are not returned as user payment channels.
- Legacy X402 and fiat payment create/check handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, missing order, non-pending order, missing payment, and unsupported provider paths while X402, Stripe, and PayPal callbacks/webhooks remain covered as compatibility responses.
- User/admin MFA handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, missing field, missing user, invalid code, wrong password, and backup-code regeneration errors; Admin MFA frontend tests cover legacy, enveloped, and error config payloads.
- User login handler tests now cover enabled-MFA challenge, successful TOTP challenge completion, invalid MFA attempts, login-attempt recording, and no-token challenge responses; Login frontend tests cover the two-step MFA challenge and missing-code validation.
- User/admin notification handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, and Admin Notifications frontend tests cover legacy, enveloped, and `code=-1` templates, logs, email config, and mutation payloads.
- Admin/user Telegram panel API handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, Admin Telegram frontend tests cover legacy, enveloped, and `code=-1` bot/user/broadcast payloads, and the public Telegram webhook remains covered as a `status=ok` compatibility path.
- Admin subscription group CRUD handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, service tests cover delete-not-found behavior, and Subscriptions frontend tests cover legacy, enveloped, and `code=-1` group payloads. Admin subscription template CRUD handler tests now assert panel envelopes for success and user-error paths, service tests cover template update/delete not-found behavior, and Subscriptions frontend tests cover legacy, enveloped, and `code=-1` template payloads. Admin subscription protocol binding/read handler tests now assert panel envelopes for success and user-error paths, service tests cover missing group/protocol behavior, and Subscriptions frontend tests cover legacy, enveloped, and `code=-1` protocol payloads. Admin subscription preview handler tests now assert panel envelopes, `group_ids` alias handling, and authenticated context fallback, and Subscriptions frontend tests cover preview `code=-1`. Admin subscription user/plan group binding handler tests now assert panel envelopes for success, invalid body, missing user/plan/group, and missing relation paths, while service tests cover missing references and missing relations; format success payloads remain covered with admin API mapping coverage.
- Admin node management handler tests now assert panel `code/msg/ts/data` envelopes for CRUD, credentials, logs, protocols, raw-config validation/update, and auth-key operations; Nodes frontend tests cover legacy, enveloped, and nested payloads, with admin API mapping coverage for node management calls.
- Admin Agent handler tests now assert panel `code/msg/ts/data` envelopes for list, task-result, task-history, monitor read, task creation, and execute-command responses; success tests cover WebSocket ack dispatch, Agent frontend tests cover legacy, enveloped, and nested list/history/execute payloads, with admin API mapping coverage for Agent read and command routes.
- Admin user management handler tests now assert panel `code/msg/ts/data` envelopes for create/list/get/update/delete, ban/unban, traffic reset, and subscribe reset success and user-error paths; service tests cover missing user update/delete/ban/unban/reset behavior so missing user mutations cannot silently succeed, and Users frontend tests cover legacy, enveloped, nested, and `code=-1` mutation payloads, with admin API mapping coverage for user management calls.
- Admin plan management handler tests now assert panel `code/msg/ts/data` envelopes for list/detail/create/update/delete/assign success and user-error operations; service tests cover missing plan/user update/delete/assign behavior so missing plan updates cannot be inserted, and Plans frontend tests cover legacy, enveloped, nested, and `code=-1` payloads, with admin API mapping coverage for plan management calls.
- Admin order management handler tests now assert panel `code/msg/ts/data` envelopes for list/detail/status/paid/cancel success and user-error operations; service tests cover missing order update/cancel/complete and missing related plan behavior so missing order status updates cannot silently succeed, and Orders frontend tests cover legacy, enveloped, nested, localized, and `code=-1` payloads, with admin API mapping coverage for order management calls.
- Admin subscription stats handler tests now assert panel `code/msg/ts/data` envelopes, and Subscriptions frontend tests cover both legacy and enveloped payloads.
- Admin system backup stats handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, and System backup frontend tests cover legacy, enveloped, and `code=-1` payloads.
- Admin load balancer stats handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, and admin API mapping tests cover the stats route.
- Admin system backup config handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, sensitive-field tests preserve S3 credential masking, and System backup frontend tests cover legacy, enveloped, and `code=-1` payloads.
- Admin system backup list/create/delete/restore handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, audit tests cover backup record mutations, and System backup frontend tests cover legacy, enveloped, and `code=-1` payloads.
- Admin subscription settings handler tests now assert panel `code/msg/ts/data` envelopes, with System and Users frontend tests plus admin API mapping coverage for legacy and enveloped payloads.
- User registration and order-save handler tests now assert panel `code/msg/ts/data` envelopes while checking the preserved token/order payload fields under `data` plus registration invalid-body, disabled, invite-required, invalid-email, short-password, order invalid-body, and missing-plan errors.
- User login handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, wrong-password, and rate-limit `Retry-After` paths, and Login frontend tests cover enveloped token and error payloads.
- Auth E2E tests now assert login/register business errors as HTTP 200 panel `code=-1` envelopes while keeping JWT middleware failures as HTTP 401.
- User order list/detail handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid ID, missing order, and cross-user hidden order paths; user Orders frontend tests cover legacy and enveloped payloads.
- User subscription info handler tests now assert panel `code/msg/ts/data` envelopes for success, missing user context, invalid user ID, and missing user paths; user Subscribe frontend tests cover legacy and enveloped payloads.
- User profile handler tests now assert panel `code/msg/ts/data` envelopes for success, missing user context, invalid user ID, and missing user paths; user store tests cover the enveloped profile payload.
- User dashboard handler tests now assert panel `code/msg/ts/data` envelopes for success, missing user context, invalid user ID, and missing user paths; user API mapping coverage remains for the route.
- User plan list handler tests now assert panel `code/msg/ts/data` envelopes for success and database-error paths, and user Plans frontend tests cover legacy, enveloped, and `code=-1` payloads.
- User coupon-check handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, invalid code, not-started, expired, and usage-limit paths; user Plans frontend tests cover legacy and enveloped coupon payloads.
- User knowledge list/detail handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid ID, missing article, and hidden-article paths; user Knowledge frontend tests cover legacy and enveloped article-list payloads.
- User ticket list/create/detail/reply/close handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, invalid ID, missing ticket, and closed-ticket reply paths; user Tickets frontend tests cover legacy and enveloped ticket payloads.
- Public payment methods/status handler tests now assert panel `code/msg/ts/data` envelopes, key payment payload fields, and missing payment record errors.
- Admin ticket list/reply/close handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, invalid ID, missing ticket, and ticket status side effects; admin Tickets frontend tests cover legacy and enveloped payloads.
- Admin coupon list/create/delete handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, duplicate code, invalid ID, and missing coupon paths, plus delete side effects; admin Coupons frontend tests cover legacy and enveloped payloads.
- Admin knowledge list/create/update/delete handler tests now assert panel `code/msg/ts/data` envelopes for success, invalid body, invalid ID, and missing article paths, plus partial-update field preservation and delete side effects; admin Knowledge frontend tests cover legacy and enveloped payloads plus restored locale text.
- Admin system audit-log handler tests now assert panel `code/msg/ts/data` envelopes for success and database-error paths plus sensitive-content redaction, and System audit frontend tests cover legacy, enveloped, and `code=-1` payloads.
- Admin system config CRUD handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, sensitive-field tests preserve secret masking/audit redaction, and System runtime/config frontend tests cover legacy, enveloped, and `code=-1` payloads.
- Admin load balancer CRUD and health-check handler tests now assert panel `code/msg/ts/data` envelopes for success and user-error paths, and System load balancer frontend tests cover legacy, enveloped, and `code=-1` payloads.
- Production runtime `gosec` scanning is clean and now blocking in CI for `cmd/server`, generated gRPC package with generated files excluded, and non-test `internal/...` packages.
- There are no remaining full-repository `G104` findings.

Known gaps:

- `golangci-lint` is now a blocking CI gate; keep the zero-finding baseline clean.
- `.golangci.yml` now excludes `web/node_modules`, so frontend dependency source is no longer treated as first-party Go code.
- Full-repository generated-file-excluded `gosec` is now a blocking CI gate. Raw generated protobuf `G103` findings remain excluded from gosec gates.
- Full race testing is now configured in CI, but long-running behavior should be watched on GitHub Actions before making it a release blocker for every branch protection profile.
- Frontend build outputs should not be kept in the source tree. Use `config/deploy/clean_local_build_artifacts.sh --dry-run` and then `config/deploy/clean_local_build_artifacts.sh` to remove stale `web/public`, `web/public-check`, `web/coverage`, and bundle-report directories. Use `--include-deploy-backups` only after reviewing the dry-run output for ignored local deploy archive leftovers. Root-owned remnants require the exact elevated cleanup command printed by the script. CI runs from a clean checkout and is not affected by local leftovers.
- Forwarding design/API/security/compatibility docs now exist under `docs/forwarding/`, but the remaining forwarding test gaps are broader permission and quota handler tests, admin audit tests, and end-to-end runtime smoke evidence.
