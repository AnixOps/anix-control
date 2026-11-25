-- 0044_node_temporary_proxies (SQLite): v5_node_temporary_proxy, the
-- SOCKS5 proxies a proxy node serves for a limited time on an internal
-- address (internal/tempproxy, model.NodeTemporaryProxy).
CREATE TABLE `v5_node_temporary_proxy` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_id` integer NOT NULL,`listen` text NOT NULL,`port` integer NOT NULL,`username` text NOT NULL,`password` text NOT NULL,`expires_at` datetime NOT NULL,`closed_at` datetime,`created_by` integer,`created_at` datetime NOT NULL);
CREATE INDEX `idx_v5_node_temporary_proxy_node_id` ON `v5_node_temporary_proxy`(`node_id`);
CREATE INDEX `idx_v5_node_temporary_proxy_expires_at` ON `v5_node_temporary_proxy`(`expires_at`);
