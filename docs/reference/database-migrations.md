# Database Migrations

Control prepares its database schema with an ordered list of numbered
migrations (`internal/database/migrations`), applied once each and recorded
with the program version. It does the same in every environment: `env` no
longer changes how the schema is built. Design: N36 in
[`docs/design/single-binary/README.md`](../design/single-binary/README.md)
section 3.3.

## What Runs At Start

Every start (and `anix-control migrate`, which runs only this and the seeds)
calls the runner before anything else uses the database. On PostgreSQL it runs
inside the bootstrap advisory lock, so two Control processes never migrate at
once; on SQLite immediate transactions serialize them.

| Database | What the runner does |
|---|---|
| Empty | Applies every migration and creates the runner's two tables |
| Prepared by an earlier Control (tables, no `v5_schema_migration`) | Adopts it once (below), then applies the later migrations |
| Already migrated | Applies the migrations it has not recorded yet, if any |

Each migration runs in one transaction together with its record, on SQLite
and PostgreSQL alike: a failing migration leaves nothing behind, and the
start stops with the error.

Before it applies a pending step to a database that is not empty, the start
takes a `pre-migration` backup on SQLite ([`backups.md`](backups.md)); when
that backup fails, nothing is migrated and the start stops with the reason.

After the migrations the start seeds what every database has, as before: the
default service scopes, the official plugin catalog and the plugin target
locks; the node secret split state rows; the module and forward link CAs; the
kernel API views (`kapi_*`); the plugin trust root and the identity package;
the defaults. It creates no administrator: the first one claims the panel
with a one-time link, kept as a hash in `v5_control_meta` (`admin_claim`,
below). While the flux tables exist it also
normalizes the forward runtime settings and rebuilds the flux port bindings.

## The Runner's Tables

`v5_schema_migration`, one row per applied migration:

| Column | Meaning |
|---|---|
| `version` | The migration's number (`1` for `0001_baseline`) |
| `name` | Its name (`baseline`) |
| `checksum` | `sha256:` and the SHA-256 of the migration's SQL file for this database's dialect (line endings normalized), with `+go` when a Go function follows the SQL |
| `applied_at` | When it was applied (UTC) |
| `program_version` | The Control version that applied it |

`v5_control_meta`, key and value:

| Key | Meaning |
|---|---|
| `schema_version` | The newest applied migration |
| `program_version` | The Control version that last opened the database |
| `min_program_version` | Only once a migration declared one: the oldest Control that may open the database |
| `created_at` | When the runner first prepared the database (RFC 3339, UTC) |
| `origin` | `fresh` (created by the runner) or `adopted` (an earlier Control created it) |
| `admin_claim` | Only while a first-administrator claim link is pending: `{"sha256", "expires_at", "created_at"}` as JSON, the SHA-256 of the token and its expiry, never the token (`anix-control admin claim-link` writes it, a claim deletes it with the administrator's creation) |

Check them with `sqlite3 control.db 'SELECT * FROM v5_schema_migration; SELECT * FROM v5_control_meta;'`
or the same `SELECT`s in `psql`.

## When Control Refuses To Start

The runner refuses the database, and Control exits before opening any
listener, in these cases. The log line says which.

| Message starts with | Cause | What to do |
|---|---|---|
| `this database was migrated by a newer AnixOps Control` | The database has a migration this program does not know: a newer Control ran on it (an upgrade, then a rollback of the binary) | Install the Control version the message names, or a newer one; or restore the backup taken before the upgrade and run the older version on it |
| `this database needs AnixOps Control X or newer` | `min_program_version` is newer than this program | The same |
| `schema migration NNNN is recorded as ... with checksum ...` | A released migration file was edited after it was applied | A build problem, never an operator one: run a release build, and report it |
| `schema migration NNNN is not applied, but the later migration ... is` | A migration was added below the newest released one | The same |
| `the database is not empty, but it is not a database an AnixOps Control start prepared` | Adoption found a database without the tables every Control 4.x start created | Point `database.*` at an empty database, or restore a complete backup |
| `adopting the existing database failed at ...` / `could not be brought to the 0001_baseline schema` | Adoption could not add a table, column or index (for example a unique index over duplicate rows) | Fix the named problem or restore a backup; nothing was changed |

A Control release from before this change (4.2.x and older) does not know the
runner's tables: it starts on a migrated database the way it always did
(creates missing tables, runs its schema steps). From this release on, an
older program refuses a database a newer one migrated.

## Adopting An Existing Database

A database created by any 4.x release or by `go_dev` before this change has
the application tables but no `v5_schema_migration`. At its first start with
the runner it is adopted, once, in one transaction:

1. **Recognize.** The database must have the tables every Control 4.x start
   created, in production as in development: `v2_user`, `v2_node`,
   `v2_wireguard_peer` and the 29 `v3_kernel_*` tables of 4.0.0. Otherwise the
   runner refuses it and lists the anchors it lacks and every other baseline
   table it lacks.
2. **Complete.** Additively, the way the start before the runner did: it
   creates the baseline tables the database lacks (with their indexes), adds
   the baseline columns its existing tables lack (the node group, metrics and
   approval columns of `v2_node` and `v2_forward_node`, the stats columns of
   `v2_server_log`, and the columns a production start never added to an
   existing table), creates the missing indexes, and drops the plain unique
   topology name index older schemas carry. Before it creates the partial
   unique index on active flux runtime jobs it marks duplicate active jobs of
   a forward failed, keeping the running or newest one. Once
   `anix-control forward legacy drop` removed the flux tables, it does not
   create them again.
3. **Check.** Every baseline table, column and index must now be present, or
   the runner refuses and lists what is missing; the transaction rolls back,
   so nothing changed.
4. **Record.** It records `0001_baseline` with the checksum a fresh database
   gets and `origin = adopted`, then applies the later migrations.

Adoption never changes or drops an existing column or table. What still
differs from the baseline (a column's type, nullability or default, a
constraint, a table or column the baseline does not have) is kept and logged
once as `Database schema: kept a difference from the baseline: ...`. The
kernel API views are not part of the comparison: they are created after the
migrations at every start.

## How The Baseline Is Expressed

`0001_baseline` is the schema a fresh install created before the runner
(`go_dev` 4eb87485, release 4.2.1), as two SQL files:

- `sql/0001_baseline.sqlite.sql` and `sql/0001_baseline.postgres.sql`: the
  `CREATE TABLE` and `CREATE INDEX` statements the legacy start executed on an
  empty database (154 tables, the flux tables included), recorded once per
  dialect from that start and frozen. The tables keep their creation order,
  then come the indexes by table and name.
- The legacy start is kept in the tests as the oracle
  (`legacy_oracle_test.go`): the application models, the kernel models and the
  former `Ensure*` schema steps. `TestWriteBaselineFromTheLegacyOracle` shows
  how the files were produced; it writes to a directory you name, never into
  `sql/`.

Plain SQL per dialect keeps each migration's checksum meaningful and replays
the same DDL whatever the models later become. Until PostgreSQL is removed,
every migration has both files; afterwards only the SQLite file remains.

## The Migrations

| Step | What it does |
|---|---|
| `0001_baseline` | The schema of release 4.2.1 (below) |
| `0002_server_log_buckets` | `v2_server_log` holds hourly buckets: rows of one key are merged, then the unique bucket index is created |
| `0003_acme_cache` | Creates `v5_acme_cache`, the store of the panel's own certificates (the ACME account key, the certificates with their keys, challenge answers in flight, the self-signed fallback; entries holding a key are sealed) |
| `0004_drop_route_modes` | Drops `v4_kernel_route_mode_revision` and `v4_kernel_shadow_mismatch_sample`, and removes the stored route-mode map (the `routes` key) from every Control package configuration in a new revision, writing each removed map to `v2_operation_log` first (action `route_modes_removed`, user `system/migration`). Route modes come only from the rehearsed list since ([`../UPGRADE.md`](../UPGRADE.md#plugin-center-reduced-to-package-status)) |
| `0005_backups` | Creates `v5_backup`, one row per backup bundle in `<data_dir>/backups` with the result of its latest verification ([`backups.md`](backups.md)) |
| `0006_destination_stats` | Creates `v5_destination_stat`, each user's traffic by destination domain, node and hour ([`destination-stats.md`](destination-stats.md)) |

## Adding A Migration

1. Change the model in `internal/model` as the code needs it.
2. Add `internal/database/migrations/sql/NNNN_name.sqlite.sql` and
   `NNNN_name.postgres.sql` with the next number (`0002`, ...). Write the
   statements GORM would generate for the change; the baseline shows each
   dialect's column and index syntax. A statement ends with `;` at the end of
   a line; `--` starts a comment line. Never edit a migration that was merged.
3. To convert data, register a Go function for the number in `goSteps`
   (`steps.go`); it runs after the SQL, in the same transaction, with the
   dialect. If code older than this release would misread the converted
   data, set `minProgramVersions` for the number to this release's version.
4. Add a test modelled on `TestTemplateMigrationAppliesSQLAndGoInOneTransaction`
   (a database at the previous migration, with data, upgraded).
5. Run `GOWORK=off go test ./internal/database/migrations/`, with
   `ANIX_TEST_POSTGRES_DSN` for PostgreSQL. `TestFreshSchemaEqualsTheLegacySchema`
   fails when a model and the migrations disagree;
   `TestUpgradedSchemaEqualsFresh` checks the upgrade from every earlier
   migration.

### Changing A Model

The legacy oracle (`legacy_oracle_test.go`) prepares a database in two ways:

- `legacyModelsPrepare` is the legacy start with today's models, plus the
  tables later migrations create (`laterModels()`).
- `legacyPrepare` is the legacy start as it was at the baseline: the same
  model lists less what later migrations added to those models
  (`postBaselineIndexes`), without the later tables, plus the tables later
  migrations dropped with their models (`retiredKernelModels()`). The
  adoption tests build their databases with it.

`TestFreshSchemaEqualsTheLegacySchema` has two halves. The first compares the
fresh schema with the one `legacyModelsPrepare` builds: a model changed or
added without a migration fails there. The second adopts a database
`legacyPrepare` built, applies the migrations after the baseline and expects
the fresh schema exactly. A migration therefore records in the oracle what it
changes:

| The migration | The oracle |
|---|---|
| Adds an index to a model the baseline has (`0002_server_log_buckets`) | List the index in `postBaselineIndexes`: `legacyPrepare` drops it after building from the models (a column a migration adds is dropped the same way, after its index) |
| Creates the table of a new model (`0003_acme_cache`) | List the model in `laterModels()`: `legacyModelsPrepare` creates it, `legacyPrepare` does not |
| Drops a table the oracle still creates (the flux tables in the forwarding cleanup) | Remove it from the oracle's lists |
| Drops a table together with its model (`0004_drop_route_modes`) | Keep a frozen copy of the model, as the baseline had it, in `retiredKernelModels()`: `legacyPrepare` creates it, `legacyModelsPrepare` does not (today's models no longer have it) |

## Tests And PostgreSQL

The tests run on SQLite and, when `ANIX_TEST_POSTGRES_DSN` names a database
whose name contains `test`, on PostgreSQL in throwaway schemas. CI runs them
on PostgreSQL in "Package Storage PostgreSQL".

Adoption is tested on the shapes that exist: a database the legacy start
built (production and development), one an older 4.x left (built from it by
removing what later releases added, with duplicate active jobs and the old
topology index), one after `forward legacy drop`, partial and unrelated
schemas, and a completion that fails. `testdata/releases` adds the schemas
4.0.0 (development, and its partial production start), 4.1.0 and 4.2.0
actually left in an empty SQLite file, built from the tags; each adopts to the
fresh schema with only the obsolete `v2_node_group` kept.
