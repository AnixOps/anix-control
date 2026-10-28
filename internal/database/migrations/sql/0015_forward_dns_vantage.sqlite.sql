-- 0015_forward_dns_vantage (SQLite): v4_kernel_forward_dns_vantage, the
-- vantage selector of an entry HA DNS binding: the forwarding nodes with
-- that label probe the route's entry addresses from the clients' side
-- (internal/kernelforward/vantage.go, model.KernelForwardDNSVantage).
CREATE TABLE `v4_kernel_forward_dns_vantage` (`binding_id` integer,`selector` text NOT NULL,`updated_at` datetime NOT NULL,PRIMARY KEY (`binding_id`));
