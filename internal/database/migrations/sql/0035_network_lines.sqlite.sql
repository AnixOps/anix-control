-- 0035_network_lines (SQLite): the network as lines
-- (docs/design/network-lines, step 2). v5_line holds each line from its
-- entries over transports to its exit, v5_transport the links between two
-- devices, and v4_kernel_node_network.scan_cloud marks a device on a cloud
-- that scans its machines (it forwards in the kernel).
CREATE TABLE `v5_line` (`id` text,`sort` integer NOT NULL DEFAULT 0,`name` text NOT NULL DEFAULT "",`enabled` numeric NOT NULL DEFAULT false,`show` numeric NOT NULL DEFAULT false,`entries` text,`entry_hostname` text NOT NULL DEFAULT "",`listen` text NOT NULL DEFAULT "",`port` integer NOT NULL DEFAULT 0,`l4` text,`chain` text,`exit` text NOT NULL DEFAULT "",`rules` text,`protocol_id` integer,`targets` text,`policy` text,`hop_balance` text NOT NULL DEFAULT "",`rate` real NOT NULL DEFAULT 1,`serve_at` text NOT NULL DEFAULT "",`updated_at` datetime NOT NULL,PRIMARY KEY (`id`));
CREATE INDEX `idx_v5_line_protocol_id` ON `v5_line`(`protocol_id`);
CREATE TABLE `v5_transport` (`id` text,`from_device` text NOT NULL,`to_device` text NOT NULL,`kind` text NOT NULL,`carriers` text,`multipath` numeric NOT NULL DEFAULT false,`fec` numeric NOT NULL DEFAULT false,`declared` numeric NOT NULL DEFAULT false,`updated_at` datetime NOT NULL,PRIMARY KEY (`id`));
ALTER TABLE `v4_kernel_node_network` ADD `scan_cloud` numeric NOT NULL DEFAULT false;
