### Changed

- SQLite: a database file is opened with one writer connection and a pool of
  four read connections (GORM dbresolver), WAL, `synchronous=NORMAL`, a
  10 second `busy_timeout` and, for a database file created by this version,
  incremental auto-vacuum. Writes and transactions queue in Control instead of
  waiting inside SQLite, and reads no longer wait for the writer. The settings
  are built in; `database.max_open_conns` and its siblings stay PostgreSQL
  only. The package hosts that share the file use the same timeout and
  `synchronous` setting (`docs/reference/configuration.md`).
- The traffic log `v2_server_log` holds hourly buckets: a node's reports of a
  UTC hour are one row per user, server type and rate (the bytes add up), where
  every report was a row. All readers give the same numbers; a row's `log_at`
  is the hour's start. The first start merges rows that already share a bucket
  key and adds the unique index `idx_v2_server_log_bucket`
  (`docs/reference/traffic-stats-operations.md`).
- A traffic report updates the subscribers' `u` and `d` counters 100 to a
  statement instead of one statement each, with the same results.
- An Agent's heartbeat, traffic report and log batch write the node's
  `last_check_at` and status at most once a minute per node
  (`AgentLivenessWriteInterval`); the Hello of a stream always writes. Readers
  ask for nodes seen in the last five minutes, so nothing they show changes.
- The token revocation tables are read every five seconds as before but pruned
  at most every five minutes, and only when a row has expired.

### Added

- A retention worker, run by the process that holds the singleton-workers
  lease, deletes every hour, in batches of 5000 rows, the rows of
  `v2_server_log` and `v2_audit_log` older than 400 days and of `v2_node_log`
  older than 30 days, and on SQLite returns freed pages to the file and runs
  `PRAGMA optimize`. Archive older rows before upgrading if you need them
  (`docs/reference/traffic-stats-operations.md`).
