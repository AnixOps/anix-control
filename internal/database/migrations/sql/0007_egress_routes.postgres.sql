-- 0007_egress_routes (PostgreSQL): v5_egress_route, the domain egress
-- routes between proxy nodes (internal/egressroute, model.EgressRoute). Both
-- nodes of a route get it in the member "egress" of their desired
-- configuration.
CREATE TABLE "v5_egress_route" ("id" bigserial,"name" varchar(100) NOT NULL,"source_node_id" bigint NOT NULL,"exit_node_id" bigint NOT NULL,"exit_host" varchar(255) NOT NULL DEFAULT '',"relay_port" bigint NOT NULL,"method" varchar(64) NOT NULL,"relay_key" text NOT NULL,"domains" text NOT NULL,"enabled" boolean NOT NULL DEFAULT true,"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_egress_route_source_node_id" ON "v5_egress_route" ("source_node_id");
CREATE INDEX IF NOT EXISTS "idx_v5_egress_route_exit_node_id" ON "v5_egress_route" ("exit_node_id");
