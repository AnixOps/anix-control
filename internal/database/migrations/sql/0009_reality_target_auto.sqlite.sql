-- 0009_reality_target_auto (SQLite): v5_reality_target_auto, each proxy
-- node's automatic REALITY target selection (internal/realitytarget,
-- model.RealityTargetAuto).
CREATE TABLE `v5_reality_target_auto` (`node_id` integer,`enabled` numeric NOT NULL DEFAULT false,`next_run_at` datetime,`last_run_at` datetime,`last_switch_at` datetime,`fail_streak` integer NOT NULL DEFAULT 0,`last_outcome` text,`created_at` datetime,`updated_at` datetime,PRIMARY KEY (`node_id`));
