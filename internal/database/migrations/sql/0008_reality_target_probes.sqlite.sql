-- 0008_reality_target_probes (SQLite): v5_reality_target_probe, the latest
-- probe of each candidate REALITY target from each proxy node
-- (internal/realitytarget, model.RealityTargetProbe).
CREATE TABLE `v5_reality_target_probe` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_id` integer NOT NULL,`server_name` text NOT NULL,`port` integer NOT NULL,`verdict` text NOT NULL,`score` integer NOT NULL DEFAULT 0,`result` text NOT NULL,`probed_at` datetime NOT NULL,`created_at` datetime,`updated_at` datetime);
CREATE UNIQUE INDEX `idx_v5_reality_target_probe_target` ON `v5_reality_target_probe`(`node_id`,`server_name`,`port`);
