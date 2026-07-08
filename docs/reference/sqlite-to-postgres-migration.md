# SQLite To PostgreSQL Migration Runbook

This runbook is for migrating an existing single-node SQLite panel to PostgreSQL. Treat production migration as a maintenance-window operation: the import command can truncate the target PostgreSQL tables when `-reset` is used.

Do not run the import against production data until the dry run, backups, rollback path, and post-migration checks are recorded for that maintenance window.

## Prerequisites

- A recent code checkout on the target host.
- Go available for `go run ./cmd/sqlite2postgres`, or a built `sqlite2postgres` binary.
- A readable SQLite database file, usually `config/data/v2board.db` or the value from `database.database`.
- A reachable PostgreSQL database with an empty or disposable target schema.
- A PostgreSQL target config file, or a DSN passed by `-target-dsn`.
- A maintenance window where the panel can be stopped to prevent writes during the final import.

Example PostgreSQL target config:

```yaml
database:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  database: "v2board"
  username: "v2board"
  password: "replace-with-secret"
```

## Backup First

Stop writes before the final backup. On systemd deployments:

```bash
sudo systemctl stop v2board.service
```

Create a consistent SQLite backup:

```bash
cd /home/dev/anixops/v2board_AnixOps
mkdir -p /root/v2board-migration
sqlite3 config/data/v2board.db ".backup '/root/v2board-migration/v2board.sqlite.$(date +%Y%m%d%H%M%S).db'"
```

If the PostgreSQL target already contains data, back it up before using `-reset`:

```bash
pg_dump --format=custom --file=/root/v2board-migration/v2board.pg.$(date +%Y%m%d%H%M%S).dump "$POSTGRES_DSN"
```

## Dry Run

The dry run opens both databases, lists SQLite tables, prints important row counts, and exits before schema migration, truncation, or import:

```bash
cd /home/dev/anixops/v2board_AnixOps
POSTGRES_DSN='host=127.0.0.1 user=v2board password=replace dbname=v2board port=5432 sslmode=disable TimeZone=Asia/Shanghai'

PATH=/usr/local/go/bin:$PATH go run ./cmd/sqlite2postgres \
  -source config/data/v2board.db \
  -target-dsn "$POSTGRES_DSN" \
  -dry-run
```

Expected dry-run signal:

```text
Source: config/data/v2board.db
Target: PostgreSQL
Tables: ...
dry-run: no PostgreSQL data changed
```

Record the displayed counts for these tables when present:

- `v2_user`
- `v2_plan`
- `v2_node`
- `v2_node_protocol`
- `v2_subscription_group`
- `v2_forward`
- `v2_system_config`

## Import

Run the import only after the dry run and backups are complete. `-reset` is required by design; without it the tool refuses to import.

```bash
cd /home/dev/anixops/v2board_AnixOps
PATH=/usr/local/go/bin:$PATH go run ./cmd/sqlite2postgres \
  -source config/data/v2board.db \
  -target-dsn "$POSTGRES_DSN" \
  -reset
```

What the import does:

- Runs the current GORM/schema helpers on PostgreSQL.
- Selects SQLite tables that also exist in PostgreSQL.
- Sorts import order by PostgreSQL foreign keys where possible.
- Truncates selected PostgreSQL tables with `RESTART IDENTITY CASCADE`.
- Copies rows table by table.
- Resets PostgreSQL sequences for tables with an `id` column.
- Runs forward runtime job schema and forward port binding repair helpers.

## Switch The Panel

Edit the active production config so `database.driver` is `postgres`, then restart:

```bash
sudo systemctl start v2board.service
sudo journalctl -u v2board.service -n 80 --no-pager
```

Verify:

```bash
curl -fsS http://127.0.0.1:8080/health
```

Then check core admin pages:

- login
- users
- plans
- nodes
- orders/payments
- subscription generation
- traffic hourly/ranking pages
- forward rules, if enabled

## Rollback

Rollback depends on how far the migration got.

If the import failed before switching the panel:

1. Leave the production config on SQLite.
2. Start the service again:

```bash
sudo systemctl start v2board.service
```

If the panel was switched to PostgreSQL and needs rollback:

1. Stop the service.
2. Restore the SQLite config values.
3. Restore the SQLite backup if the file was changed.
4. Start the service.

```bash
sudo systemctl stop v2board.service
sqlite3 config/data/v2board.db ".restore '/root/v2board-migration/v2board.sqlite.YYYYMMDDHHMMSS.db'"
sudo systemctl start v2board.service
```

If PostgreSQL already had production data before migration and `-reset` was used, restore the PostgreSQL backup before retrying:

```bash
pg_restore --clean --if-exists --dbname "$POSTGRES_DSN" /root/v2board-migration/v2board.pg.YYYYMMDDHHMMSS.dump
```

## Evidence To Keep

For each production migration, keep:

- SQLite backup path and checksum.
- Optional PostgreSQL backup path and checksum.
- Dry-run command and output.
- Import command and output.
- Pre/post row counts for important tables.
- Health check output after restart.
- Rollback decision and final database driver in production config.
