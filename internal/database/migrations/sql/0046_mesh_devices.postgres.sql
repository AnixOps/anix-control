-- 0046_mesh_devices (PostgreSQL): mesh devices without an Agent
-- (internal/meshregion, docs/design/mesh-regions): v5_mesh_enrollment (the
-- one-time enrollment credentials, token hashes only), v5_mesh_device (the
-- enrolled devices, token hashes only) and v5_mesh_region_change (the
-- devices' region change requests).
CREATE TABLE "v5_mesh_enrollment" ("id" bigserial,"region_id" bigint NOT NULL,"kind" varchar(16) NOT NULL,"name" varchar(64) NOT NULL,"token_hash" varchar(64) NOT NULL,"expires_at" timestamptz NOT NULL,"used_at" timestamptz,"device_id" bigint,"created_by" bigint,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_enrollment_token_hash" ON "v5_mesh_enrollment" ("token_hash");
CREATE TABLE "v5_mesh_device" ("id" bigserial,"region_id" bigint NOT NULL,"kind" varchar(16) NOT NULL,"name" varchar(64) NOT NULL,"device_key" varchar(128) NOT NULL,"token_hash" varchar(64) NOT NULL,"address" varchar(15) NOT NULL,"last_seen_at" timestamptz,"revoked_at" timestamptz,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_device_region_id" ON "v5_mesh_device" ("region_id");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_device_device_key" ON "v5_mesh_device" ("device_key");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_device_token_hash" ON "v5_mesh_device" ("token_hash");
CREATE TABLE "v5_mesh_region_change" ("id" bigserial,"device_id" bigint NOT NULL,"from_region_id" bigint NOT NULL,"to_region_id" bigint NOT NULL,"reason" varchar(200) NOT NULL,"state" varchar(16) NOT NULL,"decided_by" bigint,"decided_at" timestamptz,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_mesh_region_change_device_id" ON "v5_mesh_region_change" ("device_id");
