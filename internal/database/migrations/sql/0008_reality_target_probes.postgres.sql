-- 0008_reality_target_probes (PostgreSQL): v5_reality_target_probe, the
-- latest probe of each candidate REALITY target from each proxy node
-- (internal/realitytarget, model.RealityTargetProbe).
CREATE TABLE "v5_reality_target_probe" ("id" bigserial,"node_id" bigint NOT NULL,"server_name" varchar(253) NOT NULL,"port" bigint NOT NULL,"verdict" varchar(16) NOT NULL,"score" bigint NOT NULL DEFAULT 0,"result" text NOT NULL,"probed_at" timestamptz NOT NULL,"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_reality_target_probe_target" ON "v5_reality_target_probe" ("node_id","server_name","port");
