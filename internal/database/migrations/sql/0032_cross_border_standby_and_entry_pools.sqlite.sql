-- 0032_cross_border_standby_and_entry_pools (SQLite): the cross-border
-- standby channels (v5_l2_standby, internal/l2standby), the entry pool
-- policy and assignments (v5_entry_pool_policy, v5_user_entry_pool,
-- v5_node_entry.pool, internal/entrypool) and the clients' failure reports
-- (v5_client_failure_report, internal/clientreport); anx F-54 / F-55.
CREATE TABLE `v5_l2_standby` (`id` integer PRIMARY KEY AUTOINCREMENT,`transport_id` text NOT NULL,`tier` text NOT NULL,`source_node_id` integer NOT NULL,`exit_node_id` integer NOT NULL,`exit_host` text NOT NULL DEFAULT "",`relay_port` integer NOT NULL,`uuid` text NOT NULL,`reality_private_key` text NOT NULL DEFAULT "",`reality_public_key` text NOT NULL DEFAULT "",`reality_short_id` text NOT NULL DEFAULT "",`reality_server_name` text NOT NULL DEFAULT "",`reality_dest` text NOT NULL DEFAULT "",`xhttp_domain` text NOT NULL DEFAULT "",`xhttp_path` text NOT NULL DEFAULT "",`active` numeric NOT NULL DEFAULT false,`activated_at` datetime,`created_at` datetime,`updated_at` datetime);
CREATE INDEX `idx_v5_l2_standby_source_node_id` ON `v5_l2_standby`(`source_node_id`);
CREATE INDEX `idx_v5_l2_standby_transport_id` ON `v5_l2_standby`(`transport_id`);
CREATE INDEX `idx_v5_l2_standby_exit_node_id` ON `v5_l2_standby`(`exit_node_id`);
CREATE TABLE `v5_entry_pool_policy` (`id` integer,`shards` integer NOT NULL DEFAULT 0,`quarantine_days` integer NOT NULL DEFAULT 0,`quarantine_pool` text NOT NULL DEFAULT "quarantine",`updated_at` datetime NOT NULL,PRIMARY KEY (`id`));
CREATE TABLE `v5_user_entry_pool` (`user_id` integer,`pool` text NOT NULL,`reason` text NOT NULL DEFAULT "",`updated_at` datetime NOT NULL,PRIMARY KEY (`user_id`));
CREATE INDEX `idx_v5_user_entry_pool_pool` ON `v5_user_entry_pool`(`pool`);
CREATE TABLE `v5_client_failure_report` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` integer NOT NULL,`hour` datetime NOT NULL,`node` text NOT NULL,`network` text NOT NULL,`server` text NOT NULL DEFAULT "",`proto` text NOT NULL DEFAULT "",`platform` text NOT NULL DEFAULT "",`client` text NOT NULL DEFAULT "",`ok` integer NOT NULL DEFAULT 0,`fallback` integer NOT NULL DEFAULT 0,`timeout` integer NOT NULL DEFAULT 0,`refused` integer NOT NULL DEFAULT 0,`reset` integer NOT NULL DEFAULT 0,`after_handshake` integer NOT NULL DEFAULT 0,`keepalive_lost` integer NOT NULL DEFAULT 0,`other` integer NOT NULL DEFAULT 0,`stopped` numeric NOT NULL DEFAULT false,`clock_skew_secs` integer,`updated_at` datetime NOT NULL);
CREATE INDEX `idx_v5_client_failure_report_hour` ON `v5_client_failure_report`(`hour`);
CREATE UNIQUE INDEX `idx_v5_client_failure_report_key` ON `v5_client_failure_report`(`user_id`,`hour`,`node`,`network`);
ALTER TABLE `v5_node_entry` ADD `pool` text NOT NULL DEFAULT "";
