-- 0011_subscription_group_protocols (SQLite): v5_subscription_group_protocol,
-- each subscription group's display name and order of its node protocols
-- (model.SubscriptionGroupProtocol).
CREATE TABLE `v5_subscription_group_protocol` (`group_id` integer,`node_protocol_id` integer,`display_name` text NOT NULL DEFAULT "",`sort` integer NOT NULL DEFAULT 0,PRIMARY KEY (`group_id`,`node_protocol_id`));
