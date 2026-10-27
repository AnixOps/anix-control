-- 0012_forward_route_subscriptions (PostgreSQL): v5_forward_route_subscription,
-- the forward routes published in subscriptions at their entry nodes
-- (internal/routesubscription, model.ForwardRouteSubscription).
CREATE TABLE "v5_forward_route_subscription" ("route_id" varchar(64),"display_name" varchar(128) NOT NULL DEFAULT '',"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("route_id"));
