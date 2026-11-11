-- 0033_node_region (PostgreSQL): where a device is, for the topology map
-- (v4_kernel_node_network.region, a place key).
ALTER TABLE "v4_kernel_node_network" ADD "region" varchar(32) NOT NULL DEFAULT '';
