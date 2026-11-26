-- 0054_mesh_paths (PostgreSQL): the custom paths of the region networks
-- (internal/meshregion, docs/design/mesh-regions): the path of one source
-- member to a peer or a subnet through waypoint members, stored by member
-- identity (node:<node ref>, device:<id>); addresses are resolved when a
-- path is delivered.
CREATE TABLE "v5_mesh_path" ("id" bigserial,"source_ref" varchar(32) NOT NULL,"target_kind" varchar(8) NOT NULL,"target" varchar(64) NOT NULL,"via" text NOT NULL,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_path_target" ON "v5_mesh_path" ("source_ref","target");
