-- 0009_reality_target_auto (PostgreSQL): v5_reality_target_auto, each
-- proxy node's automatic REALITY target selection (internal/realitytarget,
-- model.RealityTargetAuto).
CREATE TABLE "v5_reality_target_auto" ("node_id" bigint,"enabled" boolean NOT NULL DEFAULT false,"next_run_at" timestamptz,"last_run_at" timestamptz,"last_switch_at" timestamptz,"fail_streak" bigint NOT NULL DEFAULT 0,"last_outcome" varchar(500),"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("node_id"));
