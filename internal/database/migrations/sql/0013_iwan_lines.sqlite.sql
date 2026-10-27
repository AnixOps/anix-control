-- 0013_iwan_lines (SQLite): v5_iwan_line, the Panabit iWAN lines subscribers
-- log into with each line's shared account (internal/iwanline,
-- model.IWANLine), and the line of an iWAN node entry
-- (v5_node_entry.iwan_line_id).
CREATE TABLE `v5_iwan_line` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`host` text NOT NULL,`port` integer NOT NULL,`username` text NOT NULL,`password` text NOT NULL,`mtu` integer NOT NULL DEFAULT 0,`encrypt` numeric NOT NULL DEFAULT false,`created_at` datetime,`updated_at` datetime);
CREATE UNIQUE INDEX `idx_v5_iwan_line_name` ON `v5_iwan_line`(`name`);
ALTER TABLE `v5_node_entry` ADD `iwan_line_id` integer;
