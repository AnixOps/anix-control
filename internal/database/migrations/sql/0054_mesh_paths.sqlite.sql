-- 0054_mesh_paths (SQLite): the custom paths of the region networks
-- (internal/meshregion, docs/design/mesh-regions): the path of one source
-- member to a peer or a subnet through waypoint members, stored by member
-- identity (node:<node ref>, device:<id>); addresses are resolved when a
-- path is delivered.
CREATE TABLE `v5_mesh_path` (`id` integer PRIMARY KEY AUTOINCREMENT,`source_ref` text NOT NULL,`target_kind` text NOT NULL,`target` text NOT NULL,`via` text NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_path_target` ON `v5_mesh_path`(`source_ref`,`target`);
