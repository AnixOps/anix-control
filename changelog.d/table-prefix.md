### Changed

- The 51 tables Control kept from V2Board lose the `v2_` prefix: migration
  `0040_table_prefix` renames them to `v5_` (`v2_user` is `v5_user`), with
  their `idx_` indexes and, on PostgreSQL, the constraints and id sequences
  named after them; the node secret split's records follow. Data, columns,
  grants and the kernel API views stay. The official packages adopt the new
  names. Upgrade notes in [`docs/UPGRADE.md`](docs/UPGRADE.md#table-names-lose-the-v2_-prefix).
- `sqlite2postgres` refuses a SQLite source whose schema is behind the
  program's, instead of skipping the tables a later migration renamed.
