-- 0027_drop_node_credentials (PostgreSQL): nodes authenticate with their
-- mTLS certificate only, so the node credentials go: a proxy node's API key,
-- key hash and shared secret (v2_node), a forward node's token
-- (v2_forward_node), the clean agents (v2_forward_clean_agent), the moved
-- credentials (v4_kernel_node_credential), the kernel API views that listed
-- them and the credential split state of the tables that held only
-- credentials. Irreversible: back the database up first.
DROP VIEW IF EXISTS "kapi_node_credential_status_v1";
DROP VIEW IF EXISTS "kapi_forward_clean_agent_v1";
ALTER TABLE "v2_node" DROP COLUMN IF EXISTS "api_key";
ALTER TABLE "v2_node" DROP COLUMN IF EXISTS "api_key_hash";
ALTER TABLE "v2_node" DROP COLUMN IF EXISTS "secret";
ALTER TABLE "v2_forward_node" DROP COLUMN IF EXISTS "api_token";
DROP TABLE IF EXISTS "v2_forward_clean_agent";
DROP TABLE IF EXISTS "v4_kernel_node_credential";
DELETE FROM "v4_kernel_node_secret_split" WHERE "table_name" IN ('v2_forward_node', 'v2_forward_clean_agent');
