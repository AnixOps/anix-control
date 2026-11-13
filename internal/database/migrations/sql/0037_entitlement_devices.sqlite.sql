-- 0037_entitlement_devices (SQLite): the devices users registered for a
-- licensed feature (internal/entitlement, anx spec §2.4.1): one row per
-- user, feature and device key, with its trial flag, the end of its last
-- license and its revocation.
CREATE TABLE `v5_entitlement_device` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` integer NOT NULL,`feature` text NOT NULL,`device_id` text NOT NULL,`alg` text NOT NULL,`public_key` text NOT NULL,`trial` numeric NOT NULL DEFAULT false,`not_after` integer NOT NULL DEFAULT 0,`revoked_at` datetime,`created_at` datetime NOT NULL,`last_seen_at` datetime NOT NULL);
CREATE UNIQUE INDEX `idx_v5_entitlement_device_key` ON `v5_entitlement_device`(`user_id`,`feature`,`device_id`);
