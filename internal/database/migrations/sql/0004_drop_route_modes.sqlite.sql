-- 0004_drop_route_modes (SQLite): route modes are no longer stored or
-- switched (single-binary design, section 4.3). A route runs natively when
-- the rehearsed list (config/package-route-defaults.json) names it, identity
-- group A follows the identity authority, and every other route runs its
-- legacy handler.
--
-- The route-mode revision history and the shadow mismatch samples go with
-- the route-mode administration; every switch is also in the audit log.
-- The Go part of the step then removes the stored maps (the "routes" key of
-- each Control package's configuration) and writes each removed map to the
-- audit log (drop_route_modes.go).
DROP TABLE IF EXISTS `v4_kernel_route_mode_revision`;
DROP TABLE IF EXISTS `v4_kernel_shadow_mismatch_sample`;
