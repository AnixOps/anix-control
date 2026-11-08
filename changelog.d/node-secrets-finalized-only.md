### Removed

- The node secret split is finalized-only. The commands
  `anix-control node-secrets backfill`, `verify`, `phase`, `finalize` and
  `unsplit` are removed (they exit 2 with the usage); `node-secrets status`
  and `node-secrets validate` stay. Update runbooks and scripts that call
  them.
- The metric `anixops_node_secrets_fallback_total` is removed: readers no
  longer fall back to the legacy columns. Remove it from dashboards and alert
  rules; `anixops_node_secrets_invalid_total` stays.
- The alert kinds `node_secrets_split_stalled` and
  `node_secrets_finalize_interrupted` are removed. Open alerts of these kinds
  are resolved at the next scan.

### Changed

- Control refuses to start while a node secret split table
  (`v2_node`, `v2_node_protocol`, `v2_wireguard_peer`) holds rows and is not
  finalized with `finalized_at` set. The check runs before the migrations and
  changes nothing. Finish the split on 5.0.2 first (`docs/UPGRADE.md`,
  "Upgrading To 5.0.3"). An empty table and a fresh database are marked
  finalized at start.
- Migration `0031_drop_legacy_originals` deletes the original JSON documents
  finalize kept for `unsplit` (`v4_kernel_protocol_secret` rows of scope
  `legacy_original`). The pre-migration backup runs; going back to 5.0.2
  means restoring it.
- `GetCapabilities` of `KernelNodeOps` answers `FINALIZED` for the three
  split tables, and the kernel API views and adoptions of the split's
  credential-free remainder are always available.
