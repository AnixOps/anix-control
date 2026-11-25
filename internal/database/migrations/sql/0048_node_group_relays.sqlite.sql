-- 0048_node_group_relays (SQLite): the region relays and release caches of
-- a node group (docs/design/region-relay). Its nodes reach Control only
-- through the relays; the install command and the desired configuration
-- (control_route) carry them. JSON arrays; additive, empty means none.
ALTER TABLE `v5_node_group` ADD `relays` text;
ALTER TABLE `v5_node_group` ADD `release_caches` text;
