### Changed

- PostgreSQL is the default database. A configuration file without a
  database driver now means PostgreSQL (`127.0.0.1:5432`, role and database
  `anix_control`), no longer SQLite; SQLite needs `database.driver: sqlite`
  and suits trials only. Control never falls back to SQLite: when PostgreSQL
  cannot be reached it stops with the server it tried and how to configure
  it. The `database.*` keys are operator keys again, no longer deprecated.
- `install.sh install` asks for PostgreSQL (`--database-dsn`); without it
  the install stops, and at a terminal it first asks whether to use SQLite.
  `--sqlite` chooses SQLite. `install.sh update` writes `driver: sqlite` for
  an install that ran SQLite without a driver, so it keeps its database.
  Configurations that name their driver (PostgreSQL or SQLite) are
  unchanged.
