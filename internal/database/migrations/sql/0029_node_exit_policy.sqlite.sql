-- 0029_node_exit_policy (SQLite): v5_node_exit_policy, whether a proxy
-- node refuses mainland-China destinations as an exit and how many
-- connections its Agent reported refusing (internal/exitpolicy,
-- model.NodeExitPolicy).
CREATE TABLE `v5_node_exit_policy` (`node_id` integer,`block_mainland` numeric NOT NULL DEFAULT false,`mainland_blocked_total` integer NOT NULL DEFAULT 0,`mainland_blocked_at` datetime,`updated_at` datetime NOT NULL,PRIMARY KEY (`node_id`));
