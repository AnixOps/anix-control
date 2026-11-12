-- 0036_forward_routes_as_lines (SQLite): forward routes are lines
-- (docs/design/network-lines, step 2). v5_line, which nothing wrote yet,
-- is created again with the forward route's columns (owner, revision, the
-- route as protojson, enforcement, created_at); every route of
-- v4_kernel_forward_route moves into it and the old table goes. The line
-- columns are written from the routes after the next plan.
DROP TABLE IF EXISTS `v5_line`;
CREATE TABLE `v5_line` (`id` text,`owner` text NOT NULL DEFAULT "",`revision` integer NOT NULL DEFAULT 0,`route` text NOT NULL DEFAULT "",`enforced` text NOT NULL DEFAULT "",`sort` integer NOT NULL DEFAULT 0,`name` text NOT NULL DEFAULT "",`enabled` numeric NOT NULL DEFAULT false,`show` numeric NOT NULL DEFAULT false,`entries` text,`entry_hostname` text NOT NULL DEFAULT "",`listen` text NOT NULL DEFAULT "",`port` integer NOT NULL DEFAULT 0,`l4` text,`chain` text,`exit` text NOT NULL DEFAULT "",`rules` text,`protocol_id` integer,`targets` text,`policy` text,`hop_balance` text NOT NULL DEFAULT "",`rate` real NOT NULL DEFAULT 1,`serve_at` text NOT NULL DEFAULT "",`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL,PRIMARY KEY (`id`));
CREATE INDEX `idx_v5_line_protocol_id` ON `v5_line`(`protocol_id`);
CREATE INDEX `idx_v5_line_owner` ON `v5_line`(`owner`);
INSERT INTO `v5_line` (`id`,`owner`,`revision`,`route`,`enforced`,`created_at`,`updated_at`)
SELECT `id`,`owner`,`revision`,`route_json`,`enforced`,`created_at`,`updated_at` FROM `v4_kernel_forward_route`;
DROP TABLE `v4_kernel_forward_route`;
