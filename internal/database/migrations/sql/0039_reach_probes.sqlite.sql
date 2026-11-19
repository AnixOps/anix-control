-- 0039_reach_probes (SQLite): the reachability probe (internal/reachprobe):
-- v5_reach_vantage, the proxy nodes that probe the others from their
-- region, and v5_reach_probe, the latest answer of each vantage about each
-- target node.
CREATE TABLE `v5_reach_vantage` (`node_id` integer,`region` text NOT NULL,`updated_at` datetime NOT NULL,PRIMARY KEY (`node_id`));
CREATE TABLE `v5_reach_probe` (`vantage_node_id` integer,`target_node_id` integer,`region` text NOT NULL,`reachable` numeric NOT NULL DEFAULT false,`result` text NOT NULL,`probed_at` datetime NOT NULL,PRIMARY KEY (`vantage_node_id`,`target_node_id`));
