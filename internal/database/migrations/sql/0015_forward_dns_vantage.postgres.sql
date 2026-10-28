-- 0015_forward_dns_vantage (PostgreSQL): v4_kernel_forward_dns_vantage, the
-- vantage selector of an entry HA DNS binding: the forwarding nodes with
-- that label probe the route's entry addresses from the clients' side
-- (internal/kernelforward/vantage.go, model.KernelForwardDNSVantage).
CREATE TABLE "v4_kernel_forward_dns_vantage" ("binding_id" bigint,"selector" varchar(200) NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("binding_id"));
