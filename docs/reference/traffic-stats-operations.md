# Traffic Stats Operations

This document is the operator reference for raw traffic logs and the admin
traffic pages backed by `v2_server_log`.

## Runtime Tables

`v2_server_log` stores node traffic in hourly buckets: one row per node,
UTC hour, user, server type and rate, to which every report of that hour
adds its bytes (see [Hourly Buckets](#hourly-buckets)). It is used by:

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

## Hourly Buckets

A traffic report no longer inserts a row per user. It adds to the row of its
bucket key, `(server_id, log_at, user_id, server_type, rate)`, with an upsert
on the unique index `idx_v2_server_log_bucket`:

- `log_at` is the start of the UTC hour that holds the report, so a node's
  reports of one hour are one row per user instead of one per report. With 50
  nodes reporting every minute that is about 60 times fewer rows; the table
  grows with users and hours, not with reports.
- `u` and `d` are the bytes as reported, before `rate`; the readers apply the
  rate to each row and sum, so every reader returns the same numbers as with a
  row per report. A change of a node's rate inside an hour makes a second row
  of that hour.
- The time of a row is the hour, not the second of the report: `log_at` of a
  bucket is its hour's start. The row's `created_at` is the time of the latest
  report that added to it, which is how `meta.latest_log_at` of the hourly
  traffic response (the dashboard's "reports stopped" alert reads its age)
  stays the time of the latest report and not the hour's start. A reader that
  cuts a window inside an hour sees the whole hour or none of it: the
  dashboard's "today" starts at local midnight, which is an hour boundary in
  every zone with whole-hour offsets; in a zone with a 30 or 45 minute offset
  the first hour of the day is counted on the earlier day.
- Rows written by older versions keep their own `log_at` and are not moved.
  The first start of this version merges rows that already share a bucket key
  (the same node, second, user, type and rate) into the oldest of them, adding
  their `u` and `d`, in the same transaction that creates the unique index;
  rows with an empty value in the key are never merged.

The upsert needs `idx_v2_server_log_bucket`, which is created at startup by
`EnsureTrafficLogBuckets` (before `EnsureStatsSchema`). A table that has no
such index, for instance one restored from an older backup, gets it at the next
start.

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
alive reports fill is a five minute cache, and the traffic log above holds
hourly buckets and is pruned after its retention.

- **Writes.** The traffic and alive reports call `subscriber.RecordOnline`
  after they committed. It is best effort and throttled: one user is written
  at most once per minute (`subscriber.ActivityWriteInterval`, remembered in
  memory per process), in multi-row upserts that move a time forward only
  and only for ids that are subscribers. The stored time therefore lags a
  report by less than a minute; a Control restart writes each online user
  once more. A failed write is logged (`user activity was not recorded`) and
  does not affect the report.
- **Size.** One row of two integers per user that was ever online, so the
  retention worker leaves it alone. Deleting a user deletes their row.
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
  `idx_v2_server_log_bucket`, which starts with `(server_id, log_at)`: its
  cost follows the node's retained rows, so the retention below also bounds
  it. The measurement that follows predates the hourly buckets and the
  composite index, and counted a row per report. On a throwaway
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
| `idx_v2_server_log_bucket` (unique) | `server_id`, `log_at`, `user_id`, `server_type`, `rate` | the key of an hourly bucket, the conflict target of the upsert, and a node's hourly window |

The schema baseline (`internal/database/migrations`) creates the stats tables
and these indexes. Adopting a database an earlier Control created adds the
`v2_server_log` columns and indexes it lacks (`rate`, `log_at`,
`idx_server_log_log_at_user_id`, ...); the adoption tests
(`TestAdoptionCompletesADatabaseAnOlderReleaseLeft`) cover a legacy
`v2_server_log` without them. See
[database-migrations.md](database-migrations.md).

## Retention Policy

Control prunes `v2_server_log` itself. A singleton worker (`internal/retention`,
run by the process that holds the `control.singleton-workers` lease) wakes
every hour and deletes, oldest first, in batches of 5000 rows with a pause
between batches:

| Table | Kept | Dated by |
| --- | --- | --- |
| `v2_server_log` | 400 days | `log_at` |
| `v2_node_log` | 30 days | `created_at` |
| `v2_audit_log` | 400 days | `created_at` |

One run deletes at most 2000 batches per table, so a large backlog is cleared
over the following hours. On SQLite the run then returns up to 5000 free pages
to the file and runs `PRAGMA optimize`; the pages can only be returned in a
database created with incremental auto-vacuum, which every database this
version creates is (an existing file keeps its setting and is compacted by
`VACUUM`). A run that deletes rows logs `Retention: deleted N rows of <table>`.

The periods are built in. Raw rows older than 400 days therefore do not
survive: the daily series of `/api/v4/kernel/nodes/:id/traffic` reads
`v2_stat_server`, which is not pruned, and the 30 day admin page needs 31 days.
Archive before the retention if older traffic details matter, with the
commands below.

## Manual Archive And Purge

The retention worker covers the routine case. Run these commands to archive
before a purge, or to purge sooner than the worker does, only after taking a
database backup and confirming the retention window for the deployment.

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
