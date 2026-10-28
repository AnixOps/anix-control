-- 0014_forward_latency (SQLite): v4_kernel_forward_latency, the forward
-- routes' latency history: five-minute buckets of each upstream's health
-- checks per route, hop and node (internal/kernelforward/latency.go,
-- model.KernelForwardLatency).
CREATE TABLE `v4_kernel_forward_latency` (`route_id` text,`hop_index` integer,`node_ref` text,`upstream` text,`bucket_start` datetime,`samples` integer NOT NULL,`failures` integer NOT NULL,`rtt_sum_us` integer NOT NULL,`rtt_min_us` integer NOT NULL,`rtt_max_us` integer NOT NULL,`last_checked_at` datetime NOT NULL,PRIMARY KEY (`route_id`,`hop_index`,`node_ref`,`upstream`,`bucket_start`));
CREATE INDEX `idx_v4_kernel_forward_latency_bucket_start` ON `v4_kernel_forward_latency`(`bucket_start`);
