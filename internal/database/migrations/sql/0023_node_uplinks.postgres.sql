-- 0023_node_uplinks (PostgreSQL): each device's uplinks
-- (v4_kernel_node_uplinks): the interfaces its traffic to a route's next
-- device may leave by, the choice per next device, and the uplink Control
-- chose for each next device in auto mode.
CREATE TABLE "v4_kernel_node_uplinks" ("node_ref" varchar(32),"config_json" text NOT NULL DEFAULT '',"active_json" text NOT NULL DEFAULT '',"updated_at" timestamptz NOT NULL,PRIMARY KEY ("node_ref"));
