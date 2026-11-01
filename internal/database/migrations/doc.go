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
//     schema with the one the models build, and adopts a database at the
//     baseline; a migration that changes a model, creates a table or drops
//     one with its model records that in the legacy oracle, in
//     postBaselineIndexes, laterModels or retiredKernelModels (see
//     docs/reference/database-migrations.md, "Changing a model").
//     TestUpgradedSchemaEqualsFresh checks the upgrade from every earlier
//     step, and TestTemplateMigrationAppliesSQLAndGoInOneTransaction is the
//     template for a step's own test.
//
// # The steps
//
//   - 0001_baseline: the schema of release 4.2.1.
//   - 0002_server_log_buckets: hourly v2_server_log buckets.
//   - 0003_acme_cache: the panel certificate store (v5_acme_cache).
//   - 0004_drop_route_modes: drops the route-mode revisions and the shadow
//     mismatch samples, and removes the stored route modes from the Control
//     package configurations, audited (drop_route_modes.go).
//   - 0005_backups: the backup bundles (v5_backup).
//   - 0006_destination_stats: each user's traffic by destination
//     (v5_destination_stat).
//   - 0007_egress_routes: the domain egress routes between proxy nodes
//     (v5_egress_route).
//   - 0008_reality_target_probes: the latest probe of each candidate
//     REALITY target from each proxy node (v5_reality_target_probe).
//   - 0009_reality_target_auto: each proxy node's automatic REALITY target
//     selection (v5_reality_target_auto).
//   - 0010_node_entries: the addresses subscribers reach a proxy node at
//     beyond its host (v5_node_entry).
//   - 0011_subscription_group_protocols: each subscription group's order and
//     display names of its node protocols (v5_subscription_group_protocol).
//   - 0012_forward_route_subscriptions: the forward routes published in
//     subscriptions at their entry nodes (v5_forward_route_subscription).
//   - 0013_iwan_lines: the Panabit iWAN lines (v5_iwan_line) and the line of
//     an iWAN node entry (v5_node_entry.iwan_line_id).
//   - 0014_forward_latency: the forward routes' latency history
//     (v4_kernel_forward_latency).
//   - 0015_forward_dns_vantage: the vantage selectors of the entry HA DNS
//     bindings (v4_kernel_forward_dns_vantage).
//   - 0016_network_model: the device network model, its sealed ANX-Line
//     link keys and each device's plan generation and reported facts
//     (v4_kernel_network_model, v4_kernel_network_transport_key,
//     v4_kernel_network_device).
//   - 0017_node_groups_make_regular_nodes: every node group makes regular
//     nodes; former forward groups become proxy groups without a template.
//   - 0018_node_network: each device's own network settings, which the
//     network model's automatic components follow (v4_kernel_node_network).
//   - 0020_node_groups_without_templates: a node group no longer has a kind,
//     a protocol template or subscription groups; joining nodes stay clean.
//   - 0021_drop_load_balancer: the load balancer feature is removed; its
//     table v2_load_balancer is dropped.
//   - 0022_reverse_proxies: the panel HTTP(S) reverse proxies
//     (v5_reverse_proxy).
//   - 0023_node_uplinks: each device's uplinks and the uplink each next
//     device of its routes gets (v4_kernel_node_uplinks).
//   - 0024_traffic_categories: the administrator's traffic category rules
//     and the remote category lists (v5_traffic_category_rule,
//     v5_traffic_category_source).
//   - 0025_drop_registration_keys: node registration is removed; the
//     registration keys (v2_authorized_key, kapi_registration_key_v1) and
//     their moved credentials are dropped.
//   - 0026_drop_server_hysteria: Hysteria v1 is removed; the Xboard-era
//     v2_server_hysteria table a database imported from Xboard may hold is
//     dropped.
//   - 0027_drop_node_credentials: nodes authenticate with mTLS only; the
//     node API keys, shared secrets and forward node tokens (columns of
//     v2_node and v2_forward_node), the clean agents
//     (v2_forward_clean_agent), the moved credentials
//     (v4_kernel_node_credential) and their kernel API views are dropped.
package migrations
