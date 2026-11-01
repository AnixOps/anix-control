-- 0026_drop_server_hysteria (PostgreSQL): Hysteria v1 is removed. Its
-- Xboard-era server table v2_server_hysteria was never part of the baseline
-- and nothing reads it; a database imported from Xboard may still hold it.
DROP TABLE IF EXISTS "v2_server_hysteria";
