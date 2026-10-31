-- 0019_group_node_overrides (PostgreSQL): v5_subscription_group_protocol.overrides,
-- the edits and order of the nodes a subscription group makes of a protocol
-- (model.SubscriptionGroupProtocol).
ALTER TABLE "v5_subscription_group_protocol" ADD "overrides" text NOT NULL DEFAULT '';
