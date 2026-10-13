# Backups

Control backs itself up: every day, before every database migration and when
you ask. Each backup is one file, a **bundle**, that holds everything a new
machine needs to become this panel again: the database, the root key and the
configuration file. Control verifies every bundle by restoring it into a
scratch directory, and alerts when a bundle would not restore or when no
verified bundle is recent. Restoring is a command on the server, run while
Control is stopped; the console never restores.

Backups need **SQLite**. On PostgreSQL every backup command, the API and the
backup page say "backups need SQLite" and nothing is written: back the
database up with `pg_dump` and keep `root.key` with it.

Design: N32 and N33 in
[`../design/single-binary/README.md`](../design/single-binary/README.md)
(sections 3.8 and 3.9). Code: `internal/backup`.

## Where They Are

| Path | What |
|---|---|
| `<data_dir>/backups/` | The bundles (directory `0700`, files `0600`, owned by the user Control runs as) |
| `<data_dir>/backups/anix-control-<UTC time>-<type>.tar` | One bundle, for example `anix-control-20261013T034107Z-daily.tar` |
| `<data_dir>/tmp/` | Scratch space: the snapshot being written (`backup-*`), a bundle being verified (`verify-*`) or restored; removed afterwards, and by the retention when a crash left one behind for a day |
| `<data_dir>/control.lock` | The lock a running Control holds (see [Restore](#restore)) |
| `v5_backup` | One row per bundle with its verification ([`database-migrations.md`](database-migrations.md), `0005_backups`); the files are the truth, the rows follow them |

`data_dir` is `/var/lib/anix-control` unless the configuration sets another
([`configuration.md`](configuration.md)).

## What A Bundle Holds

A bundle is an uncompressed tar archive (the database compresses poorly and a
plain tar is readable with any tool) with these members, in this order:

| Member | Content |
|---|---|
| `manifest.json` | What the bundle is (below) |
| `database.sqlite` | A consistent snapshot of the SQLite database |
| `root.key` | The root key file of the data directory |
| `config.yaml` | The configuration file Control loaded; absent when it loaded none |

`manifest.json`:

| Field | Meaning |
|---|---|
| `format` | `anixops.backup/v1` |
| `program_version` | The Control version that wrote the bundle |
| `schema_version` | The newest migration recorded in the snapshot (`0` for a database from before the migration runner, such as a 4.2.x one) |
| `created_at` | When the snapshot was taken (UTC) |
| `type` | `daily`, `pre-migration`, `manual` or `verify` |
| `host_name` | The machine that wrote it |
| `domain` | The panel's domain, when configured |
| `root_key_fingerprint` | `sha256:` and the SHA-256 of a value derived from the root key with its own label (`anixops root key fingerprint v1`); never a digest of the key itself. Two bundles with the same fingerprint carry the same key |
| `members` | Name, size and SHA-256 of every other member |
| `row_counts` | Rows of `v2_user`, `v2_node` and `v2_order` in the snapshot |

Read it without unpacking: `tar -xOf <bundle> manifest.json`.

**Not encrypted.** A bundle sits on the same disk as the database and
`root.key`: anyone who can read the backup directory can read those files
too, so encrypting with a key kept beside the bundle would protect nothing.
The protection is the permissions. **Treat every bundle like the root key**:
it opens every key Control keeps. A copy that leaves the machine must be
encrypted by you until the encrypted off-site copy (M4) exists.

### How The Snapshot Is Taken

The snapshot is SQLite's `VACUUM INTO`, run on a dedicated short-lived
connection to the database file, not on one of the panel's own connections:

- `VACUUM INTO` reads the database in **one read transaction**, so the
  snapshot is one consistent state, with everything committed before it
  started and nothing after.
- In WAL mode a reader neither waits for the writer nor blocks it, so the
  panel's single writer connection keeps committing while the snapshot runs.
  The panel's read pool cannot run it (its connections are `query_only`,
  which refuses `VACUUM INTO` as a write), and running it on the writer would
  hold every write of the panel for the snapshot's duration.
- The output is a compacted database in rollback-journal mode, synced to disk.

Writing a bundle is atomic: the tar is written to a temporary file in the
backup directory, synced, and only then given its final name (a hard link
that never replaces an existing bundle); any failure leaves neither a partial
bundle nor the snapshot. A backup is refused when the file system has less
than three times the database's size free.

## When Control Backs Up

| Type | When |
|---|---|
| `daily` | Every day at the panel's own time between 03:00 and 05:00, local time. The minute comes from a hash of the domain (the host name without one), so the panels of a fleet do not all back up at once and one panel always backs up at the same minute. When the newest daily bundle is older than 26 hours at start (a new panel, or one that was down at its time), Control backs up two minutes after it starts |
| `pre-migration` | At a start that has a pending schema migration, before it runs, unless the database is new (empty). **When this backup fails, nothing is migrated and Control does not start**; the log says why (free space, permissions). `anix-control migrate` does the same |
| `manual` | `anix-control backup create`, or 立即备份 on 系统设置 → 备份 |
| `verify` | `anix-control backup create -type verify`: a bundle taken only to prove that backing up and verifying work end to end |

The daily backup runs in one Control process (the singleton workers' lease).
One backup or verification runs at a time per process; a second request is
told to try again.

## Verification

Control verifies a bundle by restoring it the way a restore would, into
`<data_dir>/tmp/verify-*`:

1. The file is a tar archive of regular files, `manifest.json` first, of
   format `anixops.backup/v1`, listing `database.sqlite` and `root.key`.
2. Every member the manifest lists is there with its size and SHA-256, and
   no other.
3. The snapshot opens and `PRAGMA integrity_check` answers `ok`.
4. Its migration records match the manifest's `schema_version`, and this
   Control may open it: a database a newer Control migrated fails with the
   start's own message.
5. `root.key` parses, has the manifest's fingerprint, and **opens what the
   snapshot holds sealed**: the module and forward link CA keys, else the
   ACME cache and a node group join token (the same check a start runs).
   A bundle whose key and database are not a pair fails here.
6. The key tables have the manifest's row counts.

When: after the daily backup, after a pre-migration or manual backup (the
pre-migration bundle when Control has started), at the backup worker's
start (the newest bundle not verified yet), and on demand with
`anix-control backup verify`. The result is recorded in `v5_backup`
(`verify_status` `passed` or `failed`, `verified_at`, `verify_error`).

Alerts ([`kernel-alerts.md`](kernel-alerts.md)):

| Alert | When |
|---|---|
| `backup_verify_failed` (critical) | The newest bundle that was verified failed its verification |
| `backup_stale` | No bundle that passed its verification is younger than 36 hours (and the database is older than that); critical after 72 hours |

The dashboard shows 上次验证通过的备份 (last verified backup) with its age,
and a warning past 36 hours.

## Retention

After each daily backup Control keeps:

- the newest **7** daily bundles;
- the newest daily bundle of each of the **4** most recent ISO weeks before
  the current one that have one;
- every pre-migration bundle of the last **30** days;
- the newest **3** manual bundles and the newest verify bundle;
- and, whatever its type and age, **the newest bundle that passed its
  verification**.

Everything else in the directory that is named like a bundle is removed;
other files are left alone. A bundle being read (verified, downloaded or
restored) is never removed: readers hold a shared lock on the file and the
retention takes it exclusively, without waiting, before it removes one; it
tries again next time. The numbers are built in; there is nothing to
configure.

## Commands

```sh
sudo anix-control backup create            # a manual bundle, verified at once
sudo anix-control backup create -type verify
sudo anix-control backup list              # -json for scripts
sudo anix-control backup verify            # the newest bundle
sudo anix-control backup verify /root/anix-control-20261013T034107Z-daily.tar
sudo anix-control backup restore <bundle> [--replace] [--data-dir /var/lib/anix-control]
```

They read the configuration the way Control does (`-config`, else
`ANIX_CONTROL_CONFIG`, else `config/config.yaml`, plus the `ANIX_CONTROL_*`
variables), open the database without migrating it, and run next to a
running Control (except `restore`). Run as root, every file and directory
they create gets the owner of its directory (the data directory's), so the
service can still verify and remove it; run them as the service user
otherwise. `verify` of a file outside
`<data_dir>/backups` (a copy from another machine) checks it and records
nothing.

| Exit status | Meaning |
|---|---|
| `0` | Done |
| `1` | The backup could not be written, or a verification found a problem |
| `2` | A usage or configuration error |
| `3` | Refused: PostgreSQL; Control is running (restore); a restore is running (the others); the target is not empty without `--replace`; the bundle's database was migrated by a newer Control |

## Restore

```sh
sudo systemctl stop anix-control
sudo anix-control backup restore /var/lib/anix-control/backups/anix-control-20261013T034107Z-daily.tar --replace
sudo systemctl start anix-control
```

`restore`:

1. **Refuses while Control runs.** A running Control (and `anix-control
   migrate`) holds a shared `flock` on `<data_dir>/control.lock` for its
   whole life; the restore takes it exclusively without waiting and stops
   with "the AnixOps Control service is running on this data directory"
   when it cannot. The kernel drops the lock when a process ends, however it
   ends. As a second check it asks the kernel, without changing anything
   (`F_GETLK`), whether another process holds SQLite's own lock on the
   database file or its `-shm` file, which every process with the database
   open does (a Control without the lock, a package host left behind). A
   Control that starts while a restore holds the lock stops with "a backup
   restore is running".
2. **Verifies the bundle** as above, into `<data_dir>/tmp`. A bundle that
   fails is refused. A bundle whose database a newer Control migrated is
   refused with the start's own message (`this database was migrated by a
   newer AnixOps Control ...`): install that version or a newer one.
3. **Refuses a target that is not empty**: an existing database file (or its
   `-wal`, `-shm`, `-journal`), `root.key`, or a configuration file other
   than the bundle's. With `--replace` it moves them into
   `<data_dir>/replaced-<UTC time>/` first. **Nothing is ever deleted**; a
   configuration file identical to the bundle's is left in place.
4. **Writes** the database to the configured SQLite path, `root.key` to
   `<data_dir>`, and the configuration to the file Control loads (when
   Control loads none, to `<data_dir>/config.yaml.restored`, and it says how
   to use it). Each file is synced and renamed into place, mode `0600`; run
   as root, each gets the owner of its directory.
5. **Starts nothing.** The next start migrates an older bundle's database
   as usual, after a pre-migration backup of it. A bundle of a 4.2.x
   database (schema `0`) is adopted at that start.

`--data-dir D` restores into another data directory (the same as
`ANIX_CONTROL_DATA_DIR=D`). The database goes where the configuration's
`database.database` says, as for the server.

### A New Machine

Install the same Control version (or a newer one) without starting it, put
the bundle on the machine, then run the three commands above (without
`--replace` on an empty machine). The domain, the root key and therefore the
CA, the ACME certificates, the node credentials and every sealed secret come
with the bundle: nodes reconnect without a new install command. No first
administrator link is generated: the administrators are in the database.

## The Console

系统设置 → 备份 lists the bundles with their type, size, time and
verification, shows the last verified backup and the next automatic one,
backs up on request (any administrator, `POST /api/v5/backups`) and
downloads a bundle. A bundle holds the root key, so **only a super
administrator may download one** (`GET /api/v5/backups/:id/download`), and
every download is in the audit log as `backups/download`. There is no
restore button. API: Swagger (`/api/v5/backups`).

## What Went Away

The backup settings of 4.x (automatic backups, interval, retention, what to
back up, local or S3 storage) ran nothing, and the online restore closed the
running process's database and copied a file over it. They are gone with
their routes (`/api/v2/admin/system/backup*`, which answer 404), the
`backup` settings namespace and the platform package's backup handlers. The
tables `v2_backup_config` and `v2_backup_record` stay, unused, until a
cleanup migration drops them; a file the old service wrote stays where it
was (by default `backups/` under the working directory) and is yours to
keep or delete.
