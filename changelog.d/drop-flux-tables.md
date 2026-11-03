### Removed

- Migration `0028_drop_flux_tables` drops the 4.1 flux forwarding tables
  (`v2_forward`, `v2_forward_tunnel`, `v2_forward_user_tunnel`,
  `v2_forward_port_binding`, `v2_speed_limit`, `v2_forward_runtime_job`,
  `v2_forward_traffic_cursor`, `v2_forward_agent_bridge_task`,
  `v2_forward_rule`, `v2_forward_route`, `v2_forward_log`, `v2_forward_stats`)
  and the v4.2 upgrade's records (`v4_forward_legacy_archive`,
  `v4_forward_legacy_node`, `v4_forward_legacy_drop`), with the
  `internal/forwardlegacy` package. `v2_forward_node` stays. **Irreversible:
  back the database up before upgrading.** Node operations on a flux forward,
  tunnel or legacy rule answer not found.
