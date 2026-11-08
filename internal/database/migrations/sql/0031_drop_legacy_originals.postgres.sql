-- 0031_drop_legacy_originals (PostgreSQL): the node secret split is
-- finalized and unsplit is removed. The original JSON documents finalize
-- kept for it (scope legacy_original) have no reader left; they are
-- deleted.
DELETE FROM "v4_kernel_protocol_secret" WHERE "scope" = 'legacy_original';
