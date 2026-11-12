-- 0035_network_lines (PostgreSQL): the network as lines
-- (docs/design/network-lines, step 2). v5_line holds each line from its
-- entries over transports to its exit, v5_transport the links between two
-- devices, and v4_kernel_node_network.scan_cloud marks a device on a cloud
-- that scans its machines (it forwards in the kernel).
CREATE TABLE "v5_line" ("id" varchar(64),"sort" bigint NOT NULL DEFAULT 0,"name" varchar(191) NOT NULL DEFAULT '',"enabled" boolean NOT NULL DEFAULT false,"show" boolean NOT NULL DEFAULT false,"entries" text,"entry_hostname" varchar(255) NOT NULL DEFAULT '',"listen" varchar(64) NOT NULL DEFAULT '',"port" bigint NOT NULL DEFAULT 0,"l4" text,"chain" text,"exit" varchar(64) NOT NULL DEFAULT '',"rules" text,"protocol_id" bigint,"targets" text,"policy" text,"hop_balance" varchar(32) NOT NULL DEFAULT '',"rate" decimal NOT NULL DEFAULT 1,"serve_at" varchar(32) NOT NULL DEFAULT '',"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_line_protocol_id" ON "v5_line" ("protocol_id");
CREATE TABLE "v5_transport" ("id" varchar(191),"from_device" varchar(32) NOT NULL,"to_device" varchar(32) NOT NULL,"kind" varchar(16) NOT NULL,"carriers" text,"multipath" boolean NOT NULL DEFAULT false,"fec" boolean NOT NULL DEFAULT false,"declared" boolean NOT NULL DEFAULT false,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
ALTER TABLE "v4_kernel_node_network" ADD "scan_cloud" boolean NOT NULL DEFAULT false;
