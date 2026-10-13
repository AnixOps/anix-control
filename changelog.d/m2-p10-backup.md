### Added

- Control backs itself up on SQLite (`internal/backup`,
  `docs/reference/backups.md`). A bundle is one tar file in
  `data_dir/backups` (0700, files 0600) named
  `anix-control-<UTC time>-<type>.tar`, holding a consistent snapshot of the
  database (`VACUUM INTO` on its own connection: the panel's writer keeps
  committing meanwhile), `root.key`, the configuration file Control loaded
  and a manifest (format, program and schema version, type, host, domain,
  the root key's fingerprint, every member's size and SHA-256, key row
  counts). Bundles are not encrypted: treat them like the root key.
- Backups run every day at the panel's own time between 03:00 and 05:00
  local (derived from the domain), before every pending schema migration of
  a database that is not empty (`pre-migration`; when it fails, nothing is
  migrated and the start stops) and on request: `anix-control backup create`
  or 系统设置 → 备份 → 立即备份.
- Every bundle is verified by restoring it into `data_dir/tmp`: checksums,
  `PRAGMA integrity_check`, the migration records, and that the bundle's root
  key opens the snapshot's CAs and sealed keys. Results are in the new table
  `v5_backup` (migration `0005_backups`); kernel alerts
  `backup_verify_failed` and `backup_stale` (no verified bundle younger than
  36 hours) join the alert scan, and the dashboard shows the last verified
  backup with its age.
- Retention: the newest 7 daily bundles, one per week for the 4 weeks before,
  every pre-migration bundle of 30 days, the newest 3 manual ones; the newest
  verified bundle is never removed, nor a bundle being read.
- `anix-control backup create [-type manual|daily|pre-migration|verify]`,
  `backup list [-json]`, `backup verify [<bundle>]` and
  `backup restore <bundle> [--replace] [--data-dir D]`. A restore refuses
  while Control runs (a running Control holds a shared lock on
  `data_dir/control.lock`), refuses a target that is not empty unless
  `--replace` moves it into `data_dir/replaced-<UTC time>/` (never deleting),
  and refuses a bundle a newer Control migrated. Exit status 0 done, 1 failed,
  2 usage, 3 refused.
- `GET /api/v5/backups`, `POST /api/v5/backups` and
  `GET /api/v5/backups/:id/download` (a super administrator only, written to
  the audit log as `backups/download`). The audit log now covers `/api/v5`.

### Changed

- 系统设置 → 备份 lists the bundles with their verification, the last
  verified backup and the next automatic one, backs up and downloads; it no
  longer restores. Restore on the server with Control stopped.
- `anix-control forward legacy drop` accepts a verified Control bundle of
  the last 24 hours as its database backup, besides `--backup-taken`.

### Removed

- The backup settings (automatic backups, interval, retention, contents,
  local folder, S3), which nothing executed, the backup statistics,
  deletion and the online restore, with the routes
  `/api/v2/admin/system/backup/config` (GET, PUT), `/backup`, `/backups`,
  `/backup/stats`, `/backups/:id` and `/backups/:id/restore` (they answer
  404), the platform package's backup handlers and the KernelSettings
  `backup` namespace. The tables `v2_backup_config` and `v2_backup_record`
  stay, unused, until a cleanup migration drops them.
