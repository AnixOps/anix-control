-- 0046_mesh_addresses (PostgreSQL): the intranet mesh's addresses and names
-- (internal/meshregion, docs/design/mesh-regions): the global settings,
-- the personal devices with their pool addresses, the host groups, the
-- names gateway nodes publish, and the region changes.
CREATE TABLE "v5_mesh_settings" ("id" bigserial,"name_suffix" varchar(200) NOT NULL,"host_group_supernet" varchar(43) NOT NULL,"host_group_prefix" bigint NOT NULL,"reserved_ranges" text NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE TABLE "v5_mesh_device" ("id" bigserial,"region_id" bigint NOT NULL,"device_key" varchar(128),"hostname" varchar(63) NOT NULL,"address" varchar(15),"address_used_at" timestamptz,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_device_region_address" ON "v5_mesh_device" ("region_id","address");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_device_device_key" ON "v5_mesh_device" ("device_key");
CREATE TABLE "v5_mesh_host_group" ("id" bigserial,"name" varchar(63) NOT NULL,"segment" varchar(43) NOT NULL,"gateway_ref" varchar(32) NOT NULL,"standby_ref" varchar(32),"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_host_group_segment" ON "v5_mesh_host_group" ("segment");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_host_group_gateway_ref" ON "v5_mesh_host_group" ("gateway_ref");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_host_group_standby_ref" ON "v5_mesh_host_group" ("standby_ref");
CREATE TABLE "v5_mesh_device_name" ("id" bigserial,"gateway_ref" varchar(32) NOT NULL,"host_group_id" bigint,"name" varchar(63) NOT NULL,"address" varchar(15) NOT NULL,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_device_name_gateway_ref" ON "v5_mesh_device_name" ("gateway_ref");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_device_name_host_group_id" ON "v5_mesh_device_name" ("host_group_id");
CREATE TABLE "v5_mesh_region_change" ("id" bigserial,"node_ref" varchar(32),"device_id" bigint,"from_region_id" bigint NOT NULL,"to_region_id" bigint NOT NULL,"source" varchar(16) NOT NULL,"status" varchar(16) NOT NULL,"reason" varchar(500) NOT NULL,"note" varchar(500) NOT NULL,"old_address" varchar(15) NOT NULL,"new_address" varchar(15) NOT NULL,"requested_by" bigint,"decided_by" bigint,"decided_at" timestamptz,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_region_change_node_ref" ON "v5_mesh_region_change" ("node_ref");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_region_change_device_id" ON "v5_mesh_region_change" ("device_id");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_region_change_status" ON "v5_mesh_region_change" ("status");
