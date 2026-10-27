-- 0013_iwan_lines (PostgreSQL): v5_iwan_line, the Panabit iWAN lines
-- subscribers log into with each line's shared account (internal/iwanline,
-- model.IWANLine), and the line of an iWAN node entry
-- (v5_node_entry.iwan_line_id).
CREATE TABLE "v5_iwan_line" ("id" bigserial,"name" varchar(64) NOT NULL,"host" varchar(255) NOT NULL,"port" bigint NOT NULL,"username" varchar(128) NOT NULL,"password" varchar(128) NOT NULL,"mtu" bigint NOT NULL DEFAULT 0,"encrypt" boolean NOT NULL DEFAULT false,"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_iwan_line_name" ON "v5_iwan_line" ("name");
ALTER TABLE "v5_node_entry" ADD "iwan_line_id" bigint;
