-- 0047_mesh_enrollment (PostgreSQL): devices without an Agent enroll in the
-- intranet mesh (internal/meshregion, docs/design/mesh-regions):
-- v5_mesh_enrollment (one-time credentials) and v5_mesh_device_token (the
-- enrolled devices' tokens), SHA-256 hashes only.
CREATE TABLE "v5_mesh_enrollment" ("id" bigserial,"region_id" bigint NOT NULL,"hostname" varchar(63) NOT NULL,"token_hash" varchar(64) NOT NULL,"expires_at" timestamptz NOT NULL,"used_at" timestamptz,"device_id" bigint,"created_by" bigint,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_enrollment_token_hash" ON "v5_mesh_enrollment" ("token_hash");
CREATE TABLE "v5_mesh_device_token" ("id" bigserial,"device_id" bigint NOT NULL,"token_hash" varchar(64) NOT NULL,"last_seen_at" timestamptz,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_device_token_device_id" ON "v5_mesh_device_token" ("device_id");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_device_token_token_hash" ON "v5_mesh_device_token" ("token_hash");
