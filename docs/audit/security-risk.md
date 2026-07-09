# Security Risk Register

## 2026-07-08 Dependency Vulnerability Scan

Command:

```bash
PATH=/usr/local/go/bin:/tmp/v2board-vuln-tools:$PATH govulncheck ./...
```

Initial result:

- `GO-2026-5676`: `github.com/quic-go/quic-go@v0.59.0`, fixed in `v0.59.1`.
- `GO-2026-5004`: `github.com/jackc/pgx/v5@v5.5.5`, fixed in `v5.9.2`.
- `GO-2026-4762`: `google.golang.org/grpc@v1.79.2`, fixed in `v1.79.3`.

Remediation:

- Upgraded `github.com/quic-go/quic-go` to `v0.59.1`.
- Upgraded `github.com/jackc/pgx/v5` to `v5.9.2`.
- Upgraded `google.golang.org/grpc` to `v1.79.3`.

Verification:

```bash
PATH=/usr/local/go/bin:/tmp/v2board-vuln-tools:$PATH govulncheck ./...
```

Result after remediation:

- No reachable vulnerabilities found.
- The scan still reports vulnerabilities in imported packages and required modules that are not currently reached by project code. Keep `govulncheck` in CI to detect any future reachable path.

## 2026-07-08 Gosec Baseline

Command:

```bash
PATH=/usr/local/go/bin:/tmp/v2board-go-tools:$PATH gosec -exclude-generated -quiet ./...
```

Current result:

- `gosec -exclude-generated ./...` reports zero findings.
- Raw `gosec ./...` still reports generated protobuf unsafe blocks (`G103`) in `api/grpc/v2boardpb/v2board.pb.go`.

Current policy:

- `govulncheck` is a blocking CI gate.
- Production runtime `gosec` is a blocking CI gate for `cmd/server`, generated gRPC package with generated files excluded, and non-test `internal/...` packages.
- Full-repository `gosec -exclude-generated ./...` is a blocking CI gate and uploads a JSON report artifact.

Next actions:

- Watch generated-file-excluded full-repository gosec runtime in CI and keep branch-protection policy aligned with the blocking gate.
- Keep generated protobuf `G103` excluded from production and full-repository gosec gates.

### 2026-07-08 G115 Node Cache Key Remediation

Finding:

- `internal/service/node_service.go` used `string(rune(id))` when clearing `CacheKeyNode`, which could delete `node:{` for ID `123` instead of `node:123`.

Impact:

- Node updates could leave stale per-node cache entries behind.
- The same conversion was also flagged by `gosec` as `G115`.

Remediation:

- Added `nodeCacheKey(id uint)` using `strconv.FormatUint(uint64(id), 10)`.
- `UpdateNode` now clears the decimal `node:<id>` key.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestNodeService/TestUpdateNode|TestNodeService/TestUpdateNodeClearsNodeCacheKey|TestNodeService/TestUpdateNode_Rejects' -count=1
```

### 2026-07-08 Bootstrap Admin Password Remediation

Finding:

- `internal/service/init_admin.go` used a fixed `"password"` bootstrap password when `admin.password` was empty.
- The existing password generator used `math/rand`, which was reported by `gosec` as `G404`.

Impact:

- A deployment with an empty `admin.password` could start with a known administrator password.
- Predictable random generation is not acceptable for credentials.

Remediation:

- Empty `admin.password` now generates a 32-character bootstrap password from `crypto/rand`.
- The generated password is printed once during bootstrap, matching the existing operational flow of logging initial credentials.
- Database migration and admin existence checks now handle errors explicitly.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestInitService/TestGenerateRandomPassword|TestInitService/TestInitAdmin_DefaultCredentials|TestInitService/TestInitAdmin_NoAdminExists|TestInitService/TestInitAdmin_AdminAlreadyExists' -count=1
```

### 2026-07-08 Load Balancer Random Selection Remediation

Finding:

- `internal/service/loadbalancer_service.go` used `math/rand.Intn` for random and weighted node selection.
- `gosec` reported both call sites as `G404`.

Impact:

- Load balancing randomness is not a credential by itself, but weak randomness in traffic distribution can make selection behavior easier to predict.
- The random source also had no error path, so entropy failures could not be surfaced.

Remediation:

- Replaced `math/rand` with `crypto/rand`.
- Random source failures now propagate through `SelectNode`.
- Added tests for random and weighted strategies when entropy generation fails.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestLoadBalancerService/TestSelectNode_(RandomStrategy|WeightStrategy|RandomStrategyReturnsRandomSourceError|WeightStrategyReturnsRandomSourceError)' -count=1
PATH=/usr/local/go/bin:/tmp/v2board-go-tools:$PATH gosec -quiet ./internal/service
```

### 2026-07-08 Forward Node Error Handling Remediation

Finding:

- `internal/service/forward_node_service.go` ignored errors from health-check connection close and tag JSON parsing.
- Forward node random and weighted selection used a single random byte with modulo arithmetic.
- Forward node API token generation did not return entropy-source failures to callers.

Impact:

- Health check persistence or socket cleanup failures could be hidden from handlers and background callers.
- Invalid tag JSON could silently collapse to an empty tag list.
- Random node selection was biased when the candidate range exceeded one byte or did not divide 256 evenly.
- API token generation failures could not be reported by the admin node creation path.

Remediation:

- Forward node selection now uses `crypto/rand.Int` through the shared secure random helper.
- API token generation returns `(string, error)`, and the admin create-node handler returns HTTP 500 if entropy fails.
- Health checks now use `DialContext`, respect canceled contexts, check close/update errors, and aggregate fan-out errors.
- Added `ParseTagsWithError` while keeping `ParseTags` as a safe compatibility wrapper.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestForwardNodeService/(TestGenerateAPIToken|TestParseTags|TestParseTagsWithErrorRejectsInvalidJSON|TestSetTags|TestSelectBestNode_(RandomMode|RandomModeReturnsEntropyError|WeightMode|WeightModeReturnsEntropyError|RoundRobinMode)|TestRandBytes|TestHealthCheckAll|TestHealthCheck_NodeNotFound|TestHealthCheckReturnsCanceledContext)' -count=1
PATH=/usr/local/go/bin:/tmp/v2board-go-tools:$PATH gosec -quiet ./internal/service
```

### 2026-07-08 Notification TLS Verification Remediation

Finding:

- `internal/service/notification_service.go` used `InsecureSkipVerify: true` for SMTPS delivery.
- SMTP TLS connection, SMTP client close, message writer close, and email template rendering errors were not consistently returned.
- Notification trigger methods started goroutines with ignored `Send` errors and passed user ID pointers from caller/range state into asynchronous work.

Impact:

- SMTP delivery could be vulnerable to certificate spoofing or man-in-the-middle attacks.
- Delivery/template failures could be hidden from callers or logs.
- Background notification work could observe a mutated user ID pointer or fail silently.

Remediation:

- SMTP TLS now uses certificate verification with `ServerName` and TLS 1.2 minimum.
- TLS/SMTP close and template rendering errors are returned.
- Background notification dispatch now copies the user ID before starting a goroutine and logs send failures through structured logging.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestNotificationService/(TestSend|TestGetLogs|TestSendAsyncCopiesUserID|TestSendEmail|TestSMTPTLSConfigVerifiesCertificates|TestGetEmailTemplateEscapesContent|TestNotifyUserExpire|TestNotifyTrafficLow|TestNotifyTicketReply|TestNotifyOrderPaid|TestNotifyNodeOffline|TestBroadcast)' -count=1
PATH=/usr/local/go/bin:$PATH go test -race ./internal/service -run 'TestNotificationService/(TestSendAsyncCopiesUserID|TestNotifyUserExpire|TestNotifyTrafficLow|TestNotifyTicketReply|TestNotifyOrderPaid|TestNotifyNodeOffline|TestBroadcast)' -count=1
PATH=/usr/local/go/bin:/tmp/v2board-go-tools:$PATH gosec -quiet ./internal/service
PATH=/usr/local/go/bin:/tmp/v2board-lint-tools:$PATH golangci-lint run --timeout=8m ./internal/service
```

### 2026-07-08 Forwarding Security Documentation Baseline

Finding:

- The forwarding subsystem spans panel APIs, runtime jobs, NodeX/gost, local
  Ansible execution, clean-agent execution, and traffic ingestion, but the
  security boundaries were previously spread across operational notes.

Impact:

- Operators and contributors could confuse saved panel state with relay
  attachment, treat coarse TCP node health as runtime readiness, or miss token,
  quota, SSRF, runtime command, and audit-log risks when changing forwarding
  code.

Remediation:

- Added `docs/forwarding/security.md` to centralize forwarding trust boundaries,
  authentication controls, secret handling, local command execution constraints,
  SSRF and quota risks, clean-agent controls, and test expectations.
- Added `docs/forwarding/design.md`, `docs/forwarding/api.md`, and
  `docs/forwarding/compatibility.md` so security requirements are tied to the
  real current API and runtime architecture.

Remaining gaps:

- This is a documentation baseline, not completion of every security control.
  Broader permission tests, admin audit logging, runtime smoke evidence, and
  speed-limit runtime propagation remain open.

### 2026-07-08 SQLite to PostgreSQL Command Lint Remediation

Finding:

- `cmd/sqlite2postgres/main.go` ignored SQLite/PostgreSQL database close errors and `sql.Rows.Close()` errors.
- Full-repository `golangci-lint` also scanned `web/node_modules`, causing third-party frontend dependency Go files to appear as first-party lint findings.

Impact:

- Migration command cleanup failures could be hidden during operator-led SQLite to PostgreSQL imports.
- Row iterator close failures could be lost after successful iteration, making database driver cleanup issues invisible.
- Third-party dependency lint noise made the Go lint report less actionable.

Remediation:

- Added observable logging for source and target database close failures.
- Joined `sql.Rows.Close()` failures into `postgresBoolColumns` and `copyTable` return errors.
- Added table-copy regression coverage for `copyTable`.
- Added `.golangci.yml` to exclude `web/node_modules` from Go lint scans.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/sqlite2postgres -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./cmd/sqlite2postgres
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint config verify
```

Result:

- `cmd/sqlite2postgres` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output now reports 98 visible findings; uncapped output reports 541 findings.

### 2026-07-08 Integration Test Command Lint Remediation

Finding:

- `cmd/integration-test/main_test.go` ignored stdout pipe close and copy errors while testing JSON output.
- `cmd/integration-test/main.go` had a simple staticcheck branch-shape finding in client generator selection.

Impact:

- JSON output tests could hide pipe/copy failures.
- The command package contributed low-signal lint noise to the full-repository baseline.

Remediation:

- Added a shared `capturePrintJSON` test helper that checks pipe creation, close, and copy errors.
- Replaced the client selection `if` chain with a tagged switch.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/integration-test -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./cmd/integration-test
```

Result:

- `cmd/integration-test` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 98 to 97 visible findings; uncapped output is down from 541 to 534 findings.

### 2026-07-08 Report Command Lint Remediation

Finding:

- `cmd/report/report_test.go` ignored listener close errors.
- `TestEchoServerStartStop` made an unused request to a fixed port instead of validating the echo server's actual assigned port.

Impact:

- Report command tests could hide listener cleanup failures.
- The echo server lifecycle test was weaker than intended and contributed lint noise.

Remediation:

- Checked listener and response body close errors.
- Changed the echo server test to call `/ping` on the actual assigned server port and assert HTTP 200.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/report -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./cmd/report
```

Result:

- `cmd/report` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 97 to 96 visible findings; uncapped output is down from 534 to 531 findings.

### 2026-07-08 Integration Echo Server Lint Remediation

Finding:

- `internal/tests/integration/echo/server.go` ignored status response write errors and retained no-op `fmt.Sprintf` calls.
- `internal/tests/integration/echo/server_test.go` ignored response body and server shutdown cleanup errors.
- The echo stats test had an ineffectual initial assignment.

Impact:

- Integration echo helpers could hide failed writes or cleanup failures while testing proxy connectivity.
- Low-signal lint findings kept the full-repository `golangci-lint` report noisier than necessary.

Remediation:

- Logged HTTP server serve and status response write failures.
- Replaced no-op `fmt.Sprintf` calls with direct string literals.
- Checked response body closes, server shutdowns, and response body reads in echo tests.
- Removed the ineffectual stats assignment.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/echo -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./internal/tests/integration/echo
```

Result:

- `internal/tests/integration/echo` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 96 to 91 visible findings; uncapped output is down from 531 to 506 findings.

### 2026-07-08 Integration Local Environment Lint Remediation

Finding:

- `internal/tests/integration/local/environment_test.go` ignored echo server shutdown, subprocess signal/wait, and HTTP response body close errors.
- `internal/tests/integration/local/environment.go` retained unused private environment state and cleanup helper code.

Impact:

- Local integration helpers could hide subprocess cleanup failures after proxy connectivity tests.
- Unused private state made the test environment API harder to audit and kept low-signal lint findings in the full-repository report.

Remediation:

- Added test helpers that check response body closes and log subprocess signal/wait cleanup errors.
- Made `StopEchoServer` assert echo shutdown success and clear the stored echo server pointer.
- Returned HTTP response close failures from the fallback HTTP client path.
- Removed unused private environment state and helper method.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/local -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./internal/tests/integration/local
```

Result:

- `internal/tests/integration/local` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 91 to 88 visible findings; uncapped output is down from 506 to 495 findings.

### 2026-07-08 Integration Mock Server Lint Remediation

Finding:

- `internal/tests/integration/mock/server.go` ignored the free-port probe listener close error.
- `internal/tests/integration/mock/server_test.go` ignored response body close errors, JSON decode failures, server shutdown errors, and JSON fixture encoding failures.

Impact:

- Mock node-panel integration helpers could hide response cleanup or malformed response decoding failures.
- The full-repository lint report retained low-signal `errcheck` findings in an otherwise isolated test helper package.

Remediation:

- Logged free-port listener close failures in the mock server helper.
- Added shared test helpers for response body close and server shutdown checks.
- Required JSON response decoding to succeed in mock server tests.
- Made JSON fixture encoding failures explicit in the test reader helper.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/mock -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./internal/tests/integration/mock
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/mock
```

Result:

- `internal/tests/integration/mock` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output remains at 88 visible findings due to default issue caps; uncapped output is down from 495 to 478 findings.

### 2026-07-08 Integration Binary Manager Lint Remediation

Finding:

- `internal/tests/integration/binary/manager_test.go` ignored mock HTTP response writes and had an ineffectual expected-suffix assignment.
- `internal/tests/integration/binary/manager.go` used nested `if` platform selection that `staticcheck` flagged as lower-signal than a tagged switch.

Impact:

- Binary manager tests could hide mock response write failures.
- The last remaining `ineffassign` finding kept the full-repository lint baseline noisier and less actionable.

Remediation:

- Checked mock release response writes in `TestGetLatestVersion`.
- Removed the ineffectual expected-suffix assignment in cache path coverage.
- Rewrote Xray and Mihomo download filename platform selection as tagged switches.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/binary -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./internal/tests/integration/binary
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/binary
```

Result:

- `internal/tests/integration/binary` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 88 to 85 visible findings; uncapped output is down from 478 to 474 findings.
- Full-repository `ineffassign` findings are now zero.

### 2026-07-08 Integration Runner Lint Remediation

Finding:

- `internal/tests/integration/runner/runner.go` ignored `client.Stop()` failures after starting an integration client.
- `runTest` attempted to set `Duration` in a deferred function while returning an unnamed `TestResult`, so duration updates could be lost when returning.

Impact:

- Integration runner reports could hide client cleanup failures.
- Duration telemetry in runner results could be missing, weakening failure triage and benchmark-style diagnostics.

Remediation:

- Changed `runTest` to use a named return result so deferred duration and cleanup updates are preserved.
- Propagated client stop failures into the returned `TestResult`, appending to any existing error and collecting client logs.
- Extended the unsupported-protocol runner test to assert result durations are populated.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/runner -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./internal/tests/integration/runner
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/runner
```

Result:

- `internal/tests/integration/runner` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output remains at 85 visible findings due to default issue caps; uncapped output is down from 474 to 473 findings.

### 2026-07-08 Verify Command Lint Remediation

Finding:

- `cmd/verify/main.go` returned a capitalized error string for a missing local Xray binary, which `staticcheck` reported as `ST1005`.

Impact:

- The local E2E verification command still had a lint finding after earlier gosec hardening, keeping the command package from being a clean CI target.

Remediation:

- Normalized the missing-Xray error text to begin with lowercase `xray` while preserving the operator-facing recovery hint.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/verify -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./cmd/verify
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./cmd/verify
```

Result:

- `cmd/verify` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 85 to 84 visible findings; uncapped output is down from 473 to 472 findings.

### 2026-07-08 WebSocket Origin Utility Lint Remediation

Finding:

- `internal/utils/websocket_origin.go` used an `if`/`else` scheme mapping for `ws` and `wss` that `staticcheck` reported as a tagged-switch cleanup opportunity.

Impact:

- The shared WebSocket Origin allowlist helper retained a low-signal lint finding despite already having behavior coverage for same-host, wildcard, invalid, and WebSocket-scheme origins.

Remediation:

- Rewrote WebSocket scheme normalization as a tagged switch without changing allowed-origin semantics.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/utils -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./internal/utils
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/utils
```

Result:

- `internal/utils` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 84 to 83 visible findings; uncapped output is down from 472 to 471 findings.

### 2026-07-08 Database Package Lint Remediation

Finding:

- `internal/database/database_test.go` ignored database close and temporary cleanup errors in several SQLite tests.
- `internal/database/database.go` retained low-signal staticcheck findings for log-level selection and embedded Dialector method selectors.

Impact:

- Database tests could hide close failures from the underlying SQL connection.
- SQLite path tests wrote under process-global temp paths, which made cleanup more fragile and less isolated.
- The database package still contributed both `errcheck` and `staticcheck` noise to the full-repository lint report.

Remediation:

- Added a shared test helper that requires `Close()` to succeed.
- Moved SQLite path tests under `t.TempDir()` so the test framework owns cleanup.
- Checked previously unverified initialization calls in close-path tests.
- Rewrote database log-level selection as a tagged switch and used promoted `db.Name()` selectors for database-type checks.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/database -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./internal/database
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/database
```

Result:

- `internal/database` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output is down from 83 to 82 visible findings; uncapped output is down from 471 to 441 findings.

### 2026-07-08 Integration Setup Script Lint Remediation

Finding:

- `config/scripts/setup_integration.go` ignored `database.Close()` after preparing integration test seed data.

Impact:

- Integration setup could hide database close failures during local or CI environment preparation.

Remediation:

- Logged database close failures from the setup script deferred cleanup path.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./config/scripts -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./config/scripts
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./config/scripts
```

Result:

- `config/scripts` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output remains at 82 visible findings due to default issue caps; uncapped output is down from 441 to 440 findings.

### 2026-07-08 Middleware Package Lint Remediation

Finding:

- `internal/middleware/middleware_test.go` ignored node table migration and database close errors in the shared NodeAuth test setup.

Impact:

- Middleware authentication tests could hide schema setup failures or database cleanup failures.

Remediation:

- Required `AutoMigrate(&model.Node{})` to succeed during middleware test setup.
- Required database close cleanup to succeed after each middleware test using the shared setup helper.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/middleware -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/middleware
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/middleware
```

Result:

- `internal/middleware` exits with zero package-local golangci-lint findings.
- Default full-repository golangci-lint output remains at 82 visible findings due to default issue caps; uncapped output is down from 440 to 438 findings.

### 2026-07-08 Integration Clients Lint Remediation

Finding:

- `internal/tests/integration/clients/client_test.go` ignored `Manager.Add` errors in list/status/watch tests.
- `internal/tests/integration/clients/xray_mihomo.go` deferred HTTP response body closes without checking or logging close failures.

Impact:

- Integration client tests could proceed without proving manager registration succeeded.
- Proxy health/proxy API probes could hide HTTP response cleanup failures, keeping the integration lint baseline noisy.

Remediation:

- Required test client registration to succeed before list/status/watch assertions.
- Added a shared response body close helper that logs deferred probe cleanup failures without changing health-check return semantics.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/clients -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/tests/integration/clients
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/tests/integration/clients/...
```

Result:

- `internal/tests/integration/clients` exits with zero package-local golangci-lint findings.
- `internal/tests/integration/clients` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output remains at 82 visible findings due to default issue caps; uncapped output is down from 438 to 428 findings.

### 2026-07-08 Command Entrypoint Close Lint Remediation

Finding:

- `cmd/migrate/main.go` ignored target database close failures after successful migration imports.
- `cmd/server/main.go` ignored database close failures during graceful shutdown.

Impact:

- Operator-facing commands could hide database-driver cleanup or final flush failures.
- The command entrypoints kept low-risk errcheck findings in the full-repository lint report.

Remediation:

- Wrapped both database close defers with explicit error logging.
- Kept the existing fatal-exit behavior unchanged; `log.Fatal` paths already bypass deferred cleanup.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/migrate ./cmd/server -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/migrate ./cmd/server
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./cmd/migrate/... ./cmd/server/...
```

Result:

- `cmd/migrate` and `cmd/server` exit with zero package-local golangci-lint findings.
- `cmd/migrate` and `cmd/server` exit with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output remains at 82 visible findings due to default issue caps; uncapped output is down from 428 to 426 findings.

### 2026-07-08 Small Test Package Lint Remediation

Finding:

- `internal/router` tests ignored schema migration and database close failures.
- `internal/tests/smoke` ignored full schema migration failures during suite setup.
- `internal/tests/integration` ignored health-check response body close failures and external port parsing failures.

Impact:

- Router and smoke tests could continue after incomplete schema setup.
- Integration tests could hide resource cleanup failures or silently fall back to the default port after malformed `TEST_SERVER_PORT` input.

Remediation:

- Required router and smoke schema migrations to succeed before test execution continues.
- Required router database cleanup to succeed through the shared teardown helper.
- Required integration health-check response cleanup and optional external test port parsing to succeed.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/router ./internal/tests/smoke -count=1
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration -short -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/tests/smoke ./internal/router ./internal/tests/integration
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/tests/smoke/... ./internal/router/... ./internal/tests/integration
```

Result:

- `internal/router`, `internal/tests/smoke`, and `internal/tests/integration` exit with zero package-local golangci-lint findings.
- The selected packages exit with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output remains at 82 visible findings due to default issue caps; uncapped output is down from 426 to 419 findings.

### 2026-07-08 WebSocket Package Lint Remediation

Finding:

- `internal/websocket/subscription.go` retained an unused per-client mutex after write handling moved to the send channel/write pump.
- WebSocket handler tests ignored close failures for successfully dialed test clients.

Impact:

- The unused field made the client concurrency model less clear during review.
- Test WebSocket cleanup failures could be hidden after successful upgrade tests.

Remediation:

- Removed the unused client mutex field; the existing `send` channel and single write pump remain the concurrency boundary for outbound frames.
- Required test WebSocket client close operations to succeed.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/websocket -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/websocket
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/websocket/...
```

Result:

- `internal/websocket` exits with zero package-local golangci-lint findings.
- `internal/websocket` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output remains at 82 visible findings due to default issue caps; uncapped output is down from 419 to 416 findings.

### 2026-07-08 GOST Package Lint Remediation

Finding:

- `internal/gost/client.go` deferred HTTP response body closes without checking failures on API and metrics requests.
- GOST client and manager tests ignored JSON response encoding, plain response write, database close, and node registration failures.
- `internal/gost/manager.go` retained an unused mutex while node clients are stored through `sync.Map`.

Impact:

- GOST API callers could miss final response cleanup failures from HTTP bodies.
- GOST tests could hide broken mock responses or failed test database cleanup.
- The unused mutex made manager concurrency ownership less clear during review.

Remediation:

- Returned response body close failures from the GOST API and metrics read paths.
- Required mock response encoding/writes, test database cleanup, and registration setup to succeed in tests.
- Removed the unused manager mutex while keeping `sync.Map` as the client cache concurrency primitive.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/gost -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/gost
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/gost/...
```

Result:

- `internal/gost` exits with zero package-local golangci-lint findings.
- `internal/gost` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output is down from 82 to 81 visible findings; uncapped output is down from 416 to 403 findings.

### 2026-07-08 Cache Package Lint Remediation

Finding:

- `internal/cache/memory_test.go` ignored cache mutation, set/hash mutation, read, delete, and set-cardinality errors in multiple tests.
- The concurrent cache access test wrote integer values but called `GetString`, hiding the type mismatch because the return value was ignored.

Impact:

- Cache tests could pass while setup writes, reads, deletes, or set/hash mutations failed.
- Concurrent access coverage did not actually prove typed reads succeeded after writes.

Remediation:

- Added cache test helpers for required set, set-add, hash-set, and generic get operations.
- Checked set cardinality and LRU/expiration setup calls.
- Changed concurrent access coverage to read the typed value with `Get` and collect goroutine errors back in the main test.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/cache -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/cache
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/cache/...
```

Result:

- `internal/cache` exits with zero package-local golangci-lint findings.
- `internal/cache` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output remains at 81 visible findings due to default issue caps; uncapped output is down from 403 to 373 findings.

### 2026-07-08 gRPC Package Lint Remediation

Finding:

- `internal/grpc` tests ignored database initialization, schema migration, server `Serve`, client connection close, database close, and stream `CloseSend` errors.
- One config-version integration test depended on an earlier test leaving the in-memory SQLite database initialized.
- The primary gRPC test suite still used deprecated `grpc.Dial` options.

Impact:

- gRPC integration tests could pass after failed setup, failed cleanup, or failed stream shutdown.
- Test order affected whether config-version coverage had a usable database.
- Deprecated client setup kept staticcheck findings in the package-local report.

Remediation:

- Added shared gRPC test helpers for required in-memory database setup, migrations, database close, client close, stream close, and server serve/stop error checks.
- Updated gRPC suites and standalone tests to use explicit lifecycle checks.
- Made config-version integration tests initialize and migrate their own database state.
- Replaced the remaining deprecated test client setup with `grpc.NewClient`.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/grpc -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/grpc
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/grpc/...
```

Result:

- `internal/grpc` exits with zero package-local golangci-lint findings.
- `internal/grpc` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output is down from 81 to 78 visible findings; uncapped output is down from 373 to 271 findings.

### 2026-07-08 E2E Test Package Lint Remediation

Finding:

- `internal/tests/e2e` tests ignored JSON response decoding failures in auth, admin, and UniProxy flows.
- E2E suite teardown ignored database close failures.
- Subscription E2E setup ignored the subscription group protocol association append result.

Impact:

- E2E tests could continue after malformed JSON responses, failed cleanup, or incomplete subscription group setup.
- Subscription coverage could pass without proving the protocol-to-group association was established.

Remediation:

- Added shared E2E test helpers for required JSON decoding and database close checks.
- Updated auth, admin, UniProxy, and subscription E2E tests to assert setup, decode, and teardown errors.
- Checked subscription group association append during suite setup.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/e2e -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/tests/e2e
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/tests/e2e/...
```

Result:

- `internal/tests/e2e` exits with zero package-local golangci-lint findings.
- `internal/tests/e2e` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output remains at 78 visible findings due to default issue caps; uncapped output is down from 271 to 257 findings.

### 2026-07-08 Integration E2E Test Package Lint Remediation

Finding:

- `internal/tests/integration/e2e` ignored echo server shutdown, free-port listener close, port probe connection close, process interrupt/kill, and HTTP response body close failures.

Impact:

- Integration E2E cleanup and connectivity checks could pass while leaking resources or hiding failed shutdown paths.
- Concurrent and large-data E2E tests could miss response body close failures.

Remediation:

- Made echo shutdown, free-port listener close, port probe close, and process signal/kill errors observable.
- Checked response body close failures in connectivity, echo startup, concurrent request, and large-data tests.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/e2e -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/tests/integration/e2e
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/tests/integration/e2e/...
```

Result:

- `internal/tests/integration/e2e` exits with zero package-local golangci-lint findings.
- `internal/tests/integration/e2e` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output remains at 78 visible findings due to default issue caps; uncapped output is down from 257 to 248 findings.

### 2026-07-08 Handler Package Lint Remediation

Finding:

- `internal/handler` had unchecked WebSocket close, request body close, PayPal response body close, Telegram update handling, test database setup/cleanup, test JSON decoding, and test WebSocket read deadline errors.
- The package also contained a stale unused helper and two staticcheck findings in invite/payment handler logic.

Impact:

- Handler tests could pass after malformed JSON responses or failed database setup.
- Runtime cleanup and asynchronous update handling failures were not observable.
- Stale code and low-signal static findings kept the package from being eligible for a blocking lint gate.

Remediation:

- Logged runtime WebSocket, request body, response body, and Telegram update handling failures.
- Checked handler test database migration/cleanup, JSON decoding, request body close, and WebSocket read deadline errors.
- Removed the unused helper and simplified staticcheck-reported branches.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/handler -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/handler
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/handler/...
```

Result:

- `internal/handler` exits with zero package-local golangci-lint findings.
- `internal/handler` exits with zero generated-file-excluded gosec findings.
- Default full-repository golangci-lint output is down from 78 to 74 visible findings; uncapped output is down from 248 to 208 findings.

### 2026-07-08 Service Small-File Lint Remediation

Finding:

- Service tests and helpers outside the consolidated `service_test.go` suite still ignored migration, benchmark setup, zip reader close, and Telegram JSON encoding errors.
- Several service files retained stale private helpers, unused struct fields, and low-signal staticcheck findings.

Impact:

- Focused service tests could pass after failed schema setup or malformed benchmark fixtures.
- HTTP response and archive close failures could remain invisible in runtime-adjacent paths.
- Staticcheck/unused noise kept the service package from having an actionable lint baseline.

Remediation:

- Checked service test migrations, benchmark JSON parsing, benchmark parser calls, zip reader close, and Telegram test response encoding errors.
- Propagated response body close failures in NodeX, diagnostics, notification webhook, and Telegram API request paths.
- Removed stale private helpers/fields and applied staticcheck simplifications without changing public API behavior.

Verification:

```bash
rm -f /tmp/v2board_service_test.db /tmp/v2board_service_test.db-*
PATH=/usr/local/go/bin:$PATH go test ./internal/service -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/service
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/service/...
```

Result:

- `internal/service` tests pass.
- `internal/service` generated-file-excluded gosec exits with zero findings.
- Service package golangci-lint is reduced to 156 `errcheck` findings, all in `internal/service/service_test.go`.
- Default full-repository golangci-lint output is down from 74 to 40 visible findings; uncapped output is down from 208 to 156 findings.

### 2026-07-08 Service Test Suite Lint Remediation

Finding:

- The consolidated `internal/service/service_test.go` suite still ignored fixture setup errors for nodes, plans, orders, subscription groups/templates, payment records, load balancers, system config, registration, auth-key generation, and test directory cleanup.
- These findings were the final remaining full-repository `golangci-lint` baseline.

Impact:

- Service tests could continue after failed fixture creation and report misleading downstream assertions.
- The full-repository lint gate could not be made blocking while these test errors remained.

Remediation:

- Added explicit assertions for service fixture setup calls and checked test database directory cleanup in `TestMain`.
- Kept intentionally tolerated notification send failures explicit by assigning and documenting the ignored error.
- Promoted the GitHub Actions `go-lint` job from report-only to a blocking lint gate while keeping the artifact upload.

Verification:

```bash
rm -f /tmp/v2board_service_test.db /tmp/v2board_service_test.db-*
PATH=/usr/local/go/bin:$PATH go test ./internal/service -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./internal/service
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m ./...
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH golangci-lint run --timeout=8m --max-issues-per-linter=0 --max-same-issues=0 ./...
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./internal/service/...
PATH=/usr/local/go/bin:$PATH go vet ./...
PATH=/usr/local/go/bin:$PATH go test ./... -count=1 -p=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated ./...
```

Result:

- `internal/service` tests pass.
- `internal/service` package golangci-lint exits with zero findings.
- Full-repository default and uncapped golangci-lint both exit with zero findings.
- `go vet ./...` and `go test ./... -count=1 -p=1` pass.
- `internal/service` and full-repository generated-file-excluded gosec exit with zero findings.

### 2026-07-08 EPay Callback Amount Verification Remediation

Finding:

- EPay callback verification checked the signed callback parameters but did not propagate the signed `money` amount into the payment marking path.
- The callback handler could mark a payment record paid without comparing the gateway-reported amount to `v2_payment_record.actual_amount`.

Impact:

- If a valid signed callback was produced for a lower amount against the same merchant order number, the local order could be marked paid for less than the expected amount.
- Callback regression coverage did not prove the handler rejected signed amount mismatches end to end.

Remediation:

- EPay now parses the signed `money` field for paid callbacks and rejects missing or invalid amounts.
- Callback signature comparison now decodes MD5 hex signatures and uses constant-time comparison. MD5 remains a narrow EPay protocol compatibility requirement and is annotated with `#nosec` justification.
- `PaymentGatewayService.MarkOrderPaidWithAmount` compares the signed callback amount with the stored `ActualAmount` before updating payment/order state.
- Added gateway, service, and handler regression tests for valid amounts, mismatched signed amounts, missing trade numbers, invalid money, and uppercase callback signatures.
- Added a plugin registry guard test that enumerates `payment.Types()` and fails if any registered callback gateway lacks both a valid callback case and a tampered-signature rejection case.
- Stripe, PayPal, and X402 remain on the legacy `PaymentHandler` webhook paths; their dedicated signature tests are tracked separately from the plugin `/payment/callback/:type` registry.
- PayPal webhook tests now mock the official OAuth and verify-webhook-signature endpoints to cover both remote signature success and rejection without external network calls.
- Runtime callback coverage is complete for currently implemented handlers: EPay, Stripe, PayPal, and X402. Alipay, WeChat, and USDT must not be treated as callback-complete until callback implementations and tests are added.
- `PaymentGatewayService` now rejects enabling Alipay, WeChat, and USDT gateways until their live callback or confirmation implementations and tests exist. Enabled historical rows for those types are filtered from user channels and rejected during payment creation.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/payment/... -count=1
rm -f /tmp/v2board_service_test.db /tmp/v2board_service_test.db-*
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestPaymentGatewayService/TestMarkOrderPaidWithAmount' -count=1
PATH=/usr/local/go/bin:$PATH go test ./internal/handler -run 'TestPaymentGatewayCallback|TestPaymentGatewayHandler' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/payment/... ./internal/handler
```

### 2026-07-08 Server Config Error Handling Remediation

Finding:

- `internal/service/server_service.go` ignored cache operation errors while updating online user state.
- Node config generation ignored JSON parse failures for protocol settings such as `tls_settings`, `network_settings`, `encryption_settings`, and `padding_scheme`.
- Route lookup failures during node config generation were ignored.

Impact:

- Node online state updates could appear successful even if cache operations failed.
- A malformed protocol JSON field could silently produce a partial or empty node config, making node-side failures harder to diagnose.
- Nodes could receive configs without the intended route list if route loading failed.

Remediation:

- `UpdateOnlineStatus` now returns cache key listing, delete, set, and expiry errors with context.
- `BuildNodeConfig` now rejects malformed optional JSON settings with field-specific errors.
- Route loading errors now stop config generation instead of returning an incomplete config.
- Removed the debug `BuildNodeConfig` log line from the hot node polling path.

Verification:

```bash
rm -f /tmp/v2board_service_test.db /tmp/v2board_service_test.db-*
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestServerService|TestParseTrafficData|TestParseOnlineData' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/service
```

Result:

- `server_service.go` no longer appears in the service-package `gosec` report.
- Service-package `gosec` findings are down to 18, with remaining issues concentrated in `telegram_service.go`, `system_service.go`, and legacy MD5 compatibility.

### 2026-07-08 Telegram Error Handling Remediation

Finding:

- `internal/service/telegram_service.go` ignored send failures while broadcasting Telegram messages.
- Telegram API request JSON encoding errors were ignored before sending the HTTP request.
- Telegram admin ID JSON parsing errors were ignored, causing malformed `admin_ids` config to behave like an empty administrator list.

Impact:

- Broadcast jobs could report success even when every Telegram API call failed.
- Invalid request payloads could proceed to the HTTP layer with an empty or partial body.
- Malformed administrator configuration could silently deny all admin commands instead of surfacing a configuration integrity error.

Remediation:

- `Broadcast` now returns database query errors and aggregates per-recipient send errors with recipient context.
- Telegram API request encoding failures are returned before any HTTP request is attempted.
- Added `parseAdminIDsWithError` for strict internal paths while keeping `parseAdminIDs` as a compatibility wrapper.
- Message persistence and bot user-count updates now return database errors.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestTelegramBotService|TestParseAdminIDs' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/service
```

Result:

- `telegram_service.go` no longer appears in the service-package `gosec` report.
- Service-package `gosec` findings are down to 15, with remaining issues in `system_service.go` and legacy subscription MD5 compatibility.

### 2026-07-08 Backup Path And Archive Restore Remediation

Finding:

- `internal/service/system_service.go` created backup and restore directories with `0755`.
- Backup archive restore used zip entry paths and copied compressed content without explicit per-file or total uncompressed size limits.
- Several backup read/write points were reported by `gosec` as variable file paths and needed either stronger validation or narrow reviewed exceptions.

Impact:

- Local backup directories could be readable by other users on multi-user hosts.
- A crafted backup archive could attempt path traversal, restore non-regular entries, or force excessive decompression during restore.
- Security scans could not distinguish internally generated backup paths from unreviewed user-controlled file inclusion.

Remediation:

- Backup directories now use `0750`; the local backup root is chmodded to the private mode after creation.
- Archive restore now rejects path traversal, absolute paths, non-regular entries, oversized entries, and archives exceeding a total uncompressed budget.
- Extraction copies with a declared-size limit and rejects archives that emit more bytes than declared.
- Reviewed local backup source/destination file operations now use narrow `#nosec G304` annotations with inline justification.

Verification:

```bash
rm -f /tmp/v2board_service_test.db /tmp/v2board_service_test.db-*
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestBackupService|TestValidateArchiveFileForRestore' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/service
```

Result:

- `system_service.go` no longer appears in the service-package `gosec` report.
- Service-package `gosec` findings are down to 5: NodeX token wording false-positive review, forward runtime command execution review, and legacy subscription MD5 compatibility.

### 2026-07-08 Forward Runtime Command Execution Remediation

Finding:

- `internal/service/forward_runtime_job_executor.go` passed a runtime-configured command string into `exec.CommandContext`.

Impact:

- The executor does not use a shell, but allowing arbitrary command names would still let a compromised or overly broad runtime configuration execute unexpected binaries on the panel host.

Remediation:

- The OS command runner now validates the executable before spawning it.
- Only `ansible-playbook` or an absolute path whose basename is `ansible-playbook` is accepted.
- Relative paths, shell names, and commands containing control characters are rejected before `exec.CommandContext`.
- The remaining `#nosec G204` annotation is scoped to the validated call site and documents the guard.

Verification:

```bash
rm -f /tmp/v2board_service_test.db /tmp/v2board_service_test.db-*
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestPanelForwardRuntimeJobExecutor|TestResolveAnsiblePlaybookCommand' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/service
```

Result:

- `forward_runtime_job_executor.go` no longer appears in the service-package `gosec` report.
- Service-package `gosec` findings are down to 4: NodeX token wording false-positive review and legacy subscription MD5 compatibility.

### 2026-07-08 Service Gosec Completion

Finding:

- `internal/service/subscription_service.go` used MD5 both for internal subscription template node IDs and for legacy SS2022 `server_key` derivation.
- `internal/service/forward_panel_runtime_diagnostics.go` triggered `G101` on a diagnostic string that names the `forward.runtime.nodex.token` configuration key.

Impact:

- The subscription template node ID hash did not require a weak primitive and could be strengthened without changing a public contract.
- SS2022 `server_key` derivation must remain byte-for-byte compatible with the historical XBoard/V2bX algorithm or generated subscriptions and node config will diverge.
- The NodeX diagnostic string did not embed a credential, but leaving it unreviewed kept service-package security scanning noisy.

Remediation:

- Replaced subscription template node ID hashing with SHA-256 while preserving the existing 16-character digest shape.
- Added deterministic coverage for the SHA-256 node ID digest.
- Kept SS2022 MD5 derivation unchanged and marked it with scoped `#nosec` annotations that document the compatibility reason.
- Marked the NodeX diagnostic configuration-key string with a scoped `#nosec G101` annotation.

Verification:

```bash
rm -f /tmp/v2board_service_test.db /tmp/v2board_service_test.db-*
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestGenerateNodeIDUsesStableSHA256Digest|TestDeriveSS2022ServerKey|TestSubscriptionService' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/service
```

Result:

- `gosec -quiet ./internal/service` exits successfully with zero reported issues.

### 2026-07-08 Handler And WebSocket Gosec Remediation

Finding:

- `internal/websocket/subscription.go` and `internal/handler/monitor_ws.go` ignored WebSocket close, deadline, frame write, writer write, and JSON marshal errors.
- WebSocket response helpers could block when the per-client send queue was full.
- `internal/handler/uniproxy.go` used MD5 for ETag generation.
- `internal/handler/metrics.go` converted signed integers to unsigned integers when formatting metric values.

Impact:

- Broken WebSocket clients and full send queues could leave failures invisible and hold read loops longer than necessary.
- A JSON marshal failure could enqueue an empty or invalid response without an operator-visible signal.
- MD5 was unnecessary for ETags and kept handler-package security scanning noisy.
- Signed-to-unsigned conversion could render negative values incorrectly and was flagged as an overflow risk.

Remediation:

- Added checked WebSocket close, deadline, text frame, ping frame, close frame, and writer close handling.
- Centralized WebSocket message enqueueing with explicit JSON marshal error logging and non-blocking backpressure handling.
- Replaced UniProxy ETag hashing with SHA-256.
- Replaced metrics signed integer formatting with `strconv.FormatInt`.
- Added tests for WebSocket marshal failure and full-queue behavior, plus updated ETag and metrics assertions.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/websocket -count=1
PATH=/usr/local/go/bin:$PATH go test ./internal/handler -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/handler ./internal/websocket
```

Result:

- `gosec -quiet ./internal/handler ./internal/websocket` exits successfully with zero reported issues.
- Full-repository `gosec` issue count is down to 146, with no remaining `internal/handler` or `internal/websocket` findings in the refreshed JSON report.

### 2026-07-08 WebSocket Origin And Deadline Review

Finding:

- The user subscription, admin monitor, and agent WebSocket upgraders each allowed every `Origin`.
- Agent WebSocket read loops did not set read limits, read deadlines, pong handlers, or write deadlines on JSON sends.

Impact:

- Browser-based cross-site WebSocket attempts were not bounded by the HTTP CORS middleware.
- Slow, idle, or half-open agent WebSocket connections could hold a goroutine longer than intended.

Remediation:

- Added a shared WebSocket Origin policy in `internal/utils`: no-Origin clients are allowed for non-browser/node clients, same-host origins are allowed, `server.cors.allowed_origins` are allowed, and unlisted cross-site origins are rejected.
- Connected the shared policy to the user subscription, admin monitor, and agent WebSocket upgraders.
- Added agent WebSocket read limits, read deadlines, pong deadline refresh, message-read deadline refresh, and write deadlines for JSON sends.
- Documented `server.cors` in development and production config templates.
- Added tests for the shared Origin policy, each upgrader's Origin behavior, and agent read-deadline enforcement.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/utils ./internal/websocket ./internal/handler -count=1
PATH=/usr/local/go/bin:$PATH go test -race ./internal/utils ./internal/websocket ./internal/handler -run 'TestIsWebSocketOriginAllowed|TestSubscriptionWebSocketUpgraderOriginPolicy|TestMonitorWSUpgraderOriginPolicy|TestAgentWebSocketUpgraderOriginPolicy|TestPrepareAgentWebSocketSetsReadDeadline|TestHandleWebSocketMessage_RequireAckSendsAck' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/handler ./internal/websocket ./internal/utils
```

Result:

- WebSocket auth/origin/deadline/ping/pong/cleanup review item is closed for current upgrade paths.

### 2026-07-08 Production G104 Outside Service Remediation

Finding:

- Production packages outside `internal/service` still had unchecked error findings after the service-package G104 cleanup.
- The current refreshed full-repository `G104` scan reports zero findings.

Impact:

- Malformed subscription/server JSON helpers could silently return empty defaults instead of making corrupt stored configuration visible.
- Gost forward rule updates could hide relay delete failures before recreating runtime state.
- Server startup and verification command paths could ignore filesystem, response-body, process, and cleanup errors.

Remediation:

- Made subscription template and base server JSON helper methods return `nil` when stored JSON cannot be decoded.
- Made Gost manager delete/update paths return unexpected relay delete errors while still treating missing relay services as idempotent cleanup.
- Checked frontend directory creation, default index writes, reverse-proxy error responses, HTTP body read/close, base64 decode, temporary directory cleanup, free-port lookup, config writes, and process signal/kill errors in command packages.
- Added regression coverage for malformed JSON helpers, Gost delete idempotency/error propagation, and default frontend index creation.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/server ./cmd/verify ./internal/model ./internal/gost -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -include=G104 -quiet ./cmd/server ./cmd/verify ./internal/model ./internal/gost
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -include=G104 -quiet \
  github.com/anixops/v2board/api/grpc/v2boardpb \
  github.com/anixops/v2board/cmd/migrate \
  github.com/anixops/v2board/cmd/server \
  github.com/anixops/v2board/cmd/sqlite2postgres \
  github.com/anixops/v2board/cmd/subtest \
  github.com/anixops/v2board/cmd/verify \
  github.com/anixops/v2board/docs \
  github.com/anixops/v2board/internal/cache \
  github.com/anixops/v2board/internal/config \
  github.com/anixops/v2board/internal/database \
  github.com/anixops/v2board/internal/gost \
  github.com/anixops/v2board/internal/grpc \
  github.com/anixops/v2board/internal/handler \
  github.com/anixops/v2board/internal/middleware \
  github.com/anixops/v2board/internal/model \
  github.com/anixops/v2board/internal/parser \
  github.com/anixops/v2board/internal/payment \
  github.com/anixops/v2board/internal/payment/gateways \
  github.com/anixops/v2board/internal/router \
  github.com/anixops/v2board/internal/service \
  github.com/anixops/v2board/internal/utils \
  github.com/anixops/v2board/internal/websocket
```

Result:

- The production-package `G104` scan exits successfully with zero reported issues.
- Current full-repository `G104` findings are zero after command-line and integration-helper cleanup.

### 2026-07-08 Production Gosec Gate

Finding:

- Full-repository `gosec` previously reported tool, generated-code, integration-test, and test-helper findings, so using raw `gosec ./...` as a blocker would fail on non-runtime paths.
- Runtime packages were clean after G104, parser integer narrowing, config path review, database/frontend permission tightening, and subscription JSON credential-output review.

Impact:

- CI had no blocking security scan that protected the actual panel server and runtime internal packages from regression.
- Future runtime `#nosec` suppressions could be added without rule IDs or justifications.

Remediation:

- Added a blocking CI step that builds the production runtime package list from `go list ./cmd/server ./api/grpc/v2boardpb ./internal/...`, excludes `internal/tests`, excludes generated files, and requires `#nosec` rule IDs plus justifications.
- Kept the full-repository JSON `gosec ./...` artifact as report-only while command-line tools and test helpers were cleaned; the artifact now runs with `-exclude-generated` and is blocking.
- Replaced unchecked sing-box `uint32` conversions with range-checked conversion, tightened runtime-created database/frontend permissions, and documented reviewed config-path and subscriber credential JSON outputs with scoped `#nosec` comments.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/server ./internal/parser ./internal/config ./internal/database -count=1
mapfile -t production_packages < <(
  PATH=/usr/local/go/bin:$PATH go list -f '{{.Dir}}' ./cmd/server ./api/grpc/v2boardpb ./internal/... \
    | sed "s#^${PWD}#.#" \
    | grep -Ev '^\./internal/tests(/|$)' \
    | sort
)
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated -nosec-require-rules -nosec-require-justification -quiet "${production_packages[@]}"
```

Result:

- Production runtime `gosec` exits successfully with zero reported issues and is now a blocking CI gate.

### 2026-07-08 Cmd Verify Gosec Remediation

Finding:

- `cmd/verify` embedded a sample subscription token, fetched a variable subscription URL, spawned a variable Xray path, and wrote temporary Xray config files with `0644` permissions.

Impact:

- The sample token created a credential false positive and encouraged local verification to depend on a stale token.
- The verifier accepted a broad executable path and broad panel URL shape even though it only needs a local Xray binary and local panel instance.
- Temporary Xray configs contain proxy credentials and should not be group/world-readable.

Remediation:

- Moved the subscription token to the required `V2BOARD_VERIFY_TOKEN` environment variable.
- Added `V2BOARD_VERIFY_BASE_URL` parsing with loopback-only host validation and query/fragment stripping.
- Added `V2BOARD_VERIFY_XRAY` parsing that only accepts local `xray` or `xray.exe` binary names before execution.
- Wrote generated Xray config files with `0600` permissions.
- Added unit tests for token validation, loopback URL enforcement, Xray path allowlisting, URL construction, and config file permissions.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/verify -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./cmd/verify
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-verify.json ./...
```

Result:

- `cmd/verify` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 106 to 100.

### 2026-07-08 Cmd Subtest Gosec Remediation

Finding:

- `cmd/subtest` created sample subscription config directories with `0755`, wrote sample config files with `0644`, and read operator-provided config paths directly.

Impact:

- Sample files may include proxy credentials or private node metadata and should not be group/world-readable.
- The subscription test CLI only needs local YAML files, but it did not validate extension or reject directories before reading.

Remediation:

- Added a config reader that requires an existing regular `.yaml` or `.yml` file before reading the operator-supplied path.
- Added a scoped `#nosec G304 G703` justification for the reviewed local CLI file read.
- Changed sample config output to create directories with `0750` and files with `0600`.
- Moved the sample config test into a temporary working directory and added permission/path validation coverage.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/subtest -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./cmd/subtest
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-subtest.json ./...
```

Result:

- `cmd/subtest` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 100 to 96.

### 2026-07-08 Cmd Configgen Gosec Remediation

Finding:

- `cmd/configgen` ignored output directory and config write errors, created `output/` with `0755`, and wrote generated Xray/Mihomo configs with `0644`.

Impact:

- Failed config writes could be reported as successful.
- Generated client configs may include UUIDs, server names, and future credentials, so they should not be group/world-readable by default.

Remediation:

- Moved config generation into `run()` so errors are returned and handled by `main`.
- Added `saveGeneratedConfigs` with checked `MkdirAll`/`WriteFile` calls.
- Changed output directory permissions to `0750` and generated config file permissions to `0600`.
- Added tests for successful private-permission writes and directory creation failure propagation.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/configgen -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./cmd/configgen
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-configgen.json ./...
```

Result:

- `cmd/configgen` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 96 to 90.

### 2026-07-08 Cmd Report Gosec Remediation

Finding:

- `cmd/report` ignored errors while generating CSV, HTML, and JSON reports, writing temporary Xray configs, closing HTTP response bodies, closing probe connections, stopping subprocesses, shutting down the local echo server, and writing echo responses.
- The local echo server did not set `ReadHeaderTimeout`.
- Xray subprocess execution used a variable path without a reviewed local binary allowlist.

Impact:

- Report generation could silently drop rows or files while still printing success.
- Temporary Xray configs can contain test credentials and should be private.
- E2E cleanup and local HTTP helper failures could be invisible in the report tool.

Remediation:

- Made CSV/HTML/JSON report generation return errors, check writer/template/marshal/flush/file errors, and write report files with `0600` under a `0750` report directory.
- Checked temporary Xray config writes with private permissions.
- Checked HTTP body close/read, connection close, process signal/kill/wait, echo response writes, and echo server shutdown/serve errors.
- Added Xray binary name validation before executing `xray` or `xray.exe` and added scoped `#nosec G204` justifications.
- Added `ReadHeaderTimeout` to the local echo server.
- Added tests for report file permissions, Xray path validation, and echo server timeout configuration.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/report -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./cmd/report
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-report.json ./...
```

Result:

- `cmd/report` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 90 to 50.
- Remaining full-repository `G104` findings are limited to integration/test helper packages.

### 2026-07-08 Integration Mock Server Gosec Remediation

Finding:

- `internal/tests/integration/mock/server.go` ignored JSON encoding errors in health, node config, user list, traffic push, online report, node register, and heartbeat handlers.
- The mock HTTP server did not configure `ReadHeaderTimeout`.

Impact:

- Integration mock responses could silently fail to encode while tests still observed a status code.
- Test HTTP servers should still exercise the same timeout hygiene expected from runtime servers.

Remediation:

- Added a shared `writeJSON` helper that sets JSON headers, status, and logs encode failures.
- Routed every mock API handler through the helper.
- Added `ReadHeaderTimeout` to the mock server.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/mock -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/mock
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-mock.json ./...
```

Result:

- `internal/tests/integration/mock` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 50 to 42.
- Remaining full-repository `G104` findings are down from 24 to 17.

### 2026-07-08 Integration Echo Server Gosec Remediation

Finding:

- `internal/tests/integration/echo/server.go` ignored request body close, response writes, TCP deadline, manual HTTP response writes, and TCP echo copy errors.
- The echo handler intentionally reflects local test request data, which triggered XSS taint analysis.

Impact:

- Integration echo helpers could hide broken clients, failed writes, or connection cleanup problems.
- The intentional echo behavior needed a scoped reviewed exception so future scans stay meaningful.

Remediation:

- Checked and logged request body close, response write, deadline, manual connection write, TCP close, and TCP copy errors.
- Added a scoped `#nosec G705` justification for intentional local echo response behavior.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/echo -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/echo
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-echo.json ./...
```

Result:

- `internal/tests/integration/echo` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 42 to 36.
- Remaining full-repository `G104` findings are down from 17 to 12.

### 2026-07-08 Integration Local Environment Gosec Remediation

Finding:

- `internal/tests/integration/local/environment.go` ignored process kill, probe connection close, and HTTP body close errors.
- The local integration launcher spawned configured binaries and wrote test config files/directories without narrow permissions or binary-name review.

Impact:

- Local integration failures could be hidden by cleanup paths, making flaky process shutdown and port/HTTP probes harder to diagnose.
- Test-generated configs could be readable by other local users on shared hosts.
- Subprocess execution warnings needed a scoped review so future full-repository scans stay useful.

Remediation:

- Checked and logged interrupt, kill, wait, port probe close, and HTTP body close failures.
- Restricted local server binaries to reviewed Xray/Mihomo binary names before spawning.
- Wrote local integration config directories as `0750` and saved config files as `0600`.
- Added tests for directory/file permissions and binary allowlist acceptance/rejection.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/local -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/local
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-local.json ./...
```

Result:

- `internal/tests/integration/local` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 36 to 28.
- Remaining full-repository `G104` findings are down from 12 to 8.

### 2026-07-08 Shared Testutil Database Gosec Remediation

Finding:

- `internal/tests/testutil/database.go` ignored `sql.DB.Close()` failures in the shared in-memory SQLite helper.

Impact:

- Shared handler, smoke, and service tests could hide test database cleanup failures.
- The full-repository gosec baseline kept a `G104` finding in a commonly imported test helper.

Remediation:

- Added `CloseWithError()` to return database handle and close failures explicitly.
- Kept `Close()` as a compatibility wrapper that logs cleanup errors for existing callers.
- Added tests proving the helper closes the underlying database and allows nil receivers.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/testutil -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/testutil
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-testutil.json ./...
```

Result:

- `internal/tests/testutil` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 28 to 27.
- Remaining full-repository `G104` findings are down from 8 to 7.

### 2026-07-08 Integration Runner Gosec Remediation

Finding:

- `internal/tests/integration/runner/runner.go` ignored config-directory creation errors.
- The runner wrote generated client configs and JSON reports with broad permissions.
- Generated client config cleanup used a bare deferred remove path.

Impact:

- Integration runs could proceed after setup failed, returning misleading test reports.
- Generated configs and reports could be readable by other local users on shared hosts.
- Config cleanup failures were not observable during local integration debugging.

Remediation:

- `Run` now records an explicit setup failure when the config directory cannot be created.
- Config directories are created as `0750`; generated configs and reports are written as `0600`.
- Deferred generated-config removal now logs non-`ENOENT` cleanup failures.
- Added tests for private config directory creation, setup failure reporting, and report file permissions.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/runner -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/runner
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-runner.json ./...
```

Result:

- `internal/tests/integration/runner` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 27 to 23.
- Remaining full-repository `G104` findings are down from 7 to 6.

### 2026-07-08 Integration Binary Manager Gosec Remediation

Finding:

- `internal/tests/integration/binary/manager.go` ignored temporary download close and cached-binary chmod errors.
- Cached binary directories and copied/extracted files used broad permissions.
- ZIP/GZIP extraction copied unbounded archive content.
- Archive extraction and file-copy helpers used raw variable paths.

Impact:

- Integration binary downloads could hide failed flushes or executable-permission setup failures.
- Cached binaries and extracted files could be exposed more broadly than needed on shared local hosts.
- Malicious or corrupt archives could consume excessive disk space during integration setup.
- Variable path file operations made the integration helper harder to audit.

Remediation:

- Checked temporary file close, response-body close, archive close, chmod, and cleanup errors.
- Created cache directories with `0750`, wrote copied/extracted binaries with private permissions, then marked cached binaries executable with a narrow reviewed `0750` chmod.
- Added a bounded copy helper for archive extraction and direct file copies.
- Routed local file open/create operations through `os.Root` scoped helpers.
- Added tests for copy permissions and oversized gzip extraction rejection.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/binary -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/binary
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-binary.json ./...
```

Result:

- `internal/tests/integration/binary` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 23 to 12.
- Remaining full-repository `G104` findings are down from 6 to 4.

### 2026-07-08 Integration Clients Gosec Remediation

Finding:

- `internal/tests/integration/clients` ignored process kill and startup probe response-close errors.
- Xray and Mihomo integration clients launched subprocesses from configurable binary paths without an explicit local allowlist.
- Binary discovery used raw variable-path stat checks and logged search path values.

Impact:

- Client shutdown and startup probe cleanup failures could be hidden during local integration runs.
- A misconfigured integration binary path could spawn an unexpected executable.
- Path and log handling made the helper harder to audit under full-repository gosec.

Remediation:

- Checked process kill errors and logged process wait failures.
- Added client binary-name allowlists for Xray and Mihomo/Clash.Meta before subprocess launch.
- Routed manual binary search through `os.Root` and removed variable path values from close-failure logs.
- Checked startup probe request creation and response-body close errors.
- Logged Mihomo stdout/stderr copy failures instead of discarding them.
- Added tests for binary allowlist acceptance and rejection.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/tests/integration/clients -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./internal/tests/integration/clients
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-clients.json ./...
```

Result:

- `internal/tests/integration/clients` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 12 to 5.
- Remaining full-repository `G104` findings are down from 4 to 0.

### 2026-07-08 Migration Dump Parser Gosec Remediation

Finding:

- `cmd/migrate/dumpparser.go` opened the operator-provided dump path directly, which full-repository gosec reported as `G304`.

Impact:

- The migration tool is an operator-only command, but direct variable-path opens made the remaining non-generated gosec baseline noisy.
- Plain SQL and gzip SQL dump parsing did not have focused tests around the path-opening entry point.

Remediation:

- Added `openDumpFile`, which rejects empty paths and opens the dump through `os.Root` scoped to the dump's parent directory.
- Checked dump and gzip reader close errors through observable logs.
- Added plain SQL, gzip SQL, and empty-path regression tests.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./cmd/migrate -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -nosec-require-rules -nosec-require-justification -quiet ./cmd/migrate
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -fmt=json -out=/tmp/v2board-gosec-current-after-migrate.json ./...
```

Result:

- `cmd/migrate` exits with zero gosec findings in strict suppression mode.
- Full-repository `gosec` findings are down from 5 to 4.
- The remaining full-repository findings are generated protobuf `G103` reports only.

### 2026-07-08 Generated-Code Gosec Report Exclusion

Finding:

- After command-line and integration-helper cleanup, raw full-repository `gosec ./...` only reported generated protobuf `G103` findings from `api/grpc/v2boardpb/v2board.pb.go`.

Impact:

- The CI gosec artifact was no longer actionable because its only findings were generated-code `unsafe` calls emitted by the protobuf generator.
- Production gosec already excluded generated files, so the full-repository report did not match the actionable runtime policy.

Remediation:

- Updated the CI gosec report to run `gosec -exclude-generated -fmt=json -out=security-reports/gosec.json ./...`.
- Promoted the generated-file-excluded full-repository gosec report to a blocking CI gate.
- Kept production runtime gosec unchanged: blocking, generated-file-excluded, and requiring `#nosec` rule IDs plus justifications.
- Documented raw generated protobuf `G103` as intentionally excluded from gosec gates.

Verification:

```bash
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -exclude-generated -fmt=json -out=/tmp/v2board-gosec-exclude-generated.json ./...
```

Result:

- Generated-file-excluded full-repository gosec reports zero findings.
- Raw `gosec ./...` still reports only generated protobuf `G103` findings.

### 2026-07-08 gRPC Integer Conversion Remediation

Finding:

- `internal/grpc` narrowed database-backed `uint`, `int`, and `int64` values into protobuf `uint32` and `int32` fields without range checks.
- Traffic and online report handlers updated node heartbeat timestamps without checking or logging failures.

Impact:

- Out-of-range node IDs, user IDs, node ports, TLS flags, or device limits could be silently truncated before being returned to gRPC clients.
- A failed heartbeat timestamp update during traffic or online reporting was invisible to operators.

Remediation:

- Added shared range-checked conversion helpers for protobuf `uint32` and `int32` fields.
- Routed node config, config sync, node registration, and user-list conversion through those helpers.
- Propagated out-of-range config/user conversion failures as gRPC errors instead of returning truncated values.
- Logged non-fatal heartbeat timestamp update failures in traffic and online reporting paths.
- Added focused conversion boundary tests.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/grpc -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/grpc
```

Result:

- `gosec -quiet ./internal/grpc` exits successfully with zero reported issues.
- Full-repository `gosec` issue count is down to 132, with no remaining `internal/grpc` findings in the refreshed JSON report.

### 2026-07-08 MFA Backup Code Parsing Remediation

Finding:

- `internal/service/mfa_service.go` parsed persisted MFA backup-code JSON without checking `json.Unmarshal` errors.

Impact:

- Corrupt `backup_codes` data could be treated as an empty code list.
- Users would see a generic MFA failure while operators had no explicit signal that stored MFA data was malformed.
- Backup-code verification could silently fail instead of returning a data integrity error through the `Verify` path.

Remediation:

- Added `parseBackupCodesWithError`.
- Backup-code verification, remaining-code counts, and regeneration now return parse errors where callers can handle them.
- Kept the existing `parseBackupCodes` helper as a compatibility wrapper for display/setup paths.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestMFAService/(TestSetupTOTP|TestVerifyBackupCode|TestVerifyBackupCodeRejectsInvalidStoredJSON|TestGetRemainingBackupCodes_NoMFA|TestGetRemainingBackupCodesRejectsInvalidStoredJSON|TestRegenerateBackupCodes|TestVerify)' -count=1
PATH=/usr/local/go/bin:/tmp/v2board-go-tools:$PATH gosec -quiet ./internal/service
PATH=/usr/local/go/bin:/tmp/v2board-lint-tools:$PATH golangci-lint run --timeout=8m ./internal/service
```

### 2026-07-08 Stats Cache Error Handling Remediation

Finding:

- `internal/service/stats_service.go` ignored cache delete/write errors for dashboard and user subscription statistics.
- The struct also retained an unused mutex field, keeping lint noise in a high-traffic service.

Impact:

- A future Redis/cache backend failure could be invisible in statistics refresh and invalidation paths.
- Cache refresh jobs could report success even when the cache write failed.
- User subscription cache invalidation failures were not observable.

Remediation:

- Dashboard refresh now returns cache write errors.
- Dashboard/user subscription read paths log cache write failures while still returning fresh database results.
- Added `InvalidateUserCacheWithError` and kept `InvalidateUserCache` as a compatibility wrapper that logs failures.
- Removed the unused mutex field and simplified the GORM dialector selector.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestStatsService/(TestGetDashboardStats|TestGetDashboardStats_ForceRefresh|TestGetUserSubscription|TestInvalidateUserCache|TestInvalidateUserCacheWithError|TestRefreshDashboardCache|TestTodayTraffic_SumsTodayLogsWithRate|TestGetHourlyTraffic_BucketsAndFillsZero|TestGetUserTrafficRanking|TestGetUserTrafficRanking_IncludeZeroUsers)' -count=1
PATH=/usr/local/go/bin:/tmp/v2board-go-tools:$PATH gosec -quiet ./internal/service
PATH=/usr/local/go/bin:/tmp/v2board-lint-tools:$PATH golangci-lint run --timeout=8m ./internal/service
```
