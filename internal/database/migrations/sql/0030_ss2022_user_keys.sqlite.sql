-- 0030_ss2022_user_keys (SQLite): v5_user_ss2022_key, each user's random
-- Shadowsocks 2022 key (model.UserSS2022Key), and the protocol revision each
-- Agent named in its Hello (v4_kernel_agent_transport.protocol_revision),
-- which decides whether a node's subscriptions carry the stored key.
CREATE TABLE `v5_user_ss2022_key` (`user_id` integer,`user_key` text NOT NULL,`created_at` datetime NOT NULL,PRIMARY KEY (`user_id`));
ALTER TABLE `v4_kernel_agent_transport` ADD `protocol_revision` integer NOT NULL DEFAULT 0;
