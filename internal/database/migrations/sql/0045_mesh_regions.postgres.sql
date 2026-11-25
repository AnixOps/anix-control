-- 0045_mesh_regions (PostgreSQL): the intranet mesh's panel side
-- (internal/meshregion, model.Mesh*): the global settings, the regions,
-- the members and their virtual addresses, the host groups, the names the
-- gateways publish and the region changes.
CREATE TABLE "v5_mesh_settings" ("id" bigserial,"name_suffix" varchar(200) NOT NULL,"host_group_supernet" varchar(64) NOT NULL,"host_group_prefix" bigint NOT NULL,"reserved_subnets" text NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE TABLE "v5_mesh_region" ("id" bigserial,"code" varchar(16) NOT NULL,"name" varchar(64) NOT NULL,"sort_order" bigint NOT NULL DEFAULT 0,"subnet" varchar(64) NOT NULL,"server_range" varchar(64) NOT NULL,"device_pool" varchar(64) NOT NULL,"network_name" varchar(64) NOT NULL,"network_key_sealed" text NOT NULL,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_region_code" ON "v5_mesh_region" ("code");
CREATE TABLE "v5_mesh_member" ("id" bigserial,"region_id" bigint,"node_ref" varchar(32),"device_key" varchar(128),"hostname" varchar(63) NOT NULL,"role" varchar(16) NOT NULL,"address_mode" varchar(16) NOT NULL,"address" varchar(64),"address_used_at" timestamptz,"probe_address" varchar(255) NOT NULL DEFAULT '',"agentless" boolean NOT NULL DEFAULT false,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_member_region_id" ON "v5_mesh_member" ("region_id");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_member_address" ON "v5_mesh_member" ("region_id","address");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_member_node" ON "v5_mesh_member" ("node_ref");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_member_device_key" ON "v5_mesh_member" ("device_key");
CREATE TABLE "v5_mesh_host_group" ("id" bigserial,"name" varchar(64) NOT NULL,"segment" varchar(64) NOT NULL,"gateway_member_id" bigint NOT NULL,"standby_member_id" bigint,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_host_group_segment" ON "v5_mesh_host_group" ("segment");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_host_group_gateway_member_id" ON "v5_mesh_host_group" ("gateway_member_id");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_host_group_standby_member_id" ON "v5_mesh_host_group" ("standby_member_id");
CREATE TABLE "v5_mesh_device_name" ("id" bigserial,"member_id" bigint NOT NULL,"host_group_id" bigint,"name" varchar(63) NOT NULL,"address" varchar(64) NOT NULL,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_device_name_member_id" ON "v5_mesh_device_name" ("member_id");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_device_name_host_group_id" ON "v5_mesh_device_name" ("host_group_id");
CREATE TABLE "v5_mesh_region_change" ("id" bigserial,"member_id" bigint NOT NULL,"from_region_id" bigint NOT NULL,"to_region_id" bigint NOT NULL,"source" varchar(16) NOT NULL,"status" varchar(16) NOT NULL,"reason" varchar(500) NOT NULL DEFAULT '',"note" varchar(500) NOT NULL DEFAULT '',"old_address" varchar(64) NOT NULL DEFAULT '',"new_address" varchar(64) NOT NULL DEFAULT '',"requested_by" bigint,"decided_by" bigint,"decided_at" timestamptz,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_region_change_member_id" ON "v5_mesh_region_change" ("member_id");
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_region_change_status" ON "v5_mesh_region_change" ("status");
