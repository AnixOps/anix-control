-- 0018_node_network (PostgreSQL): each device's own network settings
-- (v4_kernel_node_network): the interface, source and gateway its
-- automatic network model egresses leave by, and its hop ingress port.
CREATE TABLE "v4_kernel_node_network" ("node_ref" varchar(32),"interface" varchar(32) NOT NULL DEFAULT '',"source_v4" varchar(64) NOT NULL DEFAULT '',"gateway_v4" varchar(64) NOT NULL DEFAULT '',"source_v6" varchar(64) NOT NULL DEFAULT '',"gateway_v6" varchar(64) NOT NULL DEFAULT '',"hop_port" bigint NOT NULL DEFAULT 0,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("node_ref"));
