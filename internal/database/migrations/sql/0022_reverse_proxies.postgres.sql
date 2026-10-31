-- 0022_reverse_proxies (PostgreSQL): v5_reverse_proxy, the panel HTTP(S)
-- reverse proxies (internal/reverseproxy, model.ReverseProxy) the entry
-- devices run as NetworkPlan reverse proxies, and
-- v5_reverse_proxy_acme_account, the ACME account their certificates are
-- issued with.
CREATE TABLE "v5_reverse_proxy" ("id" bigserial,"name" varchar(100) NOT NULL,"enabled" boolean NOT NULL DEFAULT true,"entries_json" text NOT NULL DEFAULT '',"via_json" text NOT NULL DEFAULT '',"listen_port" bigint NOT NULL DEFAULT 0,"http_port" bigint NOT NULL DEFAULT 0,"sites_json" text NOT NULL DEFAULT '',"tls_mode" varchar(16) NOT NULL DEFAULT 'none',"dns_provider_id" bigint NOT NULL DEFAULT 0,"dns_zone" varchar(253) NOT NULL DEFAULT '',"certificate_pem" text NOT NULL DEFAULT '',"sealed_private_key" text NOT NULL DEFAULT '',"certificate_not_after" timestamptz,"certificate_error" text NOT NULL DEFAULT '',"certificate_attempt_at" timestamptz,"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_reverse_proxy_name" ON "v5_reverse_proxy" ("name");
CREATE TABLE "v5_reverse_proxy_acme_account" ("id" bigserial,"directory" varchar(512) NOT NULL,"sealed_key" text NOT NULL DEFAULT '',"account_url" varchar(512) NOT NULL DEFAULT '',"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_reverse_proxy_acme_account_directory" ON "v5_reverse_proxy_acme_account" ("directory");
