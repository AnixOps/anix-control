-- 0045_mesh_regions (SQLite): the intranet mesh regions
-- (internal/meshregion, docs/design/mesh-regions): v5_mesh_region with the
-- four default regions (disabled, no network key yet), v5_mesh_member (the
-- nodes' fixed addresses) and v5_mesh_status (the latest status each
-- node's Agent reported).
CREATE TABLE `v5_mesh_region` (`id` integer PRIMARY KEY AUTOINCREMENT,`code` text NOT NULL,`name` text NOT NULL,`cidr` text NOT NULL,`secret` text NOT NULL,`enabled` numeric NOT NULL,`sort` integer NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_region_code` ON `v5_mesh_region`(`code`);
CREATE TABLE `v5_mesh_member` (`id` integer PRIMARY KEY AUTOINCREMENT,`region_id` integer NOT NULL,`node_ref` text NOT NULL,`address` text NOT NULL,`home` numeric NOT NULL,`boundary` numeric NOT NULL,`created_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_mesh_member_region_node` ON `v5_mesh_member`(`region_id`,`node_ref`);
CREATE UNIQUE INDEX `idx_v5_mesh_member_region_address` ON `v5_mesh_member`(`region_id`,`address`);
CREATE INDEX `idx_v5_mesh_member_node_ref` ON `v5_mesh_member`(`node_ref`);
CREATE TABLE `v5_mesh_status` (`node_ref` text,`payload` text NOT NULL,`observed_at` datetime NOT NULL,`received_at` datetime NOT NULL,PRIMARY KEY (`node_ref`));
INSERT INTO `v5_mesh_region` (`code`,`name`,`cidr`,`secret`,`enabled`,`sort`,`created_at`,`updated_at`) VALUES ('cn','中国大陆','10.150.0.0/16','',false,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('apac','亚太','10.151.0.0/16','',false,2,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('eu','欧洲','10.152.0.0/16','',false,3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('na','北美','10.153.0.0/16','',false,4,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
