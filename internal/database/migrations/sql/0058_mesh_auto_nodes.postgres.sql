-- 0058_mesh_auto_nodes (PostgreSQL): the Agent nodes an administrator added to
-- the mesh without a region (internal/meshregion, docs/design/mesh-regions).
-- Such a node gets no network until its status reports the latencies of the
-- regions, from which the panel chooses its region and removes the row.
CREATE TABLE "v5_mesh_auto_node" ("id" bigserial,"node_ref" varchar(32) NOT NULL,"created_by" bigint,"created_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_mesh_auto_node_node_ref" ON "v5_mesh_auto_node" ("node_ref");
