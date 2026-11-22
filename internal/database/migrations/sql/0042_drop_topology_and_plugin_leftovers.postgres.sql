-- 0042_drop_topology_and_plugin_leftovers (PostgreSQL): the tables and columns
-- of features the 2026-11 slimming audit removed, which nothing reads or
-- writes any more: the topology deployments and the Control package
-- lifecycle plans (PR 2b), with the links to them on v3_kernel_operation,
-- the plugin WebUI assets (2c) and the remote module runtime's
-- enrollments, certificates and runtime choices (2d). The kernel CA
-- (v4_kernel_service_ca) stays. Dependents first.
-- Irreversible: back the database up first.
DROP TABLE IF EXISTS "v3_kernel_topology_observed_state";
DROP TABLE IF EXISTS "v3_kernel_topology_deployment_step";
DROP TABLE IF EXISTS "v3_kernel_topology_deployment";
DROP TABLE IF EXISTS "v3_kernel_topology_edge";
DROP TABLE IF EXISTS "v3_kernel_topology_vertex";
DROP TABLE IF EXISTS "v3_kernel_topology_revision";
DROP TABLE IF EXISTS "v3_kernel_topology";
DROP TABLE IF EXISTS "v3_kernel_plugin_lifecycle_plan_step";
DROP TABLE IF EXISTS "v3_kernel_plugin_lifecycle_plan";
DROP TABLE IF EXISTS "v3_kernel_plugin_webui_asset";
DROP TABLE IF EXISTS "v4_kernel_module_certificate";
DROP TABLE IF EXISTS "v4_kernel_module_enrollment";
DROP TABLE IF EXISTS "v4_kernel_plugin_runtime";
ALTER TABLE "v3_kernel_operation" DROP COLUMN IF EXISTS "topology_deployment_id";
ALTER TABLE "v3_kernel_operation" DROP COLUMN IF EXISTS "topology_step_id";
ALTER TABLE "v3_kernel_operation" DROP COLUMN IF EXISTS "topology_revision";
ALTER TABLE "v3_kernel_operation" DROP COLUMN IF EXISTS "lifecycle_plan_id";
ALTER TABLE "v3_kernel_operation" DROP COLUMN IF EXISTS "lifecycle_plan_step_id";
ALTER TABLE "v3_kernel_operation" DROP COLUMN IF EXISTS "lifecycle_plan_phase";
ALTER TABLE "v3_kernel_operation" DROP COLUMN IF EXISTS "lifecycle_plan_sequence";
