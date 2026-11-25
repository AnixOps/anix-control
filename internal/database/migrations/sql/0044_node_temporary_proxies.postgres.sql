-- 0044_node_temporary_proxies (PostgreSQL): v5_node_temporary_proxy, the
-- SOCKS5 proxies a proxy node serves for a limited time on an internal
-- address (internal/tempproxy, model.NodeTemporaryProxy).
CREATE TABLE "v5_node_temporary_proxy" ("id" bigserial,"node_id" bigint NOT NULL,"listen" varchar(64) NOT NULL,"port" bigint NOT NULL,"username" varchar(64) NOT NULL,"password" varchar(64) NOT NULL,"expires_at" timestamptz NOT NULL,"closed_at" timestamptz,"created_by" bigint,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_node_temporary_proxy_node_id" ON "v5_node_temporary_proxy" ("node_id");
CREATE INDEX IF NOT EXISTS "idx_v5_node_temporary_proxy_expires_at" ON "v5_node_temporary_proxy" ("expires_at");
