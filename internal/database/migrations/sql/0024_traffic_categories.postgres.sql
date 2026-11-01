-- 0024_traffic_categories (PostgreSQL): the administrator's traffic
-- category rules (v5_traffic_category_rule) and the remote category lists
-- (v5_traffic_category_source, internal/destinationstats).
CREATE TABLE "v5_traffic_category_rule" ("id" bigserial,"kind" varchar(16) NOT NULL,"value" varchar(255) NOT NULL,"category" varchar(32) NOT NULL,"note" varchar(255) NOT NULL DEFAULT '',"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_traffic_category_rule_key" ON "v5_traffic_category_rule" ("kind","value");
CREATE TABLE "v5_traffic_category_source" ("id" bigserial,"name" varchar(100) NOT NULL,"url" varchar(1024) NOT NULL,"category" varchar(32) NOT NULL,"enabled" boolean NOT NULL DEFAULT true,"refresh_hours" bigint NOT NULL DEFAULT 24,"entries_json" text NOT NULL DEFAULT '',"entry_count" bigint NOT NULL DEFAULT 0,"fetched_at" timestamptz,"fetch_error" text NOT NULL DEFAULT '',"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_traffic_category_source_name" ON "v5_traffic_category_source" ("name");
