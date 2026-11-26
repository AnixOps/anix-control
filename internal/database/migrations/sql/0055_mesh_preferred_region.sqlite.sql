-- 0055_mesh_preferred_region (SQLite): the preferred region of the
-- automatic region choice (internal/meshregion, docs/design/mesh-regions):
-- on an enrollment credential and as the panel-wide default in the mesh
-- settings. Additive; NULL means none.
ALTER TABLE `v5_mesh_enrollment` ADD `preferred_region_id` integer;
ALTER TABLE `v5_mesh_settings` ADD `preferred_region_id` integer;
