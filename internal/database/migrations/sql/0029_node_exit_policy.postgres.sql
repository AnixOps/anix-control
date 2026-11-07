-- 0029_node_exit_policy (PostgreSQL): v5_node_exit_policy, whether a proxy
-- node refuses mainland-China destinations as an exit and how many
-- connections its Agent reported refusing (internal/exitpolicy,
-- model.NodeExitPolicy).
CREATE TABLE "v5_node_exit_policy" ("node_id" bigint,"block_mainland" boolean NOT NULL DEFAULT false,"mainland_blocked_total" bigint NOT NULL DEFAULT 0,"mainland_blocked_at" timestamptz,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("node_id"));
