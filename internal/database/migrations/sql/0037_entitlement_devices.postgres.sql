-- 0037_entitlement_devices (PostgreSQL): the devices users registered for
-- a licensed feature (internal/entitlement, anx spec §2.4.1): one row per
-- user, feature and device key, with its trial flag, the end of its last
-- license and its revocation.
CREATE TABLE "v5_entitlement_device" ("id" bigserial,"user_id" bigint NOT NULL,"feature" varchar(32) NOT NULL,"device_id" varchar(64) NOT NULL,"alg" varchar(16) NOT NULL,"public_key" varchar(255) NOT NULL,"trial" boolean NOT NULL DEFAULT false,"not_after" bigint NOT NULL DEFAULT 0,"revoked_at" timestamptz,"created_at" timestamptz NOT NULL,"last_seen_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_entitlement_device_key" ON "v5_entitlement_device" ("user_id","feature","device_id");
