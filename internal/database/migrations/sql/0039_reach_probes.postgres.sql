-- 0039_reach_probes (PostgreSQL): the reachability probe
-- (internal/reachprobe): v5_reach_vantage, the proxy nodes that probe the
-- others from their region, and v5_reach_probe, the latest answer of each
-- vantage about each target node.
CREATE TABLE "v5_reach_vantage" ("node_id" bigint,"region" varchar(16) NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("node_id"));
CREATE TABLE "v5_reach_probe" ("vantage_node_id" bigint,"target_node_id" bigint,"region" varchar(16) NOT NULL,"reachable" boolean NOT NULL DEFAULT false,"result" text NOT NULL,"probed_at" timestamptz NOT NULL,PRIMARY KEY ("vantage_node_id","target_node_id"));
