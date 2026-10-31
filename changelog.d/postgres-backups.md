### Added

- Backups cover PostgreSQL. A PostgreSQL panel now gets the daily, pre-migration
  and manual bundles, their verification, the retention, the alerts and the
  `backup` commands, like a SQLite one. The database member of a bundle is a
  `pg_dump` custom-format archive (`database.pgdump`, manifest
  `database_kind: postgres`) taken from one exported snapshot, so the row counts
  in the manifest are exactly the dumped state. A bundle is verified by
  restoring it into a scratch database on the same server, so the database role
  needs `CREATEDB`. `backup restore` restores into an empty database; with
  `--replace` an existing one is renamed to `<name>_replaced_<UTC time>`, never
  deleted.
  Install `pg_dump` and `pg_restore` (the `postgresql-client` package, the
  server's major version or newer) on the machine Control runs on. Without them
  a PostgreSQL panel starts and migrates as before (no pre-migration bundle),
  the backup page says what is missing and the stale-backup alert fires.
