# Traffic Stats Operations

This document is the operator reference for raw traffic logs and the admin
traffic pages backed by `v2_server_log`.

## Runtime Tables

`v2_server_log` stores raw node traffic reports. It is used by:

- admin dashboard `today_traffic`
- `/api/v2/admin/traffic/hourly`
- `/api/v2/admin/traffic/user-ranking`
- diagnostics for the admin hourly traffic page

Quota counters are stored separately on `v2_user.u` and `v2_user.d`. Purging old
`v2_server_log` rows removes historical charts and rankings for that period, but
it does not reset user quota counters.

## Query Bounds

The current service bounds traffic analytics queries before they reach the
database:

- hourly traffic defaults to 24 hours and clamps to 720 hours
- ranking defaults to 24 hours and clamps to 720 hours
- ranking limits clamp to 200 rows by default
- `include_zero_users=true` ranking limits clamp to 1000 rows

The admin UI currently requests up to 720 hours for the 30 day view.

## Indexes

The `TrafficLog` model defines these indexes:

| Index | Columns | Purpose |
| --- | --- | --- |
| `idx_v2_server_log_user_id` | `user_id` | user-scoped diagnostics and joins |
| `idx_v2_server_log_server_id` | `server_id` | node-scoped investigation |
| `idx_v2_server_log_log_at` | `log_at` | time-window scans |
| `idx_server_log_log_at_user_id` | `log_at`, `user_id` | hourly and ranking windows, including optional user filter |

`EnsureStatsSchema` runs at startup and calls `AutoMigrate` for the stats
tables. The regression test
`TestEnsureStatsSchema_CompletesLegacyTrafficLogTable` verifies that legacy
`v2_server_log` tables are expanded with `rate`, `log_at`, and
`idx_server_log_log_at_user_id`.

## Retention Policy

There is no automatic background purge for `v2_server_log` today. Treat raw
traffic-log retention as an operator-controlled database maintenance task.

Recommended defaults:

- keep at least 31 days of raw rows so the 30 day admin traffic page remains
  complete
- keep 90 days on PostgreSQL production deployments when disk budget allows
- keep 30 to 90 days on SQLite deployments, depending on file size and backup
  windows
- archive before purging if historical abuse investigation, billing support, or
  customer support workflows need older traffic details

Do not purge current-day rows while investigating traffic reporting issues.

## Manual Archive And Purge

Run these commands only after taking a database backup and confirming the
retention window for the deployment.

PostgreSQL archive and purge example for rows older than 90 days:

```sql
\copy (
  SELECT *
  FROM v2_server_log
  WHERE log_at < EXTRACT(EPOCH FROM now() - interval '90 days')::bigint
) TO 'traffic-log-archive-before-90d.csv' CSV HEADER;

DELETE FROM v2_server_log
WHERE log_at < EXTRACT(EPOCH FROM now() - interval '90 days')::bigint;

VACUUM (ANALYZE) v2_server_log;
```

SQLite archive and purge example for rows older than 90 days:

```sql
.mode csv
.headers on
.once traffic-log-archive-before-90d.csv
SELECT *
FROM v2_server_log
WHERE log_at < CAST(strftime('%s', 'now', '-90 days') AS INTEGER);

DELETE FROM v2_server_log
WHERE log_at < CAST(strftime('%s', 'now', '-90 days') AS INTEGER);

VACUUM;
```

After purging, verify that recent windows still have data:

```sql
SELECT COUNT(*) AS last_24h_rows
FROM v2_server_log
WHERE log_at >= EXTRACT(EPOCH FROM now() - interval '24 hours')::bigint;
```

For SQLite, use:

```sql
SELECT COUNT(*) AS last_24h_rows
FROM v2_server_log
WHERE log_at >= CAST(strftime('%s', 'now', '-24 hours') AS INTEGER);
```

## Operational Checks

Before and after a purge:

- check `/api/v2/admin/traffic/hourly?hours=24`
- check `/api/v2/admin/traffic/user-ranking?hours=168&limit=500&include_zero_users=true`
- compare database file or table size before and after maintenance
- keep the archive and database backup until the next successful backup cycle
