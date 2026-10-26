-- 0011_subscription_group_protocols (PostgreSQL): v5_subscription_group_protocol,
-- each subscription group's display name and order of its node protocols
-- (model.SubscriptionGroupProtocol).
CREATE TABLE "v5_subscription_group_protocol" ("group_id" bigint,"node_protocol_id" bigint,"display_name" varchar(128) NOT NULL DEFAULT '',"sort" bigint NOT NULL DEFAULT 0,PRIMARY KEY ("group_id","node_protocol_id"));
