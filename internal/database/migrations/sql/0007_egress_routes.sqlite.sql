-- 0007_egress_routes (SQLite): v5_egress_route, the domain egress routes
-- between proxy nodes (internal/egressroute, model.EgressRoute). Both nodes
-- of a route get it in the member "egress" of their desired configuration.
CREATE TABLE `v5_egress_route` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`source_node_id` integer NOT NULL,`exit_node_id` integer NOT NULL,`exit_host` text NOT NULL DEFAULT "",`relay_port` integer NOT NULL,`method` text NOT NULL,`relay_key` text NOT NULL,`domains` text NOT NULL,`enabled` numeric NOT NULL DEFAULT true,`created_at` datetime,`updated_at` datetime);
CREATE INDEX `idx_v5_egress_route_source_node_id` ON `v5_egress_route`(`source_node_id`);
CREATE INDEX `idx_v5_egress_route_exit_node_id` ON `v5_egress_route`(`exit_node_id`);
