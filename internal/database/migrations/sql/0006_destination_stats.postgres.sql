-- 0006_destination_stats (PostgreSQL): v5_destination_stat, each user's
-- traffic by destination domain, node and UTC hour, from the Agents'
-- DestinationReport batches (destinations.v1, internal/destinationstats,
-- model.DestinationStat). The retention worker deletes the rows older than
-- the configured retention.
CREATE TABLE "v5_destination_stat" ("id" bigserial,"hour_at" bigint NOT NULL,"node_id" bigint NOT NULL,"user_id" bigint NOT NULL,"domain" varchar(255) NOT NULL DEFAULT '',"protocol" varchar(32) NOT NULL DEFAULT '',"upload" bigint NOT NULL DEFAULT 0,"download" bigint NOT NULL DEFAULT 0,"connections" bigint NOT NULL DEFAULT 0,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_destination_stat_user" ON "v5_destination_stat" ("user_id","hour_at");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_destination_stat_key" ON "v5_destination_stat" ("hour_at","node_id","user_id","domain","protocol");
