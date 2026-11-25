-- 0048_mesh_full_nodes (PostgreSQL): mesh devices without an Agent that run
-- as full nodes (internal/meshregion, docs/design/mesh-regions): the
-- credential, the device, its fixed address and the subnets an
-- administrator confirmed.
CREATE TABLE "v5_mesh_full_node" ("id" bigserial,"enrollment_id" bigint NOT NULL,"device_id" bigint,"address" varchar(15) NOT NULL,"subnets" text NOT NULL,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_full_node_device_id" ON "v5_mesh_full_node" ("device_id");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_full_node_enrollment_id" ON "v5_mesh_full_node" ("enrollment_id");
