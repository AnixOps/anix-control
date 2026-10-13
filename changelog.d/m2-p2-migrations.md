### Added

- Control builds its database schema from ordered, numbered migrations
  (`internal/database/migrations`) and records them in two new tables:
  `v5_schema_migration` (version, name, checksum, time, program version) and
  `v5_control_meta` (schema version, the program version that last opened the
  database, creation time). The first migration, `0001_baseline`, is the schema
  a fresh 4.2.1 install creates. See `docs/reference/database-migrations.md`.

### Changed

- The schema no longer depends on `env`: development and test stop running
  GORM's AutoMigrate at every start, and production stops creating only the
  missing tables. The first start of this release (or `anix-control migrate`)
  adopts an existing database once: it adds the tables, columns and indexes a
  fresh install has and the database lacks, never changes or drops one, and
  logs what it added and the differences it kept. A non-empty database that a
  Control start did not prepare is refused and left unchanged.
- Control refuses to start on a database a newer Control migrated (an unknown
  migration, or a `min_program_version` above it) and on a released migration
  that was changed, and says what to do: install that version or a newer one,
  or restore the backup taken before the upgrade. Rolling back the binary
  after an upgrade now needs that backup (`docs/UPGRADE.md`, "Database
  Migrations").
- `cmd/sqlite2postgres` prepares the PostgreSQL schema with the same
  migrations and keeps the target's own migration records.

### Fixed

- `cmd/sqlite2postgres` no longer fails on the kernel tables keyed by a text
  id (`v3_kernel_plugin`, ...) when it resets the sequences.
