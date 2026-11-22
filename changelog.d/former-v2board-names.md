### Changed

- The names Control kept from V2Board, the project it was forked from, are
  gone ([`docs/BRAND_MIGRATION.md`](docs/BRAND_MIGRATION.md), upgrade notes
  in [`docs/UPGRADE.md`](docs/UPGRADE.md)):
  - `/metrics` names its metrics `anixops_control_*` instead of `v2board_*`.
  - The default SQLite file is `config/data/control.db`, no longer
    `config/data/v2board.db`. `install.sh update` writes the old file into a
    SQLite configuration that names none, so the panel keeps its data.
  - The browser keeps the theme choice under `app.theme`.
  - CI's PostgreSQL databases are `anix_control_test`,
    `anix_control_migration_test` and `anix_control_restore_rehearsal`, and
    the Agent checkout is `anix-agent`; the Agent archives of the dev and
    hotfix releases no longer carry the `V2bX` link.

### Removed

- `install.sh` no longer migrates or detects the `/opt/v2board`,
  `/usr/local/v2board` and `v2board.service` layouts: the `preflight` and
  `migrate` commands, a `rollback` without `--version`, `--plan` and the
  `bin/v2board` link are gone (`update` deletes the link), as are the
  `V2BOARD_*` environment variables of the installer and the scripts, and
  `docs/guide/control-migration.md`. `deploy_panel.sh`,
  `scripts/migrate_latest_sqlite_to_postgres.sh` and `sqlite2postgres` no
  longer look for `v2board.service`, `/opt/v2board` or `/etc/v2board`, and
  `config/scripts/v2board.service` is deleted.
