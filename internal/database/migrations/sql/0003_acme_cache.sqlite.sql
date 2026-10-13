-- 0003_acme_cache (SQLite): the panel certificate store
-- (internal/panelcert, model.ACMECacheEntry): the ACME account key, the
-- certificates with their keys, challenge answers in flight, the
-- self-signed fallback and the certificate status. Entries that hold a key
-- are sealed with module_runtime.ca_kek.
CREATE TABLE `v5_acme_cache` (`cache_key` text,`data` blob NOT NULL,`sealed` numeric NOT NULL DEFAULT false,`updated_at` datetime NOT NULL,PRIMARY KEY (`cache_key`));
