-- 0048_mesh_full_nodes (SQLite): mesh devices without an Agent that run as
-- full nodes (internal/meshregion, docs/design/mesh-regions): the
-- credential, the device, its fixed address and the subnets an
-- administrator confirmed.
CREATE TABLE `v5_mesh_full_node` (`id` integer PRIMARY KEY AUTOINCREMENT,`enrollment_id` integer NOT NULL,`device_id` integer,`address` text NOT NULL,`subnets` text NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_full_node_device_id` ON `v5_mesh_full_node`(`device_id`);
CREATE UNIQUE INDEX `idx_v5_mesh_full_node_enrollment_id` ON `v5_mesh_full_node`(`enrollment_id`);
