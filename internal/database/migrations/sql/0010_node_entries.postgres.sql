-- 0010_node_entries (PostgreSQL): v5_node_entry, the addresses subscribers
-- reach a proxy node at beyond its host, such as a line's native IPv6
-- address or a provider NAT address with its port map (internal/nodeentry,
-- model.NodeEntry).
CREATE TABLE "v5_node_entry" ("id" bigserial,"node_id" bigint NOT NULL,"label" varchar(64) NOT NULL,"kind" varchar(16) NOT NULL,"host" varchar(255) NOT NULL,"port_map" text NOT NULL DEFAULT '',"enabled" boolean NOT NULL DEFAULT true,"sort" bigint NOT NULL DEFAULT 0,"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_node_entry_node_id" ON "v5_node_entry" ("node_id");
