-- 0047_mesh_enrollment (SQLite): devices without an Agent enroll in the
-- intranet mesh (internal/meshregion, docs/design/mesh-regions):
-- v5_mesh_enrollment (one-time credentials) and v5_mesh_device_token (the
-- enrolled devices' tokens), SHA-256 hashes only.
CREATE TABLE `v5_mesh_enrollment` (`id` integer PRIMARY KEY AUTOINCREMENT,`region_id` integer NOT NULL,`hostname` text NOT NULL,`token_hash` text NOT NULL,`expires_at` datetime NOT NULL,`used_at` datetime,`device_id` integer,`created_by` integer,`created_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_enrollment_token_hash` ON `v5_mesh_enrollment`(`token_hash`);
CREATE TABLE `v5_mesh_device_token` (`id` integer PRIMARY KEY AUTOINCREMENT,`device_id` integer NOT NULL,`token_hash` text NOT NULL,`last_seen_at` datetime,`created_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_device_token_device_id` ON `v5_mesh_device_token`(`device_id`);
CREATE UNIQUE INDEX `idx_v5_mesh_device_token_token_hash` ON `v5_mesh_device_token`(`token_hash`);
