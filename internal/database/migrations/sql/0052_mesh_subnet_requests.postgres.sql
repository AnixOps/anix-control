-- 0052_mesh_subnet_requests (PostgreSQL): the subnets a mesh full node
-- without an Agent asks to advertise (R35, internal/meshregion), pending
-- until an administrator approves or rejects them. Additive; NULL while
-- nothing is pending.
ALTER TABLE "v5_mesh_full_node" ADD "requested_subnets" text;
ALTER TABLE "v5_mesh_full_node" ADD "requested_at" timestamptz;
