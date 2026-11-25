-- 0045_mesh_regions (SQLite): the intranet mesh's panel side
-- (internal/meshregion, model.Mesh*): the global settings, the regions,
-- the members and their virtual addresses, the host groups, the names the
-- gateways publish and the region changes.
CREATE TABLE `v5_mesh_settings` (`id` integer PRIMARY KEY AUTOINCREMENT,`name_suffix` text NOT NULL,`host_group_supernet` text NOT NULL,`host_group_prefix` integer NOT NULL,`reserved_subnets` text NOT NULL,`updated_at` datetime NOT NULL);
CREATE TABLE `v5_mesh_region` (`id` integer PRIMARY KEY AUTOINCREMENT,`code` text NOT NULL,`name` text NOT NULL,`sort_order` integer NOT NULL DEFAULT 0,`subnet` text NOT NULL,`server_range` text NOT NULL,`device_pool` text NOT NULL,`network_name` text NOT NULL,`network_key_sealed` text NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_region_code` ON `v5_mesh_region`(`code`);
CREATE TABLE `v5_mesh_member` (`id` integer PRIMARY KEY AUTOINCREMENT,`region_id` integer,`node_ref` text,`device_key` text,`hostname` text NOT NULL,`role` text NOT NULL,`address_mode` text NOT NULL,`address` text,`address_used_at` datetime,`probe_address` text NOT NULL DEFAULT "",`agentless` numeric NOT NULL DEFAULT false,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE INDEX `idx_v5_mesh_member_region_id` ON `v5_mesh_member`(`region_id`);
CREATE UNIQUE INDEX `idx_v5_mesh_member_address` ON `v5_mesh_member`(`region_id`,`address`);
CREATE UNIQUE INDEX `idx_v5_mesh_member_node` ON `v5_mesh_member`(`node_ref`);
CREATE UNIQUE INDEX `idx_v5_mesh_member_device_key` ON `v5_mesh_member`(`device_key`);
CREATE TABLE `v5_mesh_host_group` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`segment` text NOT NULL,`gateway_member_id` integer NOT NULL,`standby_member_id` integer,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_host_group_segment` ON `v5_mesh_host_group`(`segment`);
CREATE INDEX `idx_v5_mesh_host_group_gateway_member_id` ON `v5_mesh_host_group`(`gateway_member_id`);
CREATE INDEX `idx_v5_mesh_host_group_standby_member_id` ON `v5_mesh_host_group`(`standby_member_id`);
CREATE TABLE `v5_mesh_device_name` (`id` integer PRIMARY KEY AUTOINCREMENT,`member_id` integer NOT NULL,`host_group_id` integer,`name` text NOT NULL,`address` text NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE INDEX `idx_v5_mesh_device_name_member_id` ON `v5_mesh_device_name`(`member_id`);
CREATE INDEX `idx_v5_mesh_device_name_host_group_id` ON `v5_mesh_device_name`(`host_group_id`);
CREATE TABLE `v5_mesh_region_change` (`id` integer PRIMARY KEY AUTOINCREMENT,`member_id` integer NOT NULL,`from_region_id` integer NOT NULL,`to_region_id` integer NOT NULL,`source` text NOT NULL,`status` text NOT NULL,`reason` text NOT NULL DEFAULT "",`note` text NOT NULL DEFAULT "",`old_address` text NOT NULL DEFAULT "",`new_address` text NOT NULL DEFAULT "",`requested_by` integer,`decided_by` integer,`decided_at` datetime,`created_at` datetime NOT NULL);
CREATE INDEX `idx_v5_mesh_region_change_member_id` ON `v5_mesh_region_change`(`member_id`);
CREATE INDEX `idx_v5_mesh_region_change_status` ON `v5_mesh_region_change`(`status`);
