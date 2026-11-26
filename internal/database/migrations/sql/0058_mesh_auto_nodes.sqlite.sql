-- 0058_mesh_auto_nodes (SQLite): the Agent nodes an administrator added to the
-- mesh without a region (internal/meshregion, docs/design/mesh-regions). Such
-- a node gets no network until its status reports the latencies of the
-- regions, from which the panel chooses its region and removes the row.
CREATE TABLE `v5_mesh_auto_node` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_ref` text NOT NULL,`created_by` integer,`created_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_auto_node_node_ref` ON `v5_mesh_auto_node`(`node_ref`);
