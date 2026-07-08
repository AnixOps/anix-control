# Concurrency Risk Register

Date: 2026-07-08

This register tracks known and suspected concurrency risks. A risk is closed only when code, tests, and CI evidence prove the behavior.

## Baseline Evidence

- CI includes `go test -race ./... -count=1 -p=1`.
- The full Go test suite is run serially with `-p=1` because some tests share fixed SQLite paths.
- The in-memory cache uses `sync.RWMutex` and a cleanup goroutine.
- Forward runtime workers, gRPC streams, WebSocket flows, health checks, and integration test servers all create goroutines.

## Current Risks

### Shared Test Database State

Status: partially mitigated

Risk:

- The service package still uses a package-global database handle and cleanup flow during many suite tests.

Existing mitigation:

- `ServiceTestSuite` now creates the SQLite database under a per-process temporary directory instead of using a fixed `/tmp/v2board_service_test.db` path.
- `TestMain` closes the active GORM/sql.DB handle before removing the temporary directory.
- `TestServiceTestDatabasePathIsIsolated` verifies the isolated temporary path.

Remaining remediation:

- Continue moving state-heavy suites to per-suite or per-test database handles.
- Keep CI serial until global DB usage has been removed from service tests.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -count=1
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run TestServiceTestDatabasePathIsIsolated -count=1
```

### Global Database Handle

Status: open

Risk:

- `internal/database` keeps a package-global `*gorm.DB` and initialization flag.
- Tests and commands that reset or reinitialize this state can race if executed in the same process without strict isolation.

Required remediation:

- Prefer dependency-injected `*gorm.DB` in services and tests.
- Limit global reset helpers to test-only paths.
- Add race tests around database bootstrap helpers if they remain global.

### Memory Cache Cleanup Goroutine

Status: mitigated

Risk:

- `cache.InitMemory` starts a cleanup goroutine.
- Reinitialization without a matching `CloseMemory` can leave stale cleanup goroutines alive.

Existing mitigation:

- `InitMemoryWithSize` closes and waits for the previous cleanup goroutine before replacing the global memory cache.
- `CloseMemory` closes `stopCh` and waits for `doneCh`, making repeated close calls idempotent.
- `StartCleanup` recreates both lifecycle channels before restarting cleanup.
- Tests cover repeated init, idempotent close, restart, and race detector coverage for lifecycle/concurrent access paths.

Required remediation:

- Continue using `CloseMemory` in tests that replace the global cache, and avoid concurrent init/close during request handling.

### Forward Runtime Job Duplication

Status: mitigated

Risk:

- Repeated pause/resume actions can enqueue duplicate runtime jobs and create stale state transitions.
- Runtime workers must not leave claimed jobs stuck in `running` after context cancellation.

Existing mitigation:

- Runtime job schema guard and repair logic now collapse duplicate pending/running jobs before adding a partial unique index.
- Panel pause/resume has idempotent behavior while runtime jobs are pending or running.
- Admin UI bulk actions filter forwards that already have the requested state.
- `PanelForwardRuntimeJobExecutor` tests verify cancellation is propagated into the active runner and the claimed job is drained into a failed terminal state with `completed_at` and forward runtime status updated.
- `ForwardAgentBridgeWorker` tests verify canceled NodeX translation drains to a failed runtime job, leaves no bridge mapping, updates forward state, and retry dispatch upserts an existing bridge mapping instead of creating duplicates.
- Shared background-cycle tests verify delayed worker sleeps exit on `ctx.Done()`.
- `ForwardGostStatsWorker` tests verify the `Start(ctx)` loop exits while waiting in an idle poll delay.

Required remediation:

- Add PostgreSQL-specific checks for the partial unique index if the schema helper changes.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestPanelForwardRuntimeJobExecutor|TestForwardAgentBridgeWorker|TestForwardGostStatsWorker|TestWaitForwardBackgroundCycle' -count=1
PATH=/usr/local/go/bin:$PATH go test -race ./internal/service -run 'TestPanelForwardRuntimeJobExecutor/TestRunPendingJobs_CancelDrainsRunningJob|TestForwardAgentBridgeWorker/(TestTranslateCancellationFailsJobAndDrains|TestRetryUpsertsExistingBridgeMapping)|TestWaitForwardBackgroundCycle|TestForwardGostStatsWorker/TestStart_StopsOnContextCancellationDuringIdleDelay' -count=1
```

### Health Checks And Network Dialing

Status: partially mitigated

Risk:

- Older health check paths do not uniformly propagate `context.Context`.
- Network dials can continue past request cancellation unless `DialContext` is used.
- Goroutine fan-out health checks can hide per-node errors if errors are ignored.

Existing mitigation:

- `ForwardNodeService.HealthCheck` now uses `DialContext`, checks canceled contexts, and returns persistence/cleanup errors.
- `ForwardNodeService.HealthCheckAll` now aggregates per-node errors instead of discarding them.

Required remediation:

- Audit remaining health check paths outside `ForwardNodeService`.
- Add cancellation tests that fail fast without waiting for socket timeout.

### WebSocket Lifecycle

Status: mitigated

Risk:

- WebSocket connections can accumulate if auth, read deadlines, write deadlines, close handling, and subscription cleanup are inconsistent.
- Browser-origin WebSocket upgrades can bypass HTTP CORS if each upgrader has its own permissive `CheckOrigin` callback.

Existing mitigation:

- All current WebSocket upgrade paths were reviewed: user subscription WebSocket, admin monitor WebSocket, legacy agent WebSocket, and unified agent WebSocket.
- User subscription and admin monitor WebSockets require JWT/admin authentication before upgrade, set read deadlines, refresh them through pong handlers, set write deadlines, send pings, and remove or stop clients on disconnect.
- Agent WebSockets require node credentials through header/query auth or the legacy auth message, now set read limits/read deadlines/pong handlers, refresh read deadlines after messages, set write deadlines for JSON sends, remove node connections on disconnect, and fail pending ACK waiters on unified disconnect.
- WebSocket `CheckOrigin` now shares a tested policy: no-Origin clients are allowed, same-host origins are allowed, configured `server.cors.allowed_origins` are allowed, and unlisted cross-site origins are rejected.
- Config examples document the CORS/WebSocket Origin allowlist.

Verification:

```bash
PATH=/usr/local/go/bin:$PATH go test ./internal/utils ./internal/websocket ./internal/handler -count=1
PATH=/usr/local/go/bin:$PATH go test -race ./internal/utils ./internal/websocket ./internal/handler -run 'TestIsWebSocketOriginAllowed|TestSubscriptionWebSocketUpgraderOriginPolicy|TestMonitorWSUpgraderOriginPolicy|TestAgentWebSocketUpgraderOriginPolicy|TestPrepareAgentWebSocketSetsReadDeadline|TestHandleWebSocketMessage_RequireAckSendsAck' -count=1
PATH=/usr/local/go/bin:/home/dev/go/bin:$PATH gosec -quiet ./internal/handler ./internal/websocket ./internal/utils
```

Remaining remediation:

- Keep adding connect/disconnect tests when new WebSocket endpoints are introduced.

### gRPC Streams

Status: covered for stream cancellation

Risk:

- Bidirectional gRPC streams need clean cancellation and connection manager cleanup.

Existing coverage:

- gRPC tests cover health checks, registration/config/user/traffic error paths, connection manager behavior, and stream scaffolding.
- `StreamBidirectionalTestSuite` covers context cancellation for `StatusStream`, `UserChanges`, `ConfigChanges`, `TrafficStream`, and `OnlineStream`.
- `TestStatusStream_ContextCancellation` proves connection manager unregister behavior after client cancellation.
- gRPC stream cancellation tests pass under the race detector for the `StatusStream` and `OnlineStream` cancellation paths.

Required remediation:

- Keep adding stream tests when new long-lived RPCs are introduced.

### Background Workers

Status: open

Risk:

- Forward runtime workers, clean-agent workers, stats workers, and bridge workers depend on goroutine lifecycle discipline.

Required remediation:

- Each worker should accept context, return terminal errors, and expose stop/drain tests.
- Race tests must cover state transition and queue mutation paths.

### Notification Dispatch Goroutines

Status: partially mitigated

Risk:

- Notification trigger methods dispatch email/Telegram work in goroutines.
- Passing addresses from caller or range state into goroutines can cause incorrect user IDs or data races if the source value is mutated.
- Ignored send errors make asynchronous delivery failures hard to diagnose.

Existing mitigation:

- Notification dispatch now uses a helper that copies `userID` before starting the goroutine.
- Send failures are logged with type, event, user ID, and error.
- A race-tested regression covers copied user IDs.

Required remediation:

- Add context-aware cancellation and bounded worker queues for high-volume broadcast paths.
- Add backpressure or rate limiting for bulk notifications.

## Current Verification Commands

```bash
PATH=/usr/local/go/bin:$PATH go test -race ./... -count=1 -p=1
PATH=/usr/local/go/bin:$PATH go test ./... -count=1 -p=1
```

## Priority

1. Test DB isolation.
2. Context-aware network health checks.
3. WebSocket lifecycle audit.
4. Worker cancellation and drain tests.
5. Remove remaining shared mutable global state where practical.
