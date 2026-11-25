-- 0051_mesh_external_bridges (PostgreSQL): the mesh's external network
-- bridge nodes (internal/meshregion, docs/design/mesh-regions; spec
-- section 15.12, R36) with their external interface, advertised routes and
-- allow list, and the addresses and segments held back until a bridge
-- node confirms it no longer lets them through.
CREATE TABLE "v5_mesh_external_bridge" ("id" bigserial,"node_ref" varchar(32) NOT NULL,"interface" varchar(15) NOT NULL,"routes" text NOT NULL,"allow" text NOT NULL,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_external_bridge_node_ref" ON "v5_mesh_external_bridge" ("node_ref");
CREATE TABLE "v5_mesh_bridge_hold" ("id" bigserial,"node_ref" varchar(32) NOT NULL,"region_id" bigint NOT NULL,"prefix" varchar(43) NOT NULL,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_bridge_hold_entry" ON "v5_mesh_bridge_hold" ("node_ref","region_id","prefix");
