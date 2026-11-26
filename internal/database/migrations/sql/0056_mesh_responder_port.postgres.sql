-- 0056_mesh_responder_port (PostgreSQL): the UDP port of the border nodes'
-- region probe responders, a mesh setting (internal/meshregion,
-- docs/design/mesh-regions). Additive; NULL means the default, 11013.
ALTER TABLE "v5_mesh_settings" ADD "responder_port" bigint;
