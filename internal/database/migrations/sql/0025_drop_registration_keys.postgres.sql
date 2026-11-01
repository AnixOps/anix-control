-- 0025_drop_registration_keys (PostgreSQL): node registration is gone, so
-- the registration keys go with it: the kernel API view that listed them,
-- their table v2_authorized_key (and its key index), their moved credentials
-- and the table's credential split state.
DROP VIEW IF EXISTS "kapi_registration_key_v1";
DROP TABLE IF EXISTS "v2_authorized_key";
DELETE FROM "v4_kernel_node_credential" WHERE "subject_kind" = 'registration_key';
DELETE FROM "v4_kernel_node_secret_split" WHERE "table_name" = 'v2_authorized_key';
