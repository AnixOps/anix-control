-- 0012_forward_route_subscriptions (SQLite): v5_forward_route_subscription,
-- the forward routes published in subscriptions at their entry nodes
-- (internal/routesubscription, model.ForwardRouteSubscription).
CREATE TABLE `v5_forward_route_subscription` (`route_id` text,`display_name` text NOT NULL DEFAULT "",`created_at` datetime,`updated_at` datetime,PRIMARY KEY (`route_id`));
