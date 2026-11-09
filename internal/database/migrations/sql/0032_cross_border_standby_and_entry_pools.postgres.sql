-- 0032_cross_border_standby_and_entry_pools (PostgreSQL): the cross-border
-- standby channels (v5_l2_standby, internal/l2standby), the entry pool
-- policy and assignments (v5_entry_pool_policy, v5_user_entry_pool,
-- v5_node_entry.pool, internal/entrypool) and the clients' failure reports
-- (v5_client_failure_report, internal/clientreport); anx F-54 / F-55.
CREATE TABLE "v5_l2_standby" ("id" bigserial,"transport_id" varchar(64) NOT NULL,"tier" varchar(16) NOT NULL,"source_node_id" bigint NOT NULL,"exit_node_id" bigint NOT NULL,"exit_host" varchar(255) NOT NULL DEFAULT '',"relay_port" bigint NOT NULL,"uuid" varchar(36) NOT NULL,"reality_private_key" varchar(64) NOT NULL DEFAULT '',"reality_public_key" varchar(64) NOT NULL DEFAULT '',"reality_short_id" varchar(16) NOT NULL DEFAULT '',"reality_server_name" varchar(255) NOT NULL DEFAULT '',"reality_dest" varchar(255) NOT NULL DEFAULT '',"xhttp_domain" varchar(255) NOT NULL DEFAULT '',"xhttp_path" varchar(64) NOT NULL DEFAULT '',"active" boolean NOT NULL DEFAULT false,"activated_at" timestamptz,"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_l2_standby_source_node_id" ON "v5_l2_standby" ("source_node_id");
CREATE INDEX IF NOT EXISTS "idx_v5_l2_standby_transport_id" ON "v5_l2_standby" ("transport_id");
CREATE INDEX IF NOT EXISTS "idx_v5_l2_standby_exit_node_id" ON "v5_l2_standby" ("exit_node_id");
CREATE TABLE "v5_entry_pool_policy" ("id" bigint,"shards" bigint NOT NULL DEFAULT 0,"quarantine_days" bigint NOT NULL DEFAULT 0,"quarantine_pool" varchar(32) NOT NULL DEFAULT 'quarantine',"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE TABLE "v5_user_entry_pool" ("user_id" bigint,"pool" varchar(32) NOT NULL,"reason" varchar(128) NOT NULL DEFAULT '',"updated_at" timestamptz NOT NULL,PRIMARY KEY ("user_id"));
CREATE INDEX IF NOT EXISTS "idx_v5_user_entry_pool_pool" ON "v5_user_entry_pool" ("pool");
CREATE TABLE "v5_client_failure_report" ("id" bigserial,"user_id" bigint NOT NULL,"hour" timestamptz NOT NULL,"node" varchar(128) NOT NULL,"network" varchar(16) NOT NULL,"server" varchar(255) NOT NULL DEFAULT '',"proto" varchar(32) NOT NULL DEFAULT '',"platform" varchar(16) NOT NULL DEFAULT '',"client" varchar(64) NOT NULL DEFAULT '',"ok" bigint NOT NULL DEFAULT 0,"fallback" bigint NOT NULL DEFAULT 0,"timeout" bigint NOT NULL DEFAULT 0,"refused" bigint NOT NULL DEFAULT 0,"reset" bigint NOT NULL DEFAULT 0,"after_handshake" bigint NOT NULL DEFAULT 0,"keepalive_lost" bigint NOT NULL DEFAULT 0,"other" bigint NOT NULL DEFAULT 0,"stopped" boolean NOT NULL DEFAULT false,"clock_skew_secs" bigint,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_client_failure_report_hour" ON "v5_client_failure_report" ("hour");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_client_failure_report_key" ON "v5_client_failure_report" ("user_id","hour","node","network");
ALTER TABLE "v5_node_entry" ADD "pool" varchar(32) NOT NULL DEFAULT '';
