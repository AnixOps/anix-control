# Traffic Stats Operations

This document is the operator reference for raw traffic logs and the admin
traffic pages backed by `v2_server_log`.

## Runtime Tables

`v2_server_log` stores raw node traffic reports. It is used by:

- admin dashboard `today_traffic`
- `/api/v2/admin/traffic/hourly`
- `/api/v2/admin/traffic/user-ranking`
- `/api/v4/kernel/nodes/:id/traffic` with `granularity=hour` (one node's hourly series)
- diagnostics for the admin hourly traffic page

`v2_stat_server` keeps one row per node, server type and day (`record_type`
`d`) and per month (`m`), written in the transaction of each traffic report
with the node's traffic rate applied. `/api/v4/kernel/nodes/:id/traffic` with
`granularity=day` reads its day rows. Nothing purges it: a node adds about 365
rows a year per server type, so it needs no maintenance.

Quota counters are stored separately on `v2_user.u` and `v2_user.d`. Purging old
`v2_server_log` rows removes historical charts and rankings for that period, but
it does not reset user quota counters.

## User Last Online

`v4_kernel_user_activity` (one row per user: `user_id`, `last_online_at` as a
Unix time in seconds) holds when each user was last seen online. A user is
seen when a node reports their traffic or lists one of their connections:
a UniProxy `push` or `alive`, the gRPC node reports and the Agent Control
traffic report (`reports.v1`), which also carries the online list. Traffic
that passes through a forwarding route is not counted.

Why a table of its own: Control kept no durable value for it. `v2_user` has
no `t` column (v2board's last report time; Control never wrote it), `u` and
`d` carry no time, `last_login_at` is the last sign-in, the online set the
alive reports fill is a five minute cache, and the traffic log above is purged
by the operator.

- **Writes.** The traffic and alive reports call `subscriber.RecordOnline`
  after they committed. It is best effort and throttled: one user is written
  at most once per minute (`subscriber.ActivityWriteInterval`, remembered in
  memory per process), in multi-row upserts that move a time forward only
  and only for ids that are subscribers. The stored time therefore lags a
  report by less than a minute; a Control restart writes each online user
  once more. A failed write is logged (`user activity was not recorded`) and
  does not affect the report.
- **Size.** One row of two integers per user that was ever online, no
  retention job needed. Deleting a user deletes their row.
- **Nothing is backfilled.** The table starts empty at the upgrade: a user
  shows no last-online time until a node next reports them. To seed it from
  the traffic log (only as far back as the log was kept), run this once,
  after taking a backup:

  ```sql
  INSERT INTO v4_kernel_user_activity (user_id, last_online_at)
  SELECT l.user_id, MAX(l.log_at) FROM v2_server_log l
  JOIN v2_user u ON u.id = l.user_id WHERE l.log_at > 0 GROUP BY l.user_id
  ON CONFLICT (user_id) DO UPDATE SET last_online_at = excluded.last_online_at
  WHERE excluded.last_online_at > v4_kernel_user_activity.last_online_at;
  ```

- **Reading.** `GET /api/v4/admin/users/activity?ids=1,2,3` (administrators,
  at most 200 ids): `{"data":{"users":[{"user_id":1,"last_online_at":1760000000},
  {"user_id":2,"last_online_at":null}]}}`, in the order asked, null for a
  user never seen. The user list and detail answers keep their v2 shape; the
  console asks this route for the ids of the page it shows. See
  `docs/guide/api-reference.md`.

## Query Bounds

The current service bounds traffic analytics queries before they reach the
database:

- hourly traffic defaults to 24 hours and clamps to 720 hours
- ranking defaults to 24 hours and clamps to 720 hours
- ranking limits clamp to 200 rows by default
- `include_zero_users=true` ranking limits clamp to 1000 rows

The admin UI currently requests up to 720 hours for the 30 day view.

## Per-Node Traffic Series

`GET /api/v4/kernel/nodes/:id/traffic` (administrator) answers one proxy node's
traffic for the admin charts. It adds no table and no write: the traffic
reports already fill both tables it reads.

| `granularity` | Reads | Bounds | Buckets |
| --- | --- | --- | --- |
| `hour` (default) | `v2_server_log` rows of the node, `(u * rate)` up and `(d * rate)` down | 24 hours by default, at most 720 hourly buckets | UTC hours |
| `day` | `v2_stat_server` day rows of the node (already rate-applied) | 30 days by default, at most 366 daily buckets | local calendar days of the Control host, where the rows are recorded |

- `since` and `until` are Unix milliseconds; the window is rounded out to
  whole buckets and a longer one is `400 invalid_request`, not clamped.
  Buckets without traffic are zeros, so the series draws as it is.
- The node id is the key of both tables for every server type: agent and
  legacy node reports write `server_type` `node`, UniProxy the protocol name;
  the series sums them. Rows imported from a v2board database that keyed
  `server_id` by the per-type server tables, not by node id, can alias an
  unrelated node in old windows.
- **Hourly history ends where the raw log does.** After a purge (below) the
  early hours of the window are zeros, not data; the daily series is not
  affected by a purge.
- **Cost.** The hourly query reads one node's rows in the window, through
  `idx_v2_server_log_server_id` or both single-column indexes, and no
  composite `(server_id, log_at)` index exists: its cost follows the node's
  retained rows, so the retention below also bounds it. On a throwaway
  PostgreSQL 16 with 8.4 million synthetic rows (30 nodes, 14 days, about
  20 000 rows per node and day), one node's hourly series took about 40 ms
  for 24 hours, 90 ms for 7 days and 125 ms for 14 days, against 1.5 s for
  the existing all-node `/api/v2/admin/traffic/hourly` over 7 days on the
  same data. A busier node costs proportionally more; that is why there is
  no per-report rollup write, and a rollup table is the next step only if a
  deployment's retained rows make this query slow.
- The forwarding traffic of a node or route is a different ledger
  (`v4_kernel_forward_traffic`): `GET /api/v4/forward/stats`,
  `/api/v4/forward/routes/{id}/stats` and `/api/v4/forward/observability/trend`.

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
