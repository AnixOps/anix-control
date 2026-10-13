-- 0002_server_log_buckets (SQLite): v2_server_log holds hourly buckets. A
-- traffic report adds to the row of its (server_id, log_at, user_id,
-- server_type, rate) key, log_at being the start of the report's UTC hour,
-- with an upsert on the unique index created at the end of this file.
--
-- The index fails on a table that holds two rows of one key, which Control
-- before this release wrote (a row per report, at the report's second). The
-- statements run in order in one transaction, so the rows of a key are merged
-- first: the oldest row of each repeated key takes the sum of u and d of its
-- rows and the others are deleted. A row with a NULL in the key is never
-- merged (a unique index treats NULLs as distinct). Existing rows are not
-- moved to hour boundaries: readers sum rows, so each number they return is
-- unchanged. The step is a set of statements, not a Go function, for that
-- reason: the merge needs nothing the SQL cannot say, and the runner applies
-- a file's statements in order.
UPDATE `v2_server_log` SET `u` = (SELECT COALESCE(SUM(g.`u`), 0) FROM `v2_server_log` g WHERE g.`server_id` = `v2_server_log`.`server_id` AND g.`log_at` = `v2_server_log`.`log_at` AND g.`user_id` = `v2_server_log`.`user_id` AND g.`server_type` = `v2_server_log`.`server_type` AND g.`rate` = `v2_server_log`.`rate`), `d` = (SELECT COALESCE(SUM(g.`d`), 0) FROM `v2_server_log` g WHERE g.`server_id` = `v2_server_log`.`server_id` AND g.`log_at` = `v2_server_log`.`log_at` AND g.`user_id` = `v2_server_log`.`user_id` AND g.`server_type` = `v2_server_log`.`server_type` AND g.`rate` = `v2_server_log`.`rate`) WHERE `id` IN (SELECT MIN(`id`) FROM `v2_server_log` WHERE `server_id` IS NOT NULL AND `log_at` IS NOT NULL AND `user_id` IS NOT NULL AND `server_type` IS NOT NULL AND `rate` IS NOT NULL GROUP BY `server_id`, `log_at`, `user_id`, `server_type`, `rate` HAVING COUNT(*) > 1);
DELETE FROM `v2_server_log` WHERE `id` IN (SELECT `id` FROM (SELECT `id`, ROW_NUMBER() OVER (PARTITION BY `server_id`, `log_at`, `user_id`, `server_type`, `rate` ORDER BY `id`) AS `rank_in_bucket` FROM `v2_server_log` WHERE `server_id` IS NOT NULL AND `log_at` IS NOT NULL AND `user_id` IS NOT NULL AND `server_type` IS NOT NULL AND `rate` IS NOT NULL) AS `ranked` WHERE `rank_in_bucket` > 1);
CREATE UNIQUE INDEX IF NOT EXISTS `idx_v2_server_log_bucket` ON `v2_server_log`(`server_id`,`log_at`,`user_id`,`server_type`,`rate`);
