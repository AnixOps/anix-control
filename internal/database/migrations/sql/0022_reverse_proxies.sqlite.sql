-- 0022_reverse_proxies: v5_reverse_proxy, the panel HTTP(S) reverse
-- proxies (internal/reverseproxy, model.ReverseProxy) the entry devices run
-- as NetworkPlan reverse proxies, and v5_reverse_proxy_acme_account, the
-- ACME account their certificates are issued with.
CREATE TABLE `v5_reverse_proxy` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`enabled` numeric NOT NULL DEFAULT true,`entries_json` text NOT NULL DEFAULT "",`via_json` text NOT NULL DEFAULT "",`listen_port` integer NOT NULL DEFAULT 0,`http_port` integer NOT NULL DEFAULT 0,`sites_json` text NOT NULL DEFAULT "",`tls_mode` text NOT NULL DEFAULT "none",`dns_provider_id` integer NOT NULL DEFAULT 0,`dns_zone` text NOT NULL DEFAULT "",`certificate_pem` text NOT NULL DEFAULT "",`sealed_private_key` text NOT NULL DEFAULT "",`certificate_not_after` datetime,`certificate_error` text NOT NULL DEFAULT "",`certificate_attempt_at` datetime,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_reverse_proxy_name` ON `v5_reverse_proxy`(`name`);
CREATE TABLE `v5_reverse_proxy_acme_account` (`id` integer PRIMARY KEY AUTOINCREMENT,`directory` text NOT NULL,`sealed_key` text NOT NULL DEFAULT "",`account_url` text NOT NULL DEFAULT "",`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_reverse_proxy_acme_account_directory` ON `v5_reverse_proxy_acme_account`(`directory`);
