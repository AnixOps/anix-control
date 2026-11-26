-- 0057_mesh_region_basis (SQLite): why a mesh member or device is in its
-- region (latency, credential, admin or migration), which the region object
-- of its configuration states (internal/meshregion, docs/design/mesh-regions).
-- Additive; NULL counts as admin.
ALTER TABLE `v5_mesh_member` ADD `region_basis` text;
ALTER TABLE `v5_mesh_device` ADD `region_basis` text;
