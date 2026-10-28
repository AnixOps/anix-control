-- 0014_forward_latency (PostgreSQL): v4_kernel_forward_latency, the forward
-- routes' latency history: five-minute buckets of each upstream's health
-- checks per route, hop and node (internal/kernelforward/latency.go,
-- model.KernelForwardLatency).
CREATE TABLE "v4_kernel_forward_latency" ("route_id" varchar(64),"hop_index" bigint,"node_ref" varchar(32),"upstream" varchar(300),"bucket_start" timestamptz,"samples" bigint NOT NULL,"failures" bigint NOT NULL,"rtt_sum_us" bigint NOT NULL,"rtt_min_us" bigint NOT NULL,"rtt_max_us" bigint NOT NULL,"last_checked_at" timestamptz NOT NULL,PRIMARY KEY ("route_id","hop_index","node_ref","upstream","bucket_start"));
CREATE INDEX IF NOT EXISTS "idx_v4_kernel_forward_latency_bucket_start" ON "v4_kernel_forward_latency" ("bucket_start");
