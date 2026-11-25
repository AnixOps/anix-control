-- 0051_mesh_external_bridges (SQLite): the mesh's external network bridge
-- nodes (internal/meshregion, docs/design/mesh-regions; spec section 15.12,
-- R36) with their external interface, advertised routes and allow list,
-- and the addresses and segments held back until a bridge node confirms it
-- no longer lets them through.
CREATE TABLE `v5_mesh_external_bridge` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_ref` text NOT NULL,`interface` text NOT NULL,`routes` text NOT NULL,`allow` text NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_external_bridge_node_ref` ON `v5_mesh_external_bridge`(`node_ref`);
CREATE TABLE `v5_mesh_bridge_hold` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_ref` text NOT NULL,`region_id` integer NOT NULL,`prefix` text NOT NULL,`created_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_bridge_hold_entry` ON `v5_mesh_bridge_hold`(`node_ref`,`region_id`,`prefix`);
