-- 0024_traffic_categories: the administrator's traffic category rules
-- (v5_traffic_category_rule) and the remote category lists
-- (v5_traffic_category_source, internal/destinationstats).
CREATE TABLE `v5_traffic_category_rule` (`id` integer PRIMARY KEY AUTOINCREMENT,`kind` text NOT NULL,`value` text NOT NULL,`category` text NOT NULL,`note` text NOT NULL DEFAULT "",`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_traffic_category_rule_key` ON `v5_traffic_category_rule`(`kind`,`value`);
CREATE TABLE `v5_traffic_category_source` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`url` text NOT NULL,`category` text NOT NULL,`enabled` numeric NOT NULL DEFAULT true,`refresh_hours` integer NOT NULL DEFAULT 24,`entries_json` text NOT NULL DEFAULT "",`entry_count` integer NOT NULL DEFAULT 0,`fetched_at` datetime,`fetch_error` text NOT NULL DEFAULT "",`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_traffic_category_source_name` ON `v5_traffic_category_source`(`name`);
