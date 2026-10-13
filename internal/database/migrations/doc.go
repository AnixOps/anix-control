// Package migrations prepares Control's database schema: an ordered list of
// numbered steps, applied once each and recorded with the program version
// (design: docs/design/single-binary/README.md section 3.3; operator and
// developer reference: docs/reference/database-migrations.md).
//
// # Steps
//
// A step is a pair of SQL files in sql/, NNNN_name.sqlite.sql and
// NNNN_name.postgres.sql (until PostgreSQL is removed, every step has both),
// numbered from 0001 without gaps, and optionally a Go function for the same
// number in goSteps that runs after the SQL in the same transaction, for data
// the statements cannot convert. A statement ends with a semicolon at the end
// of a line; lines starting with "--" are comments.
//
// 0001_baseline is the schema a fresh Control built before this package: the
// statements that start executed on an empty database, recorded once from the
// legacy oracle (legacy_oracle_test.go) and frozen. The kernel API views are
// not steps: packagestore.EnsureKernelAPIViews creates them at every start,
// because some exist only once the node secret split is finalized.
//
// # Running
//
// Migrate runs at every start (cmd/server bootstrapDatabase, inside the
// PostgreSQL bootstrap lock) and in the migrate command:
//
//   - In an empty database it creates v5_schema_migration and v5_control_meta
//     and applies every step.
//   - A database an earlier Control created (tables, no v5_schema_migration)
//     is adopted once: see adopt.go. The runner completes it to the baseline
//     additively, checks that every baseline table, column and index is
//     present, records 0001_baseline and applies the later steps.
//   - Otherwise it applies the steps the database has not recorded yet.
//
// Each step runs in one transaction (SQLite and PostgreSQL both run DDL in
// transactions), together with its v5_schema_migration row (version, name,
// checksum, applied_at, program_version) and the schema_version key of
// v5_control_meta. The run then records program_version.
//
// Migrate refuses to start, with a message that says what to do, when the
// database has a step this program does not know (a newer Control migrated
// it), when an applied step's name or checksum differs from this program's
// (a released migration was edited), when a step is missing below the
// newest applied one, or when min_program_version is newer than this
// program.
//
// # Adding a migration
//
//  1. Change the model (internal/model) as the code needs it.
//  2. Add sql/NNNN_name.sqlite.sql and sql/NNNN_name.postgres.sql with the
//     next number. Write the statements GORM would generate for the change
//     (the baseline shows the exact column and index syntax of each
//     dialect). Never edit a step that was merged: its checksum is in every
//     database that applied it.
//  3. If data must be converted, register a Go function for the number in
//     goSteps; it receives the step's transaction and the dialect. If code
//     older than this release would misread the converted data, set
//     minProgramVersions for the number to this release's version.
//  4. Run the tests. TestFreshSchemaEqualsTheLegacySchema compares the fresh
//     schema with the one the models build; the first migration that
//     changes a model also refreshes its adoption half (see
//     docs/reference/database-migrations.md, "Changing a model").
//     TestUpgradedSchemaEqualsFresh checks the upgrade from every earlier
//     step, and TestTemplateMigrationAppliesSQLAndGoInOneTransaction is the
//     template for a step's own test.
package migrations
