-- 0028_drop_flux_tables (PostgreSQL): the flux forwarding of 4.1 is gone and its
-- tables were empty in production: the flux forwards, tunnels, user tunnel
-- grants, speed limits, port bindings, runtime jobs, traffic cursors and
-- agent bridge tasks, the pre-flux rules, smart routes, connection log and
-- hourly stats, and the v4.2 upgrade's archive, node check and drop records
-- (internal/forwardlegacy). Dependents first. v2_forward_node stays: it is
-- the forward node inventory. Irreversible: back the database up first.
DROP TABLE IF EXISTS "v2_forward_port_binding";
DROP TABLE IF EXISTS "v2_forward_traffic_cursor";
DROP TABLE IF EXISTS "v2_forward_agent_bridge_task";
DROP TABLE IF EXISTS "v2_forward_runtime_job";
DROP TABLE IF EXISTS "v2_forward_user_tunnel";
DROP TABLE IF EXISTS "v2_speed_limit";
DROP TABLE IF EXISTS "v2_forward";
DROP TABLE IF EXISTS "v2_forward_tunnel";
DROP TABLE IF EXISTS "v2_forward_rule";
DROP TABLE IF EXISTS "v2_forward_route";
DROP TABLE IF EXISTS "v2_forward_log";
DROP TABLE IF EXISTS "v2_forward_stats";
DROP TABLE IF EXISTS "v4_forward_legacy_drop";
DROP TABLE IF EXISTS "v4_forward_legacy_node";
DROP TABLE IF EXISTS "v4_forward_legacy_archive";
