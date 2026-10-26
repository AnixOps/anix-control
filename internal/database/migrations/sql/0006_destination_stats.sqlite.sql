-- 0006_destination_stats (SQLite): v5_destination_stat, each user's traffic
-- by destination domain, node and UTC hour, from the Agents'
-- DestinationReport batches (destinations.v1, internal/destinationstats,
-- model.DestinationStat). The retention worker deletes the rows older than
-- the configured retention.
CREATE TABLE `v5_destination_stat` (`id` integer PRIMARY KEY AUTOINCREMENT,`hour_at` integer NOT NULL,`node_id` integer NOT NULL,`user_id` integer NOT NULL,`domain` text NOT NULL DEFAULT "",`protocol` text NOT NULL DEFAULT "",`upload` integer NOT NULL DEFAULT 0,`download` integer NOT NULL DEFAULT 0,`connections` integer NOT NULL DEFAULT 0);
CREATE INDEX `idx_v5_destination_stat_user` ON `v5_destination_stat`(`user_id`,`hour_at`);
CREATE UNIQUE INDEX `idx_v5_destination_stat_key` ON `v5_destination_stat`(`hour_at`,`node_id`,`user_id`,`domain`,`protocol`);
