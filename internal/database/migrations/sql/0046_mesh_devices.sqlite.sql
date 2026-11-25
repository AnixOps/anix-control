-- 0046_mesh_devices (SQLite): mesh devices without an Agent
-- (internal/meshregion, docs/design/mesh-regions): v5_mesh_enrollment (the
-- one-time enrollment credentials, token hashes only), v5_mesh_device (the
-- enrolled devices, token hashes only) and v5_mesh_region_change (the
-- devices' region change requests).
CREATE TABLE `v5_mesh_enrollment` (`id` integer PRIMARY KEY AUTOINCREMENT,`region_id` integer NOT NULL,`kind` text NOT NULL,`name` text NOT NULL,`token_hash` text NOT NULL,`expires_at` datetime NOT NULL,`used_at` datetime,`device_id` integer,`created_by` integer,`created_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_enrollment_token_hash` ON `v5_mesh_enrollment`(`token_hash`);
CREATE TABLE `v5_mesh_device` (`id` integer PRIMARY KEY AUTOINCREMENT,`region_id` integer NOT NULL,`kind` text NOT NULL,`name` text NOT NULL,`device_key` text NOT NULL,`token_hash` text NOT NULL,`address` text NOT NULL,`last_seen_at` datetime,`revoked_at` datetime,`created_at` datetime NOT NULL);
CREATE INDEX `idx_v5_mesh_device_region_id` ON `v5_mesh_device`(`region_id`);
CREATE UNIQUE INDEX `idx_v5_mesh_device_device_key` ON `v5_mesh_device`(`device_key`);
CREATE UNIQUE INDEX `idx_v5_mesh_device_token_hash` ON `v5_mesh_device`(`token_hash`);
CREATE TABLE `v5_mesh_region_change` (`id` integer PRIMARY KEY AUTOINCREMENT,`device_id` integer NOT NULL,`from_region_id` integer NOT NULL,`to_region_id` integer NOT NULL,`reason` text NOT NULL,`state` text NOT NULL,`decided_by` integer,`decided_at` datetime,`created_at` datetime NOT NULL);
CREATE INDEX `idx_v5_mesh_region_change_device_id` ON `v5_mesh_region_change`(`device_id`);
