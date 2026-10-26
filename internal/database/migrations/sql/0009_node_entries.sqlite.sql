-- 0009_node_entries (SQLite): v5_node_entry, the addresses subscribers reach
-- a proxy node at beyond its host, such as a line's native IPv6 address or a
-- provider NAT address with its port map (internal/nodeentry,
-- model.NodeEntry).
CREATE TABLE `v5_node_entry` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_id` integer NOT NULL,`label` text NOT NULL,`kind` text NOT NULL,`host` text NOT NULL,`port_map` text NOT NULL DEFAULT "",`enabled` numeric NOT NULL DEFAULT true,`sort` integer NOT NULL DEFAULT 0,`created_at` datetime,`updated_at` datetime);
CREATE INDEX `idx_v5_node_entry_node_id` ON `v5_node_entry`(`node_id`);
