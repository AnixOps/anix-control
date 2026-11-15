-- 0038_egress_route_fallbacks: the backup exits of an egress route
-- (v5_egress_route.fallbacks, a JSON array of model.EgressExit).
ALTER TABLE `v5_egress_route` ADD `fallbacks` text;
