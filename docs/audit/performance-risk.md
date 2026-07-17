# Performance Risk Register

Date: 2026-07-08

This register tracks known performance risks and the evidence needed before treating them as solved.

## Baseline Evidence

- CI includes benchmark smoke with `go test -run '^$' -bench=. -benchtime=1x ./internal/service ./internal/handler ./internal/router`.
- The backend has SQLite and PostgreSQL paths.
- Traffic and stats features are high-volume paths.
- Forward runtime and node status features can create frequent writes and background polling.

## Current Risks

### Traffic And Stats Aggregation

Status: partially mitigated

Risk:

- Traffic logs can grow quickly.
- Hourly traffic and ranking queries can become slow without correct indexes and bounded time windows.
- Dirty historical rows with invalid rates or negative values can distort totals.

Existing mitigation:

- Stats schema helpers and tests cover traffic aggregation behavior.
- PostgreSQL stats regression exists in CI.
- Recent service fixes sanitize invalid traffic/rate inputs in key paths.
- `docs/reference/traffic-stats-operations.md` documents the current indexes, query bounds, and operator-controlled retention policy.
- `BenchmarkStatsServiceTrafficQueries` covers hourly and ranking queries against 250 active users, 750 zero-traffic users, and 168 hours of traffic logs.
- `BenchmarkForwardRuntimeJobListing` and `BenchmarkForwardRuntimeJobCleanAgentClaiming` cover runtime job listing filters and clean-agent claim batches against realistic queue shapes.

Required remediation:

- Add explain-plan checks for PostgreSQL stats queries once schema stabilizes.

### User Ranking And Include-Zero Queries

Status: partially mitigated

Risk:

- Including zero-traffic users can force broader scans and joins.
- Admin traffic ranking with large user tables needs pagination and query limits that protect the database.

Required remediation:

- Keep hard limits on ranking APIs. Current service and admin HTTP tests cover oversized request parameters and assert the 200/1000 row caps.
- Track benchmark output over time and add thresholds once the performance baseline stabilizes.

### Subscription Generation

Status: open

Risk:

- Subscription endpoints may be hit frequently by clients.
- Format generation can perform repeated database reads and serialization work.
- Token leakage can turn subscription endpoints into high-rate anonymous traffic.

Required remediation:

- Audit caching strategy for subscription responses.
- Add rate limiting or abuse controls where appropriate.
- Add benchmarks for common subscription formats.

### Node Reporting And Heartbeats

Status: open

Risk:

- Node traffic push, online state, and heartbeat paths can generate high write rates.
- Batch handling and transaction boundaries need explicit review.

Required remediation:

- Keep batch writes atomic where consistency matters.
- Add input size limits to node report endpoints.
- Benchmark traffic push and online update paths.

### Forward Runtime Jobs

Status: partially mitigated

Risk:

- Duplicate jobs can waste executor capacity and cause stale status transitions.

Existing mitigation:

- Partial unique guard for active jobs.
- Repair step for historical duplicates.
- Idempotent pause/resume behavior.
- Benchmarks now cover admin runtime job listing filters and clean-agent heartbeat claiming.

Required remediation:

- Track runtime job benchmark output over time.
- Add indexes for common executor filters if benchmark or production evidence shows queue scans.

### Memory Cache

Status: open

Risk:

- The memory cache updates LRU metadata under a write lock even on reads.
- High read concurrency can create lock contention.

Existing mitigation:

- Cache has a max size and TTL cleanup.

Required remediation:

- Benchmark hot keys and high-concurrency access.
- Consider lock sharding only if benchmark evidence shows contention.

### WebSocket And Long-Lived Connections

Status: open

Risk:

- Long-lived WebSocket connections consume file descriptors, goroutines, memory, and write buffers.
- Slow readers can block broadcast paths if not isolated.

Required remediation:

- Add connection limits, deadlines, and backpressure behavior.
- Add stress tests for connect/disconnect churn.

### Frontend Build And Bundle Size

Status: partially mitigated

Risk:

- Admin traffic/forward pages import charting and UI dependencies.
- Bundle size can affect first load, especially on mobile networks.

Existing mitigation:

- `npm run bundle:report` reads Vite output under `web/public/assets`, writes
  `web/bundle-reports/bundle-size.json` and `bundle-size.md`, and appends the
  markdown report to the GitHub Actions step summary.
- The frontend CI build uploads the bundle report as the
  `frontend-bundle-size` artifact after `npm run build`.
- The report tracks total JS/CSS/other output, largest gzip assets, and named
  heavy/admin chunks including Forward, Tunnel, Users, Nodes, NodeX,
  LocalRuntime, AnsibleMachines, TrafficHourly, Observability, System,
  forwardRuntime, echarts, G6, vendor, and vue-vendor.

Required remediation:

- Add size thresholds once several CI runs establish a stable baseline.
- Lazy-load heavy admin-only views where possible.

## Current Verification Commands

```bash
PATH=/usr/local/go/bin:$PATH go test -run '^$' -bench=. -benchtime=1x ./internal/service ./internal/handler ./internal/router
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run '^$' -bench 'BenchmarkForwardRuntimeJob' -benchtime=1x -count=1
PATH=/usr/local/go/bin:$PATH go test ./internal/service -run 'TestStatsService|TestPostgresStats' -count=1
(cd web && npm run bundle:report)
(cd web && npm run build -- --outDir ./public-check)
(cd web && BUNDLE_PUBLIC_DIR="$PWD/public-check" BUNDLE_REPORT_DIR="$PWD/bundle-reports-check" npm run bundle:report)
bash config/deploy/clean_local_build_artifacts.sh --dry-run
```

## Priority

1. PostgreSQL explain-plan checks for stats queries.
2. Benchmark threshold policy for stats/ranking queries.
3. Node report input limits and batch write review.
4. Cache contention benchmark.
5. WebSocket connection/resource limits.
