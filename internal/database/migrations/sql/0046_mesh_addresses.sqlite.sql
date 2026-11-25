-- 0046_mesh_addresses (SQLite): the intranet mesh's addresses and names
-- (internal/meshregion, docs/design/mesh-regions): the global settings,
-- the personal devices with their pool addresses, the host groups, the
-- names gateway nodes publish, and the region changes.
CREATE TABLE `v5_mesh_settings` (`id` integer PRIMARY KEY AUTOINCREMENT,`name_suffix` text NOT NULL,`host_group_supernet` text NOT NULL,`host_group_prefix` integer NOT NULL,`reserved_ranges` text NOT NULL,`updated_at` datetime NOT NULL);
CREATE TABLE `v5_mesh_device` (`id` integer PRIMARY KEY AUTOINCREMENT,`region_id` integer NOT NULL,`device_key` text,`hostname` text NOT NULL,`address` text,`address_used_at` datetime,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_device_region_address` ON `v5_mesh_device`(`region_id`,`address`);
CREATE UNIQUE INDEX `idx_v5_mesh_device_device_key` ON `v5_mesh_device`(`device_key`);
CREATE TABLE `v5_mesh_host_group` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`segment` text NOT NULL,`gateway_ref` text NOT NULL,`standby_ref` text,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_host_group_segment` ON `v5_mesh_host_group`(`segment`);
CREATE INDEX `idx_v5_mesh_host_group_gateway_ref` ON `v5_mesh_host_group`(`gateway_ref`);
CREATE INDEX `idx_v5_mesh_host_group_standby_ref` ON `v5_mesh_host_group`(`standby_ref`);
CREATE TABLE `v5_mesh_device_name` (`id` integer PRIMARY KEY AUTOINCREMENT,`gateway_ref` text NOT NULL,`host_group_id` integer,`name` text NOT NULL,`address` text NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE INDEX `idx_v5_mesh_device_name_gateway_ref` ON `v5_mesh_device_name`(`gateway_ref`);
CREATE INDEX `idx_v5_mesh_device_name_host_group_id` ON `v5_mesh_device_name`(`host_group_id`);
CREATE TABLE `v5_mesh_region_change` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_ref` text,`device_id` integer,`from_region_id` integer NOT NULL,`to_region_id` integer NOT NULL,`source` text NOT NULL,`status` text NOT NULL,`reason` text NOT NULL,`note` text NOT NULL,`old_address` text NOT NULL,`new_address` text NOT NULL,`requested_by` integer,`decided_by` integer,`decided_at` datetime,`created_at` datetime NOT NULL);
CREATE INDEX `idx_v5_mesh_region_change_node_ref` ON `v5_mesh_region_change`(`node_ref`);
CREATE INDEX `idx_v5_mesh_region_change_device_id` ON `v5_mesh_region_change`(`device_id`);
CREATE INDEX `idx_v5_mesh_region_change_status` ON `v5_mesh_region_change`(`status`);
